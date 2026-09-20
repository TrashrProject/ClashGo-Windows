package adb

// PlatformInstanceDiagnostic describes one emulator instance discovered from
// the host platform configuration.
type PlatformInstanceDiagnostic struct {
	Name      string `json:"name"`
	ADBPort   int    `json:"adb_port"`
	Preferred bool   `json:"preferred"`
}

// PlatformDiagnostics is safe to expose through Wails. It intentionally
// contains paths/status only, never credentials or private emulator data.
type PlatformDiagnostics struct {
	OS                   string                       `json:"os"`
	Supported            bool                         `json:"supported"`
	ADBExecutable        string                       `json:"adb_executable"`
	ADBFound             bool                         `json:"adb_found"`
	BlueStacksPlayer     string                       `json:"bluestacks_player"`
	BlueStacksPlayerFound bool                        `json:"bluestacks_player_found"`
	BlueStacksConfig     string                       `json:"bluestacks_config"`
	BlueStacksConfigFound bool                        `json:"bluestacks_config_found"`
	BlueStacksRunning   bool                         `json:"bluestacks_running"`
	ADBSettingPresent   bool                         `json:"adb_setting_present"`
	ADBEnabled          bool                         `json:"adb_enabled"`
	PreferredInstance   string                       `json:"preferred_instance"`
	Instances           []PlatformInstanceDiagnostic `json:"instances"`
}

// GetPlatformDiagnostics returns a read-only host/emulator snapshot.
func GetPlatformDiagnostics() PlatformDiagnostics {
	return getPlatformDiagnostics()
}
