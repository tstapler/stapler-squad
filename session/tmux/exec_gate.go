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

// runGated acquires an exec-gate slot for serverSocket (bounded by whichever
// is shorter: ctx's own deadline or execGateAcquireTimeout), runs fn, then
// releases the slot. This is the single call-site pattern every tmux
// subprocess spawn in this package should use instead of hand-rolling
// acquire/timeout/release around each one.
func runGated[T any](ctx context.Context, serverSocket string, fn func() (T, error)) (T, error) {
	gateCtx, cancel := context.WithTimeout(ctx, execGateAcquireTimeout)
	defer cancel()
	release, err := AcquireExecSlot(gateCtx, serverSocket)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("exec gate: %w", err)
	}
	defer release()
	return fn()
}

// runGatedErr is runGated for the common case of an error-only result.
func runGatedErr(ctx context.Context, serverSocket string, fn func() error) error {
	_, err := runGated(ctx, serverSocket, func() (struct{}, error) {
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
