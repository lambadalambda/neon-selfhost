package preflight

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var (
	ErrDataDirNotDirectory = errors.New("controller data dir is not a directory")
	ErrDataDirNotWritable  = errors.New("controller data dir is not writable")
)

func CheckControllerDataDir(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}

	if err := os.MkdirAll(path, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) || errors.Is(err, syscall.ENOTDIR) {
			return fmt.Errorf("%w: %v", ErrDataDirNotDirectory, err)
		}
		return fmt.Errorf("%w: %v", ErrDataDirNotWritable, err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDataDirNotWritable, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%w: %s", ErrDataDirNotDirectory, path)
	}

	tmp, err := os.CreateTemp(path, ".preflight-*.tmp")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDataDirNotWritable, err)
	}

	tmpPath := tmp.Name()
	if closeErr := tmp.Close(); closeErr != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("%w: %v", ErrDataDirNotWritable, closeErr)
	}

	if err := os.Remove(tmpPath); err != nil {
		return fmt.Errorf("%w: %v", ErrDataDirNotWritable, err)
	}

	return nil
}

func StateFilePath(dataDir string) string {
	return filepath.Join(dataDir, "branches.json")
}
