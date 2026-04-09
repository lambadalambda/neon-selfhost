package branch

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrAlreadyExists = errors.New("branch already exists")
	ErrNotFound      = errors.New("branch not found")
	ErrInvalidName   = errors.New("branch name is required")
	ErrParentMissing = errors.New("parent branch not found")
	ErrProtected     = errors.New("branch is protected")
	ErrPersistFailed = errors.New("failed to persist branch state")
	ErrNoSpace       = errors.New("insufficient disk space for branch state")
)

type Branch struct {
	Name      string
	Parent    string
	CreatedAt time.Time
	Deleted   bool
	DeletedAt *time.Time

	TenantID   string
	TimelineID string
	Password   string
}

type Store struct {
	mu       sync.RWMutex
	now      func() time.Time
	branches map[string]Branch
	persist  func([]Branch) error
}

func NewStore() *Store {
	return NewStoreWithClock(defaultClock)
}

func NewStoreWithClock(now func() time.Time) *Store {
	return newStoreWithBranches(now, defaultBranchMap(now), nil)
}

func defaultClock() time.Time {
	return time.Now().UTC()
}

func defaultBranchMap(now func() time.Time) map[string]Branch {
	if now == nil {
		now = defaultClock
	}

	mainBranch := Branch{
		Name:      "main",
		Parent:    "",
		CreatedAt: now().UTC(),
	}

	return map[string]Branch{
		mainBranch.Name: mainBranch,
	}
}

func newStoreWithBranches(now func() time.Time, branches map[string]Branch, persist func([]Branch) error) *Store {
	if now == nil {
		now = defaultClock
	}

	return &Store{
		now:      now,
		branches: cloneBranches(branches),
		persist:  persist,
	}
}

func (s *Store) ListActive() []Branch {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.branches))
	for name, branch := range s.branches {
		if branch.Deleted {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	branches := make([]Branch, 0, len(names))
	for _, name := range names {
		branches = append(branches, s.branches[name])
	}

	return branches
}

func (s *Store) GetActive(name string) (Branch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	name = strings.TrimSpace(name)
	if name == "" {
		return Branch{}, ErrNotFound
	}

	branch, exists := s.branches[name]
	if !exists || branch.Deleted {
		return Branch{}, ErrNotFound
	}

	return branch, nil
}

func (s *Store) SetAttachment(name string, tenantID string, timelineID string) (Branch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name = strings.TrimSpace(name)
	if name == "" {
		return Branch{}, ErrNotFound
	}

	tenantID = strings.TrimSpace(tenantID)
	timelineID = strings.TrimSpace(timelineID)
	if tenantID == "" || timelineID == "" {
		return Branch{}, ErrInvalidName
	}

	branch, exists := s.branches[name]
	if !exists || branch.Deleted {
		return Branch{}, ErrNotFound
	}

	branch.TenantID = tenantID
	branch.TimelineID = timelineID

	nextBranches := cloneBranches(s.branches)
	nextBranches[name] = branch
	if err := s.persistAndSwap(nextBranches); err != nil {
		return Branch{}, classifyPersistError(err)
	}

	return branch, nil
}

func (s *Store) SetPassword(name string, password string) (Branch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name = strings.TrimSpace(name)
	if name == "" {
		return Branch{}, ErrNotFound
	}

	password = strings.TrimSpace(password)
	if password == "" {
		return Branch{}, ErrInvalidName
	}

	branch, exists := s.branches[name]
	if !exists || branch.Deleted {
		return Branch{}, ErrNotFound
	}

	branch.Password = password

	nextBranches := cloneBranches(s.branches)
	nextBranches[name] = branch
	if err := s.persistAndSwap(nextBranches); err != nil {
		return Branch{}, classifyPersistError(err)
	}

	return branch, nil
}

func (s *Store) Create(name string, parent string) (Branch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.createLocked(name, parent, "", "", "")
}

func (s *Store) CreateWithPassword(name string, parent string, password string) (Branch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	password = strings.TrimSpace(password)
	if password == "" {
		return Branch{}, ErrInvalidName
	}

	return s.createLocked(name, parent, "", "", password)
}

func (s *Store) CreateWithAttachment(name string, parent string, tenantID string, timelineID string) (Branch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tenantID = strings.TrimSpace(tenantID)
	timelineID = strings.TrimSpace(timelineID)
	if tenantID == "" || timelineID == "" {
		return Branch{}, ErrInvalidName
	}

	return s.createLocked(name, parent, tenantID, timelineID, "")
}

func (s *Store) CreateWithAttachmentAndPassword(name string, parent string, tenantID string, timelineID string, password string) (Branch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tenantID = strings.TrimSpace(tenantID)
	timelineID = strings.TrimSpace(timelineID)
	password = strings.TrimSpace(password)
	if tenantID == "" || timelineID == "" || password == "" {
		return Branch{}, ErrInvalidName
	}

	return s.createLocked(name, parent, tenantID, timelineID, password)
}

func (s *Store) createLocked(name string, parent string, tenantID string, timelineID string, password string) (Branch, error) {

	name = strings.TrimSpace(name)
	if name == "" {
		return Branch{}, ErrInvalidName
	}

	parent = strings.TrimSpace(parent)
	if parent == "" {
		parent = "main"
	}

	parentBranch, exists := s.branches[parent]
	if !exists || parentBranch.Deleted {
		return Branch{}, ErrParentMissing
	}

	if _, exists := s.branches[name]; exists {
		return Branch{}, ErrAlreadyExists
	}

	created := Branch{
		Name:       name,
		Parent:     parent,
		CreatedAt:  s.now().UTC(),
		TenantID:   tenantID,
		TimelineID: timelineID,
		Password:   password,
	}
	nextBranches := cloneBranches(s.branches)
	nextBranches[name] = created
	if err := s.persistAndSwap(nextBranches); err != nil {
		return Branch{}, classifyPersistError(err)
	}

	return created, nil
}

func (s *Store) SoftDelete(name string) (Branch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name = strings.TrimSpace(name)
	if name == "" {
		return Branch{}, ErrNotFound
	}

	if name == "main" {
		return Branch{}, ErrProtected
	}

	branch, exists := s.branches[name]
	if !exists {
		return Branch{}, ErrNotFound
	}

	if branch.Deleted {
		return branch, nil
	}

	now := s.now().UTC()
	branch.Deleted = true
	branch.DeletedAt = &now
	nextBranches := cloneBranches(s.branches)
	nextBranches[name] = branch
	if err := s.persistAndSwap(nextBranches); err != nil {
		return Branch{}, classifyPersistError(err)
	}

	return branch, nil
}

func (s *Store) persistAndSwap(next map[string]Branch) error {
	if s.persist != nil {
		if err := s.persist(snapshotFromMap(next)); err != nil {
			return err
		}
	}

	s.branches = next
	return nil
}

func snapshotFromMap(branches map[string]Branch) []Branch {
	names := make([]string, 0, len(branches))
	for name := range branches {
		names = append(names, name)
	}
	sort.Strings(names)

	snapshot := make([]Branch, 0, len(names))
	for _, name := range names {
		snapshot = append(snapshot, branches[name])
	}

	return snapshot
}

func cloneBranches(branches map[string]Branch) map[string]Branch {
	cloned := make(map[string]Branch, len(branches))
	for name, b := range branches {
		cloned[name] = b
	}
	return cloned
}

func classifyPersistError(err error) error {
	if errors.Is(err, syscall.ENOSPC) {
		return fmt.Errorf("%w: %v", ErrNoSpace, err)
	}

	return fmt.Errorf("%w: %v", ErrPersistFailed, err)
}
