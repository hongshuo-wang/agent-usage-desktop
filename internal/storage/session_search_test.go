package storage

import (
	"database/sql"
	"testing"
	"time"
)

type countingSessionQueryer struct {
	db      *sql.DB
	queries int
}

func TestSessionUnknownPriceUsesStoredStatus(t *testing.T) {
	db := tempDB(t)
	eventTime := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	if _, err := db.CreatePricingSnapshot(eventTime.Add(-time.Hour), "litellm", "fuzzy", []PricingSnapshotEntry{
		{Model: "acme/claude-sonnet-4-6", InputCostPerToken: 1},
	}); err != nil {
		t.Fatalf("CreatePricingSnapshot: %v", err)
	}
	if err := db.InsertUsage(&UsageRecord{
		Source: "claude", SessionID: "priced", Model: "claude-sonnet-4.6", Timestamp: eventTime, InputTokens: 1,
	}); err != nil {
		t.Fatalf("InsertUsage(priced): %v", err)
	}
	if err := db.PriceUnpricedUsage(func(_, _ int64, _, _ int64, _ [4]float64) float64 { return 1 }); err != nil {
		t.Fatalf("PriceUnpricedUsage: %v", err)
	}
	if err := db.InsertUsageBatch([]*UsageRecord{
		{Source: "claude", SessionID: "unpriced", Model: "currently-known", Timestamp: eventTime.Add(time.Second), InputTokens: 1, CostUSD: 5},
		{Source: "claude", SessionID: "legacy", Model: "legacy-model", Timestamp: eventTime.Add(2 * time.Second), InputTokens: 1, CostUSD: 2},
	}); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}
	if _, err := db.db.Exec(`UPDATE usage_records SET pricing_status='legacy' WHERE session_id='legacy'`); err != nil {
		t.Fatalf("mark legacy usage: %v", err)
	}
	if err := db.UpsertPricing("currently-known", 1, 0, 0, 0); err != nil {
		t.Fatalf("UpsertPricing: %v", err)
	}

	sessions, err := db.SearchSessions(SessionQuery{
		From: eventTime.Add(-time.Minute), To: eventTime.Add(time.Minute), Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchSessions: %v", err)
	}
	byID := make(map[string]SessionSummary, len(sessions))
	for _, session := range sessions {
		byID[session.SessionID] = session
	}
	if byID["priced"].UnknownPrice {
		t.Error("fuzzy-resolved priced session was marked unknown without an exact live pricing key")
	}
	if !byID["unpriced"].UnknownPrice {
		t.Error("stored unpriced session was treated as known because a live pricing key exists")
	}
	if got := byID["unpriced"].TotalCost; got != 0 {
		t.Errorf("unpriced session cost = %v, want 0", got)
	}
	if byID["legacy"].UnknownPrice {
		t.Error("legacy session was marked unknown despite its preserved stored cost")
	}
}

func TestSearchSessionsProjectFallsBackToCWDAndSessionID(t *testing.T) {
	db := tempDB(t)
	timestamp := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	if err := db.UpsertSession(&SessionRecord{Source: "claude", SessionID: "cwd-session", CWD: "/work/cwd-project", StartTime: timestamp}); err != nil {
		t.Fatalf("UpsertSession cwd fallback: %v", err)
	}
	if err := db.UpsertSession(&SessionRecord{Source: "claude", SessionID: "windows-session", CWD: `C:\work\windows-project\`, StartTime: timestamp}); err != nil {
		t.Fatalf("UpsertSession windows cwd fallback: %v", err)
	}
	if err := db.UpsertSession(&SessionRecord{Source: "claude", SessionID: "relative-session", CWD: `./relative-project`, StartTime: timestamp}); err != nil {
		t.Fatalf("UpsertSession relative cwd fallback: %v", err)
	}
	if err := db.UpsertSession(&SessionRecord{Source: "claude", SessionID: "usage-project-session", CWD: "/work/cwd-fallback", StartTime: timestamp}); err != nil {
		t.Fatalf("UpsertSession usage project metadata: %v", err)
	}
	if err := db.InsertUsageBatch([]*UsageRecord{
		{Source: "claude", SessionID: "cwd-session", Model: "model", Timestamp: timestamp},
		{Source: "claude", SessionID: "id-session", Model: "model", Timestamp: timestamp.Add(time.Second)},
		{Source: "claude", SessionID: "windows-session", Model: "model", Timestamp: timestamp.Add(2 * time.Second)},
		{Source: "claude", SessionID: "relative-session", Model: "model", Timestamp: timestamp.Add(3 * time.Second)},
		{Source: "claude", SessionID: "usage-project-session", Model: "model", Project: "usage-project", Timestamp: timestamp.Add(4 * time.Second)},
	}); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	byProject, err := db.SearchSessions(SessionQuery{From: timestamp.Add(-time.Minute), To: timestamp.Add(time.Minute), Project: "cwd-project", Limit: 10})
	if err != nil {
		t.Fatalf("SearchSessions cwd project: %v", err)
	}
	if len(byProject) != 1 || byProject[0].SessionID != "cwd-session" || byProject[0].Project != "cwd-project" {
		t.Fatalf("cwd project search = %+v, want cwd-session with project cwd-project", byProject)
	}

	byID, err := db.SearchSessions(SessionQuery{From: timestamp.Add(-time.Minute), To: timestamp.Add(time.Minute), Project: "id-session", Limit: 10})
	if err != nil {
		t.Fatalf("SearchSessions session ID project: %v", err)
	}
	if len(byID) != 1 || byID[0].SessionID != "id-session" || byID[0].Project != "id-session" {
		t.Fatalf("session ID project search = %+v, want id-session fallback", byID)
	}

	byWindows, err := db.SearchSessions(SessionQuery{From: timestamp.Add(-time.Minute), To: timestamp.Add(time.Minute), Project: "windows-project", Limit: 10})
	if err != nil {
		t.Fatalf("SearchSessions windows project: %v", err)
	}
	if len(byWindows) != 1 || byWindows[0].SessionID != "windows-session" || byWindows[0].Project != "windows-project" {
		t.Fatalf("windows project search = %+v, want windows-session fallback", byWindows)
	}

	byRelative, err := db.SearchSessions(SessionQuery{From: timestamp.Add(-time.Minute), To: timestamp.Add(time.Minute), Project: "relative-project", Limit: 10})
	if err != nil {
		t.Fatalf("SearchSessions relative project: %v", err)
	}
	if len(byRelative) != 1 || byRelative[0].SessionID != "relative-session" || byRelative[0].Project != "relative-project" {
		t.Fatalf("relative project search = %+v, want relative-session fallback", byRelative)
	}

	byUsageProject, err := db.SearchSessions(SessionQuery{From: timestamp.Add(-time.Minute), To: timestamp.Add(time.Minute), Project: "usage-project", Limit: 10})
	if err != nil {
		t.Fatalf("SearchSessions usage project: %v", err)
	}
	if len(byUsageProject) != 1 || byUsageProject[0].SessionID != "usage-project-session" || byUsageProject[0].Project != "usage-project" {
		t.Fatalf("usage project search = %+v, want usage-project-session", byUsageProject)
	}
	byCWDWhenUsageProjectExists, err := db.SearchSessions(SessionQuery{From: timestamp.Add(-time.Minute), To: timestamp.Add(time.Minute), Project: "cwd-fallback", Limit: 10})
	if err != nil {
		t.Fatalf("SearchSessions cwd with usage project: %v", err)
	}
	if len(byCWDWhenUsageProjectExists) != 0 {
		t.Fatalf("cwd fallback incorrectly matched usage-project session: %+v", byCWDWhenUsageProjectExists)
	}
}

func (q *countingSessionQueryer) Query(statement string, args ...interface{}) (*sql.Rows, error) {
	q.queries++
	return q.db.Query(statement, args...)
}

func TestSearchSessionsCollapsesUnknownSourceStatuses(t *testing.T) {
	db := tempDB(t)
	timestamp := time.Date(2025, 7, 24, 1, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&UsageRecord{
		Source: "claude", SessionID: "unknown-statuses", Model: "test-model",
		Timestamp: timestamp,
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	for index, status := range []string{"degraded", "offline"} {
		if _, err := db.UpsertSessionSource(&SessionSource{
			Source: "claude", SessionID: "unknown-statuses",
			Path:          "/sessions/unknown-status-" + string(rune('a'+index)) + ".jsonl",
			ParserVersion: "v1", CoverageStatus: "complete", SourceStatus: status,
		}); err != nil {
			t.Fatalf("UpsertSessionSource(%q): %v", status, err)
		}
	}

	sessions, err := db.SearchSessions(SessionQuery{
		From: timestamp.Add(-time.Hour), To: timestamp.Add(time.Hour), Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1: %+v", len(sessions), sessions)
	}
	if got := sessions[0].SourceStatus; got != "unavailable" {
		t.Errorf("source_status = %q, want %q", got, "unavailable")
	}
}

func TestSearchSessionsUsesFixedQueryCount(t *testing.T) {
	db := tempDB(t)
	timestamp := time.Date(2025, 7, 24, 1, 0, 0, 0, time.UTC)
	for _, sessionID := range []string{"one", "two", "three"} {
		if err := db.InsertUsage(&UsageRecord{
			Source: "claude", SessionID: sessionID, Model: "test-model", Timestamp: timestamp,
		}); err != nil {
			t.Fatalf("InsertUsage(%q): %v", sessionID, err)
		}
	}

	for _, limit := range []int{1, 3} {
		queryer := &countingSessionQueryer{db: db.db}
		sessions, err := searchSessions(queryer, SessionQuery{
			From: timestamp.Add(-time.Hour), To: timestamp.Add(time.Hour), Limit: limit,
		})
		if err != nil {
			t.Fatalf("limit %d SearchSessions: %v", limit, err)
		}
		if len(sessions) != limit {
			t.Errorf("limit %d returned %d sessions", limit, len(sessions))
		}
		if queryer.queries != 6 {
			t.Errorf("limit %d executed %d SQL queries, want 6", limit, queryer.queries)
		}
	}
}

func TestSearchSessionsKeepsFirstTitleOutsideSelectedRange(t *testing.T) {
	db := tempDB(t)
	timestamp := time.Date(2025, 7, 24, 1, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&UsageRecord{
		Source: "claude", SessionID: "title-history", Model: "test-model", Timestamp: timestamp,
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	_, err := db.UpsertSessionSourceWithEvents(&SessionSource{
		Source: "claude", SessionID: "title-history", Path: "/sessions/title-history.jsonl",
		ParserVersion: "v1", CoverageStatus: "complete", SourceStatus: "available",
	}, []SessionEventRecord{
		{Source: "claude", SessionID: "title-history", EventType: "user_message", Timestamp: timestamp.Add(-48 * time.Hour), Content: "Original title", RawOffset: 0},
		{Source: "claude", SessionID: "title-history", EventType: "user_message", Timestamp: timestamp, Content: "In-range follow-up", RawOffset: 100},
	})
	if err != nil {
		t.Fatalf("UpsertSessionSourceWithEvents: %v", err)
	}
	sessions, err := db.SearchSessions(SessionQuery{
		From: timestamp.Add(-time.Hour), To: timestamp.Add(time.Hour), Limit: 10,
	})
	if err != nil {
		t.Fatalf("SearchSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Title != "Original title" {
		t.Fatalf("sessions = %+v, want all-time first user-message title", sessions)
	}
}

func TestSearchSessionsSkipsSyntheticContextWhenChoosingTitle(t *testing.T) {
	db := tempDB(t)
	timestamp := time.Date(2025, 7, 24, 1, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&UsageRecord{Source: "codex", SessionID: "synthetic-title", Model: "test-model", Timestamp: timestamp}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	_, err := db.UpsertSessionSourceWithEvents(&SessionSource{
		Source: "codex", SessionID: "synthetic-title", Path: "/sessions/synthetic-title.jsonl",
		ParserVersion: "v1", CoverageStatus: "complete", SourceStatus: "available",
	}, []SessionEventRecord{
		{Source: "codex", SessionID: "synthetic-title", EventType: "user_message", Timestamp: timestamp, Content: "<environment_context> <cwd>/work</cwd>", RawOffset: 0},
		{Source: "codex", SessionID: "synthetic-title", EventType: "user_message", Timestamp: timestamp.Add(time.Second), Content: "<user_shell_command> <command>npm test</command>", RawOffset: 100},
		{Source: "codex", SessionID: "synthetic-title", EventType: "user_message", Timestamp: timestamp.Add(2 * time.Second), Content: `<image name=[Image #1] path="/tmp/synthetic.png">`, RawOffset: 200},
		{Source: "codex", SessionID: "synthetic-title", EventType: "user_message", Timestamp: timestamp.Add(3 * time.Second), Content: "重构首页吞吐图", RawOffset: 300},
	})
	if err != nil {
		t.Fatalf("UpsertSessionSourceWithEvents: %v", err)
	}
	sessions, err := db.SearchSessions(SessionQuery{From: timestamp.Add(-time.Minute), To: timestamp.Add(time.Minute), Limit: 10})
	if err != nil {
		t.Fatalf("SearchSessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].Title != "重构首页吞吐图" {
		t.Fatalf("sessions = %+v, want human title", sessions)
	}
}
