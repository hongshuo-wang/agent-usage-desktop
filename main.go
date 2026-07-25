package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/collector"
	"github.com/hongshuo-wang/agent-usage-desktop/internal/config"
	"github.com/hongshuo-wang/agent-usage-desktop/internal/pricing"
	"github.com/hongshuo-wang/agent-usage-desktop/internal/server"
	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type collectorEntry struct {
	name string
	c    collector.Collector
	cfg  config.CollectorConfig
}

// runInitialCollection scans all enabled sources before the first pricing
// sync, so the initial LiteLLM snapshot can price historical events.
func runInitialCollection(entries []collectorEntry, sync func()) {
	for _, ce := range entries {
		if !ce.cfg.Enabled {
			continue
		}
		log.Printf("scanning %s sessions...", ce.name)
		if err := ce.c.Scan(); err != nil {
			log.Printf("%s scan: %v", ce.name, err)
		}
	}
	sync()
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("agent-usage-desktop %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	configPath := flag.String("config", "", "path to config file")
	portFlag := flag.Int("port", 0, "override server port")
	flag.Parse()

	resolvedConfigPath := config.ResolveConfigPath(*configPath)
	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	if *portFlag > 0 {
		cfg.Server.Port = *portFlag
	}

	db, err := storage.Open(cfg.Storage.Path)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	defer db.Close()

	// Check if version changed — if so, reset scan state to force full re-scan
	// (needed when prompt counting logic or other parsing changes)
	lastVer, _ := db.GetMeta("version")
	if lastVer != "" && lastVer != version {
		log.Printf("version changed (%s -> %s), resetting scan state for full re-scan", lastVer, version)
		if err := db.ResetScanState(); err != nil {
			log.Printf("reset scan state: %v", err)
		}
	}
	db.SetMeta("version", version)

	// Start web server first so health check is immediately available.
	// Data initialization (pricing sync, collector scan) runs in the background.
	addr := fmt.Sprintf("%s:%d", cfg.Server.BindAddress, cfg.Server.Port)
	srv := server.New(db, addr, server.WithConfigPath(resolvedConfigPath))
	go func() {
		log.Fatal(srv.Start())
	}()
	log.Printf("server listening on %s", addr)

	// Background: scan collectors, sync pricing, then start periodic loops.
	go func() {
		collectors := []collectorEntry{
			{"Claude Code", collector.NewClaudeCollector(db, cfg.Collectors.Claude.Paths), cfg.Collectors.Claude},
			{"Codex", collector.NewCodexCollector(db, cfg.Collectors.Codex.Paths), cfg.Collectors.Codex},
			{"OpenClaw", collector.NewOpenClawCollector(db, cfg.Collectors.OpenClaw.Paths), cfg.Collectors.OpenClaw},
			{"OpenCode", collector.NewOpenCodeCollector(db, cfg.Collectors.OpenCode.Paths), cfg.Collectors.OpenCode},
		}
		log.Println("scanning historical sessions before initial pricing sync...")
		runInitialCollection(collectors, func() {
			log.Println("syncing pricing data...")
			syncAndPriceUsage(db)
		})

		for _, ce := range collectors {
			if !ce.cfg.Enabled {
				continue
			}
			go func(ce collectorEntry) {
				ticker := time.NewTicker(ce.cfg.ScanInterval)
				for range ticker.C {
					ce.c.Scan()
					priceUnpricedUsage(db)
				}
			}(ce)
		}

		// Periodic pricing sync
		ticker := time.NewTicker(cfg.Pricing.SyncInterval)
		for range ticker.C {
			syncAndPriceUsage(db)
		}
	}()

	// Block forever (server runs in its own goroutine)
	select {}
}

func syncAndPriceUsage(db *storage.DB) {
	if err := pricing.Sync(db); err != nil {
		log.Printf("pricing sync failed: %v; pricing unpriced usage from existing historical snapshots", err)
	}
	if err := db.PriceUnpricedUsageWithHistoricalFallback(pricing.CalcCost); err != nil {
		log.Printf("price unpriced usage with historical fallback: %v", err)
	}
}

func priceUnpricedUsage(db *storage.DB) {
	if err := db.PriceUnpricedUsage(pricing.CalcCost); err != nil {
		log.Printf("price unpriced usage: %v", err)
	}
}
