# Config Manager Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add unified AI agent configuration management (Provider profiles, MCP servers, Skills) for Claude Code, Codex, OpenCode, and OpenClaw to the existing agent-usage-desktop app.

**Architecture:** New `internal/configmanager` package with Adapter pattern (one adapter per tool), BackupManager for rolling backups, SyncEngine for bidirectional sync. SQLite as SSOT with encrypted API key storage. REST API endpoints added to existing server. React frontend with 4-tab Config page.

**Tech Stack:** Go stdlib (net/http, crypto/aes, encoding/json), modernc.org/sqlite, github.com/zalando/go-keyring, github.com/BurntSushi/toml, React 18, TypeScript, Tailwind CSS v4, react-router-dom

**Spec:** `docs/superpowers/specs/2026-04-23-config-manager-design.md`

---

## File Structure

### Go Backend — New Files

| File | Responsibility |
|------|---------------|
| `internal/configmanager/types.go` | Shared types: ProviderConfig, MCPServer, AffectedFile, ConfigFileInfo, Adapter interface |
| `internal/configmanager/crypto.go` | AES-256-GCM encryption/decryption, keyring integration, machine-id fallback |
| `internal/configmanager/backup.go` | BackupManager: rolling 5-slot backups, cross-platform paths, restore |
| `internal/configmanager/fileutil.go` | Atomic write, advisory file locking (flock/LockFileEx), SHA-256 hashing |
| `internal/configmanager/sync.go` | SyncEngine: inbound/outbound sync, conflict detection, hash tracking |
| `internal/configmanager/adapter_claude.go` | Claude Code adapter: read/write settings.json, .claude.json |
| `internal/configmanager/adapter_codex.go` | Codex adapter: read/write auth.json, config.toml |
| `internal/configmanager/adapter_opencode.go` | OpenCode adapter: read/write opencode.json |
| `internal/configmanager/adapter_openclaw.go` | OpenClaw adapter: read/write openclaw.json |
| `internal/configmanager/manager.go` | ConfigManager: orchestrates adapters, sync, backup; CRUD for profiles/mcp/skills |
| `internal/configmanager/manager_test.go` | Tests for ConfigManager (integration with temp SQLite) |
| `internal/configmanager/backup_test.go` | Tests for BackupManager |
| `internal/configmanager/crypto_test.go` | Tests for encryption/decryption |
| `internal/configmanager/fileutil_test.go` | Tests for atomic write, hashing |
| `internal/configmanager/sync_test.go` | Tests for SyncEngine |
| `internal/configmanager/adapter_claude_test.go` | Tests for Claude adapter |

### Go Backend — Modified Files

| File | Changes |
|------|---------|
| `internal/storage/sqlite.go` | Add migration `005_config_manager` with 8 new tables; add `_pragma=foreign_keys(1)` to DSN in `Open()` |
| `internal/storage/configstore.go` | New file: CRUD queries for config manager tables (profiles, mcp_servers, skills, etc.) |
| `internal/server/server.go` | Update CORS to allow POST/PUT/DELETE; register config API routes using Go 1.22+ method patterns |
| `internal/server/config_handlers.go` | New file: all `/api/config/*` HTTP handlers |
| `main.go` | Wire ConfigManager, start inbound sync ticker |
| `go.mod` / `go.sum` | Add `go-keyring`, `toml` dependencies |

### Frontend — New Files

| File | Responsibility |
|------|---------------|
| `src/pages/Config.tsx` | Config page shell with tab navigation + sub-route outlet |
| `src/pages/config/Providers.tsx` | Provider profiles tab: list, edit, activate |
| `src/pages/config/MCPServers.tsx` | MCP servers tab: unified list with tool targets |
| `src/pages/config/Skills.tsx` | Skills tab: unified list with tool targets |
| `src/pages/config/FilesBackups.tsx` | Files & backups tab: file info, backup history, restore |
| `src/components/ConfirmPanel.tsx` | Reusable confirmation panel showing affected files |
| `src/components/SyncStatus.tsx` | Sync status indicator (green/orange) |
| `src/components/ToolTargets.tsx` | Reusable tool checkbox group (Claude/Codex/OpenCode/OpenClaw) |

### Frontend — Modified Files

| File | Changes |
|------|---------|
| `src/lib/api.ts` | Add `mutateAPI(method, path, body)` helper for POST/PUT/DELETE |
| `src/App.tsx` | Add `/config` route with nested sub-routes |
| `src/components/Layout.tsx` | Add "Config" to navItems |
| `src/lib/locales/en.json` | Add config manager i18n keys |
| `src/lib/locales/zh.json` | Add config manager i18n keys (Chinese) |

---
## Task 1: Database Schema Migration

**Files:**
- Modify: `internal/storage/sqlite.go` (add migration to the `migrations` slice at ~line 147)
- Create: `internal/storage/configstore.go`
- Test: `internal/storage/configstore_test.go`

- [ ] **Step 1: Write test for migration and basic profile CRUD**

```go
// internal/storage/configstore_test.go
package storage

import (
    "os"
    "path/filepath"
    "testing"
)

func TestConfigManagerMigration(t *testing.T) {
    dir := t.TempDir()
    db, err := Open(filepath.Join(dir, "test.db"))
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()

    // Verify tables exist by inserting a profile
    err = db.CreateProfile("test-profile", `{"api_key":"enc:xxx","base_url":"https://api.example.com","model":"gpt-4"}`)
    if err != nil {
        t.Fatal("CreateProfile failed:", err)
    }

    profiles, err := db.ListProfiles()
    if err != nil {
        t.Fatal(err)
    }
    if len(profiles) != 1 || profiles[0].Name != "test-profile" {
        t.Fatalf("expected 1 profile named test-profile, got %v", profiles)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/storage/ -run TestConfigManagerMigration -v`
Expected: FAIL — `CreateProfile` method not defined

- [ ] **Step 3: Enable foreign keys in SQLite DSN and add migration**

In `internal/storage/sqlite.go`, update the `Open()` function DSN to enable foreign keys on every connection:

```go
// Before:
db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
// After:
db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
```

Then add to the `migrations` slice (after the existing `004_...` entry):

```go
{"005_config_manager", `
    CREATE TABLE IF NOT EXISTS provider_profiles (
        id          INTEGER PRIMARY KEY,
        name        TEXT NOT NULL UNIQUE,
        is_active   BOOLEAN DEFAULT 0,
        config      TEXT NOT NULL,
        created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
    CREATE TABLE IF NOT EXISTS profile_tool_targets (
        profile_id  INTEGER NOT NULL REFERENCES provider_profiles(id) ON DELETE CASCADE,
        tool        TEXT NOT NULL,
        enabled     BOOLEAN DEFAULT 1,
        tool_config TEXT,
        PRIMARY KEY (profile_id, tool)
    );
    CREATE TABLE IF NOT EXISTS mcp_servers (
        id          INTEGER PRIMARY KEY,
        name        TEXT NOT NULL UNIQUE,
        command     TEXT NOT NULL,
        args        TEXT,
        env         TEXT,
        enabled     BOOLEAN DEFAULT 1,
        created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
    CREATE TABLE IF NOT EXISTS mcp_server_targets (
        server_id   INTEGER NOT NULL REFERENCES mcp_servers(id) ON DELETE CASCADE,
        tool        TEXT NOT NULL,
        enabled     BOOLEAN DEFAULT 1,
        PRIMARY KEY (server_id, tool)
    );
    CREATE TABLE IF NOT EXISTS skills (
        id          INTEGER PRIMARY KEY,
        name        TEXT NOT NULL UNIQUE,
        source_path TEXT NOT NULL,
        description TEXT,
        enabled     BOOLEAN DEFAULT 1,
        created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );
    CREATE TABLE IF NOT EXISTS skill_targets (
        skill_id    INTEGER NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
        tool        TEXT NOT NULL,
        method      TEXT DEFAULT 'symlink',
        enabled     BOOLEAN DEFAULT 1,
        PRIMARY KEY (skill_id, tool)
    );
    CREATE TABLE IF NOT EXISTS config_backups (
        id          INTEGER PRIMARY KEY,
        tool        TEXT NOT NULL,
        file_path   TEXT NOT NULL,
        backup_path TEXT NOT NULL,
        slot        INTEGER NOT NULL,
        created_at  TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        trigger_type TEXT
    );
    CREATE TABLE IF NOT EXISTS sync_state (
        tool        TEXT NOT NULL,
        file_path   TEXT NOT NULL,
        last_hash   TEXT NOT NULL,
        last_sync   TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        last_sync_dir TEXT,
        PRIMARY KEY (tool, file_path)
    );
`},
```

Note: SQLite column name `trigger` is a reserved word — use `trigger_type` instead.

- [ ] **Step 4: Implement configstore.go with profile CRUD**

```go
// internal/storage/configstore.go
package storage

import "time"

type Profile struct {
    ID        int64  `json:"id"`
    Name      string `json:"name"`
    IsActive  bool   `json:"is_active"`
    Config    string `json:"config"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

func (d *DB) CreateProfile(name, config string) error {
    d.mu.Lock()
    defer d.mu.Unlock()
    _, err := d.db.Exec(
        `INSERT INTO provider_profiles (name, config) VALUES (?, ?)`,
        name, config,
    )
    return err
}

func (d *DB) ListProfiles() ([]Profile, error) {
    rows, err := d.db.Query(`SELECT id, name, is_active, config, created_at, updated_at FROM provider_profiles ORDER BY id`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    var profiles []Profile
    for rows.Next() {
        var p Profile
        if err := rows.Scan(&p.ID, &p.Name, &p.IsActive, &p.Config, &p.CreatedAt, &p.UpdatedAt); err != nil {
            return nil, err
        }
        profiles = append(profiles, p)
    }
    return profiles, rows.Err()
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/storage/ -run TestConfigManagerMigration -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/storage/sqlite.go internal/storage/configstore.go internal/storage/configstore_test.go
git commit -m "feat: add config manager database schema migration and profile CRUD"
```

---

## Task 2: Complete Config Store (MCP, Skills, Backups, Sync State)

**Files:**
- Modify: `internal/storage/configstore.go` (add remaining CRUD methods)
- Test: `internal/storage/configstore_test.go`

- [ ] **Step 1: Write tests for MCP server CRUD**

```go
func TestMCPServerCRUD(t *testing.T) {
    db := openTestDB(t)
    defer db.Close()

    // Create
    id, err := db.CreateMCPServer("test-mcp", "npx", `["-y","@modelcontextprotocol/server"]`, `{"NODE_ENV":"production"}`)
    if err != nil { t.Fatal(err) }
    if id == 0 { t.Fatal("expected non-zero id") }

    // List
    servers, err := db.ListMCPServers()
    if err != nil { t.Fatal(err) }
    if len(servers) != 1 { t.Fatalf("expected 1 server, got %d", len(servers)) }

    // Update
    err = db.UpdateMCPServer(id, "updated-mcp", "node", `["server.js"]`, `{}`, false)
    if err != nil { t.Fatal(err) }

    // Delete
    err = db.DeleteMCPServer(id)
    if err != nil { t.Fatal(err) }
    servers, _ = db.ListMCPServers()
    if len(servers) != 0 { t.Fatal("expected 0 servers after delete") }
}
```

Extract `openTestDB` helper:
```go
func openTestDB(t *testing.T) *DB {
    t.Helper()
    db, err := Open(filepath.Join(t.TempDir(), "test.db"))
    if err != nil { t.Fatal(err) }
    return db
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/storage/ -run TestMCPServerCRUD -v`
Expected: FAIL

- [ ] **Step 3: Implement MCP server CRUD in configstore.go**

Add types and methods: `MCPServerRecord`, `CreateMCPServer`, `ListMCPServers`, `GetMCPServer`, `UpdateMCPServer`, `DeleteMCPServer`. Follow the same raw SQL pattern as profile CRUD.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/storage/ -run TestMCPServerCRUD -v`
Expected: PASS

- [ ] **Step 5: Write tests for Skills CRUD, targets, sync state, and backup records**

Add tests: `TestSkillCRUD`, `TestProfileToolTargets`, `TestMCPServerTargets`, `TestSkillTargets`, `TestSyncState`, `TestBackupRecords`. Each follows the same create/list/update/delete pattern.

- [ ] **Step 6: Implement remaining CRUD methods**

Add to `configstore.go`:
- Skills: `CreateSkill`, `ListSkills`, `GetSkill`, `UpdateSkill`, `DeleteSkill`
- Profile targets: `SetProfileToolTargets`, `GetProfileToolTargets`
- MCP targets: `SetMCPServerTargets`, `GetMCPServerTargets`
- Skill targets: `SetSkillTargets`, `GetSkillTargets`
- Sync state: `GetSyncState`, `UpsertSyncState`
- Backups: `InsertBackupRecord`, `ListBackups`, `GetBackupByID`
- Profile activation: `ActivateProfile` (deactivate current, activate new — in a transaction)

- [ ] **Step 7: Run all tests**

Run: `go test ./internal/storage/ -v`
Expected: ALL PASS

- [ ] **Step 8: Commit**

```bash
git add internal/storage/configstore.go internal/storage/configstore_test.go
git commit -m "feat: complete config store with MCP, skills, targets, sync state, backup CRUD"
```

---

## Task 4: Crypto Module (API Key Encryption)

**Files:**
- Create: `internal/configmanager/crypto.go`
- Test: `internal/configmanager/crypto_test.go`

- [ ] **Step 1: Write test for encrypt/decrypt round-trip**

```go
// internal/configmanager/crypto_test.go
package configmanager

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
    key := make([]byte, 32) // zero key for testing
    plaintext := "sk-ant-api03-xxxxxxxxxxxx"

    encrypted, err := Encrypt(plaintext, key)
    if err != nil { t.Fatal(err) }

    if encrypted == plaintext { t.Fatal("encrypted should differ from plaintext") }
    if encrypted[:4] != "enc:" { t.Fatal("encrypted should start with enc: prefix") }

    decrypted, err := Decrypt(encrypted, key)
    if err != nil { t.Fatal(err) }
    if decrypted != plaintext { t.Fatalf("expected %q, got %q", plaintext, decrypted) }
}

func TestDecryptPlaintext(t *testing.T) {
    key := make([]byte, 32)
    // Non-encrypted value should pass through unchanged
    result, err := Decrypt("sk-plain-key", key)
    if err != nil { t.Fatal(err) }
    if result != "sk-plain-key" { t.Fatal("plaintext should pass through") }
}

func TestMaskAPIKey(t *testing.T) {
    masked := MaskAPIKey("sk-ant-api03-xxxxxxxxxxxx")
    if masked != "sk-a...xxxx" { t.Fatalf("unexpected mask: %s", masked) }

    short := MaskAPIKey("abc")
    if short != "***" { t.Fatal("short key should be fully masked") }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/configmanager/ -run TestEncrypt -v`
Expected: FAIL

- [ ] **Step 3: Implement crypto.go**

```go
// internal/configmanager/crypto.go
package configmanager

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "strings"
)

const encPrefix = "enc:"

func Encrypt(plaintext string, key []byte) (string, error) {
    block, err := aes.NewCipher(key)
    if err != nil { return "", err }
    gcm, err := cipher.NewGCM(block)
    if err != nil { return "", err }
    nonce := make([]byte, gcm.NonceSize())
    if _, err := rand.Read(nonce); err != nil { return "", err }
    ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
    return encPrefix + base64.StdEncoding.EncodeToString(ciphertext), nil
}

func Decrypt(value string, key []byte) (string, error) {
    if !strings.HasPrefix(value, encPrefix) {
        return value, nil // plaintext passthrough
    }
    data, err := base64.StdEncoding.DecodeString(value[len(encPrefix):])
    if err != nil { return "", err }
    block, err := aes.NewCipher(key)
    if err != nil { return "", err }
    gcm, err := cipher.NewGCM(block)
    if err != nil { return "", err }
    nonceSize := gcm.NonceSize()
    if len(data) < nonceSize {
        return "", fmt.Errorf("ciphertext too short")
    }
    plaintext, err := gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
    if err != nil { return "", err }
    return string(plaintext), nil
}

func MaskAPIKey(key string) string {
    if len(key) <= 8 { return "***" }
    return key[:4] + "..." + key[len(key)-4:]
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/configmanager/ -run "TestEncrypt|TestDecrypt|TestMask" -v`
Expected: ALL PASS

- [ ] **Step 5: Add keyring integration**

Add `GetOrCreateEncryptionKey()` function that tries `go-keyring` first, falls back to machine-id derived key with a log warning. Add `go get github.com/zalando/go-keyring`.

- [ ] **Step 6: Commit**

```bash
git add internal/configmanager/crypto.go internal/configmanager/crypto_test.go go.mod go.sum
git commit -m "feat: add API key encryption with AES-256-GCM and keyring integration"
```

---

## Task 5: File Utilities (Atomic Write, Hashing, Locking)

**Files:**
- Create: `internal/configmanager/fileutil.go`
- Test: `internal/configmanager/fileutil_test.go`

- [ ] **Step 1: Write tests**

```go
func TestAtomicWrite(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "test.json")
    content := []byte(`{"key": "value"}`)

    err := AtomicWrite(path, content)
    if err != nil { t.Fatal(err) }

    got, _ := os.ReadFile(path)
    if string(got) != string(content) { t.Fatalf("content mismatch") }
}

func TestFileHash(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "test.txt")
    os.WriteFile(path, []byte("hello"), 0644)

    hash, err := FileHash(path)
    if err != nil { t.Fatal(err) }
    if len(hash) != 64 { t.Fatal("expected SHA-256 hex string") }

    // Same content = same hash
    hash2, _ := FileHash(path)
    if hash != hash2 { t.Fatal("hash should be deterministic") }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/configmanager/ -run "TestAtomicWrite|TestFileHash" -v`
Expected: FAIL

- [ ] **Step 3: Implement fileutil.go**

Implement `AtomicWrite(path string, data []byte) error` (write to temp file in same dir, then `os.Rename`), `FileHash(path string) (string, error)` (SHA-256 hex), and `WithFileLock(path string, fn func() error) error` (advisory lock using `syscall.Flock` on Unix, stub on Windows for now).

- [ ] **Step 4: Run tests**

Run: `go test ./internal/configmanager/ -run "TestAtomicWrite|TestFileHash" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/configmanager/fileutil.go internal/configmanager/fileutil_test.go
git commit -m "feat: add atomic file write, SHA-256 hashing, and advisory file locking"
```

---

## Task 6: Backup Manager

**Files:**
- Create: `internal/configmanager/backup.go`
- Test: `internal/configmanager/backup_test.go`

- [ ] **Step 1: Write tests**

```go
func TestBackupManagerCreateAndRestore(t *testing.T) {
    dir := t.TempDir()
    bm := NewBackupManager(dir)

    // Create a source file
    srcDir := t.TempDir()
    srcFile := filepath.Join(srcDir, "settings.json")
    os.WriteFile(srcFile, []byte(`{"original": true}`), 0644)

    // Backup
    slot, backupPath, err := bm.Backup("claude", "settings.json", srcFile, "auto")
    if err != nil { t.Fatal(err) }
    if slot != 1 { t.Fatalf("expected slot 1, got %d", slot) }

    // Verify backup file is a valid copy
    got, _ := os.ReadFile(backupPath)
    if string(got) != `{"original": true}` { t.Fatal("backup content mismatch") }

    // Verify meta file exists
    metaPath := backupPath[:len(backupPath)-4] + ".meta"
    if _, err := os.Stat(metaPath); err != nil { t.Fatal("meta file missing") }

    // Modify source, then restore
    os.WriteFile(srcFile, []byte(`{"modified": true}`), 0644)
    err = bm.Restore(backupPath, srcFile)
    if err != nil { t.Fatal(err) }
    restored, _ := os.ReadFile(srcFile)
    if string(restored) != `{"original": true}` { t.Fatal("restore failed") }
}

func TestBackupManagerRolling(t *testing.T) {
    dir := t.TempDir()
    bm := NewBackupManager(dir)
    srcFile := filepath.Join(t.TempDir(), "test.json")

    // Create 6 backups — slot 6 should wrap to slot 1
    for i := 1; i <= 6; i++ {
        os.WriteFile(srcFile, []byte(fmt.Sprintf(`{"v":%d}`, i)), 0644)
        slot, _, _ := bm.Backup("claude", "test.json", srcFile, "auto")
        if i <= 5 && slot != i { t.Fatalf("iteration %d: expected slot %d, got %d", i, i, slot) }
        if i == 6 && slot != 1 { t.Fatalf("iteration 6: expected slot 1 (wrap), got %d", slot) }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/configmanager/ -run TestBackupManager -v`
Expected: FAIL

- [ ] **Step 3: Implement backup.go**

```go
type BackupManager struct {
    baseDir string // e.g. ~/Library/Application Support/agent-usage/backups/
}

func NewBackupManager(baseDir string) *BackupManager
func BackupBaseDir() (string, error) // uses os.UserConfigDir() + "agent-usage/backups"
func (bm *BackupManager) Backup(tool, fileName, srcPath, trigger string) (slot int, backupPath string, err error)
func (bm *BackupManager) Restore(backupPath, destPath string) error
func (bm *BackupManager) ListBackups(tool string) ([]BackupInfo, error)
```

Slot calculation: scan existing `.bak` files for the tool/fileName, find max slot, next = `(max % 5) + 1`. Meta file is JSON: `{"time":"...","trigger":"...","hash":"..."}`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/configmanager/ -run TestBackupManager -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/configmanager/backup.go internal/configmanager/backup_test.go
git commit -m "feat: add backup manager with rolling 5-slot backups and restore"
```

---

## Task 3: Types and Adapter Interface

**Files:**
- Create: `internal/configmanager/types.go`

This task MUST come before crypto, fileutil, and backup tasks because those modules reference these shared types.

- [ ] **Step 1: Create types.go with shared types and Adapter interface**

```go
package configmanager

type ProviderConfig struct {
    APIKey   string            `json:"api_key"`
    BaseURL  string            `json:"base_url"`
    Model    string            `json:"model"`
    ModelMap map[string]string `json:"model_map,omitempty"`
}

type MCPServerConfig struct {
    Name    string            `json:"name"`
    Command string            `json:"command"`
    Args    []string          `json:"args,omitempty"`
    Env     map[string]string `json:"env,omitempty"`
}

type AffectedFile struct {
    Path      string `json:"path"`
    Tool      string `json:"tool"`
    Operation string `json:"operation"`
    Diff      string `json:"diff,omitempty"`
}

type ConfigFileInfo struct {
    Path        string `json:"path"`
    Tool        string `json:"tool"`
    Description string `json:"description"`
    DocURL      string `json:"doc_url"`
    Exists      bool   `json:"exists"`
}

type Adapter interface {
    Tool() string
    IsInstalled() bool
    GetProviderConfig() (*ProviderConfig, error)
    SetProviderConfig(cfg *ProviderConfig) ([]AffectedFile, error)
    GetMCPServers() ([]MCPServerConfig, error)
    SetMCPServers(servers []MCPServerConfig) ([]AffectedFile, error)
    GetSkillPaths() []string
    ConfigFiles() []ConfigFileInfo
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/configmanager/types.go
git commit -m "feat: add config manager types and Adapter interface"
```

---

## Task 7: Claude Code Adapter

**Files:**
- Create: `internal/configmanager/adapter_claude.go`
- Test: `internal/configmanager/adapter_claude_test.go`

- [ ] **Step 1: Write test for reading Claude Code provider config**

```go
func TestClaudeAdapterGetProviderConfig(t *testing.T) {
    dir := t.TempDir()
    settingsPath := filepath.Join(dir, "settings.json")
    os.WriteFile(settingsPath, []byte(`{
        "env": {"ANTHROPIC_API_KEY": "sk-test-123"},
        "permissions": {"allow": ["Bash"]}
    }`), 0644)

    adapter := NewClaudeAdapter(dir, filepath.Join(dir, ".claude.json"))
    cfg, err := adapter.GetProviderConfig()
    if err != nil { t.Fatal(err) }
    if cfg.APIKey != "sk-test-123" { t.Fatalf("expected sk-test-123, got %s", cfg.APIKey) }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/configmanager/ -run TestClaudeAdapter -v`
Expected: FAIL

- [ ] **Step 3: Implement Claude adapter**

The adapter reads `settings.json` for provider config (API key from `env.ANTHROPIC_API_KEY`, base URL from `env.ANTHROPIC_BASE_URL`). Reads `.claude.json` for MCP servers (from `mcpServers` key). Uses `encoding/json` with `map[string]interface{}` to preserve unknown fields during read-modify-write.

Key: `SetProviderConfig` must read the existing file, merge only the provider-related fields, and write back — preserving all other fields like `permissions`, `hooks`, etc.

- [ ] **Step 4: Write test for MCP server read/write**

Test that `GetMCPServers` reads from `.claude.json` and `SetMCPServers` writes back without losing other fields.

- [ ] **Step 5: Run all Claude adapter tests**

Run: `go test ./internal/configmanager/ -run TestClaudeAdapter -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add internal/configmanager/adapter_claude.go internal/configmanager/adapter_claude_test.go
git commit -m "feat: add Claude Code config adapter"
```

---

## Task 8: Codex Adapter

**Files:**
- Create: `internal/configmanager/adapter_codex.go`
- Test: `internal/configmanager/adapter_codex_test.go`

- [ ] **Step 1: Write test for Codex provider config (TOML format)**

Test reading `config.toml` for provider settings and `auth.json` for API key. Test that `SetProviderConfig` preserves non-provider TOML sections.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/configmanager/ -run TestCodexAdapter -v`
Expected: FAIL

- [ ] **Step 3: Add TOML dependency and implement Codex adapter**

```bash
go get github.com/BurntSushi/toml
```

Codex stores MCP servers in `config.toml` under `[mcp_servers.<name>]` sections. Provider config is in `config.toml` under `[model]` and `[provider]` sections. Auth is in `auth.json`.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/configmanager/ -run TestCodexAdapter -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/configmanager/adapter_codex.go internal/configmanager/adapter_codex_test.go go.mod go.sum
git commit -m "feat: add Codex CLI config adapter with TOML support"
```

---

## Task 9: OpenCode and OpenClaw Adapters

**Files:**
- Create: `internal/configmanager/adapter_opencode.go`
- Create: `internal/configmanager/adapter_openclaw.go`
- Test: `internal/configmanager/adapter_opencode_test.go`
- Test: `internal/configmanager/adapter_openclaw_test.go`

Both use JSON config files with similar structure. OpenCode stores everything in `opencode.json`, OpenClaw in `openclaw.json`. MCP servers are inline in the main config under `"mcp"` key.

- [ ] **Step 1: Write tests for OpenCode adapter**

Test read/write of provider config and MCP servers from `opencode.json`. Verify non-managed fields are preserved.

- [ ] **Step 2: Implement OpenCode adapter**

- [ ] **Step 3: Run OpenCode tests**

Run: `go test ./internal/configmanager/ -run TestOpenCodeAdapter -v`
Expected: PASS

- [ ] **Step 4: Write tests for OpenClaw adapter**

- [ ] **Step 5: Implement OpenClaw adapter**

- [ ] **Step 6: Run all adapter tests**

Run: `go test ./internal/configmanager/ -v`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add internal/configmanager/adapter_opencode.go internal/configmanager/adapter_openclaw.go \
       internal/configmanager/adapter_opencode_test.go internal/configmanager/adapter_openclaw_test.go
git commit -m "feat: add OpenCode and OpenClaw config adapters"
```

---

## Task 10: Sync Engine

**Files:**
- Create: `internal/configmanager/sync.go`
- Test: `internal/configmanager/sync_test.go`

- [ ] **Step 1: Write test for outbound sync (no conflict)**

```go
func TestSyncEngineOutbound(t *testing.T) {
    db := openTestDB(t)
    defer db.Close()
    dir := t.TempDir()
    bm := NewBackupManager(filepath.Join(dir, "backups"))

    // Create a config file
    cfgFile := filepath.Join(dir, "settings.json")
    os.WriteFile(cfgFile, []byte(`{"old": true}`), 0644)

    se := NewSyncEngine(db, bm)

    // First outbound sync — no prior state, should succeed
    affected, err := se.OutboundSync("claude", cfgFile, []byte(`{"new": true}`))
    if err != nil { t.Fatal(err) }
    if len(affected) != 1 { t.Fatal("expected 1 affected file") }

    // Verify file was written
    got, _ := os.ReadFile(cfgFile)
    if string(got) != `{"new": true}` { t.Fatal("file not updated") }

    // Verify sync state was recorded
    state, err := db.GetSyncState("claude", cfgFile)
    if err != nil { t.Fatal(err) }
    if state.LastHash == "" { t.Fatal("hash not recorded") }
}
```

- [ ] **Step 2: Write test for conflict detection**

```go
func TestSyncEngineConflict(t *testing.T) {
    db := openTestDB(t)
    defer db.Close()
    dir := t.TempDir()
    bm := NewBackupManager(filepath.Join(dir, "backups"))
    se := NewSyncEngine(db, bm)

    cfgFile := filepath.Join(dir, "settings.json")
    os.WriteFile(cfgFile, []byte(`{"v":1}`), 0644)

    // Record initial state
    se.OutboundSync("claude", cfgFile, []byte(`{"v":1}`))

    // Simulate external modification
    os.WriteFile(cfgFile, []byte(`{"v":2,"external":true}`), 0644)

    // Outbound sync should detect conflict
    _, err := se.OutboundSync("claude", cfgFile, []byte(`{"v":3}`))
    if err == nil { t.Fatal("expected conflict error") }
    // Error should be a ConflictError type
    var ce *ConflictError
    if !errors.As(err, &ce) { t.Fatalf("expected ConflictError, got %T", err) }
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/configmanager/ -run TestSyncEngine -v`
Expected: FAIL

- [ ] **Step 4: Implement sync.go**

```go
type ConflictError struct {
    Tool     string
    FilePath string
    Expected string // hash we expected
    Actual   string // hash we found
}

type SyncEngine struct {
    db     *storage.DB
    backup *BackupManager
}

func NewSyncEngine(db *storage.DB, backup *BackupManager) *SyncEngine
func (se *SyncEngine) OutboundSync(tool, filePath string, newContent []byte) ([]AffectedFile, error)
func (se *SyncEngine) InboundScan(tool string, adapter Adapter) ([]SyncChange, error)
func (se *SyncEngine) ForceWrite(tool, filePath string, content []byte) ([]AffectedFile, error)
```

`OutboundSync` flow: check hash against sync_state → if mismatch, return ConflictError → if match (or no prior state), backup → atomic write → update sync_state.

`InboundScan` flow: for each config file of the adapter, compute hash → compare with sync_state → if different, read and return changes.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/configmanager/ -run TestSyncEngine -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add internal/configmanager/sync.go internal/configmanager/sync_test.go
git commit -m "feat: add sync engine with conflict detection and outbound/inbound sync"
```

---

## Task 11: Config Manager (Orchestrator)

**Files:**
- Create: `internal/configmanager/manager.go`
- Test: `internal/configmanager/manager_test.go`

- [ ] **Step 1: Write integration test for profile activation**

```go
func TestManagerActivateProfile(t *testing.T) {
    db := openTestDB(t)
    defer db.Close()
    dir := t.TempDir()

    // Set up a fake Claude config dir
    claudeDir := filepath.Join(dir, ".claude")
    os.MkdirAll(claudeDir, 0755)
    os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{}`), 0644)

    mgr := NewManager(db, filepath.Join(dir, "backups"),
        WithClaudeAdapter(claudeDir, filepath.Join(dir, ".claude.json")),
    )

    // Create and activate a profile
    profileID, _ := mgr.CreateProfile("test", `{"api_key":"sk-123","base_url":"https://api.anthropic.com"}`,
        map[string]bool{"claude": true})

    affected, err := mgr.ActivateProfile(profileID)
    if err != nil { t.Fatal(err) }
    if len(affected) == 0 { t.Fatal("expected affected files") }

    // Verify the config was written to Claude's settings.json
    data, _ := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
    if !strings.Contains(string(data), "sk-123") { t.Fatal("API key not written") }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/configmanager/ -run TestManagerActivateProfile -v`
Expected: FAIL

- [ ] **Step 3: Implement manager.go**

```go
type Manager struct {
    db       *storage.DB
    sync     *SyncEngine
    backup   *BackupManager
    adapters map[string]Adapter
    crypto   []byte // encryption key
}

func NewManager(db *storage.DB, backupDir string, opts ...ManagerOption) *Manager
func (m *Manager) CreateProfile(name, config string, toolTargets map[string]bool) (int64, error)
func (m *Manager) UpdateProfile(id int64, name, config string) error
func (m *Manager) DeleteProfile(id int64) error
func (m *Manager) ActivateProfile(id int64) ([]AffectedFile, error)
func (m *Manager) ListProfiles() ([]storage.Profile, error)

func (m *Manager) CreateMCPServer(name, command, args, env string, targets map[string]bool) (int64, error)
func (m *Manager) UpdateMCPServer(id int64, ...) error
func (m *Manager) DeleteMCPServer(id int64) error
func (m *Manager) SyncMCPServers() ([]AffectedFile, error)

func (m *Manager) CreateSkill(name, sourcePath, description string, targets map[string]SkillTarget) (int64, error)
func (m *Manager) DeleteSkill(id int64) error
func (m *Manager) SyncSkills() ([]AffectedFile, error)

func (m *Manager) TriggerInboundSync() ([]SyncChange, error)
func (m *Manager) GetSyncStatus() (*SyncStatus, error)
func (m *Manager) ResolveConflict(tool, filePath, strategy string) error

func (m *Manager) ManualBackup() ([]AffectedFile, error)
func (m *Manager) RestoreBackup(backupID int64) ([]AffectedFile, error)

func (m *Manager) ListConfigFiles() []ConfigFileInfo
```

`ActivateProfile` implements the 5-step flow from the spec: backup → deactivate current → write each tool → rollback on failure → activate new.

Conflict resolution strategies (from spec):
- Provider config conflicts: external changes win automatically (auto-merge on inbound sync)
- MCP/Skills conflicts: return `ConflictError` to frontend, user decides via `ResolveConflict`

- [ ] **Step 4: Write test for Default profile auto-creation on first run**

```go
func TestManagerBootstrapDefaultProfile(t *testing.T) {
    db := openTestDB(t)
    defer db.Close()
    dir := t.TempDir()

    // Set up a Claude config with existing API key
    claudeDir := filepath.Join(dir, ".claude")
    os.MkdirAll(claudeDir, 0755)
    os.WriteFile(filepath.Join(claudeDir, "settings.json"),
        []byte(`{"env":{"ANTHROPIC_API_KEY":"sk-existing-key"}}`), 0644)

    mgr := NewManager(db, filepath.Join(dir, "backups"),
        WithClaudeAdapter(claudeDir, filepath.Join(dir, ".claude.json")),
    )

    // Bootstrap should auto-create a Default profile from existing configs
    err := mgr.Bootstrap()
    if err != nil { t.Fatal(err) }

    profiles, _ := mgr.ListProfiles()
    if len(profiles) != 1 { t.Fatalf("expected 1 default profile, got %d", len(profiles)) }
    if profiles[0].Name != "Default" { t.Fatal("expected profile named Default") }
    if !profiles[0].IsActive { t.Fatal("default profile should be active") }
}
```

- [ ] **Step 5: Implement Bootstrap() method**

`Bootstrap()` checks if `provider_profiles` table is empty. If so, reads existing configs from each installed adapter, creates a "Default" profile with the discovered settings, and activates it. Called once from `main.go` after Manager creation.

- [ ] **Step 6: Run tests**

Run: `go test ./internal/configmanager/ -run TestManager -v`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add internal/configmanager/manager.go internal/configmanager/manager_test.go
git commit -m "feat: add config manager orchestrator with profile activation and sync"
```

---

## Task 12: Update Server Infrastructure (CORS + Routing)

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: Update CORS middleware**

In `internal/server/server.go`, change line 27:
```go
// Before:
w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
// After:
w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
```

- [ ] **Step 2: Verify existing tests still pass**

Run: `go test ./internal/server/ -v` (if tests exist) or `go build ./...`
Expected: PASS / builds successfully

- [ ] **Step 3: Commit**

```bash
git add internal/server/server.go
git commit -m "fix: update CORS to allow POST/PUT/DELETE methods for config API"
```

---

## Task 13: Config API Handlers — Profiles

**Files:**
- Create: `internal/server/config_handlers.go`
- Modify: `internal/server/server.go` (add `mgr *configmanager.Manager` field, register profile routes)
- Modify: `main.go` (update `server.New` call)

Note: Update `server.New` signature and `main.go` call site in the same step to avoid compilation errors.

- [ ] **Step 1: Update Server struct and main.go together**

In `server.go`, add `mgr` field and update constructor:
```go
type Server struct {
    db   *storage.DB
    mgr  *configmanager.Manager
    addr string
}
func New(db *storage.DB, mgr *configmanager.Manager, addr string) *Server {
    return &Server{db: db, mgr: mgr, addr: addr}
}
```

In `main.go`, update the `server.New` call and add Manager wiring:
```go
backupDir, _ := configmanager.BackupBaseDir()
mgr := configmanager.NewManager(db, backupDir,
    configmanager.WithClaudeAdapter(...),
    configmanager.WithCodexAdapter(...),
    configmanager.WithOpenCodeAdapter(...),
    configmanager.WithOpenClawAdapter(...),
)
mgr.Bootstrap() // auto-create Default profile on first run
srv := server.New(db, mgr, addr)
```

Also add inbound sync ticker (30s) in the background goroutine section.

- [ ] **Step 2: Add JSON helpers to config_handlers.go**

```go
package server

import (
    "encoding/json"
    "net/http"
    "strconv"
)

func readJSON(r *http.Request, v interface{}) error {
    defer r.Body.Close()
    return json.NewDecoder(r.Body).Decode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string, details interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "error": code, "message": message, "details": details,
    })
}

func pathID(r *http.Request) (int64, error) {
    return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
```

- [ ] **Step 3: Register profile routes and implement handlers**

In `Handler()`, add:
```go
mux.HandleFunc("GET /api/config/profiles", s.handleListProfiles)
mux.HandleFunc("POST /api/config/profiles", s.handleCreateProfile)
mux.HandleFunc("PUT /api/config/profiles/{id}", s.handleUpdateProfile)
mux.HandleFunc("DELETE /api/config/profiles/{id}", s.handleDeleteProfile)
mux.HandleFunc("POST /api/config/profiles/{id}/activate", s.handleActivateProfile)
```

Request/response types for profile handlers:
```go
// POST /api/config/profiles
type createProfileReq struct {
    Name        string          `json:"name"`
    Config      string          `json:"config"`      // JSON string with api_key, base_url, model, model_map
    ToolTargets map[string]bool `json:"tool_targets"` // {"claude": true, "codex": false, ...}
}

// PUT /api/config/profiles/{id}
type updateProfileReq struct {
    Name   string `json:"name"`
    Config string `json:"config"`
}

// POST /api/config/profiles/{id}/activate response
type activateResp struct {
    AffectedFiles []configmanager.AffectedFile `json:"affected_files"`
}
```

Each handler: parse request → validate (400 on missing fields) → call `s.mgr.Method()` → on error, map to appropriate status code → `writeJSON` response.

- [ ] **Step 4: Build and verify**

Run: `go build ./...`
Expected: builds successfully

- [ ] **Step 5: Commit**

```bash
git add internal/server/config_handlers.go internal/server/server.go main.go
git commit -m "feat: add profile API handlers and wire config manager into server"
```

---

## Task 14: Config API Handlers — MCP, Skills, Sync, Backups, Files

**Files:**
- Modify: `internal/server/config_handlers.go` (add remaining handlers)
- Modify: `internal/server/server.go` (register remaining routes)

- [ ] **Step 1: Register all remaining routes**

```go
// MCP Servers
mux.HandleFunc("GET /api/config/mcp", s.handleListMCPServers)
mux.HandleFunc("POST /api/config/mcp", s.handleCreateMCPServer)
mux.HandleFunc("PUT /api/config/mcp/{id}", s.handleUpdateMCPServer)
mux.HandleFunc("DELETE /api/config/mcp/{id}", s.handleDeleteMCPServer)
mux.HandleFunc("PUT /api/config/mcp/{id}/targets", s.handleSetMCPTargets)

// Skills
mux.HandleFunc("GET /api/config/skills", s.handleListSkills)
mux.HandleFunc("POST /api/config/skills", s.handleCreateSkill)
mux.HandleFunc("PUT /api/config/skills/{id}", s.handleUpdateSkill)
mux.HandleFunc("DELETE /api/config/skills/{id}", s.handleDeleteSkill)
mux.HandleFunc("PUT /api/config/skills/{id}/targets", s.handleSetSkillTargets)

// Sync
mux.HandleFunc("POST /api/config/sync", s.handleTriggerSync)
mux.HandleFunc("GET /api/config/sync/status", s.handleSyncStatus)
mux.HandleFunc("POST /api/config/sync/resolve", s.handleResolveConflict)

// Backups
mux.HandleFunc("GET /api/config/backups", s.handleListBackups)
mux.HandleFunc("POST /api/config/backups", s.handleManualBackup)
mux.HandleFunc("POST /api/config/backups/{id}/restore", s.handleRestoreBackup)

// Files
mux.HandleFunc("GET /api/config/files", s.handleListConfigFiles)
```

- [ ] **Step 2: Implement MCP server handlers**

Request types:
```go
type createMCPReq struct {
    Name    string          `json:"name"`
    Command string          `json:"command"`
    Args    string          `json:"args"`    // JSON array string
    Env     string          `json:"env"`     // JSON object string
    Targets map[string]bool `json:"targets"`
}
type setTargetsReq struct {
    Targets map[string]bool `json:"targets"`
}
```

- [ ] **Step 3: Implement Skills, Sync, Backups, Files handlers**

Follow the same pattern. `handleListConfigFiles` calls `s.mgr.ListConfigFiles()` which returns file info with descriptions and doc URLs. `handleResolveConflict` accepts `{"tool":"...","file_path":"...","strategy":"keep_external|keep_ours"}`.

- [ ] **Step 4: Build and verify**

Run: `go build ./...`
Expected: builds successfully

- [ ] **Step 5: Commit**

```bash
git add internal/server/config_handlers.go internal/server/server.go
git commit -m "feat: add MCP, skills, sync, backups, and files API handlers"
```

---

## Task 15: Frontend API Layer Update

**Files:**
- Modify: `src/lib/api.ts`

- [ ] **Step 1: Add mutateAPI helper**

Add to `src/lib/api.ts`:

```typescript
export async function mutateAPI<T>(
  method: "POST" | "PUT" | "DELETE",
  path: string,
  body?: unknown
): Promise<T> {
  const port = await getPort();
  const res = await fetch(`http://127.0.0.1:${port}/api/${path}`, {
    method,
    headers: { "Content-Type": "application/json" },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: "UNKNOWN", message: `API error: ${res.status}` }));
    throw new ApiError(err.error, err.message, err.details, res.status);
  }
  return res.json();
}

export class ApiError extends Error {
  constructor(
    public code: string,
    message: string,
    public details: unknown,
    public status: number
  ) {
    super(message);
  }
}
```

- [ ] **Step 2: Verify existing pages still work**

Run: `npx tsc --noEmit` to check TypeScript compilation.
Expected: no errors

- [ ] **Step 3: Commit**

```bash
git add src/lib/api.ts
git commit -m "feat: add mutateAPI helper and ApiError class for config API"
```

---

## Task 16: Frontend Routing and Navigation

**Files:**
- Modify: `src/App.tsx`
- Modify: `src/components/Layout.tsx`
- Create: `src/pages/Config.tsx`
- Modify: `src/lib/locales/en.json`
- Modify: `src/lib/locales/zh.json`

- [ ] **Step 1: Add i18n keys for config page**

Add to both `en.json` and `zh.json` the config-related translation keys:

```json
// en.json additions:
"config": "Config",
"providers": "Providers",
"mcpServers": "MCP Servers",
"skills": "Skills",
"filesBackups": "Files & Backups",
"profileName": "Profile Name",
"apiKey": "API Key",
"baseUrl": "Base URL",
"activate": "Activate",
"active": "Active",
"save": "Save",
"delete": "Delete",
"cancel": "Cancel",
"create": "Create",
"edit": "Edit",
"syncStatus": "Sync Status",
"synced": "Synced",
"conflict": "Conflict",
"noConflicts": "All files in sync",
"affectedFiles": "Affected Files",
"confirmChanges": "Confirm Changes",
"backup": "Backup",
"restore": "Restore",
"backupHistory": "Backup History",
"triggerReason": "Trigger",
"slot": "Slot",
"openFolder": "Open Folder",
"description": "Description",
"command": "Command",
"arguments": "Arguments",
"envVars": "Environment Variables",
"syncTargets": "Sync Targets",
"syncMethod": "Sync Method",
"symlink": "Symlink",
"copy": "Copy",
"sourcePath": "Source Path",
"docLink": "Documentation",
"keepExternal": "Keep External",
"keepOurs": "Keep Ours",
"manualBackup": "Manual Backup",
"profileActivated": "Profile activated",
"noProfiles": "No profiles yet. Create one to get started.",
"noMCPServers": "No MCP servers configured.",
"noSkills": "No skills configured.",
"noBackups": "No backups yet."
```

```json
// zh.json additions:
"config": "配置管理",
"providers": "Provider 配置",
"mcpServers": "MCP 服务",
"skills": "Skills 技能",
"filesBackups": "文件与备份",
"profileName": "配置名称",
"apiKey": "API Key",
"baseUrl": "Base URL",
"activate": "激活",
"active": "已激活",
"save": "保存",
"delete": "删除",
"cancel": "取消",
"create": "新建",
"edit": "编辑",
"syncStatus": "同步状态",
"synced": "已同步",
"conflict": "有冲突",
"noConflicts": "所有文件已同步",
"affectedFiles": "影响的文件",
"confirmChanges": "确认修改",
"backup": "备份",
"restore": "恢复",
"backupHistory": "备份历史",
"triggerReason": "触发原因",
"slot": "槽位",
"openFolder": "打开目录",
"description": "描述",
"command": "命令",
"arguments": "参数",
"envVars": "环境变量",
"syncTargets": "同步目标",
"syncMethod": "同步方式",
"symlink": "符号链接",
"copy": "复制",
"sourcePath": "源路径",
"docLink": "文档",
"keepExternal": "保留外部修改",
"keepOurs": "使用我们的",
"manualBackup": "手动备份",
"profileActivated": "配置已激活",
"noProfiles": "暂无配置，点击新建开始。",
"noMCPServers": "暂无 MCP 服务配置。",
"noSkills": "暂无 Skills 配置。",
"noBackups": "暂无备份记录。"
```

- [ ] **Step 2: Add Config to navigation**

In `src/components/Layout.tsx`, add to `navItems`:
```typescript
{ path: "/config", label: "config" },
```

Also update the active state detection to support sub-routes. The existing code uses `location.pathname === item.path` which won't highlight "Config" when on `/config/providers`. Change to:
```typescript
const isActive = item.path === "/" 
  ? location.pathname === "/" 
  : location.pathname.startsWith(item.path);
```

- [ ] **Step 3: Create Config page shell with tab navigation**

```typescript
// src/pages/Config.tsx
import { NavLink, Outlet, Navigate, useLocation } from "react-router-dom";
import { useTranslation } from "react-i18next";

const tabs = [
  { path: "/config/providers", label: "providers" },
  { path: "/config/mcp", label: "mcpServers" },
  { path: "/config/skills", label: "skills" },
  { path: "/config/files", label: "filesBackups" },
];

export default function Config() {
  const { t } = useTranslation();
  const location = useLocation();

  if (location.pathname === "/config") {
    return <Navigate to="/config/providers" replace />;
  }

  return (
    <div className="flex flex-col gap-4">
      <nav className="flex gap-1 border-b border-border">
        {tabs.map((tab) => (
          <NavLink key={tab.path} to={tab.path}
            className={({ isActive }) =>
              `px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
                isActive ? "border-accent text-accent" : "border-transparent text-muted-foreground hover:text-foreground"
              }`
            }>
            {t(tab.label)}
          </NavLink>
        ))}
      </nav>
      <Outlet />
    </div>
  );
}
```

- [ ] **Step 4: Add routes to App.tsx**

```typescript
import Config from "./pages/Config";
// Lazy-load sub-pages or import directly
import Providers from "./pages/config/Providers";
import MCPServers from "./pages/config/MCPServers";
import Skills from "./pages/config/Skills";
import FilesBackups from "./pages/config/FilesBackups";

// In the Routes:
<Route path="/config" element={<Config />}>
  <Route path="providers" element={<Providers />} />
  <Route path="mcp" element={<MCPServers />} />
  <Route path="skills" element={<Skills />} />
  <Route path="files" element={<FilesBackups />} />
</Route>
```

- [ ] **Step 5: Create placeholder sub-pages**

Create minimal placeholder components for `Providers.tsx`, `MCPServers.tsx`, `Skills.tsx`, `FilesBackups.tsx` in `src/pages/config/` — each just renders a `<div>{t("tabName")}</div>`.

- [ ] **Step 6: Verify TypeScript compiles and navigation works**

Run: `npx tsc --noEmit`
Expected: no errors

- [ ] **Step 7: Commit**

```bash
git add src/pages/Config.tsx src/pages/config/ src/App.tsx src/components/Layout.tsx \
       src/lib/locales/en.json src/lib/locales/zh.json
git commit -m "feat: add config page with tab navigation and i18n"
```

---

## Task 17: Shared UI Components

**Files:**
- Create: `src/components/ConfirmPanel.tsx`
- Create: `src/components/SyncStatus.tsx`
- Create: `src/components/ToolTargets.tsx`

- [ ] **Step 1: Implement ToolTargets component**

Reusable checkbox group for selecting which tools to sync to. Props: `targets: Record<string, boolean>`, `onChange: (targets) => void`.

```typescript
const TOOLS = ["claude", "codex", "opencode", "openclaw"] as const;
const TOOL_LABELS: Record<string, string> = {
  claude: "Claude Code", codex: "Codex", opencode: "OpenCode", openclaw: "OpenClaw",
};
```

- [ ] **Step 2: Implement ConfirmPanel component**

Modal/panel that shows a list of `AffectedFile[]` with path, tool, operation. Has "Confirm" and "Cancel" buttons.

- [ ] **Step 3: Implement SyncStatus component**

Small indicator: green dot + "Synced" when no conflicts, orange dot + "N conflicts" when conflicts exist. Fetches from `GET /api/config/sync/status`.

- [ ] **Step 4: Verify TypeScript compiles**

Run: `npx tsc --noEmit`
Expected: no errors

- [ ] **Step 5: Commit**

```bash
git add src/components/ConfirmPanel.tsx src/components/SyncStatus.tsx src/components/ToolTargets.tsx
git commit -m "feat: add shared UI components for config management"
```

---

## Task 18: Providers Tab

**Files:**
- Modify: `src/pages/config/Providers.tsx`

- [ ] **Step 1: Implement Providers page**

Left panel: profile list fetched from `GET /api/config/profiles`. Active profile highlighted. "Create" button at top.

Right panel: edit form for selected profile — name, API key (masked in display, full in edit), base URL, model. Tool targets checkboxes below.

"Activate" button triggers `POST /api/config/profiles/{id}/activate`, shows ConfirmPanel with affected files before executing.

Uses `fetchRaw` for GET, `mutateAPI` for POST/PUT/DELETE.

- [ ] **Step 2: Verify TypeScript compiles**

Run: `npx tsc --noEmit`

- [ ] **Step 3: Commit**

```bash
git add src/pages/config/Providers.tsx
git commit -m "feat: implement providers tab with profile CRUD and activation"
```

---

## Task 19: MCP Servers Tab

**Files:**
- Modify: `src/pages/config/MCPServers.tsx`

- [ ] **Step 1: Implement MCP Servers page**

Table/list of MCP servers. Each row: name, command, enabled toggle, tool target checkboxes. Add/Edit opens a form (inline or modal) with name, command, args (JSON array editor), env (key-value editor).

Save triggers `POST /api/config/mcp` or `PUT /api/config/mcp/{id}`, shows ConfirmPanel.

- [ ] **Step 2: Verify TypeScript compiles**

Run: `npx tsc --noEmit`

- [ ] **Step 3: Commit**

```bash
git add src/pages/config/MCPServers.tsx
git commit -m "feat: implement MCP servers tab with unified management"
```

---

## Task 20: Skills Tab

**Files:**
- Modify: `src/pages/config/Skills.tsx`

- [ ] **Step 1: Implement Skills page**

List of skills. Each row: name, description, source path, sync method (symlink/copy dropdown), tool target checkboxes, "Open Folder" button (calls Tauri `shell.open()`).

Add skill: input source path (local directory), name, description. Tool targets with method selection.

Note: GitHub URL skill import is deferred to a future iteration. For now, only local directory paths are supported. The UI should have a disabled "Import from GitHub" button with a "Coming soon" tooltip.

- [ ] **Step 2: Verify TypeScript compiles**

Run: `npx tsc --noEmit`

- [ ] **Step 3: Commit**

```bash
git add src/pages/config/Skills.tsx
git commit -m "feat: implement skills tab with unified management and folder open"
```

---

## Task 21: Files & Backups Tab

**Files:**
- Modify: `src/pages/config/FilesBackups.tsx`

- [ ] **Step 1: Implement Files section**

Top half: table of all managed config files from `GET /api/config/files`. Columns: tool icon, file path, description, doc link (external link), sync status, "Open Folder" button.

Conflict rows highlighted in orange. Click to expand diff view with "Keep External" / "Keep Ours" buttons.

- [ ] **Step 2: Implement Backups section**

Bottom half: backup history from `GET /api/config/backups`. Columns: time, tool, file, trigger reason, slot. "Restore" button per row. "Manual Backup" button at top.

- [ ] **Step 3: Verify TypeScript compiles**

Run: `npx tsc --noEmit`

- [ ] **Step 4: Commit**

```bash
git add src/pages/config/FilesBackups.tsx
git commit -m "feat: implement files & backups tab with restore and conflict resolution"
```

---

## Task 22: Integration Testing and Polish

**Files:**
- All files from previous tasks

- [ ] **Step 1: Run all Go tests**

Run: `go test ./... -v`
Expected: ALL PASS

- [ ] **Step 2: Run TypeScript type check**

Run: `npx tsc --noEmit`
Expected: no errors

- [ ] **Step 3: Build Go binary**

Run: `go build -o agent-usage-desktop .`
Expected: builds successfully

- [ ] **Step 4: Start dev server and test manually**

Run: `./agent-usage-desktop --port 9800` in one terminal.
Test each API endpoint with curl:
```bash
# Create profile
curl -X POST http://127.0.0.1:9800/api/config/profiles \
  -H "Content-Type: application/json" \
  -d '{"name":"test","config":"{\"api_key\":\"sk-test\",\"base_url\":\"https://api.anthropic.com\"}"}'

# List profiles
curl http://127.0.0.1:9800/api/config/profiles

# List config files
curl http://127.0.0.1:9800/api/config/files

# Sync status
curl http://127.0.0.1:9800/api/config/sync/status
```

- [ ] **Step 5: Test frontend in browser**

Run: `npx tauri dev` and navigate to `/config`. Verify:
- Tab navigation works (URL changes, back button works)
- Providers tab: can create, edit, delete, activate profiles
- MCP tab: can add, edit, remove MCP servers with tool targets
- Skills tab: can add skills, open folder works
- Files tab: shows all config files with descriptions and doc links
- Backups tab: shows history, manual backup works

- [ ] **Step 6: Final commit**

```bash
git add -A
git commit -m "feat: config manager integration testing and polish"
```
