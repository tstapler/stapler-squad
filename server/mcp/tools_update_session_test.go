package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// TestUpdateSession_Title_PublishesSnapshot regression-tests the raw
// `inst.Title = title` write the handler used to do: that bypassed
// SetTitleDirect's snapshot.Store call, so Snapshot().Title (the race-free
// read path documented in .claude/rules/instance-lock-free-reads.md) would
// keep returning the stale title even though the raw field had already
// changed. Routing through SetTitleDirect keeps both in sync.
func TestUpdateSession_Title_PublishesSnapshot(t *testing.T) {
	const renamedTitle = "tstapler/stapler-squad#789"

	occupant := newLiveOccupant(t, "workflow-name-before", t.TempDir())
	lh := newWorktreeGuardHandlers(t, occupant)

	res, err := lh.updateSession(context.Background(), makeToolReq(map[string]interface{}{
		"session_id": "workflow-name-before",
		"title":      renamedTitle,
	}))
	require.NoError(t, err)
	m := parseResult(t, res)
	require.True(t, m["success"].(bool), "expected success, got: %+v", m)

	assert.Equal(t, renamedTitle, occupant.Title)
	assert.Equal(t, renamedTitle, occupant.Snapshot().Title,
		"Snapshot() must reflect the rename -- a raw field write would leave this stale")
}

// TestUpdateSession_Title_RejectsCollisionWithExistingSession verifies the
// pre-check every other title-setting MCP tool (create_session,
// create_session_for_pr) already runs: since session_id/title is the lookup
// key FindLiveInstance/MatchesID use, renaming one live session to another's
// title would make one of them unaddressable.
func TestUpdateSession_Title_RejectsCollisionWithExistingSession(t *testing.T) {
	const titleA = "session-a"
	const titleB = "session-b"

	occupantA := newLiveOccupant(t, titleA, t.TempDir())
	occupantB := newLiveOccupant(t, titleB, t.TempDir())
	occupantB.Program = "claude"
	lh := newWorktreeGuardHandlers(t, occupantA, occupantB)
	// titleCollisionResult checks the persisted store (ListInstanceData), matching
	// create_session/create_session_for_pr's identical pre-check -- so occupantB
	// must actually be saved, not just registered on the in-memory poller.
	require.NoError(t, lh.store.SaveInstances([]*session.Instance{occupantB}))

	res, err := lh.updateSession(context.Background(), makeToolReq(map[string]interface{}{
		"session_id": titleA,
		"title":      titleB,
	}))
	require.NoError(t, err)
	m := parseResult(t, res)

	assert.Equal(t, string(ErrInvalidArgument), m["error"].(map[string]interface{})["code"].(string))
	assert.Equal(t, titleA, occupantA.Title, "rejected rename must not mutate the title")
}
