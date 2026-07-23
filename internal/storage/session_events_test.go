package storage

import (
	"testing"
	"time"
)

func testSessionSource(path, sessionID string) *SessionSource {
	return &SessionSource{
		Source:         "claude",
		SessionID:      sessionID,
		SourceKind:     "jsonl",
		Path:           path,
		ParserVersion:  "v1",
		HeadHash:       "head",
		FileSize:       2048,
		IndexedOffset:  1024,
		CoverageStatus: "complete",
		SourceStatus:   "available",
		LastIndexedAt:  time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

func testSessionEvent(sourceID int64, sessionID string, rawOffset int64) SessionEventRecord {
	duration := int64(125)
	return SessionEventRecord{
		SessionSourceID: sourceID,
		Source:          "claude",
		SessionID:       sessionID,
		EventType:       "tool_result",
		SourceEventType: "assistant",
		Timestamp:       time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
		Role:            "assistant",
		Content:         "inspect the migration schema",
		ToolName:        "Read",
		ToolCallID:      "tool-1",
		ToolInput:       `{"path":"internal/storage/sqlite.go"}`,
		ToolOutput:      "CREATE VIRTUAL TABLE session_events_fts",
		EventStatus:     "completed",
		DurationMS:      &duration,
		RawOffset:       rawOffset,
		RawLength:       320,
		RawIndex:        0,
	}
}

func TestSessionSourceAndEventInsertDedup(t *testing.T) {
	db := tempDB(t)
	source := testSessionSource("/sessions/one.jsonl", "session-one")
	id, err := db.UpsertSessionSource(source)
	if err != nil {
		t.Fatalf("UpsertSessionSource: %v", err)
	}
	source.FileSize = 4096
	sameID, err := db.UpsertSessionSource(source)
	if err != nil {
		t.Fatalf("UpsertSessionSource update: %v", err)
	}
	if sameID != id {
		t.Fatalf("source id changed across path upsert: %d -> %d", id, sameID)
	}

	event := testSessionEvent(id, source.SessionID, 10)
	if err := db.InsertSessionEvents([]SessionEventRecord{event, event}); err != nil {
		t.Fatalf("InsertSessionEvents: %v", err)
	}
	if err := db.InsertSessionEvents(nil); err != nil {
		t.Fatalf("empty InsertSessionEvents: %v", err)
	}

	gotSource, err := db.GetSessionSourceByPath(source.Path)
	if err != nil {
		t.Fatalf("GetSessionSourceByPath: %v", err)
	}
	if gotSource == nil || gotSource.ID != id || gotSource.FileSize != 4096 || gotSource.LastIndexedAt.IsZero() {
		t.Fatalf("unexpected source: %+v", gotSource)
	}
	events, err := db.ListSessionEvents(source.Source, source.SessionID, 10, 0)
	if err != nil {
		t.Fatalf("ListSessionEvents: %v", err)
	}
	if len(events) != 1 || events[0].DurationMS == nil || *events[0].DurationMS != 125 {
		t.Fatalf("unexpected deduped events: %+v", events)
	}
	gotEvent, err := db.GetSessionEvent(source.Source, source.SessionID, events[0].ID)
	if err != nil {
		t.Fatalf("GetSessionEvent: %v", err)
	}
	if gotEvent == nil || gotEvent.RawOffset != 10 || gotEvent.ToolCallID != "tool-1" {
		t.Fatalf("unexpected event: %+v", gotEvent)
	}
}

func TestSessionEventsFTSTriggers(t *testing.T) {
	db := tempDB(t)
	source := testSessionSource("/sessions/fts.jsonl", "fts-session")
	sourceID, err := db.UpsertSessionSource(source)
	if err != nil {
		t.Fatalf("UpsertSessionSource: %v", err)
	}
	event := testSessionEvent(sourceID, source.SessionID, 20)
	if err := db.InsertSessionEvents([]SessionEventRecord{event}); err != nil {
		t.Fatalf("InsertSessionEvents: %v", err)
	}

	for _, term := range []string{"migration", "Read", "sqlite", "VIRTUAL"} {
		if got := ftsMatchCount(t, db, term); got != 1 {
			t.Errorf("MATCH %q after insert = %d, want 1", term, got)
		}
	}
	var eventID int64
	if err := db.db.QueryRow(`SELECT id FROM session_events`).Scan(&eventID); err != nil {
		t.Fatalf("read event id: %v", err)
	}
	if _, err := db.db.Exec(`UPDATE session_events SET content='replacement phrase', tool_name='', tool_input='', tool_output='' WHERE id=?`, eventID); err != nil {
		t.Fatalf("update indexed event: %v", err)
	}
	for _, term := range []string{"migration", "Read", "sqlite", "VIRTUAL"} {
		if got := ftsMatchCount(t, db, term); got != 0 {
			t.Errorf("old FTS term %q remained after update: %d", term, got)
		}
	}
	if got := ftsMatchCount(t, db, "replacement"); got != 1 {
		t.Errorf("new FTS content missing after update: %d", got)
	}
	if _, err := db.db.Exec(`DELETE FROM session_events WHERE id=?`, eventID); err != nil {
		t.Fatalf("delete indexed event: %v", err)
	}
	if got := ftsMatchCount(t, db, "replacement"); got != 0 {
		t.Errorf("FTS content remained after delete: %d", got)
	}
}

func TestClearSessionContentPreservesUsageAndSession(t *testing.T) {
	db := tempDB(t)
	source := testSessionSource("/sessions/clear.jsonl", "clear-session")
	sourceID, err := db.UpsertSessionSource(source)
	if err != nil {
		t.Fatalf("UpsertSessionSource: %v", err)
	}
	if err := db.InsertSessionEvents([]SessionEventRecord{testSessionEvent(sourceID, source.SessionID, 30)}); err != nil {
		t.Fatalf("InsertSessionEvents: %v", err)
	}
	if err := db.UpsertSession(&SessionRecord{Source: source.Source, SessionID: source.SessionID, Project: "kept"}); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if err := db.InsertUsage(&UsageRecord{Source: source.Source, SessionID: source.SessionID, Model: "m", InputTokens: 3, OutputTokens: 2, Timestamp: time.Now()}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}

	if err := db.ClearSessionContent(source.Source, source.SessionID, "stale_parser"); err != nil {
		t.Fatalf("ClearSessionContent: %v", err)
	}
	assertTableCount(t, db, "session_events", 0)
	assertTableCount(t, db, "sessions", 1)
	assertTableCount(t, db, "usage_records", 1)
	if got := ftsMatchCount(t, db, "migration"); got != 0 {
		t.Errorf("FTS content remained after clear: %d", got)
	}
	got, err := db.GetSessionSourceByPath(source.Path)
	if err != nil {
		t.Fatalf("GetSessionSourceByPath: %v", err)
	}
	if got == nil || got.SourceStatus != "stale_parser" {
		t.Fatalf("unexpected source after clear: %+v", got)
	}
}

func TestDeleteSourceIndexCascadesOnlyIndexedEvents(t *testing.T) {
	db := tempDB(t)
	source := testSessionSource("/sessions/delete.jsonl", "delete-session")
	sourceID, err := db.UpsertSessionSource(source)
	if err != nil {
		t.Fatalf("UpsertSessionSource: %v", err)
	}
	if err := db.InsertSessionEvents([]SessionEventRecord{testSessionEvent(sourceID, source.SessionID, 40)}); err != nil {
		t.Fatalf("InsertSessionEvents: %v", err)
	}
	if err := db.UpsertSession(&SessionRecord{Source: source.Source, SessionID: source.SessionID, Project: "kept"}); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if err := db.InsertUsage(&UsageRecord{Source: source.Source, SessionID: source.SessionID, Model: "m", Timestamp: time.Now()}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}

	if err := db.DeleteSourceIndex(source.Path); err != nil {
		t.Fatalf("DeleteSourceIndex: %v", err)
	}
	assertTableCount(t, db, "session_sources", 0)
	assertTableCount(t, db, "session_events", 0)
	assertTableCount(t, db, "sessions", 1)
	assertTableCount(t, db, "usage_records", 1)
}

func TestMarkMissingSessionSourcesClearsOnlyUnseenContent(t *testing.T) {
	db := tempDB(t)
	seen := testSessionSource("/sessions/seen.jsonl", "seen-session")
	missing := testSessionSource("/sessions/missing.jsonl", "missing-session")
	seen.SourceStatus = "missing_source"
	seenID, err := db.UpsertSessionSource(seen)
	if err != nil {
		t.Fatalf("UpsertSessionSource(seen): %v", err)
	}
	missingID, err := db.UpsertSessionSource(missing)
	if err != nil {
		t.Fatalf("UpsertSessionSource(missing): %v", err)
	}
	if err := db.InsertSessionEvents([]SessionEventRecord{
		testSessionEvent(seenID, seen.SessionID, 50),
		testSessionEvent(missingID, missing.SessionID, 60),
	}); err != nil {
		t.Fatalf("InsertSessionEvents: %v", err)
	}

	if err := db.MarkMissingSessionSources("claude", map[string]struct{}{seen.Path: {}}); err != nil {
		t.Fatalf("MarkMissingSessionSources: %v", err)
	}
	seenAfter, _ := db.GetSessionSourceByPath(seen.Path)
	missingAfter, _ := db.GetSessionSourceByPath(missing.Path)
	if seenAfter == nil || seenAfter.SourceStatus != "available" {
		t.Fatalf("seen source not available: %+v", seenAfter)
	}
	if missingAfter == nil || missingAfter.SourceStatus != "missing_source" {
		t.Fatalf("missing source status not retained: %+v", missingAfter)
	}
	seenEvents, err := db.ListSessionEvents("claude", seen.SessionID, 10, 0)
	if err != nil {
		t.Fatalf("ListSessionEvents(seen): %v", err)
	}
	missingEvents, err := db.ListSessionEvents("claude", missing.SessionID, 10, 0)
	if err != nil {
		t.Fatalf("ListSessionEvents(missing): %v", err)
	}
	if len(seenEvents) != 1 || len(missingEvents) != 0 {
		t.Fatalf("unexpected event retention: seen=%d missing=%d", len(seenEvents), len(missingEvents))
	}
}

func ftsMatchCount(t *testing.T, db *DB, query string) int {
	t.Helper()
	var count int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM session_events_fts WHERE session_events_fts MATCH ?`, query).Scan(&count); err != nil {
		t.Fatalf("FTS MATCH %q: %v", query, err)
	}
	return count
}

func assertTableCount(t *testing.T, db *DB, table string, want int) {
	t.Helper()
	var got int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&got); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Errorf("%s count = %d, want %d", table, got, want)
	}
}
