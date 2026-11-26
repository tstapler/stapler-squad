package instancestore

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/diffstats"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
	"github.com/tstapler/stapler-squad/session/ent/session"
	entsessionpr "github.com/tstapler/stapler-squad/session/ent/sessionpr"
	"github.com/tstapler/stapler-squad/session/ent/tag"
	"github.com/tstapler/stapler-squad/session/internal/entdbtest"
	"github.com/tstapler/stapler-squad/session/sessionpr"
)

func newWithPRPorts(c *ent.Client) *Instances {
	return New(c, Ports{UpsertSessionPRSet: sessionpr.UpsertSessionPRSetTx, DeleteOwnerPRRows: sessionpr.DeleteOwnerPRRows})
}

// fullCreateData populates every field Create persists, so a dropped write shows as a diff.
func fullCreateData(t *testing.T) domain.InstanceData {
	t.Helper()
	ref, err := domain.NewPRRef("", "own", "rep", 9)
	require.NoError(t, err)
	at := func(h int) time.Time { return time.Date(2026, 3, 4, h, 5, 6, 0, time.UTC) }
	archived := at(20)
	return domain.InstanceData{
		Title: "full", UUID: "uuid-full", Path: "/p/full", WorkingDir: "sub", Branch: "br",
		Status: domain.Paused, Height: 31, Width: 97, CreatedAt: at(1), UpdatedAt: at(2),
		AutoYes: true, AutoApprove: true, AutonomousMode: true, Prompt: "pr", InitialPrompt: "ip",
		InitialPromptSentAt: at(3), Program: "claude", ExistingWorktree: "/ex", IsExpanded: true, Pinned: true,
		Category: "cat", Note: "note", Tags: []string{"t-new", "t-old"},
		RuleTagProvenance: map[string]string{"t-new": "rule-1"}, SuppressedRuleTags: map[string]bool{"s1": true, "s2": false},
		SessionType: domain.SessionType("new_worktree"),
		GitHubPRURL: "https://github.com/own/rep/pull/9", GitHubPRNumber: 9, GitHubPRStatusTerminal: true,
		GitHubOwner: "own", GitHubRepo: "rep", GitHubHost: "github.com",
		TmuxPrefix: "pfx", TmuxTitle: "named-after", Backend: domain.ProcessManagerBackend("tymux"), TymuxSessionID: "ty-1",
		LastTerminalUpdate: at(4), LastMeaningfulOutput: at(5), LastOutputSignature: "sig",
		LastAddedToQueue: at(6), LastViewed: at(7), LastAcknowledged: at(8),
		MCPServerURL: "http://mcp", OneShot: true, Hidden: true, ArchivedAt: &archived,
		CreationProgressUpdatedAt: at(9), ProjectID: "proj",
		WorkflowID: "wf-1", PauseReason: "paused-why", ExitReason: "exit-why", LastUserResponse: at(12),
		ProcessingGraceUntil: at(13), LastPromptDetected: at(14), LastPromptSignature: "prompt-sig",
		Worktree:  domain.GitWorktreeData{RepoPath: "/repo", WorktreePath: "/wt", SessionName: "sn", BranchName: "bn", BaseCommitSHA: "base"},
		DiffStats: domain.DiffStatsData{Added: 11, Removed: 4, Content: "the diff"},
		ClaudeSession: domain.ClaudeSessionData{
			ConversationUUID: "conv-1", SquadSessionID: "sq-1", ProjectName: "pn", LastAttached: at(10),
			Settings: domain.ClaudeSettings{AutoReattach: true, PreferredSessionName: "pref", CreateNewOnMissing: true, ShowSessionSelector: true, SessionTimeoutMinutes: 15},
			Metadata: map[string]string{"k1": "v1", "k2": "v2"},
		},
		PRs: []domain.PRView{{Ref: ref, Role: domain.PRRolePrimary, AttachSource: domain.AttachSourceCreatedFromURL, AttachedAt: at(11)}},
	}
}

func TestInstances_should_PersistEveryColumnAndEdge_When_CreateCalled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	_, err := c.Project.Create().SetName("proj").Save(ctx)
	require.NoError(t, err)
	_, err = c.Tag.Create().SetName("t-old").Save(ctx)
	require.NoError(t, err)
	in := newWithPRPorts(c)
	data := fullCreateData(t)

	require.NoError(t, in.Create(ctx, data))

	got, err := in.GetWithOptions(ctx, "full", domain.LoadFull)
	require.NoError(t, err)
	want := data
	want.UpdatedAt = got.UpdatedAt // ent's updated_at default hook stamps the row at write time
	want.Tags = []string{"t-old", "t-new"}
	want.SuppressedRuleTags = map[string]bool{"s1": true} // false entries are not persisted
	want.PRs = nil                                        // PR links are stored in session_prs, not the instance mapper
	got.PRs = nil
	got.HasPRLinks, got.LegacyPRUnrepresented = false, false
	assert.Equal(t, want, *got)

	assert.Equal(t, 1, c.Tag.Query().Where(tag.Name("t-old")).CountX(ctx), "existing tag must be reused, not duplicated")
	assert.Equal(t, 2, c.Tag.Query().CountX(ctx))
	assert.Equal(t, 1, c.SessionPR.Query().Where(entsessionpr.SessionUUID("uuid-full")).CountX(ctx), "PR link written in the same tx")
}

func TestInstances_should_CreateWithoutOptionalRows_When_DataIsMinimal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)

	updated := time.Date(2026, 5, 6, 7, 8, 9, 0, time.UTC)
	require.NoError(t, in.Create(ctx, domain.InstanceData{Title: "min", Path: "/p", Program: "claude", Status: domain.Active, UpdatedAt: updated}))

	r := row(t, c, "min")
	assert.True(t, updated.Equal(r.UpdatedAt), "updated_at is written as given (no tags, so nothing re-stamps it): %v", r.UpdatedAt)
	assert.False(t, r.IsExpanded)
	assert.False(t, r.Pinned)
	assert.False(t, r.AutoYes)
	assert.Zero(t, c.Worktree.Query().CountX(ctx), "no worktree row without a repo path")
	assert.Equal(t, 1, c.DiffStats.Query().CountX(ctx), "diff stats row is always written")
	assert.Zero(t, c.ClaudeSession.Query().CountX(ctx), "no claude row without a conversation uuid")
	assert.Zero(t, c.Tag.Query().CountX(ctx))
}

func TestInstances_should_CreateWithoutProjectLink_When_ProjectNameUnknown(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)

	require.NoError(t, in.Create(ctx, domain.InstanceData{Title: "np", Path: "/p", Program: "claude", ProjectID: "ghost"}))

	got, err := in.GetWithOptions(ctx, "np", domain.LoadFull)
	require.NoError(t, err)
	assert.Empty(t, got.ProjectID)
}

func TestInstances_should_RollBackSessionRow_When_ALaterCreateStepFails(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := entdbtest.NewClient(t)
	c.ClaudeSession.Use(failHook(ent.OpCreate))
	in := newWithPRPorts(c)

	err := in.Create(ctx, fullCreateData(t))

	require.ErrorContains(t, err, "forced failure")
	assert.Zero(t, c.Session.Query().CountX(ctx), "session row must roll back with the failed tx")
	assert.Zero(t, c.Worktree.Query().CountX(ctx))
	assert.Zero(t, c.DiffStats.Query().CountX(ctx))
	assert.Zero(t, c.Tag.Query().CountX(ctx))
}

func TestInstances_should_RejectDuplicateTitle_When_CreateCalledTwice(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)
	d := domain.InstanceData{Title: "dup", Path: "/p", Program: "claude"}
	require.NoError(t, in.Create(ctx, d))

	require.Error(t, in.Create(ctx, d))
	assert.Equal(t, 1, c.Session.Query().CountX(ctx))
}

func failHook(op ent.Op) ent.Hook {
	return func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			if m.Op().Is(op) {
				return nil, errors.New("forced failure")
			}
			return next.Mutate(ctx, m)
		})
	}
}

func seedItemSession(t *testing.T, c *ent.Client, sessionUUID string) {
	t.Helper()
	_, err := itemSessionCreate(t, c, sessionUUID, "work").Save(context.Background())
	require.NoError(t, err)
}

func itemSessionCreate(t *testing.T, c *ent.Client, sessionUUID, role string) *ent.ItemSessionCreate {
	t.Helper()
	item, err := c.BacklogItem.Create().SetTitle("item-" + sessionUUID + role).SetStatus("idea").Save(context.Background())
	require.NoError(t, err)
	return c.ItemSession.Create().SetSessionUUID(sessionUUID).SetSessionRole(role).SetBacklogItemID(item.ID)
}

func TestInstances_should_RemoveSessionAndChildren_When_DeleteCalled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)
	require.NoError(t, in.Create(ctx, fullCreateData(t)))
	otherRef, err := domain.NewPRRef("", "own", "rep", 77)
	require.NoError(t, err)
	require.NoError(t, in.Create(ctx, domain.InstanceData{Title: "other", UUID: "uuid-other", Path: "/p", Program: "claude",
		Worktree: domain.GitWorktreeData{RepoPath: "/r", WorktreePath: "/r-wt", SessionName: "o", BranchName: "b", BaseCommitSHA: "s"}, Tags: []string{"t-new"},
		ClaudeSession: domain.ClaudeSessionData{ConversationUUID: "conv-o", Metadata: map[string]string{"a": "b"}},
		PRs:           []domain.PRView{{Ref: otherRef, Role: domain.PRRolePrimary, AttachSource: domain.AttachSourceCreatedFromURL}}}))
	sess := row(t, c, "full")
	_, err = c.Shell.Create().SetID("shell-1").SetSessionID(sess.ID).SetName("sh").SetTmuxSessionName("sh-tmux").Save(ctx)
	require.NoError(t, err)
	_, err = c.Shell.Create().SetID("shell-2").SetSessionID(row(t, c, "other").ID).SetName("sh2").SetTmuxSessionName("sh2-tmux").Save(ctx)
	require.NoError(t, err)
	seedItemSession(t, c, "uuid-other")

	require.NoError(t, in.Delete(ctx, "full"))

	assert.Zero(t, c.Session.Query().Where(session.Title("full")).CountX(ctx))
	assert.Equal(t, 1, c.Session.Query().Where(session.Title("other")).CountX(ctx), "only the named session is deleted")
	assert.Equal(t, 1, c.Worktree.Query().CountX(ctx), "only the other session's worktree remains")
	assert.Equal(t, 1, c.DiffStats.Query().Where(diffstats.HasSessionWith(session.Title("other"))).CountX(ctx))
	assert.Equal(t, 1, c.ClaudeSession.Query().CountX(ctx))
	assert.Equal(t, 1, c.ClaudeMetadata.Query().CountX(ctx), "only the other session's metadata remains")
	assert.Equal(t, 1, c.Shell.Query().CountX(ctx), "only the other session's shell remains")
	assert.Equal(t, 1, c.ItemSession.Query().Where(itemsession.SessionUUID("uuid-other"), itemsession.ConversationUUID("")).CountX(ctx), "other session's item_session is not stamped")
	assert.Equal(t, 1, c.SessionPR.Query().Where(entsessionpr.SessionUUID("uuid-other")).CountX(ctx), "other session's PR rows remain")
	assert.Zero(t, c.SessionPR.Query().Where(entsessionpr.SessionUUID("uuid-full")).CountX(ctx))
	assert.Equal(t, 2, c.Tag.Query().CountX(ctx), "tags themselves survive; only the association is cleared")
	assert.Zero(t, c.Tag.Query().Where(tag.Name("t-old"), tag.HasSessions()).CountX(ctx), "tag link to the deleted session is cleared")
	assert.Equal(t, 1, c.Session.Query().Where(session.Title("other")).QueryTags().CountX(ctx))
}

func TestInstances_should_StampConversationUUID_When_DeleteRemovesClaudeSession(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)
	require.NoError(t, in.Create(ctx, fullCreateData(t)))
	seedItemSession(t, c, "uuid-full")
	seedItemSession(t, c, "uuid-unrelated")
	_, err := itemSessionCreate(t, c, "uuid-full", "review").SetConversationUUID("already").Save(ctx)
	require.NoError(t, err)

	require.NoError(t, in.Delete(ctx, "full"))

	assert.Equal(t, 1, c.ItemSession.Query().Where(itemsession.SessionUUID("uuid-full"), itemsession.SessionRole("work"), itemsession.ConversationUUID("conv-1")).CountX(ctx), "blank row is stamped")
	assert.Equal(t, 1, c.ItemSession.Query().Where(itemsession.SessionUUID("uuid-full"), itemsession.ConversationUUID("already")).CountX(ctx), "an existing conversation uuid is not overwritten")
	assert.Equal(t, 1, c.ItemSession.Query().Where(itemsession.SessionUUID("uuid-unrelated"), itemsession.ConversationUUID("")).CountX(ctx), "other sessions' rows are untouched")
}

func TestInstances_should_NotStamp_When_SessionHasNoConversation(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)
	require.NoError(t, in.Create(ctx, domain.InstanceData{Title: "nc", UUID: "uuid-nc", Path: "/p", Program: "aider"}))
	seedItemSession(t, c, "uuid-nc")

	require.NoError(t, in.Delete(ctx, "nc"))

	assert.Equal(t, 1, c.ItemSession.Query().Where(itemsession.ConversationUUID("")).CountX(ctx))
}

func TestInstances_should_NotStampBlankSessionUUIDRows_When_DeletedSessionHasNoUUID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)
	require.NoError(t, in.Create(ctx, domain.InstanceData{Title: "nu", Path: "/p", Program: "claude",
		ClaudeSession: domain.ClaudeSessionData{ConversationUUID: "conv-nu"}}))
	seedItemSession(t, c, "")

	require.NoError(t, in.Delete(ctx, "nu"))

	assert.Equal(t, 1, c.ItemSession.Query().Where(itemsession.ConversationUUID("")).CountX(ctx))
}

func TestInstances_should_ReturnError_When_DeleteTitleNotFound(t *testing.T) {
	t.Parallel()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)

	err := in.Delete(context.Background(), "ghost")

	require.ErrorContains(t, err, "failed to find session")
	assert.True(t, ent.IsNotFound(errors.Unwrap(err)))
}

func TestInstances_should_RollBackEverything_When_FinalSessionDeleteFails(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)
	require.NoError(t, in.Create(ctx, fullCreateData(t)))
	seedItemSession(t, c, "uuid-full")
	c.Session.Use(failHook(ent.OpDeleteOne))

	err := in.Delete(ctx, "full")

	require.ErrorContains(t, err, "forced failure")
	assert.Equal(t, 1, c.Session.Query().Where(session.Title("full")).CountX(ctx))
	assert.Equal(t, 1, c.Worktree.Query().CountX(ctx), "worktree delete rolled back")
	assert.Equal(t, 1, c.DiffStats.Query().CountX(ctx))
	assert.Equal(t, 1, c.ClaudeSession.Query().CountX(ctx))
	assert.Equal(t, 2, c.ClaudeMetadata.Query().CountX(ctx))
	assert.Equal(t, 1, c.SessionPR.Query().Where(entsessionpr.SessionUUID("uuid-full")).CountX(ctx), "PR rows delete rolled back")
	assert.Equal(t, 2, c.Session.Query().Where(session.Title("full")).QueryTags().CountX(ctx), "tag association rolled back")
	assert.Equal(t, 1, c.ItemSession.Query().Where(itemsession.ConversationUUID("")).CountX(ctx), "stamp rolled back with the tx")
}

func TestInstances_should_ReturnErrorAndRollBack_When_UpsertSessionPRSetPortFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	boom := errors.New("pr port failed")
	in := New(c, Ports{UpsertSessionPRSet: func(context.Context, *ent.Tx, domain.InstanceData) error { return boom }})

	err := in.Create(ctx, fullCreateData(t))

	require.ErrorIs(t, err, boom)
	assert.Zero(t, c.Session.Query().CountX(ctx))
	assert.Zero(t, c.Worktree.Query().CountX(ctx))
	assert.Zero(t, c.ClaudeSession.Query().CountX(ctx))
}

func TestInstances_should_ReturnErrorAndRollBack_When_DeleteOwnerPRRowsPortFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	require.NoError(t, newWithPRPorts(c).Create(ctx, fullCreateData(t)))
	boom := errors.New("owner rows port failed")
	in := New(c, Ports{DeleteOwnerPRRows: func(context.Context, *ent.Tx, domain.PROwner) error { return boom }})

	err := in.Delete(ctx, "full")

	require.ErrorIs(t, err, boom)
	assert.Equal(t, 1, c.Session.Query().Where(session.Title("full")).CountX(ctx))
	assert.Equal(t, 1, c.Worktree.Query().CountX(ctx), "earlier deletes rolled back")
	assert.Equal(t, 1, c.ClaudeSession.Query().CountX(ctx))
	assert.Equal(t, 1, c.SessionPR.Query().CountX(ctx))
}

func TestInstances_should_ReturnErrorAndRollBack_When_ItemSessionStampFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)
	require.NoError(t, in.Create(ctx, fullCreateData(t)))
	seedItemSession(t, c, "uuid-full")
	c.ItemSession.Use(failHook(ent.OpUpdate))

	err := in.Delete(ctx, "full")

	require.ErrorContains(t, err, "failed to stamp conversation uuid")
	assert.Equal(t, 1, c.Session.Query().Where(session.Title("full")).CountX(ctx))
	assert.Equal(t, 1, c.Worktree.Query().CountX(ctx), "worktree delete before the stamp rolled back")
	assert.Equal(t, 1, c.DiffStats.Query().CountX(ctx))
}
