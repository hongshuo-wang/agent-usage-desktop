package storage

import (
	"testing"
	"time"
)

func TestActivityHeatmapBucketsLocalWeekdayAndHour(t *testing.T) {
	db := tempDB(t)
	// 2025-01-01 is a Wednesday, so STRFTIME('%w') == 3.
	base := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	if err := db.InsertUsageBatch([]*UsageRecord{
		{Source: "claude", SessionID: "a", Model: "m", Timestamp: base, InputTokens: 10, OutputTokens: 1},
		{Source: "claude", SessionID: "a", Model: "m", Timestamp: base.Add(time.Minute), InputTokens: 20, OutputTokens: 2},
		{Source: "claude", SessionID: "a", Model: "m", Timestamp: base.Add(time.Hour), InputTokens: 30, OutputTokens: 3},
		// 23:00 UTC is already the next day at UTC+8, which must move the weekday.
		{Source: "claude", SessionID: "a", Model: "m", Timestamp: base.Add(11 * time.Hour), InputTokens: 40, OutputTokens: 4},
		{Source: "codex", SessionID: "b", Model: "m", Timestamp: base, InputTokens: 999},
	}); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}
	if _, err := db.db.Exec(`UPDATE usage_records SET pricing_status='priced', cost_usd=2 WHERE source='claude'`); err != nil {
		t.Fatalf("mark usage priced: %v", err)
	}
	from, to := base.Add(-time.Minute), base.Add(24*time.Hour)

	utc, err := db.GetActivityHeatmap(from, to, "claude", 0)
	if err != nil {
		t.Fatalf("GetActivityHeatmap(utc): %v", err)
	}
	want := []HeatmapCell{
		{Weekday: 3, Hour: 12, Calls: 2, Tokens: 33, Cost: 4},
		{Weekday: 3, Hour: 13, Calls: 1, Tokens: 33, Cost: 2},
		{Weekday: 3, Hour: 23, Calls: 1, Tokens: 44, Cost: 2},
	}
	if len(utc) != len(want) {
		t.Fatalf("utc cells = %+v, want %+v", utc, want)
	}
	for i, cell := range utc {
		if cell != want[i] {
			t.Errorf("utc cell[%d] = %+v, want %+v", i, cell, want[i])
		}
	}

	// UTC+8 (JS getTimezoneOffset() convention -480) shifts 23:00 Wed into Thu 07:00.
	local, err := db.GetActivityHeatmap(from, to, "claude", -480)
	if err != nil {
		t.Fatalf("GetActivityHeatmap(local): %v", err)
	}
	wantLocal := []HeatmapCell{
		{Weekday: 3, Hour: 20, Calls: 2, Tokens: 33, Cost: 4},
		{Weekday: 3, Hour: 21, Calls: 1, Tokens: 33, Cost: 2},
		{Weekday: 4, Hour: 7, Calls: 1, Tokens: 44, Cost: 2},
	}
	if len(local) != len(wantLocal) {
		t.Fatalf("local cells = %+v, want %+v", local, wantLocal)
	}
	for i, cell := range local {
		if cell != wantLocal[i] {
			t.Errorf("local cell[%d] = %+v, want %+v", i, cell, wantLocal[i])
		}
	}
}
