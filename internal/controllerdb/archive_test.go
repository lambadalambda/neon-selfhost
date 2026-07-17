package controllerdb

import (
	"archive/tar"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateVolumeArchiveAcceptsControllerDatabases(t *testing.T) {
	dir := createControllerDataDir(t, "archive")
	archivePath := filepath.Join(t.TempDir(), "controller-state.tar")
	writeTestArchive(t, archivePath, map[string]string{
		"controller.db": filepath.Join(dir, "controller.db"),
		"operations.db": filepath.Join(dir, "operations.db"),
	})

	if err := ValidateVolumeArchive(archivePath); err != nil {
		t.Fatalf("validate archive: %v", err)
	}
}

func TestValidateVolumeArchiveRejectsMissingDatabase(t *testing.T) {
	dir := createControllerDataDir(t, "archive")
	archivePath := filepath.Join(t.TempDir(), "controller-state.tar")
	writeTestArchive(t, archivePath, map[string]string{
		"controller.db": filepath.Join(dir, "controller.db"),
	})

	if err := ValidateVolumeArchive(archivePath); err == nil {
		t.Fatal("expected missing operations database error")
	}
}

func TestValidateVolumeArchiveRejectsDatabaseSymlink(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "controller-state.tar")
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	w := tar.NewWriter(archive)
	if err := w.WriteHeader(&tar.Header{Name: "controller.db", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}); err != nil {
		t.Fatalf("write symlink header: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}

	if err := ValidateVolumeArchive(archivePath); err == nil {
		t.Fatal("expected symlink rejection")
	}
}

func TestValidateVolumeArchiveRejectsEmptyDatabases(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "controller-state.tar")
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	w := tar.NewWriter(archive)
	for _, name := range []string{"controller.db", "operations.db"} {
		if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: 0}); err != nil {
			t.Fatalf("write empty database header: %v", err)
		}
	}
	_ = w.Close()
	_ = archive.Close()

	if err := ValidateVolumeArchive(archivePath); err == nil {
		t.Fatal("expected empty database rejection")
	}
}

func TestValidateVolumeArchiveRejectsUnrelatedSQLiteDatabases(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"controller.db", "operations.db"} {
		db, err := sql.Open("sqlite", filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("open unrelated database: %v", err)
		}
		if _, err := db.Exec(`CREATE TABLE unrelated (value TEXT)`); err != nil {
			t.Fatalf("create unrelated table: %v", err)
		}
		_ = db.Close()
	}
	archivePath := filepath.Join(t.TempDir(), "controller-state.tar")
	writeTestArchive(t, archivePath, map[string]string{
		"controller.db": filepath.Join(dir, "controller.db"),
		"operations.db": filepath.Join(dir, "operations.db"),
	})

	if err := ValidateVolumeArchive(archivePath); err == nil {
		t.Fatal("expected unrelated database rejection")
	}
}

func writeTestArchive(t *testing.T, archivePath string, files map[string]string) {
	t.Helper()
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	w := tar.NewWriter(archive)
	for name, source := range files {
		content, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read archive source: %v", err)
		}
		if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content))}); err != nil {
			t.Fatalf("write archive header: %v", err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("write archive content: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
}
