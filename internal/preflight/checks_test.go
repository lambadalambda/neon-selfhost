package preflight

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckControllerDataDirNoopWhenUnset(t *testing.T) {
	if err := CheckControllerDataDir(""); err != nil {
		t.Fatalf("expected nil error for empty path, got %v", err)
	}
}

func TestCheckControllerDataDirCreatesMissingDirectory(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "controller-state")

	if err := CheckControllerDataDir(path); err != nil {
		t.Fatalf("expected preflight to pass, got %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat controller data dir: %v", err)
	}

	if !info.IsDir() {
		t.Fatalf("expected %q to be directory", path)
	}
}

func TestCheckControllerDataDirRejectsFilePath(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "not-a-dir")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	err := CheckControllerDataDir(path)
	if !errors.Is(err, ErrDataDirNotDirectory) {
		t.Fatalf("expected %v, got %v", ErrDataDirNotDirectory, err)
	}
}

func TestCheckControllerDataDirDetectsNotWritableDirectory(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "readonly")
	if err := os.Mkdir(path, 0o555); err != nil {
		t.Fatalf("mkdir readonly: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(path, 0o755)
	})

	err := CheckControllerDataDir(path)
	if !errors.Is(err, ErrDataDirNotWritable) {
		t.Fatalf("expected %v, got %v", ErrDataDirNotWritable, err)
	}
}
