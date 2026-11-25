package instancestore

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
	"github.com/tstapler/stapler-squad/session/ent/session"
	entsessionpr "github.com/tstapler/stapler-squad/session/ent/sessionpr"
	"github.com/tstapler/stapler-squad/session/internal/entdbtest"
	"github.com/tstapler/stapler-squad/session/sessionpr"
)

// countingPorts wraps the real PR ports and counts invocations, so a test can
// assert they ran inside the transaction before the failed commit.
func countingPorts(upserts, deletes *atomic.Int32) Ports {
	return Ports{
		UpsertSessionPRSet: func(ctx context.Context, tx *ent.Tx, d domain.InstanceData) error {
			upserts.Add(1)
			return sessionpr.UpsertSessionPRSetTx(ctx, tx, d)
		},
		DeleteOwnerPRRows: func(ctx context.Context, tx *ent.Tx, o domain.PROwner) error {
			deletes.Add(1)
			return sessionpr.DeleteOwnerPRRows(ctx, tx, o)
		},
	}
}

func TestInstances_should_ReturnCommitErrorAndPersistNothing_When_CreateCommitFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	boom := errors.New("commit boom")
	failing, plain := entdbtest.NewClientFailingCommit(t, boom)
	var upserts, deletes atomic.Int32
	in := New(failing, countingPorts(&upserts, &deletes))

	err := in.Create(ctx, fullCreateData(t))

	require.ErrorIs(t, err, boom)
	assert.EqualValues(t, 1, upserts.Load(), "PR port ran before the commit")
	assert.Zero(t, plain.Session.Query().CountX(ctx))
	assert.Zero(t, plain.Worktree.Query().CountX(ctx))
	assert.Zero(t, plain.ClaudeSession.Query().CountX(ctx))
	assert.Zero(t, plain.SessionPR.Query().CountX(ctx))
}

func TestInstances_should_ReturnCommitErrorAndKeepSession_When_DeleteCommitFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	boom := errors.New("commit boom")
	failing, plain := entdbtest.NewClientFailingCommit(t, boom)
	require.NoError(t, newWithPRPorts(plain).Create(ctx, fullCreateData(t)))
	seedItemSession(t, plain, "uuid-full")
	var upserts, deletes atomic.Int32
	in := New(failing, countingPorts(&upserts, &deletes))

	err := in.Delete(ctx, "full")

	require.ErrorIs(t, err, boom)
	assert.EqualValues(t, 1, deletes.Load(), "owner-rows port ran before the commit")
	assert.Equal(t, 1, plain.Session.Query().Where(session.Title("full")).CountX(ctx))
	assert.Equal(t, 1, plain.Worktree.Query().CountX(ctx))
	assert.Equal(t, 1, plain.ClaudeSession.Query().CountX(ctx))
	assert.Equal(t, 1, plain.SessionPR.Query().Where(entsessionpr.SessionUUID("uuid-full")).CountX(ctx))
	assert.Equal(t, 1, plain.ItemSession.Query().Where(itemsession.ConversationUUID("")).CountX(ctx), "conversation stamp is not persisted")
}

func TestInstances_should_ReturnCommitErrorAndKeepOldRow_When_UpdateCommitFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	boom := errors.New("commit boom")
	failing, plain := entdbtest.NewClientFailingCommit(t, boom)
	plainIn := newWithPRPorts(plain)
	require.NoError(t, plainIn.Create(ctx, sectionData("t", "a")))
	before := loadFull(t, plainIn, "t")
	var upserts, deletes atomic.Int32
	in := New(failing, countingPorts(&upserts, &deletes))

	err := in.Update(ctx, sectionData("t", "bb"))

	require.ErrorIs(t, err, boom)
	assert.EqualValues(t, 1, upserts.Load(), "PR port ran before the commit")
	assert.Equal(t, before, loadFull(t, plainIn, "t"))
}

func TestInstances_should_ReturnCommitErrorAndKeepOldBlob_When_UpdateSessionArtifactsCommitFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	boom := errors.New("commit boom")
	failing, plain := entdbtest.NewClientFailingCommit(t, boom)
	require.NoError(t, newWithPRPorts(plain).Create(ctx, domain.InstanceData{Title: "art", Path: "/p", Program: "claude"}))

	err := New(failing, Ports{}).UpdateSessionArtifacts(ctx, "art", `{"new":true}`)

	require.ErrorIs(t, err, boom)
	assert.NotEqual(t, `{"new":true}`, plain.Session.Query().Where(session.Title("art")).OnlyX(ctx).SessionArtifacts)
}
