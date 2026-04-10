package server

import (
	"errors"
	"math/rand"
	"testing"
	"time"

	"neon-selfhost/internal/branch"
)

func TestResolveAttachmentForAutoPublishRetriesUntilSuccess(t *testing.T) {
	resolver := &retryAttachmentResolver{
		resolveErrs:       []error{branch.ErrNotFound, branch.ErrNotFound, nil},
		resolveAttachment: BranchAttachment{TenantID: "tenant-a", TimelineID: "timeline-a"},
	}

	attachment, err := resolveAttachmentForAutoPublish(resolver, "feature-a")
	if err != nil {
		t.Fatalf("resolve attachment after retries: %v", err)
	}

	if resolver.resolveCalls != 3 {
		t.Fatalf("expected %d resolve calls, got %d", 3, resolver.resolveCalls)
	}

	if attachment.TenantID != "tenant-a" || attachment.TimelineID != "timeline-a" {
		t.Fatalf("unexpected attachment %+v", attachment)
	}
}

func TestResolveAttachmentForAutoPublishDoesNotRetryNonNotFound(t *testing.T) {
	resolver := &retryAttachmentResolver{resolveErrs: []error{errors.New("boom")}}

	_, err := resolveAttachmentForAutoPublish(resolver, "feature-a")
	if err == nil {
		t.Fatal("expected resolve error")
	}

	if resolver.resolveCalls != 1 {
		t.Fatalf("expected one resolve call for non-not-found error, got %d", resolver.resolveCalls)
	}
}

func TestAutoPublishResolveDelayBounds(t *testing.T) {
	rng := rand.New(rand.NewSource(123))
	for attempt := 0; attempt < 12; attempt++ {
		delay := autoPublishResolveDelay(attempt, rng)
		base := autoPublishResolveBaseDelay << attempt
		if base > autoPublishResolveMaxDelay {
			base = autoPublishResolveMaxDelay
		}

		minDelay := time.Duration(float64(base) * 0.75)
		maxDelay := time.Duration(float64(base) * 1.25)
		if delay < minDelay || delay > maxDelay {
			t.Fatalf("attempt %d delay %s out of bounds [%s,%s]", attempt, delay, minDelay, maxDelay)
		}
	}
}

type retryAttachmentResolver struct {
	resolveErrs       []error
	resolveCalls      int
	resolveAttachment BranchAttachment
}

func (r *retryAttachmentResolver) Resolve(_ string) (BranchAttachment, error) {
	err := error(nil)
	if r.resolveCalls < len(r.resolveErrs) {
		err = r.resolveErrs[r.resolveCalls]
	}
	r.resolveCalls++
	if err != nil {
		return BranchAttachment{}, err
	}

	return r.resolveAttachment, nil
}

func (r *retryAttachmentResolver) ResolveReset(_ string) (BranchAttachment, error) {
	return BranchAttachment{}, errors.New("not implemented")
}

func (r *retryAttachmentResolver) ResolveRestore(_ string, _ string, _ time.Time) (BranchAttachment, string, error) {
	return BranchAttachment{}, "", errors.New("not implemented")
}
