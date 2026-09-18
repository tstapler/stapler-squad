package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingItemChangePublisher is a minimal ItemChangePublisher test double
// that records every call it receives, used to prove wiring reaches the
// concrete *EntRepository without depending on the real event-bus adapter
// (server/services.BacklogItemEventPublisher), which would introduce an
// import of the session package back into itself.
type recordingItemChangePublisher struct {
	calls []BacklogItemChange
}

func (p *recordingItemChangePublisher) PublishItemChanged(item *BacklogItemData, change BacklogItemChange) {
	p.calls = append(p.calls, change)
}

// TestStorageSetItemChangePublisher_should_forwardToConcreteEntRepository_When_RepoIsEntBacked
// exercises Task 1.3.3a's forwarding setter end-to-end: Storage.SetItemChangePublisher
// type-asserts down to *EntRepository (mirroring Storage.GetEntClient's existing
// precedent) so the publisher actually lands on the struct that owns the 9
// hooked mutation methods.
func TestStorageSetItemChangePublisher_should_forwardToConcreteEntRepository_When_RepoIsEntBacked(t *testing.T) {
	t.Parallel()
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	publisher := &recordingItemChangePublisher{}
	storage.SetItemChangePublisher(publisher)

	er := storage.repo
	assert.Same(t, ItemChangePublisher(publisher), er.itemChangePublisher, "SetItemChangePublisher must forward to the concrete *EntRepository's field")
}

// TestEntRepository_should_noOpPublish_When_ItemChangePublisherIsNil confirms
// that a repository with no publisher wired (the default, zero-value state)
// still completes TransitionBacklogItemStatus successfully — publish is
// additive/best-effort per the Risk Control section, never a precondition for
// the underlying mutation to succeed (Story 1.3.1 AC).
func TestEntRepository_should_noOpPublish_When_ItemChangePublisherIsNil(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	require.Nil(t, repo.itemChangePublisher, "test setup: no publisher should be wired by default")

	item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
		Title:  "item with no publisher wired",
		Status: string(BacklogStatusReview),
	})
	require.NoError(t, err)

	updated, err := repo.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, &BacklogItemPrecondition{
		ExpectedStatus: string(BacklogStatusReview),
	}, TriggeredBySystem)
	require.NoError(t, err, "transition must succeed even though no ItemChangePublisher is wired")
	assert.Equal(t, string(BacklogStatusDone), updated.Status)
}

var _ ItemChangePublisher = (*recordingItemChangePublisher)(nil)
