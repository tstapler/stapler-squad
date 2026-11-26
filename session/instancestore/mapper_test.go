package instancestore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	sessionpred "github.com/tstapler/stapler-squad/session/ent/session"
	"github.com/tstapler/stapler-squad/session/internal/entdbtest"
)

func TestInstances_should_ReturnExactNotFoundText_When_TitleMissing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	in := New(entdbtest.NewClient(t), Ports{})

	_, err := in.Get(ctx, "ghost")
	require.Error(t, err)
	assert.Equal(t, "session not found: ghost", err.Error())
	_, err = in.GetWithOptions(ctx, "ghost2", domain.LoadMinimal)
	require.Error(t, err)
	assert.Equal(t, "session not found: ghost2", err.Error())
}

func TestInstances_should_ExcludeEmptyArtifacts_When_GetAllSessionArtifacts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	seedSession(t, c, "has", "uuid-has", 0)
	seedSession(t, c, "empty", "uuid-empty", 0)
	_, err := c.Session.Update().Where(sessionpred.Title("has")).SetSessionArtifacts(`{"a":1}`).Save(ctx)
	require.NoError(t, err)
	in := New(c, Ports{})

	all, err := in.GetAllSessionArtifacts(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"has": `{"a":1}`}, all)

	blob, err := in.GetSessionArtifactsByUUID(ctx, "uuid-empty")
	require.NoError(t, err)
	assert.Nil(t, blob, "no stored artifacts is (nil, nil)")

	_, err = in.GetSessionArtifactsByUUID(ctx, "uuid-missing")
	assert.Error(t, err)
}

func TestInstances_should_RoundTripScalarFields_When_Mapped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := entdbtest.NewClient(t)
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	h := func(n int) time.Time { return at.Add(time.Duration(n) * time.Hour) }
	_, err := c.Session.Create().
		SetTitle("m").SetUUID("u").SetPath("/p").SetWorkingDir("wd").SetBranch("br").SetStatus(int(domain.Paused)).
		SetHeight(11).SetWidth(22).SetAutoYes(true).SetAutoApprove(true).SetAutonomousMode(true).
		SetPrompt("pr").SetInitialPrompt("ip").SetProgram("prog").SetExistingWorktree("ew").
		SetCategory("cat").SetNote("note").SetIsExpanded(true).SetTmuxPrefix("tp").
		SetBackend(string(domain.ProcessManagerBackend("tymux"))).SetTymuxSessionID("ty").
		SetLastOutputSignature("sig").SetMcpServerURL("mcp").SetOneShot(true).SetHidden(true).SetPinned(true).
		SetLastTerminalUpdate(at).SetLastMeaningfulOutput(h(1)).SetLastAddedToQueue(h(2)).
		SetLastViewed(h(3)).SetLastAcknowledged(h(4)).SetLastUserResponse(h(5)).
		SetProcessingGraceUntil(h(6)).SetLastPromptDetected(h(7)).
		SetInitialPromptSentAt(h(8)).SetCreationProgressUpdatedAt(h(9)).
		SetLastPromptSignature("lps").SetPauseReason("pause").SetExitReason("exit").SetWorkflowID("wf").
		SetArchivedAt(h(10)).
		SetGithubPrURL("https://x/pr/5").SetGithubPrNumber(5).SetGithubPrStatusTerminal(true).
		SetGithubOwner("o").SetGithubRepo("r").SetGithubHost("h").SetSessionType("sty").
		SetSuppressedRuleTags([]string{"s1"}).SetCreatedAt(h(11)).SetUpdatedAt(h(12)).
		Save(ctx)
	require.NoError(t, err)

	d, err := New(c, Ports{}).GetWithOptions(ctx, "m", domain.LoadMinimal)
	require.NoError(t, err)

	assert.Equal(t, "u", d.UUID)
	assert.True(t, h(11).Equal(d.CreatedAt))
	assert.True(t, h(12).Equal(d.UpdatedAt))
	assert.Equal(t, "/p", d.Path)
	assert.Equal(t, "wd", d.WorkingDir)
	assert.Equal(t, "br", d.Branch)
	assert.Equal(t, domain.Paused, d.Status)
	assert.Equal(t, 11, d.Height)
	assert.Equal(t, 22, d.Width)
	assert.True(t, d.AutoYes)
	assert.True(t, d.AutoApprove)
	assert.True(t, d.AutonomousMode)
	assert.Equal(t, "pr", d.Prompt)
	assert.Equal(t, "ip", d.InitialPrompt)
	assert.Equal(t, "prog", d.Program)
	assert.Equal(t, "ew", d.ExistingWorktree)
	assert.Equal(t, "cat", d.Category)
	assert.Equal(t, "note", d.Note)
	assert.True(t, d.IsExpanded)
	assert.Equal(t, "tp", d.TmuxPrefix)
	assert.Equal(t, domain.ProcessManagerBackend("tymux"), d.Backend)
	assert.Equal(t, "ty", d.TymuxSessionID)
	assert.Equal(t, "sig", d.LastOutputSignature)
	assert.Equal(t, "mcp", d.MCPServerURL)
	assert.True(t, d.OneShot)
	assert.True(t, d.Hidden)
	assert.True(t, d.Pinned)
	assert.True(t, at.Equal(d.LastTerminalUpdate))
	assert.True(t, h(1).Equal(d.LastMeaningfulOutput))
	assert.True(t, h(2).Equal(d.LastAddedToQueue))
	assert.True(t, h(3).Equal(d.LastViewed))
	assert.True(t, h(4).Equal(d.LastAcknowledged))
	assert.True(t, h(5).Equal(d.LastUserResponse))
	assert.True(t, h(6).Equal(d.ProcessingGraceUntil))
	assert.True(t, h(7).Equal(d.LastPromptDetected))
	assert.True(t, h(8).Equal(d.InitialPromptSentAt))
	assert.True(t, h(9).Equal(d.CreationProgressUpdatedAt))
	assert.Equal(t, "lps", d.LastPromptSignature)
	assert.Equal(t, "pause", d.PauseReason)
	assert.Equal(t, "exit", d.ExitReason)
	assert.Equal(t, "wf", d.WorkflowID)
	require.NotNil(t, d.ArchivedAt)
	assert.True(t, h(10).Equal(*d.ArchivedAt))
	assert.Equal(t, "https://x/pr/5", d.GitHubPRURL)
	assert.Equal(t, 5, d.GitHubPRNumber)
	assert.True(t, d.GitHubPRStatusTerminal)
	assert.Equal(t, "o", d.GitHubOwner)
	assert.Equal(t, "r", d.GitHubRepo)
	assert.Equal(t, "h", d.GitHubHost)
	assert.Equal(t, domain.SessionType("sty"), d.SessionType)
	assert.Equal(t, map[string]bool{"s1": true}, d.SuppressedRuleTags)
}
