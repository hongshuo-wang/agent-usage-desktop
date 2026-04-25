package configmanager

import (
	"os"
	"path/filepath"
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
