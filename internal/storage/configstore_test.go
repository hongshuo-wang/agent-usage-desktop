package storage

import (
	"testing"
	"time"
)

func TestConfigManagerMigration(t *testing.T) {
	db := tempDB(t)

	requiredTables := []string{
		"provider_profiles",
		"profile_tool_targets",
		"mcp_servers",
		"mcp_server_targets",
		"skills",
		"skill_targets",
		"config_backups",
		"sync_state",
	}

	for _, table := range requiredTables {
		var name string
		err := db.db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("required table %s missing: %v", table, err)
		}
	}

	rows, err := db.db.Query(`PRAGMA table_info(config_backups)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(config_backups): %v", err)
	}
	defer rows.Close()

	var hasTriggerType bool
	var hasTrigger bool
	var slotType string
	for rows.Next() {
		var cid int
		var name, columnType string
		var notnull int
		var dfltValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notnull, &dfltValue, &pk); err != nil {
			t.Fatalf("scan table_info(config_backups): %v", err)
		}
		if name == "trigger_type" {
			hasTriggerType = true
		}
		if name == "trigger" {
			hasTrigger = true
		}
		if name == "slot" {
			slotType = columnType
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error table_info(config_backups): %v", err)
	}
	if !hasTriggerType {
		t.Fatal("expected config_backups.trigger_type column to exist")
	}
	if hasTrigger {
		t.Fatal("expected config_backups.trigger column to not exist")
	}
	if slotType != "INTEGER" {
		t.Fatalf("expected config_backups.slot type INTEGER, got %q", slotType)
	}

	if err := db.CreateProfile("test-profile", `{"provider":"openai"}`); err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}

	profiles, err := db.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}
	if profiles[0].Name != "test-profile" {
		t.Fatalf("expected profile name test-profile, got %q", profiles[0].Name)
	}
}

func TestMCPServerCRUD(t *testing.T) {
	db := tempDB(t)

	id, err := db.CreateMCPServer("local-mcp", "/usr/bin/mcp", "--port 8080", `{"PATH":"/usr/bin"}`)
	if err != nil {
		t.Fatalf("CreateMCPServer: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	servers, err := db.ListMCPServers()
	if err != nil {
		t.Fatalf("ListMCPServers: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 mcp server, got %d", len(servers))
	}
	if servers[0].ID != id || !servers[0].Enabled {
		t.Fatalf("unexpected server row: %+v", servers[0])
	}

	server, err := db.GetMCPServer(id)
	if err != nil {
		t.Fatalf("GetMCPServer: %v", err)
	}
	if server == nil {
		t.Fatal("expected mcp server, got nil")
	}
	if server.Name != "local-mcp" || server.Command != "/usr/bin/mcp" {
		t.Fatalf("unexpected server data: %+v", *server)
	}

	if err := db.UpdateMCPServer(id, "remote-mcp", "/opt/mcp", "--stdio", `{"DEBUG":"1"}`, false); err != nil {
		t.Fatalf("UpdateMCPServer: %v", err)
	}

	server, err = db.GetMCPServer(id)
	if err != nil {
		t.Fatalf("GetMCPServer updated: %v", err)
	}
	if server == nil {
		t.Fatal("expected updated mcp server, got nil")
	}
	if server.Name != "remote-mcp" || server.Command != "/opt/mcp" || server.Enabled {
		t.Fatalf("unexpected updated server data: %+v", *server)
	}

	if err := db.DeleteMCPServer(id); err != nil {
		t.Fatalf("DeleteMCPServer: %v", err)
	}

	servers, err = db.ListMCPServers()
	if err != nil {
		t.Fatalf("ListMCPServers after delete: %v", err)
	}
	if len(servers) != 0 {
		t.Fatalf("expected empty list after delete, got %d", len(servers))
	}

	if err := db.UpdateMCPServer(id, "missing-mcp", "/missing", "", "", true); err == nil {
		t.Fatal("expected error updating missing mcp server")
	}
	if err := db.DeleteMCPServer(id); err == nil {
		t.Fatal("expected error deleting missing mcp server")
	}
}

func TestSkillCRUD(t *testing.T) {
	db := tempDB(t)

	id, err := db.CreateSkill("find-skills", "/tmp/find-skills", "discover skills")
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}

	skills, err := db.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].ID != id || !skills[0].Enabled {
		t.Fatalf("unexpected skill row: %+v", skills[0])
	}

	skill, err := db.GetSkill(id)
	if err != nil {
		t.Fatalf("GetSkill: %v", err)
	}
	if skill == nil {
		t.Fatal("expected skill, got nil")
	}
	if skill.Name != "find-skills" || skill.SourcePath != "/tmp/find-skills" {
		t.Fatalf("unexpected skill data: %+v", *skill)
	}

	if err := db.UpdateSkill(id, "find-skills-v2", "/tmp/find-skills-v2", "discover and install skills", false); err != nil {
		t.Fatalf("UpdateSkill: %v", err)
	}

	skill, err = db.GetSkill(id)
	if err != nil {
		t.Fatalf("GetSkill updated: %v", err)
	}
	if skill == nil {
		t.Fatal("expected updated skill, got nil")
	}
	if skill.Name != "find-skills-v2" || skill.SourcePath != "/tmp/find-skills-v2" || skill.Enabled {
		t.Fatalf("unexpected updated skill data: %+v", *skill)
	}

	if err := db.DeleteSkill(id); err != nil {
		t.Fatalf("DeleteSkill: %v", err)
	}

	skills, err = db.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills after delete: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("expected empty list after delete, got %d", len(skills))
	}

	if err := db.UpdateSkill(id, "missing-skill", "/missing", "", true); err == nil {
		t.Fatal("expected error updating missing skill")
	}
	if err := db.DeleteSkill(id); err == nil {
		t.Fatal("expected error deleting missing skill")
	}
}

func TestCreateMCPServerWithID(t *testing.T) {
	db := tempDB(t)

	record := MCPServerRecord{
		ID:        42,
		Name:      "restored-mcp",
		Command:   "/bin/restored",
		Args:      `["--stdio"]`,
		Env:       `{"DEBUG":"1"}`,
		Enabled:   false,
		CreatedAt: time.Date(2026, 4, 23, 8, 0, 0, 0, time.UTC),
	}

	if err := db.CreateMCPServerWithID(record); err != nil {
		t.Fatalf("CreateMCPServerWithID: %v", err)
	}

	stored, err := db.GetMCPServer(record.ID)
	if err != nil {
		t.Fatalf("GetMCPServer: %v", err)
	}
	if stored == nil {
		t.Fatal("expected restored server, got nil")
	}
	if stored.ID != record.ID || stored.Name != record.Name || stored.Command != record.Command || stored.Enabled != record.Enabled {
		t.Fatalf("unexpected restored server: %+v", *stored)
	}
}

func TestCreateSkillWithID(t *testing.T) {
	db := tempDB(t)

	record := SkillRecord{
		ID:          77,
		Name:        "restored-skill",
		SourcePath:  "/tmp/restored-skill",
		Description: "restored",
		Enabled:     false,
		CreatedAt:   time.Date(2026, 4, 23, 8, 30, 0, 0, time.UTC),
	}

	if err := db.CreateSkillWithID(record); err != nil {
		t.Fatalf("CreateSkillWithID: %v", err)
	}

	stored, err := db.GetSkill(record.ID)
	if err != nil {
		t.Fatalf("GetSkill: %v", err)
	}
	if stored == nil {
		t.Fatal("expected restored skill, got nil")
	}
	if stored.ID != record.ID || stored.Name != record.Name || stored.SourcePath != record.SourcePath || stored.Enabled != record.Enabled {
		t.Fatalf("unexpected restored skill: %+v", *stored)
	}
}

func TestProfileToolTargets(t *testing.T) {
	db := tempDB(t)
	profileID := createTestProfile(t, db, "profile-a")

	if err := db.SetProfileToolTargets(profileID, []ToolTarget{
		{Tool: "codex", Enabled: true, ToolConfig: `{"model":"gpt-5.4"}`},
		{Tool: "claude", Enabled: false, ToolConfig: `{"model":"sonnet"}`},
	}); err != nil {
		t.Fatalf("SetProfileToolTargets: %v", err)
	}

	targets, err := db.GetProfileToolTargets(profileID)
	if err != nil {
		t.Fatalf("GetProfileToolTargets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if !targets["codex"].Enabled || targets["claude"].Enabled {
		t.Fatalf("unexpected targets: %+v", targets)
	}

	if err := db.SetProfileToolTargets(profileID, []ToolTarget{
		{Tool: "codex", Enabled: false, ToolConfig: `{"model":"gpt-5.5"}`},
	}); err != nil {
		t.Fatalf("SetProfileToolTargets replace: %v", err)
	}

	targets, err = db.GetProfileToolTargets(profileID)
	if err != nil {
		t.Fatalf("GetProfileToolTargets replaced: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target after replace, got %d", len(targets))
	}
	if targets["codex"].Enabled || targets["codex"].ToolConfig != `{"model":"gpt-5.5"}` {
		t.Fatalf("unexpected replaced targets: %+v", targets)
	}
}

func TestProfileToolTargetsRollbackOnFailure(t *testing.T) {
	db := tempDB(t)
	profileID := createTestProfile(t, db, "profile-rollback")

	original := []ToolTarget{
		{Tool: "codex", Enabled: true, ToolConfig: `{"model":"gpt-5.4"}`},
		{Tool: "claude", Enabled: false, ToolConfig: `{"model":"sonnet"}`},
	}
	if err := db.SetProfileToolTargets(profileID, original); err != nil {
		t.Fatalf("SetProfileToolTargets seed: %v", err)
	}

	if err := db.SetProfileToolTargets(profileID, []ToolTarget{
		{Tool: "codex", Enabled: false, ToolConfig: `{"model":"gpt-5.5"}`},
		{Tool: "codex", Enabled: true, ToolConfig: `{"model":"gpt-5.6"}`},
	}); err == nil {
		t.Fatal("expected SetProfileToolTargets to fail on duplicate tool")
	}

	targets, err := db.GetProfileToolTargets(profileID)
	if err != nil {
		t.Fatalf("GetProfileToolTargets after rollback: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected original 2 targets after rollback, got %d", len(targets))
	}
	if !targets["codex"].Enabled || targets["codex"].ToolConfig != `{"model":"gpt-5.4"}` {
		t.Fatalf("unexpected codex target after rollback: %+v", targets["codex"])
	}
	if targets["claude"].Enabled || targets["claude"].ToolConfig != `{"model":"sonnet"}` {
		t.Fatalf("unexpected claude target after rollback: %+v", targets["claude"])
	}
}

func TestMCPServerTargets(t *testing.T) {
	db := tempDB(t)
	serverID, err := db.CreateMCPServer("mcp-targets", "/bin/mcp", "", "")
	if err != nil {
		t.Fatalf("CreateMCPServer: %v", err)
	}

	if err := db.SetMCPServerTargets(serverID, map[string]bool{
		"codex":  true,
		"claude": false,
	}); err != nil {
		t.Fatalf("SetMCPServerTargets: %v", err)
	}

	targets, err := db.GetMCPServerTargets(serverID)
	if err != nil {
		t.Fatalf("GetMCPServerTargets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if !targets["codex"] || targets["claude"] {
		t.Fatalf("unexpected targets: %+v", targets)
	}

	if err := db.SetMCPServerTargets(serverID, map[string]bool{"codex": false}); err != nil {
		t.Fatalf("SetMCPServerTargets replace: %v", err)
	}
	targets, err = db.GetMCPServerTargets(serverID)
	if err != nil {
		t.Fatalf("GetMCPServerTargets replaced: %v", err)
	}
	if len(targets) != 1 || targets["codex"] {
		t.Fatalf("unexpected replaced targets: %+v", targets)
	}
}

func TestMCPServerTargetsRollbackOnFailure(t *testing.T) {
	db := tempDB(t)
	serverID, err := db.CreateMCPServer("mcp-targets-rollback", "/bin/mcp", "", "")
	if err != nil {
		t.Fatalf("CreateMCPServer: %v", err)
	}

	original := map[string]bool{
		"codex":  true,
		"claude": false,
	}
	if err := db.SetMCPServerTargets(serverID, original); err != nil {
		t.Fatalf("SetMCPServerTargets seed: %v", err)
	}

	if err := db.SetMCPServerTargets(999999, map[string]bool{"codex": true}); err == nil {
		t.Fatal("expected SetMCPServerTargets to fail for missing server")
	}

	targets, err := db.GetMCPServerTargets(serverID)
	if err != nil {
		t.Fatalf("GetMCPServerTargets after rollback path: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected original 2 targets after failure, got %d", len(targets))
	}
	if !targets["codex"] || targets["claude"] {
		t.Fatalf("unexpected targets after failure: %+v", targets)
	}
}

func TestSkillTargets(t *testing.T) {
	db := tempDB(t)
	skillID, err := db.CreateSkill("skill-targets", "/tmp/skill-targets", "")
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	if err := db.SetSkillTargets(skillID, []SkillTargetRecord{
		{Tool: "codex", Method: "symlink", Enabled: true},
		{Tool: "claude", Method: "copy", Enabled: false},
	}); err != nil {
		t.Fatalf("SetSkillTargets: %v", err)
	}

	targets, err := db.GetSkillTargets(skillID)
	if err != nil {
		t.Fatalf("GetSkillTargets: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets["codex"].Method != "symlink" || targets["claude"].Enabled {
		t.Fatalf("unexpected targets: %+v", targets)
	}

	if err := db.SetSkillTargets(skillID, []SkillTargetRecord{
		{Tool: "codex", Method: "copy", Enabled: false},
	}); err != nil {
		t.Fatalf("SetSkillTargets replace: %v", err)
	}

	targets, err = db.GetSkillTargets(skillID)
	if err != nil {
		t.Fatalf("GetSkillTargets replaced: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target after replace, got %d", len(targets))
	}
	if targets["codex"].Method != "copy" || targets["codex"].Enabled {
		t.Fatalf("unexpected replaced targets: %+v", targets)
	}
}

func TestSkillTargetsRollbackOnFailure(t *testing.T) {
	db := tempDB(t)
	skillID, err := db.CreateSkill("skill-targets-rollback", "/tmp/skill-targets-rollback", "")
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	original := []SkillTargetRecord{
		{Tool: "codex", Method: "symlink", Enabled: true},
		{Tool: "claude", Method: "copy", Enabled: false},
	}
	if err := db.SetSkillTargets(skillID, original); err != nil {
		t.Fatalf("SetSkillTargets seed: %v", err)
	}

	if err := db.SetSkillTargets(skillID, []SkillTargetRecord{
		{Tool: "codex", Method: "copy", Enabled: true},
		{Tool: "codex", Method: "symlink", Enabled: false},
	}); err == nil {
		t.Fatal("expected SetSkillTargets to fail on duplicate tool")
	}

	targets, err := db.GetSkillTargets(skillID)
	if err != nil {
		t.Fatalf("GetSkillTargets after rollback: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected original 2 targets after rollback, got %d", len(targets))
	}
	if targets["codex"].Method != "symlink" || !targets["codex"].Enabled {
		t.Fatalf("unexpected codex target after rollback: %+v", targets["codex"])
	}
	if targets["claude"].Method != "copy" || targets["claude"].Enabled {
		t.Fatalf("unexpected claude target after rollback: %+v", targets["claude"])
	}
}

func TestSyncState(t *testing.T) {
	db := tempDB(t)

	state, err := db.GetSyncState("codex", "/tmp/tool.json")
	if err != nil {
		t.Fatalf("GetSyncState empty: %v", err)
	}
	if state != nil {
		t.Fatalf("expected nil state for missing row, got %+v", state)
	}

	ts1 := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	if err := db.UpsertSyncState("codex", "/tmp/tool.json", "hash-1", ts1, "push"); err != nil {
		t.Fatalf("UpsertSyncState insert: %v", err)
	}

	state, err = db.GetSyncState("codex", "/tmp/tool.json")
	if err != nil {
		t.Fatalf("GetSyncState after insert: %v", err)
	}
	if state == nil || state.LastHash != "hash-1" || state.LastSyncDir != "push" || !state.LastSync.Equal(ts1) {
		t.Fatalf("unexpected inserted sync state: %+v", state)
	}

	ts2 := time.Date(2026, 4, 2, 11, 0, 0, 0, time.UTC)
	if err := db.UpsertSyncState("codex", "/tmp/tool.json", "hash-2", ts2, "pull"); err != nil {
		t.Fatalf("UpsertSyncState update: %v", err)
	}

	state, err = db.GetSyncState("codex", "/tmp/tool.json")
	if err != nil {
		t.Fatalf("GetSyncState after update: %v", err)
	}
	if state == nil || state.LastHash != "hash-2" || state.LastSyncDir != "pull" || !state.LastSync.Equal(ts2) {
		t.Fatalf("unexpected updated sync state: %+v", state)
	}
}

func TestBackupRecords(t *testing.T) {
	db := tempDB(t)

	id1, err := db.InsertBackupRecord("codex", "/tmp/codex.json", "/tmp/backup-1.json", 0, "auto")
	if err != nil {
		t.Fatalf("InsertBackupRecord #1: %v", err)
	}
	id2, err := db.InsertBackupRecord("codex", "/tmp/codex.json", "/tmp/backup-2.json", 1, "manual")
	if err != nil {
		t.Fatalf("InsertBackupRecord #2: %v", err)
	}
	if id1 == 0 || id2 == 0 {
		t.Fatalf("expected non-zero backup ids, got %d and %d", id1, id2)
	}

	backups, err := db.ListBackups()
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 2 {
		t.Fatalf("expected 2 backups, got %d", len(backups))
	}
	if backups[0].ID != id2 || backups[1].ID != id1 {
		t.Fatalf("expected id-desc order [%d, %d], got [%d, %d]", id2, id1, backups[0].ID, backups[1].ID)
	}

	backup, err := db.GetBackupByID(id1)
	if err != nil {
		t.Fatalf("GetBackupByID: %v", err)
	}
	if backup == nil {
		t.Fatal("expected backup row, got nil")
	}
	if backup.TriggerType != "auto" || backup.FilePath != "/tmp/codex.json" {
		t.Fatalf("unexpected backup data: %+v", *backup)
	}

	backup, err = db.GetBackupByID(999999)
	if err != nil {
		t.Fatalf("GetBackupByID missing: %v", err)
	}
	if backup != nil {
		t.Fatalf("expected nil for missing backup, got %+v", backup)
	}
}

func TestActivateProfileSingleActive(t *testing.T) {
	db := tempDB(t)

	profile1 := createTestProfile(t, db, "profile-1")
	profile2 := createTestProfile(t, db, "profile-2")

	if err := db.ActivateProfile(profile1); err != nil {
		t.Fatalf("ActivateProfile profile1: %v", err)
	}
	assertSingleActiveProfile(t, db, profile1)

	if err := db.ActivateProfile(profile2); err != nil {
		t.Fatalf("ActivateProfile profile2: %v", err)
	}
	assertSingleActiveProfile(t, db, profile2)

	if err := db.ActivateProfile(999999); err == nil {
		t.Fatal("expected error activating missing profile")
	}
	assertSingleActiveProfile(t, db, profile2)
}

func TestProfileCRUD(t *testing.T) {
	db := tempDB(t)

	profileID := createTestProfile(t, db, "profile-crud")

	profile, err := db.GetProfile(profileID)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if profile == nil {
		t.Fatal("expected profile, got nil")
	}
	if profile.Name != "profile-crud" {
		t.Fatalf("profile.Name = %q, want %q", profile.Name, "profile-crud")
	}

	if err := db.UpdateProfile(profileID, "profile-updated", `{"api_key":"enc:value"}`); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	profile, err = db.GetProfile(profileID)
	if err != nil {
		t.Fatalf("GetProfile updated: %v", err)
	}
	if profile == nil {
		t.Fatal("expected updated profile, got nil")
	}
	if profile.Name != "profile-updated" {
		t.Fatalf("profile.Name = %q, want %q", profile.Name, "profile-updated")
	}
	if profile.Config != `{"api_key":"enc:value"}` {
		t.Fatalf("profile.Config = %q, want %q", profile.Config, `{"api_key":"enc:value"}`)
	}

	if err := db.DeleteProfile(profileID); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}

	profile, err = db.GetProfile(profileID)
	if err != nil {
		t.Fatalf("GetProfile deleted: %v", err)
	}
	if profile != nil {
		t.Fatalf("expected nil profile after delete, got %+v", profile)
	}

	if err := db.UpdateProfile(profileID, "missing", ""); err == nil {
		t.Fatal("expected error updating missing profile")
	}
	if err := db.DeleteProfile(profileID); err == nil {
		t.Fatal("expected error deleting missing profile")
	}
}

func TestProfileDeactivateAll(t *testing.T) {
	db := tempDB(t)

	profile1 := createTestProfile(t, db, "profile-deactivate-1")
	profile2 := createTestProfile(t, db, "profile-deactivate-2")

	if err := db.ActivateProfile(profile1); err != nil {
		t.Fatalf("ActivateProfile profile1: %v", err)
	}
	if err := db.DeactivateProfiles(); err != nil {
		t.Fatalf("DeactivateProfiles: %v", err)
	}

	profiles, err := db.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	for _, profile := range profiles {
		if profile.IsActive {
			t.Fatalf("expected no active profile, found %d", profile.ID)
		}
	}

	if err := db.ActivateProfile(profile2); err != nil {
		t.Fatalf("ActivateProfile profile2: %v", err)
	}
	assertSingleActiveProfile(t, db, profile2)
}

func createTestProfile(t *testing.T, db *DB, name string) int64 {
	t.Helper()

	if err := db.CreateProfile(name, `{"provider":"test"}`); err != nil {
		t.Fatalf("CreateProfile(%s): %v", name, err)
	}

	profiles, err := db.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	for _, profile := range profiles {
		if profile.Name == name {
			return profile.ID
		}
	}
	t.Fatalf("profile %s not found after create", name)
	return 0
}

func assertSingleActiveProfile(t *testing.T, db *DB, expectedID int64) {
	t.Helper()

	profiles, err := db.ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}

	var activeCount int
	var activeID int64
	for _, profile := range profiles {
		if profile.IsActive {
			activeCount++
			activeID = profile.ID
		}
	}

	if activeCount != 1 {
		t.Fatalf("expected exactly one active profile, got %d", activeCount)
	}
	if activeID != expectedID {
		t.Fatalf("expected active profile %d, got %d", expectedID, activeID)
	}
}
