package configmanager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type jsonToolAdapter struct {
	tool        string
	configPath  string
	docURL      string
	description string
	skillsDir   string
}

func newJSONToolAdapter(tool string, configPath string, docURL string, description string, skillsDir string) *jsonToolAdapter {
	return &jsonToolAdapter{
		tool:        tool,
		configPath:  configPath,
		docURL:      docURL,
		description: description,
		skillsDir:   skillsDir,
	}
}

func (a *jsonToolAdapter) Tool() string {
	return a.tool
}

func (a *jsonToolAdapter) IsInstalled() bool {
	if a.configPath == "" {
		return false
	}
	_, err := os.Stat(a.configPath)
	return err == nil
}

func (a *jsonToolAdapter) GetProviderConfig() (*ProviderConfig, error) {
	config, err := a.readJSONMap()
	if err != nil {
		return nil, err
	}

	provider, err := objectField(config, "provider", a.label())
	if err != nil {
		return nil, err
	}

	cfg := &ProviderConfig{}
	if provider == nil {
		return cfg, nil
	}

	if cfg.APIKey, err = optionalStrictStringField(provider, "api_key", a.label()+" provider"); err != nil {
		return nil, err
	}
	if cfg.BaseURL, err = optionalStrictStringField(provider, "base_url", a.label()+" provider"); err != nil {
		return nil, err
	}
	if cfg.Model, err = optionalStrictStringField(provider, "model", a.label()+" provider"); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (a *jsonToolAdapter) SetProviderConfig(cfg *ProviderConfig) ([]AffectedFile, error) {
	if cfg == nil {
		cfg = &ProviderConfig{}
	}

	config, err := a.readJSONMap()
	if err != nil {
		return nil, err
	}

	provider, err := objectField(config, "provider", a.label())
	if err != nil {
		return nil, err
	}
	if provider == nil {
		provider = make(map[string]any)
	}

	if err := validateProviderScalarFields(provider, a.label()+" provider"); err != nil {
		return nil, err
	}

	if cfg.APIKey != "" {
		provider["api_key"] = cfg.APIKey
	}
	if cfg.BaseURL != "" {
		provider["base_url"] = cfg.BaseURL
	}
	if cfg.Model != "" {
		provider["model"] = cfg.Model
	}
	if len(provider) > 0 {
		config["provider"] = provider
	}

	if err := a.writeJSONMap(config); err != nil {
		return nil, err
	}

	return []AffectedFile{{
		Path:      a.configPath,
		Tool:      a.tool,
		Operation: "write",
	}}, nil
}

func (a *jsonToolAdapter) GetMCPServers() ([]MCPServerConfig, error) {
	config, err := a.readJSONMap()
	if err != nil {
		return nil, err
	}

	mcp, err := objectField(config, "mcp", a.label())
	if err != nil {
		return nil, err
	}
	if mcp == nil {
		return nil, nil
	}

	servers := make([]MCPServerConfig, 0, len(mcp))
	for name, raw := range mcp {
		entry, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s mcp.%s must be an object", a.label(), name)
		}

		command, err := requiredStringField(entry, "command", a.label()+" mcp."+name)
		if err != nil {
			return nil, err
		}
		args, err := optionalStringSliceField(entry, "args", a.label()+" mcp."+name)
		if err != nil {
			return nil, err
		}
		env, err := optionalStringMapField(entry, "env", a.label()+" mcp."+name)
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

func (a *jsonToolAdapter) SetMCPServers(servers []MCPServerConfig) ([]AffectedFile, error) {
	config, err := a.readJSONMap()
	if err != nil {
		return nil, err
	}

	existingMCP, err := objectField(config, "mcp", a.label())
	if err != nil {
		return nil, err
	}

	mcp := make(map[string]any, len(servers))
	for _, server := range servers {
		if server.Name == "" {
			continue
		}

		entry := make(map[string]any)
		if existingMCP != nil {
			if existingRaw, found := existingMCP[server.Name]; found {
				existing, ok := existingRaw.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("%s mcp.%s must be an object", a.label(), server.Name)
				}
				for key, value := range existing {
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
		mcp[server.Name] = entry
	}

	config["mcp"] = mcp
	if err := a.writeJSONMap(config); err != nil {
		return nil, err
	}

	return []AffectedFile{{
		Path:      a.configPath,
		Tool:      a.tool,
		Operation: "write",
	}}, nil
}

func (a *jsonToolAdapter) GetSkillPaths() []string {
	if a.configPath == "" {
		return nil
	}

	skillsDir := a.skillsDir
	if skillsDir == "" {
		skillsDir = "skills"
	}
	return []string{filepath.Join(filepath.Dir(a.configPath), skillsDir)}
}

func (a *jsonToolAdapter) ConfigFiles() []ConfigFileInfo {
	return []ConfigFileInfo{{
		Path:        a.configPath,
		Tool:        a.tool,
		Description: a.description,
		DocURL:      a.docURL,
		Exists:      fileExists(a.configPath),
	}}
}

func (a *jsonToolAdapter) label() string {
	if a.configPath != "" {
		return a.configPath
	}
	return a.tool + ".json"
}

func (a *jsonToolAdapter) readJSONMap() (map[string]any, error) {
	content, err := os.ReadFile(a.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", a.configPath, err)
	}

	if len(content) == 0 {
		return map[string]any{}, nil
	}

	var parsed map[string]any
	if err := json.Unmarshal(content, &parsed); err != nil {
		return nil, fmt.Errorf("parse %s: %w", a.configPath, err)
	}
	if parsed == nil {
		return map[string]any{}, nil
	}
	return parsed, nil
}

func (a *jsonToolAdapter) writeJSONMap(data map[string]any) error {
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", a.configPath, err)
	}
	encoded = append(encoded, '\n')

	if err := AtomicWrite(a.configPath, encoded); err != nil {
		return fmt.Errorf("write %s: %w", a.configPath, err)
	}
	return nil
}

func validateProviderScalarFields(provider map[string]any, objectPath string) error {
	if _, err := optionalStrictStringField(provider, "api_key", objectPath); err != nil {
		return err
	}
	if _, err := optionalStrictStringField(provider, "base_url", objectPath); err != nil {
		return err
	}
	if _, err := optionalStrictStringField(provider, "model", objectPath); err != nil {
		return err
	}
	return nil
}

func optionalStrictStringField(parent map[string]any, field string, objectPath string) (string, error) {
	value, exists := parent[field]
	if !exists || value == nil {
		return "", nil
	}

	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s.%s must be a string", objectPath, field)
	}
	return text, nil
}
