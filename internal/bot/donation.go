package bot

import (
	"encoding/json"
	"image"
	"sort"
	"strings"
	"time"

	"github.com/Ducky705/ClashGO/internal/game"
	"github.com/Ducky705/ClashGO/internal/paths"
	"github.com/Ducky705/ClashGO/internal/vision"
	"gocv.io/x/gocv"
)

type donationRequest struct {
	Button    image.Point
	Requested []string
}

type donationRunReport struct {
	Timestamp      time.Time
	ChatOpened     bool
	RequestButtons int
	Recognized     []string
	Donated        []string
	SkippedReason  string
}

// maybeStartDonationCycle opportunistically handles donations while the bot is
// safely idle on the main village. It is deliberately low-frequency and never
// overlaps an attack sequence.
func (b *Bot) maybeStartDonationCycle(state gocv.Mat) bool {
	if !b.cfg.Automation.Preferences.AutoDonate || b.seqRunning.Load() {
		return false
	}
	if b.donationInFlight.Load() || time.Since(b.lastDonationScan) < 90*time.Second {
		return b.donationInFlight.Load()
	}

	// Only touch the chat handle from a positively-verified village frame.
	if state.Empty() || !b.findAttackButton(state, 0.30) {
		return false
	}

	if !b.donationInFlight.CompareAndSwap(false, true) {
		return true
	}
	b.lastDonationScan = time.Now()
	go b.runDonationCycle()
	return true
}

func (b *Bot) runDonationCycle() {
	defer b.donationInFlight.Store(false)
	report := donationRunReport{Timestamp: time.Now()}
	defer func() {
		if data, err := json.MarshalIndent(report, "", "  "); err == nil {
			_ = AsyncWriteFile(paths.ResolveConfig("last_donation_report.json"), data, 0o600)
		}
	}()

	if b.ctx.Err() != nil || b.seqRunning.Load() {
		report.SkippedReason = "bot stopping or attack started"
		return
	}

	// The chat handle is fixed UI chrome on the left edge. A wrong/stale tap is
	// never trusted: the classifier must prove StateChatOpen before we continue.
	cx, cy := b.cal.ScaleRef(18, 350)
	if err := b.client.TapFast(cx, cy, 0.6); err != nil {
		report.SkippedReason = "chat tap failed"
		return
	}
	if !b.sleepResponsive(280 * time.Millisecond) {
		report.SkippedReason = "cancelled"
		return
	}

	chat, err := b.client.CaptureToMat()
	if err != nil || chat.Empty() {
		if !chat.Empty() { chat.Close() }
		report.SkippedReason = "chat capture failed"
		return
	}
	st, _ := b.classify(chat)
	if st != game.StateChatOpen {
		chat.Close()
		report.SkippedReason = "chat state not confirmed"
		_ = b.client.Back()
		return
	}
	report.ChatOpened = true

	requests := b.findDonationRequests(chat)
	chat.Close()
	report.RequestButtons = len(requests)
	if len(requests) == 0 {
		report.SkippedReason = "no donation request found"
		_ = b.client.Back()
		return
	}

	// Prefer the newest visible request (lowest on the chat panel) that has at
	// least one troop icon we can positively identify.
	sort.SliceStable(requests, func(i, j int) bool { return requests[i].Button.Y > requests[j].Button.Y })
	var req *donationRequest
	for i := range requests {
		if len(requests[i].Requested) > 0 {
			req = &requests[i]
			break
		}
	}
	if req == nil {
		report.SkippedReason = "request found but troop type was not recognized"
		_ = b.client.Back()
		return
	}
	report.Recognized = append(report.Recognized, req.Requested...)

	// Even relaxed mode never taps an unknown troop. The requested-only switch
	// tightens policy, but both modes still require positive visual identity.
	if b.cfg.Automation.Preferences.DonateOnlyRequested && len(req.Requested) == 0 {
		report.SkippedReason = "strict requested-only mode"
		_ = b.client.Back()
		return
	}

	if err := b.client.TapFast(req.Button.X, req.Button.Y, 0.6); err != nil {
		report.SkippedReason = "could not open donation picker"
		_ = b.client.Back()
		return
	}
	if !b.sleepResponsive(260 * time.Millisecond) {
		report.SkippedReason = "cancelled"
		return
	}

	picker, err := b.client.CaptureToMat()
	if err != nil || picker.Empty() {
		if !picker.Empty() { picker.Close() }
		report.SkippedReason = "donation picker capture failed"
		_ = b.client.Back()
		_ = b.client.Back()
		return
	}

	// Donate one verified unit per cycle. This makes the first live runs easy
	// to audit and prevents an imperfect template from emptying the user's army.
	for _, name := range req.Requested {
		pt, ok := b.findDonationTroopInPicker(picker, name)
		if !ok {
			continue
		}
		before := picker.Clone()
		if err := b.client.TapFast(pt.X, pt.Y, 0.7); err != nil {
			before.Close()
			continue
		}
		if !b.sleepResponsive(180 * time.Millisecond) {
			before.Close()
			picker.Close()
			return
		}

		after, capErr := b.client.CaptureToMat()
		if capErr == nil && !after.Empty() {
			changed := donationVisualDelta(before, after, pt, int(34*b.cal.ScaleX)) >= 0.015
			after.Close()
			before.Close()
			if changed {
				report.Donated = append(report.Donated, name)
				b.logger.Info().Str("troop", name).Msg("clan donation visually confirmed")
				break
			}
			b.logger.Debug().Str("troop", name).Msg("donation tap produced no visible change; not counting it")
		} else {
			before.Close()
			if !after.Empty() { after.Close() }
		}
	}
	picker.Close()

	if len(report.Donated) == 0 && report.SkippedReason == "" {
		report.SkippedReason = "no recognized requested troop was available to donate"
	}

	// Close picker (if still open) then chat. Back is bounded and the next
	// capture loop frame will re-classify; no blind chain of additional taps.
	_ = b.client.Back()
	b.sleepResponsive(120 * time.Millisecond)
	_ = b.client.Back()
	b.recordActivity()
}

func (b *Bot) findDonationRequests(screen gocv.Mat) []donationRequest {
	if screen.Empty() {
		return nil
	}
	x0, y0 := b.cal.ScaleRef(0, 105)
	x1, y1 := b.cal.ScaleRef(500, 700)
	x0 = maxBotInt(0, x0); y0 = maxBotInt(0, y0)
	x1 = minBotInt(screen.Cols(), x1); y1 = minBotInt(screen.Rows(), y1)
	if x1-x0 < 30 || y1-y0 < 30 {
		return nil
	}

	roi := screen.Region(image.Rect(x0, y0, x1, y1))
	defer roi.Close()
	mask := vision.GetMat(roi.Rows(), roi.Cols(), gocv.MatTypeCV8UC1)
	defer vision.PutMat(mask)
	gocv.InRangeWithScalar(roi, gocv.NewScalar(20, 105, 35, 0), gocv.NewScalar(190, 255, 210, 0), &mask)
	contours := gocv.FindContours(mask, gocv.RetrievalExternal, gocv.ChainApproxSimple)
	defer contours.Close()

	var out []donationRequest
	for i := 0; i < contours.Size(); i++ {
		c := contours.At(i)
		area := gocv.ContourArea(c)
		if area < 650 {
			continue
		}
		r := gocv.BoundingRect(c)
		if r.Dx() < int(45*b.cal.ScaleX) || r.Dy() < int(20*b.cal.ScaleY) {
			continue
		}
		btn := image.Pt(x0+r.Min.X+r.Dx()/2, y0+r.Min.Y+r.Dy()/2)

		// Request icons live immediately above/left of the Donate button.
		card := image.Rect(
			maxBotInt(0, btn.X-int(330*b.cal.ScaleX)),
			maxBotInt(0, btn.Y-int(125*b.cal.ScaleY)),
			minBotInt(screen.Cols(), btn.X+int(60*b.cal.ScaleX)),
			minBotInt(screen.Rows(), btn.Y+int(35*b.cal.ScaleY)),
		)
		out = append(out, donationRequest{Button: btn, Requested: b.matchDonationTroops(screen, card)})
	}
	return out
}

func (b *Bot) matchDonationTroops(screen gocv.Mat, roi image.Rectangle) []string {
	names := []string{"electro_dragon", "balloon"}
	var found []string
	for _, name := range names {
		tpl, ok := b.templates.Get("attack/" + name)
		if !ok {
			tpl, ok = b.templates.Get(name)
		}
		if !ok || tpl.Empty() {
			continue
		}
		matches, err := vision.MatchMultiScaleROICached(screen, tpl, "donation_req_"+name, 0.45, 1.55, 8, 0.58, roi)
		if err == nil && len(matches) > 0 {
			found = append(found, strings.ReplaceAll(name, "_", " "))
		}
	}
	return found
}

func (b *Bot) findDonationTroopInPicker(screen gocv.Mat, name string) (image.Point, bool) {
	key := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), " ", "_")
	tpl, ok := b.templates.Get("attack/" + key)
	if !ok {
		tpl, ok = b.templates.Get(key)
	}
	if !ok || tpl.Empty() {
		return image.Point{}, false
	}

	// Donation picker occupies the central/right content area after pressing
	// Donate; exclude the far-left chat chrome and top global HUD.
	x0, y0 := b.cal.ScaleRef(180, 120)
	x1, y1 := b.cal.ScaleRef(850, 680)
	roi := image.Rect(maxBotInt(0,x0), maxBotInt(0,y0), minBotInt(screen.Cols(),x1), minBotInt(screen.Rows(),y1))
	matches, err := vision.MatchMultiScaleROICached(screen, tpl, "donation_picker_"+key, 0.40, 1.75, 8, 0.60, roi)
	if err != nil || len(matches) == 0 {
		return image.Point{}, false
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Confidence > matches[j].Confidence })
	return matches[0].Point, true
}

func donationVisualDelta(before, after gocv.Mat, pt image.Point, radius int) float64 {
	if before.Empty() || after.Empty() {
		return 0
	}
	if radius < 8 { radius = 8 }
	x0 := maxBotInt(0, pt.X-radius)
	y0 := maxBotInt(0, pt.Y-radius)
	x1 := minBotInt(minBotInt(before.Cols(), after.Cols())-1, pt.X+radius)
	y1 := minBotInt(minBotInt(before.Rows(), after.Rows())-1, pt.Y+radius)
	if x1 <= x0 || y1 <= y0 { return 0 }

	var sum float64
	var n int
	for y := y0; y <= y1; y += 2 {
		for x := x0; x <= x1; x += 2 {
			for c := 0; c < 3; c++ {
				d := int(before.GetUCharAt(y, x*3+c)) - int(after.GetUCharAt(y, x*3+c))
				if d < 0 { d = -d }
				sum += float64(d)
				n++
			}
		}
	}
	if n == 0 { return 0 }
	return sum / (255 * float64(n))
}

func minBotInt(a,b int) int { if a < b { return a }; return b }
func maxBotInt(a,b int) int { if a > b { return a }; return b }
