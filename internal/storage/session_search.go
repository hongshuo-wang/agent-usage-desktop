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

type sessionKey struct {
	source    string
	sessionID string
}

type sessionQueryer interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
}

// SearchSessions returns source-qualified summaries ordered by recent activity.
func (d *DB) SearchSessions(query SessionQuery) ([]SessionSummary, error) {
	return searchSessions(d.db, query)
}

func searchSessions(db sessionQueryer, query SessionQuery) ([]SessionSummary, error) {
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

	rows, err := db.Query(`WITH activity AS (
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

	result := make([]SessionSummary, len(identities))
	indices := make(map[sessionKey]int, len(identities))
	for index, identity := range identities {
		result[index] = SessionSummary{
			Source: identity.source, SessionID: identity.sessionID,
			StartTime: identity.firstActivity, LastActivity: identity.lastActivity,
			Models: []string{}, CoverageStatus: "stats_only", SourceStatus: "stats_only",
		}
		indices[sessionKey{identity.source, identity.sessionID}] = index
	}
	if len(result) == 0 {
		return result, nil
	}
	if err := loadSessionMetadata(db, query, identities, indices, result); err != nil {
		return nil, err
	}
	if err := loadSessionUsage(db, query, identities, indices, result); err != nil {
		return nil, err
	}
	if err := loadSessionPrompts(db, query, identities, indices, result); err != nil {
		return nil, err
	}
	if err := loadSessionEventSummaries(db, query, identities, indices, result); err != nil {
		return nil, err
	}
	if err := loadSessionSourceSummaries(db, identities, indices, result); err != nil {
		return nil, err
	}
	for index := range result {
		summary := &result[index]
		sort.Strings(summary.Models)
		summary.TotalTokens = summary.InputTokens + summary.OutputTokens + summary.CacheRead + summary.CacheCreate
		if summary.Title == "" {
			summary.Title = summary.Project
		}
		if summary.Title == "" {
			summary.Title = summary.CWD
		}
		if summary.Title == "" {
			summary.Title = summary.SessionID
		}
	}
	return result, nil
}

func selectedSessionsCTE(identities []sessionIdentity) (string, []interface{}) {
	values := make([]string, len(identities))
	args := make([]interface{}, 0, len(identities)*2)
	for index, identity := range identities {
		values[index] = "(?,?)"
		args = append(args, identity.source, identity.sessionID)
	}
	return `WITH selected(source, session_id) AS (VALUES ` + strings.Join(values, ",") + `)`, args
}

func loadSessionMetadata(db sessionQueryer, query SessionQuery, identities []sessionIdentity, indices map[sessionKey]int, result []SessionSummary) error {
	cte, args := selectedSessionsCTE(identities)
	args = append(args, query.From, query.To)
	rows, err := db.Query(cte+`, usage_metadata AS (
		SELECT u.source, u.session_id, COALESCE(MAX(u.project), '') AS project,
			COALESCE(MAX(u.git_branch), '') AS git_branch
		FROM usage_records u JOIN selected x ON x.source=u.source AND x.session_id=u.session_id
		WHERE u.timestamp BETWEEN ? AND ? GROUP BY u.source, u.session_id
	)
	SELECT x.source, x.session_id, COALESCE(s.project,''), COALESCE(s.cwd,''),
		COALESCE(s.git_branch,''), s.start_time,
		COALESCE(um.project,''), COALESCE(um.git_branch,'')
	FROM selected x
	LEFT JOIN sessions s ON s.source=x.source AND s.session_id=x.session_id
	LEFT JOIN usage_metadata um ON um.source=x.source AND um.session_id=x.session_id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var source, sessionID, project, cwd, branch, usageProject, usageBranch string
		var start sql.NullString
		if err := rows.Scan(&source, &sessionID, &project, &cwd, &branch, &start, &usageProject, &usageBranch); err != nil {
			return err
		}
		summary := &result[indices[sessionKey{source, sessionID}]]
		summary.Project, summary.CWD, summary.GitBranch = project, cwd, branch
		if summary.Project == "" {
			summary.Project = usageProject
		}
		if summary.GitBranch == "" {
			summary.GitBranch = usageBranch
		}
		if start.Valid {
			summary.StartTime = start.String
		}
	}
	return rows.Err()
}

func loadSessionUsage(db sessionQueryer, query SessionQuery, identities []sessionIdentity, indices map[sessionKey]int, result []SessionSummary) error {
	cte, args := selectedSessionsCTE(identities)
	args = append(args, query.From, query.To)
	modelClause := ""
	if query.Model != "" {
		modelClause = " AND u.model=?"
		args = append(args, query.Model)
	}
	rows, err := db.Query(cte+` SELECT u.source, u.session_id, u.model,
			COALESCE(SUM(u.input_tokens),0), COALESCE(SUM(u.output_tokens),0),
			COALESCE(SUM(u.cache_read_input_tokens),0), COALESCE(SUM(u.cache_creation_input_tokens),0),
			COALESCE(SUM(CASE WHEN u.pricing_status IN ('priced','legacy') THEN u.cost_usd ELSE 0 END),0),
			MAX(CASE WHEN u.pricing_status='unpriced' THEN 1 ELSE 0 END)
		FROM usage_records u JOIN selected x ON x.source=u.source AND x.session_id=u.session_id
		WHERE u.timestamp BETWEEN ? AND ?`+modelClause+`
	GROUP BY u.source, u.session_id, u.model`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var source, sessionID, model string
		var input, output, cacheRead, cacheCreate int64
		var cost float64
		var unknown int
		if err := rows.Scan(&source, &sessionID, &model, &input, &output, &cacheRead, &cacheCreate, &cost, &unknown); err != nil {
			return err
		}
		summary := &result[indices[sessionKey{source, sessionID}]]
		summary.Models = append(summary.Models, model)
		summary.InputTokens += input
		summary.OutputTokens += output
		summary.CacheRead += cacheRead
		summary.CacheCreate += cacheCreate
		summary.TotalCost += cost
		summary.UnknownPrice = summary.UnknownPrice || unknown != 0
	}
	return rows.Err()
}

func loadSessionPrompts(db sessionQueryer, query SessionQuery, identities []sessionIdentity, indices map[sessionKey]int, result []SessionSummary) error {
	cte, args := selectedSessionsCTE(identities)
	args = append(args, query.From, query.To)
	rows, err := db.Query(cte+` SELECT p.source, p.session_id, COUNT(*)
	FROM prompt_events p JOIN selected x ON x.source=p.source AND x.session_id=p.session_id
	WHERE p.timestamp BETWEEN ? AND ? GROUP BY p.source, p.session_id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var source, sessionID string
		var prompts int
		if err := rows.Scan(&source, &sessionID, &prompts); err != nil {
			return err
		}
		result[indices[sessionKey{source, sessionID}]].Prompts = prompts
	}
	return rows.Err()
}

func loadSessionEventSummaries(db sessionQueryer, query SessionQuery, identities []sessionIdentity, indices map[sessionKey]int, result []SessionSummary) error {
	cte, args := selectedSessionsCTE(identities)
	args = append(args, query.From, query.To)
	rows, err := db.Query(cte+`, event_counts AS (
		SELECT e.source, e.session_id,
			SUM(CASE WHEN e.event_type='tool_call' THEN 1 ELSE 0 END) AS tool_calls,
			SUM(CASE WHEN e.event_type='error' THEN 1 ELSE 0 END) AS errors
		FROM session_events e JOIN selected x ON x.source=e.source AND x.session_id=e.session_id
		WHERE e.timestamp BETWEEN ? AND ? GROUP BY e.source, e.session_id
	), ranked_titles AS (
		SELECT e.source, e.session_id, e.content,
			ROW_NUMBER() OVER (PARTITION BY e.source, e.session_id
				ORDER BY e.timestamp, e.raw_offset, e.raw_index, e.id) AS title_rank
		FROM session_events e JOIN selected x ON x.source=e.source AND x.session_id=e.session_id
		WHERE e.event_type='user_message' AND e.content!=''
	)
	SELECT x.source, x.session_id, COALESCE(ec.tool_calls,0), COALESCE(ec.errors,0),
		COALESCE(rt.content,'')
	FROM selected x
	LEFT JOIN event_counts ec ON ec.source=x.source AND ec.session_id=x.session_id
	LEFT JOIN ranked_titles rt ON rt.source=x.source AND rt.session_id=x.session_id AND rt.title_rank=1`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var source, sessionID, title string
		var toolCalls, errors int
		if err := rows.Scan(&source, &sessionID, &toolCalls, &errors, &title); err != nil {
			return err
		}
		summary := &result[indices[sessionKey{source, sessionID}]]
		summary.ToolCalls, summary.Errors, summary.Title = toolCalls, errors, title
	}
	return rows.Err()
}

func loadSessionSourceSummaries(db sessionQueryer, identities []sessionIdentity, indices map[sessionKey]int, result []SessionSummary) error {
	cte, args := selectedSessionsCTE(identities)
	rows, err := db.Query(cte+` SELECT ss.source, ss.session_id, COUNT(*),
		COALESCE(SUM(CASE WHEN ss.coverage_status!='complete' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ss.source_status='missing_source' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ss.source_status='rebuild_required' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ss.source_status='stale_parser' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ss.source_status NOT IN ('available','missing_source','rebuild_required','stale_parser') THEN 1 ELSE 0 END),0),
		COALESCE(SUM(ss.malformed_lines),0)
	FROM session_sources ss JOIN selected x ON x.source=ss.source AND x.session_id=ss.session_id
	GROUP BY ss.source, ss.session_id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var source, sessionID string
		var sourceCount, partialCount, missingCount, rebuildCount, staleCount, otherUnavailableCount, malformedLines int
		if err := rows.Scan(&source, &sessionID, &sourceCount, &partialCount, &missingCount,
			&rebuildCount, &staleCount, &otherUnavailableCount, &malformedLines); err != nil {
			return err
		}
		summary := &result[indices[sessionKey{source, sessionID}]]
		summary.CoverageStatus = "complete"
		summary.SourceStatus = "available"
		summary.MalformedLines = malformedLines
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
		case otherUnavailableCount > 0:
			summary.SourceStatus = "unavailable"
		}
	}
	return rows.Err()
}

func literalFTSQuery(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
