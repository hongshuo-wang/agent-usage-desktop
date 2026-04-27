package configmanager

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

const codexDocURL = "https://github.com/openai/codex"

type CodexAdapter struct {
	codexDir string
}

func NewCodexAdapter(codexDir string) *CodexAdapter {
	return &CodexAdapter{codexDir: codexDir}
}

func (a *CodexAdapter) Tool() string {
	return "codex"
}

func (a *CodexAdapter) IsInstalled() bool {
	if a.codexDir != "" {
		if info, err := os.Stat(a.codexDir); err == nil && info.IsDir() {
			return true
		}
	}

	if fileExists(a.authPath()) || fileExists(a.configPath()) {
		return true
	}

	return false
}

func (a *CodexAdapter) GetProviderConfig() (*ProviderConfig, error) {
	auth, err := a.readJSONMap(a.authPath())
	if err != nil {
		return nil, err
	}
	config, err := a.readTOMLMap(a.configPath())
	if err != nil {
		return nil, err
	}

	apiKey := toString(auth["api_key"])
	if apiKey == "" {
		apiKey = toString(auth["OPENAI_API_KEY"])
	}

	provider, err := objectField(config, "provider", "config.toml")
	if err != nil {
		return nil, err
	}

	_, modelName, _, err := codexModelField(config)
	if err != nil {
		return nil, err
	}

	cfg := &ProviderConfig{APIKey: apiKey, Model: modelName}
	if provider != nil {
		cfg.BaseURL = toString(provider["base_url"])
	}

	return cfg, nil
}

func (a *CodexAdapter) SetProviderConfig(cfg *ProviderConfig) ([]AffectedFile, error) {
	if cfg == nil {
		cfg = &ProviderConfig{}
	}

	auth, err := a.readJSONMap(a.authPath())
	if err != nil {
		return nil, err
	}
	config, err := a.readTOMLMap(a.configPath())
	if err != nil {
		return nil, err
	}

	if cfg.APIKey != "" {
		auth["api_key"] = cfg.APIKey
		auth["OPENAI_API_KEY"] = cfg.APIKey
	}

	provider, err := objectField(config, "provider", "config.toml")
	if err != nil {
		return nil, err
	}
	if provider == nil {
		provider = make(map[string]any)
	}
	if cfg.BaseURL != "" {
		provider["base_url"] = cfg.BaseURL
	}
	if len(provider) > 0 {
		config["provider"] = provider
	}

	modelSection, _, modelKind, err := codexModelField(config)
	if err != nil {
		return nil, err
	}
	if cfg.Model != "" {
		switch modelKind {
		case codexModelString:
			config["model"] = cfg.Model
		default:
			if modelSection == nil {
				modelSection = make(map[string]any)
			}
			modelSection["name"] = cfg.Model
		}
	}
	if modelKind != codexModelString && len(modelSection) > 0 {
		config["model"] = modelSection
	}

	if err := a.writeJSONMap(a.authPath(), auth); err != nil {
		return nil, err
	}
	if err := a.writeTOMLMap(a.configPath(), config); err != nil {
		return nil, err
	}

	return []AffectedFile{
		{Path: a.authPath(), Tool: a.Tool(), Operation: "write"},
		{Path: a.configPath(), Tool: a.Tool(), Operation: "write"},
	}, nil
}

func (a *CodexAdapter) GetMCPServers() ([]MCPServerConfig, error) {
	config, err := a.readTOMLMap(a.configPath())
	if err != nil {
		return nil, err
	}

	mcpServers, err := objectField(config, "mcp_servers", "config.toml")
	if err != nil {
		return nil, err
	}
	if mcpServers == nil {
		return nil, nil
	}

	servers := make([]MCPServerConfig, 0, len(mcpServers))
	for name, raw := range mcpServers {
		entry, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("config.toml mcp_servers.%s must be an object", name)
		}

		command, err := requiredStringField(entry, "command", "config.toml mcp_servers."+name)
		if err != nil {
			return nil, err
		}
		args, err := optionalStringSliceField(entry, "args", "config.toml mcp_servers."+name)
		if err != nil {
			return nil, err
		}
		env, err := optionalStringMapField(entry, "env", "config.toml mcp_servers."+name)
		if err != nil {
			return nil, err
		}

		servers = append(servers, MCPServerConfig{
			Name:    name,
			Command: command,
			Args:    args,
			Env:     env,
		})
	}

	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Name < servers[j].Name
	})

	return servers, nil
}

func (a *CodexAdapter) SetMCPServers(servers []MCPServerConfig) ([]AffectedFile, error) {
	config, err := a.readTOMLMap(a.configPath())
	if err != nil {
		return nil, err
	}

	existingMCPServers, err := objectField(config, "mcp_servers", "config.toml")
	if err != nil {
		return nil, err
	}

	mcpServers := make(map[string]any, len(servers))
	for _, server := range servers {
		if server.Name == "" {
			continue
		}

		entry := make(map[string]any)
		if existingMCPServers != nil {
			if existing, found := existingMCPServers[server.Name]; found {
				existingEntry, ok := existing.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("config.toml mcp_servers.%s must be an object", server.Name)
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

	config["mcp_servers"] = mcpServers
	if err := a.writeTOMLMap(a.configPath(), config); err != nil {
		return nil, err
	}

	return []AffectedFile{{
		Path:      a.configPath(),
		Tool:      a.Tool(),
		Operation: "write",
	}}, nil
}

func (a *CodexAdapter) GetSkillPaths() []string {
	if a.codexDir == "" {
		return nil
	}

	return []string{filepath.Join(a.codexDir, "skills")}
}

func (a *CodexAdapter) ConfigFiles() []ConfigFileInfo {
	authPath := a.authPath()
	configPath := a.configPath()

	return []ConfigFileInfo{
		{
			Path:        authPath,
			Tool:        a.Tool(),
			Description: "Codex auth",
			DocURL:      codexDocURL,
			Exists:      fileExists(authPath),
		},
		{
			Path:        configPath,
			Tool:        a.Tool(),
			Description: "Codex config",
			DocURL:      codexDocURL,
			Exists:      fileExists(configPath),
		},
	}
}

func (a *CodexAdapter) authPath() string {
	return filepath.Join(a.codexDir, "auth.json")
}

func (a *CodexAdapter) configPath() string {
	return filepath.Join(a.codexDir, "config.toml")
}

func (a *CodexAdapter) readJSONMap(path string) (map[string]any, error) {
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

func (a *CodexAdapter) writeJSONMap(path string, data map[string]any) error {
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

func (a *CodexAdapter) readTOMLMap(path string) (map[string]any, error) {
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
	if err := toml.Unmarshal(content, &parsed); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if parsed == nil {
		return map[string]any{}, nil
	}
	return parsed, nil
}

func (a *CodexAdapter) writeTOMLMap(path string, data map[string]any) error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(data); err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}

	if err := AtomicWrite(path, buf.Bytes()); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

type codexModelKind int

const (
	codexModelMissing codexModelKind = iota
	codexModelString
	codexModelObject
)

func codexModelField(config map[string]any) (map[string]any, string, codexModelKind, error) {
	value, exists := config["model"]
	if !exists || value == nil {
		return nil, "", codexModelMissing, nil
	}

	switch typed := value.(type) {
	case string:
		return nil, typed, codexModelString, nil
	case map[string]any:
		modelName := toString(typed["name"])
		if modelName == "" {
			modelName = toString(typed["default"])
		}
		return typed, modelName, codexModelObject, nil
	default:
		return nil, "", codexModelMissing, fmt.Errorf("config.toml model must be a string or object")
	}
}
