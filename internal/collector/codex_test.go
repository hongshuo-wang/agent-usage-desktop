package collector

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func TestCodexCollectorEventIndexContextAppendAndPartialTail(t *testing.T) {
	db := tempDB(t)
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	first := `{"timestamp":"2026-01-02T03:04:05Z","type":"session_meta","payload":{"id":"codex-events","cwd":"/synthetic/workspace","cli_version":"1.0.0"}}`
	turn := `{"timestamp":"2026-01-02T03:04:06Z","type":"turn_context","payload":{"model":"synthetic-model"}}`
	message := `{"timestamp":"2026-01-02T03:04:07Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"first"},{"type":"output_text","text":"second"}]}}`
	if err := os.WriteFile(path, []byte(first+"\n"+turn+"\n"+message+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	collector := NewCodexCollector(db, []string{root})
	if err := collector.Scan(); err != nil {
		t.Fatalf("Scan first: %v", err)
	}
	events, err := db.ListSessionEvents("codex", "codex-events", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("first events = %+v", events)
	}
	if events[1].RawOffset != int64(len(first)+1+len(turn)+1) || events[1].RawLength != int64(len(message)) || events[1].RawIndex != 0 || events[2].RawIndex != 1 {
		t.Fatalf("message locators = %+v", events[1:])
	}
	source, err := db.GetSessionSourceByPath(path)
	if err != nil || source == nil || source.SessionID != "codex-events" || source.ParserVersion != "codex-events-v1" || source.CoverageStatus != "complete" {
		t.Fatalf("source = %+v, %v", source, err)
	}
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions, err := db.GetSessions(from, to, "codex")
	if err != nil || len(sessions) != 1 || sessions[0].SessionID != "codex-events" || sessions[0].CWD != "/synthetic/workspace" {
		t.Fatalf("event-only session metadata = %+v, %v", sessions, err)
	}
	initialSourceID := source.ID

	partial := `{"timestamp":"2026-01-02T03:04:08Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"later"}]}}`
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(partial); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := collector.Scan(); err != nil {
		t.Fatalf("Scan partial: %v", err)
	}
	source, _ = db.GetSessionSourceByPath(path)
	if source.CoverageStatus != "partial" || source.IndexedOffset >= source.FileSize || source.ID != initialSourceID {
		t.Fatalf("partial source = %+v", source)
	}
	if err := os.WriteFile(path, []byte(first+"\n"+turn+"\n"+message+"\n"+partial+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := collector.Scan(); err != nil {
		t.Fatalf("Scan complete append: %v", err)
	}
	events, _ = db.ListSessionEvents("codex", "codex-events", 10, 0)
	if len(events) != 4 || events[3].Content != "later" {
		t.Fatalf("append events = %+v", events)
	}
	source, _ = db.GetSessionSourceByPath(path)
	if source.ID != initialSourceID || source.CoverageStatus != "complete" {
		t.Fatalf("completed source = %+v", source)
	}
}

func TestCodexCollectorMalformedUsageContinues(t *testing.T) {
	db := tempDB(t)
	root := t.TempDir()
	content := strings.Join([]string{
		`{"timestamp":"2026-01-02T03:04:05Z","type":"session_meta","payload":{"id":"codex-malformed"}}`,
		`{"type":`,
		`{"timestamp":"2026-01-02T03:04:06Z","type":"turn_context","payload":{"model":"synthetic-model"}}`,
		`{"timestamp":"2026-01-02T03:04:07Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":2,"cached_input_tokens":3,"output_tokens":1}}}}`,
		`{"timestamp":"2026-01-02T03:04:08Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"after"}]}}`,
	}, "\n") + "\n"
	path := filepath.Join(root, "bad.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := NewCodexCollector(db, []string{root}).Scan(); err != nil {
		t.Fatal(err)
	}
	events, _ := db.ListSessionEvents("codex", "codex-malformed", 10, 0)
	if len(events) != 2 || events[1].Content != "after" {
		t.Fatalf("events = %+v", events)
	}
	source, _ := db.GetSessionSourceByPath(path)
	if source == nil || source.MalformedLines != 2 || source.LastError == "" || source.CoverageStatus != "partial" {
		t.Fatalf("source = %+v", source)
	}
}

func TestCodexCollectorRebuildMissingAndLargeRecord(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "events.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	root := t.TempDir()
	path := filepath.Join(root, "session.jsonl")
	old := `{"timestamp":"2026-01-02T03:04:05Z","type":"session_meta","payload":{"id":"codex-rebuild"}}` + "\n" +
		`{"timestamp":"2026-01-02T03:04:06Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"old"}]}}` + "\n" +
		`{"timestamp":"2026-01-02T03:04:07Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":3,"cached_input_tokens":1,"output_tokens":2}}}}` + "\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	collector := NewCodexCollector(db, []string{root})
	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}
	source, _ := db.GetSessionSourceByPath(path)
	source.ParserVersion = "codex-events-v0"
	if _, err := db.UpsertSessionSource(source); err != nil {
		t.Fatal(err)
	}
	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}
	events, _ := db.ListSessionEvents("codex", "codex-rebuild", 10, 0)
	if len(events) != 1 || events[0].Content != "old" {
		t.Fatalf("parser rebuild events = %+v", events)
	}
	source, _ = db.GetSessionSourceByPath(path)
	if source == nil || source.ParserVersion != "codex-events-v1" {
		t.Fatalf("parser rebuild source = %+v", source)
	}
	replacement := strings.Replace(old, "old", "new", 1)
	if len(replacement) != len(old) {
		t.Fatal("replacement changed length")
	}
	if err := os.WriteFile(path, []byte(replacement), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}
	events, _ = db.ListSessionEvents("codex", "codex-rebuild", 10, 0)
	if len(events) != 1 || events[0].Content != "new" {
		t.Fatalf("head rebuild events = %+v", events)
	}
	truncated := strings.Replace(replacement, "new", "x", 1)
	if len(truncated) >= len(replacement) {
		t.Fatalf("truncated size = %d, want less than %d", len(truncated), len(replacement))
	}
	if err := os.WriteFile(path, []byte(truncated), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}
	events, _ = db.ListSessionEvents("codex", "codex-rebuild", 10, 0)
	if len(events) != 1 || events[0].Content != "x" {
		t.Fatalf("truncation rebuild events = %+v", events)
	}
	inspection, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer inspection.Close()
	var ftsBefore int
	if err := inspection.QueryRow(`SELECT COUNT(*) FROM session_events_fts WHERE session_events_fts MATCH 'x'`).Scan(&ftsBefore); err != nil || ftsBefore != 1 {
		t.Fatalf("FTS before missing = %d, %v", ftsBefore, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}
	missing, _ := db.GetSessionSourceByPath(path)
	if missing == nil || missing.SourceStatus != "missing_source" {
		t.Fatalf("missing source = %+v", missing)
	}
	events, _ = db.ListSessionEvents("codex", "codex-rebuild", 10, 0)
	if len(events) != 0 {
		t.Fatalf("missing source retained events = %+v", events)
	}
	var ftsAfter int
	if err := inspection.QueryRow(`SELECT COUNT(*) FROM session_events_fts WHERE session_events_fts MATCH 'x'`).Scan(&ftsAfter); err != nil || ftsAfter != 0 {
		t.Fatalf("FTS after missing = %d, %v", ftsAfter, err)
	}
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, err := db.GetDashboardStats(from, to, "codex")
	if err != nil || stats.TotalTokens != 5 {
		t.Fatalf("usage after missing = %+v, %v", stats, err)
	}
	sessions, err := db.GetSessions(from, to, "codex")
	if err != nil || len(sessions) != 1 || sessions[0].SessionID != "codex-rebuild" {
		t.Fatalf("session after missing = %+v, %v", sessions, err)
	}
	if err := os.WriteFile(path, []byte(replacement), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}
	events, _ = db.ListSessionEvents("codex", "codex-rebuild", 10, 0)
	if len(events) != 1 || events[0].Content != "new" {
		t.Fatalf("restored events = %+v", events)
	}

	large := strings.Repeat("x", 11*1024*1024)
	largePath := filepath.Join(root, "large.jsonl")
	largeLine := `{"timestamp":"2026-01-02T03:04:09Z","type":"session_meta","payload":{"id":"codex-large"}}` + "\n" +
		`{"timestamp":"2026-01-02T03:04:10Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"` + large + `"}]}}` + "\n"
	if err := os.WriteFile(largePath, []byte(largeLine), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}
	events, _ = db.ListSessionEvents("codex", "codex-large", 10, 0)
	if len(events) != 1 || len(events[0].Content) != len(large) {
		t.Fatalf("large events = %d, content length = %d", len(events), func() int {
			if len(events) == 0 {
				return 0
			}
			return len(events[0].Content)
		}())
	}
}

func TestCodexCollectorSameSessionIDRemainsSeparateFromClaude(t *testing.T) {
	db := tempDB(t)
	codexRoot := t.TempDir()
	claudeRoot := t.TempDir()
	const sessionID = "shared-synthetic-session"
	codexLine := `{"timestamp":"2026-01-02T03:04:05Z","type":"session_meta","payload":{"id":"` + sessionID + `"}}` + "\n" +
		`{"timestamp":"2026-01-02T03:04:06Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"codex-visible"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(codexRoot, "session.jsonl"), []byte(codexLine), 0o644); err != nil {
		t.Fatal(err)
	}
	claudePath := writeClaudeSessionFile(t, claudeRoot, "project", "session.jsonl", claudeVisibleLine(sessionID, "2026-01-02T03:04:06Z", "claude-visible")+"\n")
	if err := NewCodexCollector(db, []string{codexRoot}).Scan(); err != nil {
		t.Fatal(err)
	}
	if err := NewClaudeCollector(db, []string{claudeRoot}).Scan(); err != nil {
		t.Fatal(err)
	}
	codexEvents, _ := db.ListSessionEvents("codex", sessionID, 10, 0)
	claudeEvents, _ := db.ListSessionEvents("claude", sessionID, 10, 0)
	if len(codexEvents) != 1 || codexEvents[0].Content != "codex-visible" || len(claudeEvents) != 1 || claudeEvents[0].Content != "claude-visible" {
		t.Fatalf("cross-source events = codex:%+v claude:%+v", codexEvents, claudeEvents)
	}
	if _, err := os.Stat(claudePath); err != nil {
		t.Fatal(err)
	}
}

func TestCodexCollector_Scan(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "2025", "01", "01")
	os.MkdirAll(sessionDir, 0o755)

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	jsonl := `{"timestamp":"` + ts + `","type":"session_meta","payload":{"id":"codex-sess-1","cwd":"/home/user","cli_version":"0.1.0"}}
{"timestamp":"` + ts + `","type":"turn_context","payload":{"model":"o4-mini"}}
{"timestamp":"` + ts + `","type":"response_item","payload":{"role":"user"}}
{"timestamp":"` + ts + `","type":"event_msg","payload":{"type":"token_count","turn_id":"t1","info":{"last_token_usage":{"input_tokens":300,"cached_input_tokens":50,"output_tokens":150,"reasoning_output_tokens":20}}}}
`
	if err := os.WriteFile(filepath.Join(sessionDir, "session.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cx := NewCodexCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalTokens != 450 {
		t.Errorf("expected 450 tokens, got %d", stats.TotalTokens)
	}
}

func TestCodexCollector_IncrementalScanPreservesSessionContext(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "2025", "01", "01")
	os.MkdirAll(sessionDir, 0o755)

	ts1 := time.Now().UTC().Format(time.RFC3339Nano)
	fpath := filepath.Join(sessionDir, "session.jsonl")
	initial := `{"timestamp":"` + ts1 + `","type":"session_meta","payload":{"id":"codex-sess-1","cwd":"/home/user","cli_version":"0.1.0"}}
{"timestamp":"` + ts1 + `","type":"turn_context","payload":{"model":"gpt-5.4"}}
{"timestamp":"` + ts1 + `","type":"event_msg","payload":{"type":"token_count","turn_id":"t1","info":{"last_token_usage":{"input_tokens":300,"cached_input_tokens":50,"output_tokens":150,"reasoning_output_tokens":20}}}}
`
	if err := os.WriteFile(fpath, []byte(initial), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cx := NewCodexCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan 1: %v", err)
	}

	ts2 := time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano)
	incremental := `{"timestamp":"` + ts2 + `","type":"event_msg","payload":{"type":"token_count","turn_id":"t2","info":{"last_token_usage":{"input_tokens":400,"cached_input_tokens":100,"output_tokens":200,"reasoning_output_tokens":30}}}}
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

	sessions, err := db.GetSessions(from, to, "codex")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session after incremental scan, got %d", len(sessions))
	}
	if sessions[0].SessionID != "codex-sess-1" {
		t.Fatalf("expected preserved session_id codex-sess-1, got %s", sessions[0].SessionID)
	}

	details, err := db.GetSessionDetail("codex", "codex-sess-1")
	if err != nil {
		t.Fatalf("GetSessionDetail: %v", err)
	}
	if len(details) != 1 {
		t.Fatalf("expected 1 model detail row, got %d", len(details))
	}
	if details[0].Model != "gpt-5.4" {
		t.Fatalf("expected preserved model gpt-5.4, got %q", details[0].Model)
	}
	if details[0].Calls != 2 {
		t.Fatalf("expected 2 calls after incremental scan, got %d", details[0].Calls)
	}
}
