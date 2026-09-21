package collector

import (
	"encoding/json"
	"strings"
	"time"
)

// piEventAdapter normalizes pi session records into inspectable session events.
type piEventAdapter struct{}

// ParserVersion identifies the pi event extraction shape; bump it when the
// mapping changes so existing sources are re-indexed.
func (piEventAdapter) ParserVersion() string { return "pi-events-v1" }

// piPart is one content block inside a pi message. Image blocks carry base64
// payloads, so only their MIME type is ever read.
type piPart struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Thinking  string          `json:"thinking"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	MimeType  string          `json:"mimeType"`
}

// Parse maps one pi JSONL record onto zero or more normalized events. Records
// that carry no inspectable conversation content yield no events.
func (piEventAdapter) Parse(raw []byte, context *EventContext) ([]NormalizedEvent, error) {
	var entry piEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil, err
	}

	switch entry.Type {
	case "session":
		if entry.ID != "" {
			context.SessionID = entry.ID
		}
		if entry.CWD != "" {
			context.CWD = entry.CWD
		}
		return nil, nil
	case "model_change":
		if entry.ModelID != "" {
			context.Model = entry.ModelID
		}
		return nil, nil
	case "message":
	default:
		return nil, nil
	}

	if len(entry.Message) == 0 {
		return nil, nil
	}
	var msg piMessage
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return nil, err
	}
	if msg.Model != "" {
		context.Model = msg.Model
	}
	timestamp := piEventTimestamp(entry.Timestamp, msg.Timestamp)

	switch msg.Role {
	case "user":
		var events []NormalizedEvent
		for _, part := range piContentParts(msg.Content) {
			if part.Type != "text" || part.Text == "" {
				continue
			}
			events = append(events, NormalizedEvent{
				Kind:            EventUserMessage,
				SourceEventType: "message:user",
				Timestamp:       timestamp,
				Role:            "user",
				Content:         part.Text,
			})
		}
		return events, nil

	case "assistant":
		var events []NormalizedEvent
		for _, part := range piContentParts(msg.Content) {
			event := NormalizedEvent{
				SourceEventType: "message:assistant",
				Timestamp:       timestamp,
				Role:            "assistant",
			}
			switch part.Type {
			case "text":
				if part.Text == "" {
					continue
				}
				event.Kind = EventAssistantMessage
				event.Content = part.Text
			case "thinking":
				if part.Thinking == "" {
					continue
				}
				event.Kind = EventReasoning
				event.Content = part.Thinking
			case "toolCall":
				event.Kind = EventToolCall
				event.ToolCallID = part.ID
				event.ToolName = part.Name
				event.ToolInput = compactJSON(part.Arguments)
			default:
				continue
			}
			events = append(events, event)
		}
		return events, nil

	case "toolResult":
		output := piToolResultOutput(msg.Content)
		event := NormalizedEvent{
			Kind:            EventToolResult,
			SourceEventType: "message:toolResult",
			Timestamp:       timestamp,
			Role:            "toolResult",
			ToolName:        msg.ToolName,
			ToolCallID:      msg.ToolCallID,
			ToolOutput:      output,
		}
		if msg.IsError {
			event.Status = "error"
		}
		return []NormalizedEvent{event}, nil
	}

	return nil, nil
}

// piToolResultOutput joins readable result parts and keeps image payloads out
// of the index.
func piToolResultOutput(raw json.RawMessage) string {
	var parts []string
	for _, part := range piContentParts(raw) {
		switch part.Type {
		case "text":
			if part.Text != "" {
				parts = append(parts, part.Text)
			}
		case "image":
			marker := "[image]"
			if part.MimeType != "" {
				marker = "[image " + part.MimeType + "]"
			}
			parts = append(parts, marker)
		}
	}
	return strings.Join(parts, "\n")
}

// piContentParts accepts a message body that is either a raw string or a list
// of content blocks.
func piContentParts(raw json.RawMessage) []piPart {
	if len(raw) == 0 {
		return nil
	}
	var parts []piPart
	if err := json.Unmarshal(raw, &parts); err == nil {
		return parts
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil && text != "" {
		return []piPart{{Type: "text", Text: text}}
	}
	return nil
}

// piEventTimestamp prefers the message's epoch-millisecond stamp, which is
// per-message, over the record's write-time stamp.
func piEventTimestamp(recordStamp string, messageStamp int64) time.Time {
	if messageStamp > 0 {
		return time.UnixMilli(messageStamp).UTC()
	}
	ts, _ := time.Parse(time.RFC3339Nano, recordStamp)
	return ts
}
