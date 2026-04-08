package server

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"neon-selfhost/internal/branch"
)

type Config struct {
	Version     string
	BranchStore *branch.Store

	BasicAuthUser     string
	BasicAuthPassword string
}

type statusResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
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

type primaryEndpointPayload struct {
	Status   string `json:"status"`
	Branch   string `json:"branch"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	User     string `json:"user"`
	DSN      string `json:"dsn,omitempty"`
}

var errRestoreHistoryUnavailable = errors.New("timestamp is outside source branch history")

func New(cfg Config) http.Handler {
	version := cfg.Version
	if version == "" {
		version = "dev"
	}

	store := cfg.BranchStore
	if store == nil {
		store = branch.NewStore()
	}

	operations := newOperationManager(nil, defaultOperationLogLimit)
	primaryEndpoint := newPrimaryEndpointManager()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		response := statusResponse{
			Status:  "ok",
			Service: "controller",
			Version: version,
		}
		writeJSON(w, http.StatusOK, response)
	})

	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		response := healthResponse{
			Status: "ok",
			Checks: []healthCheckPayload{
				{Name: "branch_store", Status: "ok"},
				{Name: "operation_manager", Status: "ok"},
				{Name: "primary_endpoint", Status: "ok"},
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

	mux.HandleFunc("GET /api/v1/operations", func(w http.ResponseWriter, _ *http.Request) {
		entries := operations.List(defaultOperationLogLimit)
		payload := make([]operationPayload, 0, len(entries))
		for _, entry := range entries {
			payload = append(payload, makeOperationPayload(entry))
		}

		writeJSON(w, http.StatusOK, operationsResponse{Operations: payload})
	})

	mux.HandleFunc("GET /api/v1/endpoints/primary/connection", func(w http.ResponseWriter, _ *http.Request) {
		state := primaryEndpoint.Connection()
		writeJSON(w, http.StatusOK, primaryEndpointConnectionResponse{Connection: makePrimaryConnectionPayload(state)})
	})

	mux.HandleFunc("POST /api/v1/endpoints/primary/start", func(w http.ResponseWriter, _ *http.Request) {
		var state primaryEndpointState
		err := operations.Run("start_primary_endpoint", func() error {
			state = primaryEndpoint.Start()
			return nil
		})
		if err != nil {
			if errors.Is(err, ErrOperationInProgress) {
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
				return
			}

			writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}

		writeJSON(w, http.StatusOK, primaryEndpointConnectionResponse{Connection: makePrimaryConnectionPayload(state)})
	})

	mux.HandleFunc("POST /api/v1/endpoints/primary/stop", func(w http.ResponseWriter, _ *http.Request) {
		var state primaryEndpointState
		err := operations.Run("stop_primary_endpoint", func() error {
			state = primaryEndpoint.Stop()
			return nil
		})
		if err != nil {
			if errors.Is(err, ErrOperationInProgress) {
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
				return
			}

			writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			return
		}

		writeJSON(w, http.StatusOK, primaryEndpointConnectionResponse{Connection: makePrimaryConnectionPayload(state)})
	})

	mux.HandleFunc("POST /api/v1/endpoints/primary/switch", func(w http.ResponseWriter, r *http.Request) {
		var req switchPrimaryEndpointRequest
		if err := decodeJSONRequest(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
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

			state = primaryEndpoint.SwitchToBranch(targetBranch)
			return nil
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrOperationInProgress):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
			case errors.Is(err, branch.ErrParentMissing):
				writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			default:
				writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusOK, primaryEndpointConnectionResponse{Connection: makePrimaryConnectionPayload(state)})
	})

	mux.HandleFunc("POST /api/v1/restore", func(w http.ResponseWriter, r *http.Request) {
		var req restoreRequest
		if err := decodeJSONRequest(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
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
				return errRestoreHistoryUnavailable
			}

			resolvedLSN = mockResolvedLSN(restoreAt)

			var createErr error
			restored, createErr = store.Create(restoreName, source.Name)
			return createErr
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrOperationInProgress):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
			case errors.Is(err, errRestoreHistoryUnavailable):
				writeJSONError(w, http.StatusUnprocessableEntity, "history_unavailable", err.Error())
			case errors.Is(err, branch.ErrInvalidName), errors.Is(err, branch.ErrParentMissing):
				writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			case errors.Is(err, branch.ErrAlreadyExists):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
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
		if err := decodeJSONRequest(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
			return
		}

		var created branch.Branch
		err := operations.Run("create_branch", func() error {
			var createErr error
			created, createErr = store.Create(req.Name, req.Parent)
			return createErr
		})
		if err != nil {
			switch {
			case errors.Is(err, ErrOperationInProgress):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
			case errors.Is(err, branch.ErrInvalidName), errors.Is(err, branch.ErrParentMissing):
				writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			case errors.Is(err, branch.ErrAlreadyExists):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
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

	mux.HandleFunc("DELETE /api/v1/branches/{name}", func(w http.ResponseWriter, r *http.Request) {
		branchName := r.PathValue("name")
		var deleted branch.Branch
		err := operations.Run("delete_branch", func() error {
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

func decodeJSONRequest(r *http.Request, out any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}

	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain a single JSON object")
	}

	return nil
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
