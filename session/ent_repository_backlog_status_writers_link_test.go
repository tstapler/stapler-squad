package session

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
)

// fakeLinkWriter stands in for itemPRLinkWrite: it records the tx and arguments it
// was handed and proves, from inside the tx, that the item UPDATE is already visible.
type fakeLinkWriter struct {
	t         *testing.T
	err       error
	ran       bool
	itemID    string
	txSawPR   int
	afterLink func() error
	hookErr   error
}

func (f *fakeLinkWriter) WriteTx(ctx context.Context, tx *ent.Tx, itemID string, afterLink func() error) error {
	f.ran = true
	f.itemID = itemID
	f.afterLink = afterLink
	id, perr := uuid.Parse(itemID)
	require.NoError(f.t, perr)
	row, gerr := tx.BacklogItem.Get(ctx, id)
	require.NoError(f.t, gerr)
	f.txSawPR = row.PrNumber
	if f.err != nil {
		return f.err
	}
	if afterLink != nil {
		f.hookErr = afterLink()
		return f.hookErr
	}
	return nil
}

func TestRawTransitionBacklogItemStatusWithPRFields_should_RollBackEverything_When_LinkWriteFails(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item := newStatusWriterItem(t, repo, domain.BacklogStatusReview)
	published := captureChanges(repo)
	d := &recordingDispatcher{}
	repo.SetCallbackDispatcher(d)
	before, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)

	boom := errors.New("link write failed")
	link := &fakeLinkWriter{t: t, err: boom}
	_, err = repo.RawTransitionBacklogItemStatusWithPRFields(ctx, item.ID, domain.BacklogStatusPRPending,
		"https://example.test/o/r/pull/7", 7,
		&backlogdto.BacklogItemPrecondition{ExpectedStatus: string(domain.BacklogStatusReview)}, domain.TriggeredBySystem, link)

	require.ErrorIs(t, err, boom, "the link error propagates unchanged")
	require.True(t, link.ran)
	assert.Equal(t, 7, link.txSawPR, "the item UPDATE was inside the tx the link writer received")

	after, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, before.Status, after.Status)
	assert.Equal(t, before.PrNumber, after.PrNumber)
	assert.Equal(t, before.PrURL, after.PrURL)
	assert.True(t, before.UpdatedAt.Equal(after.UpdatedAt), "updated_at rolled back too")
	assert.Empty(t, statusEventsFor(t, repo, item.ID), "no audit row for a rolled-back transition")
	assert.Empty(t, published(), "nothing published")
	assert.Empty(t, d.snapshot(), "no callback fired")
}

func TestRawTransitionBacklogItemStatusWithPRFields_should_RollBackAndPropagate_When_AfterLinkHookFails(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item := newStatusWriterItem(t, repo, domain.BacklogStatusReview)
	hookErr := errors.New("hook failed")
	repo.SetAfterLinkHook(func() error { return hookErr })

	link := &fakeLinkWriter{t: t}
	_, err := repo.RawTransitionBacklogItemStatusWithPRFields(ctx, item.ID, domain.BacklogStatusPRPending,
		"https://example.test/o/r/pull/7", 7, nil, domain.TriggeredBySystem, link)

	require.ErrorIs(t, err, hookErr)
	after, err := repo.GetBacklogItem(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, string(domain.BacklogStatusReview), after.Status)
	assert.Zero(t, after.PrNumber)
}

func TestRawTransitionBacklogItemStatusWithPRFields_should_PassTxAndCallTimeHookToLinkWriter_When_Succeeding(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	ctx := context.Background()
	item := newStatusWriterItem(t, repo, domain.BacklogStatusReview)

	// Installed after construction: the getter must be read at call time.
	var mu sync.Mutex
	hookRuns := 0
	repo.SetAfterLinkHook(func() error {
		mu.Lock()
		defer mu.Unlock()
		hookRuns++
		return nil
	})

	link := &fakeLinkWriter{t: t}
	got, err := repo.RawTransitionBacklogItemStatusWithPRFields(ctx, item.ID, domain.BacklogStatusPRPending,
		"https://example.test/o/r/pull/7", 7, nil, domain.TriggeredBySystem, link)
	require.NoError(t, err)

	require.True(t, link.ran)
	id, err := repo.ResolveBacklogItemLookup(ctx, item.ID)
	require.NoError(t, err)
	assert.Equal(t, id.String(), link.itemID)
	assert.NotNil(t, link.afterLink, "the call-time hook reaches the link writer")
	mu.Lock()
	assert.Equal(t, 1, hookRuns)
	mu.Unlock()
	assert.Equal(t, 7, got.PrNumber)
	assert.Len(t, statusEventsFor(t, repo, item.ID), 1)
}

func TestRawTransitionBacklogItemStatusWithPRFields_should_PassNilHookToLinkWriter_When_NoneInstalled(t *testing.T) {
	t.Parallel()
	repo := NewTestEntRepository(t)
	item := newStatusWriterItem(t, repo, domain.BacklogStatusReview)
	link := &fakeLinkWriter{t: t}
	_, err := repo.RawTransitionBacklogItemStatusWithPRFields(context.Background(), item.ID, domain.BacklogStatusPRPending,
		"https://example.test/o/r/pull/7", 7, nil, domain.TriggeredBySystem, link)
	require.NoError(t, err)
	assert.Nil(t, link.afterLink)
}
