package instancestore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/internal/entdbtest"
)

func seedSession(t *testing.T, c *ent.Client, title, uuid string, status int, tags ...string) {
	t.Helper()
	ctx := context.Background()
	s, err := c.Session.Create().SetTitle(title).SetUUID(uuid).SetPath("/p/" + title).SetStatus(status).SetProgram("claude").Save(ctx)
	require.NoError(t, err)
	_, err = c.Worktree.Create().SetSessionID(s.ID).SetRepoPath("/repo").SetWorktreePath("/wt").
		SetSessionName(title).SetBranchName("b").SetBaseCommitSha("sha").Save(ctx)
	require.NoError(t, err)
	_, err = c.DiffStats.Create().SetSessionID(s.ID).SetAdded(7).SetRemoved(3).SetContent("diff").Save(ctx)
	require.NoError(t, err)
	for _, name := range tags {
		tg, err := c.Tag.Create().SetName(name).Save(ctx)
		require.NoError(t, err)
		_, err = c.Session.UpdateOneID(s.ID).AddTagIDs(tg.ID).Save(ctx)
		require.NoError(t, err)
	}
	cs, err := c.ClaudeSession.Create().SetSessionID(s.ID).SetClaudeSessionID("conv-" + title).Save(ctx)
	require.NoError(t, err)
	_, err = c.ClaudeMetadata.Create().SetClaudeSessionID(cs.ID).SetKey("k").SetValue("v").Save(ctx)
	require.NoError(t, err)
}

func TestInstances_should_LoadOnlyRequestedEdges_When_LoadOptionsVary(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	seedSession(t, c, "a", "uuid-a", int(domain.Active), "t1", "t2")
	in := New(c, Ports{})

	tests := []struct {
		name string
		opts domain.LoadOptions
		want func(t *testing.T, d *domain.InstanceData)
	}{
		{"minimal loads no child edges", domain.LoadMinimal, func(t *testing.T, d *domain.InstanceData) {
			assert.Empty(t, d.Worktree.RepoPath)
			assert.Zero(t, d.DiffStats.Added)
			assert.Empty(t, d.DiffStats.Content)
			assert.Empty(t, d.Tags)
			assert.Empty(t, d.ClaudeSession.ConversationUUID)
		}},
		{"worktree only", domain.LoadOptions{LoadWorktree: true}, func(t *testing.T, d *domain.InstanceData) {
			assert.Equal(t, "/repo", d.Worktree.RepoPath)
			assert.Zero(t, d.DiffStats.Added)
			assert.Empty(t, d.Tags)
			assert.Empty(t, d.ClaudeSession.ConversationUUID)
		}},
		{"diff stats only", domain.LoadOptions{LoadDiffStats: true}, func(t *testing.T, d *domain.InstanceData) {
			assert.Equal(t, 7, d.DiffStats.Added)
			assert.Empty(t, d.Worktree.RepoPath)
		}},
		{"diff content implies diff stats", domain.LoadOptions{LoadDiffContent: true}, func(t *testing.T, d *domain.InstanceData) {
			assert.Equal(t, 7, d.DiffStats.Added)
			assert.Equal(t, "diff", d.DiffStats.Content)
		}},
		{"tags only", domain.LoadOptions{LoadTags: true}, func(t *testing.T, d *domain.InstanceData) {
			assert.ElementsMatch(t, []string{"t1", "t2"}, d.Tags)
			assert.Empty(t, d.Worktree.RepoPath)
		}},
		{"claude session loads its metadata", domain.LoadOptions{LoadClaudeSession: true}, func(t *testing.T, d *domain.InstanceData) {
			assert.Equal(t, "conv-a", d.ClaudeSession.ConversationUUID)
			assert.Equal(t, map[string]string{"k": "v"}, d.ClaudeSession.Metadata)
			assert.Empty(t, d.Tags)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			d, err := in.GetWithOptions(ctx, "a", tc.opts)
			require.NoError(t, err)
			assert.Equal(t, "a", d.Title)
			tc.want(t, d)
		})
	}
}

func TestInstances_should_AlwaysLoadProject_When_AnyLoadOptions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	p, err := c.Project.Create().SetName("proj").Save(ctx)
	require.NoError(t, err)
	_, err = c.Session.Create().SetTitle("a").SetPath("/p").SetStatus(0).SetProgram("claude").SetProjectID(p.ID).Save(ctx)
	require.NoError(t, err)

	d, err := New(c, Ports{}).GetWithOptions(ctx, "a", domain.LoadMinimal)
	require.NoError(t, err)
	assert.Equal(t, "proj", d.ProjectID)
}

func TestInstances_should_FilterAndLoadListEdges_When_ListVariantsCalled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	seedSession(t, c, "a", "uuid-a", int(domain.Active), "t1")
	seedSession(t, c, "b", "uuid-b", int(domain.Paused), "t2")
	in := New(c, Ports{})

	all, err := in.List(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)
	for _, d := range all {
		assert.Equal(t, "/repo", d.Worktree.RepoPath, "list preset loads worktree")
		assert.NotEmpty(t, d.Tags, "list preset loads tags")
		assert.NotEmpty(t, d.ClaudeSession.ConversationUUID, "list preset loads claude session")
		assert.Zero(t, d.DiffStats.Added, "list preset skips diff stats")
	}

	byStatus, err := in.ListByStatus(ctx, domain.Paused)
	require.NoError(t, err)
	require.Len(t, byStatus, 1)
	assert.Equal(t, "b", byStatus[0].Title)

	byTag, err := in.ListByTag(ctx, "t1")
	require.NoError(t, err)
	require.Len(t, byTag, 1)
	assert.Equal(t, "a", byTag[0].Title)

	byStatusOpts, err := in.ListByStatusWithOptions(ctx, domain.Active, domain.LoadFull)
	require.NoError(t, err)
	require.Len(t, byStatusOpts, 1)
	assert.Equal(t, 7, byStatusOpts[0].DiffStats.Added)

	byTagOpts, err := in.ListByTagWithOptions(ctx, "t2", domain.LoadMinimal)
	require.NoError(t, err)
	require.Len(t, byTagOpts, 1)
	assert.Equal(t, "b", byTagOpts[0].Title)

	allOpts, err := in.ListWithOptions(ctx, domain.LoadMinimal)
	require.NoError(t, err)
	assert.Len(t, allOpts, 2)
}

func TestInstances_should_ResolveByTitleOrUUID_When_FindByIDWithOptions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	seedSession(t, c, "a", "uuid-a", 0)
	// uuid has no uniqueness constraint: an empty id must not match a session that never got one.
	_, err := c.Session.Create().SetTitle("no-uuid").SetPath("/p").SetStatus(0).SetProgram("claude").Save(ctx)
	require.NoError(t, err)
	in := New(c, Ports{})

	byTitle, err := in.FindByIDWithOptions(ctx, "a", domain.LoadMinimal)
	require.NoError(t, err)
	assert.Equal(t, "a", byTitle.Title)
	byUUID, err := in.FindByIDWithOptions(ctx, "uuid-a", domain.LoadMinimal)
	require.NoError(t, err)
	assert.Equal(t, "a", byUUID.Title)

	_, err = in.FindByIDWithOptions(ctx, "", domain.LoadMinimal)
	assert.ErrorIs(t, err, domain.ErrInstanceDataNotFound)
	_, err = in.FindByIDWithOptions(ctx, "nope", domain.LoadMinimal)
	assert.ErrorIs(t, err, domain.ErrInstanceDataNotFound)
}

func TestInstances_should_ReadArtifactsAndConversationUUID_When_Stored(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	seedSession(t, c, "a", "uuid-a", 0)
	_, err := c.Session.Create().SetTitle("bare").SetUUID("uuid-bare").SetPath("/p").SetStatus(0).SetProgram("claude").Save(ctx)
	require.NoError(t, err)
	_, err = c.Session.Update().Where().SetSessionArtifacts(`{}`).Save(ctx)
	require.NoError(t, err)
	in := New(c, Ports{})

	blob, err := in.GetSessionArtifacts(ctx, "a")
	require.NoError(t, err)
	assert.Equal(t, `{}`, blob)

	all, err := in.GetAllSessionArtifacts(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	parsed, err := in.GetSessionArtifactsByUUID(ctx, "uuid-a")
	require.NoError(t, err)
	assert.NotNil(t, parsed)

	got, err := in.GetClaudeConversationUUIDBySessionUUID(ctx, "a")
	require.NoError(t, err)
	assert.Equal(t, "conv-a", got)
	got, err = in.GetClaudeConversationUUIDBySessionUUID(ctx, "bare")
	require.NoError(t, err)
	assert.Empty(t, got)
	_, err = in.GetClaudeConversationUUIDBySessionUUID(ctx, "missing")
	assert.ErrorIs(t, err, domain.ErrNotFound)

	_, err = in.Get(ctx, "missing")
	assert.Error(t, err)
}
