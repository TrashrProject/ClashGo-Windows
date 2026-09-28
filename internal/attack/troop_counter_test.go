package attack

import (
	"testing"

	"gocv.io/x/gocv"
)

func TestExtractDigitsReturnsUsableROIsInLeftToRightOrder(t *testing.T) {
	tc := &TroopCounter{}
	bin := gocv.NewMatWithSize(30, 80, gocv.MatTypeCV8UC1)
	defer bin.Close()

	// Two white digit-like blobs with deliberately reversed creation order.
	for y := 8; y < 24; y++ {
		for x := 45; x < 53; x++ {
			bin.SetUCharAt(y, x, 255)
		}
	}
	for y := 7; y < 23; y++ {
		for x := 12; x < 19; x++ {
			bin.SetUCharAt(y, x, 255)
		}
	}

	digits := tc.extractDigits(bin, 8, 16)
	defer func() {
		for _, d := range digits {
			d.Close()
		}
	}()

	if len(digits) != 2 {
		t.Fatalf("digits=%d want 2", len(digits))
	}
	if digits[0].Cols() > digits[1].Cols()+3 || digits[0].Rows() < 10 || digits[1].Rows() < 10 {
		t.Fatalf("unexpected ROI dimensions: first=%dx%d second=%dx%d",
			digits[0].Cols(), digits[0].Rows(), digits[1].Cols(), digits[1].Rows())
	}
	// Parent must still be valid while ROI headers are consumed.
	if gocv.CountNonZero(digits[0]) == 0 || gocv.CountNonZero(digits[1]) == 0 {
		t.Fatal("digit ROI lost parent pixels")
	}
}
