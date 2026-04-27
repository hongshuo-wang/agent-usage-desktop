package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// Profile stores provider profile settings.
type Profile struct {
	ID        int64
	Name      string
	IsActive  bool
	Config    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type MCPServerRecord struct {
	ID        int64
	Name      string
	Command   string
	Args      string
	Env       string
	Enabled   bool
	CreatedAt time.Time
}

type SkillRecord struct {
	ID          int64
	Name        string
	SourcePath  string
	Description string
	Enabled     bool
	CreatedAt   time.Time
}

type SkillVariantRecord struct {
	ID         int64
	SkillID    int64
	SourcePath string
	OriginTool string
	CreatedAt  time.Time
}

type ToolTarget struct {
	Tool       string
	Enabled    bool
	ToolConfig string
}

type SkillTargetRecord struct {
	Tool      string
	Method    string
	Enabled   bool
	VariantID int64
}

type SyncState struct {
	Tool        string
	FilePath    string
	LastHash    string
	LastSync    time.Time
	LastSyncDir string
}

type BackupRecord struct {
	ID          int64
	Tool        string
	FilePath    string
	BackupPath  string
	Slot        int
	CreatedAt   time.Time
	TriggerType string
}

// CreateProfile inserts a new provider profile.
func (d *DB) CreateProfile(name, config string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`INSERT INTO provider_profiles(name, config)
		VALUES(?, ?)`, name, config)
	return err
}

// ListProfiles returns all provider profiles.
func (d *DB) ListProfiles() ([]Profile, error) {
	rows, err := d.db.Query(`SELECT id, name, is_active, config, created_at, updated_at
		FROM provider_profiles
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []Profile
	for rows.Next() {
		var p Profile
		if err := rows.Scan(&p.ID, &p.Name, &p.IsActive, &p.Config, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (d *DB) ActivateProfile(id int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE provider_profiles SET is_active = 0, updated_at = CURRENT_TIMESTAMP`); err != nil {
		return err
	}

	result, err := tx.Exec(`UPDATE provider_profiles SET is_active = 1, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("profile not found: %d", id)
	}

	return tx.Commit()
}

func (d *DB) DeactivateProfiles() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`UPDATE provider_profiles SET is_active = 0, updated_at = CURRENT_TIMESTAMP`)
	return err
}

func (d *DB) GetProfile(id int64) (*Profile, error) {
	var profile Profile
	err := d.db.QueryRow(`SELECT id, name, is_active, config, created_at, updated_at
		FROM provider_profiles
		WHERE id = ?`, id).Scan(
		&profile.ID,
		&profile.Name,
		&profile.IsActive,
		&profile.Config,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return &profile, nil
}

func (d *DB) UpdateProfile(id int64, name, config string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	result, err := d.db.Exec(`UPDATE provider_profiles
		SET name = ?, config = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, name, config, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("profile not found: %d", id)
	}
	return nil
}

func (d *DB) DeleteProfile(id int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	result, err := d.db.Exec(`DELETE FROM provider_profiles WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("profile not found: %d", id)
	}
	return nil
}

func (d *DB) CreateMCPServer(name, command, args, env string) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	result, err := d.db.Exec(`INSERT INTO mcp_servers(name, command, args, env, enabled)
		VALUES(?, ?, ?, ?, 1)`, name, command, args, env)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) CreateMCPServerWithID(record MCPServerRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if record.ID <= 0 {
		return fmt.Errorf("mcp server id is required")
	}

	_, err := d.db.Exec(`INSERT INTO mcp_servers(id, name, command, args, env, enabled, created_at)
		VALUES(?, ?, ?, ?, ?, ?, ?)`,
		record.ID, record.Name, record.Command, record.Args, record.Env, record.Enabled, record.CreatedAt)
	return err
}

func (d *DB) ListMCPServers() ([]MCPServerRecord, error) {
	rows, err := d.db.Query(`SELECT id, name, command, args, env, enabled, created_at
		FROM mcp_servers
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []MCPServerRecord
	for rows.Next() {
		var server MCPServerRecord
		if err := rows.Scan(&server.ID, &server.Name, &server.Command, &server.Args, &server.Env, &server.Enabled, &server.CreatedAt); err != nil {
			return nil, err
		}
		servers = append(servers, server)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return servers, nil
}

func (d *DB) GetMCPServer(id int64) (*MCPServerRecord, error) {
	var server MCPServerRecord
	err := d.db.QueryRow(`SELECT id, name, command, args, env, enabled, created_at
		FROM mcp_servers
		WHERE id = ?`, id).Scan(&server.ID, &server.Name, &server.Command, &server.Args, &server.Env, &server.Enabled, &server.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &server, nil
}

func (d *DB) UpdateMCPServer(id int64, name, command, args, env string, enabled bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	result, err := d.db.Exec(`UPDATE mcp_servers
		SET name = ?, command = ?, args = ?, env = ?, enabled = ?
		WHERE id = ?`, name, command, args, env, enabled, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("mcp server not found: %d", id)
	}
	return nil
}

func (d *DB) DeleteMCPServer(id int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	result, err := d.db.Exec(`DELETE FROM mcp_servers WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("mcp server not found: %d", id)
	}
	return nil
}

func (d *DB) CreateSkill(name, sourcePath, description string) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	result, err := d.db.Exec(`INSERT INTO skills(name, source_path, description, enabled)
		VALUES(?, ?, ?, 1)`, name, sourcePath, description)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) CreateSkillWithID(record SkillRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if record.ID <= 0 {
		return fmt.Errorf("skill id is required")
	}

	_, err := d.db.Exec(`INSERT INTO skills(id, name, source_path, description, enabled, created_at)
		VALUES(?, ?, ?, ?, ?, ?)`,
		record.ID, record.Name, record.SourcePath, record.Description, record.Enabled, record.CreatedAt)
	return err
}

func (d *DB) ListSkills() ([]SkillRecord, error) {
	rows, err := d.db.Query(`SELECT id, name, source_path, description, enabled, created_at
		FROM skills
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var skills []SkillRecord
	for rows.Next() {
		var skill SkillRecord
		if err := rows.Scan(&skill.ID, &skill.Name, &skill.SourcePath, &skill.Description, &skill.Enabled, &skill.CreatedAt); err != nil {
			return nil, err
		}
		skills = append(skills, skill)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return skills, nil
}

func (d *DB) GetSkill(id int64) (*SkillRecord, error) {
	var skill SkillRecord
	err := d.db.QueryRow(`SELECT id, name, source_path, description, enabled, created_at
		FROM skills
		WHERE id = ?`, id).Scan(&skill.ID, &skill.Name, &skill.SourcePath, &skill.Description, &skill.Enabled, &skill.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &skill, nil
}

func (d *DB) UpdateSkill(id int64, name, sourcePath, description string, enabled bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	result, err := d.db.Exec(`UPDATE skills
		SET name = ?, source_path = ?, description = ?, enabled = ?
		WHERE id = ?`, name, sourcePath, description, enabled, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("skill not found: %d", id)
	}
	return nil
}

func (d *DB) UpdateSkillWithTargets(id int64, name, sourcePath, description string, enabled bool, targets []SkillTargetRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.Exec(`UPDATE skills
		SET name = ?, source_path = ?, description = ?, enabled = ?
		WHERE id = ?`, name, sourcePath, description, enabled, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("skill not found: %d", id)
	}

	if _, err := tx.Exec(`DELETE FROM skill_targets WHERE skill_id = ?`, id); err != nil {
		return err
	}

	for _, target := range targets {
		if _, err := tx.Exec(`INSERT INTO skill_targets(skill_id, tool, method, enabled, variant_id)
			VALUES(?, ?, ?, ?, ?)`, id, target.Tool, target.Method, target.Enabled, target.VariantID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *DB) DeleteSkill(id int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	result, err := d.db.Exec(`DELETE FROM skills WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("skill not found: %d", id)
	}
	return nil
}

func (d *DB) SetProfileToolTargets(profileID int64, targets []ToolTarget) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM profile_tool_targets WHERE profile_id = ?`, profileID); err != nil {
		return err
	}

	for _, target := range targets {
		if _, err := tx.Exec(`INSERT INTO profile_tool_targets(profile_id, tool, enabled, tool_config)
			VALUES(?, ?, ?, ?)`, profileID, target.Tool, target.Enabled, target.ToolConfig); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *DB) GetProfileToolTargets(profileID int64) (map[string]ToolTarget, error) {
	rows, err := d.db.Query(`SELECT tool, enabled, tool_config
		FROM profile_tool_targets
		WHERE profile_id = ?
		ORDER BY tool`, profileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := map[string]ToolTarget{}
	for rows.Next() {
		var target ToolTarget
		if err := rows.Scan(&target.Tool, &target.Enabled, &target.ToolConfig); err != nil {
			return nil, err
		}
		targets[target.Tool] = target
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return targets, nil
}

func (d *DB) SetMCPServerTargets(serverID int64, targets map[string]bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM mcp_server_targets WHERE server_id = ?`, serverID); err != nil {
		return err
	}

	for tool, enabled := range targets {
		if _, err := tx.Exec(`INSERT INTO mcp_server_targets(server_id, tool, enabled)
			VALUES(?, ?, ?)`, serverID, tool, enabled); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *DB) GetMCPServerTargets(serverID int64) (map[string]bool, error) {
	rows, err := d.db.Query(`SELECT tool, enabled
		FROM mcp_server_targets
		WHERE server_id = ?
		ORDER BY tool`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := map[string]bool{}
	for rows.Next() {
		var tool string
		var enabled bool
		if err := rows.Scan(&tool, &enabled); err != nil {
			return nil, err
		}
		targets[tool] = enabled
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return targets, nil
}

func (d *DB) SetSkillTargets(skillID int64, targets []SkillTargetRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM skill_targets WHERE skill_id = ?`, skillID); err != nil {
		return err
	}

	for _, target := range targets {
		if _, err := tx.Exec(`INSERT INTO skill_targets(skill_id, tool, method, enabled, variant_id)
			VALUES(?, ?, ?, ?, ?)`, skillID, target.Tool, target.Method, target.Enabled, target.VariantID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (d *DB) GetSkillTargets(skillID int64) (map[string]SkillTargetRecord, error) {
	rows, err := d.db.Query(`SELECT tool, method, enabled, variant_id
		FROM skill_targets
		WHERE skill_id = ?
		ORDER BY tool`, skillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := map[string]SkillTargetRecord{}
	for rows.Next() {
		var target SkillTargetRecord
		if err := rows.Scan(&target.Tool, &target.Method, &target.Enabled, &target.VariantID); err != nil {
			return nil, err
		}
		targets[target.Tool] = target
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return targets, nil
}

func (d *DB) CreateSkillVariant(skillID int64, sourcePath, originTool string) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	result, err := d.db.Exec(`INSERT INTO skill_variants(skill_id, source_path, origin_tool)
		VALUES(?, ?, ?)`, skillID, sourcePath, originTool)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) CreateSkillVariantWithID(record SkillVariantRecord) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if record.ID <= 0 {
		return fmt.Errorf("skill variant id is required")
	}

	_, err := d.db.Exec(`INSERT INTO skill_variants(id, skill_id, source_path, origin_tool, created_at)
		VALUES(?, ?, ?, ?, ?)`,
		record.ID, record.SkillID, record.SourcePath, record.OriginTool, record.CreatedAt)
	return err
}

func (d *DB) ListSkillVariants(skillID int64) ([]SkillVariantRecord, error) {
	rows, err := d.db.Query(`SELECT id, skill_id, source_path, origin_tool, created_at
		FROM skill_variants
		WHERE skill_id = ?
		ORDER BY id`, skillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var variants []SkillVariantRecord
	for rows.Next() {
		var variant SkillVariantRecord
		if err := rows.Scan(&variant.ID, &variant.SkillID, &variant.SourcePath, &variant.OriginTool, &variant.CreatedAt); err != nil {
			return nil, err
		}
		variants = append(variants, variant)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return variants, nil
}

func (d *DB) GetSkillVariant(id int64) (*SkillVariantRecord, error) {
	var variant SkillVariantRecord
	err := d.db.QueryRow(`SELECT id, skill_id, source_path, origin_tool, created_at
		FROM skill_variants
		WHERE id = ?`, id).
		Scan(&variant.ID, &variant.SkillID, &variant.SourcePath, &variant.OriginTool, &variant.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &variant, nil
}

func (d *DB) FindSkillVariantByPath(skillID int64, sourcePath string) (*SkillVariantRecord, error) {
	var variant SkillVariantRecord
	err := d.db.QueryRow(`SELECT id, skill_id, source_path, origin_tool, created_at
		FROM skill_variants
		WHERE skill_id = ? AND source_path = ?`, skillID, sourcePath).
		Scan(&variant.ID, &variant.SkillID, &variant.SourcePath, &variant.OriginTool, &variant.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &variant, nil
}

func (d *DB) GetSyncState(tool, filePath string) (*SyncState, error) {
	var state SyncState
	err := d.db.QueryRow(`SELECT tool, file_path, last_hash, last_sync, last_sync_dir
		FROM sync_state
		WHERE tool = ? AND file_path = ?`, tool, filePath).
		Scan(&state.Tool, &state.FilePath, &state.LastHash, &state.LastSync, &state.LastSyncDir)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &state, nil
}

func (d *DB) UpsertSyncState(tool, filePath, lastHash string, lastSync time.Time, lastSyncDir string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	_, err := d.db.Exec(`INSERT INTO sync_state(tool, file_path, last_hash, last_sync, last_sync_dir)
		VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(tool, file_path) DO UPDATE SET
			last_hash = excluded.last_hash,
			last_sync = excluded.last_sync,
			last_sync_dir = excluded.last_sync_dir`,
		tool, filePath, lastHash, lastSync, lastSyncDir)
	return err
}

func (d *DB) InsertBackupRecord(tool, filePath, backupPath string, slot int, triggerType string) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	result, err := d.db.Exec(`INSERT INTO config_backups(tool, file_path, backup_path, slot, trigger_type)
		VALUES(?, ?, ?, ?, ?)`, tool, filePath, backupPath, slot, triggerType)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (d *DB) ListBackups() ([]BackupRecord, error) {
	rows, err := d.db.Query(`SELECT id, tool, file_path, backup_path, slot, created_at, trigger_type
		FROM config_backups
		ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []BackupRecord
	for rows.Next() {
		var backup BackupRecord
		if err := rows.Scan(&backup.ID, &backup.Tool, &backup.FilePath, &backup.BackupPath, &backup.Slot, &backup.CreatedAt, &backup.TriggerType); err != nil {
			return nil, err
		}
		backups = append(backups, backup)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return backups, nil
}

func (d *DB) GetBackupByID(id int64) (*BackupRecord, error) {
	var backup BackupRecord
	err := d.db.QueryRow(`SELECT id, tool, file_path, backup_path, slot, created_at, trigger_type
		FROM config_backups
		WHERE id = ?`, id).Scan(
		&backup.ID, &backup.Tool, &backup.FilePath, &backup.BackupPath, &backup.Slot, &backup.CreatedAt, &backup.TriggerType,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &backup, nil
}
