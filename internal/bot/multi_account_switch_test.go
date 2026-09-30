package bot

import (
	"testing"

	"gocv.io/x/gocv"
)

func gradientHashFrame(t *testing.T, reverse bool) gocv.Mat {
	t.Helper()
	m := gocv.NewMatWithSize(80, 90, gocv.MatTypeCV8UC1)
	for y := 0; y < m.Rows(); y++ {
		for x := 0; x < m.Cols(); x++ {
			v := uint8((x * 255) / maxInt(1, m.Cols()-1))
			if reverse {
				v = 255 - v
			}
			m.SetUCharAt(y, x, v)
		}
	}
	return m
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func TestAccountVisualHashStableForSameFrame(t *testing.T) {
	frame := gradientHashFrame(t, false)
	defer frame.Close()

	a, err := accountVisualHash(frame)
	if err != nil {
		t.Fatal(err)
	}
	b, err := accountVisualHash(frame)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("same frame produced unstable hashes: %016x != %016x", a, b)
	}
}

func TestAccountVisualHashDetectsLargeSceneChange(t *testing.T) {
	leftToRight := gradientHashFrame(t, false)
	defer leftToRight.Close()
	rightToLeft := gradientHashFrame(t, true)
	defer rightToLeft.Close()

	a, err := accountVisualHash(leftToRight)
	if err != nil {
		t.Fatal(err)
	}
	b, err := accountVisualHash(rightToLeft)
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatalf("opposite gradients produced identical visual hashes: %016x", a)
	}
}


func TestAccountSceneStableDistanceThreshold(t *testing.T) {
	base := uint64(0xAAAAAAAAAAAAAAAA)
	near := base ^ uint64(0b1111)
	far := base ^ uint64(0b111111111)

	if got := accountSceneStableDistance(base, near); got != 4 {
		t.Fatalf("near distance=%d want 4", got)
	}
	if got := accountSceneStableDistance(base, far); got != 9 {
		t.Fatalf("far distance=%d want 9", got)
	}
}
