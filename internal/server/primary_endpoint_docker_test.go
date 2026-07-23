package server

import (
	"strings"
	"testing"
)

func TestDockerPrimaryEndpointRuntimeStatusHealthyContainer(t *testing.T) {
	runtime := &dockerPrimaryEndpointRuntime{
		engine: fakeDockerEngine{container: dockerContainerSummary{
			ID:     "container-1",
			State:  "running",
			Status: "Up 12 seconds (healthy)",
		}},
		project: "neon-selfhost",
		service: "compute",
	}

	status, err := runtime.Status()
	if err != nil {
		t.Fatalf("runtime status: %v", err)
	}

	if !status.Running {
		t.Fatal("expected running=true")
	}

	if !status.Ready {
		t.Fatal("expected ready=true")
	}

	if status.State != "running" {
		t.Fatalf("expected state %q, got %q", "running", status.State)
	}

	if status.Message != "" {
		t.Fatalf("expected empty message for healthy runtime, got %q", status.Message)
	}
}

func TestDockerPrimaryEndpointRuntimeStatusStartingHealthCheck(t *testing.T) {
	runtime := &dockerPrimaryEndpointRuntime{
		engine: fakeDockerEngine{container: dockerContainerSummary{
			ID:     "container-1",
			State:  "running",
			Status: "Up 3 seconds (health: starting)",
		}},
		project: "neon-selfhost",
		service: "compute",
	}

	status, err := runtime.Status()
	if err != nil {
		t.Fatalf("runtime status: %v", err)
	}

	if !status.Running {
		t.Fatal("expected running=true")
	}

	if status.Ready {
		t.Fatal("expected ready=false while health checks are starting")
	}

	if status.Message != "container health check is starting" {
		t.Fatalf("expected startup message %q, got %q", "container health check is starting", status.Message)
	}
}

func TestDockerPrimaryEndpointRuntimeStatusRequiresExplicitHealth(t *testing.T) {
	runtime := &dockerPrimaryEndpointRuntime{
		engine:  fakeDockerEngine{container: dockerContainerSummary{ID: "container-1", State: "running", Status: "Up 12 seconds"}},
		project: "neon-selfhost",
		service: "compute",
	}

	status, err := runtime.Status()
	if err != nil {
		t.Fatalf("runtime status: %v", err)
	}
	if status.Ready || status.State != "starting" {
		t.Fatalf("expected missing health metadata to remain unready, got %#v", status)
	}
}

func TestDockerPrimaryEndpointRuntimeStatusInspectsHealthWhenSummaryOmitsIt(t *testing.T) {
	inspect := dockerContainerInspect{ID: "container-1"}
	inspect.State.Status = "running"
	inspect.State.Running = true
	inspect.State.Health.Status = "healthy"
	runtime := &dockerPrimaryEndpointRuntime{
		engine: fakeDockerEngine{
			container: dockerContainerSummary{ID: "container-1", State: "running", Status: "Up 12 seconds"},
			inspect:   inspect,
		},
		project: "neon-selfhost",
		service: "compute",
	}

	status, err := runtime.Status()
	if err != nil {
		t.Fatalf("runtime status: %v", err)
	}
	if !status.Ready || status.State != "running" {
		t.Fatalf("expected inspected healthy container to be ready, got %#v", status)
	}
}

func TestDockerPrimaryEndpointRuntimeStatusRejectsStaleHealthFromStoppedInspect(t *testing.T) {
	inspect := dockerContainerInspect{ID: "container-1"}
	inspect.State.Status = "exited"
	inspect.State.Health.Status = "healthy"
	runtime := &dockerPrimaryEndpointRuntime{
		engine: fakeDockerEngine{
			container: dockerContainerSummary{ID: "container-1", State: "running", Status: "Up 12 seconds"},
			inspect:   inspect,
		},
		project: "neon-selfhost",
		service: "compute",
	}

	status, err := runtime.Status()
	if err != nil {
		t.Fatalf("runtime status: %v", err)
	}
	if status.Running || status.Ready || status.State != "exited" {
		t.Fatalf("expected stopped inspect state to override stale summary health, got %#v", status)
	}
}

func TestDockerPrimaryEndpointRuntimeStatusStoppedContainer(t *testing.T) {
	runtime := &dockerPrimaryEndpointRuntime{
		engine: fakeDockerEngine{container: dockerContainerSummary{
			ID:     "container-1",
			State:  "exited",
			Status: "Exited (1) 2 seconds ago",
		}},
		project: "neon-selfhost",
		service: "compute",
	}

	status, err := runtime.Status()
	if err != nil {
		t.Fatalf("runtime status: %v", err)
	}

	if status.Running {
		t.Fatal("expected running=false")
	}

	if status.Ready {
		t.Fatal("expected ready=false")
	}

	if status.State != "exited" {
		t.Fatalf("expected state %q, got %q", "exited", status.State)
	}

	if status.Message != "Exited (1) 2 seconds ago" {
		t.Fatalf("expected stop message %q, got %q", "Exited (1) 2 seconds ago", status.Message)
	}
}

func TestDockerPrimaryEndpointRuntimeStatusUnhealthyContainer(t *testing.T) {
	runtime := &dockerPrimaryEndpointRuntime{
		engine: fakeDockerEngine{container: dockerContainerSummary{
			ID:     "container-1",
			State:  "running",
			Status: "Up 10 seconds (health: unhealthy)",
		}},
		project: "neon-selfhost",
		service: "compute",
	}

	status, err := runtime.Status()
	if err != nil {
		t.Fatalf("runtime status: %v", err)
	}

	if !status.Running {
		t.Fatal("expected running=true")
	}

	if status.Ready {
		t.Fatal("expected ready=false for unhealthy runtime")
	}

	if status.State != "unhealthy" {
		t.Fatalf("expected state %q, got %q", "unhealthy", status.State)
	}

	if status.Message != "Up 10 seconds (health: unhealthy)" {
		t.Fatalf("expected unhealthy message %q, got %q", "Up 10 seconds (health: unhealthy)", status.Message)
	}
}

func TestDockerPrimaryEndpointRuntimeStopsRestartingContainer(t *testing.T) {
	engine := &recordingDockerEngine{container: dockerContainerSummary{ID: "container-1", State: "restarting"}}
	runtime := &dockerPrimaryEndpointRuntime{engine: engine, project: "neon-selfhost", service: "compute"}

	if err := runtime.Stop(); err != nil {
		t.Fatalf("stop restarting runtime: %v", err)
	}
	if engine.stoppedID != "container-1" {
		t.Fatalf("expected restarting container to be stopped, got %q", engine.stoppedID)
	}
}

type fakeDockerEngine struct {
	container dockerContainerSummary
	inspect   dockerContainerInspect
	findErr   error
	startErr  error
	stopErr   error
}

type recordingDockerEngine struct {
	container dockerContainerSummary
	stoppedID string
}

func (e *recordingDockerEngine) FindComposeContainer(_ string, _ string) (dockerContainerSummary, error) {
	return e.container, nil
}

func (e *recordingDockerEngine) InspectContainerByName(_ string) (dockerContainerInspect, bool, error) {
	return dockerContainerInspect{}, false, nil
}

func (e *recordingDockerEngine) StartContainer(_ string) error {
	return nil
}

func (e *recordingDockerEngine) StopContainer(containerID string) error {
	e.stoppedID = containerID
	return nil
}

func (f fakeDockerEngine) FindComposeContainer(_ string, _ string) (dockerContainerSummary, error) {
	if f.findErr != nil {
		return dockerContainerSummary{}, f.findErr
	}

	return f.container, nil
}

func (f fakeDockerEngine) InspectContainerByName(_ string) (dockerContainerInspect, bool, error) {
	return f.inspect, strings.TrimSpace(f.inspect.ID) != "", nil
}

func (f fakeDockerEngine) StartContainer(_ string) error {
	return f.startErr
}

func (f fakeDockerEngine) StopContainer(_ string) error {
	return f.stopErr
}
