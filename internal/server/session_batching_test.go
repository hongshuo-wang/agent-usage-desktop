package server

import (
	"testing"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func TestSessionEventsValidateSharedSnapshotOnce(t *testing.T) {
	events := []storage.SessionEventRecord{
		{ID: 1, RawOffset: 0, RawLength: 10, RawLocator: &storage.RawEventLocator{
			Path: "/sessions/shared.jsonl", SourceStatus: "available", FileSize: 100,
			HeadHash: "hash", RawOffset: 0, RawLength: 10,
		}},
		{ID: 2, RawOffset: 20, RawLength: 10, RawLocator: &storage.RawEventLocator{
			Path: "/sessions/shared.jsonl", SourceStatus: "available", FileSize: 100,
			HeadHash: "hash", RawOffset: 20, RawLength: 10,
		}},
	}
	validations := 0
	responses := buildSessionEventResponses(events, func(*storage.RawEventLocator) (int64, bool) {
		validations++
		return 100, true
	})
	if validations != 1 {
		t.Fatalf("snapshot validations = %d, want 1", validations)
	}
	if len(responses) != 2 || !responses[0].HasRaw || !responses[1].HasRaw {
		t.Fatalf("raw availability = %+v, want both events available", responses)
	}
}
