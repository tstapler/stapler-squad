package services

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

func (h *blockerHarness) getItem() *sessionv1.BacklogItem {
	h.t.Helper()
	res, err := h.svc.GetBacklogItem(h.t.Context(), connect.NewRequest(&sessionv1.GetBacklogItemRequest{ItemId: h.itemID}))
	require.NoError(h.t, err)
	return res.Msg.Item
}

func holdKinds(item *sessionv1.BacklogItem) []sessionv1.PrHold_Kind {
	out := make([]sessionv1.PrHold_Kind, 0, len(item.PrHolds))
	for _, h := range item.PrHolds {
		out = append(out, h.Kind)
	}
	return out
}

func TestGetBacklogItem_ShouldFillPullRequestsAndHolds_OnlyWhenBacklogScopeOn(t *testing.T) {
	t.Parallel()
	h := newBlockerHarness(t, session.BacklogStatusPRPending)
	h.link(session.ItemPROwner(h.itemID), 21, session.PRStateMerged)
	h.link(session.ItemPROwner(h.itemID), 22, session.PRStateOpen)
	h.link(session.ItemPROwner(h.itemID), 23, session.PRStateUnknown)

	h.svc.cleanup.liveEnabled = func() bool { return false }
	off := h.getItem()
	assert.Empty(t, off.PullRequests, "scope off: today's response")
	assert.Empty(t, off.PrHolds)

	h.svc.cleanup.liveEnabled = func() bool { return true }
	on := h.getItem()
	assert.Len(t, on.PullRequests, 3)
	assert.ElementsMatch(t, []sessionv1.PrHold_Kind{sessionv1.PrHold_KIND_OPEN_PR, sessionv1.PrHold_KIND_UNKNOWN}, holdKinds(on))
}

func TestGetBacklogItem_ShouldNameSupersededRoundPR_AndLiveSession(t *testing.T) {
	t.Parallel()
	h := newBlockerHarness(t, session.BacklogStatusDone)
	h.svc.cleanup.liveEnabled = func() bool { return true }
	h.svc.SetLiveWorkProbe(&stubProbe{live: map[string]bool{"round-two": true}})
	// Round 1 links open #30; round 2 (the newest, live) links merged #31, so #30 is no
	// longer in the item's set but still holds round 1's worktree.
	h.link(session.SessionPROwner(h.uuid), 30, session.PRStateOpen)
	_, err := h.storage.CreateItemSession(t.Context(), session.ItemSessionData{ItemID: h.itemID, SessionUUID: "round-two", SessionRole: session.SessionRoleWork})
	require.NoError(t, err)
	h.link(session.SessionPROwner("round-two"), 31, session.PRStateMerged)

	item := h.getItem()

	kinds := holdKinds(item)
	assert.Contains(t, kinds, sessionv1.PrHold_KIND_SUPERSEDED_ROUND)
	assert.Contains(t, kinds, sessionv1.PrHold_KIND_SESSION_ACTIVE)
	for _, hold := range item.PrHolds {
		switch hold.Kind {
		case sessionv1.PrHold_KIND_SUPERSEDED_ROUND:
			assert.Equal(t, h.uuid, hold.OwnerSessionId)
			assert.EqualValues(t, 30, hold.Pr.Number)
		case sessionv1.PrHold_KIND_SESSION_ACTIVE:
			assert.EqualValues(t, 600, hold.IdleWindowSeconds)
		}
	}
}
