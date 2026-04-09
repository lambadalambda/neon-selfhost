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

func TestCreateWithAttachmentPersistsAttachment(t *testing.T) {
	store := NewStore()

	created, err := store.CreateWithAttachment("restore-a", "main", "tenant-1", "timeline-1")
	if err != nil {
		t.Fatalf("create with attachment: %v", err)
	}

	if created.TenantID != "tenant-1" || created.TimelineID != "timeline-1" {
		t.Fatalf("expected created branch attachment tenant=%q timeline=%q, got tenant=%q timeline=%q", "tenant-1", "timeline-1", created.TenantID, created.TimelineID)
	}

	fetched, err := store.GetActive("restore-a")
	if err != nil {
		t.Fatalf("get active branch: %v", err)
	}

	if fetched.TenantID != "tenant-1" || fetched.TimelineID != "timeline-1" {
		t.Fatalf("expected persisted attachment tenant=%q timeline=%q, got tenant=%q timeline=%q", "tenant-1", "timeline-1", fetched.TenantID, fetched.TimelineID)
	}
}

func TestCreateWithAttachmentRejectsMissingAttachment(t *testing.T) {
	store := NewStore()

	_, err := store.CreateWithAttachment("restore-a", "main", "tenant-1", "")
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected %v, got %v", ErrInvalidName, err)
	}
}

func TestSetPasswordUpdatesBranch(t *testing.T) {
	store := NewStore()

	if _, err := store.Create("feature-a", "main"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	updated, err := store.SetPassword("feature-a", "secret-1")
	if err != nil {
		t.Fatalf("set password: %v", err)
	}

	if updated.Password != "secret-1" {
		t.Fatalf("expected password %q, got %q", "secret-1", updated.Password)
	}

	fetched, err := store.GetActive("feature-a")
	if err != nil {
		t.Fatalf("get active branch: %v", err)
	}

	if fetched.Password != "secret-1" {
		t.Fatalf("expected persisted password %q, got %q", "secret-1", fetched.Password)
	}
}

func TestCreateWithPasswordPersistsPassword(t *testing.T) {
	store := NewStore()

	created, err := store.CreateWithPassword("feature-a", "main", "secret-1")
	if err != nil {
		t.Fatalf("create with password: %v", err)
	}

	if created.Password != "secret-1" {
		t.Fatalf("expected password %q, got %q", "secret-1", created.Password)
	}
}

func TestCreateWithAttachmentAndPasswordPersistsCredentials(t *testing.T) {
	store := NewStore()

	created, err := store.CreateWithAttachmentAndPassword("restore-a", "main", "tenant-1", "timeline-1", "secret-2")
	if err != nil {
		t.Fatalf("create with attachment and password: %v", err)
	}

	if created.TenantID != "tenant-1" || created.TimelineID != "timeline-1" || created.Password != "secret-2" {
		t.Fatalf("unexpected credentials on created branch: tenant=%q timeline=%q password=%q", created.TenantID, created.TimelineID, created.Password)
	}
}

func TestSetEndpointPersistsPublishedPort(t *testing.T) {
	store := NewStore()

	if _, err := store.Create("feature-a", "main"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	updated, err := store.SetEndpoint("feature-a", true, 56001)
	if err != nil {
		t.Fatalf("set endpoint: %v", err)
	}

	if !updated.EndpointPublished || updated.EndpointPort != 56001 {
		t.Fatalf("expected published endpoint on port %d, got published=%v port=%d", 56001, updated.EndpointPublished, updated.EndpointPort)
	}

	fetched, err := store.GetActive("feature-a")
	if err != nil {
		t.Fatalf("get active branch: %v", err)
	}

	if !fetched.EndpointPublished || fetched.EndpointPort != 56001 {
		t.Fatalf("expected persisted endpoint published=%v port=%d, got published=%v port=%d", true, 56001, fetched.EndpointPublished, fetched.EndpointPort)
	}
}

func TestSetEndpointClearsPortWhenUnpublished(t *testing.T) {
	store := NewStore()

	if _, err := store.Create("feature-a", "main"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	if _, err := store.SetEndpoint("feature-a", true, 56001); err != nil {
		t.Fatalf("set endpoint: %v", err)
	}

	updated, err := store.SetEndpoint("feature-a", false, 12345)
	if err != nil {
		t.Fatalf("clear endpoint: %v", err)
	}

	if updated.EndpointPublished || updated.EndpointPort != 0 {
		t.Fatalf("expected unpublished endpoint with port 0, got published=%v port=%d", updated.EndpointPublished, updated.EndpointPort)
	}
}

func TestSetEndpointRejectsInvalidPort(t *testing.T) {
	store := NewStore()

	if _, err := store.Create("feature-a", "main"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	_, err := store.SetEndpoint("feature-a", true, 0)
	if !errors.Is(err, ErrInvalidName) {
		t.Fatalf("expected %v, got %v", ErrInvalidName, err)
	}
}

func TestCreateAllowsReusingSoftDeletedBranchName(t *testing.T) {
	store := NewStore()

	if _, err := store.CreateWithAttachmentAndPassword("feature-a", "main", "tenant-1", "timeline-1", "secret-1"); err != nil {
		t.Fatalf("create branch: %v", err)
	}

	if _, err := store.SetEndpoint("feature-a", true, 56001); err != nil {
		t.Fatalf("publish endpoint: %v", err)
	}

	if _, err := store.SoftDelete("feature-a"); err != nil {
		t.Fatalf("soft delete branch: %v", err)
	}

	recreated, err := store.Create("feature-a", "main")
	if err != nil {
		t.Fatalf("recreate branch: %v", err)
	}

	if recreated.Deleted {
		t.Fatal("expected recreated branch to be active")
	}

	if recreated.DeletedAt != nil {
		t.Fatal("expected recreated branch deleted_at to be cleared")
	}

	if recreated.TenantID != "" || recreated.TimelineID != "" || recreated.Password != "" {
		t.Fatalf("expected recreated branch credentials to reset, got tenant=%q timeline=%q password=%q", recreated.TenantID, recreated.TimelineID, recreated.Password)
	}

	if recreated.EndpointPublished || recreated.EndpointPort != 0 {
		t.Fatalf("expected recreated branch endpoint metadata to reset, got published=%v port=%d", recreated.EndpointPublished, recreated.EndpointPort)
	}

	active, err := store.GetActive("feature-a")
	if err != nil {
		t.Fatalf("get active branch: %v", err)
	}

	if active.Name != "feature-a" || active.Parent != "main" {
		t.Fatalf("unexpected recreated branch payload: %+v", active)
	}
}
