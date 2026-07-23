package storage

import (
	"database/sql"
	"sort"
	"strings"
	"time"
)

// SessionQuery filters and pages session explorer summaries.
type SessionQuery struct {
	From    time.Time
	To      time.Time
	Source  string
	Model   string
	Project string
	Search  string
	Limit   int
	Offset  int
}

// SessionSummary is a range-scoped session explorer result.
type SessionSummary struct {
	Source         string   `json:"source"`
	SessionID      string   `json:"session_id"`
	Title          string   `json:"title"`
	Project        string   `json:"project"`
	CWD            string   `json:"cwd"`
	GitBranch      string   `json:"git_branch"`
	StartTime      string   `json:"start_time"`
	LastActivity   string   `json:"last_activity"`
	Models         []string `json:"models"`
	InputTokens    int64    `json:"input_tokens"`
	OutputTokens   int64    `json:"output_tokens"`
	CacheRead      int64    `json:"cache_read"`
	CacheCreate    int64    `json:"cache_create"`
	TotalTokens    int64    `json:"total_tokens"`
	TotalCost      float64  `json:"total_cost"`
	Prompts        int      `json:"prompts"`
	ToolCalls      int      `json:"tool_calls"`
	Errors         int      `json:"errors"`
	UnknownPrice   bool     `json:"unknown_price"`
	CoverageStatus string   `json:"coverage_status"`
	SourceStatus   string   `json:"source_status"`
	MalformedLines int      `json:"malformed_lines"`
}

type sessionIdentity struct {
	source        string
	sessionID     string
	firstActivity string
	lastActivity  string
}

// SearchSessions returns source-qualified summaries ordered by recent activity.
func (d *DB) SearchSessions(query SessionQuery) ([]SessionSummary, error) {
	clauses := []string{"1=1"}
	args := []interface{}{query.From, query.To, query.From, query.To, query.From, query.To}
	if query.Source != "" {
		clauses = append(clauses, "a.source=?")
		args = append(args, query.Source)
	}
	if query.Model != "" {
		clauses = append(clauses, `EXISTS (SELECT 1 FROM usage_records mu
			WHERE mu.source=a.source AND mu.session_id=a.session_id
				AND mu.timestamp BETWEEN ? AND ? AND mu.model=?)`)
		args = append(args, query.From, query.To, query.Model)
	}
	if query.Project != "" {
		clauses = append(clauses, `(EXISTS (SELECT 1 FROM sessions ps
			WHERE ps.source=a.source AND ps.session_id=a.session_id AND ps.project=?)
			OR EXISTS (SELECT 1 FROM usage_records pu
			WHERE pu.source=a.source AND pu.session_id=a.session_id
				AND pu.timestamp BETWEEN ? AND ? AND pu.project=?))`)
		args = append(args, query.Project, query.From, query.To, query.Project)
	}
	search := strings.TrimSpace(query.Search)
	if search != "" {
		clauses = append(clauses, `EXISTS (SELECT 1 FROM session_events qe
			JOIN session_events_fts ON session_events_fts.rowid=qe.id
			WHERE qe.source=a.source AND qe.session_id=a.session_id
				AND qe.timestamp BETWEEN ? AND ? AND session_events_fts MATCH ?)`)
		args = append(args, query.From, query.To, literalFTSQuery(search))
	}
	args = append(args, query.Limit, query.Offset)

	rows, err := d.db.Query(`WITH activity AS (
		SELECT source, session_id, timestamp AS ts FROM usage_records WHERE timestamp BETWEEN ? AND ?
		UNION ALL
		SELECT source, session_id, timestamp AS ts FROM prompt_events WHERE timestamp BETWEEN ? AND ?
		UNION ALL
		SELECT source, session_id, timestamp AS ts FROM session_events WHERE timestamp BETWEEN ? AND ?
	)
	SELECT a.source, a.session_id, MIN(a.ts) AS first_activity, MAX(a.ts) AS last_activity
	FROM activity a WHERE `+strings.Join(clauses, " AND ")+`
	GROUP BY a.source, a.session_id
	ORDER BY last_activity DESC, a.source, a.session_id
	LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	var identities []sessionIdentity
	for rows.Next() {
		var identity sessionIdentity
		if err := rows.Scan(&identity.source, &identity.sessionID, &identity.firstActivity, &identity.lastActivity); err != nil {
			rows.Close()
			return nil, err
		}
		identities = append(identities, identity)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	result := make([]SessionSummary, 0, len(identities))
	for _, identity := range identities {
		summary, err := d.loadSessionSummary(query, identity)
		if err != nil {
			return nil, err
		}
		result = append(result, summary)
	}
	return result, nil
}

func (d *DB) loadSessionSummary(query SessionQuery, identity sessionIdentity) (SessionSummary, error) {
	summary := SessionSummary{
		Source: identity.source, SessionID: identity.sessionID, LastActivity: identity.lastActivity,
		StartTime: identity.firstActivity, CoverageStatus: "stats_only", SourceStatus: "stats_only", Models: []string{},
	}
	var start sql.NullString
	err := d.db.QueryRow(`SELECT project, cwd, git_branch, start_time FROM sessions
		WHERE source=? AND session_id=?`, identity.source, identity.sessionID).
		Scan(&summary.Project, &summary.CWD, &summary.GitBranch, &start)
	if err != nil && err != sql.ErrNoRows {
		return SessionSummary{}, err
	}
	if start.Valid {
		summary.StartTime = start.String
	}
	if err == sql.ErrNoRows || summary.Project == "" || summary.GitBranch == "" {
		var project, branch string
		if err := d.db.QueryRow(`SELECT COALESCE(MAX(project),''), COALESCE(MAX(git_branch),'')
			FROM usage_records WHERE source=? AND session_id=? AND timestamp BETWEEN ? AND ?`,
			identity.source, identity.sessionID, query.From, query.To).Scan(&project, &branch); err != nil {
			return SessionSummary{}, err
		}
		if summary.Project == "" {
			summary.Project = project
		}
		if summary.GitBranch == "" {
			summary.GitBranch = branch
		}
	}

	usageSQL := `SELECT u.model, u.input_tokens, u.output_tokens,
		u.cache_read_input_tokens, u.cache_creation_input_tokens, u.cost_usd,
		CASE WHEN p.model IS NULL THEN 1 ELSE 0 END
		FROM usage_records u LEFT JOIN pricing p ON p.model=u.model
		WHERE u.source=? AND u.session_id=? AND u.timestamp BETWEEN ? AND ?`
	usageArgs := []interface{}{identity.source, identity.sessionID, query.From, query.To}
	if query.Model != "" {
		usageSQL += " AND u.model=?"
		usageArgs = append(usageArgs, query.Model)
	}
	usageRows, err := d.db.Query(usageSQL, usageArgs...)
	if err != nil {
		return SessionSummary{}, err
	}
	modelSet := make(map[string]struct{})
	for usageRows.Next() {
		var model string
		var input, output, cacheRead, cacheCreate int64
		var cost float64
		var unknown int
		if err := usageRows.Scan(&model, &input, &output, &cacheRead, &cacheCreate, &cost, &unknown); err != nil {
			usageRows.Close()
			return SessionSummary{}, err
		}
		modelSet[model] = struct{}{}
		summary.InputTokens += input
		summary.OutputTokens += output
		summary.CacheRead += cacheRead
		summary.CacheCreate += cacheCreate
		summary.TotalCost += cost
		summary.UnknownPrice = summary.UnknownPrice || unknown != 0
	}
	if err := usageRows.Err(); err != nil {
		usageRows.Close()
		return SessionSummary{}, err
	}
	if err := usageRows.Close(); err != nil {
		return SessionSummary{}, err
	}
	for model := range modelSet {
		summary.Models = append(summary.Models, model)
	}
	sort.Strings(summary.Models)
	summary.TotalTokens = summary.InputTokens + summary.OutputTokens + summary.CacheRead + summary.CacheCreate

	if err := d.db.QueryRow(`SELECT COUNT(*) FROM prompt_events
		WHERE source=? AND session_id=? AND timestamp BETWEEN ? AND ?`,
		identity.source, identity.sessionID, query.From, query.To).Scan(&summary.Prompts); err != nil {
		return SessionSummary{}, err
	}
	if err := d.db.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN event_type='tool_call' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN event_type='error' THEN 1 ELSE 0 END),0)
		FROM session_events WHERE source=? AND session_id=? AND timestamp BETWEEN ? AND ?`,
		identity.source, identity.sessionID, query.From, query.To).Scan(&summary.ToolCalls, &summary.Errors); err != nil {
		return SessionSummary{}, err
	}
	if err := d.db.QueryRow(`SELECT content FROM session_events
		WHERE source=? AND session_id=? AND event_type='user_message' AND content!=''
		ORDER BY timestamp, raw_offset, raw_index, id LIMIT 1`, identity.source, identity.sessionID).Scan(&summary.Title); err != nil && err != sql.ErrNoRows {
		return SessionSummary{}, err
	}
	if summary.Title == "" {
		summary.Title = summary.Project
	}
	if summary.Title == "" {
		summary.Title = summary.CWD
	}
	if summary.Title == "" {
		summary.Title = summary.SessionID
	}

	var sourceCount, partialCount, missingCount, rebuildCount, staleCount, otherUnavailableCount int
	var otherUnavailableStatus sql.NullString
	if err := d.db.QueryRow(`SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN coverage_status!='complete' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN source_status='missing_source' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN source_status='rebuild_required' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN source_status='stale_parser' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN source_status NOT IN ('available','missing_source','rebuild_required','stale_parser') THEN 1 ELSE 0 END),0),
		MAX(CASE WHEN source_status NOT IN ('available','missing_source','rebuild_required','stale_parser') THEN source_status ELSE NULL END),
		COALESCE(SUM(malformed_lines),0)
		FROM session_sources WHERE source=? AND session_id=?`, identity.source, identity.sessionID).
		Scan(&sourceCount, &partialCount, &missingCount, &rebuildCount, &staleCount, &otherUnavailableCount, &otherUnavailableStatus, &summary.MalformedLines); err != nil {
		return SessionSummary{}, err
	}
	if sourceCount > 0 {
		summary.CoverageStatus = "complete"
		summary.SourceStatus = "available"
	}
	if partialCount > 0 {
		summary.CoverageStatus = "partial"
	}
	switch {
	case missingCount > 0:
		summary.SourceStatus = "missing_source"
	case rebuildCount > 0:
		summary.SourceStatus = "rebuild_required"
	case staleCount > 0:
		summary.SourceStatus = "stale_parser"
	case otherUnavailableCount > 0 && otherUnavailableStatus.Valid:
		summary.SourceStatus = otherUnavailableStatus.String
	}
	return summary, nil
}

func literalFTSQuery(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
