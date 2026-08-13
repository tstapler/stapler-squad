package gogitstore

import (
	"io"
	"sync"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/idxfile"
)

// lockedIndex wraps an idxfile.Index (concretely an *idxfile.MemoryIndex)
// and serialises every call behind mu.
//
// This is NOT a defensive-programming nicety — it is required for
// correctness. idxfile.MemoryIndex's "read" methods are not read-only:
// FindOffset and FindHash lazily populate an internal offsetHash map as a
// side effect of the lookup (see idxfile.go: `idx.offsetHash[int64(offset)]
// = h`). go-git issue #1121 documents the resulting
// "concurrent map read and map write" crash when a single MemoryIndex is
// used from more than one goroutine without external locking — this is
// exactly the situation this package deliberately creates on purpose (one
// MemoryIndex shared by every worktree of a repo), so every access MUST go
// through this wrapper. Never hand out the underlying idxfile.Index value
// directly.
//
// One SharedObjectStore uses a single mutex (its own mu) for every
// lockedIndex it hands out, rather than one mutex per pack. That trades
// away some parallelism (see design doc, "Concurrency trade-offs") for
// simplicity and a smaller blast radius while this is a prototype; sharding
// the lock per-pack is a documented follow-up, not a correctness issue.
type lockedIndex struct {
	mu  *sync.Mutex
	idx idxfile.Index

	// handle is non-nil only when idx is backed by an mmap'd .idx file (see
	// mmapindex.go). nil for the copy-based (io.ReadFull) loader path. Used
	// by Entries() below to pin the mapping's generation for as long as the
	// returned iterator may still be drained — see mmapIndexHandle's doc
	// comment and pinnedEntryIter below.
	handle *mmapIndexHandle
}

var _ idxfile.Index = (*lockedIndex)(nil)

// unmappedLocked reports whether l.idx must NOT be touched anymore because
// its backing mmapIndexHandle has already been Unmap'd. Callers MUST already
// hold l.mu.
//
// Why this check exists (the bug it fixes, found by
// TestMmapIndex_PinnedReadersSurviveConcurrentRealRepack under -race): every
// lockedIndex method below re-acquires l.mu (== the owning SharedObjectStore's
// mu) before touching l.idx, which correctly serializes against
// SharedObjectStore.refreshIndexes/mmapIndexHandle.maybeUnmapLocked — but
// serialization alone is not enough. A caller can hold onto a *lockedIndex
// value obtained BEFORE a retire (e.g. SharedObjectStore.findObjectInPackfile
// snapshots s.index, releases mu, then calls methods on the snapshotted
// values; or, in a test, a long-lived reference is called repeatedly across
// many iterations). If retirement AND a full unmap have already completed by
// the time such a stale reference's method finally acquires l.mu, calling
// straight into l.idx would read the now-freed mapping immediately — even
// though this method never released the lock during a genuinely un-pinned
// gap. The pins mechanism (Entries()/pinnedEntryIter) only protects readers
// that were ALREADY pinned before an unmap; it does nothing to stop a NEW,
// stale call from starting after the unmap already happened. This check
// closes that gap: once a handle is confirmed Unmap'd, every method here
// refuses to touch l.idx at all, behaving instead as "this pack has no
// entries" (matching the "stale, not corrupt" contract design doc §5.3
// describes for a retiring-but-still-mapped pack, extended to the
// already-unmapped case).
func (l *lockedIndex) unmappedLocked() bool {
	return l.handle != nil && l.handle.unmapped
}

func (l *lockedIndex) Contains(h plumbing.Hash) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.unmappedLocked() {
		return false, nil
	}
	return l.idx.Contains(h)
}

func (l *lockedIndex) FindOffset(h plumbing.Hash) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.unmappedLocked() {
		return 0, plumbing.ErrObjectNotFound
	}
	return l.idx.FindOffset(h)
}

func (l *lockedIndex) FindCRC32(h plumbing.Hash) (uint32, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.unmappedLocked() {
		return 0, plumbing.ErrObjectNotFound
	}
	return l.idx.FindCRC32(h)
}

func (l *lockedIndex) FindHash(o int64) (plumbing.Hash, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.unmappedLocked() {
		return plumbing.ZeroHash, plumbing.ErrObjectNotFound
	}
	return l.idx.FindHash(o)
}

func (l *lockedIndex) Count() (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.unmappedLocked() {
		return 0, nil
	}
	return l.idx.Count()
}

// Entries and EntriesByOffset return an iterator; the lock is held only
// while the iterator itself is constructed (idxfile's iterator Next()
// methods read immutable post-decode fields — Names/Fanout/etc — and do not
// touch offsetHash, so draining the returned iterator without the lock held
// is safe). Only FindOffset/FindHash mutate state after decode.
//
// mmap generation safety (design doc §5.3): when idx is mmap-backed
// (l.handle != nil), Entries()'s returned idxfileEntryIter reads directly
// from idx.Names[...]/idx.Fanout[...] on every Next() call — i.e. it reads
// mapped memory AFTER this method's own lock has been released, which is
// exactly the access pattern that must not be allowed to outlive a munmap.
// Entries() therefore pins l.handle's generation (incrementing its pins
// counter, still under l.mu) and wraps the iterator so Close() releases the
// pin; SharedObjectStore.refreshIndexes/mmapIndexHandle.maybeUnmapLocked
// only actually call munmap once pins reaches zero — see mmapindex.go and
// pinnedEntryIter below. EntriesByOffset is NOT given the same treatment:
// per its own implementation (idxfile.go's EntriesByOffset), it fully
// drains a fresh Entries() iterator and copies every entry's hash into a
// value-typed (fixed [20]byte array) Entry before returning — entirely
// inside this method's own locked call — so nothing it hands back ever
// reads mapped memory again. Pinning it would be harmless but pointless
// (and would risk a spurious permanent pin if a caller never calls Close on
// its result), so it deliberately isn't.
func (l *lockedIndex) Entries() (idxfile.EntryIter, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.unmappedLocked() {
		return emptyEntryIter{}, nil
	}
	it, err := l.idx.Entries()
	if err != nil {
		return nil, err
	}
	if l.handle != nil {
		l.handle.pins++
		return &pinnedEntryIter{EntryIter: it, handle: l.handle, mu: l.mu}, nil
	}
	return it, nil
}

func (l *lockedIndex) EntriesByOffset() (idxfile.EntryIter, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.unmappedLocked() {
		return emptyEntryIter{}, nil
	}
	return l.idx.EntriesByOffset()
}

// emptyEntryIter is the idxfile.EntryIter returned for an already-unmapped
// lockedIndex — see unmappedLocked's doc comment. Next always reports
// io.EOF immediately; Close is a no-op.
type emptyEntryIter struct{}

func (emptyEntryIter) Next() (*idxfile.Entry, error) { return nil, io.EOF }
func (emptyEntryIter) Close() error                  { return nil }

// pinnedEntryIter wraps the idxfile.EntryIter returned by an mmap-backed
// lockedIndex.Entries() call, holding one pin on the backing
// mmapIndexHandle for as long as the iterator is undrained.
//
// Callers MUST call Close() when done with the iterator, even after
// receiving io.EOF from Next() — Next() does not release the pin itself,
// only Close() does. A caller that never calls Close() leaks a pin, which
// blocks this pack's mapping from ever being unmapped after a retire (a
// bounded resource leak, not a crash — see SharedObjectStore.stopPackWatch
// in mmapwatch.go for the "log and leak rather than risk a use-after-munmap"
// policy this mirrors at store-eviction time).
type pinnedEntryIter struct {
	idxfile.EntryIter
	handle *mmapIndexHandle
	mu     *sync.Mutex
	closed bool
}

// Close releases this iterator's pin exactly once, even if called more than
// once, and triggers mmapIndexHandle.maybeUnmapLocked so a retire that was
// waiting on this exact pin can complete immediately rather than waiting for
// the next unrelated pin release or refreshIndexes call.
func (p *pinnedEntryIter) Close() error {
	err := p.EntryIter.Close()
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		p.handle.pins--
		p.handle.maybeUnmapLocked()
	}
	p.mu.Unlock()
	return err
}
