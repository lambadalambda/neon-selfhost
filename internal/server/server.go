package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"neon-selfhost/internal/branch"
)

type Config struct {
	Version     string
	BranchStore *branch.Store
}

type statusResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version"`
}

type branchResponse struct {
	Branch branchPayload `json:"branch"`
}

type branchesResponse struct {
	Branches []branchPayload `json:"branches"`
}

type createBranchRequest struct {
	Name   string `json:"name"`
	Parent string `json:"parent"`
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

func New(cfg Config) http.Handler {
	version := cfg.Version
	if version == "" {
		version = "dev"
	}

	store := cfg.BranchStore
	if store == nil {
		store = branch.NewStore()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/status", func(w http.ResponseWriter, _ *http.Request) {
		response := statusResponse{
			Status:  "ok",
			Service: "controller",
			Version: version,
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

	mux.HandleFunc("POST /api/v1/branches", func(w http.ResponseWriter, r *http.Request) {
		var req createBranchRequest
		if err := decodeJSONRequest(r, &req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
			return
		}

		created, err := store.Create(req.Name, req.Parent)
		if err != nil {
			switch {
			case errors.Is(err, branch.ErrInvalidName), errors.Is(err, branch.ErrParentMissing):
				writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			case errors.Is(err, branch.ErrAlreadyExists):
				writeJSONError(w, http.StatusConflict, "conflict", err.Error())
			default:
				writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusCreated, branchResponse{Branch: makeBranchPayload(created)})
	})

	mux.HandleFunc("DELETE /api/v1/branches/{name}", func(w http.ResponseWriter, r *http.Request) {
		branchName := r.PathValue("name")
		deleted, err := store.SoftDelete(branchName)
		if err != nil {
			switch {
			case errors.Is(err, branch.ErrProtected):
				writeJSONError(w, http.StatusBadRequest, "validation_error", err.Error())
			case errors.Is(err, branch.ErrNotFound):
				writeJSONError(w, http.StatusNotFound, "not_found", err.Error())
			default:
				writeJSONError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
			return
		}

		writeJSON(w, http.StatusOK, branchResponse{Branch: makeBranchPayload(deleted)})
	})

	return mux
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
