package main

import (
	"bufio"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Event struct {
	InstallID string                 `json:"install_id"`
	Version   string                 `json:"version"`
	OS        string                 `json:"os"`
	Arch      string                 `json:"arch"`
	SentAt    string                 `json:"sent_at"`
	Log       map[string]interface{} `json:"log"`
}

type Store struct {
	mu   sync.Mutex
	path string
}

func (s *Store) append(events []Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return nil
}

func authBearer(expected string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if expected == "" {
			http.Error(w, "server token not configured", http.StatusServiceUnavailable)
			return
		}
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func main() {
	addr := env("CLASHGO_TELEMETRY_ADDR", "127.0.0.1:8787")
	dataPath := env("CLASHGO_TELEMETRY_DATA", "./data/events.jsonl")
	ingestToken := os.Getenv("CLASHGO_TELEMETRY_INGEST_TOKEN")
	adminToken := os.Getenv("CLASHGO_TELEMETRY_ADMIN_TOKEN")
	store := &Store{path: dataPath}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/logs", authBearer(ingestToken, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		var body struct { Events []Event `json:"events"` }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		if len(body.Events) == 0 || len(body.Events) > 256 {
			http.Error(w, "invalid batch", http.StatusBadRequest)
			return
		}
		if err := store.append(body.Events); err != nil {
			http.Error(w, "store failed", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	mux.HandleFunc("/admin/errors", authBearer(adminToken, func(w http.ResponseWriter, r *http.Request) {
		counts := map[string]int{}
		versions := map[string]int{}
		users := map[string]struct{}{}
		f, err := os.Open(dataPath)
		if err != nil && !os.IsNotExist(err) {
			http.Error(w, "read failed", 500)
			return
		}
		if f != nil {
			defer f.Close()
			sc := bufio.NewScanner(f)
			buf := make([]byte, 64*1024)
			sc.Buffer(buf, 2<<20)
			for sc.Scan() {
				var e Event
				if json.Unmarshal(sc.Bytes(), &e) != nil { continue }
				users[e.InstallID] = struct{}{}
				versions[e.Version]++
				level, _ := e.Log["level"].(string)
				if level != "warn" && level != "error" && level != "fatal" && level != "panic" { continue }
				msg, _ := e.Log["message"].(string)
				if msg == "" { msg = "(no message)" }
				counts[msg]++
			}
		}
		type row struct { Message string `json:"message"`; Count int `json:"count"` }
		rows := make([]row, 0, len(counts))
		for k,v := range counts { rows = append(rows, row{k,v}) }
		sort.Slice(rows, func(i,j int) bool { return rows[i].Count > rows[j].Count })
		if len(rows) > 100 { rows = rows[:100] }
		writeJSON(w, map[string]interface{}{
			"generated_at": time.Now().UTC().Format(time.RFC3339),
			"installations": len(users),
			"versions": versions,
			"top_errors": rows,
		})
	}))

	log.Printf("ClashGO telemetry collector listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func env(k,v string) string { if x:=strings.TrimSpace(os.Getenv(k)); x!="" { return x }; return v }
func writeJSON(w http.ResponseWriter, v interface{}) { w.Header().Set("Content-Type","application/json"); _=json.NewEncoder(w).Encode(v) }
func init(){ _ = fmt.Sprintf("") }
