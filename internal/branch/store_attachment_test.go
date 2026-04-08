package branch

import (
	"errors"
	"testing"
)

func TestSetAttachmentUpdatesBranch(t *testing.T) {
	store := NewStore()

	if _, err := store.Create("feature-a", "main"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	updated, err := store.SetAttachment("feature-a", "tenant-1", "timeline-1")
	if err != nil {
		t.Fatalf("set attachment: %v", err)
	}

	if updated.TenantID != "tenant-1" {
		t.Fatalf("expected tenant id %q, got %q", "tenant-1", updated.TenantID)
	}

	if updated.TimelineID != "timeline-1" {
		t.Fatalf("expected timeline id %q, got %q", "timeline-1", updated.TimelineID)
	}

	fetched, err := store.GetActive("feature-a")
	if err != nil {
		t.Fatalf("get active branch: %v", err)
	}

	if fetched.TenantID != "tenant-1" || fetched.TimelineID != "timeline-1" {
		t.Fatalf("expected persisted attachment tenant=%q timeline=%q, got tenant=%q timeline=%q", "tenant-1", "timeline-1", fetched.TenantID, fetched.TimelineID)
	}
}

func TestSetAttachmentRejectsMissingBranch(t *testing.T) {
	store := NewStore()

	_, err := store.SetAttachment("missing", "tenant-1", "timeline-1")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected %v, got %v", ErrNotFound, err)
	}
}
