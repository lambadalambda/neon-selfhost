package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"neon-selfhost/internal/branch"
)

func TestPageserverBranchAttachmentResolverResolvesMainAndChild(t *testing.T) {
	store := branch.NewStore()
	if _, err := store.Create("feature-a", "main"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	client := &fakePageserverAttachmentClient{}
	resolver := &pageserverBranchAttachmentResolver{
		store:     store,
		client:    client,
		pgVersion: 16,
	}

	mainAttachment, err := resolver.Resolve("main")
	if err != nil {
		t.Fatalf("resolve main attachment: %v", err)
	}

	if mainAttachment.TenantID == "" || mainAttachment.TimelineID == "" {
		t.Fatalf("expected main attachment to include tenant and timeline ids, got tenant=%q timeline=%q", mainAttachment.TenantID, mainAttachment.TimelineID)
	}

	featureAttachment, err := resolver.Resolve("feature-a")
	if err != nil {
		t.Fatalf("resolve feature attachment: %v", err)
	}

	if featureAttachment.TenantID != mainAttachment.TenantID {
		t.Fatalf("expected feature attachment to reuse tenant %q, got %q", mainAttachment.TenantID, featureAttachment.TenantID)
	}

	if featureAttachment.TimelineID == "" || featureAttachment.TimelineID == mainAttachment.TimelineID {
		t.Fatalf("expected feature timeline to differ from main timeline, got %q", featureAttachment.TimelineID)
	}

	if len(client.createTimelineCalls) < 2 {
		t.Fatalf("expected at least 2 timeline create calls, got %d", len(client.createTimelineCalls))
	}

	childCreate := client.createTimelineCalls[len(client.createTimelineCalls)-1]
	if childCreate.AncestorTimelineID != mainAttachment.TimelineID {
		t.Fatalf("expected child ancestor timeline %q, got %q", mainAttachment.TimelineID, childCreate.AncestorTimelineID)
	}
}

func TestPrimaryEndpointStartResolvesAndSetsAttachment(t *testing.T) {
	controller := &capturingPrimaryEndpointController{state: primaryEndpointState{Branch: "main"}}
	resolver := staticBranchAttachmentResolver{attachments: map[string]BranchAttachment{
		"main": {TenantID: "tenant-main", TimelineID: "timeline-main"},
	}}

	handler := New(Config{
		Version:                  "test-version",
		PrimaryEndpoint:          controller,
		BranchAttachmentResolver: resolver,
	})

	res := performRequest(t, handler, http.MethodPost, "/api/v1/endpoints/primary/start", "")

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	if controller.lastSetBranch != "main" {
		t.Fatalf("expected attachment set for branch %q, got %q", "main", controller.lastSetBranch)
	}

	if controller.lastSetTenantID != "tenant-main" || controller.lastSetTimelineID != "timeline-main" {
		t.Fatalf("expected start to set attachment tenant=%q timeline=%q, got tenant=%q timeline=%q", "tenant-main", "timeline-main", controller.lastSetTenantID, controller.lastSetTimelineID)
	}
}

func TestPrimaryEndpointSwitchResolvesAndSetsAttachment(t *testing.T) {
	controller := &capturingPrimaryEndpointController{state: primaryEndpointState{Branch: "main"}}
	resolver := staticBranchAttachmentResolver{attachments: map[string]BranchAttachment{
		"feature-a": {TenantID: "tenant-main", TimelineID: "timeline-feature"},
	}}

	handler := New(Config{
		Version:                  "test-version",
		PrimaryEndpoint:          controller,
		BranchAttachmentResolver: resolver,
	})

	createRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, createRes.Code)
	}

	switchRes := performRequest(t, handler, http.MethodPost, "/api/v1/endpoints/primary/switch", `{"branch":"feature-a"}`)
	if switchRes.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, switchRes.Code)
	}

	if controller.lastSetBranch != "feature-a" {
		t.Fatalf("expected attachment set for branch %q, got %q", "feature-a", controller.lastSetBranch)
	}

	if controller.lastSetTimelineID != "timeline-feature" {
		t.Fatalf("expected switch to set timeline %q, got %q", "timeline-feature", controller.lastSetTimelineID)
	}

	if controller.lastSwitchBranch != "feature-a" {
		t.Fatalf("expected switch call for branch %q, got %q", "feature-a", controller.lastSwitchBranch)
	}
}

func TestPrimaryEndpointSwitchReturnsUnavailableWhenResolverFails(t *testing.T) {
	handler := New(Config{
		Version:                  "test-version",
		BranchAttachmentResolver: staticBranchAttachmentResolver{err: fmt.Errorf("%w: pageserver down", ErrPrimaryEndpointUnavailable)},
	})

	createRes := performRequest(t, handler, http.MethodPost, "/api/v1/branches", `{"name":"feature-a"}`)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, createRes.Code)
	}

	res := performRequest(t, handler, http.MethodPost, "/api/v1/endpoints/primary/switch", `{"branch":"feature-a"}`)

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, res.Code)
	}

	assertAPIErrorCode(t, res, "endpoint_unavailable")
}

type staticBranchAttachmentResolver struct {
	attachments map[string]BranchAttachment
	err         error
}

func (r staticBranchAttachmentResolver) Resolve(branchName string) (BranchAttachment, error) {
	if r.err != nil {
		return BranchAttachment{}, r.err
	}

	attachment, exists := r.attachments[branchName]
	if !exists {
		return BranchAttachment{}, branch.ErrNotFound
	}

	return attachment, nil
}

type capturingPrimaryEndpointController struct {
	state primaryEndpointState

	lastSetBranch     string
	lastSetTenantID   string
	lastSetTimelineID string
	lastSwitchBranch  string
}

func (c *capturingPrimaryEndpointController) Connection() (primaryEndpointState, error) {
	return c.state, nil
}

func (c *capturingPrimaryEndpointController) SetBranchAttachment(branchName string, tenantID string, timelineID string) error {
	if strings.TrimSpace(branchName) == "" || strings.TrimSpace(tenantID) == "" || strings.TrimSpace(timelineID) == "" {
		return errors.New("invalid attachment")
	}

	c.lastSetBranch = branchName
	c.lastSetTenantID = tenantID
	c.lastSetTimelineID = timelineID

	if c.state.Branch == branchName {
		c.state.TenantID = tenantID
		c.state.TimelineID = timelineID
	}

	return nil
}

func (c *capturingPrimaryEndpointController) Start() (primaryEndpointState, error) {
	c.state.Running = true
	return c.state, nil
}

func (c *capturingPrimaryEndpointController) Stop() (primaryEndpointState, error) {
	c.state.Running = false
	return c.state, nil
}

func (c *capturingPrimaryEndpointController) SwitchToBranch(branchName string) (primaryEndpointState, error) {
	c.lastSwitchBranch = branchName
	c.state.Branch = branchName
	c.state.Running = true
	c.state.TenantID = c.lastSetTenantID
	c.state.TimelineID = c.lastSetTimelineID
	return c.state, nil
}

type fakePageserverAttachmentClient struct {
	tenants             []string
	timelinesByTenant   map[string][]string
	createTenantCalls   []string
	createTimelineCalls []fakeTimelineCreateCall
}

type fakeTimelineCreateCall struct {
	TenantID           string
	TimelineID         string
	AncestorTimelineID string
}

func (f *fakePageserverAttachmentClient) ListTenants() ([]string, error) {
	return append([]string(nil), f.tenants...), nil
}

func (f *fakePageserverAttachmentClient) CreateTenant(tenantID string) error {
	f.createTenantCalls = append(f.createTenantCalls, tenantID)
	f.tenants = append(f.tenants, tenantID)
	if f.timelinesByTenant == nil {
		f.timelinesByTenant = map[string][]string{}
	}
	return nil
}

func (f *fakePageserverAttachmentClient) ListTimelines(tenantID string) ([]string, error) {
	if f.timelinesByTenant == nil {
		return nil, nil
	}

	return append([]string(nil), f.timelinesByTenant[tenantID]...), nil
}

func (f *fakePageserverAttachmentClient) CreateTimeline(tenantID string, newTimelineID string, ancestorTimelineID string) error {
	if f.timelinesByTenant == nil {
		f.timelinesByTenant = map[string][]string{}
	}

	f.createTimelineCalls = append(f.createTimelineCalls, fakeTimelineCreateCall{TenantID: tenantID, TimelineID: newTimelineID, AncestorTimelineID: ancestorTimelineID})
	f.timelinesByTenant[tenantID] = append(f.timelinesByTenant[tenantID], newTimelineID)
	return nil
}
