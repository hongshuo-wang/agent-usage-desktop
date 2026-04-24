package configmanager

const openClawDocURL = "https://docs.openclaw.ai/gateway/configuration"

type OpenClawAdapter struct {
	base *jsonToolAdapter
}

func NewOpenClawAdapter(configPath string) *OpenClawAdapter {
	return &OpenClawAdapter{
		base: newJSONToolAdapter("openclaw", configPath, openClawDocURL, "OpenClaw config", "skills"),
	}
}

func (a *OpenClawAdapter) Tool() string {
	return a.base.Tool()
}

func (a *OpenClawAdapter) IsInstalled() bool {
	return a.base.IsInstalled()
}

func (a *OpenClawAdapter) GetProviderConfig() (*ProviderConfig, error) {
	return a.base.GetProviderConfig()
}

func (a *OpenClawAdapter) SetProviderConfig(cfg *ProviderConfig) ([]AffectedFile, error) {
	return a.base.SetProviderConfig(cfg)
}

func (a *OpenClawAdapter) GetMCPServers() ([]MCPServerConfig, error) {
	return a.base.GetMCPServers()
}

func (a *OpenClawAdapter) SetMCPServers(servers []MCPServerConfig) ([]AffectedFile, error) {
	return a.base.SetMCPServers(servers)
}

func (a *OpenClawAdapter) GetSkillPaths() []string {
	return a.base.GetSkillPaths()
}

func (a *OpenClawAdapter) ConfigFiles() []ConfigFileInfo {
	return a.base.ConfigFiles()
}
