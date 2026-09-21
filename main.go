package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
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

// recordScanHeartbeat publishes the collector liveness signal. Without it a scan
// loop that stopped silently is indistinguishable from a machine that simply did
// no work, which is exactly how "today's usage is empty" goes unnoticed for days.
// scan_interval_seconds lets readers scale the staleness window with the user's
// configured cadence instead of guessing.
func recordScanHeartbeat(db *storage.DB, entries []collectorEntry) {
	longest := time.Duration(0)
	for _, ce := range entries {
		if ce.cfg.Enabled && ce.cfg.ScanInterval > longest {
			longest = ce.cfg.ScanInterval
		}
	}
	if err := db.SetMeta("last_scan_at", time.Now().UTC().Format(time.RFC3339)); err != nil {
		log.Printf("record scan heartbeat: %v", err)
		return
	}
	if err := db.SetMeta("scan_interval_seconds", strconv.Itoa(int(longest.Seconds()))); err != nil {
		log.Printf("record scan interval: %v", err)
	}
}

// watchParentExit makes the process follow the one that spawned it. Tauri hands
// the sidecar a piped stdin and never writes to it, so EOF means the parent is
// gone. Without this a crashed or force-quit app leaves the sidecar running,
// still holding the database and still scanning in the background.
//
// Armed only when stdin really is a pipe: a terminal, /dev/null, or a redirected
// file reaches EOF on its own, and standalone use must not shut down because of
// that.
func watchParentExit() {
	if !isPipe(os.Stdin) {
		return
	}
	go func() {
		// Any return means the pipe is finished: EOF when the parent exited
		// cleanly, a read error when it died abruptly. Both mean it is gone.
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			log.Printf("stdin watch: %v", err)
		}
		log.Println("parent process closed stdin; exiting")
		os.Exit(0)
	}()
}

func isPipe(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeNamedPipe != 0
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("agent-usage-desktop %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	watchParentExit()

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
		maintainStorage(db, cfg.Storage.SessionEventRetentionDays)
		collectors := []collectorEntry{
			{"Claude Code", collector.NewClaudeCollector(db, cfg.Collectors.Claude.Paths), cfg.Collectors.Claude},
			{"Codex", collector.NewCodexCollector(db, cfg.Collectors.Codex.Paths), cfg.Collectors.Codex},
			{"OpenClaw", collector.NewOpenClawCollector(db, cfg.Collectors.OpenClaw.Paths), cfg.Collectors.OpenClaw},
			{"OpenCode", collector.NewOpenCodeCollector(db, cfg.Collectors.OpenCode.Paths), cfg.Collectors.OpenCode},
			{"Pi", collector.NewPiCollector(db, cfg.Collectors.Pi.Paths), cfg.Collectors.Pi},
		}
		log.Println("scanning historical sessions before initial pricing sync...")
		runInitialCollection(collectors, func() {
			log.Println("syncing pricing data...")
			syncAndPriceUsage(db)
		})
		recordScanHeartbeat(db, collectors)

		for _, ce := range collectors {
			if !ce.cfg.Enabled {
				continue
			}
			go func(ce collectorEntry) {
				ticker := time.NewTicker(ce.cfg.ScanInterval)
				for range ticker.C {
					ce.c.Scan()
					priceUnpricedUsage(db)
					recordScanHeartbeat(db, collectors)
				}
			}(ce)
		}

		// Periodic pricing sync
		ticker := time.NewTicker(cfg.Pricing.SyncInterval)
		for range ticker.C {
			syncAndPriceUsage(db)
			maintainStorage(db, cfg.Storage.SessionEventRetentionDays)
		}
	}()

	// Block forever (server runs in its own goroutine)
	select {}
}

// maintainStorage drops session content past the retention window and reclaims
// the freed space. PruneSessionEvents throttles itself to once per 24h.
func maintainStorage(db *storage.DB, retentionDays int) {
	deleted, err := db.PruneSessionEvents(retentionDays)
	if err != nil {
		log.Printf("prune session events: %v", err)
		return
	}
	if deleted == 0 {
		return
	}
	log.Printf("pruned %d session events older than %d days; compacting database (queries pause)...", deleted, retentionDays)
	if err := db.Vacuum(); err != nil {
		log.Printf("vacuum: %v", err)
	}
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
