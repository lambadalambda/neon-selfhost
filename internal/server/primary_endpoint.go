package server

import (
	"fmt"
	"sync"
)

const (
	defaultPrimaryEndpointHost     = "127.0.0.1"
	defaultPrimaryEndpointPort     = 5432
	defaultPrimaryEndpointDatabase = "postgres"
	defaultPrimaryEndpointUser     = "postgres"
)

type primaryEndpointState struct {
	Running  bool
	Branch   string
	Host     string
	Port     int
	Database string
	User     string
}

type primaryEndpointManager struct {
	mu    sync.Mutex
	state primaryEndpointState
}

func newPrimaryEndpointManager() *primaryEndpointManager {
	return &primaryEndpointManager{
		state: primaryEndpointState{
			Running:  false,
			Branch:   "main",
			Host:     defaultPrimaryEndpointHost,
			Port:     defaultPrimaryEndpointPort,
			Database: defaultPrimaryEndpointDatabase,
			User:     defaultPrimaryEndpointUser,
		},
	}
}

func (m *primaryEndpointManager) Connection() primaryEndpointState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *primaryEndpointManager) Start() primaryEndpointState {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Running = true
	return m.state
}

func (m *primaryEndpointManager) Stop() primaryEndpointState {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Running = false
	return m.state
}

func (m *primaryEndpointManager) SwitchToBranch(branch string) primaryEndpointState {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Branch = branch
	m.state.Running = true
	return m.state
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
