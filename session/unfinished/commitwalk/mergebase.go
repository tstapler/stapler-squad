package commitwalk

import (
	"errors"
	"fmt"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// MaxReachableSetCommits caps the number of commits ReachableSet will walk —
// named to match this file's other max* caps (maxUntrackedFiles, MaxLCSLines)
// rather than the previous reachableSetLimit. The same kind of bound
// FindMergeBase already applies via MergeBaseBFSLimit.
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
var MaxReachableSetCommits = 50_000

// ReachableSet returns the set of commits reachable from start, up to
// MaxReachableSetCommits commits.
func ReachableSet(repo *git.Repository, start plumbing.Hash) (map[plumbing.Hash]bool, error) {
	seen := make(map[plumbing.Hash]bool, 64)
	iter, err := repo.Log(&git.LogOptions{From: start})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	if err := iter.ForEach(func(c *object.Commit) error {
		if len(seen) >= MaxReachableSetCommits {
			return storer.ErrStop
		}
		seen[c.Hash] = true
		return nil
	}); err != nil {
		return nil, err
	}
	return seen, nil
}

// MergeBaseBFSLimit caps the number of commits visited per side in FindMergeBase
// and CountCommitsTo. For a 10K-commit repo the unbounded walk allocates ~640 KB;
// bounding at 2000 limits that to ~128 KB while still covering typical branch
// divergences. A var (not const) so tests can shrink it instead of building a
// repo with thousands of real commits to exercise the cap.
var MergeBaseBFSLimit = 2000

// CommitInfo is the part of a decoded commit the merge-base walks need.
type CommitInfo struct {
	Parents []plumbing.Hash
	When    int64 // committer time, unix seconds; orders the paint-down queue
}

// ParentMemo caches decoded commit info for the duration of one ahead/behind
// computation, so FindMergeBase and the two CountCommitsTo walks decode each
// commit at most once (go-git's Commit decode dominated the allocs profile).
// Not safe for concurrent use; callers hold the repo entry's lock.
type ParentMemo struct {
	repo    *git.Repository
	parents map[plumbing.Hash]CommitInfo
}

// NewParentMemo returns an empty memo over repo.
func NewParentMemo(repo *git.Repository) *ParentMemo {
	return &ParentMemo{repo: repo, parents: make(map[plumbing.Hash]CommitInfo, 64)}
}

// Info returns h's parents and committer time, decoding it at most once.
func (pm *ParentMemo) Info(h plumbing.Hash) (CommitInfo, error) {
	if ci, ok := pm.parents[h]; ok {
		return ci, nil
	}
	c, err := pm.repo.CommitObject(h)
	if err != nil {
		return CommitInfo{}, err
	}
	ci := CommitInfo{Parents: c.ParentHashes, When: c.Committer.When.Unix()}
	pm.parents[h] = ci
	return ci, nil
}

// Get returns h's parents.
func (pm *ParentMemo) Get(h plumbing.Hash) ([]plumbing.Hash, error) {
	ci, err := pm.Info(h)
	return ci.Parents, err
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

// FindMergeBase returns a best common ancestor of h1 and h2 (one that is not an
// ancestor of another common ancestor) using git's paint-down-to-common: walk
// newest-first from both tips, flag each commit with the sides that reach it, and
// stop once only stale (already-common) commits remain. Cost scales with the
// distance to the merge base, not with history depth. The walk visits at most
// 2*MergeBaseBFSLimit commits; if no merge base is found within that, an error is
// returned. With clock skew the result can differ from a full-graph answer, as in git.
func FindMergeBase(pm ParentSource, h1, h2 plumbing.Hash) (plumbing.Hash, error) {
	if h1 == h2 {
		return h1, nil
	}
	flags := map[plumbing.Hash]uint8{h1: paintFromHead, h2: paintFromBase}
	var q paintQueue
	for _, h := range []plumbing.Hash{h1, h2} {
		ci, err := pm.Info(h)
		if err != nil && !errors.Is(err, plumbing.ErrObjectNotFound) {
			return plumbing.ZeroHash, fmt.Errorf("commit object %s: %w", h, err)
		}
		q.push(paintItem{hash: h, when: ci.When})
	}
	var candidates []plumbing.Hash
	visited := 0
	for len(q.items) > 0 && anyFresh(q, flags) {
		if visited >= 2*MergeBaseBFSLimit {
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
		ci, err := pm.Info(it.hash)
		if err != nil {
			if errors.Is(err, plumbing.ErrObjectNotFound) {
				continue // missing from shallow clone or pack — skip
			}
			return plumbing.ZeroHash, fmt.Errorf("commit object %s: %w", it.hash, err)
		}
		for _, p := range ci.Parents {
			if flags[p]|f == flags[p] {
				continue
			}
			flags[p] |= f
			pci, perr := pm.Info(p)
			if perr != nil {
				if errors.Is(perr, plumbing.ErrObjectNotFound) {
					continue
				}
				return plumbing.ZeroHash, fmt.Errorf("commit object %s: %w", p, perr)
			}
			q.push(paintItem{hash: p, when: pci.When})
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
	return plumbing.ZeroHash, fmt.Errorf("merge base not found within %d commits", MergeBaseBFSLimit)
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
// missing from a shallow clone or pack are skipped, as in FindMergeBase.
// A bound hit or a miss after pruning is counted (recordRedundancyOutcome): both
// are where clock skew can leave a redundant candidate in place.
// repostate.Walk.reaches must stay in lockstep with this.
func isAncestorOfOther(pm ParentSource, c plumbing.Hash, candidates []plumbing.Hash) (bool, error) {
	if len(candidates) < 2 {
		return false, nil
	}
	cInfo, err := pm.Info(c)
	if err != nil && !errors.Is(err, plumbing.ErrObjectNotFound) {
		return false, fmt.Errorf("commit object %s: %w", c, err)
	}
	for _, o := range candidates {
		if o == c {
			continue
		}
		seen := map[plumbing.Hash]struct{}{}
		stack := []plumbing.Hash{o}
		pruned := false
		for len(stack) > 0 {
			if len(seen) >= MergeBaseBFSLimit {
				recordRedundancyOutcome(RedundancyBound)
				return false, nil
			}
			h := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if _, dup := seen[h]; dup {
				continue
			}
			seen[h] = struct{}{}
			hi, err := pm.Info(h)
			if err != nil {
				if errors.Is(err, plumbing.ErrObjectNotFound) {
					continue
				}
				return false, fmt.Errorf("commit object %s: %w", h, err)
			}
			if hi.When < cInfo.When {
				pruned = true
				continue
			}
			for _, p := range hi.Parents {
				if p == c {
					return true, nil
				}
				stack = append(stack, p)
			}
		}
		if pruned {
			recordRedundancyOutcome(RedundancyPrunedMiss)
		}
	}
	return false, nil
}

// CountCommitsTo counts commits reachable from start that are not reachable from
// stop (i.e. the number of commits between start and stop exclusive). Bounded at
// MergeBaseBFSLimit for the same reason FindMergeBase is: an unbounded walk on a
// long-lived branch that never merged into base would otherwise decode every
// commit back to the repo root. When the bound is hit the count is capped rather
// than exact — callers display it as "MergeBaseBFSLimit+" ahead/behind, which is
// the correct signal for "diverged a lot" without paying for an exact count.
func CountCommitsTo(pm ParentSource, start, stop plumbing.Hash) (int, error) {
	seen := make(map[plumbing.Hash]bool, 32)
	q := []plumbing.Hash{start}
	n := 0
	for len(q) > 0 && n < MergeBaseBFSLimit {
		h := q[len(q)-1]
		q = q[:len(q)-1]
		if seen[h] || h == stop {
			continue
		}
		seen[h] = true
		n++
		parents, err := pm.Get(h)
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
