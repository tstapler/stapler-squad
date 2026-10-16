package gogitstore

import (
	"go/build"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
)

// atomicExt is a contract-abiding Extension: Close and Bytes touch only atomics.
type atomicExt struct {
	bytes  atomic.Int64
	closed atomic.Int32
}

func (e *atomicExt) Bytes() int64 { return e.bytes.Load() }
func (e *atomicExt) Close()       { e.closed.Add(1) }

// idleStore registers a zero-refcount store whose idle timestamp is ageNs old.
func idleStore(t *testing.T, reg *Registry, key string, age time.Duration) *SharedObjectStore {
	t.Helper()
	s := reg.acquire(key, osfs.New(t.TempDir()))
	s.release()
	atomic.StoreInt64(&s.unusedSinceNs, time.Now().Add(-age).UnixNano())
	return s
}

// completesWithin runs fn and reports whether it returned before d. It never
// sleeps: it selects on completion vs a timer.
func completesWithin(d time.Duration, fn func()) (finished <-chan struct{}, ok bool) {
	done := make(chan struct{})
	go func() { defer close(done); fn() }()
	select {
	case <-done:
		return done, true
	case <-time.After(d):
		return done, false
	}
}

func TestExtension_ShouldRunNewFnOnce_WhenEightGoroutinesCall(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	s := reg.acquire("k", osfs.New(t.TempDir()))
	var calls atomic.Int32
	newFn := func() Extension { calls.Add(1); return &atomicExt{} }

	var wg sync.WaitGroup
	got := make([]Extension, 8)
	start := make(chan struct{})
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got[i] = s.Extension(newFn)
		}()
	}
	close(start)
	wg.Wait()

	if calls.Load() != 1 {
		t.Fatalf("newFn ran %d times, want 1", calls.Load())
	}
	for i, e := range got {
		if e != got[0] {
			t.Fatalf("goroutine %d got a different instance", i)
		}
	}
}

func TestExtension_ShouldNotAllocate_WhenSlotPopulated(t *testing.T) {
	reg := NewRegistry()
	s := reg.acquire("k", osfs.New(t.TempDir()))
	e := &atomicExt{}
	newFn := func() Extension { return e }
	s.Extension(newFn)
	if allocs := testing.AllocsPerRun(100, func() { _ = s.Extension(newFn) }); allocs != 0 {
		t.Errorf("cache hit allocates %v per call, want 0", allocs)
	}
}

func TestExtension_ShouldCloseOnceAndCountBytes_WhenStorePruned(t *testing.T) {
	withRegistryHeapPressure(t, 0)
	reg := &Registry{CacheMaxSize: 1}
	s := idleStore(t, reg, "k", 2*registryStoreTTL)
	e := &atomicExt{}
	e.bytes.Store(1000)
	s.Extension(func() Extension { return e })

	reg.mu.Lock()
	got := reg.approxBytesPerStore()
	reg.mu.Unlock()
	if got != 1001 {
		t.Errorf("approxBytesPerStore = %d, want CacheMaxSize(1) + Bytes(1000)", got)
	}

	reg.Prune()
	reg.Prune()
	if n := e.closed.Load(); n != 1 {
		t.Errorf("Close called %d times, want exactly 1", n)
	}
	if len(reg.RefCounts()) != 0 {
		t.Error("store not evicted")
	}
	if s.Extension(func() Extension { return &atomicExt{} }) != nil {
		t.Error("Extension after eviction must return nil, not resurrect a slot")
	}
}

func TestExtension_ShouldCloseOnce_WhenLRUTrimmed(t *testing.T) {
	withRegistryHeapPressure(t, 0)
	// 1 GiB per store against the 1.5 GiB default budget: max one entry.
	reg := &Registry{CacheMaxSize: 1 << 30}
	oldest := idleStore(t, reg, "oldest", 3*time.Minute)
	newest := idleStore(t, reg, "newest", time.Minute)
	eOld, eNew := &atomicExt{}, &atomicExt{}
	oldest.Extension(func() Extension { return eOld })
	newest.Extension(func() Extension { return eNew })

	reg.Prune()
	if eOld.closed.Load() != 1 || eNew.closed.Load() != 0 {
		t.Errorf("closed old=%d new=%d, want 1 and 0", eOld.closed.Load(), eNew.closed.Load())
	}
	if _, ok := reg.RefCounts()["newest"]; !ok {
		t.Error("newest store was trimmed")
	}
}

func TestExtensionsDo_ShouldNotHoldRegistryLockDuringCallback(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	want := make([]Extension, 0, 3)
	for _, k := range []string{"a", "b", "c"} {
		e := &atomicExt{}
		reg.acquire(k, osfs.New(t.TempDir())).Extension(func() Extension { return e })
		want = append(want, e)
	}
	reg.acquire("none", osfs.New(t.TempDir())) // no extension: skipped

	visited := map[Extension]bool{}
	_, ok := completesWithin(10*time.Second, func() {
		reg.ExtensionsDo(func(e Extension) {
			_ = reg.RefCounts() // would deadlock if r.mu were held
			visited[e] = true
		})
	})
	if !ok {
		t.Fatal("ExtensionsDo callback re-entering the registry did not return")
	}
	if len(visited) != len(want) {
		t.Errorf("visited %d extensions, want %d", len(visited), len(want))
	}
	for _, e := range want {
		if !visited[e] {
			t.Errorf("extension %p not visited", e)
		}
	}
}

// reentrantExt violates the callback contract: Close calls back into the
// registry, which blocks on the Registry.mu already held by pruneLocked.
// unblock lets the harness release the deadlock so nothing leaks.
type reentrantExt struct {
	reg     *Registry
	unblock chan struct{}
}

func (e *reentrantExt) Bytes() int64 { return 0 }
func (e *reentrantExt) Close() {
	reentered := make(chan struct{})
	go func() { _ = e.reg.RefCounts(); close(reentered) }()
	select {
	case <-reentered:
	case <-e.unblock:
	}
}

func TestExtensionClose_ShouldBeDetected_WhenCloseCallsBackIntoRegistry(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	s := idleStore(t, reg, "k", 2*registryStoreTTL)
	ext := &reentrantExt{reg: reg, unblock: make(chan struct{})}
	s.Extension(func() Extension { return ext })

	finished, ok := completesWithin(300*time.Millisecond, reg.Prune)
	if ok {
		t.Fatal("harness failed to detect a Close that re-enters Registry.RefCounts")
	}
	close(ext.unblock)
	<-finished
}

func TestExtensionClose_ShouldPassDeadline_WhenExtensionIsBenign(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	s := idleStore(t, reg, "k", 2*registryStoreTTL)
	e := &atomicExt{}
	s.Extension(func() Extension { return e })
	if _, ok := completesWithin(10*time.Second, reg.Prune); !ok {
		t.Fatal("benign Close did not return before the deadline")
	}
	if e.closed.Load() != 1 {
		t.Errorf("Close called %d times, want 1", e.closed.Load())
	}
}

// Walks imports in-process (build.ImportDir reads directories; no `go list`
// subprocess), following every in-module package gogitstore reaches.
func TestGogitstoreImports_ShouldExcludeRepostateAndUnfinished_WhenWalkedWithGoBuild(t *testing.T) {
	t.Parallel()
	root, module := findModule(t)
	banned := map[string]bool{
		module + "/session/unfinished":           true,
		module + "/session/unfinished/repostate": true,
	}
	seen := map[string]bool{}
	var visit func(pkgPath string)
	visit = func(pkgPath string) {
		if seen[pkgPath] {
			return
		}
		seen[pkgPath] = true
		dir := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(pkgPath, module+"/")))
		pkg, err := build.ImportDir(dir, 0)
		if err != nil {
			t.Errorf("ImportDir %s: %v", pkgPath, err)
			return
		}
		for _, imp := range pkg.Imports {
			if banned[imp] {
				t.Errorf("%s imports %s; the extension slot must stay opaque", pkgPath, imp)
			}
			if strings.HasPrefix(imp, module+"/") {
				visit(imp)
			}
		}
	}
	visit(module + "/session/unfinished/gogitstore")
	if len(seen) < 2 {
		t.Fatalf("walk visited %d packages, want gogitstore plus its in-module deps", len(seen))
	}
}

func findModule(t *testing.T) (root, module string) {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if rest, ok := strings.CutPrefix(line, "module "); ok {
					return dir, strings.TrimSpace(rest)
				}
			}
			t.Fatal("go.mod has no module line")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
