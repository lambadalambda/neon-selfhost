package branch

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistentStoreReloadsState(t *testing.T) {
	dir := t.TempDir()
	clock := newSequentialClock(time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC), time.Second)

	store, err := NewPersistentStoreWithClock(dir, clock.Now)
	if err != nil {
		t.Fatalf("new persistent store: %v", err)
	}

	if _, err := store.Create("feature-a", "main"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	if _, err := store.SoftDelete("feature-a"); err != nil {
		t.Fatalf("soft delete branch: %v", err)
	}

	reloaded, err := NewPersistentStoreWithClock(dir, clock.Now)
	if err != nil {
		t.Fatalf("reload persistent store: %v", err)
	}

	active := reloaded.ListActive()
	if len(active) != 1 {
		t.Fatalf("expected 1 active branch, got %d", len(active))
	}

	if active[0].Name != "main" {
		t.Fatalf("expected active branch %q, got %q", "main", active[0].Name)
	}

	if _, err := reloaded.Create("feature-a", "main"); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected %v after reload, got %v", ErrAlreadyExists, err)
	}
}

func TestPersistentStoreRejectsInvalidStateFile(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, defaultStateFileName)
	if err := os.WriteFile(statePath, []byte("{"), 0o644); err != nil {
		t.Fatalf("write invalid state file: %v", err)
	}

	_, err := NewPersistentStore(dir)
	if err == nil {
		t.Fatal("expected error for invalid persisted state")
	}
}

func TestPersistentStoreRejectsStateWithoutMainBranch(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, defaultStateFileName)
	if err := os.WriteFile(statePath, []byte(`{"branches":[]}`), 0o644); err != nil {
		t.Fatalf("write invalid state file: %v", err)
	}

	_, err := NewPersistentStore(dir)
	if err == nil {
		t.Fatal("expected error for persisted state without main branch")
	}
}

type sequentialClock struct {
	current time.Time
	step    time.Duration
}

func newSequentialClock(start time.Time, step time.Duration) *sequentialClock {
	return &sequentialClock{current: start.Add(-step), step: step}
}

func (c *sequentialClock) Now() time.Time {
	c.current = c.current.Add(c.step)
	return c.current
}
