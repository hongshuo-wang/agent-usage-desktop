package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
)

type purgeSessionEventsRequest struct {
	Mode   string `json:"mode"`
	Days   int    `json:"days"`
	Vacuum *bool  `json:"vacuum"`
}

type purgeSessionEventsResponse struct {
	Deleted    int64 `json:"deleted"`
	Vacuumed   bool  `json:"vacuumed"`
	BytesFreed int64 `json:"bytes_freed"`
}

// handlePurgeSessionEvents deletes indexed session content on demand: either
// everything older than a chosen number of days, or the whole index. Usage
// records, prompts and session metadata are never touched.
func (s *Server) handlePurgeSessionEvents(w http.ResponseWriter, r *http.Request) {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	var request purgeSessionEventsRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			http.Error(w, "request body is too large", http.StatusRequestEntityTooLarge)
			return
		}
		badRequest(w, fmt.Errorf("invalid purge request: %w", err))
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		badRequest(w, fmt.Errorf("invalid purge request: multiple JSON values"))
		return
	}

	days := request.Days
	switch request.Mode {
	case "retention":
		if request.Days < 1 || request.Days > maximumRetentionDays {
			badRequest(w, fmt.Errorf("days must be between 1 and %d", maximumRetentionDays))
			return
		}
	case "all":
		days = 0
	default:
		badRequest(w, fmt.Errorf("mode must be \"retention\" or \"all\""))
		return
	}

	vacuum := request.Vacuum == nil || *request.Vacuum
	before := s.db.SizeBytes()
	deleted, err := s.db.PurgeSessionEvents(days)
	if err != nil {
		serverError(w, err)
		return
	}
	vacuumed := false
	if vacuum {
		if err := s.db.Vacuum(); err != nil {
			serverError(w, err)
			return
		}
		vacuumed = true
	}
	freed := before - s.db.SizeBytes()
	if freed < 0 {
		freed = 0
	}
	writeJSON(w, purgeSessionEventsResponse{Deleted: deleted, Vacuumed: vacuumed, BytesFreed: freed})
}
