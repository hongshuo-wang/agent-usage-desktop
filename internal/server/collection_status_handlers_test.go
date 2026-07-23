package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func TestCollectionIndexStatusEndpointContract(t *testing.T) {
	db := tempDB(t)
	if _, err := db.UpsertSessionSource(&storage.SessionSource{
		Source: "claude", SessionID: "one", SourceKind: "jsonl", Path: "/sessions/one.jsonl",
		ParserVersion: "v1", CoverageStatus: "partial", SourceStatus: "available",
		MalformedLines: 2, LastIndexedAt: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
	}); err != nil {
		t.Fatalf("UpsertSessionSource: %v", err)
	}
	handler := New(db, "127.0.0.1:0").Handler()

	request := httptest.NewRequest(http.MethodGet, "/api/collection-index-status", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]interface{}
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	wantKeys := []string{
		"status", "last_indexed_at", "source_count", "file_count", "complete_files",
		"partial_files", "missing_files", "rebuild_required_files", "stale_parser_files", "malformed_lines",
	}
	if len(payload) != len(wantKeys) {
		t.Fatalf("response keys = %v, want exactly %v", payload, wantKeys)
	}
	for _, key := range wantKeys {
		if _, ok := payload[key]; !ok {
			t.Errorf("missing response key %q", key)
		}
	}
	if payload["status"] != "partial" || payload["malformed_lines"] != float64(2) {
		t.Errorf("response = %+v", payload)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/collection-index-status", nil)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", recorder.Code)
	}
}
