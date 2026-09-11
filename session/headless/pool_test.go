package headless

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// firstCallJSON returns a valid stream-json terminal "result" line for the
// first-call path — a single line is a valid (degenerate) stream: call()'s
// scanner treats a line whose top-level JSON "type" field is "result" as
// terminal regardless of how many lines preceded it. total_cost_usd (not
// cost_usd) matches the real CLI's actual field name — see
// firstCallJSONResult's doc comment.
func firstCallJSON(sessionID, result string) string {
	return fmt.Sprintf(`{"type":"result","session_id":%q,"result":%q,"total_cost_usd":0.001}`, sessionID, result)
}

// TestPool_CallBlocking_FirstCall_ToleratesTrailingNonJSONOutput covers the fix
// for a real claude CLI failure mode: --output-format json's stdout can be
// followed by a trailing non-JSON line (e.g. an update notice), which
// json.Unmarshal on the whole buffer rejects outright as "invalid character
// ... after top-level value" even though the leading JSON is well-formed.
// See session/headless/caller.go's json.NewDecoder usage in the first-call path.
func TestPool_CallBlocking_FirstCall_ToleratesTrailingNonJSONOutput(t *testing.T) {
	t.Parallel()
	response := firstCallJSON("abc", "hello") + "\nClaude Code v2.1.0 is available. Run `claude update` to install.\n"
	runner := NewFakeRunner(response)
	pool := newTestPool(PoolConfig{MaxCallsPerSession: 25}, runner)

	result, err := pool.CallBlocking(context.Background(), "feat1", "system", "user prompt", CallOptions{}, DiscardCost)
	require.NoError(t, err)
	assert.Equal(t, "hello", result)
}

// TestPool_CallBlocking_FirstCall_NoResultLine_ReturnsErrorWithAccumulatedText
// covers the fallback when a first-call subprocess exits after producing only
// non-terminal lines (no "type":"result" event) — a shape with no direct test
// coverage before this. The error must name the missing terminal event, and
// the accumulated output must still be delivered so captureHeadlessFailure
// has something to persist.
func TestPool_CallBlocking_FirstCall_NoResultLine_ReturnsErrorWithAccumulatedText(t *testing.T) {
	t.Parallel()
	response := `{"type":"system","subtype":"init"}` + "\n" + `{"type":"assistant","message":"partial progress"}`
	runner := NewFakeRunner(response)
	pool := newTestPool(PoolConfig{}, runner)

	result, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt", CallOptions{}, DiscardCost)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no terminal result event")
	assert.Contains(t, result, "partial progress", "accumulated text must still be delivered even though no terminal event arrived")
}

// TestPool_CallBlocking_FirstCall_MalformedResultLineJSON_ReturnsParseErrorWithAccumulatedText
// covers the other stream-json parsing branch with no prior direct coverage:
// a line that structurally IS the terminal "result" event (so isResultLine
// recognizes it) but fails firstCallJSONResult's own unmarshal — here,
// total_cost_usd as a JSON string instead of a number. A truly truncated/
// invalid-JSON line no longer reaches this branch at all after the
// structural (not substring) terminal-line check, which is deliberate: see
// isResultLine's doc comment.
func TestPool_CallBlocking_FirstCall_MalformedResultLineJSON_ReturnsParseErrorWithAccumulatedText(t *testing.T) {
	t.Parallel()
	response := `{"type":"assistant","message":"working"}` + "\n" + `{"type":"result","session_id":"sess-1","total_cost_usd":"oops"}`
	runner := NewFakeRunner(response)
	pool := newTestPool(PoolConfig{}, runner)

	result, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt", CallOptions{}, DiscardCost)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "first-call result-line JSON parse")
	assert.Contains(t, result, "working", "accumulated text must still be delivered even when the result line fails to parse")
}

// newTestPool creates a Pool with FakeRunner for unit testing.
func newTestPool(cfg PoolConfig, runner *FakeRunner) *Pool {
	return NewPoolWithRunner(cfg, runner)
}

// TestPool_CallBlocking_FirstCall_CapturesSessionID verifies that the first call
// uses the JSON path and stores the session_id.
func TestPool_CallBlocking_FirstCall_CapturesSessionID(t *testing.T) {
	t.Parallel()
	runner := NewFakeRunner(firstCallJSON("abc", "hello"))
	pool := newTestPool(PoolConfig{MaxCallsPerSession: 25}, runner)

	result, err := pool.CallBlocking(context.Background(), "feat1", "system", "user prompt", CallOptions{}, DiscardCost)
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
	t.Parallel()
	runner := NewFakeRunner(firstCallJSON("s1", "result"))
	pool := newTestPool(PoolConfig{}, runner)

	_, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt", CallOptions{}, DiscardCost)
	require.NoError(t, err)

	args := runner.ArgsForCall(0)
	require.NotNil(t, args)
	assert.True(t, runner.ArgsContainSequence(0, "--output-format", "stream-json"),
		"first call must include --output-format stream-json; got: %v", args)
	assert.True(t, runner.ArgsContainSequence(0, "--verbose"),
		"first call must include --verbose (required by the CLI for --print with --output-format=stream-json); got: %v", args)
	assert.True(t, runner.ArgsContainSequence(0, "--system-prompt", "sys"),
		"first call must include --system-prompt; got: %v", args)
	assert.Contains(t, args, "--exclude-dynamic-system-prompt-sections")
}

// TestPool_ResumedCall_ArgsContainResumeAndExclude verifies resumed-call args.
func TestPool_ResumedCall_ArgsContainResumeAndExclude(t *testing.T) {
	t.Parallel()
	runner := NewFakeRunner(
		firstCallJSON("sess-xyz", "first result"),
		"second result\n",
	)
	pool := newTestPool(PoolConfig{}, runner)

	_, _ = pool.CallBlocking(context.Background(), "f1", "sys", "prompt1", CallOptions{}, DiscardCost)
	_, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt2", CallOptions{}, DiscardCost)
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
	t.Parallel()
	runner := NewFakeRunner(firstCallJSON("s1", "ok"))
	pool := newTestPool(PoolConfig{DefaultModel: "claude-opus-4"}, runner)

	_, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt", CallOptions{}, DiscardCost)
	require.NoError(t, err)

	args := runner.ArgsForCall(0)
	assert.True(t, runner.ArgsContainSequence(0, "--model", "claude-opus-4"),
		"model flag must be present; got: %v", args)
}

// TestPool_ParsesSessionIDFromFirstCallJSON verifies session_id capture.
func TestPool_ParsesSessionIDFromFirstCallJSON(t *testing.T) {
	t.Parallel()
	runner := NewFakeRunner(firstCallJSON("abc", "hello"))
	pool := newTestPool(PoolConfig{}, runner)

	_, err := pool.CallBlocking(context.Background(), "f1", "", "prompt", CallOptions{}, DiscardCost)
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
	t.Parallel()
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

// TestPool_CallBlocking_ContextTimeout_ReturnsError_NotEmptySuccess covers the
// non-WorkDir (session-reuse) path, reusing the blockingRunner defined below
// (a ClaudeRunner whose stdout blocks until ctx is done — the exact shape of
// a genuinely hung headless call, e.g. blocked waiting on a tool-permission
// prompt no TTY can ever answer). Guards against the bug where a context
// timeout mid-call silently completed with empty output instead of a real
// error — TriggerTriage's headless calls ran for the full 30-minute timeout
// producing zero output, then failed downstream with a confusing "no JSON
// object found" parse error instead of a clear cancellation error.
func TestPool_CallBlocking_ContextTimeout_ReturnsError_NotEmptySuccess(t *testing.T) {
	t.Parallel()
	runner := &blockingRunner{}
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := pool.CallBlocking(ctx, "f1", "sys", "prompt", CallOptions{}, DiscardCost)

	require.Error(t, err, "a call cancelled mid-flight must return an error, not silently succeed with empty output")
	assert.Empty(t, result)
}

// writeSleepForeverFakeClaudeScript writes a fake `claude` binary that consumes
// stdin then blocks indefinitely without ever producing output — simulating a
// hang, for real-subprocess (ProcessRunner) testing of the WorkDir call path.
func writeSleepForeverFakeClaudeScript(t *testing.T, scriptDir string) string {
	t.Helper()
	scriptPath := filepath.Join(scriptDir, "fake-claude-hang.sh")
	script := "#!/bin/sh\ncat > /dev/null\nsleep 999\n"
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	return scriptPath
}

// TestPool_CallBlocking_WorkDirPath_ContextTimeout_ReturnsError_NotEmptySuccess
// covers the WorkDir (one-shot proxy) call path — the exact shape
// TriggerTriage uses in production — with a real subprocess that hangs and
// gets killed by ctx's timeout.
func TestPool_CallBlocking_WorkDirPath_ContextTimeout_ReturnsError_NotEmptySuccess(t *testing.T) {
	t.Parallel()
	scriptDir := t.TempDir()
	scriptPath := writeSleepForeverFakeClaudeScript(t, scriptDir)
	workDir := t.TempDir()

	runner := NewShellWrappedProcessRunnerForTesting(scriptPath)
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	result, err := pool.CallBlocking(ctx, "f1", "sys", "prompt", CallOptions{WorkDir: workDir}, DiscardCost)

	require.Error(t, err, "a WorkDir call cancelled mid-flight must return an error, not silently succeed with empty output")
	assert.Empty(t, result)
}

// TestPool_RotatesSession_AfterMaxCalls verifies session ID changes after MaxCallsPerSession.
func TestPool_RotatesSession_AfterMaxCalls(t *testing.T) {
	t.Parallel()
	// MaxCallsPerSession=2: after 2 calls the 3rd call should be a new session.
	runner := NewFakeRunner(
		firstCallJSON("session-A", "result1"),
		"result2\n",
		firstCallJSON("session-B", "result3"), // new session after rotation
	)
	pool := newTestPool(PoolConfig{MaxCallsPerSession: 2}, runner)

	_, _ = pool.CallBlocking(context.Background(), "f1", "sys", "p1", CallOptions{}, DiscardCost)
	_, _ = pool.CallBlocking(context.Background(), "f1", "sys", "p2", CallOptions{}, DiscardCost)
	_, _ = pool.CallBlocking(context.Background(), "f1", "sys", "p3", CallOptions{}, DiscardCost)

	// Third call args should be a first-call (--output-format json), not a resume.
	args := runner.ArgsForCall(2)
	assert.True(t, runner.ArgsContainSequence(2, "--output-format", "stream-json"),
		"third call should be fresh (rotation); got: %v", args)
}

// TestPool_RotatesSession_AfterConsecutiveErrors verifies circuit breaker.
func TestPool_RotatesSession_AfterConsecutiveErrors(t *testing.T) {
	t.Parallel()
	runner2 := &FakeRunner{
		responses: []string{firstCallJSON("sess1", "ok"), "", "", "", firstCallJSON("sess2", "after-reset")},
		errors:    []error{nil, fmt.Errorf("err1"), fmt.Errorf("err2"), fmt.Errorf("err3"), nil},
	}
	pool := newTestPool(PoolConfig{}, runner2)

	pool.CallBlocking(context.Background(), "f1", "sys", "p1", CallOptions{}, DiscardCost) //nolint:errcheck
	pool.CallBlocking(context.Background(), "f1", "sys", "p2", CallOptions{}, DiscardCost) //nolint:errcheck
	pool.CallBlocking(context.Background(), "f1", "sys", "p3", CallOptions{}, DiscardCost) //nolint:errcheck
	pool.CallBlocking(context.Background(), "f1", "sys", "p4", CallOptions{}, DiscardCost) //nolint:errcheck
	pool.CallBlocking(context.Background(), "f1", "sys", "p5", CallOptions{}, DiscardCost) //nolint:errcheck

	// After 3 consecutive errors, a subsequent call should be a fresh session.
	found := false
	for i := 1; i < runner2.CallCount(); i++ {
		if runner2.ArgsContainSequence(i, "--output-format", "stream-json") {
			found = true
			break
		}
	}
	assert.True(t, found, "expected a rotation (fresh first-call) after consecutive errors")
}

// TestPool_CallBlocking_ReturnsCollectedText verifies text concatenation.
func TestPool_CallBlocking_ReturnsCollectedText(t *testing.T) {
	t.Parallel()
	runner := NewFakeRunner(firstCallJSON("s1", "hello world"))
	pool := newTestPool(PoolConfig{}, runner)

	text, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt", CallOptions{}, DiscardCost)
	require.NoError(t, err)
	assert.Equal(t, "hello world", text)
}

// TestPool_Call_MultiLineOutput_StreamsInOrder verifies line-by-line streaming.
func TestPool_Call_MultiLineOutput_StreamsInOrder(t *testing.T) {
	t.Parallel()
	runner := NewFakeRunner(
		firstCallJSON("sess1", "first"),
		"line1\nline2\nline3\n",
	)
	pool := newTestPool(PoolConfig{}, runner)

	// First call to establish session.
	pool.CallBlocking(context.Background(), "f1", "sys", "p1", CallOptions{}, DiscardCost) //nolint:errcheck

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

// TestPool_CallBlocking_PropagatesSubprocessError verifies error propagation,
// and that a runner-start failure (os.Pipe()/cmd.Start() failing — e.g. under fd
// exhaustion or ENOMEM) is wrapped in ErrSubprocessStart so
// classifyHeadlessCallError (server/services/backlog_service_triage.go) can
// bucket it as "subprocess_start_error" instead of an undiagnosable "other".
func TestPool_CallBlocking_PropagatesSubprocessError(t *testing.T) {
	t.Parallel()
	startErr := errors.New("start failed")
	runner := &FakeRunner{
		errors: []error{startErr},
	}
	pool := newTestPool(PoolConfig{}, runner)

	_, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt", CallOptions{}, DiscardCost)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSubprocessStart, "runner-start failures must be classifiable, not swallowed into a generic error")
	assert.ErrorIs(t, err, startErr, "the underlying OS-level error must remain inspectable")
}

// partialErrReadCloser simulates a subprocess killed mid-write (e.g. OOM-killed):
// it returns real, useful data on the first Read, then a non-EOF error on the next.
type partialErrReadCloser struct {
	data []byte
	err  error
	sent bool
}

func (r *partialErrReadCloser) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		n := copy(p, r.data)
		return n, nil
	}
	return 0, r.err
}

func (r *partialErrReadCloser) Close() error { return nil }

// partialErrRunner always returns a partialErrReadCloser from Run.
type partialErrRunner struct {
	data []byte
	err  error
}

func (r *partialErrRunner) Run(_ context.Context, _ []string, _ io.Reader) (io.ReadCloser, func() error, error) {
	return &partialErrReadCloser{data: r.data, err: r.err}, func() error { return nil }, nil
}

// TestPool_CallBlocking_ReadError_ReturnsPartialDataAsRaw_When_SubprocessKilledMidWrite
// covers the fix mirroring the JSON-parse-failure branch's existing behavior:
// when the subprocess is killed mid-write, any real output already read before
// the pipe read failed must be delivered as a Text chunk before the terminal Err
// chunk. Before the fix, that partial data was discarded, so CallBlocking's raw
// return was always "" for this failure mode and captureHeadlessFailure
// (server/services/backlog_service_triage.go) had nothing to persist for
// diagnosis even though real output existed.
func TestPool_CallBlocking_ReadError_ReturnsPartialDataAsRaw_When_SubprocessKilledMidWrite(t *testing.T) {
	wantErr := errors.New("read |0: input/output error")
	runner := &partialErrRunner{data: []byte("partial output before kill"), err: wantErr}
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	raw, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt", CallOptions{}, DiscardCost)
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
	assert.Equal(t, "partial output before kill", raw)
}

// TestPool_DifferentKeys_RunInParallel verifies parallel execution on different keys.
func TestPool_DifferentKeys_RunInParallel(t *testing.T) {
	t.Parallel()
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
		r1, err1 = pool.CallBlocking(context.Background(), "key1", "sys", "p1", CallOptions{}, DiscardCost)
	}()
	go func() {
		defer wg.Done()
		r2, err2 = pool.CallBlocking(context.Background(), "key2", "sys", "p2", CallOptions{}, DiscardCost)
	}()
	wg.Wait()

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotEmpty(t, r1)
	assert.NotEmpty(t, r2)
}

// TestPool_SameKey_ConcurrentCalls_Serialized verifies concurrent access doesn't panic.
func TestPool_SameKey_ConcurrentCalls_Serialized(t *testing.T) {
	t.Parallel()
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
			pool.CallBlocking(context.Background(), "shared-key", "sys", "prompt", CallOptions{}, DiscardCost) //nolint:errcheck
		}()
	}
	wg.Wait()
	// No panic = pass.
}

// TestPool_ConcurrencySemaphore_LimitsToMax verifies semaphore limits concurrency.
func TestPool_ConcurrencySemaphore_LimitsToMax(t *testing.T) {
	t.Parallel()
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
			pool.CallBlocking(context.Background(), k, "sys", "p", CallOptions{}, DiscardCost) //nolint:errcheck
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
	t.Parallel()
	runner := NewFakeRunner()
	pool := NewPoolWithRunner(PoolConfig{MaxCallsPerSession: 0}, runner)
	assert.Equal(t, defaultMaxCalls, pool.cfg.MaxCallsPerSession)
}

// TestPool_DefaultPool_SetAndGet_ThreadSafe verifies concurrent DefaultPool access.
func TestPool_DefaultPool_SetAndGet_ThreadSafe(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	runner := NewFakeRunner(firstCallJSON("s1", "ok"))
	pool := newTestPool(PoolConfig{}, runner)

	result, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt", CallOptions{}, DiscardCost)
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
}

// TestPool_FirstCall_IsError_ReturnsErrorChunk verifies LLM-level error handling.
func TestPool_FirstCall_IsError_ReturnsErrorChunk(t *testing.T) {
	t.Parallel()
	errorJSON := `{"type":"result","session_id":"","result":"model refused to respond","is_error":true,"total_cost_usd":0}`
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
	t.Parallel()
	costJSON := `{"type":"result","session_id":"s1","result":"ok","is_error":false,"total_cost_usd":0.0042}`
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
	t.Parallel()
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
	t.Parallel()
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

// idlingLinesReader emits each of lines (newline-terminated as it returns them)
// with a pause of gap before each one — real pacing, not instant buffering, so a
// test can exercise idleTimeout's per-line timer reset. If blockForever is true,
// once lines is exhausted the reader blocks (until ctx is done OR killed is
// closed) instead of returning EOF, simulating a subprocess that produced some
// real output and then went silent — the exact shape idleTimeout exists to
// catch. killed is a SEPARATE signal from ctx: a real subprocess's stdout
// unblocks when the process is killed, which is independent of (and normally
// happens well before) ctx's own deadline — stop() closes killed to mimic that,
// so a test can use a ctx budget far longer than idleTimeout and still verify
// the reader actually gets unblocked once call() calls stop().
type idlingLinesReader struct {
	ctx          context.Context
	killed       chan struct{}
	lines        []string
	gap          time.Duration
	blockForever bool
	idx          int
	buf          []byte
}

// waitForEnd blocks until ctx is done or killed is closed, once lines is
// exhausted and blockForever is set — factored out so Read stays shallow.
func (r *idlingLinesReader) waitForEnd() (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	case <-r.killed:
		return 0, io.EOF
	}
}

func (r *idlingLinesReader) Read(p []byte) (int, error) {
	for len(r.buf) == 0 {
		if r.idx >= len(r.lines) {
			if r.blockForever {
				return r.waitForEnd()
			}
			return 0, io.EOF
		}
		select {
		case <-time.After(r.gap):
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		case <-r.killed:
			return 0, io.EOF
		}
		r.buf = []byte(r.lines[r.idx] + "\n")
		r.idx++
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

func (r *idlingLinesReader) Close() error { return nil }

// idlingLinesRunner is a ClaudeRunner backed by idlingLinesReader. stopped, if
// non-nil, is closed the first time stop() is invoked — lets a test assert the
// subprocess was actually killed (not just that the call errored).
type idlingLinesRunner struct {
	lines        []string
	gap          time.Duration
	blockForever bool
	stopped      chan struct{}
}

func (r *idlingLinesRunner) Run(ctx context.Context, _ []string, _ io.Reader) (io.ReadCloser, func() error, error) {
	killed := make(chan struct{})
	reader := &idlingLinesReader{ctx: ctx, killed: killed, lines: r.lines, gap: r.gap, blockForever: r.blockForever}
	// call() calls stop() at least twice on most exit paths (once explicitly,
	// once via its own deferred cleanup) — matching every real ClaudeRunner
	// implementation's idempotent stop(), this one must tolerate that too.
	var once sync.Once
	stop := func() error {
		once.Do(func() {
			close(killed)
			if r.stopped != nil {
				close(r.stopped)
			}
		})
		return nil
	}
	return reader, stop, nil
}

// TestPool_FirstCall_IdleTimeout_KillsStalledCall covers the 2026-09-08 fix
// (docs/tasks/backlog-feature-improvement.md, "why triage keeps churning" +
// follow-up): a first call that produces some real output and then goes
// silent must be killed and reported distinctly (ErrIdleTimeout) once
// idleTimeout elapses with no new line — not left to silently consume its
// entire (much larger) absolute ctx budget.
func TestPool_FirstCall_IdleTimeout_KillsStalledCall(t *testing.T) {
	// Not t.Parallel(): mutates the package-level idleTimeout var.
	origIdleTimeout := idleTimeout
	idleTimeout = 30 * time.Millisecond
	defer func() { idleTimeout = origIdleTimeout }()

	stopped := make(chan struct{})
	runner := &idlingLinesRunner{
		lines:        []string{`{"type":"system","subtype":"init"}`, `{"type":"assistant","message":"working"}`},
		gap:          5 * time.Millisecond,
		blockForever: true,
		stopped:      stopped,
	}
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	// ctx's own budget is far longer than idleTimeout — only idleTimeout should
	// be able to end this call.
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	_, err := pool.CallBlocking(ctx, "f1", "sys", "prompt", CallOptions{}, DiscardCost)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrIdleTimeout)
	assert.NotErrorIs(t, err, context.DeadlineExceeded,
		"an idle timeout must not be reported as the caller's own ctx expiring")

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("expected the stalled subprocess to be stopped")
	}
}

// TestPool_FirstCall_ActivityResetsIdleTimer_SucceedsWhileLinesKeepArriving is
// the inverse of the idle-timeout test above: as long as new lines keep
// arriving faster than idleTimeout, the call must succeed normally, even
// though the TOTAL elapsed time across all lines exceeds idleTimeout many
// times over — proving the timer resets per line rather than bounding the
// call's overall duration (that remains ctx's job).
func TestPool_FirstCall_ActivityResetsIdleTimer_SucceedsWhileLinesKeepArriving(t *testing.T) {
	// Not t.Parallel(): mutates the package-level idleTimeout var.
	origIdleTimeout := idleTimeout
	idleTimeout = 30 * time.Millisecond
	defer func() { idleTimeout = origIdleTimeout }()

	runner := &idlingLinesRunner{
		lines: []string{
			`{"type":"system","subtype":"init"}`,
			`{"type":"assistant","message":"step 1"}`,
			`{"type":"assistant","message":"step 2"}`,
			`{"type":"assistant","message":"step 3"}`,
			firstCallJSON("sess-idle", "done"),
		},
		gap: 10 * time.Millisecond, // under idleTimeout; 5 lines * 10ms > idleTimeout in total
	}
	pool := NewPoolWithRunner(PoolConfig{}, runner)

	result, err := pool.CallBlocking(context.Background(), "f1", "sys", "prompt", CallOptions{}, DiscardCost)
	require.NoError(t, err, "steady incremental activity under idleTimeout must not be killed")
	assert.Equal(t, "done", result)
}

// TestPool_CtxCancel_DuringSemaphoreWait_DecrementsCallCount verifies that a
// context cancellation while blocked on the concurrency semaphore does not
// permanently inflate callCount (decrementCallCount is called on the cancel path).
func TestPool_CtxCancel_DuringSemaphoreWait_DecrementsCallCount(t *testing.T) {
	t.Parallel()
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

// TestPool_QueueWaitTimeout_DuringSemaphoreWait_ReturnsErrPoolSaturated covers
// the 2026-09-08 fix (docs/tasks/backlog-feature-improvement.md): a call stuck
// behind other concurrent calls must fail fast with a distinct
// ErrPoolSaturated once maxQueueWait elapses, NOT silently consume its whole
// (much longer) caller-supplied budget and surface as an indistinguishable
// context.DeadlineExceeded once that budget finally expires. Mirrors
// TestPool_CtxCancel_DuringSemaphoreWait_DecrementsCallCount's shape (a
// manually-occupied slot blocks the next Call) but leaves ctx itself
// long-lived, so the only thing that can end the wait is maxQueueWait.
func TestPool_QueueWaitTimeout_DuringSemaphoreWait_ReturnsErrPoolSaturated(t *testing.T) {
	// Not t.Parallel(): mutates the package-level maxQueueWait var.
	origMaxQueueWait := maxQueueWait
	maxQueueWait = 20 * time.Millisecond
	defer func() { maxQueueWait = origMaxQueueWait }()

	runner := NewFakeRunner()
	// MaxConcurrentSessions=1 so a single manually-occupied slot blocks the next Call.
	pool := newTestPool(PoolConfig{MaxConcurrentSessions: 1, MaxCallsPerSession: 100}, runner)

	// Occupy the one semaphore slot so the next Call must block on acquire.
	pool.concurrencySem <- struct{}{}
	defer func() { <-pool.concurrencySem }()

	// ctx itself carries a budget far longer than maxQueueWait (mirrors
	// TriggerTriage's real 30-minute triageCallBudget) — nothing about ctx
	// should cause this call to fail; only the shorter internal queue-wait cap.
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	_, err := pool.Call(ctx, "f1", "sys", "prompt")

	require.Error(t, err, "expected error once maxQueueWait elapses while waiting for a slot")
	assert.ErrorIs(t, err, ErrPoolSaturated)
	assert.NotErrorIs(t, err, context.DeadlineExceeded,
		"a queue-wait timeout must not be reported as the caller's own ctx expiring")

	// callCount must be 0, same invariant as the ctx-cancel path above.
	pool.mu.Lock()
	state := pool.sessions["f1"]
	pool.mu.Unlock()
	require.NotNil(t, state, "session state must exist after Call")
	assert.Equal(t, 0, state.callCount, "callCount must be 0 after queue-wait timeout")
}

// TestPool_CallerCtxShorterThanQueueWait_PreservesRealCtxError verifies the
// other branch of the same fix: when the CALLER's own ctx is what actually
// expires first (shorter than maxQueueWait, or genuinely cancelled), that
// real signal must be preserved as-is — not masked as ErrPoolSaturated.
func TestPool_CallerCtxShorterThanQueueWait_PreservesRealCtxError(t *testing.T) {
	// Not t.Parallel(): mutates the package-level maxQueueWait var.
	origMaxQueueWait := maxQueueWait
	maxQueueWait = time.Hour // must not be what fires first in this test
	defer func() { maxQueueWait = origMaxQueueWait }()

	runner := NewFakeRunner()
	pool := newTestPool(PoolConfig{MaxConcurrentSessions: 1, MaxCallsPerSession: 100}, runner)

	pool.concurrencySem <- struct{}{}
	defer func() { <-pool.concurrencySem }()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := pool.Call(ctx, "f1", "sys", "prompt")

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotErrorIs(t, err, ErrPoolSaturated,
		"the caller's own ctx expiring must not be relabeled as pool saturation")
}

// TestPool_CallWithOptions_WorkDir_QueueWaitTimeout_ReturnsErrPoolSaturated
// covers the fix for CallWithOptions's WorkDir branch: its semaphore acquire
// used to have only a ctx.Done() case, never maxQueueWait, so BUG-093's fix
// never actually applied to a real WorkDir caller (triage, review, PR
// creation). Mirrors TestPool_QueueWaitTimeout_DuringSemaphoreWait_
// ReturnsErrPoolSaturated but through the WorkDir path.
func TestPool_CallWithOptions_WorkDir_QueueWaitTimeout_ReturnsErrPoolSaturated(t *testing.T) {
	// Not t.Parallel(): mutates the package-level maxQueueWait var.
	origMaxQueueWait := maxQueueWait
	maxQueueWait = 20 * time.Millisecond
	defer func() { maxQueueWait = origMaxQueueWait }()

	// The queue-wait must time out before ever reaching runner.Run, so a bare
	// ProcessRunner (never actually exec'd) is enough to satisfy the WorkDir
	// branch's type assertion.
	runner := &ProcessRunner{claudeBin: "unused-since-queue-wait-must-fail-first"}
	pool := NewPoolWithRunner(PoolConfig{MaxConcurrentSessions: 1}, runner)

	// Occupy the one semaphore slot so the WorkDir call must block on acquire.
	pool.concurrencySem <- struct{}{}
	defer func() { <-pool.concurrencySem }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	_, err := pool.CallWithOptions(ctx, "f1", "sys", "prompt", CallOptions{WorkDir: t.TempDir()})

	require.Error(t, err, "expected error once maxQueueWait elapses while waiting for a slot on the WorkDir path")
	assert.ErrorIs(t, err, ErrPoolSaturated)
	assert.NotErrorIs(t, err, context.DeadlineExceeded,
		"a queue-wait timeout must not be reported as the caller's own ctx expiring")
}

// TestFakeRunner_InspectsArgs_ReturnsPlainForResumedCall verifies resumed-call plain text.
func TestFakeRunner_InspectsArgs_ReturnsPlainForResumedCall(t *testing.T) {
	t.Parallel()
	runner := NewFakeRunner(
		firstCallJSON("sess1", "first"),
		"plain text response\n",
	)
	pool := newTestPool(PoolConfig{}, runner)

	pool.CallBlocking(context.Background(), "f1", "sys", "p1", CallOptions{}, DiscardCost) //nolint:errcheck
	result, err := pool.CallBlocking(context.Background(), "f1", "sys", "p2", CallOptions{}, DiscardCost)
	require.NoError(t, err)
	assert.Contains(t, result, "plain text response")
}

// TestPool_CallBlocking_ZeroValueOptions_MatchesLegacyCallBlockingBehavior verifies
// that calling the consolidated CallBlocking with a zero-value CallOptions{}
// reproduces the pre-consolidation simplest-call behavior: the session-reuse
// path is used (not the WorkDir one-shot path), the session ID from the
// first-call JSON response is captured, the returned text matches the JSON
// result, and cost_usd is forwarded even though earlier callers ignored it.
func TestPool_CallBlocking_ZeroValueOptions_MatchesLegacyCallBlockingBehavior(t *testing.T) {
	t.Parallel()
	runner := NewFakeRunner(firstCallJSON("zero-value-session", "hello"))
	pool := newTestPool(PoolConfig{MaxCallsPerSession: 25}, runner)

	var cost float64
	result, err := pool.CallBlocking(context.Background(), "feat-zero", "system", "user prompt", CallOptions{}, func(usd float64) { cost = usd })
	require.NoError(t, err)
	assert.Equal(t, "hello", result)
	assert.InDelta(t, 0.001, cost, 1e-9, "cost_usd from the JSON result must be forwarded")

	pool.mu.Lock()
	state := pool.sessions["feat-zero"]
	pool.mu.Unlock()
	require.NotNil(t, state)
	assert.Equal(t, "zero-value-session", state.sessionID,
		"zero-value opts must still capture the session ID like the pre-consolidation CallBlocking")

	args := runner.ArgsForCall(0)
	assert.True(t, runner.ArgsContainSequence(0, "--output-format", "stream-json"),
		"zero-value opts must produce a normal first-call via the session-reuse path (no WorkDir one-shot); got: %v", args)
}

// TestPool_CallBlocking_WithWorkDir_ReturnsCostAndUsesWorkDir verifies that
// CallOptions{WorkDir: ...} routes CallBlocking through the one-shot
// ProcessRunner path, runs the subprocess with its working directory set to
// opts.WorkDir, and still returns the cost reported in the JSON result —
// exercising the same code path CallBlockingWithOptions used before
// consolidation.
func TestPool_CallBlocking_WithWorkDir_ReturnsCostAndUsesWorkDir(t *testing.T) {
	t.Parallel()
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "fake-claude.sh")
	script := "#!/bin/sh\necho \"{\\\"type\\\":\\\"result\\\",\\\"session_id\\\":\\\"wd1\\\",\\\"result\\\":\\\"$(pwd)\\\",\\\"total_cost_usd\\\":0.0077}\"\n"
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))

	workDir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)

	runner := NewShellWrappedProcessRunnerForTesting(scriptPath)
	pool := NewPoolWithRunner(PoolConfig{MaxCallsPerSession: 25, MaxConcurrentSessions: 2}, runner)

	var cost float64
	result, err := pool.CallBlocking(context.Background(), "feat-workdir", "sys", "prompt", CallOptions{WorkDir: workDir}, func(usd float64) { cost = usd })
	require.NoError(t, err)
	assert.Equal(t, workDir, result, "subprocess must run with cwd set to opts.WorkDir")
	assert.InDelta(t, 0.0077, cost, 1e-9, "cost_usd must be returned for WorkDir calls too")
}
