package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

func seedItemDirectPR(t *testing.T, fix *forkTestFixture, status session.BacklogStatus, number int) (string, session.PRRef) {
	t.Helper()
	item, err := fix.storage.CreateBacklogItem(context.Background(), session.BacklogItemData{Title: "detach item", Status: string(status)})
	require.NoError(t, err)
	ref, err := session.NewPRRef("", "acme", "app", number)
	require.NoError(t, err)
	_, err = fix.storage.AttachItemPR(context.Background(), item.ID, ref,
		session.PRLink{Role: session.PRRolePrimary, Source: session.AttachSourceManual, Initial: session.PRInitial{State: session.PRStateUnknown}})
	require.NoError(t, err)
	return item.ID, ref
}

func TestDetachItemPullRequest_ShouldDeleteLinkAndTombstone_ForItemInAnyStatus(t *testing.T) {
	t.Parallel()
	for _, status := range []session.BacklogStatus{session.BacklogStatus("in_progress"), session.BacklogStatus("done")} {
		t.Run(string(status), func(t *testing.T) {
			t.Parallel()
			fix := setupForkTestFixture(t)
			defer fix.cleanup()
			itemID, ref := seedItemDirectPR(t, fix, status, 41)

			_, err := fix.svc.DetachItemPullRequest(context.Background(), connect.NewRequest(
				&sessionv1.DetachItemPullRequestRequest{ItemId: itemID, Pr: protoRef(41)}))
			require.NoError(t, err)

			guards, err := fix.storage.ListItemGuardViewsByItemIDs(context.Background(), []string{itemID})
			require.NoError(t, err)
			assert.Empty(t, guards[itemID].Direct, "link gone, so the verdict and the sweeper stop counting it")

			res, err := fix.storage.AttachItemPR(context.Background(), itemID, ref,
				session.PRLink{Role: session.PRRolePrimary, Source: session.AttachSourceBranchDiscovery})
			require.NoError(t, err)
			assert.True(t, res.Tombstoned, "an automatic re-attach is blocked by the item tombstone")
		})
	}
}

func TestDetachItemPullRequest_ShouldNotTouchSessionLinks(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	itemID, _ := seedItemDirectPR(t, fix, session.BacklogStatus("in_progress"), 42)
	addLivePRSession(t, fix, "u1", "uuid-u1", prSpec{42, session.PRStateOpen, false})

	_, err := fix.svc.DetachItemPullRequest(context.Background(), connect.NewRequest(
		&sessionv1.DetachItemPullRequestRequest{ItemId: itemID, Pr: protoRef(42)}))
	require.NoError(t, err)

	views, err := fix.storage.ListSessionPRViewsByUUIDs(context.Background(), []string{"uuid-u1"})
	require.NoError(t, err)
	assert.Len(t, views["uuid-u1"], 1, "the session keeps its own link to the same PR")
}

func TestDetachItemPullRequest_ShouldRejectBadRequests(t *testing.T) {
	t.Parallel()
	fix := setupForkTestFixture(t)
	defer fix.cleanup()
	itemID, _ := seedItemDirectPR(t, fix, session.BacklogStatus("in_progress"), 43)
	call := func(r *sessionv1.DetachItemPullRequestRequest) connect.Code {
		_, err := fix.svc.DetachItemPullRequest(context.Background(), connect.NewRequest(r))
		require.Error(t, err)
		return connect.CodeOf(err)
	}
	assert.Equal(t, connect.CodeInvalidArgument, call(&sessionv1.DetachItemPullRequestRequest{Pr: protoRef(43)}))
	assert.Equal(t, connect.CodeInvalidArgument, call(&sessionv1.DetachItemPullRequestRequest{ItemId: itemID}))
	assert.Equal(t, connect.CodeFailedPrecondition, call(&sessionv1.DetachItemPullRequestRequest{ItemId: itemID, Pr: protoRef(99)}))
}
