package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/config"
)

func settingsFixture(t *testing.T) (http.Handler, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.DefaultConfig()
	cfg.Server.Port = 4321
	cfg.Storage.Path = "/private/usage.db"
	cfg.Collectors.Claude.Paths = []string{"/claude/a", "/claude/b"}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	return New(tempDB(t), "127.0.0.1:0", WithConfigPath(path)).Handler(), path
}

func requestSettings(t *testing.T, handler http.Handler, method string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/settings/collectors", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	return w
}

const validSettingsBody = `{
  "collectors": [
    {"name":"claude","enabled":false,"paths":["/new/claude","/second/claude"],"scan_interval":"30s"},
    {"name":"codex","enabled":true,"paths":["/new/codex"],"scan_interval":"45s"},
    {"name":"openclaw","enabled":true,"paths":["/new/openclaw"],"scan_interval":"1m"},
    {"name":"opencode","enabled":false,"paths":["/new/opencode.db"],"scan_interval":"2m"}
  ],
  "pricing_sync_interval":"3h"
}`

func TestSettingsCollectorsGetUsesWhitelist(t *testing.T) {
	handler, _ := settingsFixture(t)
	w := requestSettings(t, handler, http.MethodGet, "")
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response) != 2 || response["collectors"] == nil || response["pricing_sync_interval"] == nil {
		t.Fatalf("unexpected response keys: %#v", response)
	}
	body := strings.ToLower(w.Body.String())
	for _, forbidden := range []string{"storage", "bind_address", "api_key", "provider", "mcp", "skill", "backup"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("response exposes forbidden field %q: %s", forbidden, body)
		}
	}
	var typed struct {
		Collectors []struct {
			Name         string   `json:"name"`
			Enabled      bool     `json:"enabled"`
			Paths        []string `json:"paths"`
			ScanInterval string   `json:"scan_interval"`
		} `json:"collectors"`
		PricingSyncInterval string `json:"pricing_sync_interval"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &typed); err != nil {
		t.Fatal(err)
	}
	if len(typed.Collectors) != 4 || len(typed.Collectors[0].Paths) != 2 || typed.Collectors[0].Name != "claude" || typed.Collectors[0].ScanInterval != "1m0s" {
		t.Fatalf("collectors=%+v", typed.Collectors)
	}
}

func TestSettingsCollectorsPutPersistsAndPreservesPrivateConfig(t *testing.T) {
	handler, path := settingsFixture(t)
	w := requestSettings(t, handler, http.MethodPut, validSettingsBody)
	if w.Code != http.StatusOK || strings.TrimSpace(w.Body.String()) != `{"restart_required":true}` {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 4321 || cfg.Storage.Path != "/private/usage.db" {
		t.Fatalf("private config changed: server=%+v storage=%+v", cfg.Server, cfg.Storage)
	}
	if cfg.Collectors.Claude.Enabled || len(cfg.Collectors.Claude.Paths) != 2 || cfg.Collectors.Codex.ScanInterval != 45*time.Second || cfg.Pricing.SyncInterval != 3*time.Hour {
		t.Fatalf("settings not persisted: %+v", cfg)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("saved config mode: info=%v err=%v", info, err)
	}
}

func TestSettingsCollectorsRejectsInvalidPayloadWithoutChangingConfig(t *testing.T) {
	cases := map[string]string{
		"unknown top field":            strings.Replace(validSettingsBody, `"pricing_sync_interval":"3h"`, `"pricing_sync_interval":"3h","provider":"x"`, 1),
		"unknown collector field":      strings.Replace(validSettingsBody, `"name":"claude"`, `"name":"claude","api_key":"secret"`, 1),
		"unknown collector":            strings.Replace(validSettingsBody, `"name":"claude"`, `"name":"hermes"`, 1),
		"missing paths":                strings.Replace(validSettingsBody, `,"paths":["/new/codex"]`, "", 1),
		"empty paths":                  strings.Replace(validSettingsBody, `["/new/codex"]`, `[]`, 1),
		"blank path":                   strings.Replace(validSettingsBody, `["/new/codex"]`, `[" "]`, 1),
		"malformed collector interval": strings.Replace(validSettingsBody, `"scan_interval":"30s"`, `"scan_interval":"later"`, 1),
		"short collector interval":     strings.Replace(validSettingsBody, `"scan_interval":"30s"`, `"scan_interval":"9s"`, 1),
		"malformed pricing interval":   strings.Replace(validSettingsBody, `"pricing_sync_interval":"3h"`, `"pricing_sync_interval":"daily"`, 1),
		"short pricing interval":       strings.Replace(validSettingsBody, `"pricing_sync_interval":"3h"`, `"pricing_sync_interval":"9s"`, 1),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			handler, path := settingsFixture(t)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			w := requestSettings(t, handler, http.MethodPut, body)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("invalid request changed config")
			}
		})
	}
}

func TestSettingsCollectorsRequiresConfiguredPathAndExactMethods(t *testing.T) {
	handler := New(tempDB(t), "127.0.0.1:0").Handler()
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		w := requestSettings(t, handler, method, validSettingsBody)
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "config path") {
			t.Fatalf("%s status=%d body=%s", method, w.Code, w.Body.String())
		}
	}
	w := requestSettings(t, handler, http.MethodPost, validSettingsBody)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status=%d, want 405", w.Code)
	}
}

func TestSettingsCollectorsConcurrentPutProducesValidConfig(t *testing.T) {
	handler, path := settingsFixture(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := requestSettings(t, handler, http.MethodPut, validSettingsBody)
			if w.Code != http.StatusOK {
				t.Errorf("status=%d body=%s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	if _, err := config.Load(path); err != nil {
		t.Fatalf("concurrent writes left invalid config: %v", err)
	}
}
