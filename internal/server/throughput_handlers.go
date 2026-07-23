package server

import (
	"net/http"
)

// handleThroughput returns local observed RPM and TPM, not provider limits.
func (s *Server) handleThroughput(w http.ResponseWriter, r *http.Request) {
	from, to, tzOffset, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	result, err := s.db.GetThroughput(
		from,
		to,
		r.URL.Query().Get("source"),
		r.URL.Query().Get("model"),
		tzOffset,
	)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, result)
}
