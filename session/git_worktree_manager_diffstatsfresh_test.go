package session

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/git"
)

// TestGitWorktreeManager_DiffStatsFresh covers the TTL-cache bookkeeping
// added for GetSessionDiff (see Instance.RefreshDiffStatsIfStale's doc
// comment) — a zero-value manager, and one whose stats were just cleared,
// must both report stale so a caller never skips a needed recompute.
func TestGitWorktreeManager_DiffStatsFresh(t *testing.T) {
	t.Parallel()

	gm := &GitWorktreeManager{}
	if gm.DiffStatsFresh() {
		t.Fatal("zero-value manager should report stale (never computed)")
	}

	gm.SetDiffStats(&git.DiffStats{Added: 3})
	if !gm.DiffStatsFresh() {
		t.Fatal("expected fresh immediately after SetDiffStats")
	}

	gm.ClearDiffStats()
	if gm.DiffStatsFresh() {
		t.Fatal("expected stale after ClearDiffStats — a cleared/nil diff must never be served as a cache hit")
	}
}
