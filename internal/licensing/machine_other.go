//go:build !windows

package licensing

import (
	"os"
	"strings"
)

func machineFingerprint() (string, error) {
	host, err := os.Hostname()
	if err != nil {
		return "", err
	}
	return HashMachineID(strings.TrimSpace(host)), nil
}
