package storage

import "testing"

func TestResolvePricingIsDeterministicAcrossMapOrder(t *testing.T) {
	firstKey := "acme/claude-sonnet-4-6"
	secondKey := "beta/claude-sonnet-4-6"
	firstPrices := [4]float64{0.001, 0.002, 0.003, 0.004}
	secondPrices := [4]float64{0.005, 0.006, 0.007, 0.008}

	pricesInOrder := make(map[string][4]float64)
	pricesInOrder[firstKey] = firstPrices
	pricesInOrder[secondKey] = secondPrices

	pricesInReverseOrder := make(map[string][4]float64)
	pricesInReverseOrder[secondKey] = secondPrices
	pricesInReverseOrder[firstKey] = firstPrices

	for name, prices := range map[string]map[string][4]float64{
		"forward insertion": pricesInOrder,
		"reverse insertion": pricesInReverseOrder,
	} {
		t.Run(name, func(t *testing.T) {
			match, ok := matchPricing("claude-sonnet-4.6", prices)
			if !ok {
				t.Fatal("expected fuzzy match")
			}
			if match.Key != firstKey {
				t.Fatalf("matched key = %q, want lexicographically first key %q", match.Key, firstKey)
			}
			if match.Prices != firstPrices {
				t.Fatalf("matched prices = %v, want %v", match.Prices, firstPrices)
			}
			if match.MatchKind != "fuzzy" {
				t.Fatalf("match kind = %q, want fuzzy", match.MatchKind)
			}
		})
	}
}

func TestResolvePricingReturnsCanonicalKey(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		prices    map[string][4]float64
		want      PricingMatch
		wantMatch bool
	}{
		{
			name:  "direct key",
			model: "claude-opus-4-6",
			prices: map[string][4]float64{
				"claude-opus-4-6": {0.015, 0.075, 0.0015, 0.01875},
			},
			want: PricingMatch{
				Key:       "claude-opus-4-6",
				Prices:    [4]float64{0.015, 0.075, 0.0015, 0.01875},
				MatchKind: "direct",
			},
			wantMatch: true,
		},
		{
			name:  "provider prefix",
			model: "deepseek-r1",
			prices: map[string][4]float64{
				"deepseek/deepseek-r1": {0.001, 0.002, 0.0005, 0.001},
			},
			want: PricingMatch{
				Key:       "deepseek/deepseek-r1",
				Prices:    [4]float64{0.001, 0.002, 0.0005, 0.001},
				MatchKind: "provider_prefix",
			},
			wantMatch: true,
		},
		{
			name:  "version normalization",
			model: "claude-sonnet-4.6",
			prices: map[string][4]float64{
				"claude-sonnet-4-6": {0.003, 0.015, 0.001, 0.004},
			},
			want: PricingMatch{
				Key:       "claude-sonnet-4-6",
				Prices:    [4]float64{0.003, 0.015, 0.001, 0.004},
				MatchKind: "normalized",
			},
			wantMatch: true,
		},
		{
			name:  "no match",
			model: "totally-unknown-model",
			prices: map[string][4]float64{
				"claude-opus-4-6": {0.015, 0.075, 0, 0},
			},
			want:      PricingMatch{},
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := matchPricing(tt.model, tt.prices)
			if ok != tt.wantMatch {
				t.Fatalf("matched = %t, want %t (result: %+v)", ok, tt.wantMatch, got)
			}
			if got != tt.want {
				t.Fatalf("match = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestResolvePricingRejectsAmbiguousPartialMatch(t *testing.T) {
	prices := map[string][4]float64{
		"vendor/not-deepseek-r1-preview": {0.001, 0.002, 0.003, 0.004},
	}

	match, ok := matchPricing("deepseek-r1", prices)
	if ok {
		t.Fatalf("unexpected substring match: %+v", match)
	}
	if match != (PricingMatch{}) {
		t.Fatalf("no-match result = %+v, want zero value", match)
	}
}
