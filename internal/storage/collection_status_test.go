package storage

import (
	"testing"
	"time"
)

func TestCollectionIndexStatusDistinguishesEmptyStatsOnlyAndAvailable(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got := callCollectionIndexStatus(t, tempDB(t))
		assertCollectionStatus(t, got, "empty", 0, 0)
	})

	t.Run("stats only", func(t *testing.T) {
		db := tempDB(t)
		if err := db.InsertUsage(&UsageRecord{
			Source: "claude", SessionID: "stats-only", Model: "model-a",
			Timestamp: time.Date(2025, 1, 2, 3, 0, 0, 0, time.UTC), InputTokens: 1,
		}); err != nil {
			t.Fatalf("InsertUsage: %v", err)
		}
		got := callCollectionIndexStatus(t, db)
		assertCollectionStatus(t, got, "stats_only", 0, 0)
	})

	t.Run("available", func(t *testing.T) {
		db := tempDB(t)
		first := testSessionSource("/sessions/claude.jsonl", "one")
		first.LastIndexedAt = time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
		second := testSessionSource("/sessions/codex.jsonl", "two")
		second.Source = "codex"
		second.LastIndexedAt = first.LastIndexedAt.Add(time.Hour)
		for _, source := range []*SessionSource{first, second} {
			if _, err := db.UpsertSessionSource(source); err != nil {
				t.Fatalf("UpsertSessionSource: %v", err)
			}
		}

		got := callCollectionIndexStatus(t, db)
		assertCollectionStatus(t, got, "available", 2, 2)
		if got.CompleteFiles != 2 || got.PartialFiles != 0 {
			t.Fatalf("coverage aggregate = %+v", got)
		}
		if got.LastIndexedAt == nil || !got.LastIndexedAt.Equal(second.LastIndexedAt) {
			t.Errorf("last_indexed_at = %v, want latest indexed source", got.LastIndexedAt)
		}
	})
}

func TestCollectionIndexStatusUsesExplicitPriorityAndAggregatesHealth(t *testing.T) {
	tests := []struct {
		name      string
		sources   []*SessionSource
		want      string
		malformed int
	}{
		{
			name: "missing wins",
			sources: []*SessionSource{
				statusTestSource("missing_source", "complete", 0, 1),
				statusTestSource("rebuild_required", "partial", 0, 2),
				statusTestSource("stale_parser", "complete", 0, 3),
			},
			want: "missing_source",
		},
		{
			name: "rebuild wins over stale and partial",
			sources: []*SessionSource{
				statusTestSource("rebuild_required", "complete", 0, 1),
				statusTestSource("stale_parser", "complete", 0, 2),
				statusTestSource("available", "partial", 0, 3),
			},
			want: "rebuild_required",
		},
		{
			name: "stale wins over partial",
			sources: []*SessionSource{
				statusTestSource("stale_parser", "complete", 0, 1),
				statusTestSource("available", "partial", 0, 2),
			},
			want: "stale_parser",
		},
		{
			name: "partial from malformed records",
			sources: []*SessionSource{
				statusTestSource("available", "complete", 3, 1),
			},
			want: "partial", malformed: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := tempDB(t)
			for _, source := range tt.sources {
				if _, err := db.UpsertSessionSource(source); err != nil {
					t.Fatalf("UpsertSessionSource: %v", err)
				}
			}
			got := callCollectionIndexStatus(t, db)
			assertCollectionStatus(t, got, tt.want, 1, len(tt.sources))
			if got.MalformedLines != tt.malformed {
				t.Errorf("malformed_lines = %v, want %d", got.MalformedLines, tt.malformed)
			}
		})
	}
}

func statusTestSource(sourceStatus, coverageStatus string, malformed, index int) *SessionSource {
	source := testSessionSource("/sessions/status-"+string(rune('a'+index))+".jsonl", "status-session")
	source.SourceStatus = sourceStatus
	source.CoverageStatus = coverageStatus
	source.MalformedLines = malformed
	return source
}

func callCollectionIndexStatus(t *testing.T, db *DB) *CollectionIndexStatus {
	t.Helper()
	status, err := db.GetCollectionIndexStatus()
	if err != nil {
		t.Fatalf("GetCollectionIndexStatus: %v", err)
	}
	return status
}

func assertCollectionStatus(t *testing.T, got *CollectionIndexStatus, status string, sourceCount, fileCount int) {
	t.Helper()
	if got.Status != status || got.SourceCount != sourceCount || got.FileCount != fileCount {
		t.Fatalf("collection status = %+v, want status=%s sources=%d files=%d", got, status, sourceCount, fileCount)
	}
}
