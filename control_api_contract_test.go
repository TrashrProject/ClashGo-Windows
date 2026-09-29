package main

import (
	"os"
	"strings"
	"testing"
)

func TestCloudflareDeveloperAdminContractMatchesDesktop(t *testing.T) {
	workerBytes, err := os.ReadFile("cloudflare/worker/src/index.js")
	if err != nil { t.Fatal(err) }
	appBytes, err := os.ReadFile("app.go")
	if err != nil { t.Fatal(err) }

	worker := string(workerBytes)
	app := string(appBytes)

	routes := []string{
		"/v1/developer/licenses",
		"/v1/developer/licenses/reset-machine",
		"/v1/developer/licenses/set-role",
		"/v1/developer/licenses/set-active",
		"/v1/developer/licenses/renew",
		"/v1/developer/licenses/update-customer",
		"/v1/developer/incidents",
		"/v1/developer/history",
	}
	for _, route := range routes {
		if !strings.Contains(worker, route) { t.Errorf("worker is missing desktop route %q", route) }
		if !strings.Contains(app, route) { t.Errorf("desktop app is missing worker route %q", route) }
	}

	if !strings.Contains(worker, "const adminOnly = () =>") {
		t.Fatal("worker no longer defines the admin-only guard for licensed developer endpoints")
	}

	mutatingRoutes := []string{
		`path === "/v1/developer/licenses"`,
		"/v1/developer/licenses/reset-machine",
		"/v1/developer/licenses/set-role",
		"/v1/developer/licenses/set-active",
		"/v1/developer/licenses/renew",
		"/v1/developer/licenses/update-customer",
	}
	for _, route := range mutatingRoutes {
		idx := strings.Index(worker, route)
		if idx < 0 { continue }
		windowEnd := idx + 500
		if windowEnd > len(worker) { windowEnd = len(worker) }
		if !strings.Contains(worker[idx:windowEnd], "adminOnly()") {
			t.Errorf("mutating route %q is no longer protected by adminOnly()", route)
		}
	}

	if !strings.Contains(app, `req.Header.Set("X-ClashGO-License", a.license.LicenseKey())`) ||
		!strings.Contains(app, `req.Header.Set("X-ClashGO-Machine", a.license.MachineID())`) {
		t.Fatal("desktop admin/developer calls no longer send machine-bound license authentication")
	}
}


func TestCloudflareProtectsActiveAdminFromSelfLockout(t *testing.T) {
	data, err := os.ReadFile("cloudflare/worker/src/index.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	required := []string{
		"cannot remove admin role from the active admin license",
		"cannot revoke the active admin license",
		"cannot reset the active admin license machine",
	}
	for _, message := range required {
		if !strings.Contains(src, message) {
			t.Errorf("missing active-admin self-lockout protection: %q", message)
		}
	}
}

func TestDeveloperRoleCannotReceiveCommercialCustomerData(t *testing.T) {
	workerBytes, err := os.ReadFile("cloudflare/worker/src/index.js")
	if err != nil { t.Fatal(err) }
	worker := string(workerBytes)
	for _, field := range []string{"customer_notes,", "payment_status,", "total_paid_cents,", "next_due_at,"} {
		if !strings.Contains(worker, field) { t.Errorf("worker developer redaction no longer strips %s", field) }
	}

	goBytes, err := os.ReadFile("cmd/account_proxy/main.go")
	if err != nil { t.Fatal(err) }
	goSrc := string(goBytes)
	for _, assignment := range []string{"cp.CustomerNotes = \"\"", "cp.PaymentStatus = \"\"", "cp.TotalPaidCents = 0", "cp.NextDueAt = time.Time{}"} {
		if !strings.Contains(goSrc, assignment) { t.Errorf("Go fallback developer redaction missing %q", assignment) }
	}
}
