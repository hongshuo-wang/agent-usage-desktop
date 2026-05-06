package configmanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func TestSkillsDashboardExposesImportedToolSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "codex-skills")
	localPath := filepath.Join(installRoot, "planner")
	if err := os.MkdirAll(localPath, 0o755); err != nil {
		t.Fatalf("MkdirAll localPath: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localPath, "SKILL.md"), []byte("---\nname: planner\ndescription: Plan helper\n---\n"), 0o644); err != nil {
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

	if _, err := mgr.ImportManagedSkill(ImportManagedSkillRequest{
		Name:       "planner",
		Tool:       "codex",
		SourcePath: localPath,
	}); err != nil {
		t.Fatalf("ImportManagedSkill: %v", err)
	}

	dashboard, err := mgr.SkillsDashboard()
	if err != nil {
		t.Fatalf("SkillsDashboard: %v", err)
	}
	if len(dashboard.Managed) != 1 {
		t.Fatalf("len(dashboard.Managed) = %d, want 1", len(dashboard.Managed))
	}
	if dashboard.Managed[0].Source.Type != skillSourceTypeImportedTool {
		t.Fatalf("source.type = %q, want %q", dashboard.Managed[0].Source.Type, skillSourceTypeImportedTool)
	}
	if dashboard.Managed[0].Source.Label == "" {
		t.Fatal("expected non-empty source label")
	}
}

func TestSkillsDashboardReportsSupportedToolAvailability(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	db := openManagerTestDB(t)
	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{tool: "codex", installed: true, skillPaths: []string{filepath.Join(t.TempDir(), "codex-skills")}}),
		WithEncryptionKey(make([]byte, 32)),
	)

	dashboard, err := mgr.SkillsDashboard()
	if err != nil {
		t.Fatalf("SkillsDashboard: %v", err)
	}
	if len(dashboard.ToolAvailability) != len(supportedSkillTools) {
		t.Fatalf("len(tool_availability) = %d, want %d: %+v", len(dashboard.ToolAvailability), len(supportedSkillTools), dashboard.ToolAvailability)
	}
	for _, tool := range supportedSkillTools {
		if _, ok := dashboard.ToolAvailability[tool]; !ok {
			t.Fatalf("tool_availability missing key %q: %+v", tool, dashboard.ToolAvailability)
		}
	}
	if !dashboard.ToolAvailability["codex"] {
		t.Fatalf("tool_availability[codex] = false, want true when adapter is installed")
	}
}

func TestSkillsDashboardPreservesLegacyManualSourceWhileExposingOriginTool(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	db := openManagerTestDB(t)
	sourcePath := writeDashboardSkill(t, t.TempDir(), "planner", "legacy import")
	skillID, err := db.CreateSkillWithSource("planner", sourcePath, "Plan helper", storage.SkillSourceMeta{SourceType: skillSourceTypeManual})
	if err != nil {
		t.Fatalf("CreateSkillWithSource: %v", err)
	}
	variantID, err := db.CreateSkillVariant(skillID, sourcePath, "codex")
	if err != nil {
		t.Fatalf("CreateSkillVariant: %v", err)
	}
	if err := db.SetSkillCurrentVariant(skillID, variantID); err != nil {
		t.Fatalf("SetSkillCurrentVariant: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{tool: "codex", installed: true, skillPaths: []string{filepath.Join(t.TempDir(), "codex-skills")}}),
		WithEncryptionKey(make([]byte, 32)),
	)

	dashboard, err := mgr.SkillsDashboard()
	if err != nil {
		t.Fatalf("SkillsDashboard: %v", err)
	}
	if len(dashboard.Managed) != 1 {
		t.Fatalf("len(dashboard.Managed) = %d, want 1", len(dashboard.Managed))
	}
	source := dashboard.Managed[0].Source
	if source.Type != skillSourceTypeManual {
		t.Fatalf("source.type = %q, want %q", source.Type, skillSourceTypeManual)
	}
	if source.OriginTool != "codex" {
		t.Fatalf("source.origin_tool = %q, want codex", source.OriginTool)
	}
	if source.Label != "Manual" {
		t.Fatalf("source.label = %q, want Manual", source.Label)
	}
}

func TestSkillsDashboardBuildsRepositorySourceURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	db := openManagerTestDB(t)
	sourcePath := writeDashboardSkill(t, t.TempDir(), "planner", "repo content")
	skillID, err := db.CreateSkillWithSource("planner", sourcePath, "Plan helper", storage.SkillSourceMeta{
		SourceType:  skillSourceTypeRepo,
		SourceLabel: "owner/repo:skills/planner",
		RepoOwner:   "owner",
		RepoName:    "repo",
		RepoBranch:  "develop",
		RepoSubpath: "skills/planner",
	})
	if err != nil {
		t.Fatalf("CreateSkillWithSource: %v", err)
	}
	variantID, err := db.CreateSkillVariant(skillID, sourcePath, "global")
	if err != nil {
		t.Fatalf("CreateSkillVariant: %v", err)
	}
	if err := db.SetSkillCurrentVariant(skillID, variantID); err != nil {
		t.Fatalf("SetSkillCurrentVariant: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{tool: "codex", installed: true, skillPaths: []string{filepath.Join(t.TempDir(), "codex-skills")}}),
		WithEncryptionKey(make([]byte, 32)),
	)

	dashboard, err := mgr.SkillsDashboard()
	if err != nil {
		t.Fatalf("SkillsDashboard: %v", err)
	}
	source := dashboard.Managed[0].Source
	if source.Type != skillSourceTypeRepo {
		t.Fatalf("source.type = %q, want %q", source.Type, skillSourceTypeRepo)
	}
	if source.URL != "https://github.com/owner/repo/tree/develop/skills/planner" {
		t.Fatalf("source.url = %q", source.URL)
	}
}

func TestSkillsDashboardDistributionHashComparisonStates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tests := []struct {
		name            string
		setupInstall    func(t *testing.T, mgr *Manager, installRoot, installPath string)
		wantStatus      string
		wantComparison  string
		wantInstalled   bool
		wantHashesEqual bool
	}{
		{
			name: "synced",
			setupInstall: func(t *testing.T, mgr *Manager, _, _ string) {
				t.Helper()
				if _, err := mgr.SyncSkills(); err != nil {
					t.Fatalf("SyncSkills: %v", err)
				}
			},
			wantStatus:      skillDashboardStatusHealthy,
			wantComparison:  skillHashComparisonSynced,
			wantInstalled:   true,
			wantHashesEqual: true,
		},
		{
			name: "missing",
			setupInstall: func(t *testing.T, _ *Manager, _, _ string) {
				t.Helper()
			},
			wantStatus:     skillDashboardStatusHealthy,
			wantComparison: skillHashComparisonMissing,
		},
		{
			name: "different",
			setupInstall: func(t *testing.T, mgr *Manager, _, installPath string) {
				t.Helper()
				if _, err := mgr.SyncSkills(); err != nil {
					t.Fatalf("SyncSkills: %v", err)
				}
				if err := os.WriteFile(filepath.Join(installPath, "SKILL.md"), []byte("changed locally"), 0o644); err != nil {
					t.Fatalf("WriteFile installed SKILL.md: %v", err)
				}
			},
			wantStatus:     skillDashboardStatusNeedsSync,
			wantComparison: skillHashComparisonDifferent,
			wantInstalled:  true,
		},
		{
			name: "invalid",
			setupInstall: func(t *testing.T, _ *Manager, _, installPath string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(installPath, "SKILL.md"), 0o755); err != nil {
					t.Fatalf("MkdirAll invalid SKILL.md: %v", err)
				}
			},
			wantStatus:     skillDashboardStatusBroken,
			wantComparison: skillHashComparisonInvalid,
			wantInstalled:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openManagerTestDB(t)
			sourcePath := writeDashboardSkill(t, t.TempDir(), "planner", "managed content")
			installRoot := filepath.Join(t.TempDir(), "codex-skills")
			installPath := filepath.Join(installRoot, "planner")
			mgr := NewManager(
				db,
				filepath.Join(t.TempDir(), "backups"),
				WithAdapter(&fakeManagerAdapter{tool: "codex", installed: true, skillPaths: []string{installRoot}}),
				WithEncryptionKey(make([]byte, 32)),
			)
			if _, err := mgr.CreateSkill("planner", sourcePath, "Plan helper", map[string]SkillTargetRecord{
				"codex": {Tool: "codex", Method: "copy", Enabled: true, SourceKind: "global"},
			}); err != nil {
				t.Fatalf("CreateSkill: %v", err)
			}

			tt.setupInstall(t, mgr, installRoot, installPath)

			dashboard, err := mgr.SkillsDashboard()
			if err != nil {
				t.Fatalf("SkillsDashboard: %v", err)
			}
			if len(dashboard.Managed) != 1 {
				t.Fatalf("len(dashboard.Managed) = %d, want 1", len(dashboard.Managed))
			}
			skill := dashboard.Managed[0]
			if skill.Status != tt.wantStatus {
				t.Fatalf("skill.status = %q, want %q", skill.Status, tt.wantStatus)
			}
			if len(skill.Distribution) != 1 {
				t.Fatalf("len(skill.Distribution) = %d, want 1", len(skill.Distribution))
			}
			distribution := skill.Distribution[0]
			if distribution.ComparisonState != tt.wantComparison {
				t.Fatalf("comparison_state = %q, want %q", distribution.ComparisonState, tt.wantComparison)
			}
			if distribution.LibraryHash == "" || distribution.LibraryShortHash == "" {
				t.Fatalf("library hashes must be populated: %+v", distribution)
			}
			if tt.wantInstalled && (distribution.InstalledPath == "" || distribution.InstalledHash == "" || distribution.InstalledShortHash == "") {
				t.Fatalf("installed fields must be populated: %+v", distribution)
			}
			if tt.wantHashesEqual && distribution.LibraryHash != distribution.InstalledHash {
				t.Fatalf("library hash %q != installed hash %q", distribution.LibraryHash, distribution.InstalledHash)
			}
		})
	}
}

func TestSkillsDashboardUsesInstallHashForAggregateStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	db := openManagerTestDB(t)
	globalPath := writeDashboardSkill(t, t.TempDir(), "planner", "managed content")
	localPath := writeDashboardSkill(t, t.TempDir(), "planner-local", "older imported content")
	installRoot := filepath.Join(t.TempDir(), "codex-skills")
	installPath := writeDashboardSkill(t, installRoot, "planner", "managed content")
	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{tool: "codex", installed: true, skillPaths: []string{installRoot}}),
		WithEncryptionKey(make([]byte, 32)),
	)
	if _, err := mgr.CreateSkill("planner", globalPath, "Plan helper", map[string]SkillTargetRecord{
		"codex": {
			Tool:            "codex",
			Method:          "symlink",
			Enabled:         true,
			SourceKind:      "local",
			LocalSourcePath: localPath,
			LocalOriginTool: "codex",
		},
	}); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	dashboard, err := mgr.SkillsDashboard()
	if err != nil {
		t.Fatalf("SkillsDashboard: %v", err)
	}
	if len(dashboard.Managed) != 1 {
		t.Fatalf("len(dashboard.Managed) = %d, want 1", len(dashboard.Managed))
	}
	skill := dashboard.Managed[0]
	if skill.Status != skillDashboardStatusHealthy {
		t.Fatalf("skill.status = %q, want %q", skill.Status, skillDashboardStatusHealthy)
	}
	if len(skill.Distribution) != 1 {
		t.Fatalf("len(skill.Distribution) = %d, want 1", len(skill.Distribution))
	}
	distribution := skill.Distribution[0]
	if distribution.ComparisonState != skillHashComparisonSynced {
		t.Fatalf("comparison_state = %q, want %q", distribution.ComparisonState, skillHashComparisonSynced)
	}
	if distribution.InstalledPath != installPath {
		t.Fatalf("installed_path = %q, want %q", distribution.InstalledPath, installPath)
	}
	if distribution.LibraryHash != distribution.InstalledHash {
		t.Fatalf("library hash %q != installed hash %q", distribution.LibraryHash, distribution.InstalledHash)
	}
}

func TestSkillsDashboardMissingOtherToolDoesNotCreateContentDrift(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	db := openManagerTestDB(t)
	globalPath := writeDashboardSkill(t, t.TempDir(), "ui-ux-pro-max", "managed content")
	claudeRoot := filepath.Join(t.TempDir(), "claude-skills")
	codexRoot := filepath.Join(t.TempDir(), "codex-skills")
	claudePath := writeDashboardSkill(t, claudeRoot, "ui-ux-pro-max", "managed content")
	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{tool: "claude", installed: true, skillPaths: []string{claudeRoot}}),
		WithAdapter(&fakeManagerAdapter{tool: "codex", installed: true, skillPaths: []string{codexRoot}}),
		WithEncryptionKey(make([]byte, 32)),
	)
	if _, err := mgr.CreateSkill("ui-ux-pro-max", globalPath, "UI helper", map[string]SkillTargetRecord{
		"claude": {Tool: "claude", Method: "symlink", Enabled: true, SourceKind: "global"},
		"codex":  {Tool: "codex", Method: "symlink", Enabled: true, SourceKind: "global"},
	}); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	dashboard, err := mgr.SkillsDashboard()
	if err != nil {
		t.Fatalf("SkillsDashboard: %v", err)
	}
	if len(dashboard.Managed) != 1 {
		t.Fatalf("len(dashboard.Managed) = %d, want 1", len(dashboard.Managed))
	}
	skill := dashboard.Managed[0]
	if skill.Status != skillDashboardStatusHealthy {
		t.Fatalf("skill.status = %q, want %q", skill.Status, skillDashboardStatusHealthy)
	}
	byTool := map[string]SkillDistributionView{}
	for _, distribution := range skill.Distribution {
		byTool[distribution.Tool] = distribution
	}
	if byTool["claude"].ComparisonState != skillHashComparisonSynced {
		t.Fatalf("claude comparison_state = %q, want %q", byTool["claude"].ComparisonState, skillHashComparisonSynced)
	}
	if byTool["claude"].InstalledPath != claudePath {
		t.Fatalf("claude installed_path = %q, want %q", byTool["claude"].InstalledPath, claudePath)
	}
	if byTool["codex"].ComparisonState != skillHashComparisonMissing {
		t.Fatalf("codex comparison_state = %q, want %q", byTool["codex"].ComparisonState, skillHashComparisonMissing)
	}
	if byTool["codex"].InstalledPath != "" {
		t.Fatalf("codex installed_path = %q, want empty", byTool["codex"].InstalledPath)
	}
}

func TestSkillsDashboardMissingToolIgnoresImportedLocalSourceDrift(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	db := openManagerTestDB(t)
	globalPath := writeDashboardSkill(t, t.TempDir(), "ui-ux-pro-max", "managed content")
	importedClaudePath := writeDashboardSkill(t, t.TempDir(), "ui-ux-pro-max-claude", "old claude import")
	claudeRoot := filepath.Join(t.TempDir(), "claude-skills")
	codexRoot := filepath.Join(t.TempDir(), "codex-skills")
	_ = writeDashboardSkill(t, claudeRoot, "ui-ux-pro-max", "managed content")
	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{tool: "claude", installed: true, skillPaths: []string{claudeRoot}}),
		WithAdapter(&fakeManagerAdapter{tool: "codex", installed: true, skillPaths: []string{codexRoot}}),
		WithEncryptionKey(make([]byte, 32)),
	)
	if _, err := mgr.CreateSkill("ui-ux-pro-max", globalPath, "UI helper", map[string]SkillTargetRecord{
		"claude": {Tool: "claude", Method: "symlink", Enabled: true, SourceKind: "global"},
		"codex": {
			Tool:            "codex",
			Method:          "symlink",
			Enabled:         true,
			SourceKind:      "local",
			LocalSourcePath: importedClaudePath,
			LocalOriginTool: "claude",
		},
	}); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	dashboard, err := mgr.SkillsDashboard()
	if err != nil {
		t.Fatalf("SkillsDashboard: %v", err)
	}
	if len(dashboard.Managed) != 1 {
		t.Fatalf("len(dashboard.Managed) = %d, want 1", len(dashboard.Managed))
	}
	skill := dashboard.Managed[0]
	if skill.Status != skillDashboardStatusHealthy {
		t.Fatalf("skill.status = %q, want %q", skill.Status, skillDashboardStatusHealthy)
	}
	byTool := map[string]SkillDistributionView{}
	for _, distribution := range skill.Distribution {
		byTool[distribution.Tool] = distribution
	}
	if byTool["codex"].ComparisonState != skillHashComparisonMissing {
		t.Fatalf("codex comparison_state = %q, want %q", byTool["codex"].ComparisonState, skillHashComparisonMissing)
	}
	if byTool["codex"].InstalledHash != "" || byTool["codex"].InstalledPath != "" {
		t.Fatalf("codex install fields = path %q hash %q, want empty", byTool["codex"].InstalledPath, byTool["codex"].InstalledHash)
	}
}

func TestSkillsDashboardShowsActualInstallMethodAfterSwitch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	db := openManagerTestDB(t)
	sourcePath := writeDashboardSkill(t, t.TempDir(), "planner", "managed content")
	installRoot := filepath.Join(t.TempDir(), "codex-skills")
	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{tool: "codex", installed: true, skillPaths: []string{installRoot}}),
		WithEncryptionKey(make([]byte, 32)),
	)
	skillID, err := mgr.CreateSkill("planner", sourcePath, "Plan helper", map[string]SkillTargetRecord{
		"codex": {Tool: "codex", Method: "symlink", Enabled: true, SourceKind: "global"},
	})
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}
	if _, err := mgr.SyncSkills(); err != nil {
		t.Fatalf("SyncSkills symlink: %v", err)
	}

	installPath := filepath.Join(installRoot, "planner")
	info, err := os.Lstat(installPath)
	if err != nil {
		t.Fatalf("Lstat symlink install: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("installPath mode = %v, want symlink", info.Mode())
	}

	targets, err := db.GetSkillTargets(skillID)
	if err != nil {
		t.Fatalf("GetSkillTargets: %v", err)
	}
	target := targets["codex"]
	target.Method = "copy"
	targets["codex"] = target
	if err := db.SetSkillTargets(skillID, skillTargetRecordsFromMap(targets)); err != nil {
		t.Fatalf("SetSkillTargets: %v", err)
	}
	if _, err := mgr.SyncSkill(skillID); err != nil {
		t.Fatalf("SyncSkill copy: %v", err)
	}

	info, err = os.Lstat(installPath)
	if err != nil {
		t.Fatalf("Lstat copy install: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("installPath mode = %v, want copied directory", info.Mode())
	}

	dashboard, err := mgr.SkillsDashboard()
	if err != nil {
		t.Fatalf("SkillsDashboard: %v", err)
	}
	distribution := dashboard.Managed[0].Distribution[0]
	if distribution.InstalledPath != installPath {
		t.Fatalf("installed_path = %q, want %q", distribution.InstalledPath, installPath)
	}
	if distribution.Method != "copy" {
		t.Fatalf("distribution.method = %q, want copy", distribution.Method)
	}
}

func TestSkillsDashboardPrunesMissingManualSkillRecord(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	db := openManagerTestDB(t)
	sourcePath := writeDashboardSkill(t, t.TempDir(), "superpowers", "manual content")
	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(&fakeManagerAdapter{tool: "codex", installed: true, skillPaths: []string{filepath.Join(t.TempDir(), "codex-skills")}}),
		WithEncryptionKey(make([]byte, 32)),
	)
	if _, err := mgr.CreateSkill("superpowers", sourcePath, "Manual helper", map[string]SkillTargetRecord{
		"codex": {Tool: "codex", Method: "copy", Enabled: true, SourceKind: "global"},
	}); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}
	if err := os.RemoveAll(sourcePath); err != nil {
		t.Fatalf("RemoveAll sourcePath: %v", err)
	}

	dashboard, err := mgr.SkillsDashboard()
	if err != nil {
		t.Fatalf("SkillsDashboard: %v", err)
	}
	if len(dashboard.Managed) != 0 {
		t.Fatalf("len(dashboard.Managed) = %d, want 0", len(dashboard.Managed))
	}
	skills, err := db.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("len(skills) = %d, want stale record deleted", len(skills))
	}
}

func TestSkillsDiscoverIncludesRepoRefreshResults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	db := openManagerTestDB(t)
	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithEncryptionKey(make([]byte, 32)),
		WithSkillRepoFetcher(func(_ context.Context, source storage.SkillRepoSourceRecord) ([]RepoDiscoveredSkill, error) {
			return []RepoDiscoveredSkill{{
				Name:        "find-skills",
				Description: "Find and install skills",
				Path:        "skills/find-skills",
				ReadmeURL:   "https://example.com/find-skills",
			}}, nil
		}),
	)

	if _, err := mgr.CreateSkillRepoSource(SkillRepoSourceCreateRequest{
		Owner:   "openai",
		Repo:    "codex",
		Branch:  "main",
		Subpath: "skills",
		Enabled: true,
	}); err != nil {
		t.Fatalf("CreateSkillRepoSource: %v", err)
	}

	discover, err := mgr.SkillsDiscover(context.Background())
	if err != nil {
		t.Fatalf("SkillsDiscover: %v", err)
	}
	if len(discover.Repos) != 1 {
		t.Fatalf("len(discover.Repos) = %d, want 1", len(discover.Repos))
	}
	if discover.Repos[0].SkillCount != 1 {
		t.Fatalf("discover.Repos[0].SkillCount = %d, want 1", discover.Repos[0].SkillCount)
	}
	if len(discover.Repos[0].Skills) != 1 || discover.Repos[0].Skills[0].Name != "find-skills" {
		t.Fatalf("unexpected repo discoveries: %+v", discover.Repos[0].Skills)
	}
}

func writeDashboardSkill(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", path, err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile SKILL.md: %v", err)
	}
	return path
}
