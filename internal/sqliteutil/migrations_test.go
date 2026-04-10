package sqliteutil

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestApplyMigrationsAppliesInOrderAndTracksVersion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	version, err := ApplyMigrations(db, "example", []Migration{
		{Version: 2, SQL: `CREATE TABLE IF NOT EXISTS two (id INTEGER PRIMARY KEY)`},
		{Version: 1, SQL: `CREATE TABLE IF NOT EXISTS one (id INTEGER PRIMARY KEY)`},
	})
	if err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	if version != 2 {
		t.Fatalf("expected schema version %d, got %d", 2, version)
	}

	current, err := CurrentSchemaVersion(db, "example")
	if err != nil {
		t.Fatalf("current schema version: %v", err)
	}
	if current != 2 {
		t.Fatalf("expected current schema version %d, got %d", 2, current)
	}
}

func TestApplyMigrationsIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	migrations := []Migration{{Version: 1, SQL: `CREATE TABLE IF NOT EXISTS example (id INTEGER PRIMARY KEY)`}}

	if _, err := ApplyMigrations(db, "example", migrations); err != nil {
		t.Fatalf("first apply migrations: %v", err)
	}

	if _, err := ApplyMigrations(db, "example", migrations); err != nil {
		t.Fatalf("second apply migrations: %v", err)
	}

	current, err := CurrentSchemaVersion(db, "example")
	if err != nil {
		t.Fatalf("current schema version: %v", err)
	}
	if current != 1 {
		t.Fatalf("expected current schema version %d, got %d", 1, current)
	}
}
