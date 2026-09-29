package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type limiterState struct {
	mu      sync.Mutex
	entries map[string]*clientWindow
}

type clientWindow struct {
	start time.Time
	count int
}

type cacheEntry struct {
	status int
	body   []byte
	expiry time.Time
}

type profileCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
}


type licenseRecord struct {
	ID         string    `json:"id"`
	Hint       string    `json:"hint"`
	CustomerName    string `json:"customer_name,omitempty"`
	CustomerContact string `json:"customer_contact,omitempty"`
	Role       string    `json:"role"`
	Active     bool      `json:"active"`
	MachineID  string    `json:"machine_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at,omitempty"`
	AppVersion  string    `json:"app_version,omitempty"`
	Plan        string    `json:"plan,omitempty"`
	DurationDays int      `json:"duration_days,omitempty"`
	ActivatedAt time.Time `json:"activated_at,omitempty"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
}

type supportIncident struct {
	ID         string         `json:"id"`
	At         string         `json:"at"`
	ReceivedAt time.Time      `json:"received_at"`
	License    string         `json:"license_hint"`
	Role       string         `json:"role"`
	MachineID  string         `json:"machine_id,omitempty"`
	AppVersion string         `json:"app_version,omitempty"`
	Level      string         `json:"level"`
	Message    string         `json:"message"`
	Fields     map[string]any `json:"fields,omitempty"`
}

type controlData struct {
	Licenses  map[string]*licenseRecord `json:"licenses"`
	Incidents []supportIncident         `json:"incidents"`
}

type controlStore struct {
	mu   sync.RWMutex
	path string
	data controlData
}

func newControlStore(path string) *controlStore {
	s := &controlStore{path: path}
	s.data.Licenses = make(map[string]*licenseRecord)
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &s.data)
		if s.data.Licenses == nil {
			s.data.Licenses = make(map[string]*licenseRecord)
		}
	}
	return s
}

func (s *controlStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func hashLicense(key string) string {
	sum := sha256.Sum256([]byte(strings.ToUpper(strings.TrimSpace(key))))
	return hex.EncodeToString(sum[:])
}

func licenseIDFromHash(hash string) string {
	if len(hash) > 16 {
		return hash[:16]
	}
	return hash
}

func licenseHint(key string) string {
	key = strings.ToUpper(strings.TrimSpace(key))
	if len(key) <= 4 {
		return key
	}
	return "••••-" + key[len(key)-4:]
}

func newLicenseKey() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	raw := strings.ToUpper(hex.EncodeToString(buf))
	return fmt.Sprintf("CGO-%s-%s-%s-%s", raw[0:6], raw[6:12], raw[12:18], raw[18:24]), nil
}

func validRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "developer":
		return "developer"
	case "admin":
		return "admin"
	default:
		return "member"
	}
}

func validPlan(plan string) (string, int) {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "free_2d":
		return "free_2d", 2
	case "week_1":
		return "week_1", 7
	case "month_1":
		return "month_1", 30
	default:
		return "lifetime", 0
	}
}

func licenseExpired(rec *licenseRecord, now time.Time) bool {
	return rec != nil && !rec.ExpiresAt.IsZero() && !now.Before(rec.ExpiresAt)
}

func adminAuthorized(r *http.Request, adminKey string) bool {
	if adminKey == "" {
		return false
	}
	return subtleConstantTimeEqual(strings.TrimSpace(r.Header.Get("X-ClashGO-Admin-Key")), adminKey)
}

func subtleConstantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := 0; i < len(a); i++ {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}

func (s *controlStore) lookup(key string) (*licenseRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.data.Licenses[hashLicense(key)]
	if !ok || rec == nil {
		return nil, false
	}
	cp := *rec
	return &cp, true
}


func (s *controlStore) authorizeDeveloper(key, machineID string) (*licenseRecord, bool) {
	rec, ok := s.lookup(key)
	if !ok || !rec.Active || licenseExpired(rec, time.Now().UTC()) {
		return nil, false
	}
	if rec.Role != "developer" && rec.Role != "admin" {
		return nil, false
	}
	if rec.MachineID == "" || strings.TrimSpace(machineID) == "" || rec.MachineID != strings.TrimSpace(machineID) {
		return nil, false
	}
	return rec, true
}

func (s *controlStore) authorizeAdmin(key, machineID string) bool {
	rec, ok := s.authorizeDeveloper(key, machineID)
	return ok && rec.Role == "admin"
}

func (s *controlStore) authorizedAdminLicenseID(key, machineID string) (string, bool) {
	rec, ok := s.authorizeDeveloper(key, machineID)
	if !ok || rec.Role != "admin" {
		return "", false
	}
	id := strings.TrimSpace(rec.ID)
	if id == "" {
		id = licenseIDFromHash(hashLicense(key))
	}
	return id, id != ""
}

func findLicenseRecordByIDLocked(s *controlStore, id string) *licenseRecord {
	id = strings.TrimSpace(id)
	if s == nil || id == "" {
		return nil
	}
	for hash, candidate := range s.data.Licenses {
		if candidate == nil {
			continue
		}
		candidateID := candidate.ID
		if candidateID == "" {
			candidateID = licenseIDFromHash(hash)
		}
		if candidateID == id {
			return candidate
		}
	}
	return nil
}

func (c *profileCache) get(tag string) (cacheEntry, bool) {
	c.mu.RLock()
	entry, ok := c.entries[tag]
	c.mu.RUnlock()
	if !ok || time.Now().After(entry.expiry) {
		if ok {
			c.mu.Lock()
			delete(c.entries, tag)
			c.mu.Unlock()
		}
		return cacheEntry{}, false
	}
	return entry, true
}

func (c *profileCache) put(tag string, status int, body []byte, ttl time.Duration) {
	if status != http.StatusOK || len(body) == 0 {
		return
	}
	cp := append([]byte(nil), body...)
	c.mu.Lock()
	c.entries[tag] = cacheEntry{status: status, body: cp, expiry: time.Now().Add(ttl)}
	c.mu.Unlock()
}

func (l *limiterState) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	w := l.entries[key]
	if w == nil || now.Sub(w.start) >= time.Minute {
		l.entries[key] = &clientWindow{start: now, count: 1}
		return true
	}
	if w.count >= 30 {
		return false
	}
	w.count++
	return true
}

func normalizeTag(raw string) (string, error) {
	tag, err := url.PathUnescape(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	tag = strings.ToUpper(strings.ReplaceAll(tag, " ", ""))
	if !strings.HasPrefix(tag, "#") {
		tag = "#" + tag
	}
	if len(tag) < 4 || len(tag) > 20 {
		return "", fmt.Errorf("invalid player tag")
	}
	for _, r := range tag[1:] {
		if !(r >= '0' && r <= '9') && !(r >= 'A' && r <= 'Z') {
			return "", fmt.Errorf("invalid player tag")
		}
	}
	return tag, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}


func corsMiddleware(next http.Handler, allowedOrigin string) http.Handler {
	allowedOrigin = strings.TrimRight(strings.TrimSpace(allowedOrigin), "/")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/")
		if allowedOrigin != "" && origin == allowedOrigin {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-ClashGO-Admin-Key")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			if origin != allowedOrigin || allowedOrigin == "" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	apiKey := strings.TrimSpace(os.Getenv("COC_API_KEY"))
	if apiKey == "" {
		log.Fatal("COC_API_KEY is required")
	}
	apiKey = strings.TrimPrefix(apiKey, "Bearer ")
	upperKey := strings.ToUpper(apiKey)
	if strings.Contains(upperKey, "TA_VRAIE_CLE") ||
		strings.Contains(upperKey, "TA_CLE_API") ||
		strings.Contains(upperKey, "YOUR_API_KEY") ||
		len(apiKey) < 40 {
		log.Fatal("COC_API_KEY looks like a placeholder or invalid token; paste the real Clash of Clans developer API key")
	}

	addr := strings.TrimSpace(os.Getenv("CLASHGO_ACCOUNT_LISTEN"))
	if addr == "" {
		addr = ":8787"
	}

	client := &http.Client{Timeout: 10 * time.Second}
	limiter := &limiterState{entries: make(map[string]*clientWindow)}
	cache := &profileCache{entries: make(map[string]cacheEntry)}
	mux := http.NewServeMux()

	controlPath := strings.TrimSpace(os.Getenv("CLASHGO_CONTROL_DATA"))
	if controlPath == "" {
		controlPath = filepath.Join(".", "data", "clashgo-control.json")
	}
	control := newControlStore(controlPath)
	adminKey := strings.TrimSpace(os.Getenv("CLASHGO_ADMIN_KEY"))
	webOrigin := strings.TrimSpace(os.Getenv("CLASHGO_WEB_ORIGIN"))
	if webOrigin == "" {
		webOrigin = "https://trashrproject.github.io"
	}

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("GET /v1/player/{tag}", func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			ip = host
		}
		if !limiter.allow(ip) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"reason": "rate_limited",
				"message": "Too many profile requests. Try again in a minute.",
			})
			return
		}

		tag, err := normalizeTag(r.PathValue("tag"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"reason": "invalid_tag", "message": err.Error()})
			return
		}

		if hit, ok := cache.get(tag); ok {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "private, max-age=30")
			w.Header().Set("X-ClashGO-Cache", "HIT")
			w.WriteHeader(hit.status)
			_, _ = w.Write(hit.body)
			return
		}

		endpoint := "https://api.clashofclans.com/v1/players/" + url.PathEscape(tag)
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"reason": "request_build_failed"})
			return
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"reason": "upstream_unavailable",
				"message": "Clash of Clans API is temporarily unavailable.",
			})
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"reason": "upstream_read_failed"})
			return
		}

		// Translate common authorization failures into a message that makes
		// sense to ClashGO users. The actual developer credential remains
		// server-side and is never exposed to the desktop client.
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
			var upstreamErr struct {
				Reason  string `json:"reason"`
				Message string `json:"message"`
			}
			_ = json.Unmarshal(body, &upstreamErr)
			reason := strings.ToLower(strings.TrimSpace(upstreamErr.Reason))
			msg := strings.TrimSpace(upstreamErr.Message)
			if strings.Contains(reason, "ip") || strings.Contains(strings.ToLower(msg), "ip") {
				writeJSON(w, http.StatusBadGateway, map[string]string{
					"reason":  "api_key_ip_mismatch",
					"message": "Server Clash API key is not authorized for this public IP. Create/update the key with the server public IP.",
				})
				return
			}
			writeJSON(w, http.StatusBadGateway, map[string]string{
				"reason":  "api_key_invalid",
				"message": "Server Clash API authorization failed. Check COC_API_KEY and its allowed IP.",
			})
			return
		}

		if resp.StatusCode == http.StatusOK {
			cache.put(tag, resp.StatusCode, body, 60*time.Second)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "private, max-age=30")
		w.Header().Set("X-ClashGO-Cache", "MISS")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
	})


	mux.HandleFunc("POST /v1/license/activate", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			LicenseKey string `json:"license_key"`
			MachineID  string `json:"machine_id"`
			AppVersion string `json:"app_version"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request"})
			return
		}
		in.LicenseKey = strings.ToUpper(strings.TrimSpace(in.LicenseKey))
		in.MachineID = strings.TrimSpace(in.MachineID)
		if in.LicenseKey == "" || in.MachineID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "license key and machine id are required"})
			return
		}

		h := hashLicense(in.LicenseKey)
		control.mu.Lock()
		rec := control.data.Licenses[h]
		now := time.Now().UTC()
		if rec == nil || !rec.Active {
			control.mu.Unlock()
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "license is invalid or revoked"})
			return
		}
		if licenseExpired(rec, now) {
			control.mu.Unlock()
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "license has expired"})
			return
		}
		if rec.MachineID != "" && rec.MachineID != in.MachineID {
			control.mu.Unlock()
			writeJSON(w, http.StatusConflict, map[string]string{"message": "license is already activated on another machine"})
			return
		}
		if rec.ActivatedAt.IsZero() {
			rec.ActivatedAt = now
			if rec.DurationDays > 0 {
				rec.ExpiresAt = now.Add(time.Duration(rec.DurationDays) * 24 * time.Hour)
			}
		}
		rec.MachineID = in.MachineID
		rec.LastSeenAt = now
		rec.AppVersion = strings.TrimSpace(in.AppVersion)
		_ = control.saveLocked()
		role := rec.Role
		plan := rec.Plan
		expiresAt := rec.ExpiresAt
		control.mu.Unlock()

		offlineUntil := now.Add(72 * time.Hour)
		if !expiresAt.IsZero() && expiresAt.Before(offlineUntil) {
			offlineUntil = expiresAt
		}
		payload := map[string]any{
			"ok": true,
			"role": role,
			"plan": plan,
			"offline_until": offlineUntil.Format(time.RFC3339),
		}
		if !expiresAt.IsZero() {
			payload["expires_at"] = expiresAt.Format(time.RFC3339)
		}
		writeJSON(w, http.StatusOK, payload)
	})

	mux.HandleFunc("POST /v1/support/incidents", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get("X-ClashGO-License"))
		rec, ok := control.lookup(key)
		if !ok || !rec.Active || licenseExpired(rec, time.Now().UTC()) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "valid license required"})
			return
		}
		var in supportIncident
		if json.NewDecoder(io.LimitReader(r.Body, 256<<10)).Decode(&in) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid incident"})
			return
		}
		if rec.MachineID != "" && in.MachineID != "" && rec.MachineID != in.MachineID {
			writeJSON(w, http.StatusConflict, map[string]string{"message": "machine mismatch"})
			return
		}
		if len(in.Message) > 4000 {
			in.Message = in.Message[:4000]
		}
		idBytes := make([]byte, 8)
		_, _ = rand.Read(idBytes)
		in.ID = hex.EncodeToString(idBytes)
		in.ReceivedAt = time.Now().UTC()
		in.License = rec.Hint
		in.Role = rec.Role

		control.mu.Lock()
		control.data.Incidents = append(control.data.Incidents, in)
		if len(control.data.Incidents) > 5000 {
			control.data.Incidents = append([]supportIncident(nil), control.data.Incidents[len(control.data.Incidents)-5000:]...)
		}
		_ = control.saveLocked()
		control.mu.Unlock()
		writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "id": in.ID})
	})


	mux.HandleFunc("GET /v1/developer/incidents", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get("X-ClashGO-License"))
		machineID := strings.TrimSpace(r.Header.Get("X-ClashGO-Machine"))
		if _, ok := control.authorizeDeveloper(key, machineID); !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "developer license required"})
			return
		}
		control.mu.RLock()
		rows := append([]supportIncident(nil), control.data.Incidents...)
		control.mu.RUnlock()
		if len(rows) > 500 {
			rows = rows[len(rows)-500:]
		}
		writeJSON(w, http.StatusOK, map[string]any{"incidents": rows})
	})

	mux.HandleFunc("GET /v1/developer/licenses", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get("X-ClashGO-License"))
		machineID := strings.TrimSpace(r.Header.Get("X-ClashGO-Machine"))
		actor, ok := control.authorizeDeveloper(key, machineID)
		if !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "developer license required"})
			return
		}
		control.mu.RLock()
		rows := make([]licenseRecord, 0, len(control.data.Licenses))
		for hash, rec := range control.data.Licenses {
			if rec != nil {
				cp := *rec
				if cp.ID == "" {
					cp.ID = licenseIDFromHash(hash)
				}
				if actor.Role != "admin" {
					cp.CustomerName = ""
					cp.CustomerContact = ""
				}
				rows = append(rows, cp)
			}
		}
		control.mu.RUnlock()
		writeJSON(w, http.StatusOK, map[string]any{"licenses": rows})
	})

	mux.HandleFunc("POST /v1/developer/licenses", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get("X-ClashGO-License"))
		machineID := strings.TrimSpace(r.Header.Get("X-ClashGO-Machine"))
		adminLicense, ok := control.authorizeDeveloper(key, machineID)
		if !ok || adminLicense.Role != "admin" {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "admin license required"})
			return
		}
		var in struct {
			Role            string `json:"role"`
			Plan            string `json:"plan"`
			Count           int    `json:"count"`
			CustomerName    string `json:"customer_name,omitempty"`
			CustomerContact string `json:"customer_contact,omitempty"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request"})
			return
		}
		if in.Count <= 0 {
			in.Count = 1
		}
		if in.Count > 25 {
			in.Count = 25
		}
		role := validRole(in.Role)
		plan, durationDays := validPlan(in.Plan)
		keys := make([]string, 0, in.Count)

		control.mu.Lock()
		for i := 0; i < in.Count; i++ {
			generated, err := newLicenseKey()
			if err != nil {
				control.mu.Unlock()
				writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "key generation failed"})
				return
			}
			hash := hashLicense(generated)
			control.data.Licenses[hash] = &licenseRecord{
				ID:              licenseIDFromHash(hash),
				Hint:            licenseHint(generated),
				CustomerName:    strings.TrimSpace(in.CustomerName),
				CustomerContact: strings.TrimSpace(in.CustomerContact),
				Role:            role,
				Active:       true,
				CreatedAt:    time.Now().UTC(),
				Plan:         plan,
				DurationDays: durationDays,
			}
			keys = append(keys, generated)
		}
		_ = control.saveLocked()
		control.mu.Unlock()

		writeJSON(w, http.StatusCreated, map[string]any{
			"role": role, "plan": plan, "duration_days": durationDays, "licenses": keys,
		})
	})

	mux.HandleFunc("POST /v1/developer/licenses/reset-machine", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get("X-ClashGO-License"))
		machineID := strings.TrimSpace(r.Header.Get("X-ClashGO-Machine"))
		adminLicenseID, ok := control.authorizedAdminLicenseID(key, machineID)
		if !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "admin license required"})
			return
		}
		var in struct {
			LicenseID string `json:"license_id"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in) != nil || strings.TrimSpace(in.LicenseID) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "license_id is required"})
			return
		}
		if strings.TrimSpace(in.LicenseID) == adminLicenseID {
			writeJSON(w, http.StatusConflict, map[string]string{"message": "cannot reset the active admin license machine"})
			return
		}
		control.mu.Lock()
		rec := findLicenseRecordByIDLocked(control, in.LicenseID)
		if rec == nil {
			control.mu.Unlock()
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "license not found"})
			return
		}
		rec.MachineID = ""
		rec.LastSeenAt = time.Time{}
		rec.AppVersion = ""
		_ = control.saveLocked()
		control.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("POST /v1/developer/licenses/set-active", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get("X-ClashGO-License"))
		machineID := strings.TrimSpace(r.Header.Get("X-ClashGO-Machine"))
		adminLicenseID, ok := control.authorizedAdminLicenseID(key, machineID)
		if !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "admin license required"})
			return
		}
		var in struct {
			LicenseID string `json:"license_id"`
			Active    bool   `json:"active"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in) != nil || strings.TrimSpace(in.LicenseID) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "license_id is required"})
			return
		}
		if strings.TrimSpace(in.LicenseID) == adminLicenseID && !in.Active {
			writeJSON(w, http.StatusConflict, map[string]string{"message": "cannot revoke the active admin license"})
			return
		}
		control.mu.Lock()
		rec := findLicenseRecordByIDLocked(control, in.LicenseID)
		if rec == nil {
			control.mu.Unlock()
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "license not found"})
			return
		}
		rec.Active = in.Active
		_ = control.saveLocked()
		control.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "active": in.Active})
	})

	mux.HandleFunc("POST /v1/developer/licenses/renew", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get("X-ClashGO-License"))
		machineID := strings.TrimSpace(r.Header.Get("X-ClashGO-Machine"))
		if !control.authorizeAdmin(key, machineID) {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "admin license required"})
			return
		}
		var in struct {
			LicenseID string `json:"license_id"`
			Plan      string `json:"plan,omitempty"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in) != nil || strings.TrimSpace(in.LicenseID) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "license_id is required"})
			return
		}
		control.mu.Lock()
		rec := findLicenseRecordByIDLocked(control, in.LicenseID)
		if rec == nil {
			control.mu.Unlock()
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "license not found"})
			return
		}
		plan := strings.TrimSpace(in.Plan)
		if plan == "" {
			plan = rec.Plan
		}
		plan, durationDays := validPlan(plan)
		if rec.Plan == "lifetime" && plan != "lifetime" {
			control.mu.Unlock()
			writeJSON(w, http.StatusConflict, map[string]string{"message": "lifetime license cannot be downgraded by renewal"})
			return
		}
		rec.Plan = plan
		rec.DurationDays = durationDays
		rec.Active = true
		if durationDays == 0 {
			rec.ExpiresAt = time.Time{}
		} else {
			now := time.Now().UTC()
			base := now
			if !rec.ExpiresAt.IsZero() && rec.ExpiresAt.After(now) {
				base = rec.ExpiresAt
			}
			rec.ExpiresAt = base.Add(time.Duration(durationDays) * 24 * time.Hour)
		}
		expiresAt := rec.ExpiresAt
		_ = control.saveLocked()
		control.mu.Unlock()

		payload := map[string]any{"ok": true, "plan": plan, "duration_days": durationDays}
		if !expiresAt.IsZero() {
			payload["expires_at"] = expiresAt.Format(time.RFC3339)
		}
		writeJSON(w, http.StatusOK, payload)
	})


	mux.HandleFunc("POST /v1/developer/licenses/update-customer", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get("X-ClashGO-License"))
		machineID := strings.TrimSpace(r.Header.Get("X-ClashGO-Machine"))
		if !control.authorizeAdmin(key, machineID) {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "admin license required"})
			return
		}
		var in struct {
			LicenseID       string `json:"license_id"`
			CustomerName    string `json:"customer_name"`
			CustomerContact string `json:"customer_contact,omitempty"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request"})
			return
		}
		in.LicenseID = strings.TrimSpace(in.LicenseID)
		in.CustomerName = strings.TrimSpace(in.CustomerName)
		in.CustomerContact = strings.TrimSpace(in.CustomerContact)
		if in.LicenseID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "license_id is required"})
			return
		}
		if in.CustomerName == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "customer_name is required"})
			return
		}
		if len(in.CustomerName) > 120 {
			in.CustomerName = in.CustomerName[:120]
		}
		if len(in.CustomerContact) > 180 {
			in.CustomerContact = in.CustomerContact[:180]
		}

		control.mu.Lock()
		rec := findLicenseRecordByIDLocked(control, in.LicenseID)
		if rec == nil {
			control.mu.Unlock()
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "license not found"})
			return
		}
		rec.CustomerName = in.CustomerName
		rec.CustomerContact = in.CustomerContact
		_ = control.saveLocked()
		control.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"ok": true,
			"customer_name": in.CustomerName,
			"customer_contact": in.CustomerContact,
		})
	})

	mux.HandleFunc("POST /v1/developer/licenses/set-role", func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get("X-ClashGO-License"))
		machineID := strings.TrimSpace(r.Header.Get("X-ClashGO-Machine"))
		adminLicenseID, ok := control.authorizedAdminLicenseID(key, machineID)
		if !ok {
			writeJSON(w, http.StatusForbidden, map[string]string{"message": "admin license required"})
			return
		}
		var in struct {
			LicenseID string `json:"license_id"`
			Role      string `json:"role"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in) != nil || strings.TrimSpace(in.LicenseID) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request"})
			return
		}
		role := validRole(in.Role)
		if strings.TrimSpace(in.LicenseID) == adminLicenseID && role != "admin" {
			writeJSON(w, http.StatusConflict, map[string]string{"message": "cannot remove admin role from the active admin license"})
			return
		}
		control.mu.Lock()
		rec := findLicenseRecordByIDLocked(control, in.LicenseID)
		if rec == nil {
			control.mu.Unlock()
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "license not found"})
			return
		}
		rec.Role = role
		_ = control.saveLocked()
		control.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "role": role})
	})

	mux.HandleFunc("POST /v1/admin/licenses", func(w http.ResponseWriter, r *http.Request) {
		if !adminAuthorized(r, adminKey) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "admin authorization required"})
			return
		}
		var in struct {
			Role  string `json:"role"`
			Plan  string `json:"plan"`
			Count int    `json:"count"`
		}
		_ = json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in)
		if in.Count <= 0 {
			in.Count = 1
		}
		if in.Count > 100 {
			in.Count = 100
		}
		role := validRole(in.Role)
		plan, durationDays := validPlan(in.Plan)
		keys := make([]string, 0, in.Count)
		control.mu.Lock()
		for i := 0; i < in.Count; i++ {
			key, err := newLicenseKey()
			if err != nil {
				control.mu.Unlock()
				writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "key generation failed"})
				return
			}
			keyHash := hashLicense(key)
			control.data.Licenses[keyHash] = &licenseRecord{
				ID: licenseIDFromHash(keyHash),
				Hint: licenseHint(key),
				Role: role,
				Active: true,
				CreatedAt: time.Now().UTC(),
				Plan: plan,
				DurationDays: durationDays,
			}
			keys = append(keys, key)
		}
		_ = control.saveLocked()
		control.mu.Unlock()
		writeJSON(w, http.StatusCreated, map[string]any{"role": role, "plan": plan, "duration_days": durationDays, "licenses": keys})
	})

	mux.HandleFunc("GET /v1/admin/licenses", func(w http.ResponseWriter, r *http.Request) {
		if !adminAuthorized(r, adminKey) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "admin authorization required"})
			return
		}
		control.mu.RLock()
		rows := make([]licenseRecord, 0, len(control.data.Licenses))
		for hash, rec := range control.data.Licenses {
			if rec != nil {
				cp := *rec
				if cp.ID == "" {
					cp.ID = licenseIDFromHash(hash)
				}
				rows = append(rows, cp)
			}
		}
		control.mu.RUnlock()
		writeJSON(w, http.StatusOK, map[string]any{"licenses": rows})
	})

	mux.HandleFunc("GET /v1/admin/incidents", func(w http.ResponseWriter, r *http.Request) {
		if !adminAuthorized(r, adminKey) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "admin authorization required"})
			return
		}
		control.mu.RLock()
		rows := append([]supportIncident(nil), control.data.Incidents...)
		control.mu.RUnlock()
		if len(rows) > 250 {
			rows = rows[len(rows)-250:]
		}
		writeJSON(w, http.StatusOK, map[string]any{"incidents": rows})
	})

	mux.HandleFunc("POST /v1/admin/licenses/reset-machine", func(w http.ResponseWriter, r *http.Request) {
		if !adminAuthorized(r, adminKey) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "admin authorization required"})
			return
		}
		var in struct {
			LicenseID  string `json:"license_id"`
			LicenseKey string `json:"license_key,omitempty"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request"})
			return
		}
		control.mu.Lock()
		var rec *licenseRecord
		if strings.TrimSpace(in.LicenseID) != "" {
			for hash, candidate := range control.data.Licenses {
				if candidate == nil {
					continue
				}
				id := candidate.ID
				if id == "" {
					id = licenseIDFromHash(hash)
				}
				if id == strings.TrimSpace(in.LicenseID) {
					rec = candidate
					break
				}
			}
		} else if strings.TrimSpace(in.LicenseKey) != "" {
			rec = control.data.Licenses[hashLicense(in.LicenseKey)]
		}
		if rec == nil {
			control.mu.Unlock()
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "license not found"})
			return
		}
		rec.MachineID = ""
		rec.LastSeenAt = time.Time{}
		rec.AppVersion = ""
		_ = control.saveLocked()
		control.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	mux.HandleFunc("POST /v1/admin/licenses/revoke", func(w http.ResponseWriter, r *http.Request) {
		if !adminAuthorized(r, adminKey) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "admin authorization required"})
			return
		}
		var in struct {
			LicenseID  string `json:"license_id"`
			LicenseKey string `json:"license_key,omitempty"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request"})
			return
		}
		control.mu.Lock()
		var rec *licenseRecord
		if strings.TrimSpace(in.LicenseID) != "" {
			for hash, candidate := range control.data.Licenses {
				if candidate == nil {
					continue
				}
				id := candidate.ID
				if id == "" {
					id = licenseIDFromHash(hash)
				}
				if id == strings.TrimSpace(in.LicenseID) {
					rec = candidate
					break
				}
			}
		} else if strings.TrimSpace(in.LicenseKey) != "" {
			rec = control.data.Licenses[hashLicense(in.LicenseKey)]
		}
		if rec == nil {
			control.mu.Unlock()
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "license not found"})
			return
		}
		rec.Active = false
		_ = control.saveLocked()
		control.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})


	mux.HandleFunc("POST /v1/admin/licenses/set-role", func(w http.ResponseWriter, r *http.Request) {
		if !adminAuthorized(r, adminKey) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "admin authorization required"})
			return
		}
		var in struct {
			LicenseID string `json:"license_id"`
			Role      string `json:"role"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request"})
			return
		}
		id := strings.TrimSpace(in.LicenseID)
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "license_id is required"})
			return
		}
		role := validRole(in.Role)
		control.mu.Lock()
		var rec *licenseRecord
		for hash, candidate := range control.data.Licenses {
			if candidate == nil {
				continue
			}
			candidateID := candidate.ID
			if candidateID == "" {
				candidateID = licenseIDFromHash(hash)
			}
			if candidateID == id {
				rec = candidate
				break
			}
		}
		if rec == nil {
			control.mu.Unlock()
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "license not found"})
			return
		}
		rec.Role = role
		_ = control.saveLocked()
		control.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "role": role})
	})

	mux.HandleFunc("POST /v1/admin/licenses/set-active", func(w http.ResponseWriter, r *http.Request) {
		if !adminAuthorized(r, adminKey) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "admin authorization required"})
			return
		}
		var in struct {
			LicenseID string `json:"license_id"`
			Active    bool   `json:"active"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request"})
			return
		}
		id := strings.TrimSpace(in.LicenseID)
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "license_id is required"})
			return
		}
		control.mu.Lock()
		var rec *licenseRecord
		for hash, candidate := range control.data.Licenses {
			if candidate == nil {
				continue
			}
			candidateID := candidate.ID
			if candidateID == "" {
				candidateID = licenseIDFromHash(hash)
			}
			if candidateID == id {
				rec = candidate
				break
			}
		}
		if rec == nil {
			control.mu.Unlock()
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "license not found"})
			return
		}
		rec.Active = in.Active
		_ = control.saveLocked()
		control.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "active": in.Active})
	})


	mux.HandleFunc("POST /v1/admin/licenses/renew", func(w http.ResponseWriter, r *http.Request) {
		if !adminAuthorized(r, adminKey) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "admin authorization required"})
			return
		}
		var in struct {
			LicenseID string `json:"license_id"`
			Plan      string `json:"plan,omitempty"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid request"})
			return
		}
		id := strings.TrimSpace(in.LicenseID)
		if id == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "license_id is required"})
			return
		}

		control.mu.Lock()
		var rec *licenseRecord
		for hash, candidate := range control.data.Licenses {
			if candidate == nil {
				continue
			}
			candidateID := candidate.ID
			if candidateID == "" {
				candidateID = licenseIDFromHash(hash)
			}
			if candidateID == id {
				rec = candidate
				break
			}
		}
		if rec == nil {
			control.mu.Unlock()
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "license not found"})
			return
		}

		plan := strings.TrimSpace(in.Plan)
		if plan == "" {
			plan = rec.Plan
		}
		plan, durationDays := validPlan(plan)
		if rec.Plan == "lifetime" && plan != "lifetime" {
			control.mu.Unlock()
			writeJSON(w, http.StatusConflict, map[string]string{"message": "lifetime license cannot be downgraded by renewal"})
			return
		}
		rec.Plan = plan
		rec.DurationDays = durationDays
		rec.Active = true

		if durationDays == 0 {
			rec.ExpiresAt = time.Time{}
		} else {
			now := time.Now().UTC()
			base := now
			if !rec.ExpiresAt.IsZero() && rec.ExpiresAt.After(now) {
				base = rec.ExpiresAt
			}
			rec.ExpiresAt = base.Add(time.Duration(durationDays) * 24 * time.Hour)
		}

		expiresAt := rec.ExpiresAt
		_ = control.saveLocked()
		control.mu.Unlock()

		payload := map[string]any{
			"ok": true,
			"plan": plan,
			"duration_days": durationDays,
		}
		if !expiresAt.IsZero() {
			payload["expires_at"] = expiresAt.Format(time.RFC3339)
		}
		writeJSON(w, http.StatusOK, payload)
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           corsMiddleware(mux, webOrigin),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("ClashGO account service listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
