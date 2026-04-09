package server

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestPrimaryEndpointSwitchPreservesCurrentBranchOnRuntimeFailure(t *testing.T) {
	runtime := &fakePrimaryEndpointRuntime{
		running: true,
		stopErr: errors.New("cannot stop endpoint"),
	}

	manager := newPrimaryEndpointManagerWithRuntime(runtime, primaryEndpointConnectionInfo{
		Host:     "127.0.0.1",
		Port:     5432,
		Database: "postgres",
		User:     "postgres",
	}, "")

	_, err := manager.SwitchToBranch("feature-a")
	if err == nil {
		t.Fatal("expected switch error")
	}

	state, err := manager.Connection()
	if err != nil {
		t.Fatalf("connection state: %v", err)
	}

	if state.Branch != "main" {
		t.Fatalf("expected branch %q to remain after failed switch, got %q", "main", state.Branch)
	}
}

func TestPrimaryEndpointSwitchStopsThenStartsRuntime(t *testing.T) {
	runtime := &fakePrimaryEndpointRuntime{running: true}

	manager := newPrimaryEndpointManagerWithRuntime(runtime, primaryEndpointConnectionInfo{
		Host:     "127.0.0.1",
		Port:     5432,
		Database: "postgres",
		User:     "postgres",
	}, "")

	state, err := manager.SwitchToBranch("feature-a")
	if err != nil {
		t.Fatalf("switch branch: %v", err)
	}

	if state.Branch != "feature-a" {
		t.Fatalf("expected branch %q, got %q", "feature-a", state.Branch)
	}

	if runtime.stopCalls == 0 {
		t.Fatal("expected switch to stop runtime before start")
	}

	if runtime.startCalls == 0 {
		t.Fatal("expected switch to start runtime")
	}
}

func TestPrimaryEndpointStartReturnsEndpointUnavailableErrors(t *testing.T) {
	handler := New(Config{
		Version:         "test-version",
		PrimaryEndpoint: failingPrimaryEndpointController{startErr: fmt.Errorf("%w: docker socket unavailable", ErrPrimaryEndpointUnavailable)},
	})

	res := performRequest(t, handler, http.MethodPost, "/api/v1/endpoints/primary/start", "")

	if res.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, res.Code)
	}

	assertAPIErrorCode(t, res, "endpoint_unavailable")
}

type fakePrimaryEndpointRuntime struct {
	running        bool
	ready          bool
	readySet       bool
	runtimeState   string
	runtimeMessage string
	statusErr      error
	startErr       error
	stopErr        error
	startCalls     int
	stopCalls      int
}

func (f *fakePrimaryEndpointRuntime) Status() (primaryEndpointRuntimeStatus, error) {
	if f.statusErr != nil {
		return primaryEndpointRuntimeStatus{}, f.statusErr
	}

	ready := f.running
	if f.readySet {
		ready = f.ready
	}

	state := f.runtimeState
	if state == "" {
		if f.running {
			state = "running"
		} else {
			state = "stopped"
		}
	}

	return primaryEndpointRuntimeStatus{
		Running: f.running,
		Ready:   ready,
		State:   state,
		Message: f.runtimeMessage,
	}, nil
}

func (f *fakePrimaryEndpointRuntime) Start() error {
	f.startCalls++
	if f.startErr != nil {
		return f.startErr
	}
	f.running = true
	return nil
}

func (f *fakePrimaryEndpointRuntime) Stop() error {
	f.stopCalls++
	if f.stopErr != nil {
		return f.stopErr
	}
	f.running = false
	return nil
}

type failingPrimaryEndpointController struct {
	connectionErr error
	setErr        error
	startErr      error
	stopErr       error
	switchErr     error
}

func (f failingPrimaryEndpointController) Connection() (primaryEndpointState, error) {
	if f.connectionErr != nil {
		return primaryEndpointState{}, f.connectionErr
	}
	return primaryEndpointState{Branch: "main"}, nil
}

func (f failingPrimaryEndpointController) SetBranchAttachment(_ string, _ string, _ string) error {
	return f.setErr
}

func (f failingPrimaryEndpointController) SetBranchPassword(_ string, _ string) error {
	return f.setErr
}

func (f failingPrimaryEndpointController) Start() (primaryEndpointState, error) {
	if f.startErr != nil {
		return primaryEndpointState{}, f.startErr
	}
	return primaryEndpointState{Running: true, Branch: "main"}, nil
}

func (f failingPrimaryEndpointController) Stop() (primaryEndpointState, error) {
	if f.stopErr != nil {
		return primaryEndpointState{}, f.stopErr
	}
	return primaryEndpointState{Running: false, Branch: "main"}, nil
}

func (f failingPrimaryEndpointController) SwitchToBranch(branch string) (primaryEndpointState, error) {
	if f.switchErr != nil {
		return primaryEndpointState{}, f.switchErr
	}
	return primaryEndpointState{Running: true, Branch: branch}, nil
}
