package configmanager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const (
	claudeSettingsDocURL = "https://docs.anthropic.com/en/docs/claude-code/settings"
	claudeMCPDocURL      = "https://docs.anthropic.com/en/docs/claude-code/mcp"
)

type ClaudeAdapter struct {
	settingsDir    string
	claudeJSONPath string
}

func NewClaudeAdapter(settingsDir string, claudeJSONPath string) *ClaudeAdapter {
	return &ClaudeAdapter{settingsDir: settingsDir, claudeJSONPath: claudeJSONPath}
}

func (a *ClaudeAdapter) Tool() string {
	return "claude"
}

func (a *ClaudeAdapter) IsInstalled() bool {
	if a.settingsDir != "" {
		if info, err := os.Stat(a.settingsDir); err == nil && info.IsDir() {
			return true
		}
	}

	if a.claudeJSONPath != "" {
		if _, err := os.Stat(a.claudeJSONPath); err == nil {
			return true
		}
	}

	return false
}

func (a *ClaudeAdapter) GetProviderConfig() (*ProviderConfig, error) {
	settings, err := a.readJSONMap(a.settingsPath())
	if err != nil {
		return nil, err
	}

	env, err := objectField(settings, "env", "settings.json")
	if err != nil {
		return nil, err
	}
	cfg := &ProviderConfig{
		APIKey:  toString(env["ANTHROPIC_API_KEY"]),
		BaseURL: toString(env["ANTHROPIC_BASE_URL"]),
		Model:   toString(env["ANTHROPIC_MODEL"]),
	}

	return cfg, nil
}

func (a *ClaudeAdapter) SetProviderConfig(cfg *ProviderConfig) ([]AffectedFile, error) {
	if cfg == nil {
		cfg = &ProviderConfig{}
	}

	settings, err := a.readJSONMap(a.settingsPath())
	if err != nil {
		return nil, err
	}

	env, err := objectField(settings, "env", "settings.json")
	if err != nil {
		return nil, err
	}
	if env == nil {
		env = make(map[string]any)
	}
	if cfg.APIKey != "" {
		env["ANTHROPIC_API_KEY"] = cfg.APIKey
	}
	if cfg.BaseURL != "" {
		env["ANTHROPIC_BASE_URL"] = cfg.BaseURL
	}
	if cfg.Model != "" {
		env["ANTHROPIC_MODEL"] = cfg.Model
	}
	settings["env"] = env

	if err := a.writeJSONMap(a.settingsPath(), settings); err != nil {
		return nil, err
	}

	return []AffectedFile{{
		Path:      a.settingsPath(),
		Tool:      a.Tool(),
		Operation: "write",
	}}, nil
}

func (a *ClaudeAdapter) GetMCPServers() ([]MCPServerConfig, error) {
	config, err := a.readJSONMap(a.claudeJSONPath)
	if err != nil {
		return nil, err
	}

	mcpServers, err := objectField(config, "mcpServers", ".claude.json")
	if err != nil {
		return nil, err
	}
	if mcpServers == nil {
		return nil, nil
	}

	servers := make([]MCPServerConfig, 0, len(mcpServers))
	for name, raw := range mcpServers {
		server, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf(".claude.json mcpServers.%s must be an object", name)
		}
		if isClaudeHTTPMCPServer(server) {
			continue
		}

		command, err := requiredStringField(server, "command", ".claude.json mcpServers."+name)
		if err != nil {
			return nil, err
		}
		args, err := optionalStringSliceField(server, "args", ".claude.json mcpServers."+name)
		if err != nil {
			return nil, err
		}
		env, err := optionalStringMapField(server, "env", ".claude.json mcpServers."+name)
		if err != nil {
			return nil, err
		}

		item := MCPServerConfig{
			Name:    name,
			Command: command,
			Args:    args,
			Env:     env,
		}
		servers = append(servers, item)
	}

	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Name < servers[j].Name
	})

	return servers, nil
}

func (a *ClaudeAdapter) SetMCPServers(servers []MCPServerConfig) ([]AffectedFile, error) {
	config, err := a.readJSONMap(a.claudeJSONPath)
	if err != nil {
		return nil, err
	}

	existingMCPServers, err := objectField(config, "mcpServers", ".claude.json")
	if err != nil {
		return nil, err
	}

	mcpServers := make(map[string]any, len(servers))
	for name, raw := range existingMCPServers {
		existingEntry, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if !isClaudeHTTPMCPServer(existingEntry) {
			continue
		}
		mcpServers[name] = cloneStringAnyMap(existingEntry)
	}

	for _, server := range servers {
		if server.Name == "" {
			continue
		}

		entry := make(map[string]any)
		if existingMCPServers != nil {
			if existing, found := existingMCPServers[server.Name]; found {
				existingEntry, ok := existing.(map[string]any)
				if !ok {
					return nil, fmt.Errorf(".claude.json mcpServers.%s must be an object", server.Name)
				}
				for key, value := range existingEntry {
					entry[key] = value
				}
			}
		}

		entry["command"] = server.Command
		if len(server.Args) > 0 {
			entry["args"] = server.Args
		} else {
			delete(entry, "args")
		}
		if len(server.Env) > 0 {
			entry["env"] = server.Env
		} else {
			delete(entry, "env")
		}
		mcpServers[server.Name] = entry
	}

	config["mcpServers"] = mcpServers
	if err := a.writeJSONMap(a.claudeJSONPath, config); err != nil {
		return nil, err
	}

	return []AffectedFile{{
		Path:      a.claudeJSONPath,
		Tool:      a.Tool(),
		Operation: "write",
	}}, nil
}

func (a *ClaudeAdapter) GetSkillPaths() []string {
	if a.settingsDir == "" {
		return nil
	}
	return []string{filepath.Join(a.settingsDir, "skills")}
}

func (a *ClaudeAdapter) ConfigFiles() []ConfigFileInfo {
	settingsPath := a.settingsPath()
	return []ConfigFileInfo{
		{
			Path:        settingsPath,
			Tool:        a.Tool(),
			Description: "Claude Code settings",
			DocURL:      claudeSettingsDocURL,
			Exists:      fileExists(settingsPath),
		},
		{
			Path:        a.claudeJSONPath,
			Tool:        a.Tool(),
			Description: "Claude Code MCP servers",
			DocURL:      claudeMCPDocURL,
			Exists:      fileExists(a.claudeJSONPath),
		},
	}
}

func (a *ClaudeAdapter) settingsPath() string {
	return filepath.Join(a.settingsDir, "settings.json")
}

func (a *ClaudeAdapter) readJSONMap(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	if len(content) == 0 {
		return map[string]any{}, nil
	}

	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if parsed == nil {
		return map[string]any{}, nil
	}
	return parsed, nil
}

func (a *ClaudeAdapter) writeJSONMap(path string, data map[string]any) error {
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	encoded = append(encoded, '\n')

	if err := AtomicWrite(path, encoded); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func objectField(parent map[string]any, field string, fileLabel string) (map[string]any, error) {
	value, exists := parent[field]
	if !exists || value == nil {
		return nil, nil
	}
	mapped, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s %s must be an object", fileLabel, field)
	}
	return mapped, nil
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	str, ok := value.(string)
	if !ok {
		return ""
	}
	return str
}

func requiredStringField(parent map[string]any, field string, objectPath string) (string, error) {
	value, exists := parent[field]
	if !exists {
		return "", fmt.Errorf("%s.%s is required", objectPath, field)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s.%s must be a string", objectPath, field)
	}
	return text, nil
}

func optionalStringSliceField(parent map[string]any, field string, objectPath string) ([]string, error) {
	value, exists := parent[field]
	if !exists || value == nil {
		return nil, nil
	}

	raw, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s.%s must be an array of strings", objectPath, field)
	}

	result := make([]string, 0, len(raw))
	for index, item := range raw {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s.%s[%d] must be a string", objectPath, field, index)
		}
		result = append(result, text)
	}
	return result, nil
}

func optionalStringMapField(parent map[string]any, field string, objectPath string) (map[string]string, error) {
	value, exists := parent[field]
	if !exists || value == nil {
		return nil, nil
	}

	raw, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s.%s must be an object of strings", objectPath, field)
	}

	result := make(map[string]string, len(raw))
	for key, item := range raw {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s.%s.%s must be a string", objectPath, field, key)
		}
		result[key] = text
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func isClaudeHTTPMCPServer(server map[string]any) bool {
	return toString(server["type"]) == "http" && toString(server["url"]) != ""
}

func cloneStringAnyMap(value map[string]any) map[string]any {
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
