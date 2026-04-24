package storage

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a SQLite database connection with a mutex for safe concurrent access.
type DB struct {
	db *sql.DB
	mu sync.Mutex
}

// UsageRecord represents a single API call's token usage and cost.
type UsageRecord struct {
	ID                       int64
	Source                   string // "claude" or "codex"
	SessionID                string
	Model                    string
	InputTokens              int64
	OutputTokens             int64
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
	ReasoningOutputTokens    int64
	CostUSD                  float64
	Timestamp                time.Time
	Project                  string
	GitBranch                string
}

// SessionRecord represents metadata for a coding agent session.
type SessionRecord struct {
	ID        int64
	Source    string
	SessionID string
	Project   string
	CWD       string
	Version   string
	GitBranch string
	StartTime time.Time
	Prompts   int
}

// PromptEvent represents a single user prompt with its timestamp.
type PromptEvent struct {
	Source    string
	SessionID string
	Timestamp time.Time
}

// Open creates or opens a SQLite database at the given path, enables WAL mode,
// and runs schema migrations.
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		return nil, err
	}
	return &DB{db: db}, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error { return d.db.Close() }

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS usage_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			session_id TEXT NOT NULL,
			model TEXT NOT NULL,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			cache_creation_input_tokens INTEGER DEFAULT 0,
			cache_read_input_tokens INTEGER DEFAULT 0,
			reasoning_output_tokens INTEGER DEFAULT 0,
			cost_usd REAL DEFAULT 0,
			timestamp DATETIME NOT NULL,
			project TEXT DEFAULT '',
			git_branch TEXT DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_usage_timestamp ON usage_records(timestamp);
		CREATE INDEX IF NOT EXISTS idx_usage_session ON usage_records(session_id);
		CREATE INDEX IF NOT EXISTS idx_usage_source ON usage_records(source);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_dedup ON usage_records(session_id, model, timestamp, input_tokens, output_tokens);

		CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			session_id TEXT NOT NULL UNIQUE,
			project TEXT DEFAULT '',
			cwd TEXT DEFAULT '',
			version TEXT DEFAULT '',
			git_branch TEXT DEFAULT '',
			start_time DATETIME,
			prompts INTEGER DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS prompt_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			session_id TEXT NOT NULL,
			timestamp DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_prompt_timestamp ON prompt_events(timestamp);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_prompt_dedup ON prompt_events(session_id, timestamp);

		CREATE TABLE IF NOT EXISTS file_state (
			path TEXT PRIMARY KEY,
			size INTEGER DEFAULT 0,
			last_offset INTEGER DEFAULT 0,
			scan_context TEXT DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS pricing (
			model TEXT PRIMARY KEY,
			input_cost_per_token REAL DEFAULT 0,
			output_cost_per_token REAL DEFAULT 0,
			cache_read_input_token_cost REAL DEFAULT 0,
			cache_creation_input_token_cost REAL DEFAULT 0,
			updated_at DATETIME
		);

		CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT DEFAULT ''
		);

		DELETE FROM usage_records WHERE model = '<synthetic>';
		DELETE FROM usage_records WHERE model = 'delivery-mirror';
	`)
	if err != nil {
		return err
	}

	// Add scan_context column to file_state for existing DBs (idempotent).
	db.Exec("ALTER TABLE file_state ADD COLUMN scan_context TEXT DEFAULT ''")

	// Versioned migrations: each runs once, tracked via meta table.
	migrations := []struct {
		id  string
		sql string
	}{
		{
			"001_fix_opencode_input_tokens", `
				DELETE FROM usage_records WHERE source = 'opencode';
				DELETE FROM file_state WHERE path LIKE '%opencode%';
				DELETE FROM sessions WHERE source = 'opencode';
			`,
		},
		{
			"002_input_tokens_non_overlapping", `
				DELETE FROM usage_records;
				DELETE FROM file_state;
				DELETE FROM sessions;
			`,
		},
		{
			"003_prompt_events_rescan", `
				DELETE FROM usage_records;
				DELETE FROM file_state;
				DELETE FROM sessions;
				DELETE FROM prompt_events;
			`,
		},
		{
			"004_file_state_scan_context", `
				DELETE FROM meta WHERE key LIKE 'file_scan_context:%';
				DELETE FROM file_state;
			`,
		},
		{
			"005_config_manager", `
				CREATE TABLE IF NOT EXISTS provider_profiles (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL UNIQUE,
					is_active INTEGER NOT NULL DEFAULT 0,
					config TEXT NOT NULL DEFAULT '',
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS profile_tool_targets (
					profile_id INTEGER NOT NULL,
					tool TEXT NOT NULL,
					enabled INTEGER NOT NULL DEFAULT 1,
					tool_config TEXT NOT NULL DEFAULT '',
					PRIMARY KEY (profile_id, tool),
					FOREIGN KEY (profile_id) REFERENCES provider_profiles(id) ON DELETE CASCADE
				);

				CREATE TABLE IF NOT EXISTS mcp_servers (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL UNIQUE,
					command TEXT NOT NULL,
					args TEXT NOT NULL DEFAULT '',
					env TEXT NOT NULL DEFAULT '',
					enabled INTEGER NOT NULL DEFAULT 1,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS mcp_server_targets (
					server_id INTEGER NOT NULL,
					tool TEXT NOT NULL,
					enabled INTEGER NOT NULL DEFAULT 1,
					PRIMARY KEY (server_id, tool),
					FOREIGN KEY (server_id) REFERENCES mcp_servers(id) ON DELETE CASCADE
				);

				CREATE TABLE IF NOT EXISTS skills (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL UNIQUE,
					source_path TEXT NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					enabled INTEGER NOT NULL DEFAULT 1,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS skill_targets (
					skill_id INTEGER NOT NULL,
					tool TEXT NOT NULL,
					method TEXT NOT NULL DEFAULT 'symlink',
					enabled INTEGER NOT NULL DEFAULT 1,
					PRIMARY KEY (skill_id, tool),
					FOREIGN KEY (skill_id) REFERENCES skills(id) ON DELETE CASCADE
				);

				CREATE TABLE IF NOT EXISTS config_backups (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					tool TEXT NOT NULL,
					file_path TEXT NOT NULL,
					backup_path TEXT NOT NULL,
					slot INTEGER NOT NULL DEFAULT 0,
					created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
					trigger_type TEXT NOT NULL DEFAULT ''
				);

				CREATE TABLE IF NOT EXISTS sync_state (
					tool TEXT NOT NULL,
					file_path TEXT NOT NULL,
					last_hash TEXT NOT NULL DEFAULT '',
					last_sync DATETIME,
					last_sync_dir TEXT NOT NULL DEFAULT '',
					PRIMARY KEY (tool, file_path)
				);
			`,
		},
	}
	for _, m := range migrations {
		var done string
		db.QueryRow("SELECT value FROM meta WHERE key=?", "migration_"+m.id).Scan(&done)
		if done == "done" {
			continue
		}
		if _, err := db.Exec(m.sql); err != nil {
			return fmt.Errorf("migration %s: %w", m.id, err)
		}
		db.Exec(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
			"migration_"+m.id, "done")
	}
	return nil
}
