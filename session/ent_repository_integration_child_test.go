package session

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

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
	var got []string
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

func TestIntegrationChild_should_KeepBlockingEpicDone_When_ChildArchived(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()
	epic, sibs := newEpicWithSiblings(t, repo, 1)
	child, _, err := repo.CreateIntegrationChild(ctx, epic.ID, sibs, "make e2e")
	require.NoError(t, err)

	_, err = repo.RawArchiveBacklogItem(ctx, child.ID, nil, "test", "")
	require.NoError(t, err)
	_, err = repo.RawTransitionBacklogItemStatus(ctx, epic.ID, domain.BacklogStatusDone, nil, "test")
	require.ErrorIs(t, err, backlogdto.ErrIntegrationCheckPending, "an archived child never passed the e2e check")
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
