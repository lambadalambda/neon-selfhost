package server

import (
	"errors"
	"io"
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

func TestCloseStopsListenersAndPublishedContainers(t *testing.T) {
	store := branch.NewStore()
	if _, err := store.CreateWithAttachmentAndPassword("feature-a", "main", "tenant-a", "timeline-a", "secret-1"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	port := freeTCPPort(t)
	if _, err := store.SetEndpoint("feature-a", true, port); err != nil {
		t.Fatalf("set endpoint: %v", err)
	}

	engine := &trackingBranchEndpointEngine{containers: map[string]dockerContainerInspect{}}
	controller := newTestDockerBranchEndpointController(store, t.TempDir(), port, port)
	controller.engine = engine

	if err := controller.startListener("feature-a", port); err != nil {
		t.Fatalf("start listener: %v", err)
	}

	containerName := controller.containerName("feature-a")
	inspect := dockerContainerInspect{ID: "container-feature-a", Name: containerName}
	inspect.State.Running = true
	inspect.State.Status = "running"
	engine.setContainer(containerName, inspect)

	if err := controller.Close(); err != nil {
		t.Fatalf("close controller: %v", err)
	}

	controller.mu.Lock()
	listenerCount := len(controller.listeners)
	controller.mu.Unlock()
	if listenerCount != 0 {
		t.Fatalf("expected no listeners after close, found %d", listenerCount)
	}

	stopCalls, removeCalls := engine.calls()
	if len(stopCalls) != 1 || stopCalls[0] != "container-feature-a" {
		t.Fatalf("expected stop call for published container, got %v", stopCalls)
	}

	if len(removeCalls) != 1 || removeCalls[0] != "container-feature-a" {
		t.Fatalf("expected remove call for published container, got %v", removeCalls)
	}
}

func TestIdleTimeoutStopsBranchComputeContainer(t *testing.T) {
	store := branch.NewStore()
	if _, err := store.CreateWithAttachmentAndPassword("feature-idle", "main", "tenant-idle", "timeline-idle", "secret-idle"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	port := freeTCPPort(t)
	if _, err := store.SetEndpoint("feature-idle", true, port); err != nil {
		t.Fatalf("set endpoint: %v", err)
	}

	stopCh := make(chan string, 1)
	engine := &trackingBranchEndpointEngine{containers: map[string]dockerContainerInspect{}, stopCh: stopCh}
	controller := newTestDockerBranchEndpointController(store, t.TempDir(), port, port)
	controller.engine = engine
	controller.idleTimeout = 20 * time.Millisecond

	containerName := controller.containerName("feature-idle")
	inspect := dockerContainerInspect{ID: "container-feature-idle", Name: containerName}
	inspect.State.Running = true
	inspect.State.Status = "running"
	engine.setContainer(containerName, inspect)

	if !controller.tryIncrementActive("feature-idle") {
		t.Fatal("expected initial active connection increment")
	}
	controller.decrementActive("feature-idle")

	select {
	case containerID := <-stopCh:
		if containerID != "container-feature-idle" {
			t.Fatalf("expected idle timeout stop for container-feature-idle, got %q", containerID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for idle container stop")
	}
}

func TestTryIncrementActiveRejectsWhenAtMaxConnections(t *testing.T) {
	store := branch.NewStore()
	controller := newTestDockerBranchEndpointController(store, t.TempDir(), 56000, 56049)
	controller.maxActiveConnections = 1
	controller.activeConns["main"] = 1

	if controller.tryIncrementActive("main") {
		t.Fatal("expected active connection increment to be rejected at limit")
	}
}

func TestProxyConnectionsReturnsAfterPeersClose(t *testing.T) {
	clientSide, clientProxy := net.Pipe()
	backendSide, backendProxy := net.Pipe()

	done := make(chan struct{})
	go func() {
		proxyConnections(clientProxy, backendProxy)
		close(done)
	}()

	go func() {
		_, _ = io.WriteString(clientSide, "hello")
		_ = clientSide.Close()
	}()

	buf := make([]byte, 5)
	if _, err := io.ReadFull(backendSide, buf); err != nil {
		t.Fatalf("read forwarded payload: %v", err)
	}
	if string(buf) != "hello" {
		t.Fatalf("expected payload %q, got %q", "hello", string(buf))
	}
	_ = backendSide.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("proxyConnections did not return after peers closed")
	}
}

func TestProxyConnectionsStopsAfterDrainTimeoutWithSilentPeer(t *testing.T) {
	clientSide, clientProxy := net.Pipe()
	backendSide, backendProxy := net.Pipe()
	defer backendSide.Close()

	done := make(chan struct{})
	go func() {
		proxyConnectionsWithDrainTimeout(clientProxy, backendProxy, 20*time.Millisecond)
		close(done)
	}()
	if err := clientSide.Close(); err != nil {
		t.Fatalf("close client: %v", err)
	}

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("proxy did not close a silent peer after drain timeout")
	}
	if _, err := backendSide.Read(make([]byte, 1)); err == nil {
		t.Fatal("expected backend peer to observe proxy closure")
	}
}

func TestProxyConnectionsStopsBothCopiesAfterCopyError(t *testing.T) {
	clientSide, clientProxy := net.Pipe()
	backendSide, backendProxy := net.Pipe()
	defer clientSide.Close()
	defer backendSide.Close()

	done := make(chan struct{})
	go func() {
		proxyConnectionsWithDrainTimeout(readErrorConn{Conn: clientProxy, err: errors.New("injected read failure")}, backendProxy, 20*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("proxy did not terminate both copies after an error")
	}
}

func TestProxyDrainTimeoutReleasesActiveConnection(t *testing.T) {
	controller := newTestDockerBranchEndpointController(branch.NewStore(), t.TempDir(), 56000, 56049)
	if !controller.tryIncrementActive("feature-a") {
		t.Fatal("expected active connection increment")
	}
	clientSide, clientProxy := net.Pipe()
	backendSide, backendProxy := net.Pipe()
	defer backendSide.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer controller.decrementActive("feature-a")
		proxyConnectionsWithDrainTimeout(clientProxy, backendProxy, 20*time.Millisecond)
	}()
	_ = clientSide.Close()
	select {
	case <-done:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("proxy did not release active connection after drain timeout")
	}
	controller.mu.Lock()
	active := controller.activeConns["feature-a"]
	controller.mu.Unlock()
	if active != 0 {
		t.Fatalf("expected active connection count 0, got %d", active)
	}
}

type readErrorConn struct {
	net.Conn
	err error
}

func (c readErrorConn) Read(_ []byte) (int, error) {
	return 0, c.err
}

func newTestDockerBranchEndpointController(store *branch.Store, computeDataDir string, portStart int, portEnd int) *dockerBranchEndpointController {
	return &dockerBranchEndpointController{
		store:                store,
		engine:               fakeDockerBranchEndpointEngine{},
		composeProject:       "neon-selfhost",
		advertisedHost:       "127.0.0.1",
		bindHost:             "127.0.0.1",
		portStart:            portStart,
		portEnd:              portEnd,
		database:             "postgres",
		user:                 "cloud_admin",
		computeImage:         "neon-selfhost/compute:dev",
		computeVolume:        "neon-selfhost_compute_state",
		computeNetwork:       "neon-selfhost_neon_internal",
		computeDataDir:       computeDataDir,
		pgVersion:            16,
		startupTimeout:       500 * time.Millisecond,
		idleTimeout:          50 * time.Millisecond,
		maxActiveConnections: 32,
		listeners:            map[string]net.Listener{},
		activeConns:          map[string]int{},
		idleTimers:           map[string]*time.Timer{},
		lastErrors:           map[string]string{},
		branchStartLocks:     map[string]*sync.Mutex{},
	}
}

func TestReconcileBranchComputeImageRemovesStoppedStaleContainer(t *testing.T) {
	engine := &trackingBranchEndpointEngine{containers: map[string]dockerContainerInspect{}}
	controller := newTestDockerBranchEndpointController(branch.NewStore(), t.TempDir(), 56000, 56000)
	controller.engine = engine
	inspect := dockerContainerInspect{ID: "stale-container"}
	inspect.Config.Image = "neon-selfhost/compute:old"

	exists, err := controller.reconcileComputeImage(inspect, true)
	if err != nil {
		t.Fatalf("reconcile stale image: %v", err)
	}
	if exists {
		t.Fatal("expected stopped stale container to be removed")
	}
	_, removeCalls := engine.calls()
	if len(removeCalls) != 1 || removeCalls[0] != "stale-container" {
		t.Fatalf("expected stale container removal, got %#v", removeCalls)
	}
}

func TestReconcileBranchComputeImageRejectsRunningStaleContainer(t *testing.T) {
	engine := &trackingBranchEndpointEngine{containers: map[string]dockerContainerInspect{}}
	controller := newTestDockerBranchEndpointController(branch.NewStore(), t.TempDir(), 56000, 56000)
	controller.engine = engine
	inspect := dockerContainerInspect{ID: "running-stale-container"}
	inspect.Config.Image = "neon-selfhost/compute:old"
	inspect.State.Running = true

	_, err := controller.reconcileComputeImage(inspect, true)
	if err == nil {
		t.Fatal("expected running stale compute to fail closed")
	}
	stopCalls, removeCalls := engine.calls()
	if len(stopCalls) != 0 || len(removeCalls) != 0 {
		t.Fatalf("expected active stale compute to remain untouched, stops=%#v removes=%#v", stopCalls, removeCalls)
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

type trackingBranchEndpointEngine struct {
	mu          sync.Mutex
	containers  map[string]dockerContainerInspect
	stopCalls   []string
	removeCalls []string
	stopCh      chan string
}

func (e *trackingBranchEndpointEngine) InspectContainerByName(name string) (dockerContainerInspect, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	inspect, exists := e.containers[name]
	return inspect, exists, nil
}

func (e *trackingBranchEndpointEngine) CreateContainer(_ dockerCreateContainerRequest) (string, error) {
	return "", nil
}

func (e *trackingBranchEndpointEngine) StartContainer(_ string) error {
	return nil
}

func (e *trackingBranchEndpointEngine) StopContainer(containerID string) error {
	e.mu.Lock()
	e.stopCalls = append(e.stopCalls, containerID)
	stopCh := e.stopCh
	e.mu.Unlock()
	if stopCh != nil {
		select {
		case stopCh <- containerID:
		default:
		}
	}
	return nil
}

func (e *trackingBranchEndpointEngine) RemoveContainer(containerID string, _ bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.removeCalls = append(e.removeCalls, containerID)
	return nil
}

func (e *trackingBranchEndpointEngine) setContainer(name string, inspect dockerContainerInspect) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.containers[name] = inspect
}

func (e *trackingBranchEndpointEngine) calls() ([]string, []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.stopCalls...), append([]string(nil), e.removeCalls...)
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
