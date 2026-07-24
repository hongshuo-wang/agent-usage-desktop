package storage

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

// CostCalcFunc is a function that calculates USD cost from token counts and per-token prices.
type CostCalcFunc func(inputTokens, outputTokens, cacheCreation, cacheRead int64, prices [4]float64) float64

type PricingMatch struct {
	Key       string
	Prices    [4]float64
	MatchKind string
}

// PriceUnpricedUsage binds unpriced usage records to the latest pricing snapshot
// available at the time of each usage event.
func (d *DB) PriceUnpricedUsage(calcFn CostCalcFunc) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id, model, timestamp, input_tokens, output_tokens,
		cache_creation_input_tokens, cache_read_input_tokens
		FROM usage_records WHERE pricing_status = 'unpriced'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type rec struct {
		id                    int64
		model                 string
		timestamp             time.Time
		input, output, cc, cr int64
	}
	var recs []rec
	for rows.Next() {
		var r rec
		var timestamp any
		if err := rows.Scan(&r.id, &r.model, &timestamp, &r.input, &r.output, &r.cc, &r.cr); err != nil {
			return err
		}
		r.timestamp, err = parseDatabaseTime(timestamp)
		if err != nil {
			return fmt.Errorf("parse timestamp for usage %d: %w", r.id, err)
		}
		recs = append(recs, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	if len(recs) == 0 {
		return tx.Commit()
	}
	snapshots, err := loadPricingSnapshots(tx)
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`UPDATE usage_records SET cost_usd=?, resolved_pricing_key=?,
		pricing_snapshot_id=?, pricing_status='priced', priced_at=?
		WHERE id=? AND pricing_status='unpriced'`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	type snapshotPricing struct {
		prices map[string][4]float64
	}
	snapshotCache := make(map[int64]snapshotPricing)
	pricedAt := time.Now().UTC()
	for _, r := range recs {
		snapshotID, ok := latestSnapshotAtOrBefore(snapshots, r.timestamp)
		if !ok {
			continue
		}

		snapshot, ok := snapshotCache[snapshotID]
		if !ok {
			prices, err := loadSnapshotPricing(tx, snapshotID)
			if err != nil {
				return fmt.Errorf("load pricing snapshot %d: %w", snapshotID, err)
			}
			snapshot = snapshotPricing{prices: prices}
			snapshotCache[snapshotID] = snapshot
		}

		match, ok := matchPricing(r.model, snapshot.prices)
		if !ok {
			continue
		}
		cost := calcFn(r.input, r.output, r.cc, r.cr, match.Prices)
		if _, err := stmt.Exec(cost, match.Key, snapshotID, pricedAt, r.id); err != nil {
			return fmt.Errorf("price usage %d: %w", r.id, err)
		}
	}

	return tx.Commit()
}

type pricingSnapshot struct {
	id       int64
	syncedAt time.Time
}

func loadPricingSnapshots(q queryer) ([]pricingSnapshot, error) {
	rows, err := q.Query(`SELECT id, synced_at FROM pricing_snapshots`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []pricingSnapshot
	for rows.Next() {
		var snapshot pricingSnapshot
		var syncedAt any
		if err := rows.Scan(&snapshot.id, &syncedAt); err != nil {
			return nil, err
		}
		snapshot.syncedAt, err = parseDatabaseTime(syncedAt)
		if err != nil {
			return nil, fmt.Errorf("parse pricing snapshot %d synced_at: %w", snapshot.id, err)
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].syncedAt.Equal(snapshots[j].syncedAt) {
			return snapshots[i].id < snapshots[j].id
		}
		return snapshots[i].syncedAt.Before(snapshots[j].syncedAt)
	})
	return snapshots, nil
}

func latestSnapshotAtOrBefore(snapshots []pricingSnapshot, eventTime time.Time) (int64, bool) {
	index := sort.Search(len(snapshots), func(i int) bool {
		return snapshots[i].syncedAt.After(eventTime)
	})
	if index == 0 {
		return 0, false
	}
	return snapshots[index-1].id, true
}

func parseDatabaseTime(value any) (time.Time, error) {
	switch value := value.(type) {
	case time.Time:
		return value, nil
	case []byte:
		return parseDatabaseTime(string(value))
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return parsed, nil
		}
		fields := strings.Fields(value)
		if len(fields) >= 3 {
			if parsed, err := time.Parse("2006-01-02 15:04:05 -0700", strings.Join(fields[:3], " ")); err == nil {
				return parsed, nil
			}
		}
		for _, layout := range []string{"2006-01-02 15:04:05.999999999", "2006-01-02"} {
			if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
				return parsed, nil
			}
		}
		return time.Time{}, fmt.Errorf("unsupported timestamp %q", value)
	default:
		return time.Time{}, fmt.Errorf("unsupported timestamp type %T", value)
	}
}

type queryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func loadSnapshotPricing(q queryer, snapshotID int64) (map[string][4]float64, error) {
	rows, err := q.Query(`SELECT model, input_cost_per_token, output_cost_per_token,
		cache_read_input_token_cost, cache_creation_input_token_cost
		FROM pricing_snapshot_entries WHERE snapshot_id=?`, snapshotID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prices := make(map[string][4]float64)
	for rows.Next() {
		var model string
		var values [4]float64
		if err := rows.Scan(&model, &values[0], &values[1], &values[2], &values[3]); err != nil {
			return nil, err
		}
		prices[model] = values
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return prices, nil
}

func matchPricing(model string, allPrices map[string][4]float64) (PricingMatch, bool) {
	if p, ok := allPrices[model]; ok {
		return PricingMatch{Key: model, Prices: p, MatchKind: "direct"}, true
	}

	for _, prefix := range []string{"anthropic/", "openai/", "deepseek/", "gemini/", "google/", "mistral/", "cohere/", "azure_ai/"} {
		key := prefix + model
		if p, ok := allPrices[key]; ok {
			return PricingMatch{Key: key, Prices: p, MatchKind: "provider_prefix"}, true
		}
	}

	norm := func(s string) string {
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, "/", ".")
		return s
	}

	modelNorm := norm(model)
	modelNormDash := strings.NewReplacer("4.6", "4-6", "4.5", "4-5", "3.5", "3-5", "5.4", "5-4").Replace(modelNorm)

	var best PricingMatch
	var bestScore int
	found := false
	for k := range allPrices {
		kNorm := norm(k)
		for _, mn := range []string{modelNorm, modelNormDash} {
			matchKind := "fuzzy"
			score := 10000 - len(k)
			if kNorm == mn {
				matchKind = "normalized"
				score += 100000
			} else if !strings.HasSuffix(kNorm, "."+mn) {
				continue
			}

			if !found || score > bestScore || (score == bestScore && k < best.Key) {
				best = PricingMatch{Key: k, Prices: allPrices[k], MatchKind: matchKind}
				bestScore = score
				found = true
			}
		}
	}
	if found {
		return best, true
	}
	return PricingMatch{}, false
}
