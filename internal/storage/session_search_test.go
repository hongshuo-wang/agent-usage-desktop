package storage

import (
	"testing"
	"time"
)

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
