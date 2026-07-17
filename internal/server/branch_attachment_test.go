package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

func TestResetActiveBranchAppliesAttachmentAndRestarts(t *testing.T) {
	controller := &capturingPrimaryEndpointController{state: primaryEndpointState{Branch: "feature-a", Running: true}}
	resolver := staticBranchAttachmentResolver{resets: map[string]BranchAttachment{
		"feature-a": {TenantID: "tenant-main", TimelineID: "timeline-reset"},
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

	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/feature-a/reset", "")

	if res.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, res.Code)
	}

	if controller.lastSetBranch != "feature-a" {
		t.Fatalf("expected reset to set attachment for %q, got %q", "feature-a", controller.lastSetBranch)
	}

	if controller.lastSetTimelineID != "timeline-reset" {
		t.Fatalf("expected reset timeline %q, got %q", "timeline-reset", controller.lastSetTimelineID)
	}

	if controller.lastSetPassword == "" {
		t.Fatal("expected reset to set branch password")
	}

	if controller.lastSwitchBranch != "feature-a" {
		t.Fatalf("expected reset to restart active branch %q, got %q", "feature-a", controller.lastSwitchBranch)
	}
}

func TestPageserverBranchAttachmentResolverResolveRestoreCreatesTimelineAtResolvedLSN(t *testing.T) {
	store := branch.NewStore()
	client := &fakePageserverAttachmentClient{getLSNKind: "future", getLSNValue: "0/16B6F50"}
	resolver := &pageserverBranchAttachmentResolver{
		store:     store,
		client:    client,
		pgVersion: 16,
	}

	mainAttachment, err := resolver.Resolve("main")
	if err != nil {
		t.Fatalf("resolve main attachment: %v", err)
	}

	attachment, resolvedLSN, err := resolver.ResolveRestore("main", "restore-a", time.Date(2010, 1, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("resolve restore attachment: %v", err)
	}

	if resolvedLSN != "0/16B6F50" {
		t.Fatalf("expected resolved lsn %q, got %q", "0/16B6F50", resolvedLSN)
	}

	if attachment.TenantID != mainAttachment.TenantID {
		t.Fatalf("expected restore attachment tenant %q, got %q", mainAttachment.TenantID, attachment.TenantID)
	}

	if attachment.TimelineID == "" || attachment.TimelineID == mainAttachment.TimelineID {
		t.Fatalf("expected restore timeline to differ from source timeline %q, got %q", mainAttachment.TimelineID, attachment.TimelineID)
	}

	if len(client.createTimelineCalls) < 2 {
		t.Fatalf("expected at least 2 timeline create calls, got %d", len(client.createTimelineCalls))
	}

	restoreCreate := client.createTimelineCalls[len(client.createTimelineCalls)-1]
	if restoreCreate.AncestorTimelineID != mainAttachment.TimelineID {
		t.Fatalf("expected restore ancestor timeline %q, got %q", mainAttachment.TimelineID, restoreCreate.AncestorTimelineID)
	}

	if restoreCreate.AncestorStartLSN != "0/16B6F50" {
		t.Fatalf("expected restore ancestor start lsn %q, got %q", "0/16B6F50", restoreCreate.AncestorStartLSN)
	}
}

func TestPageserverBranchAttachmentResolverResolveRestoreReturnsHistoryUnavailable(t *testing.T) {
	store := branch.NewStore()
	client := &fakePageserverAttachmentClient{getLSNKind: "past", getLSNValue: ""}
	resolver := &pageserverBranchAttachmentResolver{
		store:     store,
		client:    client,
		pgVersion: 16,
	}

	if _, err := resolver.Resolve("main"); err != nil {
		t.Fatalf("resolve main attachment: %v", err)
	}

	_, _, err := resolver.ResolveRestore("main", "restore-a", time.Date(2010, 1, 2, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrRestoreHistoryUnavailable) {
		t.Fatalf("expected %v, got %v", ErrRestoreHistoryUnavailable, err)
	}
}

func TestPageserverBranchAttachmentResolverResolveRestoreRejectsUnknownKind(t *testing.T) {
	store := branch.NewStore()
	client := &fakePageserverAttachmentClient{getLSNKind: "mystery", getLSNValue: "0/16B6F50"}
	resolver := &pageserverBranchAttachmentResolver{
		store:     store,
		client:    client,
		pgVersion: 16,
	}

	if _, err := resolver.Resolve("main"); err != nil {
		t.Fatalf("resolve main attachment: %v", err)
	}

	_, _, err := resolver.ResolveRestore("main", "restore-a", time.Date(2010, 1, 2, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrPrimaryEndpointUnavailable) {
		t.Fatalf("expected %v, got %v", ErrPrimaryEndpointUnavailable, err)
	}
}

func TestPageserverBranchAttachmentResolverResolveResetCreatesNewTimelineFromParent(t *testing.T) {
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

	resetAttachment, err := resolver.ResolveReset("feature-a")
	if err != nil {
		t.Fatalf("resolve reset attachment: %v", err)
	}

	if resetAttachment.TenantID != mainAttachment.TenantID {
		t.Fatalf("expected reset tenant %q, got %q", mainAttachment.TenantID, resetAttachment.TenantID)
	}

	if resetAttachment.TimelineID == "" || resetAttachment.TimelineID == mainAttachment.TimelineID {
		t.Fatalf("expected reset timeline to differ from main timeline %q, got %q", mainAttachment.TimelineID, resetAttachment.TimelineID)
	}

	resetCall := client.createTimelineCalls[len(client.createTimelineCalls)-1]
	if resetCall.AncestorTimelineID != mainAttachment.TimelineID {
		t.Fatalf("expected reset ancestor timeline %q, got %q", mainAttachment.TimelineID, resetCall.AncestorTimelineID)
	}

	if resetCall.AncestorStartLSN != "" {
		t.Fatalf("expected reset ancestor_start_lsn to be empty, got %q", resetCall.AncestorStartLSN)
	}
}

func TestPageserverResolverCleansCreatedTimelineWhenAttachmentPersistenceFails(t *testing.T) {
	store, err := branch.NewSQLitePersistentStore(t.TempDir())
	if err != nil {
		t.Fatalf("create SQLite store: %v", err)
	}
	if _, err := store.Create("feature-a", "main"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	if _, err := store.SetAttachment("main", "tenant-main", "timeline-main"); err != nil {
		t.Fatalf("set main attachment: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	cleanupErr := errors.New("delete timeline failed")
	client := &fakePageserverAttachmentClient{deleteTimelineErr: cleanupErr}
	resolver := &pageserverBranchAttachmentResolver{store: store, client: client, pgVersion: 16}
	_, err = resolver.Resolve("feature-a")
	if err == nil {
		t.Fatal("expected persistence failure")
	}
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("expected cleanup error identity, got %v", err)
	}
	if len(client.deleteTimelineCalls) != 1 {
		t.Fatalf("expected one timeline cleanup, got %+v", client.deleteTimelineCalls)
	}
}

func TestPageserverClientDeletesTimeline(t *testing.T) {
	var method string
	var requestPath string
	pageserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		requestPath = r.URL.Path
		w.WriteHeader(http.StatusAccepted)
	}))
	defer pageserver.Close()
	client, err := newPageserverHTTPAttachmentClient(pageserver.URL, 16, pageserver.Client())
	if err != nil {
		t.Fatalf("create pageserver client: %v", err)
	}

	if err := client.DeleteTimeline("tenant-a", "timeline-a"); err != nil {
		t.Fatalf("delete timeline: %v", err)
	}
	if method != http.MethodDelete || requestPath != "/v1/tenant/tenant-a/timeline/timeline-a" {
		t.Fatalf("expected DELETE timeline request, got %s %s", method, requestPath)
	}
}

func TestResetCleansCreatedTimelineWhenAttachmentPersistenceFails(t *testing.T) {
	store, err := branch.NewSQLitePersistentStore(t.TempDir())
	if err != nil {
		t.Fatalf("create SQLite store: %v", err)
	}
	if _, err := store.CreateWithPassword("feature-a", "main", "secret"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	resolver := &cleanupTrackingResolver{attachment: BranchAttachment{TenantID: "tenant-main", TimelineID: "timeline-reset"}}
	handler := New(Config{Version: "test-version", BranchStore: store, BranchAttachmentResolver: resolver})
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/feature-a/reset", "")
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, res.Code)
	}
	if len(resolver.deleted) != 1 || resolver.deleted[0] != resolver.attachment {
		t.Fatalf("expected reset timeline cleanup, got %+v", resolver.deleted)
	}
}

func TestResetRestoresPreviousAttachmentBeforeCleaningTimelineOnDownstreamFailure(t *testing.T) {
	store := branch.NewStore()
	if _, err := store.CreateWithPassword("feature-a", "main", "secret"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	if _, err := store.SetAttachment("feature-a", "tenant-main", "timeline-old"); err != nil {
		t.Fatalf("set old attachment: %v", err)
	}
	newAttachment := BranchAttachment{TenantID: "tenant-main", TimelineID: "timeline-new"}
	resolver := &cleanupTrackingResolver{attachment: newAttachment}
	controller := &capturingPrimaryEndpointController{
		state:               primaryEndpointState{Branch: "feature-a", Running: true},
		setAttachmentErrors: []error{fmt.Errorf("%w: set new attachment failed", ErrPrimaryEndpointUnavailable), nil},
	}
	handler := New(Config{
		Version:                  "test-version",
		BranchStore:              store,
		BranchAttachmentResolver: resolver,
		PrimaryEndpoint:          controller,
	})

	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/feature-a/reset", "")
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, res.Code)
	}
	current, err := store.GetActive("feature-a")
	if err != nil {
		t.Fatalf("get rolled-back branch: %v", err)
	}
	if current.TimelineID != "timeline-old" {
		t.Fatalf("expected old timeline after rollback, got %q", current.TimelineID)
	}
	if len(resolver.deleted) != 1 || resolver.deleted[0] != newAttachment {
		t.Fatalf("expected cleanup of new timeline, got %+v", resolver.deleted)
	}
}

func TestResetReappliesPreviousPrimarySelectionBeforeCleaningTimeline(t *testing.T) {
	store := branch.NewStore()
	if _, err := store.CreateWithPassword("feature-a", "main", "secret"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	if _, err := store.SetAttachment("feature-a", "tenant-main", "timeline-old"); err != nil {
		t.Fatalf("set old attachment: %v", err)
	}
	newAttachment := BranchAttachment{TenantID: "tenant-main", TimelineID: "timeline-new"}
	resolver := &cleanupTrackingResolver{attachment: newAttachment}
	controller := &capturingPrimaryEndpointController{
		state:        primaryEndpointState{Branch: "feature-a", Running: true, TenantID: "tenant-main", TimelineID: "timeline-old"},
		switchErrors: []error{fmt.Errorf("%w: switch failed", ErrPrimaryEndpointUnavailable), nil},
	}
	handler := New(Config{
		Version:                  "test-version",
		BranchStore:              store,
		BranchAttachmentResolver: resolver,
		PrimaryEndpoint:          controller,
	})

	res := performRequest(t, handler, http.MethodPost, "/api/v1/branches/feature-a/reset", "")
	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, res.Code)
	}
	if controller.switchCalls != 2 {
		t.Fatalf("expected failed switch and compensating switch, got %d calls", controller.switchCalls)
	}
	if controller.state.TimelineID != "timeline-old" {
		t.Fatalf("expected primary endpoint restored to old timeline, got %q", controller.state.TimelineID)
	}
	if len(resolver.deleted) != 1 || resolver.deleted[0] != newAttachment {
		t.Fatalf("expected cleanup after primary rollback, got %+v", resolver.deleted)
	}
}

type staticBranchAttachmentResolver struct {
	attachments map[string]BranchAttachment
	resets      map[string]BranchAttachment
	restore     BranchAttachment
	restoreLSN  string
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

func (r staticBranchAttachmentResolver) ResolveReset(branchName string) (BranchAttachment, error) {
	if r.err != nil {
		return BranchAttachment{}, r.err
	}

	attachment, exists := r.resets[branchName]
	if !exists {
		return BranchAttachment{}, branch.ErrNotFound
	}

	return attachment, nil
}

func (r staticBranchAttachmentResolver) ResolveRestore(_ string, _ string, _ time.Time) (BranchAttachment, string, error) {
	if r.err != nil {
		return BranchAttachment{}, "", r.err
	}

	if strings.TrimSpace(r.restore.TenantID) == "" || strings.TrimSpace(r.restore.TimelineID) == "" {
		return BranchAttachment{}, "", branch.ErrNotFound
	}

	return r.restore, r.restoreLSN, nil
}

type capturingPrimaryEndpointController struct {
	state primaryEndpointState

	lastSetBranch       string
	lastSetTenantID     string
	lastSetTimelineID   string
	lastSetPassword     string
	lastSwitchBranch    string
	setAttachmentErrors []error
	switchErrors        []error
	switchCalls         int
}

func (c *capturingPrimaryEndpointController) Connection() (primaryEndpointState, error) {
	return c.state, nil
}

func (c *capturingPrimaryEndpointController) SetBranchAttachment(branchName string, tenantID string, timelineID string) error {
	if len(c.setAttachmentErrors) > 0 {
		err := c.setAttachmentErrors[0]
		c.setAttachmentErrors = c.setAttachmentErrors[1:]
		if err != nil {
			return err
		}
	}
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

func (c *capturingPrimaryEndpointController) SetBranchPassword(branchName string, password string) error {
	if strings.TrimSpace(branchName) == "" || strings.TrimSpace(password) == "" {
		return errors.New("invalid password")
	}

	c.lastSetPassword = password
	if c.state.Branch == branchName {
		c.state.Password = password
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
	c.switchCalls++
	if len(c.switchErrors) > 0 {
		err := c.switchErrors[0]
		c.switchErrors = c.switchErrors[1:]
		if err != nil {
			return primaryEndpointState{}, err
		}
	}
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
	deleteTimelineCalls []BranchAttachment
	deleteTimelineErr   error

	getLSNKind  string
	getLSNValue string
	getLSNErr   error
}

type fakeTimelineCreateCall struct {
	TenantID           string
	TimelineID         string
	AncestorTimelineID string
	AncestorStartLSN   string
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

func (f *fakePageserverAttachmentClient) CreateTimeline(tenantID string, newTimelineID string, ancestorTimelineID string, ancestorStartLSN string) error {
	if f.timelinesByTenant == nil {
		f.timelinesByTenant = map[string][]string{}
	}

	f.createTimelineCalls = append(f.createTimelineCalls, fakeTimelineCreateCall{TenantID: tenantID, TimelineID: newTimelineID, AncestorTimelineID: ancestorTimelineID, AncestorStartLSN: ancestorStartLSN})
	f.timelinesByTenant[tenantID] = append(f.timelinesByTenant[tenantID], newTimelineID)
	return nil
}

func (f *fakePageserverAttachmentClient) DeleteTimeline(tenantID string, timelineID string) error {
	f.deleteTimelineCalls = append(f.deleteTimelineCalls, BranchAttachment{TenantID: tenantID, TimelineID: timelineID})
	return f.deleteTimelineErr
}

func (f *fakePageserverAttachmentClient) GetLSNByTimestamp(_ string, _ string, _ time.Time) (string, string, error) {
	if f.getLSNErr != nil {
		return "", "", f.getLSNErr
	}

	kind := f.getLSNKind
	if kind == "" {
		kind = "future"
	}

	lsn := f.getLSNValue
	if lsn == "" && kind == "future" {
		lsn = "0/16B6F50"
	}

	return kind, lsn, nil
}
