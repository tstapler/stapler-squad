package server

import (
	"fmt"

	"github.com/tstapler/stapler-squad/session/unfinished"
)

// worktreeFingerprint changes when commits land on the worktree's branch, which
// is when the PR poller should bypass its per-PR minimum interval. The scan
// result carries no HEAD SHA, so the ahead count plus the ahead commit
// subjects stand in for it. BehindCount is deliberately excluded: it moves on
// every upstream push and would re-poll every worktree of the repo.
func worktreeFingerprint(r unfinished.ScanResult) string {
	return fmt.Sprintf("%d/%q", r.AheadCount, r.AheadMessages)
}
