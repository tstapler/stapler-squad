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
	dirty, _, err := g.hasUncommittedSkipPass(entry, worktreePath, false)
	return dirty, err
}

// hasUncommittedSkipPass is hasUncommittedSkip; with collect it also finishes
// the stat pass after the first change and returns every file's lstat result,
// so DiffShortstat in the same scan does not stat the files again (Story
// 4.2.4). The pass is nil unless the worktree is dirty by its tracked files or
// its untracked files.
func (g *GoGitVCSReader) hasUncommittedSkipPass(entry *cachedRepo, worktreePath string, collect bool) (bool, *statPass, error) {
	force, done := g.scanSkipDecision(worktreePath)
	pi, matcher, err := g.hasUncommittedIndexPhaseCached(entry, worktreePath, force)
	if err != nil {
		return false, nil, err
	}
	if pi.stagedDirty {
		return g.storeUncommitted(worktreePath, true), nil, nil
	}

	pass := &statPass{tracked: pi.tracked}
	if collect {
		pass.res = make([]statRes, len(pi.tracked))
	}
	changed, err := g.statTrackedFiles(worktreePath, pass, collect)
	if err != nil {
		return false, nil, err
	}
	if changed {
		if !collect {
			pass = nil
		}
		return g.storeUncommitted(worktreePath, true), pass, nil
	}

	r, err := g.hasUntrackedSkip(worktreePath, pi.indexed, matcher, pi.gen(), force)
	if err != nil {
		return false, nil, err
	}
	done()
	if !r || !collect {
		pass = nil
	}
	return g.storeUncommitted(worktreePath, r), pass, nil
}

// statTrackedFiles runs the per-file lstat pass over pass.tracked and reports
// whether any file differs from its index entry. Without collect it stops at
// the first difference; with collect it records every file's result in
// pass.res and finishes the pass.
func (g *GoGitVCSReader) statTrackedFiles(worktreePath string, pass *statPass, collect bool) (changed bool, err error) {
	stat := newDirStatter(worktreePath)
	defer stat.close()
	for i, tf := range pass.tracked {
		if g.statFn != nil {
			g.statFn(tf.name)
		}
		size, mtime, serr := stat.lstat(tf.name)
		var differs bool
		switch {
		case serr == nil:
			differs = size != int64(tf.size) || !mtime.Equal(tf.modifiedAt)
			if collect {
				pass.res[i] = statRes{size: size, mtimeNs: mtime.UnixNano(), code: statOK}
			}
		case errors.Is(serr, fs.ErrNotExist):
			differs = !tf.skipWorktree // deleted tracked file; an absent sparse entry is not a deletion
			if collect {
				pass.res[i] = statRes{code: statMissing}
			}
		default:
			if collect {
				pass.res[i] = statRes{code: statFailed}
			}
		}
		if differs {
			if !collect {
				return true, nil
			}
			changed = true
		}
	}
	return changed, nil
}

// storeUncommitted caches a HasUncommitted verdict for diffStatCacheTTL.
func (g *GoGitVCSReader) storeUncommitted(worktreePath string, r bool) bool {
	g.hasUncommittedCache.Store(worktreePath, hasUncommittedEntry{
		result: r, expiry: time.Now().Add(diffStatCacheTTL),
	})
	return r
}
