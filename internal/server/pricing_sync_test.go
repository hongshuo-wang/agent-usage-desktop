package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func TestPricingSyncEndpointSyncsAndRepricesUsage(t *testing.T) {
	db := tempDB(t)
	if err := db.InsertUsage(&storage.UsageRecord{
		Source:       "claude",
		SessionID:    "session-1",
		Model:        "model-a",
		InputTokens:  10,
		OutputTokens: 5,
		Timestamp:    time.Now().UTC().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}

	syncCalled := false
	srv := New(db, "127.0.0.1:0", WithPricingSync(func(db *storage.DB) error {
		syncCalled = true
		_, err := db.CreatePricingSnapshot(time.Now().UTC(), "litellm", "test-revision", []storage.PricingSnapshotEntry{{
			Model:              "model-a",
			InputCostPerToken:  0.1,
			OutputCostPerToken: 0.2,
		}})
		return err
	}))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/pricing/sync", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var response struct {
		Status              string     `json:"status"`
		PricingLastSyncedAt *time.Time `json:"pricing_last_synced_at"`
		UnpricedRecords     int        `json:"unpriced_records"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !syncCalled {
		t.Fatal("sync callback was not called")
	}
	if response.Status != "ok" || response.PricingLastSyncedAt == nil || response.UnpricedRecords != 0 {
		t.Errorf("response = %#v, want successful sync with fresh timestamp and no unpriced records", response)
	}

	stats, err := db.GetDashboardStats(time.Time{}, time.Now().UTC().Add(time.Hour), "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.PricedCostUSD != 2 || stats.UnpricedRecords != 0 {
		t.Errorf("stats priced_cost_usd=%v unpriced_records=%d, want 2 and 0", stats.PricedCostUSD, stats.UnpricedRecords)
	}
}

func TestPricingSyncEndpointReturnsJSONErrorWithoutSnapshotOnFailure(t *testing.T) {
	db := tempDB(t)
	srv := New(db, "127.0.0.1:0", WithPricingSync(func(*storage.DB) error {
		return errors.New("download unavailable")
	}))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/pricing/sync", nil))

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s, want %d", w.Code, w.Body.String(), http.StatusBadGateway)
	}
	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if !strings.Contains(response["error"], "pricing sync failed: download unavailable") {
		t.Errorf("error = %q, want sync failure context", response["error"])
	}

	stats, err := db.GetDashboardStats(time.Time{}, time.Now().UTC().Add(time.Hour), "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.PricingLastSyncedAt != nil {
		t.Errorf("pricing_last_synced_at = %v, want nil after failed sync", stats.PricingLastSyncedAt)
	}
}

func TestPricingModelsEndpointReturnsLatestSnapshot(t *testing.T) {
	db := tempDB(t)
	syncedAt := time.Date(2026, 7, 25, 8, 0, 0, 0, time.UTC)
	if _, err := db.CreatePricingSnapshot(syncedAt, "litellm", "revision-1", []storage.PricingSnapshotEntry{
		{Model: "gpt-test", InputCostPerToken: 0.000001, OutputCostPerToken: 0.000002, CacheReadInputTokenCost: 0.0000001, CacheCreationInputTokenCost: 0.000003},
	}); err != nil {
		t.Fatalf("CreatePricingSnapshot: %v", err)
	}

	w := httptest.NewRecorder()
	srv := New(db, "127.0.0.1:0")
	srv.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/pricing/models", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var response struct {
		PricingLastSyncedAt *time.Time `json:"pricing_last_synced_at"`
		Source              string     `json:"source"`
		Revision            string     `json:"revision"`
		Models              []struct {
			Model                       string  `json:"model"`
			InputCostPerToken           float64 `json:"input_cost_per_token"`
			OutputCostPerToken          float64 `json:"output_cost_per_token"`
			CacheReadInputTokenCost     float64 `json:"cache_read_input_token_cost"`
			CacheCreationInputTokenCost float64 `json:"cache_creation_input_token_cost"`
		} `json:"models"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.PricingLastSyncedAt == nil || !response.PricingLastSyncedAt.Equal(syncedAt) {
		t.Errorf("pricing_last_synced_at = %v, want %v", response.PricingLastSyncedAt, syncedAt)
	}
	if response.Source != "litellm" || response.Revision != "revision-1" || len(response.Models) != 1 {
		t.Fatalf("response metadata/models = %#v, want latest snapshot", response)
	}
	if response.Models[0].Model != "gpt-test" || response.Models[0].InputCostPerToken != 0.000001 {
		t.Errorf("model = %#v, want gpt-test pricing", response.Models[0])
	}
}
