package gogitstore

// Tests for stage-1 production hardening: Registry TTL/budget eviction,
// refcount-safety (a SharedObjectStore must never be evicted while any
// WorktreeStorer still references it), and per-worktree pack handle
// caching. See session/unfinished/design/pluggable-gitstore.md §6 (staged
// rollout plan, stage 1) and registry.go/store.go's doc comments for the
// mechanism under test.

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	billy "github.com/go-git/go-billy/v5"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
)

// withRegistryHeapPressure overrides this package's readHeapInUse var for
// the duration of the test, restoring the original on cleanup. Mirrors
// session/unfinished/gogit_vcs_reader_memory_test.go's withHeapInUse.
func withRegistryHeapPressure(t *testing.T, bytes uint64) {
	t.Helper()
	orig := readHeapInUse
	readHeapInUse = func() uint64 { return bytes }
	t.Cleanup(func() { readHeapInUse = orig })
}

// seedFakeStore directly populates reg.stores with a synthetic
// SharedObjectStore (no real git repo needed — Prune only touches
// refCount/unusedSinceNs and the map itself). Mirrors
// gogit_vcs_reader_memory_test.go's seedFakeCacheEntries.
func seedFakeStore(reg *Registry, key string, unusedSinceNs int64) *SharedObjectStore {
	s := &SharedObjectStore{commonDirAbs: key, index: make(map[plumbing.Hash]*lockedIndex)}
	atomic.StoreInt64(&s.unusedSinceNs, unusedSinceNs)
	if reg.stores == nil {
		reg.stores = make(map[string]*SharedObjectStore)
	}
	reg.stores[key] = s
	return s
}

// --- (a) never evict while refcount > 0 -----------------------------------

func TestRegistry_Prune_should_neverEvict_When_RefCountNonzero(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	mainRepo := filepath.Join(dir, "main")
	buildPackedFixture(t, mainRepo, 20)

	reg := &Registry{CacheMaxSize: 1} // tiny budget: maximize eviction pressure
	repo, err := Open(mainRepo, reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// Deliberately never Close() the resulting WorktreeStorer — its
	// reference to the shared store stays live for the whole test.
	if _, err := repo.Head(); err != nil {
		t.Fatalf("Head: %v", err)
	}

	commonDirAbs := canonicalizeDir(filepath.Join(mainRepo, ".git"))
	store, ok := reg.stores[commonDirAbs]
	if !ok {
		t.Fatalf("no SharedObjectStore for %s", commonDirAbs)
	}
	if got := store.RefCount(); got != 1 {
		t.Fatalf("RefCount = %d, want 1 (one live WorktreeStorer)", got)
	}

	// Stack BOTH eviction signals against the store — TTL long past due,
	// AND severe memory pressure — to prove refcount safety wins over both,
	// not just one.
	atomic.StoreInt64(&store.unusedSinceNs, time.Now().Add(-registryStoreTTL-time.Hour).UnixNano())
	withRegistryHeapPressure(t, registrySevereMemoryPressureThreshold+1)

	reg.Prune()

	if _, ok := reg.stores[commonDirAbs]; !ok {
		t.Error("store was evicted despite RefCount()==1 — refcount safety property violated")
	}
	if got := store.RefCount(); got != 1 {
		t.Errorf("RefCount changed to %d across Prune(), want unchanged 1", got)
	}
}

// --- (b) becomes evictable once refcount drops to zero ---------------------

func TestRegistry_Prune_should_evict_When_RefCountZero_AndPastTTL(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	mainRepo := filepath.Join(dir, "main")
	buildPackedFixture(t, mainRepo, 20)

	reg := NewRegistry()
	repo, err := Open(mainRepo, reg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ws, ok := repo.Storer.(*WorktreeStorer)
	if !ok {
		t.Fatalf("repo.Storer is %T, want *WorktreeStorer", repo.Storer)
	}

	commonDirAbs := canonicalizeDir(filepath.Join(mainRepo, ".git"))
	store, ok := reg.stores[commonDirAbs]
	if !ok {
		t.Fatalf("no SharedObjectStore for %s", commonDirAbs)
	}
	if got := store.RefCount(); got != 1 {
		t.Fatalf("RefCount = %d, want 1 before Close", got)
	}

	if err := ws.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := store.RefCount(); got != 0 {
		t.Fatalf("RefCount = %d after Close, want 0", got)
	}

	// Not evicted yet — went idle "just now", well within the TTL.
	reg.Prune()
	if _, ok := reg.stores[commonDirAbs]; !ok {
		t.Fatal("store evicted immediately on going idle — TTL should still protect it")
	}

	// Force it past the TTL and prune again.
	atomic.StoreInt64(&store.unusedSinceNs, time.Now().Add(-registryStoreTTL-time.Minute).UnixNano())
	reg.Prune()
	if _, ok := reg.stores[commonDirAbs]; ok {
		t.Error("expected store to be TTL-evicted once idle past registryStoreTTL with RefCount()==0")
	}
}

// TestRegistry_ThreeWorktreesOfOneRepo_OnlyFullyReleasedStoreIsEvicted is
// the manual-smoke-check scenario called for in the task: open 3 worktrees
// of ONE repository (all sharing a single commondir, and therefore a single
// SharedObjectStore — refcount, not a per-worktree flag), force eviction
// pressure via a tiny TTL/budget override, and confirm the store survives
// as long as ANY of the 3 worktrees still references it, only becoming
// evictable once all 3 have released it. This specifically exercises
// counting (refcount 3 → 2 → 1 → 0), not just a binary in-use check.
func TestRegistry_ThreeWorktreesOfOneRepo_OnlyFullyReleasedStoreIsEvicted(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	mainRepo := filepath.Join(dir, "main")
	buildPackedFixture(t, mainRepo, 20)

	wt1 := filepath.Join(dir, "wt1")
	wt2 := filepath.Join(dir, "wt2")
	gitRun(t, mainRepo, "worktree", "add", "-q", "-b", "wt1-branch", wt1, "main")
	gitRun(t, mainRepo, "worktree", "add", "-q", "-b", "wt2-branch", wt2, "main")

	reg := &Registry{CacheMaxSize: 1} // tiny budget: maximize eviction pressure
	withRegistryHeapPressure(t, registrySevereMemoryPressureThreshold+1)

	repoMain, err := Open(mainRepo, reg)
	if err != nil {
		t.Fatalf("Open(main): %v", err)
	}
	repoWt1, err := Open(wt1, reg)
	if err != nil {
		t.Fatalf("Open(wt1): %v", err)
	}
	repoWt2, err := Open(wt2, reg)
	if err != nil {
		t.Fatalf("Open(wt2): %v", err)
	}

	commonDirAbs := canonicalizeDir(filepath.Join(mainRepo, ".git"))
	store, ok := reg.stores[commonDirAbs]
	if !ok {
		t.Fatalf("no SharedObjectStore for %s", commonDirAbs)
	}
	if got := store.RefCount(); got != 3 {
		t.Fatalf("RefCount = %d after opening 3 worktrees of 1 repo, want 3 (they share ONE store)", got)
	}

	forcePastTTL := func() {
		t.Helper()
		atomic.StoreInt64(&store.unusedSinceNs, time.Now().Add(-registryStoreTTL-time.Minute).UnixNano())
	}
	closerOf := func(repo *git.Repository) *WorktreeStorer {
		t.Helper()
		ws, ok := repo.Storer.(*WorktreeStorer)
		if !ok {
			t.Fatalf("Storer is %T, want *WorktreeStorer", repo.Storer)
		}
		return ws
	}

	// Release worktree #1 of 3: store must still survive (2 remain).
	if err := closerOf(repoWt1).Close(); err != nil {
		t.Fatalf("Close(wt1): %v", err)
	}
	forcePastTTL()
	reg.Prune()
	if _, ok := reg.stores[commonDirAbs]; !ok {
		t.Fatal("store evicted after releasing only 1 of 3 worktrees — refcount safety violated")
	}
	if got := store.RefCount(); got != 2 {
		t.Fatalf("RefCount = %d after releasing 1 of 3, want 2", got)
	}

	// Release worktree #2 of 3: store must STILL survive (1 remains — main).
	if err := closerOf(repoWt2).Close(); err != nil {
		t.Fatalf("Close(wt2): %v", err)
	}
	forcePastTTL()
	reg.Prune()
	if _, ok := reg.stores[commonDirAbs]; !ok {
		t.Fatal("store evicted after releasing 2 of 3 worktrees — refcount safety violated")
	}
	if got := store.RefCount(); got != 1 {
		t.Fatalf("RefCount = %d after releasing 2 of 3, want 1", got)
	}

	// Release the last (main) worktree: NOW the store has zero references
	// and, once past TTL, must actually be evicted.
	if err := closerOf(repoMain).Close(); err != nil {
		t.Fatalf("Close(main): %v", err)
	}
	if got := store.RefCount(); got != 0 {
		t.Fatalf("RefCount = %d after releasing all 3 worktrees, want 0", got)
	}
	forcePastTTL()
	reg.Prune()
	if _, ok := reg.stores[commonDirAbs]; ok {
		t.Error("store survived Prune() after all 3 worktrees released it and it went past TTL — should have been evicted")
	}
}

func TestRegistry_Prune_should_notEvict_When_RefCountZero_ButUnderTTLAndBudget(t *testing.T) {
	reg := &Registry{}
	seedFakeStore(reg, "repo-a", time.Now().UnixNano()) // just went idle

	withRegistryHeapPressure(t, 1*1024*1024*1024) // no pressure
	reg.Prune()

	if _, ok := reg.stores["repo-a"]; !ok {
		t.Error("store evicted despite being well under both TTL and budget")
	}
}

// --- (c) budget-derived shrink mirrors gogit_vcs_reader.go's tiers --------

func TestRegistry_effectiveBudgetBytes_should_returnFullBudget_When_NoPressure(t *testing.T) {
	reg := &Registry{}
	withRegistryHeapPressure(t, 1*1024*1024*1024) // below high threshold
	if got := reg.effectiveBudgetBytes(); got != registryDefaultMemoryBudgetBytes {
		t.Errorf("got %d, want full budget %d", got, registryDefaultMemoryBudgetBytes)
	}
}

func TestRegistry_effectiveBudgetBytes_should_halveBudget_When_HighPressure(t *testing.T) {
	reg := &Registry{}
	withRegistryHeapPressure(t, registryHighMemoryPressureThreshold+1)
	want := int64(registryDefaultMemoryBudgetBytes / 2)
	if got := reg.effectiveBudgetBytes(); got != want {
		t.Errorf("got %d, want halved budget %d", got, want)
	}
}

func TestRegistry_effectiveBudgetBytes_should_floorToFiveStores_When_SeverePressure(t *testing.T) {
	reg := &Registry{CacheMaxSize: 12 * 1024 * 1024}
	withRegistryHeapPressure(t, registrySevereMemoryPressureThreshold+1)
	want := reg.approxBytesPerStore() * 5
	if got := reg.effectiveBudgetBytes(); got != want {
		t.Errorf("got %d, want severe-pressure floor %d", got, want)
	}
}

func TestRegistry_effectiveMaxEntries_should_shrink_When_PressureIncreases(t *testing.T) {
	reg := &Registry{CacheMaxSize: 1}

	withRegistryHeapPressure(t, 1*1024*1024*1024)
	normal := reg.effectiveMaxEntries()

	withRegistryHeapPressure(t, registryHighMemoryPressureThreshold+1)
	high := reg.effectiveMaxEntries()

	withRegistryHeapPressure(t, registrySevereMemoryPressureThreshold+1)
	severe := reg.effectiveMaxEntries()

	if normal < high || high <= severe {
		t.Errorf("expected non-increasing caps under increasing pressure, got normal=%d high=%d severe=%d", normal, high, severe)
	}
	if severe < 1 {
		t.Errorf("effectiveMaxEntries must never go below 1, got %d", severe)
	}
	if normal > registryMaxEntries {
		t.Errorf("effectiveMaxEntries must never exceed registryMaxEntries=%d, got %d", registryMaxEntries, normal)
	}
}

// TestRegistry_Prune_should_LRUTrimZeroRefcountStores_When_OverBudget proves
// Prune's LRU-trim pass only touches zero-refcount candidates and trims down
// to the memory-derived cap, oldest-idle first — the gogitstore-package
// equivalent of gogit_vcs_reader_memory_test.go's
// TestPruneRepoCache_should_trimToMemoryDerivedCap_When_OverBudgetButUnderFlatConstant.
func TestRegistry_Prune_should_LRUTrimZeroRefcountStores_When_OverBudget(t *testing.T) {
	reg := &Registry{CacheMaxSize: 1} // approxBytesPerStore() == 1
	const n = 20
	now := time.Now().UnixNano()
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("fake-repo-%02d", i)
		// Stagger unusedSinceNs so ordering is deterministic: entry i went
		// idle before entry i+1 (i.e. entry i is "more evictable").
		seedFakeStore(reg, key, now-int64((n-i)*int(time.Second)))
	}
	if got := len(reg.stores); got != n {
		t.Fatalf("setup: got %d stores, want %d", got, n)
	}

	withRegistryHeapPressure(t, registrySevereMemoryPressureThreshold+1)
	reg.Prune()

	const want = 5 // approxBytesPerStore(1)*5 / approxBytesPerStore(1)
	if got := len(reg.stores); got != want {
		t.Fatalf("got %d stores after prune under severe pressure, want %d (the memory-derived cap)", got, want)
	}
	// The survivors must be the most-recently-unused ones (highest index).
	for i := n - want; i < n; i++ {
		key := fmt.Sprintf("fake-repo-%02d", i)
		if _, ok := reg.stores[key]; !ok {
			t.Errorf("expected %s (recently unused) to survive the LRU trim", key)
		}
	}
	for i := 0; i < n-want; i++ {
		key := fmt.Sprintf("fake-repo-%02d", i)
		if _, ok := reg.stores[key]; ok {
			t.Errorf("expected %s (longest idle) to be evicted by the LRU trim", key)
		}
	}
}

// --- concurrency: the safety property under real concurrent load ----------

// TestRegistry_ConcurrentOpenClose_NeverEvictsInUseStore is the single most
// important test in this file. It pins one repo's SharedObjectStore open for
// the whole test (never Close()'d) while hammering Registry.Prune()
// concurrently from one goroutine AND churning Open/Close on a SECOND,
// unrelated repo from several goroutines concurrently — so the pinned
// store's refcount is being read by Prune() at the same time completely
// unrelated stores are genuinely oscillating between zero and nonzero
// refcount. Must be run with -race; the checker goroutine also asserts,
// WHILE the pressure is being applied (not just once at the end), that the
// pinned store is never removed from the registry and its refcount never
// drops below 1.
func TestRegistry_ConcurrentOpenClose_NeverEvictsInUseStore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()

	pinnedRepo := filepath.Join(dir, "pinned")
	buildPackedFixture(t, pinnedRepo, 30)
	churnRepo := filepath.Join(dir, "churn")
	buildPackedFixture(t, churnRepo, 30)

	reg := &Registry{CacheMaxSize: 1} // tiny budget: maximize eviction pressure
	withRegistryHeapPressure(t, registrySevereMemoryPressureThreshold+1)

	pinnedCommonDirAbs := canonicalizeDir(filepath.Join(pinnedRepo, ".git"))

	pinnedRepoHandle, err := Open(pinnedRepo, reg)
	if err != nil {
		t.Fatalf("Open(pinned): %v", err)
	}
	if _, err := pinnedRepoHandle.Head(); err != nil {
		t.Fatalf("Head(pinned): %v", err)
	}
	pinnedStore, ok := reg.stores[pinnedCommonDirAbs]
	if !ok {
		t.Fatalf("no SharedObjectStore for pinned repo %s", pinnedCommonDirAbs)
	}

	pruneStop := make(chan struct{})
	pruneDone := make(chan struct{})
	go func() {
		defer close(pruneDone)
		for {
			select {
			case <-pruneStop:
				return
			default:
				reg.Prune()
			}
		}
	}()

	const numChurners = 8
	const iterationsPerChurner = 150
	var wg sync.WaitGroup

	for i := 0; i < numChurners; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterationsPerChurner; j++ {
				repo, err := Open(churnRepo, reg)
				if err != nil {
					t.Errorf("Open(churn): %v", err)
					return
				}
				if _, err := repo.Head(); err != nil {
					t.Errorf("Head(churn): %v", err)
					return
				}
				ws, ok := repo.Storer.(*WorktreeStorer)
				if !ok {
					t.Errorf("Storer is %T, want *WorktreeStorer", repo.Storer)
					return
				}
				if err := ws.Close(); err != nil {
					t.Errorf("Close(churn): %v", err)
					return
				}
			}
		}()
	}

	// Checker: repeatedly re-verify the pinned store's invariants WHILE the
	// churners and the pruner are both hammering concurrently.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			reg.mu.Lock()
			_, stillPresent := reg.stores[pinnedCommonDirAbs]
			reg.mu.Unlock()
			if !stillPresent {
				t.Error("pinned store was evicted from the registry while RefCount() should be >= 1")
				return
			}
			if rc := pinnedStore.RefCount(); rc < 1 {
				t.Errorf("pinned store RefCount() = %d mid-test, want >= 1 at all times (never released)", rc)
				return
			}
		}
	}()

	wg.Wait()
	close(pruneStop)
	<-pruneDone

	if _, ok := reg.stores[pinnedCommonDirAbs]; !ok {
		t.Error("pinned store missing from registry after the concurrent stress run")
	}
	if got := pinnedStore.RefCount(); got != 1 {
		t.Errorf("pinned store RefCount() = %d after stress run, want exactly 1 (never released, never double-acquired)", got)
	}

	// Sanity: the churn repo's store must never have gone negative — a
	// negative refcount would indicate a double-release bug (e.g. Close()
	// called twice for one WorktreeStorer without closeOnce protecting it).
	churnCommonDirAbs := canonicalizeDir(filepath.Join(churnRepo, ".git"))
	if churnStore, ok := reg.stores[churnCommonDirAbs]; ok {
		if rc := churnStore.RefCount(); rc < 0 {
			t.Errorf("churn store RefCount() = %d, must never go negative", rc)
		}
	}

	// Release the pinned reference and confirm it becomes evictable exactly
	// like any other zero-refcount store, closing the loop on the property
	// under test: pinned while in use, evictable once released.
	ws, ok := pinnedRepoHandle.Storer.(*WorktreeStorer)
	if !ok {
		t.Fatalf("pinnedRepoHandle.Storer is %T, want *WorktreeStorer", pinnedRepoHandle.Storer)
	}
	if err := ws.Close(); err != nil {
		t.Fatalf("Close(pinned): %v", err)
	}
	atomic.StoreInt64(&pinnedStore.unusedSinceNs, time.Now().Add(-registryStoreTTL-time.Minute).UnixNano())
	reg.Prune()
	if _, ok := reg.stores[pinnedCommonDirAbs]; ok {
		t.Error("pinned store should have become evictable once released and past TTL")
	}
}

// --- (item 3, lower priority) per-worktree pack handle caching ------------

// countingFile wraps a billy.File, tracking whether Close has been called.
type countingFile struct {
	billy.File
	name string
	mu   sync.Mutex
	shut bool
}

func (f *countingFile) Close() error {
	f.mu.Lock()
	f.shut = true
	f.mu.Unlock()
	return f.File.Close()
}

// countingFs wraps a billy.Filesystem, counting Open calls per filename and
// tracking every returned *countingFile so a test can later assert they were
// closed. Only Open is overridden — every other billy.Filesystem method is
// promoted straight through to the embedded Filesystem.
type countingFs struct {
	billy.Filesystem
	mu     sync.Mutex
	opened map[string]int
	files  []*countingFile
}

func (c *countingFs) Open(filename string) (billy.File, error) {
	f, err := c.Filesystem.Open(filename)
	if err != nil {
		return nil, err
	}
	cf := &countingFile{File: f, name: filename}
	c.mu.Lock()
	if c.opened == nil {
		c.opened = make(map[string]int)
	}
	c.opened[filename]++
	c.files = append(c.files, cf)
	c.mu.Unlock()
	return cf, nil
}

func (c *countingFs) opensWithSuffix(suffix string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for name, count := range c.opened {
		if strings.HasSuffix(name, suffix) {
			n += count
		}
	}
	return n
}

func (c *countingFs) allClosedWithSuffix(suffix string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	found := false
	for _, f := range c.files {
		if !strings.HasSuffix(f.name, suffix) {
			continue
		}
		found = true
		f.mu.Lock()
		shut := f.shut
		f.mu.Unlock()
		if !shut {
			return false
		}
	}
	return found // must have seen at least one matching file to mean anything
}

// TestPackHandleCache_ReusesOpenHandleAcrossReads_AndClosesOnRelease proves
// the stage-1 pack-handle-caching follow-up (design doc §4.1/§6): reading
// several distinct objects from the SAME pack through one WorktreeStorer's
// pack handle cache opens the underlying pack file exactly once (not once
// per object read), and WorktreeStorer.Close() closes that cached handle so
// it doesn't leak a file descriptor.
//
// This constructs a *WorktreeStorer directly (rather than via the public
// Open, which builds its own unwrapped filesystem internally) so an
// instrumented countingFs can be threaded in — only the EncodedObject path
// is exercised here (WorktreeStorer.Close and EncodedObject don't touch the
// embedded *filesystem.Storage), so leaving it nil is deliberate, not an
// oversight.
func TestPackHandleCache_ReusesOpenHandleAcrossReads_AndClosesOnRelease(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	mainRepo := filepath.Join(dir, "main")
	buildPackedFixture(t, mainRepo, 80)

	_, commonFs, _, commonDirAbs, err := resolveGitFilesystems(mainRepo)
	if err != nil {
		t.Fatalf("resolveGitFilesystems: %v", err)
	}

	counting := &countingFs{Filesystem: commonFs}
	// A tiny (1-byte) decoded-object cache means essentially every Put is
	// immediately evicted, forcing repeated getFromPackfile calls instead of
	// short-circuiting via cache.Object hits — this test wants to exercise
	// the pack-handle path specifically, not the (already well-tested)
	// decoded-object cache.
	store := newSharedObjectStore(commonDirAbs, counting, cache.NewObjectLRU(cache.FileSize(1)), 0, false)

	hashes := firstPackedHashes(t, store, 25)
	if len(hashes) == 0 {
		t.Fatal("fixture produced no packed objects to read — buildPackedFixture may need more commits")
	}

	ws := &WorktreeStorer{shared: store}
	for _, h := range hashes {
		if _, oerr := ws.EncodedObject(plumbing.AnyObject, h); oerr != nil {
			t.Fatalf("EncodedObject(%s): %v", h, oerr)
		}
	}

	if opens := counting.opensWithSuffix(".pack"); opens != 1 {
		t.Errorf("pack file opened %d times across %d object reads, want exactly 1 (handle should be cached and reused within one WorktreeStorer)", opens, len(hashes))
	}

	if err := ws.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !counting.allClosedWithSuffix(".pack") {
		t.Error("pack file handle was not closed by WorktreeStorer.Close — file descriptor leak")
	}
}

// TestPackHandleCache_DifferentWorktrees_DoNotShareOneHandle proves the
// per-worktree scoping is real: two WorktreeStorers (i.e. two separate
// packHandleCache instances) backed by the SAME SharedObjectStore each open
// their own handle to the same pack — caching must not regress into
// re-introducing the cross-worktree handle-sharing race the design doc's
// §4.1 explicitly rejects.
func TestPackHandleCache_DifferentWorktrees_DoNotShareOneHandle(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	dir := t.TempDir()
	mainRepo := filepath.Join(dir, "main")
	buildPackedFixture(t, mainRepo, 80)

	_, commonFs, _, commonDirAbs, err := resolveGitFilesystems(mainRepo)
	if err != nil {
		t.Fatalf("resolveGitFilesystems: %v", err)
	}
	counting := &countingFs{Filesystem: commonFs}
	store := newSharedObjectStore(commonDirAbs, counting, cache.NewObjectLRU(cache.FileSize(1)), 0, false)

	hashes := firstPackedHashes(t, store, 1)
	if len(hashes) == 0 {
		t.Fatal("fixture produced no packed objects to read")
	}
	h := hashes[0]

	wsA := &WorktreeStorer{shared: store}
	wsB := &WorktreeStorer{shared: store}
	if _, err := wsA.EncodedObject(plumbing.AnyObject, h); err != nil {
		t.Fatalf("EncodedObject via A: %v", err)
	}
	if _, err := wsB.EncodedObject(plumbing.AnyObject, h); err != nil {
		t.Fatalf("EncodedObject via B: %v", err)
	}

	if opens := counting.opensWithSuffix(".pack"); opens != 2 {
		t.Errorf("pack file opened %d times across 2 independent worktree handle caches, want exactly 2 (one per worktree — no cross-worktree sharing)", opens)
	}
}

// firstPackedHashes returns up to n object hashes known to live in store's
// packfile(s) (via its already-parsed idxfile.Index), for tests that need
// guaranteed-packed hashes without depending on which objects git's own gc
// heuristics happened to leave loose.
func firstPackedHashes(t *testing.T, store *SharedObjectStore, n int) []plumbing.Hash {
	t.Helper()
	if err := store.ensureIndex(); err != nil {
		t.Fatalf("ensureIndex: %v", err)
	}
	var hashes []plumbing.Hash
	for _, idx := range store.index {
		it, err := idx.Entries()
		if err != nil {
			t.Fatalf("Entries: %v", err)
		}
		for len(hashes) < n {
			e, nerr := it.Next()
			if nerr != nil {
				break
			}
			hashes = append(hashes, e.Hash)
		}
		_ = it.Close()
		break // buildPackedFixture's `git gc --aggressive` produces one pack
	}
	return hashes
}
