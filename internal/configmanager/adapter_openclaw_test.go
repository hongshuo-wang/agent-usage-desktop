package configmanager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOpenClawAdapterProviderReadWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "openclaw.json")
	initial := []byte(`{
  "provider": {"api_key":"sk-old", "base_url":"https://old.example.com", "model":"old-model", "extra":"keep"},
  "theme": "dark"
}`)
	if err := os.WriteFile(configPath, initial, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	adapter := NewOpenClawAdapter(configPath)
	cfg, err := adapter.GetProviderConfig()
	if err != nil {
		t.Fatalf("GetProviderConfig() error = %v", err)
	}
	if cfg.APIKey != "sk-old" {
		t.Fatalf("cfg.APIKey = %q, want %q", cfg.APIKey, "sk-old")
	}
	if cfg.BaseURL != "https://old.example.com" {
		t.Fatalf("cfg.BaseURL = %q, want %q", cfg.BaseURL, "https://old.example.com")
	}
	if cfg.Model != "old-model" {
		t.Fatalf("cfg.Model = %q, want %q", cfg.Model, "old-model")
	}

	_, err = adapter.SetProviderConfig(&ProviderConfig{APIKey: "sk-new", BaseURL: "https://new.example.com", Model: "new-model"})
	if err != nil {
		t.Fatalf("SetProviderConfig() error = %v", err)
	}

	updated := readJSONFileMapForOpenClawTest(t, configPath)
	provider, ok := updated["provider"].(map[string]any)
	if !ok {
		t.Fatalf("provider missing or invalid type")
	}
	if provider["api_key"] != "sk-new" {
		t.Fatalf("provider.api_key = %v, want %q", provider["api_key"], "sk-new")
	}
	if provider["base_url"] != "https://new.example.com" {
		t.Fatalf("provider.base_url = %v, want %q", provider["base_url"], "https://new.example.com")
	}
	if provider["model"] != "new-model" {
		t.Fatalf("provider.model = %v, want %q", provider["model"], "new-model")
	}
	if provider["extra"] != "keep" {
		t.Fatalf("provider.extra = %v, want %q", provider["extra"], "keep")
	}
	if updated["theme"] != "dark" {
		t.Fatalf("theme = %v, want %q", updated["theme"], "dark")
	}

	_, err = adapter.SetProviderConfig(&ProviderConfig{Model: "partial-model"})
	if err != nil {
		t.Fatalf("SetProviderConfig(partial) error = %v", err)
	}

	updated = readJSONFileMapForOpenClawTest(t, configPath)
	provider = updated["provider"].(map[string]any)
	if provider["api_key"] != "sk-new" {
		t.Fatalf("provider.api_key after partial = %v, want unchanged", provider["api_key"])
	}
	if provider["base_url"] != "https://new.example.com" {
		t.Fatalf("provider.base_url after partial = %v, want unchanged", provider["base_url"])
	}
	if provider["model"] != "partial-model" {
		t.Fatalf("provider.model after partial = %v, want %q", provider["model"], "partial-model")
	}
}

func TestOpenClawAdapterProviderErrorsOnNonObjectProvider(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "openclaw.json")
	if err := os.WriteFile(configPath, []byte(`{"provider":"invalid"}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	adapter := NewOpenClawAdapter(configPath)

	_, err := adapter.GetProviderConfig()
	if err == nil {
		t.Fatalf("GetProviderConfig() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "provider") {
		t.Fatalf("GetProviderConfig() error = %q, want mention provider", err)
	}

	_, err = adapter.SetProviderConfig(&ProviderConfig{APIKey: "sk-new"})
	if err == nil {
		t.Fatalf("SetProviderConfig() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "provider") {
		t.Fatalf("SetProviderConfig() error = %q, want mention provider", err)
	}
}

func TestOpenClawAdapterProviderErrorsOnMalformedScalarField(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "openclaw.json")
	if err := os.WriteFile(configPath, []byte(`{"provider":{"api_key":123}}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	adapter := NewOpenClawAdapter(configPath)

	_, err := adapter.GetProviderConfig()
	if err == nil {
		t.Fatalf("GetProviderConfig() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Fatalf("GetProviderConfig() error = %q, want mention api_key", err)
	}

	_, err = adapter.SetProviderConfig(&ProviderConfig{Model: "new-model"})
	if err == nil {
		t.Fatalf("SetProviderConfig() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Fatalf("SetProviderConfig() error = %q, want mention api_key", err)
	}
}

func TestOpenClawAdapterMCPReadWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "openclaw.json")
	initial := []byte(`{
  "theme": "dark",
  "mcp": {
    "filesystem": {
      "command": "npx",
      "args": ["-y"],
      "env": {"NODE_ENV": "test"},
      "metadata": {"keep": true}
    },
    "alpha": {
      "command": "node"
    }
  }
}`)
	if err := os.WriteFile(configPath, initial, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	adapter := NewOpenClawAdapter(configPath)
	servers, err := adapter.GetMCPServers()
	if err != nil {
		t.Fatalf("GetMCPServers() error = %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("len(servers) = %d, want 2", len(servers))
	}
	if servers[0].Name != "alpha" || servers[1].Name != "filesystem" {
		t.Fatalf("server order = [%s %s], want [alpha filesystem]", servers[0].Name, servers[1].Name)
	}
	if servers[1].Command != "npx" {
		t.Fatalf("filesystem.command = %q, want %q", servers[1].Command, "npx")
	}
	if !reflect.DeepEqual(servers[1].Args, []string{"-y"}) {
		t.Fatalf("filesystem.args = %v, want [-y]", servers[1].Args)
	}
	if !reflect.DeepEqual(servers[1].Env, map[string]string{"NODE_ENV": "test"}) {
		t.Fatalf("filesystem.env = %v, want NODE_ENV=test", servers[1].Env)
	}

	_, err = adapter.SetMCPServers([]MCPServerConfig{
		{Name: "filesystem", Command: "node", Args: []string{"fs.js"}, Env: map[string]string{"NODE_ENV": "prod"}},
		{Name: "github", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}},
	})
	if err != nil {
		t.Fatalf("SetMCPServers() error = %v", err)
	}

	updated := readJSONFileMapForOpenClawTest(t, configPath)
	if updated["theme"] != "dark" {
		t.Fatalf("theme = %v, want %q", updated["theme"], "dark")
	}

	mcp, ok := updated["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("mcp missing or invalid type")
	}
	if len(mcp) != 2 {
		t.Fatalf("len(mcp) = %d, want 2", len(mcp))
	}

	filesystem, ok := mcp["filesystem"].(map[string]any)
	if !ok {
		t.Fatalf("mcp.filesystem missing or invalid type")
	}
	if filesystem["command"] != "node" {
		t.Fatalf("filesystem.command = %v, want %q", filesystem["command"], "node")
	}
	if !reflect.DeepEqual(filesystem["args"], []any{"fs.js"}) {
		t.Fatalf("filesystem.args = %v, want [fs.js]", filesystem["args"])
	}
	if env, ok := filesystem["env"].(map[string]any); !ok || env["NODE_ENV"] != "prod" {
		t.Fatalf("filesystem.env = %v, want NODE_ENV=prod", filesystem["env"])
	}
	if _, ok := filesystem["metadata"].(map[string]any); !ok {
		t.Fatalf("filesystem.metadata missing or invalid type")
	}
}

func TestOpenClawAdapterMCPErrorsOnNonObjectTopLevel(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "openclaw.json")
	if err := os.WriteFile(configPath, []byte(`{"mcp":"invalid"}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	adapter := NewOpenClawAdapter(configPath)
	_, err := adapter.GetMCPServers()
	if err == nil {
		t.Fatalf("GetMCPServers() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "mcp") {
		t.Fatalf("GetMCPServers() error = %q, want mention mcp", err)
	}

	_, err = adapter.SetMCPServers([]MCPServerConfig{{Name: "github", Command: "npx"}})
	if err == nil {
		t.Fatalf("SetMCPServers() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "mcp") {
		t.Fatalf("SetMCPServers() error = %q, want mention mcp", err)
	}
}

func TestOpenClawAdapterGetMCPServersErrorsOnMalformedFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		configJSON  string
		errorSubstr string
	}{
		{
			name:        "command not string",
			configJSON:  `{"mcp":{"filesystem":{"command":1}}}`,
			errorSubstr: "command",
		},
		{
			name:        "args contain non-string",
			configJSON:  `{"mcp":{"filesystem":{"command":"npx","args":["-y",1]}}}`,
			errorSubstr: "args",
		},
		{
			name:        "env value not string",
			configJSON:  `{"mcp":{"filesystem":{"command":"npx","env":{"NODE_ENV":1}}}}`,
			errorSubstr: "env",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			configPath := filepath.Join(dir, "openclaw.json")
			if err := os.WriteFile(configPath, []byte(test.configJSON), 0o644); err != nil {
				t.Fatalf("os.WriteFile() error = %v", err)
			}

			adapter := NewOpenClawAdapter(configPath)
			_, err := adapter.GetMCPServers()
			if err == nil {
				t.Fatalf("GetMCPServers() error = nil, want non-nil")
			}
			if !strings.Contains(err.Error(), test.errorSubstr) {
				t.Fatalf("GetMCPServers() error = %q, want mention %q", err, test.errorSubstr)
			}
		})
	}
}

func TestOpenClawAdapterMetadata(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "openclaw.json")
	adapter := NewOpenClawAdapter(configPath)

	if got := adapter.Tool(); got != "openclaw" {
		t.Fatalf("Tool() = %q, want %q", got, "openclaw")
	}
	if adapter.IsInstalled() {
		t.Fatalf("IsInstalled() = true, want false")
	}

	if err := os.WriteFile(configPath, []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if !adapter.IsInstalled() {
		t.Fatalf("IsInstalled() = false, want true")
	}

	skillPaths := adapter.GetSkillPaths()
	wantSkillPaths := []string{filepath.Join(dir, "skills")}
	if !reflect.DeepEqual(skillPaths, wantSkillPaths) {
		t.Fatalf("GetSkillPaths() = %v, want %v", skillPaths, wantSkillPaths)
	}

	files := adapter.ConfigFiles()
	if len(files) != 1 {
		t.Fatalf("len(ConfigFiles()) = %d, want 1", len(files))
	}
	if files[0].Path != configPath {
		t.Fatalf("ConfigFiles()[0].Path = %q, want %q", files[0].Path, configPath)
	}
	if files[0].Tool != "openclaw" {
		t.Fatalf("ConfigFiles()[0].Tool = %q, want %q", files[0].Tool, "openclaw")
	}
	if files[0].DocURL != "https://docs.openclaw.ai/gateway/configuration" {
		t.Fatalf("ConfigFiles()[0].DocURL = %q", files[0].DocURL)
	}
	if !files[0].Exists {
		t.Fatalf("ConfigFiles()[0].Exists = false, want true")
	}
}

func readJSONFileMapForOpenClawTest(t *testing.T, path string) map[string]any {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%s) error = %v", path, err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", path, err)
	}

	return parsed
}
