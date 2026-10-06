package storage

import (
	"testing"
	"time"
)

func TestPruneSessionEventsKeepsRecentAndLeavesSourcesIndexed(t *testing.T) {
	db := tempDB(t)

	oldTime := time.Now().UTC().AddDate(0, 0, -90)
	newTime := time.Now().UTC().Add(-time.Hour)

	source := testSessionSource("/sessions/one.jsonl", "session-one")
	sourceID, err := db.UpsertSessionSource(source)
	if err != nil {
		t.Fatalf("UpsertSessionSource: %v", err)
	}
	oldEvent := testSessionEvent(sourceID, "session-one", 100)
	oldEvent.Timestamp = oldTime
	newEvent := testSessionEvent(sourceID, "session-one", 200)
	newEvent.Timestamp = newTime
	if err := db.InsertSessionEvents([]SessionEventRecord{oldEvent, newEvent}); err != nil {
		t.Fatalf("InsertSessionEvents: %v", err)
	}

	deleted, err := db.PruneSessionEvents(30)
	if err != nil {
		t.Fatalf("PruneSessionEvents: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted event, got %d", deleted)
	}

	events, err := db.ListSessionEvents("claude", "session-one", 10, 0)
	if err != nil {
		t.Fatalf("GetSessionEvents: %v", err)
	}
	if len(events) != 1 || !events[0].Timestamp.Equal(newTime) {
		t.Fatalf("expected only the recent event, got %+v", events)
	}

	// A pruned source must stay "available" and keep its offset, otherwise the
	// next collector scan would treat it as unindexed and re-insert everything.
	after, err := db.GetSessionSourceByPath("/sessions/one.jsonl")
	if err != nil {
		t.Fatalf("GetSessionSourceByPath: %v", err)
	}
	if after.SourceStatus != "available" || after.IndexedOffset != source.IndexedOffset {
		t.Fatalf("prune mutated source: status=%q offset=%d", after.SourceStatus, after.IndexedOffset)
	}

	// VACUUM is what actually returns the freed pages to the filesystem.
	if err := db.Vacuum(); err != nil {
		t.Fatalf("Vacuum: %v", err)
	}
	if events, err := db.ListSessionEvents("claude", "session-one", 10, 0); err != nil || len(events) != 1 {
		t.Fatalf("surviving event lost after vacuum: %d events, err=%v", len(events), err)
	}

	// The retention pass is throttled to once per 24h.
	again := testSessionEvent(sourceID, "session-one", 300)
	again.Timestamp = oldTime.Add(time.Minute)
	if err := db.InsertSessionEvents([]SessionEventRecord{again}); err != nil {
		t.Fatalf("InsertSessionEvents: %v", err)
	}
	deleted, err = db.PruneSessionEvents(30)
	if err != nil {
		t.Fatalf("PruneSessionEvents second: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected throttled no-op, got %d deleted", deleted)
	}
}

func TestPruneSessionEventsDisabledWhenRetentionIsZero(t *testing.T) {
	db := tempDB(t)
	sourceID, err := db.UpsertSessionSource(testSessionSource("/sessions/one.jsonl", "session-one"))
	if err != nil {
		t.Fatalf("UpsertSessionSource: %v", err)
	}
	event := testSessionEvent(sourceID, "session-one", 100)
	event.Timestamp = time.Now().UTC().AddDate(0, 0, -900)
	if err := db.InsertSessionEvents([]SessionEventRecord{event}); err != nil {
		t.Fatalf("InsertSessionEvents: %v", err)
	}
	deleted, err := db.PruneSessionEvents(0)
	if err != nil {
		t.Fatalf("PruneSessionEvents: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected retention 0 to disable pruning, got %d deleted", deleted)
	}
	if events, err := db.ListSessionEvents("claude", "session-one", 10, 0); err != nil || len(events) != 1 {
		t.Fatalf("expected event to survive, got %d events, err=%v", len(events), err)
	}
}

func TestPurgeSessionEventsIgnoresThrottleAndCanWipeEverything(t *testing.T) {
	db := tempDB(t)
	sourceID, err := db.UpsertSessionSource(testSessionSource("/sessions/one.jsonl", "session-one"))
	if err != nil {
		t.Fatalf("UpsertSessionSource: %v", err)
	}
	old := testSessionEvent(sourceID, "session-one", 100)
	old.Timestamp = time.Now().UTC().AddDate(0, 0, -90)
	recent := testSessionEvent(sourceID, "session-one", 200)
	recent.Timestamp = time.Now().UTC().Add(-time.Hour)
	if err := db.InsertSessionEvents([]SessionEventRecord{old, recent}); err != nil {
		t.Fatalf("InsertSessionEvents: %v", err)
	}
	if _, err := db.PruneSessionEvents(30); err != nil {
		t.Fatalf("PruneSessionEvents: %v", err)
	}

	// The automatic pass is now throttled, but a user-triggered purge is not.
	older := testSessionEvent(sourceID, "session-one", 300)
	older.Timestamp = time.Now().UTC().AddDate(0, 0, -60)
	if err := db.InsertSessionEvents([]SessionEventRecord{older}); err != nil {
		t.Fatalf("InsertSessionEvents: %v", err)
	}
	deleted, err := db.PurgeSessionEvents(30)
	if err != nil {
		t.Fatalf("PurgeSessionEvents: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected the throttle to be bypassed, got %d deleted", deleted)
	}

	deleted, err = db.PurgeSessionEvents(0)
	if err != nil {
		t.Fatalf("PurgeSessionEvents(0): %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected retention 0 to wipe everything, got %d deleted", deleted)
	}
	if events, err := db.ListSessionEvents("claude", "session-one", 10, 0); err != nil || len(events) != 0 {
		t.Fatalf("expected an empty index, got %d events, err=%v", len(events), err)
	}
}

func TestForgetPruneThrottleLetsTheNextPassRun(t *testing.T) {
	db := tempDB(t)
	sourceID, err := db.UpsertSessionSource(testSessionSource("/sessions/one.jsonl", "session-one"))
	if err != nil {
		t.Fatalf("UpsertSessionSource: %v", err)
	}
	event := testSessionEvent(sourceID, "session-one", 100)
	event.Timestamp = time.Now().UTC().AddDate(0, 0, -90)
	if err := db.InsertSessionEvents([]SessionEventRecord{event}); err != nil {
		t.Fatalf("InsertSessionEvents: %v", err)
	}
	if _, err := db.PruneSessionEvents(30); err != nil {
		t.Fatalf("PruneSessionEvents: %v", err)
	}
	stale := testSessionEvent(sourceID, "session-one", 200)
	stale.Timestamp = time.Now().UTC().AddDate(0, 0, -90)
	if err := db.InsertSessionEvents([]SessionEventRecord{stale}); err != nil {
		t.Fatalf("InsertSessionEvents: %v", err)
	}
	if err := db.ForgetPruneThrottle(); err != nil {
		t.Fatalf("ForgetPruneThrottle: %v", err)
	}
	deleted, err := db.PruneSessionEvents(30)
	if err != nil {
		t.Fatalf("PruneSessionEvents: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected the throttle to be forgotten, got %d deleted", deleted)
	}
}

func TestSizeBytesReportsTheDatabaseFile(t *testing.T) {
	db := tempDB(t)
	if size := db.SizeBytes(); size <= 0 {
		t.Fatalf("SizeBytes=%d, want a positive size", size)
	}
}
