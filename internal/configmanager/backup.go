package configmanager

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

const maxBackupSlots = 5

type BackupManager struct {
	baseDir string
}

type BackupInfo struct {
	Tool     string    `json:"tool"`
	FileName string    `json:"file_name"`
	Slot     int       `json:"slot"`
	Path     string    `json:"path"`
	Time     time.Time `json:"time"`
	Trigger  string    `json:"trigger"`
	Hash     string    `json:"hash"`
}

type backupMeta struct {
	Time       time.Time `json:"time"`
	Trigger    string    `json:"trigger"`
	Hash       string    `json:"hash"`
	Tool       string    `json:"tool"`
	FileName   string    `json:"file_name"`
	SourcePath string    `json:"source_path,omitempty"`
	Slot       int       `json:"slot"`
}

type backupState struct {
	LastSlot int `json:"last_slot"`
}

func NewBackupManager(baseDir string) *BackupManager {
	return &BackupManager{baseDir: baseDir}
}

func BackupBaseDir() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	return filepath.Join(cfgDir, "agent-usage", "backups"), nil
}

func (bm *BackupManager) Backup(tool, fileName, srcPath, trigger string) (slot int, backupPath string, err error) {
	if err := os.MkdirAll(bm.baseDir, 0o755); err != nil {
		return 0, "", fmt.Errorf("create backup directory: %w", err)
	}

	prefix := backupPrefix(tool, fileName)
	lockPath := filepath.Join(bm.baseDir, prefix+".lock")

	err = WithFileLock(lockPath, func() error {
		lastSlot, err := bm.readLastSlot(prefix)
		if err != nil {
			return err
		}

		slot = (lastSlot % maxBackupSlots) + 1
		backupPath = filepath.Join(bm.baseDir, fmt.Sprintf("%s.%d.bak", prefix, slot))

		content, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read source file: %w", err)
		}
		if err := AtomicWrite(backupPath, content); err != nil {
			return fmt.Errorf("write backup file: %w", err)
		}

		hash := hashContent(content)
		metaBytes, err := json.Marshal(backupMeta{
			Time:       time.Now().UTC(),
			Trigger:    trigger,
			Hash:       hash,
			Tool:       tool,
			FileName:   fileName,
			SourcePath: srcPath,
			Slot:       slot,
		})
		if err != nil {
			return fmt.Errorf("marshal backup metadata: %w", err)
		}

		metaPath := strings.TrimSuffix(backupPath, ".bak") + ".meta"
		if err := AtomicWrite(metaPath, metaBytes); err != nil {
			return fmt.Errorf("write backup metadata: %w", err)
		}

		if err := bm.writeLastSlot(prefix, slot); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return 0, "", err
	}

	return slot, backupPath, nil
}

func (bm *BackupManager) Restore(backupPath, destPath string) error {
	content, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("read backup file: %w", err)
	}

	if err := AtomicWrite(destPath, content); err != nil {
		return fmt.Errorf("restore destination file: %w", err)
	}

	return nil
}

func (bm *BackupManager) ListBackups(tool string) ([]BackupInfo, error) {
	entries, err := filepath.Glob(filepath.Join(bm.baseDir, "*.bak"))
	if err != nil {
		return nil, fmt.Errorf("list backup files: %w", err)
	}

	var backups []BackupInfo
	for _, path := range entries {
		metaPath := strings.TrimSuffix(path, ".bak") + ".meta"
		metaBytes, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}

		var meta backupMeta
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			continue
		}

		if tool != "" && meta.Tool != tool {
			continue
		}

		slot := meta.Slot
		if slot == 0 {
			slot = parseSlotFromBackupPath(path)
		}

		backups = append(backups, BackupInfo{
			Tool:     meta.Tool,
			FileName: meta.FileName,
			Slot:     slot,
			Path:     path,
			Time:     meta.Time,
			Trigger:  meta.Trigger,
			Hash:     meta.Hash,
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Path < backups[j].Path
	})

	return backups, nil
}

func (bm *BackupManager) readLastSlot(prefix string) (int, error) {
	statePath := bm.statePath(prefix)
	stateBytes, err := os.ReadFile(statePath)
	if err == nil {
		var state backupState
		if err := json.Unmarshal(stateBytes, &state); err != nil {
			return 0, fmt.Errorf("decode backup state: %w", err)
		}
		if state.LastSlot >= 1 && state.LastSlot <= maxBackupSlots {
			return state.LastSlot, nil
		}
	}
	if err != nil && !os.IsNotExist(err) {
		return 0, fmt.Errorf("read backup state: %w", err)
	}

	return bm.inferLastSlotFromExisting(prefix)
}

func (bm *BackupManager) writeLastSlot(prefix string, slot int) error {
	stateBytes, err := json.Marshal(backupState{LastSlot: slot})
	if err != nil {
		return fmt.Errorf("marshal backup state: %w", err)
	}

	if err := AtomicWrite(bm.statePath(prefix), stateBytes); err != nil {
		return fmt.Errorf("write backup state: %w", err)
	}

	return nil
}

func (bm *BackupManager) inferLastSlotFromExisting(prefix string) (int, error) {
	metaFiles, err := filepath.Glob(filepath.Join(bm.baseDir, prefix+".*.meta"))
	if err != nil {
		return 0, fmt.Errorf("scan backup metadata files: %w", err)
	}

	var latestMetaTime time.Time
	latestMetaSlot := 0
	foundMeta := false

	for _, metaPath := range metaFiles {
		metaBytes, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}

		var meta backupMeta
		if err := json.Unmarshal(metaBytes, &meta); err != nil {
			continue
		}

		slot := meta.Slot
		if slot == 0 {
			slot = parseSlotFromBackupPath(strings.TrimSuffix(metaPath, ".meta") + ".bak")
		}
		if slot < 1 || slot > maxBackupSlots {
			continue
		}

		if !foundMeta || meta.Time.After(latestMetaTime) || (meta.Time.Equal(latestMetaTime) && slot > latestMetaSlot) {
			foundMeta = true
			latestMetaTime = meta.Time
			latestMetaSlot = slot
		}
	}

	if foundMeta {
		return latestMetaSlot, nil
	}

	backupFiles, err := filepath.Glob(filepath.Join(bm.baseDir, prefix+".*.bak"))
	if err != nil {
		return 0, fmt.Errorf("scan backup files: %w", err)
	}

	maxSlot := 0
	for _, path := range backupFiles {
		slot := parseSlotFromBackupPath(path)
		if slot > maxSlot {
			maxSlot = slot
		}
	}

	return maxSlot, nil
}

func (bm *BackupManager) statePath(prefix string) string {
	return filepath.Join(bm.baseDir, prefix+".state")
}

func parseSlotFromBackupPath(path string) int {
	base := filepath.Base(path)
	if !strings.HasSuffix(base, ".bak") {
		return 0
	}

	name := strings.TrimSuffix(base, ".bak")
	idx := strings.LastIndex(name, ".")
	if idx == -1 || idx == len(name)-1 {
		return 0
	}

	slot, err := strconv.Atoi(name[idx+1:])
	if err != nil {
		return 0
	}

	return slot
}

func backupPrefix(tool, fileName string) string {
	safeTool := sanitizeName(tool)
	safeFile := sanitizeName(fileName)

	hasher := sha256.Sum256([]byte(tool + "\n" + fileName))
	shortHash := hex.EncodeToString(hasher[:4])

	return fmt.Sprintf("%s__%s__%s", safeTool, safeFile, shortHash)
}

func hashContent(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

func sanitizeName(value string) string {
	if value == "" {
		return "unknown"
	}

	var b strings.Builder
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}

	result := strings.Trim(b.String(), "._-")
	if result == "" {
		return "unknown"
	}
	return result
}
