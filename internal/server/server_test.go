package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/configmanager"
	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func tempDB(t *testing.T) *storage.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestHealthEndpoint(t *testing.T) {
	db := tempDB(t)
	srv := New(db, nil, "127.0.0.1:0")
	handler := srv.Handler()

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", resp["status"])
	}
}

func TestCORSHeaders(t *testing.T) {
	db := tempDB(t)
	srv := New(db, nil, "127.0.0.1:0")
	handler := srv.Handler()

	req := httptest.NewRequest("OPTIONS", "/api/health", nil)
	req.Header.Set("Origin", "tauri://localhost")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "tauri://localhost" {
		t.Fatalf("missing CORS header, got %q", w.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSRejectsDisallowedBrowserOrigin(t *testing.T) {
	db := tempDB(t)
	srv := New(db, nil, "127.0.0.1:0")
	handler := srv.Handler()

	req := httptest.NewRequest("OPTIONS", "/api/config/profiles", nil)
	req.Header.Set("Origin", "https://evil.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestRequestWithoutOriginStillWorks(t *testing.T) {
	db := tempDB(t)
	srv := New(db, nil, "127.0.0.1:0")
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty for no-origin request", got)
	}
}

func tempManager(t *testing.T, db *storage.DB) *configmanager.Manager {
	t.Helper()
	return configmanager.NewManager(
		db,
		t.TempDir(),
		configmanager.WithEncryptionKey([]byte("12345678901234567890123456789012")),
	)
}

func TestCreateAndListProfiles(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	body := bytes.NewBufferString(`{"name":"Work","config":"{\"api_key\":\"sk-test\",\"base_url\":\"https://example.com\",\"model\":\"gpt-test\"}","tool_targets":{"codex":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/profiles", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("created id = 0")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/config/profiles", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body.String())
	}
	var profiles []storage.Profile
	if err := json.NewDecoder(w.Body).Decode(&profiles); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(profiles) != 1 || profiles[0].Name != "Work" {
		t.Fatalf("profiles = %+v, want one Work profile", profiles)
	}
}

func TestProfileMutationsReturnAffectedFilesField(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	createBody := bytes.NewBufferString(`{"name":"Work","config":"{\"api_key\":\"sk-test\",\"base_url\":\"https://example.com\",\"model\":\"gpt-test\"}","tool_targets":{"codex":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/profiles", createBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}

	var created struct {
		ID            int64                        `json:"id"`
		AffectedFiles []configmanager.AffectedFile `json:"affected_files"`
	}
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("created id = 0")
	}
	if created.AffectedFiles == nil {
		t.Fatalf("create affected_files = nil, want empty array")
	}

	updateBody := bytes.NewBufferString(`{"name":"Work Updated","config":"{\"api_key\":\"sk-test\",\"base_url\":\"https://example.org\",\"model\":\"gpt-next\"}","tool_targets":{"codex":true}}`)
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/config/profiles/%d", created.ID), updateBody)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}

	var updated struct {
		AffectedFiles []configmanager.AffectedFile `json:"affected_files"`
	}
	if err := json.NewDecoder(w.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updated.AffectedFiles == nil {
		t.Fatalf("update affected_files = nil, want empty array")
	}

	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/config/profiles/%d", created.ID), nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", w.Code, w.Body.String())
	}

	var deleted struct {
		AffectedFiles []configmanager.AffectedFile `json:"affected_files"`
	}
	if err := json.NewDecoder(w.Body).Decode(&deleted); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if deleted.AffectedFiles == nil {
		t.Fatalf("delete affected_files = nil, want empty array")
	}
}

func TestListProfilesRedactsAPIKeyMaterial(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	body := bytes.NewBufferString(`{"name":"Secret","config":"{\"api_key\":\"sk-test\",\"base_url\":\"https://example.com\",\"model\":\"gpt-test\"}","tool_targets":{"codex":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/profiles", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/config/profiles", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body.String())
	}
	responseBody := w.Body.String()
	if strings.Contains(responseBody, "sk-test") || strings.Contains(responseBody, "enc:") {
		t.Fatalf("list response leaked API key material: %s", responseBody)
	}

	var profiles []map[string]any
	if err := json.Unmarshal([]byte(responseBody), &profiles); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(profiles))
	}
	if profiles[0]["has_api_key"] != true {
		t.Fatalf("has_api_key = %v, want true", profiles[0]["has_api_key"])
	}
	configText, ok := profiles[0]["config"].(string)
	if !ok {
		t.Fatalf("config type = %T, want string", profiles[0]["config"])
	}
	if !strings.Contains(configText, `"base_url":"https://example.com"`) {
		t.Fatalf("config = %s, want non-secret fields preserved", configText)
	}
	if !strings.Contains(fmt.Sprint(profiles[0]["tool_targets"]), "codex") {
		t.Fatalf("tool_targets = %v, want codex target", profiles[0]["tool_targets"])
	}
}

func TestUpdateProfilePreservesExistingAPIKeyAndPersistsToolTargets(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	createBody := bytes.NewBufferString(`{"name":"Work","config":"{\"api_key\":\"sk-test\",\"base_url\":\"https://example.com\",\"model\":\"gpt-test\"}","tool_targets":{"codex":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/profiles", createBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}

	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	before, err := db.GetProfile(created.ID)
	if err != nil {
		t.Fatalf("GetProfile before update: %v", err)
	}
	if before == nil {
		t.Fatal("profile missing before update")
	}
	var beforeConfig configmanager.ProviderConfig
	if err := json.Unmarshal([]byte(before.Config), &beforeConfig); err != nil {
		t.Fatalf("decode stored config before update: %v", err)
	}
	if beforeConfig.APIKey == "" {
		t.Fatal("stored API key missing before update")
	}

	updateBody := bytes.NewBufferString(`{"name":"Work Updated","config":"{\"base_url\":\"https://example.org\",\"model\":\"gpt-next\"}","tool_targets":{"claude":true,"openclaw":true}}`)
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/config/profiles/%d", created.ID), updateBody)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}

	after, err := db.GetProfile(created.ID)
	if err != nil {
		t.Fatalf("GetProfile after update: %v", err)
	}
	if after == nil {
		t.Fatal("profile missing after update")
	}
	var afterConfig configmanager.ProviderConfig
	if err := json.Unmarshal([]byte(after.Config), &afterConfig); err != nil {
		t.Fatalf("decode stored config after update: %v", err)
	}
	if afterConfig.APIKey != beforeConfig.APIKey {
		t.Fatalf("stored API key changed: before %q after %q", beforeConfig.APIKey, afterConfig.APIKey)
	}
	if afterConfig.BaseURL != "https://example.org" {
		t.Fatalf("base_url = %q, want https://example.org", afterConfig.BaseURL)
	}
	if afterConfig.Model != "gpt-next" {
		t.Fatalf("model = %q, want gpt-next", afterConfig.Model)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/config/profiles", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body.String())
	}

	var profiles []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&profiles); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(profiles))
	}
	if profiles[0]["name"] != "Work Updated" {
		t.Fatalf("name = %v, want Work Updated", profiles[0]["name"])
	}

	targets, ok := profiles[0]["tool_targets"].(map[string]any)
	if !ok {
		t.Fatalf("tool_targets type = %T, want map[string]any", profiles[0]["tool_targets"])
	}
	if got := targets["claude"]; got != true {
		t.Fatalf("tool_targets[claude] = %v, want true", got)
	}
	if got := targets["openclaw"]; got != true {
		t.Fatalf("tool_targets[openclaw] = %v, want true", got)
	}
	if _, exists := targets["codex"]; exists {
		t.Fatalf("tool_targets[codex] unexpectedly present: %v", targets["codex"])
	}
}

func TestUpdateProfileOmittedToolTargetsPreservesExistingTargets(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	createBody := bytes.NewBufferString(`{"name":"Work","config":"{\"api_key\":\"sk-test\",\"base_url\":\"https://example.com\",\"model\":\"gpt-test\"}","tool_targets":{"codex":true,"claude":false}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/profiles", createBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}

	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	updateBody := bytes.NewBufferString(`{"name":"Work Renamed","config":"{\"base_url\":\"https://example.org\",\"model\":\"gpt-next\"}"}`)
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/config/profiles/%d", created.ID), updateBody)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/config/profiles", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body.String())
	}

	var profiles []map[string]any
	if err := json.NewDecoder(w.Body).Decode(&profiles); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(profiles))
	}
	targets, ok := profiles[0]["tool_targets"].(map[string]any)
	if !ok {
		t.Fatalf("tool_targets type = %T, want map[string]any", profiles[0]["tool_targets"])
	}
	if got := targets["codex"]; got != true {
		t.Fatalf("tool_targets[codex] = %v, want true", got)
	}
	if got := targets["claude"]; got != false {
		t.Fatalf("tool_targets[claude] = %v, want false", got)
	}
}

func TestCreateProfileRequiresAPIKey(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	body := bytes.NewBufferString(`{"name":"Missing Key","config":"{\"base_url\":\"https://example.com\"}","tool_targets":{"codex":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/profiles", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "api_key") {
		t.Fatalf("body = %s, want api_key error", w.Body.String())
	}
}

func TestCreateProfileRejectsTrailingJSON(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	body := bytes.NewBufferString(`{"name":"Trailing","config":"{\"api_key\":\"sk-test\"}","tool_targets":{"codex":true}} {}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/profiles", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
}

func TestCreateProfileDuplicateReturnsConflict(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	body := `{"name":"Dup","config":"{\"api_key\":\"sk-test\"}","tool_targets":{"codex":true}}`
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/config/profiles", bytes.NewBufferString(body))
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if i == 0 && w.Code != http.StatusOK {
			t.Fatalf("first create status = %d, body = %s", w.Code, w.Body.String())
		}
		if i == 1 && w.Code != http.StatusConflict {
			t.Fatalf("duplicate create status = %d, want 409; body = %s", w.Code, w.Body.String())
		}
	}
}

func TestActivateProfileNotFound(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	req := httptest.NewRequest(http.MethodPost, "/api/config/profiles/999/activate", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}

func TestActivateProfileConflictReturnsConflict(t *testing.T) {
	db := tempDB(t)
	claudeDir := t.TempDir()
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile settings: %v", err)
	}

	mgr := configmanager.NewManager(
		db,
		t.TempDir(),
		configmanager.WithClaudeAdapter(claudeDir, filepath.Join(claudeDir, ".claude.json")),
		configmanager.WithEncryptionKey([]byte("12345678901234567890123456789012")),
	)

	oldProfileID, err := mgr.CreateProfile(
		"old",
		`{"api_key":"sk-old","base_url":"https://old.example.com","model":"old-model"}`,
		map[string]bool{"claude": true},
	)
	if err != nil {
		t.Fatalf("CreateProfile old: %v", err)
	}
	if _, err := mgr.ActivateProfile(oldProfileID); err != nil {
		t.Fatalf("ActivateProfile old: %v", err)
	}

	newProfileID, err := mgr.CreateProfile(
		"new",
		`{"api_key":"sk-new","base_url":"https://new.example.com","model":"new-model"}`,
		map[string]bool{"claude": true},
	)
	if err != nil {
		t.Fatalf("CreateProfile new: %v", err)
	}

	if err := os.WriteFile(settingsPath, []byte(`{"env":{"ANTHROPIC_API_KEY":"sk-external"}}`), 0o644); err != nil {
		t.Fatalf("WriteFile external settings: %v", err)
	}

	srv := New(db, mgr, "127.0.0.1:0")
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/config/profiles/%d/activate", newProfileID), nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", w.Code, w.Body.String())
	}
}

func TestCreateListAndSetMCPServerTargets(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	body := bytes.NewBufferString(`{"name":"fs","command":"npx","args":"[\"-y\",\"@modelcontextprotocol/server-filesystem\"]","env":"{\"ROOT\":\"/tmp\"}","targets":{"codex":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/mcp", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var created struct {
		ID            int64                        `json:"id"`
		AffectedFiles []configmanager.AffectedFile `json:"affected_files"`
	}
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("created id = 0")
	}

	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/config/mcp/%d/targets", created.ID), bytes.NewBufferString(`{"targets":{"claude":true,"codex":false}}`))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("set targets status = %d, body = %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/config/mcp", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body.String())
	}
	var servers []struct {
		ID      int64           `json:"id"`
		Name    string          `json:"name"`
		Command string          `json:"command"`
		Targets map[string]bool `json:"targets"`
	}
	if err := json.NewDecoder(w.Body).Decode(&servers); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "fs" || servers[0].Command != "npx" {
		t.Fatalf("servers = %+v, want one fs server", servers)
	}
	if servers[0].Targets["claude"] != true || servers[0].Targets["codex"] != false {
		t.Fatalf("targets = %+v, want updated claude/codex targets", servers[0].Targets)
	}
}

func TestCreateMCPServerCanBeDisabled(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	body := bytes.NewBufferString(`{"name":"disabled","command":"npx","args":"[]","env":"{}","enabled":false,"targets":{"codex":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/mcp", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/config/mcp", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body.String())
	}

	var servers []struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(w.Body).Decode(&servers); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "disabled" {
		t.Fatalf("servers = %+v, want one disabled server", servers)
	}
	if servers[0].Enabled {
		t.Fatalf("enabled = true, want false")
	}
}

func TestUpdateMCPServerPersistsTargetsAtomically(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	createBody := bytes.NewBufferString(`{"name":"fs","command":"npx","args":"[]","env":"{}","targets":{"codex":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/mcp", createBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}

	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	updateBody := bytes.NewBufferString(`{"name":"fs-updated","command":"uvx","args":"[\"server\"]","env":"{\"ROOT\":\"/tmp/project\"}","enabled":true,"targets":{"claude":true,"codex":false}}`)
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/config/mcp/%d", created.ID), updateBody)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/config/mcp", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body.String())
	}

	var servers []struct {
		Name    string          `json:"name"`
		Command string          `json:"command"`
		Targets map[string]bool `json:"targets"`
	}
	if err := json.NewDecoder(w.Body).Decode(&servers); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "fs-updated" || servers[0].Command != "uvx" {
		t.Fatalf("servers = %+v, want updated server", servers)
	}
	if servers[0].Targets["claude"] != true || servers[0].Targets["codex"] != false {
		t.Fatalf("targets = %+v, want updated claude/codex targets", servers[0].Targets)
	}
}

func TestCreateMCPServerValidatesJSONStrings(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	body := bytes.NewBufferString(`{"name":"bad","command":"cmd","args":"{\"not\":\"array\"}","env":"{}"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/mcp", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "args") {
		t.Fatalf("body = %s, want args validation error", w.Body.String())
	}
}

func TestCreateMCPServerRejectsNonStringEnvValues(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	body := bytes.NewBufferString(`{"name":"bad-env","command":"cmd","args":"[]","env":"{\"ROOT\":1}"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/mcp", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "env") {
		t.Fatalf("body = %s, want env validation error", w.Body.String())
	}
}

func TestCreateMCPServerRollsBackWhenSyncFails(t *testing.T) {
	db := tempDB(t)
	mgr := configmanager.NewManager(
		db,
		t.TempDir(),
		configmanager.WithEncryptionKey([]byte("12345678901234567890123456789012")),
		configmanager.WithAdapter(&failingServerTestAdapter{
			tool:      "codex",
			mcpErr:    errors.New("sync boom"),
			installed: true,
		}),
	)
	srv := New(db, mgr, "127.0.0.1:0")
	handler := srv.Handler()

	body := bytes.NewBufferString(`{"name":"fs","command":"npx","args":"[\"-y\"]","env":"{\"ROOT\":\"/tmp\"}","targets":{"codex":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/mcp", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", w.Code, w.Body.String())
	}

	servers, err := db.ListMCPServers()
	if err != nil {
		t.Fatalf("ListMCPServers: %v", err)
	}
	if len(servers) != 0 {
		t.Fatalf("len(servers) = %d, want 0 after rollback", len(servers))
	}
}

func TestDeleteMCPServerRollbackPreservesOriginalID(t *testing.T) {
	db := tempDB(t)
	adapter := &failingServerTestAdapter{
		tool:      "codex",
		installed: true,
	}
	mgr := configmanager.NewManager(
		db,
		t.TempDir(),
		configmanager.WithEncryptionKey([]byte("12345678901234567890123456789012")),
		configmanager.WithAdapter(adapter),
	)
	srv := New(db, mgr, "127.0.0.1:0")
	handler := srv.Handler()

	serverID, err := db.CreateMCPServer("delete-me", "npx", `["-y"]`, `{"ROOT":"/tmp"}`)
	if err != nil {
		t.Fatalf("CreateMCPServer: %v", err)
	}
	if err := db.SetMCPServerTargets(serverID, map[string]bool{"codex": true}); err != nil {
		t.Fatalf("SetMCPServerTargets: %v", err)
	}

	adapter.mcpErr = errors.New("sync boom")

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/config/mcp/%d", serverID), nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", w.Code, w.Body.String())
	}

	stored, err := db.GetMCPServer(serverID)
	if err != nil {
		t.Fatalf("GetMCPServer: %v", err)
	}
	if stored == nil {
		t.Fatalf("GetMCPServer(%d) = nil, want restored row", serverID)
	}
	if stored.ID != serverID {
		t.Fatalf("stored.ID = %d, want %d", stored.ID, serverID)
	}

	targets, err := db.GetMCPServerTargets(serverID)
	if err != nil {
		t.Fatalf("GetMCPServerTargets: %v", err)
	}
	if !targets["codex"] {
		t.Fatalf("targets = %+v, want codex=true", targets)
	}
}

func TestUpdateMCPServerRollbackRestoresBaseFieldsAndTargets(t *testing.T) {
	db := tempDB(t)
	adapter := &failingServerTestAdapter{
		tool:      "codex",
		installed: true,
		mcpErr:    errors.New("sync boom"),
	}
	mgr := configmanager.NewManager(
		db,
		t.TempDir(),
		configmanager.WithEncryptionKey([]byte("12345678901234567890123456789012")),
		configmanager.WithAdapter(adapter),
	)
	srv := New(db, mgr, "127.0.0.1:0")
	handler := srv.Handler()

	serverID, err := db.CreateMCPServer("original", "npx", `["-y"]`, `{"ROOT":"/tmp"}`)
	if err != nil {
		t.Fatalf("CreateMCPServer: %v", err)
	}
	if err := db.SetMCPServerTargets(serverID, map[string]bool{"codex": true, "claude": false}); err != nil {
		t.Fatalf("SetMCPServerTargets: %v", err)
	}

	updateBody := bytes.NewBufferString(`{"name":"updated","command":"uvx","args":"[\"server\"]","env":"{\"ROOT\":\"/srv\"}","enabled":false,"targets":{"claude":true,"codex":false}}`)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/config/mcp/%d", serverID), updateBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", w.Code, w.Body.String())
	}

	stored, err := db.GetMCPServer(serverID)
	if err != nil {
		t.Fatalf("GetMCPServer: %v", err)
	}
	if stored == nil {
		t.Fatalf("GetMCPServer(%d) = nil, want restored row", serverID)
	}
	if stored.Name != "original" || stored.Command != "npx" || stored.Args != `["-y"]` || stored.Env != `{"ROOT":"/tmp"}` || !stored.Enabled {
		t.Fatalf("stored = %+v, want original fields restored", stored)
	}

	targets, err := db.GetMCPServerTargets(serverID)
	if err != nil {
		t.Fatalf("GetMCPServerTargets: %v", err)
	}
	if targets["codex"] != true || targets["claude"] != false {
		t.Fatalf("targets = %+v, want original codex/claude targets", targets)
	}
}

func TestCreateListAndSetSkillTargets(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()
	skillDir := t.TempDir()

	body := bytes.NewBufferString(fmt.Sprintf(`{"name":"planner","source_path":%q,"description":"Plan helper","targets":{"codex":{"enabled":true}}}`, skillDir))
	req := httptest.NewRequest(http.MethodPost, "/api/config/skills", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("created id = 0")
	}

	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/config/skills/%d/targets", created.ID), bytes.NewBufferString(`{"targets":{"claude":{"method":"copy","enabled":true},"codex":{"method":"symlink","enabled":false}}}`))
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("set targets status = %d, body = %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/config/skills", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body.String())
	}
	var skills []struct {
		ID      int64  `json:"id"`
		Name    string `json:"name"`
		Targets map[string]struct {
			Method  string `json:"method"`
			Enabled bool   `json:"enabled"`
		} `json:"targets"`
	}
	if err := json.NewDecoder(w.Body).Decode(&skills); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(skills) != 1 || skills[0].Name != "planner" {
		t.Fatalf("skills = %+v, want one planner skill", skills)
	}
	if skills[0].Targets["claude"].Method != "copy" || !skills[0].Targets["claude"].Enabled {
		t.Fatalf("targets = %+v, want claude copy enabled", skills[0].Targets)
	}
}

func TestUpdateSkillCanPersistTargets(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()
	originalDir := t.TempDir()
	updatedDir := t.TempDir()

	createBody := bytes.NewBufferString(fmt.Sprintf(`{"name":"planner","source_path":%q,"description":"Plan helper","targets":{"codex":{"method":"symlink","enabled":true}}}`, originalDir))
	req := httptest.NewRequest(http.MethodPost, "/api/config/skills", createBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(w.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	updateBody := bytes.NewBufferString(fmt.Sprintf(`{"name":"planner-updated","source_path":%q,"description":"Updated helper","enabled":false,"targets":{"claude":{"method":"copy","enabled":true},"codex":{"method":"symlink","enabled":false}}}`, updatedDir))
	req = httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/config/skills/%d", created.ID), updateBody)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/config/skills", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body.String())
	}
	var skills []struct {
		Name        string `json:"name"`
		SourcePath  string `json:"source_path"`
		Description string `json:"description"`
		Enabled     bool   `json:"enabled"`
		Targets     map[string]struct {
			Method  string `json:"method"`
			Enabled bool   `json:"enabled"`
		} `json:"targets"`
	}
	if err := json.NewDecoder(w.Body).Decode(&skills); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("skills = %+v, want one skill", skills)
	}
	if skills[0].Name != "planner-updated" || skills[0].SourcePath != updatedDir || skills[0].Description != "Updated helper" || skills[0].Enabled {
		t.Fatalf("skill = %+v, want updated disabled skill", skills[0])
	}
	if skills[0].Targets["claude"].Method != "copy" || !skills[0].Targets["claude"].Enabled {
		t.Fatalf("targets = %+v, want claude copy enabled", skills[0].Targets)
	}
	if skills[0].Targets["codex"].Method != "symlink" || skills[0].Targets["codex"].Enabled {
		t.Fatalf("targets = %+v, want codex symlink disabled", skills[0].Targets)
	}
}

func TestUpdateSkillRollbackRestoresBaseFieldsAndTargets(t *testing.T) {
	db := tempDB(t)
	badSkillRoot := filepath.Join(t.TempDir(), "codex-skills-root")
	if err := os.WriteFile(badSkillRoot, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile badSkillRoot: %v", err)
	}
	adapter := &failingServerTestAdapter{
		tool:       "codex",
		installed:  true,
		skillPaths: []string{badSkillRoot},
	}
	mgr := configmanager.NewManager(
		db,
		t.TempDir(),
		configmanager.WithEncryptionKey([]byte("12345678901234567890123456789012")),
		configmanager.WithAdapter(adapter),
	)
	srv := New(db, mgr, "127.0.0.1:0")
	handler := srv.Handler()

	originalDir := t.TempDir()
	updatedDir := t.TempDir()
	skillID, err := db.CreateSkill("planner", originalDir, "Plan helper")
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}
	if err := db.SetSkillTargets(skillID, []storage.SkillTargetRecord{
		{Tool: "codex", Method: "symlink", Enabled: true},
		{Tool: "claude", Method: "copy", Enabled: false},
	}); err != nil {
		t.Fatalf("SetSkillTargets: %v", err)
	}

	updateBody := bytes.NewBufferString(fmt.Sprintf(`{"name":"planner-updated","source_path":%q,"description":"Updated helper","enabled":true,"targets":{"codex":{"method":"copy","enabled":true},"claude":{"method":"copy","enabled":true}}}`, updatedDir))
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/config/skills/%d", skillID), updateBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", w.Code, w.Body.String())
	}

	stored, err := db.GetSkill(skillID)
	if err != nil {
		t.Fatalf("GetSkill: %v", err)
	}
	if stored == nil {
		t.Fatalf("GetSkill(%d) = nil, want restored row", skillID)
	}
	if stored.Name != "planner" || stored.SourcePath != originalDir || stored.Description != "Plan helper" || !stored.Enabled {
		t.Fatalf("stored = %+v, want original fields restored", stored)
	}

	targets, err := db.GetSkillTargets(skillID)
	if err != nil {
		t.Fatalf("GetSkillTargets: %v", err)
	}
	if targets["codex"].Method != "symlink" || !targets["codex"].Enabled {
		t.Fatalf("targets = %+v, want original codex symlink enabled", targets)
	}
	if targets["claude"].Method != "copy" || targets["claude"].Enabled {
		t.Fatalf("targets = %+v, want original claude copy disabled", targets)
	}
}

func TestSetSkillTargetsRollbackRestoresOriginalTargets(t *testing.T) {
	db := tempDB(t)
	badSkillRoot := filepath.Join(t.TempDir(), "codex-skills-root")
	if err := os.WriteFile(badSkillRoot, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile badSkillRoot: %v", err)
	}
	adapter := &failingServerTestAdapter{
		tool:       "codex",
		installed:  true,
		skillPaths: []string{badSkillRoot},
	}
	mgr := configmanager.NewManager(
		db,
		t.TempDir(),
		configmanager.WithEncryptionKey([]byte("12345678901234567890123456789012")),
		configmanager.WithAdapter(adapter),
	)
	srv := New(db, mgr, "127.0.0.1:0")
	handler := srv.Handler()

	sourceDir := t.TempDir()
	skillID, err := db.CreateSkill("planner", sourceDir, "Plan helper")
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}
	if err := db.SetSkillTargets(skillID, []storage.SkillTargetRecord{
		{Tool: "codex", Method: "symlink", Enabled: true},
		{Tool: "claude", Method: "copy", Enabled: false},
	}); err != nil {
		t.Fatalf("SetSkillTargets: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/config/skills/%d/targets", skillID), bytes.NewBufferString(`{"targets":{"codex":{"method":"copy","enabled":true},"claude":{"method":"copy","enabled":true}}}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", w.Code, w.Body.String())
	}

	targets, err := db.GetSkillTargets(skillID)
	if err != nil {
		t.Fatalf("GetSkillTargets: %v", err)
	}
	if targets["codex"].Method != "symlink" || !targets["codex"].Enabled {
		t.Fatalf("targets = %+v, want original codex symlink enabled", targets)
	}
	if targets["claude"].Method != "copy" || targets["claude"].Enabled {
		t.Fatalf("targets = %+v, want original claude copy disabled", targets)
	}
}

type failingServerTestAdapter struct {
	tool       string
	mcpErr     error
	skillErr   error
	installed  bool
	skillPaths []string
}

func (a *failingServerTestAdapter) Tool() string { return a.tool }

func (a *failingServerTestAdapter) IsInstalled() bool { return a.installed }

func (a *failingServerTestAdapter) GetProviderConfig() (*configmanager.ProviderConfig, error) {
	return &configmanager.ProviderConfig{}, nil
}

func (a *failingServerTestAdapter) SetProviderConfig(*configmanager.ProviderConfig) ([]configmanager.AffectedFile, error) {
	return nil, nil
}

func (a *failingServerTestAdapter) GetMCPServers() ([]configmanager.MCPServerConfig, error) {
	return nil, nil
}

func (a *failingServerTestAdapter) SetMCPServers([]configmanager.MCPServerConfig) ([]configmanager.AffectedFile, error) {
	if a.mcpErr != nil {
		return nil, a.mcpErr
	}
	return nil, nil
}

func (a *failingServerTestAdapter) GetSkillPaths() []string { return a.skillPaths }

func (a *failingServerTestAdapter) ConfigFiles() []configmanager.ConfigFileInfo { return nil }

func TestCreateSkillRequiresExistingDirectory(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	body := bytes.NewBufferString(`{"name":"missing","source_path":"/path/that/does/not/exist","targets":{"codex":{"enabled":true}}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/config/skills", body)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "source_path") {
		t.Fatalf("body = %s, want source_path validation error", w.Body.String())
	}
}

func TestDeleteSkillRollbackPreservesOriginalID(t *testing.T) {
	db := tempDB(t)
	skillInstallRoot := filepath.Join(t.TempDir(), "skills")
	adapter := &failingServerTestAdapter{
		tool:       "codex",
		installed:  true,
		skillPaths: []string{skillInstallRoot},
	}
	mgr := configmanager.NewManager(
		db,
		t.TempDir(),
		configmanager.WithEncryptionKey([]byte("12345678901234567890123456789012")),
		configmanager.WithAdapter(adapter),
	)
	srv := New(db, mgr, "127.0.0.1:0")
	handler := srv.Handler()

	victimSource := filepath.Join(t.TempDir(), "victim-skill")
	if err := os.MkdirAll(victimSource, 0o755); err != nil {
		t.Fatalf("MkdirAll victimSource: %v", err)
	}
	keeperSource := filepath.Join(t.TempDir(), "keeper-skill")
	if err := os.MkdirAll(keeperSource, 0o755); err != nil {
		t.Fatalf("MkdirAll keeperSource: %v", err)
	}

	victimID, err := db.CreateSkill("victim-skill", victimSource, "victim")
	if err != nil {
		t.Fatalf("CreateSkill victim: %v", err)
	}
	if err := db.SetSkillTargets(victimID, []storage.SkillTargetRecord{
		{Tool: "codex", Method: "symlink", Enabled: true},
	}); err != nil {
		t.Fatalf("SetSkillTargets victim: %v", err)
	}

	keeperID, err := db.CreateSkill("keeper-skill", keeperSource, "keeper")
	if err != nil {
		t.Fatalf("CreateSkill keeper: %v", err)
	}
	if err := db.SetSkillTargets(keeperID, []storage.SkillTargetRecord{
		{Tool: "codex", Method: "copy", Enabled: true},
	}); err != nil {
		t.Fatalf("SetSkillTargets keeper: %v", err)
	}

	if err := os.RemoveAll(keeperSource); err != nil {
		t.Fatalf("RemoveAll keeperSource: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/config/skills/%d", victimID), nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", w.Code, w.Body.String())
	}

	stored, err := db.GetSkill(victimID)
	if err != nil {
		t.Fatalf("GetSkill: %v", err)
	}
	if stored == nil {
		t.Fatalf("GetSkill(%d) = nil, want restored row", victimID)
	}
	if stored.ID != victimID {
		t.Fatalf("stored.ID = %d, want %d", stored.ID, victimID)
	}

	targets, err := db.GetSkillTargets(victimID)
	if err != nil {
		t.Fatalf("GetSkillTargets: %v", err)
	}
	if !targets["codex"].Enabled || targets["codex"].Method != "symlink" {
		t.Fatalf("targets = %+v, want codex symlink enabled", targets)
	}
}

func TestConfigSyncBackupsAndFilesEndpoints(t *testing.T) {
	db := tempDB(t)
	srv := New(db, tempManager(t, db), "127.0.0.1:0")
	handler := srv.Handler()

	checks := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{http.MethodPost, "/api/config/sync", "", http.StatusOK},
		{http.MethodGet, "/api/config/sync/status", "", http.StatusOK},
		{http.MethodPost, "/api/config/sync/resolve", `{"tool":"codex","file_path":"/tmp/config","strategy":"keep_ours"}`, http.StatusNotFound},
		{http.MethodGet, "/api/config/backups", "", http.StatusOK},
		{http.MethodPost, "/api/config/backups", "", http.StatusOK},
		{http.MethodGet, "/api/config/files", "", http.StatusOK},
	}

	for _, check := range checks {
		t.Run(check.method+" "+check.path, func(t *testing.T) {
			req := httptest.NewRequest(check.method, check.path, bytes.NewBufferString(check.body))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != check.status {
				t.Fatalf("status = %d, want %d; body = %s", w.Code, check.status, w.Body.String())
			}
		})
	}
}
