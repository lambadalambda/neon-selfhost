package controllerdb

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func ValidateVolumeArchive(archivePath string) error {
	archivePath = strings.TrimSpace(archivePath)
	if archivePath == "" {
		return errors.New("controller volume archive path is required")
	}
	archive, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archive.Close()

	extractDir, err := os.MkdirTemp("", "controller-volume-validate-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(extractDir)

	allowed := map[string]bool{
		"controller.db":     true,
		"controller.db-wal": true,
		"controller.db-shm": true,
		"operations.db":     true,
		"operations.db-wal": true,
		"operations.db-shm": true,
	}
	seen := make(map[string]bool, len(allowed))
	r := tar.NewReader(archive)
	for {
		header, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read controller volume archive: %w", err)
		}

		name := strings.TrimPrefix(header.Name, "./")
		name = path.Clean(name)
		if path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("unsafe path in controller volume archive: %s", header.Name)
		}
		if !allowed[name] {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			return fmt.Errorf("controller volume archive entry %s is not a regular file", name)
		}

		destination := filepath.Join(extractDir, name)
		file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("extract controller volume archive entry %s: %w", name, err)
		}
		_, copyErr := io.Copy(file, r)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			return errors.Join(copyErr, closeErr)
		}
		seen[name] = true
	}

	for _, name := range databaseNames {
		if !seen[name] {
			return fmt.Errorf("controller volume archive is missing %s", name)
		}
		if err := validateSQLite(filepath.Join(extractDir, name)); err != nil {
			return fmt.Errorf("validate %s in controller volume archive: %w", name, err)
		}
		if err := validateDatabaseIdentity(filepath.Join(extractDir, name), name); err != nil {
			return fmt.Errorf("validate %s identity in controller volume archive: %w", name, err)
		}
	}
	return nil
}
