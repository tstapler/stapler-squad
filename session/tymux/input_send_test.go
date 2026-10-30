package tymux

import (
	"bytes"
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
)

// gatedSends is installed as fakeAttachStream.sendFn. connect-go documents that
// BidiStreamForClient.Send must not be called concurrently, so any overlap is the bug.
// The first Send it sees blocks until release() so a test can hold one Send open and watch
// whether a second caller gets into the stream underneath it.
type gatedSends struct {
	inflight, maxInflight atomic.Int64
	holdFirst             atomic.Bool
	entered               chan struct{}
	releaseCh             chan struct{}
	releaseOnce           sync.Once
}

func newGatedSends(hold bool) *gatedSends {
	g := &gatedSends{entered: make(chan struct{}), releaseCh: make(chan struct{})}
	g.holdFirst.Store(hold)
	return g
}

func (g *gatedSends) send(*v1.AttachRequest) error {
	n := g.inflight.Add(1)
	defer g.inflight.Add(-1)
	for {
		m := g.maxInflight.Load()
		if n <= m || g.maxInflight.CompareAndSwap(m, n) {
			break
		}
	}
	if g.holdFirst.CompareAndSwap(true, false) {
		close(g.entered)
		<-g.releaseCh
	} else {
		runtime.Gosched() // widen the window so an unserialized caller overlaps reliably
	}
	return nil
}

func (g *gatedSends) release() { g.releaseOnce.Do(func() { close(g.releaseCh) }) }

// letRivalRun yields the scheduler enough times that a goroutine that is not blocked on a lock
// would already have entered the stream. It bounds a negative ("must not enter") assertion
// without sleeping.
func letRivalRun(g *gatedSends) {
	for i := 0; i < 5000 && g.maxInflight.Load() < 2; i++ {
		runtime.Gosched()
	}
}

func sessionWith(t *testing.T) (*tymuxGRPCSession, *fakeAttachStream) {
	t.Helper()
	m, stream, _ := startedSessionWithStream(t)
	sess, ok := m.(*tymuxGRPCSession)
	require.True(t, ok)
	return sess, stream
}

func TestSendOnStream_ResizeWhileInputSendIsInFlight_WaitsForTheStream(t *testing.T) {
	sess, stream := sessionWith(t)
	g := newGatedSends(true)
	stream.sendFn = g.send

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = sess.SendKeys("A") }()
	<-g.entered // input A is inside stream.Send
	go func() { defer wg.Done(); _ = sess.SetWindowSize(100, 30) }()
	letRivalRun(g)
	g.release()
	wg.Wait()

	assert.EqualValues(t, 1, g.maxInflight.Load(), "a resize entered stream.Send while an input Send was in flight")
}

func TestSendOnStream_SecondInputWhileInputSendIsInFlight_WaitsForTheStream(t *testing.T) {
	sess, stream := sessionWith(t)
	g := newGatedSends(true)
	stream.sendFn = g.send

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); _, _ = sess.SendKeys("A") }()
	<-g.entered
	go func() { defer wg.Done(); _, _ = sess.SendKeys("B") }()
	letRivalRun(g)
	g.release()
	wg.Wait()

	assert.EqualValues(t, 1, g.maxInflight.Load(), "two inputs were inside stream.Send at once")
}

func TestSendOnStream_ManyWritersAndResizes_NeverOverlapAndLoseNothing(t *testing.T) {
	sess, stream := sessionWith(t)
	g := newGatedSends(false)
	stream.sendFn = g.send
	before := len(stream.sentRequests())

	const writers, perWriter, resizes = 8, 100, 100
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				_, err := sess.SendKeys("x")
				assert.NoError(t, err)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < resizes; i++ {
			assert.NoError(t, sess.SetWindowSize(80+i%5, 24))
		}
	}()
	wg.Wait()

	assert.EqualValues(t, 1, g.maxInflight.Load(), "stream.Send was entered concurrently")
	assert.Len(t, stream.sentRequests(), before+writers*perWriter+resizes)
}

func inputBytes(reqs []*v1.AttachRequest) (data []byte, sizes []int) {
	for _, r := range reqs {
		if in, ok := r.GetPayload().(*v1.AttachRequest_Input); ok {
			data = append(data, in.Input...)
			sizes = append(sizes, len(in.Input))
		}
	}
	return data, sizes
}

func TestSendKeys_PayloadUpToOneChunk_IsASingleFrame(t *testing.T) {
	sess, stream := sessionWith(t)
	paste := bytes.Repeat([]byte("p"), maxInputChunk)
	before := len(stream.sentRequests())

	_, err := sess.SendKeys(string(paste))

	require.NoError(t, err)
	_, sizes := inputBytes(stream.sentRequests()[before:])
	assert.Equal(t, []int{maxInputChunk}, sizes, "a payload that fits must never be split (bracketed paste / escape units)")
}

func TestSendKeys_LargePayload_IsChunkedInOrderWithNothingLost(t *testing.T) {
	sess, stream := sessionWith(t)
	payload := make([]byte, 2*maxInputChunk+maxInputChunk/2)
	for i := range payload {
		payload[i] = byte('a' + i%26)
	}
	before := len(stream.sentRequests())

	n, err := sess.SendKeys(string(payload))

	require.NoError(t, err)
	assert.Equal(t, len(payload), n)
	got, sizes := inputBytes(stream.sentRequests()[before:])
	assert.Equal(t, payload, got)
	assert.Equal(t, []int{maxInputChunk, maxInputChunk, maxInputChunk / 2}, sizes)
}

func TestSendKeys_LargePayload_OtherInputCannotInterleaveButResizeMay(t *testing.T) {
	sess, stream := sessionWith(t)
	g := newGatedSends(true)
	stream.sendFn = g.send
	before := len(stream.sentRequests())
	paste := bytes.Repeat([]byte("P"), 3*maxInputChunk)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); _, _ = sess.SendKeys(string(paste)) }()
	<-g.entered // first chunk of the paste is inside Send
	go func() { defer wg.Done(); _, _ = sess.SendKeys("typed") }()
	go func() { defer wg.Done(); _ = sess.SetWindowSize(90, 30) }()
	letRivalRun(g)
	g.release()
	wg.Wait()

	var order []string
	for _, r := range stream.sentRequests()[before:] {
		switch p := r.GetPayload().(type) {
		case *v1.AttachRequest_Input:
			if string(p.Input) == "typed" {
				order = append(order, "typed")
			} else {
				order = append(order, "paste")
			}
		case *v1.AttachRequest_Resize:
			order = append(order, "resize")
		}
	}
	// The typed input must come strictly before the first or after the last paste chunk.
	first, last, typed := -1, -1, -1
	for i, o := range order {
		switch o {
		case "paste":
			if first < 0 {
				first = i
			}
			last = i
		case "typed":
			typed = i
		}
	}
	require.GreaterOrEqual(t, typed, 0)
	assert.True(t, typed < first || typed > last, "typed input was interleaved into the paste: %v", order)
	assert.Equal(t, 3, countOf(order, "paste"))
	assert.Equal(t, 1, countOf(order, "resize"))
}

func countOf(s []string, v string) (n int) {
	for _, x := range s {
		if x == v {
			n++
		}
	}
	return n
}

func TestSendOnStream_WaitingForTheStreamTooLong_ReturnsAClearError(t *testing.T) {
	sess, stream := sessionWith(t)
	g := newGatedSends(true)
	stream.sendFn = g.send
	sess.mu.Lock()
	sess.sendLockWait = 20 * 1000 * 1000 // 20ms
	sess.mu.Unlock()

	done := make(chan struct{})
	go func() { defer close(done); _, _ = sess.SendKeys("stuck") }()
	<-g.entered

	_, inputErr := sess.SendKeys("blocked-input")
	resizeErr := sess.SetWindowSize(100, 40)
	g.release()
	<-done

	assert.ErrorIs(t, inputErr, errInputBusy)
	assert.ErrorIs(t, resizeErr, errSendBusy)
}

func TestSendKeys_PayloadOverItsDeadline_StopsBetweenChunksAndSaysHowFarItGot(t *testing.T) {
	sess, stream := sessionWith(t)
	sess.mu.Lock()
	sess.inputPayloadTimeout = 1 // 1ns: already expired after the first chunk
	sess.mu.Unlock()
	before := len(stream.sentRequests())

	_, err := sess.SendKeys(string(bytes.Repeat([]byte("z"), 3*maxInputChunk)))

	require.ErrorIs(t, err, errInputTimedOut)
	assert.Contains(t, err.Error(), "sent 32768 of 98304 bytes")
	_, sizes := inputBytes(stream.sentRequests()[before:])
	assert.Equal(t, []int{maxInputChunk}, sizes)
}

func TestSendOnStream_StuckSendOnOldStream_DoesNotBlockTheReplacementStream(t *testing.T) {
	sess, oldStream := sessionWith(t)
	g := newGatedSends(true)
	oldStream.sendFn = g.send

	done := make(chan struct{})
	go func() { defer close(done); _, _ = sess.SendKeys("stuck-on-old") }()
	<-g.entered // old stream's Send never returns until released

	replacement := newFakeAttachStream(context.Background())
	sess.mu.Lock()
	sess.setStreamLocked(replacement) // what a successful reconnect does
	sess.mu.Unlock()

	_, err := sess.SendKeys("on-new")

	require.NoError(t, err)
	data, _ := inputBytes(replacement.sentRequests())
	assert.Equal(t, []byte("on-new"), data)
	g.release()
	<-done
}
