package server

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

const (
	sessionSearchDefaultLimit = 50
	sessionSearchMaxLimit     = 200
	sessionEventsDefaultLimit = 100
	sessionEventsMaxLimit     = 500
)

// SessionEventResponse is a normalized event as served to the session explorer.
type SessionEventResponse struct {
	ID              int64  `json:"id"`
	EventType       string `json:"event_type"`
	SourceEventType string `json:"source_event_type"`
	Timestamp       string `json:"timestamp"`
	Role            string `json:"role"`
	Content         string `json:"content"`
	ToolName        string `json:"tool_name"`
	ToolCallID      string `json:"tool_call_id"`
	ToolInput       string `json:"tool_input"`
	ToolOutput      string `json:"tool_output"`
	EventStatus     string `json:"event_status"`
	DurationMS      *int64 `json:"duration_ms"`
}

func (s *Server) handleSessionSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	limit, offset, err := parsePage(r, sessionSearchDefaultLimit, sessionSearchMaxLimit)
	if err != nil {
		badRequest(w, err)
		return
	}
	query := storage.SessionQuery{
		From: from, To: to, Source: r.URL.Query().Get("source"),
		Model: r.URL.Query().Get("model"), Project: r.URL.Query().Get("project"),
		Limit: limit, Offset: offset,
	}
	data, err := s.db.SearchSessions(query)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleSessionEventsRoute(w http.ResponseWriter, r *http.Request) {
	s.handleSessionEvents(w, r, r.PathValue("source"), r.PathValue("session_id"))
}

func (s *Server) handleSessionEvents(w http.ResponseWriter, r *http.Request, source, sessionID string) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	limit, offset, err := parsePage(r, sessionEventsDefaultLimit, sessionEventsMaxLimit)
	if err != nil {
		badRequest(w, err)
		return
	}
	exists, err := s.db.SessionIdentityExists(source, sessionID)
	if err != nil {
		serverError(w, err)
		return
	}
	if !exists {
		http.NotFound(w, r)
		return
	}
	events, err := s.db.ListSessionEvents(source, sessionID, limit, offset)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, buildSessionEventResponses(events))
}

// buildSessionEventResponses projects stored events for the session explorer.
func buildSessionEventResponses(events []storage.SessionEventRecord) []SessionEventResponse {
	response := make([]SessionEventResponse, 0, len(events))
	for _, event := range events {
		response = append(response, SessionEventResponse{
			ID: event.ID, EventType: event.EventType, SourceEventType: event.SourceEventType,
			Timestamp: event.Timestamp.UTC().Format(time.RFC3339Nano), Role: event.Role, Content: event.Content,
			ToolName: event.ToolName, ToolCallID: event.ToolCallID, ToolInput: event.ToolInput,
			ToolOutput: event.ToolOutput, EventStatus: event.EventStatus, DurationMS: event.DurationMS,
		})
	}
	return response
}

func (s *Server) handleSessionIndexRebuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	count, err := s.db.RebuildSessionIndex()
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, map[string]interface{}{"status": "rebuild_required", "sources": count})
}

func parsePage(r *http.Request, defaultLimit, maxLimit int) (int, int, error) {
	limit := defaultLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 || value > maxLimit {
			return 0, 0, fmt.Errorf("limit must be between 1 and %d", maxLimit)
		}
		limit = value
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return 0, 0, fmt.Errorf("offset must be a non-negative integer")
		}
		offset = value
	}
	return limit, offset, nil
}
