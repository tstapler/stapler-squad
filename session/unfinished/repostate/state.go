package repostate

import "sync/atomic"

// Tier orders products for eviction (ADR-007): product LRUs go first, then the
// commit graph with every id-relative product, then the path interner.
type Tier int

// Eviction tiers, cheapest to rebuild first. The mmap'd commit-graph base
// segment is never a Product: it lives in the page cache.
const (
	TierProducts Tier = iota
	TierGraph
	TierInterner
	numTiers
)

// Product is one budgeted cache owned by a RepoState. *RepoCache satisfies it.
type Product interface {
	Bytes() int64
	// Drop discards all content. The graph implements it as one atomic
	// generation swap (ADR-009), so racing readers miss instead of reading stale ids.
	Drop()
}

type productSet [numTiers][]Product

// RepoState is the per-repository shared state hung on the gogitstore
// Extension slot, so its lifetime, byte accounting and eviction are the
// store's. It structurally satisfies gogitstore.Extension (Bytes, Close);
// this package never imports gogitstore or unfinished.
//
// Lock-free: the product set is an immutable copy-on-write snapshot replaced
// by CAS, so Bytes and Close, which gogitstore calls under Registry.mu, only
// read and swap atomics.
type RepoState struct {
	opts     Options
	budget   int64
	products atomic.Pointer[productSet]
	closed   atomic.Bool
}

// NewRepoState returns an empty state. budgetBytes is the size above which a
// non-severe Prune starts dropping tiers.
func NewRepoState(opts Options, budgetBytes int64) *RepoState {
	s := &RepoState{opts: opts, budget: budgetBytes}
	s.products.Store(&productSet{})
	return s
}

// Register adds p to tier. It reports false, and does nothing, after Close.
func (s *RepoState) Register(tier Tier, p Product) bool {
	if tier < 0 || tier >= numTiers {
		return false
	}
	for {
		if s.closed.Load() {
			return false
		}
		old := s.products.Load()
		next := *old
		next[tier] = append(append([]Product(nil), old[tier]...), p)
		if s.products.CompareAndSwap(old, &next) {
			return true
		}
	}
}

// UnderPressure reports severe memory pressure; callers skip graph fill and
// use the legacy walk while it is true.
func (s *RepoState) UnderPressure() bool { return s.opts.underPressure() }

// Bytes sums the products without walking their contents.
func (s *RepoState) Bytes() int64 {
	set := s.products.Load()
	var total int64
	for _, tier := range set {
		for _, p := range tier {
			total += p.Bytes()
		}
	}
	return total
}

// Prune frees memory in ADR-007 order. A non-severe prune drops one tier at a
// time until Bytes is within budget; a severe one drops everything.
func (s *RepoState) Prune(severe bool) {
	if severe {
		s.DropAll()
		return
	}
	set := s.products.Load()
	for _, tier := range set {
		if s.Bytes() <= s.budget {
			return
		}
		for _, p := range tier {
			p.Drop()
		}
	}
}

// DropAll empties every product (ClearCache). The state stays usable.
func (s *RepoState) DropAll() {
	set := s.products.Load()
	for _, tier := range set {
		for _, p := range tier {
			p.Drop()
		}
	}
}

// Close detaches every product so the GC reclaims them and refuses later
// registration. gogitstore calls it once, under Registry.mu, so it must not
// take locks; it deliberately does not Drop.
func (s *RepoState) Close() {
	s.closed.Store(true)
	s.products.Store(&productSet{})
}
