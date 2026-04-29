package configmanager

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

type SkillOverview struct {
	LibraryPath      string               `json:"library_path"`
	CLI              SkillCLIStatus       `json:"cli"`
	ToolAvailability map[string]bool      `json:"tool_availability"`
	Summary          SkillOverviewSummary `json:"summary"`
	Skills           []SkillOverviewItem  `json:"skills"`
}

type SkillOverviewSummary struct {
	ManagedSkills   int `json:"managed_skills"`
	VisibleSkills   int `json:"visible_skills"`
	ConnectedTools  int `json:"connected_tools"`
	IssueCount      int `json:"issue_count"`
	UnmanagedSkills int `json:"unmanaged_skills"`
}

type SkillOverviewItem struct {
	ID          int64                             `json:"id"`
	Name        string                            `json:"name"`
	Description string                            `json:"description"`
	Managed     bool                              `json:"managed"`
	Enabled     bool                              `json:"enabled"`
	PrimaryPath string                            `json:"primary_path"`
	Library     SkillOverviewLibrary              `json:"library"`
	Variants    []SkillOverviewVariant            `json:"variants"`
	Tools       map[string]SkillOverviewToolState `json:"tools"`
	Issues      []SkillOverviewIssue              `json:"issues"`
	Discovered  []SkillOverviewDiscoveredInstall  `json:"discovered"`
}

type SkillOverviewLibrary struct {
	Present   bool   `json:"present"`
	Path      string `json:"path"`
	Hash      string `json:"hash"`
	VariantID int64  `json:"variant_id"`
}

type SkillOverviewVariant struct {
	ID         int64  `json:"id"`
	SourcePath string `json:"source_path"`
	OriginTool string `json:"origin_tool"`
	Hash       string `json:"hash"`
	Managed    bool   `json:"managed"`
}

type SkillOverviewToolState struct {
	Enabled           bool                         `json:"enabled"`
	Method            string                       `json:"method"`
	SelectedVariantID int64                        `json:"selected_variant_id"`
	SelectedPath      string                       `json:"selected_path"`
	SelectedHash      string                       `json:"selected_hash"`
	Status            string                       `json:"status"`
	Actual            []SkillOverviewActualInstall `json:"actual"`
}

type SkillOverviewActualInstall struct {
	Path   string `json:"path"`
	Hash   string `json:"hash"`
	Method string `json:"method"`
}

type SkillOverviewDiscoveredInstall struct {
	Path   string `json:"path"`
	Tool   string `json:"tool"`
	Hash   string `json:"hash"`
	Method string `json:"method"`
}

type SkillOverviewIssue struct {
	Tool string `json:"tool"`
	Code string `json:"code"`
}

type ImportManagedSkillRequest struct {
	SkillID    int64  `json:"skill_id"`
	Name       string `json:"name"`
	Tool       string `json:"tool"`
	SourcePath string `json:"source_path"`
}

type ImportManagedSkillResult struct {
	SkillID    int64 `json:"skill_id"`
	VariantID  int64 `json:"variant_id"`
	CreatedNew bool  `json:"created_new"`
}

func (m *Manager) SkillsOverview() (*SkillOverview, error) {
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

	entryIndex := groupSkillEntries(entries)
	itemsByName := map[string]*SkillOverviewItem{}
	overview := &SkillOverview{
		LibraryPath:      libraryPath,
		CLI:              m.detectSkillsCLIFn(),
		ToolAvailability: map[string]bool{},
		Skills:           []SkillOverviewItem{},
	}

	for _, tool := range m.sortedAdapterTools() {
		if adapter, ok := m.adapters[tool]; ok && adapter != nil {
			overview.ToolAvailability[tool] = toolCLIAvailable(tool)
		}
	}

	for _, skill := range skills {
		item, err := m.buildManagedSkillOverview(skill, entryIndex)
		if err != nil {
			return nil, err
		}
		key := normalizeSkillName(item.Name)
		itemsByName[key] = &item
	}

	for key, group := range entryIndex {
		if _, ok := itemsByName[key]; ok {
			continue
		}
		item := buildUnmanagedSkillOverview(group, m.sortedAdapterTools())
		itemsByName[key] = &item
	}

	for _, item := range itemsByName {
		overview.Skills = append(overview.Skills, *item)
		if item.Managed {
			overview.Summary.ManagedSkills++
		} else {
			overview.Summary.UnmanagedSkills++
		}
		overview.Summary.IssueCount += len(item.Issues)
		for _, tool := range item.Tools {
			if tool.Enabled {
				overview.Summary.ConnectedTools++
			}
		}
	}
	overview.Summary.VisibleSkills = len(overview.Skills)

	sort.Slice(overview.Skills, func(i, j int) bool {
		left := overview.Skills[i]
		right := overview.Skills[j]
		if len(left.Issues) != len(right.Issues) {
			return len(left.Issues) > len(right.Issues)
		}
		if left.Managed != right.Managed {
			return left.Managed
		}
		return strings.ToLower(left.Name) < strings.ToLower(right.Name)
	})

	return overview, nil
}

func (m *Manager) ImportManagedSkill(req ImportManagedSkillRequest) (*ImportManagedSkillResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if strings.TrimSpace(req.Tool) == "" {
		return nil, fmt.Errorf("tool is required")
	}
	if strings.TrimSpace(req.SourcePath) == "" {
		return nil, fmt.Errorf("source_path is required")
	}
	if err := validateSkillSourcePath(req.SourcePath); err != nil {
		return nil, err
	}
	if req.Tool != "global" {
		if _, ok := m.adapters[req.Tool]; !ok {
			return nil, fmt.Errorf("unsupported tool: %s", req.Tool)
		}
	}

	metadata, err := parseSkillMetadata(req.SourcePath)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = strings.TrimSpace(metadata.Name)
	}
	if name == "" {
		name = filepath.Base(strings.TrimRight(req.SourcePath, string(os.PathSeparator)))
	}
	description := strings.TrimSpace(metadata.Description)

	libraryRoot, err := skillsLibraryPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(libraryRoot, 0o755); err != nil {
		return nil, err
	}

	existingSkills, err := m.db.ListSkills()
	if err != nil {
		return nil, err
	}

	var skill *storage.SkillRecord
	if req.SkillID > 0 {
		skill, err = m.db.GetSkill(req.SkillID)
		if err != nil {
			return nil, err
		}
		if skill == nil {
			return nil, fmt.Errorf("skill not found: %d", req.SkillID)
		}
	} else {
		for _, candidate := range existingSkills {
			if normalizeSkillName(candidate.Name) == normalizeSkillName(name) {
				copied := candidate
				skill = &copied
				break
			}
		}
	}

	sourceHash, err := hashSkillDirectory(req.SourcePath)
	if err != nil {
		return nil, err
	}

	if skill == nil {
		managedPath, err := m.prepareManagedSkillPath(name, req.SourcePath, sourceHash, libraryRoot)
		if err != nil {
			return nil, err
		}
		skillID, err := m.db.CreateSkill(name, managedPath, description)
		if err != nil {
			return nil, err
		}
		variantID, err := m.ensureSkillVariantLocked(skillID, managedPath, req.Tool)
		if err != nil {
			_ = m.db.DeleteSkill(skillID)
			return nil, err
		}
		if req.Tool != "global" {
			targets := map[string]storage.SkillTargetRecord{
				req.Tool: {
					Tool:      req.Tool,
					Method:    defaultSkillSyncMethodForOS(runtime.GOOS),
					Enabled:   true,
					VariantID: variantID,
				},
			}
			if err := m.db.SetSkillTargets(skillID, skillTargetRecordsFromMap(targets)); err != nil {
				_ = m.db.DeleteSkill(skillID)
				return nil, err
			}
		}
		return &ImportManagedSkillResult{SkillID: skillID, VariantID: variantID, CreatedNew: true}, nil
	}

	variants, err := m.db.ListSkillVariants(skill.ID)
	if err != nil {
		return nil, err
	}
	variantID := int64(0)
	for _, variant := range variants {
		hash, hashErr := hashSkillDirectory(variant.SourcePath)
		if hashErr == nil && hash == sourceHash {
			variantID = variant.ID
			break
		}
	}
	if variantID == 0 {
		managedPath, err := m.prepareManagedSkillPath(skill.Name, req.SourcePath, sourceHash, libraryRoot)
		if err != nil {
			return nil, err
		}
		variantID, err = m.ensureSkillVariantLocked(skill.ID, managedPath, req.Tool)
		if err != nil {
			return nil, err
		}
	}
	if req.Tool == "global" {
		return &ImportManagedSkillResult{SkillID: skill.ID, VariantID: variantID, CreatedNew: false}, nil
	}
	targets, err := m.db.GetSkillTargets(skill.ID)
	if err != nil {
		return nil, err
	}
	current := targets[req.Tool]
	if current.Method == "" {
		current.Method = defaultSkillSyncMethodForOS(runtime.GOOS)
	}
	current.Tool = req.Tool
	current.Enabled = true
	current.VariantID = variantID
	targets[req.Tool] = current
	if err := m.db.SetSkillTargets(skill.ID, skillTargetRecordsFromMap(targets)); err != nil {
		return nil, err
	}
	return &ImportManagedSkillResult{SkillID: skill.ID, VariantID: variantID, CreatedNew: false}, nil
}

func (m *Manager) buildManagedSkillOverview(skill storage.SkillRecord, entryIndex map[string]map[string][]SkillInventoryEntry) (SkillOverviewItem, error) {
	targets, err := m.db.GetSkillTargets(skill.ID)
	if err != nil {
		return SkillOverviewItem{}, err
	}
	variants, err := m.db.ListSkillVariants(skill.ID)
	if err != nil {
		return SkillOverviewItem{}, err
	}
	defaultVariant, err := m.defaultVariantForSkillLocked(skill)
	if err != nil {
		return SkillOverviewItem{}, err
	}
	if defaultVariant != nil && !containsSkillVariant(variants, defaultVariant.ID) {
		variants = append(variants, *defaultVariant)
	}

	item := SkillOverviewItem{
		ID:          skill.ID,
		Name:        skill.Name,
		Description: skill.Description,
		Managed:     true,
		Enabled:     skill.Enabled,
		PrimaryPath: skill.SourcePath,
		Library: SkillOverviewLibrary{
			Present:   true,
			Path:      skill.SourcePath,
			Hash:      mustHashSkillPath(skill.SourcePath),
			VariantID: defaultVariant.ID,
		},
		Variants:   []SkillOverviewVariant{},
		Tools:      map[string]SkillOverviewToolState{},
		Issues:     []SkillOverviewIssue{},
		Discovered: []SkillOverviewDiscoveredInstall{},
	}

	variantsByID := map[int64]SkillOverviewVariant{}
	for _, variant := range variants {
		view := SkillOverviewVariant{
			ID:         variant.ID,
			SourcePath: variant.SourcePath,
			OriginTool: variant.OriginTool,
			Hash:       mustHashSkillPath(variant.SourcePath),
			Managed:    true,
		}
		item.Variants = append(item.Variants, view)
		variantsByID[view.ID] = view
	}
	sort.Slice(item.Variants, func(i, j int) bool { return item.Variants[i].ID < item.Variants[j].ID })

	group := entryIndex[normalizeSkillName(skill.Name)]
	for _, tool := range m.sortedAdapterTools() {
		actual := toActualInstalls(group[tool])
		state := SkillOverviewToolState{
			Enabled: false,
			Method:  "",
			Status:  "not_installed",
			Actual:  actual,
		}
		target, ok := targets[tool]
		if ok {
			state.Enabled = target.Enabled
			state.Method = target.Method
			state.SelectedVariantID = target.VariantID
		}
		selectedVariant, selectedOK := variantsByID[state.SelectedVariantID]
		if !selectedOK && defaultVariant != nil {
			selectedVariant = variantsByID[defaultVariant.ID]
			if state.SelectedVariantID == 0 {
				state.SelectedVariantID = defaultVariant.ID
			}
		}
		if selectedVariant.ID != 0 {
			state.SelectedPath = selectedVariant.SourcePath
			state.SelectedHash = selectedVariant.Hash
		}
		state.Status = evaluateToolStatus(state, selectedVariant, actual)
		item.Tools[tool] = state
		item.Discovered = append(item.Discovered, toDiscoveredInstalls(tool, group[tool])...)
		if issueCode := toolIssueCode(state.Status); issueCode != "" {
			item.Issues = append(item.Issues, SkillOverviewIssue{Tool: tool, Code: issueCode})
		}
	}

	sort.Slice(item.Discovered, func(i, j int) bool {
		if item.Discovered[i].Tool == item.Discovered[j].Tool {
			return item.Discovered[i].Path < item.Discovered[j].Path
		}
		return item.Discovered[i].Tool < item.Discovered[j].Tool
	})

	return item, nil
}

func buildUnmanagedSkillOverview(group map[string][]SkillInventoryEntry, tools []string) SkillOverviewItem {
	var sample *SkillInventoryEntry
	for _, tool := range append([]string{"global"}, tools...) {
		entries := group[tool]
		if len(entries) > 0 {
			sample = &entries[0]
			break
		}
	}
	if sample == nil {
		return SkillOverviewItem{}
	}

	item := SkillOverviewItem{
		Name:        sample.Name,
		Description: sample.Description,
		Managed:     false,
		Enabled:     true,
		PrimaryPath: sample.Path,
		Library: SkillOverviewLibrary{
			Present: len(group["global"]) > 0,
		},
		Variants:   []SkillOverviewVariant{},
		Tools:      map[string]SkillOverviewToolState{},
		Issues:     []SkillOverviewIssue{},
		Discovered: []SkillOverviewDiscoveredInstall{},
	}
	if len(group["global"]) > 0 {
		item.Library.Path = group["global"][0].Path
		item.Library.Hash = group["global"][0].Hash
		item.Variants = append(item.Variants, SkillOverviewVariant{
			ID:         0,
			SourcePath: group["global"][0].Path,
			OriginTool: "global",
			Hash:       group["global"][0].Hash,
			Managed:    false,
		})
		item.Discovered = append(item.Discovered, toDiscoveredInstalls("global", group["global"])...)
	}

	for _, tool := range tools {
		actual := toActualInstalls(group[tool])
		status := "not_installed"
		if len(actual) > 0 {
			status = "unmanaged"
			item.Issues = append(item.Issues, SkillOverviewIssue{Tool: tool, Code: "unmanaged"})
		}
		item.Tools[tool] = SkillOverviewToolState{
			Enabled: false,
			Method:  "",
			Status:  status,
			Actual:  actual,
		}
		item.Discovered = append(item.Discovered, toDiscoveredInstalls(tool, group[tool])...)
	}
	sort.Slice(item.Discovered, func(i, j int) bool {
		if item.Discovered[i].Tool == item.Discovered[j].Tool {
			return item.Discovered[i].Path < item.Discovered[j].Path
		}
		return item.Discovered[i].Tool < item.Discovered[j].Tool
	})
	return item
}

func (m *Manager) prepareManagedSkillPath(name, sourcePath, sourceHash, libraryRoot string) (string, error) {
	if pathWithin(sourcePath, libraryRoot) {
		return sourcePath, nil
	}

	baseName := sanitizeSkillDirName(name)
	if baseName == "" {
		baseName = sanitizeSkillDirName(filepath.Base(sourcePath))
	}
	if baseName == "" {
		baseName = "skill"
	}

	for attempt := 0; attempt < 1000; attempt++ {
		suffix := ""
		if attempt > 0 {
			suffix = fmt.Sprintf("-%d", attempt+1)
		}
		candidate := filepath.Join(libraryRoot, baseName+suffix)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			hash, hashErr := hashSkillDirectory(candidate)
			if hashErr == nil && hash == sourceHash {
				return candidate, nil
			}
			continue
		}
		if err := replaceWithCopy(sourcePath, candidate); err != nil {
			return "", err
		}
		return candidate, nil
	}
	return "", fmt.Errorf("unable to allocate managed path for skill %q", name)
}

func groupSkillEntries(entries []SkillInventoryEntry) map[string]map[string][]SkillInventoryEntry {
	grouped := map[string]map[string][]SkillInventoryEntry{}
	for _, entry := range entries {
		key := normalizeSkillName(entry.Name)
		tool := entry.Tool
		if entry.IsLibrary {
			tool = "global"
		}
		if grouped[key] == nil {
			grouped[key] = map[string][]SkillInventoryEntry{}
		}
		grouped[key][tool] = append(grouped[key][tool], entry)
	}
	return grouped
}

func toActualInstalls(entries []SkillInventoryEntry) []SkillOverviewActualInstall {
	actual := make([]SkillOverviewActualInstall, 0, len(entries))
	for _, entry := range entries {
		actual = append(actual, SkillOverviewActualInstall{
			Path:   entry.Path,
			Hash:   entry.Hash,
			Method: skillInstallMethod(entry),
		})
	}
	sort.Slice(actual, func(i, j int) bool { return actual[i].Path < actual[j].Path })
	return actual
}

func toDiscoveredInstalls(tool string, entries []SkillInventoryEntry) []SkillOverviewDiscoveredInstall {
	discovered := make([]SkillOverviewDiscoveredInstall, 0, len(entries))
	for _, entry := range entries {
		discovered = append(discovered, SkillOverviewDiscoveredInstall{
			Path:   entry.Path,
			Tool:   tool,
			Hash:   entry.Hash,
			Method: skillInstallMethod(entry),
		})
	}
	return discovered
}

func skillInstallMethod(entry SkillInventoryEntry) string {
	if entry.IsSymlink {
		return "symlink"
	}
	return "copy"
}

func evaluateToolStatus(state SkillOverviewToolState, selected SkillOverviewVariant, actual []SkillOverviewActualInstall) string {
	if state.Enabled {
		if selected.ID == 0 {
			return "missing_variant"
		}
		if len(actual) == 0 {
			return "missing_install"
		}
		for _, install := range actual {
			if install.Hash == selected.Hash && install.Hash != "" {
				return "using_selected"
			}
		}
		return "out_of_sync"
	}
	if len(actual) > 0 {
		return "unmanaged"
	}
	return "not_installed"
}

func toolIssueCode(status string) string {
	switch status {
	case "missing_variant", "missing_install", "out_of_sync", "unmanaged":
		return status
	default:
		return ""
	}
}

func containsSkillVariant(variants []storage.SkillVariantRecord, id int64) bool {
	for _, variant := range variants {
		if variant.ID == id {
			return true
		}
	}
	return false
}

func mustHashSkillPath(path string) string {
	hash, err := hashSkillDirectory(path)
	if err != nil {
		return ""
	}
	return hash
}
