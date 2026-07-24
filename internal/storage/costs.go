package storage

import "strings"

// CostCalcFunc is a function that calculates USD cost from token counts and per-token prices.
type CostCalcFunc func(inputTokens, outputTokens, cacheCreation, cacheRead int64, prices [4]float64) float64

type PricingMatch struct {
	Key       string
	Prices    [4]float64
	MatchKind string
}

// RecalcCosts recalculates costs for all usage records where cost_usd is zero,
// using fuzzy model name matching against the provided pricing map.
func (d *DB) RecalcCosts(allPrices map[string][4]float64, calcFn CostCalcFunc) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	rows, err := d.db.Query(`SELECT id, model, input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens FROM usage_records WHERE cost_usd = 0`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type rec struct {
		id                    int64
		model                 string
		input, output, cc, cr int64
	}
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.model, &r.input, &r.output, &r.cc, &r.cr); err != nil {
			return err
		}
		recs = append(recs, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	if len(recs) == 0 {
		return nil
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("UPDATE usage_records SET cost_usd=? WHERE id=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	updated := 0
	for _, r := range recs {
		match, ok := matchPricing(r.model, allPrices)
		if !ok {
			continue
		}
		cost := calcFn(r.input, r.output, r.cc, r.cr, match.Prices)
		if cost > 0 {
			if _, err := stmt.Exec(cost, r.id); err != nil {
				return err
			}
			updated++
		}
	}

	if updated > 0 {
		return tx.Commit()
	}
	return nil
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
