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
		db.Close()
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
			"005_config_manager", `SELECT 1;`,
		},
		{
			"006_skill_variants", `SELECT 1;`,
		},
		{
			"007_session_event_index", `
				CREATE TABLE sessions_v2 (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					source TEXT NOT NULL,
					session_id TEXT NOT NULL,
					project TEXT DEFAULT '',
					cwd TEXT DEFAULT '',
					version TEXT DEFAULT '',
					git_branch TEXT DEFAULT '',
					start_time DATETIME,
					prompts INTEGER DEFAULT 0,
					UNIQUE(source, session_id)
				);

				INSERT INTO sessions_v2(id, source, session_id, project, cwd, version, git_branch, start_time, prompts)
				SELECT id, source, session_id, project, cwd, version, git_branch, start_time, prompts FROM sessions;
				DROP TABLE sessions;
				ALTER TABLE sessions_v2 RENAME TO sessions;

				DROP INDEX IF EXISTS idx_usage_dedup;
				CREATE UNIQUE INDEX idx_usage_dedup
					ON usage_records(source, session_id, model, timestamp, input_tokens, output_tokens);

				DROP INDEX IF EXISTS idx_prompt_dedup;
				CREATE UNIQUE INDEX idx_prompt_dedup
					ON prompt_events(source, session_id, timestamp);

				CREATE TABLE session_sources (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					source TEXT NOT NULL,
					session_id TEXT NOT NULL,
					source_kind TEXT NOT NULL DEFAULT 'jsonl',
					path TEXT NOT NULL UNIQUE,
					parser_version TEXT NOT NULL,
					head_hash TEXT NOT NULL DEFAULT '',
					file_size INTEGER NOT NULL DEFAULT 0,
					indexed_offset INTEGER NOT NULL DEFAULT 0,
					coverage_status TEXT NOT NULL DEFAULT 'partial',
					source_status TEXT NOT NULL DEFAULT 'available',
					malformed_lines INTEGER NOT NULL DEFAULT 0,
					last_error TEXT NOT NULL DEFAULT '',
					last_indexed_at DATETIME,
					UNIQUE(source, session_id, path)
				);
				CREATE INDEX idx_session_sources_session ON session_sources(source, session_id);
				CREATE INDEX idx_session_sources_status ON session_sources(source, source_status);

				CREATE TABLE session_events (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					session_source_id INTEGER NOT NULL,
					source TEXT NOT NULL,
					session_id TEXT NOT NULL,
					event_type TEXT NOT NULL,
					source_event_type TEXT NOT NULL DEFAULT '',
					timestamp DATETIME,
					role TEXT NOT NULL DEFAULT '',
					content TEXT NOT NULL DEFAULT '',
					tool_name TEXT NOT NULL DEFAULT '',
					tool_call_id TEXT NOT NULL DEFAULT '',
					tool_input TEXT NOT NULL DEFAULT '',
					tool_output TEXT NOT NULL DEFAULT '',
					event_status TEXT NOT NULL DEFAULT '',
					duration_ms INTEGER,
					raw_offset INTEGER NOT NULL DEFAULT 0,
					raw_length INTEGER NOT NULL DEFAULT 0,
					raw_index INTEGER NOT NULL DEFAULT 0,
					FOREIGN KEY(session_source_id) REFERENCES session_sources(id) ON DELETE CASCADE,
					UNIQUE(session_source_id, raw_offset, raw_index)
				);
				CREATE INDEX idx_session_events_session_time ON session_events(source, session_id, timestamp, id);
				CREATE INDEX idx_session_events_session_type ON session_events(source, session_id, event_type);
				CREATE INDEX idx_session_events_source_id ON session_events(session_source_id);

				CREATE VIRTUAL TABLE session_events_fts USING fts5(
					content,
					tool_name,
					tool_input,
					tool_output,
					content='session_events',
					content_rowid='id',
					tokenize='unicode61'
				);

				CREATE TRIGGER session_events_fts_insert AFTER INSERT ON session_events BEGIN
					INSERT INTO session_events_fts(rowid, content, tool_name, tool_input, tool_output)
					VALUES (new.id, new.content, new.tool_name, new.tool_input, new.tool_output);
				END;
				CREATE TRIGGER session_events_fts_delete AFTER DELETE ON session_events BEGIN
					INSERT INTO session_events_fts(session_events_fts, rowid, content, tool_name, tool_input, tool_output)
					VALUES ('delete', old.id, old.content, old.tool_name, old.tool_input, old.tool_output);
				END;
				CREATE TRIGGER session_events_fts_update AFTER UPDATE ON session_events BEGIN
					INSERT INTO session_events_fts(session_events_fts, rowid, content, tool_name, tool_input, tool_output)
					VALUES ('delete', old.id, old.content, old.tool_name, old.tool_input, old.tool_output);
					INSERT INTO session_events_fts(rowid, content, tool_name, tool_input, tool_output)
					VALUES (new.id, new.content, new.tool_name, new.tool_input, new.tool_output);
				END;
			`,
		},
		{
			"008_remove_config_management", `
				DROP TABLE IF EXISTS profile_tool_targets;
				DROP TABLE IF EXISTS mcp_server_targets;
				DROP TABLE IF EXISTS skill_targets;
				DROP TABLE IF EXISTS skill_variants;
				DROP TABLE IF EXISTS provider_profiles;
				DROP TABLE IF EXISTS mcp_servers;
				DROP TABLE IF EXISTS skills;
				DROP TABLE IF EXISTS config_backups;
				DROP TABLE IF EXISTS sync_state;
				DROP TABLE IF EXISTS skill_repo_sources;
			`,
		},
		{
			"009_pricing_ledger", `
				CREATE TABLE pricing_snapshots (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					synced_at DATETIME NOT NULL,
					source TEXT NOT NULL,
					source_revision TEXT NOT NULL
				);

				CREATE TABLE pricing_snapshot_entries (
					snapshot_id INTEGER NOT NULL,
					model TEXT NOT NULL,
					input_cost_per_token REAL NOT NULL,
					output_cost_per_token REAL NOT NULL,
					cache_read_input_token_cost REAL NOT NULL,
					cache_creation_input_token_cost REAL NOT NULL,
					PRIMARY KEY(snapshot_id, model),
					FOREIGN KEY(snapshot_id) REFERENCES pricing_snapshots(id) ON DELETE CASCADE
				);

				ALTER TABLE usage_records ADD COLUMN resolved_pricing_key TEXT NOT NULL DEFAULT '';
				ALTER TABLE usage_records ADD COLUMN pricing_snapshot_id INTEGER REFERENCES pricing_snapshots(id);
				ALTER TABLE usage_records ADD COLUMN pricing_status TEXT NOT NULL DEFAULT 'unpriced'
					CHECK(pricing_status IN ('priced', 'unpriced', 'legacy'));
				ALTER TABLE usage_records ADD COLUMN priced_at DATETIME;

				UPDATE usage_records SET pricing_status = 'legacy' WHERE cost_usd > 0;
				CREATE INDEX idx_usage_pricing_status_timestamp ON usage_records(pricing_status, timestamp);
				CREATE INDEX idx_usage_pricing_snapshot_id ON usage_records(pricing_snapshot_id);
			`,
		},
		{
			"010_semantic_prompt_rescan", `
				DELETE FROM prompt_events;
				UPDATE sessions SET prompts=0;
				DELETE FROM file_state;
			`,
		},
	}
	for _, m := range migrations {
		var done string
		err := db.QueryRow("SELECT value FROM meta WHERE key=?", "migration_"+m.id).Scan(&done)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("read migration %s marker: %w", m.id, err)
		}
		if done == "done" {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", m.id, err)
		}
		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", m.id, err)
		}
		if _, err := tx.Exec(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
			"migration_"+m.id, "done"); err != nil {
			tx.Rollback()
			return fmt.Errorf("mark migration %s: %w", m.id, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", m.id, err)
		}
	}
	return nil
}
