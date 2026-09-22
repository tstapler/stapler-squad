//go:build !windows

package clihelp

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/testutil/wait"
)

const twoFlagHelp = "Usage: tool [options]\n\n  --alpha        first flag\n  -b, --beta VAL  second flag\n"

// fakeRun counts calls, records limits and tracks the peak concurrency.
type fakeRun struct {
	calls   atomic.Int32
	live    atomic.Int32
	peak    atomic.Int32
	mu      sync.Mutex
	limits  []Limits
	ctxErrs []error
	started chan ResolvedPath // buffered; one send per call
	release chan struct{}     // nil: return immediately
	out     RunOutput
	err     error
}

func newFakeRun(text string) *fakeRun {
	return &fakeRun{started: make(chan ResolvedPath, 64), out: RunOutput{Text: HelpText(text)}}
}

func (f *fakeRun) run(ctx context.Context, path ResolvedPath, lim Limits) (RunOutput, error) {
	f.calls.Add(1)
	n := f.live.Add(1)
	for p := f.peak.Load(); n > p && !f.peak.CompareAndSwap(p, n); p = f.peak.Load() {
	}
	defer f.live.Add(-1)
	f.mu.Lock()
	f.limits = append(f.limits, lim)
	f.mu.Unlock()
	f.started <- path
	if f.release != nil {
		<-f.release
	}
	f.mu.Lock()
	f.ctxErrs = append(f.ctxErrs, ctx.Err())
	f.mu.Unlock()
	return f.out, f.err
}

func runProber(t *testing.T, dirs []string, f *fakeRun, head []byte, opts ...Option) *Prober {
	t.Helper()
	all := append([]Option{WithRun(f.run), WithReadHead(func(string) ([]byte, error) { return head, nil })}, opts...)
	return hermeticProber(t, dirs, all...)
}

func toolIn(t *testing.T, name string) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	return dir, writeExec(t, dir, name, 0o755)
}

func TestProbe_should_ParseFlagsAndCacheByMtime_When_RunnerConfigured(t *testing.T) {
	dir, path := toolIn(t, "tool")
	f := newFakeRun(twoFlagHelp)
	p := runProber(t, []string{dir}, f, elfHead())

	res := p.Probe(context.Background(), "tool --whatever", ProbeOpts{})
	assert.Equal(t, ProbeStatusFoundParsed, res.Status)
	assert.Equal(t, ResolvedPath(path), res.ResolvedPath)
	require.Len(t, res.Flags, 2)
	assert.Equal(t, "--alpha", res.Flags[0].Name)
	assert.False(t, res.CacheHit)

	again := p.Probe(context.Background(), "tool", ProbeOpts{})
	assert.True(t, again.CacheHit)
	assert.Equal(t, res.Flags, again.Flags)
	assert.EqualValues(t, 1, f.calls.Load())

	require.NoError(t, os.Chtimes(path, time.Now(), time.Now().Add(time.Hour)))
	p.Probe(context.Background(), "tool", ProbeOpts{})
	assert.EqualValues(t, 2, f.calls.Load(), "a changed mtime invalidates the cache")
}

func TestProbe_should_ReturnErrorAndKeepSlots_When_RunnerPanics(t *testing.T) {
	dir, _ := toolIn(t, "tool")
	p := hermeticProber(t, []string{dir},
		WithRun(func(context.Context, ResolvedPath, Limits) (RunOutput, error) { panic("boom") }),
		WithReadHead(func(string) ([]byte, error) { return elfHead(), nil }))

	// More probes than maxConcurrentRuns: a leaked slot would surface as BUSY.
	for i := 0; i < maxConcurrentRuns+1; i++ {
		res := p.Probe(context.Background(), "tool", ProbeOpts{})
		assert.Equal(t, ProbeStatusError, res.Status, "probe %d", i)
	}
}

func TestProbe_should_ReportNoFlagsAndTruncated_When_HelpHasNoFlags(t *testing.T) {
	dir, _ := toolIn(t, "tool")
	f := newFakeRun("nothing to see here\n")
	f.out.Truncated = true
	res := runProber(t, []string{dir}, f, elfHead()).Probe(context.Background(), "tool", ProbeOpts{})
	assert.Equal(t, ProbeStatusFoundNoFlags, res.Status)
	assert.True(t, res.Truncated)
}

func TestProbe_should_PassConfiguredLimitsToRunner_When_WithLimits(t *testing.T) {
	dir, _ := toolIn(t, "tool")
	f := newFakeRun(twoFlagHelp)
	want := Limits{Timeout: 80 * time.Millisecond, MaxBytes: 1024}
	runProber(t, []string{dir}, f, elfHead(), WithLimits(want)).Probe(context.Background(), "tool", ProbeOpts{})
	assert.Equal(t, []Limits{want}, f.limits)

	f2 := newFakeRun(twoFlagHelp)
	runProber(t, []string{dir}, f2, elfHead()).Probe(context.Background(), "tool", ProbeOpts{})
	assert.Equal(t, []Limits{DefaultLimits()}, f2.limits)
}

func TestProbe_should_CacheTimeoutFor60sAndBypassOnConfirm_When_RunTimesOut(t *testing.T) {
	dir, _ := toolIn(t, "tool")
	f := newFakeRun("")
	f.out = RunOutput{TimedOut: true}
	clock := &fakeClock{t: time.Unix(1000, 0)}
	p := runProber(t, []string{dir}, f, elfHead(), WithClock(clock.Now))

	assert.Equal(t, ProbeStatusTimeout, p.Probe(context.Background(), "tool", ProbeOpts{}).Status)
	assert.Equal(t, ProbeStatusTimeout, p.Probe(context.Background(), "tool", ProbeOpts{}).Status)
	assert.EqualValues(t, 1, f.calls.Load(), "served from cache")

	p.Probe(context.Background(), "tool", ProbeOpts{ConfirmExecute: true})
	assert.EqualValues(t, 2, f.calls.Load(), "an explicit Check bypasses a cached TIMEOUT")

	clock.Advance(59 * time.Second)
	p.Probe(context.Background(), "tool", ProbeOpts{})
	assert.EqualValues(t, 2, f.calls.Load())
	clock.Advance(2 * time.Second)
	p.Probe(context.Background(), "tool", ProbeOpts{})
	assert.EqualValues(t, 3, f.calls.Load(), "TIMEOUT expires after 60s")
}

func TestProbe_should_ExpireParsedResultAfterTenMinutes_When_FakeClockAdvances(t *testing.T) {
	dir, _ := toolIn(t, "tool")
	f := newFakeRun(twoFlagHelp)
	clock := &fakeClock{t: time.Unix(1000, 0)}
	p := runProber(t, []string{dir}, f, elfHead(), WithClock(clock.Now))
	p.Probe(context.Background(), "tool", ProbeOpts{})
	clock.Advance(9 * time.Minute)
	p.Probe(context.Background(), "tool", ProbeOpts{})
	assert.EqualValues(t, 1, f.calls.Load())
	clock.Advance(2 * time.Minute)
	p.Probe(context.Background(), "tool", ProbeOpts{})
	assert.EqualValues(t, 2, f.calls.Load())
}

func TestProbe_should_NeverCacheError_When_RunFails(t *testing.T) {
	dir, _ := toolIn(t, "tool")
	f := newFakeRun("")
	f.err = errors.New("start failed")
	p := runProber(t, []string{dir}, f, elfHead())
	for i := 0; i < 2; i++ {
		res := p.Probe(context.Background(), "tool", ProbeOpts{})
		assert.Equal(t, ProbeStatusError, res.Status)
		assert.NotEmpty(t, res.ResolvedPath)
	}
	assert.EqualValues(t, 2, f.calls.Load())
}

func TestProbe_should_NeverRunOrCache_When_NotFoundOrWrapperWithRunner(t *testing.T) {
	dir, _ := toolIn(t, "env")
	f := newFakeRun(twoFlagHelp)
	p := runProber(t, []string{dir}, f, elfHead())
	assert.Equal(t, ProbeStatusNotFound, p.Probe(context.Background(), "nope", ProbeOpts{}).Status)
	assert.True(t, p.Probe(context.Background(), "env A=b claude", ProbeOpts{}).IsWrapper)
	assert.Zero(t, f.calls.Load())
}

func TestProbe_should_LogFlagsCacheHitAndTruncated_When_ProbedTwice(t *testing.T) {
	dir, _ := toolIn(t, "tool")
	f := newFakeRun(twoFlagHelp)
	f.out.Truncated = true
	p := runProber(t, []string{dir}, f, elfHead())

	first := captureLogs(t)
	p.Probe(context.Background(), "tool", ProbeOpts{})
	assert.Contains(t, first.String(), "flags=2")
	assert.Contains(t, first.String(), "cache_hit=false")
	assert.Contains(t, first.String(), "truncated=true")

	second := captureLogs(t)
	p.Probe(context.Background(), "tool", ProbeOpts{})
	assert.Contains(t, second.String(), "cache_hit=true")
}

// cancelledProbe starts a probe on a cancellable ctx and returns its result channel.
func probeAsync(p *Prober, ctx context.Context, cmd string) <-chan ProbeResult {
	ch := make(chan ProbeResult, 1)
	go func() { ch <- p.Probe(ctx, cmd, ProbeOpts{}) }()
	return ch
}

func recv[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(wait.ScaleTimeout(10 * time.Second)):
		require.FailNow(t, "timed out waiting for channel")
	}
	panic("unreachable")
}

func TestProbe_should_ReturnErrorThenRealResult_When_LeaderCancelledMidProbe(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	dir, _ := toolIn(t, "tool")
	f := newFakeRun(twoFlagHelp)
	f.release = make(chan struct{})
	p := runProber(t, []string{dir}, f, elfHead())

	ctx, cancel := context.WithCancel(context.Background())
	leader := probeAsync(p, ctx, "tool")
	recv(t, f.started)
	cancel()
	assert.Equal(t, ProbeStatusError, recv(t, leader).Status)
	assert.Empty(t, p.cache.entries, "nothing is cached for a cancelled call")

	follower := probeAsync(p, context.Background(), "tool") // joins the still-running flight
	close(f.release)
	got := recv(t, follower)
	assert.Equal(t, ProbeStatusFoundParsed, got.Status, "a cancelled leader must not poison followers")

	again := p.Probe(context.Background(), "tool", ProbeOpts{})
	assert.Equal(t, ProbeStatusFoundParsed, again.Status)
	assert.EqualValues(t, 1, f.calls.Load())
	assert.NoError(t, f.ctxErrs[0], "the flight runs on a context that survives caller cancellation")

	wait.RequireEventually(t, func() bool { return len(p.sem) == 0 }, 5*time.Second, 5*time.Millisecond)
}

func TestProbe_should_ExitFlight_When_AllCallersCancelled(t *testing.T) {
	base := goleak.IgnoreCurrent()
	dir, _ := toolIn(t, "tool")
	f := newFakeRun(twoFlagHelp)
	f.release = make(chan struct{})
	p := runProber(t, []string{dir}, f, elfHead())

	ctx, cancel := context.WithCancel(context.Background())
	a, b := probeAsync(p, ctx, "tool"), probeAsync(p, ctx, "tool")
	recv(t, f.started)
	cancel()
	recv(t, a)
	recv(t, b)
	assert.Len(t, p.sem, 1, "the slot is held by the flight, not by the cancelled callers")

	close(f.release)
	wait.RequireEventually(t, func() bool { return len(p.sem) == 0 && goleak.Find(base) == nil },
		5*time.Second, 5*time.Millisecond, "flight goroutine exits and frees its slot")
	res := p.Probe(context.Background(), "tool", ProbeOpts{})
	assert.Equal(t, ProbeStatusFoundParsed, res.Status, "the orphaned flight still cached its result")
	assert.True(t, res.CacheHit)
}

func TestProbe_should_GiveAllTenCallersTheRealResult_When_SameBinaryProbedConcurrently(t *testing.T) {
	dir, _ := toolIn(t, "tool")
	f := newFakeRun(twoFlagHelp)
	f.release = make(chan struct{})
	p := runProber(t, []string{dir}, f, elfHead())

	var chans []<-chan ProbeResult
	for i := 0; i < 10; i++ {
		chans = append(chans, probeAsync(p, context.Background(), "tool"))
	}
	recv(t, f.started)
	close(f.release)
	for _, ch := range chans {
		assert.Equal(t, ProbeStatusFoundParsed, recv(t, ch).Status)
	}
	assert.EqualValues(t, 1, f.calls.Load())
}

func TestProbe_should_ReturnBusyUncachedAndCapConcurrency_When_TwoFlightsHoldBothSlots(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a", "b", "c"} {
		writeExec(t, dir, n, 0o755)
	}
	f := newFakeRun(twoFlagHelp)
	f.release = make(chan struct{})
	p := runProber(t, []string{dir}, f, elfHead())

	ctxA, cancelA := context.WithCancel(context.Background())
	ctxB, cancelB := context.WithCancel(context.Background())
	resA, resB := probeAsync(p, ctxA, "a"), probeAsync(p, ctxB, "b")
	recv(t, f.started)
	recv(t, f.started)
	cancelA()
	cancelB()
	recv(t, resA)
	recv(t, resB)

	busy := p.Probe(context.Background(), "c", ProbeOpts{})
	assert.Equal(t, ProbeStatusBusy, busy.Status, "cancelled callers do not free a slot; only flight completion does")

	close(f.release)
	var res ProbeResult
	wait.RequireEventually(t, func() bool {
		res = p.Probe(context.Background(), "c", ProbeOpts{})
		return res.Status != ProbeStatusBusy
	}, 5*time.Second, 5*time.Millisecond)
	assert.Equal(t, ProbeStatusFoundParsed, res.Status, "BUSY was not cached")
	assert.LessOrEqual(t, f.peak.Load(), int32(maxConcurrentRuns))
}

func TestProbe_should_RunHelpOnResolvedPathOnce_When_CommandHasExtraTokens(t *testing.T) {
	dir, path := toolIn(t, "claude")
	f := newFakeRun(twoFlagHelp)
	p := runProber(t, []string{dir}, f, elfHead())
	p.Probe(context.Background(), "claude --dangerously-skip-permissions", ProbeOpts{})
	assert.Equal(t, ResolvedPath(path), recvNow(t, f.started))
	assert.EqualValues(t, 1, f.calls.Load())
}

func recvNow[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	default:
		require.FailNow(t, "nothing received")
	}
	panic("unreachable")
}

func elfHead() []byte   { return []byte{0x7f, 'E', 'L', 'F'} }
func shebangHd() []byte { return []byte("#!/b") }
