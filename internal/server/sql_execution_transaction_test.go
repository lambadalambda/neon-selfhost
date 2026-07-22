package server

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestExecuteSQLTransactionCommitsSuccessfulWrite(t *testing.T) {
	rows := &fakeSQLRows{commandTag: pgconn.NewCommandTag("UPDATE 1")}
	tx := &fakeSQLTransaction{rows: rows}
	executor := &branchEndpointSQLQueryExecutor{maxRows: 200, maxBytes: 1024, maxCellBytes: 128}

	result, err := executor.executeTransaction(context.Background(), tx, "main", "postgres", "UPDATE app.documents SET title='x'", false)
	if err != nil {
		t.Fatalf("execute transaction: %v", err)
	}
	if tx.commitCalls != 1 {
		t.Fatalf("expected one commit, got %d", tx.commitCalls)
	}
	if tx.rollbackCalls != 1 {
		t.Fatalf("expected deferred rollback cleanup attempt after commit, got %d", tx.rollbackCalls)
	}
	if !tx.rowsClosedAtCommit {
		t.Fatal("expected result rows to close before commit")
	}
	if result.CommandTag != "UPDATE 1" || result.ReadOnly {
		t.Fatalf("unexpected write result: %#v", result)
	}
}

func TestExecuteSQLTransactionDoesNotCommitRowFailure(t *testing.T) {
	rowErr := errors.New("row stream failed")
	rows := &fakeSQLRows{err: rowErr}
	tx := &fakeSQLTransaction{rows: rows}
	executor := &branchEndpointSQLQueryExecutor{maxRows: 200, maxBytes: 1024, maxCellBytes: 128}

	_, err := executor.executeTransaction(context.Background(), tx, "main", "postgres", "UPDATE app.documents SET title='x' RETURNING id", false)
	if err == nil {
		t.Fatal("expected row failure")
	}
	if tx.commitCalls != 0 {
		t.Fatalf("expected failed execution not to commit, got %d commits", tx.commitCalls)
	}
	if tx.rollbackCalls != 1 {
		t.Fatalf("expected failed execution to roll back, got %d rollbacks", tx.rollbackCalls)
	}
}

func TestExecuteSQLTransactionReturnsCommitFailure(t *testing.T) {
	commitErr := errors.New("commit failed")
	rows := &fakeSQLRows{commandTag: pgconn.NewCommandTag("INSERT 0 1")}
	tx := &fakeSQLTransaction{rows: rows, commitErr: commitErr}
	executor := &branchEndpointSQLQueryExecutor{maxRows: 200, maxBytes: 1024, maxCellBytes: 128}

	_, err := executor.executeTransaction(context.Background(), tx, "main", "postgres", "INSERT INTO app.documents(title) VALUES ('x')", false)
	if err == nil || !strings.Contains(err.Error(), commitErr.Error()) {
		t.Fatalf("expected mapped commit failure, got %v", err)
	}
	if tx.commitCalls != 1 {
		t.Fatalf("expected one commit attempt, got %d", tx.commitCalls)
	}
	if tx.rollbackCalls != 1 {
		t.Fatalf("expected commit failure to roll back, got %d rollbacks", tx.rollbackCalls)
	}
}

func TestExecuteSQLTransactionRollsBackTruncatedWrite(t *testing.T) {
	tests := []struct {
		name     string
		maxRows  int
		maxBytes int
		values   [][]any
	}{
		{name: "row limit", maxRows: 1, maxBytes: 1024, values: [][]any{{1}, {2}}},
		{name: "byte limit", maxRows: 10, maxBytes: 4, values: [][]any{{"value too large"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := &fakeSQLRows{values: tt.values, commandTag: pgconn.NewCommandTag("INSERT 0 2")}
			tx := &fakeSQLTransaction{rows: rows}
			executor := &branchEndpointSQLQueryExecutor{maxRows: tt.maxRows, maxBytes: tt.maxBytes, maxCellBytes: 128}

			_, err := executor.executeTransaction(context.Background(), tx, "main", "postgres", "INSERT INTO app.documents(title) VALUES ('a'), ('b') RETURNING id", false)
			if !errors.Is(err, ErrSQLWriteResultTruncated) {
				t.Fatalf("expected write result limit error, got %v", err)
			}
			if tx.commitCalls != 0 || tx.rollbackCalls != 1 {
				t.Fatalf("expected truncated write rollback without commit, got commits=%d rollbacks=%d", tx.commitCalls, tx.rollbackCalls)
			}
		})
	}
}

func TestExecuteSQLTransactionKeepsReadOnlyRollbackSemantics(t *testing.T) {
	rows := &fakeSQLRows{commandTag: pgconn.NewCommandTag("SELECT 1")}
	tx := &fakeSQLTransaction{rows: rows}
	executor := &branchEndpointSQLQueryExecutor{maxRows: 200, maxBytes: 1024, maxCellBytes: 128}

	result, err := executor.executeTransaction(context.Background(), tx, "main", "postgres", "SELECT 1", true)
	if err != nil {
		t.Fatalf("execute read-only transaction: %v", err)
	}
	if !result.ReadOnly || tx.commitCalls != 0 || tx.rollbackCalls != 1 {
		t.Fatalf("expected successful read-only rollback, result=%#v commits=%d rollbacks=%d", result, tx.commitCalls, tx.rollbackCalls)
	}
}

func TestExecuteSQLTransactionClassifiesCommitOutcome(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    error
		unknown bool
	}{
		{name: "server rolled back", err: pgx.ErrTxCommitRollback, want: ErrSQLCommitFailed},
		{name: "server constraint failure", err: &pgconn.PgError{Code: "23514", Message: "deferred constraint failed"}, want: ErrSQLCommitFailed},
		{name: "transport failure", err: errors.New("connection lost"), want: ErrSQLCommitOutcomeUnknown, unknown: true},
		{name: "caller cancellation", err: context.Canceled, want: ErrSQLCommitOutcomeUnknown, unknown: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := &fakeSQLRows{commandTag: pgconn.NewCommandTag("INSERT 0 1")}
			tx := &fakeSQLTransaction{rows: rows, commitErr: tt.err}
			executor := &branchEndpointSQLQueryExecutor{maxRows: 200, maxBytes: 1024, maxCellBytes: 128}

			_, err := executor.executeTransaction(context.Background(), tx, "main", "postgres", "INSERT INTO app.documents(title) VALUES ('x')", false)
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
			if !errors.Is(err, tt.err) {
				t.Fatalf("expected classified error to preserve cause %v, got %v", tt.err, err)
			}
			if tt.unknown && errors.Is(err, ErrSQLCommitFailed) {
				t.Fatalf("did not expect uncertain commit to be classified as definitive rollback: %v", err)
			}
		})
	}
}

func TestExecuteSQLTransactionRollsBackCancellationBeforeCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rows := &fakeSQLRows{commandTag: pgconn.NewCommandTag("INSERT 0 1")}
	tx := &fakeSQLTransaction{rows: rows}
	executor := &branchEndpointSQLQueryExecutor{commitTimeout: time.Second, maxRows: 200, maxBytes: 1024, maxCellBytes: 128}

	_, err := executor.executeTransaction(ctx, tx, "main", "postgres", "INSERT INTO app.documents(title) VALUES ('x')", false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected caller cancellation, got %v", err)
	}
	if tx.commitCalls != 0 || tx.rollbackCalls != 1 {
		t.Fatalf("expected cancellation rollback without commit, got commits=%d rollbacks=%d", tx.commitCalls, tx.rollbackCalls)
	}
}

func TestExecuteSQLTransactionBoundsCommitTime(t *testing.T) {
	rows := &fakeSQLRows{commandTag: pgconn.NewCommandTag("INSERT 0 1")}
	tx := &fakeSQLTransaction{rows: rows, waitForCommitContext: true}
	executor := &branchEndpointSQLQueryExecutor{commitTimeout: 10 * time.Millisecond, maxRows: 200, maxBytes: 1024, maxCellBytes: 128}

	started := time.Now()
	_, err := executor.executeTransaction(context.Background(), tx, "main", "postgres", "INSERT INTO app.documents(title) VALUES ('x')", false)
	if !errors.Is(err, ErrSQLCommitOutcomeUnknown) {
		t.Fatalf("expected unknown commit outcome after timeout, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("expected bounded commit time, took %s", elapsed)
	}
}

type fakeSQLTransaction struct {
	rows                 *fakeSQLRows
	queryErr             error
	commitErr            error
	commitCalls          int
	rollbackCalls        int
	rowsClosedAtCommit   bool
	waitForCommitContext bool
	closed               bool
}

func (f *fakeSQLTransaction) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	if f.queryErr != nil {
		return nil, f.queryErr
	}
	return f.rows, nil
}

func (f *fakeSQLTransaction) Commit(ctx context.Context) error {
	f.commitCalls++
	f.rowsClosedAtCommit = f.rows == nil || f.rows.closed
	f.closed = true
	if f.waitForCommitContext {
		<-ctx.Done()
		return ctx.Err()
	}
	return f.commitErr
}

func (f *fakeSQLTransaction) Rollback(_ context.Context) error {
	f.rollbackCalls++
	if f.closed {
		return pgx.ErrTxClosed
	}
	f.closed = true
	return nil
}

type fakeSQLRows struct {
	fieldDescriptions []pgconn.FieldDescription
	values            [][]any
	commandTag        pgconn.CommandTag
	err               error
	index             int
	closed            bool
}

func (f *fakeSQLRows) Close() {
	f.closed = true
}

func (f *fakeSQLRows) Err() error {
	if !f.closed {
		return nil
	}
	return f.err
}

func (f *fakeSQLRows) CommandTag() pgconn.CommandTag {
	if !f.closed {
		return pgconn.CommandTag{}
	}
	return f.commandTag
}

func (f *fakeSQLRows) FieldDescriptions() []pgconn.FieldDescription {
	return f.fieldDescriptions
}

func (f *fakeSQLRows) Next() bool {
	if f.closed {
		return false
	}
	if f.index >= len(f.values) {
		f.closed = true
		return false
	}
	f.index++
	return true
}

func (f *fakeSQLRows) Scan(_ ...any) error {
	return nil
}

func (f *fakeSQLRows) Values() ([]any, error) {
	return f.values[f.index-1], nil
}

func (f *fakeSQLRows) RawValues() [][]byte {
	return nil
}

func (f *fakeSQLRows) Conn() *pgx.Conn {
	return nil
}
