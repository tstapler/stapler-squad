package backlogsvc

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// @feature backlog:multi-pr
// I-58 (AttachSessionToItem path): the earlier round's PR stays in the item's PR
// set until the new work session links one, leaves once it does, and does not
// return when the new session's only link is detached. The reopen, stale-respawn
// and bounce paths create their work ItemSession through the same
// CreateItemSession(work) call, which these assertions exercise at the data level.
func TestRoundMembership_ShouldHoldThenSwitch_WhenAttachSessionToItemCreatesWorkSession(t *testing.T) {
	t.Parallel()
	storage, repo := createTestStorageWithRepo(t)
	svc := NewBacklogService(storage, nil, nil, nil, nil, nil)
	ctx := t.Context()

	created, err := svc.CreateBacklogItem(ctx, connect.NewRequest(&sessionv1.CreateBacklogItemRequest{
		Title:              "round membership",
		AcceptanceCriteria: []*sessionv1.AcCriterion{{Index: 0, Text: "x", Status: "pending"}},
		SkipTriage:         true, SkipPlanning: true,
	}))
	require.NoError(t, err)
	itemID := created.Msg.Item.Id
	_, err = storage.RawTransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusReady, nil, session.TriggeredBySystem)
	require.NoError(t, err)

	ref := func(n int) session.PRRef {
		r, rerr := session.NewPRRef("github.com", "acme", "app", n)
		require.NoError(t, rerr)
		return r
	}
	link := func(owner session.PROwner, n int) {
		res, lerr := repo.UpsertSessionPR(ctx, owner, ref(n), session.PRLink{
			Role: session.PRRolePrimary, Source: session.AttachSourceManual,
			Initial: session.PRInitial{State: session.PRStateOpen},
		})
		require.NoError(t, lerr)
		require.False(t, res.Tombstoned)
	}
	numbers := func() []int {
		prs, perr := repo.ItemPRsByID(ctx, itemID)
		require.NoError(t, perr)
		out := make([]int, 0, len(prs))
		for _, v := range prs {
			out = append(out, v.Ref.Number())
		}
		return out
	}

	s1 := uuid.New().String()
	_, err = storage.CreateItemSession(ctx, session.ItemSessionData{ItemID: itemID, SessionUUID: s1, SessionRole: session.SessionRoleWork})
	require.NoError(t, err)
	link(session.SessionPROwner(s1), 30)

	s2 := uuid.New().String()
	_, err = svc.AttachSessionToItem(ctx, connect.NewRequest(&sessionv1.AttachSessionToItemRequest{ItemId: itemID, SessionUuid: s2}))
	require.NoError(t, err)
	assert.Equal(t, []int{30}, numbers(), "(a) earlier round stays until the new session links")

	link(session.SessionPROwner(s2), 31)
	assert.Equal(t, []int{31}, numbers(), "(b) earlier round leaves once the new session links")

	_, err = repo.DeleteSessionPR(ctx, session.SessionPROwner(s2), ref(31))
	require.NoError(t, err)
	assert.Empty(t, numbers(), "(c) a detached latest round does not resurrect the earlier one")
}
