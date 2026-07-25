package main

import (
	"testing"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/collector"
	"github.com/hongshuo-wang/agent-usage-desktop/internal/config"
)

type orderingCollector struct {
	name  string
	order *[]string
}

func (c orderingCollector) Scan() error {
	*c.order = append(*c.order, "scan:"+c.name)
	return nil
}

func TestRunInitialCollectionSyncsAfterAllHistoricalScans(t *testing.T) {
	order := []string{}
	entries := []collectorEntry{
		{name: "first", c: orderingCollector{name: "first", order: &order}, cfg: config.CollectorConfig{Enabled: true}},
		{name: "disabled", c: orderingCollector{name: "disabled", order: &order}, cfg: config.CollectorConfig{Enabled: false}},
		{name: "second", c: orderingCollector{name: "second", order: &order}, cfg: config.CollectorConfig{Enabled: true}},
	}
	runInitialCollection(entries, func() { order = append(order, "sync") })

	want := []string{"scan:first", "scan:second", "sync"}
	if len(order) != len(want) {
		t.Fatalf("initialization order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order[%d] = %q, want %q", i, order[i], want[i])
		}
	}
}

var _ collector.Collector = orderingCollector{}
