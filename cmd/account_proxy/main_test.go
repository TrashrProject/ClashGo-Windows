package main

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestNormalizeTag(t *testing.T) {
	got, err := normalizeTag(" 899qupv2q ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "#899QUPV2Q" {
		t.Fatalf("tag=%q want #899QUPV2Q", got)
	}

	if _, err := normalizeTag("#BAD-TAG"); err == nil {
		t.Fatal("expected invalid character error")
	}
}

func TestProfileCacheExpires(t *testing.T) {
	c := &profileCache{entries: make(map[string]cacheEntry)}
	c.put("#ABC123", http.StatusOK, []byte(`{"tag":"#ABC123"}`), 20*time.Millisecond)

	if _, ok := c.get("#ABC123"); !ok {
		t.Fatal("expected cache hit")
	}

	time.Sleep(30 * time.Millisecond)
	if _, ok := c.get("#ABC123"); ok {
		t.Fatal("expected expired cache miss")
	}
}


func testControlStoreWithLicense(key string, rec *licenseRecord) *controlStore {
	return &controlStore{
		data: controlData{
			Licenses: map[string]*licenseRecord{
				hashLicense(key): rec,
			},
		},
	}
}

func TestAuthorizeAdminRequiresActiveAdminAndMatchingMachine(t *testing.T) {
	const key = "CGO-AAAAAA-BBBBBB-CCCCCC-DDDDDD"
	future := time.Now().UTC().Add(24 * time.Hour)

	tests := []struct {
		name      string
		rec       *licenseRecord
		machineID string
		want      bool
	}{
		{
			name: "active admin",
			rec: &licenseRecord{Role: "admin", Active: true, MachineID: "machine-a", ExpiresAt: future},
			machineID: "machine-a",
			want: true,
		},
		{
			name: "developer is not admin",
			rec: &licenseRecord{Role: "developer", Active: true, MachineID: "machine-a", ExpiresAt: future},
			machineID: "machine-a",
			want: false,
		},
		{
			name: "wrong machine",
			rec: &licenseRecord{Role: "admin", Active: true, MachineID: "machine-a", ExpiresAt: future},
			machineID: "machine-b",
			want: false,
		},
		{
			name: "revoked",
			rec: &licenseRecord{Role: "admin", Active: false, MachineID: "machine-a", ExpiresAt: future},
			machineID: "machine-a",
			want: false,
		},
		{
			name: "expired",
			rec: &licenseRecord{Role: "admin", Active: true, MachineID: "machine-a", ExpiresAt: time.Now().UTC().Add(-time.Minute)},
			machineID: "machine-a",
			want: false,
		},
		{
			name: "unbound machine",
			rec: &licenseRecord{Role: "admin", Active: true, MachineID: "", ExpiresAt: future},
			machineID: "machine-a",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := testControlStoreWithLicense(key, tc.rec)
			if got := store.authorizeAdmin(key, tc.machineID); got != tc.want {
				t.Fatalf("authorizeAdmin() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFindLicenseRecordByIDSupportsStoredAndDerivedIDs(t *testing.T) {
	key := "CGO-AAAAAA-BBBBBB-CCCCCC-DDDDDD"
	hash := hashLicense(key)

	explicit := &licenseRecord{ID: "explicit-id", Hint: "••••-DDDD", Active: true}
	store := &controlStore{data: controlData{Licenses: map[string]*licenseRecord{hash: explicit}}}
	if got := findLicenseRecordByIDLocked(store, "explicit-id"); got != explicit {
		t.Fatal("explicit license id was not found")
	}

	legacy := &licenseRecord{Hint: "••••-DDDD", Active: true}
	store = &controlStore{data: controlData{Licenses: map[string]*licenseRecord{hash: legacy}}}
	if got := findLicenseRecordByIDLocked(store, licenseIDFromHash(hash)); got != legacy {
		t.Fatal("legacy record was not found through derived id")
	}
}

func TestLicensePlanAndRoleNormalization(t *testing.T) {
	if got := validRole(" developer "); got != "developer" {
		t.Fatalf("validRole developer = %q", got)
	}
	if got := validRole("garbage"); got != "member" {
		t.Fatalf("invalid role should fall back to member, got %q", got)
	}

	plan, days := validPlan("week_1")
	if plan != "week_1" || days != 7 {
		t.Fatalf("week plan = %q/%d", plan, days)
	}
	plan, days = validPlan("month_1")
	if plan != "month_1" || days != 30 {
		t.Fatalf("month plan = %q/%d", plan, days)
	}
	plan, days = validPlan("lifetime")
	if plan != "lifetime" || days != 0 {
		t.Fatalf("lifetime plan = %q/%d", plan, days)
	}
}


func TestControlStoreRoleAuthorization(t *testing.T) {
	memberKey := "CGO-MEMBER-TEST-KEY-0001"
	devKey := "CGO-DEVELOPER-TEST-KEY-0002"
	adminKey := "CGO-ADMIN-TEST-KEY-0003"
	expiredKey := "CGO-EXPIRED-TEST-KEY-0004"

	store := &controlStore{}
	store.data.Licenses = map[string]*licenseRecord{
		hashLicense(memberKey): {
			ID: "member-id", Role: "member", Active: true, MachineID: "pc-member",
		},
		hashLicense(devKey): {
			ID: "developer-id", Role: "developer", Active: true, MachineID: "pc-dev",
		},
		hashLicense(adminKey): {
			ID: "admin-id", Role: "admin", Active: true, MachineID: "pc-admin",
		},
		hashLicense(expiredKey): {
			ID: "expired-id", Role: "admin", Active: true, MachineID: "pc-expired",
			ExpiresAt: time.Now().UTC().Add(-time.Hour),
		},
	}

	if _, ok := store.authorizeDeveloper(memberKey, "pc-member"); ok {
		t.Fatal("member license must not receive developer access")
	}
	if _, ok := store.authorizeDeveloper(devKey, "pc-dev"); !ok {
		t.Fatal("developer license should receive developer access")
	}
	if _, ok := store.authorizeDeveloper(devKey, "wrong-pc"); ok {
		t.Fatal("developer license must be bound to the matching machine")
	}

	adminID, ok := store.authorizedAdminLicenseID(adminKey, "pc-admin")
	if !ok || adminID != "admin-id" {
		t.Fatalf("admin authorization failed: id=%q ok=%v", adminID, ok)
	}
	if _, ok := store.authorizedAdminLicenseID(devKey, "pc-dev"); ok {
		t.Fatal("developer license must not receive admin actions")
	}
	if _, ok := store.authorizedAdminLicenseID(expiredKey, "pc-expired"); ok {
		t.Fatal("expired admin license must be rejected")
	}
}

func TestAuthorizedAdminLicenseIDFallsBackToDerivedID(t *testing.T) {
	key := "CGO-ADMIN-DERIVED-ID-TEST"
	store := &controlStore{}
	store.data.Licenses = map[string]*licenseRecord{
		hashLicense(key): {
			Role: "admin", Active: true, MachineID: "pc-admin",
		},
	}

	got, ok := store.authorizedAdminLicenseID(key, "pc-admin")
	if !ok {
		t.Fatal("expected admin authorization")
	}
	want := licenseIDFromHash(hashLicense(key))
	if got != want {
		t.Fatalf("derived admin id=%q want %q", got, want)
	}
}

func TestValidLicensePlans(t *testing.T) {
	tests := []struct {
		in   string
		plan string
		days int
	}{
		{"free_2d", "free_2d", 2},
		{"week_1", "week_1", 7},
		{"month_1", "month_1", 30},
		{"lifetime", "lifetime", 0},
	}
	for _, tc := range tests {
		plan, days := validPlan(tc.in)
		if plan != tc.plan || days != tc.days {
			t.Fatalf("validPlan(%q)=(%q,%d), want (%q,%d)", tc.in, plan, days, tc.plan, tc.days)
		}
	}
}


func TestValidPaymentStatus(t *testing.T) {
	tests := map[string]string{
		" paid ":    "paid",
		"PENDING":   "pending",
		"offered":   "offered",
		"free":      "free",
		"garbage":   "unknown",
		"":          "unknown",
	}
	for input, want := range tests {
		if got := validPaymentStatus(input); got != want {
			t.Fatalf("validPaymentStatus(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAppendEventLockedCapsAndKeepsNewest(t *testing.T) {
	store := &controlStore{}
	for i := 0; i < 5005; i++ {
		store.appendEventLocked(licenseEvent{
			ID:        fmt.Sprintf("event-%04d", i),
			LicenseID: "license-1",
			EventType: "renewal",
			CreatedAt: time.Unix(int64(i), 0).UTC(),
		})
	}
	if got := len(store.data.Events); got != 5000 {
		t.Fatalf("events len = %d, want 5000", got)
	}
	if got := store.data.Events[0].ID; got != "event-0005" {
		t.Fatalf("oldest retained event = %q, want event-0005", got)
	}
	if got := store.data.Events[len(store.data.Events)-1].ID; got != "event-5004" {
		t.Fatalf("newest retained event = %q, want event-5004", got)
	}
}

func TestAppendEventLockedFillsMetadata(t *testing.T) {
	store := &controlStore{}
	store.appendEventLocked(licenseEvent{
		LicenseID: "license-1",
		EventType: "created",
	})
	if len(store.data.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(store.data.Events))
	}
	event := store.data.Events[0]
	if event.ID == "" {
		t.Fatal("expected generated event id")
	}
	if event.CreatedAt.IsZero() {
		t.Fatal("expected generated event timestamp")
	}
}


func TestApplyLicenseRenewalExtendsFutureExpiry(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	rec := &licenseRecord{
		Plan:      "week_1",
		Active:    true,
		ExpiresAt: now.Add(5 * 24 * time.Hour),
	}

	plan, days, expiresAt, err := applyLicenseRenewal(rec, "week_1", now)
	if err != nil {
		t.Fatal(err)
	}
	if plan != "week_1" || days != 7 {
		t.Fatalf("renew result = %q/%d, want week_1/7", plan, days)
	}
	want := now.Add(12 * 24 * time.Hour)
	if !expiresAt.Equal(want) {
		t.Fatalf("expiresAt = %s, want %s", expiresAt, want)
	}
	if !rec.NextDueAt.Equal(want) {
		t.Fatalf("NextDueAt = %s, want %s", rec.NextDueAt, want)
	}
}

func TestApplyLicenseRenewalExpiredStartsFromNow(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	rec := &licenseRecord{
		Plan:      "month_1",
		Active:    false,
		ExpiresAt: now.Add(-48 * time.Hour),
	}

	plan, days, expiresAt, err := applyLicenseRenewal(rec, "month_1", now)
	if err != nil {
		t.Fatal(err)
	}
	if plan != "month_1" || days != 30 {
		t.Fatalf("renew result = %q/%d, want month_1/30", plan, days)
	}
	want := now.Add(30 * 24 * time.Hour)
	if !expiresAt.Equal(want) {
		t.Fatalf("expiresAt = %s, want %s", expiresAt, want)
	}
	if !rec.Active {
		t.Fatal("renewal should reactivate an expired/revoked timed license")
	}
}

func TestApplyLicenseRenewalProtectsLifetime(t *testing.T) {
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	rec := &licenseRecord{Plan: "lifetime", Active: true}

	if _, _, _, err := applyLicenseRenewal(rec, "month_1", now); err == nil {
		t.Fatal("expected lifetime downgrade to be rejected")
	}
	if rec.Plan != "lifetime" {
		t.Fatalf("failed renewal mutated lifetime plan to %q", rec.Plan)
	}

	plan, days, expiresAt, err := applyLicenseRenewal(rec, "lifetime", now)
	if err != nil {
		t.Fatal(err)
	}
	if plan != "lifetime" || days != 0 || !expiresAt.IsZero() {
		t.Fatalf("lifetime renewal = %q/%d/%s", plan, days, expiresAt)
	}
}
