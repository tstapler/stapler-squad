package unfinished

import (
	"errors"
	"fmt"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// maxReachableSetCommits caps the number of commits reachableSet will walk —
// named to match this file's other max* caps (maxUntrackedFiles, maxLCSLines)
// rather than the previous reachableSetLimit. The same kind of bound
// findMergeBase already applies via mergeBaseBFSLimit.
// The only caller (CommitMessages, via cachedReachableSet) uses the result
// solely to detect "already merged into base" and stop printing — a partial
// set beyond this depth means a handful of already-merged commits could
// appear in the output on a repo with a base branch this deep, a cosmetic
// imperfection preferable to an unbounded walk over the full history.
// Cost at the cap: 50,000 plumbing.Hash keys (20 bytes each) plus Go map
// bucket overhead is a few MB and a few tens of milliseconds of commit-log
// walking — cheap and bounded, versus an unbounded walk that on a
// million-commit repo would cost proportionally more of both.
// Declared as var (not const) so tests can lower it without creating
// thousands of commits.
var maxReachableSetCommits = 50_000

// reachableSet returns the set of commits reachable from start, up to
// maxReachableSetCommits commits.
func reachableSet(repo *git.Repository, start plumbing.Hash) (map[plumbing.Hash]bool, error) {
	seen := make(map[plumbing.Hash]bool, 64)
	iter, err := repo.Log(&git.LogOptions{From: start})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	if err := iter.ForEach(func(c *object.Commit) error {
		if len(seen) >= maxReachableSetCommits {
			return storer.ErrStop
		}
		seen[c.Hash] = true
		return nil
	}); err != nil {
		return nil, err
	}
	return seen, nil
}

// mergeBaseBFSLimit caps the number of commits visited per side in findMergeBase
// and countCommitsTo. For a 10K-commit repo the unbounded walk allocates ~640 KB;
// bounding at 2000 limits that to ~128 KB while still covering typical branch
// divergences. A var (not const) so tests can shrink it instead of building a
// repo with thousands of real commits to exercise the cap.
var mergeBaseBFSLimit = 2000

// parentMemo caches commit parent lists for the duration of one ahead/behind
// computation, so findMergeBase and the two countCommitsTo walks decode each
// commit at most once (go-git's Commit decode dominated the allocs profile).
// Not safe for concurrent use; callers hold the repo entry's lock.
type parentMemo struct {
	repo    *git.Repository
	parents map[plumbing.Hash][]plumbing.Hash
}

func newParentMemo(repo *git.Repository) *parentMemo {
	return &parentMemo{repo: repo, parents: make(map[plumbing.Hash][]plumbing.Hash, 64)}
}

func (pm *parentMemo) get(h plumbing.Hash) ([]plumbing.Hash, error) {
	if ps, ok := pm.parents[h]; ok {
		return ps, nil
	}
	c, err := pm.repo.CommitObject(h)
	if err != nil {
		return nil, err
	}
	pm.parents[h] = c.ParentHashes
	return c.ParentHashes, nil
}

// bfsSide is one frontier of findMergeBase's two-sided walk.
type bfsSide struct {
	queue   []plumbing.Hash
	seen    map[plumbing.Hash]struct{}
	visited int
}

func newBFSSide(start plumbing.Hash) *bfsSide {
	return &bfsSide{queue: []plumbing.Hash{start}, seen: make(map[plumbing.Hash]struct{}, 32)}
}

// advance visits one new commit on s. It reports (hit, true) when that commit is
// already in other.seen, and progressed=false once s is exhausted or at the bound.
func (s *bfsSide) advance(pm *parentMemo, other *bfsSide) (hit plumbing.Hash, found, progressed bool, err error) {
	for len(s.queue) > 0 && s.visited < mergeBaseBFSLimit {
		h := s.queue[0]
		s.queue = s.queue[1:]
		if _, dup := s.seen[h]; dup {
			continue
		}
		s.seen[h] = struct{}{}
		s.visited++
		if _, ok := other.seen[h]; ok {
			return h, true, true, nil
		}
		ps, perr := pm.get(h)
		if perr != nil {
			if errors.Is(perr, plumbing.ErrObjectNotFound) {
				return plumbing.ZeroHash, false, true, nil // missing from shallow clone or pack — skip
			}
			return plumbing.ZeroHash, false, true, fmt.Errorf("commit object %s: %w", h, perr)
		}
		s.queue = append(s.queue, ps...)
		return plumbing.ZeroHash, false, true, nil
	}
	return plumbing.ZeroHash, false, false, nil
}

// findMergeBase returns a common ancestor of h1 and h2 by walking both sides
// breadth-first in lockstep and stopping at the first commit either side has
// already seen. Cost scales with the distance to the merge base, not with
// mergeBaseBFSLimit: the previous implementation fully marked h1's bounded
// ancestry before looking at h2, decoding ~2000 commits per call on any
// long-lived repo. If no merge base is found within the limit per side, an
// error is returned.
func findMergeBase(pm *parentMemo, h1, h2 plumbing.Hash) (plumbing.Hash, error) {
	if h1 == h2 {
		return h1, nil
	}
	s1, s2 := newBFSSide(h1), newBFSSide(h2)
	for {
		hit, found, p1, err := s1.advance(pm, s2)
		if err != nil || found {
			return hit, err
		}
		hit, found, p2, err := s2.advance(pm, s1)
		if err != nil || found {
			return hit, err
		}
		if !p1 && !p2 {
			return plumbing.ZeroHash, fmt.Errorf("merge base not found within %d commits", mergeBaseBFSLimit)
		}
	}
}

// countCommitsTo counts commits reachable from start that are not reachable from
// stop (i.e. the number of commits between start and stop exclusive). Bounded at
// mergeBaseBFSLimit for the same reason findMergeBase is: an unbounded walk on a
// long-lived branch that never merged into base would otherwise decode every
// commit back to the repo root. When the bound is hit the count is capped rather
// than exact — callers display it as "mergeBaseBFSLimit+" ahead/behind, which is
// the correct signal for "diverged a lot" without paying for an exact count.
func countCommitsTo(pm *parentMemo, start, stop plumbing.Hash) (int, error) {
	seen := make(map[plumbing.Hash]bool, 32)
	q := []plumbing.Hash{start}
	n := 0
	for len(q) > 0 && n < mergeBaseBFSLimit {
		h := q[len(q)-1]
		q = q[:len(q)-1]
		if seen[h] || h == stop {
			continue
		}
		seen[h] = true
		n++
		parents, err := pm.get(h)
		if err != nil {
			return 0, err
		}
		for _, p := range parents {
			if !seen[p] && p != stop {
				q = append(q, p)
			}
		}
	}
	return n, nil
}
