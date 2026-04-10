package branch

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const defaultSQLiteStateFileName = "controller.db"

func NewSQLitePersistentStore(dataDir string) (*Store, error) {
	return NewSQLitePersistentStoreWithClock(dataDir, defaultClock)
}

func NewSQLitePersistentStoreWithClock(dataDir string, now func() time.Time) (*Store, error) {
	if now == nil {
		now = defaultClock
	}

	cleanDir := strings.TrimSpace(dataDir)
	if cleanDir == "" {
		return nil, fmt.Errorf("controller data dir is required")
	}

	if err := os.MkdirAll(cleanDir, 0o755); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(cleanDir, defaultSQLiteStateFileName)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		return nil, err
	}

	if _, err := db.Exec(`PRAGMA synchronous=NORMAL`); err != nil {
		_ = db.Close()
		return nil, err
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS branches (
			name TEXT PRIMARY KEY,
			parent TEXT NOT NULL,
			created_at TEXT NOT NULL,
			deleted INTEGER NOT NULL,
			deleted_at TEXT,
			tenant_id TEXT NOT NULL,
			timeline_id TEXT NOT NULL,
			password TEXT NOT NULL,
			endpoint_published INTEGER NOT NULL,
			endpoint_port INTEGER NOT NULL
		)
	`); err != nil {
		_ = db.Close()
		return nil, err
	}

	branches, err := loadSQLiteBranchMap(db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	if len(branches) == 0 {
		legacyPath := filepath.Join(cleanDir, defaultStateFileName)
		legacySnapshot, exists, err := loadPersistedState(legacyPath)
		if err != nil {
			_ = db.Close()
			return nil, err
		}

		if exists {
			branches, err = branchMapFromSnapshot(legacySnapshot)
			if err != nil {
				_ = db.Close()
				return nil, fmt.Errorf("invalid persisted branch state: %w", err)
			}
		} else {
			branches = defaultBranchMap(now)
		}

		if err := persistSQLiteBranches(db, snapshotFromMap(branches)); err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	persist := func(snapshot []Branch) error {
		if err := persistSQLiteBranches(db, snapshot); err != nil {
			return fmt.Errorf("write branch state sqlite: %w", err)
		}
		return nil
	}

	return newStoreWithBranches(now, branches, persist), nil
}

func loadSQLiteBranchMap(db *sql.DB) (map[string]Branch, error) {
	rows, err := db.Query(`SELECT name, parent, created_at, deleted, deleted_at, tenant_id, timeline_id, password, endpoint_published, endpoint_port FROM branches ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	branches := make([]Branch, 0)
	for rows.Next() {
		var b Branch
		var createdAtRaw string
		var deletedAtRaw sql.NullString
		var deletedInt int
		var endpointPublishedInt int

		if err := rows.Scan(
			&b.Name,
			&b.Parent,
			&createdAtRaw,
			&deletedInt,
			&deletedAtRaw,
			&b.TenantID,
			&b.TimelineID,
			&b.Password,
			&endpointPublishedInt,
			&b.EndpointPort,
		); err != nil {
			return nil, err
		}

		createdAt, err := time.Parse(time.RFC3339Nano, createdAtRaw)
		if err != nil {
			return nil, err
		}
		b.CreatedAt = createdAt.UTC()
		b.Deleted = deletedInt == 1
		b.EndpointPublished = endpointPublishedInt == 1

		if deletedAtRaw.Valid {
			deletedAt, err := time.Parse(time.RFC3339Nano, deletedAtRaw.String)
			if err != nil {
				return nil, err
			}
			deletedAtUTC := deletedAt.UTC()
			b.DeletedAt = &deletedAtUTC
		}

		branches = append(branches, b)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(branches) == 0 {
		return map[string]Branch{}, nil
	}

	return branchMapFromSnapshot(branches)
}

func persistSQLiteBranches(db *sql.DB, branches []Branch) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM branches`); err != nil {
		return err
	}

	for _, b := range branches {
		createdAt := b.CreatedAt.UTC().Format(time.RFC3339Nano)
		deleted := 0
		if b.Deleted {
			deleted = 1
		}

		endpointPublished := 0
		if b.EndpointPublished {
			endpointPublished = 1
		}

		var deletedAt any
		if b.DeletedAt != nil {
			deletedAt = b.DeletedAt.UTC().Format(time.RFC3339Nano)
		}

		if _, err := tx.Exec(
			`INSERT INTO branches (name, parent, created_at, deleted, deleted_at, tenant_id, timeline_id, password, endpoint_published, endpoint_port) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			b.Name,
			b.Parent,
			createdAt,
			deleted,
			deletedAt,
			b.TenantID,
			b.TimelineID,
			b.Password,
			endpointPublished,
			b.EndpointPort,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}
