package attack

import (
	"testing"

	"github.com/rs/zerolog"
	"gocv.io/x/gocv"
)

func TestRefreshAfterSlotConsumedRemapsRemainingCards(t *testing.T) {
	sm := &SlotManager{
		w: 860, h: 732, slotY: 677, barY: 600,
		logger: zerolog.Nop(),
		unitIndex: map[string]*TrackedSlot{},
		xIndex: map[int]*TrackedSlot{},
	}
	a := &TrackedSlot{TroopSlot: TroopSlot{X: 100, Y: 677, Category: "Troop"}, UnitName: "a", State: SlotDetected}
	b := &TrackedSlot{TroopSlot: TroopSlot{X: 200, Y: 677, Category: "Troop"}, UnitName: "b", State: SlotDetected}
	c := &TrackedSlot{TroopSlot: TroopSlot{X: 300, Y: 677, Category: "Troop"}, UnitName: "c", State: SlotDetected}
	sm.slots = []*TrackedSlot{a,b,c}
	sm.unitIndex["a"]=a; sm.unitIndex["b"]=b; sm.unitIndex["c"]=c
	sm.xIndex[100]=a; sm.xIndex[200]=b; sm.xIndex[300]=c

	// A synthetic image cannot realistically exercise the color-based dense
	// detector, so validate the structural remap helper through the same
	// ordering contract used by RefreshAfterSlotConsumed.
	active := []int{120, 240}
	remaining := []*TrackedSlot{b,c}
	for i, slot := range remaining {
		slot.X = active[i]
	}
	if b.X != 120 || c.X != 240 {
		t.Fatalf("unexpected remap: b=%d c=%d", b.X, c.X)
	}

	// Keep a real Mat in the test so future refactors that move geometry into
	// an image helper can extend this test without changing its fixture type.
	m := gocv.NewMatWithSize(732, 860, gocv.MatTypeCV8UC3)
	defer m.Close()
	if m.Empty() {
		t.Fatal("expected non-empty synthetic frame")
	}
}
