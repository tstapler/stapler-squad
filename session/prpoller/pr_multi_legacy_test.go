package prpoller

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// Characterization tests of today's single-PR behavior (session-multi-pr-tracking
// T1.1.1). They pin the flag-off path so later multi-PR work provably leaves it
// unchanged; a failure here means legacy behavior moved, not that the test is wrong.

// legacyCountingGHClient counts every outbound call the poller can make.
type legacyCountingGHClient struct {
	infoCalls   atomic.Int64
	branchCalls atomic.Int64
}

func (f *legacyCountingGHClient) CheckGHAuth(context.Context) error { return nil }

func (f *legacyCountingGHClient) GetPRForBranchConditional(_ context.Context, _ github.RepoRef, _, etag string) (*github.PRInfo, string, bool, error) {
	f.branchCalls.Add(1)
	return nil, etag, false, github.ErrNoPR
}

func (f *legacyCountingGHClient) GetPRInfoConditional(context.Context, github.RepoRef, int, *github.ETagCache) (*github.PRInfo, bool, error) {
	f.infoCalls.Add(1)
	return &github.PRInfo{State: "open"}, true, nil
}

func TestLegacyPoller_TerminalSession_MakesNoGitHubCalls(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		terminal   bool
		wantInfo   int64
		wantBranch int64
	}{
		{name: "terminal flag set is skipped (pr_status_poller.go terminal skip)", terminal: true, wantInfo: 0, wantBranch: 0},
		{name: "non-terminal control session is polled", terminal: false, wantInfo: 1, wantBranch: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inst := &instance.Instance{
				Title:                  "legacy-terminal-" + tt.name,
				Status:                 domain.Active,
				GitHubOwner:            "acme",
				GitHubRepo:             "app",
				GitHubPRNumber:         11,
				GitHubPRStatusTerminal: tt.terminal,
			}
			client := &legacyCountingGHClient{}
			poller := newLegacyTestPoller(client, inst)

			poller.checkAllSessions()

			if got := client.infoCalls.Load(); got != tt.wantInfo {
				t.Errorf("GetPRInfoConditional calls = %d, want %d", got, tt.wantInfo)
			}
			if got := client.branchCalls.Load(); got != tt.wantBranch {
				t.Errorf("GetPRForBranchConditional calls = %d, want %d", got, tt.wantBranch)
			}
		})
	}
}

// Flag off, the legacy loop observes the active PR merged: the observation must
// survive buildSnapshot's active-PR projection (architecture C1) instead of being
// overwritten by the stale open view, or the session would be polled forever.
func TestLegacyPoller_ActivePRMerged_KeepsTerminalThroughSnapshot(t *testing.T) {
	t.Parallel()
	client := &recordingPRClient{info: map[int]*github.PRInfo{12: {Number: 12, State: "merged"}}}
	p, _ := newRowPoller(t, client)
	p.refresherOn = func() bool { return false }
	inst := newTwoPRInstance(t)
	inst.Status = domain.Active
	p.SetInstances([]*instance.Instance{inst})

	p.checkAllSessions()

	snap := inst.Snapshot().GitHub
	if !snap.GitHubPRStatusTerminal {
		t.Error("terminal flag lost: the projection overwrote the legacy observation")
	}
	if got := snap.PRs[1].State; got != domain.PRStateMerged {
		t.Errorf("active PR view state = %q, want merged", got)
	}
}

// With every scope off the refresher still keeps PullRequestState rows fresh
// (the cleanup gate and sweeper read them), without touching any session.
func TestRefresher_ShouldMarkRowsTerminal_WhenAllScopesOff(t *testing.T) {
	t.Parallel()
	merged := func(n int) *github.PRInfo { return &github.PRInfo{Number: n, State: "merged"} }
	client := &recordingPRClient{info: map[int]*github.PRInfo{11: merged(11), 40: merged(40), 50: merged(50)}}
	p, storage := newRowPoller(t, client)

	inst := newTwoPRInstance(t)
	inst.Status = domain.Active
	suspended := newPRTestInstance(t)
	suspended.UUID, suspended.Title, suspended.Status = "u-suspended", "suspended", domain.Paused
	require.NoError(t, suspended.AttachPR(prRefN(t, 50), domain.AttachSourceCreatedFromURL, domain.PRInitial{State: domain.PRStateOpen}))
	p.SetInstances([]*instance.Instance{inst, suspended})

	// #11 is a non-active PR of inst (seeded open here), #40 is item-only, #50 belongs to a suspended session.
	seedStateRow(t, storage.Repo, session.SessionPROwner(inst.UUID), prRefN(t, 11), domain.PRInitial{State: domain.PRStateOpen})
	seedStateRow(t, storage.Repo, session.ItemPROwner("item-1"), prRefN(t, 40), domain.PRInitial{State: domain.PRStateOpen})
	seedStateRow(t, storage.Repo, session.SessionPROwner("u-suspended"), prRefN(t, 50), domain.PRInitial{State: domain.PRStateOpen})
	beforeInst, beforeSuspended := inst.Snapshot().GitHub, suspended.Snapshot().GitHub

	p.checkAllSessions()

	for _, n := range []int{11, 40, 50} {
		got := rowOf(t, storage, n)
		assert.Equal(t, domain.PRStateMerged, got.State, "#%d", n)
		assert.True(t, got.Terminal(), "#%d", n)
	}
	assert.Equal(t, beforeInst.PRs[0], inst.Snapshot().GitHub.PRs[0], "the refresher drives no snapshots (only the legacy loop touches its active #12)")
	assert.Equal(t, beforeSuspended.PRs, suspended.Snapshot().GitHub.PRs)
}

func TestRefresher_ShouldMakeNoCalls_WhenRefresherOffScopeSet(t *testing.T) {
	t.Parallel()
	client := &recordingPRClient{}
	p, storage := newRowPoller(t, client)
	on := false
	p.refresherOn = func() bool { return on }
	seedStateRow(t, storage.Repo, session.SessionPROwner("u1"), prRefN(t, 12), domain.PRInitial{State: domain.PRStateOpen})

	p.checkAllSessions()
	assert.Equal(t, 0, client.callsFor(12), "kill switch: zero GitHub calls")

	on = true
	p.checkAllSessions()
	assert.Equal(t, 1, client.callsFor(12), "back on, the next tick polls")
}

// Flag off, one active open PR: the legacy loop and the refresher each make one
// conditional GET per interval, so at most two requests per PR.
func TestRefresher_ShouldBoundFetchesToTwoPerPR_WhenLegacyLoopAlsoRuns(t *testing.T) {
	t.Parallel()
	client := &recordingPRClient{}
	p, storage := newRowPoller(t, client)
	inst := newPRTestInstance(t)
	inst.Status = domain.Active
	require.NoError(t, inst.AttachPR(prRefN(t, 12), domain.AttachSourceBranchDiscovery, domain.PRInitial{State: domain.PRStateOpen}))
	seedStateRow(t, storage.Repo, session.SessionPROwner(inst.UUID), prRefN(t, 12), domain.PRInitial{State: domain.PRStateOpen})
	p.SetInstances([]*instance.Instance{inst})

	p.checkAllSessions()

	assert.LessOrEqual(t, client.callsFor(12), 2)
}
