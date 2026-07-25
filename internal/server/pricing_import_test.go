package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/pricing"
)

func TestPricingImportEndpoint(t *testing.T) {
	db := tempDB(t)
	handler := New(db, "127.0.0.1:0").Handler()
	body := `{"model-a":{"input_cost_per_token":0.1,"output_cost_per_token":0.2}}`
	req := httptest.NewRequest(http.MethodPost, "/api/pricing/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var result struct {
		SnapshotID int64  `json:"snapshot_id"`
		Entries    int    `json:"entries"`
		Revision   string `json:"revision"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.SnapshotID == 0 || result.Entries != 1 || result.Revision == "" {
		t.Errorf("response = %#v, want snapshot, one entry, revision", result)
	}
}

func TestPricingImportEndpointRejectsInvalidContent(t *testing.T) {
	db := tempDB(t)
	handler := New(db, "127.0.0.1:0").Handler()
	tests := []struct {
		name  string
		body  string
		ctype string
		want  int
	}{
		{name: "wrong content type", body: `{}`, ctype: "text/plain", want: http.StatusUnsupportedMediaType},
		{name: "malformed json", body: `{`, ctype: "application/json", want: http.StatusBadRequest},
		{name: "no valid entries", body: `{"image":{"output_cost_per_token":0.1}}`, ctype: "application/json", want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/pricing/import", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.ctype)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Fatalf("status = %d, body = %s, want %d", w.Code, w.Body.String(), tt.want)
			}
		})
	}
}

func TestPricingImportEndpointRejectsOversizedBody(t *testing.T) {
	db := tempDB(t)
	handler := New(db, "127.0.0.1:0").Handler()
	// Content-Length is enough to reject without allocating a body near the limit.
	req := httptest.NewRequest(http.MethodPost, "/api/pricing/import", bytes.NewReader([]byte(`{}`)))
	req.ContentLength = pricing.MaxPricingImportBytes + 1
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body = %s, want %d", w.Code, w.Body.String(), http.StatusRequestEntityTooLarge)
	}
}
