package server

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"neon-selfhost/internal/branch"
)

type closeableHandler struct {
	http.Handler
	closer io.Closer
}

func (h closeableHandler) Close() error {
	if h.closer == nil {
		return nil
	}

	return h.closer.Close()
}

const (
	defaultJSONRequestBodyMaxBytes int64 = 1 << 20
	sqlJSONRequestBodyMaxBytes     int64 = 128 * 1024
	autoPublishResolveMaxAttempts        = 6
	autoPublishResolveBaseDelay          = 80 * time.Millisecond
	autoPublishResolveMaxDelay           = 1500 * time.Millisecond
)

type Config struct {
	Version                  string
	BranchStore              *branch.Store
	BranchAttachmentResolver BranchAttachmentResolver
	PrimaryEndpoint          PrimaryEndpointController
	BranchEndpoints          BranchEndpointController
	SQLExecutor              SQLQueryExecutor
	OperationDBPath          string
	LegacyOperationLogPath   string
	BranchStoreMode          string
	BranchSchemaVersion      int

	BasicAuthUser     string
	BasicAuthPassword string
	Logger            *slog.Logger
}

type statusResponse struct {
	Status      string                   `json:"status"`
	Service     string                   `json:"service"`
	Version     string                   `json:"version"`
	Persistence persistenceStatusPayload `json:"persistence"`
}

type persistenceStatusPayload struct {
	BranchStoreMode        string `json:"branch_store_mode"`
	OperationStoreMode     string `json:"operation_store_mode"`
	DBPath                 string `json:"db_path,omitempty"`
	BranchSchemaVersion    int    `json:"branch_schema_version,omitempty"`
	OperationSchemaVersion int    `json:"operation_schema_version,omitempty"`
}

type healthResponse struct {
	Status string               `json:"status"`
	Checks []healthCheckPayload `json:"checks"`
}

type branchResponse struct {
	Branch branchPayload `json:"branch"`
}

type branchesResponse struct {
	Branches []branchPayload `json:"branches"`
}

type operationsResponse struct {
	Operations []operationPayload `json:"operations"`
}

type createBranchRequest struct {
	Name   string `json:"name"`
	Parent string `json:"parent"`
}

type restoreRequest struct {
	Name         string `json:"name"`
	SourceBranch string `json:"source_branch"`
	Timestamp    string `json:"timestamp"`
}

type switchPrimaryEndpointRequest struct {
	Branch string `json:"branch"`
}

type sqlExecuteRequest struct {
	SQL         string `json:"sql"`
	AllowWrites bool   `json:"allow_writes"`
}

type apiErrorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type branchPayload struct {
	Name      string  `json:"name"`
	Parent    string  `json:"parent"`
	CreatedAt string  `json:"created_at"`
	Deleted   bool    `json:"deleted"`
	DeletedAt *string `json:"deleted_at,omitempty"`
}

type operationPayload struct {
	Type       string  `json:"type"`
	Status     string  `json:"status"`
	Message    string  `json:"message,omitempty"`
	StartedAt  string  `json:"started_at"`
	FinishedAt *string `json:"finished_at,omitempty"`
}

type healthCheckPayload struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type restoreResponse struct {
	Restore restorePayload `json:"restore"`
}

type restorePayload struct {
	Branch      branchPayload `json:"branch"`
	RequestedAt string        `json:"requested_at"`
	ResolvedLSN string        `json:"resolved_lsn"`
}

type primaryEndpointConnectionResponse struct {
	Connection primaryEndpointPayload `json:"connection"`
}

type branchEndpointConnectionEnvelope struct {
	Connection branchEndpointPayload `json:"connection"`
}

type branchEndpointsEnvelope struct {
	Endpoints []branchEndpointPayload `json:"endpoints"`
}

type sqlExecuteEnvelope struct {
	Result sqlExecutePayload `json:"result"`
}

type sqlExecutePayload struct {
	Branch     string                    `json:"branch"`
	ReadOnly   bool                      `json:"read_only"`
	CommandTag string                    `json:"command_tag"`
	DurationMS int64                     `json:"duration_ms"`
	Truncated  bool                      `json:"truncated"`
	Limits     sqlExecuteLimitsPayload   `json:"limits"`
	Columns    []sqlExecuteColumnPayload `json:"columns,omitempty"`
	Rows       [][]any                   `json:"rows,omitempty"`
	RowCount   int                       `json:"row_count"`
}

type sqlExecuteLimitsPayload struct {
	MaxRows  int `json:"max_rows"`
	MaxBytes int `json:"max_bytes"`
}

type sqlExecuteColumnPayload struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	TypeOID uint32 `json:"type_oid"`
}

type primaryEndpointPayload struct {
	Status         string `json:"status"`
	Ready          bool   `json:"ready"`
	RuntimeState   string `json:"runtime_state,omitempty"`
	RuntimeMessage string `json:"runtime_message,omitempty"`
	Branch         string `json:"branch"`
	Host           string `json:"host"`
	Port           int    `json:"port"`
	Database       string `json:"database"`
	User           string `json:"user"`
	Password       string `json:"password,omitempty"`
	TenantID       string `json:"tenant_id,omitempty"`
	TimelineID     string `json:"timeline_id,omitempty"`
	DSN            string `json:"dsn,omitempty"`
}

type branchEndpointPayload struct {
	Branch            string `json:"branch"`
	Published         bool   `json:"published"`
	Status            string `json:"status"`
	Host              string `json:"host"`
	Port              int    `json:"port"`
	Database          string `json:"database"`
	User              string `json:"user"`
	Password          string `json:"password,omitempty"`
	TenantID          string `json:"tenant_id,omitempty"`
	TimelineID        string `json:"timeline_id,omitempty"`
	ActiveConnections int    `json:"active_connections,omitempty"`
	LastError         string `json:"last_error,omitempty"`
	DSN               string `json:"dsn,omitempty"`
}

var ErrRestoreHistoryUnavailable = errors.New("timestamp is outside source branch history")

var ErrRequestBodyTooLarge = errors.New("request body exceeds limit")

func New(cfg Config) http.Handler {
	version := cfg.Version
	if version == "" {
		version = "dev"
	}

	store := cfg.BranchStore
	if store == nil {
		store = branch.NewStore()
	}

	primaryEndpoint := cfg.PrimaryEndpoint
	if primaryEndpoint == nil {
		primaryEndpoint = newPrimaryEndpointManager()
	}

	attachmentResolver := cfg.BranchAttachmentResolver
	if attachmentResolver == nil {
		attachmentResolver = NewNoopBranchAttachmentResolver()
	}

	branchEndpoints := cfg.BranchEndpoints
	if branchEndpoints == nil {
		branchEndpoints = NewNoopBranchEndpointController(defaultPrimaryEndpointHost, defaultPrimaryEndpointDatabase, defaultPrimaryEndpointUser)
	}

	sqlExecutor := cfg.SQLExecutor
	if sqlExecutor == nil {
		sqlExecutor = NewBranchEndpointSQLQueryExecutor(branchEndpoints)
	}

	logger := cfg.Logger
	logger = loggerOrDefault(logger)

	autoPublishBranches := shouldAutoPublishBranches(branchEndpoints, attachmentResolver)
	if autoPublishBranches {
		autoPublishExistingBranches(store, attachmentResolver, branchEndpoints, logger)
	}

	opStore := operationStore(noopOperationStore{})
	opStoreStatus := "ok"
	opStoreMode := "in_memory"
	opStoreSchemaVersion := 0
	if strings.TrimSpace(cfg.OperationDBPath) != "" {
		sqliteStore, err := newSQLiteOperationStore(cfg.OperationDBPath, cfg.LegacyOperationLogPath, logger)
		if err != nil {
			logger.Warn("initialize sqlite operation store failed; using in-memory operation log", "path", cfg.OperationDBPath, "error", err)
			opStoreStatus = "degraded"
		} else {
			opStore = sqliteStore
			opStoreMode = "sqlite"
			if concrete, ok := sqliteStore.(*sqliteOperationStore); ok {
				opStoreSchemaVersion = concrete.schemaVersion
			}
		}
	}

	branchStoreMode := strings.TrimSpace(cfg.BranchStoreMode)
	if branchStoreMode == "" {
		branchStoreMode = "memory"
	}

	operations := newOperationManager(nil, defaultOperationLogLimit, logger, opStore)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		writeConsoleUI(w, version)
	})

	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		response := statusResponse{
			Status:  "ok",
			Service: "controller",
			Version: version,
			Persistence: persistenceStatusPayload{
				BranchStoreMode:        branchStoreMode,
				OperationStoreMode:     opStoreMode,
				DBPath:                 strings.TrimSpace(cfg.OperationDBPath),
				BranchSchemaVersion:    cfg.BranchSchemaVersion,
				OperationSchemaVersion: opStoreSchemaVersion,
			},
		}
		writeJSON(w, http.StatusOK, response)
	})

	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		primaryStatus := "ok"
		state, err := primaryEndpoint.Connection()
		if err != nil {
			primaryStatus = "error"
		} else if state.Running && !state.Ready {
			primaryStatus = "degraded"
		}

		overallStatus := "ok"
		if primaryStatus != "ok" || opStoreStatus != "ok" {
			overallStatus = "degraded"
		}

		response := healthResponse{
			Status: overallStatus,
			Checks: []healthCheckPayload{
				{Name: "branch_store", Status: "ok"},
				{Name: "operation_manager", Status: "ok"},
				{Name: "operation_store", Status: opStoreStatus},
				{Name: "primary_endpoint", Status: primaryStatus},
			},
		}

		writeJSON(w, http.StatusOK, response)
	})

	mux.HandleFunc("GET /api/v1/branches", func(w http.ResponseWriter, _ *http.Request) {
		branches := store.ListActive()
		payload := make([]branchPayload, 0, len(branches))
		for _, b := range branches {
			payload = append(payload, makeBranchPayload(b))
		}

		writeJSON(w, http.StatusOK, branchesResponse{Branches: payload})
	})

	mux.HandleFunc("GET /api/v1/operations", func(w http.ResponseWriter, r *http.Request) {
		limit := defaultOperationLogLimit
		rawLimit := strings.TrimSpace(r.URL.Query().Get("limit"))
		if rawLimit != "" {
			parsedLimit, err := strconv.Atoi(rawLimit)
			if err != nil || parsedLimit < 1 || parsedLimit > 1000 {
				writeJSONError(w, http.StatusBadRequest, "validation_error", "limit must be between 1 and 1000")
				return
			}
			limit = parsedLimit
		}

		offset := 0
		rawOffset := strings.TrimSpace(r.URL.Query().Get("offset"))
		if rawOffset != "" {
			parsedOffset, err := strconv.Atoi(rawOffset)
			if err != nil || parsedOffset < 0 {
				writeJSONError(w, http.StatusBadRequest, "validation_error", "offset must be zero or greater")
				return
			}
			offset = parsedOffset
		}

		filter := operationQueryFilter{
			Limit:  limit,
			Offset: offset,
			Status: strings.TrimSpace(r.URL.Query().Get("status")),
			Type:   strings.TrimSpace(r.URL.Query().Get("type")),
		}
		entries := operations.ListFiltered(filter)
		payload := make([]operationPayload, 0, len(entries))
		for _, entry := range entries {
			payload = append(payload, makeOperationPayload(entry))
		}

		writeJSON(w, http.StatusOK, operationsResponse{Operations: payload})
	})

	mux.HandleFunc("GET /api/v1/endpoints/primary/connection", func(w http.ResponseWriter, _ *http.Request) {
		state, err := primaryEndpoint.Connection()
		if err != nil {
			if isPrimaryEndpointUnavailable(err) {
				writeJSONError(w, http.StatusServiceUnavailable, "endpoint_unavailable", err.Error())
				return
			}

			writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}

		writeJSON(w, http.StatusOK, primaryEndpointConnectionResponse{Connection: makePrimaryConnectionPayload(state)})
	})

	mux.HandleFunc("GET /api/v1/endpoints", func(w http.ResponseWriter, _ *http.Request) {
		states, err := branchEndpoints.List()
		if err != nil {
			if isPrimaryEndpointUnavailable(err) {
				writeJSONError(w, http.StatusServiceUnavailable, "endpoint_unavailable", err.Error())
				return
			}

			writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}

		payload := make([]branchEndpointPayload, 0, len(states))
		for _, state := range states {
			payload = append(payload, makeBranchEndpointPayload(state))
		}

		writeJSON(w, http.StatusOK, branchEndpointsEnvelope{Endpoints: payload})
	})

	mux.HandleFunc("POST /api/v1/branches/{name}/publish", func(w http.ResponseWriter, r *http.Request) {
		branchName := strings.TrimSpace(r.PathValue("name"))
		if branchName == "" {
			writeJSONError(w, http.StatusBadRequest, "validation_error", "branch name is required")
			return
		}

		var state branchEndpointState
		err := operations.Run("publish_branch_endpoint", func() error {
			if _, getErr := store.GetActive(branchName); getErr != nil {
				return getErr
			}

			attachment, resolveErr := attachmentResolver.Resolve(branchName)
			if resolveErr != nil {
				return resolveErr
			}

			if _, setErr := store.SetAttachment(branchName, attachment.TenantID, attachment.TimelineID); setErr != nil {
				return setErr
			}

			securedBranch, passwordErr := ensureBranchPassword(store, branchName)
			if passwordErr != nil {
				return passwordErr
			}

			var publishErr error
			state, publishErr = branchEndpoints.Publish(branchName, attachment, securedBranch.Password)
			return publishErr
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrOperationInProgress):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
			case errors.Is(err, branch.ErrNotFound):
				writeJSONError(w, http.StatusNotFound, "not_found", err.Error())
			case isPrimaryEndpointUnavailable(err):
				writeJSONError(w, http.StatusServiceUnavailable, "endpoint_unavailable", err.Error())
			case errors.Is(err, branch.ErrNoSpace):
				writeJSONError(w, http.StatusInsufficientStorage, "storage_error", err.Error())
			case errors.Is(err, branch.ErrPersistFailed):
				writeJSONError(w, http.StatusServiceUnavailable, "storage_error", err.Error())
			default:
				writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusOK, branchEndpointConnectionEnvelope{Connection: makeBranchEndpointPayload(state)})
	})

	mux.HandleFunc("POST /api/v1/branches/{name}/unpublish", func(w http.ResponseWriter, r *http.Request) {
		branchName := strings.TrimSpace(r.PathValue("name"))
		if branchName == "" {
			writeJSONError(w, http.StatusBadRequest, "validation_error", "branch name is required")
			return
		}

		var state branchEndpointState
		err := operations.Run("unpublish_branch_endpoint", func() error {
			if _, getErr := store.GetActive(branchName); getErr != nil {
				return getErr
			}

			var unpublishErr error
			state, unpublishErr = branchEndpoints.Unpublish(branchName)
			return unpublishErr
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrOperationInProgress):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
			case errors.Is(err, branch.ErrNotFound):
				writeJSONError(w, http.StatusNotFound, "not_found", err.Error())
			case isPrimaryEndpointUnavailable(err):
				writeJSONError(w, http.StatusServiceUnavailable, "endpoint_unavailable", err.Error())
			case errors.Is(err, branch.ErrNoSpace):
				writeJSONError(w, http.StatusInsufficientStorage, "storage_error", err.Error())
			case errors.Is(err, branch.ErrPersistFailed):
				writeJSONError(w, http.StatusServiceUnavailable, "storage_error", err.Error())
			default:
				writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusOK, branchEndpointConnectionEnvelope{Connection: makeBranchEndpointPayload(state)})
	})

	mux.HandleFunc("GET /api/v1/branches/{name}/connection", func(w http.ResponseWriter, r *http.Request) {
		branchName := strings.TrimSpace(r.PathValue("name"))
		if branchName == "" {
			writeJSONError(w, http.StatusBadRequest, "validation_error", "branch name is required")
			return
		}

		if _, err := store.GetActive(branchName); err != nil {
			if errors.Is(err, branch.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "not_found", err.Error())
				return
			}

			writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}

		state, err := branchEndpoints.Connection(branchName)
		if err != nil {
			switch {
			case errors.Is(err, branch.ErrNotFound):
				writeJSONError(w, http.StatusNotFound, "not_found", err.Error())
			case isPrimaryEndpointUnavailable(err):
				writeJSONError(w, http.StatusServiceUnavailable, "endpoint_unavailable", err.Error())
			default:
				writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusOK, branchEndpointConnectionEnvelope{Connection: makeBranchEndpointPayload(state)})
	})

	mux.HandleFunc("POST /api/v1/branches/{name}/sql/execute", func(w http.ResponseWriter, r *http.Request) {
		branchName := strings.TrimSpace(r.PathValue("name"))
		if branchName == "" {
			writeJSONError(w, http.StatusBadRequest, "validation_error", "branch name is required")
			return
		}

		if _, err := store.GetActive(branchName); err != nil {
			if errors.Is(err, branch.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "not_found", err.Error())
				return
			}

			writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}

		var req sqlExecuteRequest
		if err := decodeJSONRequest(r, &req, sqlJSONRequestBodyMaxBytes); err != nil {
			writeJSONDecodeError(w, err, sqlJSONRequestBodyMaxBytes)
			return
		}

		if err := validateSingleStatementQuery(req.SQL); err != nil {
			writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}

		result, err := sqlExecutor.Execute(r.Context(), branchName, req.SQL, !req.AllowWrites)
		if err != nil {
			logger.Warn("sql execution failed", "branch", branchName, "read_only", !req.AllowWrites, "error", err)
			switch {
			case errors.Is(err, branch.ErrNotFound):
				writeJSONError(w, http.StatusNotFound, "not_found", err.Error())
			case isPrimaryEndpointUnavailable(err):
				writeJSONError(w, http.StatusServiceUnavailable, "endpoint_unavailable", err.Error())
			case errors.Is(err, context.DeadlineExceeded):
				writeJSONError(w, http.StatusRequestTimeout, "timeout", "query timed out")
			case errors.Is(err, context.Canceled):
				writeJSONError(w, http.StatusRequestTimeout, "canceled", "query canceled")
			default:
				var sqlErr *sqlExecutionError
				if errors.As(err, &sqlErr) {
					writeJSONError(w, http.StatusUnprocessableEntity, "sql_error", sqlErr.Error())
					return
				}

				writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
			return
		}

		logger.Info("sql execution succeeded", "branch", branchName, "read_only", result.ReadOnly, "command_tag", result.CommandTag, "duration_ms", result.DurationMS, "row_count", result.RowCount, "truncated", result.Truncated)

		writeJSON(w, http.StatusOK, sqlExecuteEnvelope{Result: makeSQLExecutePayload(result)})
	})

	mux.HandleFunc("POST /api/v1/endpoints/primary/start", func(w http.ResponseWriter, _ *http.Request) {
		var state primaryEndpointState
		err := operations.Run("start_primary_endpoint", func() error {
			current, currentErr := primaryEndpoint.Connection()
			if currentErr != nil {
				return currentErr
			}

			securedBranch, passwordErr := ensureBranchPassword(store, current.Branch)
			if passwordErr != nil {
				return passwordErr
			}

			if setPasswordErr := primaryEndpoint.SetBranchPassword(current.Branch, securedBranch.Password); setPasswordErr != nil {
				return setPasswordErr
			}

			attachment, resolveErr := attachmentResolver.Resolve(current.Branch)
			if resolveErr != nil {
				return resolveErr
			}

			if strings.TrimSpace(attachment.TenantID) != "" && strings.TrimSpace(attachment.TimelineID) != "" {
				if attachErr := primaryEndpoint.SetBranchAttachment(current.Branch, attachment.TenantID, attachment.TimelineID); attachErr != nil {
					return attachErr
				}
			}

			var startErr error
			state, startErr = primaryEndpoint.Start()
			return startErr
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrOperationInProgress):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
			case isPrimaryEndpointUnavailable(err):
				writeJSONError(w, http.StatusServiceUnavailable, "endpoint_unavailable", err.Error())
			case errors.Is(err, branch.ErrNoSpace):
				writeJSONError(w, http.StatusInsufficientStorage, "storage_error", err.Error())
			case errors.Is(err, branch.ErrPersistFailed):
				writeJSONError(w, http.StatusServiceUnavailable, "storage_error", err.Error())
			default:
				writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusOK, primaryEndpointConnectionResponse{Connection: makePrimaryConnectionPayload(state)})
	})

	mux.HandleFunc("POST /api/v1/endpoints/primary/stop", func(w http.ResponseWriter, _ *http.Request) {
		var state primaryEndpointState
		err := operations.Run("stop_primary_endpoint", func() error {
			var stopErr error
			state, stopErr = primaryEndpoint.Stop()
			return stopErr
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrOperationInProgress):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
			case isPrimaryEndpointUnavailable(err):
				writeJSONError(w, http.StatusServiceUnavailable, "endpoint_unavailable", err.Error())
			default:
				writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusOK, primaryEndpointConnectionResponse{Connection: makePrimaryConnectionPayload(state)})
	})

	mux.HandleFunc("POST /api/v1/endpoints/primary/switch", func(w http.ResponseWriter, r *http.Request) {
		var req switchPrimaryEndpointRequest
		if err := decodeJSONRequest(r, &req, defaultJSONRequestBodyMaxBytes); err != nil {
			writeJSONDecodeError(w, err, defaultJSONRequestBodyMaxBytes)
			return
		}

		targetBranch := strings.TrimSpace(req.Branch)
		if targetBranch == "" {
			writeJSONError(w, http.StatusBadRequest, "validation_error", "branch name is required")
			return
		}

		var state primaryEndpointState
		err := operations.Run("switch_primary_endpoint", func() error {
			if _, getErr := store.GetActive(targetBranch); getErr != nil {
				return branch.ErrParentMissing
			}

			securedBranch, passwordErr := ensureBranchPassword(store, targetBranch)
			if passwordErr != nil {
				return passwordErr
			}

			if setPasswordErr := primaryEndpoint.SetBranchPassword(targetBranch, securedBranch.Password); setPasswordErr != nil {
				return setPasswordErr
			}

			attachment, resolveErr := attachmentResolver.Resolve(targetBranch)
			if resolveErr != nil {
				return resolveErr
			}

			if strings.TrimSpace(attachment.TenantID) != "" && strings.TrimSpace(attachment.TimelineID) != "" {
				if attachErr := primaryEndpoint.SetBranchAttachment(targetBranch, attachment.TenantID, attachment.TimelineID); attachErr != nil {
					return attachErr
				}
			}

			var switchErr error
			state, switchErr = primaryEndpoint.SwitchToBranch(targetBranch)
			return switchErr
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrOperationInProgress):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
			case errors.Is(err, branch.ErrParentMissing):
				writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			case isPrimaryEndpointUnavailable(err):
				writeJSONError(w, http.StatusServiceUnavailable, "endpoint_unavailable", err.Error())
			case errors.Is(err, branch.ErrNoSpace):
				writeJSONError(w, http.StatusInsufficientStorage, "storage_error", err.Error())
			case errors.Is(err, branch.ErrPersistFailed):
				writeJSONError(w, http.StatusServiceUnavailable, "storage_error", err.Error())
			default:
				writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusOK, primaryEndpointConnectionResponse{Connection: makePrimaryConnectionPayload(state)})
	})

	mux.HandleFunc("POST /api/v1/restore", func(w http.ResponseWriter, r *http.Request) {
		var req restoreRequest
		if err := decodeJSONRequest(r, &req, defaultJSONRequestBodyMaxBytes); err != nil {
			writeJSONDecodeError(w, err, defaultJSONRequestBodyMaxBytes)
			return
		}

		sourceBranch, restoreAt, restoreName, err := normalizeRestoreRequest(req)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}

		var restored branch.Branch
		var resolvedLSN string
		err = operations.Run("restore_branch", func() error {
			source, sourceErr := store.GetActive(sourceBranch)
			if sourceErr != nil {
				return branch.ErrParentMissing
			}

			if restoreAt.Before(source.CreatedAt.UTC()) {
				return ErrRestoreHistoryUnavailable
			}

			attachment, resolved, resolveErr := attachmentResolver.ResolveRestore(source.Name, restoreName, restoreAt)
			if resolveErr != nil {
				return resolveErr
			}

			resolvedLSN = strings.TrimSpace(resolved)
			tenantID := strings.TrimSpace(attachment.TenantID)
			timelineID := strings.TrimSpace(attachment.TimelineID)
			if resolvedLSN == "" || tenantID == "" || timelineID == "" {
				return fmt.Errorf("%w: restore resolver returned incomplete attachment", ErrPrimaryEndpointUnavailable)
			}

			password, passwordErr := generateBranchPassword()
			if passwordErr != nil {
				return fmt.Errorf("%w: %v", ErrPrimaryEndpointUnavailable, passwordErr)
			}

			var createErr error
			restored, createErr = store.CreateWithAttachmentAndPassword(restoreName, source.Name, tenantID, timelineID, password)
			if createErr != nil {
				return createErr
			}

			if !autoPublishBranches {
				return nil
			}

			if publishErr := ensureBranchPublished(store, attachmentResolver, branchEndpoints, restored.Name); publishErr != nil {
				if _, rollbackErr := store.SoftDelete(restored.Name); rollbackErr != nil {
					return fmt.Errorf("%w: auto-publish restored branch %q: %v (rollback failed: %v)", ErrPrimaryEndpointUnavailable, restored.Name, publishErr, rollbackErr)
				}

				return publishErr
			}

			return nil
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrOperationInProgress):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
			case errors.Is(err, ErrRestoreHistoryUnavailable):
				writeJSONError(w, http.StatusUnprocessableEntity, "history_unavailable", err.Error())
			case errors.Is(err, branch.ErrInvalidName), errors.Is(err, branch.ErrParentMissing):
				writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			case errors.Is(err, branch.ErrAlreadyExists):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
			case isPrimaryEndpointUnavailable(err):
				writeJSONError(w, http.StatusServiceUnavailable, "restore_unavailable", err.Error())
			case errors.Is(err, branch.ErrNoSpace):
				writeJSONError(w, http.StatusInsufficientStorage, "storage_error", err.Error())
			case errors.Is(err, branch.ErrPersistFailed):
				writeJSONError(w, http.StatusServiceUnavailable, "storage_error", err.Error())
			default:
				writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusCreated, restoreResponse{Restore: makeRestorePayload(restored, restoreAt, resolvedLSN)})
	})

	mux.HandleFunc("POST /api/v1/branches", func(w http.ResponseWriter, r *http.Request) {
		var req createBranchRequest
		if err := decodeJSONRequest(r, &req, defaultJSONRequestBodyMaxBytes); err != nil {
			writeJSONDecodeError(w, err, defaultJSONRequestBodyMaxBytes)
			return
		}

		var created branch.Branch
		err := operations.Run("create_branch", func() error {
			password, passwordErr := generateBranchPassword()
			if passwordErr != nil {
				return fmt.Errorf("%w: %v", ErrPrimaryEndpointUnavailable, passwordErr)
			}

			var createErr error
			created, createErr = store.CreateWithPassword(req.Name, req.Parent, password)
			if createErr != nil {
				return createErr
			}

			if !autoPublishBranches {
				return nil
			}

			if publishErr := ensureBranchPublished(store, attachmentResolver, branchEndpoints, created.Name); publishErr != nil {
				if _, rollbackErr := store.SoftDelete(created.Name); rollbackErr != nil {
					return fmt.Errorf("%w: auto-publish branch %q: %v (rollback failed: %v)", ErrPrimaryEndpointUnavailable, created.Name, publishErr, rollbackErr)
				}

				return publishErr
			}

			return nil
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrOperationInProgress):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
			case errors.Is(err, branch.ErrInvalidName), errors.Is(err, branch.ErrParentMissing):
				writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			case errors.Is(err, branch.ErrAlreadyExists):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
			case isPrimaryEndpointUnavailable(err):
				writeJSONError(w, http.StatusServiceUnavailable, "endpoint_unavailable", err.Error())
			case errors.Is(err, branch.ErrNoSpace):
				writeJSONError(w, http.StatusInsufficientStorage, "storage_error", err.Error())
			case errors.Is(err, branch.ErrPersistFailed):
				writeJSONError(w, http.StatusServiceUnavailable, "storage_error", err.Error())
			default:
				writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusCreated, branchResponse{Branch: makeBranchPayload(created)})
	})

	mux.HandleFunc("POST /api/v1/branches/{name}/reset", func(w http.ResponseWriter, r *http.Request) {
		branchName := strings.TrimSpace(r.PathValue("name"))
		if branchName == "" {
			writeJSONError(w, http.StatusBadRequest, "validation_error", "branch name is required")
			return
		}
		if branchName == "main" {
			writeJSONError(w, http.StatusBadRequest, "validation_error", "branch is protected")
			return
		}

		var updated branch.Branch
		err := operations.Run("reset_branch", func() error {
			target, targetErr := store.GetActive(branchName)
			if targetErr != nil {
				return targetErr
			}
			if strings.TrimSpace(target.Parent) == "" {
				return branch.ErrProtected
			}

			securedBranch, passwordErr := ensureBranchPassword(store, branchName)
			if passwordErr != nil {
				return passwordErr
			}

			if setPasswordErr := primaryEndpoint.SetBranchPassword(branchName, securedBranch.Password); setPasswordErr != nil {
				return setPasswordErr
			}

			attachment, resolveErr := attachmentResolver.ResolveReset(branchName)
			if resolveErr != nil {
				return resolveErr
			}

			var setErr error
			updated, setErr = store.SetAttachment(branchName, attachment.TenantID, attachment.TimelineID)
			if setErr != nil {
				return setErr
			}

			if attachErr := primaryEndpoint.SetBranchAttachment(branchName, attachment.TenantID, attachment.TimelineID); attachErr != nil {
				return attachErr
			}

			if refreshErr := branchEndpoints.Refresh(branchName, attachment, securedBranch.Password); refreshErr != nil {
				return refreshErr
			}

			connectionState, connErr := primaryEndpoint.Connection()
			if connErr != nil {
				return connErr
			}

			if connectionState.Branch != branchName {
				return nil
			}

			_, switchErr := primaryEndpoint.SwitchToBranch(branchName)
			return switchErr
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrOperationInProgress):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
			case errors.Is(err, branch.ErrProtected), errors.Is(err, branch.ErrInvalidName), errors.Is(err, branch.ErrParentMissing):
				writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			case errors.Is(err, branch.ErrNotFound):
				writeJSONError(w, http.StatusNotFound, "not_found", err.Error())
			case isPrimaryEndpointUnavailable(err):
				writeJSONError(w, http.StatusServiceUnavailable, "endpoint_unavailable", err.Error())
			case errors.Is(err, branch.ErrNoSpace):
				writeJSONError(w, http.StatusInsufficientStorage, "storage_error", err.Error())
			case errors.Is(err, branch.ErrPersistFailed):
				writeJSONError(w, http.StatusServiceUnavailable, "storage_error", err.Error())
			default:
				writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusOK, branchResponse{Branch: makeBranchPayload(updated)})
	})

	mux.HandleFunc("DELETE /api/v1/branches/{name}", func(w http.ResponseWriter, r *http.Request) {
		branchName := r.PathValue("name")
		var deleted branch.Branch
		err := operations.Run("delete_branch", func() error {
			if _, getErr := store.GetActive(branchName); getErr != nil {
				return getErr
			}

			if _, unpublishErr := branchEndpoints.Unpublish(branchName); unpublishErr != nil {
				return unpublishErr
			}

			var deleteErr error
			deleted, deleteErr = store.SoftDelete(branchName)
			return deleteErr
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrOperationInProgress):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
			case errors.Is(err, branch.ErrProtected):
				writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			case errors.Is(err, branch.ErrNotFound):
				writeJSONError(w, http.StatusNotFound, "not_found", err.Error())
			case isPrimaryEndpointUnavailable(err):
				writeJSONError(w, http.StatusServiceUnavailable, "endpoint_unavailable", err.Error())
			case errors.Is(err, branch.ErrNoSpace):
				writeJSONError(w, http.StatusInsufficientStorage, "storage_error", err.Error())
			case errors.Is(err, branch.ErrPersistFailed):
				writeJSONError(w, http.StatusServiceUnavailable, "storage_error", err.Error())
			default:
				writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusOK, branchResponse{Branch: makeBranchPayload(deleted)})
	})

	var handler http.Handler = mux
	if cfg.BasicAuthUser != "" && cfg.BasicAuthPassword != "" {
		handler = withBasicAuth(handler, cfg.BasicAuthUser, cfg.BasicAuthPassword)
	}

	if closer, ok := opStore.(io.Closer); ok {
		return closeableHandler{Handler: handler, closer: closer}
	}

	return handler
}

func withBasicAuth(next http.Handler, expectedUser string, expectedPassword string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providedUser, providedPassword, ok := r.BasicAuth()
		if !ok || !secureEqual(providedUser, expectedUser) || !secureEqual(providedPassword, expectedPassword) {
			w.Header().Set("WWW-Authenticate", `Basic realm="neon-selfhost"`)
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func secureEqual(left string, right string) bool {
	if len(left) != len(right) {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func decodeJSONRequest(r *http.Request, out any, maxBytes int64) error {
	if maxBytes <= 0 {
		maxBytes = defaultJSONRequestBodyMaxBytes
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
	if err != nil {
		return err
	}

	if int64(len(body)) > maxBytes {
		return ErrRequestBodyTooLarge
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}

	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain a single JSON object")
	}

	return nil
}

func writeJSONDecodeError(w http.ResponseWriter, err error, maxBytes int64) {
	if errors.Is(err, ErrRequestBodyTooLarge) {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "request_too_large", fmt.Sprintf("request body exceeds %d bytes", maxBytes))
		return
	}

	writeJSONError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
}

func makeBranchPayload(b branch.Branch) branchPayload {
	payload := branchPayload{
		Name:      b.Name,
		Parent:    b.Parent,
		CreatedAt: b.CreatedAt.UTC().Format(time.RFC3339),
		Deleted:   b.Deleted,
	}

	if b.DeletedAt != nil {
		deletedAt := b.DeletedAt.UTC().Format(time.RFC3339)
		payload.DeletedAt = &deletedAt
	}

	return payload
}

func makeOperationPayload(op operationEntry) operationPayload {
	payload := operationPayload{
		Type:      op.Type,
		Status:    op.Status,
		Message:   op.Message,
		StartedAt: op.StartedAt.UTC().Format(time.RFC3339),
	}

	if op.FinishedAt != nil {
		finishedAt := op.FinishedAt.UTC().Format(time.RFC3339)
		payload.FinishedAt = &finishedAt
	}

	return payload
}

func makeRestorePayload(restored branch.Branch, requestedAt time.Time, resolvedLSN string) restorePayload {
	return restorePayload{
		Branch:      makeBranchPayload(restored),
		RequestedAt: requestedAt.UTC().Format(time.RFC3339),
		ResolvedLSN: resolvedLSN,
	}
}

func makeBranchEndpointPayload(state branchEndpointState) branchEndpointPayload {
	payload := branchEndpointPayload{
		Branch:            state.Branch,
		Published:         state.Published,
		Status:            state.Status,
		Host:              state.Host,
		Port:              state.Port,
		Database:          state.Database,
		User:              state.User,
		Password:          state.Password,
		TenantID:          state.TenantID,
		TimelineID:        state.TimelineID,
		ActiveConnections: state.ActiveConnections,
		LastError:         state.LastError,
	}

	ready := payload.Published && (payload.Status == "running" || payload.Status == "active")
	if ready && payload.Host != "" && payload.Port > 0 && payload.Database != "" && payload.User != "" {
		payload.DSN = (&url.URL{
			Scheme:   "postgres",
			User:     url.UserPassword(payload.User, payload.Password),
			Host:     fmt.Sprintf("%s:%d", payload.Host, payload.Port),
			Path:     "/" + url.PathEscape(payload.Database),
			RawQuery: "sslmode=disable",
		}).String()
	}

	return payload
}

func makeSQLExecutePayload(result sqlExecutionResult) sqlExecutePayload {
	columns := make([]sqlExecuteColumnPayload, 0, len(result.Columns))
	for _, column := range result.Columns {
		columns = append(columns, sqlExecuteColumnPayload{
			Name:    column.Name,
			Type:    column.Type,
			TypeOID: column.TypeOID,
		})
	}

	rows := make([][]any, len(result.Rows))
	copy(rows, result.Rows)

	return sqlExecutePayload{
		Branch:     result.Branch,
		ReadOnly:   result.ReadOnly,
		CommandTag: result.CommandTag,
		DurationMS: result.DurationMS,
		Truncated:  result.Truncated,
		Limits: sqlExecuteLimitsPayload{
			MaxRows:  result.MaxRows,
			MaxBytes: result.MaxBytes,
		},
		Columns:  columns,
		Rows:     rows,
		RowCount: result.RowCount,
	}
}

func shouldAutoPublishBranches(branchEndpoints BranchEndpointController, attachmentResolver BranchAttachmentResolver) bool {
	if branchEndpoints == nil || attachmentResolver == nil {
		return false
	}

	switch branchEndpoints.(type) {
	case noopBranchEndpointController, *noopBranchEndpointController:
		return false
	}

	switch attachmentResolver.(type) {
	case noopBranchAttachmentResolver, *noopBranchAttachmentResolver:
		return false
	}

	return true
}

func autoPublishExistingBranches(store *branch.Store, attachmentResolver BranchAttachmentResolver, branchEndpoints BranchEndpointController, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}

	for _, active := range store.ListActive() {
		if err := ensureBranchPublished(store, attachmentResolver, branchEndpoints, active.Name); err != nil {
			logger.Warn("auto publish branch failed", "branch", active.Name, "error", err)
			continue
		}

		logger.Info("auto published branch endpoint", "branch", active.Name)
	}
}

func ensureBranchPublished(store *branch.Store, attachmentResolver BranchAttachmentResolver, branchEndpoints BranchEndpointController, branchName string) error {
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return branch.ErrNotFound
	}

	securedBranch, err := ensureBranchPassword(store, branchName)
	if err != nil {
		return err
	}

	attachment, err := resolveAttachmentForAutoPublish(attachmentResolver, branchName)
	if err != nil {
		return err
	}

	tenantID := strings.TrimSpace(attachment.TenantID)
	timelineID := strings.TrimSpace(attachment.TimelineID)
	if tenantID == "" || timelineID == "" {
		return fmt.Errorf("%w: attachment resolver returned incomplete attachment for %q", ErrPrimaryEndpointUnavailable, branchName)
	}

	if _, err := store.SetAttachment(branchName, tenantID, timelineID); err != nil {
		return err
	}

	_, err = branchEndpoints.Publish(branchName, BranchAttachment{TenantID: tenantID, TimelineID: timelineID}, securedBranch.Password)
	return err
}

func resolveAttachmentForAutoPublish(attachmentResolver BranchAttachmentResolver, branchName string) (BranchAttachment, error) {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	var lastErr error
	for attempt := 0; attempt < autoPublishResolveMaxAttempts; attempt++ {
		attachment, err := attachmentResolver.Resolve(branchName)
		if err == nil {
			return attachment, nil
		}

		if !errors.Is(err, branch.ErrNotFound) {
			return BranchAttachment{}, err
		}

		lastErr = err
		if attempt == autoPublishResolveMaxAttempts-1 {
			break
		}

		time.Sleep(autoPublishResolveDelay(attempt, rng))
	}

	if lastErr == nil {
		lastErr = branch.ErrNotFound
	}

	return BranchAttachment{}, lastErr
}

func autoPublishResolveDelay(attempt int, rng *rand.Rand) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	delay := autoPublishResolveBaseDelay << attempt
	if delay > autoPublishResolveMaxDelay {
		delay = autoPublishResolveMaxDelay
	}

	if rng == nil {
		return delay
	}

	minDelay := float64(delay) * 0.75
	maxDelay := float64(delay) * 1.25
	jittered := minDelay + rng.Float64()*(maxDelay-minDelay)
	return time.Duration(jittered)
}

func normalizeRestoreRequest(req restoreRequest) (string, time.Time, string, error) {
	rawTimestamp := strings.TrimSpace(req.Timestamp)
	if rawTimestamp == "" {
		return "", time.Time{}, "", errors.New("timestamp is required")
	}

	restoreAt, err := time.Parse(time.RFC3339, rawTimestamp)
	if err != nil {
		return "", time.Time{}, "", errors.New("timestamp must be a valid RFC3339 value")
	}

	restoreAt = restoreAt.UTC()
	if restoreAt.After(time.Now().UTC()) {
		return "", time.Time{}, "", errors.New("timestamp must not be in the future")
	}

	sourceBranch := strings.TrimSpace(req.SourceBranch)
	if sourceBranch == "" {
		sourceBranch = "main"
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = defaultRestoreBranchName(restoreAt)
	}

	return sourceBranch, restoreAt, name, nil
}

func defaultRestoreBranchName(restoreAt time.Time) string {
	return "restore-" + restoreAt.UTC().Format("20060102-150405")
}

func mockResolvedLSN(restoreAt time.Time) string {
	utc := restoreAt.UTC()
	return fmt.Sprintf("%X/%X", utc.Unix(), utc.Nanosecond())
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	writeJSONBytes(w, status, body)
}

func writeJSONError(w http.ResponseWriter, status int, code string, message string) {
	payload := apiErrorResponse{Error: apiError{Code: code, Message: message}}
	body, err := json.Marshal(payload)
	if err != nil {
		writeJSONBytes(w, http.StatusInternalServerError, []byte(`{"error":{"code":"internal_error","message":"internal server error"}}`))
		return
	}

	writeJSONBytes(w, status, body)
}

func writeJSONBytes(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
	_, _ = w.Write([]byte("\n"))
}
