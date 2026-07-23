package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestSchemaCatalogAPIListsBoundedObjects(t *testing.T) {
	tableBytes := int64(4096)
	totalBytes := int64(8192)
	estimatedRows := int64(42)
	executor := &fakeSQLQueryExecutor{schemaCatalog: sqlSchemaCatalog{
		Branch: "main", Database: "pleroma",
		Schemas: []sqlSchemaSummary{{Name: "public", ObjectCount: 2}},
		Tables:  []sqlSchemaTableSummary{{Schema: "public", Name: "posts", Kind: "table", EstimatedRows: &estimatedRows, TableBytes: &tableBytes, TotalBytes: &totalBytes}},
		Limit:   25, Offset: 50, HasMore: true,
	}}
	handler := New(Config{Version: "test", SQLExecutor: executor})

	res := performRequest(t, handler, http.MethodGet, "/api/v1/branches/main/schema?database=pleroma&schema=public&search=post&limit=25&offset=50&include_system=true", "")
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, res.Code, res.Body.String())
	}
	if len(executor.schemaCatalogCalls) != 1 {
		t.Fatalf("expected one schema catalog call, got %d", len(executor.schemaCatalogCalls))
	}
	call := executor.schemaCatalogCalls[0]
	if call.branch != "main" || call.database != "pleroma" || call.filter.Schema != "public" || call.filter.Search != "post" || call.filter.Limit != 25 || call.filter.Offset != 50 || !call.filter.IncludeSystem {
		t.Fatalf("unexpected schema catalog call: %+v", call)
	}

	var payload sqlSchemaCatalogEnvelope
	decodeJSON(t, res, &payload)
	if len(payload.Catalog.Schemas) != 1 || len(payload.Catalog.Tables) != 1 || payload.Catalog.Tables[0].Name != "posts" || !payload.Catalog.HasMore {
		t.Fatalf("unexpected schema catalog payload: %+v", payload.Catalog)
	}
}

func TestSchemaTableAPIIncludesDetailsAndGeneratedQueries(t *testing.T) {
	estimatedRows := int64(42)
	executor := &fakeSQLQueryExecutor{schemaDetail: sqlSchemaTableDetail{
		Branch: "main", Database: "pleroma", Schema: "public", Name: "posts", Kind: "table", EstimatedRows: &estimatedRows,
		Columns:           []sqlSchemaColumn{{Position: 1, Name: "id", DataType: "bigint", NotNull: true, Identity: "by default"}},
		Indexes:           []sqlSchemaIndex{{Name: "posts_pkey", Primary: true, Unique: true, Definition: "CREATE UNIQUE INDEX posts_pkey ON public.posts USING btree (id)"}},
		Constraints:       []sqlSchemaConstraint{{Name: "posts_pkey", Type: "primary_key", Definition: "PRIMARY KEY (id)"}},
		InspectionQueries: []sqlInspectionQuery{{Name: "Preview rows", SQL: `SELECT * FROM "public"."posts" LIMIT 100;`}},
	}}
	handler := New(Config{Version: "test", SQLExecutor: executor})

	res := performRequest(t, handler, http.MethodGet, "/api/v1/branches/main/schema/table?database=pleroma&schema=public&table=posts&include_system=true", "")
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, res.Code, res.Body.String())
	}
	if len(executor.schemaDetailCalls) != 1 || executor.schemaDetailCalls[0].schema != "public" || executor.schemaDetailCalls[0].table != "posts" || !executor.schemaDetailCalls[0].includeSystem {
		t.Fatalf("unexpected schema detail calls: %+v", executor.schemaDetailCalls)
	}

	var payload sqlSchemaTableEnvelope
	decodeJSON(t, res, &payload)
	if len(payload.Table.Columns) != 1 || len(payload.Table.Indexes) != 1 || len(payload.Table.Constraints) != 1 || len(payload.Table.InspectionQueries) != 1 {
		t.Fatalf("unexpected schema detail payload: %+v", payload.Table)
	}
	if payload.Table.InspectionQueries[0].SQL != `SELECT * FROM "public"."posts" LIMIT 100;` {
		t.Fatalf("unexpected generated inspection query %q", payload.Table.InspectionQueries[0].SQL)
	}
}

func TestSchemaBrowserAPIValidation(t *testing.T) {
	handler := New(Config{Version: "test", SQLExecutor: &fakeSQLQueryExecutor{}})
	tests := []struct {
		name string
		path string
	}{
		{name: "missing database", path: "/api/v1/branches/main/schema"},
		{name: "invalid database", path: "/api/v1/branches/main/schema?database=bad%00db"},
		{name: "search too long", path: "/api/v1/branches/main/schema?database=postgres&search=" + strings.Repeat("x", 129)},
		{name: "limit too high", path: "/api/v1/branches/main/schema?database=postgres&limit=101"},
		{name: "negative offset", path: "/api/v1/branches/main/schema?database=postgres&offset=-1"},
		{name: "detail missing schema", path: "/api/v1/branches/main/schema/table?database=postgres&table=posts"},
		{name: "detail missing table", path: "/api/v1/branches/main/schema/table?database=postgres&schema=public"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := performRequest(t, handler, http.MethodGet, tt.path, "")
			if res.Code != http.StatusBadRequest {
				t.Fatalf("expected status %d, got %d: %s", http.StatusBadRequest, res.Code, res.Body.String())
			}
			assertAPIErrorCode(t, res, "validation_error")
		})
	}

	res := performRequest(t, handler, http.MethodGet, "/api/v1/branches/missing/schema?database=postgres", "")
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected unknown branch status %d, got %d", http.StatusNotFound, res.Code)
	}
	assertAPIErrorCode(t, res, "not_found")
}

func TestSchemaBrowserAPIMapsExecutorErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "object missing", err: errSQLSchemaObjectNotFound, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "endpoint unavailable", err: fmt.Errorf("%w: endpoint unavailable", ErrPrimaryEndpointUnavailable), wantStatus: http.StatusServiceUnavailable, wantCode: "endpoint_unavailable"},
		{name: "timeout", err: context.DeadlineExceeded, wantStatus: http.StatusRequestTimeout, wantCode: "timeout"},
		{name: "statement timeout", err: &sqlExecutionError{Message: "canceling statement", SQLState: "57014"}, wantStatus: http.StatusRequestTimeout, wantCode: "timeout"},
		{name: "database missing", err: &sqlExecutionError{Message: "database missing", SQLState: "3D000"}, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "permission denied", err: &sqlExecutionError{Message: "permission denied", SQLState: "42501"}, wantStatus: http.StatusForbidden, wantCode: "forbidden"},
		{name: "internal", err: errors.New("catalog failed"), wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &fakeSQLQueryExecutor{schemaDetailErr: tt.err}
			handler := New(Config{Version: "test", SQLExecutor: executor})
			res := performRequest(t, handler, http.MethodGet, "/api/v1/branches/main/schema/table?database=postgres&schema=public&table=posts", "")
			if res.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, res.Code, res.Body.String())
			}
			assertAPIErrorCode(t, res, tt.wantCode)
		})
	}
}

func TestSchemaInspectionQueriesQuoteIdentifiers(t *testing.T) {
	queries := makeSchemaInspectionQueries("odd schema", `table"name`, "table")
	if len(queries) != 3 {
		t.Fatalf("expected three inspection queries, got %d", len(queries))
	}
	wantQualified := `"odd schema"."table""name"`
	if !strings.Contains(queries[0].SQL, wantQualified) || !strings.Contains(queries[1].SQL, wantQualified) {
		t.Fatalf("expected quoted identifier in generated queries: %+v", queries)
	}
	if !strings.Contains(queries[2].SQL, "E'") || !strings.Contains(queries[2].SQL, wantQualified) {
		t.Fatalf("expected quoted regclass in size query: %q", queries[2].SQL)
	}
}

func TestSchemaInspectionSizeQueryEscapesStringSyntax(t *testing.T) {
	queries := makeSchemaInspectionQueries(`odd\schema`, `table'name`, "table")
	sizeSQL := queries[2].SQL
	if !strings.Contains(sizeSQL, `"odd\\schema"."table''name"`) {
		t.Fatalf("expected backslash and quote escaping in size query: %q", sizeSQL)
	}
}

func TestBoundSchemaTablePageNeverReturnsProbeRow(t *testing.T) {
	tables := make([]sqlSchemaTableSummary, 3)
	page, hasMore := boundSchemaTablePage(tables, sqlSchemaCatalogFilter{Limit: 2, Offset: maxSchemaCatalogOffset - 2})
	if len(page) != 2 || !hasMore {
		t.Fatalf("expected page before the maximum offset to expose the final page, got len=%d has_more=%t", len(page), hasMore)
	}
	page, hasMore = boundSchemaTablePage(tables, sqlSchemaCatalogFilter{Limit: 2, Offset: maxSchemaCatalogOffset})
	if len(page) != 2 || hasMore {
		t.Fatalf("expected maximum-offset page of two without another page, got len=%d has_more=%t", len(page), hasMore)
	}
	page, hasMore = boundSchemaTablePage(tables, sqlSchemaCatalogFilter{Limit: 2, Offset: 0})
	if len(page) != 2 || !hasMore {
		t.Fatalf("expected ordinary page of two with another page, got len=%d has_more=%t", len(page), hasMore)
	}
}

func TestSchemaInspectionQueriesMatchRelationStorage(t *testing.T) {
	if got := len(makeSchemaInspectionQueries("public", "a_view", "view")); got != 2 {
		t.Fatalf("expected views to omit size query, got %d actions", got)
	}
	partitioned := makeSchemaInspectionQueries("public", "events", "partitioned_table")
	if len(partitioned) != 3 || partitioned[2].Name != "Partition tree size" || !strings.Contains(partitioned[2].SQL, "pg_partition_tree") {
		t.Fatalf("expected partition tree aggregate query, got %+v", partitioned)
	}
}

func TestSchemaCatalogTransactionIsReadOnlyAndConsistent(t *testing.T) {
	options := schemaCatalogTransactionOptions()
	if options.AccessMode != pgx.ReadOnly || options.IsoLevel != pgx.RepeatableRead {
		t.Fatalf("expected repeatable-read, read-only catalog transaction, got %+v", options)
	}
}
