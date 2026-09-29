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
