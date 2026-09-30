package bot

import (
	"testing"
	"time"

	"gocv.io/x/gocv"
)

type wallCaptureTestClient struct {
	captures int
}

func (c *wallCaptureTestClient) Tap(x, y int) error { return nil }
func (c *wallCaptureTestClient) TapRandomized(x, y int) error { return nil }
func (c *wallCaptureTestClient) Swipe(x1, y1, x2, y2 int, ms int) error { return nil }
func (c *wallCaptureTestClient) Back() error { return nil }
func (c *wallCaptureTestClient) CaptureToMat() (gocv.Mat, error) {
	c.captures++
	return gocv.NewMatWithSize(4, 4, gocv.MatTypeCV8UC3), nil
}

func TestCaptureWallFramePrefersBrokerProvider(t *testing.T) {
	client := &wallCaptureTestClient{}
	providerCalls := 0
	h := &WallUpgradeHooks{
		Client: client,
		Capture: func(timeout time.Duration) (gocv.Mat, error) {
			providerCalls++
			return gocv.NewMatWithSize(5, 5, gocv.MatTypeCV8UC3), nil
		},
	}

	frame, err := captureWallFrame(h, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer frame.Close()

	if providerCalls != 1 {
		t.Fatalf("provider calls=%d want 1", providerCalls)
	}
	if client.captures != 0 {
		t.Fatalf("direct ADB captures=%d want 0 when broker provider is wired", client.captures)
	}
	if frame.Rows() != 5 || frame.Cols() != 5 {
		t.Fatalf("provider frame=%dx%d want 5x5", frame.Cols(), frame.Rows())
	}
}

func TestCaptureWallFrameDirectFallbackOnlyWithoutProvider(t *testing.T) {
	client := &wallCaptureTestClient{}
	h := &WallUpgradeHooks{Client: client}

	frame, err := captureWallFrame(h, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer frame.Close()

	if client.captures != 1 {
		t.Fatalf("direct fallback captures=%d want 1", client.captures)
	}
}
