package session

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/instance"
)

// Characterization tests of today's single-PR behavior (session-multi-pr-tracking
// T1.1.1). They pin the flag-off path so later multi-PR work provably leaves it
// unchanged; a failure here means legacy behavior moved, not that the test is wrong.

// SetGitHubPR replaces URL and number but never touches GitHubPRStatusTerminal,
// so a new PR inherits the previous PR's terminal flag (the stale-flag defect
// the backfill and AttachPR must account for).
func TestLegacySetGitHubPR_OverwriteLeavesTerminalFlagStale(t *testing.T) {
	t.Parallel()
	inst := &instance.Instance{
		Title:                  "legacy-set-pr-stale",
		GitHubOwner:            "acme",
		GitHubRepo:             "app",
		GitHubPRURL:            "https://github.com/acme/app/pull/11",
		GitHubPRNumber:         11,
		GitHubPRStatusTerminal: true,
	}
	instance.FinishInstanceConstruction(inst)

	inst.SetGitHubPR("https://github.com/acme/app/pull/12", 12)

	snap := inst.Snapshot().GitHub
	if snap.GitHubPRNumber != 12 || snap.GitHubPRURL != "https://github.com/acme/app/pull/12" {
		t.Fatalf("SetGitHubPR did not overwrite: number=%d url=%q", snap.GitHubPRNumber, snap.GitHubPRURL)
	}
	if !snap.GitHubPRStatusTerminal {
		t.Error("GitHubPRStatusTerminal was cleared by SetGitHubPR; legacy behavior leaves it stale (true)")
	}
}
