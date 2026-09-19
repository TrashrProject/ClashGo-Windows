package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Ducky705/ClashGO/internal/paths"
)

// ExportDiagnostics creates a support bundle containing only ClashGO runtime
// diagnostics/logs. It does not include screenshots, emulator userdata,
// credentials, or account tokens.
func (a *App) ExportDiagnostics() (string, error) {
	outDir := paths.ResolveConfig("diagnostics")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("create diagnostics dir: %w", err)
	}

	name := "clashgo-diagnostics-" + time.Now().Format("20060102-150405") + ".zip"
	outPath := filepath.Join(outDir, name)
	f, err := os.Create(outPath)
	if err != nil {
		return "", fmt.Errorf("create diagnostics zip: %w", err)
	}

	zw := zip.NewWriter(f)
	ok := false
	defer func() {
		if !ok {
			_ = zw.Close()
			_ = f.Close()
			_ = os.Remove(outPath)
		}
	}()

	diag := collectSystemDiagnostics()
	b, err := json.MarshalIndent(diag, "", "  ")
	if err != nil {
		return "", err
	}
	if err := writeDiagnosticBytes(zw, "system.json", b); err != nil {
		return "", err
	}

	meta := fmt.Sprintf(
		"ClashGO Windows\nVersion: %s\nCreated: %s\nOS: %s/%s\n",
		version,
		time.Now().Format(time.RFC3339),
		diag.OS,
		diag.Arch,
	)
	if err := writeDiagnosticBytes(zw, "README.txt", []byte(meta)); err != nil {
		return "", err
	}

	for _, rel := range []string{
		"logs/app.log",
		"logs/last_boot_report.json",
		"stats.json",
		"attack_history.json",
	} {
		src := paths.ResolveConfig(rel)
		if err := writeDiagnosticFile(zw, src, rel); err != nil && !os.IsNotExist(err) {
			return "", err
		}
	}

	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("finalize diagnostics zip: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close diagnostics zip: %w", err)
	}
	ok = true
	return outPath, nil
}

func writeDiagnosticBytes(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func writeDiagnosticFile(zw *zip.Writer, src, name string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	w, err := zw.Create(filepath.ToSlash(name))
	if err != nil {
		return err
	}
	_, err = io.Copy(w, in)
	return err
}
