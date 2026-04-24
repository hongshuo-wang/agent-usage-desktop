package configmanager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeAdapterGetProviderConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	settings := []byte(`{"env":{"ANTHROPIC_API_KEY":"sk-test-123"},"permissions":{"allow":["Bash"]}}`)
	if err := os.WriteFile(settingsPath, settings, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	adapter := NewClaudeAdapter(dir, filepath.Join(dir, ".claude.json"))
	cfg, err := adapter.GetProviderConfig()
	if err != nil {
		t.Fatalf("GetProviderConfig() error = %v", err)
	}
	if cfg.APIKey != "sk-test-123" {
		t.Fatalf("cfg.APIKey = %q, want %q", cfg.APIKey, "sk-test-123")
	}
}

func TestClaudeAdapterSetProviderConfigPreservesUnknownFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	initial := []byte(`{
		"env": {
			"ANTHROPIC_API_KEY": "sk-old",
			"ANTHROPIC_BASE_URL": "https://old.example.com",
			"UNCHANGED": "keep-me"
		},
		"permissions": {"allow": ["Bash"]},
		"hooks": {"postToolUse": ["echo hi"]}
	}`)
	if err := os.WriteFile(settingsPath, initial, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	adapter := NewClaudeAdapter(dir, filepath.Join(dir, ".claude.json"))
	_, err := adapter.SetProviderConfig(&ProviderConfig{
		APIKey:  "sk-new",
		BaseURL: "https://api.example.com",
		Model:   "claude-sonnet",
	})
	if err != nil {
		t.Fatalf("SetProviderConfig() error = %v", err)
	}

	updatedBytes, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	var updated map[string]any
	if err := json.Unmarshal(updatedBytes, &updated); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	env, ok := updated["env"].(map[string]any)
	if !ok {
		t.Fatalf("env missing or invalid type")
	}
	if env["ANTHROPIC_API_KEY"] != "sk-new" {
		t.Fatalf("env.ANTHROPIC_API_KEY = %v, want %q", env["ANTHROPIC_API_KEY"], "sk-new")
	}
	if env["ANTHROPIC_BASE_URL"] != "https://api.example.com" {
		t.Fatalf("env.ANTHROPIC_BASE_URL = %v, want %q", env["ANTHROPIC_BASE_URL"], "https://api.example.com")
	}
	if env["UNCHANGED"] != "keep-me" {
		t.Fatalf("env.UNCHANGED = %v, want %q", env["UNCHANGED"], "keep-me")
	}

	if env["ANTHROPIC_MODEL"] != "claude-sonnet" {
		t.Fatalf("env.ANTHROPIC_MODEL = %v, want %q", env["ANTHROPIC_MODEL"], "claude-sonnet")
	}

	permissions, ok := updated["permissions"].(map[string]any)
	if !ok {
		t.Fatalf("permissions missing or invalid type")
	}
	allow, ok := permissions["allow"].([]any)
	if !ok || len(allow) != 1 || allow[0] != "Bash" {
		t.Fatalf("permissions.allow = %v, want [Bash]", permissions["allow"])
	}

	if _, ok := updated["hooks"].(map[string]any); !ok {
		t.Fatalf("hooks missing or invalid type")
	}
}

func TestClaudeAdapterSetProviderConfigPartialDoesNotClearExisting(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	initial := []byte(`{
		"env": {
			"ANTHROPIC_API_KEY": "sk-existing",
			"ANTHROPIC_BASE_URL": "https://existing.example.com"
		}
	}`)
	if err := os.WriteFile(settingsPath, initial, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	adapter := NewClaudeAdapter(dir, filepath.Join(dir, ".claude.json"))
	_, err := adapter.SetProviderConfig(&ProviderConfig{Model: "claude-3-7-sonnet"})
	if err != nil {
		t.Fatalf("SetProviderConfig() error = %v", err)
	}

	updatedBytes, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	var updated map[string]any
	if err := json.Unmarshal(updatedBytes, &updated); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	env, ok := updated["env"].(map[string]any)
	if !ok {
		t.Fatalf("env missing or invalid type")
	}

	if env["ANTHROPIC_API_KEY"] != "sk-existing" {
		t.Fatalf("env.ANTHROPIC_API_KEY = %v, want %q", env["ANTHROPIC_API_KEY"], "sk-existing")
	}
	if env["ANTHROPIC_BASE_URL"] != "https://existing.example.com" {
		t.Fatalf("env.ANTHROPIC_BASE_URL = %v, want %q", env["ANTHROPIC_BASE_URL"], "https://existing.example.com")
	}
}

func TestClaudeAdapterProviderConfigErrorsOnNonObjectEnv(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")
	badSettings := []byte(`{"env":"invalid"}`)
	if err := os.WriteFile(settingsPath, badSettings, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	adapter := NewClaudeAdapter(dir, filepath.Join(dir, ".claude.json"))

	_, err := adapter.GetProviderConfig()
	if err == nil {
		t.Fatalf("GetProviderConfig() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "env") {
		t.Fatalf("GetProviderConfig() error = %q, want mention env", err)
	}

	_, err = adapter.SetProviderConfig(&ProviderConfig{APIKey: "sk-new"})
	if err == nil {
		t.Fatalf("SetProviderConfig() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "env") {
		t.Fatalf("SetProviderConfig() error = %q, want mention env", err)
	}
}

func TestClaudeAdapterMCPServersReadWritePreservesTopLevel(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	claudePath := filepath.Join(dir, ".claude.json")
	initial := []byte(`{
		"projectName": "demo",
		"hooks": {"onStart": ["echo start"]},
		"mcpServers": {
			"filesystem": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
				"env": {"NODE_ENV": "test"}
			}
		}
	}`)
	if err := os.WriteFile(claudePath, initial, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	adapter := NewClaudeAdapter(dir, claudePath)
	servers, err := adapter.GetMCPServers()
	if err != nil {
		t.Fatalf("GetMCPServers() error = %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("len(servers) = %d, want 1", len(servers))
	}

	if servers[0].Name != "filesystem" {
		t.Fatalf("servers[0].Name = %q, want %q", servers[0].Name, "filesystem")
	}
	if servers[0].Command != "npx" {
		t.Fatalf("servers[0].Command = %q, want %q", servers[0].Command, "npx")
	}

	_, err = adapter.SetMCPServers([]MCPServerConfig{
		{Name: "github", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}},
		{Name: "memory", Command: "node", Args: []string{"memory.js"}, Env: map[string]string{"TOKEN": "abc"}},
	})
	if err != nil {
		t.Fatalf("SetMCPServers() error = %v", err)
	}

	updatedBytes, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	var updated map[string]any
	if err := json.Unmarshal(updatedBytes, &updated); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if updated["projectName"] != "demo" {
		t.Fatalf("projectName = %v, want %q", updated["projectName"], "demo")
	}
	if _, ok := updated["hooks"].(map[string]any); !ok {
		t.Fatalf("hooks missing or invalid type")
	}

	mcpServers, ok := updated["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers missing or invalid type")
	}
	if len(mcpServers) != 2 {
		t.Fatalf("len(mcpServers) = %d, want 2", len(mcpServers))
	}

	github, ok := mcpServers["github"].(map[string]any)
	if !ok {
		t.Fatalf("github server missing or invalid type")
	}
	if github["command"] != "npx" {
		t.Fatalf("github.command = %v, want %q", github["command"], "npx")
	}

	memory, ok := mcpServers["memory"].(map[string]any)
	if !ok {
		t.Fatalf("memory server missing or invalid type")
	}
	if memory["command"] != "node" {
		t.Fatalf("memory.command = %v, want %q", memory["command"], "node")
	}
	memoryEnv, ok := memory["env"].(map[string]any)
	if !ok || memoryEnv["TOKEN"] != "abc" {
		t.Fatalf("memory.env = %v, want TOKEN=abc", memory["env"])
	}
}

func TestClaudeAdapterSetMCPServersPreservesUnknownFieldsOnExistingServer(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	claudePath := filepath.Join(dir, ".claude.json")
	initial := []byte(`{
		"mcpServers": {
			"filesystem": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
				"disabled": true,
				"metadata": {"owner": "team-a"}
			}
		}
	}`)
	if err := os.WriteFile(claudePath, initial, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	adapter := NewClaudeAdapter(dir, claudePath)
	_, err := adapter.SetMCPServers([]MCPServerConfig{{
		Name:    "filesystem",
		Command: "node",
		Args:    []string{"server.js"},
	}})
	if err != nil {
		t.Fatalf("SetMCPServers() error = %v", err)
	}

	updatedBytes, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	var updated map[string]any
	if err := json.Unmarshal(updatedBytes, &updated); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	mcpServers, ok := updated["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers missing or invalid type")
	}
	filesystem, ok := mcpServers["filesystem"].(map[string]any)
	if !ok {
		t.Fatalf("filesystem missing or invalid type")
	}

	if filesystem["command"] != "node" {
		t.Fatalf("filesystem.command = %v, want %q", filesystem["command"], "node")
	}
	if filesystem["disabled"] != true {
		t.Fatalf("filesystem.disabled = %v, want true", filesystem["disabled"])
	}
	if _, ok := filesystem["metadata"].(map[string]any); !ok {
		t.Fatalf("filesystem.metadata missing or invalid type")
	}
}

func TestClaudeAdapterGetMCPServersSkipsHTTPServers(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	claudePath := filepath.Join(dir, ".claude.json")
	config := []byte(`{
		"mcpServers": {
			"tavily": {
				"type": "http",
				"url": "https://mcp.tavily.com/server"
			},
			"filesystem": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
			}
		}
	}`)
	if err := os.WriteFile(claudePath, config, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	adapter := NewClaudeAdapter(dir, claudePath)
	servers, err := adapter.GetMCPServers()
	if err != nil {
		t.Fatalf("GetMCPServers() error = %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("len(servers) = %d, want 1 managed stdio server", len(servers))
	}
	if servers[0].Name != "filesystem" {
		t.Fatalf("servers[0].Name = %q, want %q", servers[0].Name, "filesystem")
	}
}

func TestClaudeAdapterSetMCPServersPreservesHTTPServers(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	claudePath := filepath.Join(dir, ".claude.json")
	initial := []byte(`{
		"mcpServers": {
			"tavily": {
				"type": "http",
				"url": "https://mcp.tavily.com/server",
				"headers": {"Authorization": "Bearer token"}
			},
			"filesystem": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"]
			}
		}
	}`)
	if err := os.WriteFile(claudePath, initial, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	adapter := NewClaudeAdapter(dir, claudePath)
	_, err := adapter.SetMCPServers([]MCPServerConfig{{
		Name:    "github",
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-github"},
	}})
	if err != nil {
		t.Fatalf("SetMCPServers() error = %v", err)
	}

	updatedBytes, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	var updated map[string]any
	if err := json.Unmarshal(updatedBytes, &updated); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	mcpServers, ok := updated["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers missing or invalid type")
	}
	if len(mcpServers) != 2 {
		t.Fatalf("len(mcpServers) = %d, want 2", len(mcpServers))
	}

	tavily, ok := mcpServers["tavily"].(map[string]any)
	if !ok {
		t.Fatalf("tavily server missing or invalid type")
	}
	if tavily["type"] != "http" {
		t.Fatalf("tavily.type = %v, want %q", tavily["type"], "http")
	}
	if tavily["url"] != "https://mcp.tavily.com/server" {
		t.Fatalf("tavily.url = %v, want tavily URL", tavily["url"])
	}

	github, ok := mcpServers["github"].(map[string]any)
	if !ok {
		t.Fatalf("github server missing or invalid type")
	}
	if github["command"] != "npx" {
		t.Fatalf("github.command = %v, want %q", github["command"], "npx")
	}
	if _, ok := mcpServers["filesystem"]; ok {
		t.Fatalf("filesystem should be removed when not managed anymore: %v", mcpServers["filesystem"])
	}
}

func TestClaudeAdapterMCPServersErrorsOnNonObjectMCPServers(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	claudePath := filepath.Join(dir, ".claude.json")
	badConfig := []byte(`{"mcpServers":"invalid"}`)
	if err := os.WriteFile(claudePath, badConfig, 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	adapter := NewClaudeAdapter(dir, claudePath)

	_, err := adapter.GetMCPServers()
	if err == nil {
		t.Fatalf("GetMCPServers() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "mcpServers") {
		t.Fatalf("GetMCPServers() error = %q, want mention mcpServers", err)
	}

	_, err = adapter.SetMCPServers([]MCPServerConfig{{Name: "github", Command: "npx"}})
	if err == nil {
		t.Fatalf("SetMCPServers() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "mcpServers") {
		t.Fatalf("SetMCPServers() error = %q, want mention mcpServers", err)
	}
}

func TestClaudeAdapterGetMCPServersErrorsOnMalformedServerFields(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		serverObj string
	}{
		{name: "non-string command", serverObj: `{"command":123}`},
		{name: "non-string args entry", serverObj: `{"command":"npx","args":["-y",123]}`},
		{name: "non-string env value", serverObj: `{"command":"npx","env":{"TOKEN":123}}`},
		{name: "http missing url", serverObj: `{"type":"http"}`},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			claudePath := filepath.Join(dir, ".claude.json")
			config := []byte(`{"mcpServers":{"filesystem":` + testCase.serverObj + `}}`)
			if err := os.WriteFile(claudePath, config, 0o644); err != nil {
				t.Fatalf("os.WriteFile() error = %v", err)
			}

			adapter := NewClaudeAdapter(dir, claudePath)
			_, err := adapter.GetMCPServers()
			if err == nil {
				t.Fatalf("GetMCPServers() error = nil, want non-nil")
			}
		})
	}
}
