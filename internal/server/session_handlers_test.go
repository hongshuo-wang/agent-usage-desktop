package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

const (
	searchDefaultLimit = 50
	searchMaxLimit     = 200
	eventsMaxLimit     = 500
	rawRecordMaxBytes  = 16 << 20
)

type sessionAPIFixture struct {
	db        *storage.DB
	handler   http.Handler
	rawPath   string
	rawBytes  []byte
	eventIDs  map[string]int64
	sourceIDs map[string]int64
	day       time.Time
}

func seedSessionAPI(t *testing.T) *sessionAPIFixture {
	t.Helper()
	db := tempDB(t)
	day := time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)

	for _, session := range []*storage.SessionRecord{
		{Source: "claude", SessionID: "shared", Project: "alpha", CWD: "/work/alpha", GitBranch: "main", StartTime: day.Add(-time.Hour)},
		{Source: "codex", SessionID: "shared", Project: "beta", CWD: "/work/beta", GitBranch: "dev", StartTime: day.Add(-2 * time.Hour)},
	} {
		if err := db.UpsertSession(session); err != nil {
			t.Fatalf("UpsertSession(%s): %v", session.Source, err)
		}
	}
	if err := db.UpsertPricing("priced-model", 0.01, 0.02, 0.003, 0.004); err != nil {
		t.Fatalf("UpsertPricing: %v", err)
	}
	usage := []*storage.UsageRecord{
		{Source: "claude", SessionID: "shared", Model: "priced-model", InputTokens: 10, OutputTokens: 2, CacheReadInputTokens: 3, CacheCreationInputTokens: 4, CostUSD: 1.25, Timestamp: day, Project: "alpha", GitBranch: "main"},
		{Source: "claude", SessionID: "shared", Model: "unknown-model", InputTokens: 20, OutputTokens: 5, CostUSD: 0, Timestamp: day.Add(time.Minute), Project: "alpha", GitBranch: "main"},
		{Source: "claude", SessionID: "shared", Model: "priced-model", InputTokens: 100, OutputTokens: 50, CostUSD: 9, Timestamp: day.AddDate(0, 0, -2), Project: "alpha"},
		{Source: "codex", SessionID: "shared", Model: "priced-model", InputTokens: 7, OutputTokens: 3, CostUSD: 0.5, Timestamp: day, Project: "beta", GitBranch: "dev"},
	}
	if err := db.InsertUsageBatch(usage); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}
	if err := db.InsertPromptBatch([]*storage.PromptEvent{
		{Source: "claude", SessionID: "shared", Timestamp: day.Add(10 * time.Second)},
		{Source: "claude", SessionID: "shared", Timestamp: day.Add(20 * time.Second)},
		{Source: "claude", SessionID: "shared", Timestamp: day.AddDate(0, 0, -2)},
		{Source: "codex", SessionID: "shared", Timestamp: day.Add(30 * time.Second)},
	}); err != nil {
		t.Fatalf("InsertPromptBatch: %v", err)
	}

	rawBytes := []byte("header\n{\"kind\":\"json\",\"value\":42}\nplain text record\n")
	rawPath := filepath.Join(t.TempDir(), "claude.jsonl")
	if err := os.WriteFile(rawPath, rawBytes, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	jsonRecord := []byte(`{"kind":"json","value":42}`)
	textRecord := []byte("plain text record")
	jsonOffset := int64(len("header\n"))
	textOffset := jsonOffset + int64(len(jsonRecord)) + 1

	claudeSource := &storage.SessionSource{
		Source: "claude", SessionID: "shared", Path: rawPath, ParserVersion: "v1",
		HeadHash: indexedHeadHash(rawBytes),
		FileSize: int64(len(rawBytes)), IndexedOffset: int64(len(rawBytes)),
		CoverageStatus: "complete", SourceStatus: "available",
	}
	claudeEvents := []storage.SessionEventRecord{
		{Source: "claude", SessionID: "shared", EventType: "user_message", SourceEventType: "user", Timestamp: day.Add(1 * time.Minute), Role: "user", Content: `Investigate "build" #42?`, RawOffset: jsonOffset, RawLength: int64(len(jsonRecord)), RawIndex: 0},
		{Source: "claude", SessionID: "shared", EventType: "tool_call", SourceEventType: "assistant", Timestamp: day.Add(2 * time.Minute), Role: "assistant", ToolName: "Read.Tool", ToolCallID: "call-1", ToolInput: `{"path":"a.go"}`, RawOffset: textOffset, RawLength: int64(len(textRecord)), RawIndex: 0},
		{Source: "claude", SessionID: "shared", EventType: "tool_result", SourceEventType: "user", Timestamp: day.Add(3 * time.Minute), Role: "tool", ToolCallID: "call-1", ToolOutput: "needle-output", RawOffset: 100, RawIndex: 0},
		{Source: "claude", SessionID: "shared", EventType: "error", SourceEventType: "error", Timestamp: day.Add(4 * time.Minute), Content: "failed request", EventStatus: "error", RawOffset: 200, RawIndex: 0},
	}
	claudeID, err := db.UpsertSessionSourceWithEvents(claudeSource, claudeEvents)
	if err != nil {
		t.Fatalf("UpsertSessionSourceWithEvents(claude): %v", err)
	}
	missingPath := filepath.Join(t.TempDir(), "missing.jsonl")
	if _, err := db.UpsertSessionSource(&storage.SessionSource{
		Source: "claude", SessionID: "shared", Path: missingPath, ParserVersion: "v1",
		CoverageStatus: "partial", SourceStatus: "missing_source", MalformedLines: 2,
	}); err != nil {
		t.Fatalf("UpsertSessionSource(missing): %v", err)
	}

	codexPath := filepath.Join(t.TempDir(), "codex.jsonl")
	if err := os.WriteFile(codexPath, []byte("codex"), 0o600); err != nil {
		t.Fatalf("WriteFile(codex): %v", err)
	}
	codexID, err := db.UpsertSessionSourceWithEvents(&storage.SessionSource{
		Source: "codex", SessionID: "shared", Path: codexPath, ParserVersion: "v1",
		HeadHash: indexedHeadHash([]byte("codex")),
		FileSize: 5, IndexedOffset: 5, CoverageStatus: "complete", SourceStatus: "available",
	}, []storage.SessionEventRecord{
		{Source: "codex", SessionID: "shared", EventType: "user_message", Timestamp: day.Add(time.Minute), Role: "user", Content: "Codex title", RawOffset: 0, RawLength: 5, RawIndex: 0},
	})
	if err != nil {
		t.Fatalf("UpsertSessionSourceWithEvents(codex): %v", err)
	}

	openclawPath := filepath.Join(t.TempDir(), "openclaw.jsonl")
	if err := os.WriteFile(openclawPath, []byte("event"), 0o600); err != nil {
		t.Fatalf("WriteFile(openclaw): %v", err)
	}
	openclawID, err := db.UpsertSessionSourceWithEvents(&storage.SessionSource{
		Source: "openclaw", SessionID: "events-only", Path: openclawPath, ParserVersion: "v1",
		HeadHash: indexedHeadHash([]byte("event")),
		FileSize: 5, IndexedOffset: 5, CoverageStatus: "complete", SourceStatus: "available",
	}, []storage.SessionEventRecord{
		{Source: "openclaw", SessionID: "events-only", EventType: "user_message", Timestamp: day.Add(5 * time.Minute), Role: "user", Content: "Event only title", RawOffset: 0, RawLength: 5, RawIndex: 0},
	})
	if err != nil {
		t.Fatalf("UpsertSessionSourceWithEvents(openclaw): %v", err)
	}

	eventIDs := make(map[string]int64)
	for _, identity := range []struct{ source, session string }{{"claude", "shared"}, {"codex", "shared"}, {"openclaw", "events-only"}} {
		events, err := db.ListSessionEvents(identity.source, identity.session, 100, 0)
		if err != nil {
			t.Fatalf("ListSessionEvents(%s): %v", identity.source, err)
		}
		for _, event := range events {
			key := identity.source + ":" + event.EventType
			eventIDs[key] = event.ID
		}
	}

	return &sessionAPIFixture{
		db: db, handler: New(db, "127.0.0.1:0").Handler(), rawPath: rawPath,
		rawBytes: rawBytes, eventIDs: eventIDs,
		sourceIDs: map[string]int64{"claude": claudeID, "codex": codexID, "openclaw": openclawID}, day: day,
	}
}

func indexedHeadHash(content []byte) string {
	if len(content) > 4096 {
		content = content[:4096]
	}
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum)
}

func requestJSON(t *testing.T, handler http.Handler, method, target string, response interface{}) int {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if response != nil && w.Body.Len() > 0 {
		if err := json.NewDecoder(w.Body).Decode(response); err != nil {
			t.Fatalf("decode %s %s response (status %d, body %q): %v", method, target, w.Code, w.Body.String(), err)
		}
	}
	return w.Code
}

func dayRange(day time.Time) string {
	return "from=" + day.Format("2006-01-02") + "&to=" + day.Format("2006-01-02")
}

func TestSessionSearchIntersectsFiltersAndAggregates(t *testing.T) {
	fx := seedSessionAPI(t)
	var sessions []storage.SessionSummary
	target := "/api/sessions?" + dayRange(fx.day) + "&source=claude&model=priced-model&project=alpha"
	if status := requestJSON(t, fx.handler, http.MethodGet, target, &sessions); status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions = %d, want 1: %+v", len(sessions), sessions)
	}
	got := sessions[0]
	if got.Source != "claude" || got.SessionID != "shared" || got.Title != `Investigate "build" #42?` {
		t.Errorf("unexpected identity/title: %+v", got)
	}
	if len(got.Models) != 1 || got.Models[0] != "priced-model" {
		t.Errorf("models = %v, want [priced-model]", got.Models)
	}
	if got.InputTokens != 10 || got.OutputTokens != 2 || got.CacheRead != 3 || got.CacheCreate != 4 || got.TotalTokens != 19 || got.TotalCost != 1.25 {
		t.Errorf("model-filtered totals are wrong: %+v", got)
	}
	if got.Prompts != 2 || got.ToolCalls != 1 || got.Errors != 1 {
		t.Errorf("activity counts are wrong: %+v", got)
	}
	if got.UnknownPrice {
		t.Error("priced-only result reported unknown price")
	}
	if got.CoverageStatus != "partial" || got.SourceStatus != "missing_source" || got.MalformedLines != 2 {
		t.Errorf("source state not conservatively aggregated: %+v", got)
	}

	sessions = nil
	target = "/api/sessions?" + dayRange(fx.day) + "&source=claude&project=alpha"
	if status := requestJSON(t, fx.handler, http.MethodGet, target, &sessions); status != http.StatusOK {
		t.Fatalf("unfiltered model status = %d", status)
	}
	if len(sessions) != 1 || !sessions[0].UnknownPrice || sessions[0].InputTokens != 30 || sessions[0].TotalTokens != 44 {
		t.Fatalf("unknown-price aggregate mismatch: %+v", sessions)
	}

	sessions = nil
	if status := requestJSON(t, fx.handler, http.MethodGet, "/api/sessions?"+dayRange(fx.day)+"&source=openclaw", &sessions); status != http.StatusOK {
		t.Fatalf("event-only status = %d", status)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "events-only" || sessions[0].Title != "Event only title" {
		t.Fatalf("event-only session missing: %+v", sessions)
	}
}

func TestSessionSearchUsesLiteralFTSAcrossIndexedColumns(t *testing.T) {
	fx := seedSessionAPI(t)
	queries := []string{`Investigate "build" #42?`, "Read.Tool", "a.go", "needle-output"}
	for _, query := range queries {
		var sessions []storage.SessionSummary
		values := url.Values{"from": {"2025-01-10"}, "to": {"2025-01-10"}, "q": {query}}
		if status := requestJSON(t, fx.handler, http.MethodGet, "/api/sessions?"+values.Encode(), &sessions); status != http.StatusOK {
			t.Fatalf("q=%q status = %d", query, status)
		}
		if len(sessions) != 1 || sessions[0].Source != "claude" {
			t.Errorf("q=%q returned %+v", query, sessions)
		}
	}

	var sessions []storage.SessionSummary
	values := url.Values{"from": {"2025-01-10"}, "to": {"2025-01-10"}, "q": {"needle-output OR Codex"}}
	if status := requestJSON(t, fx.handler, http.MethodGet, "/api/sessions?"+values.Encode(), &sessions); status != http.StatusOK {
		t.Fatalf("operator-like literal status = %d", status)
	}
	if len(sessions) != 0 {
		t.Fatalf("raw FTS operator changed semantics: %+v", sessions)
	}
}

func TestSessionSearchNewestPaginationIsStable(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2025, 1, 10, 12, 0, 0, 0, time.UTC)
	for _, record := range []*storage.UsageRecord{
		{Source: "claude", SessionID: "a", Model: "m", Timestamp: ts, InputTokens: 1},
		{Source: "codex", SessionID: "b", Model: "m", Timestamp: ts, InputTokens: 1},
		{Source: "claude", SessionID: "newest", Model: "m", Timestamp: ts.Add(time.Hour), InputTokens: 1},
	} {
		if err := db.InsertUsage(record); err != nil {
			t.Fatalf("InsertUsage: %v", err)
		}
	}
	handler := New(db, "127.0.0.1:0").Handler()
	want := []struct{ source, session string }{{"claude", "newest"}, {"claude", "a"}, {"codex", "b"}}
	for offset, expected := range want {
		var sessions []storage.SessionSummary
		target := fmt.Sprintf("/api/sessions?from=2025-01-10&to=2025-01-10&limit=1&offset=%d", offset)
		if status := requestJSON(t, handler, http.MethodGet, target, &sessions); status != http.StatusOK {
			t.Fatalf("offset %d status = %d", offset, status)
		}
		if len(sessions) != 1 || sessions[0].Source != expected.source || sessions[0].SessionID != expected.session {
			t.Errorf("offset %d = %+v, want %s/%s", offset, sessions, expected.source, expected.session)
		}
	}
}

func TestSessionSearchRejectsInvalidInputAndMethods(t *testing.T) {
	fx := seedSessionAPI(t)
	invalid := []string{
		"from=bad", "from=2025-01-11&to=2025-01-10", "limit=0", "limit=-1",
		"limit=abc", fmt.Sprintf("limit=%d", searchMaxLimit+1), "offset=-1", "offset=abc",
	}
	for _, query := range invalid {
		var response map[string]string
		if status := requestJSON(t, fx.handler, http.MethodGet, "/api/sessions?"+query, &response); status != http.StatusBadRequest {
			t.Errorf("query %q status = %d, want 400", query, status)
		}
	}
	if status := requestJSON(t, fx.handler, http.MethodPost, "/api/sessions", nil); status != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/sessions status = %d, want 405", status)
	}
}

func TestSessionEventsAreChronologicalPagedAndSourceBound(t *testing.T) {
	fx := seedSessionAPI(t)
	var events []SessionEventResponse
	if status := requestJSON(t, fx.handler, http.MethodGet, "/api/sessions/claude/shared/events?limit=2&offset=1", &events); status != http.StatusOK {
		t.Fatalf("events status = %d", status)
	}
	if len(events) != 2 || events[0].EventType != "tool_call" || events[1].EventType != "tool_result" {
		t.Fatalf("unexpected chronological page: %+v", events)
	}
	if !events[0].HasRaw || events[1].HasRaw {
		t.Errorf("raw availability is wrong: %+v", events)
	}

	events = nil
	if status := requestJSON(t, fx.handler, http.MethodGet, "/api/sessions/codex/shared/events", &events); status != http.StatusOK {
		t.Fatalf("codex events status = %d", status)
	}
	if len(events) != 1 || events[0].Content != "Codex title" {
		t.Fatalf("source collision leaked events: %+v", events)
	}
	if status := requestJSON(t, fx.handler, http.MethodGet, "/api/sessions/claude/unknown/events", nil); status != http.StatusNotFound {
		t.Errorf("unknown session status = %d, want 404", status)
	}
}

func TestSessionEventResponseJSONContract(t *testing.T) {
	db := tempDB(t)
	timestamp := time.Date(2025, 7, 24, 1, 10, 11, 123456789, time.UTC)
	_, err := db.UpsertSessionSourceWithEvents(&storage.SessionSource{
		Source: "claude", SessionID: "response-contract", Path: "/sessions/response-contract.jsonl",
		ParserVersion: "v1", CoverageStatus: "complete", SourceStatus: "available",
	}, []storage.SessionEventRecord{{
		Source: "claude", SessionID: "response-contract", EventType: "user_message",
		Timestamp: timestamp, Role: "user", Content: "contract event",
	}})
	if err != nil {
		t.Fatalf("seed response contract event: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/claude/response-contract/events", nil)
	w := httptest.NewRecorder()
	New(db, "127.0.0.1:0").Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var events []map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&events); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1: %+v", len(events), events)
	}
	gotTimestamp, ok := events[0]["timestamp"].(string)
	if !ok {
		t.Fatalf("timestamp = %T(%v), want JSON string", events[0]["timestamp"], events[0]["timestamp"])
	}
	if want := timestamp.UTC().Format(time.RFC3339Nano); gotTimestamp != want {
		t.Errorf("timestamp = %q, want %q", gotTimestamp, want)
	}
	if duration, exists := events[0]["duration_ms"]; !exists || duration != nil {
		t.Errorf("duration_ms = %v (exists %t), want explicit JSON null", duration, exists)
	}
}

func TestSessionEventsRejectInvalidInputAndMethods(t *testing.T) {
	fx := seedSessionAPI(t)
	base := "/api/sessions/claude/shared/events"
	for _, query := range []string{"limit=0", fmt.Sprintf("limit=%d", eventsMaxLimit+1), "limit=bad", "offset=-1", "offset=bad"} {
		var response map[string]string
		if status := requestJSON(t, fx.handler, http.MethodGet, base+"?"+query, &response); status != http.StatusBadRequest {
			t.Errorf("query %q status = %d, want 400", query, status)
		}
	}
	if status := requestJSON(t, fx.handler, http.MethodPost, base, nil); status != http.StatusMethodNotAllowed {
		t.Errorf("POST events status = %d, want 405", status)
	}
}

func TestSessionRawReturnsExactJSONAndTextRecords(t *testing.T) {
	fx := seedSessionAPI(t)
	cases := []struct {
		key, content, contentType string
	}{
		{"claude:user_message", `{"kind":"json","value":42}`, "json"},
		{"claude:tool_call", "plain text record", "text"},
	}
	for _, tc := range cases {
		var raw RawEventResponse
		target := fmt.Sprintf("/api/sessions/claude/shared/events/%d/raw", fx.eventIDs[tc.key])
		if status := requestJSON(t, fx.handler, http.MethodGet, target, &raw); status != http.StatusOK {
			t.Fatalf("%s status = %d", tc.key, status)
		}
		if raw.Content != tc.content || raw.ContentType != tc.contentType || raw.Path != fx.rawPath {
			t.Errorf("%s raw response = %+v", tc.key, raw)
		}
		if raw.Length != int64(len(tc.content)) || string(fx.rawBytes[raw.Offset:raw.Offset+raw.Length]) != tc.content {
			t.Errorf("%s locator is not exact: %+v", tc.key, raw)
		}
	}
}

func TestSessionRawRejectsRewrittenIndexedSnapshot(t *testing.T) {
	fx := seedSessionAPI(t)
	rewritten := append([]byte(nil), fx.rawBytes...)
	rewritten[0] ^= 0xff
	if err := os.WriteFile(fx.rawPath, rewritten, 0o600); err != nil {
		t.Fatalf("rewrite source: %v", err)
	}
	target := fmt.Sprintf("/api/sessions/claude/shared/events/%d/raw", fx.eventIDs["claude:user_message"])
	if status := requestJSON(t, fx.handler, http.MethodGet, target, nil); status != http.StatusGone {
		t.Errorf("rewritten snapshot status = %d, want 410", status)
	}
}

func TestSessionRawAllowsAppendOnlyGrowth(t *testing.T) {
	fx := seedSessionAPI(t)
	file, err := os.OpenFile(fx.rawPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open source for append: %v", err)
	}
	if _, err := file.WriteString("appended record\n"); err != nil {
		file.Close()
		t.Fatalf("append source: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close appended source: %v", err)
	}

	var raw RawEventResponse
	target := fmt.Sprintf("/api/sessions/claude/shared/events/%d/raw", fx.eventIDs["claude:user_message"])
	if status := requestJSON(t, fx.handler, http.MethodGet, target, &raw); status != http.StatusOK {
		t.Fatalf("appended snapshot status = %d, want 200", status)
	}
	if raw.Content != `{"kind":"json","value":42}` {
		t.Errorf("raw content after append = %q", raw.Content)
	}
}

func TestSessionRawRejectsUnsafeOrUnavailableLocators(t *testing.T) {
	fx := seedSessionAPI(t)
	claudeEventID := fx.eventIDs["claude:user_message"]
	if status := requestJSON(t, fx.handler, http.MethodGet, fmt.Sprintf("/api/sessions/codex/shared/events/%d/raw", claudeEventID), nil); status != http.StatusNotFound {
		t.Errorf("source-mismatched event status = %d, want 404", status)
	}
	if status := requestJSON(t, fx.handler, http.MethodGet, "/api/sessions/claude/shared/events/999999/raw", nil); status != http.StatusNotFound {
		t.Errorf("unknown event status = %d, want 404", status)
	}

	missingSource := &storage.SessionSource{Source: "claude", SessionID: "missing-file", Path: filepath.Join(t.TempDir(), "gone.jsonl"), ParserVersion: "v1", SourceStatus: "available"}
	missingID, err := fx.db.UpsertSessionSourceWithEvents(missingSource, []storage.SessionEventRecord{{
		Source: "claude", SessionID: "missing-file", EventType: "user_message", Timestamp: fx.day, RawOffset: 0, RawLength: 5,
	}})
	if err != nil {
		t.Fatalf("seed missing file: %v", err)
	}
	missingEvents, _ := fx.db.ListSessionEvents("claude", "missing-file", 10, 0)
	if len(missingEvents) != 1 || missingID == 0 {
		t.Fatalf("missing-file event not seeded: %+v", missingEvents)
	}
	if status := requestJSON(t, fx.handler, http.MethodGet, fmt.Sprintf("/api/sessions/claude/missing-file/events/%d/raw", missingEvents[0].ID), nil); status != http.StatusGone {
		t.Errorf("missing file status = %d, want 410", status)
	}

	for name, locator := range map[string]struct {
		offset int64
		length int64
		status int
	}{
		"invalid":   {-1, 5, http.StatusBadRequest},
		"oversized": {300, rawRecordMaxBytes + 1, http.StatusRequestEntityTooLarge},
	} {
		if err := fx.db.InsertSessionEvents([]storage.SessionEventRecord{{
			SessionSourceID: fx.sourceIDs["claude"], Source: "claude", SessionID: "shared",
			EventType: "system", Timestamp: fx.day, RawOffset: locator.offset, RawLength: locator.length, RawIndex: 9,
		}}); err != nil {
			t.Fatalf("seed %s locator: %v", name, err)
		}
		events, _ := fx.db.ListSessionEvents("claude", "shared", 100, 0)
		var eventID int64
		for _, event := range events {
			if event.RawOffset == locator.offset && event.RawLength == locator.length {
				eventID = event.ID
			}
		}
		if eventID == 0 {
			t.Fatalf("%s locator event not found", name)
		}
		target := fmt.Sprintf("/api/sessions/claude/shared/events/%d/raw", eventID)
		if status := requestJSON(t, fx.handler, http.MethodGet, target, nil); status != locator.status {
			t.Errorf("%s locator status = %d, want %d", name, status, locator.status)
		}
	}

	target := fmt.Sprintf("/api/sessions/claude/shared/events/%d/raw", claudeEventID)
	if status := requestJSON(t, fx.handler, http.MethodPost, target, nil); status != http.StatusMethodNotAllowed {
		t.Errorf("POST raw status = %d, want 405", status)
	}
}

func TestSessionIndexRebuildPreservesHistoryAndMarksSources(t *testing.T) {
	fx := seedSessionAPI(t)
	if err := fx.db.SetFileState(fx.rawPath, int64(len(fx.rawBytes)), int64(len(fx.rawBytes)), nil); err != nil {
		t.Fatalf("SetFileState: %v", err)
	}
	from := fx.day.Add(-time.Hour)
	to := fx.day.Add(time.Hour)
	before, err := fx.db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats(before): %v", err)
	}
	var response struct {
		Status  string `json:"status"`
		Sources int64  `json:"sources"`
	}
	if status := requestJSON(t, fx.handler, http.MethodPost, "/api/session-index/rebuild", &response); status != http.StatusOK {
		t.Fatalf("rebuild status = %d", status)
	}
	if response.Status != "rebuild_required" || response.Sources < 3 {
		t.Errorf("unexpected rebuild response: %+v", response)
	}
	after, err := fx.db.GetDashboardStats(from, to, "")
	if err != nil {
		t.Fatalf("GetDashboardStats(after): %v", err)
	}
	if *after != *before {
		t.Errorf("usage/prompt/session stats changed: before=%+v after=%+v", before, after)
	}
	for _, path := range []string{fx.rawPath} {
		source, err := fx.db.GetSessionSourceByPath(path)
		if err != nil {
			t.Fatalf("GetSessionSourceByPath: %v", err)
		}
		if source == nil || source.SourceStatus != "rebuild_required" || source.CoverageStatus != "partial" || source.IndexedOffset != 0 || source.MalformedLines != 0 || source.LastError != "" {
			t.Errorf("source not marked for rebuild: %+v", source)
		}
	}
	if events, err := fx.db.ListSessionEvents("claude", "shared", 10, 0); err != nil || len(events) != 0 {
		t.Errorf("normalized events remain: events=%+v err=%v", events, err)
	}
	var sessions []storage.SessionSummary
	values := url.Values{"from": {"2025-01-10"}, "to": {"2025-01-10"}, "q": {"needle-output"}}
	if status := requestJSON(t, fx.handler, http.MethodGet, "/api/sessions?"+values.Encode(), &sessions); status != http.StatusOK || len(sessions) != 0 {
		t.Errorf("FTS content remains after rebuild: status=%d sessions=%+v", status, sessions)
	}
	size, offset, _, err := fx.db.GetFileState(fx.rawPath)
	if err != nil || size != int64(len(fx.rawBytes)) || offset != int64(len(fx.rawBytes)) {
		t.Errorf("file_state changed: size=%d offset=%d err=%v", size, offset, err)
	}
	if _, err := os.Stat(fx.rawPath); err != nil {
		t.Errorf("agent source file changed: %v", err)
	}
	if status := requestJSON(t, fx.handler, http.MethodGet, "/api/session-index/rebuild", nil); status != http.StatusMethodNotAllowed {
		t.Errorf("GET rebuild status = %d, want 405", status)
	}
}

func TestSessionSearchDefaultsLimit(t *testing.T) {
	if searchDefaultLimit <= 0 || searchDefaultLimit > searchMaxLimit {
		t.Fatalf("invalid test contract for default limit")
	}
}

func TestSessionRawEventIDMustBeInteger(t *testing.T) {
	fx := seedSessionAPI(t)
	if status := requestJSON(t, fx.handler, http.MethodGet, "/api/sessions/claude/shared/events/not-a-number/raw", nil); status != http.StatusBadRequest {
		t.Errorf("non-integer event id status = %d, want 400", status)
	}
	if _, err := strconv.ParseInt("not-a-number", 10, 64); err == nil {
		t.Fatal("invalid test setup")
	}
}
