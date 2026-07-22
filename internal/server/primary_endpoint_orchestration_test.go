package server

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestPrimaryEndpointSwitchWaitsForActiveRouteLease(t *testing.T) {
	dir := t.TempDir()
	selectionPath := filepath.Join(dir, "endpoint-selection.json")
	mainSelection := endpointSelectionState{Branch: "main", TenantID: "tenant", TimelineID: "timeline-main", Password: "secret-main"}
	if err := writeEndpointSelection(selectionPath, mainSelection); err != nil {
		t.Fatalf("write desired selection: %v", err)
	}
	if err := writeEndpointSelection(endpointAppliedSelectionPath(selectionPath), endpointSelectionState{Branch: "main", TenantID: "tenant", TimelineID: "timeline-main"}); err != nil {
		t.Fatalf("write applied selection: %v", err)
	}

	runtime := &fakePrimaryEndpointRuntime{running: true}
	runtime.startHook = func() {
		selection, _, _ := loadEndpointSelection(selectionPath)
		selection.Password = ""
		_ = writeEndpointSelection(endpointAppliedSelectionPath(selectionPath), selection)
	}
	manager := newPrimaryEndpointManagerWithRuntime(runtime, primaryEndpointConnectionInfo{
		Host: "127.0.0.1", Port: 55433, Database: "postgres", User: "cloud_admin", Password: "secret-main",
	}, selectionPath)
	manager.startupTimeout = time.Second
	if err := manager.SetBranchAttachment("feature-a", "tenant", "timeline-feature"); err != nil {
		t.Fatalf("set feature attachment: %v", err)
	}

	_, releaseRoute, err := manager.AcquirePrimaryRouteState()
	if err != nil {
		t.Fatalf("acquire primary route: %v", err)
	}
	switchDone := make(chan error, 1)
	go func() {
		_, switchErr := manager.SwitchToBranch("feature-a")
		switchDone <- switchErr
	}()

	select {
	case err := <-switchDone:
		t.Fatalf("switch completed while route lease was active: %v", err)
	case <-time.After(25 * time.Millisecond):
	}

	releaseRoute()
	select {
	case err := <-switchDone:
		if err != nil {
			t.Fatalf("switch after route release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("switch did not continue after route lease release")
	}
}

func TestPrimaryEndpointSwitchRestartsPreviousRuntimeWhenSelectionWriteFails(t *testing.T) {
	runtime := &fakePrimaryEndpointRuntime{running: true}
	manager := newPrimaryEndpointManagerWithRuntime(runtime, primaryEndpointConnectionInfo{
		Host: "127.0.0.1", Port: 5432, Database: "postgres", User: "postgres",
	}, t.TempDir())
	manager.startupTimeout = time.Millisecond

	_, err := manager.SwitchToBranch("feature-a")
	if err == nil {
		t.Fatal("expected switch error")
	}
	if !runtime.running {
		t.Fatal("expected previous runtime to be restarted")
	}
	if runtime.startCalls != 1 {
		t.Fatalf("expected one rollback start, got %d", runtime.startCalls)
	}
}

func TestPrimaryEndpointSwitchRestoresSelectionAndRuntimeWhenNewStartFails(t *testing.T) {
	selectionPath := filepath.Join(t.TempDir(), "endpoint-selection.json")
	previousSelection := endpointSelectionState{Branch: "main", TenantID: "tenant-main", TimelineID: "timeline-main", Password: "secret-main"}
	if err := writeEndpointSelection(selectionPath, previousSelection); err != nil {
		t.Fatalf("write previous selection: %v", err)
	}
	if err := writeEndpointSelection(endpointAppliedSelectionPath(selectionPath), previousSelection); err != nil {
		t.Fatalf("write previous applied selection: %v", err)
	}

	runtime := &fakePrimaryEndpointRuntime{running: true, startErrors: []error{errors.New("cannot start feature"), nil}}
	manager := newPrimaryEndpointManagerWithRuntime(runtime, primaryEndpointConnectionInfo{
		Host: "127.0.0.1", Port: 5432, Database: "postgres", User: "postgres",
	}, selectionPath)
	if err := manager.SetBranchAttachment("feature-a", "tenant-feature", "timeline-feature"); err != nil {
		t.Fatalf("set feature attachment: %v", err)
	}
	if err := manager.SetBranchPassword("feature-a", "secret-feature"); err != nil {
		t.Fatalf("set feature password: %v", err)
	}

	_, err := manager.SwitchToBranch("feature-a")
	if err == nil {
		t.Fatal("expected switch error")
	}
	if !runtime.running {
		t.Fatal("expected previous runtime to be restarted")
	}
	if runtime.startCalls != 2 {
		t.Fatalf("expected failed switch start and rollback start, got %d calls", runtime.startCalls)
	}

	restored, loaded, err := loadEndpointSelection(selectionPath)
	if err != nil {
		t.Fatalf("load restored selection: %v", err)
	}
	if !loaded || restored != previousSelection {
		t.Fatalf("expected previous selection %+v, got %+v", previousSelection, restored)
	}
}

func TestPrimaryEndpointSwitchRestartsPreviouslyRestartingRuntimeOnFailure(t *testing.T) {
	selectionPath := filepath.Join(t.TempDir(), "endpoint-selection.json")
	previousSelection := endpointSelectionState{Branch: "main", TenantID: "tenant-main", TimelineID: "timeline-main", Password: "secret-main"}
	if err := writeEndpointSelection(selectionPath, previousSelection); err != nil {
		t.Fatalf("write previous selection: %v", err)
	}
	if err := writeEndpointSelection(endpointAppliedSelectionPath(selectionPath), previousSelection); err != nil {
		t.Fatalf("write previous applied selection: %v", err)
	}
	runtime := &fakePrimaryEndpointRuntime{runtimeState: "restarting", startErrors: []error{errors.New("cannot start feature"), nil}}
	manager := newPrimaryEndpointManagerWithRuntime(runtime, primaryEndpointConnectionInfo{
		Host: "127.0.0.1", Port: 5432, Database: "postgres", User: "postgres", Password: "secret-main",
	}, selectionPath)
	if err := manager.SetBranchAttachment("feature-a", "tenant-feature", "timeline-feature"); err != nil {
		t.Fatalf("set feature attachment: %v", err)
	}

	if _, err := manager.SwitchToBranch("feature-a"); err == nil {
		t.Fatal("expected switch error")
	}
	if runtime.startCalls != 2 || !runtime.running {
		t.Fatalf("expected restarting primary rollback, start calls=%d running=%v", runtime.startCalls, runtime.running)
	}
}

func TestPrimaryEndpointSwitchStopsPossiblyStartedRuntimeBeforeRollback(t *testing.T) {
	selectionPath := filepath.Join(t.TempDir(), "endpoint-selection.json")
	previousSelection := endpointSelectionState{Branch: "main", Password: "secret-main"}
	if err := writeEndpointSelection(selectionPath, previousSelection); err != nil {
		t.Fatalf("write previous selection: %v", err)
	}
	if err := writeEndpointSelection(endpointAppliedSelectionPath(selectionPath), previousSelection); err != nil {
		t.Fatalf("write previous applied selection: %v", err)
	}

	runtime := &fakePrimaryEndpointRuntime{
		running:             true,
		startErrors:         []error{errors.New("start response timed out"), nil},
		startRunningOnError: true,
	}
	manager := newPrimaryEndpointManagerWithRuntime(runtime, primaryEndpointConnectionInfo{
		Host: "127.0.0.1", Port: 5432, Database: "postgres", User: "postgres",
	}, selectionPath)

	_, err := manager.SwitchToBranch("feature-a")
	if err == nil {
		t.Fatal("expected switch error")
	}
	if runtime.stopCalls != 2 {
		t.Fatalf("expected initial stop and rollback stop, got %d calls", runtime.stopCalls)
	}
	if runtime.startCalls != 2 || !runtime.running {
		t.Fatalf("expected previous runtime restart, got start calls=%d running=%t", runtime.startCalls, runtime.running)
	}
}

func TestPrimaryEndpointSwitchReportsRollbackRestartFailure(t *testing.T) {
	selectionPath := filepath.Join(t.TempDir(), "endpoint-selection.json")
	if err := writeEndpointSelection(selectionPath, endpointSelectionState{Branch: "main", Password: "secret-main"}); err != nil {
		t.Fatalf("write previous selection: %v", err)
	}

	switchErr := errors.New("cannot start feature")
	rollbackErr := errors.New("cannot restart main")
	runtime := &fakePrimaryEndpointRuntime{
		running:     true,
		startErrors: []error{switchErr, rollbackErr},
	}
	manager := newPrimaryEndpointManagerWithRuntime(runtime, primaryEndpointConnectionInfo{
		Host: "127.0.0.1", Port: 5432, Database: "postgres", User: "postgres",
	}, selectionPath)

	_, err := manager.SwitchToBranch("feature-a")
	if err == nil {
		t.Fatal("expected switch error")
	}
	if !strings.Contains(err.Error(), "cannot start feature") || !strings.Contains(err.Error(), "cannot restart main") {
		t.Fatalf("expected switch and rollback errors, got %v", err)
	}
	if !errors.Is(err, switchErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("expected switch and rollback error identities, got %v", err)
	}
}

func TestPrimaryEndpointSwitchDoesNotRestartWhenSelectionRestoreFails(t *testing.T) {
	dir := t.TempDir()
	selectionPath := filepath.Join(dir, "endpoint-selection.json")
	if err := writeEndpointSelection(selectionPath, endpointSelectionState{Branch: "main", Password: "secret-main"}); err != nil {
		t.Fatalf("write previous selection: %v", err)
	}

	runtime := &fakePrimaryEndpointRuntime{
		running:     true,
		startErrors: []error{errors.New("cannot start feature")},
		startHook: func() {
			if err := os.Remove(selectionPath); err != nil {
				t.Fatalf("remove selection: %v", err)
			}
			if err := os.Mkdir(selectionPath, 0o755); err != nil {
				t.Fatalf("replace selection with directory: %v", err)
			}
		},
	}
	manager := newPrimaryEndpointManagerWithRuntime(runtime, primaryEndpointConnectionInfo{
		Host: "127.0.0.1", Port: 5432, Database: "postgres", User: "postgres",
	}, selectionPath)

	_, err := manager.SwitchToBranch("feature-a")
	if err == nil || !strings.Contains(err.Error(), "restore previous endpoint selection failed") {
		t.Fatalf("expected selection restore error, got %v", err)
	}
	if runtime.stopCalls != 2 {
		t.Fatalf("expected possibly started runtime to be stopped, got %d stop calls", runtime.stopCalls)
	}
	if runtime.startCalls != 1 {
		t.Fatalf("expected no restart with invalid selection, got %d start calls", runtime.startCalls)
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

func TestPrimaryEndpointStartStopsRuntimeWhenAppliedStateIsNotVerified(t *testing.T) {
	selectionPath := filepath.Join(t.TempDir(), "endpoint-selection.json")
	if err := writeEndpointSelection(selectionPath, endpointSelectionState{Branch: "main", TenantID: "tenant", TimelineID: "timeline-old", Password: "secret"}); err != nil {
		t.Fatalf("write selection: %v", err)
	}
	if err := writeEndpointSelection(endpointAppliedSelectionPath(selectionPath), endpointSelectionState{Branch: "main", TenantID: "tenant", TimelineID: "timeline-old"}); err != nil {
		t.Fatalf("write applied selection: %v", err)
	}
	runtime := &fakePrimaryEndpointRuntime{}
	manager := newPrimaryEndpointManagerWithRuntime(runtime, primaryEndpointConnectionInfo{Host: "127.0.0.1", Port: 55433, Database: "postgres", User: "cloud_admin", Password: "secret"}, selectionPath)
	manager.startupTimeout = time.Millisecond
	if err := manager.SetBranchAttachment("main", "tenant", "timeline-new"); err != nil {
		t.Fatalf("set attachment: %v", err)
	}

	if _, err := manager.Start(); err == nil {
		t.Fatal("expected applied-state verification failure")
	}
	if runtime.stopCalls != 1 || runtime.running {
		t.Fatalf("expected ambiguous runtime to be stopped, stop calls=%d running=%v", runtime.stopCalls, runtime.running)
	}
	if !manager.routeBlocked {
		t.Fatal("expected failed start to require route reconciliation")
	}
}

func TestPrimaryEndpointStartRestartsRunningComputeForNewCredentials(t *testing.T) {
	selectionPath := filepath.Join(t.TempDir(), "endpoint-selection.json")
	previous := endpointSelectionState{Generation: "old-generation", Branch: "main", TenantID: "tenant", TimelineID: "timeline", Password: "old-secret"}
	if err := writeEndpointSelection(selectionPath, previous); err != nil {
		t.Fatalf("write selection: %v", err)
	}
	applied := previous
	applied.Password = ""
	if err := writeEndpointSelection(endpointAppliedSelectionPath(selectionPath), applied); err != nil {
		t.Fatalf("write applied selection: %v", err)
	}
	runtime := &fakePrimaryEndpointRuntime{running: true}
	runtime.startHook = func() {
		selection, _, _ := loadEndpointSelection(selectionPath)
		selection.Password = ""
		_ = writeEndpointSelection(endpointAppliedSelectionPath(selectionPath), selection)
	}
	manager := newPrimaryEndpointManagerWithRuntime(runtime, primaryEndpointConnectionInfo{Host: "127.0.0.1", Port: 55433, Database: "postgres", User: "cloud_admin", Password: "old-secret"}, selectionPath)
	if err := manager.SetBranchPassword("main", "new-secret"); err != nil {
		t.Fatalf("set password: %v", err)
	}

	state, err := manager.Start()
	if err != nil {
		t.Fatalf("restart primary: %v", err)
	}
	if runtime.stopCalls != 1 || runtime.startCalls != 1 {
		t.Fatalf("expected running compute restart, stops=%d starts=%d", runtime.stopCalls, runtime.startCalls)
	}
	if state.Password != "new-secret" || !state.Ready {
		t.Fatalf("expected newly applied credentials, got %#v", state)
	}
}

type fakePrimaryEndpointRuntime struct {
	running             bool
	ready               bool
	readySet            bool
	runtimeState        string
	runtimeMessage      string
	statusErr           error
	startErr            error
	startErrors         []error
	startRunningOnError bool
	startHook           func()
	stopErr             error
	startCalls          int
	stopCalls           int
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
	if f.startHook != nil {
		startHook := f.startHook
		f.startHook = nil
		startHook()
	}
	if len(f.startErrors) > 0 {
		err := f.startErrors[0]
		f.startErrors = f.startErrors[1:]
		if err != nil {
			if f.startRunningOnError {
				f.running = true
				f.startRunningOnError = false
			}
			return err
		}
		f.running = true
		return nil
	}
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
