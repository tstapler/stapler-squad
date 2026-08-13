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
// firstCallJSONResult envelope wrapping resultText. Mirrors the pattern used in
// session/review_gate_test.go's writeOccupyAwareFakeClaudeScript.
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
	scriptDir := t.TempDir()
	countPath := filepath.Join(scriptDir, "count.txt")
	scriptPath := writeCapabilityCheckFakeClaudeScript(t, scriptDir, countPath, capabilityCheckMarkerValue)

	runner := NewProcessRunnerForTesting(scriptPath)
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
	scriptDir := t.TempDir()
	countPath := filepath.Join(scriptDir, "count.txt")
	scriptPath := writeCapabilityCheckFakeClaudeScript(t, scriptDir, countPath, capabilityCheckMarkerValue)

	runner := NewProcessRunnerForTesting(scriptPath)
	pool := NewPoolWithRunner(PoolConfig{MaxCallsPerSession: 5, MaxConcurrentSessions: 2}, runner)

	check := &CodebaseReadCapabilitySelfCheck{}

	assert.True(t, check.Ensure(context.Background(), pool))
	assert.True(t, check.Ensure(context.Background(), pool), "second call must return the cached result")
	assert.Equal(t, 1, countInvocations(t, countPath), "second Ensure call must not re-run the subprocess")
}

// TestCodebaseReadCapabilitySelfCheck_Failure_CachesFailureAndDoesNotRetry verifies
// that a smoke test whose result does not contain the marker caches ok=false and
// subsequent Ensure calls return false without re-running the subprocess.
func TestCodebaseReadCapabilitySelfCheck_Failure_CachesFailureAndDoesNotRetry(t *testing.T) {
	scriptDir := t.TempDir()
	countPath := filepath.Join(scriptDir, "count.txt")
	// Script returns a result that does NOT contain the marker — simulates a
	// degraded/misconfigured claude CLI that fails to actually read the file.
	scriptPath := writeCapabilityCheckFakeClaudeScript(t, scriptDir, countPath, "not the marker")

	runner := NewProcessRunnerForTesting(scriptPath)
	pool := NewPoolWithRunner(PoolConfig{MaxCallsPerSession: 5, MaxConcurrentSessions: 2}, runner)

	check := &CodebaseReadCapabilitySelfCheck{}

	assert.False(t, check.Ensure(context.Background(), pool))
	assert.False(t, check.Ensure(context.Background(), pool), "second call must return the cached failure result")
	assert.Equal(t, 1, countInvocations(t, countPath), "second Ensure call must not retry the subprocess after a cached failure")
	assert.True(t, check.Checked())
}

// TestCodebaseReadCapabilitySelfCheck_NilPool_ReturnsFalse verifies Ensure degrades
// gracefully (rather than panicking) when called with a nil pool.
func TestCodebaseReadCapabilitySelfCheck_NilPool_ReturnsFalse(t *testing.T) {
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
	scriptDir := t.TempDir()
	countPath := filepath.Join(scriptDir, "count.txt")
	scriptPath := writeSlowCapabilityCheckFakeClaudeScript(t, scriptDir, countPath, capabilityCheckMarkerValue, 200*time.Millisecond)

	runner := NewProcessRunnerForTesting(scriptPath)
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
