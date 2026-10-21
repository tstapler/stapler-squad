package unfinished

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/tstapler/stapler-squad/session/unfinished/repostate"
)

var errNoBaseResolved = errors.New("no worktree could resolve the base ref")

// refResolveCounters count the ref work a RefSnapshot does, so a test (and the
// soak) can assert it happened once per Burst rather than once per worktree.
type refResolveCounters struct {
	snapshots, baseResolves, headResolves, shallowStats atomic.Int64
}

// RefResolveStats is a snapshot of refResolveCounters.
type RefResolveStats struct {
	Snapshots, BaseResolves, HeadResolves, ShallowStats int64
}

// RefResolveStats returns the ref-resolution counters.
func (g *GoGitVCSReader) RefResolveStats() RefResolveStats {
	c := &g.scanSkip.refs
	return RefResolveStats{
		Snapshots:    c.snapshots.Load(),
		BaseResolves: c.baseResolves.Load(),
		HeadResolves: c.headResolves.Load(),
		ShallowStats: c.shallowStats.Load(),
	}
}

var _ refSnapshotReader = (*GoGitVCSReader)(nil)

// ResolveRefSnapshot resolves base once, each worktree's HEAD once, and stats
// `<commondir>/shallow` once. It reads refs and stats only: no commit is
// decoded and no graph is touched.
func (g *GoGitVCSReader) ResolveRefSnapshot(repoPath, base string, worktrees []string) *RefSnapshot {
	g.scanSkip.refs.snapshots.Add(1)
	dirtyGen := g.effectiveFlags().ScanSkipDirtyGen
	snap := &RefSnapshot{Base: base, Worktrees: make(map[string]WorktreeRefs, len(worktrees))}
	baseDone := false
	for _, wt := range worktrees {
		var refs WorktreeRefs
		entry, err := g.openRepoEntry(wt)
		if err != nil {
			refs.Err = fmt.Errorf("open %s: %w", wt, err)
		} else {
			entry.mu.Lock()
			g.scanSkip.refs.headResolves.Add(1)
			head, headErr := entry.repo.Head()
			if headErr != nil {
				refs.Err = fmt.Errorf("head: %w", headErr)
			} else {
				refs.Head = head.Hash()
			}
			if !baseDone {
				g.scanSkip.refs.baseResolves.Add(1)
				snap.BaseHash, snap.BaseErr = resolveRef(entry.repo, base)
				baseDone = true
			}
			entry.mu.Unlock()
		}
		if sig, ok := statPathSig(filepath.Join(worktreeGitDir(wt), "index")); ok {
			refs.Index = sig
		}
		if dirtyGen {
			refs.DirtyGen = g.dirtyGenFor(wt).Gen()
		}
		snap.Worktrees[wt] = refs
	}
	if !baseDone {
		snap.BaseErr = errNoBaseResolved
	}
	snap.Shallow = g.statShallow(repoPath)
	return snap
}

// statShallow stats `<commondir>/shallow` (repo-level: git keeps it in the
// common dir), counted by the statFn seam like every other stat.
func (g *GoGitVCSReader) statShallow(repoPath string) repostate.ShallowStamp {
	g.scanSkip.refs.shallowStats.Add(1)
	g.noteStat("shallow")
	fi, err := os.Stat(filepath.Join(gitCommonDir(repoPath), "shallow"))
	if err != nil {
		return repostate.ShallowStamp{}
	}
	return repostate.ShallowStamp{Exists: true, Size: fi.Size(), MtimeNs: fi.ModTime().UnixNano()}
}

// AheadBehindResolved is AheadBehind's phase 2 from hashes the caller already
// resolved. A cache hit needs matching hashes, not just an unexpired TTL, so a
// result for an older head or base is never served for a newer one. With
// shared_commit_state on it defers to AheadBehind, which owns the shared walk.
func (g *GoGitVCSReader) AheadBehindResolved(worktreePath, base string, head, baseHash plumbing.Hash) (int, int, error) {
	key := aheadBehindKey{worktree: worktreePath, base: base}
	if e, ok := g.aheadBehindCache.Load(key); ok && time.Now().Before(e.expiry) && e.headHash == head && e.baseHash == baseHash {
		return e.ahead, e.behind, nil
	}
	entry, err := g.openRepoEntry(worktreePath)
	if err != nil {
		return 0, 0, fmt.Errorf("open repo %s: %w", worktreePath, err)
	}
	if g.repoState(entry) != nil {
		return g.AheadBehind(worktreePath, base)
	}

	type abResult struct{ ahead, behind int }
	defer g.guardInvalidation(worktreePath)()
	r, err := sfDo(&g.aheadBehindSF, worktreePath+"\x00"+base+"\x00"+head.String()+"\x00"+baseHash.String(), func() (abResult, error) {
		entry.mu.Lock()
		defer entry.mu.Unlock() // REQUIRED: defer so a panic releases it
		var res abResult
		if head != baseHash {
			ahead, behind, abErr := aheadBehindFrom(newParentMemo(entry.repo), head, baseHash)
			if abErr != nil {
				return abResult{}, abErr
			}
			res = abResult{ahead, behind}
		}
		g.aheadBehindCache.Store(key, aheadBehindEntry{ahead: res.ahead, behind: res.behind, headHash: head, baseHash: baseHash, expiry: time.Now().Add(diffStatCacheTTL)})
		return res, nil
	})
	if err != nil {
		return 0, 0, err
	}
	return r.ahead, r.behind, nil
}
