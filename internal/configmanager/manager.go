package configmanager

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

type SkillTargetRecord = storage.SkillTargetRecord

type Manager struct {
	db                *storage.DB
	syncEngine        *SyncEngine
	backup            *BackupManager
	adapters          map[string]Adapter
	syncedSkills      map[string]AffectedFile
	skillSymlinkFn    func(string, string) error
	setSkillTargetsFn func(int64, []storage.SkillTargetRecord) error
	encryptionKey     []byte
	keyProvider       func() ([]byte, error)
	mu                sync.Mutex
}

type ManagerOption func(*Manager)

type SyncStatus struct {
	ChangesCount int          `json:"changes_count"`
	Conflicts    int          `json:"conflicts"`
	Changes      []SyncChange `json:"changes,omitempty"`
}

type adapterRollbackState struct {
	tool     string
	adapter  Adapter
	previous *ProviderConfig
}

func NewManager(db *storage.DB, backupDir string, opts ...ManagerOption) *Manager {
	if backupDir == "" {
		resolved, err := BackupBaseDir()
		if err == nil {
			backupDir = resolved
		}
	}

	m := &Manager{
		db:                db,
		backup:            NewBackupManager(backupDir),
		adapters:          map[string]Adapter{},
		syncedSkills:      map[string]AffectedFile{},
		skillSymlinkFn:    os.Symlink,
		setSkillTargetsFn: db.SetSkillTargets,
		keyProvider:       GetOrCreateEncryptionKey,
	}
	m.syncEngine = NewSyncEngine(db, m.backup)

	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}

	if len(m.encryptionKey) == 0 && m.keyProvider != nil {
		if key, err := m.keyProvider(); err == nil {
			m.encryptionKey = key
		}
	}

	return m
}

func WithClaudeAdapter(settingsDir, claudeJSONPath string) ManagerOption {
	return WithAdapter(NewClaudeAdapter(settingsDir, claudeJSONPath))
}

func WithCodexAdapter(codexDir string) ManagerOption {
	return WithAdapter(NewCodexAdapter(codexDir))
}

func WithOpenCodeAdapter(configPath string) ManagerOption {
	return WithAdapter(NewOpenCodeAdapter(configPath))
}

func WithOpenClawAdapter(configPath string) ManagerOption {
	return WithAdapter(NewOpenClawAdapter(configPath))
}

func WithEncryptionKey(key []byte) ManagerOption {
	return func(m *Manager) {
		if key == nil {
			m.encryptionKey = nil
			return
		}
		m.encryptionKey = append([]byte(nil), key...)
	}
}

func WithEncryptionKeyProvider(provider func() ([]byte, error)) ManagerOption {
	return func(m *Manager) {
		if provider != nil {
			m.keyProvider = provider
		}
	}
}

func WithAdapter(adapter Adapter) ManagerOption {
	return func(m *Manager) {
		if adapter == nil {
			return
		}
		m.adapters[adapter.Tool()] = adapter
	}
}

func (m *Manager) Bootstrap() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	profiles, err := m.db.ListProfiles()
	if err != nil {
		return err
	}
	if len(profiles) > 0 {
		return nil
	}

	if len(m.adapters) == 0 {
		return nil
	}

	discovered := map[string]*ProviderConfig{}
	var selectedTool string
	var selectedConfig *ProviderConfig

	for _, tool := range m.sortedAdapterTools() {
		adapter := m.adapters[tool]
		if !adapter.IsInstalled() {
			continue
		}

		cfg, err := adapter.GetProviderConfig()
		if err != nil {
			return err
		}
		if !providerConfigHasValues(cfg) {
			continue
		}

		cloned := cloneProviderConfig(cfg)
		discovered[tool] = cloned
		if selectedConfig == nil {
			selectedTool = tool
			selectedConfig = cloned
		}
	}

	targets := map[string]bool{}
	if selectedConfig == nil {
		selectedConfig = &ProviderConfig{}
	} else {
		targets[selectedTool] = true
		for tool, cfg := range discovered {
			if tool == selectedTool {
				continue
			}
			if providerConfigsEqual(selectedConfig, cfg) {
				targets[tool] = true
			}
		}
	}

	config, err := json.Marshal(selectedConfig)
	if err != nil {
		return err
	}

	profileID, err := m.createProfileLocked("Default", string(config), targets)
	if err != nil {
		return err
	}

	_, err = m.activateProfileLocked(profileID)
	return err
}

func (m *Manager) CreateProfile(name, config string, toolTargets map[string]bool) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.createProfileLocked(name, config, toolTargets)
}

func (m *Manager) createProfileLocked(name, config string, toolTargets map[string]bool) (int64, error) {
	normalizedConfig, err := m.normalizeAndEncryptConfig(config)
	if err != nil {
		return 0, err
	}

	if err := m.db.CreateProfile(name, normalizedConfig); err != nil {
		return 0, err
	}

	profileID, err := m.findProfileIDByName(name)
	if err != nil {
		return 0, err
	}

	if err := m.db.SetProfileToolTargets(profileID, boolTargetsToStorageTargets(toolTargets)); err != nil {
		_ = m.db.DeleteProfile(profileID)
		return 0, err
	}

	return profileID, nil
}

func (m *Manager) UpdateProfile(id int64, name, config string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	normalizedConfig, err := m.normalizeAndEncryptConfig(config)
	if err != nil {
		return err
	}

	return m.db.UpdateProfile(id, name, normalizedConfig)
}

func (m *Manager) DeleteProfile(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.db.DeleteProfile(id)
}

func (m *Manager) ActivateProfile(id int64) ([]AffectedFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activateProfileLocked(id)
}

func (m *Manager) activateProfileLocked(id int64) ([]AffectedFile, error) {
	profile, err := m.db.GetProfile(id)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, fmt.Errorf("profile not found: %d", id)
	}

	providerCfg, err := m.parseAndDecryptConfig(profile.Config)
	if err != nil {
		return nil, err
	}

	targets, err := m.db.GetProfileToolTargets(id)
	if err != nil {
		return nil, err
	}

	enabledTools := make([]string, 0, len(targets))
	for tool, target := range targets {
		if !target.Enabled {
			continue
		}
		if _, ok := m.adapters[tool]; !ok {
			continue
		}
		enabledTools = append(enabledTools, tool)
	}
	sort.Strings(enabledTools)

	previousActiveID, err := m.activeProfileID()
	if err != nil {
		return nil, err
	}

	for _, tool := range enabledTools {
		adapter := m.adapters[tool]
		if err := withManagedPathLocks(providerConfigPaths(adapter), func() error {
			return m.ensurePathsUnchangedLocked(tool, providerConfigPaths(adapter))
		}); err != nil {
			return nil, err
		}
	}

	for _, tool := range enabledTools {
		adapter := m.adapters[tool]
		if err := withManagedPathLocks(providerConfigPaths(adapter), func() error {
			for _, configFile := range adapter.ConfigFiles() {
				if !fileExists(configFile.Path) {
					continue
				}
				slot, backupPath, err := m.backup.Backup(tool, filepath.Base(configFile.Path), configFile.Path, "profile_switch")
				if err != nil {
					return err
				}
				_, _ = m.db.InsertBackupRecord(tool, configFile.Path, backupPath, slot, "profile_switch")
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}

	if err := m.db.DeactivateProfiles(); err != nil {
		return nil, err
	}

	var affected []AffectedFile
	rollbackStates := make([]adapterRollbackState, 0, len(enabledTools))

	for _, tool := range enabledTools {
		adapter := m.adapters[tool]

		var (
			previousConfig *ProviderConfig
			files          []AffectedFile
		)
		err := withManagedPathLocks(providerConfigPaths(adapter), func() error {
			var err error
			previousConfig, err = adapter.GetProviderConfig()
			if err != nil {
				return err
			}
			if err := m.ensurePathsUnchangedLocked(tool, providerConfigPaths(adapter)); err != nil {
				return err
			}
			files, err = adapter.SetProviderConfig(providerCfg)
			if err != nil {
				return err
			}
			return m.recordSyncStateForToolPathsLocked(tool, providerConfigPaths(adapter), "outbound")
		})
		if err != nil {
			rollbackErr := m.rollbackProviderWrites(rollbackStates)
			restoreErr := m.restorePreviousActiveProfile(previousActiveID)
			return nil, combineErrors(err, combineErrors(rollbackErr, restoreErr))
		}

		rollbackStates = append(rollbackStates, adapterRollbackState{
			tool:     tool,
			adapter:  adapter,
			previous: cloneProviderConfig(previousConfig),
		})
		affected = append(affected, files...)
	}

	if err := m.db.ActivateProfile(id); err != nil {
		rollbackErr := m.rollbackProviderWrites(rollbackStates)
		restoreErr := m.restorePreviousActiveProfile(previousActiveID)
		return nil, combineErrors(err, combineErrors(rollbackErr, restoreErr))
	}

	return affected, nil
}

func (m *Manager) activeProfileID() (int64, error) {
	profiles, err := m.db.ListProfiles()
	if err != nil {
		return 0, err
	}
	for _, profile := range profiles {
		if profile.IsActive {
			return profile.ID, nil
		}
	}
	return 0, nil
}

func (m *Manager) restorePreviousActiveProfile(previousActiveID int64) error {
	if previousActiveID <= 0 {
		return nil
	}
	return m.db.ActivateProfile(previousActiveID)
}

func (m *Manager) rollbackProviderWrites(states []adapterRollbackState) error {
	var rollbackErr error
	for index := len(states) - 1; index >= 0; index-- {
		state := states[index]
		if _, err := state.adapter.SetProviderConfig(state.previous); err != nil {
			wrapped := fmt.Errorf("rollback %s: %w", state.tool, err)
			rollbackErr = combineErrors(rollbackErr, wrapped)
		}
	}
	return rollbackErr
}

func combineErrors(primary error, secondary error) error {
	if primary == nil {
		return secondary
	}
	if secondary == nil {
		return primary
	}
	return fmt.Errorf("%w; rollback error: %v", primary, secondary)
}

func (m *Manager) ListProfiles() ([]storage.Profile, error) {
	return m.db.ListProfiles()
}

func (m *Manager) GetProfileToolTargets(id int64) (map[string]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	targets, err := m.db.GetProfileToolTargets(id)
	if err != nil {
		return nil, err
	}

	result := make(map[string]bool, len(targets))
	for tool, target := range targets {
		result[tool] = target.Enabled
	}
	return result, nil
}

func (m *Manager) SetProfileToolTargets(id int64, targets map[string]bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.db.SetProfileToolTargets(id, boolTargetsToStorageTargets(targets))
}

func (m *Manager) CreateMCPServer(name, command, args, env string, targets map[string]bool) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id, err := m.db.CreateMCPServer(name, command, args, env)
	if err != nil {
		return 0, err
	}
	if err := m.db.SetMCPServerTargets(id, targets); err != nil {
		return 0, err
	}
	return id, nil
}

func (m *Manager) UpdateMCPServer(id int64, name, command, args, env string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.db.UpdateMCPServer(id, name, command, args, env, enabled)
}

func (m *Manager) DeleteMCPServer(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.db.DeleteMCPServer(id)
}

func (m *Manager) SyncMCPServers() ([]AffectedFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	servers, err := m.db.ListMCPServers()
	if err != nil {
		return nil, err
	}

	grouped := map[string][]MCPServerConfig{}
	for tool := range m.adapters {
		grouped[tool] = nil
	}

	for _, server := range servers {
		if !server.Enabled {
			continue
		}

		targets, err := m.db.GetMCPServerTargets(server.ID)
		if err != nil {
			return nil, err
		}

		parsedArgs, err := parseJSONStringSlice(server.Args)
		if err != nil {
			return nil, err
		}
		parsedEnv, err := parseJSONStringMap(server.Env)
		if err != nil {
			return nil, err
		}

		item := MCPServerConfig{
			Name:    server.Name,
			Command: server.Command,
			Args:    parsedArgs,
			Env:     parsedEnv,
		}

		for tool, enabled := range targets {
			if !enabled {
				continue
			}
			if _, ok := m.adapters[tool]; !ok {
				continue
			}
			grouped[tool] = append(grouped[tool], item)
		}
	}

	var affected []AffectedFile
	for _, tool := range m.sortedAdapterTools() {
		adapter := m.adapters[tool]
		var files []AffectedFile
		err := withManagedPathLocks(mcpConfigPaths(adapter), func() error {
			if err := m.ensurePathsUnchangedLocked(tool, mcpConfigPaths(adapter)); err != nil {
				return err
			}
			var err error
			files, err = adapter.SetMCPServers(grouped[tool])
			if err != nil {
				return err
			}
			return m.recordSyncStateForToolPathsLocked(tool, mcpConfigPaths(adapter), "outbound")
		})
		if err != nil {
			return nil, err
		}
		affected = append(affected, files...)
	}

	return affected, nil
}

func (m *Manager) CreateSkill(name, sourcePath, description string, targets map[string]SkillTargetRecord) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := validateSkillSourcePath(sourcePath); err != nil {
		return 0, err
	}

	id, err := m.db.CreateSkill(name, sourcePath, description)
	if err != nil {
		return 0, err
	}
	defaultVariantID, err := m.ensureSkillVariantLocked(id, sourcePath, "global")
	if err != nil {
		_ = m.db.DeleteSkill(id)
		return 0, err
	}

	mapped := make([]storage.SkillTargetRecord, 0, len(targets))
	for _, target := range targets {
		variantID := target.VariantID
		if variantID == 0 {
			variantID = defaultVariantID
		}
		mapped = append(mapped, storage.SkillTargetRecord{
			Tool:      target.Tool,
			Method:    target.Method,
			Enabled:   target.Enabled,
			VariantID: variantID,
		})
	}
	if err := m.setSkillTargetsFn(id, mapped); err != nil {
		_ = m.db.DeleteSkill(id)
		return 0, err
	}

	return id, nil
}

func (m *Manager) UpdateSkill(id int64, name, sourcePath, description string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := validateSkillSourcePath(sourcePath); err != nil {
		return err
	}
	skill, err := m.db.GetSkill(id)
	if err != nil {
		return err
	}
	if skill == nil {
		return fmt.Errorf("skill not found: %d", id)
	}
	oldVariant, err := m.findVariantBySourceLocked(id, skill.SourcePath)
	if err != nil {
		return err
	}
	newVariantID, err := m.ensureSkillVariantLocked(id, sourcePath, "global")
	if err != nil {
		return err
	}
	if err := m.db.UpdateSkill(id, name, sourcePath, description, enabled); err != nil {
		return err
	}
	targets, err := m.db.GetSkillTargets(id)
	if err != nil {
		return err
	}
	changed := false
	for tool, target := range targets {
		if target.VariantID == 0 || (oldVariant != nil && target.VariantID == oldVariant.ID) {
			target.VariantID = newVariantID
			targets[tool] = target
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return m.db.SetSkillTargets(id, skillTargetRecordsFromMap(targets))
}

func (m *Manager) UpdateSkillWithTargets(id int64, name, sourcePath, description string, enabled bool, targets map[string]SkillTargetRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := validateSkillSourcePath(sourcePath); err != nil {
		return err
	}

	mapped := make([]storage.SkillTargetRecord, 0, len(targets))
	defaultVariantID, err := m.ensureSkillVariantLocked(id, sourcePath, "global")
	if err != nil {
		return err
	}
	for _, target := range targets {
		variantID := target.VariantID
		if variantID == 0 {
			variantID = defaultVariantID
		}
		mapped = append(mapped, storage.SkillTargetRecord{
			Tool:      target.Tool,
			Method:    target.Method,
			Enabled:   target.Enabled,
			VariantID: variantID,
		})
	}

	return m.db.UpdateSkillWithTargets(id, name, sourcePath, description, enabled, mapped)
}

func (m *Manager) DeleteSkill(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.db.DeleteSkill(id)
}

func (m *Manager) ensureSkillVariantLocked(skillID int64, sourcePath, originTool string) (int64, error) {
	variant, err := m.findVariantBySourceLocked(skillID, sourcePath)
	if err != nil {
		return 0, err
	}
	if variant != nil {
		return variant.ID, nil
	}
	return m.db.CreateSkillVariant(skillID, sourcePath, originTool)
}

func (m *Manager) findVariantBySourceLocked(skillID int64, sourcePath string) (*storage.SkillVariantRecord, error) {
	return m.db.FindSkillVariantByPath(skillID, sourcePath)
}

func (m *Manager) defaultVariantForSkillLocked(skill storage.SkillRecord) (*storage.SkillVariantRecord, error) {
	variant, err := m.findVariantBySourceLocked(skill.ID, skill.SourcePath)
	if err != nil {
		return nil, err
	}
	if variant != nil {
		return variant, nil
	}
	if _, err := m.ensureSkillVariantLocked(skill.ID, skill.SourcePath, "global"); err != nil {
		return nil, err
	}
	return m.findVariantBySourceLocked(skill.ID, skill.SourcePath)
}

func (m *Manager) SyncSkills() ([]AffectedFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	skills, err := m.db.ListSkills()
	if err != nil {
		return nil, err
	}

	var affected []AffectedFile
	desiredSkills := make(map[string]AffectedFile)
	skillTargetUpdates := map[int64]map[string]storage.SkillTargetRecord{}
	originalSkillTargets := map[int64]map[string]storage.SkillTargetRecord{}
	rollbackDir, err := os.MkdirTemp("", "configmanager-skill-sync-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(rollbackDir)
	snapshots := map[string]skillPathSnapshot{}
	snapshotOrder := make([]string, 0)
	fail := func(err error) ([]AffectedFile, error) {
		rollbackErr := rollbackSkillPathSnapshots(snapshotOrder, snapshots)
		return nil, combineErrors(err, rollbackErr)
	}
	for _, skill := range skills {
		if !skill.Enabled {
			continue
		}
		defaultVariant, err := m.defaultVariantForSkillLocked(skill)
		if err != nil {
			return fail(err)
		}
		if err := validateSkillSourcePath(defaultVariant.SourcePath); err != nil {
			return fail(err)
		}

		targets, err := m.db.GetSkillTargets(skill.ID)
		if err != nil {
			return fail(err)
		}
		variants, err := m.db.ListSkillVariants(skill.ID)
		if err != nil {
			return fail(err)
		}
		variantsByID := make(map[int64]storage.SkillVariantRecord, len(variants))
		for _, variant := range variants {
			variantsByID[variant.ID] = variant
		}
		originalTargets := cloneSkillTargets(targets)
		targetsChanged := false

		for tool, target := range targets {
			if !target.Enabled {
				continue
			}
			adapter, ok := m.adapters[tool]
			if !ok {
				continue
			}

			method := target.Method
			if method == "" {
				method = defaultSkillSyncMethodForOS(runtime.GOOS)
			}
			actualMethod := method
			sourcePath := defaultVariant.SourcePath
			if target.VariantID != 0 {
				if variant, ok := variantsByID[target.VariantID]; ok {
					sourcePath = variant.SourcePath
				}
			} else {
				target.VariantID = defaultVariant.ID
				targets[tool] = target
				targetsChanged = true
			}
			if err := validateSkillSourcePath(sourcePath); err != nil {
				return fail(err)
			}

			for _, skillRoot := range adapter.GetSkillPaths() {
				if skillRoot == "" {
					continue
				}
				if err := os.MkdirAll(skillRoot, 0o755); err != nil {
					return fail(err)
				}

				destinationPath := filepath.Join(skillRoot, filepath.Base(sourcePath))
				if err := captureSkillPathSnapshot(destinationPath, rollbackDir, snapshots, &snapshotOrder); err != nil {
					return fail(err)
				}
				rootMethod, changed, err := m.syncSkillPath(sourcePath, destinationPath, method)
				if err != nil {
					return fail(err)
				}
				actualMethod = mergeSkillSyncMethod(actualMethod, rootMethod)
				desiredSkills[skillSyncKey(tool, destinationPath)] = AffectedFile{
					Path:      destinationPath,
					Tool:      tool,
					Operation: rootMethod,
				}

				if changed {
					affected = append(affected, AffectedFile{
						Path:      destinationPath,
						Tool:      tool,
						Operation: rootMethod,
					})
				}
			}

			if target.Method != actualMethod {
				target.Method = actualMethod
				targets[tool] = target
				targetsChanged = true
			}
		}

		if targetsChanged {
			skillTargetUpdates[skill.ID] = cloneSkillTargets(targets)
			originalSkillTargets[skill.ID] = originalTargets
		}
	}

	for key, entry := range m.syncedSkills {
		if _, ok := desiredSkills[key]; ok {
			continue
		}
		if err := captureSkillPathSnapshot(entry.Path, rollbackDir, snapshots, &snapshotOrder); err != nil {
			return fail(err)
		}
		if err := os.RemoveAll(entry.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fail(err)
		}
		affected = append(affected, AffectedFile{
			Path:      entry.Path,
			Tool:      entry.Tool,
			Operation: "delete",
		})
	}

	updateIDs := make([]int64, 0, len(skillTargetUpdates))
	for skillID := range skillTargetUpdates {
		updateIDs = append(updateIDs, skillID)
	}
	sort.Slice(updateIDs, func(i, j int) bool { return updateIDs[i] < updateIDs[j] })

	appliedUpdateIDs := make([]int64, 0, len(updateIDs))
	for _, skillID := range updateIDs {
		targets := skillTargetUpdates[skillID]
		if err := m.setSkillTargetsFn(skillID, skillTargetRecordsFromMap(targets)); err != nil {
			targetRollbackErr := rollbackAppliedSkillTargetUpdates(appliedUpdateIDs, originalSkillTargets, m.db.SetSkillTargets)
			fsRollbackErr := rollbackSkillPathSnapshots(snapshotOrder, snapshots)
			return nil, combineErrors(err, combineErrors(targetRollbackErr, fsRollbackErr))
		}
		appliedUpdateIDs = append(appliedUpdateIDs, skillID)
	}

	m.syncedSkills = desiredSkills
	return affected, nil
}

func (m *Manager) syncSkillPath(sourcePath, destinationPath, method string) (string, bool, error) {
	switch method {
	case "symlink", "copy":
	default:
		return "", false, fmt.Errorf("unsupported skill sync method: %s", method)
	}

	samePath, err := skillPathsMatch(sourcePath, destinationPath)
	if err != nil {
		return "", false, err
	}
	if samePath {
		actualMethod, err := detectSkillPathMethod(destinationPath)
		if err != nil {
			return "", false, err
		}
		return actualMethod, false, nil
	}

	overlap, err := skillPathsOverlap(sourcePath, destinationPath)
	if err != nil {
		return "", false, err
	}
	if overlap {
		return "", false, fmt.Errorf("skill source path %q overlaps destination %q", sourcePath, destinationPath)
	}

	if method == "copy" {
		if err := replaceWithCopy(sourcePath, destinationPath); err != nil {
			return "", false, err
		}
		return "copy", true, nil
	}

	if err := m.replaceWithSymlink(sourcePath, destinationPath); err == nil {
		return "symlink", true, nil
	} else {
		if copyErr := replaceWithCopy(sourcePath, destinationPath); copyErr != nil {
			return "", false, fmt.Errorf("sync skill with symlink failed: %w; fallback copy failed: %v", err, copyErr)
		}
		return "copy", true, nil
	}
}

func (m *Manager) replaceWithSymlink(sourcePath, destinationPath string) error {
	if err := os.RemoveAll(destinationPath); err != nil {
		return err
	}
	return m.skillSymlinkFn(sourcePath, destinationPath)
}

func defaultSkillSyncMethodForOS(goos string) string {
	if goos == "windows" {
		return "copy"
	}
	return "symlink"
}

func mergeSkillSyncMethod(current, next string) string {
	if current == "" {
		return next
	}
	if current == "copy" || next == "copy" {
		return "copy"
	}
	return next
}

func skillTargetRecordsFromMap(targets map[string]storage.SkillTargetRecord) []storage.SkillTargetRecord {
	tools := make([]string, 0, len(targets))
	for tool := range targets {
		tools = append(tools, tool)
	}
	sort.Strings(tools)

	records := make([]storage.SkillTargetRecord, 0, len(tools))
	for _, tool := range tools {
		records = append(records, targets[tool])
	}
	return records
}

func skillPathsMatch(sourcePath, destinationPath string) (bool, error) {
	sourceAbs, destinationAbs, err := absoluteSkillPaths(sourcePath, destinationPath)
	if err != nil {
		return false, err
	}
	if sourceAbs == destinationAbs {
		return true, nil
	}

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	destinationInfo, err := os.Stat(destinationPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return os.SameFile(sourceInfo, destinationInfo), nil
}

func skillPathsOverlap(sourcePath, destinationPath string) (bool, error) {
	sourceAbs, destinationAbs, err := absoluteSkillPaths(sourcePath, destinationPath)
	if err != nil {
		return false, err
	}
	if sourceAbs == destinationAbs {
		return false, nil
	}
	return strings.HasPrefix(sourceAbs, destinationAbs+string(os.PathSeparator)) ||
		strings.HasPrefix(destinationAbs, sourceAbs+string(os.PathSeparator)), nil
}

func absoluteSkillPaths(sourcePath, destinationPath string) (string, string, error) {
	sourceAbs, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", "", err
	}
	destinationAbs, err := filepath.Abs(destinationPath)
	if err != nil {
		return "", "", err
	}
	return filepath.Clean(sourceAbs), filepath.Clean(destinationAbs), nil
}

func detectSkillPathMethod(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "symlink", nil
	}
	return "copy", nil
}

func validateSkillSourcePath(sourcePath string) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("skill source_path %q must exist and be a directory", sourcePath)
		}
		return fmt.Errorf("skill source_path %q is invalid: %w", sourcePath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("skill source_path %q must be a directory", sourcePath)
	}
	return nil
}

type skillPathSnapshot struct {
	path          string
	existed       bool
	symlinkTarget string
	backupPath    string
}

func captureSkillPathSnapshot(path, rollbackDir string, snapshots map[string]skillPathSnapshot, order *[]string) error {
	if _, ok := snapshots[path]; ok {
		return nil
	}

	snapshot, err := createSkillPathSnapshot(path, rollbackDir)
	if err != nil {
		return err
	}
	snapshots[path] = snapshot
	*order = append(*order, path)
	return nil
}

func createSkillPathSnapshot(path, rollbackDir string) (skillPathSnapshot, error) {
	snapshot := skillPathSnapshot{path: path}

	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return snapshot, nil
		}
		return snapshot, err
	}

	snapshot.existed = true
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			return snapshot, err
		}
		snapshot.symlinkTarget = target
		return snapshot, nil
	}

	backupBase, err := os.MkdirTemp(rollbackDir, "path-*")
	if err != nil {
		return snapshot, err
	}
	backupPath := filepath.Join(backupBase, "entry")
	if info.IsDir() {
		if err := copyDirectory(path, backupPath); err != nil {
			return snapshot, err
		}
	} else {
		if err := copyFile(path, backupPath, info.Mode()); err != nil {
			return snapshot, err
		}
	}
	snapshot.backupPath = backupPath
	return snapshot, nil
}

func rollbackSkillPathSnapshots(order []string, snapshots map[string]skillPathSnapshot) error {
	var rollbackErr error
	for index := len(order) - 1; index >= 0; index-- {
		snapshot := snapshots[order[index]]
		if err := restoreSkillPathSnapshot(snapshot); err != nil {
			rollbackErr = combineErrors(rollbackErr, fmt.Errorf("restore %s: %w", snapshot.path, err))
		}
	}
	return rollbackErr
}

func restoreSkillPathSnapshot(snapshot skillPathSnapshot) error {
	if err := os.RemoveAll(snapshot.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !snapshot.existed {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(snapshot.path), 0o755); err != nil {
		return err
	}
	if snapshot.symlinkTarget != "" {
		return os.Symlink(snapshot.symlinkTarget, snapshot.path)
	}
	if snapshot.backupPath == "" {
		return nil
	}

	info, err := os.Stat(snapshot.backupPath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDirectory(snapshot.backupPath, snapshot.path)
	}
	return copyFile(snapshot.backupPath, snapshot.path, info.Mode())
}

func cloneSkillTargets(targets map[string]storage.SkillTargetRecord) map[string]storage.SkillTargetRecord {
	cloned := make(map[string]storage.SkillTargetRecord, len(targets))
	for tool, target := range targets {
		cloned[tool] = target
	}
	return cloned
}

func rollbackAppliedSkillTargetUpdates(
	appliedUpdateIDs []int64,
	originalSkillTargets map[int64]map[string]storage.SkillTargetRecord,
	setter func(int64, []storage.SkillTargetRecord) error,
) error {
	var rollbackErr error
	for index := len(appliedUpdateIDs) - 1; index >= 0; index-- {
		skillID := appliedUpdateIDs[index]
		originalTargets, ok := originalSkillTargets[skillID]
		if !ok {
			continue
		}
		if err := setter(skillID, skillTargetRecordsFromMap(originalTargets)); err != nil {
			rollbackErr = combineErrors(rollbackErr, fmt.Errorf("restore skill targets %d: %w", skillID, err))
		}
	}
	return rollbackErr
}

func replaceWithCopy(sourcePath, destinationPath string) error {
	if err := os.RemoveAll(destinationPath); err != nil {
		return err
	}

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}

	if sourceInfo.IsDir() {
		return copyDirectory(sourcePath, destinationPath)
	}
	return copyFile(sourcePath, destinationPath, sourceInfo.Mode())
}

func copyDirectory(sourcePath, destinationPath string) error {
	if err := os.MkdirAll(destinationPath, 0o755); err != nil {
		return err
	}

	return filepath.WalkDir(sourcePath, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == sourcePath {
			return nil
		}

		relative, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destinationPath, relative)

		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(sourcePath, destinationPath string, fileMode os.FileMode) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return err
	}

	destinationFile, err := os.OpenFile(destinationPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileMode.Perm())
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	return err
}

func (m *Manager) TriggerInboundSync() ([]SyncChange, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var changes []SyncChange
	for _, tool := range m.sortedAdapterTools() {
		adapter := m.adapters[tool]
		if !adapter.IsInstalled() {
			continue
		}

		found, err := m.syncEngine.InboundScan(tool, adapter)
		if err != nil {
			return nil, err
		}
		for _, change := range found {
			resolved, err := m.applyInboundChangeLocked(tool, adapter, change)
			if err != nil {
				return nil, err
			}
			if !resolved {
				changes = append(changes, change)
			}
		}
	}

	return changes, nil
}

func (m *Manager) GetSyncStatus() (*SyncStatus, error) {
	changes, err := m.TriggerInboundSync()
	if err != nil {
		return nil, err
	}

	return &SyncStatus{
		ChangesCount: len(changes),
		Conflicts:    len(changes),
		Changes:      changes,
	}, nil
}

func (m *Manager) ResolveConflict(tool, filePath, strategy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch strategy {
	case "keep_external":
		return m.acceptExternalStateLocked(tool, filePath)
	case "keep_ours":
		return m.resolveKeepOursLocked(tool, filePath)
	default:
		return fmt.Errorf("unsupported strategy: %s", strategy)
	}
}

func (m *Manager) resolveKeepOursLocked(tool, filePath string) error {
	adapter, err := m.adapterForManagedFile(tool, filePath)
	if err != nil {
		return err
	}

	providerPath := containsPath(providerConfigPaths(adapter), filePath)
	mcpPath := containsPath(mcpConfigPaths(adapter), filePath)
	updatedPaths := make([]string, 0, 4)

	if providerPath {
		if err := withManagedPathLocks(providerConfigPaths(adapter), func() error {
			return m.reapplyActiveProviderProfileLocked(tool, adapter)
		}); err != nil {
			return err
		}
		updatedPaths = append(updatedPaths, providerConfigPaths(adapter)...)
	}

	if mcpPath {
		err := withManagedPathLocks(mcpConfigPaths(adapter), func() error {
			servers, err := m.managedMCPServersForToolLocked(tool)
			if err != nil {
				return err
			}
			_, err = adapter.SetMCPServers(servers)
			return err
		})
		if err != nil {
			return err
		}
		updatedPaths = append(updatedPaths, mcpConfigPaths(adapter)...)
	}

	if len(updatedPaths) == 0 {
		return fmt.Errorf("config file not managed for keep_ours: %s", filePath)
	}
	return m.recordSyncStateForToolPathsLocked(tool, updatedPaths, "outbound")
}

func (m *Manager) adapterForManagedFile(tool, filePath string) (Adapter, error) {
	adapter, ok := m.adapters[tool]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", tool)
	}

	requestedPath := filepath.Clean(filePath)
	for _, configFile := range adapter.ConfigFiles() {
		if filepath.Clean(configFile.Path) == requestedPath {
			return adapter, nil
		}
	}

	return nil, fmt.Errorf("config file not found for tool %s: %s", tool, filePath)
}

func (m *Manager) reapplyActiveProviderProfileLocked(tool string, adapter Adapter) error {
	profile, err := m.activeProfileLocked()
	if err != nil {
		return err
	}
	if profile == nil {
		return nil
	}

	targets, err := m.db.GetProfileToolTargets(profile.ID)
	if err != nil {
		return err
	}

	target, ok := targets[tool]
	if !ok || !target.Enabled {
		return nil
	}

	providerCfg, err := m.parseAndDecryptConfig(profile.Config)
	if err != nil {
		return err
	}

	_, err = adapter.SetProviderConfig(providerCfg)
	return err
}

func (m *Manager) activeProfileLocked() (*storage.Profile, error) {
	profiles, err := m.db.ListProfiles()
	if err != nil {
		return nil, err
	}

	for _, profile := range profiles {
		if profile.IsActive {
			copied := profile
			return &copied, nil
		}
	}

	return nil, nil
}

func (m *Manager) managedMCPServersForToolLocked(tool string) ([]MCPServerConfig, error) {
	servers, err := m.db.ListMCPServers()
	if err != nil {
		return nil, err
	}

	managed := make([]MCPServerConfig, 0, len(servers))
	for _, server := range servers {
		if !server.Enabled {
			continue
		}

		targets, err := m.db.GetMCPServerTargets(server.ID)
		if err != nil {
			return nil, err
		}
		if !targets[tool] {
			continue
		}

		parsedArgs, err := parseJSONStringSlice(server.Args)
		if err != nil {
			return nil, err
		}
		parsedEnv, err := parseJSONStringMap(server.Env)
		if err != nil {
			return nil, err
		}

		managed = append(managed, MCPServerConfig{
			Name:    server.Name,
			Command: server.Command,
			Args:    parsedArgs,
			Env:     parsedEnv,
		})
	}

	sort.Slice(managed, func(i, j int) bool {
		return managed[i].Name < managed[j].Name
	})

	return managed, nil
}

func (m *Manager) ManualBackup() ([]AffectedFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var affected []AffectedFile
	for _, tool := range m.sortedAdapterTools() {
		adapter := m.adapters[tool]
		if !adapter.IsInstalled() {
			continue
		}

		for _, file := range adapter.ConfigFiles() {
			if !fileExists(file.Path) {
				continue
			}

			slot, backupPath, err := m.backup.Backup(tool, filepath.Base(file.Path), file.Path, "manual")
			if err != nil {
				return nil, err
			}
			if _, err := m.db.InsertBackupRecord(tool, file.Path, backupPath, slot, "manual"); err != nil {
				return nil, err
			}

			affected = append(affected, AffectedFile{
				Path:      file.Path,
				Tool:      tool,
				Operation: "backup",
			})
		}
	}

	return affected, nil
}

func (m *Manager) RestoreBackup(backupID int64) ([]AffectedFile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, err := m.db.GetBackupByID(backupID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, fmt.Errorf("backup not found: %d", backupID)
	}

	if fileExists(record.FilePath) {
		if err := withManagedPathLocks([]string{record.FilePath}, func() error {
			slot, backupPath, err := m.backup.Backup(record.Tool, filepath.Base(record.FilePath), record.FilePath, "restore_safety")
			if err != nil {
				return err
			}
			_, err = m.db.InsertBackupRecord(record.Tool, record.FilePath, backupPath, slot, "restore_safety")
			return err
		}); err != nil {
			return nil, err
		}
	}

	if err := withManagedPathLocks([]string{record.FilePath}, func() error {
		if err := m.backup.Restore(record.BackupPath, record.FilePath); err != nil {
			return err
		}
		return m.acceptExternalStateWithExistingLockLocked(record.Tool, record.FilePath)
	}); err != nil {
		return nil, err
	}

	return []AffectedFile{{
		Path:      record.FilePath,
		Tool:      record.Tool,
		Operation: "restore",
	}}, nil
}

func (m *Manager) ListConfigFiles() []ConfigFileInfo {
	files := []ConfigFileInfo{}
	for _, tool := range m.sortedAdapterTools() {
		files = append(files, m.adapters[tool].ConfigFiles()...)
	}
	return files
}

func (m *Manager) sortedAdapterTools() []string {
	keys := make([]string, 0, len(m.adapters))
	for tool := range m.adapters {
		keys = append(keys, tool)
	}
	sort.Strings(keys)
	return keys
}

func (m *Manager) findProfileIDByName(name string) (int64, error) {
	profiles, err := m.db.ListProfiles()
	if err != nil {
		return 0, err
	}

	for _, profile := range profiles {
		if profile.Name == name {
			return profile.ID, nil
		}
	}

	return 0, fmt.Errorf("profile not found: %s", name)
}

func (m *Manager) normalizeAndEncryptConfig(config string) (string, error) {
	var providerCfg ProviderConfig
	if err := json.Unmarshal([]byte(config), &providerCfg); err != nil {
		return "", err
	}

	if providerCfg.APIKey != "" && !strings.HasPrefix(providerCfg.APIKey, encPrefix) {
		if len(m.encryptionKey) > 0 {
			encrypted, err := Encrypt(providerCfg.APIKey, m.encryptionKey)
			if err != nil {
				return "", err
			}
			providerCfg.APIKey = encrypted
		}
	}

	normalized, err := json.Marshal(providerCfg)
	if err != nil {
		return "", err
	}
	return string(normalized), nil
}

func (m *Manager) parseAndDecryptConfig(config string) (*ProviderConfig, error) {
	cfg := &ProviderConfig{}
	if config == "" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(config), cfg); err != nil {
		return nil, err
	}

	if cfg.APIKey == "" {
		return cfg, nil
	}

	decrypted, err := Decrypt(cfg.APIKey, m.encryptionKey)
	if err != nil {
		if strings.HasPrefix(cfg.APIKey, encPrefix) && errors.Is(err, errInvalidEncryptionKeyLength) && m.keyProvider != nil {
			key, keyErr := m.keyProvider()
			if keyErr != nil {
				return nil, err
			}
			m.encryptionKey = append([]byte(nil), key...)
			decrypted, err = Decrypt(cfg.APIKey, m.encryptionKey)
			if err != nil {
				return nil, err
			}
			cfg.APIKey = decrypted
			return cfg, nil
		}
		return nil, err
	}
	cfg.APIKey = decrypted
	return cfg, nil
}

func (m *Manager) MaskProfileConfig(config string) (string, bool, error) {
	cfg, err := m.parseAndDecryptConfig(config)
	if err != nil {
		return "", false, err
	}

	hasAPIKey := strings.TrimSpace(cfg.APIKey) != ""
	if hasAPIKey {
		cfg.APIKey = MaskAPIKey(cfg.APIKey)
	}

	encoded, err := json.Marshal(cfg)
	if err != nil {
		return "", false, err
	}
	return string(encoded), hasAPIKey, nil
}

func providerConfigHasValues(cfg *ProviderConfig) bool {
	if cfg == nil {
		return false
	}
	if cfg.APIKey != "" || cfg.BaseURL != "" || cfg.Model != "" {
		return true
	}
	return len(cfg.ModelMap) > 0
}

func providerConfigsEqual(left, right *ProviderConfig) bool {
	if !providerConfigHasValues(left) && !providerConfigHasValues(right) {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	if left.APIKey != right.APIKey || left.BaseURL != right.BaseURL || left.Model != right.Model {
		return false
	}
	if len(left.ModelMap) != len(right.ModelMap) {
		return false
	}
	for key, value := range left.ModelMap {
		if right.ModelMap[key] != value {
			return false
		}
	}
	return true
}

func cloneProviderConfig(cfg *ProviderConfig) *ProviderConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	if cfg.ModelMap != nil {
		cloned.ModelMap = make(map[string]string, len(cfg.ModelMap))
		for key, value := range cfg.ModelMap {
			cloned.ModelMap[key] = value
		}
	}
	return &cloned
}

func boolTargetsToStorageTargets(targets map[string]bool) []storage.ToolTarget {
	keys := make([]string, 0, len(targets))
	for tool := range targets {
		keys = append(keys, tool)
	}
	sort.Strings(keys)

	result := make([]storage.ToolTarget, 0, len(keys))
	for _, tool := range keys {
		result = append(result, storage.ToolTarget{
			Tool:    tool,
			Enabled: targets[tool],
		})
	}
	return result
}

func parseJSONStringSlice(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	var parsed []string
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func parseJSONStringMap(value string) (map[string]string, error) {
	if value == "" {
		return nil, nil
	}
	var parsed map[string]string
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func providerConfigPaths(adapter Adapter) []string {
	switch typed := adapter.(type) {
	case *ClaudeAdapter:
		return []string{typed.settingsPath()}
	case *CodexAdapter:
		return []string{typed.authPath(), typed.configPath()}
	case *OpenCodeAdapter:
		return []string{typed.base.configPath}
	case *OpenClawAdapter:
		return []string{typed.base.configPath}
	default:
		paths := make([]string, 0, len(adapter.ConfigFiles()))
		for _, file := range adapter.ConfigFiles() {
			paths = append(paths, file.Path)
		}
		return paths
	}
}

func mcpConfigPaths(adapter Adapter) []string {
	switch typed := adapter.(type) {
	case *ClaudeAdapter:
		return []string{typed.claudeJSONPath}
	case *CodexAdapter:
		return []string{typed.configPath()}
	case *OpenCodeAdapter:
		return []string{typed.base.configPath}
	case *OpenClawAdapter:
		return []string{typed.base.configPath}
	default:
		paths := make([]string, 0, len(adapter.ConfigFiles()))
		for _, file := range adapter.ConfigFiles() {
			paths = append(paths, file.Path)
		}
		return paths
	}
}

func uniqueNonEmptyPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		cleaned := filepath.Clean(path)
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		result = append(result, cleaned)
	}
	sort.Strings(result)
	return result
}

func containsPath(paths []string, target string) bool {
	target = filepath.Clean(target)
	for _, path := range uniqueNonEmptyPaths(paths) {
		if path == target {
			return true
		}
	}
	return false
}

func (m *Manager) ensurePathsUnchangedLocked(tool string, paths []string) error {
	for _, path := range uniqueNonEmptyPaths(paths) {
		_, currentHash, err := fileHashIfExists(path)
		if err != nil {
			return err
		}

		state, err := m.db.GetSyncState(tool, path)
		if err != nil {
			return err
		}
		if state != nil && state.LastHash != currentHash {
			return &ConflictError{
				Tool:     tool,
				FilePath: path,
				Expected: state.LastHash,
				Actual:   currentHash,
			}
		}
	}
	return nil
}

func (m *Manager) recordSyncStateForToolPathsLocked(tool string, paths []string, direction string) error {
	now := time.Now().UTC()
	for _, path := range uniqueNonEmptyPaths(paths) {
		_, currentHash, err := fileHashIfExists(path)
		if err != nil {
			return err
		}
		if err := m.db.UpsertSyncState(tool, path, currentHash, now, direction); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) applyInboundChangeLocked(tool string, adapter Adapter, change SyncChange) (bool, error) {
	providerPath := containsPath(providerConfigPaths(adapter), change.FilePath)
	mcpPath := containsPath(mcpConfigPaths(adapter), change.FilePath)

	mcpDiffers := false
	if mcpPath {
		externalServers, err := adapter.GetMCPServers()
		if err != nil {
			return false, err
		}
		managedServers, err := m.managedMCPServersForToolLocked(tool)
		if err != nil {
			return false, err
		}
		mcpDiffers = !mcpServersEqual(externalServers, managedServers)
	}

	if mcpDiffers {
		return false, nil
	}

	if providerPath {
		if _, err := m.importActiveProviderFromAdapterLocked(tool, adapter); err != nil {
			return false, err
		}
	}

	if err := m.syncEngine.AcceptInbound(tool, change.FilePath); err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) acceptExternalStateLocked(tool, filePath string) error {
	return withManagedPathLocks([]string{filePath}, func() error {
		return m.acceptExternalStateWithExistingLockLocked(tool, filePath)
	})
}

func (m *Manager) acceptExternalStateWithExistingLockLocked(tool, filePath string) error {
	adapter, err := m.adapterForManagedFile(tool, filePath)
	if err != nil {
		return err
	}

	if containsPath(providerConfigPaths(adapter), filePath) {
		if _, err := m.importActiveProviderFromAdapterLocked(tool, adapter); err != nil {
			return err
		}
	}
	if containsPath(mcpConfigPaths(adapter), filePath) {
		if err := m.importExternalMCPStateLocked(tool, adapter); err != nil {
			return err
		}
	}
	return m.recordSyncStateForToolPathsLocked(tool, []string{filePath}, "inbound")
}

func (m *Manager) importActiveProviderFromAdapterLocked(tool string, adapter Adapter) (bool, error) {
	profile, err := m.activeProfileLocked()
	if err != nil {
		return false, err
	}
	if profile == nil {
		return false, nil
	}

	targets, err := m.db.GetProfileToolTargets(profile.ID)
	if err != nil {
		return false, err
	}
	target, ok := targets[tool]
	if !ok || !target.Enabled {
		return false, nil
	}

	externalCfg, err := adapter.GetProviderConfig()
	if err != nil {
		return false, err
	}
	currentCfg, err := m.parseAndDecryptConfig(profile.Config)
	if err != nil {
		return false, err
	}
	if providerConfigsEqual(currentCfg, externalCfg) {
		return false, nil
	}

	encoded, err := json.Marshal(externalCfg)
	if err != nil {
		return false, err
	}
	normalized, err := m.normalizeAndEncryptConfig(string(encoded))
	if err != nil {
		return false, err
	}
	if err := m.db.UpdateProfile(profile.ID, profile.Name, normalized); err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) importExternalMCPStateLocked(tool string, adapter Adapter) error {
	externalServers, err := adapter.GetMCPServers()
	if err != nil {
		return err
	}

	existingServers, err := m.db.ListMCPServers()
	if err != nil {
		return err
	}

	type existingTarget struct {
		record  storage.MCPServerRecord
		targets map[string]bool
	}

	existingByName := make(map[string]existingTarget, len(existingServers))
	for _, server := range existingServers {
		targets, err := m.db.GetMCPServerTargets(server.ID)
		if err != nil {
			return err
		}
		existingByName[server.Name] = existingTarget{record: server, targets: targets}
	}

	seen := make(map[string]struct{}, len(externalServers))
	for _, server := range externalServers {
		args, err := marshalJSONStringSlice(server.Args)
		if err != nil {
			return err
		}
		env, err := marshalJSONStringMap(server.Env)
		if err != nil {
			return err
		}

		if existing, ok := existingByName[server.Name]; ok {
			if err := m.db.UpdateMCPServer(existing.record.ID, server.Name, server.Command, args, env, true); err != nil {
				return err
			}
			targets := cloneBoolMap(existing.targets)
			targets[tool] = true
			if err := m.db.SetMCPServerTargets(existing.record.ID, targets); err != nil {
				return err
			}
		} else {
			id, err := m.db.CreateMCPServer(server.Name, server.Command, args, env)
			if err != nil {
				return err
			}
			if err := m.db.SetMCPServerTargets(id, map[string]bool{tool: true}); err != nil {
				return err
			}
		}
		seen[server.Name] = struct{}{}
	}

	for _, server := range existingServers {
		targets, err := m.db.GetMCPServerTargets(server.ID)
		if err != nil {
			return err
		}
		if !targets[tool] {
			continue
		}
		if _, ok := seen[server.Name]; ok {
			continue
		}

		delete(targets, tool)
		hasEnabledTargets := false
		for _, enabled := range targets {
			if enabled {
				hasEnabledTargets = true
				break
			}
		}
		if hasEnabledTargets {
			if err := m.db.SetMCPServerTargets(server.ID, targets); err != nil {
				return err
			}
			continue
		}
		if err := m.db.DeleteMCPServer(server.ID); err != nil {
			return err
		}
	}

	return nil
}

func cloneBoolMap(targets map[string]bool) map[string]bool {
	cloned := make(map[string]bool, len(targets))
	for tool, enabled := range targets {
		cloned[tool] = enabled
	}
	return cloned
}

func marshalJSONStringSlice(values []string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func marshalJSONStringMap(values map[string]string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func mcpServersEqual(left, right []MCPServerConfig) bool {
	if len(left) != len(right) {
		return false
	}

	leftCopy := append([]MCPServerConfig(nil), left...)
	rightCopy := append([]MCPServerConfig(nil), right...)
	sort.Slice(leftCopy, func(i, j int) bool { return leftCopy[i].Name < leftCopy[j].Name })
	sort.Slice(rightCopy, func(i, j int) bool { return rightCopy[i].Name < rightCopy[j].Name })

	for index := range leftCopy {
		if leftCopy[index].Name != rightCopy[index].Name ||
			leftCopy[index].Command != rightCopy[index].Command ||
			!stringSlicesEqual(leftCopy[index].Args, rightCopy[index].Args) ||
			!stringMapsEqual(leftCopy[index].Env, rightCopy[index].Env) {
			return false
		}
	}
	return true
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func stringMapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func skillSyncKey(tool, path string) string {
	return tool + "\x00" + filepath.Clean(path)
}

func withManagedPathLocks(paths []string, fn func() error) error {
	uniquePaths := uniqueNonEmptyPaths(paths)
	var lockNext func(index int) error
	lockNext = func(index int) error {
		if index >= len(uniquePaths) {
			if fn == nil {
				return nil
			}
			return fn()
		}
		return WithFileLock(uniquePaths[index]+".lock", func() error {
			return lockNext(index + 1)
		})
	}
	return lockNext(0)
}
