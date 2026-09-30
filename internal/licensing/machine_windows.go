//go:build windows

package licensing

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

func machineFingerprint() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Cryptography`, registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", err
	}
	defer k.Close()
	v, _, err := k.GetStringValue("MachineGuid")
	if err != nil {
		return "", err
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("empty Windows MachineGuid")
	}
	return HashMachineID(v), nil
}
