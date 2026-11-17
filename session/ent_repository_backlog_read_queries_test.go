package session

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

func TestGetRecentReviewVerdictSummaries_should_ExcludeSessionsWithoutVerdictAndRespectLimit(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	item, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "verdicts", Status: string(domain.BacklogStatusIdea)})
	require.NoError(t, err)

	itemUUID := uuid.MustParse(item.ID)
	base := time.Now().Add(-time.Hour)
	newSession := func(uuidStr, role string, at time.Time) string {
		is, err := repo.Client.ItemSession.Create().SetSessionUUID(uuidStr).SetSessionRole(role).SetBacklogItemID(itemUUID).SetCreatedAt(at).Save(ctx)
		require.NoError(t, err)
		return is.ID.String()
	}
	for i := 0; i < 3; i++ {
		id := newSession(fmt.Sprintf("rv-%d", i), instance.SessionRoleReview, base.Add(time.Duration(i)*time.Minute))
		require.NoError(t, repo.SaveReviewVerdict(ctx, id, backlogdto.ReviewVerdictData{OverallOutcome: "PASS", Summary: fmt.Sprintf("v%d", i)}))
	}
	// A newer session with no verdict must not appear (and must not use up the limit).
	newSession("no-verdict", instance.SessionRoleWork, base.Add(time.Hour))

	got, err := repo.GetRecentReviewVerdictSummaries(ctx, item.ID, 10)
	require.NoError(t, err)
	require.Len(t, got, 3, "the session without a verdict is excluded")
	assert.Equal(t, []string{"v2", "v1", "v0"}, []string{got[0].Summary, got[1].Summary, got[2].Summary}, "most recent first")

	limited, err := repo.GetRecentReviewVerdictSummaries(ctx, item.ID, 2)
	require.NoError(t, err)
	require.Len(t, limited, 2, "limit is applied after excluding verdict-less sessions")
	assert.Equal(t, "v2", limited[0].Summary)
}

func TestGetItemSession_should_ReturnNotFound_When_IDMissingOrMalformed(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	_, err := repo.GetItemSession(ctx, uuid.NewString())
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotFound)

	_, err = repo.GetItemSession(ctx, "not-a-uuid")
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}

func TestListBacklogItemDependencies_should_OrderEdgesByCreationTimeAscending(t *testing.T) {
	t.Parallel()
	repo, cleanup := createTestEntRepository(t)
	defer cleanup()
	ctx := context.Background()

	mk := func(title string) string {
		it, err := repo.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: title, Status: string(domain.BacklogStatusIdea)})
		require.NoError(t, err)
		return it.ID
	}
	center, b1, b2, d1, d2 := mk("center"), mk("blocker-1"), mk("blocker-2"), mk("dependent-1"), mk("dependent-2")
	edges := []backlogdto.BacklogItemDependencyEdge{
		{BlockerID: b1, BlockedID: center}, {BlockerID: b2, BlockedID: center},
		{BlockerID: center, BlockedID: d1}, {BlockerID: center, BlockedID: d2},
	}
	for _, e := range edges {
		require.NoError(t, repo.AddBacklogItemDependency(ctx, e))
	}

	blockedBy, blocking, err := repo.ListBacklogItemDependencies(ctx, center)
	require.NoError(t, err)
	require.Len(t, blockedBy, 2)
	require.Len(t, blocking, 2)
	assert.Equal(t, []string{"blocker-1", "blocker-2"}, []string{blockedBy[0].Title, blockedBy[1].Title})
	assert.Equal(t, []string{"dependent-1", "dependent-2"}, []string{blocking[0].Title, blocking[1].Title})
}
