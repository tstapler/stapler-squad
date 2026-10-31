package git

import (
	"container/heap"
	"context"
	"errors"
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

// rangeWalkMaxPops bounds the walk of a pathological range (e.g. a base unrelated to head). A
// variable so a test can lower it.
var rangeWalkMaxPops = 20_000 //nolint:gochecknoglobals

// ErrRangeWalkLimit means the walk hit rangeWalkMaxPops before finding maxCommits commits, so the
// partial list can't be trusted (a long base-only stretch may hide head's commits).
var ErrRangeWalkLimit = errors.New("range walk exceeded its commit budget")

const (
	// rangeWalkSlop is how many more commits to pop after every queued commit is
	// base-reachable. A parent can carry an older timestamp than its child
	// (clock skew), so base's own paint might still be on its way to a commit
	// we've already seen; git's rev-list uses the same slack.
	rangeWalkSlop = 5
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
	w := &rangeWalker{store: s, paint: make(map[plumbing.Hash]uint8, 32), slop: rangeWalkSlop}
	w.mark(head, paintHead)
	w.mark(base, paintBase)

	pops := 0
	for w.q.Len() > 0 && pops < rangeWalkMaxPops {
		if w.settled() {
			break
		}
		if pops++; pops%rangeWalkCtxEvery == 0 {
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
		}
		if err := w.step(); err != nil {
			return nil, false, err
		}
	}
	walkLimited := pops >= rangeWalkMaxPops && w.q.Len() > 0

	for _, c := range w.emitted {
		if w.paint[c.Hash]&paintBase == 0 {
			shipped = append(shipped, c)
		}
	}
	// The cap applies only after base's boundary is settled: cutting the walk at the cap would
	// leave base-reachable commits in the result and count them toward it. (If the pop budget
	// ended the walk early, the newest-first order makes the capped head of the list reliable
	// in practice, but the boundary is not proven settled.)
	if len(shipped) >= maxCommits {
		return shipped[:maxCommits], true, nil
	}
	if walkLimited {
		return nil, false, ErrRangeWalkLimit
	}
	return shipped, false, nil
}

// rangeWalker holds the mutable state of one walkShippedRange call.
type rangeWalker struct {
	store         storer.EncodedObjectStorer
	paint         map[plumbing.Hash]uint8
	q             paintQueue
	seq           int
	notBase       int // queued items pushed without paintBase
	emitted       []*object.Commit
	oldestEmitted time.Time
	slop          int
}

func (w *rangeWalker) mark(c *object.Commit, f uint8) {
	old := w.paint[c.Hash]
	if old|f == old {
		return
	}
	w.paint[c.Hash] = old | f
	heap.Push(&w.q, paintItem{commit: c, flags: old | f, seq: w.seq})
	w.seq++
	if (old|f)&paintBase == 0 {
		w.notBase++
	}
}

// settled reports whether the walk can stop, and spends one unit of slop per call while the
// queue is entirely base-reachable. Everything queued being base-reachable is not enough on its
// own: keep going while the queue still holds commits at least as new as the oldest candidate
// (base's paint may yet reach it, also on timestamp ties), then a few more for skew.
func (w *rangeWalker) settled() bool {
	if w.notBase != 0 {
		w.slop = rangeWalkSlop
		return false
	}
	caughtUp := len(w.emitted) == 0 || w.q[0].commit.Committer.When.Before(w.oldestEmitted)
	if !caughtUp {
		return false
	}
	if w.slop == 0 {
		return true
	}
	w.slop--
	return false
}

// step pops the newest queued commit, emits it if only head reaches it, and paints its parents.
func (w *rangeWalker) step() error {
	it := heap.Pop(&w.q).(paintItem)
	if it.flags&paintBase == 0 {
		w.notBase--
	}
	cur := w.paint[it.commit.Hash]
	if cur == paintHead && it.flags == paintHead {
		w.emitted = append(w.emitted, it.commit)
		w.oldestEmitted = it.commit.Committer.When
	}
	for _, ph := range it.commit.ParentHashes {
		if w.paint[ph]|cur == w.paint[ph] {
			continue
		}
		p, err := object.GetCommit(w.store, ph)
		if err != nil {
			return err
		}
		w.mark(p, cur)
	}
	return nil
}
