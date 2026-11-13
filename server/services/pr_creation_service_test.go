package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Characterization of today's single-PR short-circuit (session-multi-pr-tracking
// T1.1.1): a session that already carries a PR never asks GitHub for another, and
// its legacy PR columns are left exactly as they were. Complements
// TestCreatePullRequest_should_SetAlreadyExisted_When_SessionHasCachedPRUrl, which
// pins the response flags.
func TestCreatePullRequest_should_KeepExistingPRColumns_When_ShortCircuiting(t *testing.T) {
	t.Parallel()
	mock := &fakePRGitExecutor{createOut: "https://github.com/tstapler/stapler-squad/pull/1000\n"}
	wt := newFakePRWorktree(t, "sess-legacy-shortcircuit", "feature/legacy", mock)
	inst := &session.Instance{Title: "Legacy short-circuit", UUID: uuid.New().String()}
	inst.SetGitWorktree(wt)
	inst.GitHubPRURL = "https://github.com/tstapler/stapler-squad/pull/999"
	inst.GitHubPRNumber = 999

	svc := NewPRCreationService(&fakePRInstanceStore{}, events.NewEventBus(1), nil, nil, findInstanceFunc("sess-legacy-shortcircuit", inst))

	resp, err := svc.CreatePullRequest(context.Background(), connect.NewRequest(&sessionv1.CreatePullRequestRequest{
		SessionId: "sess-legacy-shortcircuit",
		Title:     "A different PR",
	}))
	require.NoError(t, err)

	assert.True(t, resp.Msg.AlreadyExisted)
	assert.EqualValues(t, 999, resp.Msg.PrNumber, "the cached PR wins; no second PR is created")
	assert.False(t, mock.calledWith("gh pr list"), "the fast path must not even look up another PR")
	assert.False(t, mock.calledWith("gh pr create"))
	gh := inst.Snapshot().GitHub
	assert.Equal(t, 999, gh.GitHubPRNumber)
	assert.Equal(t, "https://github.com/tstapler/stapler-squad/pull/999", gh.GitHubPRURL)
}

// Checklist row 6: a PR created through the service joins the session's PR set as
// pr_creation_service rather than overwriting the legacy scalars.
func TestCreatePullRequest_ShouldAttachAsPRCreationService_WhenSessionHasResolvedRepo(t *testing.T) {
	t.Parallel()
	mock := &fakePRGitExecutor{createOut: "https://github.com/tstapler/stapler-squad/pull/1000\n"}
	wt := newFakePRWorktree(t, "sess-attach", "feature/attach", mock)
	inst := &session.Instance{Title: "Attach via service", UUID: uuid.New().String(), GitHubOwner: "tstapler", GitHubRepo: "stapler-squad"}
	inst.SetGitWorktree(wt)
	svc := NewPRCreationService(&fakePRInstanceStore{}, events.NewEventBus(1), nil, nil, findInstanceFunc("sess-attach", inst))

	_, err := svc.CreatePullRequest(context.Background(), connect.NewRequest(&sessionv1.CreatePullRequestRequest{
		SessionId: "sess-attach",
		Title:     "Attach",
	}))
	require.NoError(t, err)

	prs := inst.Snapshot().GitHub.PRs
	require.Len(t, prs, 1)
	assert.Equal(t, 1000, prs[0].Ref.Number())
	assert.Equal(t, domain.AttachSourcePRCreationService, prs[0].AttachSource)
	assert.Equal(t, 1000, inst.Snapshot().GitHub.GitHubPRNumber)
}

// A session whose only PR is merged may open another: the existing-PR
// short-circuit fires only for a non-terminal PR whose head is the current branch.
func TestCreatePullRequest_ShouldCreateAndAttach_WhenPriorPRMerged(t *testing.T) {
	t.Parallel()
	mock := &fakePRGitExecutor{createOut: "https://github.com/tstapler/stapler-squad/pull/1000\n"}
	wt := newFakePRWorktree(t, "sess-after-merge", "feat-b", mock)
	inst := &session.Instance{Title: "After merge", UUID: uuid.New().String(), GitHubOwner: "tstapler", GitHubRepo: "stapler-squad"}
	inst.SetGitWorktree(wt)
	r11, err := domain.NewPRRef("", "tstapler", "stapler-squad", 11)
	require.NoError(t, err)
	require.NoError(t, inst.AttachPR(r11, domain.AttachSourceBranchDiscovery,
		session.PRInitial{State: domain.PRStateMerged, Terminal: true, HeadBranch: "feat-a"}))
	svc := NewPRCreationService(&fakePRInstanceStore{}, events.NewEventBus(1), nil, nil, findInstanceFunc("sess-after-merge", inst))

	resp, err := svc.CreatePullRequest(context.Background(), connect.NewRequest(&sessionv1.CreatePullRequestRequest{
		SessionId: "sess-after-merge", Title: "Second",
	}))
	require.NoError(t, err)

	assert.False(t, resp.Msg.AlreadyExisted)
	assert.EqualValues(t, 1000, resp.Msg.PrNumber)
	assert.True(t, mock.calledWith("gh pr create"), "a merged PR must not block a new one")
	assert.Len(t, inst.Snapshot().GitHub.PRs, 2)
}

func TestCreatePullRequest_ShouldReturnExistingPR_WhenOpenPROnCurrentBranch(t *testing.T) {
	t.Parallel()
	mock := &fakePRGitExecutor{createOut: "https://github.com/tstapler/stapler-squad/pull/1000\n"}
	wt := newFakePRWorktree(t, "sess-open-here", "feat-b", mock)
	inst := &session.Instance{Title: "Open here", UUID: uuid.New().String(), GitHubOwner: "tstapler", GitHubRepo: "stapler-squad"}
	inst.SetGitWorktree(wt)
	r12, err := domain.NewPRRef("", "tstapler", "stapler-squad", 12)
	require.NoError(t, err)
	require.NoError(t, inst.AttachPR(r12, domain.AttachSourceBranchDiscovery,
		session.PRInitial{State: domain.PRStateOpen, HeadBranch: "feat-b"}))
	svc := NewPRCreationService(&fakePRInstanceStore{}, events.NewEventBus(1), nil, nil, findInstanceFunc("sess-open-here", inst))

	resp, err := svc.CreatePullRequest(context.Background(), connect.NewRequest(&sessionv1.CreatePullRequestRequest{
		SessionId: "sess-open-here", Title: "Second",
	}))
	require.NoError(t, err)

	assert.True(t, resp.Msg.AlreadyExisted)
	assert.EqualValues(t, 12, resp.Msg.PrNumber)
	assert.False(t, mock.calledWith("gh pr create"))
}

func TestOpenPROnBranch_ShouldMatchOnlyNonTerminalPRsWithThatHead(t *testing.T) {
	t.Parallel()
	mk := func(n int, st domain.PRState, head string) domain.PRView {
		ref, err := domain.NewPRRef("", "acme", "app", n)
		require.NoError(t, err)
		return domain.PRView{Ref: ref, State: st, StoredTerminal: st.Terminal(), HeadBranch: head}
	}
	prs := []domain.PRView{mk(11, domain.PRStateMerged, "feat-b"), mk(12, domain.PRStateOpen, "feat-a"), mk(13, domain.PRStateOpen, "feat-b")}
	got, ok := session.OpenPROnBranch(prs, "feat-b")
	require.True(t, ok)
	assert.Equal(t, 13, got.Ref.Number())
	_, ok = session.OpenPROnBranch(prs[:1], "feat-b")
	assert.False(t, ok, "a merged PR on the branch does not count")
	_, ok = session.OpenPROnBranch(prs, "")
	assert.False(t, ok, "no current branch, no match")
}
