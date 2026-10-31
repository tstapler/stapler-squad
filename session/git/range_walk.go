package git

import (
	"container/heap"
	"context"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// Paint colors for walkShippedRange: which tip(s) a commit is reachable from.
const (
	paintHead uint8 = 1 << iota
	paintBase
)

const (
	// rangeWalkSlop is how many more commits to pop after every queued commit is
	// base-reachable. A parent can carry an older timestamp than its child
	// (clock skew), so base's own paint might still be on its way to a commit
	// we've already seen; git's rev-list uses the same slack.
	rangeWalkSlop = 5
	// rangeWalkMaxPops bounds the walk of a pathological range (e.g. a base unrelated to head);
	// hitting it reports the result as truncated.
	rangeWalkMaxPops = 20_000
	// rangeWalkCtxEvery is how many pops pass between ctx checks.
	rangeWalkCtxEvery = 64
)

type paintItem struct {
	commit *object.Commit
	flags  uint8 // flags at push time; the live value is in the paint map
	seq    int   // insertion order, breaks equal-timestamp ties (child before parent)
}

// paintQueue is a max-heap on committer time.
type paintQueue []paintItem

func (q paintQueue) Len() int { return len(q) }
func (q paintQueue) Less(i, j int) bool {
	ti, tj := q[i].commit.Committer.When, q[j].commit.Committer.When
	if !ti.Equal(tj) {
		return ti.After(tj)
	}
	return q[i].seq < q[j].seq
}
func (q paintQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *paintQueue) Push(x any)   { *q = append(*q, x.(paintItem)) }
func (q *paintQueue) Pop() any {
	old := *q
	n := len(old)
	it := old[n-1]
	*q = old[:n-1]
	return it
}

// walkShippedRange returns the commits reachable from head but not from base,
// newest committer time first, capped at maxCommits (truncated reports the cap
// was reached).
//
// It paints head and base and walks parents newest-first, stopping once every
// queued commit is already base-reachable, so it touches roughly the commits
// of the range plus the divergence frontier instead of base's whole ancestry.
// The result is exact when committer times never decrease toward a commit's
// parents (rangeWalkSlop absorbs small violations); a commit emitted before
// base's paint reached it is dropped from the result if the paint arrives
// before the walk ends. The cap is applied after that filtering.
func walkShippedRange(ctx context.Context, s storer.EncodedObjectStorer, head, base *object.Commit, maxCommits int) (shipped []*object.Commit, truncated bool, err error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	paint := make(map[plumbing.Hash]uint8, 32)
	var q paintQueue
	seq := 0
	notBase := 0 // queued items pushed without paintBase
	mark := func(c *object.Commit, f uint8) {
		old := paint[c.Hash]
		if old|f == old {
			return
		}
		paint[c.Hash] = old | f
		heap.Push(&q, paintItem{commit: c, flags: old | f, seq: seq})
		seq++
		if (old|f)&paintBase == 0 {
			notBase++
		}
	}
	mark(head, paintHead)
	mark(base, paintBase)

	var emitted []*object.Commit
	var oldestEmitted time.Time
	slop, pops := rangeWalkSlop, 0
	for q.Len() > 0 && pops < rangeWalkMaxPops {
		if notBase == 0 {
			// Everything queued is base-reachable. Keep going while the queue still holds commits at
			// least as new as the oldest candidate (base's paint may yet reach it, also on timestamp
			// ties), then a few more for skew.
			caughtUp := len(emitted) == 0 || q[0].commit.Committer.When.Before(oldestEmitted)
			if caughtUp {
				if slop == 0 {
					break
				}
				slop--
			}
		} else {
			slop = rangeWalkSlop
		}
		if pops++; pops%rangeWalkCtxEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
		}
		it := heap.Pop(&q).(paintItem)
		if it.flags&paintBase == 0 {
			notBase--
		}
		cur := paint[it.commit.Hash]
		if cur == paintHead && it.flags == paintHead {
			emitted = append(emitted, it.commit)
			oldestEmitted = it.commit.Committer.When
		}
		for _, ph := range it.commit.ParentHashes {
			if paint[ph]|cur == paint[ph] {
				continue
			}
			p, err := object.GetCommit(s, ph)
			if err != nil {
				return nil, false, err
			}
			mark(p, cur)
		}
	}
	walkLimited := pops >= rangeWalkMaxPops && q.Len() > 0

	for _, c := range emitted {
		if paint[c.Hash]&paintBase == 0 {
			shipped = append(shipped, c)
		}
	}
	// The cap applies only after base's boundary is settled: cutting the walk at the cap would
	// leave base-reachable commits in the result and count them toward it.
	if len(shipped) >= maxCommits {
		return shipped[:maxCommits], true, nil
	}
	return shipped, walkLimited, nil
}
