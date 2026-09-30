//go:build !windows

package licensing

import "strings"

// Non-Windows builds are used for development/tests. Production ClashGO is
// distributed for Windows, where license secrets are protected by DPAPI.
func protectSecret(value string) (string, error) {
	return strings.TrimSpace(value), nil
}

func unprotectSecret(value string) (string, error) {
	return strings.TrimSpace(value), nil
}

func secretNeedsMigration(string) bool { return false }
