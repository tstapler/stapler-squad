package tymux

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// --- Injection-seam stubs ---
//
// EnsureDaemonRunning is exported with a fixed (ctx, cfg) signature, so unit
// tests substitute deterministic fakes for checkDaemonHealthyFn/
// startDaemonAttemptFn/portListeningFn (package-level vars in supervise.go)
// instead of spawning a real tymuxd subprocess or making a real network
// call. Each stub helper saves the original and returns a restore func,
// mirroring the save/restore-via-defer shape used throughout this repo's
// other global-var-injection tests.

func stubCheckDaemonHealthy(fn func(context.Context, DaemonConfig) bool) func() {
	orig := checkDaemonHealthyFn
	checkDaemonHealthyFn = fn
	return func() { checkDaemonHealthyFn = orig }
}

func stubStartDaemonAttempt(fn func(DaemonConfig) (*os.Process, error)) func() {
	orig := startDaemonAttemptFn
	startDaemonAttemptFn = fn
	return func() { startDaemonAttemptFn = orig }
}

func stubPortListening(fn func(DaemonConfig) bool) func() {
	orig := portListeningFn
	portListeningFn = fn
	return func() { portListeningFn = orig }
}

func stubStopTymuxd(fn func() error) func() {
	orig := stopTymuxdFn
	stopTymuxdFn = fn
	return func() { stopTymuxdFn = orig }
}

// withFastRetryBounds overrides daemonStartAttempts/backoffStart/backoffMax
// with small, test-friendly values so a test exercising the spawn-and-retry
// path doesn't have to wait out the multi-second production worst case.
// Restored via t.Cleanup.
func withFastRetryBounds(t *testing.T, attempts int, backoffStart, backoffMax time.Duration) {
	t.Helper()
	origAttempts, origStart, origMax := daemonStartAttempts, daemonStartBackoffStart, daemonStartBackoffMax
	daemonStartAttempts, daemonStartBackoffStart, daemonStartBackoffMax = attempts, backoffStart, backoffMax
	t.Cleanup(func() {
		daemonStartAttempts, daemonStartBackoffStart, daemonStartBackoffMax = origAttempts, origStart, origMax
	})
}

// TestEnsureDaemonRunning_ReusesAlreadyHealthyDaemon verifies the reuse case
// (Task 2.1.2a's step 1): an already-healthy daemon short-circuits with no
// subprocess spawned.
func TestEnsureDaemonRunning_ReusesAlreadyHealthyDaemon(t *testing.T) {
	defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool { return true })()

	var spawnCalls int32
	defer stubStartDaemonAttempt(func(DaemonConfig) (*os.Process, error) {
		atomic.AddInt32(&spawnCalls, 1)
		return nil, nil
	})()

	ready, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: "http://127.0.0.1:19999", BinaryPath: "tymuxd"})

	require.NoError(t, err)
	require.Equal(t, TymuxdReady{}, ready)
	require.Zero(t, atomic.LoadInt32(&spawnCalls), "an already-healthy daemon must not be spawned")
}

// TestEnsureDaemonRunning_SpawnsAndRetriesUntilHealthy verifies the
// spawn-then-poll path: a cold daemon is spawned exactly once, and
// EnsureDaemonRunning polls (via the injected health check) until it
// becomes healthy, deterministically and without any real subprocess or
// real sleep of production-sized duration.
func TestEnsureDaemonRunning_SpawnsAndRetriesUntilHealthy(t *testing.T) {
	withFastRetryBounds(t, 5, time.Millisecond, 4*time.Millisecond)

	var healthyCalls int32
	// Unhealthy for the outer pre-check and the first in-loop retry, healthy
	// from the third call onward (simulating tymuxd coming up shortly after
	// being spawned).
	defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool {
		return atomic.AddInt32(&healthyCalls, 1) >= 3
	})()

	var spawnCalls int32
	defer stubStartDaemonAttempt(func(DaemonConfig) (*os.Process, error) {
		atomic.AddInt32(&spawnCalls, 1)
		return nil, nil
	})()

	ready, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: "http://127.0.0.1:19998", BinaryPath: "tymuxd"})

	require.NoError(t, err)
	require.Equal(t, TymuxdReady{Spawned: true}, ready, "this call actually spawned tymuxd -- Spawned must be true")
	require.Equal(t, int32(1), atomic.LoadInt32(&spawnCalls), "a cold daemon must be spawned exactly once")
	require.GreaterOrEqual(t, atomic.LoadInt32(&healthyCalls), int32(3))
}

// TestEnsureDaemonRunning_PortSquattedFailsLoudly verifies Task 2.1.2d: when
// every retry after a spawn attempt still fails, but something is listening
// at cfg.Addr (the injected portListeningFn reports true, simulating a
// non-tymux process squatting the port), EnsureDaemonRunning fails loudly
// with ErrTymuxdPortSquatted rather than retrying forever or silently
// proceeding.
func TestEnsureDaemonRunning_PortSquattedFailsLoudly(t *testing.T) {
	withFastRetryBounds(t, 3, time.Millisecond, 2*time.Millisecond)

	defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool { return false })()
	defer stubPortListening(func(DaemonConfig) bool { return true })()

	var spawnCalls int32
	defer stubStartDaemonAttempt(func(DaemonConfig) (*os.Process, error) {
		atomic.AddInt32(&spawnCalls, 1)
		return nil, nil
	})()

	_, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: "http://127.0.0.1:19997", BinaryPath: "tymuxd"})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrTymuxdPortSquatted)
	require.Equal(t, int32(1), atomic.LoadInt32(&spawnCalls), "a squatted port must not be retried with repeated spawn attempts")
}

// TestEnsureDaemonRunning_UnhealthyAndPortNotListeningSurfacesPlainError
// covers the sibling branch of Task 2.1.2d's distinction: when nothing
// answers ListSessions and nothing is listening on the port either (a
// genuine spawn failure, e.g. a missing/broken binary), the error is a
// plain failure, not ErrTymuxdPortSquatted.
func TestEnsureDaemonRunning_UnhealthyAndPortNotListeningSurfacesPlainError(t *testing.T) {
	withFastRetryBounds(t, 2, time.Millisecond, 2*time.Millisecond)

	defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool { return false })()
	defer stubPortListening(func(DaemonConfig) bool { return false })()
	defer stubStartDaemonAttempt(func(DaemonConfig) (*os.Process, error) { return nil, nil })()

	_, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: "http://127.0.0.1:19995", BinaryPath: "tymuxd"})

	require.Error(t, err)
	require.NotErrorIs(t, err, ErrTymuxdPortSquatted)
}

// TestEnsureDaemonRunning_should_StopOrphanedTymuxd_When_HealthCheckRetryExhausts
// is the regression test for the BLOCKER fixed in this change: when the
// coalesced spawn-and-retry closure exhausts every retry without the daemon
// becoming healthy, EnsureDaemonRunning must call stopTymuxdFn (StopTymuxd in
// production) to reap the process it just spawned before returning the
// error -- otherwise a failed cold start leaves an orphan squatting cfg.Addr
// that poisons every future call. Covers both failure branches (port
// squatted and plain timeout) since the fix applies to both.
func TestEnsureDaemonRunning_should_StopOrphanedTymuxd_When_HealthCheckRetryExhausts(t *testing.T) {
	testCases := []struct {
		name          string
		portListening bool
	}{
		{name: "PortSquatted", portListening: true},
		{name: "PlainTimeout", portListening: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			withFastRetryBounds(t, 2, time.Millisecond, 2*time.Millisecond)

			defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool { return false })()
			defer stubPortListening(func(DaemonConfig) bool { return tc.portListening })()

			var spawned atomic.Bool
			defer stubStartDaemonAttempt(func(DaemonConfig) (*os.Process, error) {
				spawned.Store(true)
				return nil, nil
			})()

			var stopCalls int32
			defer stubStopTymuxd(func() error {
				atomic.AddInt32(&stopCalls, 1)
				return nil
			})()

			_, err := EnsureDaemonRunning(context.Background(), DaemonConfig{Addr: "http://127.0.0.1:19993", BinaryPath: "tymuxd"})

			require.Error(t, err)
			require.True(t, spawned.Load(), "test setup sanity check: a spawn attempt must have happened")
			require.Equal(t, int32(1), atomic.LoadInt32(&stopCalls), "a failed cold start must reap the spawned process exactly once")
		})
	}
}

// TestStopTymuxd_IdempotentWhenNoPIDFile mirrors daemon/daemon.go's
// StopDaemon idempotent-stop contract: stopping when no PID file exists is
// not an error.
func TestStopTymuxd_IdempotentWhenNoPIDFile(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	err := StopTymuxd()

	require.NoError(t, err)
}

// TestEnsureDaemonRunning_should_CoalesceViaSingleflight_When_ConcurrentCallersRaceOnColdStart
// is Task 2.1.2h: several goroutines call EnsureDaemonRunning concurrently
// against the same DaemonConfig when nothing is running yet. The
// singleflight guard (spawnSF, keyed by cfg.Addr) must coalesce them onto
// exactly one spawn attempt -- not just "at most twice, self-heals" but
// exactly once -- and every caller must observe the same result. Run with
// `go test -race`.
func TestEnsureDaemonRunning_should_CoalesceViaSingleflight_When_ConcurrentCallersRaceOnColdStart(t *testing.T) {
	withFastRetryBounds(t, 5, time.Millisecond, 2*time.Millisecond)

	var spawned atomic.Bool
	defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool {
		return spawned.Load()
	})()

	var spawnCalls int32
	defer stubStartDaemonAttempt(func(DaemonConfig) (*os.Process, error) {
		atomic.AddInt32(&spawnCalls, 1)
		// Hold the singleflight leader in-flight long enough that every
		// concurrent racer's cold-start check is guaranteed to land while
		// this spawn is still outstanding, so they coalesce onto it instead
		// of a lucky-timing race deciding whether they do.
		time.Sleep(50 * time.Millisecond)
		spawned.Store(true)
		return nil, nil
	})()

	cfg := DaemonConfig{Addr: "http://127.0.0.1:19996", BinaryPath: "tymuxd"}

	const racers = 8
	var wg sync.WaitGroup
	results := make([]TymuxdReady, racers)
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = EnsureDaemonRunning(context.Background(), cfg)
		}(i)
	}
	wg.Wait()

	for i := 0; i < racers; i++ {
		require.NoError(t, errs[i])
		require.Equal(t, TymuxdReady{Spawned: true}, results[i], "the coalesced spawn actually started tymuxd -- every coalesced caller must see Spawned true")
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&spawnCalls), "concurrent cold-start callers must coalesce onto exactly one spawn attempt")
}

// TestEnsureDaemonRunning_should_CoalesceViaSingleflight_When_ConcurrentCallersRaceOnFailure
// is the failure-path sibling of the test above: concurrent racers on a
// cold daemon that never becomes healthy must still coalesce onto exactly
// one spawn attempt and all observe the same (wrapped) error.
func TestEnsureDaemonRunning_should_CoalesceViaSingleflight_When_ConcurrentCallersRaceOnFailure(t *testing.T) {
	withFastRetryBounds(t, 3, time.Millisecond, 2*time.Millisecond)

	defer stubCheckDaemonHealthy(func(context.Context, DaemonConfig) bool { return false })()
	defer stubPortListening(func(DaemonConfig) bool { return true })()

	var spawnCalls int32
	defer stubStartDaemonAttempt(func(DaemonConfig) (*os.Process, error) {
		atomic.AddInt32(&spawnCalls, 1)
		time.Sleep(50 * time.Millisecond)
		return nil, nil
	})()

	cfg := DaemonConfig{Addr: "http://127.0.0.1:19994", BinaryPath: "tymuxd"}

	const racers = 8
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = EnsureDaemonRunning(context.Background(), cfg)
		}(i)
	}
	wg.Wait()

	for i := 1; i < racers; i++ {
		require.ErrorIs(t, errs[i], ErrTymuxdPortSquatted)
		require.EqualError(t, errs[i], errs[0].Error(), "every coalesced caller must observe the identical error")
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&spawnCalls), "concurrent cold-start callers must coalesce onto exactly one spawn attempt even on failure")
}
