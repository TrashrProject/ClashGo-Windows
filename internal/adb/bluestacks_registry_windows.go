//go:build windows

package adb

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

var blueStacksRegistryBasePaths = []string{
	`SOFTWARE\BlueStacks_nxt`,
	`SOFTWARE\BlueStacks_msi5`,
	`SOFTWARE\BlueStacks_nxt_cn`,
}

// blueStacksRegistryDataDirs reads BlueStacks' configured data roots. This is
// important for custom installs: HD-Player remains under Program Files, while
// the user-data directory (which owns bluestacks.conf / Engine) may live on
// another drive.
func blueStacksRegistryDataDirs() []string {
	seen := map[string]bool{}
	out := []string{}
	views := []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY}

	for _, base := range blueStacksRegistryBasePaths {
		for _, view := range views {
			k, err := registry.OpenKey(registry.LOCAL_MACHINE, base, registry.QUERY_VALUE|view)
			if err != nil {
				continue
			}
			value, _, valueErr := k.GetStringValue("DataDir")
			_ = k.Close()
			if valueErr != nil {
				continue
			}
			value = strings.TrimSpace(strings.Trim(value, `"`))
			if value == "" {
				continue
			}
			key := strings.ToLower(value)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, value)
		}
	}
	return out
}
