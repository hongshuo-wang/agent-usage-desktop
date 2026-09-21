package collector

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPiCollector_IncrementalScanPreservesSessionContext(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "--Users-me-project--")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	ts1 := time.Now().UTC().Format(time.RFC3339Nano)
	fpath := filepath.Join(sessionDir, "pi-sess-1.jsonl")
	initial := `{"type":"session","version":3,"id":"pi-sess-1","cwd":"/Users/me/project","timestamp":"` + ts1 + `"}
{"type":"model_change","timestamp":"` + ts1 + `","provider":"qiyuan-hub","modelId":"deepseek-v4.1"}
{"type":"message","timestamp":"` + ts1 + `","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}
{"type":"message","timestamp":"` + ts1 + `","message":{"role":"assistant","model":"deepseek-v4.1","usage":{"input":200,"output":100,"cacheRead":50,"cacheWrite":30,"reasoning":20}}}
`
	if err := os.WriteFile(fpath, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cx := NewPiCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan 1: %v", err)
	}

	// User messages are prompts; toolResult messages are a separate role.
	ts2 := time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano)
	incremental := `{"type":"message","timestamp":"` + ts2 + `","message":{"role":"toolResult","content":[{"type":"text","text":"out"}]}}
{"type":"message","timestamp":"` + ts2 + `","message":{"role":"assistant","model":"deepseek-v4.1","usage":{"input":300,"output":150,"cacheRead":60,"cacheWrite":40,"reasoning":10}}}
`
	f, err := os.OpenFile(fpath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := f.WriteString(incremental); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan 2: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)

	sessions, err := db.GetSessions(from, to, "pi")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionID != "pi-sess-1" {
		t.Fatalf("expected preserved session_id pi-sess-1, got %s", sessions[0].SessionID)
	}

	stats, err := db.GetDashboardStats(from, to, "pi")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalCalls != 2 {
		t.Fatalf("expected 2 calls after incremental scan, got %d", stats.TotalCalls)
	}
	if stats.TotalPrompts != 1 {
		t.Fatalf("expected 1 prompt (toolResult excluded), got %d", stats.TotalPrompts)
	}
	// Non-overlapping semantics: input=500, cache_read=110, cache_write=70, output=250.
	if stats.InputTokens != 500 || stats.CacheRead != 110 || stats.CacheCreate != 70 || stats.OutputTokens != 250 {
		t.Fatalf("unexpected token breakdown: %+v", stats)
	}
}
