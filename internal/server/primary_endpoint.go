package server

import (
	"errors"
	"fmt"
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
}

type primaryEndpointManager struct {
	mu       sync.Mutex
	runtime  primaryEndpointRuntime
	connInfo primaryEndpointConnectionInfo
	branch   string
}

func newPrimaryEndpointManager() *primaryEndpointManager {
	return newPrimaryEndpointManagerWithRuntime(
		newInMemoryPrimaryEndpointRuntime(),
		defaultPrimaryEndpointConnectionInfo(),
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

func newPrimaryEndpointManagerWithRuntime(runtime primaryEndpointRuntime, connInfo primaryEndpointConnectionInfo) *primaryEndpointManager {
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

	return &primaryEndpointManager{
		runtime:  runtime,
		connInfo: connInfo,
		branch:   "main",
	}
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

	return newPrimaryEndpointManagerWithRuntime(runtime, connInfo), nil
}

func NewInMemoryPrimaryEndpointController(host string, port int, database string, user string) PrimaryEndpointController {
	return newPrimaryEndpointManagerWithRuntime(nil, primaryEndpointConnectionInfo{
		Host:     host,
		Port:     port,
		Database: database,
		User:     user,
	})
}

func (m *primaryEndpointManager) Connection() (primaryEndpointState, error) {
	m.mu.Lock()
	branch := m.branch
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
	}, nil
}

func (m *primaryEndpointManager) Start() (primaryEndpointState, error) {
	m.mu.Lock()
	runtime := m.runtime
	m.mu.Unlock()

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
	m.mu.Unlock()

	if err := runtime.Stop(); err != nil {
		return primaryEndpointState{}, fmt.Errorf("stop primary endpoint for branch switch: %w", err)
	}

	if err := runtime.Start(); err != nil {
		return primaryEndpointState{}, fmt.Errorf("start primary endpoint for branch switch: %w", err)
	}

	m.mu.Lock()
	m.branch = branch
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

func makePrimaryConnectionPayload(state primaryEndpointState) primaryEndpointPayload {
	payload := primaryEndpointPayload{
		Status:   "stopped",
		Branch:   state.Branch,
		Host:     state.Host,
		Port:     state.Port,
		Database: state.Database,
		User:     state.User,
	}

	if state.Running {
		payload.Status = "running"
		payload.DSN = fmt.Sprintf("postgres://%s@%s:%d/%s?sslmode=disable", state.User, state.Host, state.Port, state.Database)
	}

	return payload
}
