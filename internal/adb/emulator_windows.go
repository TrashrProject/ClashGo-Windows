//go:build windows

package adb

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type blueStacksWindowsInstance struct {
	Name    string
	ADBPort int
}

var windowsInstancePortRE = regexp.MustCompile("^bst\\.instance\\.([^.]+)\\.(?:status\\.)?adb_port=(.+)$")
var windowsADBAccessRE = regexp.MustCompile("(?m)^bst\\.enable_adb_access\\s*=\\s*\"([01])\"\\s*$")

var fallbackWindowsADBPorts = []int{
	5555, 5556, 5557, 5558, 5559, 5560, 5561, 5562, 5563, 5564, 5565,
}

// ensureBlueStacksPlatform is the Windows implementation behind the
// platform-neutral API used by the boot orchestrator and recovery layer.
func (c *Client) ensureBlueStacksPlatform(ctx context.Context, width, height, dpi int) error {
	return c.ensureBlueStacksWindows(ctx, width, height, dpi)
}

// Keep the upstream Mac-named entry points for compatibility with tooling that
// may still call them directly. Core code uses EnsureBlueStacks/EnsureBlueStacksCtx.
func (c *Client) EnsureBlueStacksMac(width, height, dpi int) error {
	return c.ensureBlueStacksWindows(context.Background(), width, height, dpi)
}

func (c *Client) EnsureBlueStacksMacCtx(ctx context.Context, width, height, dpi int) error {
	return c.ensureBlueStacksWindows(ctx, width, height, dpi)
}

func (c *Client) ensureBlueStacksWindows(ctx context.Context, width, height, dpi int) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	conf := findBlueStacksWindowsConfig()
	adbSettingChanged := false
	if conf != "" {
		changed, err := ensureBlueStacksADBAccess(conf)
		if err != nil {
			c.log.Warn(fmt.Sprintf("could not auto-enable BlueStacks ADB: %v", err))
		} else if changed {
			adbSettingChanged = true
			c.log.Info("enabled BlueStacks Android Debug Bridge in bluestacks.conf (backup created)")
		}
	}

	instances := discoverBlueStacksWindowsInstances()
	preferred := chooseBlueStacksWindowsInstance(instances, c.blueStacksInstance)
	ports := windowsCandidateADBPortsSelected(instances, preferred, c.strictBlueStacksSelection())

	if !adbSettingChanged {
		if addr := c.findReachableBlueStacks(ctx, ports); addr != "" {
			c.DeviceID = addr
			c.log.Info(fmt.Sprintf("BlueStacks already reachable on %s — keeping existing instance", addr))
			return c.ensureWindowsAndroidDisplay(width, height, dpi)
		}
	} else {
		// BlueStacks reads this global setting at player startup. Restarting
		// HD-Player after changing it is more reliable than waiting for a live
		// instance to notice the file mutation.
		_ = exec.Command("taskkill", "/F", "/IM", "HD-Player.exe").Run()
		time.Sleep(800 * time.Millisecond)
	}

	player, err := findBlueStacksWindowsPlayer()
	if err != nil {
		return err
	}
	if preferred == "" {
		return errors.New("BlueStacks 5 is installed but no instance was found in bluestacks.conf; start an instance once from Multi-instance Manager, then retry")
	}

	c.log.Info(fmt.Sprintf("starting BlueStacks 5 instance %q via %s", preferred, player))
	if err := launchBlueStacksWindows(ctx, player, preferred); err != nil {
		return fmt.Errorf("start BlueStacks instance %q: %w", preferred, err)
	}
	if err := c.waitForVMProcess(ctx, 45*time.Second); err != nil {
		return err
	}
	if err := c.waitForBlueStacksADBWithPorts(ctx, 90*time.Second, ports); err != nil {
		return err
	}
	return c.ensureWindowsAndroidDisplay(width, height, dpi)
}

func discoverBlueStacksWindowsInstances() []blueStacksWindowsInstance {
	conf := findBlueStacksWindowsConfig()
	if conf == "" {
		return nil
	}
	f, err := os.Open(conf)
	if err != nil {
		return nil
	}
	defer f.Close()

	byName := map[string]int{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		m := windowsInstancePortRE.FindStringSubmatch(strings.TrimSpace(scanner.Text()))
		if len(m) != 3 {
			continue
		}
		portText := strings.Trim(strings.TrimSpace(m[2]), "\"'")
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		byName[m[1]] = port
	}

	out := make([]blueStacksWindowsInstance, 0, len(byName))
	for name, port := range byName {
		out = append(out, blueStacksWindowsInstance{Name: name, ADBPort: port})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func findBlueStacksWindowsConfig() string {
	var candidates []string
	if p := strings.TrimSpace(os.Getenv("CLASHGO_BLUESTACKS_CONF")); p != "" {
		candidates = append(candidates, p)
	}
	if data := strings.TrimSpace(os.Getenv("CLASHGO_BLUESTACKS_DATA")); data != "" {
		candidates = append(candidates, filepath.Join(data, "bluestacks.conf"))
	}
	for _, data := range blueStacksRegistryDataDirs() {
		candidates = append(candidates, filepath.Join(data, "bluestacks.conf"))
	}
	if data := strings.TrimSpace(os.Getenv("ProgramData")); data != "" {
		candidates = append(candidates,
			filepath.Join(data, "BlueStacks_nxt", "bluestacks.conf"),
			filepath.Join(data, "BlueStacks", "bluestacks.conf"),
		)
	}
	candidates = append(candidates,
		"C:\\ProgramData\\BlueStacks_nxt\\bluestacks.conf",
		"C:\\ProgramData\\BlueStacks\\bluestacks.conf",
	)
	for _, p := range candidates {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

// ensureBlueStacksADBAccess only changes BlueStacks' official ADB flag when
// that flag already exists. It never invents config keys. A one-time backup
// is created before mutation so the user can restore the original file.
func ensureBlueStacksADBAccess(conf string) (bool, error) {
	data, err := os.ReadFile(conf)
	if err != nil {
		return false, err
	}
	m := windowsADBAccessRE.FindSubmatch(data)
	if len(m) != 2 {
		return false, nil
	}
	if string(m[1]) == "1" {
		return false, nil
	}

	backup := conf + ".clashgo.bak"
	if _, err := os.Stat(backup); os.IsNotExist(err) {
		if err := os.WriteFile(backup, data, 0o644); err != nil {
			return false, fmt.Errorf("backup bluestacks.conf: %w", err)
		}
	}

	updated := windowsADBAccessRE.ReplaceAll(data, []byte("bst.enable_adb_access=\"1\""))
	if string(updated) == string(data) {
		return false, nil
	}

	mode := os.FileMode(0o644)
	if info, err := os.Stat(conf); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(conf, updated, mode); err != nil {
		return false, fmt.Errorf("write bluestacks.conf: %w", err)
	}
	return true, nil
}

func findBlueStacksWindowsPlayer() (string, error) {
	var candidates []string
	if p := strings.TrimSpace(os.Getenv("CLASHGO_BLUESTACKS_PLAYER")); p != "" {
		candidates = append(candidates, p)
	}
	if home := strings.TrimSpace(os.Getenv("CLASHGO_BLUESTACKS_HOME")); home != "" {
		candidates = append(candidates, filepath.Join(home, "HD-Player.exe"))
	}
	for _, root := range []string{
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
		"C:\\Program Files",
		"C:\\Program Files (x86)",
	} {
		if strings.TrimSpace(root) == "" {
			continue
		}
		candidates = append(candidates,
			filepath.Join(root, "BlueStacks_nxt", "HD-Player.exe"),
			filepath.Join(root, "BlueStacks", "HD-Player.exe"),
		)
	}
	for _, p := range candidates {
		if fileExists(p) {
			return p, nil
		}
	}
	return "", errors.New("BlueStacks 5 HD-Player.exe was not found; install BlueStacks 5 or set CLASHGO_BLUESTACKS_PLAYER")
}

func chooseBlueStacksWindowsInstance(instances []blueStacksWindowsInstance, configured string) string {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		for _, inst := range instances {
			if strings.EqualFold(inst.Name, configured) {
				return inst.Name
			}
		}
	}
	// Environment override is intentionally permissive for advanced/dev use:
	// BlueStacks can accept an instance name before it has appeared in the
	// currently parsed config snapshot. Persistent UI selection stays strict
	// because SetBlueStacksInstance validates detected instances before saving.
	if forced := strings.TrimSpace(os.Getenv("CLASHGO_BLUESTACKS_INSTANCE")); forced != "" {
		return forced
	}
	for _, want := range []string{"Tiramisu64", "Rvc64", "Pie64", "Nougat64", "Nougat32"} {
		for _, inst := range instances {
			if strings.EqualFold(inst.Name, want) {
				return inst.Name
			}
		}
	}
	if len(instances) > 0 {
		return instances[0].Name
	}
	return ""
}

func (c *Client) strictBlueStacksSelection() bool {
	return strings.TrimSpace(c.blueStacksInstance) != "" ||
		strings.TrimSpace(os.Getenv("CLASHGO_BLUESTACKS_INSTANCE")) != ""
}

func windowsCandidateADBPortsSelected(instances []blueStacksWindowsInstance, preferred string, strict bool) []int {
	if strict && preferred != "" {
		for _, inst := range instances {
			if strings.EqualFold(inst.Name, preferred) && inst.ADBPort > 0 {
				return []int{inst.ADBPort}
			}
		}
		// Advanced env override may name an instance not yet present in the
		// config snapshot. Do not silently attach a different instance.
		return nil
	}
	return windowsCandidateADBPortsPreferred(instances, preferred)
}

func windowsCandidateADBPortsPreferred(instances []blueStacksWindowsInstance, preferred string) []int {
	ordered := make([]blueStacksWindowsInstance, 0, len(instances))
	for _, inst := range instances {
		if preferred != "" && strings.EqualFold(inst.Name, preferred) {
			ordered = append(ordered, inst)
		}
	}
	for _, inst := range instances {
		if preferred == "" || !strings.EqualFold(inst.Name, preferred) {
			ordered = append(ordered, inst)
		}
	}
	return windowsCandidateADBPorts(ordered)
}

func windowsCandidateADBPorts(instances []blueStacksWindowsInstance) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(instances)+len(fallbackWindowsADBPorts))
	for _, inst := range instances {
		if inst.ADBPort > 0 && !seen[inst.ADBPort] {
			seen[inst.ADBPort] = true
			out = append(out, inst.ADBPort)
		}
	}
	for _, p := range fallbackWindowsADBPorts {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func launchBlueStacksWindows(ctx context.Context, player, instance string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd := exec.Command(player, "--instance", instance)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func (c *Client) findReachableBlueStacks(ctx context.Context, ports []int) string {
	for _, port := range c.tcpScanListens(ports, 120*time.Millisecond) {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		_ = exec.CommandContext(pctx, ADBExecutable(), "connect", addr).Run()
		cancel()
		if c.isBlueStacksDevice(addr) {
			return addr
		}
	}
	return ""
}

// launchBlueStacks preserves the upstream recovery API. The first Windows
// implementation restarts HD-Player and relaunches only the selected instance.
func (c *Client) launchBlueStacks(_ bool, width, height, dpi int) error {
	_ = exec.Command("taskkill", "/F", "/IM", "HD-Player.exe").Run()
	time.Sleep(800 * time.Millisecond)

	player, err := findBlueStacksWindowsPlayer()
	if err != nil {
		return err
	}
	instances := discoverBlueStacksWindowsInstances()
	instance := chooseBlueStacksWindowsInstance(instances, c.blueStacksInstance)
	if instance == "" {
		return errors.New("no BlueStacks instance found in bluestacks.conf")
	}
	if err := launchBlueStacksWindows(context.Background(), player, instance); err != nil {
		return err
	}
	if err := c.waitForVMProcess(context.Background(), 45*time.Second); err != nil {
		return err
	}
	if err := c.waitForBlueStacksADBWithPorts(
		context.Background(),
		90*time.Second,
		windowsCandidateADBPortsSelected(instances, instance, c.strictBlueStacksSelection()),
	); err != nil {
		return err
	}
	return c.ensureWindowsAndroidDisplay(width, height, dpi)
}

func (c *Client) waitForVMProcess(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.firstVMSignal() != "" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return errors.New("timeout waiting for BlueStacks HD-Player.exe")
}

func (c *Client) firstVMSignal() string {
	out, err := exec.Command(
		"tasklist",
		"/FI", "IMAGENAME eq HD-Player.exe",
		"/FO", "CSV",
		"/NH",
	).Output()
	if err == nil && strings.Contains(strings.ToLower(string(out)), "hd-player.exe") {
		return "HD-Player.exe"
	}
	return ""
}

func (c *Client) isBlueStacksDevice(id string) bool {
	t, err := NewTransport(id, c.host, c.port, 2*time.Second)
	if err != nil {
		return false
	}
	defer t.Close()

	var identity strings.Builder
	for _, cmd := range []string{
		"getprop ro.product.manufacturer",
		"getprop ro.product.brand",
		"getprop ro.product.model",
	} {
		if out, err := t.Shell(cmd); err == nil {
			identity.WriteString(" ")
			identity.WriteString(strings.ToLower(strings.TrimSpace(out)))
		}
	}

	low := identity.String()
	for _, marker := range []string{"bluestacks", "microvirt", "samsung", "oneplus", "asus"} {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

func (c *Client) waitForBlueStacksADB(ctx context.Context, timeout time.Duration) error {
	instances := discoverBlueStacksWindowsInstances()
	preferred := chooseBlueStacksWindowsInstance(instances, c.blueStacksInstance)
	return c.waitForBlueStacksADBWithPorts(
		ctx,
		timeout,
		windowsCandidateADBPortsSelected(instances, preferred, c.strictBlueStacksSelection()),
	)
}

func (c *Client) waitForBlueStacksADBWithPorts(ctx context.Context, timeout time.Duration, ports []int) error {
	deadline := time.Now().Add(timeout)
	midpoint := time.Now().Add(timeout / 2)
	resetDone := false

	for time.Now().Before(deadline) {
		if addr := c.findReachableBlueStacks(ctx, ports); addr != "" {
			c.DeviceID = addr
			c.log.Info(fmt.Sprintf("BlueStacks ADB ready at %s", addr))
			return nil
		}
		if !resetDone && time.Now().After(midpoint) {
			c.log.Warn("BlueStacks ADB not ready at half-budget; resetting local adb-server")
			_ = c.ResetAdbServer()
			resetDone = true
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("timeout waiting for BlueStacks ADB on ports %v", ports)
}

func (c *Client) ensureWindowsAndroidDisplay(width, height, dpi int) error {
	if c.DeviceID == "" || width <= 0 || height <= 0 {
		return nil
	}
	t, err := NewTransport(c.DeviceID, c.host, c.port, 5*time.Second)
	if err != nil {
		return fmt.Errorf("connect display transport: %w", err)
	}
	defer t.Close()

	if _, err := t.Shell(fmt.Sprintf("wm size %dx%d", width, height)); err != nil {
		return fmt.Errorf("set Android display size %dx%d: %w", width, height, err)
	}
	if dpi > 0 {
		if _, err := t.Shell(fmt.Sprintf("wm density %d", dpi)); err != nil {
			c.log.Warn(fmt.Sprintf("could not set Android density to %d: %v", dpi, err))
		}
	}
	return nil
}

func (c *Client) writeResolutionDefaultsIfPlistExists(_, _, _ int) {}

func (c *Client) tcpScanListens(ports []int, perPort time.Duration) []int {
	var open []int
	for _, p := range ports {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", p), perPort)
		if err != nil {
			continue
		}
		_ = conn.Close()
		open = append(open, p)
	}
	return open
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
