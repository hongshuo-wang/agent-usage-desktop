package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/configmanager"
	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

type createProfileReq struct {
	Name        string          `json:"name"`
	Config      string          `json:"config"`
	ToolTargets map[string]bool `json:"tool_targets"`
}

type updateProfileReq struct {
	Name        string          `json:"name"`
	Config      string          `json:"config"`
	ToolTargets map[string]bool `json:"tool_targets"`
}

type activateResp struct {
	AffectedFiles []configmanager.AffectedFile `json:"affected_files"`
}

type profileResp struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	IsActive    bool            `json:"is_active"`
	Config      string          `json:"config"`
	HasAPIKey   bool            `json:"has_api_key"`
	ToolTargets map[string]bool `json:"tool_targets"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type createMCPReq struct {
	Name    string          `json:"name"`
	Command string          `json:"command"`
	Args    string          `json:"args"`
	Env     string          `json:"env"`
	Enabled *bool           `json:"enabled"`
	Targets map[string]bool `json:"targets"`
}

type updateMCPReq struct {
	Name    string          `json:"name"`
	Command string          `json:"command"`
	Args    string          `json:"args"`
	Env     string          `json:"env"`
	Enabled bool            `json:"enabled"`
	Targets map[string]bool `json:"targets"`
}

type setTargetsReq struct {
	Targets map[string]bool `json:"targets"`
}

type mcpServerResp struct {
	ID        int64           `json:"id"`
	Name      string          `json:"name"`
	Command   string          `json:"command"`
	Args      string          `json:"args"`
	Env       string          `json:"env"`
	Enabled   bool            `json:"enabled"`
	Targets   map[string]bool `json:"targets"`
	CreatedAt time.Time       `json:"created_at"`
}

type createSkillReq struct {
	Name        string                    `json:"name"`
	SourcePath  string                    `json:"source_path"`
	Description string                    `json:"description"`
	Targets     map[string]skillTargetReq `json:"targets"`
}

type updateSkillReq struct {
	Name        string                    `json:"name"`
	SourcePath  string                    `json:"source_path"`
	Description string                    `json:"description"`
	Enabled     bool                      `json:"enabled"`
	Targets     map[string]skillTargetReq `json:"targets"`
}

type skillTargetReq struct {
	Method    string `json:"method"`
	Enabled   bool   `json:"enabled"`
	VariantID int64  `json:"variant_id"`
}

type setSkillTargetsReq struct {
	Targets map[string]skillTargetReq `json:"targets"`
}

type resolveSkillConflictReq struct {
	Name         string `json:"name"`
	Tool         string `json:"tool"`
	LibraryPath  string `json:"library_path"`
	ExternalPath string `json:"external_path"`
	Direction    string `json:"direction"`
}

type importManagedSkillReq struct {
	SkillID    int64  `json:"skill_id"`
	Name       string `json:"name"`
	Tool       string `json:"tool"`
	SourcePath string `json:"source_path"`
}

type agentUsageSkillActionReq struct {
	Agents []string `json:"agents"`
}

type importManagedSkillResp struct {
	SkillID       int64                        `json:"skill_id"`
	VariantID     int64                        `json:"variant_id"`
	CreatedNew    bool                         `json:"created_new"`
	AffectedFiles []configmanager.AffectedFile `json:"affected_files"`
}

type skillResp struct {
	ID          int64                     `json:"id"`
	Name        string                    `json:"name"`
	SourcePath  string                    `json:"source_path"`
	Description string                    `json:"description"`
	Enabled     bool                      `json:"enabled"`
	Targets     map[string]skillTargetReq `json:"targets"`
	CreatedAt   time.Time                 `json:"created_at"`
}

type resolveConflictReq struct {
	Tool     string `json:"tool"`
	FilePath string `json:"file_path"`
	Strategy string `json:"strategy"`
}

type mutationResp struct {
	AffectedFiles []configmanager.AffectedFile `json:"affected_files"`
}

type createMutationResp struct {
	ID            int64                        `json:"id"`
	AffectedFiles []configmanager.AffectedFile `json:"affected_files"`
}

func readJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(v); err != nil {
		return err
	}

	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("request body must contain a single JSON value")
		}
		return err
	}
	return nil
}

func readOptionalJSON(r *http.Request, v interface{}) (bool, error) {
	defer r.Body.Close()

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		return false, err
	}
	if len(bytes.TrimSpace(payload)) == 0 {
		return false, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(v); err != nil {
		return true, err
	}

	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return true, fmt.Errorf("request body must contain a single JSON value")
		}
		return true, err
	}
	return true, nil
}

func writeError(w http.ResponseWriter, status int, code, message string, details interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": code, "message": message, "details": details,
	})
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	profiles, err := s.mgr.ListProfiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_profiles_failed", "failed to list profiles", err.Error())
		return
	}

	response := make([]profileResp, 0, len(profiles))
	for _, profile := range profiles {
		item, err := s.profileResponse(profile)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list_profiles_failed", "failed to list profiles", err.Error())
			return
		}
		response = append(response, item)
	}
	writeJSON(w, response)
}

func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	var req createProfileReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON request body", err.Error())
		return
	}
	if err := validateProfileInput(req.Name, req.Config); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile", err.Error(), nil)
		return
	}

	id, err := s.mgr.CreateProfile(req.Name, req.Config, req.ToolTargets)
	if err != nil {
		writeError(w, profileErrorStatus(err), "create_profile_failed", "failed to create profile", err.Error())
		return
	}
	writeJSON(w, createMutationResp{ID: id, AffectedFiles: []configmanager.AffectedFile{}})
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile_id", "invalid profile id", err.Error())
		return
	}

	var req updateProfileReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON request body", err.Error())
		return
	}

	previous, previousTargets, err := s.snapshotProfile(id)
	if err != nil {
		writeError(w, profileErrorStatus(err), "update_profile_failed", "failed to update profile", err.Error())
		return
	}

	config, err := profileUpdateConfig(previous, req.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile", err.Error(), nil)
		return
	}
	if err := validateProfileInput(req.Name, config); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile", err.Error(), nil)
		return
	}

	if err := s.mgr.UpdateProfile(id, req.Name, config); err != nil {
		writeError(w, profileErrorStatus(err), "update_profile_failed", "failed to update profile", err.Error())
		return
	}
	if req.ToolTargets != nil {
		if err := s.mgr.SetProfileToolTargets(id, req.ToolTargets); err != nil {
			err = combineConfigMutationErrors(err, s.restoreProfile(previous, previousTargets))
			writeError(w, profileErrorStatus(err), "update_profile_failed", "failed to update profile", err.Error())
			return
		}
	}
	writeJSON(w, mutationResp{AffectedFiles: []configmanager.AffectedFile{}})
}

func profileUpdateConfig(existing *storage.Profile, config string) (string, error) {
	if strings.TrimSpace(config) == "" {
		return "", fmt.Errorf("config is required")
	}

	var providerCfg configmanager.ProviderConfig
	if err := json.Unmarshal([]byte(config), &providerCfg); err != nil {
		return "", fmt.Errorf("config must be valid JSON: %w", err)
	}
	if strings.TrimSpace(providerCfg.APIKey) != "" {
		return config, nil
	}

	if existing == nil {
		return "", fmt.Errorf("profile snapshot missing")
	}

	var existingCfg configmanager.ProviderConfig
	if strings.TrimSpace(existing.Config) != "" {
		if err := json.Unmarshal([]byte(existing.Config), &existingCfg); err != nil {
			return "", fmt.Errorf("stored config must be valid JSON: %w", err)
		}
	}
	if strings.TrimSpace(existingCfg.APIKey) == "" {
		return "", fmt.Errorf("config.api_key is required")
	}

	providerCfg.APIKey = existingCfg.APIKey
	merged, err := json.Marshal(providerCfg)
	if err != nil {
		return "", err
	}
	return string(merged), nil
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile_id", "invalid profile id", err.Error())
		return
	}

	if err := s.mgr.DeleteProfile(id); err != nil {
		writeError(w, profileErrorStatus(err), "delete_profile_failed", "failed to delete profile", err.Error())
		return
	}
	writeJSON(w, mutationResp{AffectedFiles: []configmanager.AffectedFile{}})
}

func (s *Server) handleActivateProfile(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile_id", "invalid profile id", err.Error())
		return
	}

	affected, err := s.mgr.ActivateProfile(id)
	if err != nil {
		writeError(w, profileErrorStatus(err), "activate_profile_failed", "failed to activate profile", err.Error())
		return
	}
	writeJSON(w, activateResp{AffectedFiles: affected})
}

func (s *Server) handleListMCPServers(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	servers, err := s.db.ListMCPServers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_mcp_failed", "failed to list MCP servers", err.Error())
		return
	}

	response := make([]mcpServerResp, 0, len(servers))
	for _, server := range servers {
		targets, err := s.db.GetMCPServerTargets(server.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list_mcp_failed", "failed to list MCP servers", err.Error())
			return
		}
		response = append(response, mcpServerResp{
			ID:        server.ID,
			Name:      server.Name,
			Command:   server.Command,
			Args:      server.Args,
			Env:       server.Env,
			Enabled:   server.Enabled,
			Targets:   targets,
			CreatedAt: server.CreatedAt,
		})
	}
	writeJSON(w, response)
}

func (s *Server) handleCreateMCPServer(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	var req createMCPReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON request body", err.Error())
		return
	}
	if err := validateMCPInput(req.Name, req.Command, req.Args, req.Env); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_mcp_server", err.Error(), nil)
		return
	}

	id, err := s.mgr.CreateMCPServer(req.Name, req.Command, req.Args, req.Env, req.Targets)
	if err != nil {
		writeError(w, configErrorStatus(err), "create_mcp_failed", "failed to create MCP server", err.Error())
		return
	}
	if req.Enabled != nil && !*req.Enabled {
		if err := s.mgr.UpdateMCPServer(id, req.Name, req.Command, req.Args, req.Env, false); err != nil {
			err = combineConfigMutationErrors(err, s.rollbackCreateMCPServer(id))
			writeError(w, configErrorStatus(err), "create_mcp_failed", "failed to create MCP server", err.Error())
			return
		}
	}
	affected, err := s.mgr.SyncMCPServers()
	if err != nil {
		err = combineConfigMutationErrors(err, s.rollbackCreateMCPServer(id))
		writeError(w, configErrorStatus(err), "sync_mcp_failed", "failed to sync MCP servers", err.Error())
		return
	}
	writeJSON(w, createMutationResp{ID: id, AffectedFiles: affected})
}

func (s *Server) handleUpdateMCPServer(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_mcp_id", "invalid MCP server id", err.Error())
		return
	}

	var req updateMCPReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON request body", err.Error())
		return
	}
	if err := validateMCPInput(req.Name, req.Command, req.Args, req.Env); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_mcp_server", err.Error(), nil)
		return
	}
	previous, previousTargets, err := s.snapshotMCPServer(id)
	if err != nil {
		writeError(w, configErrorStatus(err), "update_mcp_failed", "failed to update MCP server", err.Error())
		return
	}

	if err := s.mgr.UpdateMCPServer(id, req.Name, req.Command, req.Args, req.Env, req.Enabled); err != nil {
		writeError(w, configErrorStatus(err), "update_mcp_failed", "failed to update MCP server", err.Error())
		return
	}
	if req.Targets != nil {
		if err := s.db.SetMCPServerTargets(id, req.Targets); err != nil {
			err = combineConfigMutationErrors(err, s.restoreMCPServer(previous, previousTargets))
			writeError(w, configErrorStatus(err), "update_mcp_failed", "failed to update MCP server", err.Error())
			return
		}
	}
	affected, err := s.mgr.SyncMCPServers()
	if err != nil {
		err = combineConfigMutationErrors(err, s.restoreMCPServer(previous, previousTargets))
		writeError(w, configErrorStatus(err), "sync_mcp_failed", "failed to sync MCP servers", err.Error())
		return
	}
	writeJSON(w, mutationResp{AffectedFiles: affected})
}

func (s *Server) handleDeleteMCPServer(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_mcp_id", "invalid MCP server id", err.Error())
		return
	}
	previous, previousTargets, err := s.snapshotMCPServer(id)
	if err != nil {
		writeError(w, configErrorStatus(err), "delete_mcp_failed", "failed to delete MCP server", err.Error())
		return
	}
	if err := s.mgr.DeleteMCPServer(id); err != nil {
		writeError(w, configErrorStatus(err), "delete_mcp_failed", "failed to delete MCP server", err.Error())
		return
	}
	affected, err := s.mgr.SyncMCPServers()
	if err != nil {
		err = combineConfigMutationErrors(err, s.recreateMCPServer(previous, previousTargets))
		writeError(w, configErrorStatus(err), "sync_mcp_failed", "failed to sync MCP servers", err.Error())
		return
	}
	writeJSON(w, mutationResp{AffectedFiles: affected})
}

func (s *Server) handleSetMCPTargets(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_mcp_id", "invalid MCP server id", err.Error())
		return
	}
	server, err := s.db.GetMCPServer(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "set_mcp_targets_failed", "failed to set MCP targets", err.Error())
		return
	}
	if server == nil {
		writeError(w, http.StatusNotFound, "mcp_not_found", "MCP server not found", nil)
		return
	}

	var req setTargetsReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON request body", err.Error())
		return
	}
	previousTargets, err := s.db.GetMCPServerTargets(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "set_mcp_targets_failed", "failed to set MCP targets", err.Error())
		return
	}
	if err := s.db.SetMCPServerTargets(id, req.Targets); err != nil {
		writeError(w, configErrorStatus(err), "set_mcp_targets_failed", "failed to set MCP targets", err.Error())
		return
	}
	affected, err := s.mgr.SyncMCPServers()
	if err != nil {
		err = combineConfigMutationErrors(err, s.restoreMCPTargets(id, previousTargets))
		writeError(w, configErrorStatus(err), "sync_mcp_failed", "failed to sync MCP servers", err.Error())
		return
	}
	writeJSON(w, mutationResp{AffectedFiles: affected})
}

func (s *Server) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	skills, err := s.db.ListSkills()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_skills_failed", "failed to list skills", err.Error())
		return
	}

	response := make([]skillResp, 0, len(skills))
	for _, skill := range skills {
		targets, err := s.skillTargetsResponse(skill.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list_skills_failed", "failed to list skills", err.Error())
			return
		}
		response = append(response, skillResp{
			ID:          skill.ID,
			Name:        skill.Name,
			SourcePath:  skill.SourcePath,
			Description: skill.Description,
			Enabled:     skill.Enabled,
			Targets:     targets,
			CreatedAt:   skill.CreatedAt,
		})
	}
	writeJSON(w, response)
}

func (s *Server) handleSkillsInventory(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}
	inventory, err := s.mgr.SkillsInventory()
	if err != nil {
		writeError(w, configErrorStatus(err), "skills_inventory_failed", "failed to load skills inventory", err.Error())
		return
	}
	writeJSON(w, inventory)
}

func (s *Server) handleSkillsOverview(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}
	overview, err := s.mgr.SkillsOverview()
	if err != nil {
		writeError(w, configErrorStatus(err), "skills_overview_failed", "failed to load skills overview", err.Error())
		return
	}
	writeJSON(w, overview)
}

func (s *Server) handleInstallAgentUsageSkill(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	var req agentUsageSkillActionReq
	hasBody, err := readOptionalJSON(r, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON request body", err.Error())
		return
	}

	agents := req.Agents
	if !hasBody {
		agents = nil
	}

	result, err := s.mgr.InstallAgentUsageSkill(agents)
	if err != nil {
		writeError(w, agentUsageSkillErrorStatus(err), "install_agent_usage_skill_failed", "failed to install agent-usage skill", err.Error())
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleUninstallAgentUsageSkill(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	var req agentUsageSkillActionReq
	hasBody, err := readOptionalJSON(r, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON request body", err.Error())
		return
	}

	agents := req.Agents
	if !hasBody {
		agents = nil
	}

	result, err := s.mgr.UninstallAgentUsageSkill(agents)
	if err != nil {
		writeError(w, agentUsageSkillErrorStatus(err), "uninstall_agent_usage_skill_failed", "failed to uninstall agent-usage skill", err.Error())
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleImportSkills(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}
	result, err := s.mgr.ImportNonConflictingSkills()
	if err != nil {
		writeError(w, configErrorStatus(err), "import_skills_failed", "failed to import skills", err.Error())
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleImportManagedSkill(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	var req importManagedSkillReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON request body", err.Error())
		return
	}

	var previous *storage.SkillRecord
	var previousTargets map[string]storage.SkillTargetRecord
	var err error
	if req.SkillID > 0 {
		previous, previousTargets, err = s.snapshotSkill(req.SkillID)
		if err != nil {
			writeError(w, configErrorStatus(err), "import_managed_skill_failed", "failed to import skill", err.Error())
			return
		}
	}

	result, err := s.mgr.ImportManagedSkill(configmanager.ImportManagedSkillRequest{
		SkillID:    req.SkillID,
		Name:       strings.TrimSpace(req.Name),
		Tool:       strings.TrimSpace(req.Tool),
		SourcePath: strings.TrimSpace(req.SourcePath),
	})
	if err != nil {
		writeError(w, configErrorStatus(err), "import_managed_skill_failed", "failed to import skill", err.Error())
		return
	}

	affected, err := s.mgr.SyncSkills()
	if err != nil {
		if previous != nil {
			err = combineConfigMutationErrors(err, s.restoreSkill(previous, previousTargets))
		} else {
			err = combineConfigMutationErrors(err, s.rollbackCreateSkill(result.SkillID))
		}
		writeError(w, configErrorStatus(err), "sync_skills_failed", "failed to sync skills", err.Error())
		return
	}

	writeJSON(w, importManagedSkillResp{
		SkillID:       result.SkillID,
		VariantID:     result.VariantID,
		CreatedNew:    result.CreatedNew,
		AffectedFiles: affected,
	})
}

func (s *Server) handleResolveSkillConflict(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}
	var req resolveSkillConflictReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON request body", err.Error())
		return
	}
	result, err := s.mgr.ResolveSkillConflict(configmanager.SkillConflictResolveRequest{
		Name:         strings.TrimSpace(req.Name),
		Tool:         strings.TrimSpace(req.Tool),
		LibraryPath:  strings.TrimSpace(req.LibraryPath),
		ExternalPath: strings.TrimSpace(req.ExternalPath),
		Direction:    strings.TrimSpace(req.Direction),
	})
	if err != nil {
		writeError(w, configErrorStatus(err), "resolve_skill_conflict_failed", "failed to resolve skill conflict", err.Error())
		return
	}
	writeJSON(w, result)
}

func (s *Server) handleCreateSkill(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	var req createSkillReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON request body", err.Error())
		return
	}
	targets, err := validateSkillInput(req.Name, req.SourcePath, req.Targets)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_skill", err.Error(), nil)
		return
	}

	id, err := s.mgr.CreateSkill(req.Name, req.SourcePath, req.Description, targets)
	if err != nil {
		writeError(w, configErrorStatus(err), "create_skill_failed", "failed to create skill", err.Error())
		return
	}
	affected, err := s.mgr.SyncSkills()
	if err != nil {
		err = combineConfigMutationErrors(err, s.rollbackCreateSkill(id))
		writeError(w, configErrorStatus(err), "sync_skills_failed", "failed to sync skills", err.Error())
		return
	}
	writeJSON(w, createMutationResp{ID: id, AffectedFiles: affected})
}

func (s *Server) handleUpdateSkill(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_skill_id", "invalid skill id", err.Error())
		return
	}
	var req updateSkillReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON request body", err.Error())
		return
	}
	if err := validateSkillBasics(req.Name, req.SourcePath); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_skill", err.Error(), nil)
		return
	}
	var targets map[string]configmanager.SkillTargetRecord
	if req.Targets != nil {
		targets, err = validateSkillTargets(req.Targets)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_skill_targets", err.Error(), nil)
			return
		}
	}
	previous, previousTargets, err := s.snapshotSkill(id)
	if err != nil {
		writeError(w, configErrorStatus(err), "update_skill_failed", "failed to update skill", err.Error())
		return
	}

	if req.Targets != nil {
		err = s.mgr.UpdateSkillWithTargets(id, req.Name, req.SourcePath, req.Description, req.Enabled, targets)
	} else {
		err = s.mgr.UpdateSkill(id, req.Name, req.SourcePath, req.Description, req.Enabled)
	}
	if err != nil {
		writeError(w, configErrorStatus(err), "update_skill_failed", "failed to update skill", err.Error())
		return
	}
	affected, err := s.mgr.SyncSkills()
	if err != nil {
		err = combineConfigMutationErrors(err, s.restoreSkill(previous, previousTargets))
		writeError(w, configErrorStatus(err), "sync_skills_failed", "failed to sync skills", err.Error())
		return
	}
	writeJSON(w, mutationResp{AffectedFiles: affected})
}

func (s *Server) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_skill_id", "invalid skill id", err.Error())
		return
	}
	previous, previousTargets, err := s.snapshotSkill(id)
	if err != nil {
		writeError(w, configErrorStatus(err), "delete_skill_failed", "failed to delete skill", err.Error())
		return
	}
	if err := s.mgr.DeleteSkill(id); err != nil {
		writeError(w, configErrorStatus(err), "delete_skill_failed", "failed to delete skill", err.Error())
		return
	}
	affected, err := s.mgr.SyncSkills()
	if err != nil {
		err = combineConfigMutationErrors(err, s.recreateSkill(previous, previousTargets))
		writeError(w, configErrorStatus(err), "sync_skills_failed", "failed to sync skills", err.Error())
		return
	}
	writeJSON(w, mutationResp{AffectedFiles: affected})
}

func (s *Server) handleSetSkillTargets(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}

	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_skill_id", "invalid skill id", err.Error())
		return
	}
	skill, err := s.db.GetSkill(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "set_skill_targets_failed", "failed to set skill targets", err.Error())
		return
	}
	if skill == nil {
		writeError(w, http.StatusNotFound, "skill_not_found", "skill not found", nil)
		return
	}

	var req setSkillTargetsReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON request body", err.Error())
		return
	}
	targets, err := validateSkillTargets(req.Targets)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_skill_targets", err.Error(), nil)
		return
	}
	previousTargets, err := s.db.GetSkillTargets(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "set_skill_targets_failed", "failed to set skill targets", err.Error())
		return
	}
	if err := s.db.SetSkillTargets(id, skillTargetsToStorage(targets)); err != nil {
		writeError(w, configErrorStatus(err), "set_skill_targets_failed", "failed to set skill targets", err.Error())
		return
	}
	affected, err := s.mgr.SyncSkills()
	if err != nil {
		err = combineConfigMutationErrors(err, s.restoreSkillTargets(id, previousTargets))
		writeError(w, configErrorStatus(err), "sync_skills_failed", "failed to sync skills", err.Error())
		return
	}
	writeJSON(w, mutationResp{AffectedFiles: affected})
}

func (s *Server) handleTriggerSync(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}
	changes, err := s.mgr.TriggerInboundSync()
	if err != nil {
		writeError(w, configErrorStatus(err), "sync_failed", "failed to trigger sync", err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"changes": changes})
}

func (s *Server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}
	status, err := s.mgr.GetSyncStatus()
	if err != nil {
		writeError(w, configErrorStatus(err), "sync_status_failed", "failed to get sync status", err.Error())
		return
	}
	writeJSON(w, status)
}

func (s *Server) handleResolveConflict(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}
	var req resolveConflictReq
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON request body", err.Error())
		return
	}
	if err := validateResolveConflict(req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_conflict_resolution", err.Error(), nil)
		return
	}
	if err := s.mgr.ResolveConflict(req.Tool, req.FilePath, req.Strategy); err != nil {
		writeError(w, configErrorStatus(err), "resolve_conflict_failed", "failed to resolve conflict", err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}
	backups, err := s.db.ListBackups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_backups_failed", "failed to list backups", err.Error())
		return
	}
	writeJSON(w, backups)
}

func (s *Server) handleManualBackup(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}
	affected, err := s.mgr.ManualBackup()
	if err != nil {
		writeError(w, configErrorStatus(err), "manual_backup_failed", "failed to create manual backup", err.Error())
		return
	}
	writeJSON(w, mutationResp{AffectedFiles: affected})
}

func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_backup_id", "invalid backup id", err.Error())
		return
	}
	affected, err := s.mgr.RestoreBackup(id)
	if err != nil {
		writeError(w, configErrorStatus(err), "restore_backup_failed", "failed to restore backup", err.Error())
		return
	}
	writeJSON(w, mutationResp{AffectedFiles: affected})
}

func (s *Server) handleListConfigFiles(w http.ResponseWriter, r *http.Request) {
	if s.mgr == nil {
		writeError(w, http.StatusServiceUnavailable, "config_manager_unavailable", "config manager is unavailable", nil)
		return
	}
	writeJSON(w, s.mgr.ListConfigFiles())
}

func validateProfileInput(name, config string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("profile name is required")
	}
	if strings.TrimSpace(config) == "" {
		return fmt.Errorf("config is required")
	}

	var providerCfg configmanager.ProviderConfig
	if err := json.Unmarshal([]byte(config), &providerCfg); err != nil {
		return fmt.Errorf("config must be valid JSON: %w", err)
	}
	if strings.TrimSpace(providerCfg.APIKey) == "" {
		return fmt.Errorf("config.api_key is required")
	}
	return nil
}

func profileErrorStatus(err error) int {
	return configErrorStatus(err)
}

func agentUsageSkillErrorStatus(err error) int {
	if err == nil {
		return http.StatusInternalServerError
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "agents are required") || strings.Contains(message, "invalid agent") {
		return http.StatusBadRequest
	}
	return configErrorStatus(err)
}

func configErrorStatus(err error) int {
	if err == nil {
		return http.StatusInternalServerError
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "not found") {
		return http.StatusNotFound
	}
	if strings.Contains(message, "sync conflict") {
		return http.StatusConflict
	}
	if strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "constraint failed") ||
		strings.Contains(message, "duplicate") {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func validateMCPInput(name, command, args, env string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(command) == "" {
		return fmt.Errorf("command is required")
	}
	if strings.TrimSpace(args) != "" {
		var parsed []string
		if err := json.Unmarshal([]byte(args), &parsed); err != nil {
			return fmt.Errorf("args must be a JSON array string: %w", err)
		}
	}
	if strings.TrimSpace(env) != "" {
		var parsed map[string]string
		if err := json.Unmarshal([]byte(env), &parsed); err != nil {
			return fmt.Errorf("env must be a JSON object string: %w", err)
		}
	}
	return nil
}

func validateSkillInput(name, sourcePath string, targets map[string]skillTargetReq) (map[string]configmanager.SkillTargetRecord, error) {
	if err := validateSkillBasics(name, sourcePath); err != nil {
		return nil, err
	}
	return validateSkillTargets(targets)
}

func validateSkillBasics(name, sourcePath string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(sourcePath) == "" {
		return fmt.Errorf("source_path is required")
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("source_path must exist as a directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source_path must exist as a directory")
	}
	return nil
}

func validateSkillTargets(targets map[string]skillTargetReq) (map[string]configmanager.SkillTargetRecord, error) {
	records := make(map[string]configmanager.SkillTargetRecord, len(targets))
	for tool, target := range targets {
		method := target.Method
		if method == "" {
			method = "symlink"
		}
		if method != "symlink" && method != "copy" {
			return nil, fmt.Errorf("unsupported skill target method for %s: %s", tool, method)
		}
		records[tool] = configmanager.SkillTargetRecord{
			Tool:      tool,
			Method:    method,
			Enabled:   target.Enabled,
			VariantID: target.VariantID,
		}
	}
	return records, nil
}

func validateResolveConflict(req resolveConflictReq) error {
	if strings.TrimSpace(req.Tool) == "" {
		return fmt.Errorf("tool is required")
	}
	if strings.TrimSpace(req.FilePath) == "" {
		return fmt.Errorf("file_path is required")
	}
	switch req.Strategy {
	case "keep_external", "keep_ours":
		return nil
	default:
		return fmt.Errorf("strategy must be keep_external or keep_ours")
	}
}

func skillTargetsToStorage(targets map[string]configmanager.SkillTargetRecord) []storage.SkillTargetRecord {
	records := make([]storage.SkillTargetRecord, 0, len(targets))
	for _, target := range targets {
		records = append(records, storage.SkillTargetRecord{
			Tool:    target.Tool,
			Method:  target.Method,
			Enabled: target.Enabled,
		})
	}
	return records
}

func combineConfigMutationErrors(primary error, secondary error) error {
	if primary == nil {
		return secondary
	}
	if secondary == nil {
		return primary
	}
	return fmt.Errorf("%w; rollback error: %v", primary, secondary)
}

func cloneBoolTargets(targets map[string]bool) map[string]bool {
	cloned := make(map[string]bool, len(targets))
	for tool, enabled := range targets {
		cloned[tool] = enabled
	}
	return cloned
}

func cloneProfile(profile *storage.Profile) *storage.Profile {
	if profile == nil {
		return nil
	}
	copied := *profile
	return &copied
}

func cloneSkillTargets(targets map[string]storage.SkillTargetRecord) map[string]storage.SkillTargetRecord {
	cloned := make(map[string]storage.SkillTargetRecord, len(targets))
	for tool, target := range targets {
		cloned[tool] = target
	}
	return cloned
}

func (s *Server) snapshotProfile(id int64) (*storage.Profile, map[string]bool, error) {
	profile, err := s.db.GetProfile(id)
	if err != nil {
		return nil, nil, err
	}
	if profile == nil {
		return nil, nil, fmt.Errorf("profile not found: %d", id)
	}
	targets, err := s.mgr.GetProfileToolTargets(id)
	if err != nil {
		return nil, nil, err
	}
	return cloneProfile(profile), cloneBoolTargets(targets), nil
}

func (s *Server) restoreProfile(profile *storage.Profile, targets map[string]bool) error {
	if profile == nil {
		return fmt.Errorf("profile restore snapshot missing")
	}
	restoreErr := s.mgr.UpdateProfile(profile.ID, profile.Name, profile.Config)
	if restoreErr == nil {
		restoreErr = s.mgr.SetProfileToolTargets(profile.ID, cloneBoolTargets(targets))
	}
	return restoreErr
}

func (s *Server) snapshotMCPServer(id int64) (*storage.MCPServerRecord, map[string]bool, error) {
	server, err := s.db.GetMCPServer(id)
	if err != nil {
		return nil, nil, err
	}
	if server == nil {
		return nil, nil, fmt.Errorf("mcp server not found: %d", id)
	}
	targets, err := s.db.GetMCPServerTargets(id)
	if err != nil {
		return nil, nil, err
	}
	copied := *server
	return &copied, cloneBoolTargets(targets), nil
}

func (s *Server) rollbackCreateMCPServer(id int64) error {
	rollbackErr := s.mgr.DeleteMCPServer(id)
	_, syncErr := s.mgr.SyncMCPServers()
	return combineConfigMutationErrors(rollbackErr, syncErr)
}

func (s *Server) restoreMCPServer(server *storage.MCPServerRecord, targets map[string]bool) error {
	if server == nil {
		return fmt.Errorf("mcp server restore snapshot missing")
	}
	restoreErr := s.mgr.UpdateMCPServer(server.ID, server.Name, server.Command, server.Args, server.Env, server.Enabled)
	if restoreErr == nil {
		restoreErr = s.db.SetMCPServerTargets(server.ID, cloneBoolTargets(targets))
	}
	_, syncErr := s.mgr.SyncMCPServers()
	return combineConfigMutationErrors(restoreErr, syncErr)
}

func (s *Server) recreateMCPServer(server *storage.MCPServerRecord, targets map[string]bool) error {
	if server == nil {
		return fmt.Errorf("mcp server recreate snapshot missing")
	}

	recreateErr := s.db.CreateMCPServerWithID(*server)
	if recreateErr == nil {
		recreateErr = s.db.SetMCPServerTargets(server.ID, cloneBoolTargets(targets))
	}
	_, syncErr := s.mgr.SyncMCPServers()
	return combineConfigMutationErrors(recreateErr, syncErr)
}

func (s *Server) restoreMCPTargets(id int64, targets map[string]bool) error {
	restoreErr := s.db.SetMCPServerTargets(id, cloneBoolTargets(targets))
	_, syncErr := s.mgr.SyncMCPServers()
	return combineConfigMutationErrors(restoreErr, syncErr)
}

func (s *Server) snapshotSkill(id int64) (*storage.SkillRecord, map[string]storage.SkillTargetRecord, error) {
	skill, err := s.db.GetSkill(id)
	if err != nil {
		return nil, nil, err
	}
	if skill == nil {
		return nil, nil, fmt.Errorf("skill not found: %d", id)
	}
	targets, err := s.db.GetSkillTargets(id)
	if err != nil {
		return nil, nil, err
	}
	copied := *skill
	return &copied, cloneSkillTargets(targets), nil
}

func (s *Server) rollbackCreateSkill(id int64) error {
	rollbackErr := s.mgr.DeleteSkill(id)
	_, syncErr := s.mgr.SyncSkills()
	return combineConfigMutationErrors(rollbackErr, syncErr)
}

func (s *Server) restoreSkill(skill *storage.SkillRecord, targets map[string]storage.SkillTargetRecord) error {
	if skill == nil {
		return fmt.Errorf("skill restore snapshot missing")
	}
	restoreErr := s.mgr.UpdateSkill(skill.ID, skill.Name, skill.SourcePath, skill.Description, skill.Enabled)
	if restoreErr == nil {
		restoreErr = s.db.SetSkillTargets(skill.ID, mapsSkillTargetsToSlice(targets))
	}
	_, syncErr := s.mgr.SyncSkills()
	return combineConfigMutationErrors(restoreErr, syncErr)
}

func (s *Server) recreateSkill(skill *storage.SkillRecord, targets map[string]storage.SkillTargetRecord) error {
	if skill == nil {
		return fmt.Errorf("skill recreate snapshot missing")
	}

	recreateErr := s.db.CreateSkillWithID(*skill)
	if recreateErr == nil {
		recreateErr = s.db.SetSkillTargets(skill.ID, mapsSkillTargetsToSlice(targets))
	}
	_, syncErr := s.mgr.SyncSkills()
	return combineConfigMutationErrors(recreateErr, syncErr)
}

func (s *Server) restoreSkillTargets(id int64, targets map[string]storage.SkillTargetRecord) error {
	restoreErr := s.db.SetSkillTargets(id, mapsSkillTargetsToSlice(targets))
	_, syncErr := s.mgr.SyncSkills()
	return combineConfigMutationErrors(restoreErr, syncErr)
}

func mapsSkillTargetsToSlice(targets map[string]storage.SkillTargetRecord) []storage.SkillTargetRecord {
	records := make([]storage.SkillTargetRecord, 0, len(targets))
	for _, target := range targets {
		records = append(records, target)
	}
	return records
}

func (s *Server) skillTargetsResponse(skillID int64) (map[string]skillTargetReq, error) {
	targets, err := s.db.GetSkillTargets(skillID)
	if err != nil {
		return nil, err
	}
	response := make(map[string]skillTargetReq, len(targets))
	for tool, target := range targets {
		response[tool] = skillTargetReq{
			Method:    target.Method,
			Enabled:   target.Enabled,
			VariantID: target.VariantID,
		}
	}
	return response, nil
}

func (s *Server) profileResponse(profile storage.Profile) (profileResp, error) {
	redactedConfig := profile.Config
	hasAPIKey := false
	if strings.TrimSpace(profile.Config) != "" {
		if s.mgr != nil {
			masked, maskedAPIKey, err := s.mgr.MaskProfileConfig(profile.Config)
			if err != nil {
				return profileResp{}, err
			}
			redactedConfig = masked
			hasAPIKey = maskedAPIKey
		} else {
			var providerCfg configmanager.ProviderConfig
			if err := json.Unmarshal([]byte(profile.Config), &providerCfg); err != nil {
				return profileResp{}, err
			}
			hasAPIKey = providerCfg.APIKey != ""
			if hasAPIKey {
				providerCfg.APIKey = configmanager.MaskAPIKey(providerCfg.APIKey)
			}
			encoded, err := json.Marshal(providerCfg)
			if err != nil {
				return profileResp{}, err
			}
			redactedConfig = string(encoded)
		}
	}

	targets, err := s.mgr.GetProfileToolTargets(profile.ID)
	if err != nil {
		return profileResp{}, err
	}

	return profileResp{
		ID:          profile.ID,
		Name:        profile.Name,
		IsActive:    profile.IsActive,
		Config:      redactedConfig,
		HasAPIKey:   hasAPIKey,
		ToolTargets: targets,
		CreatedAt:   profile.CreatedAt,
		UpdatedAt:   profile.UpdatedAt,
	}, nil
}
