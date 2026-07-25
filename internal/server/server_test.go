package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func TestParseTimeRangeAcceptsRFC3339Bounds(t *testing.T) {
	db := tempDB(t)
	srv := New(db, "127.0.0.1:0")
	req := httptest.NewRequest(http.MethodGet, "/api/stats?from=2026-07-25T08:00:00.000Z&to=2026-07-25T14:00:00.000Z&tz_offset=-480", nil)
	from, to, _, err := srv.parseTimeRange(req)
	if err != nil { t.Fatalf("parseTimeRange: %v", err) }
	if want := 6 * time.Hour; to.Sub(from) != want { t.Fatalf("range = %s, want %s", to.Sub(from), want) }
	if from.Location() != time.UTC { t.Fatalf("from location = %s, want UTC", from.Location()) }
}

func tempDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestHealthEndpoint(t *testing.T) {
	db := tempDB(t)
	srv := New(db, "127.0.0.1:0")
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", resp["status"])
	}
}

func TestCORSHeaders(t *testing.T) {
	db := tempDB(t)
	srv := New(db, "127.0.0.1:0")
	handler := srv.Handler()

	req := httptest.NewRequest("OPTIONS", "/api/health", nil)
	req.Header.Set("Origin", "tauri://localhost")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "tauri://localhost" {
		t.Fatalf("missing CORS header, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSRejectsDisallowedBrowserOrigin(t *testing.T) {
	db := tempDB(t)
	srv := New(db, "127.0.0.1:0")
	handler := srv.Handler()

	req := httptest.NewRequest("OPTIONS", "/api/health", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestRequestWithoutOriginStillWorks(t *testing.T) {
	db := tempDB(t)
	srv := New(db, "127.0.0.1:0")
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty for no-origin request", got)
	}
}

func TestConfigManagementRoutesAreNotRegistered(t *testing.T) {
	db := tempDB(t)
	handler := New(db, "127.0.0.1:0").Handler()
	for _, path := range []string{"/api/config/profiles", "/api/config/mcp", "/api/config/skills", "/api/config/sync/status", "/api/config/backups"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 404", path, w.Code)
		}
	}
}
