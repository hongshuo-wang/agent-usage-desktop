package pricing

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

const pricingURL = "https://cdn.jsdelivr.net/gh/BerriAI/litellm@main/model_prices_and_context_window.json"

//go:embed model_prices_and_context_window.json
var bundledPricingJSON []byte

// MaxPricingImportBytes bounds uploaded pricing documents while leaving ample
// room for the current LiteLLM model catalog and future growth.
const MaxPricingImportBytes int64 = 64 << 20

type modelPricing struct {
	InputCostPerToken           *float64 `json:"input_cost_per_token"`
	OutputCostPerToken          *float64 `json:"output_cost_per_token"`
	CacheReadInputTokenCost     *float64 `json:"cache_read_input_token_cost"`
	CacheCreationInputTokenCost *float64 `json:"cache_creation_input_token_cost"`
}

// Sync fetches model pricing from the LiteLLM jsDelivr mirror and stores the
// full response as one immutable pricing snapshot.
func Sync(db *storage.DB) error {
	client := &http.Client{Timeout: 30 * time.Second}
	if err := syncFromURL(db, client, pricingURL); err != nil {
		if fallbackErr := EnsureBundledPricing(db); fallbackErr != nil {
			return fmt.Errorf("sync pricing: %w; load bundled pricing: %v", err, fallbackErr)
		}
		return err
	}
	return nil
}

// EnsureBundledPricing installs the release snapshot only when no local
// pricing catalog exists. Online syncs and manual imports always take priority.
func EnsureBundledPricing(db *storage.DB) error {
	catalog, err := db.GetLatestPricingCatalog()
	if err != nil {
		return err
	}
	if catalog != nil {
		return nil
	}

	entries, revision, err := ParsePricingJSON(bundledPricingJSON)
	if err != nil {
		return fmt.Errorf("decode bundled pricing: %w", err)
	}
	if _, err := db.CreatePricingSnapshot(time.Now().UTC(), "litellm", "bundled:"+revision, entries); err != nil {
		return err
	}
	log.Printf("pricing: loaded %d models from bundled snapshot", len(entries))
	return nil
}

func syncFromURL(db *storage.DB, client *http.Client, url string) error {
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("pricing request returned %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxPricingImportBytes+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > MaxPricingImportBytes {
		return fmt.Errorf("pricing response exceeds maximum size of %d bytes", MaxPricingImportBytes)
	}

	entries, bodyRevision, err := ParsePricingJSON(body)
	if err != nil {
		return fmt.Errorf("decode pricing response: %w", err)
	}

	revision := resp.Header.Get("ETag")
	if revision == "" {
		revision = bodyRevision
	}
	if _, err := db.CreatePricingSnapshot(time.Now().UTC(), "litellm", revision, entries); err != nil {
		return err
	}
	log.Printf("pricing: synced %d models", len(entries))
	return nil
}

// ParsePricingJSON validates and extracts the token pricing entries from a
// LiteLLM model_prices_and_context_window.json document. The returned revision
// is a stable SHA-256 hash of the exact document bytes.
func ParsePricingJSON(body []byte) ([]storage.PricingSnapshotEntry, string, error) {
	var data map[string]json.RawMessage
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, "", err
	}

	entries := make([]storage.PricingSnapshotEntry, 0, len(data))
	for model, raw := range data {
		var p modelPricing
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, "", fmt.Errorf("decode pricing entry %q: %w", model, err)
		}
		if p.InputCostPerToken == nil || p.OutputCostPerToken == nil {
			continue
		}

		var cacheRead, cacheCreate float64
		if p.CacheReadInputTokenCost != nil {
			cacheRead = *p.CacheReadInputTokenCost
		}
		if p.CacheCreationInputTokenCost != nil {
			cacheCreate = *p.CacheCreationInputTokenCost
		}

		entries = append(entries, storage.PricingSnapshotEntry{
			Model:                       model,
			InputCostPerToken:           *p.InputCostPerToken,
			OutputCostPerToken:          *p.OutputCostPerToken,
			CacheReadInputTokenCost:     cacheRead,
			CacheCreationInputTokenCost: cacheCreate,
		})
	}
	if len(entries) == 0 {
		return nil, "", fmt.Errorf("pricing response contains no valid pricing entries")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Model < entries[j].Model })
	return entries, fmt.Sprintf("%x", sha256.Sum256(body)), nil
}

// ImportResult describes a manually imported pricing snapshot.
type ImportResult struct {
	SnapshotID int64  `json:"snapshot_id"`
	Entries    int    `json:"entries"`
	Revision   string `json:"revision"`
}

// ImportPricingJSON stores an uploaded LiteLLM pricing document as an
// immutable snapshot and immediately retries pricing all unpriced usage.
func ImportPricingJSON(db *storage.DB, body []byte) (ImportResult, error) {
	if int64(len(body)) > MaxPricingImportBytes {
		return ImportResult{}, fmt.Errorf("pricing document exceeds maximum size of %d bytes", MaxPricingImportBytes)
	}
	entries, revision, err := ParsePricingJSON(body)
	if err != nil {
		return ImportResult{}, err
	}
	snapshotID, err := db.CreatePricingSnapshot(time.Now().UTC(), "litellm", revision, entries)
	if err != nil {
		return ImportResult{}, err
	}
	if err := db.PriceUnpricedUsageWithHistoricalFallback(CalcCost); err != nil {
		return ImportResult{}, fmt.Errorf("price usage after pricing import: %w", err)
	}
	log.Printf("pricing: imported %d models", len(entries))
	return ImportResult{SnapshotID: snapshotID, Entries: len(entries), Revision: revision}, nil
}

// CalcCost computes the USD cost for a single API call given token counts and
// per-token prices. The prices array is [input, output, cache_read, cache_creation].
// input_tokens is the non-cached input only (cache tokens are separate fields).
func CalcCost(inputTokens, outputTokens, cacheCreation, cacheRead int64, prices [4]float64) float64 {
	inputPrice := prices[0]
	outputPrice := prices[1]
	cacheReadPrice := prices[2]
	cacheCreatePrice := prices[3]

	cost := float64(inputTokens)*inputPrice +
		float64(cacheCreation)*cacheCreatePrice +
		float64(cacheRead)*cacheReadPrice +
		float64(outputTokens)*outputPrice
	return cost
}
