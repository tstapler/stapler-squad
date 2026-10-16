package repostate

import (
	"context"
	"math/bits"
	"sync/atomic"

	"github.com/go-git/go-git/v5/plumbing"
)

const (
	wordShift = 6  // log2 of bits per word
	wordMask  = 63 // bit index within a word
)

// reachHeaderBytes is the fixed per-set overhead counted by Bytes.
const reachHeaderBytes = 96

// defaultReachBudgetBytes bounds one repository's cached reach sets; a set for
// 50,000 commits is about 6 KB (Gate G4), so this holds roughly 160 of them.
const defaultReachBudgetBytes = 1 << 20

// ReachKey is the complete identity of a reach set. ids are only meaningful
// within one graph epoch, and a shallow repository deepened in place changes
// what is reachable, so both are part of the key.
type ReachKey struct {
	Repo    RepoKey
	Base    plumbing.Hash
	Cap     int
	Shallow ShallowStamp
	Epoch   GraphEpoch
}

// compatible reports whether a set under k can seed an incremental build for
// a different base under want.
func (k ReachKey) compatible(want ReachKey) bool {
	return k.Repo == want.Repo && k.Cap == want.Cap && k.Shallow == want.Shallow && k.Epoch == want.Epoch
}

// ReachBitset is the set of commits reachable from one base, as one bit per
// CommitID of its generation. It is immutable once published: builders own
// their words exclusively until then (plain, non-atomic writes), and readers
// only ever see the finished slice.
type ReachBitset struct {
	key       ReachKey
	gen       *Generation
	words     []uint64
	count     int
	walked    int
	truncated bool
}

// Contains reports whether h is reachable from the set's base.
func (b *ReachBitset) Contains(h plumbing.Hash) bool {
	id, ok := b.gen.Lookup(h)
	if !ok {
		return false
	}
	return b.has(id)
}

func (b *ReachBitset) has(id uint32) bool {
	w := int(id >> wordShift)
	return w < len(b.words) && b.words[w]&(1<<(id&wordMask)) != 0
}

// Len is the number of commits in the set.
func (b *ReachBitset) Len() int { return b.count }

// Truncated reports that the walk stopped at the cap (the legacy truncated set).
func (b *ReachBitset) Truncated() bool { return b.truncated }

// Walked is the number of commits newly visited by the build that produced this
// set: the whole set for a scratch build, only the delta for an incremental one.
func (b *ReachBitset) Walked() int { return b.walked }

// Key returns the identity the set was built under.
func (b *ReachBitset) Key() ReachKey { return b.key }

// Bytes is the retained size: the words plus a fixed header.
func (b *ReachBitset) Bytes() int64 { return int64(len(b.words))*8 + reachHeaderBytes }

// ReachStats are cumulative build counters of one repository's reach sets.
type ReachStats struct {
	Scratch     uint64 // sets built by a full walk
	Incremental uint64 // sets built by extending an earlier complete set
}

// ReachCache holds a repository's reach sets behind a singleflighted,
// byte-budgeted cache, and remembers the latest complete set as the seed for
// incremental builds. It is a TierGraph product: its ids die with the graph.
type ReachCache struct {
	cache       *RepoCache[ReachKey, *ReachBitset]
	latest      atomic.Pointer[ReachBitset]
	scratch     atomic.Uint64
	incremental atomic.Uint64
}

var _ Product = (*ReachCache)(nil)

func newReachCache(budget int64, opts Options) *ReachCache {
	if budget <= 0 {
		budget = defaultReachBudgetBytes
	}
	return &ReachCache{cache: NewRepoCache[ReachKey, *ReachBitset](budget, 0, func(b *ReachBitset) int { return int(b.Bytes()) }, opts)}
}

// Bytes and Drop make ReachCache a Product.
func (c *ReachCache) Bytes() int64 { return c.cache.Bytes() }

// Drop discards every set and the incremental seed.
func (c *ReachCache) Drop() {
	c.latest.Store(nil)
	c.cache.Drop()
}

// Stats returns the cache counters (hits, misses, evictions, coalesced).
func (c *ReachCache) Stats() CacheStats { return c.cache.Stats() }

// BuildStats returns the scratch/incremental build counts.
func (c *ReachCache) BuildStats() ReachStats {
	return ReachStats{Scratch: c.scratch.Load(), Incremental: c.incremental.Load()}
}

// reachCache returns the repository's reach cache, creating and registering it
// in TierGraph on first use (reach sets hold ids, so they go with the graph).
func (s *RepoState) reachCache() *ReachCache {
	if c := s.reach.Load(); c != nil {
		return c
	}
	fresh := newReachCache(s.opts.ReachBudgetBytes, s.opts)
	if !s.reach.CompareAndSwap(nil, fresh) {
		return s.reach.Load()
	}
	s.Register(TierGraph, fresh)
	return fresh
}

// ReachCacheIfBuilt returns the reach cache or nil when none was created yet;
// unlike ReachCache it never allocates, so metrics reads leave no trace.
func (s *RepoState) ReachCacheIfBuilt() *ReachCache { return s.reach.Load() }

// ReachCache exposes the repository's reach cache for metrics and tests.
func (s *RepoState) ReachCache() *ReachCache { return s.reachCache() }

// Reach returns the set of commits reachable from want.Base, built at most once per
// ReachKey across all callers and extended incrementally when base is a
// descendant of the previous complete set. The caller fills want.Repo, Base and
// Cap (the legacy maxReachableSetCommits); Shallow and Epoch are set here. The traversal order matches go-git's default
// first-parent-first pre-order Log, so a truncated set equals the legacy one.
// Any walk failure is an ErrFallback error and the caller runs the legacy walk.
func (s *RepoState) Reach(ctx context.Context, src ParentsSource, ord StampOrd, want ReachKey) (*ReachBitset, error) {
	rc := s.reachCache()
	key := want
	key.Shallow, key.Epoch = s.ShallowStamp(), s.Graph().Load().Epoch()
	if b, ok := rc.cache.Get(key); ok {
		return b, nil
	}
	return s.reachMiss(ctx, rc, src, ord, key)
}

// reachMiss builds on a miss. It is separate so the hit path allocates nothing
// (the walk and the compute closure live only here).
func (s *RepoState) reachMiss(ctx context.Context, rc *ReachCache, src ParentsSource, ord StampOrd, key ReachKey) (*ReachBitset, error) {
	w := s.NewWalk(ctx, src, ord, key.Cap)
	defer w.Close()
	if e := w.gen.Epoch(); e != key.Epoch { // the graph was dropped since the key was made
		key.Epoch = e
		if b, ok := rc.cache.Get(key); ok {
			return b, nil
		}
	}
	return rc.cache.computeMiss(key, func() (*ReachBitset, error) { return rc.build(&w, key) })
}

func (c *ReachCache) build(w *Walk, key ReachKey) (*ReachBitset, error) {
	baseID, err := w.intern(key.Base)
	if err != nil {
		return nil, err
	}
	if seed := c.latest.Load(); seed != nil && seed.usableFor(key, baseID) {
		b, ok, err := reachIncremental(w, key, seed, baseID)
		if err != nil {
			return nil, err
		}
		if ok {
			c.incremental.Add(1)
			c.publish(b)
			return b, nil
		}
	}
	b, err := reachScratch(w, key, baseID)
	if err != nil {
		return nil, err
	}
	c.scratch.Add(1)
	c.publish(b)
	return b, nil
}

func (c *ReachCache) publish(b *ReachBitset) {
	if !b.truncated {
		c.latest.Store(b)
	}
}

// usableFor reports whether b may seed a build for key: same repository, cap,
// shallow stamp and epoch, never truncated (a truncated set is not the closure
// of its base), a different base, and a base that is not already inside b (a
// base that moved backwards is not an extension).
func (b *ReachBitset) usableFor(key ReachKey, baseID uint32) bool {
	return !b.truncated && b.key.Base != key.Base && b.key.compatible(key) && !b.has(baseID)
}

// reachBuilder accumulates one set's words.
type reachBuilder struct {
	words  []uint64
	maxID  uint32
	count  int
	limit  int    // commit cap
	anchor uint32 // incremental builds: the seed's base, which must be met
}

func (r *reachBuilder) has(id uint32) bool {
	w := int(id >> wordShift)
	return w < len(r.words) && r.words[w]&(1<<(id&wordMask)) != 0
}

func (r *reachBuilder) set(id uint32) {
	w := int(id >> wordShift)
	if w >= len(r.words) {
		grown := make([]uint64, max(w+1, 2*len(r.words)))
		copy(grown, r.words)
		r.words = grown
	}
	r.words[w] |= 1 << (id & wordMask)
	r.maxID = max(r.maxID, id)
	r.count++
}

// finish trims the words to the highest set bit, copying when growth slack
// would otherwise be retained.
func (r *reachBuilder) finish() []uint64 {
	n := int(r.maxID>>wordShift) + 1
	if n > len(r.words) {
		n = len(r.words)
	}
	if cap(r.words) > n+n/8+1 {
		out := make([]uint64, n)
		copy(out, r.words)
		return out
	}
	return r.words[:n:n]
}

// reachScratch is the full walk: go-git's default Log order (pre-order DFS,
// first parent first, a commit visited once) bounded at key.Cap commits.
func reachScratch(w *Walk, key ReachKey, baseID uint32) (*ReachBitset, error) {
	r := reachBuilder{words: make([]uint64, (w.gen.Len()+wordMask)>>wordShift), limit: key.Cap}
	truncated, err := w.reachDFS(&r, baseID)
	if err != nil {
		return nil, err
	}
	return &ReachBitset{key: key, gen: w.gen, words: r.finish(), count: r.count, walked: r.count, truncated: truncated}, nil
}

// reachIncremental extends seed to key.Base by walking only the commits seed
// lacks. The seed's words are copied first (a published set is never written),
// which makes "already in seed" the visited test. It succeeds only when seed's
// base turns out to be an ancestor of the new base and the union fits the cap;
// otherwise the caller rebuilds from scratch.
func reachIncremental(w *Walk, key ReachKey, seed *ReachBitset, baseID uint32) (*ReachBitset, bool, error) {
	words := make([]uint64, max(len(seed.words), (w.gen.Len()+wordMask)>>wordShift))
	copy(words, seed.words)
	seedBase, ok := w.gen.Lookup(seed.key.Base)
	if !ok {
		return nil, false, nil
	}
	r := reachBuilder{words: words, count: seed.count, maxID: highestSetID(seed.words), limit: key.Cap, anchor: seedBase}
	reached, err := w.reachExtend(&r, baseID)
	if err != nil || !reached {
		return nil, false, err
	}
	return &ReachBitset{key: key, gen: w.gen, words: r.finish(), count: r.count, walked: r.count - seed.count}, true, nil
}

func highestSetID(words []uint64) uint32 {
	for i := len(words) - 1; i >= 0; i-- {
		if words[i] != 0 {
			return uint32(i<<wordShift + wordMask - bits.LeadingZeros64(words[i]))
		}
	}
	return 0
}

// reachDFS visits from start in pre-order, stopping before the (r.limit+1)-th
// commit. Reaching the limit with commits left is reported as truncated after
// reading the next commit once, as go-git's iterator does (an unreadable
// commit there is an error, hence a fallback to the legacy walk).
func (w *Walk) reachDFS(r *reachBuilder, start uint32) (truncated bool, err error) {
	s := w.s
	s.stack = append(s.stack[:0], start)
	for len(s.stack) > 0 {
		id := s.stack[len(s.stack)-1]
		s.stack = s.stack[:len(s.stack)-1]
		if r.has(id) {
			continue
		}
		parents, missing, err := w.reachRead(id)
		if err != nil || missing {
			return false, w.reachFail(err)
		}
		if r.count >= r.limit {
			return true, nil
		}
		r.set(id)
		for i := len(parents) - 1; i >= 0; i-- {
			if !r.has(parents[i]) {
				s.stack = append(s.stack, parents[i])
			}
		}
	}
	return false, nil
}

// reachExtend walks the commits missing from r starting at start and reports
// whether r.anchor (the seed's base) was met among their parents. It gives up,
// reporting false, when the union would exceed r.limit.
func (w *Walk) reachExtend(r *reachBuilder, start uint32) (reached bool, err error) {
	s := w.s
	s.stack = append(s.stack[:0], start)
	for len(s.stack) > 0 {
		id := s.stack[len(s.stack)-1]
		s.stack = s.stack[:len(s.stack)-1]
		if r.has(id) {
			continue
		}
		if r.count >= r.limit {
			return false, nil
		}
		parents, missing, err := w.reachRead(id)
		if err != nil || missing {
			return false, w.reachFail(err)
		}
		r.set(id)
		for _, p := range parents {
			if p == r.anchor {
				reached = true
			}
			if !r.has(p) {
				s.stack = append(s.stack, p)
			}
		}
	}
	return reached, nil
}

// reachRead reads id's parents into the walk's scratch buffer.
func (w *Walk) reachRead(id uint32) (parents []uint32, missing bool, err error) {
	s := w.s
	parents, _, missing, err = w.read(id, s.parents[:0], true)
	s.parents = parents
	return parents, missing, err
}

// reachFail turns a missing commit into a fallback: the legacy walk returns the
// matching not-found error itself, so the answer stays identical.
func (w *Walk) reachFail(err error) error {
	if err == nil {
		return fallback(plumbing.ErrObjectNotFound)
	}
	return err
}
