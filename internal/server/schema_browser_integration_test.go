//go:build integration

package server

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestSchemaBrowserCatalogQueries(t *testing.T) {
	databaseURL := os.Getenv("SQL_EXECUTION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SQL_EXECUTION_TEST_DATABASE_URL is required for schema browser integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect database: %v", err)
	}
	defer closeSQLConnection(conn)

	schema := fmt.Sprintf("schema_browser_test_%d", time.Now().UnixNano())
	qualified := pgx.Identifier{schema, "catalog_probe"}.Sanitize()
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), defaultSQLExecutionCleanupTimeout)
		defer cleanupCancel()
		if _, err := conn.Exec(cleanupCtx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
	}()
	if _, err := conn.Exec(ctx, "CREATE TABLE "+qualified+" (id bigint PRIMARY KEY, parent_id bigint, payload text NOT NULL, CONSTRAINT payload_nonempty CHECK (payload <> ''))"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := conn.Exec(ctx, "CREATE INDEX catalog_probe_parent_idx ON "+qualified+" (parent_id)"); err != nil {
		t.Fatalf("create index: %v", err)
	}

	tx, err := conn.BeginTx(ctx, schemaCatalogTransactionOptions())
	if err != nil {
		t.Fatalf("begin read-only catalog transaction: %v", err)
	}
	defer rollbackSQLTx(tx)

	schemas, _, err := querySchemaSummaries(ctx, tx, false)
	if err != nil {
		t.Fatalf("query schema summaries: %v", err)
	}
	foundSchema := false
	for _, item := range schemas {
		if item.Name == schema && item.ObjectCount == 1 {
			foundSchema = true
		}
	}
	if !foundSchema {
		t.Fatalf("expected schema %q in catalog: %+v", schema, schemas)
	}

	tables, hasMore, err := querySchemaTables(ctx, tx, sqlSchemaCatalogFilter{Schema: schema, Search: "probe", Limit: 10})
	if err != nil {
		t.Fatalf("query schema tables: %v", err)
	}
	if hasMore || len(tables) != 1 || tables[0].Name != "catalog_probe" || tables[0].Kind != "table" || tables[0].TotalBytes == nil {
		t.Fatalf("unexpected catalog tables: %+v has_more=%t", tables, hasMore)
	}

	var oid uint32
	if err := tx.QueryRow(ctx, "SELECT $1::regclass::oid", qualified).Scan(&oid); err != nil {
		t.Fatalf("resolve table oid: %v", err)
	}
	columns, truncated, err := querySchemaColumns(ctx, tx, oid)
	if err != nil || truncated || len(columns) != 3 {
		t.Fatalf("unexpected columns: %+v truncated=%t err=%v", columns, truncated, err)
	}
	indexes, truncated, err := querySchemaIndexes(ctx, tx, oid)
	if err != nil || truncated || len(indexes) != 2 {
		t.Fatalf("unexpected indexes: %+v truncated=%t err=%v", indexes, truncated, err)
	}
	constraints, truncated, err := querySchemaConstraints(ctx, tx, oid)
	if err != nil || truncated || len(constraints) != 2 {
		t.Fatalf("unexpected constraints: %+v truncated=%t err=%v", constraints, truncated, err)
	}

	if _, err := tx.Exec(ctx, "CREATE TABLE should_fail_in_read_only (id int)"); err == nil {
		t.Fatal("expected catalog transaction to reject writes")
	}
}
