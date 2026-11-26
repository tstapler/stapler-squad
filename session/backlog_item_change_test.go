package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

// recordingItemChangePublisher is a minimal ItemChangePublisher test double
// that records every call it receives, used to prove wiring reaches the
// concrete *EntRepository without depending on the real event-bus adapter
// (server/services/backlogmisc.BacklogItemEventPublisher), which would introduce an
// import of the session package back into itself.
type recordingItemChangePublisher struct {
	calls []backlogdto.BacklogItemChange
}

func (p *recordingItemChangePublisher) PublishItemChanged(item *backlogdto.BacklogItemData, change backlogdto.BacklogItemChange) {
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

	item, err := storage.Repo.CreateBacklogItem(context.Background(), backlogdto.BacklogItemData{
		Title:  "forwarding probe",
		Status: string(domain.BacklogStatusReview),
	})
	require.NoError(t, err)
	_, err = storage.Repo.RawTransitionBacklogItemStatus(context.Background(), item.ID, domain.BacklogStatusDone, nil, instance.TriggeredBySystem)
	require.NoError(t, err)
	require.NotEmpty(t, publisher.calls, "SetItemChangePublisher must forward to the concrete *EntRepository")
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

	require.False(t, repo.Backlog.Hooks().PublisherBound(), "test setup: no publisher should be wired by default")

	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title:  "item with no publisher wired",
		Status: string(domain.BacklogStatusReview),
	})
	require.NoError(t, err)

	updated, err := repo.RawTransitionBacklogItemStatus(ctx, item.ID, domain.BacklogStatusDone, &backlogdto.BacklogItemPrecondition{
		ExpectedStatus: string(domain.BacklogStatusReview),
	}, instance.TriggeredBySystem)
	require.NoError(t, err, "transition must succeed even though no backlogdto.ItemChangePublisher is wired")
	assert.Equal(t, string(domain.BacklogStatusDone), updated.Status)
}

var _ backlogdto.ItemChangePublisher = (*recordingItemChangePublisher)(nil)
