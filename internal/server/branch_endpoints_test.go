package server

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"neon-selfhost/internal/branch"
)

func TestPublishDoesNotPersistEndpointWhenSelectionWriteFails(t *testing.T) {
	store := branch.NewStore()
	if _, err := store.Create("feature-a", "main"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	blockedPath := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blockedPath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write sentinel file: %v", err)
	}

	port := freeTCPPort(t)
	controller := newTestDockerBranchEndpointController(store, blockedPath, port, port)

	_, err := controller.Publish("feature-a", BranchAttachment{TenantID: "tenant-a", TimelineID: "timeline-a"}, "secret-1")
	if err == nil {
		t.Fatal("expected publish error when selection write fails")
	}

	active, err := store.GetActive("feature-a")
	if err != nil {
		t.Fatalf("get active branch: %v", err)
	}

	if active.EndpointPublished || active.EndpointPort != 0 {
		t.Fatalf("expected endpoint metadata to remain unpublished after failed publish, got published=%v port=%d", active.EndpointPublished, active.EndpointPort)
	}

	if len(controller.listeners) != 0 {
		t.Fatalf("expected failed publish to tear down listener, found %d listeners", len(controller.listeners))
	}
}

func TestRestorePublishedListenersContinuesAfterBindFailure(t *testing.T) {
	store := branch.NewStore()
	if _, err := store.Create("a-bad", "main"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	if _, err := store.Create("b-good", "main"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	blockedListener, blockedPort := listenRandomPort(t)
	defer blockedListener.Close()

	goodPort := freeTCPPort(t)
	if goodPort == blockedPort {
		goodPort = freeTCPPort(t)
	}

	if _, err := store.SetEndpoint("a-bad", true, blockedPort); err != nil {
		t.Fatalf("set blocked endpoint: %v", err)
	}
	if _, err := store.SetEndpoint("b-good", true, goodPort); err != nil {
		t.Fatalf("set good endpoint: %v", err)
	}

	controller := newTestDockerBranchEndpointController(store, t.TempDir(), blockedPort, goodPort)

	if err := controller.restorePublishedListeners(); err != nil {
		t.Fatalf("expected restore to continue on listener bind failure, got: %v", err)
	}

	badState, err := controller.Connection("a-bad")
	if err != nil {
		t.Fatalf("bad branch connection state: %v", err)
	}
	if badState.Status != "error" {
		t.Fatalf("expected blocked branch status %q, got %q", "error", badState.Status)
	}
	if strings.TrimSpace(badState.LastError) == "" {
		t.Fatal("expected blocked branch to expose restore error")
	}

	goodState, err := controller.Connection("b-good")
	if err != nil {
		t.Fatalf("good branch connection state: %v", err)
	}
	if goodState.Status != "stopped" {
		t.Fatalf("expected restored branch status %q, got %q", "stopped", goodState.Status)
	}
}

func TestSelectionPathUsesCollisionSafeBranchIdentifier(t *testing.T) {
	store := branch.NewStore()
	controller := newTestDockerBranchEndpointController(store, "/tmp/compute", 56000, 56049)

	first := controller.selectionPath("Preview/Foo")
	second := controller.selectionPath("preview-foo")
	if first == second {
		t.Fatalf("expected distinct selection paths for colliding slugs, got %q", first)
	}

	firstContainer := controller.containerName("Preview/Foo")
	secondContainer := controller.containerName("preview-foo")
	if firstContainer == secondContainer {
		t.Fatalf("expected distinct container names for colliding slugs, got %q", firstContainer)
	}
}

func newTestDockerBranchEndpointController(store *branch.Store, computeDataDir string, portStart int, portEnd int) *dockerBranchEndpointController {
	return &dockerBranchEndpointController{
		store:            store,
		engine:           fakeDockerBranchEndpointEngine{},
		composeProject:   "neon-selfhost",
		advertisedHost:   "127.0.0.1",
		bindHost:         "127.0.0.1",
		portStart:        portStart,
		portEnd:          portEnd,
		database:         "postgres",
		user:             "cloud_admin",
		computeImage:     "neon-selfhost/compute:dev",
		computeVolume:    "neon-selfhost_compute_state",
		computeNetwork:   "neon-selfhost_neon_internal",
		computeDataDir:   computeDataDir,
		pgVersion:        16,
		startupTimeout:   500 * time.Millisecond,
		listeners:        map[string]net.Listener{},
		activeConns:      map[string]int{},
		lastErrors:       map[string]string{},
		branchStartLocks: map[string]*sync.Mutex{},
	}
}

type fakeDockerBranchEndpointEngine struct{}

func (fakeDockerBranchEndpointEngine) InspectContainerByName(_ string) (dockerContainerInspect, bool, error) {
	return dockerContainerInspect{}, false, nil
}

func (fakeDockerBranchEndpointEngine) CreateContainer(_ dockerCreateContainerRequest) (string, error) {
	return "container-id", nil
}

func (fakeDockerBranchEndpointEngine) StartContainer(_ string) error {
	return nil
}

func (fakeDockerBranchEndpointEngine) StopContainer(_ string) error {
	return nil
}

func (fakeDockerBranchEndpointEngine) RemoveContainer(_ string, _ bool) error {
	return nil
}

func freeTCPPort(t *testing.T) int {
	t.Helper()

	listener, port := listenRandomPort(t)
	if err := listener.Close(); err != nil {
		t.Fatalf("close probe listener: %v", err)
	}

	return port
}

func listenRandomPort(t *testing.T) (net.Listener, int) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on random port: %v", err)
	}

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		t.Fatal("expected tcp listener address")
	}

	return listener, addr.Port
}
