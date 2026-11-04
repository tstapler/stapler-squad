package tymux

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func TestSendInputViaControlMode_CallerContextBoundsTheLockWait(t *testing.T) {
	sess, stream := sessionWith(t)
	g := newGatedSends(true)
	stream.sendFn = g.send // sendLockWait stays at its 30s default: only ctx can end the wait

	done := make(chan struct{})
	go func() { defer close(done); _, _ = sess.SendKeys("long-paste") }()
	<-g.entered

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := sess.SendInputViaControlMode(ctx, []byte("k"))
	g.release()
	<-done

	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestSendKeys_FailureAfterFirstChunk_ReturnsTheRealPartialCount(t *testing.T) {
	sess, stream := sessionWith(t)
	var calls atomic.Int32
	stream.sendFn = func(*v1.AttachRequest) error {
		if calls.Add(1) == 2 {
			return errors.New("boom")
		}
		return nil
	}

	n, err := sess.SendKeys(string(bytes.Repeat([]byte("z"), 3*maxInputChunk)))

	require.Error(t, err)
	assert.Equal(t, maxInputChunk, n)
	assert.Contains(t, err.Error(), "after 32768 of 98304 bytes")
}

func TestSendKeys_MidPasteFailure_ClosesTheBracketedPaste(t *testing.T) {
	sess, stream := sessionWith(t)
	var calls atomic.Int32
	var sent [][]byte
	stream.sendFn = func(r *v1.AttachRequest) error {
		if calls.Add(1) == 2 {
			return errors.New("boom")
		}
		sent = append(sent, r.GetInput())
		return nil
	}
	body := bytes.Repeat([]byte("z"), 3*maxInputChunk)
	payload := append(append([]byte("\x1b[200~"), body...), "\x1b[201~"...)

	_, err := sess.SendKeys(string(payload))

	require.Error(t, err)
	require.Len(t, sent, 2) // first chunk, then the best-effort terminator
	assert.Equal(t, []byte("\x1b[201~"), sent[1])
}

func TestInBracketedPaste(t *testing.T) {
	assert.False(t, inBracketedPaste([]byte("plain")))
	assert.True(t, inBracketedPaste([]byte("\x1b[200~abc")))
	assert.False(t, inBracketedPaste([]byte("\x1b[200~abc\x1b[201~")))
}

func TestPTYAdapter_ForwardInput_RetriesWhileInputIsBusy(t *testing.T) {
	a := &tymuxPTYAdapter{closed: make(chan struct{})}
	var calls int
	a.forwardInput(func(*v1.AttachRequest) error {
		calls++
		if calls < 3 {
			return errInputBusy
		}
		return nil
	}, []byte("k"))
	assert.Equal(t, 3, calls)
}

func TestPTYAdapter_ForwardInput_StopsOnceClosed(t *testing.T) {
	a := &tymuxPTYAdapter{closed: make(chan struct{})}
	close(a.closed)
	var calls atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.forwardInput(func(*v1.AttachRequest) error {
			if calls.Add(1) > 100 {
				return nil // a hot loop ends here so the test fails on the count, not by hanging
			}
			return errInputBusy
		}, []byte("k"))
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("forwardInput did not stop after the adapter closed")
	}
	assert.EqualValues(t, 1, calls.Load())
}

func TestSendOnStreamCtx_AlreadyCancelledContext_SendsNothingEvenWithFreeLocks(t *testing.T) {
	sess, stream := sessionWith(t)
	before := len(stream.sentRequests())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	n, err := sess.sendOnStreamCtx(ctx, &v1.AttachRequest{Payload: &v1.AttachRequest_Input{Input: []byte("k")}})

	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, n)
	assert.Len(t, stream.sentRequests(), before)
}

func TestSendKeys_ContextCancelledBetweenChunks_StopsWithPartialCountAndClosesPaste(t *testing.T) {
	sess, stream := sessionWith(t)
	ctx, cancel := context.WithCancel(context.Background())
	var sent [][]byte
	stream.sendFn = func(r *v1.AttachRequest) error {
		sent = append(sent, r.GetInput())
		cancel() // caller gives up right after the first chunk is delivered
		return nil
	}
	body := bytes.Repeat([]byte("z"), 3*maxInputChunk)
	payload := append(append([]byte("\x1b[200~"), body...), "\x1b[201~"...)

	n, err := sess.sendOnStreamCtx(ctx, &v1.AttachRequest{Payload: &v1.AttachRequest_Input{Input: payload}})

	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, maxInputChunk, n)
	require.Len(t, sent, 2, "later chunks must not be sent; only the first chunk and the paste terminator")
	assert.Len(t, sent[0], maxInputChunk)
	assert.Equal(t, []byte("\x1b[201~"), sent[1])
}

func TestPasteCloser(t *testing.T) {
	start, end := "\x1b[200~", "\x1b[201~"
	tests := []struct {
		name string
		data string
		sent int
		want string
	}{
		{"no paste open", "plain" + end, 5, ""},
		{"open paste gets full terminator", start + "abcd" + end, len(start) + 2, end},
		{"already closed", start + "ab" + end + "tail", len(start) + 2 + len(end), ""},
		{"partial terminator: only the suffix", start + "ab" + end, len(start) + 2 + 4, "1~"},
		{"partial terminator, one byte missing", start + "ab" + end, len(start) + 2 + 5, "~"},
		{"sent below zero: nil, no panic", start + "ab", -1, ""},
		{"sent past the end: nil, no panic", start + "ab", len(start) + 3, ""},
		{"partial prefix that is not the terminator: skip", start + "ab\x1b[20" + "0~", len(start) + 2 + 4, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, string(pasteCloser([]byte(tc.data), tc.sent)))
		})
	}
}

func TestSendKeys_FailureInsideTerminatorBoundary_SendsOnlyTheMissingSuffix(t *testing.T) {
	sess, stream := sessionWith(t)
	var calls atomic.Int32
	var sent [][]byte
	stream.sendFn = func(r *v1.AttachRequest) error {
		if calls.Add(1) == 2 {
			return errors.New("boom")
		}
		sent = append(sent, r.GetInput())
		return nil
	}
	// First chunk ends exactly after "ESC[20" of the terminator.
	payload := append([]byte("\x1b[200~"), bytes.Repeat([]byte("z"), maxInputChunk-len("\x1b[200~")-4)...)
	payload = append(payload, "\x1b[201~"...)

	_, err := sess.SendKeys(string(payload))

	require.Error(t, err)
	require.Len(t, sent, 2)
	assert.Equal(t, []byte("1~"), sent[1])
}
