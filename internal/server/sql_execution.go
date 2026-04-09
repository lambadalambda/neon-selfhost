package server

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"neon-selfhost/internal/branch"
)

const (
	defaultSQLExecutionConnectTimeout   = 15 * time.Second
	defaultSQLExecutionStatementTimeout = 10 * time.Second
	defaultSQLExecutionLockTimeout      = 1 * time.Second
	defaultSQLExecutionCleanupTimeout   = 3 * time.Second
	defaultSQLExecutionMaxRows          = 200
	defaultSQLExecutionMaxBytes         = 1 << 20
	defaultSQLExecutionMaxQueryBytes    = 64 * 1024
	defaultSQLExecutionMaxCellBytes     = 8 * 1024
)

type SQLQueryExecutor interface {
	Execute(ctx context.Context, branchName string, query string, readOnly bool) (sqlExecutionResult, error)
}

type sqlExecutionResult struct {
	Branch     string
	ReadOnly   bool
	CommandTag string
	DurationMS int64
	Truncated  bool
	MaxRows    int
	MaxBytes   int
	Columns    []sqlExecutionColumn
	Rows       [][]any
	RowCount   int
}

type sqlExecutionColumn struct {
	Name    string
	Type    string
	TypeOID uint32
}

type sqlExecutionError struct {
	Message  string
	SQLState string
	Position int
}

func (e *sqlExecutionError) Error() string {
	if e == nil {
		return "sql execution failed"
	}

	message := strings.TrimSpace(e.Message)
	if message == "" {
		message = "sql execution failed"
	}

	if strings.TrimSpace(e.SQLState) == "" {
		return message
	}

	if e.Position > 0 {
		return fmt.Sprintf("%s (SQLSTATE %s, position %d)", message, e.SQLState, e.Position)
	}

	return fmt.Sprintf("%s (SQLSTATE %s)", message, e.SQLState)
}

type noopSQLQueryExecutor struct{}

func NewNoopSQLQueryExecutor() SQLQueryExecutor {
	return noopSQLQueryExecutor{}
}

func (noopSQLQueryExecutor) Execute(_ context.Context, _ string, _ string, _ bool) (sqlExecutionResult, error) {
	return sqlExecutionResult{}, fmt.Errorf("%w: sql execution requires docker mode", ErrPrimaryEndpointUnavailable)
}

type branchEndpointSQLQueryExecutor struct {
	branchEndpoints  BranchEndpointController
	connectTimeout   time.Duration
	statementTimeout time.Duration
	lockTimeout      time.Duration
	maxRows          int
	maxBytes         int
	maxCellBytes     int
}

func NewBranchEndpointSQLQueryExecutor(branchEndpoints BranchEndpointController) SQLQueryExecutor {
	if branchEndpoints == nil {
		return NewNoopSQLQueryExecutor()
	}

	switch branchEndpoints.(type) {
	case noopBranchEndpointController, *noopBranchEndpointController:
		return NewNoopSQLQueryExecutor()
	}

	return &branchEndpointSQLQueryExecutor{
		branchEndpoints:  branchEndpoints,
		connectTimeout:   defaultSQLExecutionConnectTimeout,
		statementTimeout: defaultSQLExecutionStatementTimeout,
		lockTimeout:      defaultSQLExecutionLockTimeout,
		maxRows:          defaultSQLExecutionMaxRows,
		maxBytes:         defaultSQLExecutionMaxBytes,
		maxCellBytes:     defaultSQLExecutionMaxCellBytes,
	}
}

func validateSingleStatementQuery(query string) error {
	if len(query) > defaultSQLExecutionMaxQueryBytes {
		return fmt.Errorf("query exceeds %d bytes", defaultSQLExecutionMaxQueryBytes)
	}

	statementCount, err := countSQLStatements(query)
	if err != nil {
		return err
	}

	if statementCount == 0 {
		return errors.New("query is required")
	}

	if statementCount > 1 {
		return errors.New("only one SQL statement is allowed per execution")
	}

	return nil
}

func countSQLStatements(query string) (int, error) {
	var count int
	var hasToken bool

	inSingle := false
	inDouble := false
	inLineComment := false
	inBlockComment := false
	dollarDelimiter := ""

	for i := 0; i < len(query); i++ {
		ch := query[i]

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}

		if inBlockComment {
			if ch == '*' && i+1 < len(query) && query[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		if dollarDelimiter != "" {
			if strings.HasPrefix(query[i:], dollarDelimiter) {
				delimiterLength := len(dollarDelimiter)
				dollarDelimiter = ""
				i += delimiterLength - 1
			}
			continue
		}

		if inSingle {
			if ch == '\\' && i+1 < len(query) {
				i++
				continue
			}
			if ch == '\'' {
				if i+1 < len(query) && query[i+1] == '\'' {
					i++
					continue
				}
				inSingle = false
			}
			continue
		}

		if inDouble {
			if ch == '"' {
				if i+1 < len(query) && query[i+1] == '"' {
					i++
					continue
				}
				inDouble = false
			}
			continue
		}

		if ch == '-' && i+1 < len(query) && query[i+1] == '-' {
			inLineComment = true
			i++
			continue
		}

		if ch == '/' && i+1 < len(query) && query[i+1] == '*' {
			inBlockComment = true
			i++
			continue
		}

		if ch == '\'' {
			hasToken = true
			inSingle = true
			continue
		}

		if ch == '"' {
			hasToken = true
			inDouble = true
			continue
		}

		if ch == '$' {
			delimiter := parseDollarDelimiter(query[i:])
			if delimiter != "" {
				hasToken = true
				dollarDelimiter = delimiter
				i += len(delimiter) - 1
				continue
			}
		}

		if ch == ';' {
			if hasToken {
				count++
				hasToken = false
			}
			continue
		}

		if !unicode.IsSpace(rune(ch)) {
			hasToken = true
		}
	}

	if inSingle || inDouble || inBlockComment || dollarDelimiter != "" {
		return 0, errors.New("query contains an unterminated string, comment, or dollar-quoted block")
	}

	if hasToken {
		count++
	}

	return count, nil
}

func parseDollarDelimiter(value string) string {
	if len(value) < 2 || value[0] != '$' {
		return ""
	}

	for i := 1; i < len(value); i++ {
		if value[i] == '$' {
			for j := 1; j < i; j++ {
				ch := value[j]
				if !(ch == '_' || (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')) {
					return ""
				}
			}
			return value[:i+1]
		}

		if value[i] == '\n' || value[i] == '\r' {
			return ""
		}
	}

	return ""
}

func (e *branchEndpointSQLQueryExecutor) Execute(ctx context.Context, branchName string, query string, readOnly bool) (sqlExecutionResult, error) {
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return sqlExecutionResult{}, branch.ErrNotFound
	}

	if err := validateSingleStatementQuery(query); err != nil {
		return sqlExecutionResult{}, err
	}

	connection, err := e.branchEndpoints.Connection(branchName)
	if err != nil {
		return sqlExecutionResult{}, err
	}

	if !connection.Published || connection.Port <= 0 {
		return sqlExecutionResult{}, fmt.Errorf("%w: branch endpoint is not published", ErrPrimaryEndpointUnavailable)
	}

	if strings.TrimSpace(connection.Password) == "" || strings.TrimSpace(connection.User) == "" || strings.TrimSpace(connection.Database) == "" {
		return sqlExecutionResult{}, fmt.Errorf("%w: branch endpoint credentials are incomplete", ErrPrimaryEndpointUnavailable)
	}

	host := strings.TrimSpace(connection.Host)
	if host == "" {
		host = defaultBranchEndpointHost
	}

	connectionURI := (&url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(connection.User, connection.Password),
		Host:     fmt.Sprintf("%s:%d", host, connection.Port),
		Path:     "/" + url.PathEscape(connection.Database),
		RawQuery: "sslmode=disable",
	}).String()

	config, err := pgx.ParseConfig(connectionURI)
	if err != nil {
		return sqlExecutionResult{}, fmt.Errorf("%w: parse branch endpoint connection: %v", ErrPrimaryEndpointUnavailable, err)
	}

	config.ConnectTimeout = e.connectTimeout
	config.RuntimeParams["application_name"] = "neon-selfhost-sql-editor"
	config.RuntimeParams["default_transaction_read_only"] = "on"
	config.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", e.statementTimeout.Milliseconds())
	config.RuntimeParams["lock_timeout"] = fmt.Sprintf("%d", e.lockTimeout.Milliseconds())
	config.RuntimeParams["idle_in_transaction_session_timeout"] = fmt.Sprintf("%d", e.statementTimeout.Milliseconds())

	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return sqlExecutionResult{}, fmt.Errorf("%w: connect to branch endpoint: %v", ErrPrimaryEndpointUnavailable, err)
	}
	defer closeSQLConnection(conn)

	accessMode := pgx.ReadOnly
	if !readOnly {
		accessMode = pgx.ReadWrite
	}

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: accessMode})
	if err != nil {
		return sqlExecutionResult{}, mapSQLExecutionError(err)
	}
	defer rollbackSQLTx(tx)

	started := time.Now()
	rows, err := tx.Query(ctx, query)
	if err != nil {
		return sqlExecutionResult{}, mapSQLExecutionError(err)
	}
	defer rows.Close()

	fieldDescriptions := rows.FieldDescriptions()
	columns := make([]sqlExecutionColumn, 0, len(fieldDescriptions))
	for _, field := range fieldDescriptions {
		columns = append(columns, sqlExecutionColumn{
			Name:    string(field.Name),
			Type:    fmt.Sprintf("oid:%d", field.DataTypeOID),
			TypeOID: field.DataTypeOID,
		})
	}

	outRows := make([][]any, 0, e.maxRows)
	bytesUsed := 0
	truncated := false

	for rows.Next() {
		values, scanErr := rows.Values()
		if scanErr != nil {
			return sqlExecutionResult{}, &sqlExecutionError{Message: strings.TrimSpace(scanErr.Error())}
		}

		normalized := make([]any, 0, len(values))
		rowBytes := 0
		for _, value := range values {
			normalizedValue := normalizeSQLResultValue(value, e.maxCellBytes)
			normalized = append(normalized, normalizedValue)
			rowBytes += len(fmt.Sprintf("%v", normalizedValue))
		}

		if len(outRows) >= e.maxRows || bytesUsed+rowBytes > e.maxBytes {
			truncated = true
			break
		}

		outRows = append(outRows, normalized)
		bytesUsed += rowBytes
	}

	if err := rows.Err(); err != nil {
		return sqlExecutionResult{}, mapSQLExecutionError(err)
	}

	result := sqlExecutionResult{
		Branch:     branchName,
		ReadOnly:   readOnly,
		CommandTag: rows.CommandTag().String(),
		DurationMS: time.Since(started).Milliseconds(),
		Truncated:  truncated,
		MaxRows:    e.maxRows,
		MaxBytes:   e.maxBytes,
		Columns:    columns,
		Rows:       outRows,
		RowCount:   len(outRows),
	}

	if result.CommandTag == "" {
		result.CommandTag = "QUERY"
	}

	return result, nil
}

func normalizeSQLResultValue(value any, maxCellBytes int) any {
	if value == nil {
		return nil
	}

	switch typed := value.(type) {
	case []byte:
		if maxCellBytes > 0 && len(typed) > maxCellBytes {
			return string(typed[:maxCellBytes]) + "…"
		}
		return string(typed)
	case string:
		if maxCellBytes > 0 && len(typed) > maxCellBytes {
			return typed[:maxCellBytes] + "…"
		}
		return typed
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return typed
	}
}

func mapSQLExecutionError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return &sqlExecutionError{
			Message:  strings.TrimSpace(pgErr.Message),
			SQLState: strings.TrimSpace(pgErr.Code),
			Position: int(pgErr.Position),
		}
	}

	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return err
	}

	return &sqlExecutionError{Message: strings.TrimSpace(err.Error())}
}

func closeSQLConnection(conn *pgx.Conn) {
	if conn == nil {
		return
	}

	cleanupCtx, cancel := newSQLExecutionCleanupContext()
	defer cancel()
	_ = conn.Close(cleanupCtx)
}

func rollbackSQLTx(tx pgx.Tx) {
	if tx == nil {
		return
	}

	cleanupCtx, cancel := newSQLExecutionCleanupContext()
	defer cancel()
	_ = tx.Rollback(cleanupCtx)
}

func newSQLExecutionCleanupContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), defaultSQLExecutionCleanupTimeout)
}
