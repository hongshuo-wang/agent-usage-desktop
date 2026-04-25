package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/configmanager"
	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

// Server serves the REST API.
type Server struct {
	db   *storage.DB
	mgr  *configmanager.Manager
	addr string
}

// New creates a Server that will listen on the given address (host:port).
func New(db *storage.DB, mgr *configmanager.Manager, addr string) *Server {
	return &Server{db: db, mgr: mgr, addr: addr}
}

var allowedCORSOrigins = map[string]bool{
	"tauri://localhost":       true,
	"http://tauri.localhost":  true,
	"https://tauri.localhost": true,
	"http://localhost:1420":   true,
	"http://127.0.0.1:1420":   true,
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !allowedCORSOrigins[origin] {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Handler builds and returns the HTTP handler with all routes and middleware.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/cost-by-model", s.handleCostByModel)
	mux.HandleFunc("/api/cost-over-time", s.handleCostOverTime)
	mux.HandleFunc("/api/tokens-over-time", s.handleTokensOverTime)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/session-detail", s.handleSessionDetail)
	mux.HandleFunc("GET /api/config/profiles", s.handleListProfiles)
	mux.HandleFunc("POST /api/config/profiles", s.handleCreateProfile)
	mux.HandleFunc("PUT /api/config/profiles/{id}", s.handleUpdateProfile)
	mux.HandleFunc("DELETE /api/config/profiles/{id}", s.handleDeleteProfile)
	mux.HandleFunc("POST /api/config/profiles/{id}/activate", s.handleActivateProfile)
	mux.HandleFunc("GET /api/config/mcp", s.handleListMCPServers)
	mux.HandleFunc("POST /api/config/mcp", s.handleCreateMCPServer)
	mux.HandleFunc("PUT /api/config/mcp/{id}", s.handleUpdateMCPServer)
	mux.HandleFunc("DELETE /api/config/mcp/{id}", s.handleDeleteMCPServer)
	mux.HandleFunc("PUT /api/config/mcp/{id}/targets", s.handleSetMCPTargets)
	mux.HandleFunc("GET /api/config/skills", s.handleListSkills)
	mux.HandleFunc("GET /api/config/skills/inventory", s.handleSkillsInventory)
	mux.HandleFunc("POST /api/config/skills/import", s.handleImportSkills)
	mux.HandleFunc("POST /api/config/skills/conflicts/resolve", s.handleResolveSkillConflict)
	mux.HandleFunc("POST /api/config/skills", s.handleCreateSkill)
	mux.HandleFunc("PUT /api/config/skills/{id}", s.handleUpdateSkill)
	mux.HandleFunc("DELETE /api/config/skills/{id}", s.handleDeleteSkill)
	mux.HandleFunc("PUT /api/config/skills/{id}/targets", s.handleSetSkillTargets)
	mux.HandleFunc("POST /api/config/sync", s.handleTriggerSync)
	mux.HandleFunc("GET /api/config/sync/status", s.handleSyncStatus)
	mux.HandleFunc("POST /api/config/sync/resolve", s.handleResolveConflict)
	mux.HandleFunc("GET /api/config/backups", s.handleListBackups)
	mux.HandleFunc("POST /api/config/backups", s.handleManualBackup)
	mux.HandleFunc("POST /api/config/backups/{id}/restore", s.handleRestoreBackup)
	mux.HandleFunc("GET /api/config/files", s.handleListConfigFiles)

	return corsMiddleware(mux)
}

// Start registers HTTP handlers and begins listening. It blocks until the server stops.
func (s *Server) Start() error {
	log.Printf("server: listening on %s", s.addr)
	return http.ListenAndServe(s.addr, s.Handler())
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) parseTimeRange(r *http.Request) (time.Time, time.Time, int, error) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	// Parse tz_offset (minutes, JS getTimezoneOffset convention: UTC+8 = -480)
	tzOffset := 0
	if tzStr := r.URL.Query().Get("tz_offset"); tzStr != "" {
		fmt.Sscanf(tzStr, "%d", &tzOffset)
	}

	var fromTime, toTime time.Time
	var err error
	if from != "" {
		fromTime, err = time.Parse("2006-01-02", from)
		if err != nil {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid 'from' date %q: expected YYYY-MM-DD", from)
		}
	}
	if to != "" {
		toTime, err = time.Parse("2006-01-02", to)
		if err != nil {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid 'to' date %q: expected YYYY-MM-DD", to)
		}
		toTime = toTime.Add(24*time.Hour - time.Second)
	}
	if fromTime.IsZero() {
		fromTime = time.Now().AddDate(0, -1, 0)
	}
	if toTime.IsZero() {
		toTime = time.Now().Add(24 * time.Hour)
	}

	// Apply timezone offset: convert local day boundaries to UTC
	if tzOffset != 0 {
		offset := time.Duration(tzOffset) * time.Minute
		fromTime = fromTime.Add(offset)
		toTime = toTime.Add(offset)
	}

	if fromTime.After(toTime) {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("'from' date (%s) is after 'to' date (%s): swap them or correct the range", from, to)
	}
	return fromTime, toTime, tzOffset, nil
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("api error: %v", err)
	http.Error(w, "internal server error", 500)
}

func badRequest(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(400)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	source := r.URL.Query().Get("source")
	stats, err := s.db.GetDashboardStats(from, to, source)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleCostByModel(w http.ResponseWriter, r *http.Request) {
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	source := r.URL.Query().Get("source")
	data, err := s.db.GetCostByModel(from, to, source)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleCostOverTime(w http.ResponseWriter, r *http.Request) {
	from, to, tzOffset, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	granularity := r.URL.Query().Get("granularity")
	source := r.URL.Query().Get("source")
	data, err := s.db.GetCostOverTime(from, to, granularity, source, tzOffset)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleTokensOverTime(w http.ResponseWriter, r *http.Request) {
	from, to, tzOffset, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	granularity := r.URL.Query().Get("granularity")
	source := r.URL.Query().Get("source")
	data, err := s.db.GetTokensOverTime(from, to, granularity, source, tzOffset)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	source := r.URL.Query().Get("source")
	data, err := s.db.GetSessions(from, to, source)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	sid := r.URL.Query().Get("session_id")
	if sid == "" {
		http.Error(w, "session_id required", 400)
		return
	}
	data, err := s.db.GetSessionDetail(sid)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, data)
}
