package storage

import (
	"database/sql"
	"time"
)

// SessionSource describes an original file indexed for session exploration.
type SessionSource struct {
	ID             int64
	Source         string
	SessionID      string
	SourceKind     string
	Path           string
	ParserVersion  string
	HeadHash       string
	FileSize       int64
	IndexedOffset  int64
	CoverageStatus string
	SourceStatus   string
	MalformedLines int
	LastError      string
	LastIndexedAt  time.Time
}

// SessionEventRecord is a normalized event with a locator back to its source file.
type SessionEventRecord struct {
	ID              int64
	SessionSourceID int64
	Source          string
	SessionID       string
	EventType       string
	SourceEventType string
	Timestamp       time.Time
	Role            string
	Content         string
	ToolName        string
	ToolCallID      string
	ToolInput       string
	ToolOutput      string
	EventStatus     string
	DurationMS      *int64
	RawOffset       int64
	RawLength       int64
	RawIndex        int
}

// UpsertSessionSource inserts or replaces source metadata keyed by path.
func (d *DB) UpsertSessionSource(source *SessionSource) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	sourceKind := source.SourceKind
	if sourceKind == "" {
		sourceKind = "jsonl"
	}
	coverageStatus := source.CoverageStatus
	if coverageStatus == "" {
		coverageStatus = "partial"
	}
	sourceStatus := source.SourceStatus
	if sourceStatus == "" {
		sourceStatus = "available"
	}
	var existingID int64
	var existingSource, existingSessionID string
	err = tx.QueryRow(`SELECT id, source, session_id FROM session_sources WHERE path=?`, source.Path).
		Scan(&existingID, &existingSource, &existingSessionID)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	if err == nil && (existingSource != source.Source || existingSessionID != source.SessionID) {
		if _, err := tx.Exec(`DELETE FROM session_events WHERE session_source_id=?`, existingID); err != nil {
			return 0, err
		}
	}
	_, err = tx.Exec(`INSERT INTO session_sources(
		source, session_id, source_kind, path, parser_version, head_hash,
		file_size, indexed_offset, coverage_status, source_status,
		malformed_lines, last_error, last_indexed_at
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(path) DO UPDATE SET
		source=excluded.source,
		session_id=excluded.session_id,
		source_kind=excluded.source_kind,
		parser_version=excluded.parser_version,
		head_hash=excluded.head_hash,
		file_size=excluded.file_size,
		indexed_offset=excluded.indexed_offset,
		coverage_status=excluded.coverage_status,
		source_status=excluded.source_status,
		malformed_lines=excluded.malformed_lines,
		last_error=excluded.last_error,
		last_indexed_at=excluded.last_indexed_at`,
		source.Source, source.SessionID, sourceKind, source.Path, source.ParserVersion,
		source.HeadHash, source.FileSize, source.IndexedOffset, coverageStatus,
		sourceStatus, source.MalformedLines, source.LastError, nullableTime(source.LastIndexedAt))
	if err != nil {
		return 0, err
	}

	var id int64
	if err := tx.QueryRow(`SELECT id FROM session_sources WHERE path=?`, source.Path).Scan(&id); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// InsertSessionEvents inserts normalized events and ignores duplicate raw locators.
func (d *DB) InsertSessionEvents(events []SessionEventRecord) error {
	if len(events) == 0 {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO session_events(
		session_source_id, source, session_id, event_type, source_event_type,
		timestamp, role, content, tool_name, tool_call_id, tool_input, tool_output,
		event_status, duration_ms, raw_offset, raw_length, raw_index
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, event := range events {
		if _, err := stmt.Exec(
			event.SessionSourceID, event.Source, event.SessionID, event.EventType,
			event.SourceEventType, nullableTime(event.Timestamp), event.Role, event.Content,
			event.ToolName, event.ToolCallID, event.ToolInput, event.ToolOutput,
			event.EventStatus, nullableInt64(event.DurationMS), event.RawOffset,
			event.RawLength, event.RawIndex,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteSourceIndex removes index metadata and events for a path, never source files.
func (d *DB) DeleteSourceIndex(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM session_sources WHERE path=?`, path); err != nil {
		return err
	}
	return tx.Commit()
}

// ClearSessionContent removes indexed content while retaining source metadata.
func (d *DB) ClearSessionContent(source, sessionID, status string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := clearSessionContentTx(tx, source, sessionID, status); err != nil {
		return err
	}
	return tx.Commit()
}

func clearSessionContentTx(tx *sql.Tx, source, sessionID, status string) error {
	if _, err := tx.Exec(`DELETE FROM session_events WHERE source=? AND session_id=?`, source, sessionID); err != nil {
		return err
	}
	_, err := tx.Exec(`UPDATE session_sources
		SET source_status=?, indexed_offset=0, coverage_status='partial'
		WHERE source=? AND session_id=?`, status, source, sessionID)
	return err
}

// MarkMissingSessionSources clears content for unseen paths and updates source availability.
func (d *DB) MarkMissingSessionSources(source string, seenPaths map[string]struct{}) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id, path FROM session_sources WHERE source=?`, source)
	if err != nil {
		return err
	}
	type sourcePath struct {
		id   int64
		path string
	}
	var paths []sourcePath
	for rows.Next() {
		var item sourcePath
		if err := rows.Scan(&item.id, &item.path); err != nil {
			rows.Close()
			return err
		}
		paths = append(paths, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, item := range paths {
		if _, seen := seenPaths[item.path]; seen {
			if _, err := tx.Exec(`UPDATE session_sources SET source_status='available' WHERE id=?`, item.id); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(`DELETE FROM session_events WHERE session_source_id=?`, item.id); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE session_sources
			SET source_status='missing_source', indexed_offset=0, coverage_status='partial'
			WHERE id=?`, item.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetSessionSourceByPath returns source metadata for a path.
func (d *DB) GetSessionSourceByPath(path string) (*SessionSource, error) {
	row := d.db.QueryRow(`SELECT id, source, session_id, source_kind, path,
		parser_version, head_hash, file_size, indexed_offset, coverage_status,
		source_status, malformed_lines, last_error, last_indexed_at
		FROM session_sources WHERE path=?`, path)
	var source SessionSource
	var lastIndexedAt sql.NullTime
	if err := row.Scan(
		&source.ID, &source.Source, &source.SessionID, &source.SourceKind, &source.Path,
		&source.ParserVersion, &source.HeadHash, &source.FileSize, &source.IndexedOffset,
		&source.CoverageStatus, &source.SourceStatus, &source.MalformedLines,
		&source.LastError, &lastIndexedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if lastIndexedAt.Valid {
		source.LastIndexedAt = lastIndexedAt.Time
	}
	return &source, nil
}

// ListSessionEvents returns a stable chronological page for a source-qualified session.
func (d *DB) ListSessionEvents(source, sessionID string, limit, offset int) ([]SessionEventRecord, error) {
	rows, err := d.db.Query(`SELECT id, session_source_id, source, session_id,
		event_type, source_event_type, timestamp, role, content, tool_name,
		tool_call_id, tool_input, tool_output, event_status, duration_ms,
		raw_offset, raw_length, raw_index
		FROM session_events WHERE source=? AND session_id=?
		ORDER BY timestamp, raw_offset, raw_index, id LIMIT ? OFFSET ?`,
		source, sessionID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []SessionEventRecord
	for rows.Next() {
		event, err := scanSessionEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

// GetSessionEvent returns one event within a source-qualified session.
func (d *DB) GetSessionEvent(source, sessionID string, eventID int64) (*SessionEventRecord, error) {
	row := d.db.QueryRow(`SELECT id, session_source_id, source, session_id,
		event_type, source_event_type, timestamp, role, content, tool_name,
		tool_call_id, tool_input, tool_output, event_status, duration_ms,
		raw_offset, raw_length, raw_index
		FROM session_events WHERE source=? AND session_id=? AND id=?`, source, sessionID, eventID)
	event, err := scanSessionEvent(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &event, nil
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanSessionEvent(row rowScanner) (SessionEventRecord, error) {
	var event SessionEventRecord
	var timestamp sql.NullTime
	var duration sql.NullInt64
	err := row.Scan(
		&event.ID, &event.SessionSourceID, &event.Source, &event.SessionID,
		&event.EventType, &event.SourceEventType, &timestamp, &event.Role, &event.Content,
		&event.ToolName, &event.ToolCallID, &event.ToolInput, &event.ToolOutput,
		&event.EventStatus, &duration, &event.RawOffset, &event.RawLength, &event.RawIndex,
	)
	if err != nil {
		return SessionEventRecord{}, err
	}
	if timestamp.Valid {
		event.Timestamp = timestamp.Time
	}
	if duration.Valid {
		event.DurationMS = &duration.Int64
	}
	return event, nil
}

func nullableTime(value time.Time) interface{} {
	if value.IsZero() {
		return nil
	}
	return value
}

func nullableInt64(value *int64) interface{} {
	if value == nil {
		return nil
	}
	return *value
}
