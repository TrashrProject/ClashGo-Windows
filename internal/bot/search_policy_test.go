package bot

import "testing"

func TestShouldForceAttackAfterSkips(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		skips int
		want  bool
	}{
		{name: "disabled", limit: 0, skips: 5000, want: false},
		{name: "below limit", limit: 999, skips: 998, want: false},
		{name: "at limit", limit: 999, skips: 999, want: true},
		{name: "above limit", limit: 10, skips: 11, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldForceAttackAfterSkips(tc.limit, tc.skips); got != tc.want {
				t.Fatalf("shouldForceAttackAfterSkips(%d, %d)=%v want %v", tc.limit, tc.skips, got, tc.want)
			}
		})
	}
}
