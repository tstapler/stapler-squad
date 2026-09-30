package session

import (
	"context"
	"math/rand"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tstapler/stapler-squad/log"
)

// changeDetectionStatWalkInterval is how often WorktreeChangeDetector re-checks
// a worktree's dirty/HEAD fingerprint. Matches today's pre-pivot
// diffStatsCacheTTL/vcsStatusCacheTTL (15s) deliberately -- this is the same
// cadence the caches already recomputed on unconditionally, just now gating
// an edge-triggered invalidation instead of a blind recompute.
const changeDetectionStatWalkInterval = 15 * time.Second

// fingerprintFunc returns the current dirty/HEAD state for one worktree.
// Injected rather than taking a *git.GitWorktree directly so
// WorktreeChangeDetector can be unit-tested without a real git repo.
type fingerprintFunc func() (dirty bool, headSHA string, err error)

// WorktreeChangeDetector watches one worktree's .git dir via fsnotify (cheap,
// ~10-20 descriptors, mirrors session/unfinished/watcher.go's proven
// .git-only pattern) and runs a jittered-start periodic fingerprint
// comparison as the primary signal for plain working-tree edits that never
// touch .git. See project_plans/diff-stats-file-watch-cache/decisions/
// ADR-029-... for why this replaces full recursive per-directory watching.
type WorktreeChangeDetector struct {
	worktreePath string
	fingerprint  fingerprintFunc
	activeFunc   func() bool // nil means "always active"; see Story 2.1.2

	// statWalkInterval defaults to changeDetectionStatWalkInterval in the
	// constructor; overridable only from same-package tests via
	// setStatWalkInterval so they run in milliseconds instead of waiting out
	// a real 15s tick (deterministic-fast-tests convention).
	statWalkInterval time.Duration

	mu       sync.Mutex
	onChange []func()

	gitWatcher *fsnotify.Watcher // nil if unavailable or Add() failed
	cancel     context.CancelFunc
	stopped    chan struct{} // closed when both goroutines have exited (or immediately if neither started)
}

// NewWorktreeChangeDetector never errors -- matches
// session/unfinished/watcher.go's NewWatchDirWatcher nil-on-unavailable
// convention for the .git sub-watch; the periodic loop always starts.
// activeFunc, if non-nil, is checked at the top of every periodic tick
// (before fingerprint()) -- returning false skips that tick's stat-walk
// entirely, so a worktree nobody has asked GetSessionDiff/GetVCSStatus for
// recently doesn't pay IsDirtyUncached's O(tracked-files) cost. See Story
// 2.1.2 (this codebase's fix for pre-mortem.md P1 item #3).
func NewWorktreeChangeDetector(worktreePath string, fingerprint fingerprintFunc, activeFunc func() bool) *WorktreeChangeDetector {
	return &WorktreeChangeDetector{
		worktreePath:     worktreePath,
		fingerprint:      fingerprint,
		activeFunc:       activeFunc,
		statWalkInterval: changeDetectionStatWalkInterval,
		stopped:          make(chan struct{}),
	}
}

// setStatWalkInterval overrides the periodic-tick interval. Test-only (same
// package), used to make Start()/Stop() unit tests deterministic and fast
// instead of waiting out a real 15s tick.
func (d *WorktreeChangeDetector) setStatWalkInterval(dur time.Duration) {
	d.statWalkInterval = dur
}

// OnChange registers an invalidation callback. Must be called before Start().
func (d *WorktreeChangeDetector) OnChange(fn func()) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onChange = append(d.onChange, fn)
}

// GitWatchActive reports whether the .git fsnotify watch is running.
// Observability-only -- does not gate TTL widening (the periodic loop is
// the primary, always-on signal post-pivot).
func (d *WorktreeChangeDetector) GitWatchActive() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.gitWatcher != nil
}

// fire invokes every registered OnChange callback, recover()-guarding each
// one individually -- matching this codebase's own repeated convention for
// cross-boundary callback dispatch (session/ent_repository_backlog.go's
// publishItemChanged, session/chain_firer.go:160, session/backlog_lifecycle.go's
// runStuckDetector, session/autonomous_driver.go:285). This detector invokes
// multiple independently-registered callbacks (GitWorktreeManager's own
// diff-stats-clearing callback, plus WorkspaceService's lazily-registered
// vcsStatusCache.Delete closure per Story 2.3.2) -- an unrecovered panic in
// any one of them would otherwise crash the whole process (Go terminates the
// program on an unrecovered panic in any goroutine), a disproportionate
// failure mode for a default-off, best-effort background caching optimization.
func (d *WorktreeChangeDetector) fire() {
	d.mu.Lock()
	callbacks := d.onChange
	d.mu.Unlock()
	for _, fn := range callbacks {
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					log.Warn("worktree change detector: OnChange callback panicked (recovered)", "worktreePath", d.worktreePath, "panic", rec)
				}
			}()
			fn()
		}()
	}
}

// Start takes a baseline fingerprint, attempts the .git fsnotify watch, and
// launches the periodic-loop goroutine (plus the fsnotify event loop, if the
// .git watch succeeded). Never blocks past the baseline fingerprint read.
func (d *WorktreeChangeDetector) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel

	lastDirty, lastHead, err := d.fingerprint()
	if err != nil {
		log.Debug("worktree change detector: initial fingerprint read failed", "worktreePath", d.worktreePath, "err", err)
	}

	watcher, err := fsnotify.NewWatcher()
	gitWatchStarted := false
	if err != nil {
		log.Warn("worktree change detector: fsnotify unavailable, falling back to periodic-check-only for this worktree", "worktreePath", d.worktreePath, "err", err)
	} else if addErr := watcher.Add(filepath.Join(d.worktreePath, ".git")); addErr != nil {
		log.Warn("worktree change detector: .git fsnotify watch failed to start, falling back to periodic-check-only for this worktree", "worktreePath", d.worktreePath, "err", addErr)
		_ = watcher.Close()
	} else {
		d.mu.Lock()
		d.gitWatcher = watcher
		d.mu.Unlock()
		gitWatchStarted = true
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go d.periodicLoop(ctx, &wg, lastDirty, lastHead)
	if gitWatchStarted {
		wg.Add(1)
		go d.gitWatchLoop(ctx, &wg, watcher)
	}
	go func() {
		wg.Wait()
		close(d.stopped)
	}()
}

func (d *WorktreeChangeDetector) periodicLoop(ctx context.Context, wg *sync.WaitGroup, lastDirty bool, lastHead string) {
	defer wg.Done()

	// Jitter the first tick so 134 worktrees whose Setup() calls land in the
	// same burst (e.g. process restart) don't all stat-walk on the same
	// wall-clock tick -- mirrors GitWorktreeManager.PrimeDirtyCacheJitter's
	// existing rand.Int63n staggering idiom.
	initialDelay := time.Duration(rand.Int63n(int64(d.statWalkInterval)))
	timer := time.NewTimer(initialDelay)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if d.activeFunc != nil && !d.activeFunc() {
				timer.Reset(d.statWalkInterval)
				continue
			}
			dirty, head, err := d.fingerprint()
			if err != nil {
				log.Debug("worktree change detector: fingerprint read failed, skipping tick", "worktreePath", d.worktreePath, "err", err)
			} else if dirty != lastDirty || head != lastHead {
				log.Debug("worktree change detector: fingerprint changed, invalidating caches", "worktreePath", d.worktreePath, "dirtyChanged", dirty != lastDirty, "headChanged", head != lastHead)
				lastDirty, lastHead = dirty, head
				d.fire()
			}
			timer.Reset(d.statWalkInterval)
		}
	}
}

// gitWatchLoop mirrors session/unfinished/watcher.go's fsnotifyLoop shape.
// No debounce: each fire() call is a cheap, idempotent invalidate, so a
// burst of raw fsnotify events firing repeatedly costs nothing.
func (d *WorktreeChangeDetector) gitWatchLoop(ctx context.Context, wg *sync.WaitGroup, watcher *fsnotify.Watcher) {
	defer wg.Done()
	defer func() { _ = watcher.Close() }()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
				d.fire()
			}
			// Deliberately do not call watcher.Remove() here even for a
			// Remove/Rename of the watched .git dir itself -- per
			// fsnotify#40, the OS has already dropped the watch; calling
			// Remove() on an already-gone path is at best a no-op and at
			// worst risks the PR#73 deadlock. Stop() (context cancellation)
			// is the only teardown path for this watcher.
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
			// Errors channel surfaces ErrEventOverflow -- advisory only per
			// pitfalls.md; the periodic loop remains authoritative.
		}
	}
}

// Stop cancels both goroutines and blocks until they have exited. Safe to
// call multiple times or on a detector that failed to start any goroutine.
func (d *WorktreeChangeDetector) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
	<-d.stopped
}
