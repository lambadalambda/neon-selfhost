package scripts

import (
	"archive/tar"
	"database/sql"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"neon-selfhost/internal/controllerdb"

	_ "modernc.org/sqlite"
)

func TestNativeRestoreRefusesRunningController(t *testing.T) {
	backupDir := createBackupDir(t)
	dataDir := createControllerDataDir(t, "old")
	binDir := createFakeTools(t)
	lock, err := controllerdb.AcquireLock(dataDir)
	if err != nil {
		t.Fatalf("acquire controller lock: %v", err)
	}
	defer lock.Close()

	result, err := runControllerDB(t, binDir, map[string]string{
		"BACKUP_DIR":          backupDir,
		"CONTROLLER_DATA_DIR": dataDir,
	}, "restore")
	if err == nil {
		t.Fatal("expected restore refusal")
	}
	if !strings.Contains(result, "controller data directory is in use") {
		t.Fatalf("expected running-controller error, got %q", result)
	}
}

func TestNativeRestoreRequiresBothBackupDatabases(t *testing.T) {
	backupDir := t.TempDir()
	createSQLiteDatabase(t, filepath.Join(backupDir, "controller.db"), "controller-backup")
	binDir := createFakeTools(t)

	result, err := runControllerDB(t, binDir, map[string]string{
		"BACKUP_DIR":          backupDir,
		"CONTROLLER_DATA_DIR": createControllerDataDir(t, "old"),
	}, "restore")
	if err == nil {
		t.Fatal("expected restore failure")
	}
	if !strings.Contains(result, "missing "+filepath.Join(backupDir, "operations.db")) {
		t.Fatalf("expected missing operations database error, got %q", result)
	}
}

func TestNativeRestoreReplacesValidatedDatabases(t *testing.T) {
	backupDir := createBackupDir(t)
	dataDir := createControllerDataDir(t, "old")
	binDir := createFakeTools(t)

	result, err := runControllerDB(t, binDir, map[string]string{
		"BACKUP_DIR":          backupDir,
		"CONTROLLER_DATA_DIR": dataDir,
	}, "restore")
	if err != nil {
		t.Fatalf("restore failed: %v: %s", err, result)
	}
	assertDatabaseValue(t, filepath.Join(dataDir, "controller.db"), "controller-backup")
	assertDatabaseValue(t, filepath.Join(dataDir, "operations.db"), "operations-backup")
}

func TestComposeRestoreRefusesRunningController(t *testing.T) {
	backupDir := t.TempDir()
	createControllerArchive(t, filepath.Join(backupDir, "controller-state.tar"), true)
	binDir := createFakeTools(t)

	result, err := runControllerDB(t, binDir, map[string]string{
		"BACKUP_DIR":          backupDir,
		"FAKE_PODMAN_RUNNING": "1",
		"PODMAN_LOG":          filepath.Join(t.TempDir(), "podman.log"),
	}, "restore-compose")
	if err == nil {
		t.Fatal("expected compose restore refusal")
	}
	if !strings.Contains(result, "Compose controller is running") {
		t.Fatalf("expected running-controller error, got %q", result)
	}
}

func TestComposeRestoreRecreatesAndImportsControllerVolume(t *testing.T) {
	backupDir := t.TempDir()
	archivePath := filepath.Join(backupDir, "controller-state.tar")
	createControllerArchive(t, archivePath, true)
	rollbackArchive := filepath.Join(t.TempDir(), "current-volume.tar")
	createControllerArchive(t, rollbackArchive, true)
	binDir := createFakeTools(t)
	logPath := filepath.Join(t.TempDir(), "podman.log")

	result, err := runControllerDB(t, binDir, map[string]string{
		"BACKUP_DIR":          backupDir,
		"CONTROLLER_VOLUME":   "test_controller_state",
		"FAKE_VOLUME_ARCHIVE": rollbackArchive,
		"PODMAN_LOG":          logPath,
	}, "restore-compose")
	if err != nil {
		t.Fatalf("compose restore failed: %v: %s", err, result)
	}

	logContent, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read podman log: %v", err)
	}
	logText := string(logContent)
	for _, expected := range []string{
		"compose ps --status running -q controller",
		"volume export test_controller_state",
		"compose down",
		"volume rm test_controller_state",
		"volume create test_controller_state",
		"volume import test_controller_state " + filepath.Join(backupDir, "controller-state.tar"),
	} {
		if !strings.Contains(logText, expected) {
			t.Fatalf("expected podman call %q, got:\n%s", expected, logText)
		}
	}
}

func TestComposeRestoreRejectsArchiveMissingDatabaseBeforeDestructiveCalls(t *testing.T) {
	backupDir := t.TempDir()
	createControllerArchive(t, filepath.Join(backupDir, "controller-state.tar"), false)
	binDir := createFakeTools(t)
	logPath := filepath.Join(t.TempDir(), "podman.log")

	result, err := runControllerDB(t, binDir, map[string]string{
		"BACKUP_DIR": backupDir,
		"PODMAN_LOG": logPath,
	}, "restore-compose")
	if err == nil {
		t.Fatal("expected invalid archive rejection")
	}
	if !strings.Contains(result, "operations.db") {
		t.Fatalf("expected missing database error, got %q", result)
	}
	logContent, _ := os.ReadFile(logPath)
	if strings.Contains(string(logContent), "compose down") || strings.Contains(string(logContent), "volume rm") {
		t.Fatalf("destructive Podman call occurred before validation:\n%s", logContent)
	}
}

func TestComposeRestoreRollsBackVolumeWhenImportFails(t *testing.T) {
	backupDir := t.TempDir()
	archivePath := filepath.Join(backupDir, "controller-state.tar")
	createControllerArchive(t, archivePath, true)
	rollbackArchive := filepath.Join(t.TempDir(), "current-volume.tar")
	createControllerArchive(t, rollbackArchive, true)
	binDir := createFakeTools(t)
	logPath := filepath.Join(t.TempDir(), "podman.log")

	result, err := runControllerDB(t, binDir, map[string]string{
		"BACKUP_DIR":          backupDir,
		"CONTROLLER_VOLUME":   "test_controller_state",
		"FAKE_VOLUME_ARCHIVE": rollbackArchive,
		"FAIL_IMPORT_ARCHIVE": archivePath,
		"PODMAN_LOG":          logPath,
	}, "restore-compose")
	if err == nil {
		t.Fatal("expected failed requested import")
	}
	if !strings.Contains(result, "previous controller volume was restored") {
		t.Fatalf("expected successful rollback message, got %q", result)
	}
	logContent, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read podman log: %v", readErr)
	}
	var imports []string
	for _, line := range strings.Split(string(logContent), "\n") {
		if strings.HasPrefix(line, "volume import test_controller_state ") {
			imports = append(imports, line)
		}
	}
	if len(imports) != 2 || strings.HasSuffix(imports[1], archivePath) {
		t.Fatalf("expected requested import followed by rollback import, got:\n%s", logContent)
	}
}

func TestComposeRestoreRollsBackVolumeWhenInterrupted(t *testing.T) {
	backupDir := t.TempDir()
	archivePath := filepath.Join(backupDir, "controller-state.tar")
	createControllerArchive(t, archivePath, true)
	rollbackArchive := filepath.Join(t.TempDir(), "current-volume.tar")
	createControllerArchive(t, rollbackArchive, true)
	binDir := createFakeTools(t)
	logPath := filepath.Join(t.TempDir(), "podman.log")

	_, err := runControllerDB(t, binDir, map[string]string{
		"BACKUP_DIR":               backupDir,
		"CONTROLLER_VOLUME":        "test_controller_state",
		"FAKE_VOLUME_ARCHIVE":      rollbackArchive,
		"INTERRUPT_IMPORT_ARCHIVE": archivePath,
		"PODMAN_LOG":               logPath,
	}, "restore-compose")
	if err == nil {
		t.Fatal("expected interrupted restore failure")
	}
	logContent, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("read podman log: %v", readErr)
	}
	var imports []string
	for _, line := range strings.Split(string(logContent), "\n") {
		if strings.HasPrefix(line, "volume import test_controller_state ") {
			imports = append(imports, line)
		}
	}
	if len(imports) < 2 || strings.HasSuffix(imports[len(imports)-1], archivePath) {
		t.Fatalf("expected rollback import after interruption, got:\n%s", logContent)
	}
}

func runControllerDB(t *testing.T, binDir string, env map[string]string, action string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", "controller_db.sh", action)
	cmd.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"))
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func createBackupDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	createSQLiteDatabase(t, filepath.Join(dir, "controller.db"), "controller-backup")
	createSQLiteDatabase(t, filepath.Join(dir, "operations.db"), "operations-backup")
	return dir
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

func createFakeTools(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeExecutable(t, filepath.Join(dir, "curl"), `#!/bin/sh
exit "${FAKE_CURL_EXIT:-7}"
`)
	writeExecutable(t, filepath.Join(dir, "sqlite3"), `#!/bin/sh
if [ "${2:-}" = "PRAGMA quick_check;" ]; then
  printf 'ok\n'
  exit 0
fi
printf 'unexpected sqlite3 invocation: %s\n' "$*" >&2
exit 1
`)
	writeExecutable(t, filepath.Join(dir, "podman"), `#!/bin/sh
printf '%s\n' "$*" >>"${PODMAN_LOG}"
if [ "${1:-} ${2:-}" = "compose ps" ] && [ "${FAKE_PODMAN_RUNNING:-0}" = "1" ]; then
  printf 'controller-id\n'
fi
if [ "${1:-} ${2:-}" = "volume export" ] && [ -n "${FAKE_VOLUME_ARCHIVE:-}" ]; then
  cp "${FAKE_VOLUME_ARCHIVE}" "${5}"
fi
if [ "${1:-} ${2:-}" = "volume import" ] && [ "${4:-}" = "${FAIL_IMPORT_ARCHIVE:-}" ]; then
  exit 1
fi
if [ "${1:-} ${2:-}" = "volume import" ] && [ "${4:-}" = "${INTERRUPT_IMPORT_ARCHIVE:-}" ]; then
  kill -TERM "${PPID}"
  sleep 0.1
  exit 1
fi
`)
	return dir
}

func createControllerArchive(t *testing.T, archivePath string, includeOperations bool) {
	t.Helper()
	dir := t.TempDir()
	createSQLiteDatabase(t, filepath.Join(dir, "controller.db"), "archive-controller")
	if includeOperations {
		createSQLiteDatabase(t, filepath.Join(dir, "operations.db"), "archive-operations")
	}

	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	w := tar.NewWriter(archive)
	for _, name := range []string{"controller.db", "operations.db"} {
		path := filepath.Join(dir, name)
		file, err := os.Open(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("open archive source: %v", err)
		}
		info, err := file.Stat()
		if err != nil {
			t.Fatalf("stat archive source: %v", err)
		}
		if err := w.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: info.Size()}); err != nil {
			t.Fatalf("write archive header: %v", err)
		}
		if _, err := io.Copy(w, file); err != nil {
			t.Fatalf("write archive content: %v", err)
		}
		_ = file.Close()
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
}

func writeExecutable(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
