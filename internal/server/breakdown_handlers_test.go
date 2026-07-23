package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func TestUsageBreakdownEndpointContractAndValidation(t *testing.T) {
	db := tempDB(t)
	timestamp := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&storage.UsageRecord{
		Source: "claude", SessionID: "one", Model: "missing-price", Project: "alpha",
		Timestamp: timestamp, InputTokens: 1, CacheReadInputTokens: 2,
		CacheCreationInputTokens: 3, OutputTokens: 4,
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	handler := New(db, "127.0.0.1:0").Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/usage-breakdown?dimension=source&from=2025-01-01&to=2025-01-01", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var rows []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&rows); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want one", rows)
	}
	wantKeys := []string{"key", "total_tokens", "total_cost", "sessions", "calls", "cache_hit_rate", "unknown_price"}
	if len(rows[0]) != len(wantKeys) {
		t.Fatalf("response keys = %v, want exactly %v", rows[0], wantKeys)
	}
	for _, key := range wantKeys {
		if _, ok := rows[0][key]; !ok {
			t.Errorf("missing response key %q", key)
		}
	}

	for _, target := range []string{
		"/api/usage-breakdown?dimension=invalid",
		"/api/usage-breakdown",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("GET %s status = %d, want 400", target, w.Code)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/usage-breakdown?dimension=source", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", recorder.Code)
	}
}
