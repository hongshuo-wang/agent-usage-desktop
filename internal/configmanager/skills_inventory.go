package configmanager

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

const (
	SkillConflictExternalOverLibrary = "external_over_library"
	SkillConflictLibraryOverExternal = "library_over_external"
)

type SkillInventory struct {
	LibraryPath      string                `json:"library_path"`
	CLI              SkillCLIStatus        `json:"cli"`
	ToolAvailability map[string]bool       `json:"tool_availability"`
	Library          []SkillInventoryEntry `json:"library"`
	Discovered       []SkillInventoryEntry `json:"discovered"`
	Conflicts        []SkillConflict       `json:"conflicts"`
	Summary          SkillInventorySummary `json:"summary"`
}

type SkillCLIStatus struct {
	Available bool   `json:"available"`
	Command   string `json:"command"`
	Message   string `json:"message"`
}

type SkillInventorySummary struct {
	LibraryCount    int `json:"library_count"`
	DiscoveredCount int `json:"discovered_count"`
	ImportableCount int `json:"importable_count"`
	ConflictCount   int `json:"conflict_count"`
}

type SkillInventoryEntry struct {
	Name          string                     `json:"name"`
	Description   string                     `json:"description"`
	Path          string                     `json:"path"`
	Tool          string                     `json:"tool"`
	Hash          string                     `json:"hash"`
	IsLibrary     bool                       `json:"is_library"`
	IsSymlink     bool                       `json:"is_symlink"`
	Importable    bool                       `json:"importable"`
	Represented   bool                       `json:"represented"`
	Conflict      bool                       `json:"conflict"`
	InstallStatus map[string]SkillInstallRef `json:"install_status,omitempty"`
}

type SkillInstallRef struct {
	Path      string `json:"path"`
	Installed bool   `json:"installed"`
	Method    string `json:"method"`
	Hash      string `json:"hash,omitempty"`
}

type SkillConflict struct {
	Name     string              `json:"name"`
	Library  SkillInventoryEntry `json:"library"`
	External SkillInventoryEntry `json:"external"`
}

type SkillImportResult struct {
	AffectedFiles []AffectedFile  `json:"affected_files"`
	ImportedCount int             `json:"imported_count"`
	SkippedCount  int             `json:"skipped_count"`
	Conflicts     []SkillConflict `json:"conflicts"`
}

type SkillConflictResolveRequest struct {
	Name         string `json:"name"`
	Tool         string `json:"tool"`
	LibraryPath  string `json:"library_path"`
	ExternalPath string `json:"external_path"`
	Direction    string `json:"direction"`
}

type SkillConflictResolveResult struct {
	AffectedFiles []AffectedFile `json:"affected_files"`
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func (m *Manager) SkillsInventory() (*SkillInventory, error) {
	libraryPath, err := skillsLibraryPath()
	if err != nil {
		return nil, err
	}

	entries, err := m.scanSkillInventoryEntries(libraryPath)
	if err != nil {
		return nil, err
	}

	return m.classifySkillInventory(libraryPath, entries), nil
}

func (m *Manager) ImportNonConflictingSkills() (*SkillImportResult, error) {
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
	inventory := m.classifySkillInventory(libraryPath, entries)

	if err := os.MkdirAll(libraryPath, 0o755); err != nil {
		return nil, err
	}

	var affected []AffectedFile
	imported := 0
	for _, entry := range inventory.Discovered {
		if !entry.Importable {
			continue
		}
		destination := filepath.Join(libraryPath, sanitizeSkillDirName(entry.Name))
		if err := replaceWithCopy(entry.Path, destination); err != nil {
			return nil, err
		}
		metadata, err := parseSkillMetadata(destination)
		if err != nil {
			return nil, err
		}
		name := metadata.Name
		if name == "" {
			name = entry.Name
		}
		if err := m.upsertSkillRecord(name, destination, metadata.Description, entry.Tool); err != nil {
			return nil, err
		}
		affected = append(affected, AffectedFile{Path: destination, Tool: "global", Operation: "import"})
		imported++
	}

	return &SkillImportResult{
		AffectedFiles: affected,
		ImportedCount: imported,
		SkippedCount:  len(inventory.Conflicts),
		Conflicts:     inventory.Conflicts,
	}, nil
}

func (m *Manager) ResolveSkillConflict(req SkillConflictResolveRequest) (*SkillConflictResolveResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if req.Name == "" || req.Tool == "" || req.LibraryPath == "" || req.ExternalPath == "" {
		return nil, fmt.Errorf("name, tool, library_path, and external_path are required")
	}
	libraryRoot, err := skillsLibraryPath()
	if err != nil {
		return nil, err
	}
	if !pathWithin(req.LibraryPath, libraryRoot) {
		return nil, fmt.Errorf("library_path must be inside %s", libraryRoot)
	}
	if err := validateSkillSourcePath(req.ExternalPath); err != nil {
		return nil, err
	}
	if _, ok := m.adapters[req.Tool]; !ok {
		return nil, fmt.Errorf("unsupported tool: %s", req.Tool)
	}

	switch req.Direction {
	case SkillConflictExternalOverLibrary:
		if err := os.MkdirAll(filepath.Dir(req.LibraryPath), 0o755); err != nil {
			return nil, err
		}
		backupDir, err := os.MkdirTemp("", "configmanager-skill-conflict-*")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(backupDir)
		snapshots := map[string]skillPathSnapshot{}
		var order []string
		if err := captureSkillPathSnapshot(req.LibraryPath, backupDir, snapshots, &order); err != nil {
			return nil, err
		}
		if err := replaceWithCopy(req.ExternalPath, req.LibraryPath); err != nil {
			rollbackErr := rollbackSkillPathSnapshots(order, snapshots)
			return nil, combineErrors(err, rollbackErr)
		}
		metadata, err := parseSkillMetadata(req.LibraryPath)
		if err != nil {
			return nil, err
		}
		description := metadata.Description
		if description == "" {
			description = req.Name
		}
		if err := m.upsertSkillRecord(req.Name, req.LibraryPath, description, req.Tool); err != nil {
			rollbackErr := rollbackSkillPathSnapshots(order, snapshots)
			return nil, combineErrors(err, rollbackErr)
		}
		return &SkillConflictResolveResult{AffectedFiles: []AffectedFile{{Path: req.LibraryPath, Tool: "global", Operation: "replace"}}}, nil
	case SkillConflictLibraryOverExternal:
		if err := validateSkillSourcePath(req.LibraryPath); err != nil {
			return nil, err
		}
		if err := m.upsertSkillRecord(req.Name, req.LibraryPath, "", req.Tool); err != nil {
			return nil, err
		}
		return &SkillConflictResolveResult{AffectedFiles: []AffectedFile{{Path: req.ExternalPath, Tool: req.Tool, Operation: "prefer_global"}}}, nil
	default:
		return nil, fmt.Errorf("unsupported conflict direction: %s", req.Direction)
	}
}

func (m *Manager) scanSkillInventoryEntries(libraryPath string) ([]SkillInventoryEntry, error) {
	var entries []SkillInventoryEntry
	libraryEntries, err := scanSkillRoot(libraryPath, "global", true)
	if err != nil {
		return nil, err
	}
	entries = append(entries, libraryEntries...)

	for _, tool := range m.sortedAdapterTools() {
		adapter := m.adapters[tool]
		for _, root := range adapter.GetSkillPaths() {
			if root == "" || pathSameOrWithin(root, libraryPath) {
				continue
			}
			toolEntries, err := scanSkillRoot(root, tool, false)
			if err != nil {
				return nil, err
			}
			entries = append(entries, toolEntries...)
		}
	}
	return entries, nil
}

func (m *Manager) classifySkillInventory(libraryPath string, entries []SkillInventoryEntry) *SkillInventory {
	byName := map[string][]SkillInventoryEntry{}
	for _, entry := range entries {
		byName[normalizeSkillName(entry.Name)] = append(byName[normalizeSkillName(entry.Name)], entry)
	}

	inventory := &SkillInventory{
		LibraryPath:      libraryPath,
		CLI:              detectSkillsCLI(),
		ToolAvailability: map[string]bool{},
		Library:          []SkillInventoryEntry{},
		Discovered:       []SkillInventoryEntry{},
		Conflicts:        []SkillConflict{},
	}
	for _, tool := range m.sortedAdapterTools() {
		if adapter, ok := m.adapters[tool]; ok && adapter != nil {
			inventory.ToolAvailability[tool] = toolCLIAvailable(tool)
		}
	}

	for _, group := range byName {
		var libraries []SkillInventoryEntry
		var external []SkillInventoryEntry
		for _, entry := range group {
			if entry.IsLibrary {
				libraries = append(libraries, entry)
			} else {
				external = append(external, entry)
			}
		}

		for _, library := range libraries {
			library.InstallStatus = m.installStatusForSkill(library)
			inventory.Library = append(inventory.Library, library)
		}

		var library *SkillInventoryEntry
		if len(libraries) > 0 {
			library = &libraries[0]
		}
		for _, entry := range external {
			if library == nil {
				entry.Importable = true
			} else if entry.Hash == library.Hash {
				entry.Represented = true
			} else {
				entry.Conflict = true
				inventory.Conflicts = append(inventory.Conflicts, SkillConflict{Name: library.Name, Library: *library, External: entry})
			}
			inventory.Discovered = append(inventory.Discovered, entry)
		}
	}

	sort.Slice(inventory.Library, func(i, j int) bool { return inventory.Library[i].Name < inventory.Library[j].Name })
	sort.Slice(inventory.Discovered, func(i, j int) bool {
		if inventory.Discovered[i].Name == inventory.Discovered[j].Name {
			return inventory.Discovered[i].Tool < inventory.Discovered[j].Tool
		}
		return inventory.Discovered[i].Name < inventory.Discovered[j].Name
	})
	sort.Slice(inventory.Conflicts, func(i, j int) bool {
		if inventory.Conflicts[i].Name == inventory.Conflicts[j].Name {
			return inventory.Conflicts[i].External.Tool < inventory.Conflicts[j].External.Tool
		}
		return inventory.Conflicts[i].Name < inventory.Conflicts[j].Name
	})

	inventory.Summary.LibraryCount = len(inventory.Library)
	inventory.Summary.DiscoveredCount = len(inventory.Discovered)
	inventory.Summary.ConflictCount = len(inventory.Conflicts)
	for _, entry := range inventory.Discovered {
		if entry.Importable {
			inventory.Summary.ImportableCount++
		}
	}

	return inventory
}

func (m *Manager) installStatusForSkill(skill SkillInventoryEntry) map[string]SkillInstallRef {
	status := map[string]SkillInstallRef{}
	for _, tool := range m.sortedAdapterTools() {
		adapter := m.adapters[tool]
		for _, root := range adapter.GetSkillPaths() {
			path := filepath.Join(root, filepath.Base(skill.Path))
			ref := SkillInstallRef{Path: path}
			if method, hash, installed := inspectSkillInstall(path, skill); installed {
				ref.Installed = true
				ref.Method = method
				ref.Hash = hash
			}
			status[tool] = ref
			break
		}
	}
	return status
}

func (m *Manager) upsertSkillRecord(name, sourcePath, description, tool string) error {
	if description == "" {
		metadata, err := parseSkillMetadata(sourcePath)
		if err == nil {
			description = metadata.Description
		}
	}
	skills, err := m.db.ListSkills()
	if err != nil {
		return err
	}
	for _, skill := range skills {
		if normalizeSkillName(skill.Name) != normalizeSkillName(name) {
			continue
		}
		variantID, err := m.ensureSkillVariantLocked(skill.ID, sourcePath, tool)
		if err != nil {
			return err
		}
		targets, err := m.db.GetSkillTargets(skill.ID)
		if err != nil {
			return err
		}
		targets[tool] = storage.SkillTargetRecord{Tool: tool, Method: "symlink", Enabled: true, VariantID: variantID}
		return m.db.UpdateSkillWithTargets(skill.ID, name, sourcePath, description, true, skillTargetRecordsFromMap(targets))
	}

	id, err := m.db.CreateSkill(name, sourcePath, description)
	if err != nil {
		return err
	}
	variantID, err := m.ensureSkillVariantLocked(id, sourcePath, tool)
	if err != nil {
		_ = m.db.DeleteSkill(id)
		return err
	}
	return m.setSkillTargetsFn(id, []storage.SkillTargetRecord{{Tool: tool, Method: "symlink", Enabled: true, VariantID: variantID}})
}

type skillMetadata struct {
	Name        string
	Description string
}

func scanSkillRoot(root, tool string, library bool) ([]SkillInventoryEntry, error) {
	children, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	entries := make([]SkillInventoryEntry, 0, len(children))
	for _, child := range children {
		if strings.HasPrefix(child.Name(), ".") {
			continue
		}
		path := filepath.Join(root, child.Name())
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		metadata, err := parseSkillMetadata(path)
		if err != nil {
			return nil, err
		}
		name := metadata.Name
		if name == "" {
			name = child.Name()
		}
		hash, err := hashSkillDirectory(path)
		if err != nil {
			return nil, err
		}
		lstat, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		entries = append(entries, SkillInventoryEntry{
			Name:        name,
			Description: metadata.Description,
			Path:        path,
			Tool:        tool,
			Hash:        hash,
			IsLibrary:   library,
			IsSymlink:   lstat.Mode()&os.ModeSymlink != 0,
		})
	}
	return entries, nil
}

func parseSkillMetadata(skillPath string) (skillMetadata, error) {
	content, err := os.ReadFile(filepath.Join(skillPath, "SKILL.md"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return skillMetadata{}, nil
		}
		return skillMetadata{}, err
	}
	trimmed := bytes.TrimSpace(content)
	if !bytes.HasPrefix(trimmed, []byte("---")) {
		return skillMetadata{}, nil
	}
	lines := strings.Split(string(trimmed), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "---" {
		return skillMetadata{}, nil
	}
	metadata := skillMetadata{}
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "---" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.TrimSpace(key) {
		case "name":
			metadata.Name = value
		case "description":
			metadata.Description = value
		}
	}
	return metadata, nil
}

func hashSkillDirectory(root string) (string, error) {
	type fileHashInput struct {
		path string
		mode string
		data []byte
	}
	var files []fileHashInput
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			files = append(files, fileHashInput{path: relative, mode: "symlink", data: []byte(target)})
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			files = append(files, fileHashInput{path: relative + "/", mode: "dir"})
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, fileHashInput{path: relative, mode: info.Mode().Perm().String(), data: data})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	hash := sha256.New()
	for _, file := range files {
		_, _ = io.WriteString(hash, file.path)
		_, _ = io.WriteString(hash, "\x00")
		_, _ = io.WriteString(hash, file.mode)
		_, _ = io.WriteString(hash, "\x00")
		_, _ = hash.Write(file.data)
		_, _ = io.WriteString(hash, "\x00")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func inspectSkillInstall(path string, library SkillInventoryEntry) (string, string, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", "", false
	}
	method := "copy"
	if info.Mode()&os.ModeSymlink != 0 {
		method = "symlink"
		if target, err := os.Readlink(path); err == nil {
			absTarget := target
			if !filepath.IsAbs(absTarget) {
				absTarget = filepath.Join(filepath.Dir(path), target)
			}
			if same, err := skillPathsMatch(absTarget, library.Path); err == nil && same {
				return method, library.Hash, true
			}
		}
	}
	stat, err := os.Stat(path)
	if err != nil || !stat.IsDir() {
		return method, "", false
	}
	hash, err := hashSkillDirectory(path)
	if err != nil {
		return method, "", false
	}
	return method, hash, hash == library.Hash
}

func skillsLibraryPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".agent-usage", "skills"), nil
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = stripANSI(strings.TrimSpace(line))
		if line != "" {
			return line
		}
	}
	return "npx skills available"
}

func stripANSI(text string) string {
	return ansiEscapePattern.ReplaceAllString(text, "")
}

func normalizeSkillName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func sanitizeSkillDirName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "skill"
	}
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' {
			return '-'
		}
		return r
	}, name)
	return name
}

func pathWithin(path, root string) bool {
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	pathAbs = filepath.Clean(pathAbs)
	rootAbs = filepath.Clean(rootAbs)
	return pathAbs == rootAbs || strings.HasPrefix(pathAbs, rootAbs+string(os.PathSeparator))
}

func pathSameOrWithin(path, root string) bool {
	return pathWithin(path, root) || pathWithin(root, path)
}
