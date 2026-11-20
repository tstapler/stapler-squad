package instancestore

import (
	"context"
	"errors"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/claudemetadata"
	entsessionpr "github.com/tstapler/stapler-squad/session/ent/sessionpr"
	"github.com/tstapler/stapler-squad/session/ent/tag"
	"github.com/tstapler/stapler-squad/session/internal/entdbtest"
	"github.com/tstapler/stapler-squad/session/sessionpr"
)

// sectionData is a session carrying one variant of every child-table section (worktree,
// diff stats, tags, Claude row + metadata), so a test can tell which variant a row holds.
func sectionData(title, variant string) domain.InstanceData {
	return domain.InstanceData{
		Title: title, UUID: "u-" + title, Path: "/p/" + variant, Program: "claude", Status: domain.Active,
		UpdatedAt: seedTime, Tags: []string{"tag-" + variant},
		Worktree:  domain.GitWorktreeData{RepoPath: "/repo-" + variant, WorktreePath: "/wt-" + variant, SessionName: "sn-" + variant, BranchName: "bn-" + variant, BaseCommitSHA: "sha-" + variant},
		DiffStats: domain.DiffStatsData{Added: len(variant), Removed: 2 * len(variant), Content: "diff-" + variant},
		ClaudeSession: domain.ClaudeSessionData{
			ConversationUUID: "conv-" + variant, SquadSessionID: "sq-" + variant, ProjectName: "pn-" + variant,
			LastAttached: time.Date(2026, 1, len(variant), 3, 4, 5, 0, time.UTC),
			Settings: domain.ClaudeSettings{
				AutoReattach: variant == "bb", CreateNewOnMissing: variant == "a", ShowSessionSelector: variant == "ccc",
				PreferredSessionName: "pref-" + variant, SessionTimeoutMinutes: len(variant),
			},
			Metadata: map[string]string{"k-" + variant: "v-" + variant},
		},
	}
}

func loadFull(t *testing.T, in *Instances, title string) domain.InstanceData {
	t.Helper()
	got, err := in.GetWithOptions(context.Background(), title, domain.LoadFull)
	require.NoError(t, err)
	return *got
}

// fullUpdateData is fullCreateData plus the columns only Update writes.
func fullUpdateData(t *testing.T) domain.InstanceData {
	t.Helper()
	data := fullCreateData(t)
	data.WorkflowID, data.PauseReason, data.ExitReason = "wf-1", "pause-r", "exit-r"
	data.LastUserResponse = time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	data.ProcessingGraceUntil = time.Date(2026, 3, 4, 13, 0, 0, 0, time.UTC)
	data.LastPromptDetected = time.Date(2026, 3, 4, 14, 0, 0, 0, time.UTC)
	data.LastPromptSignature = "psig"
	return data
}

func TestInstances_should_PersistEveryColumnAndEdge_When_UpdateCalled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	_, err := c.Project.Create().SetName("proj").Save(ctx)
	require.NoError(t, err)
	_, err = c.Tag.Create().SetName("t-old").Save(ctx)
	require.NoError(t, err)
	in := newWithPRPorts(c)
	seed := domain.InstanceData{Title: "full", UUID: "uuid-old", Path: "/seed", Program: "seed", Status: domain.Active, CreatedAt: seedTime, UpdatedAt: seedTime, AutoApprove: true, Hidden: true}
	require.NoError(t, in.Create(ctx, seed))

	data := fullUpdateData(t)
	data.AutoApprove, data.Hidden = false, false // the seed holds the opposite of AutoYes/OneShot, so a swapped pair shows

	require.NoError(t, in.Update(ctx, data))

	got := loadFull(t, in, "full")
	want := data
	want.CreatedAt = got.CreatedAt // Update never rewrites created_at
	want.UpdatedAt = got.UpdatedAt // the tag edge write re-stamps updated_at
	want.Tags = []string{"t-old", "t-new"}
	want.SuppressedRuleTags = map[string]bool{"s1": true}
	want.PRs = nil
	got.PRs = nil
	got.HasPRLinks, got.LegacyPRUnrepresented = false, false
	assert.True(t, seedTime.Equal(got.CreatedAt))
	assert.Equal(t, want, got)
	assert.Equal(t, 1, c.SessionPR.Query().Where(entsessionpr.SessionUUID("uuid-full")).CountX(ctx), "PR link written in the same tx")
	assert.Equal(t, 2, c.Tag.Query().CountX(ctx), "existing tag reused, new one created")
}

func TestInstances_should_StampGivenUpdatedAt_When_NoSectionRewritesTags(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)
	require.NoError(t, in.Create(ctx, sectionData("ua", "a")))
	given := time.Date(2026, 8, 9, 10, 11, 12, 0, time.UTC)
	data := sectionData("ua", "b")
	data.UpdatedAt = given

	require.NoError(t, in.UpdateSkipping(ctx, data, domain.UpdateSkip{Tags: true}))

	assert.True(t, given.Equal(row(t, c, "ua").UpdatedAt), "updated_at must be the value passed in: %v", row(t, c, "ua").UpdatedAt)
}

func TestInstances_should_ClearOrKeepColumns_When_UpdateCarriesEmptyValues(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	_, err := c.Project.Create().SetName("proj").Save(ctx)
	require.NoError(t, err)
	in := newWithPRPorts(c)
	full := fullCreateData(t)
	full.WorkflowID, full.PauseReason, full.ExitReason = "wf", "p", "e"
	full.ProcessingGraceUntil = time.Date(2026, 3, 4, 13, 0, 0, 0, time.UTC)
	full.TymuxSessionID = "ty"
	require.NoError(t, in.Create(ctx, full))
	full.Tags = nil
	require.NoError(t, in.Update(ctx, full)) // Create does not write these; Update does, so there is something to clear
	require.NotEmpty(t, row(t, c, "full").WorkflowID)
	require.NotNil(t, row(t, c, "full").ProcessingGraceUntil)

	require.NoError(t, in.Update(ctx, domain.InstanceData{Title: "full", Path: "/p/full", Program: "claude", Status: domain.Active}))

	r := row(t, c, "full")
	assert.Nil(t, r.ArchivedAt, "archived_at cleared")
	assert.Empty(t, r.WorkflowID)
	assert.Empty(t, r.PauseReason)
	assert.Empty(t, r.ExitReason)
	assert.Nil(t, r.ProcessingGraceUntil)
	assert.Empty(t, r.Note, "note is cleared when empty")
	assert.Empty(t, r.TymuxSessionID)
	assert.Empty(t, r.RuleTagProvenance)
	assert.Empty(t, r.SuppressedRuleTags)
	assert.False(t, r.OneShot)
	assert.False(t, r.Hidden)
	assert.Equal(t, "uuid-full", r.UUID, "an empty uuid keeps the stored one")
	assert.Equal(t, "br", r.Branch, "branch is set-only")
	assert.Equal(t, "tymux", r.Backend, "backend is set-only")
	assert.Equal(t, "cat", r.Category, "category is set-only")
	assert.Zero(t, c.Session.Query().QueryProject().CountX(ctx), "empty project id clears the link")
	assert.Equal(t, 1, c.Worktree.Query().CountX(ctx), "empty repo path leaves the worktree row")
	assert.Equal(t, 1, c.ClaudeSession.Query().CountX(ctx), "empty conversation uuid leaves the claude row")
}

func TestInstances_should_KeepOrClearProjectLink_When_ProjectIDUnknownOrEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	_, err := c.Project.Create().SetName("proj").Save(ctx)
	require.NoError(t, err)
	in := newWithPRPorts(c)
	d := sectionData("pj", "a")
	d.ProjectID = "proj"
	require.NoError(t, in.Create(ctx, d))

	d.ProjectID = "ghost"
	require.NoError(t, in.Update(ctx, d))
	assert.Equal(t, "proj", loadFull(t, in, "pj").ProjectID, "an unknown project name keeps the existing link")

	d.ProjectID = ""
	require.NoError(t, in.Update(ctx, d))
	assert.Empty(t, loadFull(t, in, "pj").ProjectID, "an empty project id clears the link")
}

func TestInstances_should_MirrorLegacyPRColumns_When_PRLinksPresentOrNot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)
	d := sectionData("mr", "a")
	d.GitHubPRURL, d.GitHubPRNumber, d.GitHubPRStatusTerminal = "https://github.com/o/r/pull/5", 5, true
	require.NoError(t, in.Create(ctx, d))

	d.GitHubPRURL, d.GitHubPRNumber, d.GitHubPRStatusTerminal = "", 0, false
	require.NoError(t, in.Update(ctx, d))
	r := row(t, c, "mr")
	assert.Equal(t, 5, r.GithubPrNumber, "link-less save is set-only: zero must not erase the number")
	assert.Equal(t, "https://github.com/o/r/pull/5", r.GithubPrURL, "link-less save is set-only: empty must not erase the url")
	assert.True(t, r.GithubPrStatusTerminal)

	d.TerminalClearPending = true
	require.NoError(t, in.Update(ctx, d))
	assert.False(t, row(t, c, "mr").GithubPrStatusTerminal, "TerminalClearPending clears the flag")

	d.TerminalClearPending, d.HasPRLinks = false, true
	require.NoError(t, in.Update(ctx, d))
	r = row(t, c, "mr")
	assert.Zero(t, r.GithubPrNumber, "a session with links writes the mirror unconditionally")
	assert.Empty(t, r.GithubPrURL)

	d.HasPRLinks, d.LegacyPRUnrepresented, d.GitHubPRStatusTerminal = false, true, false
	require.NoError(t, in.Update(ctx, d))
	assert.False(t, row(t, c, "mr").GithubPrStatusTerminal, "an unrepresented legacy save without the flag must not set it")

	d.GitHubPRStatusTerminal = true
	d.GitHubPRNumber, d.GitHubPRURL = 7, "https://github.com/o/r/pull/7"
	require.NoError(t, in.Update(ctx, d))
	r = row(t, c, "mr")
	assert.Zero(t, r.GithubPrNumber, "an unrepresented legacy number keeps the stored number untouched")
	assert.True(t, r.GithubPrStatusTerminal)
}

// TestInstances_should_SkipOnlyTheNamedSections_When_UpdateSkippingCalled: a skipped section keeps
// its previous rows, every other section and the session columns are rewritten.
func TestInstances_should_SkipOnlyTheNamedSections_When_UpdateSkippingCalled(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		skip domain.UpdateSkip
	}{
		{"none", domain.UpdateSkip{}},
		{"worktree", domain.UpdateSkip{Worktree: true}},
		{"diffstats", domain.UpdateSkip{DiffStats: true}},
		{"tags", domain.UpdateSkip{Tags: true}},
		{"claude", domain.UpdateSkip{Claude: true}},
		{"all", domain.UpdateSkip{Worktree: true, DiffStats: true, Tags: true, Claude: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			c := entdbtest.NewClient(t)
			in := newWithPRPorts(c)
			require.NoError(t, in.Create(ctx, sectionData("sk", "a")))
			require.NoError(t, in.UpdateSkipping(ctx, sectionData("sk", "a"), domain.UpdateSkip{}))

			require.NoError(t, in.UpdateSkipping(ctx, sectionData("sk", "bb"), tc.skip))

			got := loadFull(t, in, "sk")
			pick := func(skipped bool) domain.InstanceData {
				if skipped {
					return sectionData("sk", "a")
				}
				return sectionData("sk", "bb")
			}
			assert.Equal(t, "/p/bb", got.Path, "session columns are always rewritten")
			assert.Equal(t, pick(tc.skip.Worktree).Worktree, got.Worktree, "worktree")
			assert.Equal(t, pick(tc.skip.DiffStats).DiffStats, got.DiffStats, "diff stats")
			assert.Equal(t, pick(tc.skip.Tags).Tags, got.Tags, "tags")
			assert.Equal(t, pick(tc.skip.Claude).ClaudeSession, got.ClaudeSession, "claude session and metadata")
		})
	}
}

func TestInstances_should_AlwaysRunPRPort_When_AllSectionsSkipped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	var calls atomic.Int32
	in := New(c, Ports{UpsertSessionPRSet: func(_ context.Context, tx *ent.Tx, d domain.InstanceData) error {
		calls.Add(1)
		assert.Equal(t, "sk", d.Title)
		assert.NotNil(t, tx)
		return nil
	}})
	require.NoError(t, in.Create(ctx, sectionData("sk", "a")))
	calls.Store(0)

	require.NoError(t, in.UpdateSkipping(ctx, sectionData("sk", "b"), domain.UpdateSkip{Worktree: true, DiffStats: true, Tags: true, Claude: true}))

	assert.EqualValues(t, 1, calls.Load())
}

func TestInstances_should_WritePRLinksInsideTheTx_When_UpdateCalled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)
	d := fullCreateData(t)
	d.PRs = nil
	require.NoError(t, in.Create(ctx, d))
	require.Zero(t, c.SessionPR.Query().CountX(ctx))

	d = fullCreateData(t)
	require.NoError(t, in.Update(ctx, d))
	assert.Equal(t, 1, c.SessionPR.Query().Where(entsessionpr.SessionUUID("uuid-full")).CountX(ctx))

}

func TestInstances_should_RollBackPRLinks_When_PortFailsAfterWritingThem(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := entdbtest.NewClient(t)
	boom := errors.New("fail after link write")
	in := New(c, Ports{UpsertSessionPRSet: func(ctx context.Context, tx *ent.Tx, d domain.InstanceData) error {
		if err := sessionpr.UpsertSessionPRSetTx(ctx, tx, d); err != nil {
			return err
		}
		return boom
	}})
	seed := fullCreateData(t)
	seed.PRs = nil
	require.NoError(t, newWithPRPorts(c).Create(ctx, seed))

	require.ErrorIs(t, in.Update(ctx, fullCreateData(t)), boom)

	assert.Zero(t, c.SessionPR.Query().CountX(ctx), "the link written inside the tx rolls back with it")
}

func TestInstances_should_ReturnErrorAndRollBack_When_UpsertSessionPRSetPortFailsOnUpdate(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c := entdbtest.NewClient(t)
	boom := errors.New("pr port failed")
	in := New(c, Ports{UpsertSessionPRSet: func(context.Context, *ent.Tx, domain.InstanceData) error { return nil }})
	require.NoError(t, in.Create(ctx, sectionData("pf", "a")))
	failing := New(c, Ports{UpsertSessionPRSet: func(context.Context, *ent.Tx, domain.InstanceData) error { return boom }})

	err := failing.Update(ctx, sectionData("pf", "bb"))

	require.ErrorIs(t, err, boom)
	got := loadFull(t, in, "pf")
	want := sectionData("pf", "a")
	assert.Equal(t, want.Path, got.Path, "session columns roll back")
	assert.Equal(t, want.Worktree, got.Worktree)
	assert.Equal(t, want.DiffStats, got.DiffStats)
	assert.Equal(t, want.Tags, got.Tags)
	assert.Equal(t, want.ClaudeSession, got.ClaudeSession)
	// A leaked open tx would hold the single connection; the next write must still go through.
	require.NoError(t, in.Update(ctx, sectionData("pf", "ccc")))
}

func TestInstances_should_RollBackEverything_When_AnUpdateStepFails(t *testing.T) {
	t.Parallel()
	steps := []struct {
		name    string
		install func(c *ent.Client)
		wantErr string
	}{
		{"session update", func(c *ent.Client) { c.Session.Use(failHook(ent.OpUpdateOne)) }, "failed to update session"},
		{"worktree update", func(c *ent.Client) { c.Worktree.Use(failHook(ent.OpUpdateOne)) }, "failed to update worktree"},
		{"diff stats upsert", func(c *ent.Client) { c.DiffStats.Use(failHook(ent.OpCreate)) }, "failed to upsert diff stats"},
		{"tag create", func(c *ent.Client) { c.Tag.Use(failHook(ent.OpCreate)) }, "failed to create tag"},
		{"claude update", func(c *ent.Client) { c.ClaudeSession.Use(failHook(ent.OpUpdateOne)) }, "failed to update claude session"},
		{"claude metadata create", func(c *ent.Client) { c.ClaudeMetadata.Use(failHook(ent.OpCreate)) }, "failed to create claude metadata"},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			c := entdbtest.NewClient(t)
			in := newWithPRPorts(c)
			require.NoError(t, in.Create(ctx, sectionData("rb", "a")))
			st.install(c)

			err := in.Update(ctx, sectionData("rb", "bb"))

			require.ErrorContains(t, err, st.wantErr)
			got := loadFull(t, in, "rb")
			want := sectionData("rb", "a")
			assert.Equal(t, want.Path, got.Path, "session columns rolled back")
			assert.Equal(t, want.Worktree, got.Worktree)
			assert.Equal(t, want.DiffStats, got.DiffStats)
			assert.Equal(t, want.Tags, got.Tags)
			assert.Equal(t, want.ClaudeSession, got.ClaudeSession)
			assert.Equal(t, 1, c.Tag.Query().Where(tag.Name("tag-a")).CountX(ctx))
			assert.Zero(t, c.Tag.Query().Where(tag.Name("tag-bb")).CountX(ctx), "no tag from the failed tx")
			assert.Equal(t, 1, c.ClaudeMetadata.Query().Where(claudemetadata.Key("k-a")).CountX(ctx))
		})
	}
}

func TestInstances_should_ReturnNotFound_When_UpdatingMissingTitle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)
	require.NoError(t, in.Create(ctx, sectionData("present", "a")))

	err := in.Update(ctx, sectionData("absent", "bb"))

	require.ErrorContains(t, err, "failed to find session")
	assert.Equal(t, 1, c.Session.Query().CountX(ctx), "no row created")
	assert.Equal(t, sectionData("present", "a").Worktree, loadFull(t, in, "present").Worktree)
}

func TestInstances_should_LeaveNeighbourRowsUntouched_When_UpdateCalled(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)
	require.NoError(t, in.Create(ctx, sectionData("neighbour", "n"))) // older rows sort first, so an unscoped lookup would hit it
	require.NoError(t, in.Create(ctx, sectionData("target", "a")))
	before := loadFull(t, in, "neighbour")

	require.NoError(t, in.Update(ctx, sectionData("target", "bb")))

	gotTarget := loadFull(t, in, "target")
	assert.Equal(t, "/p/bb", gotTarget.Path)
	assert.Equal(t, sectionData("target", "bb").Worktree, gotTarget.Worktree)
	assert.Equal(t, sectionData("target", "bb").ClaudeSession, gotTarget.ClaudeSession)
	after := loadFull(t, in, "neighbour")
	assert.Equal(t, before, after, "a title-keyed update must not touch another session or its child rows")
	all := c.Tag.Query().AllX(ctx)
	names := make([]string, 0, len(all))
	for _, tg := range all {
		names = append(names, tg.Name)
	}
	sort.Strings(names)
	assert.Equal(t, []string{"tag-a", "tag-bb", "tag-n"}, names)
}

func TestInstances_should_WriteEachClaudeSettingToItsOwnColumn_When_ClaudeRowUpdated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	in := newWithPRPorts(c)
	require.NoError(t, in.Create(ctx, sectionData("cs", "a")))

	for _, variant := range []string{"bb", "ccc", "a"} {
		require.NoError(t, in.Update(ctx, sectionData("cs", variant)))
		assert.Equal(t, sectionData("cs", variant).ClaudeSession, loadFull(t, in, "cs").ClaudeSession, variant)
	}
}

func TestInstances_should_KeepStoredValue_When_SetOnlyFieldIsZeroInMemory(t *testing.T) {
	t.Parallel()
	zeroers := map[string]func(*domain.InstanceData){
		"WorkingDir":                func(d *domain.InstanceData) { d.WorkingDir = "" },
		"Height":                    func(d *domain.InstanceData) { d.Height = 0 },
		"Width":                     func(d *domain.InstanceData) { d.Width = 0 },
		"Prompt":                    func(d *domain.InstanceData) { d.Prompt = "" },
		"ExistingWorktree":          func(d *domain.InstanceData) { d.ExistingWorktree = "" },
		"SessionType":               func(d *domain.InstanceData) { d.SessionType = "" },
		"TmuxPrefix":                func(d *domain.InstanceData) { d.TmuxPrefix = "" },
		"LastTerminalUpdate":        func(d *domain.InstanceData) { d.LastTerminalUpdate = time.Time{} },
		"LastMeaningfulOutput":      func(d *domain.InstanceData) { d.LastMeaningfulOutput = time.Time{} },
		"LastOutputSignature":       func(d *domain.InstanceData) { d.LastOutputSignature = "" },
		"LastAddedToQueue":          func(d *domain.InstanceData) { d.LastAddedToQueue = time.Time{} },
		"LastViewed":                func(d *domain.InstanceData) { d.LastViewed = time.Time{} },
		"LastAcknowledged":          func(d *domain.InstanceData) { d.LastAcknowledged = time.Time{} },
		"LastUserResponse":          func(d *domain.InstanceData) { d.LastUserResponse = time.Time{} },
		"LastPromptDetected":        func(d *domain.InstanceData) { d.LastPromptDetected = time.Time{} },
		"LastPromptSignature":       func(d *domain.InstanceData) { d.LastPromptSignature = "" },
		"MCPServerURL":              func(d *domain.InstanceData) { d.MCPServerURL = "" },
		"InitialPrompt":             func(d *domain.InstanceData) { d.InitialPrompt = "" },
		"InitialPromptSentAt":       func(d *domain.InstanceData) { d.InitialPromptSentAt = time.Time{} },
		"CreationProgressUpdatedAt": func(d *domain.InstanceData) { d.CreationProgressUpdatedAt = time.Time{} },
		"GitHubOwner":               func(d *domain.InstanceData) { d.GitHubOwner = "" },
		"GitHubRepo":                func(d *domain.InstanceData) { d.GitHubRepo = "" },
		"GitHubHost":                func(d *domain.InstanceData) { d.GitHubHost = "" },
	}
	for name, zero := range zeroers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			c := entdbtest.NewClient(t)
			_, err := c.Project.Create().SetName("proj").Save(ctx)
			require.NoError(t, err)
			in := newWithPRPorts(c)
			full := fullUpdateData(t)
			require.NoError(t, in.Create(ctx, domain.InstanceData{Title: "full", UUID: "uuid-full", Path: "/seed", Program: "seed", Status: domain.Active}))
			require.NoError(t, in.Update(ctx, full))
			before := loadFull(t, in, "full")

			zeroed := full
			zero(&zeroed)
			require.NoError(t, in.Update(ctx, zeroed))

			after := loadFull(t, in, "full")
			after.UpdatedAt = before.UpdatedAt // the tag edge write re-stamps it
			assert.Equal(t, before, after, "a zero %s must leave the stored value alone", name)
		})
	}
}

func TestInstances_should_LinkTheNamedProject_When_OtherProjectsExist(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	for _, n := range []string{"other-first", "proj", "other-last"} {
		_, err := c.Project.Create().SetName(n).Save(ctx)
		require.NoError(t, err)
	}
	in := newWithPRPorts(c)
	require.NoError(t, in.Create(ctx, sectionData("lp", "a")))
	d := sectionData("lp", "bb")
	d.ProjectID = "proj"

	require.NoError(t, in.Update(ctx, d))

	assert.Equal(t, "proj", loadFull(t, in, "lp").ProjectID, "the edge must point at the named project, not a neighbour")
}
