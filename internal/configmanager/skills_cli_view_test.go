package configmanager

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportManagedSkillKeepsLocalBindingByDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "codex-skills")
	localPath := filepath.Join(installRoot, "planner")
	if err := os.MkdirAll(localPath, 0o755); err != nil {
		t.Fatalf("MkdirAll localPath: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localPath, "SKILL.md"), []byte("local-v1"), 0o644); err != nil {
		t.Fatalf("WriteFile SKILL.md: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{
			tool:       "codex",
			installed:  true,
			skillPaths: []string{installRoot},
		}),
		WithEncryptionKey(make([]byte, 32)),
	)

	result, err := mgr.ImportManagedSkill(ImportManagedSkillRequest{
		Name:       "planner",
		Tool:       "codex",
		SourcePath: localPath,
	})
	if err != nil {
		t.Fatalf("ImportManagedSkill: %v", err)
	}

	targets, err := db.GetSkillTargets(result.SkillID)
	if err != nil {
		t.Fatalf("GetSkillTargets: %v", err)
	}
	target := targets["codex"]
	if target.SourceKind != "local" {
		t.Fatalf("target.SourceKind = %q, want local", target.SourceKind)
	}
	if target.LocalSourcePath != localPath {
		t.Fatalf("target.LocalSourcePath = %q, want %q", target.LocalSourcePath, localPath)
	}

	overview, err := mgr.SkillsCLIOverview("codex")
	if err != nil {
		t.Fatalf("SkillsCLIOverview: %v", err)
	}
	if len(overview.Skills) != 1 {
		t.Fatalf("len(overview.Skills) = %d, want 1", len(overview.Skills))
	}
	row := overview.Skills[0]
	if row.Binding.SourceKind != "local" {
		t.Fatalf("overview binding source_kind = %q, want local", row.Binding.SourceKind)
	}
	if row.SourceKind != "local" {
		t.Fatalf("overview source_kind = %q, want local", row.SourceKind)
	}
	if row.Status != "ok" || !row.IsValid {
		t.Fatalf("overview status = %q is_valid = %v, want ok/true", row.Status, row.IsValid)
	}
	if row.SyncState != skillsCLISyncStateInSync {
		t.Fatalf("sync_state = %q, want in_sync", row.SyncState)
	}
	if row.PrimaryAction != skillsCLIPrimaryActionNone {
		t.Fatalf("primary_action = %q, want none", row.PrimaryAction)
	}
}

func TestSetSkillCurrentVariantUpdatesGlobalBindings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "codex-skills")
	globalV1 := filepath.Join(t.TempDir(), "planner-v1")
	globalV2 := filepath.Join(t.TempDir(), "planner-v2")
	for path, content := range map[string]string{
		globalV1: "version-one",
		globalV2: "version-two",
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{
			tool:       "codex",
			installed:  true,
			skillPaths: []string{installRoot},
		}),
		WithEncryptionKey(make([]byte, 32)),
	)

	skillID, err := mgr.CreateSkill("planner", globalV1, "Plan helper", map[string]SkillTargetRecord{
		"codex": {Tool: "codex", Method: "copy", Enabled: true, SourceKind: "global"},
	})
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}
	if _, err := mgr.SyncSkills(); err != nil {
		t.Fatalf("SyncSkills initial: %v", err)
	}

	installedFile := filepath.Join(installRoot, "planner", "SKILL.md")
	data, err := os.ReadFile(installedFile)
	if err != nil {
		t.Fatalf("ReadFile initial: %v", err)
	}
	if string(data) != "version-one" {
		t.Fatalf("initial content = %q, want version-one", string(data))
	}

	variantID, err := db.CreateSkillVariant(skillID, globalV2, "global")
	if err != nil {
		t.Fatalf("CreateSkillVariant: %v", err)
	}
	if err := mgr.SetSkillCurrentVariant(skillID, variantID); err != nil {
		t.Fatalf("SetSkillCurrentVariant: %v", err)
	}
	if _, err := mgr.SyncSkills(); err != nil {
		t.Fatalf("SyncSkills updated: %v", err)
	}

	data, err = os.ReadFile(installedFile)
	if err != nil {
		t.Fatalf("ReadFile updated: %v", err)
	}
	if string(data) != "version-two" {
		t.Fatalf("updated content = %q, want version-two", string(data))
	}

	targets, err := db.GetSkillTargets(skillID)
	if err != nil {
		t.Fatalf("GetSkillTargets: %v", err)
	}
	if targets["codex"].VariantID != variantID {
		t.Fatalf("targets[\"codex\"].VariantID = %d, want %d", targets["codex"].VariantID, variantID)
	}
}

func TestSkillsCLIOverviewIgnoresPlainDirectoryWithoutSkillDefinition(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "codex-skills")
	invalidPath := filepath.Join(installRoot, "broken-skill")
	if err := os.MkdirAll(invalidPath, 0o755); err != nil {
		t.Fatalf("MkdirAll invalidPath: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{
			tool:       "codex",
			installed:  true,
			skillPaths: []string{installRoot},
		}),
		WithEncryptionKey(make([]byte, 32)),
	)

	overview, err := mgr.SkillsCLIOverview("codex")
	if err != nil {
		t.Fatalf("SkillsCLIOverview: %v", err)
	}
	if len(overview.Skills) != 0 {
		t.Fatalf("len(overview.Skills) = %d, want 0", len(overview.Skills))
	}
}

func TestSkillsCLIOverviewTreatsInstallMismatchAsOverrideOpportunity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "codex-skills")
	actualPath := filepath.Join(installRoot, "planner")
	globalPath := filepath.Join(t.TempDir(), "planner-global")

	for path, content := range map[string]string{
		actualPath: "installed-local",
		globalPath: "managed-global",
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", path, err)
		}
		if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{
			tool:       "codex",
			installed:  true,
			skillPaths: []string{installRoot},
		}),
		WithEncryptionKey(make([]byte, 32)),
	)

	if _, err := mgr.CreateSkill("planner", globalPath, "Plan helper", map[string]SkillTargetRecord{
		"codex": {Tool: "codex", Method: "copy", Enabled: true, SourceKind: "global"},
	}); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	overview, err := mgr.SkillsCLIOverview("codex")
	if err != nil {
		t.Fatalf("SkillsCLIOverview: %v", err)
	}
	if overview.Summary.IssueCount != 0 {
		t.Fatalf("summary.issue_count = %d, want 0", overview.Summary.IssueCount)
	}
	if len(overview.Skills) != 1 {
		t.Fatalf("len(overview.Skills) = %d, want 1", len(overview.Skills))
	}

	row := overview.Skills[0]
	if row.SourceKind != "global" {
		t.Fatalf("source_kind = %q, want global", row.SourceKind)
	}
	if row.Status != "ok" {
		t.Fatalf("status = %q, want ok", row.Status)
	}
	if !row.IsValid {
		t.Fatalf("is_valid = false, want true")
	}
	if row.DefinitionStatus != skillDefinitionStatusValid {
		t.Fatalf("definition_status = %q, want valid", row.DefinitionStatus)
	}
	if row.SyncState != skillsCLISyncStateManagedUsingLocal {
		t.Fatalf("sync_state = %q, want managed_using_local", row.SyncState)
	}
	if row.PrimaryAction != skillsCLIPrimaryActionOverrideGlobal {
		t.Fatalf("primary_action = %q, want override_global", row.PrimaryAction)
	}
	if row.ActionSourcePath != actualPath {
		t.Fatalf("action_source_path = %q, want %q", row.ActionSourcePath, actualPath)
	}
	if len(row.Issues) != 0 {
		t.Fatalf("issues = %+v, want none", row.Issues)
	}
}

func TestSkillsCLIOverviewKeepsManagedRowWhenOnlyScannedInstallExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "codex-skills")
	globalPath := filepath.Join(t.TempDir(), "planner-global")
	actualPath := filepath.Join(installRoot, "planner")

	writeSkillFile(t, globalPath, "planner", "Plan helper", "managed-global")
	writeSkillFile(t, actualPath, "planner", "Plan helper", "installed-local")

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{
			tool:       "codex",
			installed:  true,
			skillPaths: []string{installRoot},
		}),
		WithEncryptionKey(make([]byte, 32)),
	)

	if _, err := mgr.CreateSkill("planner", globalPath, "Plan helper", nil); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	overview, err := mgr.SkillsCLIOverview("codex")
	if err != nil {
		t.Fatalf("SkillsCLIOverview: %v", err)
	}
	if len(overview.Skills) != 1 {
		t.Fatalf("len(overview.Skills) = %d, want 1", len(overview.Skills))
	}
	if !overview.Skills[0].Managed {
		t.Fatalf("row.Managed = false, want true")
	}
	if overview.Skills[0].Name != "planner" {
		t.Fatalf("row.Name = %q, want planner", overview.Skills[0].Name)
	}
}

func TestSkillsCLIOverviewAcceptsSymlinkLocalInstall(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "codex-skills")
	globalPath := filepath.Join(t.TempDir(), "planner-global")
	localLink := filepath.Join(installRoot, "planner")

	writeSkillFile(t, globalPath, "planner", "Plan helper", "global-version")
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll installRoot: %v", err)
	}
	if err := os.Symlink(globalPath, localLink); err != nil {
		t.Skipf("Symlink() unsupported: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{
			tool:       "codex",
			installed:  true,
			skillPaths: []string{installRoot},
		}),
		WithEncryptionKey(make([]byte, 32)),
	)

	if _, err := mgr.CreateSkill("planner", globalPath, "Plan helper", map[string]SkillTargetRecord{
		"codex": {Tool: "codex", Method: "symlink", Enabled: true, SourceKind: "global"},
	}); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	overview, err := mgr.SkillsCLIOverview("codex")
	if err != nil {
		t.Fatalf("SkillsCLIOverview: %v", err)
	}
	if len(overview.Skills) != 1 {
		t.Fatalf("len(overview.Skills) = %d, want 1", len(overview.Skills))
	}
	row := overview.Skills[0]
	if row.Status != "ok" || !row.IsValid {
		t.Fatalf("status/is_valid = %q/%v, want ok/true", row.Status, row.IsValid)
	}
	if row.SourceKind != "global" {
		t.Fatalf("source_kind = %q, want global", row.SourceKind)
	}
	if row.SyncState != skillsCLISyncStateInSync {
		t.Fatalf("sync_state = %q, want in_sync", row.SyncState)
	}
	if row.Global.CurrentHash == "" || row.Local.Hash == "" {
		t.Fatalf("global/local hashes should be populated: global=%+v local=%+v", row.Global, row.Local)
	}
	if row.Global.CurrentHash != row.Local.Hash {
		t.Fatalf("hash mismatch: global=%q local=%q", row.Global.CurrentHash, row.Local.Hash)
	}
}

func TestSkillsCLIOverviewSkipsContainerDirectoryRows(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "codex-skills")
	writeSkillFile(t, filepath.Join(installRoot, "superpowers", "planner"), "planner", "Planner", "planner")
	writeSkillFile(t, filepath.Join(installRoot, "superpowers", "reviewer"), "reviewer", "Reviewer", "reviewer")

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{
			tool:       "codex",
			installed:  true,
			skillPaths: []string{installRoot},
		}),
		WithEncryptionKey(make([]byte, 32)),
	)

	overview, err := mgr.SkillsCLIOverview("codex")
	if err != nil {
		t.Fatalf("SkillsCLIOverview: %v", err)
	}
	if len(overview.Skills) != 2 {
		t.Fatalf("len(overview.Skills) = %d, want 2", len(overview.Skills))
	}
	names := []string{overview.Skills[0].Name, overview.Skills[1].Name}
	if strings.Join(names, ",") != "planner,reviewer" {
		t.Fatalf("skill names = %v, want [planner reviewer]", names)
	}
	for _, row := range overview.Skills {
		if row.Name == "superpowers" {
			t.Fatalf("container row should not be visible: %+v", row)
		}
		if row.Status != skillsCLIStatusOK {
			t.Fatalf("row.Status = %q, want ok for sync opportunity", row.Status)
		}
		if row.SyncState != skillsCLISyncStateCanImportToGlobal {
			t.Fatalf("row.SyncState = %q, want can_import_to_global", row.SyncState)
		}
		if row.PrimaryAction != skillsCLIPrimaryActionImportToGlobal {
			t.Fatalf("row.PrimaryAction = %q, want import_to_global", row.PrimaryAction)
		}
	}
}

func TestSkillsCLIOverviewTreatsMissingManagedInstallAsSyncOpportunity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "codex-skills")
	globalPath := filepath.Join(t.TempDir(), "planner-global")
	writeSkillFile(t, globalPath, "planner", "Plan helper", "global-version")

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{
			tool:       "codex",
			installed:  true,
			skillPaths: []string{installRoot},
		}),
		WithEncryptionKey(make([]byte, 32)),
	)

	if _, err := mgr.CreateSkill("planner", globalPath, "Plan helper", map[string]SkillTargetRecord{
		"codex": {Tool: "codex", Method: "copy", Enabled: true, SourceKind: "global"},
	}); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	overview, err := mgr.SkillsCLIOverview("codex")
	if err != nil {
		t.Fatalf("SkillsCLIOverview: %v", err)
	}
	if overview.Summary.IssueCount != 0 {
		t.Fatalf("summary.issue_count = %d, want 0", overview.Summary.IssueCount)
	}
	row := overview.Skills[0]
	if row.Status != "ok" {
		t.Fatalf("status = %q, want ok", row.Status)
	}
	if row.SyncState != skillsCLISyncStateCanSyncToCLI {
		t.Fatalf("sync_state = %q, want can_sync_to_cli", row.SyncState)
	}
	if row.PrimaryAction != skillsCLIPrimaryActionSyncToCLI {
		t.Fatalf("primary_action = %q, want sync_to_cli", row.PrimaryAction)
	}
	if len(row.Issues) != 0 {
		t.Fatalf("issues = %+v, want none", row.Issues)
	}
}

func TestSkillsCLIOverviewKeepsValidInstallWhenInvalidDirectoryAlsoExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "codex-skills")
	globalPath := filepath.Join(t.TempDir(), "planner-global")
	invalidInstall := filepath.Join(installRoot, "planner")
	validInstall := filepath.Join(installRoot, "superpowers", "planner")

	writeSkillFile(t, globalPath, "planner", "Plan helper", "global-version")
	writeSkillFile(t, validInstall, "planner", "Plan helper", "global-version")
	if err := os.MkdirAll(filepath.Join(invalidInstall, "SKILL.md"), 0o755); err != nil {
		t.Fatalf("MkdirAll invalidInstall: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{
			tool:       "codex",
			installed:  true,
			skillPaths: []string{installRoot},
		}),
		WithEncryptionKey(make([]byte, 32)),
	)

	if _, err := mgr.CreateSkill("planner", globalPath, "Plan helper", map[string]SkillTargetRecord{
		"codex": {Tool: "codex", Method: "copy", Enabled: true, SourceKind: "global"},
	}); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	overview, err := mgr.SkillsCLIOverview("codex")
	if err != nil {
		t.Fatalf("SkillsCLIOverview: %v", err)
	}
	if len(overview.Skills) != 1 {
		t.Fatalf("len(overview.Skills) = %d, want 1", len(overview.Skills))
	}
	row := overview.Skills[0]
	if row.SyncState != skillsCLISyncStateDefinitionError {
		t.Fatalf("sync_state = %q, want definition_error", row.SyncState)
	}
	if row.Status != "problem" {
		t.Fatalf("status = %q, want problem", row.Status)
	}
	if len(row.Issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1", len(row.Issues))
	}
	if row.Issues[0].Code != skillsCLIInstallStatusInvalid || row.Issues[0].Path != invalidInstall {
		t.Fatalf("issues[0] = %+v, want invalid install at %q", row.Issues[0], invalidInstall)
	}
}

func TestSkillsCLIOverviewIgnoresLegacyContainerManagedRecord(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "codex-skills")
	containerPath := filepath.Join(t.TempDir(), "library", "superpowers")
	writeSkillFile(t, filepath.Join(containerPath, "planner"), "planner", "Planner", "planner")

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{
			tool:       "codex",
			installed:  true,
			skillPaths: []string{installRoot},
		}),
		WithEncryptionKey(make([]byte, 32)),
	)

	if _, err := db.CreateSkill("superpowers", containerPath, "Legacy container"); err != nil {
		t.Fatalf("CreateSkill legacy: %v", err)
	}

	overview, err := mgr.SkillsCLIOverview("codex")
	if err != nil {
		t.Fatalf("SkillsCLIOverview: %v", err)
	}
	if len(overview.Skills) != 0 {
		t.Fatalf("len(overview.Skills) = %d, want 0", len(overview.Skills))
	}
	if overview.Summary.IssueCount != 0 {
		t.Fatalf("summary.issue_count = %d, want 0", overview.Summary.IssueCount)
	}
}

func TestSkillsCLIOverviewHidesSystemSkills(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "codex-skills")
	writeSkillFile(t, filepath.Join(installRoot, ".system", "planner"), "planner", "Planner", "planner")

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{
			tool:       "codex",
			installed:  true,
			skillPaths: []string{installRoot},
		}),
		WithEncryptionKey(make([]byte, 32)),
	)

	overview, err := mgr.SkillsCLIOverview("codex")
	if err != nil {
		t.Fatalf("SkillsCLIOverview: %v", err)
	}
	if len(overview.Skills) != 0 {
		t.Fatalf("len(overview.Skills) = %d, want 0", len(overview.Skills))
	}
	if overview.Summary.VisibleSkills != 0 || overview.Summary.IssueCount != 0 {
		t.Fatalf("summary = %+v, want zero visible skills and issues", overview.Summary)
	}
}

func TestSkillsCLIOverviewPrioritizesCurrentCLILocalVersionWithoutCrossToolWarnings(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db := openManagerTestDB(t)
	codexRoot := filepath.Join(t.TempDir(), "codex-skills")
	claudeRoot := filepath.Join(t.TempDir(), "claude-skills")
	codexPath := filepath.Join(codexRoot, "planner")
	claudePath := filepath.Join(claudeRoot, "planner")
	writeSkillFile(t, codexPath, "planner", "Planner", "codex-version")
	writeSkillFile(t, claudePath, "planner", "Planner", "claude-version")

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{
			tool:       "codex",
			installed:  true,
			skillPaths: []string{codexRoot},
		}),
		WithAdapter(&fakeManagerAdapter{
			tool:       "claude",
			installed:  true,
			skillPaths: []string{claudeRoot},
		}),
		WithEncryptionKey(make([]byte, 32)),
	)

	codexOverview, err := mgr.SkillsCLIOverview("codex")
	if err != nil {
		t.Fatalf("SkillsCLIOverview(codex): %v", err)
	}
	if len(codexOverview.Skills) != 1 {
		t.Fatalf("len(codexOverview.Skills) = %d, want 1", len(codexOverview.Skills))
	}
	if codexOverview.Summary.IssueCount != 0 {
		t.Fatalf("codex summary.issue_count = %d, want 0", codexOverview.Summary.IssueCount)
	}
	if codexOverview.Skills[0].SyncState != skillsCLISyncStateCanImportToGlobal {
		t.Fatalf("codex sync_state = %q, want can_import_to_global", codexOverview.Skills[0].SyncState)
	}
	if codexOverview.Skills[0].ActionSourcePath != codexPath {
		t.Fatalf("codex action_source_path = %q, want %q", codexOverview.Skills[0].ActionSourcePath, codexPath)
	}

	claudeOverview, err := mgr.SkillsCLIOverview("claude")
	if err != nil {
		t.Fatalf("SkillsCLIOverview(claude): %v", err)
	}
	if len(claudeOverview.Skills) != 1 {
		t.Fatalf("len(claudeOverview.Skills) = %d, want 1", len(claudeOverview.Skills))
	}
	if claudeOverview.Summary.IssueCount != 0 {
		t.Fatalf("claude summary.issue_count = %d, want 0", claudeOverview.Summary.IssueCount)
	}
	if claudeOverview.Skills[0].SyncState != skillsCLISyncStateCanImportToGlobal {
		t.Fatalf("claude sync_state = %q, want can_import_to_global", claudeOverview.Skills[0].SyncState)
	}
	if claudeOverview.Skills[0].ActionSourcePath != claudePath {
		t.Fatalf("claude action_source_path = %q, want %q", claudeOverview.Skills[0].ActionSourcePath, claudePath)
	}
}
