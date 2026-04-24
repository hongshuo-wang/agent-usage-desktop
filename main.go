package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/collector"
	"github.com/hongshuo-wang/agent-usage-desktop/internal/config"
	"github.com/hongshuo-wang/agent-usage-desktop/internal/configmanager"
	"github.com/hongshuo-wang/agent-usage-desktop/internal/pricing"
	"github.com/hongshuo-wang/agent-usage-desktop/internal/server"
	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("agent-usage-desktop %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	configPath := flag.String("config", "", "path to config file")
	portFlag := flag.Int("port", 0, "override server port")
	flag.Parse()

	cfg, err := config.Load(config.ResolveConfigPath(*configPath))
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

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("home dir: %v", err)
	}
	backupDir, err := configmanager.BackupBaseDir()
	if err != nil {
		log.Fatalf("config backup dir: %v", err)
	}
	mgr := configmanager.NewManager(db, backupDir,
		configmanager.WithClaudeAdapter(filepath.Join(homeDir, ".claude"), filepath.Join(homeDir, ".claude.json")),
		configmanager.WithCodexAdapter(filepath.Join(homeDir, ".codex")),
		configmanager.WithOpenCodeAdapter(filepath.Join(homeDir, ".config", "opencode", "opencode.json")),
		configmanager.WithOpenClawAdapter(filepath.Join(homeDir, ".openclaw", "openclaw.json")),
	)
	if err := mgr.Bootstrap(); err != nil {
		log.Printf("config bootstrap failed: %v", err)
	}

	// Start web server first so health check is immediately available.
	// Data initialization (pricing sync, collector scan) runs in the background.
	addr := fmt.Sprintf("%s:%d", cfg.Server.BindAddress, cfg.Server.Port)
	srv := server.New(db, mgr, addr)
	go func() {
		log.Fatal(srv.Start())
	}()
	log.Printf("server listening on %s", addr)

	// Background: sync pricing, scan collectors, then start periodic loops
	go func() {
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				if changes, err := mgr.TriggerInboundSync(); err != nil {
					log.Printf("config inbound sync: %v", err)
				} else if len(changes) > 0 {
					log.Printf("config inbound sync: %d external changes detected", len(changes))
				}
			}
		}()

		log.Println("syncing pricing data...")
		if err := pricing.Sync(db); err != nil {
			log.Printf("pricing sync failed: %v (continuing without pricing)", err)
		}
		recalcCosts(db)

		type collectorEntry struct {
			name string
			c    collector.Collector
			cfg  config.CollectorConfig
		}
		collectors := []collectorEntry{
			{"Claude Code", collector.NewClaudeCollector(db, cfg.Collectors.Claude.Paths), cfg.Collectors.Claude},
			{"Codex", collector.NewCodexCollector(db, cfg.Collectors.Codex.Paths), cfg.Collectors.Codex},
			{"OpenClaw", collector.NewOpenClawCollector(db, cfg.Collectors.OpenClaw.Paths), cfg.Collectors.OpenClaw},
			{"OpenCode", collector.NewOpenCodeCollector(db, cfg.Collectors.OpenCode.Paths), cfg.Collectors.OpenCode},
		}
		for _, ce := range collectors {
			if !ce.cfg.Enabled {
				continue
			}
			log.Printf("scanning %s sessions...", ce.name)
			if err := ce.c.Scan(); err != nil {
				log.Printf("%s scan: %v", ce.name, err)
			}
			recalcCosts(db)

			go func(ce collectorEntry) {
				ticker := time.NewTicker(ce.cfg.ScanInterval)
				for range ticker.C {
					ce.c.Scan()
					recalcCosts(db)
				}
			}(ce)
		}

		// Periodic pricing sync
		ticker := time.NewTicker(cfg.Pricing.SyncInterval)
		for range ticker.C {
			pricing.Sync(db)
			recalcCosts(db)
		}
	}()

	// Block forever (server runs in its own goroutine)
	select {}
}

func recalcCosts(db *storage.DB) {
	prices, err := db.GetAllPricing()
	if err != nil {
		return
	}
	if err := db.RecalcCosts(prices, pricing.CalcCost); err != nil {
		log.Printf("recalc costs: %v", err)
	}
}
