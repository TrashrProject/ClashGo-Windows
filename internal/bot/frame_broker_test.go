package bot

import (
	"context"
	"testing"
	"time"

	"gocv.io/x/gocv"
)

func TestFrameBrokerPublishesAndSnapshotsClone(t *testing.T) {
	b := NewFrameBroker()
	defer b.Close()

	src := gocv.NewMatWithSize(8, 8, gocv.MatTypeCV8UC3)
	defer src.Close()

	now := time.Now()
	b.Publish(src, now)

	got, at, seq, ok := b.Snapshot(time.Second)
	if !ok {
		t.Fatal("expected broker snapshot")
	}
	defer got.Close()
	if seq == 0 {
		t.Fatal("expected non-zero sequence")
	}
	if !at.Equal(now) {
		t.Fatalf("timestamp=%v, want %v", at, now)
	}
	if got.Empty() || got.Rows() != 8 || got.Cols() != 8 {
		t.Fatalf("invalid cloned frame %dx%d", got.Cols(), got.Rows())
	}
}

func TestFrameBrokerWaitAfterRequiresNewerFrame(t *testing.T) {
	b := NewFrameBroker()
	defer b.Close()

	first := gocv.NewMatWithSize(4, 4, gocv.MatTypeCV8UC3)
	defer first.Close()
	b.Publish(first, time.Now())
	_, _, seq, ok := b.Snapshot(time.Second)
	if !ok {
		t.Fatal("missing first frame")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan uint64, 1)
	go func() {
		m, _, next, err := b.WaitAfter(ctx, seq, time.Second, time.Second)
		if err == nil {
			m.Close()
			result <- next
			return
		}
		result <- 0
	}()

	time.Sleep(20 * time.Millisecond)
	second := gocv.NewMatWithSize(5, 5, gocv.MatTypeCV8UC3)
	defer second.Close()
	b.Publish(second, time.Now())

	select {
	case next := <-result:
		if next <= seq {
			t.Fatalf("next seq=%d, want > %d", next, seq)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("broker waiter did not receive newer frame")
	}
}
