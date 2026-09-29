package licensing

import (
	"encoding/json"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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


func TestStateFromStoredRejectsExpiredEntitlementImmediately(t *testing.T) {
	svc := &Service{}
	st := storedLicense{
		Key:          "CGO-ABCDEF-GHIJKL-MNOPQR-STUVWX",
		Role:         RoleMember,
		MemberName:   "Nathan",
		MachineID:    "machine-hash",
		Plan:         "month_1",
		ExpiresAt:    time.Now().UTC().Add(-time.Minute).Format(time.RFC3339),
		OfflineUntil: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
	}

	got := svc.stateFromStored(st)
	if got.Activated {
		t.Fatal("expired stored entitlement must never restore as active")
	}
	if got.Error != "license has expired" {
		t.Fatalf("unexpected expiry error %q", got.Error)
	}
}

func TestStateFromStoredKeepsFutureEntitlementActive(t *testing.T) {
	svc := &Service{}
	st := storedLicense{
		Key:          "CGO-ABCDEF-GHIJKL-MNOPQR-STUVWX",
		Role:         RoleMember,
		Plan:         "week_1",
		ExpiresAt:    time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		OfflineUntil: time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}

	got := svc.stateFromStored(st)
	if !got.Activated {
		t.Fatalf("future entitlement should restore active, error=%q", got.Error)
	}
	if got.Error != "" {
		t.Fatalf("future entitlement restored with unexpected error %q", got.Error)
	}
}


func TestProfileIDIsStableAndDoesNotExposeLicenseKey(t *testing.T) {
	svc := &Service{
		stored: storedLicense{Key: "CGO-ABCDEF-GHIJKL-MNOPQR-STUVWX"},
	}

	id1 := svc.ProfileID()
	id2 := svc.ProfileID()
	if id1 == "" {
		t.Fatal("expected non-empty profile id")
	}
	if id1 != id2 {
		t.Fatalf("profile id is not stable: %q != %q", id1, id2)
	}
	if strings.Contains(id1, "ABCDEF") || strings.Contains(id1, "STUVWX") {
		t.Fatalf("profile id leaked license material: %q", id1)
	}
	if len(id1) != 16 {
		t.Fatalf("profile id length=%d want 16", len(id1))
	}
}

func TestProfileIDDifferentLicensesDoNotCollideInBasicCase(t *testing.T) {
	a := &Service{stored: storedLicense{Key: "CGO-AAAAAA-BBBBBB-CCCCCC-DDDDDD"}}
	b := &Service{stored: storedLicense{Key: "CGO-111111-222222-333333-444444"}}

	if a.ProfileID() == b.ProfileID() {
		t.Fatalf("different licenses produced same profile id %q", a.ProfileID())
	}
}

func TestProfileIDEmptyWithoutLicense(t *testing.T) {
	svc := &Service{}
	if got := svc.ProfileID(); got != "" {
		t.Fatalf("empty license profile id=%q want empty", got)
	}
}


func TestLicenseSaveAndReloadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "license.json")
	svc := &Service{
		path: path,
		stored: storedLicense{
			Key:           "CGO-ABCDEF-GHIJKL-MNOPQR-STUVWX",
			Role:          RoleMember,
			MemberName:    "Nathan",
			MachineID:     "machine-hash",
			LastValidated: time.Now().UTC().Format(time.RFC3339),
			OfflineUntil:  time.Now().UTC().Add(72 * time.Hour).Format(time.RFC3339),
			Plan:          "month_1",
			ExpiresAt:     time.Now().UTC().Add(30 * 24 * time.Hour).Format(time.RFC3339),
		},
	}
	svc.state = svc.stateFromStored(svc.stored)
	if err := svc.saveLocked(); err != nil {
		t.Fatalf("saveLocked: %v", err)
	}

	reloaded := &Service{path: path}
	reloaded.load()
	if reloaded.stored.Key != svc.stored.Key {
		t.Fatalf("reloaded key mismatch: %q", reloaded.stored.Key)
	}
	if reloaded.state.MemberName != "Nathan" || reloaded.state.Plan != "month_1" {
		t.Fatalf("reloaded metadata mismatch: %+v", reloaded.state)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary license file survived successful save: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup license file survived successful save: %v", err)
	}
}

func TestLicenseLoadRecoversBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "license.json")
	backupState := storedLicense{
		Key:          "CGO-ABCDEF-GHIJKL-MNOPQR-STUVWX",
		Role:         RoleDeveloper,
		MemberName:   "Recovered",
		MachineID:    "machine-hash",
		Plan:         "week_1",
		ExpiresAt:    time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339),
		OfflineUntil: time.Now().UTC().Add(72 * time.Hour).Format(time.RFC3339),
	}

	protected, err := protectSecret(backupState.Key)
	if err != nil {
		t.Fatalf("protectSecret: %v", err)
	}
	disk := backupState
	disk.Key = protected
	blob, err := json.Marshal(disk)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", blob, 0o600); err != nil {
		t.Fatal(err)
	}

	svc := &Service{path: path}
	svc.load()
	if !svc.state.Activated {
		t.Fatalf("backup should restore active state: %+v", svc.state)
	}
	if svc.state.Role != RoleDeveloper || svc.state.MemberName != "Recovered" {
		t.Fatalf("backup metadata mismatch: %+v", svc.state)
	}
}
