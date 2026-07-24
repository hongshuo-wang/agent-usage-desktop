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
