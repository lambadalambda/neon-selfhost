package server

import (
	"net/http"
	"sort"
	"testing"

	"neon-selfhost/internal/branch"
)

type branchEndpointConnectionResponse struct {
	Connection struct {
		Branch     string `json:"branch"`
		Published  bool   `json:"published"`
		Status     string `json:"status"`
		Host       string `json:"host"`
		Port       int    `json:"port"`
		Database   string `json:"database"`
		User       string `json:"user"`
		Password   string `json:"password"`
		TenantID   string `json:"tenant_id"`
		TimelineID string `json:"timeline_id"`
		DSN        string `json:"dsn"`
	} `json:"connection"`
}

type branchEndpointListResponse struct {
	Endpoints []struct {
		Branch    string `json:"branch"`
		Published bool   `json:"published"`
		Port      int    `json:"port"`
	} `json:"endpoints"`
}

func TestPublishBranchEndpointReturnsConnectionAndPersistsAttachment(t *testing.T) {
	store := branch.NewStore()
	controller := &fakeBranchEndpointController{}
	handler := New(Config{
		Version:     "test-version",
		BranchStore: store,
		BranchAttachmentResolver: staticBranchAttachmentResolver{attachments: map[string]BranchAttachment{
			"feature-a": {TenantID: "tenant-a", TimelineID: "timeline-a"},
		}},
		BranchEndpoints: controller,
	})

	createRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, createRes.Code)
	}

	publishRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches/feature-a/publish", "")
	if publishRes.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, publishRes.Code)
	}

	if len(controller.publishCalls) != 1 || controller.publishCalls[0] != "feature-a" {
		t.Fatalf("expected publish call for feature-a, got %#v", controller.publishCalls)
	}

	var payload branchEndpointConnectionResponse
	decodeJSON(t, publishRes, &payload)

	if !payload.Connection.Published {
		t.Fatal("expected published=true in publish response")
	}

	if payload.Connection.TenantID != "tenant-a" || payload.Connection.TimelineID != "timeline-a" {
		t.Fatalf("expected tenant/timeline tenant-a/timeline-a, got %s/%s", payload.Connection.TenantID, payload.Connection.TimelineID)
	}

	if payload.Connection.Password == "" {
		t.Fatal("expected non-empty branch password in publish response")
	}

	b, err := store.GetActive("feature-a")
	if err != nil {
		t.Fatalf("get active branch: %v", err)
	}

	if b.TenantID != "tenant-a" || b.TimelineID != "timeline-a" {
		t.Fatalf("expected persisted attachment tenant-a/timeline-a, got %s/%s", b.TenantID, b.TimelineID)
	}
}

func TestUnpublishBranchEndpointReturnsUnpublishedState(t *testing.T) {
	store := branch.NewStore()
	controller := &fakeBranchEndpointController{}
	handler := New(Config{
		Version:     "test-version",
		BranchStore: store,
		BranchAttachmentResolver: staticBranchAttachmentResolver{attachments: map[string]BranchAttachment{
			"feature-a": {TenantID: "tenant-a", TimelineID: "timeline-a"},
		}},
		BranchEndpoints: controller,
	})

	createRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, createRes.Code)
	}

	publishRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches/feature-a/publish", "")
	if publishRes.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, publishRes.Code)
	}

	unpublishRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches/feature-a/unpublish", "")
	if unpublishRes.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, unpublishRes.Code)
	}

	if len(controller.unpublishCalls) != 1 || controller.unpublishCalls[0] != "feature-a" {
		t.Fatalf("expected unpublish call for feature-a, got %#v", controller.unpublishCalls)
	}

	var payload branchEndpointConnectionResponse
	decodeJSON(t, unpublishRes, &payload)
	if payload.Connection.Published {
		t.Fatal("expected published=false after unpublish")
	}
	if payload.Connection.Status != "unpublished" {
		t.Fatalf("expected status %q, got %q", "unpublished", payload.Connection.Status)
	}
}

func TestListBranchEndpointsReturnsPublishedEndpoints(t *testing.T) {
	store := branch.NewStore()
	controller := &fakeBranchEndpointController{
		states: map[string]branchEndpointState{
			"feature-a": {
				Branch:    "feature-a",
				Published: true,
				Port:      56000,
			},
			"feature-b": {
				Branch:    "feature-b",
				Published: true,
				Port:      56001,
			},
		},
	}

	handler := New(Config{Version: "test-version", BranchStore: store, BranchEndpoints: controller})
	res := performRequest(t, handler, http.MethodGet, "/api/v1/endpoints", "")
	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	var payload branchEndpointListResponse
	decodeJSON(t, res, &payload)

	if len(payload.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(payload.Endpoints))
	}

	if payload.Endpoints[0].Branch != "feature-a" || payload.Endpoints[1].Branch != "feature-b" {
		t.Fatalf("expected sorted endpoints for feature-a and feature-b, got %+v", payload.Endpoints)
	}
}

func TestBranchConnectionEndpointReturnsNotFoundForUnknownBranch(t *testing.T) {
	handler := New(Config{Version: "test-version", BranchEndpoints: &fakeBranchEndpointController{}})
	res := performRequest(t, handler, http.MethodGet, "/api/v1/branches/missing/connection", "")
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, res.Code)
	}

	assertAPIErrorCode(t, res, "not_found")
}

type fakeBranchEndpointController struct {
	states         map[string]branchEndpointState
	publishCalls   []string
	unpublishCalls []string
	refreshCalls   []string

	publishErr    error
	unpublishErr  error
	connectionErr error
	listErr       error
	refreshErr    error
}

func (f *fakeBranchEndpointController) Publish(branchName string, attachment BranchAttachment, password string) (branchEndpointState, error) {
	if f.publishErr != nil {
		return branchEndpointState{}, f.publishErr
	}
	f.publishCalls = append(f.publishCalls, branchName)
	if f.states == nil {
		f.states = map[string]branchEndpointState{}
	}

	state := branchEndpointState{
		Branch:     branchName,
		Published:  true,
		Status:     "running",
		Host:       "127.0.0.1",
		Port:       56000 + len(f.states),
		Database:   "postgres",
		User:       "cloud_admin",
		Password:   password,
		TenantID:   attachment.TenantID,
		TimelineID: attachment.TimelineID,
	}
	f.states[branchName] = state
	return state, nil
}

func (f *fakeBranchEndpointController) Unpublish(branchName string) (branchEndpointState, error) {
	if f.unpublishErr != nil {
		return branchEndpointState{}, f.unpublishErr
	}
	f.unpublishCalls = append(f.unpublishCalls, branchName)
	if f.states == nil {
		f.states = map[string]branchEndpointState{}
	}

	state := f.states[branchName]
	state.Branch = branchName
	state.Published = false
	state.Status = "unpublished"
	f.states[branchName] = state
	return state, nil
}

func (f *fakeBranchEndpointController) Connection(branchName string) (branchEndpointState, error) {
	if f.connectionErr != nil {
		return branchEndpointState{}, f.connectionErr
	}
	if f.states == nil {
		return branchEndpointState{}, branch.ErrNotFound
	}
	state, exists := f.states[branchName]
	if !exists {
		return branchEndpointState{}, branch.ErrNotFound
	}
	return state, nil
}

func (f *fakeBranchEndpointController) List() ([]branchEndpointState, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	states := make([]branchEndpointState, 0, len(f.states))
	for _, state := range f.states {
		if state.Published {
			states = append(states, state)
		}
	}
	sort.Slice(states, func(i int, j int) bool {
		return states[i].Branch < states[j].Branch
	})
	return states, nil
}

func (f *fakeBranchEndpointController) Refresh(branchName string, _ BranchAttachment, _ string) error {
	if f.refreshErr != nil {
		return f.refreshErr
	}
	f.refreshCalls = append(f.refreshCalls, branchName)
	return nil
}
