package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"net"
	"os"
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
	mux := http.NewServeMux()

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

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "private, max-age=30")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("ClashGO account service listening on %s", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
