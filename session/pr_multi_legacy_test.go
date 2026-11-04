package session

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/github"
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

func newLegacyTestPoller(client prGitHubClient, insts ...*Instance) *PRStatusPoller {
	poller := NewPRStatusPoller(nil)
	poller.ghClient = client
	poller.ctx = context.Background()
	poller.cancel = func() {}
	poller.authState.Store(pollerAuthResult{ok: true, checkedAt: time.Now()})
	poller.SetInstances(insts)
	return poller
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
			inst := &Instance{
				Title:                  "legacy-terminal-" + tt.name,
				Status:                 Active,
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

// SetGitHubPR replaces URL and number but never touches GitHubPRStatusTerminal,
// so a new PR inherits the previous PR's terminal flag (the stale-flag defect
// the backfill and AttachPR must account for).
func TestLegacySetGitHubPR_OverwriteLeavesTerminalFlagStale(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:                  "legacy-set-pr-stale",
		GitHubOwner:            "acme",
		GitHubRepo:             "app",
		GitHubPRURL:            "https://github.com/acme/app/pull/11",
		GitHubPRNumber:         11,
		GitHubPRStatusTerminal: true,
	}
	finishInstanceConstruction(inst)

	inst.SetGitHubPR("https://github.com/acme/app/pull/12", 12)

	snap := inst.Snapshot().GitHub
	if snap.GitHubPRNumber != 12 || snap.GitHubPRURL != "https://github.com/acme/app/pull/12" {
		t.Fatalf("SetGitHubPR did not overwrite: number=%d url=%q", snap.GitHubPRNumber, snap.GitHubPRURL)
	}
	if !snap.GitHubPRStatusTerminal {
		t.Error("GitHubPRStatusTerminal was cleared by SetGitHubPR; legacy behavior leaves it stale (true)")
	}
}

// scanAndLinkPRURL links at most once: with prURLLinked already true it never
// rewrites the PR, and a first link also leaves a stale terminal flag in place.
func TestLegacyScanAndLinkPRURL_LinksOnce(t *testing.T) {
	t.Parallel()
	const first = "remote:      https://github.com/acme/app/pull/42\n"
	const second = "remote:      https://github.com/acme/app/pull/43\n"

	inst := &Instance{
		Title:                  "legacy-scan-link-once",
		GitHubOwner:            "acme",
		GitHubRepo:             "app",
		GitHubPRStatusTerminal: true,
	}
	finishInstanceConstruction(inst)

	linked := scanAndLinkPRURL(inst, true, false, nil, first)
	if !linked {
		t.Fatal("first scan: want linked=true")
	}
	if got := inst.Snapshot().GitHub.GitHubPRNumber; got != 42 {
		t.Fatalf("first scan: number = %d, want 42", got)
	}
	if !inst.Snapshot().GitHub.GitHubPRStatusTerminal {
		t.Error("scanAndLinkPRURL clears GitHubPRStatusTerminal; legacy behavior leaves it stale (true)")
	}

	linked = scanAndLinkPRURL(inst, true, linked, nil, second)
	if !linked {
		t.Error("second scan: want linked to stay true")
	}
	if got := inst.Snapshot().GitHub.GitHubPRNumber; got != 42 {
		t.Errorf("second scan: number = %d, want 42 (link-once, never rewritten)", got)
	}
}
