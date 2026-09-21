package storage

import (
	"database/sql"
	"strconv"
	"time"
)

// staleScanFloor keeps a transient gap (a closed lid, a long build) from being
// reported as a stalled collector. The window widens with the user's own scan
// interval, and never shrinks below this.
const staleScanFloor = 2 * time.Hour

// CollectionIndexStatus summarizes the durable local session index state.
type CollectionIndexStatus struct {
	Status               string     `json:"status"`
	LastIndexedAt        *time.Time `json:"last_indexed_at"`
	SourceCount          int        `json:"source_count"`
	FileCount            int        `json:"file_count"`
	CompleteFiles        int        `json:"complete_files"`
	PartialFiles         int        `json:"partial_files"`
	MissingFiles         int        `json:"missing_files"`
	RebuildRequiredFiles int        `json:"rebuild_required_files"`
	StaleParserFiles     int        `json:"stale_parser_files"`
	MalformedLines       int        `json:"malformed_lines"`
	LastScanAt           *time.Time `json:"last_scan_at"`
	Stale                bool       `json:"stale"`
}

// GetCollectionIndexStatus reports only state persisted by the local indexer.
func (d *DB) GetCollectionIndexStatus() (*CollectionIndexStatus, error) {
	status := &CollectionIndexStatus{}
	var unavailableFiles int
	err := d.db.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT source),
		COALESCE(SUM(CASE WHEN coverage_status='complete' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN coverage_status!='complete' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN source_status='missing_source' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN source_status='rebuild_required' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN source_status='stale_parser' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN source_status NOT IN ('available','missing_source','rebuild_required','stale_parser') THEN 1 ELSE 0 END),0),
		COALESCE(SUM(malformed_lines),0)
		FROM session_sources`).Scan(
		&status.FileCount, &status.SourceCount, &status.CompleteFiles, &status.PartialFiles,
		&status.MissingFiles, &status.RebuildRequiredFiles, &status.StaleParserFiles,
		&unavailableFiles, &status.MalformedLines,
	)
	if err != nil {
		return nil, err
	}
	if err := d.readScanHeartbeat(status); err != nil {
		return nil, err
	}

	if status.FileCount == 0 {
		var usageExists bool
		if err := d.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM usage_records LIMIT 1)`).Scan(&usageExists); err != nil {
			return nil, err
		}
		if usageExists {
			status.Status = "stats_only"
		} else {
			status.Status = "empty"
		}
		return status, nil
	}
	var lastIndexedAt sql.NullTime
	err = d.db.QueryRow(`SELECT last_indexed_at FROM session_sources
		WHERE last_indexed_at IS NOT NULL ORDER BY last_indexed_at DESC LIMIT 1`).Scan(&lastIndexedAt)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if lastIndexedAt.Valid {
		status.LastIndexedAt = &lastIndexedAt.Time
	}

	switch {
	case status.MissingFiles > 0:
		status.Status = "missing_source"
	case status.RebuildRequiredFiles > 0:
		status.Status = "rebuild_required"
	case status.StaleParserFiles > 0:
		status.Status = "stale_parser"
	case status.PartialFiles > 0 || status.MalformedLines > 0 || unavailableFiles > 0:
		status.Status = "partial"
	default:
		status.Status = "available"
	}
	// A specific fault is a better answer than "the scan loop is behind", so the
	// heartbeat only speaks up when nothing else is wrong.
	if status.Stale && status.Status == "available" {
		status.Status = "stale"
	}
	return status, nil
}

// readScanHeartbeat reports how fresh the collector liveness signal is. An
// absent or unparseable heartbeat means unknown, which is not the same as stale:
// databases written before the heartbeat existed must not raise a false alarm.
func (d *DB) readScanHeartbeat(status *CollectionIndexStatus) error {
	raw, err := d.GetMeta("last_scan_at")
	if err != nil || raw == "" {
		return err
	}
	lastScan, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil
	}
	status.LastScanAt = &lastScan

	tolerance := staleScanFloor
	if intervalRaw, err := d.GetMeta("scan_interval_seconds"); err == nil {
		if seconds, err := strconv.Atoi(intervalRaw); err == nil && seconds > 0 {
			if scaled := 3 * time.Duration(seconds) * time.Second; scaled > tolerance {
				tolerance = scaled
			}
		}
	}
	status.Stale = time.Since(lastScan) > tolerance
	return nil
}
