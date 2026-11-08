package services

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

type fakeInputSink struct {
	mu      sync.Mutex
	updates []string
	marks   int
	blockOn chan struct{} // when set, UpdateTerminalTimestamps waits on it (a held i.mu)
	entered chan struct{}
}

func (f *fakeInputSink) UpdateTerminalTimestamps(content string, _ bool) {
	if f.entered != nil {
		f.entered <- struct{}{}
	}
	if f.blockOn != nil {
		<-f.blockOn
	}
	f.mu.Lock()
	f.updates = append(f.updates, content)
	f.mu.Unlock()
}

func (f *fakeInputSink) MarkUserResponded() time.Time {
	f.mu.Lock()
	f.marks++
	f.mu.Unlock()
	return time.Time{}
}

func newManualBookkeeper(sink humanInputSink) (*inputBookkeeper, *[]func()) {
	var timers []func()
	b := newInputBookkeeper(sink, &streamLatency{})
	b.after = func(_ time.Duration, f func()) { timers = append(timers, f) }
	return b, &timers
}

func TestInputBookkeeper_should_CoalesceKeystrokesIntoOneFlushWithTheLatestChunk(t *testing.T) {
	t.Parallel()
	sink := &fakeInputSink{}
	b, timers := newManualBookkeeper(sink)

	for _, k := range []string{"l", "s", "\r"} {
		b.note([]byte(k))
	}
	require.Len(t, *timers, 1, "one flush is scheduled per window, not one per keystroke")
	assert.Empty(t, sink.updates, "nothing touches the instance on the typing path")

	(*timers)[0]()
	assert.Equal(t, []string{"s"}, sink.updates) // the trailing Enter is blank and must not displace "s"
	assert.Equal(t, 1, sink.marks)

	b.note([]byte("x"))
	assert.Len(t, *timers, 2, "a flush re-arms the next window")
}

func TestInputBookkeeper_should_NotBlockTheTypingPathWhenTheInstanceLockIsHeld(t *testing.T) {
	t.Parallel()
	sink := &fakeInputSink{blockOn: make(chan struct{}), entered: make(chan struct{}, 1)}
	b := newInputBookkeeper(sink, &streamLatency{})
	b.delay = time.Millisecond

	b.note([]byte("a"))
	<-sink.entered // the flush is now stuck inside the instance, as if i.mu were held

	returned := make(chan struct{})
	go func() {
		b.note([]byte("b"))
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("note blocked behind the held instance lock")
	}
	close(sink.blockOn)
}

func TestSlowRequestLane_should_RunJobsInOrderWithoutBlockingTheSubmitter(t *testing.T) {
	t.Parallel()
	errCh := make(chan error, 2)
	lane := newSlowRequestLane(errCh, nil)
	t.Cleanup(lane.close)

	release := make(chan struct{})
	var order []int
	var mu sync.Mutex
	done := make(chan struct{})
	record := func(n int) func() error {
		return func() error {
			mu.Lock()
			order = append(order, n)
			mu.Unlock()
			return nil
		}
	}
	lane.submit(func() error { <-release; return record(1)() })
	lane.submit(record(2))
	lane.submit(func() error { defer close(done); return record(3)() })

	mu.Lock()
	assert.Empty(t, order, "submit returned while job 1 is still running")
	mu.Unlock()
	close(release)
	<-done
	mu.Lock()
	assert.Equal(t, []int{1, 2, 3}, order)
	mu.Unlock()
}

func TestSlowRequestLane_should_ReportAFailureAndEndTheStream(t *testing.T) {
	t.Parallel()
	errCh := make(chan error, 2)
	failed := make(chan struct{})
	lane := newSlowRequestLane(errCh, func() { close(failed) })
	t.Cleanup(lane.close)

	boom := errors.New("pane gone")
	lane.submit(func() error { return boom })
	<-failed
	assert.ErrorIs(t, <-errCh, boom)
}

func TestSlowRequestLane_should_DiscardQueuedJobsOnClose(t *testing.T) {
	t.Parallel()
	lane := newSlowRequestLane(make(chan error, 1), nil)
	gate := make(chan struct{})
	started := make(chan struct{})
	var ran atomic.Int32
	lane.submit(func() error { close(started); <-gate; ran.Add(1); return nil })
	<-started
	lane.submit(func() error { ran.Add(1); return nil })
	lane.close()
	close(gate)
	lane.close() // idempotent
	assert.Eventually(t, func() bool { return ran.Load() == 1 }, 5*time.Second, time.Millisecond)
	assert.Never(t, func() bool { return ran.Load() > 1 }, 50*time.Millisecond, 10*time.Millisecond)
}

func TestDispatchInputReadLoopFrame_should_ReturnWhileAScrollbackCaptureIsInFlight(t *testing.T) {
	t.Parallel()
	errCh := make(chan error, 2)
	lane := newSlowRequestLane(errCh, nil)
	t.Cleanup(lane.close)

	started := make(chan struct{})
	gate := make(chan struct{})
	p := inputReadLoopParams{
		slowLane:  lane,
		sessionID: "typing-while-scrollback",
		onScrollbackRequest: func(string, string) (ScrollbackResult, error) {
			close(started)
			<-gate
			return ScrollbackResult{}, errors.New("capture abandoned") // handleScrollbackRequest logs and writes nothing
		},
	}
	frame := &sessionv1.TerminalData{Data: &sessionv1.TerminalData_ScrollbackRequest{
		ScrollbackRequest: &sessionv1.ScrollbackRequest{Limit: 10},
	}}

	dispatched := make(chan error, 1)
	go func() { dispatched <- dispatchInputReadLoopFrame(p, frame) }()
	select {
	case err := <-dispatched:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("dispatch blocked on the capture; keystrokes behind it would wait")
	}
	<-started // the capture is running on the lane while the read loop is free
	close(gate)
}

func TestLatestResize_should_ApplyOnlyTheNewestSizeAndNeverBlockThePusher(t *testing.T) {
	t.Parallel()
	lane := newSlowRequestLane(make(chan error, 1), nil)
	t.Cleanup(lane.close)
	var r latestResize

	gate := make(chan struct{})
	started := make(chan struct{})
	var mu sync.Mutex
	var applied [][2]int
	apply := func(cols, rows int) {
		mu.Lock()
		applied = append(applied, [2]int{cols, rows})
		first := len(applied) == 1
		mu.Unlock()
		if first {
			close(started)
			<-gate // the first resize is stuck in SetWindowSize/capture
		}
	}

	pushed := make(chan struct{})
	go func() {
		r.push(lane, 80, 24, apply)
		<-started
		r.push(lane, 90, 30, apply) // superseded
		r.push(lane, 100, 35, apply)
		close(pushed)
	}()
	select {
	case <-pushed:
	case <-time.After(5 * time.Second):
		t.Fatal("push blocked behind an in-flight resize")
	}
	close(gate)
	assert.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(applied) == 2
	}, 5*time.Second, time.Millisecond)
	mu.Lock()
	assert.Equal(t, [][2]int{{80, 24}, {100, 35}}, applied, "the superseded 90x30 is never applied")
	mu.Unlock()
}

func TestSlowRequestLane_should_NotBlockSubmittersAfterAFailure(t *testing.T) {
	t.Parallel()
	failed := make(chan struct{})
	lane := newSlowRequestLane(make(chan error, 2), func() { close(failed) })
	lane.submit(func() error { return errors.New("boom") })
	<-failed

	done := make(chan struct{})
	go func() {
		for i := 0; i < slowLaneDepth*3; i++ { // more than the queue holds, with no worker left
			lane.submit(func() error { return nil })
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("submit blocked on a dead lane; the read loop would never see its deadline")
	}
}
