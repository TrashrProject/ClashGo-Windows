package licensing

import (
	"net/http"
	"testing"
)

func TestStateFromStoredKeepsMemberMetadata(t *testing.T) {
	s := &Service{}
	st := storedLicense{
		Key:           "CGO-ABCDEF-GHIJKL-MNOPQR-STUVWX",
		Role:          RoleMember,
		MemberName:    "Nathan",
		MachineID:     "machine-hash",
		LastValidated: "2026-09-29T04:00:00Z",
		OfflineUntil:  "2026-10-02T04:00:00Z",
		Plan:          "month_1",
		ExpiresAt:     "2026-10-29T04:00:00Z",
	}

	got := s.stateFromStored(st)
	if !got.Activated {
		t.Fatal("expected stored key to restore an activated state")
	}
	if got.MemberName != "Nathan" {
		t.Fatalf("MemberName = %q, want Nathan", got.MemberName)
	}
	if got.LicenseHint != "••••-UVWX" {
		t.Fatalf("LicenseHint = %q", got.LicenseHint)
	}
	if got.Plan != "month_1" || got.ExpiresAt != st.ExpiresAt {
		t.Fatalf("plan/expiry metadata not restored: %+v", got)
	}
}

func TestRemoteErrorDefinitive(t *testing.T) {
	for _, status := range []int{
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusConflict,
		http.StatusNotFound,
	} {
		if !(&RemoteError{Status: status}).Definitive() {
			t.Fatalf("status %d should be definitive", status)
		}
	}
	if (&RemoteError{Status: http.StatusServiceUnavailable}).Definitive() {
		t.Fatal("service unavailable must allow offline-grace handling")
	}
}

func TestLicenseHintDoesNotExposeFullKey(t *testing.T) {
	key := "CGO-ABCDEF-GHIJKL-MNOPQR-STUVWX"
	hint := licenseHint(key)
	if hint != "••••-UVWX" {
		t.Fatalf("unexpected hint %q", hint)
	}
	if hint == key {
		t.Fatal("license hint exposed the full key")
	}
}
