package storage

import (
	"strconv"
	"testing"
	"time"
)

func TestCollectionIndexStatusReportsStalledScanLoop(t *testing.T) {
	db := tempDB(t)
	if _, err := db.UpsertSessionSource(&SessionSource{
		Source: "claude", SessionID: "s1", Path: "/p/s1.jsonl",
		CoverageStatus: "complete", SourceStatus: "available",
		LastIndexedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpsertSessionSource: %v", err)
	}

	// No heartbeat yet: unknown, not stale. Pre-heartbeat databases must stay quiet.
	status, err := db.GetCollectionIndexStatus()
	if err != nil {
		t.Fatalf("GetCollectionIndexStatus: %v", err)
	}
	if status.Stale || status.LastScanAt != nil {
		t.Errorf("absent heartbeat = stale=%v last_scan_at=%v, want unknown", status.Stale, status.LastScanAt)
	}
	if status.Status != "available" {
		t.Errorf("status = %q, want available", status.Status)
	}

	// Unparseable heartbeat is also unknown rather than a stalled loop.
	if err := db.SetMeta("last_scan_at", "not-a-timestamp"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	status, err = db.GetCollectionIndexStatus()
	if err != nil {
		t.Fatalf("GetCollectionIndexStatus: %v", err)
	}
	if status.Stale {
		t.Error("unparseable heartbeat reported as stale")
	}

	// A fresh heartbeat stays below the two hour floor.
	heartbeat(t, db, 30*time.Minute, 60)
	status, err = db.GetCollectionIndexStatus()
	if err != nil {
		t.Fatalf("GetCollectionIndexStatus: %v", err)
	}
	if status.Stale || status.Status != "available" || status.LastScanAt == nil {
		t.Errorf("fresh heartbeat = %+v, want available and not stale", status)
	}

	// Past the floor the loop is presumed dead.
	heartbeat(t, db, 5*time.Hour, 60)
	status, err = db.GetCollectionIndexStatus()
	if err != nil {
		t.Fatalf("GetCollectionIndexStatus: %v", err)
	}
	if !status.Stale || status.Status != "stale" {
		t.Errorf("stalled scan = stale=%v status=%q, want stale", status.Stale, status.Status)
	}

	// A deliberately long cadence widens the window past the floor: 6h > 5h.
	heartbeat(t, db, 5*time.Hour, 7200)
	status, err = db.GetCollectionIndexStatus()
	if err != nil {
		t.Fatalf("GetCollectionIndexStatus: %v", err)
	}
	if status.Stale {
		t.Errorf("slow cadence reported stale at %+v, want tolerance scaled to 6h", status)
	}

	// A specific fault outranks the generic stall.
	if _, err := db.UpsertSessionSource(&SessionSource{
		Source: "codex", SessionID: "s2", Path: "/p/missing.jsonl",
		CoverageStatus: "complete", SourceStatus: "missing_source",
		LastIndexedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("UpsertSessionSource: %v", err)
	}
	heartbeat(t, db, 9*time.Hour, 60)
	status, err = db.GetCollectionIndexStatus()
	if err != nil {
		t.Fatalf("GetCollectionIndexStatus: %v", err)
	}
	if !status.Stale || status.Status != "missing_source" {
		t.Errorf("status = %q stale=%v, want missing_source to keep precedence", status.Status, status.Stale)
	}
}

func heartbeat(t *testing.T, db *DB, age time.Duration, intervalSeconds int) {
	t.Helper()
	if err := db.SetMeta("last_scan_at", time.Now().UTC().Add(-age).Format(time.RFC3339)); err != nil {
		t.Fatalf("SetMeta(last_scan_at): %v", err)
	}
	if err := db.SetMeta("scan_interval_seconds", strconv.Itoa(intervalSeconds)); err != nil {
		t.Fatalf("SetMeta(scan_interval_seconds): %v", err)
	}
}
