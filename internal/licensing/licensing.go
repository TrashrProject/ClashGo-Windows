package licensing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Ducky705/ClashGO/internal/paths"
)

type Role string

const (
	RoleMember    Role = "member"
	RoleDeveloper Role = "developer"
	RoleAdmin     Role = "admin"
)

type State struct {
	Activated     bool   `json:"activated"`
	Role          Role   `json:"role"`
	LicenseID     string `json:"license_id,omitempty"`
	MemberName    string `json:"member_name,omitempty"`
	LicenseHint   string `json:"license_hint,omitempty"`
	MachineID     string `json:"machine_id,omitempty"`
	LastValidated string `json:"last_validated,omitempty"`
	OfflineUntil  string `json:"offline_until,omitempty"`
	Plan          string `json:"plan,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	Error         string `json:"error,omitempty"`
}

type storedLicense struct {
	Key           string `json:"license_key"`
	Role          Role   `json:"role"`
	LicenseID     string `json:"license_id,omitempty"`
	MemberName    string `json:"member_name,omitempty"`
	MachineID     string `json:"machine_id"`
	LastValidated string `json:"last_validated"`
	OfflineUntil  string `json:"offline_until"`
	Plan          string `json:"plan,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
}

type activateRequest struct {
	LicenseKey string `json:"license_key"`
	MachineID  string `json:"machine_id"`
	AppVersion string `json:"app_version"`
}

type activateResponse struct {
	OK           bool   `json:"ok"`
	Role         Role   `json:"role"`
	LicenseID    string `json:"license_id,omitempty"`
	MemberName   string `json:"member_name,omitempty"`
	OfflineUntil string `json:"offline_until"`
	Plan         string `json:"plan,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
	Message      string `json:"message,omitempty"`
}

type RemoteError struct {
	Status  int
	Message string
}

func (e *RemoteError) Error() string {
	if e == nil {
		return "license server error"
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return fmt.Sprintf("license server HTTP %d", e.Status)
}

func (e *RemoteError) Definitive() bool {
	if e == nil {
		return false
	}
	return e.Status == http.StatusUnauthorized ||
		e.Status == http.StatusForbidden ||
		e.Status == http.StatusConflict ||
		e.Status == http.StatusNotFound
}

type Service struct {
	baseURL    string
	appVersion string
	httpClient *http.Client
	path       string
	mu         sync.RWMutex
	stored     storedLicense
	state      State
}

func New(baseURL, appVersion string) *Service {
	s := &Service{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		appVersion: strings.TrimSpace(appVersion),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		path: paths.ResolveConfig("license.json"),
	}
	s.load()
	return s
}

func (s *Service) SetBaseURL(raw string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.baseURL = strings.TrimRight(strings.TrimSpace(raw), "/")
	s.mu.Unlock()
}

func (s *Service) baseURLSnapshot() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.baseURL
}

func decodeStoredLicense(data []byte) (storedLicense, bool, error) {
	var st storedLicense
	if err := json.Unmarshal(data, &st); err != nil {
		return storedLicense{}, false, err
	}
	legacyPlaintext := secretNeedsMigration(st.Key)
	key, err := unprotectSecret(st.Key)
	if err != nil {
		return storedLicense{}, false, err
	}
	st.Key = strings.ToUpper(strings.TrimSpace(key))
	return st, legacyPlaintext, nil
}

func (s *Service) load() {
	var (
		st              storedLicense
		legacyPlaintext bool
		loadErr         error
	)

	if b, err := os.ReadFile(s.path); err == nil {
		st, legacyPlaintext, loadErr = decodeStoredLicense(b)
	} else {
		loadErr = err
	}

	// Transactional writes keep the previous license as .bak during the swap.
	// Recover it if the primary file is missing/corrupt after an interrupted
	// Windows write.
	if loadErr != nil {
		if backup, err := os.ReadFile(s.path + ".bak"); err == nil {
			if recovered, legacy, err := decodeStoredLicense(backup); err == nil {
				st = recovered
				legacyPlaintext = legacy
				loadErr = nil
			}
		}
	}
	if loadErr != nil {
		// Missing state is normal on first launch. Corrupt/unreadable state is
		// surfaced only when a file actually exists.
		if !os.IsNotExist(loadErr) {
			s.state = State{Activated: false, Error: "stored license could not be loaded"}
		}
		return
	}

	s.stored = st
	s.state = s.stateFromStored(st)

	// Migrate legacy plaintext license.json files transparently. The in-memory
	// representation remains plaintext because it is needed for server
	// validation; only the on-disk copy is protected.
	if legacyPlaintext && st.Key != "" {
		_ = s.saveLocked()
	}
}

func (s *Service) stateFromStored(st storedLicense) State {
	activated := st.Key != ""
	stateErr := ""
	if activated && strings.TrimSpace(st.ExpiresAt) != "" {
		if expiry, err := time.Parse(time.RFC3339, st.ExpiresAt); err == nil && !time.Now().Before(expiry) {
			activated = false
			stateErr = "license has expired"
		}
	}

	return State{
		Activated: activated,
		Role: st.Role,
		LicenseID: st.LicenseID,
		MemberName: st.MemberName,
		LicenseHint: licenseHint(st.Key),
		MachineID: st.MachineID,
		LastValidated: st.LastValidated,
		OfflineUntil: st.OfflineUntil,
		Plan: st.Plan,
		ExpiresAt: st.ExpiresAt,
		Error: stateErr,
	}
}

func (s *Service) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}

	disk := s.stored
	protectedKey, err := protectSecret(disk.Key)
	if err != nil {
		return fmt.Errorf("protect license secret: %w", err)
	}
	disk.Key = protectedKey

	b, err := json.MarshalIndent(disk, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.path + ".tmp"
	backupPath := s.path + ".bak"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	_ = os.Remove(backupPath)
	hadOriginal := false
	if _, err := os.Stat(s.path); err == nil {
		if err := os.Rename(s.path, backupPath); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		hadOriginal = true
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		if hadOriginal {
			_ = os.Rename(backupPath, s.path)
		}
		_ = os.Remove(tmpPath)
		return err
	}
	_ = os.Remove(backupPath)
	return nil
}

func licenseHint(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 4 {
		return ""
	}
	return "••••-" + key[len(key)-4:]
}

func (s *Service) GetState() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *Service) LicenseKey() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stored.Key
}

func (s *Service) MachineID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stored.MachineID
}

func (s *Service) ProfileID() string {
	s.mu.RLock()
	key := strings.TrimSpace(s.stored.Key)
	s.mu.RUnlock()
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.ToUpper(key)))
	encoded := hex.EncodeToString(sum[:])
	if len(encoded) > 16 {
		return encoded[:16]
	}
	return encoded
}

func (s *Service) Activate(ctx context.Context, key string) (State, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	if len(key) < 12 {
		return s.GetState(), errors.New("invalid license key")
	}
	machineID, err := machineFingerprint()
	if err != nil {
		return s.GetState(), fmt.Errorf("machine fingerprint: %w", err)
	}
	baseURL := s.baseURLSnapshot()
	if baseURL == "" {
		return s.GetState(), errors.New("license service is not configured")
	}

	reqBody, _ := json.Marshal(activateRequest{
		LicenseKey: key,
		MachineID: machineID,
		AppVersion: s.appVersion,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/license/activate", bytes.NewReader(reqBody))
	if err != nil {
		return s.GetState(), err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return s.GetState(), err
	}
	defer resp.Body.Close()

	var out activateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return s.GetState(), fmt.Errorf("decode license response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || !out.OK {
		if out.Message == "" {
			out.Message = fmt.Sprintf("license server HTTP %d", resp.StatusCode)
		}
		return s.GetState(), &RemoteError{Status: resp.StatusCode, Message: out.Message}
	}

	now := time.Now().UTC()
	s.mu.Lock()
	previousStored := s.stored
	previousState := s.state

	nextStored := storedLicense{
		Key:           key,
		Role:          out.Role,
		LicenseID:     strings.TrimSpace(out.LicenseID),
		MemberName:    strings.TrimSpace(out.MemberName),
		MachineID:     machineID,
		LastValidated: now.Format(time.RFC3339),
		OfflineUntil:  out.OfflineUntil,
		Plan:          out.Plan,
		ExpiresAt:     out.ExpiresAt,
	}
	s.stored = nextStored
	if err = s.saveLocked(); err != nil {
		s.stored = previousStored
		s.state = previousState
		state := s.state
		s.mu.Unlock()
		return state, err
	}
	s.state = s.stateFromStored(nextStored)
	state := s.state
	s.mu.Unlock()
	return state, nil
}

func (s *Service) Validate(ctx context.Context) State {
	s.mu.RLock()
	key := s.stored.Key
	s.mu.RUnlock()
	if key == "" {
		return s.GetState()
	}
	state, err := s.Activate(ctx, key)
	if err == nil {
		return state
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// A server-side rejection is authoritative. Offline grace exists only for
	// transport/service outages, never for a revoked license or machine clash.
	var remoteErr *RemoteError
	if errors.As(err, &remoteErr) && remoteErr.Definitive() {
		s.state = s.stateFromStored(s.stored)
		s.state.Activated = false
		s.state.Error = remoteErr.Error()
		return s.state
	}

	if expiry, parseErr := time.Parse(time.RFC3339, s.stored.ExpiresAt); parseErr == nil && !time.Now().Before(expiry) {
		s.state = s.stateFromStored(s.stored)
		s.state.Activated = false
		s.state.Error = "license has expired"
		return s.state
	}

	if until, parseErr := time.Parse(time.RFC3339, s.stored.OfflineUntil); parseErr == nil && time.Now().Before(until) {
		s.state = s.stateFromStored(s.stored)
		s.state.Error = "offline grace period"
		return s.state
	}
	s.state = s.stateFromStored(s.stored)
	s.state.Activated = false
	s.state.Error = err.Error()
	return s.state
}

func (s *Service) DeactivateLocal() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clear every transactional artifact. Leaving a stale .bak behind could
	// make load() recover an intentionally deactivated licence on next launch.
	var firstErr error
	for _, candidate := range []string{s.path, s.path + ".bak", s.path + ".tmp"} {
		if err := os.Remove(candidate); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return firstErr
	}

	s.stored = storedLicense{}
	s.state = State{}
	return nil
}

func HashMachineID(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:])
}
