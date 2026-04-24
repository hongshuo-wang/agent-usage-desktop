package configmanager

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

func TestAtomicWrite(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "nested", "config.json")
	want := []byte(`{"model":"gpt-5","enabled":true}`)

	if err := AtomicWrite(path, want); err != nil {
		t.Fatalf("AtomicWrite() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	if string(got) != string(want) {
		t.Fatalf("content mismatch: got %q, want %q", got, want)
	}
}

func TestAtomicWriteOverwrite(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "config.json")

	if err := os.WriteFile(path, []byte(`{"old":true}`), 0o644); err != nil {
		t.Fatalf("os.WriteFile() setup error = %v", err)
	}

	want := []byte(`{"new":true}`)
	if err := AtomicWrite(path, want); err != nil {
		t.Fatalf("AtomicWrite() overwrite error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile() error = %v", err)
	}

	if string(got) != string(want) {
		t.Fatalf("overwrite mismatch: got %q, want %q", got, want)
	}
}

func TestFileHash(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "hash.txt")

	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	first, err := FileHash(path)
	if err != nil {
		t.Fatalf("FileHash() error = %v", err)
	}

	const helloSHA256 = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if first != helloSHA256 {
		t.Fatalf("hash mismatch: got %q, want %q", first, helloSHA256)
	}

	if len(first) != 64 {
		t.Fatalf("hash length = %d, want 64", len(first))
	}

	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(first) {
		t.Fatalf("hash is not valid lowercase SHA-256 hex: %q", first)
	}

	second, err := FileHash(path)
	if err != nil {
		t.Fatalf("FileHash() second call error = %v", err)
	}

	if first != second {
		t.Fatalf("hash not deterministic: first=%q second=%q", first, second)
	}
}

func TestFileHashDirectoryChangesWhenContentsChange(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "skill")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile initial: %v", err)
	}

	first, err := FileHash(root)
	if err != nil {
		t.Fatalf("FileHash first: %v", err)
	}
	second, err := FileHash(root)
	if err != nil {
		t.Fatalf("FileHash second: %v", err)
	}
	if first != second {
		t.Fatalf("directory hash not deterministic: %q != %q", first, second)
	}

	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("goodbye"), 0o644); err != nil {
		t.Fatalf("WriteFile updated: %v", err)
	}
	third, err := FileHash(root)
	if err != nil {
		t.Fatalf("FileHash third: %v", err)
	}
	if third == first {
		t.Fatalf("directory hash did not change after content update")
	}
}

func TestWithFileLockExecutesFn(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	lockPath := filepath.Join(tempDir, "lock", "config.lock")

	called := false
	err := WithFileLock(lockPath, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("WithFileLock() error = %v", err)
	}

	if !called {
		t.Fatalf("expected lock callback to execute")
	}
}

func TestWithFileLockMutualExclusion(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	lockPath := filepath.Join(tempDir, "lock", "config.lock")

	firstEntered := make(chan struct{})
	firstRelease := make(chan struct{})
	secondEntered := make(chan struct{})
	secondStarted := make(chan struct{})
	firstErr := make(chan error, 1)
	secondErr := make(chan error, 1)

	go func() {
		firstErr <- WithFileLock(lockPath, func() error {
			close(firstEntered)
			<-firstRelease
			return nil
		})
	}()

	select {
	case <-firstEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("first lock holder did not enter callback in time")
	}

	go func() {
		close(secondStarted)
		secondErr <- WithFileLock(lockPath, func() error {
			close(secondEntered)
			return nil
		})
	}()

	select {
	case <-secondStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("second lock attempt did not start in time")
	}

	select {
	case <-secondEntered:
		t.Fatal("second callback executed before first lock released")
	case <-time.After(150 * time.Millisecond):
	}

	close(firstRelease)

	select {
	case <-secondEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("second callback did not execute after first lock release")
	}

	select {
	case err := <-firstErr:
		if err != nil {
			t.Fatalf("first WithFileLock() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first WithFileLock() did not return in time")
	}

	select {
	case err := <-secondErr:
		if err != nil {
			t.Fatalf("second WithFileLock() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second WithFileLock() did not return in time")
	}
}
