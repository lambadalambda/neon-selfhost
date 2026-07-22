package server

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
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

func TestResolvePrimaryBranchRoute(t *testing.T) {
	primary := &staticPrimaryEndpointRouteProvider{state: primaryEndpointRouteState{Applied: true, Connection: primaryEndpointState{
		Running: true, Ready: true, RuntimeState: "running", Branch: "main",
		Database: "postgres", User: "cloud_admin", Password: "primary-secret",
		TenantID: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", TimelineID: "BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
	}}}
	controller := newTestDockerBranchEndpointController(branch.NewStore(), t.TempDir(), 56000, 56000)
	controller.primaryRouteProvider = primary
	controller.primaryBackendHost = "compute"
	controller.primaryBackendPort = 55433

	route, primaryBacked, err := controller.resolvePrimaryBranchRoute("main", BranchAttachment{
		TenantID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", TimelineID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	})
	if err != nil {
		t.Fatalf("resolve primary route: %v", err)
	}
	if !primaryBacked || route.Address != "compute:55433" || route.Password != "primary-secret" {
		t.Fatalf("unexpected primary route: backed=%v route=%#v", primaryBacked, route)
	}

	_, aliasBacked, err := controller.resolvePrimaryBranchRoute("primary-alias", BranchAttachment{
		TenantID: primary.state.Connection.TenantID, TimelineID: primary.state.Connection.TimelineID,
	})
	if err != nil || !aliasBacked {
		t.Fatalf("expected attachment alias to remain primary-backed, backed=%v err=%v", aliasBacked, err)
	}

	_, childBacked, err := controller.resolvePrimaryBranchRoute("feature-a", BranchAttachment{
		TenantID: primary.state.Connection.TenantID, TimelineID: "cccccccccccccccccccccccccccccccc",
	})
	if err != nil || childBacked {
		t.Fatalf("expected distinct child to use lazy compute, backed=%v err=%v", childBacked, err)
	}
}

func TestResolvePrimaryBranchRouteFailsClosed(t *testing.T) {
	tests := []struct {
		name  string
		state primaryEndpointState
	}{
		{name: "stopped", state: primaryEndpointState{Branch: "main", TenantID: "tenant", TimelineID: "timeline"}},
		{name: "starting", state: primaryEndpointState{Running: true, Branch: "main", TenantID: "tenant", TimelineID: "timeline"}},
		{name: "attachment mismatch", state: primaryEndpointState{Running: true, Ready: true, Branch: "main", TenantID: "other", TimelineID: "timeline"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := newTestDockerBranchEndpointController(branch.NewStore(), t.TempDir(), 56000, 56000)
			controller.primaryRouteProvider = &staticPrimaryEndpointRouteProvider{state: primaryEndpointRouteState{Applied: true, Connection: tt.state}}
			controller.primaryBackendHost = "compute"
			controller.primaryBackendPort = 55433

			_, primaryBacked, err := controller.resolvePrimaryBranchRoute("main", BranchAttachment{TenantID: "tenant", TimelineID: "timeline"})
			if err == nil || !primaryBacked {
				t.Fatalf("expected primary reservation to fail closed, backed=%v err=%v", primaryBacked, err)
			}
		})
	}
}

func TestResolvePrimaryBranchRouteFailsClosedDuringTransition(t *testing.T) {
	controller := newTestDockerBranchEndpointController(branch.NewStore(), t.TempDir(), 56000, 56000)
	controller.primaryRouteProvider = &staticPrimaryEndpointRouteProvider{state: primaryEndpointRouteState{
		Applied:       true,
		Transitioning: true,
		Connection: primaryEndpointState{
			Running: true, Ready: true, Branch: "main", TenantID: "tenant", TimelineID: "timeline-main",
		},
	}}
	controller.primaryBackendHost = "compute"
	controller.primaryBackendPort = 55433

	_, primaryBacked, err := controller.resolvePrimaryBranchRoute("feature-a", BranchAttachment{TenantID: "tenant", TimelineID: "timeline-feature"})
	if err == nil || !primaryBacked {
		t.Fatalf("expected all routes to fail closed during transition, backed=%v err=%v", primaryBacked, err)
	}
}

func TestResolvePrimaryBranchRouteFailsClosedForReservedTarget(t *testing.T) {
	controller := newTestDockerBranchEndpointController(branch.NewStore(), t.TempDir(), 56000, 56000)
	controller.primaryRouteProvider = &staticPrimaryEndpointRouteProvider{state: primaryEndpointRouteState{
		Applied:          true,
		ReservedBranches: []string{"feature-a"},
		Connection: primaryEndpointState{
			Running: true, Ready: true, Branch: "main", TenantID: "tenant", TimelineID: "timeline-main",
		},
	}}
	controller.primaryBackendHost = "compute"
	controller.primaryBackendPort = 55433

	_, primaryBacked, err := controller.resolvePrimaryBranchRoute("feature-a", BranchAttachment{TenantID: "tenant", TimelineID: "timeline-feature"})
	if err == nil || !primaryBacked {
		t.Fatalf("expected reserved switch target to fail closed, backed=%v err=%v", primaryBacked, err)
	}
}

func TestResolvePrimaryBranchRouteFailsClosedWithoutAppliedSelection(t *testing.T) {
	controller := newTestDockerBranchEndpointController(branch.NewStore(), t.TempDir(), 56000, 56000)
	controller.primaryRouteProvider = &staticPrimaryEndpointRouteProvider{state: primaryEndpointRouteState{Connection: primaryEndpointState{
		Running: true, Ready: true, Branch: "main", TenantID: "tenant", TimelineID: "timeline",
	}}}
	controller.primaryBackendHost = "compute"
	controller.primaryBackendPort = 55433

	_, primaryBacked, err := controller.resolvePrimaryBranchRoute("feature-a", BranchAttachment{TenantID: "tenant", TimelineID: "timeline-feature"})
	if err == nil || !primaryBacked {
		t.Fatalf("expected unverified applied state to fail closed, backed=%v err=%v", primaryBacked, err)
	}
}

func TestPrimaryBackedConnectionUsesPrimaryCredentialsWithoutContainerInspect(t *testing.T) {
	store := branch.NewStore()
	if _, err := store.SetAttachment("main", "tenant", "timeline"); err != nil {
		t.Fatalf("set main attachment: %v", err)
	}
	if _, err := store.SetPassword("main", "branch-secret"); err != nil {
		t.Fatalf("set main password: %v", err)
	}
	port := freeTCPPort(t)
	if _, err := store.SetEndpoint("main", true, port); err != nil {
		t.Fatalf("publish main: %v", err)
	}
	engine := &trackingBranchEndpointEngine{containers: map[string]dockerContainerInspect{}}
	controller := newTestDockerBranchEndpointController(store, t.TempDir(), port, port)
	controller.engine = engine
	controller.primaryRouteProvider = &staticPrimaryEndpointRouteProvider{state: primaryEndpointRouteState{Applied: true, Connection: primaryEndpointState{
		Running: true, Ready: true, RuntimeState: "running", Branch: "main",
		Database: "postgres", User: "primary_user", Password: "primary-secret", TenantID: "tenant", TimelineID: "timeline",
	}}}
	controller.primaryBackendHost = "compute"
	controller.primaryBackendPort = 55433
	if err := controller.startListener("main", port); err != nil {
		t.Fatalf("start listener: %v", err)
	}
	defer controller.Close()

	state, err := controller.Connection("main")
	if err != nil {
		t.Fatalf("main connection: %v", err)
	}
	if state.Status != "running" || state.User != "primary_user" || state.Password != "primary-secret" {
		t.Fatalf("unexpected primary-backed connection: %#v", state)
	}
	if engine.inspectCount() != 0 {
		t.Fatalf("expected no branch container inspection, got %d", engine.inspectCount())
	}
}

func TestPrimaryBackedListenerProxiesWithoutBranchContainer(t *testing.T) {
	backend, backendPort := listenRandomPort(t)
	defer backend.Close()
	backendDone := make(chan error, 1)
	go func() {
		conn, err := backend.Accept()
		if err != nil {
			backendDone <- err
			return
		}
		defer conn.Close()
		request := make([]byte, 4)
		if _, err := io.ReadFull(conn, request); err != nil {
			backendDone <- err
			return
		}
		if string(request) != "ping" {
			backendDone <- errors.New("unexpected proxy request")
			return
		}
		_, err = conn.Write([]byte("pong"))
		backendDone <- err
	}()

	store := branch.NewStore()
	if _, err := store.SetAttachment("main", "tenant", "timeline"); err != nil {
		t.Fatalf("set main attachment: %v", err)
	}
	if _, err := store.SetPassword("main", "branch-secret"); err != nil {
		t.Fatalf("set main password: %v", err)
	}
	branchPort := freeTCPPort(t)
	if _, err := store.SetEndpoint("main", true, branchPort); err != nil {
		t.Fatalf("publish main: %v", err)
	}
	engine := &trackingBranchEndpointEngine{containers: map[string]dockerContainerInspect{}}
	controller := newTestDockerBranchEndpointController(store, t.TempDir(), branchPort, branchPort)
	controller.engine = engine
	controller.primaryRouteProvider = &staticPrimaryEndpointRouteProvider{state: primaryEndpointRouteState{Applied: true, Connection: primaryEndpointState{
		Running: true, Ready: true, Branch: "main", Database: "postgres", User: "cloud_admin", Password: "primary-secret", TenantID: "tenant", TimelineID: "timeline",
	}}}
	controller.primaryBackendHost = "127.0.0.1"
	controller.primaryBackendPort = backendPort
	if err := controller.startListener("main", branchPort); err != nil {
		t.Fatalf("start listener: %v", err)
	}
	defer controller.Close()

	client, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(branchPort)), time.Second)
	if err != nil {
		t.Fatalf("dial branch listener: %v", err)
	}
	defer client.Close()
	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("write proxy request: %v", err)
	}
	response := make([]byte, 4)
	if _, err := io.ReadFull(client, response); err != nil {
		t.Fatalf("read proxy response: %v", err)
	}
	if string(response) != "pong" {
		t.Fatalf("unexpected proxy response %q", response)
	}
	if err := <-backendDone; err != nil {
		t.Fatalf("backend proxy: %v", err)
	}
	if engine.inspectCount() != 0 || engine.createCount() != 0 {
		t.Fatalf("expected no branch container lifecycle, inspects=%d creates=%d", engine.inspectCount(), engine.createCount())
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

func TestPreparePrimarySwitchStopsAndRemovesTargetCompute(t *testing.T) {
	store := branch.NewStore()
	if _, err := store.CreateWithAttachmentAndPassword("feature-a", "main", "tenant", "timeline", "secret"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	engine := &trackingBranchEndpointEngine{containers: map[string]dockerContainerInspect{}}
	controller := newTestDockerBranchEndpointController(store, t.TempDir(), 56000, 56000)
	controller.engine = engine
	containerName := controller.containerName("feature-a")
	inspect := dockerContainerInspect{ID: "feature-container", Name: containerName}
	inspect.State.Running = true
	engine.setContainer(containerName, inspect)

	if err := controller.PreparePrimarySwitch("feature-a"); err != nil {
		t.Fatalf("prepare primary switch: %v", err)
	}
	stopCalls, removeCalls := engine.calls()
	if len(stopCalls) != 1 || stopCalls[0] != "feature-container" || len(removeCalls) != 1 || removeCalls[0] != "feature-container" {
		t.Fatalf("expected target compute stop/remove, stops=%v removes=%v", stopCalls, removeCalls)
	}
}

func TestPreparePrimarySwitchFailsWithoutStoppingActiveTarget(t *testing.T) {
	store := branch.NewStore()
	if _, err := store.CreateWithAttachmentAndPassword("feature-a", "main", "tenant", "timeline", "secret"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	engine := &trackingBranchEndpointEngine{containers: map[string]dockerContainerInspect{}}
	controller := newTestDockerBranchEndpointController(store, t.TempDir(), 56000, 56000)
	controller.engine = engine
	controller.drainTimeout = 20 * time.Millisecond
	controller.activeConns["feature-a"] = 1
	containerName := controller.containerName("feature-a")
	inspect := dockerContainerInspect{ID: "feature-container", Name: containerName}
	inspect.State.Running = true
	engine.setContainer(containerName, inspect)

	if err := controller.PreparePrimarySwitch("feature-a"); err == nil {
		t.Fatal("expected active target drain timeout")
	}
	stopCalls, removeCalls := engine.calls()
	if len(stopCalls) != 0 || len(removeCalls) != 0 {
		t.Fatalf("expected active target to remain running, stops=%v removes=%v", stopCalls, removeCalls)
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
		drainTimeout:         50 * time.Millisecond,
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

type staticPrimaryEndpointRouteProvider struct {
	state primaryEndpointRouteState
	err   error
}

func (p *staticPrimaryEndpointRouteProvider) AcquirePrimaryRouteState() (primaryEndpointRouteState, func(), error) {
	return p.state, func() {}, p.err
}

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
	mu           sync.Mutex
	containers   map[string]dockerContainerInspect
	stopCalls    []string
	removeCalls  []string
	stopCh       chan string
	inspectCalls int
	createCalls  int
}

func (e *trackingBranchEndpointEngine) InspectContainerByName(name string) (dockerContainerInspect, bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.inspectCalls++
	inspect, exists := e.containers[name]
	return inspect, exists, nil
}

func (e *trackingBranchEndpointEngine) inspectCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.inspectCalls
}

func (e *trackingBranchEndpointEngine) createCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.createCalls
}

func (e *trackingBranchEndpointEngine) CreateContainer(_ dockerCreateContainerRequest) (string, error) {
	e.mu.Lock()
	e.createCalls++
	e.mu.Unlock()
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
