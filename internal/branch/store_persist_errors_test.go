package branch

import (
	"errors"
	"syscall"
	"testing"
	"time"
)

func TestCreateReturnsNoSpaceErrorWhenPersistFailsWithENOSPC(t *testing.T) {
	base := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	store := newStoreWithBranches(func() time.Time { return base }, defaultBranchMap(func() time.Time { return base }), func([]Branch) error {
		return syscall.ENOSPC
	})

	_, err := store.Create("feature-a", "main")
	if !errors.Is(err, ErrNoSpace) {
		t.Fatalf("expected %v, got %v", ErrNoSpace, err)
	}

	active := store.ListActive()
	if len(active) != 1 {
		t.Fatalf("expected only main branch after failed create, got %d active branches", len(active))
	}
}

func TestCreateReturnsPersistErrorWhenPersistenceFails(t *testing.T) {
	base := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	store := newStoreWithBranches(func() time.Time { return base }, defaultBranchMap(func() time.Time { return base }), func([]Branch) error {
		return errors.New("write failed")
	})

	_, err := store.Create("feature-a", "main")
	if !errors.Is(err, ErrPersistFailed) {
		t.Fatalf("expected %v, got %v", ErrPersistFailed, err)
	}

	active := store.ListActive()
	if len(active) != 1 {
		t.Fatalf("expected only main branch after failed create, got %d active branches", len(active))
	}
}
