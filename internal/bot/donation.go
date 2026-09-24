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
	if b.donationInFlight.Load() {
		return true
	}
	if next := b.donationNextCheck.Load(); next > 0 && time.Now().UnixNano() < next {
		return false
	}
	if time.Since(b.lastDonationScan) < 90*time.Second {
		return false
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
	b.donationChecks.Add(1)
	report := donationRunReport{Timestamp: time.Now()}
	defer func() {
		if data, err := json.MarshalIndent(report, "", "  "); err == nil {
			_ = AsyncWriteFile(paths.ResolveConfig("last_donation_report.json"), data, 0o600)
		}

		// Adaptive retry: no-request scans are naturally low priority, while
		// technical failures should retry sooner. This avoids opening chat every
		// 90 seconds forever when the clan is quiet.
		delay := 90 * time.Second
		result := "check complete"
		switch {
		case len(report.Donated) > 0:
			delay = 90 * time.Second
			result = "donation verified"
		case report.SkippedReason == "no donation request found":
			delay = 3 * time.Minute
			result = "no requests"
		case report.SkippedReason == "request found but troop type was not recognized":
			delay = 60 * time.Second
			result = "request needs better recognition"
		case report.SkippedReason != "":
			delay = 30 * time.Second
			result = report.SkippedReason
		}
		b.donationNextCheck.Store(time.Now().Add(delay).UnixNano())
		b.statusMu.Lock()
		b.lastDonationResult = result
		b.statusMu.Unlock()
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
		b.returnToVillageAfterDonation(3)
		return
	}
	report.ChatOpened = true

	requests := b.findDonationRequests(chat)
	chat.Close()
	report.RequestButtons = len(requests)
	if len(requests) == 0 {
		report.SkippedReason = "no donation request found"
		b.returnToVillageAfterDonation(3)
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
		b.returnToVillageAfterDonation(3)
		return
	}
	report.Recognized = append(report.Recognized, req.Requested...)

	// Even relaxed mode never taps an unknown troop. The requested-only switch
	// tightens policy, but both modes still require positive visual identity.
	if b.cfg.Automation.Preferences.DonateOnlyRequested && len(req.Requested) == 0 {
		report.SkippedReason = "strict requested-only mode"
		b.returnToVillageAfterDonation(3)
		return
	}

	if err := b.client.TapFast(req.Button.X, req.Button.Y, 0.6); err != nil {
		report.SkippedReason = "could not open donation picker"
		b.returnToVillageAfterDonation(3)
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
		b.returnToVillageAfterDonation(3)
		return
	}

	// Donate multiple units when the same verified request still accepts them,
	// but keep a strict bounded ceiling. Every individual tap must produce a
	// local visual change before it is counted; a rejected/full request stops
	// immediately rather than hammering the button.
	const maxVerifiedDonationsPerCycle = 6
	verifiedThisCycle := 0
	for _, name := range req.Requested {
		for verifiedThisCycle < maxVerifiedDonationsPerCycle {
			pt, ok := b.findDonationTroopInPicker(picker, name)
			if !ok {
				break
			}

			before := picker.Clone()
			if err := b.client.TapFast(pt.X, pt.Y, 0.7); err != nil {
				before.Close()
				break
			}
			if !b.sleepResponsive(180 * time.Millisecond) {
				before.Close()
				picker.Close()
				return
			}

			after, capErr := b.client.CaptureToMat()
			if capErr != nil || after.Empty() {
				before.Close()
				if !after.Empty() { after.Close() }
				break
			}

			changed := donationVisualDelta(before, after, pt, int(34*b.cal.ScaleX)) >= 0.015
			before.Close()
			if !changed {
				after.Close()
				b.logger.Debug().Str("troop", name).Msg("donation tap produced no visible change; request may be full or troop unavailable")
				break
			}

			report.Donated = append(report.Donated, name)
			b.donationsSent.Add(1)
			b.lastDonationUnix.Store(time.Now().Unix())
			verifiedThisCycle++
			b.logger.Info().
				Str("troop", name).
				Int("verified_cycle", verifiedThisCycle).
				Msg("clan donation visually confirmed")

			// Continue from the fresh verified frame. If the picker auto-closed
			// after filling the request, the next template lookup simply fails.
			picker.Close()
			picker = after
		}
		if verifiedThisCycle >= maxVerifiedDonationsPerCycle {
			break
		}
	}
	picker.Close()

	if len(report.Donated) == 0 && report.SkippedReason == "" {
		report.SkippedReason = "no recognized requested troop was available to donate"
	}

	// Return to the village with proof after every Back press. Never issue a
	// fixed two-Back sequence: if the donation picker auto-closes after a
	// successful donation, the second blind Back would hit the village and open
	// Clash's quit-confirm dialog.
	b.returnToVillageAfterDonation(3)
	b.recordActivity()
}

func (b *Bot) returnToVillageAfterDonation(maxBacks int) bool {
	return b.returnToVillageVerified(maxBacks, "donation cleanup")
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
	names := b.supportedDonationTemplateNames()
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


func (b *Bot) supportedDonationTemplateNames() []string {
	if b.templates == nil {
		return nil
	}

	// Reuse every compatible unit template already shipped in the EXE. New
	// troop templates automatically become donation-capable without another
	// code change. Heroes/spells are excluded; siege machines are valid clan
	// reinforcements and may be recognized when a template exists.
	excluded := map[string]bool{
		"archer_queen": true,
		"barbarian_king": true,
		"grand_warden": true,
		"minion_prince": true,
		"dragon_duke": true,
		"ice_spell": true,
		"rage_spell": true,
	}

	seen := map[string]bool{}
	var out []string
	for _, full := range b.templates.List(game.StateUnknown) {
		if !strings.HasPrefix(full, "attack/") {
			continue
		}
		name := strings.TrimPrefix(full, "attack/")
		if name == "" || excluded[name] || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
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
	return localVisualDelta(before, after, pt, radius, radius)
}
