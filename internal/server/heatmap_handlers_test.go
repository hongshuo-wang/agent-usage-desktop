package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func TestActivityHeatmapEndpointRoutesAndFilters(t *testing.T) {
	db := tempDB(t)
	// 2025-01-01 10:00 UTC is Wednesday hour 10 / 18:00 at UTC+8.
	base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	if err := db.InsertUsageBatch([]*storage.UsageRecord{
		{Source: "claude", SessionID: "s1", Model: "model-a", Timestamp: base, InputTokens: 10, OutputTokens: 5},
		{Source: "codex", SessionID: "s2", Model: "model-b", Timestamp: base, InputTokens: 999, OutputTokens: 999},
	}); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}
	handler := New(db, "127.0.0.1:0").Handler()

	req := httptest.NewRequest(http.MethodGet,
		"/api/activity-heatmap?from=2025-01-01&to=2025-01-01&source=claude&tz_offset=-480", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var cells []storage.HeatmapCell
	if err := json.Unmarshal(w.Body.Bytes(), &cells); err != nil {
		t.Fatalf("decode heatmap response: %v", err)
	}
	want := storage.HeatmapCell{Weekday: 3, Hour: 18, Calls: 1, Tokens: 15}
	if len(cells) != 1 || cells[0] != want {
		t.Fatalf("cells = %+v, want exactly [%+v]", cells, want)
	}

	for _, target := range []string{
		"/api/activity-heatmap?from=2025-01-02&to=2025-01-01",
		"/api/activity-heatmap?tz_offset=abc",
		"/api/activity-heatmap?tz_offset=721",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("GET %s status = %d, want 400: %s", target, w.Code, w.Body.String())
		}
	}
}
