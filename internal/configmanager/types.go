package configmanager

type ProviderConfig struct {
	APIKey   string            `json:"api_key"`
	BaseURL  string            `json:"base_url"`
	Model    string            `json:"model"`
	ModelMap map[string]string `json:"model_map,omitempty"`
}

type MCPServerConfig struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type AffectedFile struct {
	Path      string `json:"path"`
	Tool      string `json:"tool"`
	Operation string `json:"operation"`
	Diff      string `json:"diff,omitempty"`
}

type ConfigFileInfo struct {
	Path        string `json:"path"`
	Tool        string `json:"tool"`
	Description string `json:"description"`
	DocURL      string `json:"doc_url"`
	Exists      bool   `json:"exists"`
}

type Adapter interface {
	Tool() string
	IsInstalled() bool
	GetProviderConfig() (*ProviderConfig, error)
	SetProviderConfig(cfg *ProviderConfig) ([]AffectedFile, error)
	GetMCPServers() ([]MCPServerConfig, error)
	SetMCPServers(servers []MCPServerConfig) ([]AffectedFile, error)
	GetSkillPaths() []string
	ConfigFiles() []ConfigFileInfo
}
