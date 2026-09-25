package attack

import "testing"

func TestIsDeploymentRedRGB(t *testing.T) {
	tests := []struct {
		name string
		r, g, b int
		want bool
	}{
		{name: "strong red overlay", r: 190, g: 90, b: 80, want: true},
		{name: "translucent red overlay", r: 135, g: 95, b: 90, want: true},
		{name: "orange button", r: 230, g: 170, b: 45, want: false},
		{name: "green terrain", r: 90, g: 135, b: 70, want: false},
		{name: "neutral gray", r: 145, g: 140, b: 135, want: false},
		{name: "dark red too weak", r: 90, g: 35, b: 30, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDeploymentRedRGB(tc.r, tc.g, tc.b); got != tc.want {
				t.Fatalf("isDeploymentRedRGB(%d,%d,%d)=%v want %v", tc.r, tc.g, tc.b, got, tc.want)
			}
		})
	}
}
