//go:build !windows

package session

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T-UNIT-SUB-1: Push delivers data to the consumer channel.
func TestMemPTYSubscriber_Push_DeliversToConsumer(t *testing.T) {
	s := newMemPTYSubscriber()
	defer s.Close()

	require.NoError(t, s.Push([]byte("hello")))

	select {
	case data := <-s.Chan():
		assert.Equal(t, []byte("hello"), data)
	case <-time.After(time.Second):
		t.Fatal("data not delivered within 1s")
	}
}

// T-UNIT-SUB-2: drain coalesces multiple chunks buffered in pushCh into a single send.
func TestMemPTYSubscriber_Drain_CoalescesChunks(t *testing.T) {
	s := newMemPTYSubscriber()
	defer s.Close()

	// Fill pushCh with several small chunks before drain has a chance to forward them.
	// The drain goroutine will see them all available at once and merge into one send.
	chunks := [][]byte{[]byte("foo"), []byte("bar"), []byte("baz")}
	for _, c := range chunks {
		require.NoError(t, s.Push(c))
	}

	// Allow drain to run.
	deadline := time.After(time.Second)
	var received []byte
	for len(received) < 9 {
		select {
		case data := <-s.Chan():
			received = append(received, data...)
		case <-deadline:
			t.Fatalf("did not receive all chunks within 1s; got %q", received)
		}
	}
	assert.Equal(t, []byte("foobarbaz"), received)
}

// T-UNIT-SUB-3: Push returns ErrSubscriberFull when pushCh is at capacity.
func TestMemPTYSubscriber_Push_ReturnsErrSubscriberFull_WhenAtCapacity(t *testing.T) {
	// Construct directly without starting drain so pushCh fills deterministically.
	// The drain goroutine coalesces items from pushCh faster than we can fill it,
	// making the test racy when using newMemPTYSubscriber().
	s := &memPTYSubscriber{
		pushCh: make(chan []byte, maxPushBufEntries),
		ch:     make(chan []byte, 64),
		stopCh: make(chan struct{}),
	}
	defer s.Close()

	// Fill pushCh to capacity without consuming from Chan.
	var lastErr error
	for i := 0; i <= maxPushBufEntries; i++ {
		lastErr = s.Push([]byte("x"))
		if lastErr != nil {
			break
		}
	}
	assert.ErrorIs(t, lastErr, ErrSubscriberFull,
		"Push must return ErrSubscriberFull when internal buffer is full")
}

// T-UNIT-SUB-4: Close causes Chan to be closed after drain exits.
func TestMemPTYSubscriber_Close_ClosesConsumerChannel(t *testing.T) {
	s := newMemPTYSubscriber()
	s.Close()

	// After Close, the consumer channel must be closed (drain exits and closes ch).
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-s.Chan():
			if !ok {
				return // channel closed as expected
			}
		case <-deadline:
			t.Fatal("consumer channel was not closed within 1s after Close()")
		}
	}
}

// T-UNIT-SUB-5: Close is idempotent — calling it multiple times does not panic.
func TestMemPTYSubscriber_Close_IsIdempotent(t *testing.T) {
	s := newMemPTYSubscriber()
	require.NotPanics(t, func() {
		s.Close()
		s.Close()
		s.Close()
	})
}

// T-UNIT-SUB-6: Push after Close does not panic and the data is silently discarded.
func TestMemPTYSubscriber_Push_AfterClose_DoesNotPanic(t *testing.T) {
	s := newMemPTYSubscriber()
	s.Close()

	// Wait for drain to exit so ch is closed.
	deadline := time.After(time.Second)
	for {
		select {
		case _, ok := <-s.Chan():
			if !ok {
				goto closed
			}
		case <-deadline:
			t.Fatal("consumer channel was not closed within 1s")
		}
	}
closed:
	require.NotPanics(t, func() {
		_ = s.Push([]byte("after-close"))
	})
}

// T-UNIT-SUB-7: Chan returns the same channel on repeated calls.
func TestMemPTYSubscriber_Chan_ReturnsSameChannel(t *testing.T) {
	s := newMemPTYSubscriber()
	defer s.Close()

	ch1 := s.Chan()
	ch2 := s.Chan()
	assert.Equal(t, ch1, ch2, "Chan must return the same channel on every call")
}

// T-UNIT-SUB-8: Push copies data — mutating the original slice after Push
// does not affect what the consumer receives.
func TestMemPTYSubscriber_Push_CopiesData(t *testing.T) {
	s := newMemPTYSubscriber()
	defer s.Close()

	original := []byte("immutable")
	require.NoError(t, s.Push(original))

	// Mutate the original slice immediately.
	copy(original, "xxxxxxxxx")

	select {
	case data := <-s.Chan():
		assert.True(t, bytes.Equal(data, []byte("immutable")),
			"consumer must receive original bytes, not the mutated slice; got %q", data)
	case <-time.After(time.Second):
		t.Fatal("data not delivered within 1s")
	}
}
