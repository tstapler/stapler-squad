package server

import (
	"fmt"

	"github.com/tstapler/stapler-squad/session/unfinished"
)

// worktreeFingerprint changes when commits land or the upstream moves, which
// is when the PR poller should bypass its per-PR minimum interval. The scan
// result carries no HEAD SHA, so ahead/behind counts plus the ahead commit
// subjects stand in for it.
func worktreeFingerprint(r unfinished.ScanResult) string {
	return fmt.Sprintf("%d/%d/%q", r.AheadCount, r.BehindCount, r.AheadMessages)
}
