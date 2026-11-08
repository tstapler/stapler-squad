package session

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

func TestSlowOpRecorder_should_DumpTheBlockedGoroutineWhileTheOpIsStillRunning(t *testing.T) {
	t.Parallel()
	reports := make(chan slowOpReport, 4)
	r := newSlowOpRecorder(time.Hour, func(rep slowOpReport) { reports <- rep })

	release := make(chan struct{})
	entered := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer r.watch("session.backend.capture_pane", backendLabelTymux)()
		_ = timeBackendOpErr(newTimedBackendOp(backendLabelTymux, "capture_pane"), func() error {
			blockedInSlowOpTestHelper(entered, release)
			return nil
		})
	}()
	<-entered
	go r.fire("session.backend.capture_pane", backendLabelTymux)

	select {
	case rep := <-reports:
		assert.Equal(t, "session.backend.capture_pane", rep.Op)
		assert.Equal(t, "tymux", rep.Backend)
		assert.Contains(t, rep.Stacks, "blockedInSlowOpTestHelper", "the dump names what the op is blocked in")
		assert.NotContains(t, rep.Stacks, "slowOpRecorder", "the recorder's own goroutine is excluded")
	case <-time.After(10 * time.Second):
		t.Fatal("no report for an op blocked past the threshold")
	}
	close(release)
	wg.Wait()
}

func TestSlowOpRecorder_should_StayQuiet_When_TheOpFinishesInTime(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	fired := 0
	r := newSlowOpRecorder(time.Hour, func(slowOpReport) { mu.Lock(); fired++; mu.Unlock() })
	r.watch("session.backend.resize", backendLabelTmux)()
	mu.Lock()
	defer mu.Unlock()
	assert.Zero(t, fired)
}

func TestSlowOpRecorder_should_RateLimitDumps(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700_000_000, 0)
	var dumps int
	r := newSlowOpRecorder(time.Hour, func(slowOpReport) { dumps++ })
	r.now = func() time.Time { return now }

	r.fire("op", backendLabelTmux)
	r.fire("op", backendLabelTmux)
	require.Equal(t, 1, dumps, "a second slow op inside the window is counted but not dumped")
	now = now.Add(slowOpReportEvery + time.Second)
	r.fire("op", backendLabelTmux)
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
