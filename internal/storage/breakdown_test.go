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

func TestUsageBreakdownRejectsUnknownDimension(t *testing.T) {
	db := tempDB(t)
	if _, err := db.GetUsageBreakdown(time.Time{}, time.Now(), "", "invalid"); err == nil {
		t.Fatal("invalid dimension returned nil error")
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
