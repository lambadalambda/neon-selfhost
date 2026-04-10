package sqliteutil

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Migration struct {
	Version int
	SQL     string
}

func ApplyMigrations(db *sql.DB, schemaName string, migrations []Migration) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("sqlite db is required")
	}

	schemaName = strings.TrimSpace(schemaName)
	if schemaName == "" {
		return 0, fmt.Errorf("schema name is required")
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			schema_name TEXT NOT NULL,
			version INTEGER NOT NULL,
			applied_at TEXT NOT NULL,
			PRIMARY KEY(schema_name, version)
		)
	`); err != nil {
		return 0, err
	}

	sorted := append([]Migration(nil), migrations...)
	sort.Slice(sorted, func(i int, j int) bool {
		return sorted[i].Version < sorted[j].Version
	})

	for i := 1; i < len(sorted); i++ {
		if sorted[i].Version == sorted[i-1].Version {
			return 0, fmt.Errorf("duplicate migration version %d for schema %q", sorted[i].Version, schemaName)
		}
	}

	currentVersion, err := currentSchemaVersion(db, schemaName)
	if err != nil {
		return 0, err
	}

	for _, migration := range sorted {
		if migration.Version <= currentVersion {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return 0, err
		}

		if _, err := tx.Exec(migration.SQL); err != nil {
			_ = tx.Rollback()
			return 0, err
		}

		if _, err := tx.Exec(
			`INSERT INTO schema_migrations (schema_name, version, applied_at) VALUES (?, ?, ?)`,
			schemaName,
			migration.Version,
			time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			_ = tx.Rollback()
			return 0, err
		}

		if err := tx.Commit(); err != nil {
			return 0, err
		}

		currentVersion = migration.Version
	}

	return currentVersion, nil
}

func CurrentSchemaVersion(db *sql.DB, schemaName string) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("sqlite db is required")
	}

	return currentSchemaVersion(db, strings.TrimSpace(schemaName))
}

func currentSchemaVersion(db *sql.DB, schemaName string) (int, error) {
	if schemaName == "" {
		return 0, nil
	}

	var version sql.NullInt64
	err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations WHERE schema_name = ?`, schemaName).Scan(&version)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}

	if !version.Valid {
		return 0, nil
	}

	return int(version.Int64), nil
}
