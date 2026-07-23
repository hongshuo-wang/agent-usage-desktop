package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func TestThroughputEndpointReturnsLocalObservedFilteredMetrics(t *testing.T) {
	db := tempDB(t)
	base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	if err := db.InsertUsageBatch([]*storage.UsageRecord{
		{
			Source: "claude", SessionID: "s1", Model: "model-a",
			Timestamp: base.Add(59 * time.Second), InputTokens: 10,
			CacheReadInputTokens: 20, CacheCreationInputTokens: 30, OutputTokens: 40,
		},
		{
			Source: "claude", SessionID: "s1", Model: "model-a",
			Timestamp: base.Add(time.Minute + time.Second), InputTokens: 1,
			CacheReadInputTokens: 2, CacheCreationInputTokens: 3, OutputTokens: 4,
		},
		{
			Source: "codex", SessionID: "s2", Model: "model-b",
			Timestamp: base.Add(time.Minute), InputTokens: 999, OutputTokens: 999,
		},
	}); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet,
		"/api/throughput?from=2025-01-01&to=2025-01-01&source=claude&model=model-a&tz_offset=-480", nil)
	w := httptest.NewRecorder()
	New(db, "127.0.0.1:0").Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	body := w.Body.Bytes()
	for _, forbidden := range []string{`"quota"`, `"limit"`, `"remaining"`, `"utilization"`} {
		if strings.Contains(string(body), forbidden) {
			t.Errorf("local observed response contains provider-capacity field %s: %s", forbidden, body)
		}
	}

	var shape map[string]json.RawMessage
	if err := json.Unmarshal(body, &shape); err != nil {
		t.Fatalf("decode response shape: %v", err)
	}
	wantTopLevel := []string{"average_active_minute", "peak_rolling_60s", "p95_rolling_60s", "series"}
	if len(shape) != len(wantTopLevel) {
		t.Fatalf("top-level fields = %v, want exactly %v", mapKeys(shape), wantTopLevel)
	}
	for _, key := range wantTopLevel {
		if _, ok := shape[key]; !ok {
			t.Errorf("missing top-level field %q", key)
		}
	}

	var result storage.ThroughputResult
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode throughput response: %v", err)
	}
	if result.PeakRolling60s.RPM != 2 || result.PeakRolling60s.TotalTPM != 110 {
		t.Errorf("rolling peak = %+v, want filtered local-observed RPM 2 and total TPM 110", result.PeakRolling60s)
	}
	if len(result.Series) != 2 || result.Series[0].Minute != "2025-01-01 18:00" || result.Series[1].Minute != "2025-01-01 18:01" {
		t.Errorf("local-minute series = %+v", result.Series)
	}

	var peakShape map[string]json.RawMessage
	if err := json.Unmarshal(shape["peak_rolling_60s"], &peakShape); err != nil {
		t.Fatalf("decode peak shape: %v", err)
	}
	wantValueKeys := []string{"rpm", "input_tpm", "cache_read_tpm", "cache_create_tpm", "output_tpm", "total_tpm"}
	if len(peakShape) != len(wantValueKeys) {
		t.Fatalf("throughput fields = %v, want exactly %v", mapKeys(peakShape), wantValueKeys)
	}
	for _, key := range wantValueKeys {
		if _, ok := peakShape[key]; !ok {
			t.Errorf("missing throughput field %q", key)
		}
	}
}

func TestThroughputEndpointRejectsInvalidRangesOffsetsAndMethods(t *testing.T) {
	handler := New(tempDB(t), "127.0.0.1:0").Handler()
	for _, target := range []string{
		"/api/throughput?from=2025-01-02&to=2025-01-01",
		"/api/throughput?tz_offset=abc",
		"/api/throughput?tz_offset=12junk",
		"/api/throughput?tz_offset=721",
		"/api/throughput?tz_offset=-841",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("GET %s status = %d, want 400: %s", target, w.Code, w.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/throughput", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/throughput status = %d, want 405", w.Code)
	}
}

func TestTimeSeriesEndpointsRejectMalformedTimezoneOffset(t *testing.T) {
	handler := New(tempDB(t), "127.0.0.1:0").Handler()
	for _, path := range []string{"/api/cost-over-time", "/api/tokens-over-time"} {
		req := httptest.NewRequest(http.MethodGet, path+"?tz_offset=not-a-number", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("GET %s malformed tz_offset status = %d, want 400", path, w.Code)
		}
	}
}

func mapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
