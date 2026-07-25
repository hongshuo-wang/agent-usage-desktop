package pricing

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func openPricingTestDB(t *testing.T) (*storage.DB, *sql.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "pricing.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ledger, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open ledger for verification: %v", err)
	}
	t.Cleanup(func() { ledger.Close() })
	return db, ledger
}

func newPricingServer(t *testing.T, status int, etag, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if etag != "" {
			w.Header().Set("ETag", etag)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func snapshotCount(t *testing.T, ledger *sql.DB) int {
	t.Helper()
	var count int
	if err := ledger.QueryRow(`SELECT COUNT(*) FROM pricing_snapshots`).Scan(&count); err != nil {
		t.Fatalf("count pricing snapshots: %v", err)
	}
	return count
}

func TestPricingSyncCreatesOneAtomicSnapshot(t *testing.T) {
	db, ledger := openPricingTestDB(t)
	body := `{
		"model-a": {
			"input_cost_per_token": 0.1,
			"output_cost_per_token": 0.2,
			"cache_read_input_token_cost": 0.03,
			"cache_creation_input_token_cost": 0.04
		},
		"model-b": {
			"input_cost_per_token": 1.1,
			"output_cost_per_token": 1.2
		}
	}`
	server := newPricingServer(t, http.StatusOK, `"revision-123"`, body)

	for call := 1; call <= 2; call++ {
		if err := syncFromURL(db, server.Client(), server.URL); err != nil {
			t.Fatalf("sync call %d: %v", call, err)
		}
	}

	var snapshotCount, entryCount, mutablePricingCount int
	if err := ledger.QueryRow(`SELECT COUNT(*) FROM pricing_snapshots`).Scan(&snapshotCount); err != nil {
		t.Fatalf("read snapshots: %v", err)
	}
	if err := ledger.QueryRow(`SELECT COUNT(*) FROM pricing_snapshot_entries`).Scan(&entryCount); err != nil {
		t.Fatalf("count snapshot entries: %v", err)
	}
	if err := ledger.QueryRow(`SELECT COUNT(*) FROM pricing`).Scan(&mutablePricingCount); err != nil {
		t.Fatalf("count mutable pricing rows: %v", err)
	}
	if snapshotCount != 2 {
		t.Errorf("snapshot count = %d, want one per successful call (2)", snapshotCount)
	}
	if entryCount != 4 {
		t.Errorf("snapshot entry count = %d, want 2 entries in each of 2 snapshots", entryCount)
	}
	if mutablePricingCount != 0 {
		t.Errorf("mutable pricing row count = %d, want 0", mutablePricingCount)
	}

	rows, err := ledger.Query(`SELECT source_revision, COUNT(*) FROM pricing_snapshots AS snapshots
		JOIN pricing_snapshot_entries AS entries ON entries.snapshot_id=snapshots.id
		GROUP BY snapshots.id ORDER BY snapshots.id`)
	if err != nil {
		t.Fatalf("read atomic snapshots: %v", err)
	}
	defer rows.Close()
	for snapshot := 1; rows.Next(); snapshot++ {
		var revision string
		var entries int
		if err := rows.Scan(&revision, &entries); err != nil {
			t.Fatalf("scan snapshot %d: %v", snapshot, err)
		}
		if revision != `"revision-123"` || entries != 2 {
			t.Errorf("snapshot %d: revision=%q entries=%d, want ETag and 2 entries", snapshot, revision, entries)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate snapshots: %v", err)
	}

	var input, output, cacheRead, cacheCreation float64
	if err := ledger.QueryRow(`SELECT input_cost_per_token, output_cost_per_token,
		cache_read_input_token_cost, cache_creation_input_token_cost
		FROM pricing_snapshot_entries WHERE model='model-a' ORDER BY snapshot_id DESC LIMIT 1`).Scan(
		&input, &output, &cacheRead, &cacheCreation); err != nil {
		t.Fatalf("read persisted prices: %v", err)
	}
	if input != 0.1 || output != 0.2 || cacheRead != 0.03 || cacheCreation != 0.04 {
		t.Errorf("persisted prices = [%v %v %v %v], want [0.1 0.2 0.03 0.04]",
			input, output, cacheRead, cacheCreation)
	}
}

func TestPricingSyncUsesBodySHA256WithoutETag(t *testing.T) {
	db, ledger := openPricingTestDB(t)
	body := "{\n  \"model-a\": {\"input_cost_per_token\": 0.1, \"output_cost_per_token\": 0.2}\n}\n"
	server := newPricingServer(t, http.StatusOK, "", body)

	if err := syncFromURL(db, server.Client(), server.URL); err != nil {
		t.Fatalf("syncFromURL: %v", err)
	}

	var revision string
	if err := ledger.QueryRow(`SELECT source_revision FROM pricing_snapshots`).Scan(&revision); err != nil {
		t.Fatalf("read source revision: %v", err)
	}
	want := fmt.Sprintf("%x", sha256.Sum256([]byte(body)))
	if revision != want {
		t.Errorf("source revision = %q, want exact-body SHA-256 %q", revision, want)
	}
}

func TestPricingSyncRejectsMalformedOrEmptyPayloadWithoutSnapshot(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{
			name:    "malformed entry",
			body:    `{"model-a":{"input_cost_per_token":0.1,"output_cost_per_token":0.2},"broken":"not pricing"}`,
			wantErr: "broken",
		},
		{
			name:    "empty payload",
			body:    `{}`,
			wantErr: "no valid pricing entries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, ledger := openPricingTestDB(t)
			if _, err := db.CreatePricingSnapshot(time.Now().Add(-time.Hour), "litellm", "existing", []storage.PricingSnapshotEntry{{
				Model: "existing", InputCostPerToken: 1, OutputCostPerToken: 2,
			}}); err != nil {
				t.Fatalf("seed pricing snapshot: %v", err)
			}
			before := snapshotCount(t, ledger)
			server := newPricingServer(t, http.StatusOK, `"invalid"`, tt.body)

			err := syncFromURL(db, server.Client(), server.URL)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("sync error = %v, want error containing %q", err, tt.wantErr)
			}
			if after := snapshotCount(t, ledger); after != before {
				t.Errorf("snapshot count after validation failure = %d, want unchanged %d", after, before)
			}
		})
	}
}

func TestPricingSyncSkipsUnsupportedEntries(t *testing.T) {
	db, ledger := openPricingTestDB(t)
	body := `{
		"token-model":{"input_cost_per_token":0.1,"output_cost_per_token":0.2},
		"image-model":{"output_cost_per_token":0.3},
		"embedding-model":{"input_cost_per_token":0.4}
	}`
	server := newPricingServer(t, http.StatusOK, `"mixed"`, body)

	if err := syncFromURL(db, server.Client(), server.URL); err != nil {
		t.Fatalf("syncFromURL mixed payload: %v", err)
	}

	var count int
	var model string
	if err := ledger.QueryRow(`SELECT COUNT(*), COALESCE(MAX(model), '') FROM pricing_snapshot_entries`).Scan(&count, &model); err != nil {
		t.Fatalf("read persisted pricing entries: %v", err)
	}
	if count != 1 || model != "token-model" {
		t.Errorf("persisted entries = count %d model %q, want only token-model", count, model)
	}
	if got := snapshotCount(t, ledger); got != 1 {
		t.Errorf("snapshot count = %d, want 1", got)
	}
}

func TestPricingSyncRejectsMalformedNumericPriceWithoutSnapshot(t *testing.T) {
	db, ledger := openPricingTestDB(t)
	body := `{
		"token-model":{"input_cost_per_token":0.1,"output_cost_per_token":0.2},
		"broken":{"input_cost_per_token":"not-a-number","output_cost_per_token":0.3}
	}`
	server := newPricingServer(t, http.StatusOK, `"malformed"`, body)

	err := syncFromURL(db, server.Client(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("sync error = %v, want contextual error for broken model", err)
	}
	if got := snapshotCount(t, ledger); got != 0 {
		t.Errorf("snapshot count = %d after malformed price, want 0", got)
	}
}

func TestPricingSyncRejectsAllUnsupportedEntriesWithoutSnapshot(t *testing.T) {
	db, ledger := openPricingTestDB(t)
	body := `{
		"image-model":{"output_cost_per_token":0.3},
		"embedding-model":{"input_cost_per_token":0.4}
	}`
	server := newPricingServer(t, http.StatusOK, `"unsupported"`, body)

	err := syncFromURL(db, server.Client(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "no valid pricing entries") {
		t.Fatalf("sync error = %v, want no valid pricing entries error", err)
	}
	if got := snapshotCount(t, ledger); got != 0 {
		t.Errorf("snapshot count = %d after all-unsupported payload, want 0", got)
	}
}

func TestPricingSyncRejectsNonSuccessWithoutSnapshot(t *testing.T) {
	db, ledger := openPricingTestDB(t)
	server := newPricingServer(t, http.StatusBadGateway, "", `{"message":"unavailable"}`)

	if err := syncFromURL(db, server.Client(), server.URL); err == nil {
		t.Fatal("syncFromURL accepted a non-success response")
	}
	if got := snapshotCount(t, ledger); got != 0 {
		t.Errorf("snapshot count = %d after non-success response, want 0", got)
	}
}

func TestParsePricingJSONReturnsEntriesAndRevision(t *testing.T) {
	body := []byte(`{"model-a":{"input_cost_per_token":0.1,"output_cost_per_token":0.2,"cache_read_input_token_cost":0.03},"unsupported":{"input_cost_per_token":0.4}}`)
	entries, revision, err := ParsePricingJSON(body)
	if err != nil {
		t.Fatalf("ParsePricingJSON: %v", err)
	}
	if len(entries) != 1 || entries[0].Model != "model-a" {
		t.Fatalf("entries = %#v, want model-a only", entries)
	}
	wantRevision := fmt.Sprintf("%x", sha256.Sum256(body))
	if revision != wantRevision {
		t.Errorf("revision = %q, want %q", revision, wantRevision)
	}
}

func TestImportPricingJSONCreatesSnapshotAndPricesUsage(t *testing.T) {
	db, ledger := openPricingTestDB(t)
	if err := db.InsertUsage(&storage.UsageRecord{
		Source: "claude", SessionID: "import-session", Model: "model-a",
		InputTokens: 10, OutputTokens: 5, Timestamp: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	body := []byte(`{"model-a":{"input_cost_per_token":0.1,"output_cost_per_token":0.2}}`)
	result, err := ImportPricingJSON(db, body)
	if err != nil {
		t.Fatalf("ImportPricingJSON: %v", err)
	}
	if result.SnapshotID == 0 || result.Entries != 1 {
		t.Fatalf("result = %#v, want snapshot and one entry", result)
	}
	var status string
	var cost float64
	if err := ledger.QueryRow(`SELECT pricing_status, cost_usd FROM usage_records WHERE session_id='import-session'`).Scan(&status, &cost); err != nil {
		t.Fatalf("read usage: %v", err)
	}
	if status != "priced" || cost != 2 {
		t.Errorf("usage = status %q cost %v, want priced and 2", status, cost)
	}
}

func TestCalcCost_Basic(t *testing.T) {
	// prices: [input, output, cache_read, cache_creation]
	prices := [4]float64{0.003, 0.015, 0.001, 0.004}

	cost := CalcCost(1000, 500, 0, 0, prices)
	// 1000 * 0.003 + 500 * 0.015 = 3.0 + 7.5 = 10.5
	if cost != 10.5 {
		t.Errorf("expected 10.5, got %f", cost)
	}
}

func TestCalcCost_WithCache(t *testing.T) {
	prices := [4]float64{0.003, 0.015, 0.001, 0.004}

	// input=500 (non-cached), output=500, cacheCreation=200, cacheRead=300
	// cost = 500*0.003 + 200*0.004 + 300*0.001 + 500*0.015
	//      = 1.5 + 0.8 + 0.3 + 7.5 = 10.1
	cost := CalcCost(500, 500, 200, 300, prices)
	expected := 10.1
	if diff := cost - expected; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("expected %f, got %f", expected, cost)
	}
}

func TestCalcCost_ZeroNonCachedInput(t *testing.T) {
	prices := [4]float64{0.003, 0.015, 0.001, 0.004}

	// All input is cached, non-cached input = 0
	cost := CalcCost(0, 500, 200, 300, prices)
	// cost = 0 + 200*0.004 + 300*0.001 + 500*0.015 = 0.8 + 0.3 + 7.5 = 8.6
	expected := 8.6
	if diff := cost - expected; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("expected %f, got %f", expected, cost)
	}
}

func TestCalcCost_ZeroTokens(t *testing.T) {
	prices := [4]float64{0.003, 0.015, 0.001, 0.004}
	cost := CalcCost(0, 0, 0, 0, prices)
	if cost != 0 {
		t.Errorf("expected 0, got %f", cost)
	}
}

func TestCalcCost_ZeroPrices(t *testing.T) {
	prices := [4]float64{0, 0, 0, 0}
	cost := CalcCost(1000, 500, 200, 300, prices)
	if cost != 0 {
		t.Errorf("expected 0, got %f", cost)
	}
}
