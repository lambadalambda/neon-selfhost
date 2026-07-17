package controllerdb

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestAcquireLockRejectsSecondControllerDataUser(t *testing.T) {
	dataDir := t.TempDir()
	first, err := AcquireLock(dataDir)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	defer first.Close()

	_, err = AcquireLock(dataDir)
	if !errors.Is(err, ErrDataDirInUse) {
		t.Fatalf("expected data-dir-in-use error, got %v", err)
	}
}

func TestRestoreRefusesControllerDataDirectoryHeldByRuntime(t *testing.T) {
	dataDir := createControllerDataDir(t, "old")
	backupDir := createControllerDataDir(t, "new")
	lock, err := AcquireLock(dataDir)
	if err != nil {
		t.Fatalf("acquire runtime lock: %v", err)
	}
	defer lock.Close()

	err = Restore(dataDir, backupDir)
	if !errors.Is(err, ErrDataDirInUse) {
		t.Fatalf("expected data-dir-in-use error, got %v", err)
	}
	assertDatabaseValue(t, filepath.Join(dataDir, "controller.db"), "old-controller")
}

func TestRestoreReplacesBothDatabasesFromValidatedCopies(t *testing.T) {
	dataDir := createControllerDataDir(t, "old")
	backupDir := createControllerDataDir(t, "new")

	if err := Restore(dataDir, backupDir); err != nil {
		t.Fatalf("restore: %v", err)
	}
	assertDatabaseValue(t, filepath.Join(dataDir, "controller.db"), "new-controller")
	assertDatabaseValue(t, filepath.Join(dataDir, "operations.db"), "new-operations")
}

func TestRestoreRejectsMissingBackupWithoutChangingTarget(t *testing.T) {
	dataDir := createControllerDataDir(t, "old")
	backupDir := t.TempDir()
	createSQLiteDatabase(t, filepath.Join(backupDir, "controller.db"), "new-controller")

	if err := Restore(dataDir, backupDir); err == nil {
		t.Fatal("expected missing backup error")
	}
	assertDatabaseValue(t, filepath.Join(dataDir, "controller.db"), "old-controller")
	assertDatabaseValue(t, filepath.Join(dataDir, "operations.db"), "old-operations")
}

func TestRestoreMaterializesCommittedWALData(t *testing.T) {
	dataDir := createControllerDataDir(t, "old")
	backupDir := createControllerDataDir(t, "initial")
	db, err := sql.Open("sqlite", filepath.Join(backupDir, "controller.db"))
	if err != nil {
		t.Fatalf("open WAL database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		t.Fatalf("enable WAL: %v", err)
	}
	if _, err := db.Exec(`PRAGMA wal_autocheckpoint=0`); err != nil {
		t.Fatalf("disable WAL autocheckpoint: %v", err)
	}
	if _, err := db.Exec(`UPDATE state SET value = 'wal-controller'`); err != nil {
		t.Fatalf("write WAL value: %v", err)
	}

	if err := Restore(dataDir, backupDir); err != nil {
		t.Fatalf("restore WAL backup: %v", err)
	}
	assertDatabaseValue(t, filepath.Join(dataDir, "controller.db"), "wal-controller")
}

func TestAcquireLockRecoversInterruptedRestore(t *testing.T) {
	dataDir := createControllerDataDir(t, "mixed")
	rollbackDir := filepath.Join(dataDir, "pre-restore-interrupted")
	if err := os.Mkdir(rollbackDir, 0o700); err != nil {
		t.Fatalf("create rollback directory: %v", err)
	}
	createSQLiteDatabase(t, filepath.Join(rollbackDir, "controller.db"), "old-controller")
	createSQLiteDatabase(t, filepath.Join(rollbackDir, "operations.db"), "old-operations")
	if err := writeRestoreJournal(dataDir, rollbackDir, map[string]bool{"controller.db": true, "operations.db": true}); err != nil {
		t.Fatalf("write restore journal: %v", err)
	}

	lock, err := AcquireLock(dataDir)
	if err != nil {
		t.Fatalf("acquire lock and recover: %v", err)
	}
	defer lock.Close()
	assertDatabaseValue(t, filepath.Join(dataDir, "controller.db"), "old-controller")
	assertDatabaseValue(t, filepath.Join(dataDir, "operations.db"), "old-operations")
	if _, err := os.Stat(filepath.Join(dataDir, restoreJournalName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected restore journal removal, got %v", err)
	}
}

func TestAcquireLockFailsClosedWhenRollbackArtifactIsMissing(t *testing.T) {
	dataDir := createControllerDataDir(t, "mixed")
	rollbackDir := filepath.Join(dataDir, "pre-restore-incomplete")
	if err := os.Mkdir(rollbackDir, 0o700); err != nil {
		t.Fatalf("create rollback directory: %v", err)
	}
	createSQLiteDatabase(t, filepath.Join(rollbackDir, "controller.db"), "old-controller")
	if err := writeRestoreJournal(dataDir, rollbackDir, map[string]bool{"controller.db": true, "operations.db": true}); err != nil {
		t.Fatalf("write restore journal: %v", err)
	}

	if _, err := AcquireLock(dataDir); err == nil {
		t.Fatal("expected recovery failure")
	}
	assertDatabaseValue(t, filepath.Join(dataDir, "controller.db"), "mixed-controller")
	assertDatabaseValue(t, filepath.Join(dataDir, "operations.db"), "mixed-operations")
	if _, err := os.Stat(filepath.Join(dataDir, restoreJournalName)); err != nil {
		t.Fatalf("expected restore journal to remain after failed recovery: %v", err)
	}
}

func createControllerDataDir(t *testing.T, prefix string) string {
	t.Helper()
	dir := t.TempDir()
	createSQLiteDatabase(t, filepath.Join(dir, "controller.db"), prefix+"-controller")
	createSQLiteDatabase(t, filepath.Join(dir, "operations.db"), prefix+"-operations")
	return dir
}

func createSQLiteDatabase(t *testing.T, path string, value string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE schema_migrations (schema_name TEXT NOT NULL, version INTEGER NOT NULL, applied_at TEXT NOT NULL, PRIMARY KEY(schema_name, version))`); err != nil {
		t.Fatalf("create migrations table: %v", err)
	}
	switch filepath.Base(path) {
	case "controller.db":
		if _, err := db.Exec(`CREATE TABLE branches (name TEXT PRIMARY KEY, parent TEXT NOT NULL, created_at TEXT NOT NULL, deleted INTEGER NOT NULL, deleted_at TEXT, tenant_id TEXT NOT NULL, timeline_id TEXT NOT NULL, password TEXT NOT NULL, endpoint_published INTEGER NOT NULL, endpoint_port INTEGER NOT NULL)`); err != nil {
			t.Fatalf("create branches table: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations VALUES ('branches', 1, '2026-01-01T00:00:00Z')`); err != nil {
			t.Fatalf("insert branches migration: %v", err)
		}
	case "operations.db":
		if _, err := db.Exec(`CREATE TABLE operations (id INTEGER PRIMARY KEY, type TEXT NOT NULL, status TEXT NOT NULL, message TEXT NOT NULL DEFAULT '', started_at TEXT NOT NULL, finished_at TEXT)`); err != nil {
			t.Fatalf("create operations table: %v", err)
		}
		if _, err := db.Exec(`INSERT INTO schema_migrations VALUES ('operations', 1, '2026-01-01T00:00:00Z')`); err != nil {
			t.Fatalf("insert operations migration: %v", err)
		}
	default:
		t.Fatalf("unexpected controller database name %s", path)
	}
	if _, err := db.Exec(`CREATE TABLE state (value TEXT NOT NULL)`); err != nil {
		t.Fatalf("create state table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO state (value) VALUES (?)`, value); err != nil {
		t.Fatalf("insert state: %v", err)
	}
}

func assertDatabaseValue(t *testing.T, path string, expected string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open restored database: %v", err)
	}
	defer db.Close()
	var value string
	if err := db.QueryRow(`SELECT value FROM state`).Scan(&value); err != nil {
		t.Fatalf("query restored database: %v", err)
	}
	if value != expected {
		t.Fatalf("expected database value %q, got %q", expected, value)
	}
}
