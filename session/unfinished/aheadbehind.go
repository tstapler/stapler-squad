package unfinished

import (
	"context"
	"errors"
	"fmt"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"

	"github.com/tstapler/stapler-squad/session/unfinished/repostate"
)

// parentSource supplies commit parents and committer times to the merge-base
// and counting walks. parentMemo is the per-call implementation.
type parentSource interface {
	info(h plumbing.Hash) (commitInfo, error)
	get(h plumbing.Hash) ([]plumbing.Hash, error)
}

var _ parentSource = (*parentMemo)(nil)

// aheadBehindLocked computes (ahead, behind) for entry's HEAD against key.base
// on the plain walk, reusing or storing the TTL cache entry. The caller holds
// entry.mu across the whole call (the legacy path; the shared path calls
// aheadBehindShared, which holds it only for phase 1).
func (g *GoGitVCSReader) aheadBehindLocked(entry *cachedRepo, key aheadBehindKey) (int, int, error) {
	head, baseHash, err := resolveAheadBehindRefs(entry, key.base)
	if err != nil {
		return 0, 0, err
	}
	if ahead, behind, ok := g.aheadBehindMemoized(key, head, baseHash); ok {
		return ahead, behind, nil
	}
	return g.aheadBehindPlain(entry, key, head, baseHash)
}

// aheadBehindMemoized answers without any walk when head == base or when a
// TTL-expired entry's head/base hashes still match the current ones: neither
// ref moved since the last computation, so the result can't have changed, and
// reusing it skips findMergeBase and two countCommitsTo walks (the actual
// expensive work; resolving HEAD/base is cheap by comparison).
func (g *GoGitVCSReader) aheadBehindMemoized(key aheadBehindKey, head, baseHash plumbing.Hash) (ahead, behind int, ok bool) {
	if head == baseHash {
		g.aheadBehindCache.Store(key, aheadBehindEntry{headHash: head, baseHash: baseHash, expiry: time.Now().Add(diffStatCacheTTL)})
		return 0, 0, true
	}
	if e, hit := g.aheadBehindCache.Load(key); hit && e.headHash == head && e.baseHash == baseHash {
		e.expiry = time.Now().Add(diffStatCacheTTL)
		g.aheadBehindCache.Store(key, e)
		return e.ahead, e.behind, true
	}
	return 0, 0, false
}

// aheadBehindPlain is the plainWalk: per-call parentMemo, caller holds entry.mu.
func (g *GoGitVCSReader) aheadBehindPlain(entry *cachedRepo, key aheadBehindKey, head, baseHash plumbing.Hash) (int, int, error) {
	ahead, behind, err := aheadBehindFrom(newParentMemo(entry.repo), head, baseHash)
	if err != nil {
		return 0, 0, err
	}
	g.storeAheadBehind(key, ahead, behind, head, baseHash)
	return ahead, behind, nil
}

func (g *GoGitVCSReader) storeAheadBehind(key aheadBehindKey, ahead, behind int, head, baseHash plumbing.Hash) {
	g.aheadBehindCache.Store(key, aheadBehindEntry{ahead: ahead, behind: behind, headHash: head, baseHash: baseHash, expiry: time.Now().Add(diffStatCacheTTL)})
}

// aheadBehindCompute is the call-site switch behind shared_commit_state: flag
// off (or a storer without the extension slot) runs the legacy path under
// entry.mu; flag on walks the shared commit graph without it.
func (g *GoGitVCSReader) aheadBehindCompute(entry *cachedRepo, key aheadBehindKey) (int, int, error) {
	rs := g.repoState(entry)
	if rs == nil {
		entry.mu.Lock()
		defer entry.mu.Unlock() // REQUIRED: defer, never explicit unlocks, so a panic releases it
		return g.aheadBehindLocked(entry, key)
	}
	return g.aheadBehindShared(entry, key, rs)
}

// aheadBehindShared: phase 1 (refs, under entry.mu, released before any graph
// work), then phase 2 on the shared id graph with no entry.mu. A fallback error
// (full arena, severe pressure, closed or failing storer) reruns the plain walk
// under entry.mu for this call; its result is the same one.
func (g *GoGitVCSReader) aheadBehindShared(entry *cachedRepo, key aheadBehindKey, rs *repostate.RepoState) (int, int, error) {
	head, baseHash, err := g.resolveRefsLocked(entry, key.base)
	if err != nil {
		return 0, 0, err
	}
	if ahead, behind, ok := g.aheadBehindMemoized(key, head, baseHash); ok {
		return ahead, behind, nil
	}
	ord, shallow := g.observeShallow(entry, rs)
	ahead, behind, err := aheadBehindOnGraph(rs, newGraphSource(entry.repo.Storer, shallow), ord, head, baseHash)
	if errors.Is(err, repostate.ErrFallback) {
		entry.mu.Lock()
		defer entry.mu.Unlock()
		return g.aheadBehindPlain(entry, key, head, baseHash)
	}
	if err != nil {
		return 0, 0, err
	}
	g.storeAheadBehind(key, ahead, behind, head, baseHash)
	return ahead, behind, nil
}

// resolveRefsLocked is phase 1 under a brief entry.mu.
func (g *GoGitVCSReader) resolveRefsLocked(entry *cachedRepo, base string) (head, baseHash plumbing.Hash, err error) {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	return resolveAheadBehindRefs(entry, base)
}

// aheadBehindOnGraph runs phase 2 on the id graph; it reads no cachedRepo.
func aheadBehindOnGraph(rs *repostate.RepoState, src repostate.ParentsSource, ord repostate.StampOrd, head, base plumbing.Hash) (int, int, error) {
	w := rs.NewWalk(context.Background(), src, ord, mergeBaseBFSLimit)
	defer w.Close()
	return aheadBehindWalk(&w, head, base)
}

// aheadBehindWalk is aheadBehindFrom over interned ids.
func aheadBehindWalk(w *repostate.Walk, head, base plumbing.Hash) (ahead, behind int, err error) {
	mb, err := w.FindMergeBase(head, base)
	if err != nil {
		if errors.Is(err, repostate.ErrFallback) {
			return 0, 0, err
		}
		return 0, 0, fmt.Errorf("merge base: %w", err)
	}
	if ahead, err = w.CountCommitsTo(head, mb); err != nil {
		return 0, 0, err
	}
	if behind, err = w.CountCommitsTo(base, mb); err != nil {
		return 0, 0, err
	}
	return ahead, behind, nil
}

// resolveAheadBehindRefs is phase 1: HEAD and base as hashes. The caller holds
// entry.mu; no commit graph is walked here.
func resolveAheadBehindRefs(entry *cachedRepo, base string) (head, baseHash plumbing.Hash, err error) {
	headRef, headErr := entry.repo.Head()
	if headErr != nil {
		return head, baseHash, fmt.Errorf("head: %w", headErr)
	}
	baseHash, baseErr := resolveRef(entry.repo, base)
	if baseErr != nil {
		return head, baseHash, fmt.Errorf("resolve base %q: %w", base, baseErr)
	}
	return headRef.Hash(), baseHash, nil
}

// aheadBehindFrom is phase 2: a pure function of the commit graph behind src.
// It reads nothing from any cachedRepo.
func aheadBehindFrom(src parentSource, head, base plumbing.Hash) (ahead, behind int, err error) {
	mb, mbErr := findMergeBase(src, head, base)
	if mbErr != nil {
		return 0, 0, fmt.Errorf("merge base: %w", mbErr)
	}
	if ahead, err = countCommitsTo(src, head, mb); err != nil {
		return 0, 0, err
	}
	if behind, err = countCommitsTo(src, base, mb); err != nil {
		return 0, 0, err
	}
	return ahead, behind, nil
}

// resolveCommitMessageRefs snapshots HEAD and base as plain hash values under a
// brief entry.mu, so the walks that follow never hold it.
func resolveCommitMessageRefs(entry *cachedRepo, base string) (head, baseHash plumbing.Hash, err error) {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	headRef, headErr := entry.repo.Head()
	if headErr != nil {
		return head, baseHash, headErr
	}
	baseHash, err = resolveRef(entry.repo, base)
	return headRef.Hash(), baseHash, err
}

// commitMessagesFrom is commitMessagesFromSet over the legacy map.
func commitMessagesFrom(entry *cachedRepo, head plumbing.Hash, baseReachable map[plumbing.Hash]bool, max int) ([]string, error) {
	return commitMessagesFromSet(entry, head, hashSet(baseReachable), max)
}

// commitMessagesFromSet lists up to max "short-hash subject" lines from head back
// to the first commit already in baseReachable. go-git's packfile reader needs
// entry.mu, so it is held for the walk.
func commitMessagesFromSet(entry *cachedRepo, head plumbing.Hash, baseReachable reachSet, max int) ([]string, error) {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	iter, err := entry.repo.Log(&git.LogOptions{From: head})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var result []string
	err = iter.ForEach(func(c *object.Commit) error {
		if baseReachable.Contains(c.Hash) {
			return storer.ErrStop
		}
		if len(result) < max {
			// Mimic `git log --oneline`: short hash + first line of message.
			result = append(result, c.Hash.String()[:7]+" "+firstLine(c.Message))
		}
		return nil
	})
	return result, err
}
