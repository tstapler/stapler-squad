// Package gogitstore is a prototype of a pluggable go-git storage.Storer
// that shares the two expensive, commondir-scoped pieces of repository
// state — the parsed packfile index and the decoded-object cache — across
// every worktree of one repository, instead of paying for them once per
// opened worktree the way git.PlainOpenWithOptions does.
//
// See session/unfinished/design/pluggable-gitstore.md for the full design
// rationale, the concurrency-safety analysis this package is built around,
// and the staged rollout plan. This package is a read-only prototype: it
// implements enough of storer.EncodedObjectStorer to satisfy git.Open() and
// to serve the three read operations session/unfinished's Scanner actually
// needs (HasUncommitted, DiffShortstat, AheadBehind — see
// session/unfinished/vcsreader.go). Write-path methods
// (SetEncodedObject/PackfileWriter/etc.) are intentionally not implemented.
package gogitstore

import (
	"errors"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/format/idxfile"
	"github.com/go-git/go-git/v5/plumbing/format/objfile"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage/filesystem/dotgit"
	"github.com/tstapler/stapler-squad/log"
)

// errNotImplemented is returned by the write-path EncodedObjectStorer
// methods this prototype deliberately does not implement (see package doc).
var errNotImplemented = errors.New("gogitstore: not implemented in this read-only prototype")

// packIndexEntry pairs a pack's hash with its parsed index, for
// SharedObjectStore.indexSnapshot — see that field's doc comment.
type packIndexEntry struct {
	hash plumbing.Hash
	li   *lockedIndex
}

// SharedObjectStore holds everything that is scoped to a repository's
// commondir (the shared git object database root: $GIT_COMMON_DIR/objects)
// rather than to any single worktree: the parsed packfile indexes and the
// decoded-object LRU cache. Exactly one SharedObjectStore should exist per
// commondir for the lifetime of the process — obtain one via Registry,
// never construct directly from outside this package.
//
// Every worktree of one repository shares the SAME on-disk object
// database (that's what "commondir" means in git's own worktree design —
// see storage/filesystem/dotgit/repository_filesystem.go upstream, which
// this package's split mirrors at the Storer layer instead of the raw
// filesystem layer). There is therefore no correctness reason for each
// worktree to parse its own copy of every .idx file or keep its own
// decoded-object cache — this type exists purely to stop paying that cost
// N times.
type SharedObjectStore struct {
	commonDirAbs string
	dir          *dotgit.DotGit // rooted at the commondir filesystem
	fs           billy.Filesystem
	objectCache  cache.Object // shared decoded-object LRU; already internally mutex-protected (plumbing/cache/object_lru.go)

	largeObjectThreshold int64

	// mu guards TWO independently-discovered pieces of unsynchronized
	// upstream state that this store deliberately shares across worktrees:
	//   1. index/indexBuilt, and every lockedIndex (index.go) handed out by
	//      this store — idxfile.MemoryIndex mutates internal state
	//      (offsetHash) even on lookups (go-git issue #1121).
	//   2. every call into s.dir (dirObject/dirObjectPack below) —
	//      *dotgit.DotGit has its own unguarded lazily-populated caches
	//      (incomingChecked/objectList/packList fields in
	//      storage/filesystem/dotgit/dotgit.go's DotGit struct) that are NOT
	//      safe to touch from more than one goroutine either. This was found
	//      empirically, not anticipated up front — see the design doc §4.3.
	// See index.go's package doc for why a single mutex per store (not per
	// pack) is used for (1); the same reasoning applies to (2).
	mu         sync.Mutex
	index      map[plumbing.Hash]*lockedIndex
	indexBuilt bool

	// indexSnapshot is a read-only []packIndexEntry mirror of index, rebuilt
	// only inside ensureIndex/refreshIndexes (the only two places index is
	// ever mutated) rather than on every object lookup. findObjectInPackfile
	// used to clone the whole index map on every single call — allocating and
	// hashing len(index) entries per lookup — purely to avoid holding mu
	// while calling into a *lockedIndex (each of which re-locks the SAME mu,
	// since lockedIndex.mu is always &s.mu; see index.go's package doc). A
	// slice handed out under a short lock and never mutated in place (only
	// ever replaced wholesale) is safe to iterate lock-free after that lock
	// is released, so the hot path becomes one lock/read/unlock instead of a
	// full map copy. PerfFix-6.
	indexSnapshot []packIndexEntry

	// IndexBuildCount and IndexEntryCount are prototype instrumentation
	// (exported so the package's own test — and any caller who wants to
	// verify the sharing actually happened — can assert on them without
	// reaching into unexported state). IndexBuildCount should be exactly 1
	// no matter how many worktrees share this store; a second parse would
	// mean the sharing is broken.
	IndexBuildCount int32
	IndexEntryCount int64

	// refCount is the number of live WorktreeStorers currently referencing
	// this store (bumped by Registry.acquire, dropped by release — called
	// from WorktreeStorer.Close). A Registry must NEVER evict a store while
	// refCount > 0 — see registry.go's Prune. Accessed only via atomic
	// operations; never touched under mu.
	refCount int32

	// unusedSinceNs records the UnixNano timestamp at which refCount most
	// recently dropped to zero. Only meaningful when refCount is currently
	// zero (a store that is back in use has a stale, ignorable
	// unusedSinceNs — Prune always checks refCount first). Zero means "never
	// gone idle since creation." Accessed only via atomic operations.
	unusedSinceNs int64

	// useMmapIndex mirrors the owning Registry's UseMmapIndex at the moment
	// this store was created (see registry.go) — stage 2's mmap-backed .idx
	// loader (mmapindex.go), design doc §5/§6. false is the copy-based
	// io.ReadFull loader this package has used since stage 0; true engages
	// the zero-copy mmap loader plus its generation/refcount safety scheme
	// and the fsnotify-driven staleness watcher in mmapwatch.go.
	useMmapIndex bool

	// packWatchStarted/packWatchStop back mmapwatch.go's per-store fsnotify
	// watcher on objects/pack, started at most once (on the first
	// ensureIndex build) and only when useMmapIndex is true. Guarded by mu.
	packWatchStarted bool
	packWatchStop    chan struct{}
}

// acquireRefLocked increments refCount and clears unusedSinceNs. Callers
// MUST already hold the owning Registry's mu — see registry.go's acquire,
// the only caller. Keeping the increment inside that critical section (rather
// than a standalone atomic call made after acquire() returns) is what
// prevents a TOCTOU race against Prune: both "hand out a reference" and
// "decide a store is evictable" are serialized behind the same Registry.mu,
// so a store's refCount can never be observed as zero by Prune at the exact
// instant a new acquire() is handing out a fresh reference to it.
func (s *SharedObjectStore) acquireRefLocked() {
	atomic.AddInt32(&s.refCount, 1)
	atomic.StoreInt64(&s.unusedSinceNs, 0)
}

// release drops one reference to this store, called exactly once per
// WorktreeStorer.Close() (see storer.go). When the count reaches zero,
// unusedSinceNs is stamped so Registry.Prune's TTL pass has a "went idle at"
// timestamp to measure against. release does NOT need the owning Registry's
// mu: it never touches the Registry's map, only this store's own atomic
// fields, and Prune always re-checks refCount itself (under its own mu)
// before trusting unusedSinceNs — see Prune's doc comment in registry.go for
// why the resulting narrow race is benign.
func (s *SharedObjectStore) release() {
	if atomic.AddInt32(&s.refCount, -1) == 0 {
		atomic.StoreInt64(&s.unusedSinceNs, time.Now().UnixNano())
	}
}

// RefCount returns the current number of live WorktreeStorers referencing
// this store. Exported for tests/introspection, mirroring
// IndexBuildCount/IndexEntryCount above.
func (s *SharedObjectStore) RefCount() int32 {
	return atomic.LoadInt32(&s.refCount)
}

func newSharedObjectStore(commonDirAbs string, commonFs billy.Filesystem, objectCache cache.Object, largeObjectThreshold int64, useMmapIndex bool) *SharedObjectStore {
	return &SharedObjectStore{
		commonDirAbs:         commonDirAbs,
		dir:                  dotgit.New(commonFs),
		fs:                   commonFs,
		objectCache:          objectCache,
		largeObjectThreshold: largeObjectThreshold,
		index:                make(map[plumbing.Hash]*lockedIndex),
		useMmapIndex:         useMmapIndex,
	}
}

// ensureIndex parses (or, in mmap mode, mmaps) every packfile .idx once.
// Concurrent callers racing on the first call all block on mu; only the
// first actually does the work.
func (s *SharedObjectStore) ensureIndex() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.indexBuilt {
		return nil
	}

	packs, err := s.dir.ObjectPacks()
	if err != nil {
		return err
	}

	built := make(map[plumbing.Hash]*lockedIndex, len(packs))
	var totalEntries int64
	for _, h := range packs {
		li, n, err := s.buildIndexEntryLocked(h)
		if err != nil {
			return err
		}
		built[h] = li
		totalEntries += n
	}

	s.index = built
	s.indexBuilt = true
	s.rebuildIndexSnapshotLocked()
	atomic.AddInt32(&s.IndexBuildCount, 1)
	atomic.AddInt64(&s.IndexEntryCount, totalEntries)

	if s.useMmapIndex {
		// Started only once real index data exists to watch over, and only
		// in mmap mode — the copy-based loader has no stale-mapping problem
		// to detect (its staleness is instead bounded by Registry.Prune's
		// coarser TTL-driven store recreation). See mmapwatch.go.
		s.startPackWatchLocked()
	}
	return nil
}

// buildIndexEntryLocked parses (or mmaps, in mmap mode) pack h's .idx file
// into a *lockedIndex, returning it along with its object count. Callers
// MUST hold s.mu — shared by ensureIndex (initial build) and refreshIndexes
// (incremental staleness-driven rebuild) below.
//
// In mmap mode, a per-pack mmap failure (e.g. the .idx file vanished between
// ObjectPacks() listing it and this call — a real possibility during a
// concurrent repack — or a non-OS-backed billy.Filesystem in some future
// caller) falls back to the copy-based decoder for THAT pack only, rather
// than failing the whole store. Logged at Warn so a persistent fallback is
// visible without being fatal to the caller.
func (s *SharedObjectStore) buildIndexEntryLocked(h plumbing.Hash) (*lockedIndex, int64, error) {
	if s.useMmapIndex {
		if handle, err := openMmapIndexHandle(s.commonDirAbs, h); err == nil {
			n, _ := handle.idx.Count()
			return &lockedIndex{mu: &s.mu, idx: handle.idx, handle: handle}, n, nil
		} else {
			log.Warn("gogitstore: mmap index load failed, falling back to copy-based loader for this pack", "pack", h.String(), "err", err)
		}
	}

	f, err := s.dir.ObjectPackIdx(h)
	if err != nil {
		return nil, 0, err
	}
	mi := idxfile.NewMemoryIndex()
	derr := idxfile.NewDecoder(f).Decode(mi)
	_ = f.Close()
	if derr != nil {
		return nil, 0, derr
	}
	n, _ := mi.Count()
	return &lockedIndex{mu: &s.mu, idx: mi}, n, nil
}

// refreshIndexes re-diffs s against the commondir's current ObjectPacks()
// set, building entries for packs that appeared since the last build/
// refresh and retiring entries for packs that disappeared — superseded by a
// repack (design doc §5.3: git repack/gc never mutates an existing .idx/
// .pack file in place; it always writes new content-hash-named files, then
// unlinks the old ones). No-op if the index has never been built yet (a
// later ensureIndex call will do a full, fresh build from current on-disk
// state instead — there's nothing to "refresh" before anything exists).
//
// This is gogitstore's analogue of filesystem.ObjectStorage.Reindex,
// intended to be triggered by an fsnotify watch on objects/pack — see
// mmapwatch.go, which owns the actual watching and debouncing and calls
// this method on activity.
//
// A retired pack's *lockedIndex is removed from s.index immediately (so new
// FindOffset/getFromPackfile calls stop finding it — "stale", not
// "corrupt"), but its mmapIndexHandle's underlying mapping is only actually
// released via maybeUnmapLocked, which defers to zero live pins — see
// mmapIndexHandle's doc comment and index.go's pinnedEntryIter for what can
// hold a pin past this method's own return.
func (s *SharedObjectStore) refreshIndexes() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.indexBuilt {
		return nil
	}

	packs, err := s.dir.ObjectPacks()
	if err != nil {
		return err
	}
	current := make(map[plumbing.Hash]struct{}, len(packs))
	for _, h := range packs {
		current[h] = struct{}{}
	}

	for h, li := range s.index {
		if _, stillPresent := current[h]; stillPresent {
			continue
		}
		delete(s.index, h)
		if li.handle == nil {
			continue // copy-based entry: nothing to unmap, GC reclaims it once unreferenced
		}
		li.handle.retiring = true
		li.handle.maybeUnmapLocked() // unmaps immediately if pins == 0 (the common case)
	}

	var addedEntries int64
	for h := range current {
		if _, known := s.index[h]; known {
			continue
		}
		li, n, err := s.buildIndexEntryLocked(h)
		if err != nil {
			log.Warn("gogitstore: failed to build index for newly-discovered pack", "pack", h.String(), "err", err)
			continue
		}
		s.index[h] = li
		addedEntries += n
	}
	s.rebuildIndexSnapshotLocked()
	if addedEntries > 0 {
		atomic.AddInt64(&s.IndexEntryCount, addedEntries)
	}
	return nil
}

// rebuildIndexSnapshotLocked refreshes indexSnapshot from the current
// contents of s.index. Callers MUST hold s.mu — shared by ensureIndex and
// refreshIndexes, the only two places s.index is mutated. The old slice
// value is simply replaced, never mutated in place, so callers that grabbed
// the previous slice under a prior lock/unlock keep seeing a consistent
// (if now-stale) view rather than a torn one.
func (s *SharedObjectStore) rebuildIndexSnapshotLocked() {
	snapshot := make([]packIndexEntry, 0, len(s.index))
	for h, li := range s.index {
		snapshot = append(snapshot, packIndexEntry{hash: h, li: li})
	}
	s.indexSnapshot = snapshot
}

// dirObject and dirObjectPack are locked wrappers around s.dir.Object and
// s.dir.ObjectPack. This is NOT the same lock-scope story as lockedIndex —
// see the package/type doc above: dotgit.DotGit itself (storage/filesystem/
// dotgit/dotgit.go) has multiple unguarded, lazily-populated caches
// (incomingChecked/incomingDirName, objectList/objectMap, packList/packMap)
// that Object/ObjectPack/ObjectPacks/hasObject/hasPack read and populate
// with no locking of their own — sharing one *dotgit.DotGit across worktrees
// (which this store deliberately does, for the same reason it shares the
// parsed index) means every call into it needs external synchronization
// too, not just calls into the parsed idxfile.Index. The lock is held only
// around the call that resolves a path and opens the billy.File — once a
// handle is returned, reading FROM it (decompression, packfile Scanner
// work) never touches d's shared state again, so the lock does not extend
// over the expensive part of the read.
func (s *SharedObjectStore) dirObject(h plumbing.Hash) (billy.File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dir.Object(h)
}

func (s *SharedObjectStore) dirObjectPack(pack plumbing.Hash) (billy.File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := s.dir.ObjectPack(pack)
	if err != nil {
		if errors.Is(err, dotgit.ErrPackfileNotFound) || os.IsNotExist(err) {
			// s.index (built by ensureIndex, refreshed by refreshIndexes) can
			// legitimately name a pack whose underlying file a CONCURRENT
			// repack has since unlinked — refreshIndexes/mmapwatch.go's
			// fsnotify watcher (mmap mode) or Registry.Prune's coarser
			// TTL-driven store recreation (copy-based mode, which has no
			// automatic refresh at all — see mmapwatch.go's package doc)
			// eventually catches up, but there is always a window where a
			// lookup can race ahead of that catch-up. Found and measured via
			// this task's soak test (TestGogitstore_SoakUnderSustainedLoad,
			// soak_test.go): under an intentionally adversarial concurrent-
			// repack cadence, copy-based mode hit this on ~98% of
			// operations (no fsnotify catch-up at all) vs mmap mode's ~19%
			// (sub-second fsnotify catch-up) — informative for the
			// UseMmapIndex activation decision, but the error-translation
			// fix here applies to BOTH modes equally.
			//
			// dotgit.ObjectPack returns dotgit.ErrPackfileNotFound in this
			// situation — a DIFFERENT sentinel than plumbing.ErrObjectNotFound,
			// and go-git's own upstream ObjectStorage.getFromPackfile
			// (storage/filesystem/object.go) has this identical gap: it
			// propagates whatever raw error the pack-open call returns,
			// unwrapped — so this is not a gogitstore-introduced regression.
			// It matters here because gogit_vcs_reader.go's callers (e.g.
			// findMergeBase) already tolerate plumbing.ErrObjectNotFound
			// specifically as "skip and continue" — translating the sentinel
			// here lets that existing tolerance logic actually catch this
			// case, instead of a transient staleness race surfacing as an
			// unhandled scan failure.
			return nil, plumbing.ErrObjectNotFound
		}
		return nil, err
	}
	return f, nil
}

// findObjectInPackfile returns which pack (if any) contains h and its
// offset within that pack. It reads the cached indexSnapshot under mu, then
// releases mu before calling FindOffset on each entry (each of those calls
// re-acquires mu itself via lockedIndex) — never held twice at once, so
// this cannot deadlock against lockedIndex's own locking. indexSnapshot is
// only ever replaced wholesale (never mutated in place) by
// rebuildIndexSnapshotLocked, so handing out the current slice value under
// a short lock and iterating it lock-free is safe — no per-call map copy
// needed on this hot path. PerfFix-6.
func (s *SharedObjectStore) findObjectInPackfile(h plumbing.Hash) (pack plumbing.Hash, offset int64, ok bool) {
	s.mu.Lock()
	packs := s.indexSnapshot
	s.mu.Unlock()

	for _, entry := range packs {
		off, err := entry.li.FindOffset(h)
		if err == nil {
			return entry.hash, off, true
		}
	}
	return plumbing.ZeroHash, 0, false
}

// EncodedObject implements the shared, read-only half of
// storer.EncodedObjectStorer. WorktreeStorer.EncodedObject delegates to
// encodedObject (below) with its own per-worktree pack handle cache instead
// of calling this method directly — there is no per-worktree object storage
// at all in this prototype, only per-worktree Reference/Index/Shallow/Config
// storage (see storer.go). This exported method passes a nil handle cache,
// preserving the original fresh-handle-per-call behavior for any direct
// caller/test that constructs a SharedObjectStore without a WorktreeStorer.
func (s *SharedObjectStore) EncodedObject(t plumbing.ObjectType, h plumbing.Hash) (plumbing.EncodedObject, error) {
	return s.encodedObject(t, h, nil)
}

// encodedObject is EncodedObject's implementation, parameterized on an
// optional per-worktree packHandleCache (nil means "no caching, open a fresh
// pack handle per call" — see getFromPackfile's doc comment).
func (s *SharedObjectStore) encodedObject(t plumbing.ObjectType, h plumbing.Hash, handles *packHandleCache) (plumbing.EncodedObject, error) {
	obj, err := s.getFromUnpacked(h)
	if errors.Is(err, plumbing.ErrObjectNotFound) {
		obj, err = s.getFromPackfile(h, handles)
	}
	if err != nil {
		return nil, err
	}
	if t != plumbing.AnyObject && obj.Type() != t {
		return nil, plumbing.ErrObjectNotFound
	}
	return obj, nil
}

func (s *SharedObjectStore) getFromUnpacked(h plumbing.Hash) (obj plumbing.EncodedObject, err error) {
	f, err := s.dirObject(h)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, plumbing.ErrObjectNotFound
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	if cached, found := s.objectCache.Get(h); found {
		return cached, nil
	}

	r, err := objfile.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()

	typ, size, err := r.Header()
	if err != nil {
		return nil, err
	}

	mo := &plumbing.MemoryObject{}
	mo.SetType(typ)
	mo.SetSize(size)
	w, err := mo.Writer()
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(w, r); err != nil {
		return nil, err
	}
	s.objectCache.Put(mo)
	return mo, nil
}

// getFromPackfile resolves h from a packfile. When handles is nil it opens a
// FRESH billy.File handle to the pack for every call rather than caching one
// — the original prototype behavior, kept as the default for any direct
// SharedObjectStore caller/test. This is a deliberate prototype
// simplification, not an accident: a *packfile.Packfile / *packfile.Scanner
// pair holds a seek cursor over its billy.File, so sharing ONE open pack
// handle across concurrent worktree reads would race exactly like sharing
// the raw MemoryIndex does. Opening a fresh os-level file handle per call
// sidesteps that: concurrent reads of the same pack get independent file
// descriptors (cheap; the OS page cache is already shared underneath them),
// while the expensive parts — the parsed idxfile.Index and the decoded
// cache.Object — are still the single shared instances from this store.
//
// When handles is non-nil (the production path — see storer.go's
// WorktreeStorer, which owns a packHandleCache PER WORKTREE, not per store),
// one open pack handle is reused across calls for THIS worktree's reads of
// the given pack, cutting the per-object-read syscall overhead the design
// doc's stage-1 follow-up calls out. This is safe and does not reintroduce
// the concurrent-handle race above: the handle cache is scoped to one
// worktree (never shared across worktrees, unlike the index/object cache),
// and each cachedPackHandle carries its own mutex serializing reads through
// it, so even concurrent callers on the SAME WorktreeStorer (not how
// session/unfinished's GoGitVCSReader uses this today — every call already
// goes through cachedRepo.mu — but not guaranteed by this package's own
// contract) cannot corrupt the shared seek cursor. GetByOffset always seeks
// from the start of the file before reading (see go-git's
// packfile.Packfile.GetByOffset → Scanner.SeekFromStart), so reusing the
// underlying handle across sequential reads is correct; a fresh
// *packfile.Packfile/*packfile.Scanner pair is still constructed per call
// (cheap — a small struct, no I/O) so no state leaks between reads.
// Crucially, when handles is non-nil the returned Packfile is never Close()'d
// here — that would close the cached handle out from under later calls. The
// handle is closed once, on WorktreeStorer.Close (packHandleCache.closeAll),
// tying its lifetime to the worktree, not to a single read.
func (s *SharedObjectStore) getFromPackfile(h plumbing.Hash, handles *packHandleCache) (plumbing.EncodedObject, error) {
	if err := s.ensureIndex(); err != nil {
		return nil, err
	}
	pack, offset, ok := s.findObjectInPackfile(h)
	if !ok {
		return nil, plumbing.ErrObjectNotFound
	}

	s.mu.Lock()
	idx := s.index[pack]
	s.mu.Unlock()

	if handles == nil {
		f, err := s.dirObjectPack(pack)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()

		p := packfile.NewPackfileWithCache(idx, s.fs, f, s.objectCache, s.largeObjectThreshold)
		defer func() { _ = p.Close() }()

		return p.GetByOffset(offset)
	}

	ch, err := handles.get(pack, func() (billy.File, error) { return s.dirObjectPack(pack) })
	if err != nil {
		return nil, err
	}
	ch.mu.Lock()
	defer ch.mu.Unlock()

	if debugStallHeldPackHandleRead != nil {
		debugStallHeldPackHandleRead()
	}

	p := packfile.NewPackfileWithCache(idx, s.fs, ch.f, s.objectCache, s.largeObjectThreshold)
	// Deliberately NOT p.Close() — see doc comment above.
	return p.GetByOffset(offset)
}

// debugStallHeldPackHandleRead is a test-only injection point (nil in
// production) letting a test force a getFromPackfile read to hold ch.mu for
// an arbitrary duration, to deterministically reproduce the
// closeAll/getFromPackfile lock-contention hang this file's closeAll doc
// comment describes, instead of relying on hitting it by pure timing luck
// under soak load.
var debugStallHeldPackHandleRead func()

// HasEncodedObject implements storer.EncodedObjectStorer.
func (s *SharedObjectStore) HasEncodedObject(h plumbing.Hash) error {
	if f, err := s.dirObject(h); err == nil {
		_ = f.Close()
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := s.ensureIndex(); err != nil {
		return err
	}
	if _, _, ok := s.findObjectInPackfile(h); !ok {
		return plumbing.ErrObjectNotFound
	}
	return nil
}

// EncodedObjectSize implements storer.EncodedObjectStorer.
//
// This is a prototype simplification: it fully materializes the object via
// EncodedObject rather than reading just the size from the object/pack
// header the way filesystem.ObjectStorage.EncodedObjectSize does. None of
// the three scanner operations this package targets (HasUncommitted,
// DiffShortstat, AheadBehind) call EncodedObjectSize, so this is not on the
// hot path — flagged here rather than optimized, per the task's read-only
// read-path-only scope.
func (s *SharedObjectStore) EncodedObjectSize(h plumbing.Hash) (int64, error) {
	return s.encodedObjectSize(h, nil)
}

func (s *SharedObjectStore) encodedObjectSize(h plumbing.Hash, handles *packHandleCache) (int64, error) {
	obj, err := s.encodedObject(plumbing.AnyObject, h, handles)
	if err != nil {
		return 0, err
	}
	return obj.Size(), nil
}

// IterEncodedObjects is intentionally unimplemented. None of
// HasUncommitted/DiffShortstat/AheadBehind (session/unfinished/vcsreader.go)
// walk the full object set — they all resolve specific hashes (HEAD,
// blobs, commits reachable via parent links) via EncodedObject. A full
// materialization/GC/fsck-style caller would need this implemented
// (mirroring filesystem.ObjectStorage.IterEncodedObjects — loose-object
// directory scan + per-pack iterators); left as a documented follow-up.
func (s *SharedObjectStore) IterEncodedObjects(plumbing.ObjectType) (storer.EncodedObjectIter, error) {
	return nil, errNotImplemented
}

// packHandleCache is a per-worktree cache of open pack file handles, owned
// by exactly one WorktreeStorer (see storer.go) and threaded through
// SharedObjectStore.getFromPackfile's handles parameter. Deliberately NOT a
// field on SharedObjectStore: caching MUST be scoped per-worktree, not
// per-commondir, because sharing one open handle across CONCURRENT worktree
// reads of the same pack would race exactly like sharing a raw MemoryIndex
// does (see getFromPackfile's doc comment) — keeping this cache on
// WorktreeStorer instead preserves the cross-worktree read parallelism the
// original fresh-handle-per-call design intentionally provided, while still
// avoiding repeated opens within one worktree's own read sequence.
//
// Zero value is ready to use (handles is lazily allocated by get).
type packHandleCache struct {
	mu      sync.Mutex
	handles map[plumbing.Hash]*cachedPackHandle
}

// get returns the cached handle for pack, opening one via open() on first
// use. open() is called at most once per distinct pack for this cache's
// lifetime (subsequent calls return the same *cachedPackHandle).
func (c *packHandleCache) get(pack plumbing.Hash, open func() (billy.File, error)) (*cachedPackHandle, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.handles == nil {
		c.handles = make(map[plumbing.Hash]*cachedPackHandle)
	}
	if h, ok := c.handles[pack]; ok {
		return h, nil
	}
	f, err := open()
	if err != nil {
		return nil, err
	}
	h := &cachedPackHandle{f: f}
	c.handles[pack] = h
	return h, nil
}

// packHandleCloseTimeout bounds how long closeAll waits for any single
// handle's h.mu before giving up on that handle and moving on — see
// closeAll's doc comment for why a concurrent reader can legitimately hold
// h.mu for longer than this (a stuck/slow getFromPackfile read), confirmed
// via a fresh goroutine dump captured from TestGogitstore_SoakUnderSustainedLoad
// (a worker's Close() blocked in sync.Once.doSlow while the Once-executing
// goroutine was itself blocked acquiring this same h.mu inside closeAll).
const packHandleCloseTimeout = 5 * time.Second

// closeAll closes every cached pack handle. It used to hold c.mu across a
// per-handle h.mu.Lock() loop — but getFromPackfile can legitimately hold a
// handle's h.mu for the duration of a raw packfile read, and closeAll runs
// inside WorktreeStorer.Close()'s sync.Once.Do body, so a single stuck read
// wedged closeAll forever, which wedged the Once, which wedged every other
// concurrent Close() caller in sync.Once.doSlow. Confirmed via a real
// pprof.Lookup("goroutine") dump (see closeall_hang_repro_test.go).
//
// Fix: snapshot-and-clear the map under a brief c.mu critical section (so
// closeAll no longer serializes with packHandleCache.get), then close every
// handle concurrently with a bounded per-handle wait on h.mu. A handle whose
// read doesn't finish within packHandleCloseTimeout is hedged off to a
// detached background goroutine that keeps waiting and closes it once free —
// so closeAll's (and thus Close()'s) own wall-clock time is bounded to about
// one timeout period regardless of how many handles are stuck, while no fd
// is ever leaked.
func (c *packHandleCache) closeAll() {
	c.mu.Lock()
	handles := c.handles
	c.handles = nil
	c.mu.Unlock()

	var wg sync.WaitGroup
	for pack, h := range handles {
		wg.Add(1)
		go func(pack plumbing.Hash, h *cachedPackHandle) {
			defer wg.Done()
			closeHandleBounded(pack, h)
		}(pack, h)
	}
	wg.Wait()
}

// closeHandleBounded closes h, waiting up to packHandleCloseTimeout to
// acquire h.mu before handing off to a detached background goroutine that
// closes it once free — guaranteeing the fd is eventually closed without
// making the caller (closeAll) wait indefinitely for a stuck reader.
func closeHandleBounded(pack plumbing.Hash, h *cachedPackHandle) {
	acquired := make(chan struct{})
	go func() {
		h.mu.Lock()
		close(acquired)
	}()

	select {
	case <-acquired:
		defer h.mu.Unlock()
		_ = h.f.Close()
	case <-time.After(packHandleCloseTimeout):
		log.Warn("gogitstore: pack handle close is taking longer than expected — a concurrent read is likely still in flight; will close it in the background once free to avoid an fd leak", "pack", pack.String(), "timeout", packHandleCloseTimeout)
		go func() {
			<-acquired
			defer h.mu.Unlock()
			_ = h.f.Close()
			log.Warn("gogitstore: slow pack handle close completed", "pack", pack.String())
			if debugHedgedPackHandleCloseDone != nil {
				debugHedgedPackHandleCloseDone()
			}
		}()
	}
}

// debugHedgedPackHandleCloseDone is a test-only injection point (nil in
// production) letting a test observe when closeHandleBounded's hedged-off
// background goroutine has finished closing a handle, so a test can wait
// for that goroutine to complete instead of returning while it is still
// running — see closeHandleBounded's timeout branch.
var debugHedgedPackHandleCloseDone func()

// cachedPackHandle wraps one long-lived billy.File open on a pack, plus a
// mutex serializing reads through it — packfile.Packfile/Scanner hold a seek
// cursor over the underlying file, so concurrent reads through the SAME
// handle would race exactly like sharing a raw MemoryIndex does (see the
// design doc §4.1). This mutex is per-pack, not per-cache, so concurrent
// reads of DIFFERENT packs (even within the same worktree) are unaffected by
// each other.
type cachedPackHandle struct {
	mu sync.Mutex
	f  billy.File
}
