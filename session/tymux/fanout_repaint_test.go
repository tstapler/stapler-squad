package tymux

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A frame dropped for a full subscriber leaves a hole in the byte stream the browser parses
// statefully; the fanout must say so, and the repaint must replace the stale queue for exactly
// the subscribers that lost something.
func TestClientFanout_Repaint_ReplacesStaleQueueOfOverflowedSubscriberOnly(t *testing.T) {
	t.Parallel()
	f := NewClientFanout()
	drops := 0
	f.SetOnDrop(func() { drops++ })
	_, slow := f.Subscribe()
	_, healthy := f.Subscribe()

	for i := 0; i < subscriberBufferSize; i++ {
		f.Broadcast([]byte("old"))
		<-healthy // the healthy subscriber keeps up
	}
	require.Zero(t, drops)
	f.Broadcast([]byte("lost")) // slow's buffer is full
	assert.Equal(t, 1, drops)
	<-healthy

	ids := f.BeginRepaint()
	require.Len(t, ids, 1)
	assert.Equal(t, 1, f.FinishRepaint(ids, []byte("REPAINT")))
	assert.Empty(t, f.BeginRepaint(), "the dropped flag clears once claimed")

	require.Len(t, slow, 1)
	assert.Equal(t, []byte("REPAINT"), <-slow)
	assert.Empty(t, healthy)
}

// Output that arrives during the capture is not in the repaint, so it follows it, even when the
// consumer keeps draining meanwhile (a busy pane must not make the repaint stale or refused).
func TestClientFanout_Repaint_KeepsOutputBroadcastDuringTheCapture(t *testing.T) {
	t.Parallel()
	f := NewClientFanout()
	_, sub := f.Subscribe()
	for i := 0; i < subscriberBufferSize; i++ {
		f.Broadcast([]byte("old"))
	}
	f.Broadcast([]byte("lost"))
	ids := f.BeginRepaint() // the capture starts here
	<-sub
	f.Broadcast([]byte("late1"))
	for len(sub) > 0 {
		<-sub // the consumer drains everything, including output newer than the capture
	}
	f.Broadcast([]byte("late2"))

	require.Equal(t, 1, f.FinishRepaint(ids, []byte("REPAINT")))
	require.Len(t, sub, 3)
	assert.Equal(t, []string{"REPAINT", "late1", "late2"}, []string{string(<-sub), string(<-sub), string(<-sub)})
	assert.False(t, f.HasDropped())
}

func TestClientFanout_FinishRepaint_KeepsQueueAndReflags_When_CaptureFailed(t *testing.T) {
	t.Parallel()
	f := NewClientFanout()
	_, sub := f.Subscribe()
	for i := 0; i < subscriberBufferSize; i++ {
		f.Broadcast([]byte("old"))
	}
	f.Broadcast([]byte("lost"))
	ids := f.BeginRepaint()
	<-sub
	f.Broadcast([]byte("late"))

	assert.Zero(t, f.FinishRepaint(ids, nil))
	assert.Len(t, sub, subscriberBufferSize)
	assert.True(t, f.HasDropped())
}

func TestRepaintDroppedLoop_should_BackOffAndRecover_When_CaptureFails(t *testing.T) {
	t.Parallel()
	f := NewClientFanout()
	_, sub := f.Subscribe()
	for i := 0; i <= subscriberBufferSize; i++ {
		f.Broadcast([]byte("x"))
	}
	var sleeps []time.Duration
	var fails, calls int
	exhausted := repaintDroppedLoop(f, func() ([]byte, error) {
		calls++
		if calls <= 2 {
			return nil, errors.New("pane dead")
		}
		return []byte("REPAINT"), nil
	}, 250*time.Millisecond, func(d time.Duration) bool { sleeps = append(sleeps, d); return true },
		func(int) {}, func(error) { fails++ })

	assert.False(t, exhausted)
	assert.Equal(t, 1, fails, "a failing pane logs once per run")
	assert.Equal(t, []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, 250 * time.Millisecond}, sleeps)
	require.Len(t, sub, 1)
	assert.Equal(t, []byte("REPAINT"), <-sub)
}

func TestRepaintDroppedLoop_should_GiveUpAndStayFlagged_When_CaptureKeepsFailing(t *testing.T) {
	t.Parallel()
	f := NewClientFanout()
	f.Subscribe()
	for i := 0; i <= subscriberBufferSize; i++ {
		f.Broadcast([]byte("x"))
	}
	calls := 0
	exhausted := repaintDroppedLoop(f, func() ([]byte, error) { calls++; return nil, errors.New("dead") },
		time.Millisecond, func(time.Duration) bool { return true }, func(int) {}, func(error) {})

	assert.True(t, exhausted)
	assert.Equal(t, dropRepaintMaxAttempts, calls, "capture attempts are bounded")
	assert.True(t, f.HasDropped(), "the next dropped frame can start another run")
}

func TestRepaintDroppedLoop_should_StopAtOnce_When_SessionAborted(t *testing.T) {
	t.Parallel()
	f := NewClientFanout()
	f.Subscribe()
	for i := 0; i <= subscriberBufferSize; i++ {
		f.Broadcast([]byte("x"))
	}
	calls := 0
	repaintDroppedLoop(f, func() ([]byte, error) { calls++; return nil, errors.New("dead") },
		time.Millisecond, func(time.Duration) bool { return false }, func(int) {}, func(error) {})
	assert.Equal(t, 1, calls)
}
