package session

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

func TestListItemSessionsBySessionUUIDs_should_returnNewestPerUUID_And_loadVerdict(t *testing.T) {
	t.Parallel()
	storage := newTestStorageForItemLink(t)
	ctx := context.Background()
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "batch"})
	require.NoError(t, err)

	review, err := storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: "u-review", SessionRole: instance.SessionRoleReview})
	require.NoError(t, err)
	require.NoError(t, storage.Repo.SaveReviewVerdict(ctx, review.ID, backlogdto.ReviewVerdictData{OverallOutcome: "PASS"}))
	_, err = storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: "u-other", SessionRole: instance.SessionRoleDiagnose})
	require.NoError(t, err)

	got, err := storage.ListItemSessionsBySessionUUIDs(ctx, []string{"u-review", "u-other", "u-missing"})
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.NotNil(t, got["u-review"].ReviewVerdict)
	assert.NotContains(t, got, "u-missing")

	empty, err := storage.ListItemSessionsBySessionUUIDs(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}
