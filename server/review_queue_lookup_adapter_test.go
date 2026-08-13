package server

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session"
)

// newTestStorageForReviewQueueLookup constructs a real ent-backed *session.Storage
// for reviewQueueLookupAdapter tests, mirroring the setup convention used
// elsewhere in this package (server/review_queue_manager_test.go).
func newTestStorageForReviewQueueLookup(t *testing.T) *session.Storage {
	t.Helper()
	dir := t.TempDir()
	repo, err := session.NewEntRepository(session.WithDatabasePath(filepath.Join(dir, "sessions.db")))
	if err != nil {
		t.Fatalf("NewEntRepository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		t.Fatalf("NewStorageWithRepository: %v", err)
	}
	return storage
}

// TestReviewQueueLookupAdapter_should_ReturnZeroZeroNil_When_NoLinkedBacklogItem
// covers the ErrNotFound / no-linked-backlog-item case: a session with no
// ItemSession row at all (never attached to a backlog item) must not be
// treated as an error — ReviewQueueResolvedCount degrades to (0, 0, nil), FR-6's
// first-class empty case.
func TestReviewQueueLookupAdapter_should_ReturnZeroZeroNil_When_NoLinkedBacklogItem(t *testing.T) {
	storage := newTestStorageForReviewQueueLookup(t)
	adapter := &reviewQueueLookupAdapter{storage: storage}

	resolved, stillOpen, err := adapter.ReviewQueueResolvedCount(context.Background(), "session-with-no-backlog-item")
	if err != nil {
		t.Fatalf("expected no error for a session with no linked backlog item, got %v", err)
	}
	if resolved != 0 || stillOpen != 0 {
		t.Fatalf("expected (0, 0), got (%d, %d)", resolved, stillOpen)
	}
}

// TestReviewQueueLookupAdapter_should_BucketSiblingItemSessionsByOverallOutcome_When_MultipleSessionsShareBacklogItem
// covers the core classification logic: every ItemSession attached to the same
// backlog item as the queried session's own ItemSession (siblings included) is
// bucketed into resolved (OverallOutcome set, from a ReviewVerdict) vs still-open
// (OverallOutcome empty, no verdict yet) — mirroring
// review_queue_lookup_adapter's doc comment that the backlog item, not just one
// session, is the review-queue scope.
func TestReviewQueueLookupAdapter_should_BucketSiblingItemSessionsByOverallOutcome_When_MultipleSessionsShareBacklogItem(t *testing.T) {
	storage := newTestStorageForReviewQueueLookup(t)
	ctx := context.Background()
	adapter := &reviewQueueLookupAdapter{storage: storage}

	item, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:  "shared backlog item",
		Status: string(session.BacklogStatusInProgress),
	})
	if err != nil {
		t.Fatalf("CreateBacklogItem: %v", err)
	}

	// Two resolved siblings (verdict recorded: PASS, FAIL) and one still-open
	// sibling (no verdict yet) attached to the same backlog item.
	queriedSessionUUID := uuid.NewString()
	if _, err := storage.CreateItemSessionWithVerdict(ctx,
		session.ItemSessionData{ItemID: item.ID, SessionUUID: queriedSessionUUID, SessionRole: "work"},
		session.ReviewVerdictData{OverallOutcome: session.ReviewOutcomePass, Summary: "looks good"},
	); err != nil {
		t.Fatalf("CreateItemSessionWithVerdict (queried session): %v", err)
	}
	if _, err := storage.CreateItemSessionWithVerdict(ctx,
		session.ItemSessionData{ItemID: item.ID, SessionUUID: uuid.NewString(), SessionRole: "review"},
		session.ReviewVerdictData{OverallOutcome: session.ReviewOutcomeFail, Summary: "needs work"},
	); err != nil {
		t.Fatalf("CreateItemSessionWithVerdict (resolved sibling): %v", err)
	}
	if _, err := storage.CreateItemSession(ctx,
		session.ItemSessionData{ItemID: item.ID, SessionUUID: uuid.NewString(), SessionRole: "review"},
	); err != nil {
		t.Fatalf("CreateItemSession (still-open sibling): %v", err)
	}

	resolved, stillOpen, err := adapter.ReviewQueueResolvedCount(ctx, queriedSessionUUID)
	if err != nil {
		t.Fatalf("ReviewQueueResolvedCount: %v", err)
	}
	if resolved != 2 {
		t.Errorf("expected 2 resolved (OverallOutcome set), got %d", resolved)
	}
	if stillOpen != 1 {
		t.Errorf("expected 1 still-open (no OverallOutcome), got %d", stillOpen)
	}
}
