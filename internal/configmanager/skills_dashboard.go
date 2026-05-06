package configmanager

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

const (
	skillSourceTypeRepo          = "repo"
	skillSourceTypeImportedTool  = "imported_tool"
	skillSourceTypeImportedLocal = "imported_local"
	skillSourceTypeManual        = "manual"

	skillDashboardStatusHealthy         = "healthy"
	skillDashboardStatusNeedsSync       = "needs_sync"
	skillDashboardStatusUpdateAvailable = "update_available"
	skillDashboardStatusBroken          = "broken"

	skillDashboardActionNone              = "none"
	skillDashboardActionImportIntoLibrary = "import_into_library"
	skillDashboardActionSyncDistribution  = "sync_distribution"
	skillDashboardActionUpdateFromSource  = "update_from_source"
	skillDashboardActionRepair            = "repair"

	skillHashComparisonSynced    = "synced"
	skillHashComparisonDifferent = "different"
	skillHashComparisonMissing   = "missing"
	skillHashComparisonInvalid   = "invalid"
	skillHashComparisonDisabled  = "disabled"
)

type SkillSourceView struct {
	Type          string `json:"type"`
	Label         string `json:"label"`
	RepoOwner     string `json:"repo_owner,omitempty"`
	RepoName      string `json:"repo_name,omitempty"`
	RepoBranch    string `json:"repo_branch,omitempty"`
	RepoSubpath   string `json:"repo_subpath,omitempty"`
	OriginTool    string `json:"origin_tool,omitempty"`
	URL           string `json:"url,omitempty"`
	ReadmeURL     string `json:"readme_url,omitempty"`
	Updatable     bool   `json:"updatable"`
	LastCheckedAt string `json:"last_checked_at,omitempty"`
	LastSyncedAt  string `json:"last_synced_at,omitempty"`
}

type SkillDistributionView struct {
	Tool               string                       `json:"tool"`
	Enabled            bool                         `json:"enabled"`
	Method             string                       `json:"method"`
	Healthy            bool                         `json:"healthy"`
	Status             string                       `json:"status"`
	SyncState          string                       `json:"sync_state"`
	SourceKind         string                       `json:"source_kind"`
	VariantID          int64                        `json:"variant_id,omitempty"`
	LocalSourcePath    string                       `json:"local_source_path,omitempty"`
	LocalSourceHash    string                       `json:"local_source_hash,omitempty"`
	LocalOriginTool    string                       `json:"local_origin_tool,omitempty"`
	LibraryPath        string                       `json:"library_path,omitempty"`
	LibraryHash        string                       `json:"library_hash,omitempty"`
	LibraryShortHash   string                       `json:"library_short_hash,omitempty"`
	InstalledPath      string                       `json:"installed_path,omitempty"`
	InstalledHash      string                       `json:"installed_hash,omitempty"`
	InstalledShortHash string                       `json:"installed_short_hash,omitempty"`
	ComparisonState    string                       `json:"comparison_state"`
	Actual             []SkillOverviewActualInstall `json:"actual"`
}

type SkillDetailsView struct {
	DefinitionStatus string                           `json:"definition_status,omitempty"`
	ProblemReason    string                           `json:"problem_reason,omitempty"`
	CurrentPath      string                           `json:"current_path,omitempty"`
	CurrentHash      string                           `json:"current_hash,omitempty"`
	ArchivedVariants []SkillOverviewVariant           `json:"archived_variants,omitempty"`
	PerTool          []SkillsCLISkillRow              `json:"per_tool"`
	Discovered       []SkillOverviewDiscoveredInstall `json:"discovered,omitempty"`
}

type ManagedSkillView struct {
	ID            int64                   `json:"id"`
	Name          string                  `json:"name"`
	Description   string                  `json:"description"`
	Managed       bool                    `json:"managed"`
	Source        SkillSourceView         `json:"source"`
	Library       SkillOverviewLibrary    `json:"library"`
	Distribution  []SkillDistributionView `json:"distribution"`
	Status        string                  `json:"status"`
	PrimaryAction string                  `json:"primary_action"`
	IssueSummary  string                  `json:"issue_summary"`
	Details       SkillDetailsView        `json:"details"`
}

type SkillsDashboardSummary struct {
	ManagedCount     int `json:"managed_count"`
	HealthyCount     int `json:"healthy_count"`
	IssueCount       int `json:"issue_count"`
	NeedsActionCount int `json:"needs_action_count"`
	SourceCount      int `json:"source_count"`
}

type SkillsDashboard struct {
	LibraryPath string                 `json:"library_path"`
	Summary     SkillsDashboardSummary `json:"summary"`
	Managed     []ManagedSkillView     `json:"managed"`
}

type LocalDiscoveredSkillView struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Path          string          `json:"path"`
	OriginTool    string          `json:"origin_tool"`
	Hash          string          `json:"hash"`
	Importable    bool            `json:"importable"`
	Status        string          `json:"status"`
	PrimaryAction string          `json:"primary_action"`
	IssueSummary  string          `json:"issue_summary"`
	Source        SkillSourceView `json:"source"`
}

type RepoDiscoveredSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	ReadmeURL   string `json:"readme_url"`
	Hash        string `json:"hash,omitempty"`
}

type RepoDiscoveryGroup struct {
	SourceID    int64                 `json:"source_id"`
	SourceLabel string                `json:"source_label"`
	Owner       string                `json:"owner"`
	Repo        string                `json:"repo"`
	Branch      string                `json:"branch"`
	Subpath     string                `json:"subpath"`
	Enabled     bool                  `json:"enabled"`
	SkillCount  int                   `json:"skill_count"`
	Error       string                `json:"error,omitempty"`
	Skills      []RepoDiscoveredSkill `json:"skills"`
}

type SkillsDiscoverSummary struct {
	LocalCount      int `json:"local_count"`
	RepoCount       int `json:"repo_count"`
	ImportableCount int `json:"importable_count"`
}

type SkillsDiscover struct {
	Summary SkillsDiscoverSummary      `json:"summary"`
	Local   []LocalDiscoveredSkillView `json:"local"`
	Repos   []RepoDiscoveryGroup       `json:"repos"`
}

type SkillRepoSourceView struct {
	ID            int64  `json:"id"`
	Owner         string `json:"owner"`
	Repo          string `json:"repo"`
	Branch        string `json:"branch"`
	Subpath       string `json:"subpath"`
	Enabled       bool   `json:"enabled"`
	Label         string `json:"label"`
	SkillCount    int    `json:"skill_count"`
	LastError     string `json:"last_error,omitempty"`
	LastCheckedAt string `json:"last_checked_at,omitempty"`
}

type SkillRepoSourceCreateRequest struct {
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
	Branch  string `json:"branch"`
	Subpath string `json:"subpath"`
	Enabled bool   `json:"enabled"`
}

type ImportExistingSkillsResult struct {
	AffectedFiles []AffectedFile `json:"affected_files"`
	ImportedCount int            `json:"imported_count"`
	SkippedCount  int            `json:"skipped_count"`
}

type SkillsSyncAllResult struct {
	AffectedFiles []AffectedFile `json:"affected_files"`
}

type SkillsRepairIssue struct {
	SkillID      int64  `json:"skill_id"`
	SkillName    string `json:"skill_name"`
	Tool         string `json:"tool,omitempty"`
	Status       string `json:"status"`
	IssueSummary string `json:"issue_summary"`
}

type SkillsRepairResult struct {
	AffectedFiles []AffectedFile      `json:"affected_files"`
	RepairedCount int                 `json:"repaired_count"`
	Unresolved    []SkillsRepairIssue `json:"unresolved"`
}

func (m *Manager) SkillsDashboard() (*SkillsDashboard, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

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
	skills, err = m.pruneStaleLocalSkillRecordsLocked(skills)
	if err != nil {
		return nil, err
	}
	repoSources, err := m.db.ListSkillRepoSources()
	if err != nil {
		return nil, err
	}

	entryIndex := groupSkillEntries(entries)
	result := &SkillsDashboard{
		LibraryPath: libraryPath,
		Managed:     make([]ManagedSkillView, 0, len(skills)),
	}
	result.Summary.SourceCount = len(repoSources)

	for _, skill := range skills {
		view, err := m.buildManagedSkillDashboardView(skill, entryIndex, libraryPath)
		if err != nil {
			return nil, err
		}
		result.Managed = append(result.Managed, view)
		result.Summary.ManagedCount++
		if view.Status == skillDashboardStatusHealthy {
			result.Summary.HealthyCount++
		} else {
			result.Summary.NeedsActionCount++
		}
		if view.Status == skillDashboardStatusBroken {
			result.Summary.IssueCount++
		}
	}

	sort.Slice(result.Managed, func(i, j int) bool {
		left := result.Managed[i]
		right := result.Managed[j]
		if managedStatusPriority(left.Status) != managedStatusPriority(right.Status) {
			return managedStatusPriority(left.Status) < managedStatusPriority(right.Status)
		}
		return strings.ToLower(left.Name) < strings.ToLower(right.Name)
	})

	return result, nil
}

func (m *Manager) buildManagedSkillDashboardView(skill storage.SkillRecord, entryIndex map[string]map[string][]SkillInventoryEntry, libraryPath string) (ManagedSkillView, error) {
	group := entryIndex[normalizeSkillName(skill.Name)]
	perToolRows := make([]SkillsCLISkillRow, 0, len(m.sortedAdapterTools()))
	distribution := make([]SkillDistributionView, 0, len(m.sortedAdapterTools()))
	status := skillDashboardStatusHealthy
	primaryAction := skillDashboardActionNone
	issueSummary := ""

	for _, tool := range m.sortedAdapterTools() {
		row, include, err := m.buildManagedCLISkillRow(skill, tool, group, libraryPath)
		if err != nil {
			return ManagedSkillView{}, err
		}
		if !include {
			continue
		}
		distributionView := skillDistributionViewFromRow(tool, row)
		perToolRows = append(perToolRows, row)
		distribution = append(distribution, distributionView)

		switch {
		case distributionView.ComparisonState == skillHashComparisonInvalid:
			status = skillDashboardStatusBroken
			if primaryAction == skillDashboardActionNone {
				primaryAction = skillDashboardActionRepair
			}
			if issueSummary == "" {
				issueSummary = summarizeCLIRowIssue(tool, row)
			}
		case distributionView.ComparisonState == skillHashComparisonDifferent:
			if status != skillDashboardStatusBroken {
				status = skillDashboardStatusNeedsSync
			}
			if primaryAction == skillDashboardActionNone {
				primaryAction = skillDashboardActionSyncDistribution
			}
			if issueSummary == "" {
				issueSummary = summarizeDistributionComparison(tool, distributionView)
			}
		}
	}

	if skill.Updatable && skill.LastCheckedAt.Valid && skill.LastSyncedAt.Valid && skill.LastCheckedAt.Time.After(skill.LastSyncedAt.Time) {
		status = skillDashboardStatusUpdateAvailable
		primaryAction = skillDashboardActionUpdateFromSource
		if issueSummary == "" {
			issueSummary = "A newer version is available from the source repository."
		}
	}
	if issueSummary == "" {
		issueSummary = "Managed library and installed tool content match."
	}

	defaultVariant, err := m.defaultVariantForSkillLocked(skill)
	if err != nil {
		return ManagedSkillView{}, err
	}
	variants, err := m.db.ListSkillVariants(skill.ID)
	if err != nil {
		return ManagedSkillView{}, err
	}
	archived := make([]SkillOverviewVariant, 0, len(variants))
	for _, variant := range variants {
		if defaultVariant != nil && variant.ID == defaultVariant.ID {
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

	library := SkillOverviewLibrary{
		Present:   defaultVariant != nil,
		Path:      skill.SourcePath,
		Hash:      mustHashSkillPath(skill.SourcePath),
		VariantID: skill.CurrentVariantID,
	}
	if defaultVariant != nil {
		library.Path = defaultVariant.SourcePath
		library.Hash = mustHashSkillPath(defaultVariant.SourcePath)
		library.VariantID = defaultVariant.ID
	}

	details := SkillDetailsView{
		DefinitionStatus: deriveDashboardDefinitionStatus(perToolRows),
		ProblemReason:    deriveDashboardProblemReason(perToolRows),
		CurrentPath:      library.Path,
		CurrentHash:      library.Hash,
		ArchivedVariants: archived,
		PerTool:          perToolRows,
		Discovered:       append(toDiscoveredInstalls("global", group["global"]), collectPerToolDiscovered(group, m.sortedAdapterTools())...),
	}

	return ManagedSkillView{
		ID:            skill.ID,
		Name:          skill.Name,
		Description:   skill.Description,
		Managed:       true,
		Source:        skillSourceViewFromRecord(skill, variants, perToolRows),
		Library:       library,
		Distribution:  distribution,
		Status:        status,
		PrimaryAction: primaryAction,
		IssueSummary:  issueSummary,
		Details:       details,
	}, nil
}

func skillDistributionViewFromRow(tool string, row SkillsCLISkillRow) SkillDistributionView {
	install := preferredActualInstall(row.Binding.Actual)
	libraryHash := row.Global.CurrentHash
	installedHash := install.Hash
	installedPath := install.Path
	method := row.Binding.Method
	if install.Path != "" && install.Method != "" {
		method = install.Method
	}
	return SkillDistributionView{
		Tool:               tool,
		Enabled:            row.Binding.Enabled,
		Method:             method,
		Healthy:            row.Status == skillsCLIStatusOK,
		Status:             row.Status,
		SyncState:          row.SyncState,
		SourceKind:         row.Binding.SourceKind,
		VariantID:          row.Binding.VariantID,
		LocalSourcePath:    row.Binding.LocalSourcePath,
		LocalSourceHash:    row.Binding.LocalSourceHash,
		LocalOriginTool:    row.Local.OriginTool,
		LibraryPath:        row.Global.CurrentPath,
		LibraryHash:        libraryHash,
		LibraryShortHash:   shortSkillHash(libraryHash),
		InstalledPath:      installedPath,
		InstalledHash:      installedHash,
		InstalledShortHash: shortSkillHash(installedHash),
		ComparisonState:    comparisonStateForRow(row, install),
		Actual:             row.Binding.Actual,
	}
}

func preferredActualInstall(actual []SkillOverviewActualInstall) SkillOverviewActualInstall {
	for _, install := range actual {
		if install.Valid {
			return install
		}
	}
	if len(actual) == 0 {
		return SkillOverviewActualInstall{}
	}
	return actual[0]
}

func comparisonStateForRow(row SkillsCLISkillRow, install SkillOverviewActualInstall) string {
	if !row.Binding.Enabled {
		return skillHashComparisonDisabled
	}
	if row.Status == skillsCLIStatusProblem || !row.Global.Valid || (install.Path != "" && !install.Valid) {
		return skillHashComparisonInvalid
	}
	if row.Global.Valid && install.Path == "" {
		return skillHashComparisonMissing
	}
	if row.Global.CurrentHash != "" {
		if install.Hash != "" && row.Global.CurrentHash == install.Hash {
			return skillHashComparisonSynced
		}
		if install.Hash != "" {
			return skillHashComparisonDifferent
		}
	}
	if row.SyncState == skillsCLISyncStateInSync {
		return skillHashComparisonSynced
	}
	if row.SyncState == skillsCLISyncStateCanSyncToCLI {
		return skillHashComparisonMissing
	}
	if row.SyncState == skillsCLISyncStateManagedUsingLocal || row.SyncState == skillsCLISyncStateCanImportToGlobal {
		return skillHashComparisonDifferent
	}
	return skillHashComparisonSynced
}

func shortSkillHash(hash string) string {
	if len(hash) <= 12 {
		return hash
	}
	return hash[:12]
}

func (m *Manager) SkillsDiscover(ctx context.Context) (*SkillsDiscover, error) {
	inventory, err := m.SkillsInventory()
	if err != nil {
		return nil, err
	}
	repos, err := m.RefreshSkillRepoSources(ctx)
	if err != nil {
		return nil, err
	}

	result := &SkillsDiscover{
		Local: make([]LocalDiscoveredSkillView, 0, len(inventory.Discovered)),
		Repos: repos,
	}
	for _, entry := range inventory.Discovered {
		if !entry.Importable {
			continue
		}
		result.Local = append(result.Local, LocalDiscoveredSkillView{
			Name:          entry.Name,
			Description:   entry.Description,
			Path:          entry.Path,
			OriginTool:    entry.Tool,
			Hash:          entry.Hash,
			Importable:    entry.Importable,
			Status:        localDiscoveredStatus(entry),
			PrimaryAction: skillDashboardActionImportIntoLibrary,
			IssueSummary:  localDiscoveredSummary(entry),
			Source: SkillSourceView{
				Type:       skillSourceTypeImportedTool,
				Label:      "Imported from " + toolLabel(entry.Tool),
				OriginTool: entry.Tool,
			},
		})
		result.Summary.LocalCount++
		if entry.Importable {
			result.Summary.ImportableCount++
		}
	}
	result.Summary.RepoCount = len(repos)

	sort.Slice(result.Local, func(i, j int) bool {
		if strings.ToLower(result.Local[i].OriginTool) != strings.ToLower(result.Local[j].OriginTool) {
			return strings.ToLower(result.Local[i].OriginTool) < strings.ToLower(result.Local[j].OriginTool)
		}
		return strings.ToLower(result.Local[i].Name) < strings.ToLower(result.Local[j].Name)
	})
	return result, nil
}

func (m *Manager) ListSkillRepoSources(ctx context.Context) ([]SkillRepoSourceView, error) {
	sources, err := m.db.ListSkillRepoSources()
	if err != nil {
		return nil, err
	}
	refreshed, err := m.RefreshSkillRepoSources(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[int64]RepoDiscoveryGroup{}
	for _, group := range refreshed {
		byID[group.SourceID] = group
	}

	result := make([]SkillRepoSourceView, 0, len(sources))
	for _, source := range sources {
		group := byID[source.ID]
		result = append(result, SkillRepoSourceView{
			ID:            source.ID,
			Owner:         source.Owner,
			Repo:          source.Repo,
			Branch:        source.Branch,
			Subpath:       source.Subpath,
			Enabled:       source.Enabled,
			Label:         formatRepoSourceLabel(source),
			SkillCount:    group.SkillCount,
			LastError:     group.Error,
			LastCheckedAt: time.Now().Format(time.RFC3339),
		})
	}
	return result, nil
}

func (m *Manager) CreateSkillRepoSource(req SkillRepoSourceCreateRequest) (int64, error) {
	owner := strings.TrimSpace(req.Owner)
	repo := strings.TrimSpace(req.Repo)
	if owner == "" || repo == "" {
		return 0, fmt.Errorf("owner and repo are required")
	}
	branch := strings.TrimSpace(req.Branch)
	if branch == "" {
		branch = "main"
	}
	return m.db.CreateSkillRepoSource(owner, repo, branch, strings.Trim(strings.TrimSpace(req.Subpath), "/"), req.Enabled)
}

func (m *Manager) DeleteSkillRepoSource(id int64) error {
	return m.db.DeleteSkillRepoSource(id)
}

func (m *Manager) RefreshSkillRepoSources(ctx context.Context) ([]RepoDiscoveryGroup, error) {
	sources, err := m.db.ListSkillRepoSources()
	if err != nil {
		return nil, err
	}
	result := make([]RepoDiscoveryGroup, 0, len(sources))
	for _, source := range sources {
		group := RepoDiscoveryGroup{
			SourceID:    source.ID,
			SourceLabel: formatRepoSourceLabel(source),
			Owner:       source.Owner,
			Repo:        source.Repo,
			Branch:      source.Branch,
			Subpath:     source.Subpath,
			Enabled:     source.Enabled,
			Skills:      []RepoDiscoveredSkill{},
		}
		if !source.Enabled {
			result = append(result, group)
			continue
		}
		skills, err := m.skillRepoFetcher(ctx, source)
		if err != nil {
			group.Error = err.Error()
			result = append(result, group)
			continue
		}
		sort.Slice(skills, func(i, j int) bool { return strings.ToLower(skills[i].Name) < strings.ToLower(skills[j].Name) })
		group.Skills = skills
		group.SkillCount = len(skills)
		result = append(result, group)
	}
	return result, nil
}

func (m *Manager) ImportExistingSkills() (*ImportExistingSkillsResult, error) {
	result, err := m.ImportNonConflictingSkills()
	if err != nil {
		return nil, err
	}
	return &ImportExistingSkillsResult{
		AffectedFiles: result.AffectedFiles,
		ImportedCount: result.ImportedCount,
		SkippedCount:  result.SkippedCount,
	}, nil
}

func (m *Manager) SyncAllSkills() (*SkillsSyncAllResult, error) {
	affected, err := m.SyncSkills()
	if err != nil {
		return nil, err
	}
	return &SkillsSyncAllResult{AffectedFiles: affected}, nil
}

func (m *Manager) RepairSkills() (*SkillsRepairResult, error) {
	dashboard, err := m.SkillsDashboard()
	if err != nil {
		return nil, err
	}
	unresolved := make([]SkillsRepairIssue, 0)
	shouldSync := false
	for _, skill := range dashboard.Managed {
		switch skill.Status {
		case skillDashboardStatusBroken:
			autoRepairable := false
			for _, row := range skill.Details.PerTool {
				if row.Status == skillsCLIStatusProblem && len(row.Binding.Actual) > 0 {
					autoRepairable = true
					break
				}
			}
			if autoRepairable {
				shouldSync = true
				continue
			}
			unresolved = append(unresolved, SkillsRepairIssue{
				SkillID:      skill.ID,
				SkillName:    skill.Name,
				Status:       skill.Status,
				IssueSummary: skill.IssueSummary,
			})
		case skillDashboardStatusNeedsSync:
			shouldSync = true
		}
	}

	result := &SkillsRepairResult{Unresolved: unresolved}
	if shouldSync {
		affected, err := m.SyncSkills()
		if err != nil {
			return nil, err
		}
		result.AffectedFiles = affected
		result.RepairedCount = len(affected)
	}
	return result, nil
}

func skillSourceViewFromRecord(skill storage.SkillRecord, variants []storage.SkillVariantRecord, perToolRows []SkillsCLISkillRow) SkillSourceView {
	sourceType := skill.SourceType
	if strings.TrimSpace(sourceType) == "" {
		sourceType = skillSourceTypeManual
	}
	originTool := ""
	if sourceType == skillSourceTypeManual {
		originTool = inferredOriginTool(variants, perToolRows)
	}
	label := strings.TrimSpace(skill.SourceLabel)
	if label == "" {
		label = defaultSourceLabel(skill)
	}
	sourceURL := skillSourceURL(skill)
	return SkillSourceView{
		Type:          sourceType,
		Label:         label,
		RepoOwner:     skill.RepoOwner,
		RepoName:      skill.RepoName,
		RepoBranch:    skill.RepoBranch,
		RepoSubpath:   skill.RepoSubpath,
		OriginTool:    originTool,
		URL:           sourceURL,
		ReadmeURL:     skill.ReadmeURL,
		Updatable:     skill.Updatable,
		LastCheckedAt: formatNullTime(skill.LastCheckedAt),
		LastSyncedAt:  formatNullTime(skill.LastSyncedAt),
	}
}

func skillSourceURL(skill storage.SkillRecord) string {
	if strings.TrimSpace(skill.ReadmeURL) != "" {
		return strings.TrimSpace(skill.ReadmeURL)
	}
	if strings.TrimSpace(skill.RepoOwner) == "" || strings.TrimSpace(skill.RepoName) == "" {
		return ""
	}
	branch := strings.TrimSpace(skill.RepoBranch)
	if branch == "" {
		branch = "main"
	}
	repoURL := fmt.Sprintf("https://github.com/%s/%s", strings.TrimSpace(skill.RepoOwner), strings.TrimSpace(skill.RepoName))
	subpath := strings.Trim(strings.TrimSpace(skill.RepoSubpath), "/")
	if subpath == "" {
		return repoURL
	}
	return fmt.Sprintf("%s/tree/%s/%s", repoURL, branch, subpath)
}

func inferredOriginTool(variants []storage.SkillVariantRecord, perToolRows []SkillsCLISkillRow) string {
	for _, variant := range variants {
		origin := strings.TrimSpace(variant.OriginTool)
		if origin != "" && origin != "global" {
			return origin
		}
	}
	for _, row := range perToolRows {
		origin := strings.TrimSpace(row.Local.OriginTool)
		if origin != "" && origin != "global" {
			return origin
		}
	}
	return ""
}

func defaultSourceLabel(skill storage.SkillRecord) string {
	switch strings.TrimSpace(skill.SourceType) {
	case skillSourceTypeRepo:
		if skill.RepoOwner != "" && skill.RepoName != "" {
			return fmt.Sprintf("%s/%s", skill.RepoOwner, skill.RepoName)
		}
		return "Repository"
	case skillSourceTypeImportedTool:
		if skill.SourceLabel != "" {
			return skill.SourceLabel
		}
		return "Imported from tool"
	case skillSourceTypeImportedLocal:
		return "Imported from local folder"
	default:
		return "Manual"
	}
}

func formatNullTime(value sql.NullTime) string {
	if !value.Valid {
		return ""
	}
	return value.Time.Format(time.RFC3339)
}

func managedStatusPriority(status string) int {
	switch status {
	case skillDashboardStatusBroken:
		return 0
	case skillDashboardStatusUpdateAvailable:
		return 1
	case skillDashboardStatusNeedsSync:
		return 2
	default:
		return 3
	}
}

func summarizeCLIRowIssue(tool string, row SkillsCLISkillRow) string {
	if len(row.Issues) > 0 {
		return fmt.Sprintf("%s has a broken skill definition or install that needs repair.", toolLabel(tool))
	}
	return fmt.Sprintf("%s needs repair.", toolLabel(tool))
}

func summarizeCLIRowAction(tool string, row SkillsCLISkillRow) string {
	switch row.PrimaryAction {
	case skillsCLIPrimaryActionImportToGlobal:
		return fmt.Sprintf("%s has a local version that should be imported into managed storage.", toolLabel(tool))
	case skillsCLIPrimaryActionOverrideGlobal:
		return fmt.Sprintf("%s is using a different version than the managed library.", toolLabel(tool))
	case skillsCLIPrimaryActionSyncToCLI:
		return fmt.Sprintf("%s is missing the managed distribution.", toolLabel(tool))
	default:
		return fmt.Sprintf("%s needs attention.", toolLabel(tool))
	}
}

func summarizeDistributionComparison(tool string, distribution SkillDistributionView) string {
	switch distribution.ComparisonState {
	case skillHashComparisonDifferent:
		return fmt.Sprintf("%s installed content differs from the managed library.", toolLabel(tool))
	case skillHashComparisonMissing:
		return fmt.Sprintf("%s does not have an installed copy of the managed skill.", toolLabel(tool))
	case skillHashComparisonInvalid:
		return fmt.Sprintf("%s has an installed copy that cannot be read as a valid skill.", toolLabel(tool))
	default:
		return fmt.Sprintf("%s needs attention.", toolLabel(tool))
	}
}

func deriveDashboardDefinitionStatus(rows []SkillsCLISkillRow) string {
	for _, row := range rows {
		if row.DefinitionStatus != "" && row.DefinitionStatus != skillDefinitionStatusValid {
			return row.DefinitionStatus
		}
	}
	for _, row := range rows {
		if row.DefinitionStatus != "" {
			return row.DefinitionStatus
		}
	}
	return ""
}

func deriveDashboardProblemReason(rows []SkillsCLISkillRow) string {
	for _, row := range rows {
		if row.ProblemReason != "" {
			return row.ProblemReason
		}
	}
	return ""
}

func collectPerToolDiscovered(group map[string][]SkillInventoryEntry, tools []string) []SkillOverviewDiscoveredInstall {
	discovered := make([]SkillOverviewDiscoveredInstall, 0)
	for _, tool := range tools {
		discovered = append(discovered, toDiscoveredInstalls(tool, group[tool])...)
	}
	return discovered
}

func localDiscoveredStatus(entry SkillInventoryEntry) string {
	if entry.Valid && entry.Importable {
		return skillDashboardStatusNeedsSync
	}
	return skillDashboardStatusBroken
}

func localDiscoveredSummary(entry SkillInventoryEntry) string {
	if entry.Valid && entry.Importable {
		return fmt.Sprintf("%s can be imported into the managed library.", toolLabel(entry.Tool))
	}
	if entry.ProblemReason != "" {
		return entry.ProblemReason
	}
	return "This skill cannot be imported yet."
}

func formatRepoSourceLabel(source storage.SkillRepoSourceRecord) string {
	label := fmt.Sprintf("%s/%s", source.Owner, source.Repo)
	if source.Branch != "" && source.Branch != "main" {
		label += "@" + source.Branch
	}
	if source.Subpath != "" {
		label += "/" + source.Subpath
	}
	return label
}

func toolLabel(tool string) string {
	switch strings.TrimSpace(tool) {
	case "claude":
		return "Claude Code"
	case "codex":
		return "Codex"
	case "opencode":
		return "OpenCode"
	case "openclaw":
		return "OpenClaw"
	default:
		return tool
	}
}
