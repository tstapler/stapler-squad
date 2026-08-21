package tmux

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/gofrs/flock"
	appconfig "github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
)

// execGateAcquireBackoffMax caps the retry backoff while waiting for a slot.
const execGateAcquireBackoffMax = 100 * time.Millisecond

// execGateAcquireBackoffStart is the initial retry backoff.
const execGateAcquireBackoffStart = 5 * time.Millisecond

// nonAlnum matches runs of characters unsafe for use in a filesystem path
// segment, for deriving a lock-directory key from a tmux server socket name.
var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

// AcquireExecSlot blocks until a tmux-subprocess execution slot is free —
// across this and every other process touching the same tmux server — or ctx
// is done. release must be called exactly once, after the subprocess this
// slot guards has fully exited (after Output()/Run()/CombinedOutput()/Wait()
// returns, not merely after Start()). The returned closure is idempotent: an
// accidental double-release is a safe no-op rather than a double-unlock.
//
// tmux's server is single-threaded (confirmed from its own source: a
// libevent event loop with no worker threads, doing an unconditional O(n)
// scan of every connected client on each wakeup) and gets measurably slower
// as concurrent load and client count rise. This bounds how many tmux
// subprocesses can be in flight at once so callers queue client-side instead
// of piling onto the server.
func AcquireExecSlot(ctx context.Context, serverSocket string) (release func(), err error) {
	dir, err := gateDir(serverSocket)
	if err != nil {
		return nil, fmt.Errorf("exec gate: resolve dir: %w", err)
	}
	n := appconfig.LoadConfig().TmuxExecGate.SlotsOrDefault()
	rel, _, acquireErr := acquireSlot(ctx, dir, n, true)
	if acquireErr != nil {
		return nil, acquireErr
	}
	return rel, nil
}

// resyncGateKeySuffix separates the resync fast-lane pool's lock directory
// from the default pool's, for the same serverSocket. gateDir itself is
// unmodified — this just changes the key passed into it.
const resyncGateKeySuffix = "#resync"

// AcquireResyncExecSlot blocks until a slot in the resync fast-lane pool is
// free, or ctx is done. It draws from a pool that is entirely separate from
// AcquireExecSlot's default pool — keyed on serverSocket+"#resync" rather
// than serverSocket — sized by ResyncFastLaneSlotsOrDefault() rather than
// TmuxExecGate.SlotsOrDefault(). Saturating one pool never blocks or borrows
// capacity from the other. release must be called exactly once, after the
// subprocess this slot guards has fully exited, same contract as
// AcquireExecSlot.
func AcquireResyncExecSlot(ctx context.Context, serverSocket string) (release func(), err error) {
	dir, err := gateDir(serverSocket + resyncGateKeySuffix)
	if err != nil {
		return nil, fmt.Errorf("resync exec gate: resolve dir: %w", err)
	}
	n := appconfig.LoadConfig().TmuxExecGate.ResyncFastLaneSlotsOrDefault()
	// Task 7.1.1.2 (Epic 7.1 observability) — measure how long a resync call
	// actually waited for a fast-lane slot. resyncFastLaneAcquireTimeout
	// bounds this at 3s; logging the real wait lets us tell "fast lane is
	// consistently near-saturated" apart from "fine most of the time" without
	// needing to reproduce contention interactively.
	waitStart := time.Now()
	rel, _, acquireErr := acquireSlot(ctx, dir, n, true)
	waitElapsed := time.Since(waitStart)
	if acquireErr != nil {
		log.Debug("resync exec gate: wait for fast-lane slot failed", "serverSocket", serverSocket, "waitMs", waitElapsed.Milliseconds(), "err", acquireErr)
		return nil, acquireErr
	}
	log.Debug("resync exec gate: acquired fast-lane slot", "serverSocket", serverSocket, "waitMs", waitElapsed.Milliseconds())
	return rel, nil
}

// TryAcquireExecSlot is the non-blocking variant for periodic background
// pollers: if no slot is free right now, ok is false so the caller can skip
// this cycle rather than queue behind interactive traffic.
func TryAcquireExecSlot(serverSocket string) (release func(), ok bool) {
	dir, err := gateDir(serverSocket)
	if err != nil {
		log.Warn("exec gate: resolve dir failed, allowing unguarded", "err", err)
		return func() {}, true
	}
	n := appconfig.LoadConfig().TmuxExecGate.SlotsOrDefault()
	rel, ok, _ := acquireSlot(context.Background(), dir, n, false)
	return rel, ok
}

// execGateAcquireTimeout bounds how long a call waits for a free exec-gate
// slot before giving up, when ctx doesn't already carry a shorter deadline.
const execGateAcquireTimeout = 5 * time.Second

// resyncFastLaneAcquireTimeout bounds how long a resync-triggered call waits
// for a free fast-lane slot. Deliberately shorter than execGateAcquireTimeout
// (5s) and NOT inherited from it: the client's own stall watchdog
// (RESYNC_STALL_TIMEOUT_MS) gives up at 4s, so a server-side acquire timeout
// at or above that ceiling can "succeed" after the client has already stopped
// waiting. 3s leaves a full 1s of margin under the 4s client ceiling for
// network/marshal/dispatch latency after the slot is acquired. If load
// characterization (Task 4.2.1.10a) later shows 3s is itself too tight, tune
// it down further — never up past the 4s ceiling.
const resyncFastLaneAcquireTimeout = 3 * time.Second

// runGatedWith is the shared acquire/run/release body behind both runGated
// (execGateAcquireTimeout, the default pool) and runGatedFastLane
// (resyncFastLaneAcquireTimeout, the resync-only pool) so the two timeout
// variants don't hand-copy the same 11 lines.
func runGatedWith[T any](ctx context.Context, serverSocket string, timeout time.Duration, acquire func(ctx context.Context, serverSocket string) (func(), error), fn func() (T, error)) (T, error) {
	gateCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	release, err := acquire(gateCtx, serverSocket)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("exec gate: %w", err)
	}
	defer release()
	return fn()
}

// runGated acquires an exec-gate slot for serverSocket (bounded by whichever
// is shorter: ctx's own deadline or execGateAcquireTimeout), runs fn, then
// releases the slot. This is the single call-site pattern every tmux
// subprocess spawn in this package should use instead of hand-rolling
// acquire/timeout/release around each one.
func runGated[T any](ctx context.Context, serverSocket string, fn func() (T, error)) (T, error) {
	return runGatedWith(ctx, serverSocket, execGateAcquireTimeout, AcquireExecSlot, fn)
}

// runGatedFastLane is runGated's resync-only counterpart: it acquires from
// the separate "<serverSocket>#resync" pool (AcquireResyncExecSlot) instead
// of the shared default pool, bounded by resyncFastLaneAcquireTimeout instead
// of execGateAcquireTimeout. Used by CapturePaneContentPriority/
// RefreshClientPriority so resync-triggered tmux subprocess calls never queue
// behind ordinary tmux traffic on the same socket.
func runGatedFastLane[T any](ctx context.Context, serverSocket string, fn func() (T, error)) (T, error) {
	return runGatedWith(ctx, serverSocket, resyncFastLaneAcquireTimeout, AcquireResyncExecSlot, fn)
}

// runGatedErr is runGated for the common case of an error-only result.
func runGatedErr(ctx context.Context, serverSocket string, fn func() error) error {
	_, err := runGated(ctx, serverSocket, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

// runGatedErrFastLane is runGatedFastLane for the common case of an
// error-only result, mirroring runGatedErr's relationship to runGated.
func runGatedErrFastLane(ctx context.Context, serverSocket string, fn func() error) error {
	_, err := runGatedFastLane(ctx, serverSocket, func() (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

// gateDir returns the lock-file directory for the given tmux server socket.
// Keyed per socket (not global) so isolated servers (test suites run many
// concurrently via -L test-isolated-<pid>) never contend with the production
// gate or each other.
func gateDir(serverSocket string) (string, error) {
	configDir, err := appconfig.GetConfigDir()
	if err != nil {
		return "", err
	}
	key := "default"
	if serverSocket != "" {
		key = nonAlnum.ReplaceAllString(serverSocket, "-")
	}
	dir := filepath.Join(configDir, "tmux-exec-gate", key)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create gate dir: %w", err)
	}
	return dir, nil
}

// acquireSlot tries to lock any one of the n slot files in dir. When blocking
// is true it retries with backoff until ctx is done; otherwise it returns
// immediately after one scan across all n slots.
func acquireSlot(ctx context.Context, dir string, n int, blocking bool) (release func(), ok bool, err error) {
	if n <= 0 {
		n = 1
	}
	if blocking {
		// Checked up front, before the first TryLock attempt: an uncontended
		// slot lock succeeds immediately regardless of ctx, so without this a
		// caller whose ctx was already canceled/expired before the call would
		// still get a slot and run its subprocess anyway — defeating
		// PreviewContext's cancellation guarantee (session/instance_terminal.go)
		// for the common, uncontended case. See
		// TestInstance_PreviewContext_ReturnsCtxErrDirectlyOnCancellation.
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		default:
		}
	}
	order := rand.Perm(n) // spreads contention across processes racing at the same instant
	backoff := execGateAcquireBackoffStart
	for {
		for _, i := range order {
			path := filepath.Join(dir, fmt.Sprintf("slot-%d.lock", i))
			fl := flock.New(path)
			locked, lockErr := fl.TryLock()
			if lockErr != nil {
				continue // try the next slot; a transient FS error on one slot shouldn't block all
			}
			if locked {
				var once sync.Once
				return func() { once.Do(func() { _ = fl.Unlock() }) }, true, nil
			}
		}
		if !blocking {
			return nil, false, nil
		}
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case <-time.After(backoff):
			if backoff < execGateAcquireBackoffMax {
				backoff *= 2
			}
		}
	}
}
