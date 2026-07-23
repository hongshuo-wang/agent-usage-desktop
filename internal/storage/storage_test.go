package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func tempDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenAndClose(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestFileState(t *testing.T) {
	db := tempDB(t)

	size, offset, ctx, err := db.GetFileState("/tmp/test.jsonl")
	if err != nil {
		t.Fatalf("GetFileState: %v", err)
	}
	if size != 0 || offset != 0 || ctx != nil {
		t.Errorf("expected (0,0,nil), got (%d,%d,%v)", size, offset, ctx)
	}

	if err := db.SetFileState("/tmp/test.jsonl", 1024, 512, nil); err != nil {
		t.Fatalf("SetFileState: %v", err)
	}

	size, offset, ctx, err = db.GetFileState("/tmp/test.jsonl")
	if err != nil {
		t.Fatalf("GetFileState: %v", err)
	}
	if size != 1024 || offset != 512 {
		t.Errorf("expected (1024,512), got (%d,%d)", size, offset)
	}
	if ctx != nil {
		t.Errorf("expected nil ctx, got %v", ctx)
	}
}

func TestInsertUsageAndDedup(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	rec := &UsageRecord{
		Source:       "claude",
		SessionID:    "sess-1",
		Model:        "claude-sonnet-4-20250514",
		InputTokens:  1000,
		OutputTokens: 500,
		Timestamp:    ts,
	}

	if err := db.InsertUsage(rec); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}

	// Insert same record again — should be silently ignored (dedup)
	if err := db.InsertUsage(rec); err != nil {
		t.Fatalf("InsertUsage duplicate: %v", err)
	}

	// Verify only one record exists
	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalTokens != 1500 {
		t.Errorf("expected 1500 tokens, got %d", stats.TotalTokens)
	}
}

func TestInsertUsageBatch(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	records := []*UsageRecord{
		{Source: "claude", SessionID: "sess-1", Model: "claude-sonnet-4-20250514", InputTokens: 100, OutputTokens: 50, Timestamp: ts},
		{Source: "claude", SessionID: "sess-1", Model: "claude-sonnet-4-20250514", InputTokens: 200, OutputTokens: 100, Timestamp: ts.Add(time.Second)},
	}

	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalTokens != 450 {
		t.Errorf("expected 450 tokens, got %d", stats.TotalTokens)
	}
}

func TestUpsertSession(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	sess := &SessionRecord{
		Source:    "claude",
		SessionID: "sess-1",
		Project:   "myproject",
		CWD:       "/home/user/code",
		Version:   "1.0.0",
		StartTime: ts,
		Prompts:   5,
	}
	if err := db.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	// Upsert again with updated prompts
	sess2 := &SessionRecord{
		Source:    "claude",
		SessionID: "sess-1",
		Prompts:   3,
		StartTime: ts.Add(time.Hour),
	}
	if err := db.UpsertSession(sess2); err != nil {
		t.Fatalf("UpsertSession update: %v", err)
	}
}

func TestUpsertSessionAllowsSameIDAcrossSources(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	for _, source := range []string{"claude", "codex"} {
		if err := db.UpsertSession(&SessionRecord{
			Source: source, SessionID: "shared-session", Project: source, StartTime: ts,
		}); err != nil {
			t.Fatalf("UpsertSession(%s): %v", source, err)
		}
	}

	var count int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE session_id = ?`, "shared-session").Scan(&count); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected two source-qualified sessions, got %d", count)
	}
}

func TestUsageAndPromptDedupIncludeSource(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	for _, source := range []string{"claude", "codex"} {
		if err := db.InsertUsage(&UsageRecord{
			Source: source, SessionID: "shared-session", Model: "same-model",
			InputTokens: 10, OutputTokens: 5, Timestamp: ts,
		}); err != nil {
			t.Fatalf("InsertUsage(%s): %v", source, err)
		}
		if err := db.InsertPromptBatch([]*PromptEvent{{
			Source: source, SessionID: "shared-session", Timestamp: ts,
		}}); err != nil {
			t.Fatalf("InsertPromptBatch(%s): %v", source, err)
		}
	}

	for _, table := range []string{"usage_records", "prompt_events"} {
		var count int
		if err := db.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 2 {
			t.Errorf("expected two %s rows, got %d", table, count)
		}
	}

	stats, err := db.GetDashboardStats(ts.Add(-time.Minute), ts.Add(time.Minute), "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalSessions != 2 {
		t.Errorf("expected two source-qualified sessions in stats, got %d", stats.TotalSessions)
	}
}

func TestSessionReadsUseCompositeIdentity(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	wants := map[string]struct {
		tokens  int64
		prompts int
	}{
		"claude": {tokens: 15, prompts: 1},
		"codex":  {tokens: 35, prompts: 2},
	}
	for source, want := range wants {
		if err := db.UpsertSession(&SessionRecord{
			Source: source, SessionID: "shared-session", Project: source, StartTime: ts,
		}); err != nil {
			t.Fatalf("UpsertSession(%s): %v", source, err)
		}
		if err := db.InsertUsage(&UsageRecord{
			Source: source, SessionID: "shared-session", Model: "same-model",
			InputTokens: want.tokens, Timestamp: ts,
		}); err != nil {
			t.Fatalf("InsertUsage(%s): %v", source, err)
		}
		prompts := make([]*PromptEvent, want.prompts)
		for i := range prompts {
			prompts[i] = &PromptEvent{Source: source, SessionID: "shared-session", Timestamp: ts.Add(time.Duration(i) * time.Second)}
		}
		if err := db.InsertPromptBatch(prompts); err != nil {
			t.Fatalf("InsertPromptBatch(%s): %v", source, err)
		}
	}

	sessions, err := db.GetSessions(ts.Add(-time.Minute), ts.Add(time.Minute), "")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected two source-qualified sessions, got %d", len(sessions))
	}
	for _, session := range sessions {
		want := wants[session.Source]
		if session.Tokens != want.tokens || session.Prompts != want.prompts {
			t.Errorf("%s session mixed source data: tokens=%d prompts=%d", session.Source, session.Tokens, session.Prompts)
		}
	}
	for source, want := range wants {
		details, err := db.GetSessionDetail(source, "shared-session")
		if err != nil {
			t.Fatalf("GetSessionDetail(%s): %v", source, err)
		}
		if len(details) != 1 || details[0].InputTokens != want.tokens {
			t.Fatalf("%s detail mixed source data: %+v", source, details)
		}
	}
}

func TestPricing(t *testing.T) {
	db := tempDB(t)

	if err := db.UpsertPricing("claude-sonnet-4-20250514", 0.003, 0.015, 0.001, 0.004); err != nil {
		t.Fatalf("UpsertPricing: %v", err)
	}

	inp, out, cr, cc, err := db.GetPricing("claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("GetPricing: %v", err)
	}
	if inp != 0.003 || out != 0.015 || cr != 0.001 || cc != 0.004 {
		t.Errorf("unexpected pricing: %f %f %f %f", inp, out, cr, cc)
	}

	// Non-existent model returns zeros
	inp, out, cr, cc, err = db.GetPricing("nonexistent")
	if err != nil {
		t.Fatalf("GetPricing nonexistent: %v", err)
	}
	if inp != 0 || out != 0 || cr != 0 || cc != 0 {
		t.Errorf("expected zeros for nonexistent model")
	}

	all, err := db.GetAllPricing()
	if err != nil {
		t.Fatalf("GetAllPricing: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("expected 1 model, got %d", len(all))
	}
}

func TestRecalcCosts(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// Insert a record with zero cost
	rec := &UsageRecord{
		Source:       "claude",
		SessionID:    "sess-1",
		Model:        "claude-sonnet-4-20250514",
		InputTokens:  1000,
		OutputTokens: 500,
		CostUSD:      0,
		Timestamp:    ts,
	}
	if err := db.InsertUsage(rec); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}

	// Set up pricing
	prices := map[string][4]float64{
		"claude-sonnet-4-20250514": {0.003, 0.015, 0.001, 0.004},
	}

	calcFn := func(input, output, cc, cr int64, p [4]float64) float64 {
		return float64(input)*p[0] + float64(output)*p[1]
	}

	if err := db.RecalcCosts(prices, calcFn); err != nil {
		t.Fatalf("RecalcCosts: %v", err)
	}

	// Verify cost was updated
	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	// 1000*0.003 + 500*0.015 = 3.0 + 7.5 = 10.5
	if stats.TotalCost < 10.4 || stats.TotalCost > 10.6 {
		t.Errorf("expected ~10.5, got %f", stats.TotalCost)
	}
}

func TestGetCostByModel(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	records := []*UsageRecord{
		{Source: "claude", SessionID: "s1", Model: "model-a", InputTokens: 100, OutputTokens: 50, CostUSD: 1.5, Timestamp: ts},
		{Source: "claude", SessionID: "s1", Model: "model-b", InputTokens: 200, OutputTokens: 100, CostUSD: 3.0, Timestamp: ts.Add(time.Second)},
		{Source: "claude", SessionID: "s1", Model: "model-a", InputTokens: 100, OutputTokens: 50, CostUSD: 1.5, Timestamp: ts.Add(2 * time.Second)},
	}
	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	result, err := db.GetCostByModel(from, to, "")
	if err != nil {
		t.Fatalf("GetCostByModel: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 models, got %d", len(result))
	}
	// Ordered by cost DESC: model-b (3.0), model-a (3.0)
	if result[0].Model != "model-a" && result[0].Model != "model-b" {
		t.Errorf("unexpected model: %s", result[0].Model)
	}
}

func TestGetCostOverTime(t *testing.T) {
	db := tempDB(t)
	ts1 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	ts2 := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)

	records := []*UsageRecord{
		{Source: "claude", SessionID: "s1", Model: "model-a", CostUSD: 1.0, Timestamp: ts1},
		{Source: "claude", SessionID: "s1", Model: "model-a", CostUSD: 2.0, Timestamp: ts2},
	}
	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	from := ts1.Add(-time.Hour)
	to := ts2.Add(time.Hour)
	result, err := db.GetCostOverTime(from, to, "1d", "", 0)
	if err != nil {
		t.Fatalf("GetCostOverTime: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 points, got %d", len(result))
	}
}

func TestGetCostOverTimeWithTimezone(t *testing.T) {
	db := tempDB(t)
	// 2025-01-01 17:00 UTC = 2025-01-02 01:00 UTC+8
	ts := time.Date(2025, 1, 1, 17, 0, 0, 0, time.UTC)

	records := []*UsageRecord{
		{Source: "claude", SessionID: "s1", Model: "model-a", CostUSD: 1.0, Timestamp: ts},
	}
	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	// UTC+8 local day: 2025-01-02 00:00 ~ 23:59 local = 2025-01-01 16:00 ~ 2025-01-02 15:59 UTC
	from := time.Date(2025, 1, 1, 16, 0, 0, 0, time.UTC)
	to := time.Date(2025, 1, 2, 15, 59, 59, 0, time.UTC)
	result, err := db.GetCostOverTime(from, to, "1d", "", -480)
	if err != nil {
		t.Fatalf("GetCostOverTime: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 point, got %d", len(result))
	}
	// Should bucket into local date 2025-01-02, not UTC date 2025-01-01
	if result[0].Date != "2025-01-02" {
		t.Errorf("expected date 2025-01-02, got %s", result[0].Date)
	}
}

func TestGetSessions(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	sess := &SessionRecord{
		Source: "claude", SessionID: "sess-1", Project: "proj",
		CWD: "/home", StartTime: ts, Prompts: 3,
	}
	if err := db.UpsertSession(sess); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	rec := &UsageRecord{
		Source: "claude", SessionID: "sess-1", Model: "model-a",
		InputTokens: 100, OutputTokens: 50, CostUSD: 1.5, Timestamp: ts,
	}
	if err := db.InsertUsage(rec); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}

	// Insert 2 prompt events in range
	prompts := []*PromptEvent{
		{Source: "claude", SessionID: "sess-1", Timestamp: ts},
		{Source: "claude", SessionID: "sess-1", Timestamp: ts.Add(time.Minute)},
	}
	if err := db.InsertPromptBatch(prompts); err != nil {
		t.Fatalf("InsertPromptBatch: %v", err)
	}

	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	sessions, err := db.GetSessions(from, to, "")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Prompts != 2 {
		t.Errorf("expected 2 prompts from prompt_events, got %d", sessions[0].Prompts)
	}
	if sessions[0].TotalCost != 1.5 {
		t.Errorf("expected cost 1.5, got %f", sessions[0].TotalCost)
	}
}

func TestGetSessionsUsesInRangeActivityForMembership(t *testing.T) {
	db := tempDB(t)
	activity := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	from := activity.Add(-time.Minute)
	to := activity.Add(time.Minute)

	for _, session := range []*SessionRecord{
		{Source: "claude", SessionID: "shared-session", StartTime: activity.Add(-24 * time.Hour)},
		{Source: "codex", SessionID: "shared-session", StartTime: activity},
		{Source: "claude", SessionID: "inactive-session", StartTime: activity.Add(-48 * time.Hour)},
		{Source: "codex", SessionID: "no-activity-session", StartTime: activity},
	} {
		if err := db.UpsertSession(session); err != nil {
			t.Fatalf("UpsertSession(%s/%s): %v", session.Source, session.SessionID, err)
		}
	}
	if err := db.InsertUsage(&UsageRecord{
		Source: "claude", SessionID: "shared-session", Model: "synthetic-model",
		InputTokens: 10, OutputTokens: 5, CostUSD: 1.25, Timestamp: activity,
	}); err != nil {
		t.Fatalf("InsertUsage(in range): %v", err)
	}
	if err := db.InsertUsage(&UsageRecord{
		Source: "claude", SessionID: "inactive-session", Model: "synthetic-model",
		InputTokens: 100, OutputTokens: 50, Timestamp: activity.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatalf("InsertUsage(out of range): %v", err)
	}

	source := testSessionSource("/synthetic/codex-session.jsonl", "shared-session")
	source.Source = "codex"
	sourceID, err := db.UpsertSessionSource(source)
	if err != nil {
		t.Fatalf("UpsertSessionSource: %v", err)
	}
	event := testSessionEvent(sourceID, source.SessionID, 10)
	event.Source = "codex"
	event.Timestamp = activity
	event.Content = "synthetic event-only content"
	if err := db.InsertSessionEvents([]SessionEventRecord{event}); err != nil {
		t.Fatalf("InsertSessionEvents: %v", err)
	}

	sessions, err := db.GetSessions(from, to, "")
	if err != nil {
		t.Fatalf("GetSessions(all): %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %+v, want only two active source-qualified sessions", sessions)
	}
	seen := make(map[string]SessionInfo, len(sessions))
	for _, session := range sessions {
		seen[session.Source+"/"+session.SessionID] = session
	}
	claude, ok := seen["claude/shared-session"]
	if !ok || claude.Tokens != 15 || claude.TotalCost != 1.25 {
		t.Fatalf("in-range usage session = %+v, present=%v", claude, ok)
	}
	if codex, ok := seen["codex/shared-session"]; !ok || codex.Tokens != 0 {
		t.Fatalf("event-only session = %+v, present=%v", codex, ok)
	}

	codexSessions, err := db.GetSessions(from, to, "codex")
	if err != nil {
		t.Fatalf("GetSessions(codex): %v", err)
	}
	if len(codexSessions) != 1 || codexSessions[0].Source != "codex" || codexSessions[0].SessionID != "shared-session" {
		t.Fatalf("source-filtered sessions = %+v", codexSessions)
	}
}

func TestGetDashboardStatsCacheHitRate(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	// input_tokens is non-cached input only (non-overlapping with cache fields)
	records := []*UsageRecord{
		{Source: "claude", SessionID: "s1", Model: "model-a", InputTokens: 1000, OutputTokens: 200, CacheReadInputTokens: 600, Timestamp: ts},
		{Source: "claude", SessionID: "s1", Model: "model-a", InputTokens: 500, OutputTokens: 100, CacheReadInputTokens: 200, Timestamp: ts.Add(time.Second)},
	}
	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	// total_input = (1000+600) + (500+200) = 2300, cache_read=800 → 800/2300
	expected := 800.0 / 2300.0
	if stats.CacheHitRate < expected-0.001 || stats.CacheHitRate > expected+0.001 {
		t.Errorf("expected CacheHitRate ~%f, got %f", expected, stats.CacheHitRate)
	}
	// total_tokens = (1000+600+200) + (500+200+100) = 2600
	if stats.TotalTokens != 2600 {
		t.Errorf("expected TotalTokens 2600, got %d", stats.TotalTokens)
	}
}

func TestGetDashboardStatsCacheHitRateZeroInput(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.CacheHitRate != 0 {
		t.Errorf("expected CacheHitRate 0 for empty data, got %f", stats.CacheHitRate)
	}
}

func TestFileStateUpsert(t *testing.T) {
	db := tempDB(t)

	if err := db.SetFileState("/tmp/a.jsonl", 100, 50, nil); err != nil {
		t.Fatalf("SetFileState: %v", err)
	}
	if err := db.SetFileState("/tmp/a.jsonl", 200, 200, nil); err != nil {
		t.Fatalf("SetFileState update: %v", err)
	}

	size, offset, _, err := db.GetFileState("/tmp/a.jsonl")
	if err != nil {
		t.Fatalf("GetFileState: %v", err)
	}
	if size != 200 || offset != 200 {
		t.Errorf("expected (200,200), got (%d,%d)", size, offset)
	}
}

func TestFileStateScanContext(t *testing.T) {
	db := tempDB(t)

	ctx := &FileScanContext{
		SessionID: "sess-1",
		CWD:       "/home/user",
		Version:   "1.0",
		Model:     "gpt-4",
	}
	if err := db.SetFileState("/tmp/test.jsonl", 1024, 512, ctx); err != nil {
		t.Fatalf("SetFileState with context: %v", err)
	}

	size, offset, got, err := db.GetFileState("/tmp/test.jsonl")
	if err != nil {
		t.Fatalf("GetFileState: %v", err)
	}
	if size != 1024 || offset != 512 {
		t.Errorf("expected (1024,512), got (%d,%d)", size, offset)
	}
	if got == nil {
		t.Fatal("expected non-nil scan context")
	}
	if got.SessionID != "sess-1" || got.CWD != "/home/user" || got.Version != "1.0" || got.Model != "gpt-4" {
		t.Errorf("unexpected context: %+v", got)
	}

	// Update with new context
	ctx2 := &FileScanContext{SessionID: "sess-2", Model: "gpt-5"}
	if err := db.SetFileState("/tmp/test.jsonl", 2048, 2048, ctx2); err != nil {
		t.Fatalf("SetFileState update: %v", err)
	}
	_, _, got2, _ := db.GetFileState("/tmp/test.jsonl")
	if got2 == nil || got2.SessionID != "sess-2" || got2.Model != "gpt-5" {
		t.Errorf("expected updated context, got %+v", got2)
	}

	// Set nil context clears it
	if err := db.SetFileState("/tmp/test.jsonl", 2048, 2048, nil); err != nil {
		t.Fatalf("SetFileState nil ctx: %v", err)
	}
	_, _, got3, _ := db.GetFileState("/tmp/test.jsonl")
	if got3 != nil {
		t.Errorf("expected nil context after clearing, got %+v", got3)
	}
}

func TestMigrateDeletesAllData(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// First open: create DB and insert data from multiple sources
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	records := []*UsageRecord{
		{Source: "claude", SessionID: "c-1", Model: "model-a", InputTokens: 100, OutputTokens: 50, Timestamp: ts},
		{Source: "opencode", SessionID: "oc-1", Model: "model-a", InputTokens: 200, OutputTokens: 100, Timestamp: ts.Add(time.Second)},
	}
	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}
	db.SetFileState("/sessions/claude/test.jsonl", 1024, 512, nil)
	db.UpsertSession(&SessionRecord{Source: "claude", SessionID: "c-1", StartTime: ts, Prompts: 1})
	// Re-run the destructive migration this test covers.
	db.db.Exec("DELETE FROM meta WHERE key = 'migration_002_input_tokens_non_overlapping'")
	db.Close()

	// Second open: migration 002 should delete ALL data
	db2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open after migration: %v", err)
	}
	defer db2.Close()

	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	stats, err := db2.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalCalls != 0 {
		t.Errorf("expected 0 records after migration, got %d", stats.TotalCalls)
	}
	size, offset, _, _ := db2.GetFileState("/sessions/claude/test.jsonl")
	if size != 0 || offset != 0 {
		t.Errorf("expected file_state cleared, got size=%d offset=%d", size, offset)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// First open: migrations run
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Insert data after migration (correct semantics)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	rec := &UsageRecord{
		Source: "claude", SessionID: "c-2", Model: "model-a",
		InputTokens: 100, OutputTokens: 50,
		CacheReadInputTokens: 8000, CacheCreationInputTokens: 2000,
		Timestamp: ts,
	}
	if err := db.InsertUsage(rec); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	db.Close()

	// Second open: migration should NOT delete the new data (already marked done)
	db2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db2.Close()

	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	stats, err := db2.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalCalls != 1 {
		t.Errorf("expected 1 record preserved, got %d", stats.TotalCalls)
	}
}

func TestMigration007PreservesLegacyData(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	legacySchema := `
		CREATE TABLE usage_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT, source TEXT NOT NULL, session_id TEXT NOT NULL,
			model TEXT NOT NULL, input_tokens INTEGER DEFAULT 0, output_tokens INTEGER DEFAULT 0,
			cache_creation_input_tokens INTEGER DEFAULT 0, cache_read_input_tokens INTEGER DEFAULT 0,
			reasoning_output_tokens INTEGER DEFAULT 0, cost_usd REAL DEFAULT 0, timestamp DATETIME NOT NULL,
			project TEXT DEFAULT '', git_branch TEXT DEFAULT ''
		);
		CREATE UNIQUE INDEX idx_usage_dedup ON usage_records(session_id, model, timestamp, input_tokens, output_tokens);
		CREATE TABLE sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT, source TEXT NOT NULL, session_id TEXT NOT NULL UNIQUE,
			project TEXT DEFAULT '', cwd TEXT DEFAULT '', version TEXT DEFAULT '', git_branch TEXT DEFAULT '',
			start_time DATETIME, prompts INTEGER DEFAULT 0
		);
		CREATE TABLE prompt_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT, source TEXT NOT NULL, session_id TEXT NOT NULL, timestamp DATETIME NOT NULL
		);
		CREATE UNIQUE INDEX idx_prompt_dedup ON prompt_events(session_id, timestamp);
		CREATE TABLE file_state (path TEXT PRIMARY KEY, size INTEGER DEFAULT 0, last_offset INTEGER DEFAULT 0, scan_context TEXT DEFAULT '');
		CREATE TABLE pricing (model TEXT PRIMARY KEY, input_cost_per_token REAL DEFAULT 0, output_cost_per_token REAL DEFAULT 0,
			cache_read_input_token_cost REAL DEFAULT 0, cache_creation_input_token_cost REAL DEFAULT 0, updated_at DATETIME);
		CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT DEFAULT '');
		CREATE TABLE config_backups (
			id INTEGER PRIMARY KEY AUTOINCREMENT, tool TEXT NOT NULL, file_path TEXT NOT NULL,
			backup_path TEXT NOT NULL, slot INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP, trigger_type TEXT NOT NULL DEFAULT ''
		);
		CREATE TABLE skills (
			id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL UNIQUE, source_path TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '', enabled INTEGER NOT NULL DEFAULT 1,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE skill_targets (
			skill_id INTEGER NOT NULL, tool TEXT NOT NULL, method TEXT NOT NULL DEFAULT 'symlink',
			enabled INTEGER NOT NULL DEFAULT 1, PRIMARY KEY (skill_id, tool)
		);
		CREATE TABLE skill_variants (
			id INTEGER PRIMARY KEY AUTOINCREMENT, skill_id INTEGER NOT NULL, source_path TEXT NOT NULL,
			origin_tool TEXT NOT NULL DEFAULT 'global', created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (skill_id, source_path)
		);
		INSERT INTO usage_records(source,session_id,model,input_tokens,output_tokens,cost_usd,timestamp,project,git_branch)
			VALUES('claude','legacy-session','legacy-model',17,9,1.25,'2025-01-01 12:00:00+00:00','legacy-project','main');
		INSERT INTO sessions(source,session_id,project,cwd,version,git_branch,start_time,prompts)
			VALUES('claude','legacy-session','legacy-project','/legacy','1.2.3','main','2025-01-01 12:00:00+00:00',4);
		INSERT INTO prompt_events(source,session_id,timestamp)
			VALUES('claude','legacy-session','2025-01-01 12:00:01+00:00');
		INSERT INTO config_backups(tool,file_path,backup_path,slot,trigger_type)
			VALUES('claude','/legacy/config','/legacy/backup',2,'manual');
	`
	if _, err := legacy.Exec(legacySchema); err != nil {
		legacy.Close()
		t.Fatalf("seed legacy schema: %v", err)
	}
	for _, id := range []string{
		"001_fix_opencode_input_tokens", "002_input_tokens_non_overlapping",
		"003_prompt_events_rescan", "004_file_state_scan_context",
		"005_config_manager", "006_skill_variants",
	} {
		if _, err := legacy.Exec(`INSERT INTO meta(key,value) VALUES(?, 'done')`, "migration_"+id); err != nil {
			legacy.Close()
			t.Fatalf("seed migration marker %s: %v", id, err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open legacy database: %v", err)
	}
	defer db.Close()

	var usageCount, inputTokens, outputTokens int64
	var totalCost float64
	if err := db.db.QueryRow(`SELECT COUNT(*), SUM(input_tokens), SUM(output_tokens), SUM(cost_usd) FROM usage_records`).
		Scan(&usageCount, &inputTokens, &outputTokens, &totalCost); err != nil {
		t.Fatalf("read preserved usage: %v", err)
	}
	if usageCount != 1 || inputTokens != 17 || outputTokens != 9 || totalCost != 1.25 {
		t.Errorf("usage changed during migration: count=%d input=%d output=%d cost=%v", usageCount, inputTokens, outputTokens, totalCost)
	}

	checks := []struct {
		query string
		want  int
	}{
		{`SELECT COUNT(*) FROM sessions WHERE source='claude' AND session_id='legacy-session' AND prompts=4`, 1},
		{`SELECT COUNT(*) FROM prompt_events WHERE source='claude' AND session_id='legacy-session'`, 1},
		{`SELECT COUNT(*) FROM config_backups WHERE tool='claude' AND slot=2 AND trigger_type='manual'`, 1},
		{`SELECT COUNT(*) FROM meta WHERE key='migration_007_session_event_index' AND value='done'`, 1},
	}
	for _, check := range checks {
		var got int
		if err := db.db.QueryRow(check.query).Scan(&got); err != nil {
			t.Fatalf("preservation query %q: %v", check.query, err)
		}
		if got != check.want {
			t.Errorf("preservation query %q: got %d, want %d", check.query, got, check.want)
		}
	}
}

func TestMigration007RollsBackSchemaWhenFTSCreationFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "atomic.db")
	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open seed database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seed database: %v", err)
	}

	raw, err := sql.Open("sqlite", dbPath+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("open raw database: %v", err)
	}
	revert007 := `
		DROP TRIGGER session_events_fts_update;
		DROP TRIGGER session_events_fts_delete;
		DROP TRIGGER session_events_fts_insert;
		DROP TABLE session_events_fts;
		DROP TABLE session_events;
		DROP TABLE session_sources;
		CREATE TABLE sessions_legacy (
			id INTEGER PRIMARY KEY AUTOINCREMENT, source TEXT NOT NULL, session_id TEXT NOT NULL UNIQUE,
			project TEXT DEFAULT '', cwd TEXT DEFAULT '', version TEXT DEFAULT '', git_branch TEXT DEFAULT '',
			start_time DATETIME, prompts INTEGER DEFAULT 0
		);
		INSERT INTO sessions_legacy SELECT * FROM sessions;
		DROP TABLE sessions;
		ALTER TABLE sessions_legacy RENAME TO sessions;
		DROP INDEX idx_usage_dedup;
		CREATE UNIQUE INDEX idx_usage_dedup ON usage_records(session_id, model, timestamp, input_tokens, output_tokens);
		DROP INDEX idx_prompt_dedup;
		CREATE UNIQUE INDEX idx_prompt_dedup ON prompt_events(session_id, timestamp);
		DELETE FROM meta WHERE key='migration_007_session_event_index';
		CREATE TABLE session_events_fts(blocker TEXT);
	`
	if _, err := raw.Exec(revert007); err != nil {
		raw.Close()
		t.Fatalf("restore pre-007 schema: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close raw database: %v", err)
	}

	if reopened, err := Open(dbPath); err == nil {
		reopened.Close()
		t.Fatal("expected migration failure from conflicting FTS table")
	}

	check, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database after failed migration: %v", err)
	}
	defer check.Close()
	var markerCount, sourceTableCount int
	if err := check.QueryRow(`SELECT COUNT(*) FROM meta WHERE key='migration_007_session_event_index'`).Scan(&markerCount); err != nil {
		t.Fatalf("read migration marker: %v", err)
	}
	if err := check.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='session_sources'`).Scan(&sourceTableCount); err != nil {
		t.Fatalf("read session_sources schema: %v", err)
	}
	if markerCount != 0 || sourceTableCount != 0 {
		t.Fatalf("failed migration left partial state: marker=%d session_sources=%d", markerCount, sourceTableCount)
	}
	if _, err := check.Exec(`INSERT INTO sessions(source,session_id) VALUES('claude','same'),('codex','same')`); err == nil {
		t.Fatal("sessions table rebuild was not rolled back")
	}
}

func TestInsertPromptBatchAndDedup(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	events := []*PromptEvent{
		{Source: "claude", SessionID: "s1", Timestamp: ts},
		{Source: "claude", SessionID: "s1", Timestamp: ts.Add(time.Minute)},
	}
	if err := db.InsertPromptBatch(events); err != nil {
		t.Fatalf("InsertPromptBatch: %v", err)
	}
	// Insert same events again — should be ignored (dedup)
	if err := db.InsertPromptBatch(events); err != nil {
		t.Fatalf("InsertPromptBatch duplicate: %v", err)
	}

	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	var count int
	db.db.QueryRow("SELECT COUNT(*) FROM prompt_events WHERE timestamp BETWEEN ? AND ?", from, to).Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 prompt events after dedup, got %d", count)
	}
}

func TestGetDashboardStatsPromptsTimeRange(t *testing.T) {
	db := tempDB(t)
	day1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	day2 := time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC)

	// Insert prompt events across two days for the same session
	events := []*PromptEvent{
		{Source: "claude", SessionID: "s1", Timestamp: day1},
		{Source: "claude", SessionID: "s1", Timestamp: day1.Add(time.Hour)},
		{Source: "claude", SessionID: "s1", Timestamp: day2},
	}
	if err := db.InsertPromptBatch(events); err != nil {
		t.Fatalf("InsertPromptBatch: %v", err)
	}

	// Also insert usage records so the session shows up
	rec := &UsageRecord{Source: "claude", SessionID: "s1", Model: "m", InputTokens: 1, Timestamp: day1}
	db.InsertUsage(rec)
	rec2 := &UsageRecord{Source: "claude", SessionID: "s1", Model: "m", InputTokens: 1, Timestamp: day2}
	db.InsertUsage(rec2)

	// Query day1 only — should get 2 prompts, not 3
	from1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to1 := time.Date(2025, 1, 1, 23, 59, 59, 0, time.UTC)
	stats, err := db.GetDashboardStats(from1, to1, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalPrompts != 2 {
		t.Errorf("expected 2 prompts for day1, got %d", stats.TotalPrompts)
	}

	// Query day2 only — should get 1 prompt
	from2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	to2 := time.Date(2025, 1, 2, 23, 59, 59, 0, time.UTC)
	stats2, err := db.GetDashboardStats(from2, to2, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats2.TotalPrompts != 1 {
		t.Errorf("expected 1 prompt for day2, got %d", stats2.TotalPrompts)
	}
}
