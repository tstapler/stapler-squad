package slowops

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockedInSlowOpTestHelper is the frame the dump must show for the operation that is stuck.
//
//go:noinline
func blockedInSlowOpTestHelper(entered chan<- struct{}, release <-chan struct{}) {
	close(entered)
	<-release
}

// timeBackendOpTestFrame stands in for session's timeBackendOp frame, whose name marks the
// goroutine as inside a timed op so it outranks the parallel tests' goroutines under the cap.
//
//go:noinline
func timeBackendOpTestFrame(fn func()) { fn() }

func TestRecorder_should_DumpTheBlockedGoroutineWhileTheOpIsStillRunning(t *testing.T) {
	t.Parallel()
	reports := make(chan DumpReport, 4)
	r := NewRecorder(time.Hour, func(rep DumpReport) { reports <- rep })

	release := make(chan struct{})
	entered := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer r.Watch("session.backend.capture_pane", "tymux")()
		timeBackendOpTestFrame(func() { blockedInSlowOpTestHelper(entered, release) })
	}()
	<-entered
	go r.fire("session.backend.capture_pane", "tymux")

	select {
	case rep := <-reports:
		assert.Equal(t, "session.backend.capture_pane", rep.Op)
		assert.Equal(t, "tymux", rep.Backend)
		assert.Contains(t, rep.Stacks, "blockedInSlowOpTestHelper", "the dump names what the op is blocked in")
		assert.NotContains(t, rep.Stacks, "slowops.(*Recorder)", "the recorder's own goroutine is excluded")
	case <-time.After(10 * time.Second):
		t.Fatal("no report for an op blocked past the threshold")
	}
	close(release)
	wg.Wait()
}

func TestRecorder_should_StayQuiet_When_TheOpFinishesInTime(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	fired := 0
	r := NewRecorder(time.Hour, func(DumpReport) { mu.Lock(); fired++; mu.Unlock() })
	r.Watch("session.backend.resize", "tmux")()
	mu.Lock()
	defer mu.Unlock()
	assert.Zero(t, fired)
}

func TestRecorder_should_RateLimitDumps(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	var dumps int
	r := NewRecorder(time.Hour, func(DumpReport) { dumps++ })
	r.now = func() time.Time { return now }

	r.fire("op", "tmux")
	r.fire("op", "tmux")
	require.Equal(t, 1, dumps, "a second slow op inside the window is counted but not dumped")
	now = now.Add(slowOpReportEvery + time.Second)
	r.fire("op", "tmux")
	assert.Equal(t, 2, dumps)
}

func TestRankBlockedStacks_should_KeepTimedOpGoroutine_When_OverTheCap(t *testing.T) {
	t.Parallel()
	var dump []string
	for i := 0; i < slowOpMaxGoroutines+5; i++ {
		dump = append(dump, fmt.Sprintf("goroutine %d [select, 9 minutes]:\nstapler-squad/idle.worker()", i))
	}
	dump = append(dump, "goroutine 999 [chan receive]:\nstapler-squad/session.timeBackendOp[...]()\nstapler-squad/session.capturePane()")

	out := rankBlockedStacks(strings.Join(dump, "\n\n"))

	assert.Contains(t, out, "capturePane", "the newest goroutine, inside the op, outranks older idle ones")
	assert.Equal(t, slowOpMaxGoroutines, strings.Count(out, "goroutine "), "still capped")
}

func TestGoroutineWaitMinutes_should_ParseTheHeaderOrReturnZero(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 5, goroutineWaitMinutes("goroutine 7 [chan receive, 5 minutes]:\nfoo()"))
	assert.Equal(t, 9, goroutineWaitMinutes("\ngoroutine 7 [select, locked to thread, 9 minutes]:\nfoo()"))
	assert.Zero(t, goroutineWaitMinutes("goroutine 7 [running]:\nfoo()"))
}

func TestRankBlockedStacks_should_RankLongestWaitFirst_And_TruncateOversizedStacks(t *testing.T) {
	t.Parallel()
	long := "goroutine 2 [select, 3 minutes]:\nstapler-squad/a()\n" + strings.Repeat("x", slowOpMaxStackBytes)
	older := "goroutine 3 [select, 8 minutes]:\nstapler-squad/b()"
	foreign := "goroutine 4 [select, 99 minutes]:\nnot-ours()"

	out := rankBlockedStacks(strings.Join([]string{long, older, foreign}, "\n\n"))

	assert.NotContains(t, out, "not-ours", "goroutines outside the module are dropped")
	assert.Less(t, strings.Index(out, "stapler-squad/b()"), strings.Index(out, "stapler-squad/a()"), "longest wait first")
	assert.Contains(t, out, "...(truncated)")
}

func TestRankBlockedStacks_should_DropTheRecordersOwnGoroutine(t *testing.T) {
	t.Parallel()
	own := "goroutine 1 [running]:\nstapler-squad/session/slowops.(*Recorder).fire(0x1)"
	other := "goroutine 2 [select]:\nstapler-squad/other.work()"

	out := rankBlockedStacks(own + "\n\n" + other)

	assert.NotContains(t, out, "slowops.(*Recorder)")
	assert.Contains(t, out, "other.work")
}

func TestThresholdFromMs_should_FallBackToTheDefault_When_UnsetOrInvalid(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 250*time.Millisecond, thresholdFromMs("250"))
	for _, v := range []string{"", "abc", "0", "-5"} {
		assert.Equal(t, defaultSlowOpThreshold, thresholdFromMs(v), "value %q", v)
	}
}

func TestWatchTerminalOp_should_ReturnAStopFunc(t *testing.T) {
	t.Parallel()
	WatchTerminalOp("session.backend.resize", "tmux")()
}

func TestBlockedStacks_should_IncludeThisTestsGoroutine(t *testing.T) {
	t.Parallel()
	assert.Contains(t, blockedStacks(), "TestBlockedStacks_should_IncludeThisTestsGoroutine")
}

func TestLogSlowOp_should_NotPanic(t *testing.T) {
	t.Parallel()
	logSlowOp(DumpReport{Op: "op", Backend: "tmux", Stacks: "s"})
}

func TestRecorder_should_ResolveTheCounterOnce(t *testing.T) {
	t.Parallel()
	r := NewRecorder(time.Hour, func(DumpReport) {})
	r.fire("op", "tmux")
	require.NotNil(t, r.counter)
	first := r.counter
	r.fire("op", "tmux")
	assert.Equal(t, first, r.counter, "the instrument is resolved once")
}

func TestRankBlockedStacks_should_DropExactlyTheOverflowGoroutine_When_OneOverTheCap(t *testing.T) {
	t.Parallel()
	var dump []string
	for i := 0; i < slowOpMaxGoroutines+1; i++ {
		dump = append(dump, fmt.Sprintf("goroutine %d [select, 1 minutes]:\nstapler-squad/idle.worker()", i))
	}

	out := rankBlockedStacks(strings.Join(dump, "\n\n"))

	assert.Equal(t, slowOpMaxGoroutines, strings.Count(out, "goroutine "))
}
