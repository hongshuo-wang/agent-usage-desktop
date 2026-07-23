package server

import "net/http"

func (s *Server) handleCollectionIndexStatus(w http.ResponseWriter, _ *http.Request) {
	status, err := s.db.GetCollectionIndexStatus()
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, status)
}
