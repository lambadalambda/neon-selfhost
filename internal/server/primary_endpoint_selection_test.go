package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteEndpointSelectionPersistsSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "endpoint-selection.json")
	want := endpointSelectionState{
		Branch:     "feature-a",
		TenantID:   "tenant-1",
		TimelineID: "timeline-1",
		Password:   "secret-1",
	}

	if err := writeEndpointSelection(path, want); err != nil {
		t.Fatalf("write endpoint selection: %v", err)
	}

	got, loaded, err := loadEndpointSelection(path)
	if err != nil {
		t.Fatalf("load endpoint selection: %v", err)
	}
	if !loaded {
		t.Fatal("expected endpoint selection file to load")
	}

	if got != want {
		t.Fatalf("expected selection %+v, got %+v", want, got)
	}
}

func TestWriteEndpointSelectionUsesReadablePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "endpoint-selection.json")

	if err := writeEndpointSelection(path, endpointSelectionState{Branch: "main", TenantID: "tenant-1", TimelineID: "timeline-1"}); err != nil {
		t.Fatalf("write endpoint selection: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat endpoint selection: %v", err)
	}

	mode := info.Mode().Perm()
	if mode&0o044 != 0o044 {
		t.Fatalf("expected group/other read permissions, got %#o", mode)
	}
}
