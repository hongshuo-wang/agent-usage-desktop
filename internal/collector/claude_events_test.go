package collector

import (
	"reflect"
	"testing"
	"time"
)

func TestClaudeEventAdapterStandardKindsInSourceOrder(t *testing.T) {
	t.Parallel()

	adapter := claudeEventAdapter{}
	ctx := &EventContext{}
	lines := []string{
		`{"type":"system","subtype":"init","timestamp":"2026-01-02T03:04:05Z","sessionId":"sess-1","cwd":"/work","version":"2.1.0","model":"claude-opus-4"}`,
		`{"type":"user","timestamp":"2026-01-02T03:04:06Z","message":{"role":"user","content":"hello"}}`,
		`{"type":"assistant","timestamp":"2026-01-02T03:04:07Z","message":{"role":"assistant","model":"claude-sonnet-4","content":[{"type":"thinking","thinking":"considering"},{"type":"text","text":"answer"},{"type":"tool_use","id":"tool-1","name":"Read","input":{"path":"/tmp/a","line":2}}]}}`,
		`{"type":"user","timestamp":"2026-01-02T03:04:08Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-1","content":"contents"},{"type":"text","text":"continue"}]}}`,
		`{"type":"error","timestamp":"2026-01-02T03:04:09Z","error":{"type":"api_error","message":"overloaded"}}`,
		`{"type":"summary","timestamp":"2026-01-02T03:04:10Z","summary":"earlier work"}`,
		`{"type":"future_visible","timestamp":"2026-01-02T03:04:11Z","payload":{"value":1}}`,
	}

	var got []NormalizedEvent
	for _, line := range lines {
		events, err := adapter.Parse([]byte(line), ctx)
		if err != nil {
			t.Fatalf("Parse(%s): %v", line, err)
		}
		got = append(got, events...)
	}

	if adapter.ParserVersion() != "claude-events-v1" {
		t.Fatalf("ParserVersion = %q", adapter.ParserVersion())
	}
	if ctx.Source != "claude" || ctx.SessionID != "sess-1" || ctx.CWD != "/work" || ctx.Version != "2.1.0" || ctx.Model != "claude-sonnet-4" {
		t.Fatalf("context = %+v", ctx)
	}

	wantKinds := []EventKind{
		EventMetadata, EventUserMessage, EventReasoning, EventAssistantMessage,
		EventToolCall, EventToolResult, EventUserMessage, EventError,
		EventMetadata, EventUnknown,
	}
	var gotKinds []EventKind
	for _, event := range got {
		gotKinds = append(gotKinds, event.Kind)
	}
	if !reflect.DeepEqual(gotKinds, wantKinds) {
		t.Fatalf("kinds = %#v, want %#v", gotKinds, wantKinds)
	}

	assertEvent := func(index int, kind EventKind, role, content string) NormalizedEvent {
		t.Helper()
		event := got[index]
		if event.Kind != kind || event.Role != role || event.Content != content {
			t.Errorf("event %d = %+v", index, event)
		}
		return event
	}
	assertEvent(1, EventUserMessage, "user", "hello")
	assertEvent(2, EventReasoning, "assistant", "considering")
	assertEvent(3, EventAssistantMessage, "assistant", "answer")
	toolCall := assertEvent(4, EventToolCall, "assistant", "")
	if toolCall.ToolCallID != "tool-1" || toolCall.ToolName != "Read" || toolCall.ToolInput != `{"path":"/tmp/a","line":2}` {
		t.Errorf("tool call = %+v", toolCall)
	}
	toolResult := assertEvent(5, EventToolResult, "user", "")
	if toolResult.ToolCallID != "tool-1" || toolResult.ToolOutput != "contents" || toolResult.Status != "" {
		t.Errorf("tool result = %+v", toolResult)
	}
	assertEvent(6, EventUserMessage, "user", "continue")
	if got[7].Content != `{"type":"api_error","message":"overloaded"}` || got[7].Status != "error" {
		t.Errorf("error event = %+v", got[7])
	}
	if got[0].SourceEventType != "system:init" || got[8].SourceEventType != "summary" || got[9].SourceEventType != "future_visible" {
		t.Errorf("source event types = %q, %q, %q", got[0].SourceEventType, got[8].SourceEventType, got[9].SourceEventType)
	}
	if got[1].Timestamp != time.Date(2026, 1, 2, 3, 4, 6, 0, time.UTC) {
		t.Errorf("timestamp = %v", got[1].Timestamp)
	}
}

func TestClaudeEventAdapterToolResultJSONAndErrorStatus(t *testing.T) {
	t.Parallel()

	line := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":{"ok":true,"count":2}},{"type":"tool_result","tool_use_id":"b","is_error":true,"content":[{"type":"text","text":"failed"}]}]}}`
	events, err := (claudeEventAdapter{}).Parse([]byte(line), &EventContext{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v", events)
	}
	if events[0].ToolOutput != `{"ok":true,"count":2}` || events[0].Status != "" {
		t.Errorf("json result = %+v", events[0])
	}
	if events[1].ToolOutput != `[{"type":"text","text":"failed"}]` || events[1].Status != "error" {
		t.Errorf("error result = %+v", events[1])
	}
}

func TestClaudeEventAdapterExplicitErrorPayloadOverridesMetadataType(t *testing.T) {
	t.Parallel()

	line := `{"type":"system","subtype":"api_error","error":{"message":"rate limited"}}`
	events, err := (claudeEventAdapter{}).Parse([]byte(line), &EventContext{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(events) != 1 || events[0].Kind != EventError || events[0].Status != "error" || events[0].Content != `{"message":"rate limited"}` {
		t.Fatalf("events = %+v", events)
	}
}

func TestClaudeEventAdapterMixedContentShapesDoNotPanic(t *testing.T) {
	t.Parallel()

	line := `{"type":"assistant","message":{"role":"assistant","content":[null,7,"loose",{"type":"text","text":9},{"type":"mystery","value":true}]}}`
	events, err := (claudeEventAdapter{}).Parse([]byte(line), &EventContext{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(events) == 0 || events[len(events)-1].Kind != EventUnknown {
		t.Fatalf("mixed events = %+v", events)
	}
}

func TestClaudeEventAdapterUsageOnlyAssistantIsNotVisible(t *testing.T) {
	t.Parallel()

	line := `{"type":"assistant","message":{"role":"assistant","model":"claude-sonnet-4","usage":{"input_tokens":1,"output_tokens":2}}}`
	events, err := (claudeEventAdapter{}).Parse([]byte(line), &EventContext{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("usage-only events = %+v", events)
	}
}

func TestClaudeEventAdapterMalformedJSON(t *testing.T) {
	t.Parallel()

	if _, err := (claudeEventAdapter{}).Parse([]byte(`{"type":`), &EventContext{}); err == nil {
		t.Fatal("Parse malformed JSON returned nil error")
	}
}
