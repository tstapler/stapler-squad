package tmux

import (
	"context"
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

func TestExecGate_BoundsPeakConcurrency(t *testing.T) {
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
