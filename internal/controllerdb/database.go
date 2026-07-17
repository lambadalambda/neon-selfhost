package controllerdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

var ErrDataDirInUse = errors.New("controller data directory is in use")

var databaseNames = []string{"controller.db", "operations.db"}

const restoreJournalName = ".restore-transaction.json"

type restoreJournal struct {
	RollbackDir string          `json:"rollback_dir"`
	HadOriginal map[string]bool `json:"had_original"`
}

type Lock struct {
	mu   sync.Mutex
	file *os.File
}

func AcquireLock(dataDir string) (*Lock, error) {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return nil, errors.New("controller data directory is required")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create controller data directory: %w", err)
	}

	file, err := os.OpenFile(filepath.Join(dataDir, ".controller.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open controller data lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrDataDirInUse
		}
		return nil, fmt.Errorf("lock controller data directory: %w", err)
	}

	lock := &Lock{file: file}
	if err := recoverInterruptedRestore(dataDir); err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("recover interrupted controller database restore: %w", err)
	}
	return lock, nil
}

func (l *Lock) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}

	file := l.file
	l.file = nil
	unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	closeErr := file.Close()
	return errors.Join(unlockErr, closeErr)
}

func Backup(dataDir string, backupDir string) error {
	dataDir = strings.TrimSpace(dataDir)
	backupDir = strings.TrimSpace(backupDir)
	if dataDir == "" || backupDir == "" {
		return errors.New("controller data directory and backup directory are required")
	}
	if _, err := os.Stat(dataDir); err != nil {
		return fmt.Errorf("inspect controller data directory: %w", err)
	}
	if err := requireDatabases(dataDir); err != nil {
		return err
	}
	if _, err := os.Stat(backupDir); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return fmt.Errorf("backup directory already exists: %s", backupDir)
		}
		return fmt.Errorf("inspect backup directory: %w", err)
	}

	lock, err := AcquireLock(dataDir)
	if err != nil {
		return err
	}
	defer lock.Close()

	parentDir := filepath.Dir(backupDir)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("create backup parent directory: %w", err)
	}
	stageDir, err := os.MkdirTemp(parentDir, ".controller-backup-*")
	if err != nil {
		return fmt.Errorf("create backup staging directory: %w", err)
	}
	defer os.RemoveAll(stageDir)

	for _, name := range databaseNames {
		if err := materializeSQLite(filepath.Join(dataDir, name), filepath.Join(stageDir, name)); err != nil {
			return fmt.Errorf("backup %s: %w", name, err)
		}
	}
	if err := syncDirectory(stageDir); err != nil {
		return fmt.Errorf("sync backup staging directory: %w", err)
	}
	if err := os.Rename(stageDir, backupDir); err != nil {
		return fmt.Errorf("publish controller backup: %w", err)
	}
	return syncDirectory(parentDir)
}

func Restore(dataDir string, backupDir string) error {
	dataDir = strings.TrimSpace(dataDir)
	backupDir = strings.TrimSpace(backupDir)
	if dataDir == "" || backupDir == "" {
		return errors.New("controller data directory and backup directory are required")
	}
	info, err := os.Stat(dataDir)
	if err != nil {
		return fmt.Errorf("inspect controller data directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("controller data path is not a directory: %s", dataDir)
	}
	if err := requireDatabases(backupDir); err != nil {
		return err
	}

	lock, err := AcquireLock(dataDir)
	if err != nil {
		return err
	}
	defer lock.Close()

	stageDir, err := os.MkdirTemp(dataDir, ".restore-stage-*")
	if err != nil {
		return fmt.Errorf("create restore staging directory: %w", err)
	}
	defer os.RemoveAll(stageDir)
	for _, name := range databaseNames {
		if err := materializeSQLite(filepath.Join(backupDir, name), filepath.Join(stageDir, name)); err != nil {
			return fmt.Errorf("validate restore database %s: %w", name, err)
		}
	}

	rollbackDir := filepath.Join(dataDir, "pre-restore-"+time.Now().UTC().Format("20060102-150405.000000000"))
	if err := os.Mkdir(rollbackDir, 0o700); err != nil {
		return fmt.Errorf("create restore rollback directory: %w", err)
	}
	journalWritten := false
	restoreCommitted := false
	defer func() {
		if !journalWritten && !restoreCommitted {
			_ = os.RemoveAll(rollbackDir)
		}
	}()

	hadOriginal := make(map[string]bool, len(databaseNames))
	for _, name := range databaseNames {
		target := filepath.Join(dataDir, name)
		if _, err := os.Stat(target); err == nil {
			hadOriginal[name] = true
			if err := materializeSQLite(target, filepath.Join(rollbackDir, name)); err != nil {
				return fmt.Errorf("create rollback database %s: %w", name, err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect target database %s: %w", name, err)
		}
	}
	if err := syncDirectory(rollbackDir); err != nil {
		return fmt.Errorf("sync restore rollback directory: %w", err)
	}
	if err := writeRestoreJournal(dataDir, rollbackDir, hadOriginal); err != nil {
		return err
	}
	journalWritten = true

	for _, name := range databaseNames {
		if err := removeSQLiteFiles(filepath.Join(dataDir, name)); err != nil {
			rollbackErr := rollbackInterruptedRestore(dataDir, rollbackDir, hadOriginal)
			if rollbackErr == nil {
				journalWritten = false
			}
			return errors.Join(fmt.Errorf("remove existing %s: %w", name, err), rollbackErr)
		}
		if err := os.Rename(filepath.Join(stageDir, name), filepath.Join(dataDir, name)); err != nil {
			rollbackErr := rollbackInterruptedRestore(dataDir, rollbackDir, hadOriginal)
			if rollbackErr == nil {
				journalWritten = false
			}
			return errors.Join(fmt.Errorf("replace %s: %w", name, err), rollbackErr)
		}
	}
	if err := syncDirectory(dataDir); err != nil {
		rollbackErr := rollbackInterruptedRestore(dataDir, rollbackDir, hadOriginal)
		if rollbackErr == nil {
			journalWritten = false
		}
		return errors.Join(err, rollbackErr)
	}
	if err := removeRestoreJournal(dataDir); err != nil {
		return fmt.Errorf("commit controller database restore: %w", err)
	}
	journalWritten = false
	restoreCommitted = true
	return nil
}

func materializeSQLite(source string, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("SQLite database is empty or not a regular file")
	}

	if err := validateSQLite(source); err != nil {
		return err
	}
	if err := validateDatabaseIdentity(source, filepath.Base(source)); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", source)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := os.Remove(destination); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := db.Exec(`VACUUM INTO ?`, destination); err != nil {
		return err
	}
	if err := os.Chmod(destination, 0o600); err != nil {
		return err
	}
	if err := validateSQLite(destination); err != nil {
		return err
	}
	file, err := os.Open(destination)
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(syncErr, closeErr)
}

func validateSQLite(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("SQLite database is empty or not a regular file")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRow(`PRAGMA quick_check`).Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return fmt.Errorf("SQLite integrity check failed: %s", integrity)
	}
	return nil
}

func validateDatabaseIdentity(path string, name string) error {
	var schemaName string
	var tableName string
	var expectedColumns []string
	switch name {
	case "controller.db":
		schemaName = "branches"
		tableName = "branches"
		expectedColumns = []string{"name", "parent", "created_at", "deleted", "deleted_at", "tenant_id", "timeline_id", "password", "endpoint_published", "endpoint_port"}
	case "operations.db":
		schemaName = "operations"
		tableName = "operations"
		expectedColumns = []string{"id", "type", "status", "message", "started_at", "finished_at"}
	default:
		return fmt.Errorf("unknown controller database %q", name)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations WHERE schema_name = ?`, schemaName).Scan(&version); err != nil {
		return fmt.Errorf("read %s schema version: %w", schemaName, err)
	}
	if version != 1 {
		return fmt.Errorf("unsupported %s schema version %d", schemaName, version)
	}

	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, tableName)
	if err != nil {
		return err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			return err
		}
		columns[column] = true
	}
	for _, column := range expectedColumns {
		if !columns[column] {
			return fmt.Errorf("%s is missing required column %s", tableName, column)
		}
	}
	return rows.Err()
}

func requireDatabases(dir string) error {
	for _, name := range databaseNames {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("missing %s", path)
		}
		if err != nil {
			return fmt.Errorf("inspect %s: %w", path, err)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("SQLite database is empty or not a regular file: %s", path)
		}
	}
	return nil
}

func restoreRollback(dataDir string, rollbackDir string, hadOriginal map[string]bool) error {
	for _, name := range databaseNames {
		if !hadOriginal[name] {
			continue
		}
		rollbackPath := filepath.Join(rollbackDir, name)
		if err := validateSQLite(rollbackPath); err != nil {
			return fmt.Errorf("validate rollback database %s: %w", name, err)
		}
		if err := validateDatabaseIdentity(rollbackPath, name); err != nil {
			return fmt.Errorf("validate rollback database %s identity: %w", name, err)
		}
	}

	var rollbackErrs []error
	for _, name := range databaseNames {
		target := filepath.Join(dataDir, name)
		if !hadOriginal[name] {
			if err := removeSQLiteFiles(target); err != nil {
				rollbackErrs = append(rollbackErrs, err)
			}
			continue
		}
		rollbackPath := filepath.Join(rollbackDir, name)
		tmp, err := os.CreateTemp(dataDir, ".restore-copy-*")
		if err != nil {
			rollbackErrs = append(rollbackErrs, err)
			continue
		}
		tmpPath := tmp.Name()
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		if err := materializeSQLite(rollbackPath, tmpPath); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("copy rollback database %s: %w", name, err))
			_ = os.Remove(tmpPath)
			continue
		}
		if err := removeSQLiteFiles(target); err != nil {
			rollbackErrs = append(rollbackErrs, err)
			_ = os.Remove(tmpPath)
			continue
		}
		if err := os.Rename(tmpPath, target); err != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("restore rollback database %s: %w", name, err))
			_ = os.Remove(tmpPath)
		}
	}
	if err := syncDirectory(dataDir); err != nil {
		rollbackErrs = append(rollbackErrs, err)
	}
	return errors.Join(rollbackErrs...)
}

func removeSQLiteFiles(path string) error {
	var removeErrs []error
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(candidate); err != nil && !errors.Is(err, os.ErrNotExist) {
			removeErrs = append(removeErrs, fmt.Errorf("remove %s: %w", candidate, err))
		}
	}
	return errors.Join(removeErrs...)
}

func writeRestoreJournal(dataDir string, rollbackDir string, hadOriginal map[string]bool) error {
	content, err := json.Marshal(restoreJournal{RollbackDir: filepath.Base(rollbackDir), HadOriginal: hadOriginal})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dataDir, ".restore-journal-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, filepath.Join(dataDir, restoreJournalName)); err != nil {
		return err
	}
	return syncDirectory(dataDir)
}

func recoverInterruptedRestore(dataDir string) error {
	journalPath := filepath.Join(dataDir, restoreJournalName)
	content, err := os.ReadFile(journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var journal restoreJournal
	if err := json.Unmarshal(content, &journal); err != nil {
		return err
	}
	if journal.RollbackDir == "" || filepath.Base(journal.RollbackDir) != journal.RollbackDir {
		return errors.New("invalid rollback directory in restore journal")
	}
	return rollbackInterruptedRestore(dataDir, filepath.Join(dataDir, journal.RollbackDir), journal.HadOriginal)
}

func rollbackInterruptedRestore(dataDir string, rollbackDir string, hadOriginal map[string]bool) error {
	if err := restoreRollback(dataDir, rollbackDir, hadOriginal); err != nil {
		return err
	}
	return removeRestoreJournal(dataDir)
}

func removeRestoreJournal(dataDir string) error {
	if err := os.Remove(filepath.Join(dataDir, restoreJournalName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectory(dataDir)
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
