package repostate

import (
	"errors"
	"sync/atomic"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/puzpuzpuz/xsync/v4"
)

// ErrGraphFull means the arena reached Options.MaxGraphCommits (or its side
// arena is exhausted); callers fall back to the plain walk for that call.
var ErrGraphFull = errors.New("repostate: commit graph full")

// ErrPressure means severe memory pressure forbade filling a new row; callers
// run the plain walk for that call. Rows already published are still served.
var ErrPressure = errors.New("repostate: severe memory pressure")

// GraphEpoch names one assignment of ids. Ids are stable and never reused
// within an epoch; a Drop starts a new one.
type GraphEpoch uint64

// graphHooks are test seams carried by the graph, not package variables.
type graphHooks struct {
	// Yield is called before every CAS and publishing store.
	Yield func()
	// PayloadWritten is called by the claim holder after it writes a row's payload.
	PayloadWritten func(id uint32)
	// ClaimWait is the loser's bounded wait for the holder (see Options.ClaimWait).
	ClaimWait time.Duration
}

// bindingIndexBytesPerRow estimates the pointer-bearing hash->id index
// (xsync.Map entry, key and bucket share) so Bytes() is not an undercount.
// Calibrated against HeapAlloc in TestGraphBytes_ShouldMatchHeapDelta.
const sideWordsPerRow = 4

const bindingIndexBytesPerRow = 48

type graphStats struct {
	fillLostCAS          padded[atomic.Uint64]
	fillLostDecode       padded[atomic.Uint64]
	reprobe              padded[atomic.Uint64]
	rollback             padded[atomic.Uint64]
	casRetries           padded[atomic.Uint64]
	graphFull            padded[atomic.Uint64]
	redundancyBound      padded[atomic.Uint64] // reaches() hit its visit bound before finding c
	redundancyPrunedMiss padded[atomic.Uint64] // reaches() found nothing after pruning older commits
	fullRebuild          padded[atomic.Uint64] // generations replaced because the arena stayed full
	rowHits              padded[atomic.Uint64] // walk reads served from a Filled/Truncated row
	rowMisses            padded[atomic.Uint64] // walk reads that had to Fill a row
}

// GraphStats is a point-in-time copy of the contention counters, cumulative
// across generations (the plan's *_total metrics).
type GraphStats struct {
	FillLostCAS, FillLostDecode, Reprobe, Rollback, CASRetries, GraphFull, FullRebuild uint64
	// RedundancyBound and RedundancyPrunedMiss count merge-base redundancy checks
	// (Walk.reaches) that answered "not redundant" by bound or by pruned miss.
	RedundancyBound, RedundancyPrunedMiss uint64
}

// Generation is one immutable-shape assignment of ids: a fixed segment
// directory of pointer-free rows plus the hash->id index. Every operation
// loads a Generation once and uses it throughout, so ids from two
// assignments are never mixed. A replaced Generation is never mutated again;
// the GC reclaims it once the last reader lets go (ADR-009 item 8).
type Generation struct {
	epoch  GraphEpoch
	born   int64 // creation time (unix ns); gates the rate-limited full-arena rebuild
	onFull func(*Generation)
	cap    uint64
	hooks  graphHooks
	stats  *graphStats

	dir     []atomic.Pointer[segment]
	side    []atomic.Pointer[sideChunk]
	sideCap uint64
	index   *xsync.Map[plumbing.Hash, uint32]

	segs       atomic.Int64
	sideChunks atomic.Int64

	// waiters are striped wake channels: a claim loser registers one per
	// stripe, the holder closes it after publishing (Generation.awaitPublish).
	waiters [waiterStripes]atomic.Pointer[chan struct{}]

	next     padded[atomic.Uint64]
	sideNext padded[atomic.Uint64]
	stampOrd padded[atomic.Uint32] // holds a StampOrd
}

func newGeneration(epoch GraphEpoch, maxRows uint64, hooks graphHooks, stats *graphStats, bornNs int64) *Generation {
	// Octopus merges are rare; four words per row is generous and only touched chunks cost memory.
	sideCap := maxRows * sideWordsPerRow
	if sideCap < sideChunkWords {
		sideCap = sideChunkWords
	}
	return &Generation{
		epoch:   epoch,
		born:    bornNs,
		cap:     maxRows,
		hooks:   hooks,
		stats:   stats,
		dir:     make([]atomic.Pointer[segment], (maxRows+segRows-1)>>segShift),
		side:    make([]atomic.Pointer[sideChunk], (sideCap+sideChunkWords-1)>>sideShift),
		sideCap: sideCap,
		index:   xsync.NewMap[plumbing.Hash, uint32](),
	}
}

func (g *Generation) yield() {
	if g.hooks.Yield != nil {
		g.hooks.Yield()
	}
}

// Epoch returns the id-assignment epoch of this generation.
func (g *Generation) Epoch() GraphEpoch { return g.epoch }

// Len counts allocated rows (interned hashes), not reachable commits.
func (g *Generation) Len() int { return int(g.next.V.Load()) }

// Cap is the fixed row capacity.
func (g *Generation) Cap() int { return int(g.cap) }

// Bytes estimates the heap held by this generation without walking rows. An
// empty generation reports zero: its fixed pointer directories (about 10 KB at
// the default capacity) are not worth a nonzero floor on every idle RepoState.
func (g *Generation) Bytes() int64 {
	if g.Len() == 0 {
		return 0
	}
	dir := int64(len(g.dir)+len(g.side)) * 8
	return g.segs.Load()*segmentBytes + g.sideChunks.Load()*sideChunkBytes + dir +
		int64(g.Len())*bindingIndexBytesPerRow
}

// StampOrd returns the current shallow-stamp ordinal.
func (g *Generation) StampOrd() StampOrd { return StampOrd(g.stampOrd.V.Load()) }

// BumpStamp installs the next ordinal in one atomic step and returns it. Phase
// 1 of a call invokes it when the shallow stamp changed, before any object
// read. No row is written: stale Truncated rows read as Pending lazily. The
// ordinal wraps at 2^30 bumps, which is unreachable in a process lifetime.
func (g *Generation) BumpStamp() StampOrd {
	g.yield()
	return StampOrd(g.stampOrd.V.Add(1)) & stampMask
}

func (g *Generation) segmentFor(idx uint64) *segment {
	p := &g.dir[idx]
	if s := p.Load(); s != nil {
		return s
	}
	g.yield()
	fresh := new(segment)
	if p.CompareAndSwap(nil, fresh) {
		g.segs.Add(1)
		return fresh
	}
	return p.Load()
}

// Lookup returns the id of an interned hash. It never allocates a row.
func (g *Generation) Lookup(h plumbing.Hash) (uint32, bool) { return g.index.Load(h) }

// Intern returns the id for h, allocating a Pending row on first sight. The
// compute function does only the bounded id allocation, the hash store and the
// Pending store; it never blocks or calls a storer (ADR-009 item 4).
func (g *Generation) Intern(h plumbing.Hash) (uint32, error) {
	if id, ok := g.index.Load(h); ok {
		return id, nil
	}
	full := false
	id, _ := g.index.LoadOrCompute(h, func() (uint32, bool) {
		n, ok := g.allocRow()
		if !ok {
			full = true
			return 0, true
		}
		seg := g.segmentFor(n >> segShift)
		seg.hash[n&segMask] = h
		seg.state[n&segMask].Store(packState(Pending, 0))
		return uint32(n), false
	})
	if full {
		g.noteFull()
		return 0, ErrGraphFull
	}
	return id, nil
}

// noteFull counts a full-arena refusal and lets the owning Graph replace this
// generation if it is old enough.
func (g *Generation) noteFull() {
	g.stats.graphFull.V.Add(1)
	if g.onFull != nil {
		g.onFull(g)
	}
}

// allocRow is a bounded CAS increment: the cap is checked first, so next never
// exceeds it and cannot wrap.
func (g *Generation) allocRow() (uint64, bool) {
	for {
		n := g.next.V.Load()
		if n >= g.cap {
			return 0, false
		}
		g.yield()
		if g.next.V.CompareAndSwap(n, n+1) {
			return n, true
		}
		g.stats.casRetries.V.Add(1)
	}
}

// Hash returns the hash of an id obtained from Intern, Lookup or a Parents
// call; arbitrary ids below Len may not have their hash published yet.
func (g *Generation) Hash(id uint32) plumbing.Hash {
	return g.dir[id>>segShift].Load().hash[id&segMask]
}

func (g *Generation) stateWord(id uint32) *atomic.Uint32 {
	return &g.dir[id>>segShift].Load().state[id&segMask]
}

// Row returns the effective state of id for a caller holding stamp ordinal
// ord: a Truncated row stamped under a different ordinal reads as Pending.
func (g *Generation) Row(id uint32, ord StampOrd) RowState {
	st, o := unpackState(g.stateWord(id).Load())
	if st == Truncated && o != ord {
		return Pending
	}
	return st
}

// AppendParents appends the parent ids of id to dst when the row is Filled.
// For any other state it appends nothing; the payload is read only after an
// atomic load observed Filled.
func (g *Generation) AppendParents(dst []uint32, id uint32) ([]uint32, RowState) {
	seg := g.dir[id>>segShift].Load()
	st, _ := unpackState(seg.state[id&segMask].Load())
	if st != Filled {
		return dst, st
	}
	par := seg.parents[id&segMask]
	if par[0] == noParent {
		return dst, Filled
	}
	dst = append(dst, par[0])
	switch {
	case par[1] == noParent:
	case par[1]&sideFlag == 0:
		dst = append(dst, par[1])
	default:
		start := uint64(par[1] &^ sideFlag)
		chunk := g.side[start>>sideShift].Load()
		off := start & sideMask
		n := uint64(chunk[off])
		for i := uint64(1); i <= n; i++ {
			dst = append(dst, chunk[off+i])
		}
	}
	return dst, Filled
}

// When returns the committer time of a row observed Filled; it is meaningless
// for any other state. Written with the payload, before the publishing store.
func (g *Generation) When(id uint32) int64 {
	return g.dir[id>>segShift].Load().when[id&segMask]
}

// writePayload stores ids and the committer time as the row's payload. Only
// the claim holder calls it, before the publishing store.
func (g *Generation) writePayload(id uint32, ids []uint32, when int64) error {
	var word [2]uint32
	switch {
	case len(ids) == 0:
		word = [2]uint32{noParent, noParent}
	case len(ids) == 1:
		word = [2]uint32{ids[0], noParent}
	case len(ids) == 2:
		word = [2]uint32{ids[0], ids[1]}
	default:
		start, err := g.allocSide(uint64(len(ids)))
		if err != nil {
			return err
		}
		chunk := g.sideChunkFor(start >> sideShift)
		off := start & sideMask
		chunk[off] = uint32(len(ids) - 1)
		copy(chunk[off+1:], ids[1:])
		word = [2]uint32{ids[0], sideFlag | uint32(start)}
	}
	seg := g.segmentFor(uint64(id) >> segShift)
	seg.parents[id&segMask] = word
	seg.when[id&segMask] = when
	if g.hooks.PayloadWritten != nil {
		g.hooks.PayloadWritten(id)
	}
	return nil
}

// allocSide reserves 1+(n-1)=n words that do not straddle a chunk boundary.
func (g *Generation) allocSide(n uint64) (uint64, error) {
	if n > sideChunkWords {
		return 0, ErrGraphFull
	}
	for {
		cur := g.sideNext.V.Load()
		start := cur
		if start&sideMask+n > sideChunkWords {
			start = (start | sideMask) + 1
		}
		if start+n > g.sideCap || start+n >= uint64(sideFlag) {
			return 0, ErrGraphFull
		}
		g.yield()
		if g.sideNext.V.CompareAndSwap(cur, start+n) {
			return start, nil
		}
		g.stats.casRetries.V.Add(1)
	}
}

func (g *Generation) sideChunkFor(idx uint64) *sideChunk {
	p := &g.side[idx]
	if c := p.Load(); c != nil {
		return c
	}
	g.yield()
	fresh := new(sideChunk)
	if p.CompareAndSwap(nil, fresh) {
		g.sideChunks.Add(1)
		return fresh
	}
	return p.Load()
}

// Graph holds the current Generation behind one atomic pointer. It satisfies
// Product so RepoState registers it in TierGraph; Drop is the ADR-009 item 5
// atomic generation swap.
type Graph struct {
	maxRows uint64
	hooks   graphHooks
	stats   *graphStats
	gen     atomic.Pointer[Generation]
	now     func() time.Time
	// rebuildEvery is the minimum age of a generation before a full arena may
	// replace it; negative disables the rebuild.
	rebuildEvery time.Duration
}

var _ Product = (*Graph)(nil)

// NewGraph returns an empty graph bounded by opts.MaxGraphCommits (a default
// applies when it is zero or above 2^30).
func NewGraph(opts Options) *Graph { return newGraph(opts, graphHooks{}) }

func newGraph(opts Options, hooks graphHooks) *Graph {
	n := uint64(opts.MaxGraphCommits)
	if opts.MaxGraphCommits <= 0 {
		n = defaultMaxRows
	}
	if n > maxRows {
		n = maxRows
	}
	hooks.ClaimWait = opts.ClaimWait
	if hooks.ClaimWait <= 0 {
		hooks.ClaimWait = defaultClaimWait
	}
	g := &Graph{maxRows: n, hooks: hooks, stats: &graphStats{}, now: opts.now, rebuildEvery: opts.GraphRebuildInterval}
	if g.rebuildEvery == 0 {
		g.rebuildEvery = defaultGraphRebuildInterval
	}
	g.gen.Store(g.newGen(1))
	return g
}

// newGen builds a generation wired to this graph's clock and full-arena hook.
func (g *Graph) newGen(epoch GraphEpoch) *Generation {
	gen := newGeneration(epoch, g.maxRows, g.hooks, g.stats, g.now().UnixNano())
	gen.onFull = g.rebuildIfDue
	return gen
}

// successor is the generation that replaces old. Ordinals stay monotonic across
// generations so a Truncated row written under a stale ordinal can never match
// a later one.
func (g *Graph) successor(old *Generation) *Generation {
	next := g.newGen(old.epoch + 1)
	next.stampOrd.V.Store(old.stampOrd.V.Load())
	return next
}

// rebuildIfDue replaces a full generation that is at least rebuildEvery old, so
// the arena does not stay full (every walk falling back) until something else
// calls Drop. The CAS from the exact full generation makes it once-only: a
// racing Drop or rebuild wins and this one does nothing. In-flight walks keep
// the generation they loaded.
func (g *Graph) rebuildIfDue(full *Generation) {
	if g.rebuildEvery < 0 || g.now().UnixNano()-full.born < int64(g.rebuildEvery) {
		return
	}
	if g.gen.CompareAndSwap(full, g.successor(full)) {
		g.stats.fullRebuild.V.Add(1)
	}
}

// Load returns the current generation; hold it for the whole operation.
func (g *Graph) Load() *Generation { return g.gen.Load() }

// Bytes reports the current generation's estimated heap.
func (g *Graph) Bytes() int64 { return g.Load().Bytes() }

// Drop installs a fresh generation with epoch+1 by CAS. Operations holding the
// old generation finish against it consistently; the old one is never mutated
// after the swap.
func (g *Graph) Drop() {
	for {
		old := g.gen.Load()
		if g.gen.CompareAndSwap(old, g.successor(old)) {
			return
		}
	}
}

// CacheStats reports the arena as a cache: a hit is a walk read served from a
// published row, a miss one that had to fill it (the engagement signal the
// shared-path tests assert on). Entries is the allocated row count.
func (g *Graph) CacheStats() CacheStats {
	gen := g.Load()
	return CacheStats{
		Hits:    int64(g.stats.rowHits.V.Load()),
		Misses:  int64(g.stats.rowMisses.V.Load()),
		Entries: gen.Len(),
		Bytes:   gen.Bytes(),
	}
}

// Stats copies the cumulative contention counters.
func (g *Graph) Stats() GraphStats {
	s := g.stats
	return GraphStats{
		FillLostCAS:          s.fillLostCAS.V.Load(),
		FillLostDecode:       s.fillLostDecode.V.Load(),
		Reprobe:              s.reprobe.V.Load(),
		Rollback:             s.rollback.V.Load(),
		CASRetries:           s.casRetries.V.Load(),
		GraphFull:            s.graphFull.V.Load(),
		FullRebuild:          s.fullRebuild.V.Load(),
		RedundancyBound:      s.redundancyBound.V.Load(),
		RedundancyPrunedMiss: s.redundancyPrunedMiss.V.Load(),
	}
}
