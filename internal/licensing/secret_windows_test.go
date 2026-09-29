//go:build windows

package licensing

import (
	"strings"
	"testing"
)

func TestDPAPIProtectSecretRoundTrip(t *testing.T) {
	const original = "CGO-ABCDEF-GHIJKL-MNOPQR-STUVWX"

	protected, err := protectSecret(original)
	if err != nil {
		t.Fatalf("protectSecret failed: %v", err)
	}
	if protected == original {
		t.Fatal("protected secret must not equal plaintext")
	}
	if !strings.HasPrefix(protected, protectedSecretPrefix) {
		t.Fatalf("protected secret prefix = %q", protected)
	}

	plain, err := unprotectSecret(protected)
	if err != nil {
		t.Fatalf("unprotectSecret failed: %v", err)
	}
	if plain != original {
		t.Fatalf("round trip = %q, want %q", plain, original)
	}
}

func TestDPAPILegacyPlaintextCanBeReadForMigration(t *testing.T) {
	const legacy = "CGO-LEGACY-PLAINT-EXTKEY"

	if !secretNeedsMigration(legacy) {
		t.Fatal("legacy plaintext should require migration")
	}
	plain, err := unprotectSecret(legacy)
	if err != nil {
		t.Fatalf("legacy plaintext read failed: %v", err)
	}
	if plain != legacy {
		t.Fatalf("legacy plaintext changed: %q", plain)
	}
}
