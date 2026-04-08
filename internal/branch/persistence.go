package branch

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultStateFileName = "branches.json"

type persistedState struct {
	Branches []Branch `json:"branches"`
}

func NewPersistentStore(dataDir string) (*Store, error) {
	return NewPersistentStoreWithClock(dataDir, defaultClock)
}

func NewPersistentStoreWithClock(dataDir string, now func() time.Time) (*Store, error) {
	if now == nil {
		now = defaultClock
	}

	cleanDir := strings.TrimSpace(dataDir)
	if cleanDir == "" {
		return nil, fmt.Errorf("controller data dir is required")
	}

	statePath := filepath.Join(cleanDir, defaultStateFileName)
	loaded, exists, err := loadPersistedState(statePath)
	if err != nil {
		return nil, err
	}

	var branches map[string]Branch
	if !exists {
		branches = defaultBranchMap(now)
		if err := persistStateFile(statePath, snapshotFromMap(branches)); err != nil {
			return nil, fmt.Errorf("persist initial branch state: %w", err)
		}
	} else {
		branches, err = branchMapFromSnapshot(loaded)
		if err != nil {
			return nil, fmt.Errorf("invalid persisted branch state: %w", err)
		}
	}

	persist := func(snapshot []Branch) error {
		if err := persistStateFile(statePath, snapshot); err != nil {
			return fmt.Errorf("write branch state file: %w", err)
		}
		return nil
	}

	return newStoreWithBranches(now, branches, persist), nil
}

func loadPersistedState(path string) ([]Branch, bool, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read branch state file: %w", err)
	}

	var state persistedState
	if err := json.Unmarshal(content, &state); err != nil {
		return nil, false, fmt.Errorf("decode branch state file: %w", err)
	}

	return state.Branches, true, nil
}

func branchMapFromSnapshot(snapshot []Branch) (map[string]Branch, error) {
	if len(snapshot) == 0 {
		return nil, fmt.Errorf("snapshot must include branches")
	}

	branches := make(map[string]Branch, len(snapshot))
	for _, b := range snapshot {
		name := strings.TrimSpace(b.Name)
		if name == "" {
			return nil, fmt.Errorf("branch name is required")
		}
		if _, exists := branches[name]; exists {
			return nil, fmt.Errorf("duplicate branch %q", name)
		}

		normalized := b
		normalized.Name = name
		normalized.CreatedAt = normalized.CreatedAt.UTC()
		if normalized.DeletedAt != nil {
			deletedAt := normalized.DeletedAt.UTC()
			normalized.DeletedAt = &deletedAt
		}

		branches[name] = normalized
	}

	mainBranch, exists := branches["main"]
	if !exists {
		return nil, fmt.Errorf("main branch is required")
	}
	if mainBranch.Deleted {
		return nil, fmt.Errorf("main branch cannot be deleted")
	}

	for name, b := range branches {
		if name == "main" {
			continue
		}

		parent := strings.TrimSpace(b.Parent)
		if parent == "" {
			parent = "main"
			b.Parent = parent
			branches[name] = b
		}

		if _, ok := branches[parent]; !ok {
			return nil, fmt.Errorf("branch %q has unknown parent %q", name, parent)
		}
	}

	return branches, nil
}

func persistStateFile(path string, branches []Branch) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	content, err := json.MarshalIndent(persistedState{Branches: branches}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "branches-*.tmp")
	if err != nil {
		return err
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
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	succeeded = true
	return nil
}
