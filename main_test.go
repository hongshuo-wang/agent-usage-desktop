package main

import (
	"os"
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

// A release build reports its version as the git tag ("v2.1.0") and a local build
// as a bare number, so the re-scan gate must treat them as the same release.
func TestNormalizedVersionMatchesTagAndBareForms(t *testing.T) {
	for _, test := range []struct{ version, want string }{
		{"v2.1.0", "2.1.0"},
		{"2.1.0", "2.1.0"},
		{" v2.1.0 ", "2.1.0"},
		{"2.1.1-rc.1", "2.1.1-rc.1"},
	} {
		if got := normalizedVersion(test.version); got != test.want {
			t.Errorf("normalizedVersion(%q) = %q, want %q", test.version, got, test.want)
		}
	}
	if normalizedVersion("v2.1.0") != normalizedVersion("2.1.0") {
		t.Error("a release build and a local build of the same version must not trigger a re-scan")
	}
}

// The pipe check is the only thing standing between the parent watch and
// breaking standalone use: answer yes for anything EOF-prone and the server
// exits the moment it starts.
func TestIsPipeOnlyMatchesPipes(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer reader.Close()
	defer writer.Close()
	if !isPipe(reader) {
		t.Error("anonymous pipe not detected; the sidecar would outlive its parent")
	}

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer devNull.Close()
	if isPipe(devNull) {
		t.Errorf("%s detected as a pipe; a standalone server would exit immediately", os.DevNull)
	}
}
