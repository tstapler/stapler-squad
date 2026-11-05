package services

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// streamedResponseSession builds a session holding one finished call whose response
// is larger than a frame, so respond() streams it from rec.body.
func streamedResponseSession(t *testing.T, id uint32, size int) (*unarySession, callResult) {
	t.Helper()
	b := &UnaryWsBridge{metrics: newUnaryMetrics(), bufferMax: 64 << 20}
	s := &unarySession{inflight: map[uint32]*inflightCall{id: {cancel: func() {}}}, b: b, writer: newUnaryWriter(nil)}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	t.Cleanup(s.cancel)
	rec := s.newRecorder()
	rec.WriteHeader(http.StatusOK)
	_, err := rec.Write(make([]byte, size))
	require.NoError(t, err)
	require.EqualValues(t, size, b.buffered.Load())
	return s, callResult{rec: rec}
}

func TestUnarySession_should_HoldBudget_UntilStreamedResponseIsFullyWritten(t *testing.T) {
	const size = 3 * unaryMaxFrameBytes
	s, res := streamedResponseSession(t, 7, size)
	s.respond(unaryFrame{ID: 7, Method: listFilesMethod}, res)
	assert.EqualValues(t, size, s.b.buffered.Load(), "the queued tail still aliases rec.body, so its bytes stay charged")

	for f := s.writer.next(); f != nil; f = s.writer.next() {
	}
	assert.Zero(t, s.b.buffered.Load(), "released once the last CHUNK is popped")
}

func TestUnarySession_should_ReleaseBudget_When_QueuedStreamIsAbortedAtSocketClose(t *testing.T) {
	s, res := streamedResponseSession(t, 7, 3*unaryMaxFrameBytes)
	s.respond(unaryFrame{ID: 7, Method: listFilesMethod}, res)
	require.NotZero(t, s.b.buffered.Load())
	s.writer.abort()
	assert.Zero(t, s.b.buffered.Load(), "a stream that will never be written must not leak budget")
}

func TestUnarySession_should_ReleaseBudget_When_StreamedResponseIsCanceled(t *testing.T) {
	s, res := streamedResponseSession(t, 7, 3*unaryMaxFrameBytes)
	s.respond(unaryFrame{ID: 7, Method: listFilesMethod}, res)
	s.cancelCall(7)
	assert.Zero(t, s.b.buffered.Load())
}

func TestTryCharge_should_NeverExceedMax_And_NotDisturbBudgetOnFailure(t *testing.T) {
	var budget atomic.Int64
	var wg sync.WaitGroup
	var ok atomic.Int64
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if tryCharge(&budget, 1, 100) {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.EqualValues(t, 100, ok.Load(), "exactly max one-byte charges succeed")
	assert.EqualValues(t, 100, budget.Load(), "failed attempts leave no residue")
	assert.False(t, tryCharge(&budget, 1, 100))
	assert.EqualValues(t, 100, budget.Load())
}

func TestUnaryRecorder_should_LetALaterSmallerWriteSucceed_After_AnotherOverflowed(t *testing.T) {
	var budget atomic.Int64
	a := &unaryRecorder{header: http.Header{}, budget: &budget, budgetMax: 10}
	b := &unaryRecorder{header: http.Header{}, budget: &budget, budgetMax: 10}
	c := &unaryRecorder{header: http.Header{}, budget: &budget, budgetMax: 10}
	_, err := a.Write(make([]byte, 6))
	require.NoError(t, err)
	_, err = b.Write(make([]byte, 6))
	require.Error(t, err)
	assert.True(t, b.overflow)
	_, err = c.Write(make([]byte, 4))
	assert.NoError(t, err, "an overflowed write is refunded, so a smaller one in the same batch can still fit")
	assert.EqualValues(t, 10, budget.Load())
}
