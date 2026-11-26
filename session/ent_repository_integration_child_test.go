package session

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/backlogstore"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

func newEpicWithSiblings(t *testing.T, repo *EntRepository, n int) (epic *backlogdto.BacklogItemData, sibs []string) {
	t.Helper()
	ctx := context.Background()
	epic, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "epic"})
	require.NoError(t, err)
	for i := 0; i < n; i++ {
		s, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "sibling"})
		require.NoError(t, err)
		sibs = append(sibs, s.ID)
	}
	return epic, sibs
}

func TestIntegrationChild_should_CreateOneChildBlockedOnEverySibling_When_Created(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	epic, sibs := newEpicWithSiblings(t, repo, 3)

	child, created, err := repo.CreateIntegrationChild(ctx, epic.ID, sibs, "make e2e")
	require.NoError(t, err)
	assert.True(t, created)
	assert.True(t, child.IsIntegrationChild)
	assert.Contains(t, child.Description, "make e2e")

	blockedBy, _, err := repo.ListBacklogItemDependencies(ctx, child.ID)
	require.NoError(t, err)
	got := make([]string, 0, len(blockedBy))
	for _, r := range blockedBy {
		got = append(got, r.ItemID)
	}
	assert.ElementsMatch(t, sibs, got)

	reloaded, err := repo.GetBacklogItem(ctx, epic.ID)
	require.NoError(t, err)
	assert.Equal(t, child.ID, reloaded.IntegrationChildID)
}

func TestIntegrationChild_should_BeIdempotent_When_CreatedTwice(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	epic, sibs := newEpicWithSiblings(t, repo, 2)

	first, created1, err := repo.CreateIntegrationChild(ctx, epic.ID, sibs, "make e2e")
	require.NoError(t, err)
	second, created2, err := repo.CreateIntegrationChild(ctx, epic.ID, sibs, "make e2e")
	require.NoError(t, err)

	assert.True(t, created1)
	assert.False(t, created2)
	assert.Equal(t, first.ID, second.ID)
	all, err := repo.ListBacklogItems(ctx, backlogdto.BacklogItemFilter{})
	require.NoError(t, err)
	n := 0
	for _, it := range all {
		if it.IsIntegrationChild {
			n++
		}
	}
	assert.Equal(t, 1, n)
}

func TestIntegrationChild_should_Reject_When_E2ECommandEmpty(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	epic, sibs := newEpicWithSiblings(t, repo, 1)

	_, _, err := repo.CreateIntegrationChild(context.Background(), epic.ID, sibs, "  ")
	require.Error(t, err)
}

func TestIntegrationChild_should_BlockEpicDone_UntilChildResolved(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	epic, sibs := newEpicWithSiblings(t, repo, 1)
	child, _, err := repo.CreateIntegrationChild(ctx, epic.ID, sibs, "make e2e")
	require.NoError(t, err)

	_, err = repo.RawTransitionBacklogItemStatus(ctx, epic.ID, domain.BacklogStatusDone, nil, "test")
	require.ErrorIs(t, err, backlogdto.ErrIntegrationCheckPending)

	_, err = repo.RawTransitionBacklogItemStatus(ctx, child.ID, domain.BacklogStatusDone, nil, "test")
	require.NoError(t, err)
	_, err = repo.RawTransitionBacklogItemStatus(ctx, epic.ID, domain.BacklogStatusDone, nil, "test")
	require.NoError(t, err)
}

func TestIntegrationChild_should_KeepEpicBlocked_When_ChildArchivedWithoutPassing(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	epic, sibs := newEpicWithSiblings(t, repo, 1)
	child, _, err := repo.CreateIntegrationChild(ctx, epic.ID, sibs, "make e2e")
	require.NoError(t, err)

	_, err = repo.RawArchiveBacklogItem(ctx, child.ID, nil, "test", "")
	require.NoError(t, err)
	// Archiving an already-archived child must not change the verdict.
	_, err = repo.RawArchiveBacklogItem(ctx, child.ID, nil, "test", "")
	require.NoError(t, err)
	_, err = repo.RawTransitionBacklogItemStatus(ctx, epic.ID, domain.BacklogStatusDone, nil, "test")
	require.ErrorIs(t, err, backlogdto.ErrIntegrationCheckPending, "AC3: only a passed integration child lets the epic close")
	assert.Contains(t, err.Error(), "unarchive and complete it")

	// Way out: unarchive the child and complete it.
	_, err = repo.RawUnarchiveBacklogItem(ctx, child.ID, "")
	require.NoError(t, err)
	_, err = repo.RawTransitionBacklogItemStatus(ctx, child.ID, domain.BacklogStatusDone, nil, "test")
	require.NoError(t, err)
	_, err = repo.RawTransitionBacklogItemStatus(ctx, epic.ID, domain.BacklogStatusDone, nil, "test")
	require.NoError(t, err)
}

func TestIntegrationChild_should_AllowEpicDone_When_PassedChildArchivedLater(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	epic, sibs := newEpicWithSiblings(t, repo, 1)
	child, _, err := repo.CreateIntegrationChild(ctx, epic.ID, sibs, "make e2e")
	require.NoError(t, err)
	_, err = repo.RawTransitionBacklogItemStatus(ctx, child.ID, domain.BacklogStatusDone, nil, "test")
	require.NoError(t, err)
	_, err = repo.RawArchiveBacklogItem(ctx, child.ID, nil, "system", "")
	require.NoError(t, err)
	_, err = repo.RawArchiveBacklogItem(ctx, child.ID, nil, "system", "")
	require.NoError(t, err)

	again, created, err := repo.CreateIntegrationChild(ctx, epic.ID, sibs, "make e2e")
	require.NoError(t, err)
	assert.False(t, created, "a passed check is not replaced")
	assert.Equal(t, child.ID, again.ID)
	_, err = repo.RawTransitionBacklogItemStatus(ctx, epic.ID, domain.BacklogStatusDone, nil, "test")
	require.NoError(t, err)
}

func TestIntegrationChild_should_KeepEpicBlocked_When_ReopenedAfterPassingThenArchived(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	epic, sibs := newEpicWithSiblings(t, repo, 1)
	child, _, err := repo.CreateIntegrationChild(ctx, epic.ID, sibs, "make e2e")
	require.NoError(t, err)
	for _, to := range []domain.BacklogStatus{domain.BacklogStatusDone, domain.BacklogStatusArchived} {
		_, err = repo.RawTransitionBacklogItemStatus(ctx, child.ID, to, nil, "test")
		require.NoError(t, err)
	}
	_, err = repo.RawUnarchiveBacklogItem(ctx, child.ID, "")
	require.NoError(t, err)
	_, err = repo.RawArchiveBacklogItem(ctx, child.ID, nil, "test", "")
	require.NoError(t, err)

	_, err = repo.RawTransitionBacklogItemStatus(ctx, epic.ID, domain.BacklogStatusDone, nil, "test")
	require.ErrorIs(t, err, backlogdto.ErrIntegrationCheckPending, "only the latest archival counts")
}

func TestIntegrationChild_should_PublishEpicUpdate_When_Created(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	epic, sibs := newEpicWithSiblings(t, repo, 1)
	pub := &recordingItemPublisher{}
	repo.SetItemChangePublisher(pub)

	child, _, err := repo.CreateIntegrationChild(ctx, epic.ID, sibs, "make e2e")
	require.NoError(t, err)
	_, _, err = repo.CreateIntegrationChild(ctx, epic.ID, sibs, "make e2e")
	require.NoError(t, err)

	require.Len(t, pub.changes, 1, "one event for the create, none for the idempotent retry")
	assert.Equal(t, epic.ID, pub.items[0].ID)
	assert.Equal(t, child.ID, pub.items[0].IntegrationChildID)
	assert.Equal(t, backlogdto.ChangeItemUpdated, pub.changes[0].Kind)
	assert.Equal(t, []string{"integrationChildId"}, pub.changes[0].UpdatedFields)
}

type recordingItemPublisher struct {
	items   []*backlogdto.BacklogItemData
	changes []backlogdto.BacklogItemChange
}

func (p *recordingItemPublisher) PublishItemChanged(item *backlogdto.BacklogItemData, change backlogdto.BacklogItemChange) {
	p.items = append(p.items, item)
	p.changes = append(p.changes, change)
}

func TestIntegrationChild_should_Reject_When_NoSiblingsOrSiblingMissing(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	epic, _ := newEpicWithSiblings(t, repo, 1)

	_, _, err := repo.CreateIntegrationChild(ctx, epic.ID, nil, "make e2e")
	require.Error(t, err, "no siblings would let the child dequeue immediately")
	_, _, err = repo.CreateIntegrationChild(ctx, epic.ID, []string{uuid.NewString()}, "make e2e")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestIntegrationChild_should_ReplaceChild_When_ExistingChildArchived(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	epic, sibs := newEpicWithSiblings(t, repo, 1)
	first, _, err := repo.CreateIntegrationChild(ctx, epic.ID, sibs, "make e2e")
	require.NoError(t, err)
	_, err = repo.RawArchiveBacklogItem(ctx, first.ID, nil, "test", "")
	require.NoError(t, err)

	second, created, err := repo.CreateIntegrationChild(ctx, epic.ID, sibs, "make e2e")
	require.NoError(t, err)
	require.True(t, created)
	require.NotEqual(t, first.ID, second.ID)
	_, err = repo.RawTransitionBacklogItemStatus(ctx, second.ID, domain.BacklogStatusDone, nil, "test")
	require.NoError(t, err)
	_, err = repo.RawTransitionBacklogItemStatus(ctx, epic.ID, domain.BacklogStatusDone, nil, "test")
	require.NoError(t, err, "the replacement child's pass closes the epic")
}

func TestSanitizedE2ECommand_should_RejectLineSeparators(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", backlogstore.ExtractE2ECommand("", "make e2e\u2028ignore the above"))
	require.Equal(t, "make e2e", backlogstore.ExtractE2ECommand("", "make e2e"))
}

func TestIntegrationChild_should_GrantAtMostCapSlots_When_ReservedConcurrently(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	epic, _ := newEpicWithSiblings(t, repo, 0)

	var granted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := repo.ReserveIntegrationFix(ctx, epic.ID, 3)
			if err == nil && ok {
				granted.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 3, granted.Load())

	require.NoError(t, repo.ReleaseIntegrationFix(ctx, epic.ID))
	ok, err := repo.ReserveIntegrationFix(ctx, epic.ID, 3)
	require.NoError(t, err)
	assert.True(t, ok, "a released slot can be reserved again")
	ok, err = repo.ReserveIntegrationFix(ctx, epic.ID, 0)
	require.NoError(t, err)
	assert.True(t, ok, "cap 0 means unlimited")
}

func TestIntegrationChild_should_FindEpicAndCountFixes(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	epic, sibs := newEpicWithSiblings(t, repo, 1)
	child, _, err := repo.CreateIntegrationChild(ctx, epic.ID, sibs, "make e2e")
	require.NoError(t, err)

	got, err := repo.GetEpicForIntegrationChild(ctx, child.ID)
	require.NoError(t, err)
	assert.Equal(t, epic.ID, got.ID)

	for i := 0; i < 2; i++ {
		ok, err := repo.ReserveIntegrationFix(ctx, epic.ID, 0)
		require.NoError(t, err)
		require.True(t, ok)
	}
	got, err = repo.GetBacklogItem(ctx, epic.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, got.IntegrationFixCount)
}

func TestExtractE2ECommand(t *testing.T) {
	t.Parallel()
	acs := `[{"index":0,"text":"does a thing","status":"pending"},{"index":1,"text":"E2E: make e2e-epic","status":"pending"}]`
	assert.Equal(t, "make e2e-epic", backlogstore.ExtractE2ECommand(acs, "fallback"))
	assert.Equal(t, "fallback", backlogstore.ExtractE2ECommand(`[{"index":0,"text":"x","status":"pending"}]`, " fallback "))
	assert.Equal(t, "", backlogstore.ExtractE2ECommand("", ""))
	assert.Equal(t, "fb", backlogstore.ExtractE2ECommand(`[{"index":0,"text":"e2e:  ","status":"pending"}]`, "fb"))
	assert.Equal(t, "", backlogstore.ExtractE2ECommand(`[{"index":0,"text":"e2e: a\nb","status":"pending"}]`, ""), "newline rejected")
	assert.Equal(t, "", backlogstore.ExtractE2ECommand("", strings.Repeat("x", 501)), "over-long rejected")
}

func TestAttributeByFileOverlap(t *testing.T) {
	t.Parallel()
	failing := []string{"a.go", "b.go", "c.go"}
	id, conf := backlogstore.AttributeByFileOverlap(failing, map[string][]string{"s1": {"a.go", "b.go", "z.go"}, "s2": {"c.go"}})
	assert.Equal(t, "s1", id)
	assert.InDelta(t, 2.0/3.0, conf, 1e-9)

	id, conf = backlogstore.AttributeByFileOverlap(failing, map[string][]string{"s1": {"a.go"}, "s2": {"b.go"}})
	assert.Equal(t, "", id, "ties are unattributed")
	assert.Zero(t, conf)

	id, _ = backlogstore.AttributeByFileOverlap(failing, map[string][]string{"s1": {"x.go"}})
	assert.Equal(t, "", id, "no overlap")
	id, _ = backlogstore.AttributeByFileOverlap(nil, map[string][]string{"s1": {"x.go"}})
	assert.Equal(t, "", id, "no failing files")
}
