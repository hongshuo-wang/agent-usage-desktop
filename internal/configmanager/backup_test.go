package configmanager

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupManagerCreateAndRestore(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bm := NewBackupManager(dir)

	srcFile := filepath.Join(dir, "settings.json")
	original := []byte(`{"original": true}`)
	if err := os.WriteFile(srcFile, original, 0o644); err != nil {
		t.Fatalf("os.WriteFile() setup error = %v", err)
	}

	slot, backupPath, err := bm.Backup("claude", "settings.json", srcFile, "auto")
	if err != nil {
		t.Fatalf("Backup() error = %v", err)
	}
	if slot != 1 {
		t.Fatalf("slot = %d, want 1", slot)
	}

	backupContent, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("os.ReadFile(backupPath) error = %v", err)
	}
	if string(backupContent) != string(original) {
		t.Fatalf("backup content mismatch: got %q, want %q", backupContent, original)
	}

	metaPath := backupPath[:len(backupPath)-4] + ".meta"
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("os.ReadFile(metaPath) error = %v", err)
	}

	var meta backupMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("json.Unmarshal(meta) error = %v", err)
	}
	if meta.Trigger != "auto" {
		t.Fatalf("meta trigger = %q, want %q", meta.Trigger, "auto")
	}

	wantHash := hashBytes(original)
	if meta.Hash != wantHash {
		t.Fatalf("meta hash = %q, want %q", meta.Hash, wantHash)
	}
	if meta.Tool != "claude" {
		t.Fatalf("meta tool = %q, want %q", meta.Tool, "claude")
	}
	if meta.FileName != "settings.json" {
		t.Fatalf("meta file_name = %q, want %q", meta.FileName, "settings.json")
	}

	modified := []byte(`{"original": false}`)
	if err := os.WriteFile(srcFile, modified, 0o644); err != nil {
		t.Fatalf("os.WriteFile() modify error = %v", err)
	}

	if err := bm.Restore(backupPath, srcFile); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	restored, err := os.ReadFile(srcFile)
	if err != nil {
		t.Fatalf("os.ReadFile(srcFile) error = %v", err)
	}
	if string(restored) != string(original) {
		t.Fatalf("restored content mismatch: got %q, want %q", restored, original)
	}
}

func TestBackupManagerRolling(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bm := NewBackupManager(dir)

	srcFile := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(srcFile, []byte(`{"k":1}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() setup error = %v", err)
	}

	expected := []int{1, 2, 3, 4, 5, 1, 2}
	for i, want := range expected {
		slot, _, err := bm.Backup("claude", "settings.json", srcFile, "auto")
		if err != nil {
			t.Fatalf("Backup() #%d error = %v", i+1, err)
		}
		if slot != want {
			t.Fatalf("Backup() #%d slot = %d, want %d", i+1, slot, want)
		}
	}
}

func TestBackupManagerRollingIgnoresMtimeTies(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bm := NewBackupManager(dir)

	srcFile := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(srcFile, []byte(`{"k":1}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() setup error = %v", err)
	}

	for i := 0; i < 7; i++ {
		slot, _, err := bm.Backup("claude", "settings.json", srcFile, "auto")
		if err != nil {
			t.Fatalf("Backup() #%d error = %v", i+1, err)
		}
		_ = slot
	}

	backupFiles, err := filepath.Glob(filepath.Join(dir, "*.bak"))
	if err != nil {
		t.Fatalf("filepath.Glob() error = %v", err)
	}
	if len(backupFiles) == 0 {
		t.Fatalf("expected backup files")
	}

	tiedTime := time.Unix(1_700_000_000, 0)
	for _, path := range backupFiles {
		if err := os.Chtimes(path, tiedTime, tiedTime); err != nil {
			t.Fatalf("os.Chtimes(%q) error = %v", path, err)
		}
	}

	slot, _, err := bm.Backup("claude", "settings.json", srcFile, "auto")
	if err != nil {
		t.Fatalf("Backup() after tie error = %v", err)
	}
	if slot != 3 {
		t.Fatalf("slot after tied mtimes = %d, want 3", slot)
	}
}

func TestBackupManagerListBackupsRawNames(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	bm := NewBackupManager(dir)

	srcFile := filepath.Join(dir, "settings prod.json")
	content := []byte(`{"enabled": true}`)
	if err := os.WriteFile(srcFile, content, 0o644); err != nil {
		t.Fatalf("os.WriteFile() setup error = %v", err)
	}

	tool := "claude__code"
	fileName := "settings prod.json"
	if _, _, err := bm.Backup(tool, fileName, srcFile, "manual"); err != nil {
		t.Fatalf("Backup() error = %v", err)
	}

	backups, err := bm.ListBackups(tool)
	if err != nil {
		t.Fatalf("ListBackups() error = %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("len(backups) = %d, want 1", len(backups))
	}

	if backups[0].Tool != tool {
		t.Fatalf("backups[0].Tool = %q, want %q", backups[0].Tool, tool)
	}
	if backups[0].FileName != fileName {
		t.Fatalf("backups[0].FileName = %q, want %q", backups[0].FileName, fileName)
	}
}

func hashBytes(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
