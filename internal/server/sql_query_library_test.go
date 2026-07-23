package server

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"neon-selfhost/internal/branch"

	_ "modernc.org/sqlite"
)

func TestSQLQueryLibrarySharesControllerDatabaseWithBranchStore(t *testing.T) {
	dataDir := t.TempDir()
	branchStore, err := branch.NewSQLitePersistentStore(dataDir)
	if err != nil {
		t.Fatalf("create branch store: %v", err)
	}
	defer branchStore.Close()

	queryStore, err := newSQLiteSQLQueryLibraryStore(filepath.Join(dataDir, "controller.db"), 200)
	if err != nil {
		t.Fatalf("create SQL query library in controller database: %v", err)
	}
	defer queryStore.Close()
	if _, err := queryStore.CreateSavedQuery(context.Background(), savedSQLQuery{Name: "one", SQL: "SELECT 1", Branch: "main", Database: "postgres"}); err != nil {
		t.Fatalf("create saved query: %v", err)
	}
	if _, err := branchStore.Create("feature-a", "main"); err != nil {
		t.Fatalf("write branch store after SQL library initialization: %v", err)
	}

	db := queryStore.(*sqliteSQLQueryLibraryStore).db
	for schema, expected := range map[string]int{"branches": branch.SQLiteBranchSchemaVersion, "sql_query_library": sqliteSQLQueryLibrarySchemaVersion} {
		var version int
		if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations WHERE schema_name = ?`, schema).Scan(&version); err != nil {
			t.Fatalf("read %s schema version: %v", schema, err)
		}
		if version != expected {
			t.Fatalf("expected %s schema version %d, got %d", schema, expected, version)
		}
	}
}

func TestMemorySQLQueryLibraryAppliesHistoryRetention(t *testing.T) {
	store := newMemorySQLQueryLibraryStore(2)
	for i := 0; i < 3; i++ {
		if _, err := store.AddExecutionHistory(context.Background(), sqlExecutionHistory{SQL: "SELECT 1", DurationMS: int64(i)}); err != nil {
			t.Fatalf("add memory history: %v", err)
		}
	}
	history, err := store.ListExecutionHistory(context.Background(), sqlQueryLibraryFilter{})
	if err != nil {
		t.Fatalf("list memory history: %v", err)
	}
	if len(history) != 2 || history[0].DurationMS != 2 || history[1].DurationMS != 1 {
		t.Fatalf("unexpected retained memory history: %+v", history)
	}
}

func TestSQLiteSQLQueryLibrarySavedQueriesSurviveRestartAndFilter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller.db")
	store, err := newSQLiteSQLQueryLibraryStore(path, 200)
	if err != nil {
		t.Fatalf("create sqlite SQL query library: %v", err)
	}

	first, err := store.CreateSavedQuery(context.Background(), savedSQLQuery{
		Name: "Recent posts", SQL: "SELECT * FROM posts", Branch: "main", Database: "pleroma",
	})
	if err != nil {
		t.Fatalf("create first saved query: %v", err)
	}
	second, err := store.CreateSavedQuery(context.Background(), savedSQLQuery{
		Name: "Recent posts", SQL: "SELECT id FROM posts", Branch: "feature-a", Database: "pleroma",
	})
	if err != nil {
		t.Fatalf("create duplicate-name saved query: %v", err)
	}

	newName := "Newest posts"
	newSQL := "SELECT id FROM posts ORDER BY id DESC"
	updated, err := store.UpdateSavedQuery(context.Background(), first.ID, &newName, &newSQL)
	if err != nil {
		t.Fatalf("update saved query: %v", err)
	}
	if updated.Branch != "main" || updated.Database != "pleroma" {
		t.Fatalf("scope changed during update: %+v", updated)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close first store: %v", err)
	}

	reopened, err := newSQLiteSQLQueryLibraryStore(path, 200)
	if err != nil {
		t.Fatalf("reopen sqlite SQL query library: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })

	project, err := reopened.ListSavedQueries(context.Background(), sqlQueryLibraryFilter{Limit: 100})
	if err != nil {
		t.Fatalf("list project saved queries: %v", err)
	}
	if len(project) != 2 {
		t.Fatalf("expected 2 project-wide queries, got %d", len(project))
	}
	if project[0].ID != second.ID || project[1].Name != newName || project[1].SQL != newSQL {
		t.Fatalf("unexpected restarted query list: %+v", project)
	}

	filtered, err := reopened.ListSavedQueries(context.Background(), sqlQueryLibraryFilter{Branch: "main", Database: "pleroma", Limit: 1})
	if err != nil {
		t.Fatalf("list filtered saved queries: %v", err)
	}
	if len(filtered) != 1 || filtered[0].ID != first.ID {
		t.Fatalf("unexpected filtered list: %+v", filtered)
	}

	if err := reopened.DeleteSavedQuery(context.Background(), second.ID); err != nil {
		t.Fatalf("delete saved query: %v", err)
	}
	if err := reopened.DeleteSavedQuery(context.Background(), second.ID); !errors.Is(err, errSQLSavedQueryNotFound) {
		t.Fatalf("expected not found deleting saved query twice, got %v", err)
	}
}

func TestSQLiteSQLQueryLibraryRejectsNewerSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite database: %v", err)
	}
	if _, err := db.Exec(`
		CREATE TABLE schema_migrations (schema_name TEXT NOT NULL, version INTEGER NOT NULL, applied_at TEXT NOT NULL, PRIMARY KEY(schema_name, version));
		INSERT INTO schema_migrations (schema_name, version, applied_at) VALUES ('sql_query_library', ?, '2026-07-23T00:00:00Z')
	`, sqliteSQLQueryLibrarySchemaVersion+1); err != nil {
		db.Close()
		t.Fatalf("seed future schema: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seeded database: %v", err)
	}

	store, err := newSQLiteSQLQueryLibraryStore(path, 200)
	if err == nil {
		store.Close()
		t.Fatal("expected newer SQL query library schema to be rejected")
	}
	if !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("expected actionable newer-schema error, got %v", err)
	}
}

func TestSQLiteSQLQueryLibraryHistoryRetentionIsPhysicalAndGlobal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller.db")
	store, err := newSQLiteSQLQueryLibraryStore(path, 3)
	if err != nil {
		t.Fatalf("create sqlite SQL query library: %v", err)
	}

	for i := 0; i < 5; i++ {
		branch := "main"
		if i%2 == 1 {
			branch = "feature-a"
		}
		_, err := store.AddExecutionHistory(context.Background(), sqlExecutionHistory{
			Title: "query", SQL: "SELECT 1", Branch: branch, Database: "postgres", ReadOnly: true,
			Status: sqlHistoryStatusSucceeded, CommandTag: "SELECT 1", DurationMS: int64(i), ExecutedAt: time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("add history %d: %v", i, err)
		}
	}

	var physicalCount int
	if err := store.(*sqliteSQLQueryLibraryStore).db.QueryRow(`SELECT COUNT(*) FROM sql_execution_history`).Scan(&physicalCount); err != nil {
		t.Fatalf("count physical history: %v", err)
	}
	if physicalCount != 3 {
		t.Fatalf("expected 3 physical history rows, got %d", physicalCount)
	}

	project, err := store.ListExecutionHistory(context.Background(), sqlQueryLibraryFilter{Limit: 100})
	if err != nil {
		t.Fatalf("list history: %v", err)
	}
	if len(project) != 3 || project[0].DurationMS != 4 || project[2].DurationMS != 2 {
		t.Fatalf("unexpected retained history: %+v", project)
	}
	filtered, err := store.ListExecutionHistory(context.Background(), sqlQueryLibraryFilter{Branch: "feature-a", Limit: 100})
	if err != nil {
		t.Fatalf("filter history: %v", err)
	}
	if len(filtered) != 1 || filtered[0].DurationMS != 3 {
		t.Fatalf("retention was not global before filtering: %+v", filtered)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	reopened, err := newSQLiteSQLQueryLibraryStore(path, 1)
	if err != nil {
		t.Fatalf("reopen with lower retention: %v", err)
	}
	defer reopened.Close()
	if err := reopened.(*sqliteSQLQueryLibraryStore).db.QueryRow(`SELECT COUNT(*) FROM sql_execution_history`).Scan(&physicalCount); err != nil {
		t.Fatalf("count history after restart pruning: %v", err)
	}
	if physicalCount != 1 {
		t.Fatalf("expected restart pruning to leave 1 row, got %d", physicalCount)
	}
}

func TestSQLiteSQLQueryLibrarySchemaContainsOnlyAllowedColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "controller.db")
	store, err := newSQLiteSQLQueryLibraryStore(path, 200)
	if err != nil {
		t.Fatalf("create sqlite SQL query library: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite schema: %v", err)
	}
	defer db.Close()

	want := map[string][]string{
		"sql_saved_queries":     {"id", "name", "sql_text", "branch", "database_name", "created_at", "updated_at"},
		"sql_execution_history": {"id", "title", "sql_text", "branch", "database_name", "read_only", "status", "command_tag", "duration_ms", "error_code", "executed_at"},
	}
	for table, expected := range want {
		rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
		if err != nil {
			t.Fatalf("read %s schema: %v", table, err)
		}
		var columns []string
		for rows.Next() {
			var cid, notNull, primaryKey int
			var name, columnType string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
				rows.Close()
				t.Fatalf("scan %s schema: %v", table, err)
			}
			columns = append(columns, name)
		}
		rows.Close()
		if !reflect.DeepEqual(columns, expected) {
			t.Fatalf("unexpected %s columns: got %v want %v", table, columns, expected)
		}
	}

	var version int
	if err := db.QueryRow(`SELECT MAX(version) FROM schema_migrations WHERE schema_name = 'sql_query_library'`).Scan(&version); err != nil {
		t.Fatalf("read SQL query library schema version: %v", err)
	}
	if version != sqliteSQLQueryLibrarySchemaVersion {
		t.Fatalf("expected schema version %d, got %d", sqliteSQLQueryLibrarySchemaVersion, version)
	}
}
