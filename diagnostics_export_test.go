package main

import (
	"strings"
	"testing"
)

func TestRedactDiagnosticLogMasksClashGOLicenseKeys(t *testing.T) {
	input := []byte("activation failed key=CGO-ABCDEF-GHIJKL-MNOPQR-STUVWX user=nathan\nnormal log line")
	got := string(redactDiagnosticLog(input))

	if strings.Contains(got, "CGO-ABCDEF-GHIJKL-MNOPQR-STUVWX") {
		t.Fatalf("diagnostic export leaked full license key: %q", got)
	}
	if !strings.Contains(got, "CGO-[REDACTED]") {
		t.Fatalf("redacted marker missing: %q", got)
	}
	if !strings.Contains(got, "normal log line") {
		t.Fatalf("ordinary diagnostic content was altered: %q", got)
	}
}
