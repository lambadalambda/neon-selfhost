package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	defaultPrimaryEndpointHost     = "127.0.0.1"
	defaultPrimaryEndpointPort     = 5432
	defaultPrimaryEndpointDatabase = "postgres"
	defaultPrimaryEndpointUser     = "postgres"
)

var (
	ErrPrimaryEndpointUnavailable = errors.New("primary endpoint orchestration unavailable")
	ErrPrimaryEndpointNotFound    = errors.New("primary endpoint container not found")
)

type PrimaryEndpointController interface {
	Connection() (primaryEndpointState, error)
	SetBranchAttachment(branch string, tenantID string, timelineID string) error
	Start() (primaryEndpointState, error)
	Stop() (primaryEndpointState, error)
	SwitchToBranch(branch string) (primaryEndpointState, error)
}

type DockerPrimaryEndpointOptions struct {
	SocketPath     string
	ComposeProject string
	Service        string

	Host     string
	Port     int
	Database string
	User     string

	SelectionPath string
}

type primaryEndpointConnectionInfo struct {
	Host     string
	Port     int
	Database string
	User     string
}

type primaryEndpointRuntime interface {
	Running() (bool, error)
	Start() error
	Stop() error
}

type primaryEndpointState struct {
	Running  bool
	Branch   string
	Host     string
	Port     int
	Database string
	User     string

	TenantID   string
	TimelineID string
}

type primaryEndpointAttachment struct {
	TenantID   string
	TimelineID string
}

type endpointSelectionState struct {
	Branch     string `json:"branch"`
	TenantID   string `json:"tenant_id,omitempty"`
	TimelineID string `json:"timeline_id,omitempty"`
}

type primaryEndpointManager struct {
	mu          sync.Mutex
	runtime     primaryEndpointRuntime
	connInfo    primaryEndpointConnectionInfo
	branch      string
	attachment  primaryEndpointAttachment
	attachments map[string]primaryEndpointAttachment

	selectionPath string
}

func newPrimaryEndpointManager() *primaryEndpointManager {
	return newPrimaryEndpointManagerWithRuntime(
		newInMemoryPrimaryEndpointRuntime(),
		defaultPrimaryEndpointConnectionInfo(),
		"",
	)
}

func defaultPrimaryEndpointConnectionInfo() primaryEndpointConnectionInfo {
	return primaryEndpointConnectionInfo{
		Host:     defaultPrimaryEndpointHost,
		Port:     defaultPrimaryEndpointPort,
		Database: defaultPrimaryEndpointDatabase,
		User:     defaultPrimaryEndpointUser,
	}
}

func newPrimaryEndpointManagerWithRuntime(runtime primaryEndpointRuntime, connInfo primaryEndpointConnectionInfo, selectionPath string) *primaryEndpointManager {
	if runtime == nil {
		runtime = newInMemoryPrimaryEndpointRuntime()
	}

	if strings.TrimSpace(connInfo.Host) == "" {
		connInfo.Host = defaultPrimaryEndpointHost
	}
	if connInfo.Port == 0 {
		connInfo.Port = defaultPrimaryEndpointPort
	}
	if strings.TrimSpace(connInfo.Database) == "" {
		connInfo.Database = defaultPrimaryEndpointDatabase
	}
	if strings.TrimSpace(connInfo.User) == "" {
		connInfo.User = defaultPrimaryEndpointUser
	}

	manager := &primaryEndpointManager{
		runtime:       runtime,
		connInfo:      connInfo,
		branch:        "main",
		attachments:   map[string]primaryEndpointAttachment{},
		selectionPath: strings.TrimSpace(selectionPath),
	}

	if selection, loaded, err := loadEndpointSelection(manager.selectionPath); err == nil && loaded {
		selectionBranch := strings.TrimSpace(selection.Branch)
		if selectionBranch != "" {
			manager.branch = selectionBranch
		}

		if strings.TrimSpace(selection.TenantID) != "" && strings.TrimSpace(selection.TimelineID) != "" {
			attachment := primaryEndpointAttachment{TenantID: strings.TrimSpace(selection.TenantID), TimelineID: strings.TrimSpace(selection.TimelineID)}
			manager.attachment = attachment
			manager.attachments[manager.branch] = attachment
		}
	}

	return manager
}

func NewDockerPrimaryEndpointController(opts DockerPrimaryEndpointOptions) (PrimaryEndpointController, error) {
	runtime, err := newDockerPrimaryEndpointRuntime(opts.SocketPath, opts.ComposeProject, opts.Service)
	if err != nil {
		return nil, err
	}

	connInfo := primaryEndpointConnectionInfo{
		Host:     opts.Host,
		Port:     opts.Port,
		Database: opts.Database,
		User:     opts.User,
	}

	return newPrimaryEndpointManagerWithRuntime(runtime, connInfo, opts.SelectionPath), nil
}

func NewInMemoryPrimaryEndpointController(host string, port int, database string, user string, selectionPath string) PrimaryEndpointController {
	return newPrimaryEndpointManagerWithRuntime(nil, primaryEndpointConnectionInfo{
		Host:     host,
		Port:     port,
		Database: database,
		User:     user,
	}, selectionPath)
}

func (m *primaryEndpointManager) Connection() (primaryEndpointState, error) {
	m.mu.Lock()
	branch := m.branch
	attachment := m.attachment
	connInfo := m.connInfo
	runtime := m.runtime
	m.mu.Unlock()

	running, err := runtime.Running()
	if err != nil {
		return primaryEndpointState{}, fmt.Errorf("query primary endpoint runtime: %w", err)
	}

	return primaryEndpointState{
		Running:  running,
		Branch:   branch,
		Host:     connInfo.Host,
		Port:     connInfo.Port,
		Database: connInfo.Database,
		User:     connInfo.User,

		TenantID:   attachment.TenantID,
		TimelineID: attachment.TimelineID,
	}, nil
}

func (m *primaryEndpointManager) SetBranchAttachment(branch string, tenantID string, timelineID string) error {
	branch = strings.TrimSpace(branch)
	tenantID = strings.TrimSpace(tenantID)
	timelineID = strings.TrimSpace(timelineID)

	if branch == "" {
		return errors.New("branch name is required")
	}
	if tenantID == "" || timelineID == "" {
		return errors.New("tenant and timeline ids are required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	attachment := primaryEndpointAttachment{TenantID: tenantID, TimelineID: timelineID}
	m.attachments[branch] = attachment
	if m.branch == branch {
		m.attachment = attachment
	}

	return nil
}

func (m *primaryEndpointManager) Start() (primaryEndpointState, error) {
	m.mu.Lock()
	runtime := m.runtime
	selectionPath := m.selectionPath
	selection := endpointSelectionState{
		Branch:     m.branch,
		TenantID:   m.attachment.TenantID,
		TimelineID: m.attachment.TimelineID,
	}
	m.mu.Unlock()

	if err := writeEndpointSelection(selectionPath, selection); err != nil {
		return primaryEndpointState{}, err
	}

	if err := runtime.Start(); err != nil {
		return primaryEndpointState{}, fmt.Errorf("start primary endpoint runtime: %w", err)
	}

	return m.Connection()
}

func (m *primaryEndpointManager) Stop() (primaryEndpointState, error) {
	m.mu.Lock()
	runtime := m.runtime
	m.mu.Unlock()

	if err := runtime.Stop(); err != nil {
		return primaryEndpointState{}, fmt.Errorf("stop primary endpoint runtime: %w", err)
	}

	return m.Connection()
}

func (m *primaryEndpointManager) SwitchToBranch(branch string) (primaryEndpointState, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return primaryEndpointState{}, errors.New("branch name is required")
	}

	m.mu.Lock()
	runtime := m.runtime
	selectionPath := m.selectionPath
	previousSelection := endpointSelectionState{Branch: m.branch, TenantID: m.attachment.TenantID, TimelineID: m.attachment.TimelineID}
	attachment := m.attachments[branch]
	m.mu.Unlock()

	if err := runtime.Stop(); err != nil {
		return primaryEndpointState{}, fmt.Errorf("stop primary endpoint for branch switch: %w", err)
	}

	nextSelection := endpointSelectionState{Branch: branch, TenantID: attachment.TenantID, TimelineID: attachment.TimelineID}
	if err := writeEndpointSelection(selectionPath, nextSelection); err != nil {
		return primaryEndpointState{}, err
	}

	if err := runtime.Start(); err != nil {
		_ = writeEndpointSelection(selectionPath, previousSelection)
		return primaryEndpointState{}, fmt.Errorf("start primary endpoint for branch switch: %w", err)
	}

	m.mu.Lock()
	m.branch = branch
	m.attachment = attachment
	m.mu.Unlock()

	return m.Connection()
}

type inMemoryPrimaryEndpointRuntime struct {
	mu      sync.Mutex
	running bool
}

func newInMemoryPrimaryEndpointRuntime() *inMemoryPrimaryEndpointRuntime {
	return &inMemoryPrimaryEndpointRuntime{}
}

func (r *inMemoryPrimaryEndpointRuntime) Running() (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running, nil
}

func (r *inMemoryPrimaryEndpointRuntime) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = true
	return nil
}

func (r *inMemoryPrimaryEndpointRuntime) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
	return nil
}

func isPrimaryEndpointUnavailable(err error) bool {
	return errors.Is(err, ErrPrimaryEndpointUnavailable) || errors.Is(err, ErrPrimaryEndpointNotFound)
}

func loadEndpointSelection(path string) (endpointSelectionState, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return endpointSelectionState{}, false, nil
	}

	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return endpointSelectionState{}, false, nil
	}
	if err != nil {
		return endpointSelectionState{}, false, fmt.Errorf("%w: read endpoint selection: %v", ErrPrimaryEndpointUnavailable, err)
	}

	var selection endpointSelectionState
	if err := json.Unmarshal(content, &selection); err != nil {
		return endpointSelectionState{}, false, fmt.Errorf("%w: decode endpoint selection: %v", ErrPrimaryEndpointUnavailable, err)
	}

	return selection, true, nil
}

func writeEndpointSelection(path string, selection endpointSelectionState) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("%w: create endpoint selection directory: %v", ErrPrimaryEndpointUnavailable, err)
	}

	content, err := json.MarshalIndent(selection, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: encode endpoint selection: %v", ErrPrimaryEndpointUnavailable, err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "endpoint-selection-*.tmp")
	if err != nil {
		return fmt.Errorf("%w: create endpoint selection temp file: %v", ErrPrimaryEndpointUnavailable, err)
	}

	tmpPath := tmp.Name()
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(append(content, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%w: write endpoint selection: %v", ErrPrimaryEndpointUnavailable, err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("%w: close endpoint selection file: %v", ErrPrimaryEndpointUnavailable, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("%w: persist endpoint selection: %v", ErrPrimaryEndpointUnavailable, err)
	}

	succeeded = true
	return nil
}

func makePrimaryConnectionPayload(state primaryEndpointState) primaryEndpointPayload {
	payload := primaryEndpointPayload{
		Status:     "stopped",
		Branch:     state.Branch,
		Host:       state.Host,
		Port:       state.Port,
		Database:   state.Database,
		User:       state.User,
		TenantID:   state.TenantID,
		TimelineID: state.TimelineID,
	}

	if state.Running {
		payload.Status = "running"
		payload.DSN = fmt.Sprintf("postgres://%s@%s:%d/%s?sslmode=disable", state.User, state.Host, state.Port, state.Database)
	}

	return payload
}
