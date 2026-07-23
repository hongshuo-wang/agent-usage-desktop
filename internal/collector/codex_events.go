package collector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type codexEventAdapter struct{}

func (codexEventAdapter) ParserVersion() string { return "codex-events-v1" }

type codexEventEnvelope struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexEventSessionMeta struct {
	ID         string `json:"id"`
	SessionID  string `json:"session_id"`
	CWD        string `json:"cwd"`
	Version    string `json:"version"`
	CLIVersion string `json:"cli_version"`
}

type codexEventTurnContext struct {
	Model string `json:"model"`
}

type codexEventResponse struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Summary json.RawMessage `json:"summary"`
	Text    json.RawMessage `json:"text"`
	Name    string          `json:"name"`
	CallID  string          `json:"call_id"`
	Input   json.RawMessage `json:"input"`
	Args    json.RawMessage `json:"arguments"`
	Output  json.RawMessage `json:"output"`
}

type codexEventMessagePart struct {
	Type string          `json:"type"`
	Text json.RawMessage `json:"text"`
}

type codexEventMessage struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
	Error   json.RawMessage `json:"error"`
}

func (codexEventAdapter) Parse(raw []byte, context *EventContext) ([]NormalizedEvent, error) {
	var envelope codexEventEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("parse codex event: %w", err)
	}
	if context != nil {
		context.Source = "codex"
	}
	timestamp, _ := time.Parse(time.RFC3339Nano, envelope.Timestamp)

	switch envelope.Type {
	case "session_meta":
		var meta codexEventSessionMeta
		if err := json.Unmarshal(envelope.Payload, &meta); err != nil {
			return nil, fmt.Errorf("parse codex session metadata: %w", err)
		}
		if context != nil {
			if meta.SessionID != "" {
				context.SessionID = meta.SessionID
			} else if meta.ID != "" {
				context.SessionID = meta.ID
			}
			if meta.CWD != "" {
				context.CWD = meta.CWD
			}
			if meta.CLIVersion != "" {
				context.Version = meta.CLIVersion
			} else if meta.Version != "" {
				context.Version = meta.Version
			}
		}
		return nil, nil
	case "turn_context":
		var turn codexEventTurnContext
		if err := json.Unmarshal(envelope.Payload, &turn); err != nil {
			return nil, fmt.Errorf("parse codex turn context: %w", err)
		}
		changed := turn.Model != "" && (context == nil || context.Model != turn.Model)
		if context != nil && turn.Model != "" {
			context.Model = turn.Model
		}
		if !changed {
			return nil, nil
		}
		return []NormalizedEvent{{
			Kind: EventMetadata, SourceEventType: "turn_context", Timestamp: timestamp,
			Content: stableJSON(envelope.Payload),
		}}, nil
	case "response_item":
		return parseCodexResponseItem(envelope.Payload, timestamp)
	case "event_msg":
		return parseCodexEventMessage(envelope.Payload, timestamp)
	default:
		return []NormalizedEvent{{
			Kind: EventUnknown, SourceEventType: sourceEventType(envelope.Type), Timestamp: timestamp,
			Content: stableJSON(envelope.Payload),
		}}, nil
	}
}

func parseCodexResponseItem(raw json.RawMessage, timestamp time.Time) ([]NormalizedEvent, error) {
	var response codexEventResponse
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("parse codex response item: %w", err)
	}
	sourceType := "response_item"
	if response.Type != "" {
		sourceType += ":" + response.Type
	}
	switch response.Type {
	case "message":
		if response.Role != "user" && response.Role != "assistant" {
			return []NormalizedEvent{{Kind: EventUnknown, SourceEventType: sourceType, Timestamp: timestamp, Content: stableJSON(raw)}}, nil
		}
		kind := EventUserMessage
		if response.Role == "assistant" {
			kind = EventAssistantMessage
		}
		return codexVisibleTextEvents(response.Content, kind, response.Role, sourceType, timestamp), nil
	case "reasoning":
		events := codexVisibleTextEvents(response.Summary, EventReasoning, "", sourceType, timestamp)
		if len(events) == 0 {
			events = codexVisibleTextEvents(response.Text, EventReasoning, "", sourceType, timestamp)
		}
		return events, nil
	case "function_call", "custom_tool_call":
		input := response.Args
		if len(bytes.TrimSpace(input)) == 0 {
			input = response.Input
		}
		return []NormalizedEvent{{
			Kind: EventToolCall, SourceEventType: sourceType, Timestamp: timestamp,
			ToolName: response.Name, ToolCallID: response.CallID, ToolInput: stableToolPayload(input),
		}}, nil
	case "function_call_output", "custom_tool_call_output":
		return []NormalizedEvent{{
			Kind: EventToolResult, SourceEventType: sourceType, Timestamp: timestamp,
			ToolCallID: response.CallID, ToolOutput: stableToolPayload(response.Output),
		}}, nil
	default:
		return []NormalizedEvent{{Kind: EventUnknown, SourceEventType: sourceType, Timestamp: timestamp, Content: stableJSON(raw)}}, nil
	}
}

func parseCodexEventMessage(raw json.RawMessage, timestamp time.Time) ([]NormalizedEvent, error) {
	var message codexEventMessage
	if err := json.Unmarshal(raw, &message); err != nil {
		return nil, fmt.Errorf("parse codex event message: %w", err)
	}
	if message.Type == "token_count" {
		return nil, nil
	}
	// These records mirror canonical response_item messages and would duplicate them.
	switch message.Type {
	case "agent_message", "agent_message_delta", "user_message", "user_message_delta":
		return nil, nil
	}
	if strings.Contains(message.Type, "error") || len(bytes.TrimSpace(message.Error)) > 0 && !bytes.Equal(bytes.TrimSpace(message.Error), []byte("null")) {
		content := stableJSONOrString(message.Message)
		if content == "" {
			content = stableJSONOrString(message.Error)
		}
		return []NormalizedEvent{{
			Kind: EventError, SourceEventType: "event_msg:" + message.Type, Timestamp: timestamp,
			Content: content, Status: "error",
		}}, nil
	}
	return []NormalizedEvent{{Kind: EventUnknown, SourceEventType: "event_msg:" + sourceEventType(message.Type), Timestamp: timestamp, Content: stableJSON(raw)}}, nil
}

func codexVisibleTextEvents(raw json.RawMessage, kind EventKind, role, sourceType string, timestamp time.Time) []NormalizedEvent {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	if text, ok := rawString(trimmed); ok {
		if kind == EventReasoning && role == "" {
			role = "assistant"
		}
		return []NormalizedEvent{{Kind: kind, SourceEventType: sourceType, Timestamp: timestamp, Role: role, Content: text}}
	}
	var parts []json.RawMessage
	if json.Unmarshal(trimmed, &parts) != nil {
		parts = []json.RawMessage{trimmed}
	}
	events := make([]NormalizedEvent, 0, len(parts))
	for _, part := range parts {
		var item codexEventMessagePart
		if json.Unmarshal(part, &item) != nil {
			continue
		}
		if item.Type != "input_text" && item.Type != "output_text" && item.Type != "text" && item.Type != "summary_text" {
			continue
		}
		if text, ok := rawString(item.Text); ok {
			eventRole := role
			if kind == EventReasoning && eventRole == "" {
				eventRole = "assistant"
			}
			events = append(events, NormalizedEvent{Kind: kind, SourceEventType: sourceType + ":" + item.Type, Timestamp: timestamp, Role: role, Content: text})
			events[len(events)-1].Role = eventRole
		}
	}
	return events
}

func stableJSONOrString(raw json.RawMessage) string {
	if text, ok := rawString(raw); ok {
		return text
	}
	return stableJSON(raw)
}

func stableToolPayload(raw json.RawMessage) string {
	if text, ok := rawString(raw); ok {
		if json.Valid([]byte(text)) {
			return stableJSON([]byte(text))
		}
		return text
	}
	return stableJSON(raw)
}

func stableJSON(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	var value any
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return string(trimmed)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return string(trimmed)
	}
	return string(encoded)
}

func sourceEventType(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
