package collector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

type claudeEventAdapter struct{}

func (claudeEventAdapter) ParserVersion() string {
	return "claude-events-v1"
}

type claudeEventEnvelope struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"sessionId"`
	CWD       string          `json:"cwd"`
	Version   string          `json:"version"`
	Model     string          `json:"model"`
	Message   json.RawMessage `json:"message"`
	Error     json.RawMessage `json:"error"`
}

type claudeEventMessage struct {
	Role    string          `json:"role"`
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

type claudeContentBlock struct {
	Type      string          `json:"type"`
	Text      json.RawMessage `json:"text"`
	Thinking  json.RawMessage `json:"thinking"`
	Message   json.RawMessage `json:"message"`
	Error     json.RawMessage `json:"error"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
	IsError   bool            `json:"is_error"`
}

func (claudeEventAdapter) Parse(raw []byte, context *EventContext) ([]NormalizedEvent, error) {
	var envelope claudeEventEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("parse claude event: %w", err)
	}
	if context != nil {
		context.Source = "claude"
		if envelope.SessionID != "" {
			context.SessionID = envelope.SessionID
		}
		if envelope.CWD != "" {
			context.CWD = envelope.CWD
		}
		if envelope.Version != "" {
			context.Version = envelope.Version
		}
		if envelope.Model != "" {
			context.Model = envelope.Model
		}
	}

	timestamp, _ := time.Parse(time.RFC3339Nano, envelope.Timestamp)
	sourceType := envelope.Type
	if envelope.Subtype != "" {
		sourceType += ":" + envelope.Subtype
	}
	if errorPayload := bytes.TrimSpace(envelope.Error); len(errorPayload) > 0 && !bytes.Equal(errorPayload, []byte("null")) {
		return []NormalizedEvent{{
			Kind: EventError, SourceEventType: sourceType, Timestamp: timestamp,
			Content: compactOrString(errorPayload), Status: "error",
		}}, nil
	}

	switch envelope.Type {
	case "user", "assistant":
		return parseClaudeMessage(envelope, timestamp, context)
	case "error":
		content := compactOrString(envelope.Error)
		if content == "" {
			content = compactJSON(raw)
		}
		return []NormalizedEvent{{
			Kind: EventError, SourceEventType: sourceType, Timestamp: timestamp,
			Content: content, Status: "error",
		}}, nil
	case "system", "summary", "compact", "model", "metadata":
		return []NormalizedEvent{{
			Kind: EventMetadata, SourceEventType: sourceType, Timestamp: timestamp,
			Content: compactJSON(raw),
		}}, nil
	case "":
		return []NormalizedEvent{{
			Kind: EventUnknown, SourceEventType: "unknown", Timestamp: timestamp,
			Content: compactJSON(raw),
		}}, nil
	default:
		return []NormalizedEvent{{
			Kind: EventUnknown, SourceEventType: sourceType, Timestamp: timestamp,
			Content: compactJSON(raw),
		}}, nil
	}
}

func parseClaudeMessage(envelope claudeEventEnvelope, timestamp time.Time, context *EventContext) ([]NormalizedEvent, error) {
	if len(envelope.Message) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Message), []byte("null")) {
		return nil, nil
	}
	var message claudeEventMessage
	if err := json.Unmarshal(envelope.Message, &message); err != nil {
		return []NormalizedEvent{{
			Kind: EventUnknown, SourceEventType: envelope.Type, Timestamp: timestamp,
			Content: compactJSON(envelope.Message),
		}}, nil
	}
	if context != nil && message.Model != "" {
		context.Model = message.Model
	}
	role := message.Role
	if role == "" {
		role = envelope.Type
	}
	content := bytes.TrimSpace(message.Content)
	if len(content) == 0 || bytes.Equal(content, []byte("null")) {
		return nil, nil
	}
	if content[0] == '"' {
		var text string
		if err := json.Unmarshal(content, &text); err != nil {
			return nil, nil
		}
		kind := EventAssistantMessage
		if envelope.Type == "user" {
			kind = EventUserMessage
		}
		return []NormalizedEvent{{
			Kind: kind, SourceEventType: envelope.Type, Timestamp: timestamp,
			Role: role, Content: text,
		}}, nil
	}
	if content[0] != '[' {
		return []NormalizedEvent{{
			Kind: EventUnknown, SourceEventType: envelope.Type, Timestamp: timestamp,
			Role: role, Content: compactJSON(content),
		}}, nil
	}

	var blocks []json.RawMessage
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil, nil
	}
	events := make([]NormalizedEvent, 0, len(blocks))
	for _, rawBlock := range blocks {
		events = append(events, normalizeClaudeBlock(envelope.Type, role, timestamp, rawBlock)...)
	}
	return events, nil
}

func normalizeClaudeBlock(recordType, role string, timestamp time.Time, raw json.RawMessage) []NormalizedEvent {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	var block claudeContentBlock
	if trimmed[0] != '{' || json.Unmarshal(trimmed, &block) != nil || block.Type == "" {
		return []NormalizedEvent{{
			Kind: EventUnknown, SourceEventType: recordType + ":content",
			Timestamp: timestamp, Role: role, Content: compactJSON(trimmed),
		}}
	}
	base := NormalizedEvent{
		SourceEventType: recordType + ":" + block.Type,
		Timestamp:       timestamp,
		Role:            role,
	}
	switch block.Type {
	case "text":
		text, ok := rawString(block.Text)
		if !ok {
			base.Kind = EventUnknown
			base.Content = compactJSON(trimmed)
			return []NormalizedEvent{base}
		}
		if recordType == "user" {
			base.Kind = EventUserMessage
		} else {
			base.Kind = EventAssistantMessage
		}
		base.Content = text
	case "thinking":
		text, ok := rawString(block.Thinking)
		if !ok {
			base.Kind = EventUnknown
			base.Content = compactJSON(trimmed)
			return []NormalizedEvent{base}
		}
		base.Kind = EventReasoning
		base.Content = text
	case "tool_use":
		base.Kind = EventToolCall
		base.ToolCallID = block.ID
		base.ToolName = block.Name
		base.ToolInput = compactJSON(block.Input)
	case "tool_result":
		base.Kind = EventToolResult
		base.ToolCallID = block.ToolUseID
		base.ToolOutput = compactOrString(block.Content)
		if block.IsError {
			base.Status = "error"
		}
	case "error":
		base.Kind = EventError
		base.Status = "error"
		base.Content = compactOrString(block.Error)
		if base.Content == "" {
			base.Content = compactOrString(block.Message)
		}
		if base.Content == "" {
			base.Content = compactJSON(trimmed)
		}
	default:
		base.Kind = EventUnknown
		base.Content = compactJSON(trimmed)
	}
	return []NormalizedEvent{base}
}

func rawString(raw json.RawMessage) (string, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func compactOrString(raw json.RawMessage) string {
	if value, ok := rawString(raw); ok {
		return value
	}
	return compactJSON(raw)
}

func compactJSON(raw []byte) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		return string(raw)
	}
	return out.String()
}
