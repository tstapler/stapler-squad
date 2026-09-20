package headless

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/tokens"
)

// fakeGeminiRunner is a test double for geminiRunner: returns scripted
// responses/errors in order and records the args/workDir passed to Run, and
// can optionally block until a caller-supplied signal so tests can hold a
// concurrency slot open (mirrors FakeRunner's role for ClaudeRunner).
type fakeGeminiRunner struct {
	responses []string
	errs      []error
	index     int

	calls    [][]string
	workDirs []string

	// block, if non-nil, is closed by the test to release a call that's
	// intentionally stalled inside Run — used only by the pool-saturation test.
	block <-chan struct{}
}

func (f *fakeGeminiRunner) Run(_ context.Context, _ string, args []string, workDir string) ([]byte, error) {
	if f.block != nil {
		<-f.block
	}
	argsCopy := make([]string, len(args))
	copy(argsCopy, args)
	f.calls = append(f.calls, argsCopy)
	f.workDirs = append(f.workDirs, workDir)

	idx := f.index
	f.index++

	var err error
	if idx < len(f.errs) {
		err = f.errs[idx]
	}
	var resp string
	if idx < len(f.responses) {
		resp = f.responses[idx]
	}
	return []byte(resp), err
}

// testPricingTable returns a fixture pricing table with a gemini-2.5-pro
// entry — Epic 3.2 (not yet landed) adds the real Gemini pricing rows to the
// canonical DefaultPricingTable(); this fixture is sufficient to exercise
// GeminiCaller's cost computation independent of that epic's landing.
func testGeminiPricingTable() *tokens.PricingTable {
	return &tokens.PricingTable{
		Prices: map[string]tokens.ModelPricing{
			"gemini-2.5-pro": {
				ModelFamily:        "gemini-2.5-pro",
				InputPricePerMTok:  1.25,
				OutputPricePerMTok: 5.00,
			},
		},
	}
}

func newTestGeminiCaller(runner geminiRunner, maxConcurrent int) *GeminiCaller {
	gc := NewGeminiCaller("gemini", testGeminiPricingTable(), maxConcurrent)
	gc.runner = runner
	return gc
}

// ─── CallBlocking: success/pricing ──────────────────────────────────────────

func TestGeminiCaller_CallBlocking_should_InvokeSinkWithPricedTrue_When_ResponseHasKnownModelTokens(t *testing.T) {
	t.Parallel()
	runner := &fakeGeminiRunner{responses: []string{
		`{"response":"ok","stats":{"models":{"gemini-2.5-pro":{"tokens":{"prompt":1000,"candidates":500,"total":1500}}}}}`,
	}}
	gc := newTestGeminiCaller(runner, 5)

	var gotCost float64
	var gotPriced bool
	sink := func(usd float64, priced bool) { gotCost, gotPriced = usd, priced }

	text, err := gc.CallBlocking(context.Background(), "key", "sys", "user", CallOptions{}, sink)
	require.NoError(t, err)
	assert.Equal(t, "ok", text)
	assert.True(t, gotPriced)
	wantCost := 1000.0/1_000_000.0*1.25 + 500.0/1_000_000.0*5.00
	assert.InDelta(t, wantCost, gotCost, 1e-9)
}

func TestGeminiCaller_CallBlocking_should_InvokeSinkWithPricedFalse_When_ModelFamilyUnrecognized(t *testing.T) {
	t.Parallel()
	runner := &fakeGeminiRunner{responses: []string{
		`{"response":"ok","stats":{"models":{"gemini-3.0-ultra":{"tokens":{"prompt":100,"candidates":50,"total":150}}}}}`,
	}}
	gc := newTestGeminiCaller(runner, 5)

	var gotCost float64
	var gotPriced bool
	sinkCalls := 0
	sink := func(usd float64, priced bool) { gotCost, gotPriced = usd, priced; sinkCalls++ }

	_, err := gc.CallBlocking(context.Background(), "key", "sys", "user", CallOptions{}, sink)
	require.NoError(t, err)
	assert.Equal(t, 1, sinkCalls, "sink must be invoked exactly once")
	assert.False(t, gotPriced)
	assert.Zero(t, gotCost, "an unpriced family must never report a fabricated non-zero cost")
}

func TestGeminiCaller_CallBlocking_should_InvokeSinkWithPricedFalse_When_ResponseMixesPricedAndUnpricedModels(t *testing.T) {
	t.Parallel()
	runner := &fakeGeminiRunner{responses: []string{
		`{"response":"ok","stats":{"models":{
			"gemini-2.5-pro":{"tokens":{"prompt":1000,"candidates":500,"total":1500}},
			"gemini-3.0-ultra":{"tokens":{"prompt":100,"candidates":50,"total":150}}
		}}}`,
	}}
	gc := newTestGeminiCaller(runner, 5)

	var gotCost float64
	var gotPriced bool
	sinkCalls := 0
	sink := func(usd float64, priced bool) { gotCost, gotPriced = usd, priced; sinkCalls++ }

	_, err := gc.CallBlocking(context.Background(), "key", "sys", "user", CallOptions{}, sink)
	require.NoError(t, err)
	assert.Equal(t, 1, sinkCalls, "sink must be invoked exactly once even for a mixed response")
	assert.False(t, gotPriced, "any unpriced family in the response makes the whole call unpriced")
	wantPricedPortion := 1000.0/1_000_000.0*1.25 + 500.0/1_000_000.0*5.00
	assert.InDelta(t, wantPricedPortion, gotCost, 1e-9, "the partial-priced-sum is still reported, just marked unpriced overall")
}

// ─── CallBlocking: error paths ──────────────────────────────────────────────

func TestGeminiCaller_CallBlocking_should_ReturnNonNilError_When_ResponseContainsErrorObject(t *testing.T) {
	t.Parallel()
	runner := &fakeGeminiRunner{responses: []string{
		`{"error":{"type":"rate_limit","message":"quota exceeded","code":429}}`,
	}}
	gc := newTestGeminiCaller(runner, 5)

	text, err := gc.CallBlocking(context.Background(), "key", "sys", "user", CallOptions{}, DiscardCost)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "quota exceeded")
	assert.Empty(t, text)
}

func TestGeminiCaller_CallBlocking_should_ReturnErrorNotPanic_When_ResponseIsMalformedJSON(t *testing.T) {
	t.Parallel()
	runner := &fakeGeminiRunner{responses: []string{"not json at all {{{"}}
	gc := newTestGeminiCaller(runner, 5)

	text, err := gc.CallBlocking(context.Background(), "key", "sys", "user", CallOptions{}, DiscardCost)
	require.Error(t, err)
	assert.Empty(t, text)
}

func TestGeminiCaller_CallBlocking_should_ReturnError_When_SubprocessFailsToStart(t *testing.T) {
	t.Parallel()
	runner := &fakeGeminiRunner{errs: []error{errors.New("exec: fork/exec failed")}}
	gc := newTestGeminiCaller(runner, 5)

	_, err := gc.CallBlocking(context.Background(), "key", "sys", "user", CallOptions{}, DiscardCost)
	require.Error(t, err)
}

// ─── CallBlocking: ignored Claude-only CallOptions fields ──────────────────

func TestGeminiCaller_CallBlocking_should_WarnAndSucceed_When_ClaudeOnlyCallOptionFieldSet(t *testing.T) {
	t.Parallel()
	runner := &fakeGeminiRunner{responses: []string{`{"response":"ok","stats":{"models":{}}}`}}
	gc := newTestGeminiCaller(runner, 5)

	// No assertion on the log output itself (no test hook into the log package
	// here) — this asserts the documented behavior that matters operationally:
	// the call still succeeds using only WorkDir/Model, never errors just
	// because a Claude-only field was set.
	text, err := gc.CallBlocking(context.Background(), "key", "sys", "user",
		CallOptions{PermissionMode: "acceptEdits", AllowedTools: "Read,Grep"}, DiscardCost)
	require.NoError(t, err)
	assert.Equal(t, "ok", text)
}

// ─── CallBlocking: WorkDir validation (BUG-062 precedent) ──────────────────

func TestGeminiCaller_CallBlocking_should_RejectNonAbsoluteWorkDir_When_WorkDirIsRelative(t *testing.T) {
	t.Parallel()
	runner := &fakeGeminiRunner{}
	gc := newTestGeminiCaller(runner, 5)

	_, err := gc.CallBlocking(context.Background(), "key", "sys", "user", CallOptions{WorkDir: "relative/path"}, DiscardCost)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not an absolute path")
	assert.Empty(t, runner.calls, "the subprocess must never be started for an invalid WorkDir")
}

func TestGeminiCaller_CallBlocking_should_RejectNonExistentWorkDir_When_WorkDirDoesNotExist(t *testing.T) {
	t.Parallel()
	runner := &fakeGeminiRunner{}
	gc := newTestGeminiCaller(runner, 5)

	_, err := gc.CallBlocking(context.Background(), "key", "sys", "user", CallOptions{WorkDir: "/nonexistent/definitely/not/here"}, DiscardCost)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")
	assert.Empty(t, runner.calls)
}

// ─── CallBlocking: pool saturation (BUG-093 precedent) ─────────────────────

func TestGeminiCaller_CallBlocking_should_ReturnPoolSaturatedError_When_ConcurrencyLimitExceededAndQueueWaitElapses(t *testing.T) {
	// Not t.Parallel(): mutates the package-level maxQueueWait var.
	origMaxQueueWait := maxQueueWait
	maxQueueWait = 20 * time.Millisecond
	defer func() { maxQueueWait = origMaxQueueWait }()

	runner := &fakeGeminiRunner{}
	gc := newTestGeminiCaller(runner, 1)

	// Occupy the one semaphore slot so the next CallBlocking must block on acquire.
	gc.concurrencySem <- struct{}{}
	defer func() { <-gc.concurrencySem }()

	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	_, err := gc.CallBlocking(ctx, "key", "sys", "user", CallOptions{}, DiscardCost)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPoolSaturated)
	assert.NotErrorIs(t, err, context.DeadlineExceeded,
		"a queue-wait timeout must not be reported as the caller's own ctx expiring")
}

func TestGeminiCaller_CallBlocking_should_PreserveCallerCtxError_When_CallerCtxExpiresBeforeQueueWait(t *testing.T) {
	// Not t.Parallel(): mutates the package-level maxQueueWait var.
	origMaxQueueWait := maxQueueWait
	maxQueueWait = time.Hour // must not be what fires first in this test
	defer func() { maxQueueWait = origMaxQueueWait }()

	runner := &fakeGeminiRunner{}
	gc := newTestGeminiCaller(runner, 1)

	gc.concurrencySem <- struct{}{}
	defer func() { <-gc.concurrencySem }()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := gc.CallBlocking(ctx, "key", "sys", "user", CallOptions{}, DiscardCost)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotErrorIs(t, err, ErrPoolSaturated)
}

// ─── Available(): TTL/cache behavior ────────────────────────────────────────

func TestGeminiCaller_Available_should_ReturnTrue_When_BinaryPresent(t *testing.T) {
	t.Parallel()
	gc := newTestGeminiCaller(&fakeGeminiRunner{}, 5)
	gc.lookPath = func(string) (string, error) { return "/usr/local/bin/gemini", nil }
	gc.now = time.Now

	assert.True(t, gc.Available())
}

func TestGeminiCaller_Available_should_ReturnFalse_When_BinaryAbsent(t *testing.T) {
	t.Parallel()
	gc := newTestGeminiCaller(&fakeGeminiRunner{}, 5)
	gc.lookPath = func(string) (string, error) { return "", errors.New("not found") }
	gc.now = time.Now

	assert.False(t, gc.Available())
}

func TestGeminiCaller_Available_should_ReuseCachedResult_When_CalledWithinTTL(t *testing.T) {
	t.Parallel()
	gc := newTestGeminiCaller(&fakeGeminiRunner{}, 5)
	calls := 0
	gc.lookPath = func(string) (string, error) {
		calls++
		return "/usr/local/bin/gemini", nil
	}
	fakeNow := time.Now()
	gc.now = func() time.Time { return fakeNow }

	require.True(t, gc.Available())
	fakeNow = fakeNow.Add(geminiAvailabilityTTL / 2)
	require.True(t, gc.Available())
	assert.Equal(t, 1, calls, "a call within the TTL window must reuse the cached result, not re-probe")
}

func TestGeminiCaller_Available_should_ReProbe_When_TTLElapsed(t *testing.T) {
	t.Parallel()
	gc := newTestGeminiCaller(&fakeGeminiRunner{}, 5)
	calls := 0
	gc.lookPath = func(string) (string, error) {
		calls++
		if calls == 1 {
			return "/usr/local/bin/gemini", nil
		}
		return "", errors.New("uninstalled")
	}
	fakeNow := time.Now()
	gc.now = func() time.Time { return fakeNow }

	require.True(t, gc.Available())
	fakeNow = fakeNow.Add(geminiAvailabilityTTL + time.Second)
	require.False(t, gc.Available(), "a re-probe past the TTL must reflect the binary going missing")
	assert.Equal(t, 2, calls)
}
