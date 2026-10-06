package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/config"
	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

// offsetBase keeps raw locators unique across calls: the unique index on
// (session_source_id, raw_offset, raw_index) silently drops repeats.
func seedSessionEvents(t *testing.T, db *storage.DB, count int, age time.Duration, offsetBase int) {
	t.Helper()
	sourceID, err := db.UpsertSessionSource(&storage.SessionSource{
		Source:         "claude",
		SessionID:      "session-one",
		SourceKind:     "jsonl",
		Path:           "/sessions/one.jsonl",
		ParserVersion:  "v1",
		HeadHash:       "head",
		FileSize:       2048,
		IndexedOffset:  1024,
		CoverageStatus: "complete",
		SourceStatus:   "available",
		LastIndexedAt:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("UpsertSessionSource: %v", err)
	}
	events := make([]storage.SessionEventRecord, 0, count)
	for i := 0; i < count; i++ {
		events = append(events, storage.SessionEventRecord{
			SessionSourceID: sourceID,
			Source:          "claude",
			SessionID:       "session-one",
			EventType:       "user_message",
			Timestamp:       time.Now().UTC().Add(-age).Add(time.Duration(i) * time.Second),
			RawOffset:       int64(offsetBase + i*100),
			RawLength:       10,
			RawIndex:        i,
		})
	}
	if err := db.InsertSessionEvents(events); err != nil {
		t.Fatalf("InsertSessionEvents: %v", err)
	}
}

func sessionEventCount(t *testing.T, db *storage.DB) int {
	t.Helper()
	events, err := db.ListSessionEvents("claude", "session-one", 1000, 0)
	if err != nil {
		t.Fatalf("ListSessionEvents: %v", err)
	}
	return len(events)
}

func requestPurge(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/maintenance/purge-session-events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

func TestPurgeSessionEventsEndpointRetentionMode(t *testing.T) {
	db := tempDB(t)
	seedSessionEvents(t, db, 2, 90*24*time.Hour, 0)
	seedSessionEvents(t, db, 3, time.Hour, 1000)
	handler := New(db, "127.0.0.1:0").Handler()

	w := requestPurge(t, handler, `{"mode":"retention","days":30,"vacuum":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var response struct {
		Deleted    int64 `json:"deleted"`
		Vacuumed   bool  `json:"vacuumed"`
		BytesFreed int64 `json:"bytes_freed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Deleted != 2 || response.Vacuumed {
		t.Fatalf("response=%+v, want 2 deleted and no vacuum", response)
	}
	if remaining := sessionEventCount(t, db); remaining != 3 {
		t.Fatalf("remaining=%d, want 3", remaining)
	}
}

func TestPurgeSessionEventsEndpointAllMode(t *testing.T) {
	db := tempDB(t)
	seedSessionEvents(t, db, 4, time.Hour, 0)
	handler := New(db, "127.0.0.1:0").Handler()

	w := requestPurge(t, handler, `{"mode":"all"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var response struct {
		Deleted  int64 `json:"deleted"`
		Vacuumed bool  `json:"vacuumed"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	// vacuum defaults to true when the field is omitted.
	if response.Deleted != 4 || !response.Vacuumed {
		t.Fatalf("response=%+v, want 4 deleted and vacuumed", response)
	}
	if remaining := sessionEventCount(t, db); remaining != 0 {
		t.Fatalf("remaining=%d, want 0", remaining)
	}
}

func TestPurgeSessionEventsEndpointRejectsBadRequests(t *testing.T) {
	handler := New(tempDB(t), "127.0.0.1:0").Handler()
	for name, body := range map[string]string{
		"unknown mode":    `{"mode":"nope"}`,
		"zero days":       `{"mode":"retention","days":0}`,
		"negative days":   `{"mode":"retention","days":-5}`,
		"days over bound": `{"mode":"retention","days":99999}`,
		"unknown field":   `{"mode":"all","vacuum":true,"extra":1}`,
		"missing mode":    `{}`,
		"malformed json":  `{"mode":`,
		"multiple values": `{"mode":"all"}{"mode":"all"}`,
	} {
		w := requestPurge(t, handler, body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status=%d body=%s", name, w.Code, w.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/maintenance/purge-session-events", strings.NewReader(`{"mode":"all"}`))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("non-json content type: status=%d", w.Code)
	}
}

func TestSettingsRetentionDaysRoundTrip(t *testing.T) {
	handler, path := settingsFixture(t)
	withRetention := func(days int) string {
		return strings.Replace(validSettingsBody, "\n}",
			fmt.Sprintf(",\n  \"session_event_retention_days\":%d\n}", days), 1)
	}
	loadRetention := func() int {
		t.Helper()
		cfg, err := config.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return cfg.Storage.SessionEventRetentionDays
	}

	// retention 0 is a deliberate value ("keep forever"), not "unset".
	if w := requestSettings(t, handler, http.MethodPut, withRetention(0)); w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := loadRetention(); got != 0 {
		t.Fatalf("retention=%d, want 0", got)
	}

	// Omitting the field must leave the configured value untouched, otherwise an
	// older client would silently disable retention.
	if w := requestSettings(t, handler, http.MethodPut, validSettingsBody); w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := loadRetention(); got != 0 {
		t.Fatalf("omitted retention changed the value to %d", got)
	}

	if w := requestSettings(t, handler, http.MethodPut, withRetention(90)); w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := loadRetention(); got != 90 {
		t.Fatalf("retention=%d, want 90", got)
	}

	for _, invalid := range []int{-1, maximumRetentionDays + 1} {
		if w := requestSettings(t, handler, http.MethodPut, withRetention(invalid)); w.Code != http.StatusBadRequest {
			t.Fatalf("retention=%d: status=%d body=%s", invalid, w.Code, w.Body.String())
		}
	}
	if got := loadRetention(); got != 90 {
		t.Fatalf("rejected request changed retention to %d", got)
	}
}
