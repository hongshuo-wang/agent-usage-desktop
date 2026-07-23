package storage

import (
	"fmt"
	"time"
)

// UsageBreakdown is one grouped usage row for the overview.
type UsageBreakdown struct {
	Key          string  `json:"key"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalCost    float64 `json:"total_cost"`
	Sessions     int     `json:"sessions"`
	Calls        int     `json:"calls"`
	CacheHitRate float64 `json:"cache_hit_rate"`
	UnknownPrice bool    `json:"unknown_price"`
}

// GetUsageBreakdown groups range-scoped usage by one predefined dimension.
func (d *DB) GetUsageBreakdown(from, to time.Time, source, dimension string) ([]UsageBreakdown, error) {
	var dimensionExpression string
	switch dimension {
	case "source":
		dimensionExpression = "u.source"
	case "model":
		dimensionExpression = "u.model"
	case "project":
		dimensionExpression = "u.project"
	default:
		return nil, fmt.Errorf("invalid breakdown dimension %q", dimension)
	}

	filter := ""
	args := []interface{}{from, to}
	if source != "" {
		filter = " AND u.source=?"
		args = append(args, source)
	}
	rows, err := d.db.Query(`WITH filtered AS (
		SELECT `+dimensionExpression+` AS key, u.source, u.session_id,
			u.input_tokens, u.output_tokens, u.cache_read_input_tokens,
			u.cache_creation_input_tokens, u.cost_usd,
			CASE WHEN p.model IS NULL THEN 1 ELSE 0 END AS unknown_price
		FROM usage_records u LEFT JOIN pricing p ON p.model=u.model
		WHERE u.timestamp BETWEEN ? AND ?`+filter+`
	), aggregates AS (
		SELECT key,
			COALESCE(SUM(input_tokens+output_tokens+cache_read_input_tokens+cache_creation_input_tokens),0) AS total_tokens,
			COALESCE(SUM(cost_usd),0) AS total_cost,
			COUNT(*) AS calls,
			CASE WHEN SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens)>0
				THEN CAST(SUM(cache_read_input_tokens) AS REAL)/SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens)
				ELSE 0 END AS cache_hit_rate,
			MAX(unknown_price) AS unknown_price
		FROM filtered GROUP BY key
	), session_counts AS (
		SELECT key, COUNT(*) AS sessions FROM (
			SELECT DISTINCT key, source, session_id FROM filtered
		) GROUP BY key
	)
	SELECT a.key, a.total_tokens, a.total_cost, s.sessions, a.calls,
		a.cache_hit_rate, a.unknown_price
	FROM aggregates a JOIN session_counts s ON s.key=a.key
	ORDER BY a.total_tokens DESC, a.key`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]UsageBreakdown, 0)
	for rows.Next() {
		var item UsageBreakdown
		var unknownPrice int
		if err := rows.Scan(
			&item.Key, &item.TotalTokens, &item.TotalCost, &item.Sessions,
			&item.Calls, &item.CacheHitRate, &unknownPrice,
		); err != nil {
			return nil, err
		}
		item.UnknownPrice = unknownPrice != 0
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
