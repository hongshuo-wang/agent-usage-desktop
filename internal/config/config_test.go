package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestSaveConfigRoundTripsCollectorSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.yaml")
	cfg := DefaultConfig()
	cfg.Collectors.Claude = CollectorConfig{Enabled: false, Paths: []string{"/claude/a", "/claude/b"}, ScanInterval: 45 * time.Second}
	cfg.Collectors.Codex = CollectorConfig{Enabled: true, Paths: []string{"/codex"}, ScanInterval: 2 * time.Minute}
	cfg.Collectors.OpenClaw = CollectorConfig{Enabled: false, Paths: []string{"/openclaw"}, ScanInterval: 3 * time.Minute}
	cfg.Collectors.OpenCode = CollectorConfig{Enabled: true, Paths: []string{"/opencode.db"}, ScanInterval: 4 * time.Minute}
	cfg.Pricing.SyncInterval = 6 * time.Hour

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Collectors.Claude.Enabled || len(loaded.Collectors.Claude.Paths) != 2 || loaded.Collectors.Claude.ScanInterval != 45*time.Second {
		t.Fatalf("claude = %+v", loaded.Collectors.Claude)
	}
	if loaded.Collectors.Codex.ScanInterval != 2*time.Minute || loaded.Collectors.OpenClaw.ScanInterval != 3*time.Minute || loaded.Collectors.OpenCode.ScanInterval != 4*time.Minute {
		t.Fatalf("collector intervals not preserved: %+v", loaded.Collectors)
	}
	if loaded.Pricing.SyncInterval != 6*time.Hour {
		t.Fatalf("pricing interval = %s", loaded.Pricing.SyncInterval)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
	if info, err := os.Stat(filepath.Dir(path)); err != nil || !info.IsDir() {
		t.Fatalf("config directory not created: info=%v err=%v", info, err)
	}
}

func TestSaveConfigFailurePreservesExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory mode enforcement is not portable to Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	original := []byte("storage:\n  path: old.db\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := Save(path, DefaultConfig())
	if err == nil {
		t.Skip("current user can write to a read-only directory")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(original) {
		t.Fatalf("existing config changed after failed save: %q", got)
	}
}
