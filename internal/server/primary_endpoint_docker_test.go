package server

import "testing"

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

type fakeDockerEngine struct {
	container dockerContainerSummary
	findErr   error
	startErr  error
	stopErr   error
}

func (f fakeDockerEngine) FindComposeContainer(_ string, _ string) (dockerContainerSummary, error) {
	if f.findErr != nil {
		return dockerContainerSummary{}, f.findErr
	}

	return f.container, nil
}

func (f fakeDockerEngine) StartContainer(_ string) error {
	return f.startErr
}

func (f fakeDockerEngine) StopContainer(_ string) error {
	return f.stopErr
}
