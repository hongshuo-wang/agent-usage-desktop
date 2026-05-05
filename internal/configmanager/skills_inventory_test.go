package configmanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSkillMetadata(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "demo", "Demo skill", "body")

	metadata, err := parseSkillMetadata(dir)
	if err != nil {
		t.Fatalf("parseSkillMetadata() error = %v", err)
	}
	if metadata.Name != "demo" || metadata.Description != "Demo skill" {
		t.Fatalf("metadata = %+v", metadata)
	}
}

func TestHashSkillDirectoryChangesWhenContentsChange(t *testing.T) {
	dir := t.TempDir()
	writeSkillFile(t, dir, "demo", "Demo skill", "one")

	first, err := hashSkillDirectory(dir)
	if err != nil {
		t.Fatalf("hashSkillDirectory first: %v", err)
	}
	writeSkillFile(t, dir, "demo", "Demo skill", "two")
	second, err := hashSkillDirectory(dir)
	if err != nil {
		t.Fatalf("hashSkillDirectory second: %v", err)
	}
	if first == second {
		t.Fatalf("hash did not change after content update")
	}
}

func TestHashSkillDirectoryFollowsSkillSymlink(t *testing.T) {
	target := filepath.Join(t.TempDir(), "planner-target")
	writeSkillFile(t, target, "planner", "Planner skill", "body")

	linkPath := filepath.Join(t.TempDir(), "planner-link")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Skipf("Symlink() unsupported: %v", err)
	}

	valid, reason, err := validateSkillDirectory(linkPath)
	if err != nil {
		t.Fatalf("validateSkillDirectory() error = %v", err)
	}
	if !valid || reason != "" {
		t.Fatalf("validateSkillDirectory() = (%v, %q), want (true, empty)", valid, reason)
	}

	targetHash, err := hashSkillDirectory(target)
	if err != nil {
		t.Fatalf("hashSkillDirectory(target) error = %v", err)
	}
	linkHash, err := hashSkillDirectory(linkPath)
	if err != nil {
		t.Fatalf("hashSkillDirectory(link) error = %v", err)
	}
	if targetHash != linkHash {
		t.Fatalf("hash mismatch through symlink: target=%q link=%q", targetHash, linkHash)
	}
}

func TestFirstNonEmptyLineStripsANSI(t *testing.T) {
	text := "\n\x1b[1mUsage:\x1b[0m skills <command> [options]\n"
	got := firstNonEmptyLine(text)
	want := "Usage: skills <command> [options]"
	if got != want {
		t.Fatalf("firstNonEmptyLine() = %q, want %q", got, want)
	}
}

func TestSkillsInventoryClassifiesImportableAndConflict(t *testing.T) {
	db := openManagerTestDB(t)
	libraryRoot := filepath.Join(t.TempDir(), "library")
	toolRoot := filepath.Join(t.TempDir(), "tool-skills")
	writeSkillFile(t, filepath.Join(libraryRoot, "shared"), "shared", "Library", "library")
	writeSkillFile(t, filepath.Join(toolRoot, "shared"), "shared", "External", "external")
	writeSkillFile(t, filepath.Join(toolRoot, "new"), "new", "New", "new")

	mgr := NewManager(db, filepath.Join(t.TempDir(), "backups"), WithAdapter(&fakeManagerAdapter{tool: "codex", installed: true, skillPaths: []string{toolRoot}}), WithEncryptionKey(make([]byte, 32)))
	entries, err := mgr.scanSkillInventoryEntries(libraryRoot)
	if err != nil {
		t.Fatalf("scanSkillInventoryEntries() error = %v", err)
	}
	inventory := mgr.classifySkillInventory(libraryRoot, entries)

	if inventory.Summary.ImportableCount != 1 {
		t.Fatalf("ImportableCount = %d, want 1", inventory.Summary.ImportableCount)
	}
	if inventory.Summary.ConflictCount != 1 {
		t.Fatalf("ConflictCount = %d, want 1", inventory.Summary.ConflictCount)
	}
}

func TestScanSkillInventoryEntriesTreatsContainerDirsAsNamespaces(t *testing.T) {
	db := openManagerTestDB(t)
	libraryRoot := filepath.Join(t.TempDir(), "library")
	toolRoot := filepath.Join(t.TempDir(), "tool-skills")
	writeSkillFile(t, filepath.Join(toolRoot, "superpowers", "planner"), "planner", "Planner", "planner")
	writeSkillFile(t, filepath.Join(toolRoot, "superpowers", "reviewer"), "reviewer", "Reviewer", "reviewer")

	mgr := NewManager(db, filepath.Join(t.TempDir(), "backups"), WithAdapter(&fakeManagerAdapter{tool: "codex", installed: true, skillPaths: []string{toolRoot}}), WithEncryptionKey(make([]byte, 32)))
	entries, err := mgr.scanSkillInventoryEntries(libraryRoot)
	if err != nil {
		t.Fatalf("scanSkillInventoryEntries() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}

	names := []string{entries[0].Name, entries[1].Name}
	if strings.Join(names, ",") != "planner,reviewer" {
		t.Fatalf("entry names = %v, want [planner reviewer]", names)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Path, string(filepath.Separator)+"superpowers"+string(filepath.Separator)) && filepath.Base(entry.Path) == "superpowers" {
			t.Fatalf("container directory surfaced as skill entry: %+v", entry)
		}
		if !entry.Valid {
			t.Fatalf("nested skill marked invalid: %+v", entry)
		}
	}
}

func TestScanSkillInventoryEntriesIgnoresEmptyContainerDirectory(t *testing.T) {
	db := openManagerTestDB(t)
	libraryRoot := filepath.Join(t.TempDir(), "library")
	toolRoot := filepath.Join(t.TempDir(), "tool-skills")
	emptyContainer := filepath.Join(toolRoot, "superpowers")
	if err := os.MkdirAll(emptyContainer, 0o755); err != nil {
		t.Fatalf("MkdirAll emptyContainer: %v", err)
	}

	mgr := NewManager(db, filepath.Join(t.TempDir(), "backups"), WithAdapter(&fakeManagerAdapter{tool: "codex", installed: true, skillPaths: []string{toolRoot}}), WithEncryptionKey(make([]byte, 32)))
	entries, err := mgr.scanSkillInventoryEntries(libraryRoot)
	if err != nil {
		t.Fatalf("scanSkillInventoryEntries() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("len(entries) = %d, want 0", len(entries))
	}
}

func TestScanSkillInventoryEntriesHidesSystemSkillNamespace(t *testing.T) {
	db := openManagerTestDB(t)
	libraryRoot := filepath.Join(t.TempDir(), "library")
	toolRoot := filepath.Join(t.TempDir(), "tool-skills")
	writeSkillFile(t, filepath.Join(toolRoot, ".system", "planner"), "planner", "Planner", "planner")

	mgr := NewManager(db, filepath.Join(t.TempDir(), "backups"), WithAdapter(&fakeManagerAdapter{tool: "codex", installed: true, skillPaths: []string{toolRoot}}), WithEncryptionKey(make([]byte, 32)))
	entries, err := mgr.scanSkillInventoryEntries(libraryRoot)
	if err != nil {
		t.Fatalf("scanSkillInventoryEntries() error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("len(entries) = %d, want 0", len(entries))
	}
}

func TestResolveSkillConflictExternalOverLibrary(t *testing.T) {
	db := openManagerTestDB(t)
	libraryRoot := filepath.Join(t.TempDir(), ".agent-usage", "skills")
	toolRoot := filepath.Join(t.TempDir(), "tool-skills")
	libraryPath := filepath.Join(libraryRoot, "shared")
	externalPath := filepath.Join(toolRoot, "shared")
	writeSkillFile(t, libraryPath, "shared", "Library", "library")
	writeSkillFile(t, externalPath, "shared", "External", "external")

	oldHome := os.Getenv("HOME")
	t.Setenv("HOME", filepath.Dir(filepath.Dir(libraryRoot)))
	t.Cleanup(func() { _ = os.Setenv("HOME", oldHome) })
	mgr := NewManager(db, filepath.Join(t.TempDir(), "backups"), WithAdapter(&fakeManagerAdapter{tool: "codex", installed: true, skillPaths: []string{toolRoot}}), WithEncryptionKey(make([]byte, 32)))

	result, err := mgr.ResolveSkillConflict(SkillConflictResolveRequest{Name: "shared", Tool: "codex", LibraryPath: libraryPath, ExternalPath: externalPath, Direction: SkillConflictExternalOverLibrary})
	if err != nil {
		t.Fatalf("ResolveSkillConflict() error = %v", err)
	}
	if len(result.AffectedFiles) != 1 {
		t.Fatalf("affected files = %d, want 1", len(result.AffectedFiles))
	}
	content, err := os.ReadFile(filepath.Join(libraryPath, "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !containsString(string(content), "external") {
		t.Fatalf("library was not replaced with external content: %s", string(content))
	}
	tools, err := db.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills() error = %v", err)
	}
	if len(tools) != 1 || tools[0].SourcePath != libraryPath {
		t.Fatalf("skills = %+v", tools)
	}
}

func writeSkillFile(t *testing.T, dir, name, description, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func containsString(value, needle string) bool {
	return len(needle) == 0 || filepath.Base(value) == needle || len(value) >= len(needle) && (value == needle || containsString(value[1:], needle) || value[:len(needle)] == needle)
}
