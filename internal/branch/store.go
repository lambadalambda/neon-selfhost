package branch

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrAlreadyExists = errors.New("branch already exists")
	ErrNotFound      = errors.New("branch not found")
	ErrInvalidName   = errors.New("branch name is required")
	ErrParentMissing = errors.New("parent branch not found")
	ErrProtected     = errors.New("branch is protected")
)

type Branch struct {
	Name      string
	Parent    string
	CreatedAt time.Time
	Deleted   bool
	DeletedAt *time.Time
}

type Store struct {
	mu       sync.RWMutex
	now      func() time.Time
	branches map[string]Branch
}

func NewStore() *Store {
	return NewStoreWithClock(func() time.Time {
		return time.Now().UTC()
	})
}

func NewStoreWithClock(now func() time.Time) *Store {
	if now == nil {
		now = func() time.Time {
			return time.Now().UTC()
		}
	}

	mainBranch := Branch{
		Name:      "main",
		Parent:    "",
		CreatedAt: now().UTC(),
	}

	return &Store{
		now: now,
		branches: map[string]Branch{
			mainBranch.Name: mainBranch,
		},
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

func (s *Store) Create(name string, parent string) (Branch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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
		Name:      name,
		Parent:    parent,
		CreatedAt: s.now().UTC(),
	}
	s.branches[name] = created

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
	s.branches[name] = branch

	return branch, nil
}
