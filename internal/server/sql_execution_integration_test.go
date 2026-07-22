//go:build integration

package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestSQLExecutionTransactionDurability(t *testing.T) {
	databaseURL := os.Getenv("SQL_EXECUTION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("SQL_EXECUTION_TEST_DATABASE_URL is required for SQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	writer, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect writer: %v", err)
	}
	defer closeSQLConnection(writer)

	observer, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect observer: %v", err)
	}
	defer closeSQLConnection(observer)

	schema := fmt.Sprintf("sql_execution_test_%d", time.Now().UnixNano())
	table := pgx.Identifier{schema, "commit_probe"}.Sanitize()
	if _, err := writer.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatalf("create probe schema: %v", err)
	}
	defer func() {
		dropSchema := func(conn *pgx.Conn) error {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), defaultSQLExecutionCleanupTimeout)
			defer cleanupCancel()
			_, dropErr := conn.Exec(cleanupCtx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
			return dropErr
		}
		if dropErr := dropSchema(writer); dropErr != nil {
			if retryErr := dropSchema(observer); retryErr != nil {
				t.Errorf("drop probe schema: writer: %v; observer: %v", dropErr, retryErr)
			}
		}
	}()
	if _, err := writer.Exec(ctx, "CREATE TABLE "+table+" (id bigserial PRIMARY KEY, marker text NOT NULL)"); err != nil {
		t.Fatalf("create probe table: %v", err)
	}

	executor := &branchEndpointSQLQueryExecutor{
		commitTimeout: 5 * time.Second,
		maxRows:       200,
		maxBytes:      1024,
		maxCellBytes:  128,
	}

	writeTx, err := writer.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadWrite})
	if err != nil {
		t.Fatalf("begin write transaction: %v", err)
	}
	result, err := executor.executeTransaction(ctx, writeTx, "integration", "postgres", "INSERT INTO "+table+" (marker) VALUES ('committed') RETURNING id", false)
	if err != nil {
		t.Fatalf("execute committed write: %v", err)
	}
	if result.CommandTag != "INSERT 0 1" {
		t.Fatalf("unexpected command tag %q", result.CommandTag)
	}
	assertSQLProbeCount(t, ctx, observer, table, "committed", 1)

	limitedExecutor := *executor
	limitedExecutor.maxRows = 1
	truncatedTx, err := writer.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadWrite})
	if err != nil {
		t.Fatalf("begin truncated transaction: %v", err)
	}
	_, err = limitedExecutor.executeTransaction(ctx, truncatedTx, "integration", "postgres", "INSERT INTO "+table+" (marker) VALUES ('truncated'), ('truncated') RETURNING id", false)
	if !errors.Is(err, ErrSQLWriteResultTruncated) {
		t.Fatalf("expected truncated write error, got %v", err)
	}
	assertSQLProbeCount(t, ctx, observer, table, "truncated", 0)

	readOnlyTx, err := writer.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		t.Fatalf("begin read-only transaction: %v", err)
	}
	_, err = executor.executeTransaction(ctx, readOnlyTx, "integration", "postgres", "INSERT INTO "+table+" (marker) VALUES ('read-only')", true)
	if err == nil {
		t.Fatal("expected read-only write rejection")
	}
	assertSQLProbeCount(t, ctx, observer, table, "read-only", 0)
}

func assertSQLProbeCount(t *testing.T, ctx context.Context, conn *pgx.Conn, table string, marker string, want int) {
	t.Helper()
	var count int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE marker=$1", marker).Scan(&count); err != nil {
		t.Fatalf("count probe marker %q: %v", marker, err)
	}
	if count != want {
		t.Fatalf("expected %d rows for marker %q, got %d", want, marker, count)
	}
}
