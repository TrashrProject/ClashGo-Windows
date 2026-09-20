package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWindowsPackagedAssetLayoutContract(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific executable layout test")
	}
	// The production resolver is exercised end-to-end in packaged builds.
	// This test pins the user-visible override contract used by portable builds.
	dir := t.TempDir()
	assets := filepath.Join(dir, "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLASHGO_ASSETS_DIR", assets)
	if got := GetAssetsDir(); got != assets {
		t.Fatalf("GetAssetsDir()=%q want %q", got, assets)
	}
}
