package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ducky705/ClashGO/internal/config"
)

func TestNormalizeControlServiceURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"https worker", " https://clashgo.example.workers.dev/ ", "https://clashgo.example.workers.dev", false},
		{"https path", "https://example.com/api/", "https://example.com/api", false},
		{"local http", "http://127.0.0.1:8787/", "http://127.0.0.1:8787", false},
		{"localhost http", "http://localhost:8787", "http://localhost:8787", false},
		{"remote http rejected", "http://example.com", "", true},
		{"credentials rejected", "https://user:pass@example.com", "", true},
		{"query rejected", "https://example.com?token=x", "", true},
		{"empty rejected", "", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeControlServiceURL(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("normalizeControlServiceURL(%q) expected error, got %q", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeControlServiceURL(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("normalizeControlServiceURL(%q)=%q want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestClashControlServiceURLPrecedence(t *testing.T) {
	oldEmbedded := controlServiceURL
	defer func() { controlServiceURL = oldEmbedded }()

	cfg := config.DefaultConfig()
	cfg.Account.ControlURL = "https://local-override.example/"
	cfg.Account.ProxyURL = "https://account.example/"

	t.Setenv("CLASHGO_CONTROL_API_URL", "")
	controlServiceURL = ""
	if got := clashControlServiceURL(cfg); got != "https://local-override.example" {
		t.Fatalf("local control override=%q", got)
	}
	if !clashControlServiceConfigured(cfg) {
		t.Fatal("local control override should mark service configured")
	}

	controlServiceURL = "https://embedded.example/"
	if got := clashControlServiceURL(cfg); got != "https://embedded.example" {
		t.Fatalf("embedded URL should win over local override, got %q", got)
	}

	t.Setenv("CLASHGO_CONTROL_API_URL", "https://env.example/")
	if got := clashControlServiceURL(cfg); got != "https://env.example" {
		t.Fatalf("environment URL should have highest priority, got %q", got)
	}
}

func TestClashControlServiceFallbackDoesNotEnforceLicensing(t *testing.T) {
	oldEmbedded := controlServiceURL
	defer func() { controlServiceURL = oldEmbedded }()
	controlServiceURL = ""
	t.Setenv("CLASHGO_CONTROL_API_URL", "")

	cfg := config.DefaultConfig()
	cfg.Account.ControlURL = ""
	cfg.Account.ProxyURL = "https://account-only.example/"

	if got := clashControlServiceURL(cfg); got != "https://account-only.example" {
		t.Fatalf("combined development fallback=%q", got)
	}
	if clashControlServiceConfigured(cfg) {
		t.Fatal("account-service fallback must not enable license enforcement")
	}
}


func TestStableBuildIgnoresLocalControlOverride(t *testing.T) {
	oldEmbedded := controlServiceURL
	oldChannel := updateChannel
	oldVersion := version
	defer func() {
		controlServiceURL = oldEmbedded
		updateChannel = oldChannel
		version = oldVersion
	}()

	controlServiceURL = ""
	t.Setenv("CLASHGO_CONTROL_API_URL", "")
	updateChannel = "stable"
	version = "1.0.0"

	cfg := config.DefaultConfig()
	cfg.Account.ControlURL = "https://attacker.example/"
	cfg.Account.ProxyURL = "https://account.example/"

	if betaControlOverrideAllowed() {
		t.Fatal("stable build unexpectedly allows local control override")
	}
	if clashControlServiceConfigured(cfg) {
		t.Fatal("stable build must ignore local control override for enforcement")
	}
	if got := clashControlServiceURL(cfg); got != "https://account.example" {
		t.Fatalf("stable fallback=%q want account service fallback", got)
	}
}

func TestBetaBuildAllowsLocalControlOverride(t *testing.T) {
	oldEmbedded := controlServiceURL
	oldChannel := updateChannel
	oldVersion := version
	defer func() {
		controlServiceURL = oldEmbedded
		updateChannel = oldChannel
		version = oldVersion
	}()

	controlServiceURL = ""
	t.Setenv("CLASHGO_CONTROL_API_URL", "")
	updateChannel = "beta"
	version = "1.0.0-beta.1"

	cfg := config.DefaultConfig()
	cfg.Account.ControlURL = "https://beta-control.example/"

	if !betaControlOverrideAllowed() {
		t.Fatal("beta build should allow local control override")
	}
	if !clashControlServiceConfigured(cfg) {
		t.Fatal("beta local control override should enable enforcement")
	}
	if got := clashControlServiceURL(cfg); got != "https://beta-control.example" {
		t.Fatalf("beta control URL=%q", got)
	}
}


func TestProbeControlServiceURL(t *testing.T) {
	t.Run("healthy server", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/healthz" {
				t.Fatalf("unexpected path %q", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer server.Close()

		if err := probeControlServiceURL(server.URL); err != nil {
			t.Fatalf("healthy server rejected: %v", err)
		}
	})

	t.Run("unhealthy status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", http.StatusServiceUnavailable)
		}))
		defer server.Close()

		if err := probeControlServiceURL(server.URL); err == nil {
			t.Fatal("unhealthy server unexpectedly accepted")
		}
	})

	t.Run("invalid health payload", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"maybe"}`))
		}))
		defer server.Close()

		if err := probeControlServiceURL(server.URL); err == nil {
			t.Fatal("invalid health payload unexpectedly accepted")
		}
	})
}
