package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neon-selfhost/internal/branch"
)

type branchesListResponse struct {
	Branches []struct {
		Name      string  `json:"name"`
		Parent    string  `json:"parent"`
		Deleted   bool    `json:"deleted"`
		DeletedAt *string `json:"deleted_at,omitempty"`
	} `json:"branches"`
}

type testBranchResponse struct {
	Branch struct {
		Name      string  `json:"name"`
		Parent    string  `json:"parent"`
		Deleted   bool    `json:"deleted"`
		DeletedAt *string `json:"deleted_at,omitempty"`
	} `json:"branch"`
}

type testAPIErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func TestBranchesListIncludesMainByDefault(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	res := performRequest(t, handler, http.MethodGet, "/api/v1/branches", "")

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload branchesListResponse
	decodeJSON(t, res, &payload)

	if len(payload.Branches) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(payload.Branches))
	}

	if payload.Branches[0].Name != "main" {
		t.Fatalf("expected default branch %q, got %q", "main", payload.Branches[0].Name)
	}

	if payload.Branches[0].Deleted {
		t.Fatal("expected default branch to not be deleted")
	}
}

func TestCreateBranchAndList(t *testing.T) {
	handler := New(Config{Version: "test-version"})

	createBody := `{"name":"feature-a","parent":"main"}`
	createRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches", createBody)

	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, createRes.Code)
	}

	var created testBranchResponse
	decodeJSON(t, createRes, &created)

	if created.Branch.Name != "feature-a" {
		t.Fatalf("expected created branch name %q, got %q", "feature-a", created.Branch.Name)
	}

	if created.Branch.Parent != "main" {
		t.Fatalf("expected created branch parent %q, got %q", "main", created.Branch.Parent)
	}

	listRes := performRequest(t, handler, http.MethodGet, "/api/v1/branches", "")
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, listRes.Code)
	}

	var listed branchesListResponse
	decodeJSON(t, listRes, &listed)

	if len(listed.Branches) != 2 {
		t.Fatalf("expected 2 branches, got %d", len(listed.Branches))
	}
}

func TestCreateBranchValidationError(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"parent":"main"}`)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}

	assertAPIErrorCode(t, res, "validation_error")
}

func TestCreateBranchInvalidJSON(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches", "{")

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}

	assertAPIErrorCode(t, res, "invalid_json")
}

func TestCreateBranchConflict(t *testing.T) {
	handler := New(Config{Version: "test-version"})

	first := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if first.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, first.Code)
	}

	second := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if second.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, second.Code)
	}

	assertAPIErrorCode(t, second, "conflict")
}

func TestDeleteBranchSoftDelete(t *testing.T) {
	handler := New(Config{Version: "test-version"})

	createRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, createRes.Code)
	}

	deleteRes := performRequest(t, handler, http.MethodDelete, "/api/v1/branches/feature-a", "")
	if deleteRes.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, deleteRes.Code)
	}

	var deleted testBranchResponse
	decodeJSON(t, deleteRes, &deleted)

	if !deleted.Branch.Deleted {
		t.Fatal("expected deleted branch to have deleted=true")
	}

	if deleted.Branch.DeletedAt == nil || *deleted.Branch.DeletedAt == "" {
		t.Fatal("expected deleted branch to include deleted_at")
	}

	listRes := performRequest(t, handler, http.MethodGet, "/api/v1/branches", "")
	if listRes.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, listRes.Code)
	}

	var listed branchesListResponse
	decodeJSON(t, listRes, &listed)

	if len(listed.Branches) != 1 {
		t.Fatalf("expected 1 active branch after delete, got %d", len(listed.Branches))
	}

	if listed.Branches[0].Name != "main" {
		t.Fatalf("expected remaining branch %q, got %q", "main", listed.Branches[0].Name)
	}
}

func TestDeleteMainBranchValidationError(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	res := performRequest(t, handler, http.MethodDelete, "/api/v1/branches/main", "")

	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}

	assertAPIErrorCode(t, res, "validation_error")
}

func TestDeleteUnknownBranchNotFound(t *testing.T) {
	handler := New(Config{Version: "test-version"})
	res := performRequest(t, handler, http.MethodDelete, "/api/v1/branches/missing", "")

	if res.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, res.Code)
	}

	assertAPIErrorCode(t, res, "not_found")
}

func TestResetBranchUpdatesAttachment(t *testing.T) {
	store := branch.NewStore()
	handler := New(Config{
		Version:     "test-version",
		BranchStore: store,
		BranchAttachmentResolver: staticBranchAttachmentResolver{resets: map[string]BranchAttachment{
			"feature-a": {TenantID: "tenant-main", TimelineID: "timeline-reset"},
		}},
	})

	createRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, createRes.Code)
	}

	if _, err := store.SetAttachment("feature-a", "tenant-main", "timeline-old"); err != nil {
		t.Fatalf("set initial attachment: %v", err)
	}

	resetRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches/feature-a/reset", "")
	if resetRes.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resetRes.Code)
	}

	updated, err := store.GetActive("feature-a")
	if err != nil {
		t.Fatalf("get active branch: %v", err)
	}

	if updated.TenantID != "tenant-main" || updated.TimelineID != "timeline-reset" {
		t.Fatalf("expected reset attachment tenant=%q timeline=%q, got tenant=%q timeline=%q", "tenant-main", "timeline-reset", updated.TenantID, updated.TimelineID)
	}
}

func TestResetBranchRejectsMain(t *testing.T) {
	handler := New(Config{Version: "test-version"})

	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/main/reset", "")
	if res.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, res.Code)
	}

	assertAPIErrorCode(t, res, "validation_error")
}

func TestResetBranchReturnsUnavailableWithoutPageserverResolver(t *testing.T) {
	handler := New(Config{Version: "test-version"})

	createRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, createRes.Code)
	}

	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/feature-a/reset", "")
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, res.Code)
	}

	assertAPIErrorCode(t, res, "endpoint_unavailable")
}

func TestResetBranchRefreshesPublishedEndpoint(t *testing.T) {
	store := branch.NewStore()
	branchEndpoints := &fakeBranchEndpointController{}
	handler := New(Config{
		Version:     "test-version",
		BranchStore: store,
		BranchAttachmentResolver: staticBranchAttachmentResolver{resets: map[string]BranchAttachment{
			"feature-a": {TenantID: "tenant-main", TimelineID: "timeline-reset"},
		}},
		BranchEndpoints: branchEndpoints,
	})

	createRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, createRes.Code)
	}

	if _, err := store.SetAttachment("feature-a", "tenant-main", "timeline-old"); err != nil {
		t.Fatalf("set initial attachment: %v", err)
	}

	resetRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches/feature-a/reset", "")
	if resetRes.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resetRes.Code)
	}

	if len(branchEndpoints.refreshCalls) != 1 || branchEndpoints.refreshCalls[0] != "feature-a" {
		t.Fatalf("expected refresh call for feature-a, got %#v", branchEndpoints.refreshCalls)
	}
}

func TestDeleteBranchUnpublishesEndpointBeforeDelete(t *testing.T) {
	branchEndpoints := &fakeBranchEndpointController{}
	handler := New(Config{Version: "test-version", BranchEndpoints: branchEndpoints})

	createRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, createRes.Code)
	}

	deleteRes := performRequest(t, handler, http.MethodDelete, "/api/v1/branches/feature-a", "")
	if deleteRes.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, deleteRes.Code)
	}

	if len(branchEndpoints.unpublishCalls) != 1 || branchEndpoints.unpublishCalls[0] != "feature-a" {
		t.Fatalf("expected unpublish call for feature-a, got %#v", branchEndpoints.unpublishCalls)
	}
}

func performRequest(t *testing.T, handler http.Handler, method string, path string, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	return res
}

func decodeJSON(t *testing.T, res *httptest.ResponseRecorder, out any) {
	t.Helper()

	contentType := res.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("expected application/json content type, got %q", contentType)
	}

	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		t.Fatalf("decode json: %v", err)
	}
}

func assertAPIErrorCode(t *testing.T, res *httptest.ResponseRecorder, code string) {
	t.Helper()

	var payload testAPIErrorResponse
	decodeJSON(t, res, &payload)

	if payload.Error.Code != code {
		t.Fatalf("expected error code %q, got %q", code, payload.Error.Code)
	}

	if payload.Error.Message == "" {
		t.Fatal("expected error message to be non-empty")
	}
}
