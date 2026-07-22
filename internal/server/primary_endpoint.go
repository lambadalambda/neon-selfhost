package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultPrimaryEndpointHost     = "127.0.0.1"
	defaultPrimaryEndpointPort     = 5432
	defaultPrimaryEndpointDatabase = "postgres"
	defaultPrimaryEndpointUser     = "postgres"
	defaultPrimaryStartupTimeout   = 60 * time.Second
)

var (
	ErrPrimaryEndpointUnavailable = errors.New("primary endpoint orchestration unavailable")
	ErrPrimaryEndpointNotFound    = errors.New("primary endpoint container not found")
)

type PrimaryEndpointController interface {
	Connection() (primaryEndpointState, error)
	SetBranchAttachment(branch string, tenantID string, timelineID string) error
	SetBranchPassword(branch string, password string) error
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
	Password string

	SelectionPath string
}

type primaryEndpointConnectionInfo struct {
	Host     string
	Port     int
	Database string
	User     string
	Password string
}

type primaryEndpointRuntime interface {
	Status() (primaryEndpointRuntimeStatus, error)
	Start() error
	Stop() error
}

type primaryEndpointRuntimeStatus struct {
	Running bool
	Ready   bool
	State   string
	Message string
}

func primaryRuntimeWasActive(status primaryEndpointRuntimeStatus) bool {
	return status.Running || strings.EqualFold(strings.TrimSpace(status.State), "restarting")
}

type primaryEndpointState struct {
	Running        bool
	Ready          bool
	RuntimeState   string
	RuntimeMessage string
	Branch         string
	Host           string
	Port           int
	Database       string
	User           string
	Password       string

	TenantID   string
	TimelineID string
}

type primaryEndpointRouteState struct {
	Connection       primaryEndpointState
	Applied          bool
	Transitioning    bool
	Blocked          bool
	ReservedBranches []string
}

type primaryEndpointRouteProvider interface {
	AcquirePrimaryRouteState() (primaryEndpointRouteState, func(), error)
}

type primaryEndpointRouteCoordinator interface {
	ReservePrimaryRoute(branch string)
	ReleasePrimaryRoute(branch string)
}

type primaryEndpointAttachment struct {
	TenantID   string
	TimelineID string
}

type endpointSelectionState struct {
	Generation string `json:"generation,omitempty"`
	Branch     string `json:"branch"`
	TenantID   string `json:"tenant_id,omitempty"`
	TimelineID string `json:"timeline_id,omitempty"`
	Password   string `json:"password,omitempty"`
}

type primaryEndpointManager struct {
	opMu          sync.Mutex
	routeMu       sync.RWMutex
	mu            sync.Mutex
	runtime       primaryEndpointRuntime
	connInfo      primaryEndpointConnectionInfo
	branch        string
	attachment    primaryEndpointAttachment
	attachments   map[string]primaryEndpointAttachment
	passwords     map[string]string
	generation    string
	transitioning bool
	routeBlocked  bool
	reservations  map[string]int

	selectionPath  string
	appliedPath    string
	startupTimeout time.Duration
}

func newPrimaryEndpointManager() *primaryEndpointManager {
	return newPrimaryEndpointManagerWithRuntime(
		newInMemoryPrimaryEndpointRuntime(),
		defaultPrimaryEndpointConnectionInfo(),
		"",
	)
}

func defaultPrimaryEndpointConnectionInfo() primaryEndpointConnectionInfo {
	return primaryEndpointConnectionInfo{
		Host:     defaultPrimaryEndpointHost,
		Port:     defaultPrimaryEndpointPort,
		Database: defaultPrimaryEndpointDatabase,
		User:     defaultPrimaryEndpointUser,
		Password: defaultPrimaryEndpointUser,
	}
}

func newPrimaryEndpointManagerWithRuntime(runtime primaryEndpointRuntime, connInfo primaryEndpointConnectionInfo, selectionPath string) *primaryEndpointManager {
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
	if connInfo.Password == "" {
		connInfo.Password = connInfo.User
	}

	manager := &primaryEndpointManager{
		runtime:        runtime,
		connInfo:       connInfo,
		branch:         "main",
		attachments:    map[string]primaryEndpointAttachment{},
		passwords:      map[string]string{"main": connInfo.Password},
		reservations:   map[string]int{},
		selectionPath:  strings.TrimSpace(selectionPath),
		appliedPath:    endpointAppliedSelectionPath(selectionPath),
		startupTimeout: defaultPrimaryStartupTimeout,
	}

	if selection, loaded, err := loadEndpointSelection(manager.selectionPath); err == nil && loaded {
		selectionBranch := strings.TrimSpace(selection.Branch)
		if selectionBranch != "" {
			manager.branch = selectionBranch
		}

		if strings.TrimSpace(selection.TenantID) != "" && strings.TrimSpace(selection.TimelineID) != "" {
			attachment := primaryEndpointAttachment{TenantID: strings.TrimSpace(selection.TenantID), TimelineID: strings.TrimSpace(selection.TimelineID)}
			manager.attachment = attachment
			manager.attachments[manager.branch] = attachment
		}
		manager.generation = strings.TrimSpace(selection.Generation)

		selectionPassword := strings.TrimSpace(selection.Password)
		if selectionPassword != "" {
			manager.passwords[manager.branch] = selectionPassword
			manager.connInfo.Password = selectionPassword
		}
	}

	return manager
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
		Password: opts.Password,
	}

	return newPrimaryEndpointManagerWithRuntime(runtime, connInfo, opts.SelectionPath), nil
}

func NewInMemoryPrimaryEndpointController(host string, port int, database string, user string, password string, selectionPath string) PrimaryEndpointController {
	return newPrimaryEndpointManagerWithRuntime(nil, primaryEndpointConnectionInfo{
		Host:     host,
		Port:     port,
		Database: database,
		User:     user,
		Password: password,
	}, selectionPath)
}

func (m *primaryEndpointManager) Connection() (primaryEndpointState, error) {
	m.mu.Lock()
	branch := m.branch
	attachment := m.attachment
	connInfo := m.connInfo
	runtime := m.runtime
	appliedPath := m.appliedPath
	generation := m.generation
	transitioning := m.transitioning
	blocked := m.routeBlocked
	m.mu.Unlock()

	runtimeStatus, err := runtime.Status()
	if err != nil {
		return primaryEndpointState{}, fmt.Errorf("query primary endpoint runtime: %w", err)
	}
	if transitioning {
		runtimeStatus.Ready = false
		runtimeStatus.State = "switching"
		runtimeStatus.Message = "primary attachment transition is in progress"
	} else if blocked {
		runtimeStatus.Ready = false
		runtimeStatus.State = "unverified"
		runtimeStatus.Message = "primary attachment state requires reconciliation"
	} else if appliedPath != "" {
		applied, loaded, loadErr := loadEndpointSelection(appliedPath)
		if loadErr != nil {
			return primaryEndpointState{}, fmt.Errorf("load applied primary endpoint selection: %w", loadErr)
		}
		if loaded && runtimeStatus.Running && runtimeStatus.Ready && strings.TrimSpace(attachment.TenantID) == "" && strings.TrimSpace(attachment.TimelineID) == "" {
			adopted, adoptErr := m.adoptAppliedSelection(applied)
			if adoptErr != nil {
				return primaryEndpointState{}, adoptErr
			}
			if adopted {
				branch = strings.TrimSpace(applied.Branch)
				attachment = primaryEndpointAttachment{TenantID: strings.TrimSpace(applied.TenantID), TimelineID: strings.TrimSpace(applied.TimelineID)}
				generation = strings.TrimSpace(applied.Generation)
			}
		}
		if !loaded || !endpointSelectionsMatch(applied, endpointSelectionState{Generation: generation, Branch: branch, TenantID: attachment.TenantID, TimelineID: attachment.TimelineID}) {
			runtimeStatus.Ready = false
			runtimeStatus.State = "unverified"
			runtimeStatus.Message = "applied primary attachment is unverified"
		}
	}

	return primaryEndpointState{
		Running:        runtimeStatus.Running,
		Ready:          runtimeStatus.Ready,
		RuntimeState:   strings.TrimSpace(runtimeStatus.State),
		RuntimeMessage: strings.TrimSpace(runtimeStatus.Message),
		Branch:         branch,
		Host:           connInfo.Host,
		Port:           connInfo.Port,
		Database:       connInfo.Database,
		User:           connInfo.User,
		Password:       connInfo.Password,

		TenantID:   attachment.TenantID,
		TimelineID: attachment.TimelineID,
	}, nil
}

func (m *primaryEndpointManager) PrimaryRouteState() (primaryEndpointRouteState, error) {
	state, release, err := m.AcquirePrimaryRouteState()
	if err != nil {
		return primaryEndpointRouteState{}, err
	}
	defer release()
	return state, nil
}

func (m *primaryEndpointManager) AcquirePrimaryRouteState() (primaryEndpointRouteState, func(), error) {
	m.routeMu.RLock()
	state, err := m.primaryRouteState()
	if err != nil {
		m.routeMu.RUnlock()
		return primaryEndpointRouteState{}, nil, err
	}
	return state, m.routeMu.RUnlock, nil
}

func (m *primaryEndpointManager) primaryRouteState() (primaryEndpointRouteState, error) {
	m.mu.Lock()
	runtime := m.runtime
	connInfo := m.connInfo
	branch := m.branch
	attachment := m.attachment
	generation := m.generation
	appliedPath := m.appliedPath
	transitioning := m.transitioning
	blocked := m.routeBlocked
	reservedBranches := make([]string, 0, len(m.reservations))
	for reservedBranch, count := range m.reservations {
		if count > 0 {
			reservedBranches = append(reservedBranches, reservedBranch)
		}
	}
	m.mu.Unlock()

	runtimeStatus, err := runtime.Status()
	if err != nil {
		return primaryEndpointRouteState{}, fmt.Errorf("query primary endpoint runtime for route: %w", err)
	}

	appliedSelection, applied, err := loadEndpointSelection(appliedPath)
	if err != nil {
		return primaryEndpointRouteState{}, fmt.Errorf("load applied primary endpoint selection: %w", err)
	}
	if appliedPath == "" {
		appliedSelection = endpointSelectionState{Generation: generation, Branch: branch, TenantID: attachment.TenantID, TimelineID: attachment.TimelineID}
		applied = strings.TrimSpace(attachment.TenantID) != "" && strings.TrimSpace(attachment.TimelineID) != ""
	}
	if applied && runtimeStatus.Running && runtimeStatus.Ready && strings.TrimSpace(attachment.TenantID) == "" && strings.TrimSpace(attachment.TimelineID) == "" {
		adopted, adoptErr := m.adoptAppliedSelection(appliedSelection)
		if adoptErr != nil {
			return primaryEndpointRouteState{}, adoptErr
		}
		if adopted {
			branch = strings.TrimSpace(appliedSelection.Branch)
			attachment = primaryEndpointAttachment{TenantID: strings.TrimSpace(appliedSelection.TenantID), TimelineID: strings.TrimSpace(appliedSelection.TimelineID)}
			generation = strings.TrimSpace(appliedSelection.Generation)
		}
	}

	applied = applied && endpointSelectionsMatch(appliedSelection, endpointSelectionState{Generation: generation, Branch: branch, TenantID: attachment.TenantID, TimelineID: attachment.TimelineID})

	return primaryEndpointRouteState{
		Applied:          applied,
		Transitioning:    transitioning,
		Blocked:          blocked,
		ReservedBranches: reservedBranches,
		Connection: primaryEndpointState{
			Running:        runtimeStatus.Running,
			Ready:          runtimeStatus.Ready,
			RuntimeState:   strings.TrimSpace(runtimeStatus.State),
			RuntimeMessage: strings.TrimSpace(runtimeStatus.Message),
			Branch:         strings.TrimSpace(appliedSelection.Branch),
			Host:           connInfo.Host,
			Port:           connInfo.Port,
			Database:       connInfo.Database,
			User:           connInfo.User,
			Password:       connInfo.Password,
			TenantID:       strings.TrimSpace(appliedSelection.TenantID),
			TimelineID:     strings.TrimSpace(appliedSelection.TimelineID),
		},
	}, nil
}

func (m *primaryEndpointManager) ReservePrimaryRoute(branch string) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return
	}
	m.routeMu.Lock()
	defer m.routeMu.Unlock()
	m.mu.Lock()
	m.reservations[branch]++
	m.mu.Unlock()
}

func (m *primaryEndpointManager) ReleasePrimaryRoute(branch string) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return
	}
	m.routeMu.Lock()
	defer m.routeMu.Unlock()
	m.mu.Lock()
	if m.reservations[branch] <= 1 {
		delete(m.reservations, branch)
	} else {
		m.reservations[branch]--
	}
	m.mu.Unlock()
}

func (m *primaryEndpointManager) beginRouteTransition() {
	m.routeMu.Lock()
	defer m.routeMu.Unlock()
	m.mu.Lock()
	m.transitioning = true
	m.mu.Unlock()
}

func (m *primaryEndpointManager) finishRouteTransition(stable bool) {
	m.routeMu.Lock()
	defer m.routeMu.Unlock()
	m.mu.Lock()
	m.transitioning = false
	m.routeBlocked = !stable
	m.mu.Unlock()
}

func (m *primaryEndpointManager) adoptAppliedSelection(applied endpointSelectionState) (bool, error) {
	branch := strings.TrimSpace(applied.Branch)
	if branch == "" {
		branch = "main"
	}
	attachment := primaryEndpointAttachment{TenantID: strings.TrimSpace(applied.TenantID), TimelineID: strings.TrimSpace(applied.TimelineID)}
	if attachment.TenantID == "" || attachment.TimelineID == "" {
		return false, nil
	}

	m.mu.Lock()
	if strings.TrimSpace(m.attachment.TenantID) != "" || strings.TrimSpace(m.attachment.TimelineID) != "" {
		m.mu.Unlock()
		return false, nil
	}
	password := m.passwords[branch]
	if strings.TrimSpace(password) == "" {
		password = m.connInfo.Password
	}
	selectionPath := m.selectionPath
	m.mu.Unlock()

	desired := applied
	desired.Branch = branch
	desired.Password = password
	if err := writeEndpointSelection(selectionPath, desired); err != nil {
		return false, fmt.Errorf("persist adopted primary endpoint selection: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.TrimSpace(m.attachment.TenantID) != "" || strings.TrimSpace(m.attachment.TimelineID) != "" {
		return false, nil
	}
	m.branch = branch
	m.attachment = attachment
	m.attachments[branch] = attachment
	m.passwords[branch] = password
	m.connInfo.Password = password
	m.generation = strings.TrimSpace(applied.Generation)
	return true, nil
}

func (m *primaryEndpointManager) SetBranchAttachment(branch string, tenantID string, timelineID string) error {
	branch = strings.TrimSpace(branch)
	tenantID = strings.TrimSpace(tenantID)
	timelineID = strings.TrimSpace(timelineID)

	if branch == "" {
		return errors.New("branch name is required")
	}
	if tenantID == "" || timelineID == "" {
		return errors.New("tenant and timeline ids are required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	attachment := primaryEndpointAttachment{TenantID: tenantID, TimelineID: timelineID}
	m.attachments[branch] = attachment

	return nil
}

func (m *primaryEndpointManager) SetBranchPassword(branch string, password string) error {
	branch = strings.TrimSpace(branch)
	password = strings.TrimSpace(password)

	if branch == "" {
		return errors.New("branch name is required")
	}
	if password == "" {
		return errors.New("password is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.passwords[branch] = password

	return nil
}

func (m *primaryEndpointManager) Start() (primaryEndpointState, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.beginRouteTransition()
	stable := false
	transitionFinished := false
	defer func() {
		if !transitionFinished {
			m.finishRouteTransition(stable)
		}
	}()
	generation, err := newEndpointSelectionGeneration()
	if err != nil {
		return primaryEndpointState{}, err
	}

	m.mu.Lock()
	runtime := m.runtime
	selectionPath := m.selectionPath
	attachment := m.attachments[m.branch]
	if strings.TrimSpace(attachment.TenantID) == "" || strings.TrimSpace(attachment.TimelineID) == "" {
		attachment = m.attachment
	}
	selection := endpointSelectionState{
		Generation: generation,
		Branch:     m.branch,
		TenantID:   attachment.TenantID,
		TimelineID: attachment.TimelineID,
		Password:   m.passwords[m.branch],
	}
	if strings.TrimSpace(selection.Password) == "" {
		selection.Password = m.connInfo.Password
	}
	m.mu.Unlock()
	previousRuntimeStatus, err := runtime.Status()
	if err != nil {
		return primaryEndpointState{}, fmt.Errorf("status primary endpoint before start: %w", err)
	}

	if err := writeEndpointSelection(selectionPath, selection); err != nil {
		return primaryEndpointState{}, err
	}
	if primaryRuntimeWasActive(previousRuntimeStatus) {
		if err := runtime.Stop(); err != nil {
			return primaryEndpointState{}, fmt.Errorf("stop primary endpoint before start: %w", err)
		}
	}

	startErr := runtime.Start()
	if startErr == nil {
		startErr = waitForAppliedPrimary(runtime, m.appliedPath, selection, m.startupTimeout)
	}
	if startErr != nil {
		errs := []error{fmt.Errorf("start primary endpoint runtime: %w", startErr)}
		if stopErr := runtime.Stop(); stopErr != nil {
			errs = append(errs, fmt.Errorf("stop failed primary endpoint after start failure: %w", stopErr))
		}
		return primaryEndpointState{}, errors.Join(errs...)
	}

	m.mu.Lock()
	m.attachment = attachment
	m.attachments[m.branch] = attachment
	m.connInfo.Password = selection.Password
	m.generation = generation
	m.mu.Unlock()
	stable = true
	m.finishRouteTransition(stable)
	transitionFinished = true

	return m.Connection()
}

func (m *primaryEndpointManager) Stop() (primaryEndpointState, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.beginRouteTransition()
	stable := false
	transitionFinished := false
	defer func() {
		if !transitionFinished {
			m.finishRouteTransition(stable)
		}
	}()

	m.mu.Lock()
	runtime := m.runtime
	m.mu.Unlock()

	if err := runtime.Stop(); err != nil {
		return primaryEndpointState{}, fmt.Errorf("stop primary endpoint runtime: %w", err)
	}
	stable = true
	m.finishRouteTransition(stable)
	transitionFinished = true

	return m.Connection()
}

func (m *primaryEndpointManager) SwitchToBranch(branch string) (primaryEndpointState, error) {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return primaryEndpointState{}, errors.New("branch name is required")
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.beginRouteTransition()
	stable := false
	transitionFinished := false
	defer func() {
		if !transitionFinished {
			m.finishRouteTransition(stable)
		}
	}()
	generation, err := newEndpointSelectionGeneration()
	if err != nil {
		return primaryEndpointState{}, err
	}

	m.mu.Lock()
	runtime := m.runtime
	selectionPath := m.selectionPath
	previousSelection := endpointSelectionState{Generation: m.generation, Branch: m.branch, TenantID: m.attachment.TenantID, TimelineID: m.attachment.TimelineID, Password: m.passwords[m.branch]}
	attachment := m.attachments[branch]
	password := m.passwords[branch]
	fallbackPassword := m.connInfo.User
	m.mu.Unlock()

	previousRuntimeStatus, err := runtime.Status()
	if err != nil {
		return primaryEndpointState{}, fmt.Errorf("status primary endpoint before branch switch: %w", err)
	}

	if strings.TrimSpace(password) == "" {
		password = fallbackPassword
	}

	if err := runtime.Stop(); err != nil {
		return primaryEndpointState{}, fmt.Errorf("stop primary endpoint for branch switch: %w", err)
	}

	nextSelection := endpointSelectionState{Generation: generation, Branch: branch, TenantID: attachment.TenantID, TimelineID: attachment.TimelineID, Password: password}
	if err := writeEndpointSelection(selectionPath, nextSelection); err != nil {
		if primaryRuntimeWasActive(previousRuntimeStatus) {
			if rollbackErr := runtime.Start(); rollbackErr != nil {
				return primaryEndpointState{}, errors.Join(err, fmt.Errorf("restart previous primary endpoint: %w", rollbackErr))
			}
			if rollbackErr := waitForAppliedPrimary(runtime, m.appliedPath, previousSelection, m.startupTimeout); rollbackErr != nil {
				return primaryEndpointState{}, errors.Join(err, fmt.Errorf("verify previous primary endpoint: %w", rollbackErr))
			}
		}
		stable = true
		return primaryEndpointState{}, err
	}

	startErr := runtime.Start()
	if startErr == nil {
		startErr = waitForAppliedPrimary(runtime, m.appliedPath, nextSelection, m.startupTimeout)
	}
	if startErr != nil {
		switchErr := fmt.Errorf("start primary endpoint for branch switch: %w", startErr)
		rollbackErrs := []error{switchErr}
		stopErr := runtime.Stop()
		if stopErr != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("stop failed primary endpoint before rollback: %w", stopErr))
		}
		if rollbackErr := writeEndpointSelection(selectionPath, previousSelection); rollbackErr != nil {
			rollbackErrs = append(rollbackErrs, fmt.Errorf("restore previous endpoint selection failed: %w", rollbackErr))
			return primaryEndpointState{}, errors.Join(rollbackErrs...)
		}
		if stopErr != nil {
			return primaryEndpointState{}, errors.Join(rollbackErrs...)
		}
		if primaryRuntimeWasActive(previousRuntimeStatus) {
			if rollbackErr := runtime.Start(); rollbackErr != nil {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("restart previous primary endpoint: %w", rollbackErr))
				return primaryEndpointState{}, errors.Join(rollbackErrs...)
			}
			if rollbackErr := waitForAppliedPrimary(runtime, m.appliedPath, previousSelection, m.startupTimeout); rollbackErr != nil {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("verify previous primary endpoint: %w", rollbackErr))
				return primaryEndpointState{}, errors.Join(rollbackErrs...)
			}
		}
		stable = true
		return primaryEndpointState{}, errors.Join(rollbackErrs...)
	}

	m.mu.Lock()
	m.branch = branch
	m.attachment = attachment
	if password == "" {
		password = m.connInfo.User
		m.passwords[branch] = password
	}
	m.connInfo.Password = password
	m.generation = generation
	m.mu.Unlock()
	stable = true
	m.finishRouteTransition(stable)
	transitionFinished = true

	return m.Connection()
}

type inMemoryPrimaryEndpointRuntime struct {
	mu      sync.Mutex
	running bool
}

func newInMemoryPrimaryEndpointRuntime() *inMemoryPrimaryEndpointRuntime {
	return &inMemoryPrimaryEndpointRuntime{}
}

func (r *inMemoryPrimaryEndpointRuntime) Status() (primaryEndpointRuntimeStatus, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	status := primaryEndpointRuntimeStatus{Running: r.running, Ready: false, State: "stopped"}
	if r.running {
		status.Ready = true
		status.State = "running"
	}

	return status, nil
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

func loadEndpointSelection(path string) (endpointSelectionState, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return endpointSelectionState{}, false, nil
	}

	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return endpointSelectionState{}, false, nil
	}
	if err != nil {
		return endpointSelectionState{}, false, fmt.Errorf("%w: read endpoint selection: %v", ErrPrimaryEndpointUnavailable, err)
	}

	var selection endpointSelectionState
	if err := json.Unmarshal(content, &selection); err != nil {
		return endpointSelectionState{}, false, fmt.Errorf("%w: decode endpoint selection: %v", ErrPrimaryEndpointUnavailable, err)
	}

	return selection, true, nil
}

func endpointAppliedSelectionPath(selectionPath string) string {
	selectionPath = strings.TrimSpace(selectionPath)
	if selectionPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(selectionPath), "applied", "primary.json")
}

func waitForAppliedPrimary(runtime primaryEndpointRuntime, appliedPath string, expected endpointSelectionState, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = defaultPrimaryStartupTimeout
	}
	deadline := time.Now().Add(timeout)
	for {
		status, err := runtime.Status()
		if err != nil {
			return fmt.Errorf("query primary endpoint readiness: %w", err)
		}
		if status.Running && status.Ready {
			if strings.TrimSpace(appliedPath) == "" {
				return nil
			}
			applied, loaded, err := loadEndpointSelection(appliedPath)
			if err != nil {
				return err
			}
			if loaded && endpointSelectionsMatch(applied, expected) {
				return nil
			}
		}
		if strings.EqualFold(strings.TrimSpace(status.State), "unhealthy") {
			return fmt.Errorf("%w: primary compute is unhealthy: %s", ErrPrimaryEndpointUnavailable, strings.TrimSpace(status.Message))
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: primary compute did not apply the selected attachment before timeout", ErrPrimaryEndpointUnavailable)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func endpointSelectionsMatch(applied endpointSelectionState, expected endpointSelectionState) bool {
	generationMatches := strings.TrimSpace(expected.Generation) == "" || strings.TrimSpace(applied.Generation) == strings.TrimSpace(expected.Generation)
	return generationMatches && strings.EqualFold(strings.TrimSpace(applied.Branch), strings.TrimSpace(expected.Branch)) &&
		strings.EqualFold(strings.TrimSpace(applied.TenantID), strings.TrimSpace(expected.TenantID)) &&
		strings.EqualFold(strings.TrimSpace(applied.TimelineID), strings.TrimSpace(expected.TimelineID))
}

func newEndpointSelectionGeneration() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("%w: generate endpoint selection generation: %v", ErrPrimaryEndpointUnavailable, err)
	}
	return hex.EncodeToString(value), nil
}

func writeEndpointSelection(path string, selection endpointSelectionState) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("%w: create endpoint selection directory: %v", ErrPrimaryEndpointUnavailable, err)
	}

	content, err := json.MarshalIndent(selection, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: encode endpoint selection: %v", ErrPrimaryEndpointUnavailable, err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "endpoint-selection-*.tmp")
	if err != nil {
		return fmt.Errorf("%w: create endpoint selection temp file: %v", ErrPrimaryEndpointUnavailable, err)
	}

	tmpPath := tmp.Name()
	succeeded := false
	defer func() {
		if !succeeded {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(append(content, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%w: write endpoint selection: %v", ErrPrimaryEndpointUnavailable, err)
	}

	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("%w: set endpoint selection permissions: %v", ErrPrimaryEndpointUnavailable, err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("%w: close endpoint selection file: %v", ErrPrimaryEndpointUnavailable, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("%w: persist endpoint selection: %v", ErrPrimaryEndpointUnavailable, err)
	}

	succeeded = true
	return nil
}

func makePrimaryConnectionPayload(state primaryEndpointState) primaryEndpointPayload {
	ready := state.Running && state.Ready

	payload := primaryEndpointPayload{
		Status:         "stopped",
		Ready:          ready,
		RuntimeState:   state.RuntimeState,
		RuntimeMessage: state.RuntimeMessage,
		Branch:         state.Branch,
		Host:           state.Host,
		Port:           state.Port,
		Database:       state.Database,
		User:           state.User,
		Password:       state.Password,
		TenantID:       state.TenantID,
		TimelineID:     state.TimelineID,
	}

	if state.Running {
		payload.Status = "starting"
		if strings.EqualFold(strings.TrimSpace(state.RuntimeState), "unhealthy") {
			payload.Status = "unhealthy"
		}

		if ready {
			payload.Status = "running"
			payload.DSN = (&url.URL{
				Scheme:   "postgres",
				User:     url.UserPassword(state.User, state.Password),
				Host:     fmt.Sprintf("%s:%d", state.Host, state.Port),
				Path:     "/" + url.PathEscape(state.Database),
				RawQuery: "sslmode=disable",
			}).String()
		}
	}

	return payload
}
