package attack

import (
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"gocv.io/x/gocv"
)

// newTestSlotManager builds a SlotManager with the given pre-assigned
// slots (unit names already resolved, e.g. by template matching) and no
// filesystem dependency — enough to exercise applyManualLabels.
func newTestSlotManager(slots []*TrackedSlot) *SlotManager {
	sm := &SlotManager{
		slots:     slots,
		unitIndex: make(map[string]*TrackedSlot),
		xIndex:    make(map[int]*TrackedSlot),
		logger:    zerolog.Nop(),
	}
	for _, s := range slots {
		sm.xIndex[s.X] = s
		if s.UnitName != "" {
			sm.unitIndex[strings.ToLower(s.UnitName)] = s
		}
	}
	return sm
}

// TestApplyManualLabels_SkipsDuplicateName guards against the live bug
// seen with valk_spam: manual_labels.json (calibrated on an old army)
// labels the unidentified Dragon Duke card as "Grand Warden" while the
// REAL warden slot was already identified by template matching. Without
// the dedup, "grand warden" resolves to the wrong card, the real hero
// never gets its main-phase deploy, and the mislabeled card is never
// swept (it looks deployed).
func TestApplyManualLabels_SkipsDuplicateName(t *testing.T) {
	// x=360 already identified as the warden via template match;
	// x=433 is an unidentified card (the Duke) that the stale manual
	// labels want to call "Grand Warden".
	warden := &TrackedSlot{TroopSlot: TroopSlot{X: 360, Y: 682}, UnitName: "grand warden", Confidence: 0.94}
	duke := &TrackedSlot{TroopSlot: TroopSlot{X: 433, Y: 682}}
	sm := newTestSlotManager([]*TrackedSlot{warden, duke})

	sm.applyManualLabels([]byte(`{
	  "slots": [
	    {"x": 62,  "name": "Valkyrie"},
	    {"x": 360, "name": "Dragon Duke"},
	    {"x": 433, "name": "Grand Warden"}
	  ]
	}`))

	if duke.UnitName != "" {
		t.Fatalf("stale fallback label duplicated a template-assigned name: duke slot got %q, want empty", duke.UnitName)
	}
	if duke.FallbackLabeled {
		t.Fatal("duplicate fallback should not have marked the slot fallback-labeled")
	}
	if warden.UnitName != "grand warden" {
		t.Fatalf("template-assigned warden was clobbered: got %q", warden.UnitName)
	}
}

// TestApplyManualLabels_FillsUnidentified ensures the fallback still
// labels genuinely unidentified slots (the valkyrie card has no
// template, so it must come from manual_labels.json).
func TestApplyManualLabels_FillsUnidentified(t *testing.T) {
	valk := &TrackedSlot{TroopSlot: TroopSlot{X: 62, Y: 682}}
	queen := &TrackedSlot{TroopSlot: TroopSlot{X: 212, Y: 682}, UnitName: "archer queen", Confidence: 0.95}
	sm := newTestSlotManager([]*TrackedSlot{valk, queen})

	sm.applyManualLabels([]byte(`{
	  "slots": [
	    {"x": 62,  "name": "Valkyrie"},
	    {"x": 134, "name": "Siege Machine"},
	    {"x": 212, "name": "Archer Queen"},
	    {"x": 433, "name": "Grand Warden"}
	  ]
	}`))

	if valk.UnitName != "valkyrie" {
		t.Fatalf("unidentified valkyrie slot not labeled: got %q, want %q", valk.UnitName, "valkyrie")
	}
	if !valk.FallbackLabeled {
		t.Fatal("fallback-labeled slot should carry FallbackLabeled=true so the sweep dumps it as an event troop")
	}
	if queen.UnitName != "archer queen" {
		t.Fatalf("template-assigned queen clobbered: got %q", queen.UnitName)
	}
}

// TestApplyManualLabels_IgnoresEmptyLabels ensures "Empty" entries and
// missing coordinates don't label anything.
func TestApplyManualLabels_IgnoresEmptyLabels(t *testing.T) {
	slot := &TrackedSlot{TroopSlot: TroopSlot{X: 730, Y: 682}}
	sm := newTestSlotManager([]*TrackedSlot{slot})

	sm.applyManualLabels([]byte(`{
	  "slots": [
	    {"x": 730, "name": "Empty"},
	    {"x": 999, "name": "Event Troop"}
	  ]
	}`))

	if slot.UnitName != "" {
		t.Fatalf("Empty/manual-only slot got labeled: %q", slot.UnitName)
	}
}

// TestApplyManualLabels_InvalidJSON silently no-ops like the production path.
func TestApplyManualLabels_InvalidJSON(t *testing.T) {
	slot := &TrackedSlot{TroopSlot: TroopSlot{X: 62, Y: 682}}
	sm := newTestSlotManager([]*TrackedSlot{slot})

	sm.applyManualLabels([]byte(`not json`))
	if slot.UnitName != "" {
		t.Fatalf("invalid config labeled a slot: %q", slot.UnitName)
	}
}


func TestLooksLikeHeroCardStaticGreenHealthBar(t *testing.T) {
	screen := gocv.NewMatWithSize(732, 860, gocv.MatTypeCV8UC3)
	defer screen.Close()

	// Synthetic green health strip inside the structural hero ROI.
	for y := 604; y < 616; y++ {
		for x := 280; x < 320; x++ {
			screen.SetUCharAt(y, x*3+0, 30)
			screen.SetUCharAt(y, x*3+1, 220)
			screen.SetUCharAt(y, x*3+2, 40)
		}
	}

	if !looksLikeHeroCardStatic(screen, 300, 600, 860, 732) {
		t.Fatal("expected green hero health strip to classify as hero")
	}

	if looksLikeHeroCardStatic(screen, 500, 600, 860, 732) {
		t.Fatal("empty generic card region should not classify as hero")
	}
}

func TestWindowsSlotActivityProfileMatchesLegacyWindowMath(t *testing.T) {
	const (
		w = 240
		h = 140
		slotY = 95
	)
	screen := gocv.NewMatWithSize(h, w, gocv.MatTypeCV8UC3)
	defer screen.Close()

	// Dark/map-like background.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			screen.SetVecbAt(y, x, gocv.Vecb{20, 45, 20})
		}
	}
	// Add a vivid card-like patch around x=120.
	for y := 72; y < 118; y++ {
		for x := 98; x < 142; x++ {
			screen.SetVecbAt(y, x, gocv.Vecb{40, 70, 220})
		}
	}

	profile := newWindowsSlotActivityProfile(screen, slotY, w)
	if profile == nil {
		t.Fatal("expected activity profile")
	}
	defer profile.Close()

	for _, x := range []int{40, 92, 120, 148, 200} {
		legacy := GetSlotActivityRatioStatic(screen, x, slotY, w)
		fast := profile.ActivityAt(x)
		diff := legacy - fast
		if diff < 0 { diff = -diff }
		if diff > 0.000001 {
			t.Fatalf("x=%d legacy=%f fast=%f diff=%f", x, legacy, fast, diff)
		}
	}
}

func TestWindowsSlotActivityProfilePreservesActiveThreshold(t *testing.T) {
	screen := gocv.NewMatWithSize(140, 240, gocv.MatTypeCV8UC3)
	defer screen.Close()

	for y := 0; y < 140; y++ {
		for x := 0; x < 240; x++ {
			screen.SetVecbAt(y, x, gocv.Vecb{20, 45, 20})
		}
	}
	for y := 76; y < 114; y++ {
		for x := 104; x < 136; x++ {
			screen.SetVecbAt(y, x, gocv.Vecb{30, 50, 230})
		}
	}

	profile := newWindowsSlotActivityProfile(screen, 95, 240)
	if profile == nil {
		t.Fatal("expected activity profile")
	}
	defer profile.Close()

	if legacy, fast := GetSlotActivityRatioStatic(screen, 120, 95, 240), profile.ActivityAt(120); (legacy >= 0.085) != (fast >= 0.085) {
		t.Fatalf("active threshold changed: legacy=%f fast=%f", legacy, fast)
	}
}

func TestWindowsLiveRescanTemplateFiltersNormalTroops(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"electro_dragon", false},
		{"balloon", false},
		{"barbarian_king", true},
		{"archer_queen", true},
		{"rage_spell", true},
		{"stone_slammer", true},
		{"clan_castle", true},
		{"cc", true},
	}
	for _, tc := range cases {
		if got := windowsLiveRescanTemplate(tc.name); got != tc.want {
			t.Fatalf("windowsLiveRescanTemplate(%q)=%v want %v", tc.name, got, tc.want)
		}
	}
}

func TestWindowsSlotActivityProfilePrefixMatchesWindowRatio(t *testing.T) {
	// Five columns over four rows with non-zero counts:
	// [0, 2, 4, 2, 0]. Prefix = [0, 0, 2, 6, 8, 8].
	p := &windowsSlotActivityProfile{
		prefix: []int{0, 0, 2, 6, 8, 8},
		rows:   4,
		cols:   5,
		size:   1,
	}
	if got := p.ActivityAt(2); got != 0.75 {
		t.Fatalf("ActivityAt(2)=%.3f want 0.750", got)
	}
	// Edge clamp: x=0 -> columns [0,1), all zero.
	if got := p.ActivityAt(0); got != 0 {
		t.Fatalf("ActivityAt(0)=%.3f want 0", got)
	}
	// x=4 -> columns [3,5): 2 / 8 = 0.25.
	if got := p.ActivityAt(4); got != 0.25 {
		t.Fatalf("ActivityAt(4)=%.3f want 0.250", got)
	}
}

func TestWindowsSlotActivityProfileInvalidIsZero(t *testing.T) {
	var p *windowsSlotActivityProfile
	if got := p.ActivityAt(100); got != 0 {
		t.Fatalf("nil profile activity=%.3f want 0", got)
	}
}
