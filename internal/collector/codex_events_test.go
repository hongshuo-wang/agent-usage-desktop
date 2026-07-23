package collector

import "testing"

func TestCodexEventAdapterNormalizesVisibleRecordsAndContext(t *testing.T) {
	t.Parallel()

	adapter := codexEventAdapter{}
	context := &EventContext{}
	lines := []string{
		`{"timestamp":"2026-01-02T03:04:05Z","type":"session_meta","payload":{"session_id":"codex-session","cwd":"/synthetic/workspace","cli_version":"9.9.9"}}`,
		`{"timestamp":"2026-01-02T03:04:06Z","type":"turn_context","payload":{"model":"synthetic-model"}}`,
		`{"timestamp":"2026-01-02T03:04:07Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"synthetic question"}]}}`,
		`{"timestamp":"2026-01-02T03:04:08Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"synthetic answer"}]}}`,
		`{"timestamp":"2026-01-02T03:04:09Z","type":"response_item","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"visible summary"}],"encrypted_content":"must-not-index"}}`,
		`{"timestamp":"2026-01-02T03:04:10Z","type":"response_item","payload":{"type":"function_call","name":"lookup","call_id":"call-1","arguments":{"z":2,"a":1}}}`,
		`{"timestamp":"2026-01-02T03:04:11Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call-1","output":{"ok":true}}}`,
		`{"timestamp":"2026-01-02T03:04:12Z","type":"event_msg","payload":{"type":"error","message":"visible failure"}}`,
	}

	var events []NormalizedEvent
	for _, line := range lines {
		got, err := adapter.Parse([]byte(line), context)
		if err != nil {
			t.Fatalf("Parse(%s): %v", line, err)
		}
		events = append(events, got...)
	}

	if context.Source != "codex" || context.SessionID != "codex-session" || context.CWD != "/synthetic/workspace" || context.Version != "9.9.9" || context.Model != "synthetic-model" {
		t.Fatalf("context = %+v", context)
	}
	if len(events) != 7 {
		t.Fatalf("events = %+v, want 7", events)
	}
	if events[0].Kind != EventMetadata || events[0].SourceEventType != "turn_context" {
		t.Errorf("metadata event = %+v", events[0])
	}
	if events[1].Kind != EventUserMessage || events[1].Content != "synthetic question" || events[1].Role != "user" {
		t.Errorf("user event = %+v", events[1])
	}
	if events[2].Kind != EventAssistantMessage || events[2].Content != "synthetic answer" || events[2].Role != "assistant" {
		t.Errorf("assistant event = %+v", events[2])
	}
	if events[3].Kind != EventReasoning || events[3].Role != "assistant" || events[3].Content != "visible summary" {
		t.Errorf("reasoning event = %+v", events[3])
	}
	if events[4].Kind != EventToolCall || events[4].ToolName != "lookup" || events[4].ToolCallID != "call-1" || events[4].ToolInput != `{"a":1,"z":2}` {
		t.Errorf("tool call = %+v", events[4])
	}
	if events[5].Kind != EventToolResult || events[5].ToolCallID != "call-1" || events[5].ToolOutput != `{"ok":true}` {
		t.Errorf("tool result = %+v", events[5])
	}
	if events[6].Kind != EventError || events[6].Status != "error" || events[6].Content != "visible failure" {
		t.Errorf("error event = %+v", events[6])
	}
}

func TestCodexEventAdapterMetadataUnknownAndMalformed(t *testing.T) {
	t.Parallel()

	adapter := codexEventAdapter{}
	context := &EventContext{}
	metadata, err := adapter.Parse([]byte(`{"timestamp":"2026-01-02T03:04:05Z","type":"turn_context","payload":{"model":"changed-model"}}`), context)
	if err != nil || len(metadata) != 1 || metadata[0].Kind != EventMetadata {
		t.Fatalf("turn context = %+v, %v", metadata, err)
	}
	unknown, err := adapter.Parse([]byte(`{"timestamp":"2026-01-02T03:04:05Z","type":"response_item","payload":{"type":"future_visible","text":"kept raw"}}`), context)
	if err != nil || len(unknown) != 1 || unknown[0].Kind != EventUnknown {
		t.Fatalf("unknown = %+v, %v", unknown, err)
	}
	tokens, err := adapter.Parse([]byte(`{"timestamp":"2026-01-02T03:04:05Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":7}}}}`), context)
	if err != nil || len(tokens) != 0 {
		t.Fatalf("token count = %+v, %v", tokens, err)
	}
	if _, err := adapter.Parse([]byte(`{"type":`), context); err == nil {
		t.Fatal("Parse malformed JSON returned nil error")
	}
}

func TestCodexEventAdapterCustomToolsAndMirrors(t *testing.T) {
	t.Parallel()

	adapter := codexEventAdapter{}
	context := &EventContext{}
	call, err := adapter.Parse([]byte(`{"type":"response_item","payload":{"type":"custom_tool_call","name":"synthetic_tool","call_id":"custom-1","arguments":"{\"z\":2,\"a\":1}"}}`), context)
	if err != nil || len(call) != 1 || call[0].Kind != EventToolCall || call[0].ToolInput != `{"a":1,"z":2}` {
		t.Fatalf("custom tool call = %+v, %v", call, err)
	}
	result, err := adapter.Parse([]byte(`{"type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"custom-1","output":"visible output"}}`), context)
	if err != nil || len(result) != 1 || result[0].Kind != EventToolResult || result[0].ToolOutput != "visible output" {
		t.Fatalf("custom tool result = %+v, %v", result, err)
	}
	mirror, err := adapter.Parse([]byte(`{"type":"event_msg","payload":{"type":"agent_message","message":"duplicate"}}`), context)
	if err != nil || len(mirror) != 0 {
		t.Fatalf("mirror events = %+v, %v", mirror, err)
	}
	streamError, err := adapter.Parse([]byte(`{"type":"event_msg","payload":{"type":"stream_error","message":"stream failed"}}`), context)
	if err != nil || len(streamError) != 1 || streamError[0].Kind != EventError || streamError[0].Content != "stream failed" {
		t.Fatalf("stream error = %+v, %v", streamError, err)
	}
	nonNullError, err := adapter.Parse([]byte(`{"type":"event_msg","payload":{"type":"notice","error":"reported failure"}}`), context)
	if err != nil || len(nonNullError) != 1 || nonNullError[0].Kind != EventError || nonNullError[0].Content != "reported failure" {
		t.Fatalf("non-null error = %+v, %v", nonNullError, err)
	}
}

func TestCodexEventAdapterErrorOverridesSuppressedEventTypes(t *testing.T) {
	t.Parallel()

	adapter := codexEventAdapter{}
	for _, test := range []struct {
		name     string
		typeName string
	}{
		{name: "token count", typeName: "token_count"},
		{name: "message mirror", typeName: "agent_message"},
	} {
		t.Run(test.name, func(t *testing.T) {
			line := `{"type":"event_msg","payload":{"type":"` + test.typeName + `","error":"synthetic failure"}}`
			events, err := adapter.Parse([]byte(line), &EventContext{})
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(events) != 1 || events[0].Kind != EventError || events[0].Status != "error" || events[0].Content != "synthetic failure" {
				t.Fatalf("events = %+v, want visible error", events)
			}
		})
	}
}

func TestCodexEventAdapterToolPayloadPreservesLargeInteger(t *testing.T) {
	t.Parallel()

	events, err := (codexEventAdapter{}).Parse([]byte(`{"type":"response_item","payload":{"type":"function_call","name":"synthetic_tool","call_id":"large-1","arguments":{"count":9007199254740993}}}`), &EventContext{})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(events) != 1 || events[0].ToolInput != `{"count":9007199254740993}` {
		t.Fatalf("events = %+v, want exact large integer", events)
	}
}
