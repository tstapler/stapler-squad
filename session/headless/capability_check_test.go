package headless

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeCapabilityCheckFakeClaudeScript writes a fake `claude` binary that records one
// invocation per call (by appending a line to countPath) and emits a
// firstCallJSONResult envelope wrapping resultText. Invoked via
// NewShellWrappedProcessRunnerForTesting (not exec'd directly by path — see that
// constructor's doc comment).
func writeCapabilityCheckFakeClaudeScript(t *testing.T, scriptDir, countPath, resultText string) string {
	t.Helper()
	scriptPath := filepath.Join(scriptDir, "fake-claude.sh")
	outerJSON := fmt.Sprintf(`{"session_id":"s1","result":%q,"cost_usd":0}`, resultText)
	script := fmt.Sprintf("#!/bin/sh\ncat > /dev/null\necho call >> %s\ncat <<'HEADLESSTESTEOF'\n%s\nHEADLESSTESTEOF\n", countPath, outerJSON)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	return scriptPath
}

func countInvocations(t *testing.T, countPath string) int {
	t.Helper()
	data, err := os.ReadFile(countPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		require.NoError(t, err)
	}
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	return lines
}

// TestCodebaseReadCapabilitySelfCheck_RunsOnceAcrossConcurrentCallers verifies that
// many goroutines calling Ensure concurrently on the same instance only trigger the
// underlying marker-file smoke test subprocess once.
func TestCodebaseReadCapabilitySelfCheck_RunsOnceAcrossConcurrentCallers(t *testing.T) {
	t.Parallel()
	scriptDir := t.TempDir()
	countPath := filepath.Join(scriptDir, "count.txt")
	scriptPath := writeCapabilityCheckFakeClaudeScript(t, scriptDir, countPath, capabilityCheckMarkerValue)

	runner := NewShellWrappedProcessRunnerForTesting(scriptPath)
	pool := NewPoolWithRunner(PoolConfig{MaxCallsPerSession: 5, MaxConcurrentSessions: 8}, runner)

	check := &CodebaseReadCapabilitySelfCheck{}

	const goroutines = 10
	var wg sync.WaitGroup
	results := make([]bool, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = check.Ensure(context.Background(), pool)
		}(i)
	}
	wg.Wait()

	for i, ok := range results {
		assert.True(t, ok, "goroutine %d should observe the cached success result", i)
	}
	assert.Equal(t, 1, countInvocations(t, countPath), "the underlying smoke test subprocess must run exactly once")
	assert.True(t, check.Checked())
}

// TestCodebaseReadCapabilitySelfCheck_Success_CachesOK verifies a successful smoke
// test caches ok=true and does not re-run the subprocess on subsequent Ensure calls.
func TestCodebaseReadCapabilitySelfCheck_Success_CachesOK(t *testing.T) {
	t.Parallel()
	scriptDir := t.TempDir()
	countPath := filepath.Join(scriptDir, "count.txt")
	scriptPath := writeCapabilityCheckFakeClaudeScript(t, scriptDir, countPath, capabilityCheckMarkerValue)

	runner := NewShellWrappedProcessRunnerForTesting(scriptPath)
	pool := NewPoolWithRunner(PoolConfig{MaxCallsPerSession: 5, MaxConcurrentSessions: 2}, runner)

	check := &CodebaseReadCapabilitySelfCheck{}

	assert.True(t, check.Ensure(context.Background(), pool))
	assert.True(t, check.Ensure(context.Background(), pool), "second call must return the cached result")
	assert.Equal(t, 1, countInvocations(t, countPath), "second Ensure call must not re-run the subprocess")
}

// TestCodebaseReadCapabilitySelfCheck_Success_SurvivesPastFailureWindow verifies a
// cached success, unlike a cached failure, is trusted indefinitely: it must still
// short-circuit Ensure (no subprocess re-run) even long after
// capabilityCheckFailureCacheWindow has elapsed.
func TestCodebaseReadCapabilitySelfCheck_Success_SurvivesPastFailureWindow(t *testing.T) {
	t.Parallel()
	scriptDir := t.TempDir()
	countPath := filepath.Join(scriptDir, "count.txt")
	scriptPath := writeCapabilityCheckFakeClaudeScript(t, scriptDir, countPath, capabilityCheckMarkerValue)

	runner := NewShellWrappedProcessRunnerForTesting(scriptPath)
	pool := NewPoolWithRunner(PoolConfig{MaxCallsPerSession: 5, MaxConcurrentSessions: 2}, runner)

	fakeNow := time.Now()
	check := &CodebaseReadCapabilitySelfCheck{now: func() time.Time { return fakeNow }}

	assert.True(t, check.Ensure(context.Background(), pool))

	fakeNow = fakeNow.Add(10 * capabilityCheckFailureCacheWindow)
	assert.True(t, check.Ensure(context.Background(), pool), "a cached success must survive well past the failure-cache window")
	assert.Equal(t, 1, countInvocations(t, countPath), "must not re-run the subprocess for a cached success, regardless of elapsed time")
}

// TestCodebaseReadCapabilitySelfCheck_Failure_CachesFailureWithinWindow verifies
// that a smoke test whose result does not contain the marker caches ok=false and
// subsequent Ensure calls, while still within capabilityCheckFailureCacheWindow,
// return false without re-running the subprocess (Story 2.2.6c's original intent —
// don't hammer the smoke test on every single call — preserved under bounded
// rather than permanent caching).
func TestCodebaseReadCapabilitySelfCheck_Failure_CachesFailureWithinWindow(t *testing.T) {
	t.Parallel()
	scriptDir := t.TempDir()
	countPath := filepath.Join(scriptDir, "count.txt")
	// Script returns a result that does NOT contain the marker — simulates a
	// degraded/misconfigured claude CLI that fails to actually read the file.
	scriptPath := writeCapabilityCheckFakeClaudeScript(t, scriptDir, countPath, "not the marker")

	runner := NewShellWrappedProcessRunnerForTesting(scriptPath)
	pool := NewPoolWithRunner(PoolConfig{MaxCallsPerSession: 5, MaxConcurrentSessions: 2}, runner)

	fakeNow := time.Now()
	check := &CodebaseReadCapabilitySelfCheck{now: func() time.Time { return fakeNow }}

	assert.False(t, check.Ensure(context.Background(), pool))

	// Advance the clock, but stay inside the failure-cache window.
	fakeNow = fakeNow.Add(capabilityCheckFailureCacheWindow - time.Second)
	assert.False(t, check.Ensure(context.Background(), pool), "second call must return the cached failure result while still within the window")
	assert.Equal(t, 1, countInvocations(t, countPath), "second Ensure call must not retry the subprocess while the cached failure is still fresh")
	assert.True(t, check.Checked())
}

// TestCodebaseReadCapabilitySelfCheck_TransientFailureExpires_LaterEnsureRetriesAndCanSucceed
// is the regression test for the bug that stuck backlog item bd337ac9: one
// transient smoke-test failure must not permanently block every future
// codebase-read review. It simulates a single transient failure, advances the
// clock past capabilityCheckFailureCacheWindow, and confirms the next Ensure call
// re-attempts the probe (and can now succeed) rather than trusting the stale
// cached failure forever.
func TestCodebaseReadCapabilitySelfCheck_TransientFailureExpires_LaterEnsureRetriesAndCanSucceed(t *testing.T) {
	t.Parallel()
	scriptDir := t.TempDir()
	countPath := filepath.Join(scriptDir, "count.txt")
	// swapResultPath controls what the fake script returns on the NEXT invocation —
	// starts failing, then is flipped to succeed once the transient issue "clears".
	resultPath := filepath.Join(scriptDir, "result.txt")
	require.NoError(t, os.WriteFile(resultPath, []byte("not the marker"), 0o600))
	scriptPath := writeSwappableCapabilityCheckFakeClaudeScript(t, scriptDir, countPath, resultPath)

	runner := NewShellWrappedProcessRunnerForTesting(scriptPath)
	pool := NewPoolWithRunner(PoolConfig{MaxCallsPerSession: 5, MaxConcurrentSessions: 2}, runner)

	fakeNow := time.Now()
	check := &CodebaseReadCapabilitySelfCheck{now: func() time.Time { return fakeNow }}

	// One transient failure.
	assert.False(t, check.Ensure(context.Background(), pool))
	assert.Equal(t, 1, countInvocations(t, countPath))

	// The underlying issue clears, but the cached failure is still fresh — a call
	// right after the first must not retry yet.
	require.NoError(t, os.WriteFile(resultPath, []byte(capabilityCheckMarkerValue), 0o600))
	assert.False(t, check.Ensure(context.Background(), pool), "must still trust the recent cached failure")
	assert.Equal(t, 1, countInvocations(t, countPath), "must not have retried yet")

	// Advance the clock past the failure-cache window.
	fakeNow = fakeNow.Add(capabilityCheckFailureCacheWindow + time.Second)

	assert.True(t, check.Ensure(context.Background(), pool),
		"once the failure-cache window has elapsed, Ensure must re-attempt the probe and reflect the now-passing result")
	assert.Equal(t, 2, countInvocations(t, countPath), "the expired cache must trigger exactly one retry")
	assert.True(t, check.Checked())
}

// writeSwappableCapabilityCheckFakeClaudeScript writes a fake claude binary that,
// on each invocation, re-reads resultPath and echoes back whatever it currently
// contains — letting a test flip between failure and success responses across
// separate Ensure calls without needing a new script/runner.
func writeSwappableCapabilityCheckFakeClaudeScript(t *testing.T, scriptDir, countPath, resultPath string) string {
	t.Helper()
	scriptPath := filepath.Join(scriptDir, "fake-claude-swappable.sh")
	// Both values ever written to resultPath in this test file are plain,
	// quote-free ASCII (capabilityCheckMarkerValue or "not the marker"), so a bare
	// double-quoted substitution is safe — no JSON-escaping helper needed.
	script := fmt.Sprintf(`#!/bin/sh
cat > /dev/null
echo call >> %s
result=$(cat %s)
printf '{"session_id":"s1","result":"%%s","cost_usd":0}\n' "$result"
`, countPath, resultPath)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	return scriptPath
}

// TestCodebaseReadCapabilitySelfCheck_NilPool_ReturnsFalse verifies Ensure degrades
// gracefully (rather than panicking) when called with a nil pool.
func TestCodebaseReadCapabilitySelfCheck_NilPool_ReturnsFalse(t *testing.T) {
	t.Parallel()
	check := &CodebaseReadCapabilitySelfCheck{}
	assert.False(t, check.Ensure(context.Background(), nil))
}

// TestCodebaseReadCapabilitySelfCheck_CallerCtxAlreadyExpired_ProbeStillSucceeds is the
// MUST FIX #2 regression test: it verifies the process-lifetime cached verdict is not
// poisoned by whichever caller's context happens to win the sync.Once race. run()
// must derive its own probe context from context.Background() (bounded only by
// capabilityCheckTimeout), not from the caller's ctx — otherwise a transient
// cancellation/deadline on the FIRST caller permanently disables the codebase-read
// review path for the rest of the process's lifetime even though the underlying
// capability is fine.
//
// The fake claude script here sleeps briefly before responding so a checkCtx
// mistakenly derived from the caller's already-expired ctx would fail with
// context.DeadlineExceeded; a checkCtx correctly derived from context.Background()
// comfortably outlives the sleep and the probe succeeds.
func TestCodebaseReadCapabilitySelfCheck_CallerCtxAlreadyExpired_ProbeStillSucceeds(t *testing.T) {
	t.Parallel()
	scriptDir := t.TempDir()
	countPath := filepath.Join(scriptDir, "count.txt")
	scriptPath := writeSlowCapabilityCheckFakeClaudeScript(t, scriptDir, countPath, capabilityCheckMarkerValue, 200*time.Millisecond)

	runner := NewShellWrappedProcessRunnerForTesting(scriptPath)
	pool := NewPoolWithRunner(PoolConfig{MaxCallsPerSession: 5, MaxConcurrentSessions: 2}, runner)

	check := &CodebaseReadCapabilitySelfCheck{}

	// A context that is already expired by the time Ensure is called — simulates the
	// short-lived per-review ctx that triggered this bug in production.
	expiredCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(5 * time.Millisecond)
	require.Error(t, expiredCtx.Err(), "sanity check: ctx must actually be expired before calling Ensure")

	assert.True(t, check.Ensure(expiredCtx, pool),
		"a caller ctx that is already expired must not fail/cache-negative the self-check when the underlying probe would otherwise succeed")
	assert.True(t, check.Checked())
	assert.Equal(t, 1, countInvocations(t, countPath))
}

// writeSlowCapabilityCheckFakeClaudeScript is like writeCapabilityCheckFakeClaudeScript
// but sleeps for delay before responding, so tests can distinguish a probe context
// derived from context.Background() (survives the sleep) from one mistakenly derived
// from an already-expired caller ctx (would fail immediately).
func writeSlowCapabilityCheckFakeClaudeScript(t *testing.T, scriptDir, countPath, resultText string, delay time.Duration) string {
	t.Helper()
	scriptPath := filepath.Join(scriptDir, "fake-claude-slow.sh")
	outerJSON := fmt.Sprintf(`{"session_id":"s1","result":%q,"cost_usd":0}`, resultText)
	sleepSeconds := delay.Seconds()
	script := fmt.Sprintf("#!/bin/sh\ncat > /dev/null\necho call >> %s\nsleep %f\ncat <<'HEADLESSTESTEOF'\n%s\nHEADLESSTESTEOF\n", countPath, sleepSeconds, outerJSON)
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	return scriptPath
}
