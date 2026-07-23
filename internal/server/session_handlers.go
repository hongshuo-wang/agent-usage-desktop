package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

const (
	sessionSearchDefaultLimit = 50
	sessionSearchMaxLimit     = 200
	sessionEventsDefaultLimit = 100
	sessionEventsMaxLimit     = 500
	sessionRawMaxBytes        = 16 << 20
)

// SessionEventResponse adds raw-record availability to a normalized event.
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
	HasRaw          bool   `json:"has_raw"`
}

// RawEventResponse returns the exact bytes addressed by an event locator.
type RawEventResponse struct {
	Path        string `json:"path"`
	Offset      int64  `json:"offset"`
	Length      int64  `json:"length"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"`
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
		Search: r.URL.Query().Get("q"), Limit: limit, Offset: offset,
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

func (s *Server) handleSessionRawRoute(w http.ResponseWriter, r *http.Request) {
	eventID, err := strconv.ParseInt(r.PathValue("event_id"), 10, 64)
	if err != nil || eventID <= 0 {
		badRequest(w, fmt.Errorf("event id must be a positive integer"))
		return
	}
	s.handleSessionRaw(w, r, r.PathValue("source"), r.PathValue("session_id"), eventID)
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
	response := make([]SessionEventResponse, 0, len(events))
	for _, event := range events {
		item := SessionEventResponse{
			ID: event.ID, EventType: event.EventType, SourceEventType: event.SourceEventType,
			Timestamp: event.Timestamp.UTC().Format(time.RFC3339Nano), Role: event.Role, Content: event.Content,
			ToolName: event.ToolName, ToolCallID: event.ToolCallID, ToolInput: event.ToolInput,
			ToolOutput: event.ToolOutput, EventStatus: event.EventStatus, DurationMS: event.DurationMS,
		}
		if event.RawOffset >= 0 && event.RawLength > 0 && event.RawLength <= sessionRawMaxBytes {
			locator, err := s.db.GetRawEventLocator(source, sessionID, event.ID)
			if err != nil {
				serverError(w, err)
				return
			}
			item.HasRaw = rawLocatorAvailable(locator)
		}
		response = append(response, item)
	}
	writeJSON(w, response)
}

func (s *Server) handleSessionRaw(w http.ResponseWriter, r *http.Request, source, sessionID string, eventID int64) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	locator, err := s.db.GetRawEventLocator(source, sessionID, eventID)
	if err != nil {
		serverError(w, err)
		return
	}
	if locator == nil {
		http.NotFound(w, r)
		return
	}
	if locator.SourceStatus != "available" || locator.Path == "" {
		http.Error(w, "source unavailable", http.StatusGone)
		return
	}
	if locator.RawOffset < 0 || locator.RawLength <= 0 {
		badRequest(w, fmt.Errorf("invalid raw locator"))
		return
	}
	if locator.RawLength > sessionRawMaxBytes {
		http.Error(w, "raw record too large", http.StatusRequestEntityTooLarge)
		return
	}
	if locator.RawOffset > math.MaxInt64-locator.RawLength {
		badRequest(w, fmt.Errorf("invalid raw locator"))
		return
	}
	file, err := os.Open(locator.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "source unavailable", http.StatusGone)
			return
		}
		serverError(w, err)
		return
	}
	defer file.Close()
	content := make([]byte, int(locator.RawLength))
	read, err := file.ReadAt(content, locator.RawOffset)
	if err != nil && err != io.EOF {
		serverError(w, err)
		return
	}
	if int64(read) != locator.RawLength {
		http.Error(w, "source unavailable", http.StatusGone)
		return
	}
	contentType := "text"
	if json.Valid(content) {
		contentType = "json"
	}
	writeJSON(w, RawEventResponse{
		Path: locator.Path, Offset: locator.RawOffset, Length: locator.RawLength,
		ContentType: contentType, Content: string(content),
	})
}

func rawLocatorAvailable(locator *storage.RawEventLocator) bool {
	if locator == nil || locator.SourceStatus != "available" || locator.Path == "" ||
		locator.RawOffset < 0 || locator.RawLength <= 0 || locator.RawLength > sessionRawMaxBytes ||
		locator.RawOffset > math.MaxInt64-locator.RawLength {
		return false
	}
	info, err := os.Stat(locator.Path)
	return err == nil && !info.IsDir() && locator.RawOffset+locator.RawLength <= info.Size()
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
