package server

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"neon-selfhost/internal/branch"
)

const (
	defaultBranchEndpointHost      = "127.0.0.1"
	defaultBranchEndpointBindHost  = "127.0.0.1"
	defaultBranchEndpointPortStart = 56000
	defaultBranchEndpointPortEnd   = 56049
	defaultBranchEndpointImage     = "neon-selfhost/compute:dev"
	defaultBranchComposeProject    = "neon-selfhost"
	defaultBranchEndpointTimeout   = 60 * time.Second
	branchComputePort              = 55433
)

type BranchEndpointController interface {
	Publish(branchName string, attachment BranchAttachment, password string) (branchEndpointState, error)
	Unpublish(branchName string) (branchEndpointState, error)
	Connection(branchName string) (branchEndpointState, error)
	List() ([]branchEndpointState, error)
	Refresh(branchName string, attachment BranchAttachment, password string) error
}

type branchEndpointState struct {
	Branch            string
	Published         bool
	Status            string
	Host              string
	Port              int
	Database          string
	User              string
	Password          string
	TenantID          string
	TimelineID        string
	ActiveConnections int
	LastError         string
}

type DockerBranchEndpointOptions struct {
	Store *branch.Store

	SocketPath     string
	ComposeProject string

	AdvertisedHost string
	BindHost       string
	PortStart      int
	PortEnd        int

	Database string
	User     string

	ComputeImage   string
	ComputeVolume  string
	ComputeNetwork string
	ComputeDataDir string
	PGVersion      int

	StartupTimeout time.Duration
}

type noopBranchEndpointController struct {
	host     string
	database string
	user     string
}

func NewNoopBranchEndpointController(host string, database string, user string) BranchEndpointController {
	host = strings.TrimSpace(host)
	if host == "" {
		host = defaultBranchEndpointHost
	}

	database = strings.TrimSpace(database)
	if database == "" {
		database = defaultPrimaryEndpointDatabase
	}

	user = strings.TrimSpace(user)
	if user == "" {
		user = defaultPrimaryEndpointUser
	}

	return noopBranchEndpointController{host: host, database: database, user: user}
}

func (n noopBranchEndpointController) Publish(_ string, _ BranchAttachment, _ string) (branchEndpointState, error) {
	return branchEndpointState{}, fmt.Errorf("%w: branch endpoint publishing requires docker mode", ErrPrimaryEndpointUnavailable)
}

func (n noopBranchEndpointController) Unpublish(branchName string) (branchEndpointState, error) {
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return branchEndpointState{}, branch.ErrNotFound
	}

	return branchEndpointState{Branch: branchName, Published: false, Status: "unpublished", Host: n.host, Database: n.database, User: n.user}, nil
}

func (n noopBranchEndpointController) Connection(branchName string) (branchEndpointState, error) {
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return branchEndpointState{}, branch.ErrNotFound
	}

	return branchEndpointState{Branch: branchName, Published: false, Status: "unpublished", Host: n.host, Database: n.database, User: n.user}, nil
}

func (n noopBranchEndpointController) List() ([]branchEndpointState, error) {
	return []branchEndpointState{}, nil
}

func (n noopBranchEndpointController) Refresh(_ string, _ BranchAttachment, _ string) error {
	return nil
}

type dockerBranchEndpointEngine interface {
	InspectContainerByName(name string) (dockerContainerInspect, bool, error)
	CreateContainer(req dockerCreateContainerRequest) (string, error)
	StartContainer(containerID string) error
	StopContainer(containerID string) error
	RemoveContainer(containerID string, force bool) error
}

type dockerBranchEndpointController struct {
	store  *branch.Store
	engine dockerBranchEndpointEngine

	composeProject string
	advertisedHost string
	bindHost       string
	portStart      int
	portEnd        int
	database       string
	user           string
	computeImage   string
	computeVolume  string
	computeNetwork string
	computeDataDir string
	pgVersion      int
	startupTimeout time.Duration

	mu               sync.Mutex
	listeners        map[string]net.Listener
	activeConns      map[string]int
	lastErrors       map[string]string
	branchStartLocks map[string]*sync.Mutex
}

func NewDockerBranchEndpointController(opts DockerBranchEndpointOptions) (BranchEndpointController, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("%w: branch store is required", ErrPrimaryEndpointUnavailable)
	}

	composeProject := strings.TrimSpace(opts.ComposeProject)
	if composeProject == "" {
		composeProject = defaultBranchComposeProject
	}

	advertisedHost := strings.TrimSpace(opts.AdvertisedHost)
	if advertisedHost == "" {
		advertisedHost = defaultBranchEndpointHost
	}

	bindHost := strings.TrimSpace(opts.BindHost)
	if bindHost == "" {
		bindHost = defaultBranchEndpointBindHost
	}

	portStart := opts.PortStart
	if portStart == 0 {
		portStart = defaultBranchEndpointPortStart
	}
	portEnd := opts.PortEnd
	if portEnd == 0 {
		portEnd = defaultBranchEndpointPortEnd
	}
	if portStart < 1 || portEnd < portStart || portEnd > 65535 {
		return nil, fmt.Errorf("%w: invalid branch endpoint port range %d-%d", ErrPrimaryEndpointUnavailable, portStart, portEnd)
	}

	database := strings.TrimSpace(opts.Database)
	if database == "" {
		database = defaultPrimaryEndpointDatabase
	}

	user := strings.TrimSpace(opts.User)
	if user == "" {
		user = defaultPrimaryEndpointUser
	}

	computeImage := strings.TrimSpace(opts.ComputeImage)
	if computeImage == "" {
		computeImage = defaultBranchEndpointImage
	}

	computeVolume := strings.TrimSpace(opts.ComputeVolume)
	if computeVolume == "" {
		computeVolume = composeProject + "_compute_state"
	}

	computeNetwork := strings.TrimSpace(opts.ComputeNetwork)
	if computeNetwork == "" {
		computeNetwork = composeProject + "_neon_internal"
	}

	computeDataDir := strings.TrimSpace(opts.ComputeDataDir)
	if computeDataDir == "" {
		return nil, fmt.Errorf("%w: compute data dir is required", ErrPrimaryEndpointUnavailable)
	}

	pgVersion := opts.PGVersion
	if pgVersion <= 0 {
		pgVersion = defaultPageserverPGVersion
	}

	startupTimeout := opts.StartupTimeout
	if startupTimeout <= 0 {
		startupTimeout = defaultBranchEndpointTimeout
	}

	engineClient, err := newDockerEngineClient(opts.SocketPath)
	if err != nil {
		return nil, err
	}

	controller := &dockerBranchEndpointController{
		store:            opts.Store,
		engine:           engineClient,
		composeProject:   composeProject,
		advertisedHost:   advertisedHost,
		bindHost:         bindHost,
		portStart:        portStart,
		portEnd:          portEnd,
		database:         database,
		user:             user,
		computeImage:     computeImage,
		computeVolume:    computeVolume,
		computeNetwork:   computeNetwork,
		computeDataDir:   computeDataDir,
		pgVersion:        pgVersion,
		startupTimeout:   startupTimeout,
		listeners:        map[string]net.Listener{},
		activeConns:      map[string]int{},
		lastErrors:       map[string]string{},
		branchStartLocks: map[string]*sync.Mutex{},
	}

	if err := controller.restorePublishedListeners(); err != nil {
		return nil, err
	}

	return controller, nil
}

func (c *dockerBranchEndpointController) restorePublishedListeners() error {
	for _, b := range c.store.ListActive() {
		if !b.EndpointPublished || b.EndpointPort <= 0 {
			continue
		}

		if err := c.startListener(b.Name, b.EndpointPort); err != nil {
			c.recordError(b.Name, fmt.Errorf("%w: restore branch endpoint listener for %q: %v", ErrPrimaryEndpointUnavailable, b.Name, err))
			continue
		}

		c.clearError(b.Name)
	}

	return nil
}

func (c *dockerBranchEndpointController) Publish(branchName string, attachment BranchAttachment, password string) (branchEndpointState, error) {
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return branchEndpointState{}, branch.ErrNotFound
	}

	attachment.TenantID = strings.TrimSpace(attachment.TenantID)
	attachment.TimelineID = strings.TrimSpace(attachment.TimelineID)
	password = strings.TrimSpace(password)
	if attachment.TenantID == "" || attachment.TimelineID == "" || password == "" {
		return branchEndpointState{}, fmt.Errorf("%w: branch endpoint requires tenant, timeline, and password", ErrPrimaryEndpointUnavailable)
	}

	b, err := c.store.GetActive(branchName)
	if err != nil {
		return branchEndpointState{}, err
	}

	port := b.EndpointPort
	newPublish := !b.EndpointPublished || port <= 0
	if newPublish {
		allocatedPort, allocErr := c.allocatePortAndStartListener(branchName)
		if allocErr != nil {
			return branchEndpointState{}, allocErr
		}
		port = allocatedPort
	} else if err := c.startListener(branchName, port); err != nil {
		return branchEndpointState{}, fmt.Errorf("%w: start branch endpoint listener: %v", ErrPrimaryEndpointUnavailable, err)
	}

	if err := c.writeSelection(branchName, attachment, password); err != nil {
		if newPublish {
			c.stopListener(branchName)
		}
		return branchEndpointState{}, err
	}

	if newPublish {
		if _, setErr := c.store.SetEndpoint(branchName, true, port); setErr != nil {
			c.stopListener(branchName)
			return branchEndpointState{}, setErr
		}
	}

	return c.Connection(branchName)
}

func (c *dockerBranchEndpointController) Unpublish(branchName string) (branchEndpointState, error) {
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return branchEndpointState{}, branch.ErrNotFound
	}

	b, err := c.store.GetActive(branchName)
	if err != nil {
		return branchEndpointState{}, err
	}

	if b.EndpointPublished {
		c.stopListener(branchName)

		containerName := c.containerName(branchName)
		inspect, exists, inspectErr := c.engine.InspectContainerByName(containerName)
		if inspectErr != nil {
			return branchEndpointState{}, inspectErr
		}
		if exists {
			_ = c.engine.StopContainer(inspect.ID)
			if removeErr := c.engine.RemoveContainer(inspect.ID, true); removeErr != nil {
				return branchEndpointState{}, removeErr
			}
		}

		if _, setErr := c.store.SetEndpoint(branchName, false, 0); setErr != nil {
			return branchEndpointState{}, setErr
		}
	}

	return c.Connection(branchName)
}

func (c *dockerBranchEndpointController) Refresh(branchName string, attachment BranchAttachment, password string) error {
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return branch.ErrNotFound
	}

	b, err := c.store.GetActive(branchName)
	if err != nil {
		return err
	}

	if !b.EndpointPublished || b.EndpointPort <= 0 {
		return nil
	}

	if err := c.writeSelection(branchName, attachment, password); err != nil {
		return err
	}

	containerName := c.containerName(branchName)
	inspect, exists, err := c.engine.InspectContainerByName(containerName)
	if err != nil {
		return err
	}
	if !exists || !inspect.State.Running {
		return nil
	}

	if err := c.engine.StopContainer(inspect.ID); err != nil {
		return err
	}

	if err := c.engine.StartContainer(inspect.ID); err != nil {
		return err
	}

	_, err = c.waitForBackend(containerName)
	return err
}

func (c *dockerBranchEndpointController) Connection(branchName string) (branchEndpointState, error) {
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return branchEndpointState{}, branch.ErrNotFound
	}

	b, err := c.store.GetActive(branchName)
	if err != nil {
		return branchEndpointState{}, err
	}

	state := branchEndpointState{
		Branch:     b.Name,
		Published:  b.EndpointPublished,
		Status:     "unpublished",
		Host:       c.advertisedHost,
		Port:       b.EndpointPort,
		Database:   c.database,
		User:       c.user,
		Password:   b.Password,
		TenantID:   b.TenantID,
		TimelineID: b.TimelineID,
	}

	c.mu.Lock()
	state.ActiveConnections = c.activeConns[branchName]
	state.LastError = c.lastErrors[branchName]
	_, listenerExists := c.listeners[branchName]
	c.mu.Unlock()

	if !b.EndpointPublished || b.EndpointPort <= 0 {
		return state, nil
	}

	if !listenerExists {
		state.Status = "error"
		if strings.TrimSpace(state.LastError) == "" {
			state.LastError = "listener unavailable"
		}
		return state, nil
	}

	state.Status = "stopped"
	inspect, exists, inspectErr := c.engine.InspectContainerByName(c.containerName(branchName))
	if inspectErr != nil {
		state.Status = "error"
		state.LastError = inspectErr.Error()
		return state, nil
	}

	if exists {
		containerStatus := strings.TrimSpace(inspect.State.Status)
		if containerStatus != "" {
			state.Status = containerStatus
		}
		if inspect.State.Running {
			state.Status = "running"
		}
	}

	if state.ActiveConnections > 0 && state.Status == "running" {
		state.Status = "active"
	}

	return state, nil
}

func (c *dockerBranchEndpointController) List() ([]branchEndpointState, error) {
	branches := c.store.ListActive()
	states := make([]branchEndpointState, 0, len(branches))
	for _, b := range branches {
		if !b.EndpointPublished || b.EndpointPort <= 0 {
			continue
		}

		state, err := c.Connection(b.Name)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}

	sort.Slice(states, func(i int, j int) bool {
		return states[i].Branch < states[j].Branch
	})

	return states, nil
}

func (c *dockerBranchEndpointController) allocatePortAndStartListener(branchName string) (int, error) {
	usedPorts := map[int]bool{}
	for _, b := range c.store.ListActive() {
		if b.EndpointPublished && b.EndpointPort > 0 {
			usedPorts[b.EndpointPort] = true
		}
	}

	var lastBindErr error

	for port := c.portStart; port <= c.portEnd; port++ {
		if usedPorts[port] {
			continue
		}

		if err := c.startListener(branchName, port); err != nil {
			lastBindErr = err
			continue
		}

		return port, nil
	}

	if lastBindErr != nil {
		return 0, fmt.Errorf("%w: no available branch endpoint port in range %d-%d: %v", ErrPrimaryEndpointUnavailable, c.portStart, c.portEnd, lastBindErr)
	}

	return 0, fmt.Errorf("%w: branch endpoint port range exhausted", ErrPrimaryEndpointUnavailable)
}

func (c *dockerBranchEndpointController) startListener(branchName string, port int) error {
	c.mu.Lock()
	if existing, exists := c.listeners[branchName]; exists {
		if addr, ok := existing.Addr().(*net.TCPAddr); ok && addr.Port == port {
			c.mu.Unlock()
			return nil
		}

		delete(c.listeners, branchName)
		c.mu.Unlock()
		_ = existing.Close()
	} else {
		c.mu.Unlock()
	}

	listener, err := net.Listen("tcp", net.JoinHostPort(c.bindHost, strconv.Itoa(port)))
	if err != nil {
		return err
	}

	c.mu.Lock()
	if existing, exists := c.listeners[branchName]; exists {
		c.mu.Unlock()
		_ = listener.Close()
		_ = existing
		return nil
	}
	c.listeners[branchName] = listener
	c.mu.Unlock()

	go c.acceptLoop(branchName, listener)
	return nil
}

func (c *dockerBranchEndpointController) stopListener(branchName string) {
	c.mu.Lock()
	listener, exists := c.listeners[branchName]
	if exists {
		delete(c.listeners, branchName)
	}
	delete(c.activeConns, branchName)
	delete(c.lastErrors, branchName)
	c.mu.Unlock()

	if exists {
		_ = listener.Close()
	}
}

func (c *dockerBranchEndpointController) acceptLoop(branchName string, listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			c.recordError(branchName, fmt.Errorf("accept branch endpoint connection: %w", err))
			continue
		}

		go c.handleClientConnection(branchName, conn)
	}
}

func (c *dockerBranchEndpointController) handleClientConnection(branchName string, clientConn net.Conn) {
	c.incrementActive(branchName)
	defer c.decrementActive(branchName)
	defer clientConn.Close()

	b, err := c.store.GetActive(branchName)
	if err != nil {
		c.recordError(branchName, err)
		return
	}

	if !b.EndpointPublished || b.EndpointPort <= 0 {
		c.recordError(branchName, fmt.Errorf("branch endpoint is not published"))
		return
	}

	if strings.TrimSpace(b.TenantID) == "" || strings.TrimSpace(b.TimelineID) == "" || strings.TrimSpace(b.Password) == "" {
		c.recordError(branchName, fmt.Errorf("branch endpoint credentials are incomplete"))
		return
	}

	backendAddress, err := c.ensureComputeRunning(branchName, BranchAttachment{TenantID: b.TenantID, TimelineID: b.TimelineID}, b.Password)
	if err != nil {
		c.recordError(branchName, err)
		return
	}

	backendConn, err := net.DialTimeout("tcp", backendAddress, 10*time.Second)
	if err != nil {
		c.recordError(branchName, fmt.Errorf("dial branch compute backend: %w", err))
		return
	}
	defer backendConn.Close()

	c.clearError(branchName)
	proxyConnections(clientConn, backendConn)
}

func proxyConnections(clientConn net.Conn, backendConn net.Conn) {
	errCh := make(chan error, 2)

	go func() {
		_, err := io.Copy(backendConn, clientConn)
		errCh <- err
	}()

	go func() {
		_, err := io.Copy(clientConn, backendConn)
		errCh <- err
	}()

	<-errCh
}

func (c *dockerBranchEndpointController) ensureComputeRunning(branchName string, attachment BranchAttachment, password string) (string, error) {
	lock := c.branchLock(branchName)
	lock.Lock()
	defer lock.Unlock()

	if err := c.writeSelection(branchName, attachment, password); err != nil {
		return "", err
	}

	containerName := c.containerName(branchName)
	inspect, exists, err := c.engine.InspectContainerByName(containerName)
	if err != nil {
		return "", err
	}

	if !exists {
		createReq := dockerCreateContainerRequest{
			Name:       containerName,
			Image:      c.computeImage,
			Entrypoint: []string{"/shell/compute.sh"},
			Env: []string{
				fmt.Sprintf("PG_VERSION=%d", c.pgVersion),
				fmt.Sprintf("ENDPOINT_SELECTION_FILE=%s", c.selectionPath(branchName)),
				"TENANT_ID=",
				"TIMELINE_ID=",
			},
			Labels: map[string]string{
				"neon.selfhost.endpoint":     "branch",
				"neon.selfhost.branch":       branchName,
				"com.docker.compose.project": c.composeProject,
			},
			HostConfig: dockerCreateHostConfig{
				NetworkMode: c.computeNetwork,
				Mounts: []dockerMountConfig{{
					Type:   "volume",
					Source: c.computeVolume,
					Target: "/var/lib/neon/compute",
				}},
			},
		}

		containerID, createErr := c.engine.CreateContainer(createReq)
		if createErr != nil {
			return "", createErr
		}

		if startErr := c.engine.StartContainer(containerID); startErr != nil {
			return "", startErr
		}
	} else if !inspect.State.Running {
		if startErr := c.engine.StartContainer(inspect.ID); startErr != nil {
			return "", startErr
		}
	}

	return c.waitForBackend(containerName)
}

func (c *dockerBranchEndpointController) waitForBackend(containerName string) (string, error) {
	backendAddress := net.JoinHostPort(containerName, strconv.Itoa(branchComputePort))
	deadline := time.Now().Add(c.startupTimeout)

	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("%w: branch compute startup timed out", ErrPrimaryEndpointUnavailable)
		}

		conn, err := net.DialTimeout("tcp", backendAddress, 1*time.Second)
		if err == nil {
			_ = conn.Close()
			return backendAddress, nil
		}

		time.Sleep(500 * time.Millisecond)
	}
}

func (c *dockerBranchEndpointController) writeSelection(branchName string, attachment BranchAttachment, password string) error {
	attachment.TenantID = strings.TrimSpace(attachment.TenantID)
	attachment.TimelineID = strings.TrimSpace(attachment.TimelineID)
	password = strings.TrimSpace(password)
	if attachment.TenantID == "" || attachment.TimelineID == "" || password == "" {
		return fmt.Errorf("%w: branch endpoint requires tenant, timeline, and password", ErrPrimaryEndpointUnavailable)
	}

	selection := endpointSelectionState{
		Branch:     branchName,
		TenantID:   attachment.TenantID,
		TimelineID: attachment.TimelineID,
		Password:   password,
	}

	return writeEndpointSelection(c.selectionPath(branchName), selection)
}

func (c *dockerBranchEndpointController) selectionPath(branchName string) string {
	return filepath.Join(c.computeDataDir, "endpoints", endpointBranchIdentifier(branchName), "endpoint-selection.json")
}

func (c *dockerBranchEndpointController) containerName(branchName string) string {
	return fmt.Sprintf("%s-branch-%s", c.composeProject, endpointBranchIdentifier(branchName))
}

func endpointBranchIdentifier(branchName string) string {
	trimmed := strings.TrimSpace(branchName)
	if trimmed == "" {
		trimmed = "main"
	}

	slug := sanitizeEndpointBranchName(trimmed)
	hash := sha1.Sum([]byte(trimmed))
	return slug + "-" + hex.EncodeToString(hash[:4])
}

func sanitizeEndpointBranchName(branchName string) string {
	branchName = strings.ToLower(strings.TrimSpace(branchName))
	if branchName == "" {
		return "main"
	}

	var out strings.Builder
	for _, r := range branchName {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out.WriteRune(r)
			continue
		}
		out.WriteRune('-')
	}

	value := strings.Trim(out.String(), "-")
	if value == "" {
		return "branch"
	}

	if len(value) > 48 {
		value = value[:48]
	}

	return value
}

func (c *dockerBranchEndpointController) incrementActive(branchName string) {
	c.mu.Lock()
	c.activeConns[branchName]++
	c.mu.Unlock()
}

func (c *dockerBranchEndpointController) decrementActive(branchName string) {
	c.mu.Lock()
	if c.activeConns[branchName] > 1 {
		c.activeConns[branchName]--
	} else {
		delete(c.activeConns, branchName)
	}
	c.mu.Unlock()
}

func (c *dockerBranchEndpointController) recordError(branchName string, err error) {
	if err == nil {
		return
	}
	c.mu.Lock()
	c.lastErrors[branchName] = err.Error()
	c.mu.Unlock()
}

func (c *dockerBranchEndpointController) clearError(branchName string) {
	c.mu.Lock()
	delete(c.lastErrors, branchName)
	c.mu.Unlock()
}

func (c *dockerBranchEndpointController) branchLock(branchName string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()

	if lock, exists := c.branchStartLocks[branchName]; exists {
		return lock
	}

	lock := &sync.Mutex{}
	c.branchStartLocks[branchName] = lock
	return lock
}
