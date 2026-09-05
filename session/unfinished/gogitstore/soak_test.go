// soak_test.go is a soak/longevity test for the eviction+refcount+retire
// lifecycle under sustained, production-shaped load — this task's item 4.
// It simulates several repos with several worktrees each, repeated
// opens/closes, periodic real repacks, and Registry.Prune() running on its
// normal cadence, all concurrently, for a sustained wall-clock period —
// then confirms:
//
//   - no goroutine leak (runtime.NumGoroutine() returns to baseline once
//     everything is closed/evicted — accounting for the fact that a live
//     mmap-enabled commondir keeps its fsnotify watcher goroutine running
//     for as long as its SharedObjectStore exists, which is expected and by
//     design; only after full eviction should that goroutine be gone too)
//   - no file descriptor leak (/proc/self/fd count returns to baseline)
//   - eviction genuinely reclaims memory, not merely "doesn't crash" —
//     forces a full TTL-based eviction pass at the end (the same
//     unusedSinceNs-manipulation technique registry_eviction_test.go's
//     other tests use, rather than waiting 30 real minutes) and asserts
//     zero SharedObjectStores remain.
//
// Duration here is intentionally modest for a `go test -bench` run (~20s of
// sustained concurrent load) so this stays practical to run repeatedly —
// see this task's report for a longer (~2 minute), manually-run variant of
// the same scenario and whether its results changed the conclusion.
//
// This is a Benchmark, not a Test: go test never runs Benchmarks unless
// invoked with -bench, which keeps this out of `make test`/`make
// test-race`/`make test-integration` (and CI) structurally — no flag
// (-short or otherwise) needs to be remembered at every call site. Run it
// explicitly via `make benchmark-soak` or
// `go test -bench BenchmarkGogitstoreSoakUnderSustainedLoad -benchtime=1x
// -run '^$' ./session/unfinished/gogitstore/`.
package gogitstore

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"go.uber.org/goleak"
)

// soakWaitTimeout bounds how long the soak test waits on any single
// sync.WaitGroup before giving up and failing with a full goroutine dump,
// instead of relying on Go's 600s per-package test timeout (which kills the
// whole test binary with only a partial/interleaved stack trace, and no
// signal about which of the three WaitGroups was actually stuck). Set well
// above soakDuration (so normal teardown never trips it) but far below 600s,
// so a real hang is caught with time to spare and with output attributable
// to a specific stage.
const soakWaitTimeout = 45 * time.Second

// waitBounded waits for wg with a soakWaitTimeout bound. On timeout, it
// dumps every goroutine's complete stack (via pprof.Lookup("goroutine"),
// which — unlike runtime.Stack — is not silently truncated by a fixed
// buffer size) to t's log and fails the test, naming which wait stage
// timed out so the dump can be correlated with what the test was doing.
func waitBounded(tb testing.TB, stage string, wg *sync.WaitGroup) {
	tb.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(soakWaitTimeout):
		var buf strings.Builder
		_ = pprof.Lookup("goroutine").WriteTo(&buf, 2)
		tb.Fatalf("soak test hung waiting on %s (exceeded %s) — full goroutine dump follows:\n%s", stage, soakWaitTimeout, buf.String())
	}
}

// countOpenFDs returns this process's current open file descriptor count via
// /proc/self/fd. Linux-specific (this project's documented deployment
// targets are macOS + Linux systemd per CLAUDE.md; this sandbox is Linux).
// Returns ok=false if /proc/self/fd is unavailable, so callers on an
// unsupported platform skip the FD-count assertion rather than failing on
// something this helper cannot measure there.
func countOpenFDs() (n int, ok bool) {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return 0, false
	}
	return len(entries), true
}

func BenchmarkGogitstoreSoakUnderSustainedLoad(b *testing.B) {
	if _, err := exec.LookPath("git"); err != nil {
		b.Skip("git binary not available")
	}

	const numRepos = 3
	const worktreesPerRepo = 3
	const soakDuration = 20 * time.Second
	const numWorkers = 8

	root := b.TempDir()
	type repoInfo struct {
		mainDir   string
		worktrees []string
	}
	repos := make([]repoInfo, numRepos)
	for i := range repos {
		mainDir := filepath.Join(root, fmt.Sprintf("repo%d", i))
		buildPackedFixture(b, mainDir, 25)
		wts := []string{mainDir}
		for w := 0; w < worktreesPerRepo; w++ {
			wtDir := filepath.Join(root, fmt.Sprintf("repo%d-wt%d", i, w))
			addWorktree(b, mainDir, wtDir, fmt.Sprintf("wt-%d-%d", i, w))
			wts = append(wts, wtDir)
		}
		repos[i] = repoInfo{mainDir: mainDir, worktrees: wts}
	}

	// Small CacheMaxSize maximizes cache-eviction pressure per store (not
	// the same thing as store eviction, but exercises the decoded-object
	// cache's own bounded behavior under sustained reads); UseMmapIndex
	// true exercises the newest, highest-risk code path (mmapindex.go +
	// mmapwatch.go) under the sustained load, not just the simpler
	// copy-based path.
	reg := &Registry{UseMmapIndex: true, CacheMaxSize: 256 * 1024}

	// goroutineBaseline snapshots the pre-benchmark goroutine set via
	// goleak.IgnoreCurrent() rather than asserting a bare process-wide
	// goleak.VerifyNone() — this benchmark runs inside the full `go test
	// -bench` binary alongside every other package's own background
	// goroutines (see session/actor_test.go's TestActorNoLeak for the same
	// pattern).
	goroutineBaseline := goleak.IgnoreCurrent()

	runtime.GC()
	var baselineMem runtime.MemStats
	runtime.ReadMemStats(&baselineMem)
	baselineFDs, fdSupported := countOpenFDs()

	var opsDone atomic.Int64
	// opErrorsUnexpected counts any error OTHER than
	// plumbing.ErrObjectNotFound — a genuinely unexpected failure this test
	// should fail on. opErrorsStaleIndex counts plumbing.ErrObjectNotFound
	// specifically: EXPECTED, tolerated noise under this test's
	// intentionally adversarial concurrent-repack cadence (a repack roughly
	// once per second per repo — far more frequent than any realistic
	// production repack schedule) — s.index can legitimately name a pack a
	// concurrent repack has since unlinked, and the store's own staleness
	// recovery (mmapwatch.go's fsnotify watcher in mmap mode; Registry.Prune's
	// coarser TTL-driven recreation in copy-based mode) has a real, nonzero
	// window before it catches up. See store.go's dirObjectPack, which this
	// soak test's findings led to being hardened to return
	// plumbing.ErrObjectNotFound (rather than the unwrapped
	// dotgit.ErrPackfileNotFound go-git itself would otherwise surface) for
	// exactly this race, so existing plumbing.ErrObjectNotFound-tolerant
	// callers elsewhere in this codebase (e.g. gogit_vcs_reader.go's
	// findMergeBase) can actually catch it.
	var opErrorsStaleIndex atomic.Int64
	var opErrorsUnexpected atomic.Int64
	var repackCount atomic.Int64
	var repackErrors atomic.Int64
	var errSamplesMu sync.Mutex
	errSamples := make(map[string]int)
	recordErr := func(err error) {
		if errors.Is(err, plumbing.ErrObjectNotFound) {
			opErrorsStaleIndex.Add(1)
			return
		}
		opErrorsUnexpected.Add(1)
		errSamplesMu.Lock()
		defer errSamplesMu.Unlock()
		errSamples[err.Error()]++
	}

	stop := make(chan struct{})
	deadline := time.Now().Add(soakDuration)

	// Worker goroutines: repeatedly open/read/close a worktree, cycling
	// through every repo x worktree combination — the realistic "many
	// worktrees across several repos, repeated opens/closes" shape this
	// task's item 4 describes. Never call testing.TB reporting methods from
	// here (only the goroutine running the benchmark function may — see
	// mmap_stage2_test.go's TestMmapIndex_PinnedReadersSurviveConcurrentRealRepack
	// doc comment for the same constraint) — errors are recorded via
	// atomics and reported from the main goroutine after joining.
	var workerWG sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		workerWG.Add(1)
		go func(seed int) {
			defer workerWG.Done()
			i := 0
			for time.Now().Before(deadline) {
				repo := repos[(seed+i)%len(repos)]
				wt := repo.worktrees[(seed+i)%len(repo.worktrees)]
				i++

				gr, err := Open(wt, reg)
				if err != nil {
					recordErr(err)
					continue
				}
				head, herr := gr.Head()
				if herr != nil {
					recordErr(herr)
				} else if _, cerr := gr.CommitObject(head.Hash()); cerr != nil {
					recordErr(cerr)
				}
				if closer, ok := gr.Storer.(interface{ Close() error }); ok {
					_ = closer.Close()
				}
				opsDone.Add(1)
			}
		}(w)
	}

	// Repacker goroutine: periodically commits + real `git gc --aggressive`
	// repacks, cycling through every repo, exercising refreshIndexes/retire
	// under real concurrent load throughout the soak — not just once, the
	// way most of this package's other tests do. Uses runGitCmd/
	// commitFileNoT (mmap_stage2_test.go), the goroutine-safe helpers that
	// return errors instead of calling *testing.T methods.
	var repackWG sync.WaitGroup
	repackWG.Add(1)
	go func() {
		defer repackWG.Done()
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
			}
			repo := repos[i%len(repos)]
			i++
			marker := "soak-marker-" + strconv.Itoa(i) + ".txt"
			if err := commitFileNoT(repo.mainDir, marker, "soak\n", "soak commit "+strconv.Itoa(i)); err != nil {
				repackErrors.Add(1)
			} else if err := runGitCmd(repo.mainDir, "gc", "-q", "--aggressive"); err != nil {
				repackErrors.Add(1)
			} else {
				repackCount.Add(1)
			}
			time.Sleep(300 * time.Millisecond)
		}
	}()

	// Pruner goroutine: mimics Scanner's periodic PruneToMemoryBudget ticker
	// (production runs this every minute; sped up here to actually exercise
	// Prune repeatedly within this test's much shorter duration).
	var prunerWG sync.WaitGroup
	prunerWG.Add(1)
	go func() {
		defer prunerWG.Done()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				reg.Prune()
			}
		}
	}()

	waitBounded(b, "workerWG (soak workers)", &workerWG)
	close(stop)
	waitBounded(b, "repackWG (repacker goroutine)", &repackWG)
	waitBounded(b, "prunerWG (pruner goroutine)", &prunerWG)

	b.Logf("soak summary: opsDone=%d opErrorsStaleIndex(tolerated)=%d opErrorsUnexpected=%d repackCount=%d repackErrors=%d",
		opsDone.Load(), opErrorsStaleIndex.Load(), opErrorsUnexpected.Load(), repackCount.Load(), repackErrors.Load())
	errSamplesMu.Lock()
	for msg, count := range errSamples {
		b.Logf("UNEXPECTED error sample (x%d): %s", count, msg)
	}
	errSamplesMu.Unlock()
	if opsDone.Load() == 0 {
		b.Fatal("soak loop completed zero operations — benchmark isn't exercising anything")
	}
	if opErrorsUnexpected.Load() > 0 {
		b.Errorf("%d worker operations failed with an UNEXPECTED error type during sustained load (see error samples above) — plumbing.ErrObjectNotFound alone is tolerated as an expected consequence of this benchmark's intentionally adversarial concurrent-repack cadence; anything else is a real regression to investigate", opErrorsUnexpected.Load())
	}
	if repackErrors.Load() > 0 {
		b.Errorf("%d repack-goroutine operations failed during the soak", repackErrors.Load())
	}

	// Force full eviction: manipulate every remaining zero-refcount store's
	// unusedSinceNs to simulate having gone idle past registryStoreTTL,
	// mirroring registry_eviction_test.go's own technique, rather than
	// waiting 30 real minutes — then Prune once more. This is what actually
	// answers "does eviction reclaim resources", not just "did the soak
	// avoid crashing."
	reg.mu.Lock()
	for _, s := range reg.stores {
		if s.RefCount() == 0 {
			atomic.StoreInt64(&s.unusedSinceNs, time.Now().Add(-registryStoreTTL-time.Minute).UnixNano())
		}
	}
	reg.mu.Unlock()
	reg.Prune()

	reg.mu.Lock()
	remaining := len(reg.stores)
	reg.mu.Unlock()
	if remaining != 0 {
		b.Errorf("expected zero SharedObjectStores remaining after forced full TTL eviction, got %d — eviction is not actually reclaiming stores", remaining)
	}

	// Poll goleak.Find (rather than a bare single call, or goleak.VerifyNone
	// — which would b.Fatal and skip the FD/mem checks below) to give
	// just-stopped watcher goroutines (stopPackWatch closes their stop
	// channel, but goroutine scheduling isn't instantaneous) time to actually
	// exit before this fails.
	grDeadline := time.Now().Add(5 * time.Second)
	var leakErr error
	for {
		if leakErr = goleak.Find(goroutineBaseline); leakErr == nil || time.Now().After(grDeadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if leakErr != nil {
		b.Errorf("goroutine leak after full eviction: %v", leakErr)
	}

	if fdSupported {
		afterFDs, _ := countOpenFDs()
		b.Logf("open FDs: baseline=%d after-soak-and-full-eviction=%d", baselineFDs, afterFDs)
		if afterFDs > baselineFDs+5 { // small slack for test-harness-owned fds unrelated to this package
			b.Errorf("file descriptor count did not return to baseline after full eviction: baseline=%d after=%d (possible fd leak of ~%d)", baselineFDs, afterFDs, afterFDs-baselineFDs)
		}
	} else {
		b.Log("skipping FD-count assertion: /proc/self/fd unavailable on this platform")
	}

	runtime.GC()
	var afterMem runtime.MemStats
	runtime.ReadMemStats(&afterMem)
	b.Logf("heap: baseline HeapAlloc=%d bytes, after-soak-and-full-eviction HeapAlloc=%d bytes (delta=%d)",
		baselineMem.HeapAlloc, afterMem.HeapAlloc, int64(afterMem.HeapAlloc)-int64(baselineMem.HeapAlloc))
}
