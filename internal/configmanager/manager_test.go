package configmanager

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func openManagerTestDB(t *testing.T) *storage.DB {
	t.Helper()

	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "manager-test.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestManagerActivateProfile(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	backupDir := filepath.Join(t.TempDir(), "backups")
	claudeDir := t.TempDir()
	claudeJSONPath := filepath.Join(claudeDir, ".claude.json")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	if err := writeJSON(settingsPath, map[string]any{}); err != nil {
		t.Fatalf("writeJSON(settings.json) setup error = %v", err)
	}

	mgr := NewManager(
		db,
		backupDir,
		WithClaudeAdapter(claudeDir, claudeJSONPath),
		WithEncryptionKey(make([]byte, 32)),
	)

	profileID, err := mgr.CreateProfile(
		"test",
		`{"api_key":"sk-123","base_url":"https://api.anthropic.com","model":"claude-sonnet"}`,
		map[string]bool{"claude": true},
	)
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	affected, err := mgr.ActivateProfile(profileID)
	if err != nil {
		t.Fatalf("ActivateProfile() error = %v", err)
	}
	if len(affected) == 0 {
		t.Fatalf("len(affected) = 0, want > 0")
	}

	var settings map[string]any
	if err := readJSON(settingsPath, &settings); err != nil {
		t.Fatalf("readJSON(settings.json) error = %v", err)
	}
	env, ok := settings["env"].(map[string]any)
	if !ok {
		t.Fatalf("settings.env missing or invalid type")
	}
	if env["ANTHROPIC_API_KEY"] != "sk-123" {
		t.Fatalf("env.ANTHROPIC_API_KEY = %v, want %q", env["ANTHROPIC_API_KEY"], "sk-123")
	}
	if env["ANTHROPIC_BASE_URL"] != "https://api.anthropic.com" {
		t.Fatalf("env.ANTHROPIC_BASE_URL = %v, want %q", env["ANTHROPIC_BASE_URL"], "https://api.anthropic.com")
	}
	if env["ANTHROPIC_MODEL"] != "claude-sonnet" {
		t.Fatalf("env.ANTHROPIC_MODEL = %v, want %q", env["ANTHROPIC_MODEL"], "claude-sonnet")
	}

	profiles, err := mgr.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}

	var activeCount int
	for _, profile := range profiles {
		if profile.IsActive {
			activeCount++
			if profile.ID != profileID {
				t.Fatalf("active profile id = %d, want %d", profile.ID, profileID)
			}
		}
	}
	if activeCount != 1 {
		t.Fatalf("active profile count = %d, want 1", activeCount)
	}

	backups, err := db.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups() error = %v", err)
	}
	if len(backups) == 0 {
		t.Fatalf("expected backups to be created during activation")
	}
	if backups[0].TriggerType != "profile_switch" {
		t.Fatalf("backup trigger_type = %q, want %q", backups[0].TriggerType, "profile_switch")
	}
}

func TestManagerBootstrapDefaultProfile(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	backupDir := filepath.Join(t.TempDir(), "backups")
	claudeDir := t.TempDir()
	claudeJSONPath := filepath.Join(claudeDir, ".claude.json")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	if err := writeJSON(settingsPath, map[string]any{
		"env": map[string]any{
			"ANTHROPIC_API_KEY": "sk-bootstrap",
		},
	}); err != nil {
		t.Fatalf("writeJSON(settings.json) setup error = %v", err)
	}

	mgr := NewManager(
		db,
		backupDir,
		WithClaudeAdapter(claudeDir, claudeJSONPath),
		WithEncryptionKey(make([]byte, 32)),
	)

	if err := mgr.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	profiles, err := mgr.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("len(profiles) = %d, want 1", len(profiles))
	}
	if profiles[0].Name != "Default" {
		t.Fatalf("profiles[0].Name = %q, want %q", profiles[0].Name, "Default")
	}
	if !profiles[0].IsActive {
		t.Fatalf("profiles[0].IsActive = false, want true")
	}

	var cfg ProviderConfig
	if err := json.Unmarshal([]byte(profiles[0].Config), &cfg); err != nil {
		t.Fatalf("json.Unmarshal(profile.Config) error = %v", err)
	}
	if cfg.APIKey == "" {
		t.Fatalf("bootstrap profile API key is empty")
	}
}

func TestManagerBootstrapAndGetSyncStatusAcceptCompatibleExternalConfigShapes(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	backupDir := filepath.Join(t.TempDir(), "backups")

	codexDir := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll(codexDir) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "auth.json"), []byte(`{"api_key":"sk-codex"}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile(auth.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(codexDir, "config.toml"), []byte(`
model = "gpt-5.4"

[provider]
base_url = "https://api.openai.com/v1"
`), 0o644); err != nil {
		t.Fatalf("os.WriteFile(config.toml) error = %v", err)
	}

	claudeDir := t.TempDir()
	if err := writeJSON(filepath.Join(claudeDir, "settings.json"), map[string]any{
		"env": map[string]any{
			"ANTHROPIC_API_KEY": "sk-claude",
		},
	}); err != nil {
		t.Fatalf("writeJSON(settings.json) error = %v", err)
	}
	if err := writeJSON(filepath.Join(claudeDir, ".claude.json"), map[string]any{
		"mcpServers": map[string]any{
			"tavily": map[string]any{
				"type": "http",
				"url":  "https://mcp.tavily.com/server",
			},
		},
	}); err != nil {
		t.Fatalf("writeJSON(.claude.json) error = %v", err)
	}

	mgr := NewManager(
		db,
		backupDir,
		WithCodexAdapter(codexDir),
		WithClaudeAdapter(claudeDir, filepath.Join(claudeDir, ".claude.json")),
		WithEncryptionKey(make([]byte, 32)),
	)

	if err := mgr.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap() error = %v", err)
	}

	status, err := mgr.GetSyncStatus()
	if err != nil {
		t.Fatalf("GetSyncStatus() error = %v", err)
	}
	if status == nil {
		t.Fatalf("GetSyncStatus() = nil, want non-nil")
	}
}

func TestManagerActivateProfileRollbackOnAdapterFailure(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	key := make([]byte, 32)

	adapterOne := &fakeManagerAdapter{tool: "one", installed: true}
	adapterTwo := &fakeManagerAdapter{tool: "two", installed: true}
	adapterOneFile := filepath.Join(t.TempDir(), "one.json")
	adapterTwoFile := filepath.Join(t.TempDir(), "two.json")
	if err := os.WriteFile(adapterOneFile, []byte(`{"k":"v1"}`), 0o644); err != nil {
		t.Fatalf("WriteFile adapterOneFile: %v", err)
	}
	if err := os.WriteFile(adapterTwoFile, []byte(`{"k":"v2"}`), 0o644); err != nil {
		t.Fatalf("WriteFile adapterTwoFile: %v", err)
	}
	adapterOne.configFiles = []ConfigFileInfo{{Path: adapterOneFile, Tool: "one", Exists: true}}
	adapterTwo.configFiles = []ConfigFileInfo{{Path: adapterTwoFile, Tool: "two", Exists: true}}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(adapterOne),
		WithAdapter(adapterTwo),
		WithEncryptionKey(key),
	)

	oldProfileID, err := mgr.CreateProfile(
		"old",
		`{"api_key":"sk-old","base_url":"https://old","model":"m-old"}`,
		map[string]bool{"one": true, "two": true},
	)
	if err != nil {
		t.Fatalf("CreateProfile old: %v", err)
	}
	if _, err := mgr.ActivateProfile(oldProfileID); err != nil {
		t.Fatalf("ActivateProfile old: %v", err)
	}

	newProfileID, err := mgr.CreateProfile(
		"new",
		`{"api_key":"sk-new","base_url":"https://new","model":"m-new"}`,
		map[string]bool{"one": true, "two": true},
	)
	if err != nil {
		t.Fatalf("CreateProfile new: %v", err)
	}

	adapterTwo.failSet = errors.New("boom")
	if _, err := mgr.ActivateProfile(newProfileID); err == nil {
		t.Fatalf("ActivateProfile new error = nil, want non-nil")
	}

	if adapterOne.current == nil {
		t.Fatalf("adapterOne.current = nil, want restored config")
	}
	if adapterOne.current.APIKey != "sk-old" {
		t.Fatalf("adapterOne.current.APIKey = %q, want %q", adapterOne.current.APIKey, "sk-old")
	}

	profiles, err := mgr.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	var activeID int64
	for _, profile := range profiles {
		if profile.IsActive {
			activeID = profile.ID
		}
	}
	if activeID != oldProfileID {
		t.Fatalf("active profile id = %d, want %d", activeID, oldProfileID)
	}

	backups, err := db.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) < 2 {
		t.Fatalf("expected at least 2 backup rows, got %d", len(backups))
	}
}

func TestManagerCreateAndUpdateProfileSkipDoubleEncryption(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	key := make([]byte, 32)
	adapter := &fakeManagerAdapter{tool: "fake", installed: true}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(adapter),
		WithEncryptionKey(key),
	)

	encrypted, err := Encrypt("sk-original", key)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	profileID, err := mgr.CreateProfile(
		"enc-profile",
		fmt.Sprintf(`{"api_key":%q,"base_url":"https://x","model":"m"}`, encrypted),
		map[string]bool{"fake": true},
	)
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	if err := mgr.UpdateProfile(profileID, "enc-profile", fmt.Sprintf(`{"api_key":%q,"base_url":"https://x2","model":"m2"}`, encrypted)); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	profile, err := db.GetProfile(profileID)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if profile == nil {
		t.Fatalf("GetProfile = nil, want profile")
	}

	var stored ProviderConfig
	if err := json.Unmarshal([]byte(profile.Config), &stored); err != nil {
		t.Fatalf("json.Unmarshal profile.Config: %v", err)
	}
	if stored.APIKey != encrypted {
		t.Fatalf("stored API key changed unexpectedly; got %q, want %q", stored.APIKey, encrypted)
	}
	if strings.Count(stored.APIKey, "enc:") != 1 {
		t.Fatalf("stored API key should contain one enc prefix, got %q", stored.APIKey)
	}

	if _, err := mgr.ActivateProfile(profileID); err != nil {
		t.Fatalf("ActivateProfile: %v", err)
	}
	if adapter.current == nil {
		t.Fatalf("adapter current = nil")
	}
	if adapter.current.APIKey != "sk-original" {
		t.Fatalf("adapter API key = %q, want %q", adapter.current.APIKey, "sk-original")
	}
}

func TestManagerActivateProfileEncryptedWithoutValidKeyFails(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	validKey := make([]byte, 32)
	encrypted, err := Encrypt("sk-secret", validKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	adapter := &fakeManagerAdapter{tool: "fake", installed: true}
	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(adapter),
		WithEncryptionKey([]byte{1}),
		WithEncryptionKeyProvider(func() ([]byte, error) { return nil, errors.New("no key") }),
	)

	profileID, err := mgr.CreateProfile(
		"encrypted",
		fmt.Sprintf(`{"api_key":%q,"base_url":"https://x","model":"m"}`, encrypted),
		map[string]bool{"fake": true},
	)
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	if _, err := mgr.ActivateProfile(profileID); err == nil {
		t.Fatalf("ActivateProfile error = nil, want non-nil")
	}
}

func TestManagerActivateProfileEncryptedRetriesKeyProvider(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	validKey := make([]byte, 32)
	encrypted, err := Encrypt("sk-secret", validKey)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	adapter := &fakeManagerAdapter{tool: "fake", installed: true}
	keyCalls := 0
	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(adapter),
		WithEncryptionKey([]byte{1}),
		WithEncryptionKeyProvider(func() ([]byte, error) {
			keyCalls++
			return validKey, nil
		}),
	)

	profileID, err := mgr.CreateProfile(
		"encrypted-retry",
		fmt.Sprintf(`{"api_key":%q,"base_url":"https://x","model":"m"}`, encrypted),
		map[string]bool{"fake": true},
	)
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	if _, err := mgr.ActivateProfile(profileID); err != nil {
		t.Fatalf("ActivateProfile: %v", err)
	}
	if keyCalls != 1 {
		t.Fatalf("key provider calls = %d, want 1", keyCalls)
	}
	if adapter.current == nil {
		t.Fatalf("adapter current = nil")
	}
	if adapter.current.APIKey != "sk-secret" {
		t.Fatalf("adapter API key = %q, want %q", adapter.current.APIKey, "sk-secret")
	}
}

func TestManagerSyncSkillsCopy(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	adapter := &fakeManagerAdapter{
		tool:       "fake",
		installed:  true,
		skillPaths: []string{filepath.Join(t.TempDir(), "skills-target")},
	}

	srcRoot := filepath.Join(t.TempDir(), "skill-src")
	srcFile := filepath.Join(srcRoot, "SKILL.md")
	if err := os.MkdirAll(srcRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll srcRoot: %v", err)
	}
	if err := os.WriteFile(srcFile, []byte("hello-skill"), 0o644); err != nil {
		t.Fatalf("WriteFile srcFile: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(adapter),
		WithEncryptionKey(make([]byte, 32)),
	)

	if _, err := mgr.CreateSkill("demo", srcRoot, "desc", map[string]SkillTargetRecord{
		"fake": {Tool: "fake", Method: "copy", Enabled: true},
	}); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	affected, err := mgr.SyncSkills()
	if err != nil {
		t.Fatalf("SyncSkills: %v", err)
	}
	if len(affected) == 0 {
		t.Fatalf("len(affected) = 0, want > 0")
	}

	dstFile := filepath.Join(adapter.skillPaths[0], filepath.Base(srcRoot), "SKILL.md")
	data, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("ReadFile dstFile: %v", err)
	}
	if string(data) != "hello-skill" {
		t.Fatalf("copied skill content = %q, want %q", string(data), "hello-skill")
	}
}

func TestManagerCreateSkillRejectsMissingSourceDirectory(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithEncryptionKey(make([]byte, 32)),
	)

	missingPath := filepath.Join(t.TempDir(), "missing-skill")
	if _, err := mgr.CreateSkill("missing", missingPath, "desc", nil); err == nil {
		t.Fatalf("CreateSkill error = nil, want non-nil")
	} else if !strings.Contains(err.Error(), "source_path") {
		t.Fatalf("CreateSkill error = %q, want source_path validation error", err)
	}

	skills, err := db.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("len(skills) = %d, want 0", len(skills))
	}
}

func TestManagerCreateSkillRejectsNonDirectorySourcePath(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithEncryptionKey(make([]byte, 32)),
	)

	filePath := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(filePath, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatalf("WriteFile filePath: %v", err)
	}

	if _, err := mgr.CreateSkill("file", filePath, "desc", nil); err == nil {
		t.Fatalf("CreateSkill error = nil, want non-nil")
	} else if !strings.Contains(err.Error(), "directory") {
		t.Fatalf("CreateSkill error = %q, want directory validation error", err)
	}
}

func TestManagerCreateSkillCleansUpRowWhenSetSkillTargetsFails(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	sourceRoot := filepath.Join(t.TempDir(), "skill-src")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll sourceRoot: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithEncryptionKey(make([]byte, 32)),
	)
	mgr.setSkillTargetsFn = func(skillID int64, targets []storage.SkillTargetRecord) error {
		return errors.New("boom")
	}

	if _, err := mgr.CreateSkill("demo", sourceRoot, "desc", map[string]SkillTargetRecord{
		"fake": {Tool: "fake", Method: "copy", Enabled: true},
	}); err == nil {
		t.Fatalf("CreateSkill error = nil, want non-nil")
	}

	skills, err := db.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("len(skills) = %d, want 0", len(skills))
	}
}

func TestManagerUpdateSkillRejectsMissingSourceDirectory(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	sourceRoot := filepath.Join(t.TempDir(), "skill-src")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll sourceRoot: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithEncryptionKey(make([]byte, 32)),
	)

	skillID, err := mgr.CreateSkill("demo", sourceRoot, "desc", nil)
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	missingPath := filepath.Join(t.TempDir(), "missing-skill")
	if err := mgr.UpdateSkill(skillID, "demo", missingPath, "desc", true); err == nil {
		t.Fatalf("UpdateSkill error = nil, want non-nil")
	} else if !strings.Contains(err.Error(), "source_path") {
		t.Fatalf("UpdateSkill error = %q, want source_path validation error", err)
	}

	skill, err := db.GetSkill(skillID)
	if err != nil {
		t.Fatalf("GetSkill: %v", err)
	}
	if skill == nil {
		t.Fatalf("GetSkill = nil")
	}
	if skill.SourcePath != sourceRoot {
		t.Fatalf("skill.SourcePath = %q, want %q", skill.SourcePath, sourceRoot)
	}
}

func TestManagerUpdateSkillWithTargetsRejectsNonDirectorySourcePath(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	sourceRoot := filepath.Join(t.TempDir(), "skill-src")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll sourceRoot: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithEncryptionKey(make([]byte, 32)),
	)

	skillID, err := mgr.CreateSkill("demo", sourceRoot, "desc", map[string]SkillTargetRecord{
		"fake": {Tool: "fake", Method: "copy", Enabled: true},
	})
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	filePath := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(filePath, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatalf("WriteFile filePath: %v", err)
	}

	if err := mgr.UpdateSkillWithTargets(skillID, "demo", filePath, "desc", true, map[string]SkillTargetRecord{
		"fake": {Tool: "fake", Method: "copy", Enabled: true},
	}); err == nil {
		t.Fatalf("UpdateSkillWithTargets error = nil, want non-nil")
	} else if !strings.Contains(err.Error(), "directory") {
		t.Fatalf("UpdateSkillWithTargets error = %q, want directory validation error", err)
	}
}

func TestDefaultSkillSyncMethodForOS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		goos string
		want string
	}{
		{name: "windows defaults to copy", goos: "windows", want: "copy"},
		{name: "darwin defaults to symlink", goos: "darwin", want: "symlink"},
		{name: "linux defaults to symlink", goos: "linux", want: "symlink"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := defaultSkillSyncMethodForOS(tt.goos); got != tt.want {
				t.Fatalf("defaultSkillSyncMethodForOS(%q) = %q, want %q", tt.goos, got, tt.want)
			}
		})
	}
}

func TestManagerSyncSkillsPersistsDefaultMethodUsed(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	adapter := &fakeManagerAdapter{
		tool:       "fake",
		installed:  true,
		skillPaths: []string{filepath.Join(t.TempDir(), "skills-target")},
	}

	sourceRoot := filepath.Join(t.TempDir(), "skill-src")
	sourceFile := filepath.Join(sourceRoot, "SKILL.md")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll sourceRoot: %v", err)
	}
	if err := os.WriteFile(sourceFile, []byte("hello-skill"), 0o644); err != nil {
		t.Fatalf("WriteFile sourceFile: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(adapter),
		WithEncryptionKey(make([]byte, 32)),
	)

	skillID, err := mgr.CreateSkill("demo", sourceRoot, "desc", map[string]SkillTargetRecord{
		"fake": {Tool: "fake", Enabled: true},
	})
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	if _, err := mgr.SyncSkills(); err != nil {
		t.Fatalf("SyncSkills: %v", err)
	}

	targets, err := db.GetSkillTargets(skillID)
	if err != nil {
		t.Fatalf("GetSkillTargets: %v", err)
	}

	wantMethod := defaultSkillSyncMethodForOS(runtime.GOOS)
	if targets["fake"].Method != wantMethod {
		t.Fatalf("targets[\"fake\"].Method = %q, want %q", targets["fake"].Method, wantMethod)
	}

	dstPath := filepath.Join(adapter.skillPaths[0], filepath.Base(sourceRoot))
	info, err := os.Lstat(dstPath)
	if err != nil {
		t.Fatalf("Lstat dstPath: %v", err)
	}
	if wantMethod == "symlink" && info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("dstPath mode = %v, want symlink", info.Mode())
	}
	if wantMethod == "copy" && info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("dstPath mode = %v, want copied directory", info.Mode())
	}
}

func TestManagerSyncSkillsFallsBackToCopyAndPersistsActualMethod(t *testing.T) {
	db := openManagerTestDB(t)
	adapter := &fakeManagerAdapter{
		tool:       "fake",
		installed:  true,
		skillPaths: []string{filepath.Join(t.TempDir(), "skills-target")},
	}

	sourceRoot := filepath.Join(t.TempDir(), "skill-src")
	sourceFile := filepath.Join(sourceRoot, "SKILL.md")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll sourceRoot: %v", err)
	}
	if err := os.WriteFile(sourceFile, []byte("hello-skill"), 0o644); err != nil {
		t.Fatalf("WriteFile sourceFile: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(adapter),
		WithEncryptionKey(make([]byte, 32)),
	)
	mgr.skillSymlinkFn = func(oldname, newname string) error {
		return errors.New("symlink not permitted")
	}

	skillID, err := mgr.CreateSkill("demo", sourceRoot, "desc", map[string]SkillTargetRecord{
		"fake": {Tool: "fake", Method: "symlink", Enabled: true},
	})
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	if _, err := mgr.SyncSkills(); err != nil {
		t.Fatalf("SyncSkills: %v", err)
	}

	dstPath := filepath.Join(adapter.skillPaths[0], filepath.Base(sourceRoot))
	info, err := os.Lstat(dstPath)
	if err != nil {
		t.Fatalf("Lstat dstPath: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("dstPath mode = %v, want copied directory after fallback", info.Mode())
	}

	data, err := os.ReadFile(filepath.Join(dstPath, "SKILL.md"))
	if err != nil {
		t.Fatalf("ReadFile fallback copy: %v", err)
	}
	if string(data) != "hello-skill" {
		t.Fatalf("fallback copied content = %q, want %q", string(data), "hello-skill")
	}

	targets, err := db.GetSkillTargets(skillID)
	if err != nil {
		t.Fatalf("GetSkillTargets: %v", err)
	}
	if targets["fake"].Method != "copy" {
		t.Fatalf("targets[\"fake\"].Method = %q, want %q", targets["fake"].Method, "copy")
	}
}

func TestManagerSyncSkillsSourcePathMatchingDestinationIsSafe(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "skills-target")
	sourceRoot := filepath.Join(installRoot, "skill-src")
	sourceFile := filepath.Join(sourceRoot, "SKILL.md")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll sourceRoot: %v", err)
	}
	if err := os.WriteFile(sourceFile, []byte("hello-skill"), 0o644); err != nil {
		t.Fatalf("WriteFile sourceFile: %v", err)
	}

	adapter := &fakeManagerAdapter{
		tool:       "fake",
		installed:  true,
		skillPaths: []string{installRoot},
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(adapter),
		WithEncryptionKey(make([]byte, 32)),
	)

	if _, err := mgr.CreateSkill("demo", sourceRoot, "desc", map[string]SkillTargetRecord{
		"fake": {Tool: "fake", Method: "copy", Enabled: true},
	}); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	if _, err := mgr.SyncSkills(); err != nil {
		t.Fatalf("SyncSkills: %v", err)
	}

	data, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Fatalf("ReadFile sourceFile after self-sync: %v", err)
	}
	if string(data) != "hello-skill" {
		t.Fatalf("source content after self-sync = %q, want %q", string(data), "hello-skill")
	}
}

func TestManagerSyncSkillsRejectsMissingSourceDirectory(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	adapter := &fakeManagerAdapter{
		tool:       "fake",
		installed:  true,
		skillPaths: []string{filepath.Join(t.TempDir(), "skills-target")},
	}

	sourceRoot := filepath.Join(t.TempDir(), "skill-src")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll sourceRoot: %v", err)
	}
	sourceFile := filepath.Join(sourceRoot, "SKILL.md")
	if err := os.WriteFile(sourceFile, []byte("hello-skill"), 0o644); err != nil {
		t.Fatalf("WriteFile sourceFile: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(adapter),
		WithEncryptionKey(make([]byte, 32)),
	)

	if _, err := mgr.CreateSkill("demo", sourceRoot, "desc", map[string]SkillTargetRecord{
		"fake": {Tool: "fake", Method: "copy", Enabled: true},
	}); err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	if err := os.RemoveAll(sourceRoot); err != nil {
		t.Fatalf("RemoveAll sourceRoot: %v", err)
	}

	if _, err := mgr.SyncSkills(); err == nil {
		t.Fatalf("SyncSkills error = nil, want non-nil")
	} else if !strings.Contains(err.Error(), "source_path") {
		t.Fatalf("SyncSkills error = %q, want source_path validation error", err)
	}

	dstPath := filepath.Join(adapter.skillPaths[0], filepath.Base(sourceRoot))
	if fileExists(dstPath) {
		t.Fatalf("destination %s should not exist after failed sync", dstPath)
	}
}

func TestManagerSyncSkillsRejectsNonDirectorySourcePath(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	adapter := &fakeManagerAdapter{
		tool:       "fake",
		installed:  true,
		skillPaths: []string{filepath.Join(t.TempDir(), "skills-target")},
	}

	sourceRoot := filepath.Join(t.TempDir(), "skill-src")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll sourceRoot: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(adapter),
		WithEncryptionKey(make([]byte, 32)),
	)

	skillID, err := mgr.CreateSkill("demo", sourceRoot, "desc", map[string]SkillTargetRecord{
		"fake": {Tool: "fake", Method: "copy", Enabled: true},
	})
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	filePath := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(filePath, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatalf("WriteFile filePath: %v", err)
	}
	if err := mgr.UpdateSkill(skillID, "demo", filePath, "desc", true); err == nil {
		t.Fatalf("UpdateSkill error = nil, want non-nil")
	}
	if err := db.UpdateSkill(skillID, "demo", filePath, "desc", true); err != nil {
		t.Fatalf("db.UpdateSkill bypass setup: %v", err)
	}

	if _, err := mgr.SyncSkills(); err == nil {
		t.Fatalf("SyncSkills error = nil, want non-nil")
	} else if !strings.Contains(err.Error(), "directory") {
		t.Fatalf("SyncSkills error = %q, want directory validation error", err)
	}
}

func TestManagerSyncSkillsRejectsDanglingSourceSymlink(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	adapter := &fakeManagerAdapter{
		tool:       "fake",
		installed:  true,
		skillPaths: []string{filepath.Join(t.TempDir(), "skills-target")},
	}

	sourceRoot := filepath.Join(t.TempDir(), "skill-src")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll sourceRoot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "SKILL.md"), []byte("hello-skill"), 0o644); err != nil {
		t.Fatalf("WriteFile source skill: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(adapter),
		WithEncryptionKey(make([]byte, 32)),
	)

	skillID, err := mgr.CreateSkill("demo", sourceRoot, "desc", map[string]SkillTargetRecord{
		"fake": {Tool: "fake", Method: "symlink", Enabled: true},
	})
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	danglingPath := filepath.Join(t.TempDir(), "dangling-skill")
	if err := os.Symlink(filepath.Join(t.TempDir(), "missing-target"), danglingPath); err != nil {
		t.Fatalf("Symlink danglingPath: %v", err)
	}
	if err := db.UpdateSkill(skillID, "demo", danglingPath, "desc", true); err != nil {
		t.Fatalf("db.UpdateSkill bypass setup: %v", err)
	}

	if _, err := mgr.SyncSkills(); err == nil {
		t.Fatalf("SyncSkills error = nil, want non-nil")
	} else if !strings.Contains(err.Error(), "source_path") {
		t.Fatalf("SyncSkills error = %q, want source_path validation error", err)
	}

	dstPath := filepath.Join(adapter.skillPaths[0], filepath.Base(danglingPath))
	if fileExists(dstPath) {
		t.Fatalf("destination %s should not exist for dangling source symlink", dstPath)
	}
}

func TestManagerSyncSkillsRollsBackEarlierChangesWhenLaterSkillFails(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "skills-target")
	adapter := &fakeManagerAdapter{
		tool:       "fake",
		installed:  true,
		skillPaths: []string{installRoot},
	}

	sourceOne := filepath.Join(t.TempDir(), "skill-one")
	if err := os.MkdirAll(sourceOne, 0o755); err != nil {
		t.Fatalf("MkdirAll sourceOne: %v", err)
	}
	sourceOneFile := filepath.Join(sourceOne, "SKILL.md")
	if err := os.WriteFile(sourceOneFile, []byte("v1"), 0o644); err != nil {
		t.Fatalf("WriteFile sourceOneFile: %v", err)
	}

	sourceTwo := filepath.Join(t.TempDir(), "skill-two")
	if err := os.MkdirAll(sourceTwo, 0o755); err != nil {
		t.Fatalf("MkdirAll sourceTwo: %v", err)
	}
	sourceTwoFile := filepath.Join(sourceTwo, "SKILL.md")
	if err := os.WriteFile(sourceTwoFile, []byte("v2"), 0o644); err != nil {
		t.Fatalf("WriteFile sourceTwoFile: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(adapter),
		WithEncryptionKey(make([]byte, 32)),
	)

	if _, err := mgr.CreateSkill("one", sourceOne, "desc", map[string]SkillTargetRecord{
		"fake": {Tool: "fake", Method: "copy", Enabled: true},
	}); err != nil {
		t.Fatalf("CreateSkill one: %v", err)
	}
	if _, err := mgr.SyncSkills(); err != nil {
		t.Fatalf("SyncSkills initial: %v", err)
	}

	dstOne := filepath.Join(installRoot, filepath.Base(sourceOne), "SKILL.md")
	data, err := os.ReadFile(dstOne)
	if err != nil {
		t.Fatalf("ReadFile dstOne initial: %v", err)
	}
	if string(data) != "v1" {
		t.Fatalf("initial dstOne = %q, want %q", string(data), "v1")
	}

	if err := os.WriteFile(sourceOneFile, []byte("v1-updated"), 0o644); err != nil {
		t.Fatalf("WriteFile sourceOneFile updated: %v", err)
	}
	if _, err := mgr.CreateSkill("two", sourceTwo, "desc", map[string]SkillTargetRecord{
		"fake": {Tool: "fake", Method: "copy", Enabled: true},
	}); err != nil {
		t.Fatalf("CreateSkill two: %v", err)
	}
	if err := os.RemoveAll(sourceTwo); err != nil {
		t.Fatalf("RemoveAll sourceTwo: %v", err)
	}

	if _, err := mgr.SyncSkills(); err == nil {
		t.Fatalf("SyncSkills error = nil, want non-nil")
	}

	data, err = os.ReadFile(dstOne)
	if err != nil {
		t.Fatalf("ReadFile dstOne after rollback: %v", err)
	}
	if string(data) != "v1" {
		t.Fatalf("dstOne after rollback = %q, want %q", string(data), "v1")
	}

	dstTwo := filepath.Join(installRoot, filepath.Base(sourceTwo))
	if fileExists(dstTwo) {
		t.Fatalf("destination %s should not exist after rollback", dstTwo)
	}
}

func TestManagerSyncSkillsRollsBackAppliedTargetMethodUpdatesWhenLaterUpdateFails(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "skills-target")
	adapter := &fakeManagerAdapter{
		tool:       "fake",
		installed:  true,
		skillPaths: []string{installRoot},
	}

	sourceOne := filepath.Join(t.TempDir(), "skill-one")
	if err := os.MkdirAll(sourceOne, 0o755); err != nil {
		t.Fatalf("MkdirAll sourceOne: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceOne, "SKILL.md"), []byte("v1"), 0o644); err != nil {
		t.Fatalf("WriteFile sourceOne skill: %v", err)
	}

	sourceTwo := filepath.Join(t.TempDir(), "skill-two")
	if err := os.MkdirAll(sourceTwo, 0o755); err != nil {
		t.Fatalf("MkdirAll sourceTwo: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceTwo, "SKILL.md"), []byte("v2"), 0o644); err != nil {
		t.Fatalf("WriteFile sourceTwo skill: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(adapter),
		WithEncryptionKey(make([]byte, 32)),
	)
	mgr.skillSymlinkFn = func(oldname, newname string) error {
		return errors.New("symlink not permitted")
	}

	skillOneID, err := mgr.CreateSkill("one", sourceOne, "desc", map[string]SkillTargetRecord{
		"fake": {Tool: "fake", Method: "symlink", Enabled: true},
	})
	if err != nil {
		t.Fatalf("CreateSkill one: %v", err)
	}
	skillTwoID, err := mgr.CreateSkill("two", sourceTwo, "desc", map[string]SkillTargetRecord{
		"fake": {Tool: "fake", Method: "symlink", Enabled: true},
	})
	if err != nil {
		t.Fatalf("CreateSkill two: %v", err)
	}

	updateCalls := 0
	mgr.setSkillTargetsFn = func(skillID int64, targets []storage.SkillTargetRecord) error {
		updateCalls++
		if updateCalls == 2 {
			return errors.New("boom")
		}
		return db.SetSkillTargets(skillID, targets)
	}

	if _, err := mgr.SyncSkills(); err == nil {
		t.Fatalf("SyncSkills error = nil, want non-nil")
	}

	if updateCalls != 2 {
		t.Fatalf("setSkillTargets calls = %d, want 2", updateCalls)
	}

	targetsOne, err := db.GetSkillTargets(skillOneID)
	if err != nil {
		t.Fatalf("GetSkillTargets one: %v", err)
	}
	if targetsOne["fake"].Method != "symlink" {
		t.Fatalf("targetsOne[\"fake\"].Method = %q, want %q", targetsOne["fake"].Method, "symlink")
	}

	targetsTwo, err := db.GetSkillTargets(skillTwoID)
	if err != nil {
		t.Fatalf("GetSkillTargets two: %v", err)
	}
	if targetsTwo["fake"].Method != "symlink" {
		t.Fatalf("targetsTwo[\"fake\"].Method = %q, want %q", targetsTwo["fake"].Method, "symlink")
	}

	if fileExists(filepath.Join(installRoot, filepath.Base(sourceOne))) {
		t.Fatalf("expected sourceOne install to be rolled back")
	}
	if fileExists(filepath.Join(installRoot, filepath.Base(sourceTwo))) {
		t.Fatalf("expected sourceTwo install to be rolled back")
	}
}

func TestManagerSyncSkillsRemovesDeletedInstallations(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	installRoot := filepath.Join(t.TempDir(), "skills-target")
	adapter := &fakeManagerAdapter{
		tool:       "codex",
		installed:  true,
		skillPaths: []string{installRoot},
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(adapter),
		WithEncryptionKey(make([]byte, 32)),
	)

	sourceRoot := filepath.Join(t.TempDir(), "skill-src")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll sourceRoot: %v", err)
	}
	sourceFile := filepath.Join(sourceRoot, "SKILL.md")
	if err := os.WriteFile(sourceFile, []byte("hello-skill"), 0o644); err != nil {
		t.Fatalf("WriteFile sourceFile: %v", err)
	}

	skillID, err := mgr.CreateSkill(
		"planner",
		sourceRoot,
		"Plan helper",
		map[string]SkillTargetRecord{
			"codex": {Tool: "codex", Method: "copy", Enabled: true},
		},
	)
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	if _, err := mgr.SyncSkills(); err != nil {
		t.Fatalf("SyncSkills initial: %v", err)
	}

	installedPath := filepath.Join(installRoot, filepath.Base(sourceRoot))
	if !fileExists(installedPath) {
		t.Fatalf("expected installed skill at %s", installedPath)
	}

	if err := mgr.DeleteSkill(skillID); err != nil {
		t.Fatalf("DeleteSkill: %v", err)
	}
	if _, err := mgr.SyncSkills(); err != nil {
		t.Fatalf("SyncSkills after delete: %v", err)
	}

	if fileExists(installedPath) {
		t.Fatalf("expected deleted skill installation to be removed from %s", installedPath)
	}
}

func TestManagerResolveConflictKeepOursRewritesManagedState(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	configPath := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(configPath, []byte(`{
  "theme": "dark",
  "provider": {
    "api_key": "sk-initial",
    "base_url": "https://initial.example.com",
    "model": "initial-model"
  },
  "mcp": {
    "stale": {
      "command": "node",
      "args": ["stale.js"]
    }
  }
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile configPath: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithOpenCodeAdapter(configPath),
		WithEncryptionKey(make([]byte, 32)),
	)

	profileID, err := mgr.CreateProfile(
		"managed",
		`{"api_key":"sk-managed","base_url":"https://managed.example.com","model":"managed-model"}`,
		map[string]bool{"opencode": true},
	)
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if _, err := mgr.ActivateProfile(profileID); err != nil {
		t.Fatalf("ActivateProfile: %v", err)
	}

	if _, err := mgr.CreateMCPServer(
		"github",
		"npx",
		`["-y","@modelcontextprotocol/server-github"]`,
		`{"GITHUB_TOKEN":"secret"}`,
		map[string]bool{"opencode": true},
	); err != nil {
		t.Fatalf("CreateMCPServer: %v", err)
	}
	if _, err := mgr.SyncMCPServers(); err != nil {
		t.Fatalf("SyncMCPServers: %v", err)
	}

	initialHash, err := FileHash(configPath)
	if err != nil {
		t.Fatalf("FileHash initial: %v", err)
	}
	if err := db.UpsertSyncState("opencode", configPath, initialHash, time.Now().UTC(), "outbound"); err != nil {
		t.Fatalf("UpsertSyncState seed: %v", err)
	}

	if err := os.WriteFile(configPath, []byte(`{
  "theme": "dark",
  "provider": {
    "api_key": "sk-external",
    "base_url": "https://external.example.com",
    "model": "external-model"
  },
  "mcp": {
    "external": {
      "command": "python",
      "args": ["external.py"]
    }
  }
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile external config: %v", err)
	}

	status, err := mgr.GetSyncStatus()
	if err != nil {
		t.Fatalf("GetSyncStatus before resolve: %v", err)
	}
	if status.ChangesCount != 1 {
		t.Fatalf("status.ChangesCount before resolve = %d, want 1", status.ChangesCount)
	}
	if status.Changes[0].FilePath != configPath {
		t.Fatalf("status.Changes[0].FilePath = %q, want %q", status.Changes[0].FilePath, configPath)
	}

	if err := mgr.ResolveConflict("opencode", configPath, "keep_ours"); err != nil {
		t.Fatalf("ResolveConflict keep_ours: %v", err)
	}

	status, err = mgr.GetSyncStatus()
	if err != nil {
		t.Fatalf("GetSyncStatus after resolve: %v", err)
	}
	if status.ChangesCount != 0 {
		t.Fatalf("status.ChangesCount after resolve = %d, want 0", status.ChangesCount)
	}

	updated := readJSONFileMapForOpenCodeTest(t, configPath)
	if updated["theme"] != "dark" {
		t.Fatalf("theme = %v, want dark", updated["theme"])
	}

	provider, ok := updated["provider"].(map[string]any)
	if !ok {
		t.Fatalf("provider missing or invalid type")
	}
	if provider["api_key"] != "sk-managed" {
		t.Fatalf("provider.api_key = %v, want sk-managed", provider["api_key"])
	}
	if provider["base_url"] != "https://managed.example.com" {
		t.Fatalf("provider.base_url = %v, want managed URL", provider["base_url"])
	}
	if provider["model"] != "managed-model" {
		t.Fatalf("provider.model = %v, want managed-model", provider["model"])
	}

	mcp, ok := updated["mcp"].(map[string]any)
	if !ok {
		t.Fatalf("mcp missing or invalid type")
	}
	if len(mcp) != 1 {
		t.Fatalf("len(mcp) = %d, want 1", len(mcp))
	}
	if _, ok := mcp["github"]; !ok {
		t.Fatalf("mcp entries = %+v, want github only", mcp)
	}
	if _, ok := mcp["external"]; ok {
		t.Fatalf("mcp entries = %+v, should not contain external", mcp)
	}

	state, err := db.GetSyncState("opencode", configPath)
	if err != nil {
		t.Fatalf("GetSyncState after resolve: %v", err)
	}
	if state == nil {
		t.Fatalf("GetSyncState after resolve = nil")
	}
	if state.LastSyncDir != "outbound" {
		t.Fatalf("state.LastSyncDir = %q, want outbound", state.LastSyncDir)
	}
	currentHash, err := FileHash(configPath)
	if err != nil {
		t.Fatalf("FileHash after resolve: %v", err)
	}
	if state.LastHash != currentHash {
		t.Fatalf("state.LastHash = %q, want %q", state.LastHash, currentHash)
	}
}

func TestManagerResolveConflictKeepOursDoesNotClobberUnrelatedCodexMCPState(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	codexDir := t.TempDir()
	authPath := filepath.Join(codexDir, "auth.json")
	configPath := filepath.Join(codexDir, "config.toml")

	if err := os.WriteFile(authPath, []byte("{\n  \"api_key\": \"sk-old\"\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile authPath: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(""), 0o644); err != nil {
		t.Fatalf("WriteFile configPath: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithCodexAdapter(codexDir),
		WithEncryptionKey(make([]byte, 32)),
	)

	profileID, err := mgr.CreateProfile(
		"managed",
		`{"api_key":"sk-managed","base_url":"https://managed.example.com","model":"managed-model"}`,
		map[string]bool{"codex": true},
	)
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if _, err := mgr.ActivateProfile(profileID); err != nil {
		t.Fatalf("ActivateProfile: %v", err)
	}

	authHash, err := FileHash(authPath)
	if err != nil {
		t.Fatalf("FileHash authPath: %v", err)
	}
	if err := db.UpsertSyncState("codex", authPath, authHash, time.Now().UTC(), "outbound"); err != nil {
		t.Fatalf("UpsertSyncState authPath: %v", err)
	}

	if err := os.WriteFile(authPath, []byte("{\n  \"api_key\": \"sk-external\"\n}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile external authPath: %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`[mcp_servers.external]
command = "python"
args = ["external.py"]
`), 0o644); err != nil {
		t.Fatalf("WriteFile external configPath: %v", err)
	}

	if err := mgr.ResolveConflict("codex", authPath, "keep_ours"); err != nil {
		t.Fatalf("ResolveConflict keep_ours: %v", err)
	}

	authJSON := readJSONFileMapForOpenCodeTest(t, authPath)
	if authJSON["api_key"] != "sk-managed" {
		t.Fatalf("auth.api_key = %v, want sk-managed", authJSON["api_key"])
	}

	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile configPath: %v", err)
	}
	configText := string(configBytes)
	if !strings.Contains(configText, "external.py") {
		t.Fatalf("config.toml = %s, want external MCP entry preserved", configText)
	}
}

func TestManagerActivateProfileRejectsExternalConflict(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	claudeDir := t.TempDir()
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := writeJSON(settingsPath, map[string]any{}); err != nil {
		t.Fatalf("writeJSON settings: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithClaudeAdapter(claudeDir, filepath.Join(claudeDir, ".claude.json")),
		WithEncryptionKey(make([]byte, 32)),
	)

	oldProfileID, err := mgr.CreateProfile(
		"old",
		`{"api_key":"sk-old","base_url":"https://old.example.com","model":"old-model"}`,
		map[string]bool{"claude": true},
	)
	if err != nil {
		t.Fatalf("CreateProfile old: %v", err)
	}
	if _, err := mgr.ActivateProfile(oldProfileID); err != nil {
		t.Fatalf("ActivateProfile old: %v", err)
	}

	newProfileID, err := mgr.CreateProfile(
		"new",
		`{"api_key":"sk-new","base_url":"https://new.example.com","model":"new-model"}`,
		map[string]bool{"claude": true},
	)
	if err != nil {
		t.Fatalf("CreateProfile new: %v", err)
	}

	external := map[string]any{
		"env": map[string]any{
			"ANTHROPIC_API_KEY": "sk-external",
		},
	}
	if err := writeJSON(settingsPath, external); err != nil {
		t.Fatalf("writeJSON external settings: %v", err)
	}

	if _, err := mgr.ActivateProfile(newProfileID); err == nil {
		t.Fatalf("ActivateProfile new error = nil, want conflict")
	} else if !strings.Contains(err.Error(), "sync conflict") {
		t.Fatalf("ActivateProfile new error = %q, want sync conflict", err)
	}

	var got map[string]any
	if err := readJSON(settingsPath, &got); err != nil {
		t.Fatalf("readJSON settings after conflict: %v", err)
	}
	env, ok := got["env"].(map[string]any)
	if !ok {
		t.Fatalf("settings.env missing after conflict")
	}
	if env["ANTHROPIC_API_KEY"] != "sk-external" {
		t.Fatalf("env.ANTHROPIC_API_KEY = %v, want external value preserved", env["ANTHROPIC_API_KEY"])
	}

	activeCfg := activeProfileConfig(t, mgr)
	if activeCfg.APIKey != "sk-old" {
		t.Fatalf("active profile API key = %q, want %q", activeCfg.APIKey, "sk-old")
	}
}

func TestManagerTriggerInboundSyncImportsExternalProviderChanges(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	configPath := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(configPath, []byte(`{
  "theme": "dark",
  "provider": {
    "api_key": "sk-old",
    "base_url": "https://old.example.com",
    "model": "old-model"
  }
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile initial config: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithOpenCodeAdapter(configPath),
		WithEncryptionKey(make([]byte, 32)),
	)

	profileID, err := mgr.CreateProfile(
		"managed",
		`{"api_key":"sk-old","base_url":"https://old.example.com","model":"old-model"}`,
		map[string]bool{"opencode": true},
	)
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if _, err := mgr.ActivateProfile(profileID); err != nil {
		t.Fatalf("ActivateProfile: %v", err)
	}

	if err := os.WriteFile(configPath, []byte(`{
  "theme": "dark",
  "provider": {
    "api_key": "sk-external",
    "base_url": "https://external.example.com",
    "model": "external-model"
  }
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile external config: %v", err)
	}

	changes, err := mgr.TriggerInboundSync()
	if err != nil {
		t.Fatalf("TriggerInboundSync: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("len(changes) = %d, want 0 after auto-import", len(changes))
	}

	activeCfg := activeProfileConfig(t, mgr)
	if activeCfg.APIKey != "sk-external" {
		t.Fatalf("active profile API key = %q, want %q", activeCfg.APIKey, "sk-external")
	}
	if activeCfg.BaseURL != "https://external.example.com" {
		t.Fatalf("active profile base URL = %q, want external URL", activeCfg.BaseURL)
	}
	if activeCfg.Model != "external-model" {
		t.Fatalf("active profile model = %q, want external-model", activeCfg.Model)
	}
}

func TestManagerResolveConflictKeepExternalImportsExternalMCPState(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	configPath := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(configPath, []byte(`{
  "provider": {
    "api_key": "sk-initial",
    "base_url": "https://initial.example.com",
    "model": "initial-model"
  },
  "mcp": {}
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile initial config: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithOpenCodeAdapter(configPath),
		WithEncryptionKey(make([]byte, 32)),
	)

	initialHash, err := FileHash(configPath)
	if err != nil {
		t.Fatalf("FileHash initial: %v", err)
	}
	if err := db.UpsertSyncState("opencode", configPath, initialHash, time.Now().UTC(), "outbound"); err != nil {
		t.Fatalf("UpsertSyncState initial: %v", err)
	}

	if err := os.WriteFile(configPath, []byte(`{
  "provider": {
    "api_key": "sk-initial",
    "base_url": "https://initial.example.com",
    "model": "initial-model"
  },
  "mcp": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {"GITHUB_TOKEN": "secret"}
    }
  }
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile external config: %v", err)
	}

	if err := mgr.ResolveConflict("opencode", configPath, "keep_external"); err != nil {
		t.Fatalf("ResolveConflict keep_external: %v", err)
	}

	servers, err := db.ListMCPServers()
	if err != nil {
		t.Fatalf("ListMCPServers: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("len(servers) = %d, want 1", len(servers))
	}
	if servers[0].Name != "github" {
		t.Fatalf("servers[0].Name = %q, want github", servers[0].Name)
	}

	targets, err := db.GetMCPServerTargets(servers[0].ID)
	if err != nil {
		t.Fatalf("GetMCPServerTargets: %v", err)
	}
	if !targets["opencode"] {
		t.Fatalf("targets = %+v, want opencode target enabled", targets)
	}

	state, err := db.GetSyncState("opencode", configPath)
	if err != nil {
		t.Fatalf("GetSyncState: %v", err)
	}
	if state == nil {
		t.Fatalf("GetSyncState = nil")
	}
	currentHash, err := FileHash(configPath)
	if err != nil {
		t.Fatalf("FileHash updated: %v", err)
	}
	if state.LastHash != currentHash {
		t.Fatalf("state.LastHash = %q, want %q", state.LastHash, currentHash)
	}
}

func TestManagerRestoreBackupRefreshesStateAndCreatesSafetyBackup(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	claudeDir := t.TempDir()
	settingsPath := filepath.Join(claudeDir, "settings.json")
	if err := writeJSON(settingsPath, map[string]any{
		"env": map[string]any{
			"ANTHROPIC_API_KEY":  "sk-old",
			"ANTHROPIC_BASE_URL": "https://old.example.com",
			"ANTHROPIC_MODEL":    "old-model",
		},
	}); err != nil {
		t.Fatalf("writeJSON initial settings: %v", err)
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithClaudeAdapter(claudeDir, filepath.Join(claudeDir, ".claude.json")),
		WithEncryptionKey(make([]byte, 32)),
	)

	profileID, err := mgr.CreateProfile(
		"managed",
		`{"api_key":"sk-old","base_url":"https://old.example.com","model":"old-model"}`,
		map[string]bool{"claude": true},
	)
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if _, err := mgr.ActivateProfile(profileID); err != nil {
		t.Fatalf("ActivateProfile: %v", err)
	}

	slot, backupPath, err := mgr.backup.Backup("claude", "settings.json", settingsPath, "manual")
	if err != nil {
		t.Fatalf("Backup old settings: %v", err)
	}
	backupID, err := db.InsertBackupRecord("claude", settingsPath, backupPath, slot, "manual")
	if err != nil {
		t.Fatalf("InsertBackupRecord: %v", err)
	}

	currentConfig := `{"api_key":"sk-current","base_url":"https://current.example.com","model":"current-model"}`
	if err := mgr.UpdateProfile(profileID, "managed", currentConfig); err != nil {
		t.Fatalf("UpdateProfile current: %v", err)
	}
	if err := writeJSON(settingsPath, map[string]any{
		"env": map[string]any{
			"ANTHROPIC_API_KEY":  "sk-current",
			"ANTHROPIC_BASE_URL": "https://current.example.com",
			"ANTHROPIC_MODEL":    "current-model",
		},
	}); err != nil {
		t.Fatalf("writeJSON current settings: %v", err)
	}
	currentHash, err := FileHash(settingsPath)
	if err != nil {
		t.Fatalf("FileHash current settings: %v", err)
	}
	if err := db.UpsertSyncState("claude", settingsPath, currentHash, time.Now().UTC(), "outbound"); err != nil {
		t.Fatalf("UpsertSyncState current: %v", err)
	}

	beforeBackups, err := db.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups before restore: %v", err)
	}

	if _, err := mgr.RestoreBackup(backupID); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}

	activeCfg := activeProfileConfig(t, mgr)
	if activeCfg.APIKey != "sk-old" {
		t.Fatalf("active profile API key = %q, want %q", activeCfg.APIKey, "sk-old")
	}
	if activeCfg.BaseURL != "https://old.example.com" {
		t.Fatalf("active profile base URL = %q, want old URL", activeCfg.BaseURL)
	}
	if activeCfg.Model != "old-model" {
		t.Fatalf("active profile model = %q, want old-model", activeCfg.Model)
	}

	state, err := db.GetSyncState("claude", settingsPath)
	if err != nil {
		t.Fatalf("GetSyncState after restore: %v", err)
	}
	if state == nil {
		t.Fatalf("GetSyncState after restore = nil")
	}
	restoredHash, err := FileHash(settingsPath)
	if err != nil {
		t.Fatalf("FileHash restored settings: %v", err)
	}
	if state.LastHash != restoredHash {
		t.Fatalf("state.LastHash = %q, want %q", state.LastHash, restoredHash)
	}

	afterBackups, err := db.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups after restore: %v", err)
	}
	if len(afterBackups) != len(beforeBackups)+1 {
		t.Fatalf("len(afterBackups) = %d, want %d", len(afterBackups), len(beforeBackups)+1)
	}
}

func TestManagerBootstrapSkipsDivergentConfigs(t *testing.T) {
	t.Parallel()

	db := openManagerTestDB(t)
	adapterA := &fakeManagerAdapter{
		tool:      "a",
		installed: true,
		current: &ProviderConfig{
			APIKey:  "sk-a",
			BaseURL: "https://a",
			Model:   "model-a",
		},
	}
	adapterB := &fakeManagerAdapter{
		tool:      "b",
		installed: true,
		current: &ProviderConfig{
			APIKey:  "sk-b",
			BaseURL: "https://b",
			Model:   "model-b",
		},
	}

	mgr := NewManager(
		db,
		filepath.Join(t.TempDir(), "backups"),
		WithAdapter(adapterA),
		WithAdapter(adapterB),
		WithEncryptionKey(make([]byte, 32)),
	)

	if err := mgr.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}

	profiles, err := mgr.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("len(profiles) = %d, want 1", len(profiles))
	}

	targets, err := db.GetProfileToolTargets(profiles[0].ID)
	if err != nil {
		t.Fatalf("GetProfileToolTargets: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("len(targets) = %d, want 1", len(targets))
	}
	if _, ok := targets["a"]; !ok {
		t.Fatalf("expected tool a to be targeted, got %+v", targets)
	}
}

type fakeManagerAdapter struct {
	tool        string
	installed   bool
	current     *ProviderConfig
	failSet     error
	skillPaths  []string
	configFiles []ConfigFileInfo
}

func (a *fakeManagerAdapter) Tool() string { return a.tool }

func (a *fakeManagerAdapter) IsInstalled() bool { return a.installed }

func (a *fakeManagerAdapter) GetProviderConfig() (*ProviderConfig, error) {
	return cloneProviderConfig(a.current), nil
}

func (a *fakeManagerAdapter) SetProviderConfig(cfg *ProviderConfig) ([]AffectedFile, error) {
	if a.failSet != nil {
		return nil, a.failSet
	}
	a.current = cloneProviderConfig(cfg)
	return []AffectedFile{{Path: filepath.Join("/fake", a.tool), Tool: a.tool, Operation: "write"}}, nil
}

func (a *fakeManagerAdapter) GetMCPServers() ([]MCPServerConfig, error) { return nil, nil }

func (a *fakeManagerAdapter) SetMCPServers(servers []MCPServerConfig) ([]AffectedFile, error) {
	return nil, nil
}

func (a *fakeManagerAdapter) GetSkillPaths() []string { return a.skillPaths }

func (a *fakeManagerAdapter) ConfigFiles() []ConfigFileInfo { return a.configFiles }

func writeJSON(path string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o644)
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func activeProfileConfig(t *testing.T, mgr *Manager) *ProviderConfig {
	t.Helper()

	profiles, err := mgr.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	for _, profile := range profiles {
		if !profile.IsActive {
			continue
		}
		cfg, err := mgr.parseAndDecryptConfig(profile.Config)
		if err != nil {
			t.Fatalf("parseAndDecryptConfig: %v", err)
		}
		return cfg
	}

	t.Fatal("no active profile found")
	return nil
}
