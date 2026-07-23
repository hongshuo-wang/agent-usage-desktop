package collector

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func tempDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func writeClaudeSessionFile(t *testing.T, root, project, name, content string) string {
	t.Helper()
	dir := filepath.Join(root, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func claudeVisibleLine(sessionID, timestamp, text string) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"sessionId":%q,"cwd":"/work","version":"1.0","message":{"role":"user","content":%q}}`, timestamp, sessionID, text)
}

func claudeUsageLine(sessionID, timestamp string, input, output int) string {
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"sessionId":%q,"message":{"role":"assistant","model":"claude-sonnet-4","usage":{"input_tokens":%d,"output_tokens":%d,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`, timestamp, sessionID, input, output)
}

func TestClaudeCollectorEventOffsetsAppendAndDedup(t *testing.T) {
	db := tempDB(t)
	root := t.TempDir()
	ts := "2026-01-02T03:04:05Z"
	first := claudeVisibleLine("sess-events", ts, "hello")
	path := writeClaudeSessionFile(t, root, "proj", "session.jsonl", first+"\n")
	collector := NewClaudeCollector(db, []string{root})

	if err := collector.Scan(); err != nil {
		t.Fatalf("Scan first: %v", err)
	}
	events, err := db.ListSessionEvents("claude", "sess-events", 100, 0)
	if err != nil {
		t.Fatalf("ListSessionEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events after first scan = %+v", events)
	}
	if events[0].RawOffset != 0 || events[0].RawLength != int64(len(first)) || events[0].RawIndex != 0 {
		t.Errorf("first locator = offset %d length %d index %d", events[0].RawOffset, events[0].RawLength, events[0].RawIndex)
	}

	second := fmt.Sprintf(`{"type":"assistant","timestamp":"2026-01-02T03:04:06Z","sessionId":"sess-events","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"world"}]}}`)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := f.WriteString(second + "\n"); err != nil {
		f.Close()
		t.Fatalf("append: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := collector.Scan(); err != nil {
		t.Fatalf("Scan append: %v", err)
	}
	if err := collector.Scan(); err != nil {
		t.Fatalf("Scan repeat: %v", err)
	}
	events, _ = db.ListSessionEvents("claude", "sess-events", 100, 0)
	if len(events) != 3 {
		t.Fatalf("events after append/repeat = %+v", events)
	}
	for i, event := range events[1:] {
		if event.RawOffset != int64(len(first)+1) || event.RawLength != int64(len(second)) || event.RawIndex != i {
			t.Errorf("appended locator %d = %+v", i, event)
		}
	}
	source, err := db.GetSessionSourceByPath(path)
	if err != nil || source == nil {
		t.Fatalf("source = %+v, %v", source, err)
	}
	if source.ParserVersion != "claude-events-v1" || source.HeadHash == "" || source.IndexedOffset != int64(len(first)+len(second)+2) || source.FileSize != source.IndexedOffset || source.CoverageStatus != "complete" {
		t.Errorf("source metadata = %+v", source)
	}
}

func TestClaudeCollectorPartialLineWaitsForNewline(t *testing.T) {
	db := tempDB(t)
	root := t.TempDir()
	ts := "2026-01-02T03:04:05Z"
	complete := claudeUsageLine("sess-partial", ts, 10, 5)
	partial := claudeVisibleLine("sess-partial", "2026-01-02T03:04:06Z", "later")
	path := writeClaudeSessionFile(t, root, "proj", "session.jsonl", complete+"\n"+partial)
	collector := NewClaudeCollector(db, []string{root})

	if err := collector.Scan(); err != nil {
		t.Fatalf("Scan partial: %v", err)
	}
	events, _ := db.ListSessionEvents("claude", "sess-partial", 100, 0)
	if len(events) != 0 {
		t.Fatalf("partial event indexed early: %+v", events)
	}
	size, offset, _, err := db.GetFileState(path)
	if err != nil {
		t.Fatalf("GetFileState: %v", err)
	}
	if size != int64(len(complete)+1+len(partial)) || offset != int64(len(complete)+1) {
		t.Fatalf("file state = size %d offset %d", size, offset)
	}
	source, err := db.GetSessionSourceByPath(path)
	if err != nil || source == nil {
		t.Fatalf("GetSessionSourceByPath = %+v, %v", source, err)
	}
	if source.CoverageStatus != "partial" || source.IndexedOffset >= source.FileSize {
		t.Fatalf("partial source metadata = %+v", source)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := f.WriteString("\n"); err != nil {
		f.Close()
		t.Fatalf("append newline: %v", err)
	}
	f.Close()
	if err := collector.Scan(); err != nil {
		t.Fatalf("Scan completed: %v", err)
	}
	if err := collector.Scan(); err != nil {
		t.Fatalf("Scan repeat: %v", err)
	}
	events, _ = db.ListSessionEvents("claude", "sess-partial", 100, 0)
	if len(events) != 1 || events[0].Content != "later" {
		t.Fatalf("completed events = %+v", events)
	}
	source, err = db.GetSessionSourceByPath(path)
	if err != nil || source == nil {
		t.Fatalf("GetSessionSourceByPath completed = %+v, %v", source, err)
	}
	if source.CoverageStatus != "complete" || source.IndexedOffset != source.FileSize {
		t.Fatalf("completed source metadata = %+v", source)
	}
	from, to := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, _ := db.GetDashboardStats(from, to, "claude")
	if stats.TotalTokens != 15 {
		t.Fatalf("usage duplicated or lost: %+v", stats)
	}
}

func TestClaudeCollectorPartialOnlyAppendMarksCoveragePartial(t *testing.T) {
	db := tempDB(t)
	root := t.TempDir()
	complete := claudeVisibleLine("sess-partial-append", "2026-01-02T03:04:05Z", strings.Repeat("x", 5000))
	path := writeClaudeSessionFile(t, root, "proj", "session.jsonl", complete+"\n")
	collector := NewClaudeCollector(db, []string{root})
	if err := collector.Scan(); err != nil {
		t.Fatalf("Scan complete: %v", err)
	}

	partial := claudeVisibleLine("sess-partial-append", "2026-01-02T03:04:06Z", "unfinished")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	if _, err := f.WriteString(partial); err != nil {
		f.Close()
		t.Fatalf("append partial: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := collector.Scan(); err != nil {
		t.Fatalf("Scan partial append: %v", err)
	}

	source, err := db.GetSessionSourceByPath(path)
	if err != nil || source == nil {
		t.Fatalf("GetSessionSourceByPath = %+v, %v", source, err)
	}
	if source.CoverageStatus != "partial" || source.IndexedOffset >= source.FileSize {
		t.Fatalf("partial-only append source metadata = %+v", source)
	}
}

func TestClaudeCollectorIndexesElevenMiBVisibleLine(t *testing.T) {
	db := tempDB(t)
	root := t.TempDir()
	text := strings.Repeat("x", 11*1024*1024)
	line := claudeVisibleLine("sess-large", "2026-01-02T03:04:05Z", text)
	writeClaudeSessionFile(t, root, "proj", "large.jsonl", line+"\n")

	if err := NewClaudeCollector(db, []string{root}).Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	events, _ := db.ListSessionEvents("claude", "sess-large", 10, 0)
	if len(events) != 1 || len(events[0].Content) != len(text) {
		t.Fatalf("large event count/content length = %d/%d", len(events), func() int {
			if len(events) == 0 {
				return 0
			}
			return len(events[0].Content)
		}())
	}
}

func TestClaudeCollectorMalformedLineContinuesWithPartialCoverage(t *testing.T) {
	db := tempDB(t)
	root := t.TempDir()
	valid := claudeVisibleLine("sess-malformed", "2026-01-02T03:04:06Z", "after")
	path := writeClaudeSessionFile(t, root, "proj", "bad.jsonl", `{"type":`+"\n"+valid+"\n")

	if err := NewClaudeCollector(db, []string{root}).Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	events, _ := db.ListSessionEvents("claude", "sess-malformed", 10, 0)
	if len(events) != 1 || events[0].Content != "after" {
		t.Fatalf("events = %+v", events)
	}
	source, _ := db.GetSessionSourceByPath(path)
	if source == nil || source.MalformedLines != 1 || source.CoverageStatus != "partial" || source.LastError == "" {
		t.Fatalf("source = %+v", source)
	}
}

func TestClaudeCollectorRebuildTriggersReplaceOnlyEventIndex(t *testing.T) {
	triggers := []struct {
		name   string
		mutate func(t *testing.T, db *storage.DB, path, original string)
	}{
		{"truncation", func(t *testing.T, _ *storage.DB, path, _ string) {
			write := claudeVisibleLine("sess-rebuild", "2026-01-02T03:04:05Z", "new") + "\n"
			if err := os.WriteFile(path, []byte(write), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"head change", func(t *testing.T, _ *storage.DB, path, original string) {
			replaced := strings.Replace(original, "old", "new", 1)
			if len(replaced) != len(original) {
				t.Fatal("replacement changed size")
			}
			if err := os.WriteFile(path, []byte(replaced), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"head change with growth", func(t *testing.T, _ *storage.DB, path, original string) {
			replaced := strings.Replace(original, "old", "new", 1)
			replaced += `{"type":"system","timestamp":"2026-01-02T03:04:07Z","sessionId":"sess-rebuild","subtype":"checkpoint"}` + "\n"
			if err := os.WriteFile(path, []byte(replaced), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"parser version", func(t *testing.T, db *storage.DB, path, _ string) {
			source, err := db.GetSessionSourceByPath(path)
			if err != nil || source == nil {
				t.Fatalf("source: %+v %v", source, err)
			}
			source.ParserVersion = "claude-events-v0"
			if _, err := db.UpsertSessionSource(source); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, trigger := range triggers {
		t.Run(trigger.name, func(t *testing.T) {
			db := tempDB(t)
			root := t.TempDir()
			ts := "2026-01-02T03:04:05Z"
			visible := claudeVisibleLine("sess-rebuild", ts, "old")
			usage := claudeUsageLine("sess-rebuild", "2026-01-02T03:04:06Z", 10, 5)
			original := visible + "\n" + usage + "\n"
			if trigger.name == "truncation" {
				original += `{"type":"system","timestamp":"2026-01-02T03:04:06Z","sessionId":"sess-rebuild","subtype":"checkpoint","padding":"` + strings.Repeat("padding", 40) + `"}` + "\n"
			}
			path := writeClaudeSessionFile(t, root, "proj", "session.jsonl", original)
			collector := NewClaudeCollector(db, []string{root})
			if err := collector.Scan(); err != nil {
				t.Fatalf("first Scan: %v", err)
			}
			trigger.mutate(t, db, path, original)
			if err := collector.Scan(); err != nil {
				t.Fatalf("rebuild Scan: %v", err)
			}

			events, _ := db.ListSessionEvents("claude", "sess-rebuild", 100, 0)
			var oldCount int
			for _, event := range events {
				if event.Content == "old" {
					oldCount++
				}
			}
			if trigger.name != "parser version" && oldCount != 0 {
				t.Fatalf("stale event retained: %+v", events)
			}
			if trigger.name == "parser version" && (len(events) != 1 || oldCount != 1) {
				t.Fatalf("parser rebuild duplicated unchanged content: %+v", events)
			}
			from, to := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
			stats, _ := db.GetDashboardStats(from, to, "claude")
			if stats.TotalTokens != 15 {
				t.Fatalf("historical usage changed: %+v", stats)
			}
			sessions, _ := db.GetSessions(from, to, "claude")
			if len(sessions) != 1 || sessions[0].Prompts != 1 {
				t.Fatalf("session/prompt count duplicated: %+v", sessions)
			}
			source, _ := db.GetSessionSourceByPath(path)
			if source == nil || source.ParserVersion != "claude-events-v1" || source.IndexedOffset == 0 {
				t.Fatalf("rebuilt source = %+v", source)
			}
		})
	}
}

func TestClaudeCollectorSourceMissingAndRestored(t *testing.T) {
	db := tempDB(t)
	root := t.TempDir()
	visible := claudeVisibleLine("sess-missing", "2026-01-02T03:04:05Z", "kept")
	usage := claudeUsageLine("sess-missing", "2026-01-02T03:04:06Z", 10, 5)
	content := visible + "\n" + usage + "\n"
	path := writeClaudeSessionFile(t, root, "proj", "session.jsonl", content)
	other := writeClaudeSessionFile(t, root, "proj", "other.jsonl", claudeVisibleLine("sess-other", "2026-01-02T03:04:07Z", "other")+"\n")
	collector := NewClaudeCollector(db, []string{root})
	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}

	events, _ := db.ListSessionEvents("claude", "sess-missing", 10, 0)
	if len(events) != 0 {
		t.Fatalf("missing events retained: %+v", events)
	}
	otherEvents, _ := db.ListSessionEvents("claude", "sess-other", 10, 0)
	if len(otherEvents) != 1 {
		t.Fatalf("other source affected: %+v", otherEvents)
	}
	source, _ := db.GetSessionSourceByPath(path)
	if source == nil || source.SourceStatus != "missing_source" {
		t.Fatalf("missing source = %+v", source)
	}
	from, to := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, _ := db.GetDashboardStats(from, to, "claude")
	sessions, _ := db.GetSessions(from, to, "claude")
	if stats.TotalTokens != 15 || len(sessions) != 1 || sessions[0].SessionID != "sess-missing" {
		t.Fatalf("historical data removed: stats=%+v sessions=%+v", stats, sessions)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := collector.Scan(); err != nil {
		t.Fatal(err)
	}
	events, _ = db.ListSessionEvents("claude", "sess-missing", 10, 0)
	if len(events) != 1 || events[0].Content != "kept" {
		t.Fatalf("restored events = %+v", events)
	}
	source, _ = db.GetSessionSourceByPath(path)
	if source.SourceStatus != "available" {
		t.Fatalf("restored source = %+v", source)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal(err)
	}
}

func TestClaudeCollectorEventOnlySessionCreatesMetadataAndSource(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "events.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	root := t.TempDir()
	path := writeClaudeSessionFile(t, root, "proj", "event-only.jsonl", claudeVisibleLine("sess-event-only", "2026-01-02T03:04:05Z", "visible")+"\n")
	if err := NewClaudeCollector(db, []string{root}).Scan(); err != nil {
		t.Fatal(err)
	}
	inspection, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer inspection.Close()
	var project string
	if err := inspection.QueryRow(`SELECT project FROM sessions WHERE source='claude' AND session_id='sess-event-only'`).Scan(&project); err != nil || project != "proj" {
		t.Fatalf("event-only session project = %q, %v", project, err)
	}
	source, _ := db.GetSessionSourceByPath(path)
	if source == nil || source.SessionID != "sess-event-only" {
		t.Fatalf("source = %+v", source)
	}
}

func TestClaudeCollector_Scan(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	projDir := filepath.Join(dir, "myproject")
	os.MkdirAll(projDir, 0o755)

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	jsonl := `{"type":"system","timestamp":"` + ts + `","sessionId":"sess-abc","version":"1.2.3","cwd":"/home/user","gitBranch":"main"}
{"type":"user","timestamp":"` + ts + `"}
{"type":"assistant","timestamp":"` + ts + `","message":{"role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":500,"output_tokens":200,"cache_creation_input_tokens":100,"cache_read_input_tokens":50}}}
`
	if err := os.WriteFile(filepath.Join(projDir, "session.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cc := NewClaudeCollector(db, []string{dir})
	if err := cc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Use wide UTC range to avoid timezone issues with SQLite string comparison
	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalTokens != 850 {
		t.Errorf("expected 850 tokens, got %d", stats.TotalTokens)
	}
}

func TestClaudeCollector_IncrementalScan(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	projDir := filepath.Join(dir, "proj")
	os.MkdirAll(projDir, 0o755)

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	line1 := `{"type":"assistant","timestamp":"` + ts + `","message":{"role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}` + "\n"

	fpath := filepath.Join(projDir, "sess.jsonl")
	if err := os.WriteFile(fpath, []byte(line1), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cc := NewClaudeCollector(db, []string{dir})
	if err := cc.Scan(); err != nil {
		t.Fatalf("Scan 1: %v", err)
	}

	ts2 := time.Now().Add(time.Second).UTC().Format(time.RFC3339Nano)
	line2 := `{"type":"assistant","timestamp":"` + ts2 + `","message":{"role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}` + "\n"

	f, err := os.OpenFile(fpath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	f.WriteString(line2)
	f.Close()

	if err := cc.Scan(); err != nil {
		t.Fatalf("Scan 2: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	stats, err := db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats: %v", err)
	}
	if stats.TotalTokens != 450 {
		t.Errorf("expected 450 tokens after incremental scan, got %d", stats.TotalTokens)
	}
}

func TestClaudeCollector_MissingPath(t *testing.T) {
	db := tempDB(t)
	cc := NewClaudeCollector(db, []string{"/nonexistent/path"})
	if err := cc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}
}

func TestIsRealUserPrompt(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{"string content", `{"content":"hello"}`, true},
		{"text block", `{"content":[{"type":"text","text":"hello"}]}`, true},
		{"tool result", `{"content":[{"type":"tool_result","tool_use_id":"abc","content":"ok"}]}`, false},
		{"mixed with tool_use_id", `{"content":[{"tool_use_id":"abc","type":"tool_result"}]}`, false},
		{"empty message", `{}`, false},
		{"nil message", ``, false},
		{"empty content array", `{"content":[]}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRealUserPrompt([]byte(tt.raw))
			if got != tt.want {
				t.Errorf("isRealUserPrompt(%s) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestClaudeCollector_PromptCounting(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	projDir := filepath.Join(dir, "myproject")
	os.MkdirAll(projDir, 0o755)

	ts := time.Now().UTC().Format(time.RFC3339Nano)
	// 1 real user prompt + 2 tool results (should count as 1 prompt, not 3)
	jsonl := `{"type":"system","timestamp":"` + ts + `","sessionId":"sess-prompt","version":"1.0","cwd":"/tmp"}
{"type":"user","timestamp":"` + ts + `","message":{"role":"user","content":"please help me"}}
{"type":"assistant","timestamp":"` + ts + `","message":{"role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
{"type":"user","timestamp":"` + ts + `","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":"file contents"}]}}
{"type":"assistant","timestamp":"` + ts + `","message":{"role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
{"type":"user","timestamp":"` + ts + `","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu2","content":"done"}]}}
{"type":"assistant","timestamp":"` + ts + `","message":{"role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":150,"output_tokens":75,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}
`
	if err := os.WriteFile(filepath.Join(projDir, "session.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cc := NewClaudeCollector(db, []string{dir})
	if err := cc.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions, err := db.GetSessions(from, to, "")
	if err != nil {
		t.Fatalf("GetSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Prompts != 1 {
		t.Errorf("expected 1 prompt (real user message only), got %d", sessions[0].Prompts)
	}
}

func TestCodexCollector_EmptyFile(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "2025", "01", "01")
	os.MkdirAll(sessionDir, 0o755)

	if err := os.WriteFile(filepath.Join(sessionDir, "empty.jsonl"), []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cx := NewCodexCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}
}
