package gogitstore

import (
	"sync"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// WorktreeStorer implements storage.Storer for exactly one worktree, while
// delegating all object storage to a SharedObjectStore that may be shared
// by many other WorktreeStorers pointed at sibling worktrees of the same
// repository.
//
// It embeds *filesystem.Storage purely to get correctly-wired, genuinely
// per-worktree Reference/Index/Shallow/Config/Module storage for free:
// filesystem.Storage's ReferenceStorage/IndexStorage/etc. types have
// unexported `dir *dotgit.DotGit` fields with no exported constructor, so
// they cannot be built standalone from outside the filesystem package — the
// only way to obtain correctly-configured instances is to let
// filesystem.NewStorageWithOptions build a full Storage and keep the parts
// we want.
//
// filesystem.Storage ALSO embeds an ObjectStorage (which implements
// storer.EncodedObjectStorer), but every method of that interface is
// re-declared directly on WorktreeStorer below. Go's method promotion
// resolves methods at the shallowest embedding depth, so WorktreeStorer's
// own methods always win — the embedded ObjectStorage's methods are
// therefore dead code, present only because embedding is all-or-nothing.
// (Confirmed by TestWorktreeStorer_UsesSharedStore_NotEmbeddedObjectStorage
// in storer_test.go, not just by reading the method-resolution rules.)
//
// The fs/cache passed to filesystem.NewStorageWithOptions when constructing
// the embedded *filesystem.Storage is intentionally minimal (see open.go) —
// its ObjectStorage is never invoked through WorktreeStorer's own
// interface satisfaction, so there is no reason to spend real memory on it.
type WorktreeStorer struct {
	*filesystem.Storage
	shared *SharedObjectStore

	// packHandles is this worktree's own cache of open pack file handles —
	// see store.go's packHandleCache doc comment for why this must live here
	// (per worktree) rather than on the shared SharedObjectStore (per
	// commondir). Zero value is ready to use.
	packHandles packHandleCache

	// closeOnce guards release() (SharedObjectStore.release, called via
	// Close below) against being invoked more than once for this
	// WorktreeStorer. release() is not idempotent — it always decrements an
	// atomic refcount — so a double call would under-count live references
	// and could cause Registry.Prune to evict a store still in use by
	// another worktree. Callers (session/unfinished's GoGitVCSReader) are
	// expected to call Close exactly once when evicting the cachedRepo that
	// wraps this WorktreeStorer, but closeOnce makes a defensive double-call
	// harmless rather than a silent correctness bug.
	closeOnce sync.Once
}

// Close releases this WorktreeStorer's claim on its SharedObjectStore's
// reference count and closes any cached pack file handles (see
// packHandles). Callers MUST call this exactly once when a cachedRepo
// wrapping this WorktreeStorer is evicted from GoGitVCSReader.repoCache —
// see gogit_vcs_reader.go's pruneRepoCache/ClearCache/openRepoEntry. Safe to
// call more than once; only the first call has any effect. Returns nil
// unconditionally — pack handle close errors are logged, not propagated,
// since a failed close on an already-evicted worktree has no actionable
// recovery for the caller.
func (w *WorktreeStorer) Close() error {
	w.closeOnce.Do(func() {
		w.packHandles.closeAll()
		w.shared.release()
	})
	return nil
}

var _ storer.EncodedObjectStorer = (*WorktreeStorer)(nil)

// NewEncodedObject implements storer.EncodedObjectStorer. Constructing a
// blank in-memory object is not a "write" in the sense the rest of this
// prototype leaves unimplemented — it's the same MemoryObject{} allocation
// filesystem.ObjectStorage.NewEncodedObject performs.
func (w *WorktreeStorer) NewEncodedObject() plumbing.EncodedObject {
	return &plumbing.MemoryObject{}
}

// SetEncodedObject implements storer.EncodedObjectStorer. Unimplemented:
// see package doc — this is a read-only prototype and none of
// HasUncommitted/DiffShortstat/AheadBehind write objects.
func (w *WorktreeStorer) SetEncodedObject(plumbing.EncodedObject) (plumbing.Hash, error) {
	return plumbing.ZeroHash, errNotImplemented
}

// EncodedObject implements storer.EncodedObjectStorer by delegating to the
// shared, commondir-scoped object store, passing this worktree's own
// per-worktree pack handle cache (packHandles) so repeated packfile reads
// reuse one open handle instead of opening a fresh one every call.
func (w *WorktreeStorer) EncodedObject(t plumbing.ObjectType, h plumbing.Hash) (plumbing.EncodedObject, error) {
	return w.shared.encodedObject(t, h, &w.packHandles)
}

// IterEncodedObjects implements storer.EncodedObjectStorer. See
// SharedObjectStore.IterEncodedObjects's doc comment for why this is
// unimplemented in the prototype.
func (w *WorktreeStorer) IterEncodedObjects(t plumbing.ObjectType) (storer.EncodedObjectIter, error) {
	return w.shared.IterEncodedObjects(t)
}

// HasEncodedObject implements storer.EncodedObjectStorer.
func (w *WorktreeStorer) HasEncodedObject(h plumbing.Hash) error {
	return w.shared.HasEncodedObject(h)
}

// EncodedObjectSize implements storer.EncodedObjectStorer.
func (w *WorktreeStorer) EncodedObjectSize(h plumbing.Hash) (int64, error) {
	return w.shared.encodedObjectSize(h, &w.packHandles)
}

// AddAlternate implements storer.EncodedObjectStorer. Unimplemented in the
// prototype — alternates (git's `objects/info/alternates`, e.g. for
// `git clone --shared`) are rare for the worktrees this scanner targets and
// were out of scope for the time available; a production version should
// route this to the shared store's dotgit.DotGit.AddAlternate and extend
// findObjectInPackfile/getFromUnpacked to fall back to alternate object
// stores, mirroring filesystem.ObjectStorage.EncodedObject's alternates
// fallback.
func (w *WorktreeStorer) AddAlternate(string) error {
	return errNotImplemented
}

// throwawayObjectCache backs the embedded *filesystem.Storage's own
// ObjectStorage, which — per the type doc above — is never actually
// invoked through WorktreeStorer's interface methods. Sized as small as
// go-git's cache.Object implementations allow rather than 0, since a couple
// of go-git internal codepaths (e.g. Storage.Filesystem()) merely need a
// non-nil cache.Object, not a functioning one.
func throwawayObjectCache() cache.Object {
	return cache.NewObjectLRU(1)
}
