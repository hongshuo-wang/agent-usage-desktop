package storage

import (
	"math"
	"testing"
	"time"
)

func TestThroughputUsesLocalMinuteBucketsAndRollingWindows(t *testing.T) {
	db := tempDB(t)
	base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	records := []*UsageRecord{
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
			Timestamp: base.Add(2*time.Minute + 30*time.Second), InputTokens: 500,
			CacheReadInputTokens: 1, CacheCreationInputTokens: 2, OutputTokens: 3,
		},
	}
	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	result, err := db.GetThroughput(base, base.Add(3*time.Minute), "", "", -480)
	if err != nil {
		t.Fatalf("GetThroughput: %v", err)
	}
	if len(result.Series) != 3 {
		t.Fatalf("series length = %d, want 3: %+v", len(result.Series), result.Series)
	}
	for i, wantMinute := range []string{
		"2025-01-01 18:00", "2025-01-01 18:01", "2025-01-01 18:02",
	} {
		if result.Series[i].Minute != wantMinute {
			t.Errorf("series[%d].minute = %q, want %q", i, result.Series[i].Minute, wantMinute)
		}
	}
	assertThroughputValues(t, "series[0]", result.Series[0].ThroughputValues, ThroughputValues{
		RPM: 1, InputTPM: 10, CacheRead: 20, CacheCreate: 30, OutputTPM: 40, TotalTPM: 100,
	})
	assertThroughputValues(t, "series[1]", result.Series[1].ThroughputValues, ThroughputValues{
		RPM: 1, InputTPM: 1, CacheRead: 2, CacheCreate: 3, OutputTPM: 4, TotalTPM: 10,
	})
	assertThroughputValues(t, "series[2]", result.Series[2].ThroughputValues, ThroughputValues{
		RPM: 1, InputTPM: 500, CacheRead: 1, CacheCreate: 2, OutputTPM: 3, TotalTPM: 506,
	})
	assertThroughputValues(t, "average active minute", result.AverageActiveMinute, ThroughputValues{
		RPM: 1, InputTPM: 511.0 / 3, CacheRead: 23.0 / 3,
		CacheCreate: 35.0 / 3, OutputTPM: 47.0 / 3, TotalTPM: 616.0 / 3,
	})
	assertThroughputValues(t, "peak rolling 60s", result.PeakRolling60s, ThroughputValues{
		RPM: 2, InputTPM: 500, CacheRead: 22, CacheCreate: 33, OutputTPM: 44, TotalTPM: 506,
	})
	assertThroughputValues(t, "p95 rolling 60s", result.P95Rolling60s, ThroughputValues{
		RPM: 2, InputTPM: 500, CacheRead: 22, CacheCreate: 33, OutputTPM: 44, TotalTPM: 506,
	})
}

func TestThroughputIntersectsSourceAndModelFilters(t *testing.T) {
	db := tempDB(t)
	base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	if err := db.InsertUsageBatch([]*UsageRecord{
		{Source: "claude", SessionID: "s1", Model: "model-a", Timestamp: base, InputTokens: 10},
		{Source: "codex", SessionID: "s2", Model: "model-b", Timestamp: base.Add(time.Second), InputTokens: 20},
	}); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	result, err := db.GetThroughput(base.Add(-time.Minute), base.Add(time.Minute), "claude", "model-b", 0)
	if err != nil {
		t.Fatalf("GetThroughput: %v", err)
	}
	if len(result.Series) != 0 {
		t.Fatalf("filtered series = %+v, want empty", result.Series)
	}
	assertThroughputValues(t, "filtered average", result.AverageActiveMinute, ThroughputValues{})
	assertThroughputValues(t, "filtered peak", result.PeakRolling60s, ThroughputValues{})
	assertThroughputValues(t, "filtered p95", result.P95Rolling60s, ThroughputValues{})
}

func TestThroughputRollingWindowExcludesRecordExactlySixtySecondsOld(t *testing.T) {
	db := tempDB(t)
	base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	if err := db.InsertUsageBatch([]*UsageRecord{
		{Source: "claude", SessionID: "first", Model: "model-a", Timestamp: base, InputTokens: 1},
		{Source: "claude", SessionID: "second", Model: "model-a", Timestamp: base.Add(time.Minute), InputTokens: 2},
	}); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	result, err := db.GetThroughput(base, base.Add(time.Minute), "", "", 0)
	if err != nil {
		t.Fatalf("GetThroughput: %v", err)
	}
	if result.PeakRolling60s.RPM != 1 || result.PeakRolling60s.InputTPM != 2 {
		t.Fatalf("peak rolling 60s = %+v, want exact t-60s record excluded", result.PeakRolling60s)
	}
}

func TestThroughputNearestRankP95(t *testing.T) {
	if got := nearestRankP95(nil); got != 0 {
		t.Errorf("nearestRankP95(nil) = %d, want 0", got)
	}
	values := []int64{20, 1, 19, 2, 18, 3, 17, 4, 16, 5, 15, 6, 14, 7, 13, 8, 12, 9, 11, 10}
	if got := nearestRankP95(values); got != 19 {
		t.Errorf("nearestRankP95(values) = %d, want 19", got)
	}
}

func TestThroughputP95RepeatsCompleteWindowForEqualTimestamps(t *testing.T) {
	db := tempDB(t)
	base := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	records := make([]*UsageRecord, 0, 21)
	for i := 0; i < 19; i++ {
		records = append(records, &UsageRecord{
			Source: "claude", SessionID: "single", Model: "model-a",
			Timestamp: base.Add(time.Duration(i) * 2 * time.Minute), InputTokens: 1,
		})
	}
	sharedTimestamp := base.Add(40 * time.Minute)
	for i := 0; i < 2; i++ {
		records = append(records, &UsageRecord{
			Source: "claude", SessionID: "shared-" + string(rune('a'+i)), Model: "model-a",
			Timestamp: sharedTimestamp, InputTokens: 1,
		})
	}
	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	result, err := db.GetThroughput(base, sharedTimestamp, "", "", 0)
	if err != nil {
		t.Fatalf("GetThroughput: %v", err)
	}
	if result.P95Rolling60s.RPM != 2 || result.P95Rolling60s.InputTPM != 2 {
		t.Fatalf("p95 rolling 60s = %+v, want equal-timestamp window repeated per record", result.P95Rolling60s)
	}
}

func assertThroughputValues(t *testing.T, label string, got, want ThroughputValues) {
	t.Helper()
	gotValues := []float64{got.RPM, got.InputTPM, got.CacheRead, got.CacheCreate, got.OutputTPM, got.TotalTPM}
	wantValues := []float64{want.RPM, want.InputTPM, want.CacheRead, want.CacheCreate, want.OutputTPM, want.TotalTPM}
	for i := range gotValues {
		if math.Abs(gotValues[i]-wantValues[i]) > 1e-9 {
			t.Errorf("%s = %+v, want %+v", label, got, want)
			return
		}
	}
}
