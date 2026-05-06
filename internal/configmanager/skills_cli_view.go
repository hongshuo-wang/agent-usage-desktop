package configmanager

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

const (
	skillsCLIScopeGlobalDefinition = "global_definition"
	skillsCLIScopeLocalDefinition  = "local_definition"
	skillsCLIScopeActualInstall    = "actual_install"

	skillsCLIInstallStatusInvalid = "invalid_install"

	skillsCLISyncStateInSync            = "in_sync"
	skillsCLISyncStateCanImportToGlobal = "can_import_to_global"
	skillsCLISyncStateCanSyncToCLI      = "can_sync_to_cli"
	skillsCLISyncStateManagedUsingLocal = "managed_using_local"
	skillsCLISyncStateDefinitionError   = "definition_error"

	skillsCLIPrimaryActionNone           = "none"
	skillsCLIPrimaryActionImportToGlobal = "import_to_global"
	skillsCLIPrimaryActionSyncToCLI      = "sync_to_cli"
	skillsCLIPrimaryActionOverrideGlobal = "override_global"

	skillsCLIStatusOK      = "ok"
	skillsCLIStatusProblem = "problem"
)

type SkillsCLIOverview struct {
	Tool          string              `json:"tool"`
	LibraryPath   string              `json:"library_path"`
	CLI           SkillCLIStatus      `json:"cli"`
	ToolAvailable bool                `json:"tool_available"`
	Summary       SkillsCLISummary    `json:"summary"`
	Skills        []SkillsCLISkillRow `json:"skills"`
}

type SkillsCLISummary struct {
	VisibleSkills  int `json:"visible_skills"`
	GlobalBindings int `json:"global_bindings"`
	LocalBindings  int `json:"local_bindings"`
	IssueCount     int `json:"issue_count"`
}

type SkillsCLIIssue struct {
	Scope      string            `json:"scope"`
	Code       string            `json:"code"`
	Path       string            `json:"path"`
	MessageKey string            `json:"message_key"`
	Details    map[string]string `json:"details,omitempty"`
}

type SkillsCLISkillRow struct {
	ID               int64                            `json:"id"`
	Name             string                           `json:"name"`
	Description      string                           `json:"description"`
	Managed          bool                             `json:"managed"`
	SourceKind       string                           `json:"source_kind"`
	Status           string                           `json:"status"`
	IsValid          bool                             `json:"is_valid"`
	ProblemReason    string                           `json:"problem_reason"`
	DefinitionStatus string                           `json:"definition_status,omitempty"`
	SyncState        string                           `json:"sync_state"`
	PrimaryAction    string                           `json:"primary_action"`
	ActionSourcePath string                           `json:"action_source_path,omitempty"`
	ActionTargetPath string                           `json:"action_target_path,omitempty"`
	CanDelete        bool                             `json:"can_delete"`
	DeletePath       string                           `json:"delete_path"`
	LegacyIgnored    bool                             `json:"legacy_ignored,omitempty"`
	HiddenReason     string                           `json:"hidden_reason,omitempty"`
	Issues           []SkillsCLIIssue                 `json:"issues"`
	Binding          SkillsCLIBinding                 `json:"binding"`
	Global           SkillsCLIGlobal                  `json:"global"`
	Local            SkillsCLILocal                   `json:"local"`
	ArchivedVersions []SkillOverviewVariant           `json:"archived_versions"`
	Discovered       []SkillOverviewDiscoveredInstall `json:"discovered"`
}

type SkillsCLIBinding struct {
	Enabled         bool                         `json:"enabled"`
	Method          string                       `json:"method"`
	SourceKind      string                       `json:"source_kind"`
	VariantID       int64                        `json:"variant_id"`
	LocalSourcePath string                       `json:"local_source_path"`
	LocalSourceHash string                       `json:"local_source_hash"`
	Actual          []SkillOverviewActualInstall `json:"actual"`
}

type SkillsCLIGlobal struct {
	Present          bool   `json:"present"`
	Valid            bool   `json:"valid"`
	DefinitionStatus string `json:"definition_status,omitempty"`
	ProblemReason    string `json:"problem_reason"`
	CurrentVariantID int64  `json:"current_variant_id"`
	CurrentPath      string `json:"current_path"`
	CurrentHash      string `json:"current_hash"`
}

type SkillsCLILocal struct {
	Present          bool   `json:"present"`
	Valid            bool   `json:"valid"`
	DefinitionStatus string `json:"definition_status,omitempty"`
	ProblemReason    string `json:"problem_reason"`
	Path             string `json:"path"`
	Hash             string `json:"hash"`
	OriginTool       string `json:"origin_tool"`
}

type skillPathState struct {
	Present          bool
	Valid            bool
	DefinitionStatus string
	ProblemReason    string
	Hash             string
}

type cliRowEvaluation struct {
	Status           string
	IsValid          bool
	ProblemReason    string
	DefinitionStatus string
	SyncState        string
	PrimaryAction    string
	ActionSourcePath string
	ActionTargetPath string
	DeletePath       string
	CanDelete        bool
	Issues           []SkillsCLIIssue
}

func (m *Manager) SkillsCLIOverview(tool string) (*SkillsCLIOverview, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.adapters[tool]; !ok {
		return nil, fmt.Errorf("unsupported tool: %s", tool)
	}

	libraryPath, err := skillsLibraryPath()
	if err != nil {
		return nil, err
	}
	entries, err := m.scanSkillInventoryEntries(libraryPath)
	if err != nil {
		return nil, err
	}
	skills, err := m.db.ListSkills()
	if err != nil {
		return nil, err
	}

	entryIndex := groupSkillEntries(entries)
	managedNames := map[string]struct{}{}
	view := &SkillsCLIOverview{
		Tool:          tool,
		LibraryPath:   libraryPath,
		CLI:           m.detectSkillsCLIFn(),
		ToolAvailable: m.skillToolAvailable(tool),
		Skills:        []SkillsCLISkillRow{},
	}

	for _, skill := range skills {
		group := entryIndex[normalizeSkillName(skill.Name)]
		row, include, err := m.buildManagedCLISkillRow(skill, tool, group, libraryPath)
		if err != nil {
			return nil, err
		}
		if !include {
			continue
		}
		view.Skills = append(view.Skills, row)
		managedNames[normalizeSkillName(skill.Name)] = struct{}{}
		accumulateCLISummary(&view.Summary, row)
	}

	for name, group := range entryIndex {
		if _, ok := managedNames[name]; ok {
			continue
		}
		row, include := m.buildUnmanagedCLISkillRow(tool, group, libraryPath)
		if !include {
			continue
		}
		view.Skills = append(view.Skills, row)
		accumulateCLISummary(&view.Summary, row)
	}

	view.Summary.VisibleSkills = len(view.Skills)
	sort.Slice(view.Skills, func(i, j int) bool {
		left := view.Skills[i]
		right := view.Skills[j]
		if cliStatusPriority(left.Status) != cliStatusPriority(right.Status) {
			return cliStatusPriority(left.Status) < cliStatusPriority(right.Status)
		}
		return left.Name < right.Name
	})
	return view, nil
}

func (m *Manager) buildManagedCLISkillRow(skill storage.SkillRecord, tool string, group map[string][]SkillInventoryEntry, libraryPath string) (SkillsCLISkillRow, bool, error) {
	targets, err := m.db.GetSkillTargets(skill.ID)
	if err != nil {
		return SkillsCLISkillRow{}, false, err
	}
	target, hasTarget := targets[tool]
	target = normalizeSkillTargetRecord(target)
	actual := toActualInstalls(group[tool])

	defaultVariant, err := m.defaultVariantForSkillLocked(skill)
	if err != nil {
		return SkillsCLISkillRow{}, false, err
	}

	if ignored, err := shouldIgnoreLegacyContainerRecord(skill.SourcePath); err != nil {
		return SkillsCLISkillRow{}, false, err
	} else if ignored {
		return SkillsCLISkillRow{}, false, nil
	}
	if target.SourceKind == "local" {
		if ignored, err := shouldIgnoreLegacyContainerRecord(target.LocalSourcePath); err != nil {
			return SkillsCLISkillRow{}, false, err
		} else if ignored {
			return SkillsCLISkillRow{}, false, nil
		}
	}
	if defaultVariant != nil {
		if ignored, err := shouldIgnoreLegacyContainerRecord(defaultVariant.SourcePath); err != nil {
			return SkillsCLISkillRow{}, false, err
		} else if ignored {
			return SkillsCLISkillRow{}, false, nil
		}
	}

	global, err := buildCLIGlobalState(defaultVariant, skill.CurrentVariantID)
	if err != nil {
		return SkillsCLISkillRow{}, false, err
	}
	local, err := buildCLILocalState(tool, target, group[tool])
	if err != nil {
		return SkillsCLISkillRow{}, false, err
	}

	variants, err := m.db.ListSkillVariants(skill.ID)
	if err != nil {
		return SkillsCLISkillRow{}, false, err
	}
	archived := make([]SkillOverviewVariant, 0, len(variants))
	for _, variant := range variants {
		if global.CurrentVariantID == variant.ID {
			continue
		}
		archived = append(archived, SkillOverviewVariant{
			ID:         variant.ID,
			SourcePath: variant.SourcePath,
			OriginTool: variant.OriginTool,
			Hash:       mustHashSkillPath(variant.SourcePath),
			Managed:    true,
		})
	}
	sort.Slice(archived, func(i, j int) bool { return archived[i].ID < archived[j].ID })

	row := SkillsCLISkillRow{
		ID:          skill.ID,
		Name:        skill.Name,
		Description: skill.Description,
		Managed:     true,
		Issues:      []SkillsCLIIssue{},
		Binding: SkillsCLIBinding{
			Enabled:         hasTarget && target.Enabled,
			Method:          target.Method,
			SourceKind:      target.SourceKind,
			VariantID:       target.VariantID,
			LocalSourcePath: target.LocalSourcePath,
			LocalSourceHash: target.LocalSourceHash,
			Actual:          actual,
		},
		Global:           global,
		Local:            local,
		ArchivedVersions: archived,
		Discovered:       append(toDiscoveredInstalls("global", group["global"]), toDiscoveredInstalls(tool, group[tool])...),
	}
	row.SourceKind = deriveCurrentSourceKind(row.Binding, row.Local)

	evaluation := evaluateManagedCLIRow(row)
	if evaluation.PrimaryAction == skillsCLIPrimaryActionSyncToCLI && evaluation.ActionTargetPath == "" {
		evaluation.ActionTargetPath = m.defaultSkillInstallPath(tool, row.Name)
	}
	if evaluation.PrimaryAction == skillsCLIPrimaryActionImportToGlobal || evaluation.PrimaryAction == skillsCLIPrimaryActionOverrideGlobal {
		evaluation.ActionTargetPath = libraryPath
	}
	applyCLIRowEvaluation(&row, evaluation)
	return row, true, nil
}

func (m *Manager) buildUnmanagedCLISkillRow(tool string, group map[string][]SkillInventoryEntry, libraryPath string) (SkillsCLISkillRow, bool) {
	if len(group[tool]) == 0 {
		return SkillsCLISkillRow{}, false
	}
	localEntry := preferredSkillInventoryEntry(group[tool])
	globalEntry := SkillInventoryEntry{}
	if len(group["global"]) > 0 {
		globalEntry = preferredSkillInventoryEntry(group["global"])
	}
	row := SkillsCLISkillRow{
		Name:        localEntry.Name,
		Description: localEntry.Description,
		Managed:     false,
		SourceKind:  "local",
		Issues:      []SkillsCLIIssue{},
		Binding: SkillsCLIBinding{
			Method:     skillInstallMethod(localEntry),
			SourceKind: "local",
			Actual:     toActualInstalls(group[tool]),
		},
		Global: SkillsCLIGlobal{
			Present:          globalEntry.Path != "",
			Valid:            globalEntry.Valid,
			DefinitionStatus: definitionStatusForInventoryEntry(globalEntry),
			ProblemReason:    globalEntry.ProblemReason,
			CurrentPath:      globalEntry.Path,
			CurrentHash:      globalEntry.Hash,
			CurrentVariantID: 0,
		},
		Local: SkillsCLILocal{
			Present:          true,
			Valid:            localEntry.Valid,
			DefinitionStatus: definitionStatusForInventoryEntry(localEntry),
			ProblemReason:    localEntry.ProblemReason,
			Path:             localEntry.Path,
			Hash:             localEntry.Hash,
			OriginTool:       tool,
		},
		Discovered: append(toDiscoveredInstalls("global", group["global"]), toDiscoveredInstalls(tool, group[tool])...),
	}
	evaluation := evaluateUnmanagedCLIRow(row)
	if evaluation.PrimaryAction == skillsCLIPrimaryActionImportToGlobal || evaluation.PrimaryAction == skillsCLIPrimaryActionOverrideGlobal {
		evaluation.ActionTargetPath = libraryPath
	}
	applyCLIRowEvaluation(&row, evaluation)
	return row, true
}

func buildCLIGlobalState(defaultVariant *storage.SkillVariantRecord, currentVariantID int64) (SkillsCLIGlobal, error) {
	global := SkillsCLIGlobal{CurrentVariantID: currentVariantID}
	if defaultVariant == nil {
		return global, nil
	}
	global.CurrentVariantID = defaultVariant.ID
	global.CurrentPath = defaultVariant.SourcePath
	global.CurrentHash = mustHashSkillPath(defaultVariant.SourcePath)

	state, err := inspectSkillPathState(defaultVariant.SourcePath, global.CurrentHash, skillsCLIScopeGlobalDefinition)
	if err != nil {
		return SkillsCLIGlobal{}, err
	}
	global.Present = state.Present
	global.Valid = state.Valid
	global.DefinitionStatus = state.DefinitionStatus
	global.ProblemReason = state.ProblemReason
	global.CurrentHash = state.Hash
	return global, nil
}

func buildCLILocalState(tool string, target storage.SkillTargetRecord, entries []SkillInventoryEntry) (SkillsCLILocal, error) {
	if target.SourceKind == "local" && target.LocalSourcePath != "" {
		state, err := inspectSkillPathState(target.LocalSourcePath, target.LocalSourceHash, skillsCLIScopeLocalDefinition)
		if err != nil {
			return SkillsCLILocal{}, err
		}
		return SkillsCLILocal{
			Present:          state.Present,
			Valid:            state.Valid,
			DefinitionStatus: state.DefinitionStatus,
			ProblemReason:    state.ProblemReason,
			Path:             target.LocalSourcePath,
			Hash:             state.Hash,
			OriginTool:       firstNonEmpty(target.LocalOriginTool, tool),
		}, nil
	}
	if len(entries) == 0 {
		return SkillsCLILocal{}, nil
	}
	entry := preferredSkillInventoryEntry(entries)
	return SkillsCLILocal{
		Present:          true,
		Valid:            entry.Valid,
		DefinitionStatus: definitionStatusForInventoryEntry(entry),
		ProblemReason:    entry.ProblemReason,
		Path:             entry.Path,
		Hash:             entry.Hash,
		OriginTool:       tool,
	}, nil
}

func preferredSkillInventoryEntry(entries []SkillInventoryEntry) SkillInventoryEntry {
	for _, entry := range entries {
		if entry.Valid {
			return entry
		}
	}
	if len(entries) == 0 {
		return SkillInventoryEntry{}
	}
	return entries[0]
}

func inspectSkillPathState(path, hash, scope string) (skillPathState, error) {
	status, err := inspectSkillDefinitionStatus(path)
	if err != nil {
		return skillPathState{}, err
	}
	state := skillPathState{
		Present:          status != skillDefinitionStatusMissingPath,
		Valid:            status == skillDefinitionStatusValid,
		DefinitionStatus: status,
		ProblemReason:    legacyProblemReasonForDefinitionStatus(status, scope),
		Hash:             hash,
	}
	if state.Valid && state.Hash == "" {
		state.Hash = mustHashSkillPath(path)
	}
	return state, nil
}

func definitionStatusForInventoryEntry(entry SkillInventoryEntry) string {
	if entry.Path == "" {
		return ""
	}
	status, err := inspectSkillDefinitionStatus(entry.Path)
	if err == nil {
		return status
	}
	if entry.Valid {
		return skillDefinitionStatusValid
	}
	return skillDefinitionStatusMissingSkillMD
}

func deriveCurrentSourceKind(binding SkillsCLIBinding, local SkillsCLILocal) string {
	if binding.Enabled {
		if binding.SourceKind == "local" {
			return "local"
		}
		return "global"
	}
	if local.Present || local.Path != "" {
		return "local"
	}
	return "global"
}

func evaluateManagedCLIRow(row SkillsCLISkillRow) cliRowEvaluation {
	evaluation := baseCLIRowEvaluation(row)

	appendInvalidDefinitionIssues(&evaluation, row.Global.CurrentPath, row.Global.DefinitionStatus, skillsCLIScopeGlobalDefinition)
	appendInvalidDefinitionIssues(&evaluation, row.Local.Path, row.Local.DefinitionStatus, skillsCLIScopeLocalDefinition)
	invalidActuals := invalidActualInstallIssues(row.Binding.Actual)
	evaluation.Issues = append(evaluation.Issues, invalidActuals...)
	if len(evaluation.Issues) > 0 {
		if row.SourceKind == "global" && row.Global.CurrentPath != "" && row.Global.DefinitionStatus != "" && row.Global.DefinitionStatus != skillDefinitionStatusValid {
			evaluation.CanDelete = true
			evaluation.DeletePath = row.Global.CurrentPath
		} else if row.Local.Path != "" && row.Local.DefinitionStatus != "" && row.Local.DefinitionStatus != skillDefinitionStatusValid {
			evaluation.CanDelete = true
			evaluation.DeletePath = row.Local.Path
		} else if len(invalidActuals) == 1 {
			evaluation.CanDelete = true
			evaluation.DeletePath = invalidActuals[0].Path
		}
		return finalizeCLIRowEvaluation(evaluation)
	}

	return finalizeCLIRowEvaluation(assignCLISyncState(row, evaluation))
}

func evaluateUnmanagedCLIRow(row SkillsCLISkillRow) cliRowEvaluation {
	evaluation := baseCLIRowEvaluation(row)

	appendInvalidDefinitionIssues(&evaluation, row.Global.CurrentPath, row.Global.DefinitionStatus, skillsCLIScopeGlobalDefinition)
	appendInvalidDefinitionIssues(&evaluation, row.Local.Path, row.Local.DefinitionStatus, skillsCLIScopeLocalDefinition)
	invalidActuals := invalidActualInstallIssues(row.Binding.Actual)
	evaluation.Issues = append(evaluation.Issues, invalidActuals...)
	if len(evaluation.Issues) > 0 {
		if row.Local.Path != "" {
			evaluation.CanDelete = true
			evaluation.DeletePath = row.Local.Path
		} else if len(invalidActuals) == 1 {
			evaluation.CanDelete = true
			evaluation.DeletePath = invalidActuals[0].Path
		}
		return finalizeCLIRowEvaluation(evaluation)
	}

	return finalizeCLIRowEvaluation(assignCLISyncState(row, evaluation))
}

func baseCLIRowEvaluation(row SkillsCLISkillRow) cliRowEvaluation {
	return cliRowEvaluation{
		Status:           skillsCLIStatusOK,
		IsValid:          true,
		DefinitionStatus: deriveRowDefinitionStatus(row),
		SyncState:        skillsCLISyncStateInSync,
		PrimaryAction:    skillsCLIPrimaryActionNone,
		Issues:           []SkillsCLIIssue{},
	}
}

func appendInvalidDefinitionIssues(evaluation *cliRowEvaluation, path, definitionStatus, scope string) {
	if definitionStatus == "" || definitionStatus == skillDefinitionStatusValid {
		return
	}
	evaluation.Issues = append(evaluation.Issues, newDefinitionIssue(scope, definitionStatus, path))
}

func assignCLISyncState(row SkillsCLISkillRow, evaluation cliRowEvaluation) cliRowEvaluation {
	switch {
	case row.Local.Valid && !row.Global.Valid:
		evaluation.SyncState = skillsCLISyncStateCanImportToGlobal
		evaluation.PrimaryAction = skillsCLIPrimaryActionImportToGlobal
		evaluation.ActionSourcePath = row.Local.Path
	case row.Global.Valid && !row.Local.Valid:
		evaluation.SyncState = skillsCLISyncStateCanSyncToCLI
		evaluation.PrimaryAction = skillsCLIPrimaryActionSyncToCLI
		evaluation.ActionSourcePath = row.Global.CurrentPath
	case row.Global.Valid && row.Local.Valid:
		switch {
		case row.Global.CurrentHash != "" && row.Global.CurrentHash == row.Local.Hash:
			evaluation.SyncState = skillsCLISyncStateInSync
		case row.Local.Hash != "":
			evaluation.SyncState = skillsCLISyncStateManagedUsingLocal
			evaluation.PrimaryAction = skillsCLIPrimaryActionOverrideGlobal
			evaluation.ActionSourcePath = row.Local.Path
		default:
			evaluation.SyncState = skillsCLISyncStateInSync
		}
	default:
		evaluation.SyncState = skillsCLISyncStateInSync
	}
	return evaluation
}

func applyCLIRowEvaluation(row *SkillsCLISkillRow, evaluation cliRowEvaluation) {
	row.Status = evaluation.Status
	row.IsValid = evaluation.IsValid
	row.ProblemReason = evaluation.ProblemReason
	row.DefinitionStatus = evaluation.DefinitionStatus
	row.SyncState = evaluation.SyncState
	row.PrimaryAction = evaluation.PrimaryAction
	row.ActionSourcePath = evaluation.ActionSourcePath
	row.ActionTargetPath = evaluation.ActionTargetPath
	row.DeletePath = evaluation.DeletePath
	row.CanDelete = evaluation.CanDelete
	row.Issues = evaluation.Issues
}

func finalizeCLIRowEvaluation(evaluation cliRowEvaluation) cliRowEvaluation {
	sort.SliceStable(evaluation.Issues, func(i, j int) bool {
		left := evaluation.Issues[i]
		right := evaluation.Issues[j]
		if cliIssuePriority(left) != cliIssuePriority(right) {
			return cliIssuePriority(left) < cliIssuePriority(right)
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		return left.Code < right.Code
	})

	if len(evaluation.Issues) > 0 {
		evaluation.Status = skillsCLIStatusProblem
		evaluation.IsValid = false
		evaluation.SyncState = skillsCLISyncStateDefinitionError
		evaluation.PrimaryAction = skillsCLIPrimaryActionNone
		evaluation.ActionSourcePath = ""
		evaluation.ActionTargetPath = ""
		evaluation.ProblemReason = legacyProblemReasonFromIssue(evaluation.Issues[0])
	}
	return evaluation
}

func deriveRowDefinitionStatus(row SkillsCLISkillRow) string {
	for _, status := range []string{row.Local.DefinitionStatus, row.Global.DefinitionStatus} {
		if status != "" && status != skillDefinitionStatusValid {
			return status
		}
	}
	if row.Local.DefinitionStatus != "" {
		return row.Local.DefinitionStatus
	}
	return row.Global.DefinitionStatus
}

func invalidActualInstallIssues(actual []SkillOverviewActualInstall) []SkillsCLIIssue {
	issues := make([]SkillsCLIIssue, 0)
	for _, install := range actual {
		if install.Valid {
			continue
		}
		issues = append(issues, newInstallIssue(
			skillsCLIInstallStatusInvalid,
			install.Path,
			map[string]string{
				"actual_path": install.Path,
			},
		))
	}
	return issues
}

func newDefinitionIssue(scope, code, path string) SkillsCLIIssue {
	return SkillsCLIIssue{
		Scope:      scope,
		Code:       code,
		Path:       path,
		MessageKey: cliIssueMessageKey(code),
	}
}

func newInstallIssue(code, path string, details map[string]string) SkillsCLIIssue {
	return SkillsCLIIssue{
		Scope:      skillsCLIScopeActualInstall,
		Code:       code,
		Path:       path,
		MessageKey: cliIssueMessageKey(code),
		Details:    details,
	}
}

func cliIssueMessageKey(code string) string {
	switch code {
	case skillDefinitionStatusMissingPath:
		return "skillsIssueDefinitionMissingPath"
	case skillDefinitionStatusNotDirectory:
		return "skillsIssueDefinitionNotDirectory"
	case skillDefinitionStatusMissingSkillMD:
		return "skillsIssueDefinitionMissingSkillFile"
	case skillDefinitionStatusInvalidSkillMD:
		return "skillsIssueDefinitionInvalidSkillFile"
	case skillsCLIInstallStatusInvalid:
		return "skillsIssueInstallInvalid"
	default:
		return "skillsStatusProblem"
	}
}

func legacyProblemReasonFromIssue(issue SkillsCLIIssue) string {
	switch issue.Code {
	case skillDefinitionStatusMissingPath:
		return legacyProblemReasonForDefinitionStatus(issue.Code, issue.Scope)
	case skillDefinitionStatusNotDirectory, skillDefinitionStatusMissingSkillMD, skillDefinitionStatusInvalidSkillMD, skillsCLIInstallStatusInvalid:
		return skillProblemInvalidDefinition
	default:
		return ""
	}
}

func cliIssuePriority(issue SkillsCLIIssue) int {
	if issue.Scope == skillsCLIScopeGlobalDefinition || issue.Scope == skillsCLIScopeLocalDefinition {
		return 0
	}
	return 1
}

func shouldIgnoreLegacyContainerRecord(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	status, err := inspectSkillDefinitionStatus(path)
	if err != nil {
		return false, err
	}
	return status == skillDefinitionStatusLegacyContainer, nil
}

func accumulateCLISummary(summary *SkillsCLISummary, row SkillsCLISkillRow) {
	if row.SourceKind == "local" {
		summary.LocalBindings++
	} else {
		summary.GlobalBindings++
	}
	if row.Status == skillsCLIStatusProblem {
		summary.IssueCount++
	}
}

func cliStatusPriority(status string) int {
	switch status {
	case skillsCLIStatusProblem:
		return 0
	default:
		return 1
	}
}

func (m *Manager) defaultSkillInstallPath(tool, name string) string {
	adapter := m.adapters[tool]
	if adapter == nil {
		return ""
	}
	for _, root := range adapter.GetSkillPaths() {
		if strings.TrimSpace(root) == "" {
			continue
		}
		return filepath.Join(root, sanitizeSkillDirName(name))
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
