package storage

import (
	"testing"
	"time"
)

func TestLatestSnapshotAtOrBeforePrefersHigherIDAtEqualTime(t *testing.T) {
	effectiveAt := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	snapshots := []pricingSnapshot{
		{id: 1, syncedAt: effectiveAt.Add(-time.Hour)},
		{id: 2, syncedAt: effectiveAt},
		{id: 3, syncedAt: effectiveAt},
		{id: 4, syncedAt: effectiveAt.Add(time.Hour)},
	}

	got, ok := latestSnapshotAtOrBefore(snapshots, effectiveAt)
	if !ok {
		t.Fatal("latestSnapshotAtOrBefore returned no snapshot")
	}
	if got != 3 {
		t.Errorf("snapshot ID = %d, want higher ID 3 at equal effective time", got)
	}
}

func TestPriceUnpricedUsageUsesLatestSnapshotAtOrBeforeEvent(t *testing.T) {
	db := tempDB(t)
	eventTime := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	oldSnapshotID, err := db.CreatePricingSnapshot(eventTime.Add(-2*time.Hour), "litellm", "old", []PricingSnapshotEntry{
		{Model: "claude-sonnet-4-6", InputCostPerToken: 1, OutputCostPerToken: 2, CacheReadInputTokenCost: 3, CacheCreationInputTokenCost: 4},
	})
	if err != nil {
		t.Fatalf("CreatePricingSnapshot(old): %v", err)
	}
	latestSnapshotID, err := db.CreatePricingSnapshot(eventTime.Add(-time.Hour), "litellm", "latest", []PricingSnapshotEntry{
		{Model: "anthropic/claude-sonnet-4-6", InputCostPerToken: 10, OutputCostPerToken: 20, CacheReadInputTokenCost: 30, CacheCreationInputTokenCost: 40},
	})
	if err != nil {
		t.Fatalf("CreatePricingSnapshot(latest): %v", err)
	}
	if oldSnapshotID == latestSnapshotID {
		t.Fatal("pricing snapshots have the same ID")
	}

	if err := db.InsertUsage(&UsageRecord{
		Source: "claude", SessionID: "event-time", Model: "claude-sonnet-4-6", Timestamp: eventTime,
		InputTokens: 1, OutputTokens: 2, CacheReadInputTokens: 3, CacheCreationInputTokens: 4,
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}

	pricedAtFloor := time.Now().Add(-time.Second)
	if err := db.PriceUnpricedUsage(func(input, output, cacheCreation, cacheRead int64, prices [4]float64) float64 {
		return float64(input)*prices[0] + float64(cacheCreation)*prices[3] +
			float64(cacheRead)*prices[2] + float64(output)*prices[1]
	}); err != nil {
		t.Fatalf("PriceUnpricedUsage: %v", err)
	}

	var cost float64
	var resolvedKey, status string
	var snapshotID int64
	var pricedAt time.Time
	if err := db.db.QueryRow(`SELECT cost_usd, resolved_pricing_key, pricing_snapshot_id, pricing_status, priced_at
		FROM usage_records WHERE session_id=?`, "event-time").Scan(&cost, &resolvedKey, &snapshotID, &status, &pricedAt); err != nil {
		t.Fatalf("read priced usage: %v", err)
	}
	if cost != 300 {
		t.Errorf("cost_usd = %v, want 300 from latest historical snapshot", cost)
	}
	if resolvedKey != "anthropic/claude-sonnet-4-6" {
		t.Errorf("resolved_pricing_key = %q, want canonical snapshot key", resolvedKey)
	}
	if snapshotID != latestSnapshotID {
		t.Errorf("pricing_snapshot_id = %d, want %d", snapshotID, latestSnapshotID)
	}
	if status != "priced" {
		t.Errorf("pricing_status = %q, want priced", status)
	}
	if pricedAt.Before(pricedAtFloor) {
		t.Errorf("priced_at = %v, want pricing time after %v", pricedAt, pricedAtFloor)
	}
}

func TestPriceUnpricedUsageDoesNotUseFutureSnapshot(t *testing.T) {
	db := tempDB(t)
	eventInstant := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	eventTime := eventInstant.In(time.FixedZone("UTC+14", 14*60*60))
	if _, err := db.CreatePricingSnapshot(eventInstant.Add(time.Second), "litellm", "future", []PricingSnapshotEntry{
		{Model: "model-a", InputCostPerToken: 1, OutputCostPerToken: 2},
	}); err != nil {
		t.Fatalf("CreatePricingSnapshot: %v", err)
	}
	if err := db.InsertUsage(&UsageRecord{
		Source: "claude", SessionID: "future-only", Model: "model-a", Timestamp: eventTime,
		InputTokens: 1, OutputTokens: 1,
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}

	if err := db.PriceUnpricedUsage(func(_, _ int64, _, _ int64, _ [4]float64) float64 { return 99 }); err != nil {
		t.Fatalf("PriceUnpricedUsage: %v", err)
	}

	var cost float64
	var resolvedKey, status string
	var snapshotID, pricedAt any
	if err := db.db.QueryRow(`SELECT cost_usd, resolved_pricing_key, pricing_snapshot_id, pricing_status, priced_at
		FROM usage_records WHERE session_id=?`, "future-only").Scan(&cost, &resolvedKey, &snapshotID, &status, &pricedAt); err != nil {
		t.Fatalf("read unpriced usage: %v", err)
	}
	if cost != 0 || resolvedKey != "" || snapshotID != nil || status != "unpriced" || pricedAt != nil {
		t.Errorf("future snapshot changed usage: cost=%v key=%q snapshot=%v status=%q priced_at=%v",
			cost, resolvedKey, snapshotID, status, pricedAt)
	}
}

func TestPriceUnpricedUsageHistoricalFallbackPricesBeforeFirstSnapshot(t *testing.T) {
	db := tempDB(t)
	eventTime := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	if _, err := db.CreatePricingSnapshot(eventTime.Add(time.Hour), "litellm", "initial-sync", []PricingSnapshotEntry{
		{Model: "model-a", InputCostPerToken: 2, OutputCostPerToken: 3},
	}); err != nil {
		t.Fatalf("CreatePricingSnapshot: %v", err)
	}
	if err := db.InsertUsage(&UsageRecord{
		Source: "claude", SessionID: "historical", Model: "model-a", Timestamp: eventTime,
		InputTokens: 2, OutputTokens: 1,
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}

	// The strict API must continue to reject a future snapshot for the event.
	if err := db.PriceUnpricedUsage(func(input, output, _ int64, _ int64, prices [4]float64) float64 {
		return float64(input)*prices[0] + float64(output)*prices[1]
	}); err != nil {
		t.Fatalf("PriceUnpricedUsage: %v", err)
	}
	var status string
	if err := db.db.QueryRow(`SELECT pricing_status FROM usage_records WHERE session_id='historical'`).Scan(&status); err != nil {
		t.Fatalf("read strict pricing status: %v", err)
	}
	if status != "unpriced" {
		t.Fatalf("strict pricing status = %q, want unpriced", status)
	}

	if err := db.PriceUnpricedUsageWithHistoricalFallback(func(input, output, _ int64, _ int64, prices [4]float64) float64 {
		return float64(input)*prices[0] + float64(output)*prices[1]
	}); err != nil {
		t.Fatalf("PriceUnpricedUsageWithHistoricalFallback: %v", err)
	}
	var cost float64
	if err := db.db.QueryRow(`SELECT cost_usd FROM usage_records WHERE session_id='historical'`).Scan(&cost); err != nil {
		t.Fatalf("read fallback cost: %v", err)
	}
	if cost != 7 {
		t.Errorf("historical fallback cost = %v, want 7", cost)
	}
}

func TestPriceUnpricedUsageWithoutSnapshotRemainsUnpriced(t *testing.T) {
	db := tempDB(t)
	eventTime := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&UsageRecord{
		Source: "claude", SessionID: "no-snapshot", Model: "model-a", Timestamp: eventTime,
		InputTokens: 1, OutputTokens: 1,
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	if err := db.PriceUnpricedUsageWithHistoricalFallback(func(_, _ int64, _, _ int64, _ [4]float64) float64 { return 99 }); err != nil {
		t.Fatalf("PriceUnpricedUsageWithHistoricalFallback: %v", err)
	}
	var status string
	if err := db.db.QueryRow(`SELECT pricing_status FROM usage_records WHERE session_id='no-snapshot'`).Scan(&status); err != nil {
		t.Fatalf("read pricing status: %v", err)
	}
	if status != "unpriced" {
		t.Errorf("pricing status = %q, want unpriced", status)
	}
}

func TestPriceUnpricedUsageKeepsUnmatchedModelUnpriced(t *testing.T) {
	db := tempDB(t)
	eventTime := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	if _, err := db.CreatePricingSnapshot(eventTime.Add(-time.Hour), "litellm", "known-model", []PricingSnapshotEntry{
		{Model: "known-model", InputCostPerToken: 1, OutputCostPerToken: 2},
	}); err != nil {
		t.Fatalf("CreatePricingSnapshot: %v", err)
	}
	if err := db.InsertUsage(&UsageRecord{
		Source: "claude", SessionID: "unmatched", Model: "not-in-snapshot", Timestamp: eventTime,
		InputTokens: 10, OutputTokens: 5,
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	if err := db.PriceUnpricedUsage(func(_, _ int64, _, _ int64, _ [4]float64) float64 { return 99 }); err != nil {
		t.Fatalf("PriceUnpricedUsage: %v", err)
	}

	var cost float64
	var key, status string
	var snapshotID, pricedAt any
	if err := db.db.QueryRow(`SELECT cost_usd, resolved_pricing_key, pricing_snapshot_id, pricing_status, priced_at
		FROM usage_records WHERE session_id=?`, "unmatched").Scan(&cost, &key, &snapshotID, &status, &pricedAt); err != nil {
		t.Fatalf("read unmatched usage: %v", err)
	}
	if cost != 0 || key != "" || snapshotID != nil || status != "unpriced" || pricedAt != nil {
		t.Errorf("unmatched usage was priced: cost=%v key=%q snapshot=%v status=%q priced_at=%v",
			cost, key, snapshotID, status, pricedAt)
	}
}

func TestPriceUsageNeverRewritesPricedOrLegacyRows(t *testing.T) {
	db := tempDB(t)
	eventTime := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	snapshotID, err := db.CreatePricingSnapshot(eventTime.Add(-time.Hour), "litellm", "current", []PricingSnapshotEntry{
		{Model: "model-a", InputCostPerToken: 100, OutputCostPerToken: 200},
	})
	if err != nil {
		t.Fatalf("CreatePricingSnapshot: %v", err)
	}
	for _, sessionID := range []string{"already-priced", "legacy"} {
		if err := db.InsertUsage(&UsageRecord{
			Source: "claude", SessionID: sessionID, Model: "model-a", Timestamp: eventTime,
			InputTokens: 1, OutputTokens: 1,
		}); err != nil {
			t.Fatalf("InsertUsage(%s): %v", sessionID, err)
		}
	}
	originalPricedAt := eventTime.Add(2 * time.Hour)
	if _, err := db.db.Exec(`UPDATE usage_records SET cost_usd=7, resolved_pricing_key='original-key',
		pricing_snapshot_id=?, pricing_status='priced', priced_at=? WHERE session_id='already-priced'`, snapshotID, originalPricedAt); err != nil {
		t.Fatalf("mark priced usage: %v", err)
	}
	if _, err := db.db.Exec(`UPDATE usage_records SET cost_usd=8, pricing_status='legacy'
		WHERE session_id='legacy'`); err != nil {
		t.Fatalf("mark legacy usage: %v", err)
	}

	if err := db.PriceUnpricedUsage(func(_, _ int64, _, _ int64, _ [4]float64) float64 { return 999 }); err != nil {
		t.Fatalf("PriceUnpricedUsage: %v", err)
	}

	var pricedCost float64
	var pricedKey, pricedStatus string
	var pricedSnapshotID int64
	var pricedAt time.Time
	if err := db.db.QueryRow(`SELECT cost_usd, resolved_pricing_key, pricing_snapshot_id, pricing_status, priced_at
		FROM usage_records WHERE session_id='already-priced'`).Scan(
		&pricedCost, &pricedKey, &pricedSnapshotID, &pricedStatus, &pricedAt); err != nil {
		t.Fatalf("read priced usage: %v", err)
	}
	if pricedCost != 7 || pricedKey != "original-key" || pricedSnapshotID != snapshotID ||
		pricedStatus != "priced" || !pricedAt.Equal(originalPricedAt) {
		t.Errorf("priced usage was rewritten: cost=%v key=%q snapshot=%d status=%q priced_at=%v",
			pricedCost, pricedKey, pricedSnapshotID, pricedStatus, pricedAt)
	}

	var legacyCost float64
	var legacyKey, legacyStatus string
	var legacySnapshotID, legacyPricedAt any
	if err := db.db.QueryRow(`SELECT cost_usd, resolved_pricing_key, pricing_snapshot_id, pricing_status, priced_at
		FROM usage_records WHERE session_id='legacy'`).Scan(
		&legacyCost, &legacyKey, &legacySnapshotID, &legacyStatus, &legacyPricedAt); err != nil {
		t.Fatalf("read legacy usage: %v", err)
	}
	if legacyCost != 8 || legacyKey != "" || legacySnapshotID != nil || legacyStatus != "legacy" || legacyPricedAt != nil {
		t.Errorf("legacy usage was rewritten: cost=%v key=%q snapshot=%v status=%q priced_at=%v",
			legacyCost, legacyKey, legacySnapshotID, legacyStatus, legacyPricedAt)
	}
}

func TestResolvePricingIsDeterministicAcrossMapOrder(t *testing.T) {
	firstKey := "acme/claude-sonnet-4-6"
	secondKey := "beta/claude-sonnet-4-6"
	firstPrices := [4]float64{0.001, 0.002, 0.003, 0.004}
	secondPrices := [4]float64{0.005, 0.006, 0.007, 0.008}

	pricesInOrder := make(map[string][4]float64)
	pricesInOrder[firstKey] = firstPrices
	pricesInOrder[secondKey] = secondPrices

	pricesInReverseOrder := make(map[string][4]float64)
	pricesInReverseOrder[secondKey] = secondPrices
	pricesInReverseOrder[firstKey] = firstPrices

	for name, prices := range map[string]map[string][4]float64{
		"forward insertion": pricesInOrder,
		"reverse insertion": pricesInReverseOrder,
	} {
		t.Run(name, func(t *testing.T) {
			match, ok := matchPricing("claude-sonnet-4.6", prices)
			if !ok {
				t.Fatal("expected fuzzy match")
			}
			if match.Key != firstKey {
				t.Fatalf("matched key = %q, want lexicographically first key %q", match.Key, firstKey)
			}
			if match.Prices != firstPrices {
				t.Fatalf("matched prices = %v, want %v", match.Prices, firstPrices)
			}
			if match.MatchKind != "fuzzy" {
				t.Fatalf("match kind = %q, want fuzzy", match.MatchKind)
			}
		})
	}
}

func TestResolvePricingReturnsCanonicalKey(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		prices    map[string][4]float64
		want      PricingMatch
		wantMatch bool
	}{
		{
			name:  "direct key",
			model: "claude-opus-4-6",
			prices: map[string][4]float64{
				"claude-opus-4-6": {0.015, 0.075, 0.0015, 0.01875},
			},
			want: PricingMatch{
				Key:       "claude-opus-4-6",
				Prices:    [4]float64{0.015, 0.075, 0.0015, 0.01875},
				MatchKind: "direct",
			},
			wantMatch: true,
		},
		{
			name:  "provider prefix",
			model: "deepseek-r1",
			prices: map[string][4]float64{
				"deepseek/deepseek-r1": {0.001, 0.002, 0.0005, 0.001},
			},
			want: PricingMatch{
				Key:       "deepseek/deepseek-r1",
				Prices:    [4]float64{0.001, 0.002, 0.0005, 0.001},
				MatchKind: "provider_prefix",
			},
			wantMatch: true,
		},
		{
			name:  "version normalization",
			model: "claude-sonnet-4.6",
			prices: map[string][4]float64{
				"claude-sonnet-4-6": {0.003, 0.015, 0.001, 0.004},
			},
			want: PricingMatch{
				Key:       "claude-sonnet-4-6",
				Prices:    [4]float64{0.003, 0.015, 0.001, 0.004},
				MatchKind: "normalized",
			},
			wantMatch: true,
		},
		{
			name:  "no match",
			model: "totally-unknown-model",
			prices: map[string][4]float64{
				"claude-opus-4-6": {0.015, 0.075, 0, 0},
			},
			want:      PricingMatch{},
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := matchPricing(tt.model, tt.prices)
			if ok != tt.wantMatch {
				t.Fatalf("matched = %t, want %t (result: %+v)", ok, tt.wantMatch, got)
			}
			if got != tt.want {
				t.Fatalf("match = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestResolvePricingRejectsAmbiguousPartialMatch(t *testing.T) {
	prices := map[string][4]float64{
		"vendor/not-deepseek-r1-preview": {0.001, 0.002, 0.003, 0.004},
	}

	match, ok := matchPricing("deepseek-r1", prices)
	if ok {
		t.Fatalf("unexpected substring match: %+v", match)
	}
	if match != (PricingMatch{}) {
		t.Fatalf("no-match result = %+v, want zero value", match)
	}
}
