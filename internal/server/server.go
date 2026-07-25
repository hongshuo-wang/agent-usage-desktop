package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/pricing"
	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

// Server serves the REST API.
type Server struct {
	db          *storage.DB
	addr        string
	configPath  string
	configMu    sync.Mutex
	pricingSync func(*storage.DB) error
}

// Option configures optional server features.
type Option func(*Server)

// WithConfigPath enables app-settings reads and atomic mutations at path.
func WithConfigPath(path string) Option {
	return func(server *Server) {
		server.configPath = path
	}
}

// WithPricingSync replaces the pricing download operation. It is primarily
// useful for tests; production servers use pricing.Sync by default.
func WithPricingSync(syncFunc func(*storage.DB) error) Option {
	return func(server *Server) {
		if syncFunc != nil {
			server.pricingSync = syncFunc
		}
	}
}

// New creates a Server that will listen on the given address (host:port).
func New(db *storage.DB, addr string, options ...Option) *Server {
	server := &Server{db: db, addr: addr, pricingSync: pricing.Sync}
	for _, option := range options {
		option(server)
	}
	return server
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
	mux.HandleFunc("GET /api/throughput", s.handleThroughput)
	mux.HandleFunc("GET /api/usage-breakdown", s.handleUsageBreakdown)
	mux.HandleFunc("GET /api/collection-index-status", s.handleCollectionIndexStatus)
	mux.HandleFunc("GET /api/settings/collectors", s.handleCollectorSettingsGet)
	mux.HandleFunc("PUT /api/settings/collectors", s.handleCollectorSettingsPut)
	mux.HandleFunc("GET /api/sessions", s.handleSessionSearch)
	mux.HandleFunc("GET /api/sessions/{source}/{session_id}/events", s.handleSessionEventsRoute)
	mux.HandleFunc("GET /api/sessions/{source}/{session_id}/events/{event_id}/raw", s.handleSessionRawRoute)
	mux.HandleFunc("POST /api/session-index/rebuild", s.handleSessionIndexRebuild)
	mux.HandleFunc("POST /api/pricing/import", s.handlePricingImport)
	mux.HandleFunc("POST /api/pricing/sync", s.handlePricingSync)
	mux.HandleFunc("GET /api/pricing/models", s.handlePricingModels)

	return corsMiddleware(mux)
}

func (s *Server) handlePricingModels(w http.ResponseWriter, _ *http.Request) {
	catalog, err := s.db.GetLatestPricingCatalog()
	if err != nil {
		serverError(w, err)
		return
	}
	response := struct {
		PricingLastSyncedAt *time.Time                     `json:"pricing_last_synced_at"`
		Source              string                         `json:"source"`
		Revision            string                         `json:"revision"`
		Models              []storage.PricingSnapshotEntry `json:"models"`
	}{Models: []storage.PricingSnapshotEntry{}}
	if catalog != nil {
		response.PricingLastSyncedAt = &catalog.SyncedAt
		response.Source = catalog.Source
		response.Revision = catalog.Revision
		response.Models = catalog.Entries
	}
	writeJSON(w, response)
}

func (s *Server) handlePricingSync(w http.ResponseWriter, _ *http.Request) {
	if err := s.pricingSync(s.db); err != nil {
		badRequestStatus(w, http.StatusBadGateway, fmt.Errorf("pricing sync failed: %w", err))
		return
	}
	if err := s.db.PriceUnpricedUsageWithHistoricalFallback(pricing.CalcCost); err != nil {
		badRequestStatus(w, http.StatusInternalServerError, fmt.Errorf("price usage after pricing sync: %w", err))
		return
	}
	status, err := s.db.GetPricingStatus()
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, struct {
		Status              string     `json:"status"`
		PricingLastSyncedAt *time.Time `json:"pricing_last_synced_at"`
		UnpricedRecords     int        `json:"unpriced_records"`
	}{
		Status:              "ok",
		PricingLastSyncedAt: status.PricingLastSyncedAt,
		UnpricedRecords:     status.UnpricedRecords,
	})
}

func (s *Server) handlePricingImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnsupportedMediaType)
		json.NewEncoder(w).Encode(map[string]string{"error": "Content-Type must be application/json"})
		return
	}
	if r.ContentLength > pricing.MaxPricingImportBytes {
		badRequestStatus(w, http.StatusRequestEntityTooLarge, fmt.Errorf("pricing file exceeds maximum size of %d bytes", pricing.MaxPricingImportBytes))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, pricing.MaxPricingImportBytes+1)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if err == io.ErrUnexpectedEOF || strings.Contains(err.Error(), "request body too large") {
			badRequestStatus(w, http.StatusRequestEntityTooLarge, fmt.Errorf("pricing file exceeds maximum size of %d bytes", pricing.MaxPricingImportBytes))
			return
		}
		badRequest(w, fmt.Errorf("read pricing file: %w", err))
		return
	}
	if int64(len(body)) > pricing.MaxPricingImportBytes {
		badRequestStatus(w, http.StatusRequestEntityTooLarge, fmt.Errorf("pricing file exceeds maximum size of %d bytes", pricing.MaxPricingImportBytes))
		return
	}
	result, err := pricing.ImportPricingJSON(s.db, body)
	if err != nil {
		badRequest(w, err)
		return
	}
	writeJSON(w, result)
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
		var err error
		tzOffset, err = strconv.Atoi(tzStr)
		if err != nil || tzOffset < -840 || tzOffset > 720 {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid 'tz_offset' %q: expected minutes between -840 and 720", tzStr)
		}
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
	badRequestStatus(w, http.StatusBadRequest, err)
}

func badRequestStatus(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
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

func (s *Server) handleUsageBreakdown(w http.ResponseWriter, r *http.Request) {
	dimension := r.URL.Query().Get("dimension")
	switch dimension {
	case "source", "model", "project":
	default:
		badRequest(w, fmt.Errorf("dimension must be source, model, or project"))
		return
	}
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	data, err := s.db.GetUsageBreakdown(from, to, r.URL.Query().Get("source"), dimension)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, data)
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
