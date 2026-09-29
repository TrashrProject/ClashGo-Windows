package main

import "testing"

func TestNormalizePlayerTag(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"adds hash", "abc12", "#ABC12", false},
		{"keeps hash", "#abc12", "#ABC12", false},
		{"removes spaces", " ab c12 ", "#ABC12", false},
		{"minimum length", "A1B2C", "#A1B2C", false},
		{"maximum length", "ABCDEFGHIJKLMNO", "#ABCDEFGHIJKLMNO", false},
		{"too short", "A1B2", "", true},
		{"too long", "ABCDEFGHIJKLMNOP", "", true},
		{"invalid punctuation", "#ABC-12", "", true},
		{"empty", "   ", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizePlayerTag(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("normalizePlayerTag(%q) expected error, got %q", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizePlayerTag(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("normalizePlayerTag(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
