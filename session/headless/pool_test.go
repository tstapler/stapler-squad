package headless

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// firstCallJSON returns a valid JSON response for the first-call path.
func firstCallJSON(sessionID, result string) string {
	return fmt.Sprintf(`{"session_id":%q,"result":%q,"cost_usd":0.001}`, sessionID, result)
}

// newTestPool creates a Pool with FakeRunner for unit testing.
func newTestPool(cfg PoolConfig, runner *FakeRunner) *Pool {
	return NewPoolWithRunner(cfg, runner)
}

// TestPool_CallBlocking_FirstCall_CapturesSessionID verifies that the first call
// uses the JSON path and stores the session_id.
func TestPool_CallBlocking_FirstCall_CapturesSessionID(t *testing.T) {
	runner := NewFakeRunner(firstCallJSON("abc", "hello"))
	pool := newTestPool(PoolConfig{MaxCallsPerSession: 25}, runner)

	result, err := pool.CallBlocking(context.Background(), "feat1", "system", "user prompt")
	require.NoError(t, err)
	assert.Equal(t, "hello", result)

	// Session ID must be stored.
	pool.mu.Lock()
	state := pool.sessions["feat1"]
	pool.mu.Unlock()
	require.NotNil(t, state)
	assert.Equal(t, "abc", state.sessionID)
}

// TestPool_FirstCall_ArgsContainOutputFormatJSON verifies the first-call args.
func TestPool_FirstCall_ArgsContainOutputFormatJSON(t *testing.T) {
	runner := NewFakeRunner(firstCallJSON("s1", "result"))
	pool := newTestPool(PoolConfig{}, runner)

	_, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt")
	require.NoError(t, err)

	args := runner.ArgsForCall(0)
	require.NotNil(t, args)
	assert.True(t, runner.ArgsContainSequence(0, "--output-format", "json"),
		"first call must include --output-format json; got: %v", args)
	assert.True(t, runner.ArgsContainSequence(0, "--system-prompt", "sys"),
		"first call must include --system-prompt; got: %v", args)
	assert.Contains(t, args, "--exclude-dynamic-system-prompt-sections")
}

// TestPool_ResumedCall_ArgsContainResumeAndExclude verifies resumed-call args.
func TestPool_ResumedCall_ArgsContainResumeAndExclude(t *testing.T) {
	runner := NewFakeRunner(
		firstCallJSON("sess-xyz", "first result"),
		"second result\n",
	)
	pool := newTestPool(PoolConfig{}, runner)

	_, _ = pool.CallBlocking(context.Background(), "f1", "sys", "prompt1")
	_, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt2")
	require.NoError(t, err)

	args := runner.ArgsForCall(1)
	require.NotNil(t, args)
	assert.True(t, runner.ArgsContainSequence(1, "--resume", "sess-xyz"),
		"second call must resume session sess-xyz; got: %v", args)
	assert.Contains(t, args, "--exclude-dynamic-system-prompt-sections")
	// Must NOT contain --output-format on a resumed call.
	assert.NotContains(t, args, "--output-format")
}

// TestPool_FirstCall_ModelFlagIncluded_WhenNonEmpty verifies model flag injection.
func TestPool_FirstCall_ModelFlagIncluded_WhenNonEmpty(t *testing.T) {
	runner := NewFakeRunner(firstCallJSON("s1", "ok"))
	pool := newTestPool(PoolConfig{DefaultModel: "claude-opus-4"}, runner)

	_, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt")
	require.NoError(t, err)

	args := runner.ArgsForCall(0)
	assert.True(t, runner.ArgsContainSequence(0, "--model", "claude-opus-4"),
		"model flag must be present; got: %v", args)
}

// TestPool_ParsesSessionIDFromFirstCallJSON verifies session_id capture.
func TestPool_ParsesSessionIDFromFirstCallJSON(t *testing.T) {
	runner := NewFakeRunner(firstCallJSON("abc", "hello"))
	pool := newTestPool(PoolConfig{}, runner)

	_, err := pool.CallBlocking(context.Background(), "f1", "", "prompt")
	require.NoError(t, err)

	pool.mu.Lock()
	state := pool.sessions["f1"]
	pool.mu.Unlock()
	require.NotNil(t, state)
	assert.Equal(t, "abc", state.sessionID)
}

// TestPool_Call_ContextCancel_ClosesChannel verifies that a pre-cancelled context
// either returns an error immediately or closes the channel without hanging.
func TestPool_Call_ContextCancel_ClosesChannel(t *testing.T) {
	runner := NewFakeRunner(firstCallJSON("s1", "result"))
	pool := newTestPool(PoolConfig{}, runner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	ch, err := pool.Call(ctx, "f1", "sys", "prompt")
	if err != nil {
		// Error on pre-cancelled context is the expected fast path.
		return
	}

	// If no error, channel must still close promptly.
	timeout := time.After(3 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // channel closed — pass
			}
		case <-timeout:
			t.Fatal("channel did not close within 3s after context cancellation")
		}
	}
}

// TestPool_RotatesSession_AfterMaxCalls verifies session ID changes after MaxCallsPerSession.
func TestPool_RotatesSession_AfterMaxCalls(t *testing.T) {
	// MaxCallsPerSession=2: after 2 calls the 3rd call should be a new session.
	runner := NewFakeRunner(
		firstCallJSON("session-A", "result1"),
		"result2\n",
		firstCallJSON("session-B", "result3"), // new session after rotation
	)
	pool := newTestPool(PoolConfig{MaxCallsPerSession: 2}, runner)

	_, _ = pool.CallBlocking(context.Background(), "f1", "sys", "p1")
	_, _ = pool.CallBlocking(context.Background(), "f1", "sys", "p2")
	_, _ = pool.CallBlocking(context.Background(), "f1", "sys", "p3")

	// Third call args should be a first-call (--output-format json), not a resume.
	args := runner.ArgsForCall(2)
	assert.True(t, runner.ArgsContainSequence(2, "--output-format", "json"),
		"third call should be fresh (rotation); got: %v", args)
}

// TestPool_RotatesSession_AfterConsecutiveErrors verifies circuit breaker.
func TestPool_RotatesSession_AfterConsecutiveErrors(t *testing.T) {
	runner2 := &FakeRunner{
		responses: []string{firstCallJSON("sess1", "ok"), "", "", "", firstCallJSON("sess2", "after-reset")},
		errors:    []error{nil, fmt.Errorf("err1"), fmt.Errorf("err2"), fmt.Errorf("err3"), nil},
	}
	pool := newTestPool(PoolConfig{}, runner2)

	pool.CallBlocking(context.Background(), "f1", "sys", "p1") //nolint:errcheck
	pool.CallBlocking(context.Background(), "f1", "sys", "p2") //nolint:errcheck
	pool.CallBlocking(context.Background(), "f1", "sys", "p3") //nolint:errcheck
	pool.CallBlocking(context.Background(), "f1", "sys", "p4") //nolint:errcheck
	pool.CallBlocking(context.Background(), "f1", "sys", "p5") //nolint:errcheck

	// After 3 consecutive errors, a subsequent call should be a fresh session.
	found := false
	for i := 1; i < runner2.CallCount(); i++ {
		if runner2.ArgsContainSequence(i, "--output-format", "json") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected a rotation (fresh first-call) after consecutive errors")
}

// TestPool_CallBlocking_ReturnsCollectedText verifies text concatenation.
func TestPool_CallBlocking_ReturnsCollectedText(t *testing.T) {
	runner := NewFakeRunner(firstCallJSON("s1", "hello world"))
	pool := newTestPool(PoolConfig{}, runner)

	text, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt")
	require.NoError(t, err)
	assert.Equal(t, "hello world", text)
}

// TestPool_Call_MultiLineOutput_StreamsInOrder verifies line-by-line streaming.
func TestPool_Call_MultiLineOutput_StreamsInOrder(t *testing.T) {
	runner := NewFakeRunner(
		firstCallJSON("sess1", "first"),
		"line1\nline2\nline3\n",
	)
	pool := newTestPool(PoolConfig{}, runner)

	// First call to establish session.
	pool.CallBlocking(context.Background(), "f1", "sys", "p1") //nolint:errcheck

	// Second call (resumed): streams lines.
	ch, err := pool.Call(context.Background(), "f1", "sys", "p2")
	require.NoError(t, err)

	var lines []string
	for chunk := range ch {
		if chunk.Done {
			break
		}
		if chunk.Text != "" {
			lines = append(lines, chunk.Text)
		}
	}
	assert.Equal(t, []string{"line1", "line2", "line3"}, lines)
}

// TestPool_CallBlocking_PropagatesSubprocessError verifies error propagation.
func TestPool_CallBlocking_PropagatesSubprocessError(t *testing.T) {
	runner := &FakeRunner{
		errors: []error{fmt.Errorf("start failed")},
	}
	pool := newTestPool(PoolConfig{}, runner)

	_, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt")
	assert.Error(t, err)
}

// TestPool_DifferentKeys_RunInParallel verifies parallel execution on different keys.
func TestPool_DifferentKeys_RunInParallel(t *testing.T) {
	runner := &FakeRunner{
		responses: []string{
			firstCallJSON("s1", "result1"),
			firstCallJSON("s2", "result2"),
		},
	}
	pool := newTestPool(PoolConfig{MaxConcurrentSessions: 5}, runner)

	var wg sync.WaitGroup
	var err1, err2 error
	var r1, r2 string

	wg.Add(2)
	go func() {
		defer wg.Done()
		r1, err1 = pool.CallBlocking(context.Background(), "key1", "sys", "p1")
	}()
	go func() {
		defer wg.Done()
		r2, err2 = pool.CallBlocking(context.Background(), "key2", "sys", "p2")
	}()
	wg.Wait()

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotEmpty(t, r1)
	assert.NotEmpty(t, r2)
}

// TestPool_SameKey_ConcurrentCalls_Serialized verifies concurrent access doesn't panic.
func TestPool_SameKey_ConcurrentCalls_Serialized(t *testing.T) {
	responses := make([]string, 10)
	responses[0] = firstCallJSON("s1", "r1")
	for i := 1; i < 10; i++ {
		responses[i] = fmt.Sprintf("result%d\n", i)
	}
	runner := NewFakeRunner(responses...)
	pool := newTestPool(PoolConfig{MaxConcurrentSessions: 5}, runner)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.CallBlocking(context.Background(), "shared-key", "sys", "prompt") //nolint:errcheck
		}()
	}
	wg.Wait()
	// No panic = pass.
}

// TestPool_ConcurrencySemaphore_LimitsToMax verifies semaphore limits concurrency.
func TestPool_ConcurrencySemaphore_LimitsToMax(t *testing.T) {
	const maxConcurrent = 2
	var activeCount atomic.Int32
	var peakCount atomic.Int32

	var peakMu sync.Mutex
	tr := &trackingRunner{
		responses: make([]string, 10),
		onStart: func() {
			n := activeCount.Add(1)
			peakMu.Lock()
			if n > peakCount.Load() {
				peakCount.Store(n)
			}
			peakMu.Unlock()
		},
		onStop: func() { activeCount.Add(-1) },
	}
	for i := range tr.responses {
		tr.responses[i] = firstCallJSON(fmt.Sprintf("s%d", i), "ok")
	}

	pool := NewPoolWithRunner(PoolConfig{MaxConcurrentSessions: maxConcurrent, MaxCallsPerSession: 1}, tr)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		key := FeatureKey(fmt.Sprintf("key%d", i))
		go func(k FeatureKey) {
			defer wg.Done()
			pool.CallBlocking(context.Background(), k, "sys", "p") //nolint:errcheck
		}(key)
	}
	wg.Wait()

	assert.LessOrEqual(t, int(peakCount.Load()), maxConcurrent,
		"peak concurrent calls %d exceeded limit %d", peakCount.Load(), maxConcurrent)
}

// trackingRunner wraps to add concurrency tracking hooks.
type trackingRunner struct {
	mu        sync.Mutex
	responses []string
	index     int
	onStart   func()
	onStop    func()
}

func (r *trackingRunner) Run(ctx context.Context, args []string, _ io.Reader) (io.ReadCloser, func() error, error) {
	r.onStart()

	r.mu.Lock()
	idx := r.index
	r.index++
	text := ""
	if idx < len(r.responses) {
		text = r.responses[idx]
	}
	r.mu.Unlock()

	stop := func() error {
		r.onStop()
		return nil
	}
	return newSlowReader(ctx, text), stop, nil
}

// slowReadCloser simulates subprocess execution with a small delay.
type slowReadCloser struct {
	ctx  context.Context
	text string
	done bool
}

func newSlowReader(ctx context.Context, text string) io.ReadCloser {
	return &slowReadCloser{ctx: ctx, text: text}
}

func (s *slowReadCloser) Read(p []byte) (int, error) {
	if !s.done {
		s.done = true
		select {
		case <-time.After(50 * time.Millisecond):
		case <-s.ctx.Done():
		}
		n := copy(p, s.text)
		return n, io.EOF
	}
	return 0, io.EOF
}

func (s *slowReadCloser) Close() error { return nil }

// TestNewPool_ReturnsErrClaudeNotFound_WhenBinaryMissing verifies NewPool
// fails only when claude is absent from BOTH PATH and every fallback
// location (see findClaudeBinary in caller.go). HOME and claudeFallbackDirs
// must both be neutralized here — a broken PATH alone isn't enough to prove
// "not found anywhere" on a machine that happens to have claude installed at
// $HOME/.local/bin or a system fallback dir like /usr/local/bin.
func TestNewPool_ReturnsErrClaudeNotFound_WhenBinaryMissing(t *testing.T) {
	t.Setenv("PATH", "/tmp/nonexistent-path-for-test-headless")
	t.Setenv("HOME", t.TempDir())
	origDirs := claudeFallbackDirs
	claudeFallbackDirs = nil
	defer func() { claudeFallbackDirs = origDirs }()

	_, err := NewPool(PoolConfig{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrClaudeNotFound)
}

// TestNewPoolWithRunner_DoesNotCallLookPath verifies no PATH lookup with FakeRunner.
func TestNewPoolWithRunner_DoesNotCallLookPath(t *testing.T) {
	t.Setenv("PATH", "/tmp/nonexistent-path-for-test-headless")
	runner := NewFakeRunner()
	pool := NewPoolWithRunner(PoolConfig{}, runner)
	assert.NotNil(t, pool)
}

// TestPool_ZeroCallsPerSession_UsesDefault25 verifies default application.
func TestPool_ZeroCallsPerSession_UsesDefault25(t *testing.T) {
	runner := NewFakeRunner()
	pool := NewPoolWithRunner(PoolConfig{MaxCallsPerSession: 0}, runner)
	assert.Equal(t, defaultMaxCalls, pool.cfg.MaxCallsPerSession)
}

// TestPool_DefaultPool_SetAndGet_ThreadSafe verifies concurrent DefaultPool access.
func TestPool_DefaultPool_SetAndGet_ThreadSafe(t *testing.T) {
	original := DefaultPool()
	defer SetDefaultPool(original)

	pool := NewPoolWithRunner(PoolConfig{}, NewFakeRunner())

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = DefaultPool()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 100; j++ {
			SetDefaultPool(pool)
		}
	}()
	wg.Wait()
}

// TestFakeRunner_InspectsArgs_ReturnsJSONForFirstCall verifies FakeRunner JSON path.
func TestFakeRunner_InspectsArgs_ReturnsJSONForFirstCall(t *testing.T) {
	runner := NewFakeRunner(firstCallJSON("s1", "ok"))
	pool := newTestPool(PoolConfig{}, runner)

	result, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt")
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
}

// TestPool_FirstCall_IsError_ReturnsErrorChunk verifies LLM-level error handling.
func TestPool_FirstCall_IsError_ReturnsErrorChunk(t *testing.T) {
	errorJSON := `{"session_id":"","result":"model refused to respond","is_error":true,"cost_usd":0}`
	runner := NewFakeRunner(errorJSON)
	pool := newTestPool(PoolConfig{}, runner)

	ch, err := pool.Call(context.Background(), "f1", "sys", "prompt")
	require.NoError(t, err)

	var gotErr bool
	var errMsg string
	for chunk := range ch {
		if chunk.Err != nil {
			gotErr = true
			errMsg = chunk.Err.Error()
		}
	}
	assert.True(t, gotErr, "expected an error chunk from is_error=true response")
	assert.Contains(t, errMsg, "model refused")
}

// TestPool_FirstCall_CostUSD_ForwardedOnDoneChunk verifies cost_usd propagation.
func TestPool_FirstCall_CostUSD_ForwardedOnDoneChunk(t *testing.T) {
	costJSON := `{"session_id":"s1","result":"ok","is_error":false,"cost_usd":0.0042}`
	runner := NewFakeRunner(costJSON)
	pool := newTestPool(PoolConfig{}, runner)

	ch, err := pool.Call(context.Background(), "f1", "sys", "prompt")
	require.NoError(t, err)

	var doneCost float64
	for chunk := range ch {
		if chunk.Done {
			doneCost = chunk.CostUSD
		}
	}
	assert.InDelta(t, 0.0042, doneCost, 1e-9, "cost_usd must be forwarded on the done chunk")
}

// TestPool_CallWithOptions_WorkDir_FakeRunner_ReturnsError verifies that WorkDir
// with a non-ProcessRunner returns an error immediately (not silent fallback).
func TestPool_CallWithOptions_WorkDir_FakeRunner_ReturnsError(t *testing.T) {
	runner := NewFakeRunner(firstCallJSON("s1", "ok"))
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	_, err := pool.CallWithOptions(context.Background(), "f1", "sys", "prompt", CallOptions{
		WorkDir: "/some/dir",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ProcessRunner")
}

// TestPool_CtxCancel_DuringFirstCall_DoesNotHang verifies that a context cancelled
// while io.ReadAll is in progress causes the call to return promptly.
func TestPool_CtxCancel_DuringFirstCall_DoesNotHang(t *testing.T) {
	// Use a blocking reader that never completes — the cancel should unblock it.
	runner := &blockingRunner{}
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	ctx, cancel := context.WithCancel(context.Background())

	var ch <-chan StreamChunk
	var err error
	started := make(chan struct{})
	go func() {
		ch, err = pool.Call(ctx, "f1", "sys", "prompt")
		close(started)
	}()

	// Cancel context immediately after Call is invoked to simulate midpoint cancel.
	cancel()
	<-started

	if err != nil {
		return // cancelled before subprocess started — OK
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range ch {
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("channel did not drain within 3s after context cancellation")
	}
}

// blockingRunner is a ClaudeRunner whose stdout blocks until the context is done.
type blockingRunner struct{}

func (r *blockingRunner) Run(ctx context.Context, _ []string, _ io.Reader) (io.ReadCloser, func() error, error) {
	pr, pw := io.Pipe()
	go func() {
		<-ctx.Done()
		_ = pw.CloseWithError(ctx.Err())
	}()
	return pr, func() error { return pw.CloseWithError(nil) }, nil
}

// TestPool_CtxCancel_DuringSemaphoreWait_DecrementsCallCount verifies that a
// context cancellation while blocked on the concurrency semaphore does not
// permanently inflate callCount (decrementCallCount is called on the cancel path).
func TestPool_CtxCancel_DuringSemaphoreWait_DecrementsCallCount(t *testing.T) {
	runner := NewFakeRunner()
	// MaxConcurrentSessions=1 so a single manually-occupied slot blocks the next Call.
	pool := newTestPool(PoolConfig{MaxConcurrentSessions: 1, MaxCallsPerSession: 100}, runner)

	// Occupy the one semaphore slot so the next Call must block on acquire.
	pool.concurrencySem <- struct{}{}

	// Pre-cancel the context so the select in call() fires ctx.Done() immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := pool.Call(ctx, "f1", "sys", "prompt")

	// Release the semaphore slot we manually occupied.
	<-pool.concurrencySem

	// The call must return an error (context cancelled), not succeed.
	require.Error(t, err, "expected error when ctx cancelled during semaphore wait")
	assert.ErrorIs(t, err, context.Canceled)

	// callCount must be 0 — decrementCallCount was called on the cancellation path,
	// undoing the increment from acquireSession.
	pool.mu.Lock()
	state := pool.sessions["f1"]
	pool.mu.Unlock()
	require.NotNil(t, state, "session state must exist after Call")
	assert.Equal(t, 0, state.callCount, "callCount must be 0 after ctx cancel during semaphore wait")
}

// TestFakeRunner_InspectsArgs_ReturnsPlainForResumedCall verifies resumed-call plain text.
func TestFakeRunner_InspectsArgs_ReturnsPlainForResumedCall(t *testing.T) {
	runner := NewFakeRunner(
		firstCallJSON("sess1", "first"),
		"plain text response\n",
	)
	pool := newTestPool(PoolConfig{}, runner)

	pool.CallBlocking(context.Background(), "f1", "sys", "p1") //nolint:errcheck
	result, err := pool.CallBlocking(context.Background(), "f1", "sys", "p2")
	require.NoError(t, err)
	assert.Contains(t, result, "plain text response")
}
