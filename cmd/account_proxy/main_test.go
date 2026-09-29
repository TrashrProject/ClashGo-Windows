package main

import (
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
