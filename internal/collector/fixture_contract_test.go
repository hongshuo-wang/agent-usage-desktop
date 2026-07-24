package collector

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

const (
	claudeFixtureSessionID = "fixture-claude-session"
	codexFixtureSessionID  = "fixture-codex-session"
)

func TestSanitizedSessionFixturesExerciseCollectorsAndSQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fixture.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	claudeRoot := t.TempDir()
	claudePath := filepath.Join(claudeRoot, "fixture-project", "claude-session.jsonl")
	copySessionFixture(t, "claude-session.jsonl", claudePath)
	codexRoot := t.TempDir()
	copySessionFixture(t, "codex-session.jsonl", filepath.Join(codexRoot, "codex-session.jsonl"))

	if err := NewClaudeCollector(db, []string{claudeRoot}).Scan(); err != nil {
		t.Fatalf("scan Claude fixture: %v", err)
	}
	if err := NewCodexCollector(db, []string{codexRoot}).Scan(); err != nil {
		t.Fatalf("scan Codex fixture: %v", err)
	}

	wantKinds := []EventKind{
		EventUserMessage, EventAssistantMessage, EventToolCall,
		EventToolResult, EventError, EventUnknown,
	}
	for source, sessionID := range map[string]string{
		"claude": claudeFixtureSessionID,
		"codex":  codexFixtureSessionID,
	} {
		events, err := db.ListSessionEvents(source, sessionID, 100, 0)
		if err != nil {
			t.Fatalf("list %s fixture events: %v", source, err)
		}
		seen := make(map[string]bool)
		for _, event := range events {
			seen[event.EventType] = true
		}
		for _, kind := range wantKinds {
			if !seen[string(kind)] {
				t.Errorf("%s fixture event kinds = %v, missing %s", source, seen, kind)
			}
		}
	}

	from := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	to := from.Add(24*time.Hour - time.Nanosecond)
	sessions, err := db.SearchSessions(storage.SessionQuery{
		From: from, To: to, Search: "fixture", Limit: 10,
	})
	if err != nil {
		t.Fatalf("FTS fixture search: %v", err)
	}
	if len(sessions) != 2 || sessions[0].Source == sessions[1].Source {
		t.Fatalf("FTS fixture sessions = %+v, want Claude and Codex", sessions)
	}

	throughput, err := db.GetThroughput(from, to, "", "", 0)
	if err != nil {
		t.Fatalf("fixture throughput: %v", err)
	}
	if throughput.PeakRolling60s.RPM != 2 {
		t.Fatalf("fixture peak rolling RPM = %v, want 2", throughput.PeakRolling60s.RPM)
	}

	assertFixtureUsageRows(t, dbPath)
}

func copySessionFixture(t *testing.T, name, destination string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if !strings.Contains(strings.ToLower(string(data)), "fixture") {
		t.Fatalf("%s must contain FTS-searchable fixture text", name)
	}
	for _, forbidden := range []string{"/Users/", `C:\\Users\\`, "api_key", "secret"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("%s contains forbidden private marker %q", name, forbidden)
		}
	}
	for lineNumber, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if !json.Valid([]byte(line)) {
			t.Fatalf("%s line %d is not JSON", name, lineNumber+1)
		}
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertFixtureUsageRows(t *testing.T, dbPath string) {
	t.Helper()
	inspection, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer inspection.Close()
	rows, err := inspection.Query(`SELECT source, model, timestamp, input_tokens,
		cache_read_input_tokens, cache_creation_input_tokens, output_tokens,
		reasoning_output_tokens
		FROM usage_records ORDER BY timestamp`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type usageRow struct {
		source, model                 string
		timestamp                     time.Time
		input, cacheRead, cacheCreate int64
		output, reasoningOutput       int64
	}
	var got []usageRow
	for rows.Next() {
		var row usageRow
		if err := rows.Scan(
			&row.source, &row.model, &row.timestamp, &row.input,
			&row.cacheRead, &row.cacheCreate, &row.output, &row.reasoningOutput,
		); err != nil {
			t.Fatal(err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("fixture usage rows = %+v, want exactly two API calls", got)
	}
	assertUsageRow(t, got[0], usageRow{
		source: "claude", model: "claude-sonnet-4-20250514",
		timestamp: time.Date(2026, 7, 15, 10, 0, 59, 0, time.UTC),
		input:     100, cacheRead: 20, cacheCreate: 30, output: 40,
	})
	assertUsageRow(t, got[1], usageRow{
		source: "codex", model: "gpt-5-codex",
		timestamp: time.Date(2026, 7, 15, 10, 1, 1, 0, time.UTC),
		input:     100, cacheRead: 20, output: 40, reasoningOutput: 15,
	})
	if got[1].reasoningOutput > got[1].output {
		t.Fatalf("reasoning output %d is not a subset of output %d", got[1].reasoningOutput, got[1].output)
	}
	if total := got[1].input + got[1].cacheRead + got[1].cacheCreate + got[1].output; total != 160 {
		t.Fatalf("Codex non-overlapping total = %d, want 160", total)
	}
}

func assertUsageRow(t *testing.T, got, want struct {
	source, model                 string
	timestamp                     time.Time
	input, cacheRead, cacheCreate int64
	output, reasoningOutput       int64
}) {
	t.Helper()
	if got != want {
		t.Fatalf("usage row = %+v, want %+v", got, want)
	}
}
