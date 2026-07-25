package storage

import (
	"math"
	"testing"
	"time"
)

func TestUsageBreakdownUsesExactTokenSessionCacheAndPriceSemantics(t *testing.T) {
	db := tempDB(t)
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := db.UpsertPricing("known-zero-price", 0, 0, 0, 0); err != nil {
		t.Fatalf("UpsertPricing: %v", err)
	}
	if err := db.InsertUsageBatch([]*UsageRecord{
		{Source: "ab", SessionID: "c", Model: "known-zero-price", Project: "project-a", Timestamp: base, InputTokens: 10, CacheReadInputTokens: 20, CacheCreationInputTokens: 30, OutputTokens: 40},
		{Source: "ab", SessionID: "c", Model: "known-zero-price", Project: "project-a", Timestamp: base.Add(time.Second), InputTokens: 1, CacheReadInputTokens: 2, CacheCreationInputTokens: 3, OutputTokens: 4},
		{Source: "a", SessionID: "bc", Model: "missing-price", Project: "project-a", Timestamp: base.Add(2 * time.Second), InputTokens: 5, CacheReadInputTokens: 5, OutputTokens: 5},
		{Source: "codex", SessionID: "filtered", Model: "known-zero-price", Project: "project-b", Timestamp: base.Add(3 * time.Second), InputTokens: 999},
	}); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}
	if _, err := db.db.Exec(`UPDATE usage_records SET pricing_status='priced' WHERE model='known-zero-price'`); err != nil {
		t.Fatalf("mark known usage priced: %v", err)
	}

	rows, err := db.GetUsageBreakdown(base.Add(-time.Minute), base.Add(time.Minute), "", "project")
	if err != nil {
		t.Fatalf("GetUsageBreakdown: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("breakdown rows = %+v, want two projects", rows)
	}
	project := findBreakdown(t, rows, "project-a")
	if project.TotalTokens != 125 || project.Calls != 3 || project.Sessions != 2 {
		t.Errorf("project aggregate = %+v, want tokens=125 calls=3 sessions=2", project)
	}
	if want := 27.0 / 76.0; math.Abs(project.CacheHitRate-want) > 1e-12 {
		t.Errorf("cache_hit_rate = %.12f, want %.12f", project.CacheHitRate, want)
	}
	if !project.UnknownPrice {
		t.Error("missing pricing row did not mark unknown_price")
	}

	models, err := db.GetUsageBreakdown(base.Add(-time.Minute), base.Add(time.Minute), "ab", "model")
	if err != nil {
		t.Fatalf("GetUsageBreakdown: %v", err)
	}
	if len(models) != 1 || models[0].Key != "known-zero-price" || models[0].UnknownPrice {
		t.Fatalf("known zero-price model treated as unknown or source filter leaked: %+v", models)
	}
	if models[0].TotalTokens != 110 || models[0].Sessions != 1 || models[0].Calls != 2 {
		t.Errorf("model aggregate = %+v", models[0])
	}
}

func TestBreakdownUnknownPriceUsesStoredStatus(t *testing.T) {
	db := tempDB(t)
	eventTime := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	if _, err := db.CreatePricingSnapshot(eventTime.Add(-time.Hour), "litellm", "fuzzy", []PricingSnapshotEntry{
		{Model: "acme/claude-sonnet-4-6", InputCostPerToken: 1},
	}); err != nil {
		t.Fatalf("CreatePricingSnapshot: %v", err)
	}
	if err := db.InsertUsage(&UsageRecord{
		Source: "claude", SessionID: "priced", Model: "claude-sonnet-4.6",
		Project: "priced", Timestamp: eventTime, InputTokens: 1,
	}); err != nil {
		t.Fatalf("InsertUsage(priced): %v", err)
	}
	if err := db.PriceUnpricedUsage(func(_, _ int64, _, _ int64, _ [4]float64) float64 { return 1 }); err != nil {
		t.Fatalf("PriceUnpricedUsage: %v", err)
	}
	if err := db.InsertUsageBatch([]*UsageRecord{
		{Source: "claude", SessionID: "unpriced", Model: "currently-known", Project: "unpriced", Timestamp: eventTime.Add(time.Second), InputTokens: 1, CostUSD: 4},
		{Source: "claude", SessionID: "legacy", Model: "legacy-model", Project: "legacy", Timestamp: eventTime.Add(2 * time.Second), InputTokens: 1, CostUSD: 2},
	}); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}
	if _, err := db.db.Exec(`UPDATE usage_records SET pricing_status='legacy' WHERE session_id='legacy'`); err != nil {
		t.Fatalf("mark legacy usage: %v", err)
	}
	if err := db.UpsertPricing("currently-known", 1, 0, 0, 0); err != nil {
		t.Fatalf("UpsertPricing: %v", err)
	}

	rows, err := db.GetUsageBreakdown(eventTime.Add(-time.Minute), eventTime.Add(time.Minute), "", "project")
	if err != nil {
		t.Fatalf("GetUsageBreakdown: %v", err)
	}
	if findBreakdown(t, rows, "priced").UnknownPrice {
		t.Error("fuzzy-resolved priced row was marked unknown without an exact live pricing key")
	}
	if !findBreakdown(t, rows, "unpriced").UnknownPrice {
		t.Error("stored unpriced row was treated as known because a live pricing key exists")
	}
	if got := findBreakdown(t, rows, "unpriced").TotalCost; got != 0 {
		t.Errorf("unpriced breakdown cost = %v, want 0", got)
	}
	if findBreakdown(t, rows, "legacy").UnknownPrice {
		t.Error("legacy row was marked unknown despite its preserved stored cost")
	}
}

func TestUsageBreakdownRejectsUnknownDimension(t *testing.T) {
	db := tempDB(t)
	if _, err := db.GetUsageBreakdown(time.Time{}, time.Now(), "", "invalid"); err == nil {
		t.Fatal("invalid dimension returned nil error")
	}
}

func TestUsageBreakdownProjectFallsBackToSessionCWDAndID(t *testing.T) {
	db := tempDB(t)
	base := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	if err := db.UpsertSession(&SessionRecord{Source: "claude", SessionID: "cwd-session", CWD: "/work/cwd-project"}); err != nil {
		t.Fatalf("UpsertSession cwd fallback: %v", err)
	}
	if err := db.UpsertSession(&SessionRecord{Source: "claude", SessionID: "mixed-session", Project: "session-project", CWD: "/work/other-project"}); err != nil {
		t.Fatalf("UpsertSession mixed metadata: %v", err)
	}
	if err := db.InsertUsageBatch([]*UsageRecord{
		{Source: "claude", SessionID: "cwd-session", Model: "model", Timestamp: base, InputTokens: 3},
		{Source: "claude", SessionID: "id-session", Model: "model", Timestamp: base.Add(time.Second), InputTokens: 2},
		{Source: "claude", SessionID: "explicit-session", Model: "model", Project: "explicit-project", Timestamp: base.Add(2 * time.Second), InputTokens: 1},
		{Source: "claude", SessionID: "mixed-session", Model: "model", Project: "usage-project", Timestamp: base.Add(3 * time.Second), InputTokens: 4},
	}); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	rows, err := db.GetUsageBreakdown(base.Add(-time.Minute), base.Add(time.Minute), "", "project")
	if err != nil {
		t.Fatalf("GetUsageBreakdown: %v", err)
	}
	for key, wantTokens := range map[string]int64{"cwd-project": 3, "id-session": 2, "explicit-project": 1, "session-project": 4} {
		row := findBreakdown(t, rows, key)
		if row.TotalTokens != wantTokens {
			t.Errorf("breakdown[%q].TotalTokens = %d, want %d", key, row.TotalTokens, wantTokens)
		}
	}
	if len(rows) != 4 {
		t.Fatalf("breakdown rows = %+v, want 4 project keys", rows)
	}
}

func findBreakdown(t *testing.T, rows []UsageBreakdown, key string) UsageBreakdown {
	t.Helper()
	for _, row := range rows {
		if row.Key == key {
			return row
		}
	}
	t.Fatalf("breakdown key %q not found in %+v", key, rows)
	return UsageBreakdown{}
}
