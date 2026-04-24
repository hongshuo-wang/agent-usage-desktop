package configmanager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hongshuo-wang/agent-usage-desktop/internal/storage"
)

type ConflictError struct {
	Tool     string
	FilePath string
	Expected string
	Actual   string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("sync conflict for %s (%s): expected hash %s, got %s", e.Tool, e.FilePath, e.Expected, e.Actual)
}

type SyncEngine struct {
	db     *storage.DB
	backup *BackupManager
}

type SyncChange struct {
	Tool     string
	FilePath string
	OldHash  string
	NewHash  string
	Exists   bool
}

func NewSyncEngine(db *storage.DB, backup *BackupManager) *SyncEngine {
	return &SyncEngine{
		db:     db,
		backup: backup,
	}
}

func (se *SyncEngine) OutboundSync(tool, filePath string, newContent []byte) ([]AffectedFile, error) {
	var affected []AffectedFile

	lockPath := filePath + ".lock"
	err := WithFileLock(lockPath, func() error {
		exists, currentHash, err := fileHashIfExists(filePath)
		if err != nil {
			return err
		}

		state, err := se.db.GetSyncState(tool, filePath)
		if err != nil {
			return fmt.Errorf("load sync state: %w", err)
		}
		if state != nil && state.LastHash != currentHash {
			return &ConflictError{
				Tool:     tool,
				FilePath: filePath,
				Expected: state.LastHash,
				Actual:   currentHash,
			}
		}

		if exists {
			if _, _, err := se.backup.Backup(tool, filepath.Base(filePath), filePath, "outbound"); err != nil {
				return fmt.Errorf("backup file: %w", err)
			}
		}

		if err := AtomicWrite(filePath, newContent); err != nil {
			return fmt.Errorf("write file: %w", err)
		}

		newHash, err := FileHash(filePath)
		if err != nil {
			return fmt.Errorf("hash new content: %w", err)
		}

		if err := se.db.UpsertSyncState(tool, filePath, newHash, time.Now().UTC(), "outbound"); err != nil {
			return fmt.Errorf("save sync state: %w", err)
		}

		op := "created"
		if exists {
			op = "modified"
		}
		affected = []AffectedFile{{
			Path:      filePath,
			Tool:      tool,
			Operation: op,
		}}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return affected, nil
}

func (se *SyncEngine) ForceWrite(tool, filePath string, content []byte) ([]AffectedFile, error) {
	var affected []AffectedFile

	lockPath := filePath + ".lock"
	err := WithFileLock(lockPath, func() error {
		exists, _, err := fileHashIfExists(filePath)
		if err != nil {
			return err
		}

		if exists {
			if _, _, err := se.backup.Backup(tool, filepath.Base(filePath), filePath, "force"); err != nil {
				return fmt.Errorf("backup file: %w", err)
			}
		}

		if err := AtomicWrite(filePath, content); err != nil {
			return fmt.Errorf("write file: %w", err)
		}

		newHash, err := FileHash(filePath)
		if err != nil {
			return fmt.Errorf("hash written content: %w", err)
		}

		if err := se.db.UpsertSyncState(tool, filePath, newHash, time.Now().UTC(), "outbound"); err != nil {
			return fmt.Errorf("save sync state: %w", err)
		}

		op := "created"
		if exists {
			op = "modified"
		}
		affected = []AffectedFile{{
			Path:      filePath,
			Tool:      tool,
			Operation: op,
		}}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return affected, nil
}

func (se *SyncEngine) InboundScan(tool string, adapter Adapter) ([]SyncChange, error) {
	files := adapter.ConfigFiles()
	changes := make([]SyncChange, 0, len(files))

	for _, file := range files {
		exists, currentHash, err := fileHashIfExists(file.Path)
		if err != nil {
			return nil, err
		}

		state, err := se.db.GetSyncState(tool, file.Path)
		if err != nil {
			return nil, fmt.Errorf("load sync state for %s: %w", file.Path, err)
		}

		if !exists && state == nil {
			continue
		}

		if state == nil || state.LastHash != currentHash {
			oldHash := ""
			if state != nil {
				oldHash = state.LastHash
			}
			changes = append(changes, SyncChange{
				Tool:     tool,
				FilePath: file.Path,
				OldHash:  oldHash,
				NewHash:  currentHash,
				Exists:   exists,
			})
		}
	}

	return changes, nil
}

func (se *SyncEngine) AcceptInbound(tool, filePath string) error {
	lockPath := filePath + ".lock"
	return WithFileLock(lockPath, func() error {
		_, currentHash, err := fileHashIfExists(filePath)
		if err != nil {
			return err
		}

		if err := se.db.UpsertSyncState(tool, filePath, currentHash, time.Now().UTC(), "inbound"); err != nil {
			return fmt.Errorf("save inbound sync state: %w", err)
		}
		return nil
	})
}

func fileHashIfExists(path string) (exists bool, hash string, err error) {
	hash, err = FileHash(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || os.IsNotExist(err) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("hash file: %w", err)
	}
	return true, hash, nil
}
