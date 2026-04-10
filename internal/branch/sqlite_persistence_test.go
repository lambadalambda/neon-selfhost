package branch

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSQLitePersistentStoreRoundtrip(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC) }
	dataDir := t.TempDir()

	store, err := NewSQLitePersistentStoreWithClock(dataDir, now)
	if err != nil {
		t.Fatalf("new sqlite persistent store: %v", err)
	}

	if _, err := store.Create("feature-a", "main"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	reloaded, err := NewSQLitePersistentStoreWithClock(dataDir, now)
	if err != nil {
		t.Fatalf("reload sqlite persistent store: %v", err)
	}

	if _, err := reloaded.GetActive("feature-a"); err != nil {
		t.Fatalf("expected persisted branch after reload: %v", err)
	}
}

func TestSQLitePersistentStoreImportsLegacyJSONState(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC) }
	dataDir := t.TempDir()
	legacyPath := filepath.Join(dataDir, defaultStateFileName)

	legacySnapshot := []Branch{{
		Name:      "main",
		Parent:    "",
		CreatedAt: now(),
	}, {
		Name:      "legacy-branch",
		Parent:    "main",
		CreatedAt: now(),
	}}
	if err := persistStateFile(legacyPath, legacySnapshot); err != nil {
		t.Fatalf("persist legacy snapshot: %v", err)
	}

	store, err := NewSQLitePersistentStoreWithClock(dataDir, now)
	if err != nil {
		t.Fatalf("new sqlite persistent store with legacy import: %v", err)
	}

	if _, err := store.GetActive("legacy-branch"); err != nil {
		t.Fatalf("expected imported legacy branch: %v", err)
	}
}
