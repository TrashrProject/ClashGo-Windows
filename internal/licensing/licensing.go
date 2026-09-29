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
	LicenseHint   string `json:"license_hint,omitempty"`
	MachineID     string `json:"machine_id,omitempty"`
	LastValidated string `json:"last_validated,omitempty"`
	OfflineUntil  string `json:"offline_until,omitempty"`
	Error         string `json:"error,omitempty"`
}

type storedLicense struct {
	Key           string `json:"license_key"`
	Role          Role   `json:"role"`
	MachineID     string `json:"machine_id"`
	LastValidated string `json:"last_validated"`
	OfflineUntil  string `json:"offline_until"`
}

type activateRequest struct {
	LicenseKey string `json:"license_key"`
	MachineID  string `json:"machine_id"`
	AppVersion string `json:"app_version"`
}

type activateResponse struct {
	OK           bool   `json:"ok"`
	Role         Role   `json:"role"`
	OfflineUntil string `json:"offline_until"`
	Message      string `json:"message,omitempty"`
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

func (s *Service) load() {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	var st storedLicense
	if json.Unmarshal(b, &st) != nil {
		return
	}
	s.stored = st
	s.state = s.stateFromStored(st)
}

func (s *Service) stateFromStored(st storedLicense) State {
	return State{
		Activated: st.Key != "",
		Role: st.Role,
		LicenseHint: licenseHint(st.Key),
		MachineID: st.MachineID,
		LastValidated: st.LastValidated,
		OfflineUntil: st.OfflineUntil,
	}
}

func (s *Service) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.stored, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, b, 0o600)
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

func (s *Service) Activate(ctx context.Context, key string) (State, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	if len(key) < 12 {
		return s.GetState(), errors.New("invalid license key")
	}
	machineID, err := machineFingerprint()
	if err != nil {
		return s.GetState(), fmt.Errorf("machine fingerprint: %w", err)
	}
	if s.baseURL == "" {
		return s.GetState(), errors.New("license service is not configured")
	}

	reqBody, _ := json.Marshal(activateRequest{
		LicenseKey: key,
		MachineID: machineID,
		AppVersion: s.appVersion,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/v1/license/activate", bytes.NewReader(reqBody))
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
		return s.GetState(), errors.New(out.Message)
	}

	now := time.Now().UTC()
	s.mu.Lock()
	s.stored = storedLicense{
		Key: key,
		Role: out.Role,
		MachineID: machineID,
		LastValidated: now.Format(time.RFC3339),
		OfflineUntil: out.OfflineUntil,
	}
	s.state = s.stateFromStored(s.stored)
	err = s.saveLocked()
	state := s.state
	s.mu.Unlock()
	return state, err
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
	s.stored = storedLicense{}
	s.state = State{}
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func HashMachineID(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:])
}
