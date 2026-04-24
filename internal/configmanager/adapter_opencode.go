package configmanager

const openCodeDocURL = "https://opencode.ai/docs/config"

type OpenCodeAdapter struct {
	base *jsonToolAdapter
}

func NewOpenCodeAdapter(configPath string) *OpenCodeAdapter {
	return &OpenCodeAdapter{
		base: newJSONToolAdapter("opencode", configPath, openCodeDocURL, "OpenCode config", "skills"),
	}
}

func (a *OpenCodeAdapter) Tool() string {
	return a.base.Tool()
}

func (a *OpenCodeAdapter) IsInstalled() bool {
	return a.base.IsInstalled()
}

func (a *OpenCodeAdapter) GetProviderConfig() (*ProviderConfig, error) {
	return a.base.GetProviderConfig()
}

func (a *OpenCodeAdapter) SetProviderConfig(cfg *ProviderConfig) ([]AffectedFile, error) {
	return a.base.SetProviderConfig(cfg)
}

func (a *OpenCodeAdapter) GetMCPServers() ([]MCPServerConfig, error) {
	return a.base.GetMCPServers()
}

func (a *OpenCodeAdapter) SetMCPServers(servers []MCPServerConfig) ([]AffectedFile, error) {
	return a.base.SetMCPServers(servers)
}

func (a *OpenCodeAdapter) GetSkillPaths() []string {
	return a.base.GetSkillPaths()
}

func (a *OpenCodeAdapter) ConfigFiles() []ConfigFileInfo {
	return a.base.ConfigFiles()
}
