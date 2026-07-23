package collector

import "time"

// EventKind categorizes an event emitted by a source-specific session adapter.
type EventKind string

const (
	EventUserMessage      EventKind = "user_message"
	EventAssistantMessage EventKind = "assistant_message"
	EventReasoning        EventKind = "reasoning"
	EventToolCall         EventKind = "tool_call"
	EventToolResult       EventKind = "tool_result"
	EventError            EventKind = "error"
	EventMetadata         EventKind = "metadata"
	EventUnknown          EventKind = "unknown"
)

// EventContext carries session metadata accumulated while parsing a source.
type EventContext struct {
	Source    string
	SessionID string
	CWD       string
	Version   string
	Model     string
}

// NormalizedEvent is a source-neutral representation of a session event.
type NormalizedEvent struct {
	Kind            EventKind
	SourceEventType string
	Timestamp       time.Time
	Role            string
	Content         string
	ToolName        string
	ToolCallID      string
	ToolInput       string
	ToolOutput      string
	Status          string
	DurationMS      *int64
}

// SessionEventAdapter parses raw source records into normalized session events.
type SessionEventAdapter interface {
	ParserVersion() string
	Parse(raw []byte, context *EventContext) ([]NormalizedEvent, error)
}
