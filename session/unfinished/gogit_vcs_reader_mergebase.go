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

// commitInfo is the part of a decoded commit the merge-base walks need.
type commitInfo struct {
	parents []plumbing.Hash
	when    int64 // committer time, unix seconds; orders the paint-down queue
}

// parentMemo caches decoded commit info for the duration of one ahead/behind
// computation, so findMergeBase and the two countCommitsTo walks decode each
// commit at most once (go-git's Commit decode dominated the allocs profile).
// Not safe for concurrent use; callers hold the repo entry's lock.
type parentMemo struct {
	repo    *git.Repository
	parents map[plumbing.Hash]commitInfo
}

func newParentMemo(repo *git.Repository) *parentMemo {
	return &parentMemo{repo: repo, parents: make(map[plumbing.Hash]commitInfo, 64)}
}

func (pm *parentMemo) info(h plumbing.Hash) (commitInfo, error) {
	if ci, ok := pm.parents[h]; ok {
		return ci, nil
	}
	c, err := pm.repo.CommitObject(h)
	if err != nil {
		return commitInfo{}, err
	}
	ci := commitInfo{parents: c.ParentHashes, when: c.Committer.When.Unix()}
	pm.parents[h] = ci
	return ci, nil
}

func (pm *parentMemo) get(h plumbing.Hash) ([]plumbing.Hash, error) {
	ci, err := pm.info(h)
	return ci.parents, err
}

const (
	paintFromHead uint8 = 1 << iota
	paintFromBase
	paintStale
)

type paintItem struct {
	hash plumbing.Hash
	when int64
	seq  int // insertion order; breaks committer-time ties FIFO so equal-time history walks breadth-first
}

// paintQueue is a max-heap on committer time (newest first, then oldest insertion),
// like git's paint_down_to_common priority queue.
type paintQueue struct {
	items []paintItem
	next  int
}

func (a paintItem) before(b paintItem) bool {
	return a.when > b.when || (a.when == b.when && a.seq < b.seq)
}

func (q *paintQueue) push(it paintItem) {
	it.seq = q.next
	q.next++
	q.items = append(q.items, it)
	i := len(q.items) - 1
	for i > 0 {
		p := (i - 1) / 2
		if !q.items[i].before(q.items[p]) {
			break
		}
		q.items[p], q.items[i] = q.items[i], q.items[p]
		i = p
	}
}

func (q *paintQueue) pop() paintItem {
	top := q.items[0]
	n := len(q.items) - 1
	q.items[0] = q.items[n]
	q.items = q.items[:n]
	i := 0
	for {
		l, r, m := 2*i+1, 2*i+2, i
		if l < n && q.items[l].before(q.items[m]) {
			m = l
		}
		if r < n && q.items[r].before(q.items[m]) {
			m = r
		}
		if m == i {
			break
		}
		q.items[m], q.items[i] = q.items[i], q.items[m]
		i = m
	}
	return top
}

// findMergeBase returns a best common ancestor of h1 and h2 (one that is not an
// ancestor of another common ancestor) using git's paint-down-to-common: walk
// newest-first from both tips, flag each commit with the sides that reach it, and
// stop once only stale (already-common) commits remain. Cost scales with the
// distance to the merge base, not with history depth. The walk visits at most
// 2*mergeBaseBFSLimit commits; if no merge base is found within that, an error is
// returned. With clock skew the result can differ from a full-graph answer, as in git.
func findMergeBase(pm parentSource, h1, h2 plumbing.Hash) (plumbing.Hash, error) {
	if h1 == h2 {
		return h1, nil
	}
	flags := map[plumbing.Hash]uint8{h1: paintFromHead, h2: paintFromBase}
	var q paintQueue
	for _, h := range []plumbing.Hash{h1, h2} {
		ci, err := pm.info(h)
		if err != nil && !errors.Is(err, plumbing.ErrObjectNotFound) {
			return plumbing.ZeroHash, fmt.Errorf("commit object %s: %w", h, err)
		}
		q.push(paintItem{hash: h, when: ci.when})
	}
	var candidates []plumbing.Hash
	visited := 0
	for len(q.items) > 0 && anyFresh(q, flags) {
		if visited >= 2*mergeBaseBFSLimit {
			break
		}
		it := q.pop()
		visited++
		f := flags[it.hash]
		if f&paintStale != 0 {
			f = paintStale | paintFromHead | paintFromBase
		} else if f&(paintFromHead|paintFromBase) == paintFromHead|paintFromBase {
			candidates = append(candidates, it.hash)
			f |= paintStale
			flags[it.hash] = f
		}
		ci, err := pm.info(it.hash)
		if err != nil {
			if errors.Is(err, plumbing.ErrObjectNotFound) {
				continue // missing from shallow clone or pack — skip
			}
			return plumbing.ZeroHash, fmt.Errorf("commit object %s: %w", it.hash, err)
		}
		for _, p := range ci.parents {
			if flags[p]|f == flags[p] {
				continue
			}
			flags[p] |= f
			pci, perr := pm.info(p)
			if perr != nil {
				if errors.Is(perr, plumbing.ErrObjectNotFound) {
					continue
				}
				return plumbing.ZeroHash, fmt.Errorf("commit object %s: %w", p, perr)
			}
			q.push(paintItem{hash: p, when: pci.when})
		}
	}
	// Re-check staleness: a candidate painted stale by a later candidate is
	// redundant. The first survivor wins, so later candidates are never checked
	// (and cannot raise a bound error), matching repostate.Walk.bestCandidate.
	for _, c := range candidates {
		if flags[c]&paintStale != 0 {
			redundant, err := isAncestorOfOther(pm, c, candidates)
			if err != nil {
				return plumbing.ZeroHash, err
			}
			if redundant {
				continue
			}
		}
		return c, nil
	}
	return plumbing.ZeroHash, fmt.Errorf("merge base not found within %d commits", mergeBaseBFSLimit)
}

// anyFresh reports whether q holds a commit not yet marked stale.
func anyFresh(q paintQueue, flags map[plumbing.Hash]uint8) bool {
	for _, it := range q.items {
		if flags[it.hash]&paintStale == 0 {
			return true
		}
	}
	return false
}

// isAncestorOfOther reports whether c is a strict ancestor of any other candidate.
// Only runs when several common ancestors were found (criss-cross merges).
// The DFS prunes commits whose committer time is older than c's (they cannot
// have c as an ancestor, the same assumption as the paint-down), so a genuine
// "not an ancestor" ends early instead of walking all of o's history. If the
// visit bound is hit anyway, c is treated as NOT redundant: a valid common
// ancestor, where failing would turn a usable answer into an error. Commits
// missing from a shallow clone or pack are skipped, as in findMergeBase.
// repostate.Walk.reaches must stay in lockstep with this.
func isAncestorOfOther(pm parentSource, c plumbing.Hash, candidates []plumbing.Hash) (bool, error) {
	if len(candidates) < 2 {
		return false, nil
	}
	cInfo, err := pm.info(c)
	if err != nil && !errors.Is(err, plumbing.ErrObjectNotFound) {
		return false, fmt.Errorf("commit object %s: %w", c, err)
	}
	for _, o := range candidates {
		if o == c {
			continue
		}
		seen := map[plumbing.Hash]struct{}{}
		stack := []plumbing.Hash{o}
		for len(stack) > 0 {
			if len(seen) >= mergeBaseBFSLimit {
				return false, nil
			}
			h := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if _, dup := seen[h]; dup {
				continue
			}
			seen[h] = struct{}{}
			hi, err := pm.info(h)
			if err != nil {
				if errors.Is(err, plumbing.ErrObjectNotFound) {
					continue
				}
				return false, fmt.Errorf("commit object %s: %w", h, err)
			}
			if hi.when < cInfo.when {
				continue
			}
			for _, p := range hi.parents {
				if p == c {
					return true, nil
				}
				stack = append(stack, p)
			}
		}
	}
	return false, nil
}

// countCommitsTo counts commits reachable from start that are not reachable from
// stop (i.e. the number of commits between start and stop exclusive). Bounded at
// mergeBaseBFSLimit for the same reason findMergeBase is: an unbounded walk on a
// long-lived branch that never merged into base would otherwise decode every
// commit back to the repo root. When the bound is hit the count is capped rather
// than exact — callers display it as "mergeBaseBFSLimit+" ahead/behind, which is
// the correct signal for "diverged a lot" without paying for an exact count.
func countCommitsTo(pm parentSource, start, stop plumbing.Hash) (int, error) {
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
