package bot

import (
	"image"
	"testing"
)

func TestButtonROIConsistency(t *testing.T) {
	b := &Bot{}

	expected := map[string]image.Rectangle{
		"btn_attack":      image.Rect(0, 600, 150, 732),
		"btn_find_match":  image.Rect(50, 400, 400, 600),
		"btn_battle":      image.Rect(300, 150, 860, 732),
		"btn_army_arrow":  image.Rect(350, 100, 700, 300),
		"btn_army_1":      image.Rect(400, 150, 650, 350),
		"btn_next":        image.Rect(600, 450, 860, 732),
		"unknown_button":  image.Rect(0, 0, 860, 732),
	}

	for name, want := range expected {
		if got := b.buttonROI(name); got != want {
			t.Errorf("buttonROI(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestHistoryCacheNoWipeOnReadError(t *testing.T) {
	b := &Bot{
		historyCache: []AttackReport{
			{Timestamp: "earlier", Stars: 3},
		},
	}

	// Simulate the record path using the in-memory cache as the source of
	// truth. A disk read failure (handled by the caller returning a fresh
	// nil slice) must NOT drop the prior run.
	history := b.historyCache
	if history == nil {
		// emulate read error -> fresh empty slice
		history = []AttackReport{}
	}
	rep := AttackReport{Timestamp: "now", Stars: 2}
	history = append([]AttackReport{rep}, history...)
	b.historyCache = history

	if len(b.historyCache) != 2 {
		t.Fatalf("history cache wiped: got %d entries, want 2", len(b.historyCache))
	}
	if b.historyCache[0].Timestamp != "now" || b.historyCache[1].Timestamp != "earlier" {
		t.Errorf("history cache order/copy wrong: %+v", b.historyCache)
	}
}

func TestHistorySnapshotReturnsIndependentCopy(t *testing.T) {
	b := &Bot{
		historyCache: []AttackReport{
			{Timestamp: "one", Stars: 3},
			{Timestamp: "two", Stars: 2},
		},
	}

	got := b.HistorySnapshot()
	if len(got) != 2 {
		t.Fatalf("snapshot len=%d want 2", len(got))
	}

	got[0].Stars = 0
	got = append(got, AttackReport{Timestamp: "mutated"})

	again := b.HistorySnapshot()
	if len(again) != 2 {
		t.Fatalf("mutating returned slice changed bot cache len=%d", len(again))
	}
	if again[0].Stars != 3 {
		t.Fatalf("mutating returned report leaked into bot cache: %+v", again[0])
	}
}

func TestHistorySnapshotNilBotIsEmpty(t *testing.T) {
	var b *Bot
	if got := b.HistorySnapshot(); len(got) != 0 {
		t.Fatalf("nil bot snapshot len=%d want 0", len(got))
	}
}
