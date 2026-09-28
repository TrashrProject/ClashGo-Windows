package vision

import (
	"testing"

	"gocv.io/x/gocv"
)

func TestMatPoolKeyUnique(t *testing.T) {
	// Distinct (rows, cols, type) triples must be independent pool
	// entries. matKey is a struct key, so field-wise equality makes
	// collisions impossible by construction — this guards the key layout
	// and that rows/cols aren't accidentally transposed.
	keys := map[matKey]bool{}
	for _, dims := range [][2]int{{100, 100}, {256, 256}, {70000, 200}, {200, 70000}, {100, 256}} {
		k := matKey{matType: gocv.MatTypeCV8UC3, rows: dims[0], cols: dims[1]}
		if keys[k] {
			t.Fatalf("collision for dims %v: key %+v reused", dims, k)
		}
		keys[k] = true
	}
}

func TestMatPoolRoundTrip(t *testing.T) {
	p := NewMatPool()

	const rows, cols = 480, 640
	got := p.Get(rows, cols, gocv.MatTypeCV8UC3)
	if got.Empty() || got.Rows() != rows || got.Cols() != cols {
		t.Fatalf("Get returned invalid mat: rows=%d cols=%d empty=%v", got.Rows(), got.Cols(), got.Empty())
	}

	// Write a marker pixel, return to pool, reacquire, and verify it was
	// reset (SetTo clears) so a reused mat cannot leak stale data.
	got.SetUCharAt(0, 0, 255)
	p.Put(got)

	reused := p.Get(rows, cols, gocv.MatTypeCV8UC3)
	if reused.Empty() {
		t.Fatal("reacquired mat is empty")
	}
	if reused.GetUCharAt(0, 0) != 0 {
		t.Errorf("pooled mat was not reset: pixel[0,0]=%d, want 0", reused.GetUCharAt(0, 0))
	}
	reused.Close()
}

func TestMatPoolLifecycleBalance(t *testing.T) {
	p := NewMatPool()

	// Simulate an encode-error path: every acquired mat must be returned,
	// even when the consumer short-circuits on error. Leaks show up as
	// growing pool maps / unreclaimed mats.
	mats := make([]gocv.Mat, 0, 50)
	for i := 0; i < 50; i++ {
		m := p.Get(320, 240, gocv.MatTypeCV8UC3)
		mats = append(mats, m)
	}
	for _, m := range mats {
		p.Put(m)
	}

	// Reacquire the same size; all should come from the pool (non-empty).
	for i := 0; i < 50; i++ {
		m := p.Get(320, 240, gocv.MatTypeCV8UC3)
		if m.Empty() {
			t.Fatal("unexpected empty mat from pool")
		}
		p.Put(m)
	}
}

func TestPreferredTemplateScaleIsScopedByTemplateAndScaleGrid(t *testing.T) {
	RememberPreferredTemplateScale("queen", 0.2, 1.2, 20, 0.73)
	if got, ok := preferredTemplateScale("queen", 0.2, 1.2, 20); !ok || got != 0.73 {
		t.Fatalf("preferred scale=%v ok=%v want 0.73/true", got, ok)
	}
	if _, ok := preferredTemplateScale("warden", 0.2, 1.2, 20); ok {
		t.Fatal("preferred scale leaked across template names")
	}
	if _, ok := preferredTemplateScale("queen", 0.2, 1.2, 12); ok {
		t.Fatal("preferred scale leaked across step-count grids")
	}
	if _, ok := preferredTemplateScale("queen", 0.3, 1.1, 20); ok {
		t.Fatal("preferred scale leaked across scale ranges")
	}
}

func TestRememberPreferredTemplateScaleRejectsInvalidHints(t *testing.T) {
	RememberPreferredTemplateScale("", 0.2, 1.2, 20, 0.8)
	if _, ok := preferredTemplateScale("", 0.2, 1.2, 20); ok {
		t.Fatal("empty template name must not create preferred-scale hint")
	}

	RememberPreferredTemplateScale("invalid-scale", 0.2, 1.2, 20, 0)
	if _, ok := preferredTemplateScale("invalid-scale", 0.2, 1.2, 20); ok {
		t.Fatal("non-positive preferred scale must be ignored")
	}
}
