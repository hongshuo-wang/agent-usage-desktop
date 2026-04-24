package configmanager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestCodexAdapterProviderReadWritePreservesUnknownFields(t *testing.T) {
	t.Parallel()

	codexDir := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	authPath := filepath.Join(codexDir, "auth.json")
	auth := []byte(`{
		"api_key": "sk-old",
		"OPENAI_API_KEY": "sk-env-old",
		"extra": "keep"
	}`)
	if err := os.WriteFile(authPath, auth, 0o644); err != nil {
		t.Fatalf("os.WriteFile(auth.json) error = %v", err)
	}

	configPath := filepath.Join(codexDir, "config.toml")
	config := []byte(`
[provider]
base_url = "https://old.example.com/v1"

[model]
name = "gpt-old"

[unrelated]
flag = true
`)
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatalf("os.WriteFile(config.toml) error = %v", err)
	}

	adapter := NewCodexAdapter(codexDir)
	cfg, err := adapter.GetProviderConfig()
	if err != nil {
		t.Fatalf("GetProviderConfig() error = %v", err)
	}

	if cfg.APIKey != "sk-old" {
		t.Fatalf("cfg.APIKey = %q, want %q", cfg.APIKey, "sk-old")
	}
	if cfg.BaseURL != "https://old.example.com/v1" {
		t.Fatalf("cfg.BaseURL = %q, want %q", cfg.BaseURL, "https://old.example.com/v1")
	}
	if cfg.Model != "gpt-old" {
		t.Fatalf("cfg.Model = %q, want %q", cfg.Model, "gpt-old")
	}

	_, err = adapter.SetProviderConfig(&ProviderConfig{
		APIKey:  "sk-new",
		BaseURL: "https://api.openai.com/v1",
		Model:   "gpt-5",
	})
	if err != nil {
		t.Fatalf("SetProviderConfig() error = %v", err)
	}

	authBytes, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("os.ReadFile(auth.json) error = %v", err)
	}

	var updatedAuth map[string]any
	if err := json.Unmarshal(authBytes, &updatedAuth); err != nil {
		t.Fatalf("json.Unmarshal(auth.json) error = %v", err)
	}
	if updatedAuth["api_key"] != "sk-new" {
		t.Fatalf("auth.api_key = %v, want %q", updatedAuth["api_key"], "sk-new")
	}
	if updatedAuth["OPENAI_API_KEY"] != "sk-new" {
		t.Fatalf("auth.OPENAI_API_KEY = %v, want %q", updatedAuth["OPENAI_API_KEY"], "sk-new")
	}
	if updatedAuth["extra"] != "keep" {
		t.Fatalf("auth.extra = %v, want %q", updatedAuth["extra"], "keep")
	}

	var updatedConfig map[string]any
	if _, err := toml.DecodeFile(configPath, &updatedConfig); err != nil {
		t.Fatalf("toml.DecodeFile(config.toml) error = %v", err)
	}
	provider, ok := updatedConfig["provider"].(map[string]any)
	if !ok {
		t.Fatalf("provider section missing or invalid")
	}
	if provider["base_url"] != "https://api.openai.com/v1" {
		t.Fatalf("provider.base_url = %v, want %q", provider["base_url"], "https://api.openai.com/v1")
	}
	model, ok := updatedConfig["model"].(map[string]any)
	if !ok {
		t.Fatalf("model section missing or invalid")
	}
	if model["name"] != "gpt-5" {
		t.Fatalf("model.name = %v, want %q", model["name"], "gpt-5")
	}
	unrelated, ok := updatedConfig["unrelated"].(map[string]any)
	if !ok || unrelated["flag"] != true {
		t.Fatalf("unrelated section not preserved: %v", updatedConfig["unrelated"])
	}

	_, err = adapter.SetProviderConfig(&ProviderConfig{Model: "gpt-5-mini"})
	if err != nil {
		t.Fatalf("SetProviderConfig(partial) error = %v", err)
	}
	if _, err := toml.DecodeFile(configPath, &updatedConfig); err != nil {
		t.Fatalf("toml.DecodeFile(config.toml) after partial update error = %v", err)
	}
	provider = updatedConfig["provider"].(map[string]any)
	if provider["base_url"] != "https://api.openai.com/v1" {
		t.Fatalf("provider.base_url after partial = %v, want unchanged", provider["base_url"])
	}
}

func TestCodexAdapterMCPServersReadWritePreservesNonMCP(t *testing.T) {
	t.Parallel()

	codexDir := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	configPath := filepath.Join(codexDir, "config.toml")
	config := []byte(`
[provider]
base_url = "https://api.openai.com/v1"

[feature]
enabled = true

[mcp_servers.beta]
command = "node"

[mcp_servers.alpha]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]

[mcp_servers.alpha.env]
NODE_ENV = "test"
`)
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatalf("os.WriteFile(config.toml) error = %v", err)
	}

	adapter := NewCodexAdapter(codexDir)
	servers, err := adapter.GetMCPServers()
	if err != nil {
		t.Fatalf("GetMCPServers() error = %v", err)
	}

	if len(servers) != 2 {
		t.Fatalf("len(servers) = %d, want 2", len(servers))
	}
	if servers[0].Name != "alpha" || servers[1].Name != "beta" {
		t.Fatalf("server order = [%s %s], want [alpha beta]", servers[0].Name, servers[1].Name)
	}
	if servers[0].Command != "npx" {
		t.Fatalf("servers[0].Command = %q, want %q", servers[0].Command, "npx")
	}
	if !reflect.DeepEqual(servers[0].Args, []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"}) {
		t.Fatalf("servers[0].Args = %v, want filesystem args", servers[0].Args)
	}
	if !reflect.DeepEqual(servers[0].Env, map[string]string{"NODE_ENV": "test"}) {
		t.Fatalf("servers[0].Env = %v, want NODE_ENV=test", servers[0].Env)
	}

	_, err = adapter.SetMCPServers([]MCPServerConfig{
		{Name: "github", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}},
		{Name: "memory", Command: "node", Args: []string{"memory.js"}, Env: map[string]string{"TOKEN": "abc"}},
	})
	if err != nil {
		t.Fatalf("SetMCPServers() error = %v", err)
	}

	var updated map[string]any
	if _, err := toml.DecodeFile(configPath, &updated); err != nil {
		t.Fatalf("toml.DecodeFile(updated config.toml) error = %v", err)
	}

	if feature, ok := updated["feature"].(map[string]any); !ok || feature["enabled"] != true {
		t.Fatalf("feature section not preserved: %v", updated["feature"])
	}

	mcpRaw, ok := updated["mcp_servers"].(map[string]any)
	if !ok {
		t.Fatalf("mcp_servers missing or invalid")
	}
	if len(mcpRaw) != 2 {
		t.Fatalf("len(mcp_servers) = %d, want 2", len(mcpRaw))
	}

	github, ok := mcpRaw["github"].(map[string]any)
	if !ok {
		t.Fatalf("mcp_servers.github missing or invalid")
	}
	if github["command"] != "npx" {
		t.Fatalf("github.command = %v, want %q", github["command"], "npx")
	}

	memory, ok := mcpRaw["memory"].(map[string]any)
	if !ok {
		t.Fatalf("mcp_servers.memory missing or invalid")
	}
	if memory["command"] != "node" {
		t.Fatalf("memory.command = %v, want %q", memory["command"], "node")
	}
	memoryEnv, ok := memory["env"].(map[string]any)
	if !ok || memoryEnv["TOKEN"] != "abc" {
		t.Fatalf("memory.env = %v, want TOKEN=abc", memory["env"])
	}
}

func TestCodexAdapterMetadata(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	codexDir := filepath.Join(t.TempDir(), ".codex")
	adapter := NewCodexAdapter(codexDir)

	if got := adapter.Tool(); got != "codex" {
		t.Fatalf("Tool() = %q, want %q", got, "codex")
	}
	if adapter.IsInstalled() {
		t.Fatalf("IsInstalled() = true, want false")
	}

	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}
	if !adapter.IsInstalled() {
		t.Fatalf("IsInstalled() = false, want true")
	}

	skillPaths := adapter.GetSkillPaths()
	wantSkills := []string{filepath.Join(homeDir, ".agents", "skills")}
	if !reflect.DeepEqual(skillPaths, wantSkills) {
		t.Fatalf("GetSkillPaths() = %v, want %v", skillPaths, wantSkills)
	}

	configFiles := adapter.ConfigFiles()
	if len(configFiles) != 2 {
		t.Fatalf("len(ConfigFiles()) = %d, want 2", len(configFiles))
	}

	if configFiles[0].Path != filepath.Join(codexDir, "auth.json") {
		t.Fatalf("ConfigFiles()[0].Path = %q, want auth.json path", configFiles[0].Path)
	}
	if configFiles[1].Path != filepath.Join(codexDir, "config.toml") {
		t.Fatalf("ConfigFiles()[1].Path = %q, want config.toml path", configFiles[1].Path)
	}
	if configFiles[0].DocURL != "https://github.com/openai/codex" || configFiles[1].DocURL != "https://github.com/openai/codex" {
		t.Fatalf("ConfigFiles() DocURL = [%q %q], want codex doc URL", configFiles[0].DocURL, configFiles[1].DocURL)
	}
}

func TestCodexAdapterProviderConfigSupportsStringModel(t *testing.T) {
	t.Parallel()

	codexDir := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	authPath := filepath.Join(codexDir, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"api_key":"sk-old"}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile(auth.json) error = %v", err)
	}

	configPath := filepath.Join(codexDir, "config.toml")
	config := []byte(`
model = "gpt-5.4"

[provider]
base_url = "https://api.openai.com/v1"

[unrelated]
flag = true
`)
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatalf("os.WriteFile(config.toml) error = %v", err)
	}

	adapter := NewCodexAdapter(codexDir)
	cfg, err := adapter.GetProviderConfig()
	if err != nil {
		t.Fatalf("GetProviderConfig() error = %v", err)
	}
	if cfg.APIKey != "sk-old" {
		t.Fatalf("cfg.APIKey = %q, want %q", cfg.APIKey, "sk-old")
	}
	if cfg.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("cfg.BaseURL = %q, want %q", cfg.BaseURL, "https://api.openai.com/v1")
	}
	if cfg.Model != "gpt-5.4" {
		t.Fatalf("cfg.Model = %q, want %q", cfg.Model, "gpt-5.4")
	}

	_, err = adapter.SetProviderConfig(&ProviderConfig{
		BaseURL: "https://mirror.example.com/v1",
		Model:   "gpt-5.5",
	})
	if err != nil {
		t.Fatalf("SetProviderConfig() error = %v", err)
	}

	var updated map[string]any
	if _, err := toml.DecodeFile(configPath, &updated); err != nil {
		t.Fatalf("toml.DecodeFile(config.toml) error = %v", err)
	}
	if updated["model"] != "gpt-5.5" {
		t.Fatalf("model = %v, want %q", updated["model"], "gpt-5.5")
	}

	provider, ok := updated["provider"].(map[string]any)
	if !ok {
		t.Fatalf("provider section missing or invalid")
	}
	if provider["base_url"] != "https://mirror.example.com/v1" {
		t.Fatalf("provider.base_url = %v, want %q", provider["base_url"], "https://mirror.example.com/v1")
	}

	unrelated, ok := updated["unrelated"].(map[string]any)
	if !ok || unrelated["flag"] != true {
		t.Fatalf("unrelated section not preserved: %v", updated["unrelated"])
	}
}

func TestCodexAdapterSetMCPServersPreservesUnknownFieldsOnExistingServer(t *testing.T) {
	t.Parallel()

	codexDir := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	configPath := filepath.Join(codexDir, "config.toml")
	config := []byte(`
[mcp_servers.github]
command = "node"
timeout = 30
transport = "stdio"

[mcp_servers.github.env]
TOKEN = "old"
KEEP = "true"
`)
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatalf("os.WriteFile(config.toml) error = %v", err)
	}

	adapter := NewCodexAdapter(codexDir)
	_, err := adapter.SetMCPServers([]MCPServerConfig{{
		Name:    "github",
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-github"},
		Env:     map[string]string{"TOKEN": "new"},
	}})
	if err != nil {
		t.Fatalf("SetMCPServers() error = %v", err)
	}

	var updated map[string]any
	if _, err := toml.DecodeFile(configPath, &updated); err != nil {
		t.Fatalf("toml.DecodeFile(updated config.toml) error = %v", err)
	}

	mcpServers, ok := updated["mcp_servers"].(map[string]any)
	if !ok {
		t.Fatalf("mcp_servers missing or invalid")
	}
	github, ok := mcpServers["github"].(map[string]any)
	if !ok {
		t.Fatalf("mcp_servers.github missing or invalid")
	}

	if github["timeout"] != int64(30) {
		t.Fatalf("github.timeout = %v, want 30", github["timeout"])
	}
	if github["transport"] != "stdio" {
		t.Fatalf("github.transport = %v, want %q", github["transport"], "stdio")
	}
	if github["command"] != "npx" {
		t.Fatalf("github.command = %v, want %q", github["command"], "npx")
	}
}

func TestCodexAdapterMCPServersErrorsOnNonObjectTopLevel(t *testing.T) {
	t.Parallel()

	codexDir := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll() error = %v", err)
	}

	configPath := filepath.Join(codexDir, "config.toml")
	config := []byte(`mcp_servers = "invalid"`)
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatalf("os.WriteFile(config.toml) error = %v", err)
	}

	adapter := NewCodexAdapter(codexDir)
	_, err := adapter.GetMCPServers()
	if err == nil {
		t.Fatalf("GetMCPServers() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "mcp_servers") {
		t.Fatalf("GetMCPServers() error = %q, want mention mcp_servers", err)
	}

	_, err = adapter.SetMCPServers([]MCPServerConfig{{Name: "github", Command: "npx"}})
	if err == nil {
		t.Fatalf("SetMCPServers() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "mcp_servers") {
		t.Fatalf("SetMCPServers() error = %q, want mention mcp_servers", err)
	}
}

func TestCodexAdapterGetMCPServersErrorsOnMalformedServerFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		configTOML  string
		errorSubstr string
	}{
		{
			name: "command not string",
			configTOML: `
[mcp_servers.github]
command = 1
`,
			errorSubstr: "command",
		},
		{
			name: "args contain non-string",
			configTOML: `
[mcp_servers.github]
command = "npx"
args = ["-y", 1]
`,
			errorSubstr: "args",
		},
		{
			name: "env value not string",
			configTOML: `
[mcp_servers.github]
command = "npx"

[mcp_servers.github.env]
TOKEN = 123
`,
			errorSubstr: "env",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			codexDir := filepath.Join(t.TempDir(), ".codex")
			if err := os.MkdirAll(codexDir, 0o755); err != nil {
				t.Fatalf("os.MkdirAll() error = %v", err)
			}
			configPath := filepath.Join(codexDir, "config.toml")
			if err := os.WriteFile(configPath, []byte(test.configTOML), 0o644); err != nil {
				t.Fatalf("os.WriteFile(config.toml) error = %v", err)
			}

			adapter := NewCodexAdapter(codexDir)
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

func TestCodexAdapterProviderConfigErrorsOnNonObjectSections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		configTOML  string
		errorSubstr string
	}{
		{
			name:        "provider not object",
			configTOML:  `provider = "invalid"`,
			errorSubstr: "provider",
		},
		{
			name: "model unsupported type",
			configTOML: `
model = 123

[provider]
base_url = "https://api.openai.com/v1"
`,
			errorSubstr: "model",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			codexDir := filepath.Join(t.TempDir(), ".codex")
			if err := os.MkdirAll(codexDir, 0o755); err != nil {
				t.Fatalf("os.MkdirAll() error = %v", err)
			}

			configPath := filepath.Join(codexDir, "config.toml")
			if err := os.WriteFile(configPath, []byte(test.configTOML), 0o644); err != nil {
				t.Fatalf("os.WriteFile(config.toml) error = %v", err)
			}

			adapter := NewCodexAdapter(codexDir)
			_, err := adapter.GetProviderConfig()
			if err == nil {
				t.Fatalf("GetProviderConfig() error = nil, want non-nil")
			}
			if !strings.Contains(err.Error(), test.errorSubstr) {
				t.Fatalf("GetProviderConfig() error = %q, want mention %q", err, test.errorSubstr)
			}

			_, err = adapter.SetProviderConfig(&ProviderConfig{BaseURL: "https://new.example.com/v1", Model: "gpt-5"})
			if err == nil {
				t.Fatalf("SetProviderConfig() error = nil, want non-nil")
			}
			if !strings.Contains(err.Error(), test.errorSubstr) {
				t.Fatalf("SetProviderConfig() error = %q, want mention %q", err, test.errorSubstr)
			}
		})
	}
}
