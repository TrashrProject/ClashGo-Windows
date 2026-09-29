package licensing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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


func TestValidateDoesNotExtendExpiredLicenseThroughOfflineGrace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"temporarily unavailable"}`))
	}))
	defer server.Close()

	now := time.Now().UTC()
	svc := &Service{
		baseURL:    server.URL,
		appVersion: "test",
		httpClient: server.Client(),
		stored: storedLicense{
			Key:          "CGO-ABCDEF-GHIJKL-MNOPQR-STUVWX",
			Role:         RoleMember,
			Plan:         "free_2d",
			ExpiresAt:    now.Add(-time.Minute).Format(time.RFC3339),
			OfflineUntil: now.Add(48 * time.Hour).Format(time.RFC3339),
		},
	}
	svc.state = svc.stateFromStored(svc.stored)

	state := svc.Validate(context.Background())
	if state.Activated {
		t.Fatalf("expired license stayed active through offline grace: %+v", state)
	}
	if state.Error != "license has expired" {
		t.Fatalf("Error = %q, want license has expired", state.Error)
	}
}

func TestValidateAllowsOfflineGraceBeforeExpiry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"message":"temporarily unavailable"}`))
	}))
	defer server.Close()

	now := time.Now().UTC()
	svc := &Service{
		baseURL:    server.URL,
		appVersion: "test",
		httpClient: server.Client(),
		stored: storedLicense{
			Key:           "CGO-ABCDEF-GHIJKL-MNOPQR-STUVWX",
			Role:          RoleMember,
			MemberName:    "Test",
			Plan:          "month_1",
			ExpiresAt:     now.Add(10 * 24 * time.Hour).Format(time.RFC3339),
			OfflineUntil:  now.Add(24 * time.Hour).Format(time.RFC3339),
			LastValidated: now.Add(-time.Hour).Format(time.RFC3339),
		},
	}
	svc.state = svc.stateFromStored(svc.stored)

	state := svc.Validate(context.Background())
	if !state.Activated {
		t.Fatalf("valid offline-grace license was deactivated: %+v", state)
	}
	if state.Error != "offline grace period" {
		t.Fatalf("Error = %q, want offline grace period", state.Error)
	}
	if state.MemberName != "Test" {
		t.Fatalf("member metadata lost during offline grace: %+v", state)
	}
}
