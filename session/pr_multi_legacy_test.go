package session

import (
	"testing"
)

// Characterization tests of today's single-PR behavior (session-multi-pr-tracking
// T1.1.1). They pin the flag-off path so later multi-PR work provably leaves it
// unchanged; a failure here means legacy behavior moved, not that the test is wrong.

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
	FinishInstanceConstruction(inst)

	inst.SetGitHubPR("https://github.com/acme/app/pull/12", 12)

	snap := inst.Snapshot().GitHub
	if snap.GitHubPRNumber != 12 || snap.GitHubPRURL != "https://github.com/acme/app/pull/12" {
		t.Fatalf("SetGitHubPR did not overwrite: number=%d url=%q", snap.GitHubPRNumber, snap.GitHubPRURL)
	}
	if !snap.GitHubPRStatusTerminal {
		t.Error("GitHubPRStatusTerminal was cleared by SetGitHubPR; legacy behavior leaves it stale (true)")
	}
}

// The terminal scan now attaches every push URL it sees instead of linking once,
// and clears the stored terminal flag when it changes the number of a session
// that cannot hold a link (T2.2.1). A session that already tracks #42 keeps it.
func TestLegacyScanAndLinkPRURL_AccumulatesLinks(t *testing.T) {
	t.Parallel()
	const first = "remote:      https://github.com/acme/app/pull/42\n"
	const second = "remote:      https://github.com/acme/app/pull/43\n"

	inst := newPRTestInstance(t)
	seen := scannedPRURLs{}

	scanAndLinkPRURL(inst, true, seen, nil, first)
	if got := attachNumbers(t, inst); len(got) != 1 || got[0] != 42 {
		t.Fatalf("first scan: PRs = %v, want [42]", got)
	}
	scanAndLinkPRURL(inst, true, seen, nil, first+second)
	if got := attachNumbers(t, inst); len(got) != 2 || got[0] != 42 || got[1] != 43 {
		t.Errorf("second scan: PRs = %v, want [42 43] (a later push URL is scanned too)", got)
	}
}
