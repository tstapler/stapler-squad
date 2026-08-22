package tmux

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureDebugLog temporarily redirects the slog default logger to a buffer
// (mirrors server/services/session_service_client_log_test.go's
// captureInfoLog) and returns a function that restores it and returns the
// captured output.
func captureDebugLog(t *testing.T) func() string {
	t.Helper()
	var buf bytes.Buffer
	h := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	original := slog.Default()
	slog.SetDefault(slog.New(h))
	return func() string {
		slog.SetDefault(original)
		return buf.String()
	}
}

// setupExecGateTestConfig points STAPLER_SQUAD_TEST_DIR at a fresh t.TempDir()
// and writes a minimal config.json setting TmuxExecGate.Slots/ResyncFastLaneSlots,
// isolating this test's gate directories (and slot counts) from any other test
// or from a real ~/.stapler-squad/config.json on the machine running the suite.
// Returns a unique-per-test serverSocket key so parallel (sub)tests never share
// a gate directory even under the same STAPLER_SQUAD_TEST_DIR.
func setupExecGateTestConfig(t *testing.T, defaultSlots, resyncSlots int) (serverSocket string) {
	return setupExecGateTestConfigWithInput(t, defaultSlots, resyncSlots, 0)
}

// setupExecGateTestConfigWithInput is setupExecGateTestConfig plus an
// inputSlots value for the input fast-lane pool (0 = use the built-in
// default, same convention as the other two slot counts).
func setupExecGateTestConfigWithInput(t *testing.T, defaultSlots, resyncSlots, inputSlots int) (serverSocket string) {
	t.Helper()
	testDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)

	cfg := map[string]any{
		"tmux_exec_gate": map[string]any{
			"slots":               defaultSlots,
			"resyncFastLaneSlots": resyncSlots,
			"inputFastLaneSlots":  inputSlots,
		},
	}
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "config.json"), data, 0o600))

	return "exec-gate-test-" + t.Name()
}

func TestAcquireResyncExecSlot_should_AcquireFromSeparatePool_When_DefaultPoolSaturated(t *testing.T) {
	serverSocket := setupExecGateTestConfig(t, 1, 1)

	releaseDefault, err := AcquireExecSlot(context.Background(), serverSocket)
	require.NoError(t, err)
	defer releaseDefault()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	releaseFastLane, err := AcquireResyncExecSlot(ctx, serverSocket)
	elapsed := time.Since(start)
	require.NoError(t, err)
	defer releaseFastLane()
	assert.Less(t, elapsed, 100*time.Millisecond, "fast lane acquire should not wait behind the saturated default pool")
}

func TestAcquireResyncExecSlot_should_BlockUntilSlotAvailable_When_FastLanePoolExhausted(t *testing.T) {
	serverSocket := setupExecGateTestConfig(t, 4, 1)

	releaseFirst, err := AcquireResyncExecSlot(context.Background(), serverSocket)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = AcquireResyncExecSlot(ctx, serverSocket)
	elapsed := time.Since(start)
	require.Error(t, err)
	assert.GreaterOrEqual(t, elapsed, 90*time.Millisecond, "should have blocked for close to the full timeout, not returned immediately")

	releaseFirst()
	release2, err := AcquireResyncExecSlot(context.Background(), serverSocket)
	require.NoError(t, err)
	release2()
}

// TestAcquireResyncExecSlot_should_LogWaitTimeInMilliseconds_When_SlotAcquiredAfterContention
// covers Task 7.1.1.2 (Epic 7.1 observability): the fast-lane wait time must
// actually be logged, not just measured and discarded, so operators can
// distinguish "usually instant" from "usually near the 3s ceiling" without
// reproducing contention interactively.
func TestAcquireResyncExecSlot_should_LogWaitTimeInMilliseconds_When_SlotAcquiredAfterContention(t *testing.T) {
	serverSocket := setupExecGateTestConfig(t, 4, 1)
	restore := captureDebugLog(t)

	releaseFirst, err := AcquireResyncExecSlot(context.Background(), serverSocket)
	require.NoError(t, err)

	go func() {
		time.Sleep(50 * time.Millisecond)
		releaseFirst()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	release2, err := AcquireResyncExecSlot(ctx, serverSocket)
	require.NoError(t, err)
	defer release2()

	logOutput := restore()
	assert.Contains(t, logOutput, "acquired fast-lane slot")
	assert.Regexp(t, `waitMs=[1-9]\d*`, logOutput, "should record a nonzero wait after contention, not a zero placeholder")
}

// TestAcquireResyncExecSlot_should_NotConsumeDefaultPoolCapacity_When_FastLanePoolIsFull is an
// integration test: it drives both pools concurrently against real flock-backed gate files (no
// mocking of acquireSlot/flock) to prove the fast lane pool and default pool are truly
// independent -- saturating the fast lane pool must have zero effect on default pool capacity.
func TestAcquireResyncExecSlot_should_NotConsumeDefaultPoolCapacity_When_FastLanePoolIsFull(t *testing.T) {
	serverSocket := setupExecGateTestConfig(t, 2, 2)

	var fastLaneReleases []func()
	for i := 0; i < 2; i++ {
		release, err := AcquireResyncExecSlot(context.Background(), serverSocket)
		require.NoError(t, err)
		fastLaneReleases = append(fastLaneReleases, release)
	}
	defer func() {
		for _, release := range fastLaneReleases {
			release()
		}
	}()

	var wg sync.WaitGroup
	errs := make([]error, 2)
	releases := make([]func(), 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			release, err := AcquireExecSlot(ctx, serverSocket)
			errs[i] = err
			releases[i] = release
		}()
	}
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "default pool acquire %d should succeed despite fast lane pool being fully saturated", i)
		releases[i]()
	}
}

func TestAcquireInputExecSlot_should_AcquireFromSeparatePool_When_DefaultPoolSaturated(t *testing.T) {
	serverSocket := setupExecGateTestConfigWithInput(t, 1, 1, 1)

	releaseDefault, err := AcquireExecSlot(context.Background(), serverSocket)
	require.NoError(t, err)
	defer releaseDefault()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	releaseInput, err := AcquireInputExecSlot(ctx, serverSocket)
	elapsed := time.Since(start)
	require.NoError(t, err)
	defer releaseInput()
	assert.Less(t, elapsed, 100*time.Millisecond, "input fast lane acquire should not wait behind the saturated default pool")
}

// TestAcquireInputExecSlot_should_NotConsumeDefaultPoolCapacity_When_InputPoolIsFull is an
// integration test: it drives both pools concurrently against real flock-backed gate files (no
// mocking of acquireSlot/flock) to prove the input fast lane pool and default pool are truly
// independent -- saturating the input pool must have zero effect on default pool capacity. This
// is the property that keeps a poller's capture-pane traffic on the default pool from ever
// stalling user keystrokes routed through the input pool, and vice versa.
func TestAcquireInputExecSlot_should_NotConsumeDefaultPoolCapacity_When_InputPoolIsFull(t *testing.T) {
	serverSocket := setupExecGateTestConfigWithInput(t, 2, 1, 2)

	var inputReleases []func()
	for i := 0; i < 2; i++ {
		release, err := AcquireInputExecSlot(context.Background(), serverSocket)
		require.NoError(t, err)
		inputReleases = append(inputReleases, release)
	}
	defer func() {
		for _, release := range inputReleases {
			release()
		}
	}()

	var wg sync.WaitGroup
	errs := make([]error, 2)
	releases := make([]func(), 2)
	wg.Add(2)
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			release, err := AcquireExecSlot(ctx, serverSocket)
			errs[i] = err
			releases[i] = release
		}()
	}
	wg.Wait()

	for i, err := range errs {
		require.NoError(t, err, "default pool acquire %d should succeed despite input pool being fully saturated", i)
		releases[i]()
	}
}

func TestExecGate_BoundsPeakConcurrency(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const n = 3
	const workers = 20

	var current atomic.Int32
	var peak atomic.Int32
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			release, ok, err := acquireSlot(context.Background(), dir, n, true)
			require.NoError(t, err)
			require.True(t, ok)
			defer release()

			cur := current.Add(1)
			for {
				p := peak.Load()
				if cur <= p || peak.CompareAndSwap(p, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			current.Add(-1)
		}()
	}
	wg.Wait()

	assert.LessOrEqual(t, peak.Load(), int32(n), "peak concurrent holders exceeded the gate weight")
	assert.Equal(t, int32(n), peak.Load(), "gate never reached full saturation -- test isn't proving anything")
}

func TestExecGate_TryAcquireFailsWhenFull(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	const n = 2

	// Fill every slot via blocking acquire, synchronized so all n are held
	// before we probe TryAcquire.
	var wg sync.WaitGroup
	var startWg sync.WaitGroup
	startWg.Add(n)
	releases := make([]func(), n)
	for i := 0; i < n; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, ok, err := acquireSlot(context.Background(), dir, n, true)
			require.NoError(t, err)
			require.True(t, ok)
			releases[i] = release
			startWg.Done()
		}()
	}
	startWg.Wait()

	start := time.Now()
	_, ok, err := acquireSlot(context.Background(), dir, n, false)
	elapsed := time.Since(start)
	require.NoError(t, err)
	assert.False(t, ok, "TryAcquire should fail when the gate is fully held")
	assert.Less(t, elapsed, 50*time.Millisecond, "non-blocking TryAcquire must return immediately")

	// Release one slot; TryAcquire should now succeed.
	releases[0]()
	release, ok, err := acquireSlot(context.Background(), dir, n, false)
	require.NoError(t, err)
	assert.True(t, ok, "TryAcquire should succeed once a slot is freed")
	release()

	for i := 1; i < n; i++ {
		releases[i]()
	}
	wg.Wait()
}

func TestExecGate_ReleaseIsIdempotentSafe(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	release, ok, err := acquireSlot(context.Background(), dir, 1, true)
	require.NoError(t, err)
	require.True(t, ok)

	assert.NotPanics(t, func() {
		release()
		release()
	}, "double-release must be a safe no-op, not a double-unlock panic")
}

// TestExecGate_CrossProcess proves the property a plain in-memory
// semaphore.Weighted could not: the gate bounds concurrency across separate
// OS processes (matters because every `--mcp` invocation is its own process
// with its own address space), not just within one process's goroutines.
//
// Mirrors the re-exec pattern in safeexec_pdeathsig_linux_test.go: this test
// binary re-execs itself as N helper processes (env-var gated, intercepted in
// TestMain before any other setup), each of which acquires a slot on the same
// gate directory and holds it for a fixed duration while dropping a marker
// file. The parent polls the marker directory and asserts the peak count
// never exceeds the configured weight.
const execGateCrossProcessHelperEnvVar = "EXEC_GATE_CROSS_PROCESS_HELPER"
const execGateDirEnvVar = "EXEC_GATE_DIR"
const execGateNEnvVar = "EXEC_GATE_N"
const execGateHoldMSEnvVar = "EXEC_GATE_HOLD_MS"
const execGateMarkerDirEnvVar = "EXEC_GATE_MARKER_DIR"

func runExecGateCrossProcessHelper() {
	dir := os.Getenv(execGateDirEnvVar)
	n, _ := strconv.Atoi(os.Getenv(execGateNEnvVar))
	holdMS, _ := strconv.Atoi(os.Getenv(execGateHoldMSEnvVar))
	markerDir := os.Getenv(execGateMarkerDirEnvVar)

	release, ok, err := acquireSlot(context.Background(), dir, n, true)
	if err != nil || !ok {
		os.Exit(1)
	}
	markerPath := filepath.Join(markerDir, "held-"+strconv.Itoa(os.Getpid()))
	if err := os.WriteFile(markerPath, nil, 0o600); err != nil {
		os.Exit(1)
	}
	time.Sleep(time.Duration(holdMS) * time.Millisecond)
	_ = os.Remove(markerPath)
	release()
	os.Exit(0)
}

func TestExecGate_CrossProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: spawns real subprocesses")
	}

	dir := t.TempDir()
	markerDir := t.TempDir()
	const n = 2
	const numProcs = 5
	const holdMS = 150

	procs := make([]*exec.Cmd, numProcs)
	for i := range procs {
		cmd := exec.Command(os.Args[0], "-test.run=^$") //nolint:norawexec test-only re-exec of this test binary as a helper process
		cmd.Env = append(os.Environ(),
			execGateCrossProcessHelperEnvVar+"=1",
			execGateDirEnvVar+"="+dir,
			execGateNEnvVar+"="+strconv.Itoa(n),
			execGateHoldMSEnvVar+"="+strconv.Itoa(holdMS),
			execGateMarkerDirEnvVar+"="+markerDir,
		)
		require.NoError(t, cmd.Start())
		procs[i] = cmd
	}

	allDone := make(chan struct{})
	go func() {
		for _, cmd := range procs {
			_ = cmd.Wait()
		}
		close(allDone)
	}()

	maxHeld := 0
poll:
	for {
		entries, _ := os.ReadDir(markerDir)
		if len(entries) > maxHeld {
			maxHeld = len(entries)
		}
		select {
		case <-allDone:
			break poll
		case <-time.After(5 * time.Millisecond):
		}
	}

	entries, _ := os.ReadDir(markerDir)
	assert.Empty(t, entries, "all marker files should be cleaned up after every helper exits")
	assert.LessOrEqual(t, maxHeld, n, "more than n separate processes held the gate simultaneously")
	assert.Equal(t, n, maxHeld, "gate should reach full saturation across processes at some point during the test")
}
