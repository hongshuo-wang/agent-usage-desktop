//go:build windows

package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsReplaceFailurePreservesExistingFile(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "config.yaml")
	tempPath := filepath.Join(dir, "config.tmp")
	original := []byte("old config")
	if err := os.WriteFile(targetPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tempPath, []byte("new config"), 0o600); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("replace failed")
	originalMove := moveConfigFile
	moveConfigFile = func(_, _ string) error { return wantErr }
	t.Cleanup(func() { moveConfigFile = originalMove })

	err := replaceConfigFile(tempPath, targetPath)
	if !errors.Is(err, wantErr) {
		t.Fatalf("replace error=%v, want %v", err, wantErr)
	}
	got, readErr := os.ReadFile(targetPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("existing config changed after replacement failure: %q", got)
	}
}
