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

func TestPrimaryRouteStateUsesAppliedSelectionInsteadOfDesiredAttachment(t *testing.T) {
	dir := t.TempDir()
	selectionPath := filepath.Join(dir, "endpoint-selection.json")
	appliedPath := endpointAppliedSelectionPath(selectionPath)
	mainSelection := endpointSelectionState{Branch: "main", TenantID: "tenant", TimelineID: "timeline-main", Password: "secret-main"}
	if err := writeEndpointSelection(selectionPath, mainSelection); err != nil {
		t.Fatalf("write desired selection: %v", err)
	}
	if err := writeEndpointSelection(appliedPath, endpointSelectionState{Branch: "main", TenantID: "tenant", TimelineID: "timeline-main"}); err != nil {
		t.Fatalf("write applied selection: %v", err)
	}

	manager := newPrimaryEndpointManagerWithRuntime(&fakePrimaryEndpointRuntime{running: true}, primaryEndpointConnectionInfo{
		Host: "127.0.0.1", Port: 55433, Database: "postgres", User: "cloud_admin", Password: "secret-main",
	}, selectionPath)
	if err := manager.SetBranchAttachment("main", "tenant", "timeline-new"); err != nil {
		t.Fatalf("set desired attachment: %v", err)
	}

	state, err := manager.PrimaryRouteState()
	if err != nil {
		t.Fatalf("primary route state: %v", err)
	}
	if !state.Applied || state.Connection.TimelineID != "timeline-main" {
		t.Fatalf("expected applied timeline to remain timeline-main, got %#v", state)
	}
}

func TestPrimaryConnectionAdoptsHealthyAppliedSelectionOnCleanState(t *testing.T) {
	selectionPath := filepath.Join(t.TempDir(), "endpoint-selection.json")
	applied := endpointSelectionState{Branch: "main", TenantID: "tenant", TimelineID: "timeline"}
	if err := writeEndpointSelection(endpointAppliedSelectionPath(selectionPath), applied); err != nil {
		t.Fatalf("write applied selection: %v", err)
	}
	manager := newPrimaryEndpointManagerWithRuntime(&fakePrimaryEndpointRuntime{running: true}, primaryEndpointConnectionInfo{
		Host: "127.0.0.1", Port: 55433, Database: "postgres", User: "cloud_admin", Password: "secret",
	}, selectionPath)

	state, err := manager.Connection()
	if err != nil {
		t.Fatalf("primary connection: %v", err)
	}
	if !state.Ready || state.TenantID != "tenant" || state.TimelineID != "timeline" {
		t.Fatalf("expected healthy applied selection adoption, got %#v", state)
	}
	desired, loaded, err := loadEndpointSelection(selectionPath)
	if err != nil || !loaded {
		t.Fatalf("load adopted desired selection: loaded=%v err=%v", loaded, err)
	}
	if desired.TenantID != "tenant" || desired.TimelineID != "timeline" || desired.Password != "secret" {
		t.Fatalf("unexpected adopted desired selection %#v", desired)
	}
}
