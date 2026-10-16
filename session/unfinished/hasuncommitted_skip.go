package unfinished

import (
	"errors"
	"io/fs"
	"time"
)

// hasUncommittedSkip is HasUncommitted under scan_skip: the parsed index comes
// from the IndexStatSig cache (Story 4.2.2) instead of a decode per scan. The
// per-file stat pass always runs: no cache state may license skipping it
// (ADR-006 hard rule; a tracked file edited in place changes no index bytes).
// Verdicts equal the legacy path except for one deliberate fix: an absent
// skip-worktree (sparse) entry is not reported deleted.
func (g *GoGitVCSReader) hasUncommittedSkip(entry *cachedRepo, worktreePath string) (bool, error) {
	force, done := g.scanSkipDecision(worktreePath)
	pi, matcher, err := g.hasUncommittedIndexPhaseCached(entry, worktreePath, force)
	if err != nil {
		return false, err
	}
	if pi.stagedDirty {
		return g.storeUncommitted(worktreePath, true), nil
	}

	stat := newDirStatter(worktreePath)
	defer stat.close()
	for _, tf := range pi.tracked {
		if g.statFn != nil {
			g.statFn(tf.name)
		}
		size, mtime, serr := stat.lstat(tf.name)
		if serr != nil {
			if errors.Is(serr, fs.ErrNotExist) && !tf.skipWorktree {
				return g.storeUncommitted(worktreePath, true), nil // tracked file deleted
			}
			continue
		}
		if size != int64(tf.size) || !mtime.Equal(tf.modifiedAt) {
			return g.storeUncommitted(worktreePath, true), nil
		}
	}

	r, err := g.hasUntrackedSkip(worktreePath, pi.indexed, matcher, pi.gen(), force)
	if err != nil {
		return false, err
	}
	done()
	return g.storeUncommitted(worktreePath, r), nil
}

// storeUncommitted caches a HasUncommitted verdict for diffStatCacheTTL.
func (g *GoGitVCSReader) storeUncommitted(worktreePath string, r bool) bool {
	g.hasUncommittedCache.Store(worktreePath, hasUncommittedEntry{
		result: r, expiry: time.Now().Add(diffStatCacheTTL),
	})
	return r
}
