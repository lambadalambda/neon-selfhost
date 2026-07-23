package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"neon-selfhost/internal/branch"
)

const (
	defaultSchemaCatalogLimit     = 50
	maxSchemaCatalogLimit         = 100
	maxSchemaCatalogOffset        = 100000
	maxSchemaCatalogSearchBytes   = 128
	maxSchemaCatalogSchemas       = 200
	maxSchemaTableColumns         = 200
	maxSchemaTableIndexes         = 100
	maxSchemaTableConstraints     = 100
	schemaCatalogStatementTimeout = 5 * time.Second
	schemaCatalogRequestTimeout   = 5 * time.Second
)

var errSQLSchemaObjectNotFound = errors.New("schema object not found")

type sqlSchemaCatalogFilter struct {
	Schema        string
	Search        string
	Limit         int
	Offset        int
	IncludeSystem bool
}

type sqlSchemaSummary struct {
	Name        string `json:"name"`
	ObjectCount int64  `json:"object_count"`
}

type sqlSchemaTableSummary struct {
	Schema        string `json:"schema"`
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	EstimatedRows *int64 `json:"estimated_rows"`
	TableBytes    *int64 `json:"table_bytes"`
	TotalBytes    *int64 `json:"total_bytes"`
}

type sqlSchemaCatalog struct {
	Branch           string                  `json:"branch"`
	Database         string                  `json:"database"`
	Schemas          []sqlSchemaSummary      `json:"schemas"`
	Tables           []sqlSchemaTableSummary `json:"tables"`
	Limit            int                     `json:"limit"`
	Offset           int                     `json:"offset"`
	HasMore          bool                    `json:"has_more"`
	SchemasTruncated bool                    `json:"schemas_truncated"`
}

type sqlSchemaColumn struct {
	Position   int    `json:"position"`
	Name       string `json:"name"`
	DataType   string `json:"data_type"`
	NotNull    bool   `json:"not_null"`
	Default    string `json:"default,omitempty"`
	Identity   string `json:"identity,omitempty"`
	Generation string `json:"generation,omitempty"`
}

type sqlSchemaIndex struct {
	Name       string `json:"name"`
	Primary    bool   `json:"primary"`
	Unique     bool   `json:"unique"`
	Definition string `json:"definition"`
}

type sqlSchemaConstraint struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Definition string `json:"definition"`
}

type sqlInspectionQuery struct {
	Name string `json:"name"`
	SQL  string `json:"sql"`
}

type sqlSchemaTableDetail struct {
	Branch               string                `json:"branch"`
	Database             string                `json:"database"`
	Schema               string                `json:"schema"`
	Name                 string                `json:"name"`
	Kind                 string                `json:"kind"`
	EstimatedRows        *int64                `json:"estimated_rows"`
	TableBytes           *int64                `json:"table_bytes"`
	TotalBytes           *int64                `json:"total_bytes"`
	Columns              []sqlSchemaColumn     `json:"columns"`
	Indexes              []sqlSchemaIndex      `json:"indexes"`
	Constraints          []sqlSchemaConstraint `json:"constraints"`
	InspectionQueries    []sqlInspectionQuery  `json:"inspection_queries"`
	Truncated            bool                  `json:"truncated"`
	ColumnsTruncated     bool                  `json:"columns_truncated"`
	IndexesTruncated     bool                  `json:"indexes_truncated"`
	ConstraintsTruncated bool                  `json:"constraints_truncated"`
}

type sqlSchemaCatalogEnvelope struct {
	Catalog sqlSchemaCatalog `json:"catalog"`
}

type sqlSchemaTableEnvelope struct {
	Table sqlSchemaTableDetail `json:"table"`
}

func (e *branchEndpointSQLQueryExecutor) ListSchemaTables(ctx context.Context, branchName string, database string, filter sqlSchemaCatalogFilter) (sqlSchemaCatalog, error) {
	ctx, cancel := context.WithTimeout(ctx, schemaCatalogRequestTimeout)
	defer cancel()
	conn, resolvedDatabase, err := e.connect(ctx, branchName, database)
	if err != nil {
		return sqlSchemaCatalog{}, err
	}
	defer closeSQLConnection(conn)

	tx, err := conn.BeginTx(ctx, schemaCatalogTransactionOptions())
	if err != nil {
		return sqlSchemaCatalog{}, mapSQLExecutionError(err)
	}
	defer rollbackSQLTx(tx)
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = '%dms'", schemaCatalogStatementTimeout.Milliseconds())); err != nil {
		return sqlSchemaCatalog{}, mapSQLExecutionError(err)
	}

	schemas, schemasTruncated, err := querySchemaSummaries(ctx, tx, filter.IncludeSystem)
	if err != nil {
		return sqlSchemaCatalog{}, err
	}
	tables, hasMore, err := querySchemaTables(ctx, tx, filter)
	if err != nil {
		return sqlSchemaCatalog{}, err
	}
	return sqlSchemaCatalog{
		Branch: branchName, Database: resolvedDatabase, Schemas: schemas, Tables: tables,
		Limit: filter.Limit, Offset: filter.Offset, HasMore: hasMore, SchemasTruncated: schemasTruncated,
	}, nil
}

func querySchemaSummaries(ctx context.Context, tx pgx.Tx, includeSystem bool) ([]sqlSchemaSummary, bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT n.nspname, count(c.oid)::bigint
		FROM pg_catalog.pg_namespace n
		LEFT JOIN pg_catalog.pg_class c
		  ON c.relnamespace = n.oid
		 AND c.relkind IN ('r', 'p', 'v', 'm', 'f')
		 AND pg_catalog.has_table_privilege(c.oid, 'SELECT')
		WHERE pg_catalog.has_schema_privilege(n.oid, 'USAGE')
		  AND ($1 OR (n.nspname <> 'information_schema' AND left(n.nspname, 3) <> 'pg_'))
		GROUP BY n.nspname
		ORDER BY n.nspname
		LIMIT $2`, includeSystem, maxSchemaCatalogSchemas+1)
	if err != nil {
		return nil, false, mapSQLExecutionError(err)
	}
	defer rows.Close()

	schemas := make([]sqlSchemaSummary, 0)
	for rows.Next() {
		var schema sqlSchemaSummary
		if err := rows.Scan(&schema.Name, &schema.ObjectCount); err != nil {
			return nil, false, mapSQLExecutionError(err)
		}
		schemas = append(schemas, schema)
	}
	if err := rows.Err(); err != nil {
		return nil, false, mapSQLExecutionError(err)
	}
	truncated := len(schemas) > maxSchemaCatalogSchemas
	if truncated {
		schemas = schemas[:maxSchemaCatalogSchemas]
	}
	return schemas, truncated, nil
}

func querySchemaTables(ctx context.Context, tx pgx.Tx, filter sqlSchemaCatalogFilter) ([]sqlSchemaTableSummary, bool, error) {
	rows, err := tx.Query(ctx, `
		WITH candidates AS (
			SELECT c.oid, n.nspname, c.relname, c.relkind,
			       CASE WHEN c.reltuples < 0 OR c.relkind = 'v' THEN NULL ELSE c.reltuples::bigint END AS estimated_rows
			FROM pg_catalog.pg_class c
			JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
			WHERE c.relkind IN ('r', 'p', 'v', 'm', 'f')
			  AND pg_catalog.has_schema_privilege(n.oid, 'USAGE')
			  AND pg_catalog.has_table_privilege(c.oid, 'SELECT')
			  AND ($1 OR (n.nspname <> 'information_schema' AND left(n.nspname, 3) <> 'pg_'))
			  AND ($2 = '' OR n.nspname = $2)
			  AND ($3 = '' OR strpos(lower(n.nspname || '.' || c.relname), lower($3)) > 0)
			ORDER BY n.nspname, c.relname
			LIMIT $4 OFFSET $5
		)
		SELECT nspname, relname,
		       CASE relkind WHEN 'r' THEN 'table' WHEN 'p' THEN 'partitioned_table' WHEN 'v' THEN 'view' WHEN 'm' THEN 'materialized_view' ELSE 'foreign_table' END,
		       estimated_rows,
		       CASE WHEN relkind IN ('r', 'm') THEN pg_catalog.pg_relation_size(oid) END,
		       CASE WHEN relkind IN ('r', 'm') THEN pg_catalog.pg_total_relation_size(oid) END
		FROM candidates
		ORDER BY nspname, relname`, filter.IncludeSystem, filter.Schema, filter.Search, filter.Limit+1, filter.Offset)
	if err != nil {
		return nil, false, mapSQLExecutionError(err)
	}
	defer rows.Close()

	tables := make([]sqlSchemaTableSummary, 0, filter.Limit+1)
	for rows.Next() {
		var table sqlSchemaTableSummary
		if err := rows.Scan(&table.Schema, &table.Name, &table.Kind, &table.EstimatedRows, &table.TableBytes, &table.TotalBytes); err != nil {
			return nil, false, mapSQLExecutionError(err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, false, mapSQLExecutionError(err)
	}
	bounded, hasMore := boundSchemaTablePage(tables, filter)
	return bounded, hasMore, nil
}

func boundSchemaTablePage(tables []sqlSchemaTableSummary, filter sqlSchemaCatalogFilter) ([]sqlSchemaTableSummary, bool) {
	hasExtra := len(tables) > filter.Limit
	if hasExtra {
		tables = tables[:filter.Limit]
	}
	return tables, hasExtra && filter.Offset+filter.Limit <= maxSchemaCatalogOffset
}

func (e *branchEndpointSQLQueryExecutor) InspectSchemaTable(ctx context.Context, branchName string, database string, schemaName string, tableName string, includeSystem bool) (sqlSchemaTableDetail, error) {
	ctx, cancel := context.WithTimeout(ctx, schemaCatalogRequestTimeout)
	defer cancel()
	conn, resolvedDatabase, err := e.connect(ctx, branchName, database)
	if err != nil {
		return sqlSchemaTableDetail{}, err
	}
	defer closeSQLConnection(conn)

	tx, err := conn.BeginTx(ctx, schemaCatalogTransactionOptions())
	if err != nil {
		return sqlSchemaTableDetail{}, mapSQLExecutionError(err)
	}
	defer rollbackSQLTx(tx)
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = '%dms'", schemaCatalogStatementTimeout.Milliseconds())); err != nil {
		return sqlSchemaTableDetail{}, mapSQLExecutionError(err)
	}

	detail := sqlSchemaTableDetail{Branch: branchName, Database: resolvedDatabase, Schema: schemaName, Name: tableName}
	var oid uint32
	var relkind string
	err = tx.QueryRow(ctx, `
		SELECT c.oid, c.relkind::text,
		       CASE WHEN c.reltuples < 0 OR c.relkind = 'v' THEN NULL ELSE c.reltuples::bigint END,
		       CASE WHEN c.relkind IN ('r', 'm') THEN pg_catalog.pg_relation_size(c.oid) END,
		       CASE WHEN c.relkind IN ('r', 'm') THEN pg_catalog.pg_total_relation_size(c.oid) END
		FROM pg_catalog.pg_class c
		JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relname = $2
		  AND c.relkind IN ('r', 'p', 'v', 'm', 'f')
		  AND pg_catalog.has_schema_privilege(n.oid, 'USAGE')
		  AND pg_catalog.has_table_privilege(c.oid, 'SELECT')
		  AND ($3 OR (n.nspname <> 'information_schema' AND left(n.nspname, 3) <> 'pg_'))`, schemaName, tableName, includeSystem).
		Scan(&oid, &relkind, &detail.EstimatedRows, &detail.TableBytes, &detail.TotalBytes)
	if errors.Is(err, pgx.ErrNoRows) {
		return sqlSchemaTableDetail{}, errSQLSchemaObjectNotFound
	}
	if err != nil {
		return sqlSchemaTableDetail{}, mapSQLExecutionError(err)
	}
	detail.Kind = schemaRelationKind(relkind)

	detail.Columns, detail.ColumnsTruncated, err = querySchemaColumns(ctx, tx, oid)
	if err != nil {
		return sqlSchemaTableDetail{}, err
	}
	detail.Indexes, detail.IndexesTruncated, err = querySchemaIndexes(ctx, tx, oid)
	if err != nil {
		return sqlSchemaTableDetail{}, err
	}
	detail.Constraints, detail.ConstraintsTruncated, err = querySchemaConstraints(ctx, tx, oid)
	if err != nil {
		return sqlSchemaTableDetail{}, err
	}
	detail.Truncated = detail.ColumnsTruncated || detail.IndexesTruncated || detail.ConstraintsTruncated
	detail.InspectionQueries = makeSchemaInspectionQueries(schemaName, tableName, detail.Kind)
	return detail, nil
}

func schemaCatalogTransactionOptions() pgx.TxOptions {
	return pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead}
}

func querySchemaColumns(ctx context.Context, tx pgx.Tx, oid uint32) ([]sqlSchemaColumn, bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.attnum, a.attname, pg_catalog.format_type(a.atttypid, a.atttypmod), a.attnotnull,
		       left(coalesce(pg_catalog.pg_get_expr(d.adbin, d.adrelid), ''), 2048),
		       CASE a.attidentity WHEN 'a' THEN 'always' WHEN 'd' THEN 'by default' ELSE '' END,
		       CASE a.attgenerated WHEN 's' THEN 'stored' ELSE '' END
		FROM pg_catalog.pg_attribute a
		LEFT JOIN pg_catalog.pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
		WHERE a.attrelid = $1 AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum
		LIMIT $2`, oid, maxSchemaTableColumns+1)
	if err != nil {
		return nil, false, mapSQLExecutionError(err)
	}
	defer rows.Close()
	columns := make([]sqlSchemaColumn, 0)
	for rows.Next() {
		var column sqlSchemaColumn
		if err := rows.Scan(&column.Position, &column.Name, &column.DataType, &column.NotNull, &column.Default, &column.Identity, &column.Generation); err != nil {
			return nil, false, mapSQLExecutionError(err)
		}
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, false, mapSQLExecutionError(err)
	}
	truncated := len(columns) > maxSchemaTableColumns
	if truncated {
		columns = columns[:maxSchemaTableColumns]
	}
	return columns, truncated, nil
}

func querySchemaIndexes(ctx context.Context, tx pgx.Tx, oid uint32) ([]sqlSchemaIndex, bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT ci.relname, i.indisprimary, i.indisunique, left(pg_catalog.pg_get_indexdef(i.indexrelid), 2048)
		FROM pg_catalog.pg_index i
		JOIN pg_catalog.pg_class ci ON ci.oid = i.indexrelid
		WHERE i.indrelid = $1
		ORDER BY i.indisprimary DESC, ci.relname
		LIMIT $2`, oid, maxSchemaTableIndexes+1)
	if err != nil {
		return nil, false, mapSQLExecutionError(err)
	}
	defer rows.Close()
	indexes := make([]sqlSchemaIndex, 0)
	for rows.Next() {
		var index sqlSchemaIndex
		if err := rows.Scan(&index.Name, &index.Primary, &index.Unique, &index.Definition); err != nil {
			return nil, false, mapSQLExecutionError(err)
		}
		indexes = append(indexes, index)
	}
	if err := rows.Err(); err != nil {
		return nil, false, mapSQLExecutionError(err)
	}
	truncated := len(indexes) > maxSchemaTableIndexes
	if truncated {
		indexes = indexes[:maxSchemaTableIndexes]
	}
	return indexes, truncated, nil
}

func querySchemaConstraints(ctx context.Context, tx pgx.Tx, oid uint32) ([]sqlSchemaConstraint, bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT conname,
		       CASE contype WHEN 'p' THEN 'primary_key' WHEN 'f' THEN 'foreign_key' WHEN 'u' THEN 'unique' WHEN 'c' THEN 'check' WHEN 'x' THEN 'exclusion' ELSE contype::text END,
		       left(pg_catalog.pg_get_constraintdef(oid, true), 2048)
		FROM pg_catalog.pg_constraint
		WHERE conrelid = $1
		ORDER BY conname
		LIMIT $2`, oid, maxSchemaTableConstraints+1)
	if err != nil {
		return nil, false, mapSQLExecutionError(err)
	}
	defer rows.Close()
	constraints := make([]sqlSchemaConstraint, 0)
	for rows.Next() {
		var constraint sqlSchemaConstraint
		if err := rows.Scan(&constraint.Name, &constraint.Type, &constraint.Definition); err != nil {
			return nil, false, mapSQLExecutionError(err)
		}
		constraints = append(constraints, constraint)
	}
	if err := rows.Err(); err != nil {
		return nil, false, mapSQLExecutionError(err)
	}
	truncated := len(constraints) > maxSchemaTableConstraints
	if truncated {
		constraints = constraints[:maxSchemaTableConstraints]
	}
	return constraints, truncated, nil
}

func schemaRelationKind(relkind string) string {
	switch relkind {
	case "r":
		return "table"
	case "p":
		return "partitioned_table"
	case "v":
		return "view"
	case "m":
		return "materialized_view"
	default:
		return "foreign_table"
	}
}

func makeSchemaInspectionQueries(schemaName string, tableName string, kind string) []sqlInspectionQuery {
	qualified := pgx.Identifier{schemaName, tableName}.Sanitize()
	regclass := strings.ReplaceAll(strings.ReplaceAll(qualified, `\`, `\\`), "'", "''")
	queries := []sqlInspectionQuery{
		{Name: "Preview rows", SQL: "SELECT * FROM " + qualified + " LIMIT 100;"},
		{Name: "Exact row count", SQL: "SELECT count(*) FROM " + qualified + ";"},
	}
	switch kind {
	case "table", "materialized_view":
		queries = append(queries, sqlInspectionQuery{Name: "Table size", SQL: "SELECT pg_size_pretty(pg_total_relation_size(E'" + regclass + "'::regclass));"})
	case "partitioned_table":
		queries = append(queries, sqlInspectionQuery{Name: "Partition tree size", SQL: "SELECT pg_size_pretty(sum(pg_total_relation_size(relid))) FROM pg_partition_tree(E'" + regclass + "'::regclass) WHERE isleaf;"})
	}
	return queries
}

func registerSchemaBrowserRoutes(mux *http.ServeMux, branches *branch.Store, executor SQLQueryExecutor, logger *slog.Logger) {
	mux.HandleFunc("GET /api/v1/branches/{name}/schema", func(w http.ResponseWriter, r *http.Request) {
		branchName, database, ok := validateSchemaBrowserContext(w, r, branches)
		if !ok {
			return
		}
		filter, err := parseSchemaCatalogFilter(r)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		catalog, err := executor.ListSchemaTables(r.Context(), branchName, database, filter)
		if err != nil {
			logger.Warn("schema catalog listing failed", "branch", branchName, "database", database, "error", err)
			writeSchemaBrowserError(w, err, "schema catalog")
			return
		}
		writeJSON(w, http.StatusOK, sqlSchemaCatalogEnvelope{Catalog: catalog})
	})

	mux.HandleFunc("GET /api/v1/branches/{name}/schema/table", func(w http.ResponseWriter, r *http.Request) {
		branchName, database, ok := validateSchemaBrowserContext(w, r, branches)
		if !ok {
			return
		}
		schemaName := r.URL.Query().Get("schema")
		tableName := r.URL.Query().Get("table")
		if err := validateSchemaIdentifier(schemaName, "schema"); err != nil {
			writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		if err := validateSchemaIdentifier(tableName, "table"); err != nil {
			writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		includeSystem, err := parseSchemaIncludeSystem(r)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		detail, err := executor.InspectSchemaTable(r.Context(), branchName, database, schemaName, tableName, includeSystem)
		if err != nil {
			logger.Warn("schema table inspection failed", "branch", branchName, "database", database, "schema", schemaName, "table", tableName, "error", err)
			writeSchemaBrowserError(w, err, "schema table inspection")
			return
		}
		writeJSON(w, http.StatusOK, sqlSchemaTableEnvelope{Table: detail})
	})
}

func validateSchemaBrowserContext(w http.ResponseWriter, r *http.Request, branches *branch.Store) (string, string, bool) {
	branchName := strings.TrimSpace(r.PathValue("name"))
	if branchName == "" {
		writeJSONError(w, http.StatusBadRequest, "validation_error", "branch name is required")
		return "", "", false
	}
	if _, err := branches.GetActive(branchName); err != nil {
		if errors.Is(err, branch.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "not_found", err.Error())
		} else {
			writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		}
		return "", "", false
	}
	database := r.URL.Query().Get("database")
	if database == "" {
		writeJSONError(w, http.StatusBadRequest, "validation_error", "database is required")
		return "", "", false
	}
	if err := validateSQLDatabaseName(database); err != nil {
		writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
		return "", "", false
	}
	return branchName, database, true
}

func parseSchemaCatalogFilter(r *http.Request) (sqlSchemaCatalogFilter, error) {
	filter := sqlSchemaCatalogFilter{
		Schema: r.URL.Query().Get("schema"), Search: strings.TrimSpace(r.URL.Query().Get("search")), Limit: defaultSchemaCatalogLimit,
	}
	if filter.Schema != "" {
		if err := validateSchemaIdentifier(filter.Schema, "schema"); err != nil {
			return sqlSchemaCatalogFilter{}, err
		}
	}
	if len(filter.Search) > maxSchemaCatalogSearchBytes || !utf8.ValidString(filter.Search) {
		return sqlSchemaCatalogFilter{}, fmt.Errorf("search must be valid UTF-8 and at most %d bytes", maxSchemaCatalogSearchBytes)
	}
	for _, value := range filter.Search {
		if value == 0 || unicode.IsControl(value) {
			return sqlSchemaCatalogFilter{}, errors.New("search contains invalid control characters")
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > maxSchemaCatalogLimit {
			return sqlSchemaCatalogFilter{}, fmt.Errorf("limit must be between 1 and %d", maxSchemaCatalogLimit)
		}
		filter.Limit = limit
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("offset")); raw != "" {
		offset, err := strconv.Atoi(raw)
		if err != nil || offset < 0 || offset > maxSchemaCatalogOffset {
			return sqlSchemaCatalogFilter{}, fmt.Errorf("offset must be between 0 and %d", maxSchemaCatalogOffset)
		}
		filter.Offset = offset
	}
	includeSystem, err := parseSchemaIncludeSystem(r)
	if err != nil {
		return sqlSchemaCatalogFilter{}, err
	}
	filter.IncludeSystem = includeSystem
	return filter, nil
}

func parseSchemaIncludeSystem(r *http.Request) (bool, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("include_system"))
	if raw == "" {
		return false, nil
	}
	includeSystem, err := strconv.ParseBool(raw)
	if err != nil {
		return false, errors.New("include_system must be true or false")
	}
	return includeSystem, nil
}

func validateSchemaIdentifier(value string, label string) error {
	if value == "" {
		return fmt.Errorf("%s is required", label)
	}
	if len(value) > 63 {
		return fmt.Errorf("%s exceeds 63 bytes", label)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", label)
	}
	for _, character := range value {
		if character == 0 || unicode.IsControl(character) {
			return fmt.Errorf("%s contains invalid control characters", label)
		}
	}
	return nil
}

func writeSchemaBrowserError(w http.ResponseWriter, err error, operation string) {
	var sqlErr *sqlExecutionError
	errors.As(err, &sqlErr)
	switch {
	case errors.Is(err, errSQLSchemaObjectNotFound), errors.Is(err, branch.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, "not_found", err.Error())
	case isPrimaryEndpointUnavailable(err):
		writeJSONError(w, http.StatusServiceUnavailable, "endpoint_unavailable", err.Error())
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		writeJSONError(w, http.StatusRequestTimeout, "timeout", operation+" timed out")
	case sqlErr != nil && sqlErr.SQLState == "57014":
		writeJSONError(w, http.StatusRequestTimeout, "timeout", operation+" timed out")
	case sqlErr != nil && sqlErr.SQLState == "3D000":
		writeJSONError(w, http.StatusNotFound, "not_found", "database does not exist")
	case sqlErr != nil && sqlErr.SQLState == "42501":
		writeJSONError(w, http.StatusForbidden, "forbidden", "catalog access is not permitted")
	default:
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
