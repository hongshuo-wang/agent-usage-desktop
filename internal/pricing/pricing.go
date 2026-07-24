package pricing

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

const pricingURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

type modelPricing struct {
	InputCostPerToken           *float64 `json:"input_cost_per_token"`
	OutputCostPerToken          *float64 `json:"output_cost_per_token"`
	CacheReadInputTokenCost     *float64 `json:"cache_read_input_token_cost"`
	CacheCreationInputTokenCost *float64 `json:"cache_creation_input_token_cost"`
}

// Sync fetches model pricing from the litellm GitHub repository and stores the
// full response as one immutable pricing snapshot.
func Sync(db *storage.DB) error {
	client := &http.Client{Timeout: 30 * time.Second}
	return syncFromURL(db, client, pricingURL)
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var data map[string]json.RawMessage
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("decode pricing response: %w", err)
	}

	entries := make([]storage.PricingSnapshotEntry, 0, len(data))
	for model, raw := range data {
		var p modelPricing
		if err := json.Unmarshal(raw, &p); err != nil {
			return fmt.Errorf("decode pricing entry %q: %w", model, err)
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
		return fmt.Errorf("pricing response contains no valid pricing entries")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Model < entries[j].Model })

	revision := resp.Header.Get("ETag")
	if revision == "" {
		revision = fmt.Sprintf("%x", sha256.Sum256(body))
	}
	if _, err := db.CreatePricingSnapshot(time.Now().UTC(), "litellm", revision, entries); err != nil {
		return err
	}
	log.Printf("pricing: synced %d models", len(entries))
	return nil
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
