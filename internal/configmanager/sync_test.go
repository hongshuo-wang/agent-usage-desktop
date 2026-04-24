package configmanager

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

func openSyncTestDB(t *testing.T) *storage.DB {
	t.Helper()

	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "sync-test.db"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSyncEngineOutbound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	db := openSyncTestDB(t)
	cfgFile := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(cfgFile, []byte(`{"old": true}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() setup error = %v", err)
	}

	bm := NewBackupManager(filepath.Join(dir, "backups"))
	se := NewSyncEngine(db, bm)

	affected, err := se.OutboundSync("claude", cfgFile, []byte(`{"new": true}`))
	if err != nil {
		t.Fatalf("OutboundSync() error = %v", err)
	}
	if len(affected) != 1 {
		t.Fatalf("len(affected) = %d, want 1", len(affected))
	}

	got, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(got) != `{"new": true}` {
		t.Fatalf("file content = %q, want %q", got, `{"new": true}`)
	}

	state, err := db.GetSyncState("claude", cfgFile)
	if err != nil {
		t.Fatalf("GetSyncState() error = %v", err)
	}
	if state == nil {
		t.Fatalf("GetSyncState() = nil, want state")
	}
	if state.LastHash == "" {
		t.Fatalf("state.LastHash is empty")
	}
	if state.LastSyncDir != "outbound" {
		t.Fatalf("state.LastSyncDir = %q, want %q", state.LastSyncDir, "outbound")
	}
}

func TestSyncEngineConflict(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	db := openSyncTestDB(t)
	cfgFile := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(cfgFile, []byte(`{"v":1}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() setup error = %v", err)
	}

	bm := NewBackupManager(filepath.Join(dir, "backups"))
	se := NewSyncEngine(db, bm)

	if _, err := se.OutboundSync("claude", cfgFile, []byte(`{"v":1}`)); err != nil {
		t.Fatalf("OutboundSync() initial error = %v", err)
	}

	if err := os.WriteFile(cfgFile, []byte(`{"v":2,"external":true}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() external modify error = %v", err)
	}
	actualHash, err := FileHash(cfgFile)
	if err != nil {
		t.Fatalf("FileHash() actual error = %v", err)
	}

	_, err = se.OutboundSync("claude", cfgFile, []byte(`{"v":3}`))
	var conflictErr *ConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("OutboundSync() error = %T, want *ConflictError", err)
	}
	if conflictErr.Expected == "" {
		t.Fatalf("conflictErr.Expected is empty")
	}
	if conflictErr.Actual == "" {
		t.Fatalf("conflictErr.Actual is empty")
	}
	if conflictErr.Actual != actualHash {
		t.Fatalf("conflictErr.Actual = %q, want %q", conflictErr.Actual, actualHash)
	}
	state, err := db.GetSyncState("claude", cfgFile)
	if err != nil {
		t.Fatalf("GetSyncState() error = %v", err)
	}
	if state == nil {
		t.Fatalf("GetSyncState() = nil, want state")
	}
	if conflictErr.Expected != state.LastHash {
		t.Fatalf("conflictErr.Expected = %q, want %q", conflictErr.Expected, state.LastHash)
	}
}

func TestSyncEngineForceWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	db := openSyncTestDB(t)
	cfgFile := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(cfgFile, []byte(`{"v":1}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() setup error = %v", err)
	}

	bm := NewBackupManager(filepath.Join(dir, "backups"))
	se := NewSyncEngine(db, bm)

	if _, err := se.OutboundSync("claude", cfgFile, []byte(`{"v":1}`)); err != nil {
		t.Fatalf("OutboundSync() initial error = %v", err)
	}

	if err := os.WriteFile(cfgFile, []byte(`{"v":2,"external":true}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() external modify error = %v", err)
	}

	if _, err := se.ForceWrite("claude", cfgFile, []byte(`{"v":3}`)); err != nil {
		t.Fatalf("ForceWrite() error = %v", err)
	}

	got, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}
	if string(got) != `{"v":3}` {
		t.Fatalf("file content = %q, want %q", got, `{"v":3}`)
	}

	state, err := db.GetSyncState("claude", cfgFile)
	if err != nil {
		t.Fatalf("GetSyncState() error = %v", err)
	}
	if state == nil || state.LastHash == "" {
		t.Fatalf("state invalid after ForceWrite(): %#v", state)
	}
}

func TestSyncEngineInboundScan(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	db := openSyncTestDB(t)
	cfgFile := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(cfgFile, []byte(`{"v":1}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() setup error = %v", err)
	}

	bm := NewBackupManager(filepath.Join(dir, "backups"))
	se := NewSyncEngine(db, bm)

	if _, err := se.OutboundSync("claude", cfgFile, []byte(`{"v":1}`)); err != nil {
		t.Fatalf("OutboundSync() initial error = %v", err)
	}

	if err := os.WriteFile(cfgFile, []byte(`{"v":2}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() change error = %v", err)
	}

	changes, err := se.InboundScan("claude", &syncTestAdapter{
		tool: "claude",
		files: []ConfigFileInfo{
			{Path: cfgFile, Tool: "claude", Exists: true},
		},
	})
	if err != nil {
		t.Fatalf("InboundScan() error = %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("len(changes) = %d, want 1", len(changes))
	}
	if changes[0].FilePath != cfgFile {
		t.Fatalf("changes[0].FilePath = %q, want %q", changes[0].FilePath, cfgFile)
	}
	if changes[0].OldHash == "" || changes[0].NewHash == "" {
		t.Fatalf("changes[0] hashes should be non-empty: %#v", changes[0])
	}
}

func TestSyncEngineInboundScanSkipsMissingUnmanaged(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	db := openSyncTestDB(t)
	missingFile := filepath.Join(dir, "missing.json")

	bm := NewBackupManager(filepath.Join(dir, "backups"))
	se := NewSyncEngine(db, bm)

	changes, err := se.InboundScan("claude", &syncTestAdapter{
		tool: "claude",
		files: []ConfigFileInfo{
			{Path: missingFile, Tool: "claude", Exists: false},
		},
	})
	if err != nil {
		t.Fatalf("InboundScan() error = %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("len(changes) = %d, want 0", len(changes))
	}
}

func TestSyncEngineAcceptInbound(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	db := openSyncTestDB(t)
	cfgFile := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(cfgFile, []byte(`{"v":1}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() setup error = %v", err)
	}

	bm := NewBackupManager(filepath.Join(dir, "backups"))
	se := NewSyncEngine(db, bm)

	if _, err := se.OutboundSync("claude", cfgFile, []byte(`{"v":1}`)); err != nil {
		t.Fatalf("OutboundSync() initial error = %v", err)
	}

	if err := os.WriteFile(cfgFile, []byte(`{"v":2}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() inbound change error = %v", err)
	}

	changes, err := se.InboundScan("claude", &syncTestAdapter{
		tool: "claude",
		files: []ConfigFileInfo{
			{Path: cfgFile, Tool: "claude", Exists: true},
		},
	})
	if err != nil {
		t.Fatalf("InboundScan() error = %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("len(changes) = %d, want 1", len(changes))
	}

	if err := se.AcceptInbound("claude", cfgFile); err != nil {
		t.Fatalf("AcceptInbound() error = %v", err)
	}

	state, err := db.GetSyncState("claude", cfgFile)
	if err != nil {
		t.Fatalf("GetSyncState() error = %v", err)
	}
	if state == nil {
		t.Fatalf("GetSyncState() = nil, want state")
	}
	currentHash, err := FileHash(cfgFile)
	if err != nil {
		t.Fatalf("FileHash() error = %v", err)
	}
	if state.LastHash != currentHash {
		t.Fatalf("state.LastHash = %q, want %q", state.LastHash, currentHash)
	}
	if state.LastSyncDir != "inbound" {
		t.Fatalf("state.LastSyncDir = %q, want %q", state.LastSyncDir, "inbound")
	}

	changes, err = se.InboundScan("claude", &syncTestAdapter{
		tool: "claude",
		files: []ConfigFileInfo{
			{Path: cfgFile, Tool: "claude", Exists: true},
		},
	})
	if err != nil {
		t.Fatalf("InboundScan() post-accept error = %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("len(changes) post-accept = %d, want 0", len(changes))
	}
}

type syncTestAdapter struct {
	tool  string
	files []ConfigFileInfo
}

func (a *syncTestAdapter) Tool() string { return a.tool }

func (a *syncTestAdapter) IsInstalled() bool { return true }

func (a *syncTestAdapter) GetProviderConfig() (*ProviderConfig, error) { return nil, nil }

func (a *syncTestAdapter) SetProviderConfig(*ProviderConfig) ([]AffectedFile, error) { return nil, nil }

func (a *syncTestAdapter) GetMCPServers() ([]MCPServerConfig, error) { return nil, nil }

func (a *syncTestAdapter) SetMCPServers([]MCPServerConfig) ([]AffectedFile, error) { return nil, nil }

func (a *syncTestAdapter) GetSkillPaths() []string { return nil }

func (a *syncTestAdapter) ConfigFiles() []ConfigFileInfo { return a.files }
