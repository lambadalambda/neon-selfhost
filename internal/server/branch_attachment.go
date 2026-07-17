package server

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"neon-selfhost/internal/branch"
)

const (
	defaultPageserverAPIBaseURL = "http://pageserver:9898"
	defaultPageserverPGVersion  = 16
)

type BranchAttachment struct {
	TenantID   string
	TimelineID string
}

type BranchAttachmentResolver interface {
	Resolve(branchName string) (BranchAttachment, error)
	ResolveReset(branchName string) (BranchAttachment, error)
	ResolveRestore(sourceBranch string, restoreBranch string, restoreAt time.Time) (BranchAttachment, string, error)
}

type BranchAttachmentCleaner interface {
	DeleteCreatedAttachment(attachment BranchAttachment) error
}

type operationCompensationError struct {
	primary      error
	compensation []error
}

func (e *operationCompensationError) Error() string {
	errs := append([]error{e.primary}, e.compensation...)
	return errors.Join(errs...).Error()
}

func (e *operationCompensationError) Unwrap() []error {
	return append([]error{e.primary}, e.compensation...)
}

func addOperationCompensation(primary error, compensation ...error) error {
	if primary == nil {
		return errors.Join(compensation...)
	}
	var existing *operationCompensationError
	if errors.As(primary, &existing) && existing == primary {
		return &operationCompensationError{
			primary:      existing.primary,
			compensation: append(append([]error(nil), existing.compensation...), compensation...),
		}
	}
	return &operationCompensationError{primary: primary, compensation: append([]error(nil), compensation...)}
}

func primaryOperationError(err error) error {
	var compensated *operationCompensationError
	if errors.As(err, &compensated) {
		return compensated.primary
	}
	return err
}

type PageserverBranchAttachmentOptions struct {
	Store      *branch.Store
	BaseURL    string
	PGVersion  int
	HTTPClient *http.Client
}

type noopBranchAttachmentResolver struct{}

func NewNoopBranchAttachmentResolver() BranchAttachmentResolver {
	return noopBranchAttachmentResolver{}
}

func (noopBranchAttachmentResolver) Resolve(_ string) (BranchAttachment, error) {
	return BranchAttachment{}, nil
}

func (noopBranchAttachmentResolver) ResolveReset(_ string) (BranchAttachment, error) {
	return BranchAttachment{}, fmt.Errorf("%w: reset requires pageserver integration", ErrPrimaryEndpointUnavailable)
}

func (noopBranchAttachmentResolver) ResolveRestore(_ string, _ string, _ time.Time) (BranchAttachment, string, error) {
	return BranchAttachment{}, "", fmt.Errorf("%w: restore requires pageserver integration", ErrPrimaryEndpointUnavailable)
}

type pageserverBranchAttachmentResolver struct {
	store     *branch.Store
	client    pageserverAttachmentClient
	pgVersion int

	mu sync.Mutex
}

type pageserverAttachmentClient interface {
	ListTenants() ([]string, error)
	CreateTenant(tenantID string) error
	ListTimelines(tenantID string) ([]string, error)
	CreateTimeline(tenantID string, newTimelineID string, ancestorTimelineID string, ancestorStartLSN string) error
	DeleteTimeline(tenantID string, timelineID string) error
	GetLSNByTimestamp(tenantID string, timelineID string, timestamp time.Time) (string, string, error)
}

func NewPageserverBranchAttachmentResolver(opts PageserverBranchAttachmentOptions) (BranchAttachmentResolver, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("%w: branch store is required", ErrPrimaryEndpointUnavailable)
	}

	baseURL := strings.TrimSpace(opts.BaseURL)
	if baseURL == "" {
		baseURL = defaultPageserverAPIBaseURL
	}

	pgVersion := opts.PGVersion
	if pgVersion <= 0 {
		pgVersion = defaultPageserverPGVersion
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}

	client, err := newPageserverHTTPAttachmentClient(baseURL, pgVersion, httpClient)
	if err != nil {
		return nil, err
	}

	return &pageserverBranchAttachmentResolver{
		store:     opts.Store,
		client:    client,
		pgVersion: pgVersion,
	}, nil
}

func (r *pageserverBranchAttachmentResolver) Resolve(branchName string) (BranchAttachment, error) {
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return BranchAttachment{}, branch.ErrNotFound
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.resolveLocked(branchName, map[string]bool{})
}

func (r *pageserverBranchAttachmentResolver) resolveLocked(branchName string, visiting map[string]bool) (BranchAttachment, error) {
	if visiting[branchName] {
		return BranchAttachment{}, fmt.Errorf("%w: cycle detected while resolving branch attachment for %q", ErrPrimaryEndpointUnavailable, branchName)
	}

	b, err := r.store.GetActive(branchName)
	if err != nil {
		return BranchAttachment{}, err
	}

	if strings.TrimSpace(b.TenantID) != "" && strings.TrimSpace(b.TimelineID) != "" {
		return BranchAttachment{TenantID: b.TenantID, TimelineID: b.TimelineID}, nil
	}

	visiting[branchName] = true
	defer delete(visiting, branchName)

	if branchName == "main" || strings.TrimSpace(b.Parent) == "" {
		mainAttachment, created, err := r.ensureMainAttachment()
		if err != nil {
			return BranchAttachment{}, err
		}

		if _, err := r.store.SetAttachment(branchName, mainAttachment.TenantID, mainAttachment.TimelineID); err != nil {
			if created {
				return BranchAttachment{}, addOperationCompensation(err, r.deleteTimelineError(mainAttachment))
			}
			return BranchAttachment{}, err
		}

		return mainAttachment, nil
	}

	parentAttachment, err := r.resolveLocked(strings.TrimSpace(b.Parent), visiting)
	if err != nil {
		return BranchAttachment{}, err
	}

	newTimelineID, err := randomHexID(16)
	if err != nil {
		return BranchAttachment{}, fmt.Errorf("%w: generate timeline id: %v", ErrPrimaryEndpointUnavailable, err)
	}

	if err := r.client.CreateTimeline(parentAttachment.TenantID, newTimelineID, parentAttachment.TimelineID, ""); err != nil {
		return BranchAttachment{}, err
	}

	if _, err := r.store.SetAttachment(branchName, parentAttachment.TenantID, newTimelineID); err != nil {
		return BranchAttachment{}, addOperationCompensation(err, r.deleteTimelineError(BranchAttachment{TenantID: parentAttachment.TenantID, TimelineID: newTimelineID}))
	}

	return BranchAttachment{TenantID: parentAttachment.TenantID, TimelineID: newTimelineID}, nil
}

func (r *pageserverBranchAttachmentResolver) ensureMainAttachment() (BranchAttachment, bool, error) {
	tenants, err := r.client.ListTenants()
	if err != nil {
		return BranchAttachment{}, false, err
	}

	tenantID := ""
	if len(tenants) > 0 {
		tenantID = tenants[0]
	} else {
		tenantID, err = randomHexID(16)
		if err != nil {
			return BranchAttachment{}, false, fmt.Errorf("%w: generate tenant id: %v", ErrPrimaryEndpointUnavailable, err)
		}

		if err := r.client.CreateTenant(tenantID); err != nil {
			return BranchAttachment{}, false, err
		}
	}

	timelines, err := r.client.ListTimelines(tenantID)
	if err != nil {
		return BranchAttachment{}, false, err
	}

	if len(timelines) > 0 {
		return BranchAttachment{TenantID: tenantID, TimelineID: timelines[0]}, false, nil
	}

	newTimelineID, err := randomHexID(16)
	if err != nil {
		return BranchAttachment{}, false, fmt.Errorf("%w: generate initial timeline id: %v", ErrPrimaryEndpointUnavailable, err)
	}

	if err := r.client.CreateTimeline(tenantID, newTimelineID, "", ""); err != nil {
		return BranchAttachment{}, false, err
	}

	return BranchAttachment{TenantID: tenantID, TimelineID: newTimelineID}, true, nil
}

func (r *pageserverBranchAttachmentResolver) DeleteCreatedAttachment(attachment BranchAttachment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.deleteTimelineError(attachment)
}

func (r *pageserverBranchAttachmentResolver) deleteTimelineError(attachment BranchAttachment) error {
	tenantID := strings.TrimSpace(attachment.TenantID)
	timelineID := strings.TrimSpace(attachment.TimelineID)
	if tenantID == "" || timelineID == "" {
		return fmt.Errorf("%w: timeline cleanup requires tenant and timeline ids", ErrPrimaryEndpointUnavailable)
	}
	if err := r.client.DeleteTimeline(tenantID, timelineID); err != nil {
		return fmt.Errorf("delete pageserver timeline %s/%s: %w", tenantID, timelineID, err)
	}
	return nil
}

func (r *pageserverBranchAttachmentResolver) ResolveRestore(sourceBranch string, _ string, restoreAt time.Time) (BranchAttachment, string, error) {
	sourceAttachment, err := r.Resolve(sourceBranch)
	if err != nil {
		return BranchAttachment{}, "", err
	}

	kind, lsn, err := r.client.GetLSNByTimestamp(sourceAttachment.TenantID, sourceAttachment.TimelineID, restoreAt)
	if err != nil {
		return BranchAttachment{}, "", err
	}

	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "present", "future":
		// Continue with timeline creation.
	case "past", "nodata":
		return BranchAttachment{}, "", ErrRestoreHistoryUnavailable
	default:
		return BranchAttachment{}, "", fmt.Errorf("%w: unknown timestamp lookup result kind %q", ErrPrimaryEndpointUnavailable, kind)
	}

	if strings.TrimSpace(lsn) == "" {
		return BranchAttachment{}, "", fmt.Errorf("%w: empty LSN returned by pageserver", ErrPrimaryEndpointUnavailable)
	}

	newTimelineID, err := randomHexID(16)
	if err != nil {
		return BranchAttachment{}, "", fmt.Errorf("%w: generate restore timeline id: %v", ErrPrimaryEndpointUnavailable, err)
	}

	if err := r.client.CreateTimeline(sourceAttachment.TenantID, newTimelineID, sourceAttachment.TimelineID, lsn); err != nil {
		return BranchAttachment{}, "", err
	}

	return BranchAttachment{TenantID: sourceAttachment.TenantID, TimelineID: newTimelineID}, lsn, nil
}

func (r *pageserverBranchAttachmentResolver) ResolveReset(branchName string) (BranchAttachment, error) {
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return BranchAttachment{}, branch.ErrNotFound
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	target, err := r.store.GetActive(branchName)
	if err != nil {
		return BranchAttachment{}, err
	}

	parentName := strings.TrimSpace(target.Parent)
	if branchName == "main" || parentName == "" {
		return BranchAttachment{}, branch.ErrProtected
	}

	parentAttachment, err := r.resolveLocked(parentName, map[string]bool{})
	if err != nil {
		return BranchAttachment{}, err
	}

	newTimelineID, err := randomHexID(16)
	if err != nil {
		return BranchAttachment{}, fmt.Errorf("%w: generate reset timeline id: %v", ErrPrimaryEndpointUnavailable, err)
	}

	if err := r.client.CreateTimeline(parentAttachment.TenantID, newTimelineID, parentAttachment.TimelineID, ""); err != nil {
		return BranchAttachment{}, err
	}

	return BranchAttachment{TenantID: parentAttachment.TenantID, TimelineID: newTimelineID}, nil
}

func randomHexID(byteLength int) (string, error) {
	if byteLength <= 0 {
		return "", errors.New("byte length must be positive")
	}

	raw := make([]byte, byteLength)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	return hex.EncodeToString(raw), nil
}

type pageserverHTTPAttachmentClient struct {
	baseURL   *url.URL
	pgVersion int
	client    *http.Client
}

func newPageserverHTTPAttachmentClient(baseURL string, pgVersion int, client *http.Client) (*pageserverHTTPAttachmentClient, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("%w: invalid pageserver url %q", ErrPrimaryEndpointUnavailable, baseURL)
	}

	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: invalid pageserver url %q", ErrPrimaryEndpointUnavailable, baseURL)
	}

	return &pageserverHTTPAttachmentClient{
		baseURL:   parsed,
		pgVersion: pgVersion,
		client:    client,
	}, nil
}

func (c *pageserverHTTPAttachmentClient) ListTenants() ([]string, error) {
	body, err := c.request(http.MethodGet, "/v1/tenant", nil)
	if err != nil {
		return nil, err
	}

	var payload []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: decode tenant list response: %v", ErrPrimaryEndpointUnavailable, err)
	}

	tenants := make([]string, 0, len(payload))
	for _, tenant := range payload {
		id := strings.TrimSpace(tenant.ID)
		if id != "" {
			tenants = append(tenants, id)
		}
	}

	return tenants, nil
}

func (c *pageserverHTTPAttachmentClient) CreateTenant(tenantID string) error {
	body := map[string]any{
		"mode":        "AttachedSingle",
		"generation":  1,
		"tenant_conf": map[string]any{},
	}

	_, err := c.request(http.MethodPut, "/v1/tenant/"+tenantID+"/location_config", body)
	return err
}

func (c *pageserverHTTPAttachmentClient) ListTimelines(tenantID string) ([]string, error) {
	body, err := c.request(http.MethodGet, "/v1/tenant/"+tenantID+"/timeline", nil)
	if err != nil {
		return nil, err
	}

	var payload []struct {
		TimelineID string `json:"timeline_id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("%w: decode timeline list response: %v", ErrPrimaryEndpointUnavailable, err)
	}

	timelines := make([]string, 0, len(payload))
	for _, timeline := range payload {
		id := strings.TrimSpace(timeline.TimelineID)
		if id != "" {
			timelines = append(timelines, id)
		}
	}

	return timelines, nil
}

func (c *pageserverHTTPAttachmentClient) CreateTimeline(tenantID string, newTimelineID string, ancestorTimelineID string, ancestorStartLSN string) error {
	body := map[string]any{
		"new_timeline_id": newTimelineID,
		"pg_version":      c.pgVersion,
	}

	ancestorTimelineID = strings.TrimSpace(ancestorTimelineID)
	if ancestorTimelineID != "" {
		body["ancestor_timeline_id"] = ancestorTimelineID
	}

	ancestorStartLSN = strings.TrimSpace(ancestorStartLSN)
	if ancestorStartLSN != "" {
		body["ancestor_start_lsn"] = ancestorStartLSN
	}

	_, err := c.request(http.MethodPost, "/v1/tenant/"+tenantID+"/timeline", body)
	return err
}

func (c *pageserverHTTPAttachmentClient) DeleteTimeline(tenantID string, timelineID string) error {
	_, err := c.request(http.MethodDelete, "/v1/tenant/"+tenantID+"/timeline/"+timelineID, nil)
	return err
}

func (c *pageserverHTTPAttachmentClient) GetLSNByTimestamp(tenantID string, timelineID string, timestamp time.Time) (string, string, error) {
	timestampQuery := url.Values{}
	timestampQuery.Set("timestamp", timestamp.UTC().Format(time.RFC3339))
	timestampQuery.Set("with_lease", "false")

	body, err := c.request(http.MethodGet, "/v1/tenant/"+tenantID+"/timeline/"+timelineID+"/get_lsn_by_timestamp?"+timestampQuery.Encode(), nil)
	if err != nil {
		return "", "", err
	}

	var payload struct {
		Kind string `json:"kind"`
		LSN  string `json:"lsn"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", "", fmt.Errorf("%w: decode timestamp-to-lsn response: %v", ErrPrimaryEndpointUnavailable, err)
	}

	return strings.TrimSpace(payload.Kind), strings.TrimSpace(payload.LSN), nil
}

func (c *pageserverHTTPAttachmentClient) request(method string, requestPath string, body any) ([]byte, error) {
	pathPart, rawQuery, _ := strings.Cut(requestPath, "?")
	pathPart = strings.TrimSpace(pathPart)
	if pathPart == "" {
		pathPart = "/"
	}

	target := *c.baseURL
	target.Path = path.Join(strings.TrimSuffix(c.baseURL.Path, "/"), pathPart)
	target.RawQuery = rawQuery

	var requestBody io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("%w: encode pageserver request payload: %v", ErrPrimaryEndpointUnavailable, err)
		}
		requestBody = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, target.String(), requestBody)
	if err != nil {
		return nil, fmt.Errorf("%w: build pageserver request: %v", ErrPrimaryEndpointUnavailable, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: pageserver request failed: %v", ErrPrimaryEndpointUnavailable, err)
	}
	defer res.Body.Close()

	payload, readErr := io.ReadAll(res.Body)
	if readErr != nil {
		return nil, fmt.Errorf("%w: read pageserver response: %v", ErrPrimaryEndpointUnavailable, readErr)
	}

	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return payload, nil
	}

	return nil, fmt.Errorf("%w: pageserver returned status %d: %s", ErrPrimaryEndpointUnavailable, res.StatusCode, strings.TrimSpace(string(payload)))
}
