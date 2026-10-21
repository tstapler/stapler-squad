package repostate

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/go-git/go-git/v5/plumbing"
)

// ErrFallback marks a walk that could not finish on the graph (full arena,
// severe pressure, a storer failure). The caller reruns the plain walk, whose
// result is by construction the same one; every other error is the walk's own
// answer (for example "merge base not found") and must be returned as is.
var ErrFallback = errors.New("repostate: walk needs the plain-walk fallback")

// Paint flags of the paint-down-to-common merge-base walk. They mirror the
// plain walk's flags bit for bit.
const (
	paintFromHead uint8 = 1 << iota
	paintFromBase
	paintStale
)

// scratchSlots bounds how many idle Scratch values a RepoState keeps.
const scratchSlots = 4

// maxPooledMarks bounds the mark array a pooled Scratch may keep (bytes).
const maxPooledMarks = 4 << 20

// markSet is a sparse byte-per-id map over the dense id space. Only the ids
// touched by a walk are cleared afterwards, so a walk on a huge graph costs
// O(visited), not O(graph).
type markSet struct {
	bits    []uint8
	touched []uint32
}

func (m *markSet) get(id uint32) uint8 {
	if int(id) >= len(m.bits) {
		return 0
	}
	return m.bits[id]
}

func (m *markSet) or(id uint32, v uint8) {
	if int(id) >= len(m.bits) {
		grown := make([]uint8, max(int(id)+1, 2*len(m.bits), 1024))
		copy(grown, m.bits)
		m.bits = grown
	}
	if m.bits[id] == 0 && v != 0 {
		m.touched = append(m.touched, id)
	}
	m.bits[id] |= v
}

func (m *markSet) count() int { return len(m.touched) }

func (m *markSet) reset() {
	for _, id := range m.touched {
		m.bits[id] = 0
	}
	m.touched = m.touched[:0]
}

type paintItem struct {
	id   uint32
	when int64
	seq  int // insertion order; breaks committer-time ties FIFO
}

func (a paintItem) before(b paintItem) bool {
	return a.when > b.when || (a.when == b.when && a.seq < b.seq)
}

// Scratch holds the reusable buffers of one walk. Take one from the RepoState
// with NewWalk; a warm walk then allocates nothing.
type Scratch struct {
	flags, seen markSet
	heap        []paintItem
	nextSeq     int
	cands       []uint32
	stack       []uint32
	parents     []uint32
	sink        []uint32
}

func (s *Scratch) push(it paintItem) {
	it.seq = s.nextSeq
	s.nextSeq++
	s.heap = append(s.heap, it)
	i := len(s.heap) - 1
	for i > 0 {
		p := (i - 1) / 2
		if !s.heap[i].before(s.heap[p]) {
			break
		}
		s.heap[p], s.heap[i] = s.heap[i], s.heap[p]
		i = p
	}
}

func (s *Scratch) pop() paintItem {
	top := s.heap[0]
	n := len(s.heap) - 1
	s.heap[0] = s.heap[n]
	s.heap = s.heap[:n]
	i := 0
	for {
		l, r, m := 2*i+1, 2*i+2, i
		if l < n && s.heap[l].before(s.heap[m]) {
			m = l
		}
		if r < n && s.heap[r].before(s.heap[m]) {
			m = r
		}
		if m == i {
			break
		}
		s.heap[m], s.heap[i] = s.heap[i], s.heap[m]
		i = m
	}
	return top
}

// scratchPool is a bounded, lock-free free list: an idle Scratch sits in one
// slot, taken by Swap and returned by CAS into an empty slot, so there is no
// ABA window and no mutex. A full pool just lets the GC have the extra one.
type scratchPool struct {
	slots [scratchSlots]atomic.Pointer[Scratch]
}

func (p *scratchPool) get() *Scratch {
	for i := range p.slots {
		if s := p.slots[i].Swap(nil); s != nil {
			return s
		}
	}
	return new(Scratch)
}

func (p *scratchPool) put(s *Scratch) {
	if len(s.flags.bits) > maxPooledMarks || len(s.seen.bits) > maxPooledMarks {
		return
	}
	for i := range p.slots {
		if p.slots[i].CompareAndSwap(nil, s) {
			return
		}
	}
}

// Walk is one operation's read-through view of a Generation: rows already
// published are read lock-free, missing ones are filled through Src. Create
// it with RepoState.NewWalk and Close it; it is not safe for concurrent use.
type Walk struct {
	ctx      context.Context
	gen      *Generation
	graph    *Graph
	pool     *scratchPool
	src      ParentsSource
	ord      StampOrd
	pressure func() bool
	limit    int
	s        *Scratch
	hits     uint64
	misses   uint64
}

// NewWalk starts a walk over the current generation under stamp ordinal ord.
// limit is the per-side visit cap (the plain walk's mergeBaseBFSLimit).
func (s *RepoState) NewWalk(ctx context.Context, src ParentsSource, ord StampOrd, limit int) Walk {
	g := s.Graph()
	return Walk{
		ctx: ctx, gen: g.Load(), graph: g, pool: &s.scratch, src: src, ord: ord,
		pressure: s.opts.PressureFunc, limit: limit, s: s.scratch.get(),
	}
}

// Close returns the scratch to the pool and publishes the walk's hit/miss counts.
func (w *Walk) Close() {
	if w.s == nil {
		return
	}
	if w.hits != 0 {
		w.graph.stats.rowHits.V.Add(w.hits)
	}
	if w.misses != 0 {
		w.graph.stats.rowMisses.V.Add(w.misses)
	}
	// A recovered panic mid-walk skips the walk's own resets; stale marks would
	// corrupt the next borrower. O(touched), so free after a clean walk.
	w.s.flags.reset()
	w.s.seen.reset()
	w.pool.put(w.s)
	w.s = nil
}

// Generation exposes the generation the walk reads, for tests and metrics.
func (w *Walk) Generation() *Generation { return w.gen }

// read returns row id's committer time and, when wantParents, appends its
// parent ids to dst. missing means the commit object is absent (a shallow
// boundary or a not-found read), which the plain walk handles through
// errors.Is(err, plumbing.ErrObjectNotFound). Any other error is a fallback
// error.
func (w *Walk) read(id uint32, dst []uint32, wantParents bool) (out []uint32, when int64, missing bool, err error) {
	switch w.gen.Row(id, w.ord) {
	case Filled:
		w.hits++
		if wantParents {
			dst, _ = w.gen.AppendParents(dst, id)
		}
		return dst, w.gen.When(id), false, nil
	case Truncated:
		w.hits++
		return dst, 0, true, nil
	}
	if w.pressure != nil && w.pressure() {
		return dst, 0, false, fallback(ErrPressure)
	}
	w.misses++
	filled, when, st, err := w.gen.fill(w.ctx, id, w.src, w.ord, dst)
	switch {
	case errors.Is(err, plumbing.ErrObjectNotFound):
		return dst, 0, true, nil
	case err != nil:
		return dst, 0, false, fallback(err)
	case st == Truncated:
		return dst, 0, true, nil
	case wantParents:
		return filled, when, false, nil
	}
	return dst, when, false, nil
}

func fallback(err error) error { return fmt.Errorf("%w: %w", ErrFallback, err) }

func (w *Walk) intern(h plumbing.Hash) (uint32, error) {
	id, err := w.gen.Intern(h)
	if err != nil {
		return 0, fallback(err)
	}
	return id, nil
}

// FindMergeBase is git's paint-down-to-common over interned ids: the exact
// port of the plain walk's findMergeBase (same heap order, flag propagation,
// 2*limit visit cap and candidate filtering).
func (w *Walk) FindMergeBase(h1, h2 plumbing.Hash) (plumbing.Hash, error) {
	if h1 == h2 {
		return h1, nil
	}
	a, err := w.intern(h1)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	b, err := w.intern(h2)
	if err != nil {
		return plumbing.ZeroHash, err
	}
	best, err := w.paintDown(a, b)
	w.s.flags.reset()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return w.gen.Hash(best), nil
}

func (w *Walk) paintDown(a, b uint32) (uint32, error) {
	s := w.s
	s.heap, s.nextSeq, s.cands = s.heap[:0], 0, s.cands[:0]
	s.flags.or(a, paintFromHead)
	s.flags.or(b, paintFromBase)
	for _, id := range [2]uint32{a, b} {
		_, when, _, err := w.read(id, s.sink[:0], false)
		if err != nil {
			return 0, err
		}
		s.push(paintItem{id: id, when: when})
	}
	visited := 0
	for len(s.heap) > 0 && s.anyFresh() && visited < 2*w.limit {
		it := s.pop()
		visited++
		f := s.flags.get(it.id)
		if f&paintStale != 0 {
			f = paintStale | paintFromHead | paintFromBase
		} else if f&(paintFromHead|paintFromBase) == paintFromHead|paintFromBase {
			s.cands = append(s.cands, it.id)
			f |= paintStale
			s.flags.or(it.id, f)
		}
		parents, _, missing, err := w.read(it.id, s.parents[:0], true)
		s.parents = parents
		if err != nil {
			return 0, err
		}
		if missing {
			continue
		}
		if err := w.paintParents(parents, f); err != nil {
			return 0, err
		}
	}
	return w.bestCandidate()
}

// paintParents propagates f to the parents and queues the ones newly painted.
func (w *Walk) paintParents(parents []uint32, f uint8) error {
	s := w.s
	for _, p := range parents {
		if cur := s.flags.get(p); cur|f == cur {
			continue
		}
		s.flags.or(p, f)
		_, when, missing, err := w.read(p, s.sink[:0], false)
		if err != nil {
			return err
		}
		if missing {
			continue
		}
		s.push(paintItem{id: p, when: when})
	}
	return nil
}

// anyFresh reports whether the queue holds a commit not yet marked stale.
func (s *Scratch) anyFresh() bool {
	for _, it := range s.heap {
		if s.flags.get(it.id)&paintStale == 0 {
			return true
		}
	}
	return false
}

// bestCandidate drops candidates painted stale by a later candidate (redundant
// common ancestors) and returns the first survivor.
func (w *Walk) bestCandidate() (uint32, error) {
	s := w.s
	for _, c := range s.cands {
		if s.flags.get(c)&paintStale != 0 {
			redundant, err := w.isAncestorOfOther(c)
			if err != nil {
				return 0, err
			}
			if redundant {
				continue
			}
		}
		return c, nil
	}
	return 0, fmt.Errorf("merge base not found within %d commits", w.limit)
}

// isAncestorOfOther reports whether c is a strict ancestor of another
// candidate. It only runs when several common ancestors exist (criss-cross
// merges). A row that cannot be read is skipped, as the plain walk skips it,
// except fallback errors, which end the walk.
func (w *Walk) isAncestorOfOther(c uint32) (bool, error) {
	s := w.s
	if len(s.cands) < 2 {
		return false, nil
	}
	// s.parents is free here: bestCandidate runs after the paint loop.
	for _, o := range s.cands {
		if o == c {
			continue
		}
		found, err := w.reaches(o, c)
		if found || err != nil {
			return found, err
		}
	}
	return false, nil
}

// reaches reports whether c is a strict ancestor of from, walking at most
// limit distinct commits depth-first like the plain walk. Commits older than c
// are pruned (they cannot have c as an ancestor). Hitting the limit before c
// is found answers false, so c stays a valid, non-redundant merge base. Must
// stay in lockstep with the plain walk's isAncestorOfOther.
func (w *Walk) reaches(from, c uint32) (bool, error) {
	s := w.s
	defer s.seen.reset()
	_, cWhen, _, err := w.read(c, s.sink[:0], false)
	if err != nil {
		return false, err
	}
	s.stack = append(s.stack[:0], from)
	for len(s.stack) > 0 {
		if s.seen.count() >= w.limit {
			return false, nil
		}
		h := s.stack[len(s.stack)-1]
		s.stack = s.stack[:len(s.stack)-1]
		if s.seen.get(h) != 0 {
			continue
		}
		s.seen.or(h, 1)
		parents, when, missing, err := w.read(h, s.parents[:0], true)
		s.parents = parents
		if err != nil {
			return false, err
		}
		if missing || when < cWhen {
			continue
		}
		for _, p := range parents {
			if p == c {
				return true, nil
			}
			s.stack = append(s.stack, p)
		}
	}
	return false, nil
}

// CountCommitsTo counts commits reachable from start but not from stop, with
// the plain walk's depth-first order and cap: at most limit commits are
// counted. A commit whose object is absent fails with plumbing.ErrObjectNotFound,
// as the plain walk's decode does.
func (w *Walk) CountCommitsTo(start, stop plumbing.Hash) (int, error) {
	from, err := w.intern(start)
	if err != nil {
		return 0, err
	}
	to, err := w.intern(stop)
	if err != nil {
		return 0, err
	}
	s := w.s
	defer s.seen.reset()
	s.stack = append(s.stack[:0], from)
	n := 0
	for len(s.stack) > 0 && n < w.limit {
		h := s.stack[len(s.stack)-1]
		s.stack = s.stack[:len(s.stack)-1]
		if s.seen.get(h) != 0 || h == to {
			continue
		}
		s.seen.or(h, 1)
		n++
		parents, _, missing, err := w.read(h, s.parents[:0], true)
		s.parents = parents
		if err != nil {
			return 0, err
		}
		if missing {
			return 0, plumbing.ErrObjectNotFound
		}
		for _, p := range parents {
			if s.seen.get(p) == 0 && p != to {
				s.stack = append(s.stack, p)
			}
		}
	}
	return n, nil
}
