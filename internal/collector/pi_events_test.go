package collector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func parsePiRecord(t *testing.T, line string) ([]NormalizedEvent, *EventContext) {
	t.Helper()
	context := &EventContext{Source: "pi"}
	events, err := piEventAdapter{}.Parse([]byte(line), context)
	if err != nil {
		t.Fatalf("Parse(%s): %v", line, err)
	}
	return events, context
}

func TestPiEventAdapterMapsMessagesAndToolCalls(t *testing.T) {
	cases := []struct {
		name            string
		line            string
		want            []NormalizedEvent
		wantSourceEvent string
	}{
		{
			name: "user prompt",
			line: `{"type":"message","timestamp":"2026-09-18T03:23:22.689Z","message":{"role":"user","content":[{"type":"text","text":"fix the bug"}]}}`,
			want: []NormalizedEvent{{
				Kind: EventUserMessage, Role: "user", Content: "fix the bug",
			}},
		},
		{
			name: "assistant text",
			line: `{"type":"message","timestamp":"2026-09-18T03:23:22.689Z","message":{"role":"assistant","model":"deepseek-v4.1","content":[{"type":"text","text":"on it"}]}}`,
			want: []NormalizedEvent{{
				Kind: EventAssistantMessage, Role: "assistant", Content: "on it",
			}},
		},
		{
			name: "assistant thinking",
			line: `{"type":"message","timestamp":"2026-09-18T03:23:22.689Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"let me check","thinkingSignature":"sig"}]}}`,
			want: []NormalizedEvent{{
				Kind: EventReasoning, Role: "assistant", Content: "let me check",
			}},
		},
		{
			name: "assistant tool call",
			line: `{"type":"message","timestamp":"2026-09-18T03:23:22.689Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"call-1","name":"bash","arguments":{"command":"ls -la"}}]}}`,
			want: []NormalizedEvent{{
				Kind: EventToolCall, Role: "assistant",
				ToolName: "bash", ToolCallID: "call-1", ToolInput: `{"command":"ls -la"}`,
			}},
		},
		{
			name: "tool result",
			line: `{"type":"message","timestamp":"2026-09-18T03:23:22.689Z","message":{"role":"toolResult","toolCallId":"call-1","toolName":"bash","isError":false,"content":[{"type":"text","text":"total 0"}]}}`,
			want: []NormalizedEvent{{
				Kind: EventToolResult, Role: "toolResult",
				ToolName: "bash", ToolCallID: "call-1", ToolOutput: "total 0",
			}},
		},
		{
			name: "failed tool result",
			line: `{"type":"message","timestamp":"2026-09-18T03:23:22.689Z","message":{"role":"toolResult","toolCallId":"call-2","toolName":"bash","isError":true,"content":[{"type":"text","text":"exit 1"}]}}`,
			want: []NormalizedEvent{{
				Kind: EventToolResult, Role: "toolResult", Status: "error",
				ToolName: "bash", ToolCallID: "call-2", ToolOutput: "exit 1",
			}},
		},
		{
			name: "string content body",
			line: `{"type":"message","timestamp":"2026-09-18T03:23:22.689Z","message":{"role":"user","content":"plain string prompt"}}`,
			want: []NormalizedEvent{{
				Kind: EventUserMessage, Role: "user", Content: "plain string prompt",
			}},
		},
		{
			name: "records without conversation content yield nothing",
			line: `{"type":"compaction","timestamp":"2026-09-18T03:23:22.689Z","summary":"long summary","tokensBefore":1000}`,
			want: nil,
		},
		{
			name: "session header yields nothing but carries identity",
			line: `{"type":"session","version":3,"id":"pi-sess-9","timestamp":"2026-09-18T03:23:22.689Z","cwd":"/Users/me/project"}`,
			want: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			events, context := parsePiRecord(t, tc.line)
			if len(events) != len(tc.want) {
				t.Fatalf("got %d events, want %d: %+v", len(events), len(tc.want), events)
			}
			for i, want := range tc.want {
				got := events[i]
				if got.Kind != want.Kind || got.Role != want.Role || got.Content != want.Content ||
					got.ToolName != want.ToolName || got.ToolCallID != want.ToolCallID ||
					got.ToolInput != want.ToolInput || got.ToolOutput != want.ToolOutput ||
					got.Status != want.Status {
					t.Fatalf("event %d = %+v, want %+v", i, got, want)
				}
				if got.SourceEventType == "" {
					t.Fatalf("event %d has no source_event_type", i)
				}
				if got.Timestamp.IsZero() {
					t.Fatalf("event %d has no timestamp", i)
				}
			}
			if strings.Contains(tc.line, `"id":"pi-sess-9"`) && context.SessionID != "pi-sess-9" {
				t.Fatalf("session header should set context.SessionID, got %q", context.SessionID)
			}
			if strings.Contains(tc.line, `"modelId"`) && context.Model == "" {
				t.Fatalf("model_change should set context.Model")
			}
		})
	}
}

// A tool result can embed a base64 image; the index must keep the payload out.
func TestPiEventAdapterKeepsImagePayloadsOutOfTheIndex(t *testing.T) {
	const payload = "BASE64PAYLOADMARKER"
	line := `{"type":"message","timestamp":"2026-09-18T03:23:22.689Z","message":{"role":"toolResult","toolCallId":"call-3","toolName":"read","content":[{"type":"text","text":"see screenshot"},{"type":"image","data":"` + payload + `","mimeType":"image/png"}]}}`

	events, _ := parsePiRecord(t, line)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	event := events[0]
	if strings.Contains(event.ToolOutput, payload) {
		t.Fatalf("image payload leaked into the index: %q", event.ToolOutput)
	}
	if event.ToolOutput != "see screenshot\n[image image/png]" {
		t.Fatalf("unexpected tool output %q", event.ToolOutput)
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(encoded), payload) {
		t.Fatalf("image payload leaked through event serialization")
	}
}

func TestPiEventTimestampPrefersMessageStamp(t *testing.T) {
	messageStamp := time.Date(2026, 9, 18, 3, 23, 1, 854000000, time.UTC)
	line := `{"type":"message","timestamp":"2026-09-18T03:23:22.689Z","message":{"role":"user","timestamp":` +
		strconv.FormatInt(messageStamp.UnixMilli(), 10) + `,"content":[{"type":"text","text":"hi"}]}}`

	events, _ := parsePiRecord(t, line)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if !events[0].Timestamp.Equal(messageStamp) {
		t.Fatalf("timestamp = %s, want %s (the per-message stamp)", events[0].Timestamp, messageStamp)
	}

	// Without a message stamp the record stamp is the fallback.
	fallback := `{"type":"message","timestamp":"2026-09-18T03:23:22.689Z","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}`
	events, _ = parsePiRecord(t, fallback)
	if got := events[0].Timestamp.Format(time.RFC3339Nano); got != "2026-09-18T03:23:22.689Z" {
		t.Fatalf("fallback timestamp = %s", got)
	}
}

// One JSONL line can hold several events; the unique index on
// (session_source_id, raw_offset, raw_index) would collapse them without raw_index.
func TestPiCollectorIndexesEveryEventOnASharedLine(t *testing.T) {
	db := tempDB(t)

	dir := t.TempDir()
	sessionDir := filepath.Join(dir, "--Users-me-project--")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	fpath := filepath.Join(sessionDir, "2026-09-18T03-23-22-689Z_pi-ev-1.jsonl")
	content := `{"type":"session","version":3,"id":"pi-ev-1","cwd":"/Users/me/project","timestamp":"` + ts + `"}
{"type":"model_change","timestamp":"` + ts + `","provider":"p","modelId":"deepseek-v4.1"}
{"type":"message","timestamp":"` + ts + `","message":{"role":"user","content":[{"type":"text","text":"run the tests"}]}}
{"type":"message","timestamp":"` + ts + `","message":{"role":"assistant","model":"deepseek-v4.1","usage":{"input":100,"output":50},"content":[{"type":"thinking","thinking":"plan"},{"type":"text","text":"running"},{"type":"toolCall","id":"call-1","name":"bash","arguments":{"command":"go test ./..."}}]}}
{"type":"message","timestamp":"` + ts + `","message":{"role":"toolResult","toolCallId":"call-1","toolName":"bash","isError":true,"content":[{"type":"text","text":"FAIL"}]}}
{"type":"compaction","timestamp":"` + ts + `","summary":"summarized","tokensBefore":900}
`
	if err := os.WriteFile(fpath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cx := NewPiCollector(db, []string{dir})
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	events, err := db.ListSessionEvents("pi", "pi-ev-1", 100, 0)
	if err != nil {
		t.Fatalf("ListSessionEvents: %v", err)
	}
	byKind := map[string]int{}
	for _, event := range events {
		byKind[event.EventType]++
	}
	// thinking + text + toolCall share one line, so all three must survive.
	want := map[string]int{
		string(EventUserMessage):      1,
		string(EventReasoning):        1,
		string(EventAssistantMessage): 1,
		string(EventToolCall):         1,
		string(EventToolResult):       1,
	}
	if len(events) != 5 {
		t.Fatalf("got %d events %v, want 5", len(events), byKind)
	}
	for kind, count := range want {
		if byKind[kind] != count {
			t.Fatalf("kind %s = %d, want %d (all: %v)", kind, byKind[kind], count, byKind)
		}
	}

	from := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC)
	summaries, err := db.SearchSessions(storage.SessionQuery{From: from, To: to, Source: "pi", Limit: 10})
	if err != nil {
		t.Fatalf("SearchSessions: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 session, got %d", len(summaries))
	}
	if summaries[0].ToolCalls != 1 {
		t.Fatalf("tool_calls counter = %d, want 1", summaries[0].ToolCalls)
	}
	if summaries[0].Title != "run the tests" {
		t.Fatalf("title = %q, want the first user message", summaries[0].Title)
	}

	source, err := db.GetSessionSourceByPath(fpath)
	if err != nil {
		t.Fatalf("GetSessionSourceByPath: %v", err)
	}
	if source == nil {
		t.Fatal("expected a session source row for the pi file")
	}
	// Ignored record types (compaction) are not malformed.
	if source.CoverageStatus != "complete" || source.MalformedLines != 0 {
		t.Fatalf("coverage = %s, malformed = %d", source.CoverageStatus, source.MalformedLines)
	}

	// A second scan of unchanged data must not duplicate events.
	if err := cx.Scan(); err != nil {
		t.Fatalf("Scan 2: %v", err)
	}
	again, err := db.ListSessionEvents("pi", "pi-ev-1", 100, 0)
	if err != nil {
		t.Fatalf("ListSessionEvents 2: %v", err)
	}
	if len(again) != len(events) {
		t.Fatalf("rescan produced %d events, want %d", len(again), len(events))
	}
}
