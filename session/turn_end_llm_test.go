package session

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeModel struct {
	calls  atomic.Int32
	answer string
	err    error
	gate   chan struct{} // when set, the call blocks until closed or ctx ends
	mu     sync.Mutex
	prompt string
}

func (f *fakeModel) call(ctx context.Context, _, user string) (string, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.prompt = user
	f.mu.Unlock()
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return f.answer, f.err
}

func TestLLMTiebreak_Should_ReturnEnumAndCacheByTextHash(t *testing.T) {
	t.Parallel()
	m := &fakeModel{answer: "QUESTION"}
	tb := NewLLMTurnEndTiebreaker(m.call, time.Second)

	got, err := tb.Tiebreak(context.Background(), "ep1", "Is this what you had in mind?")
	require.NoError(t, err)
	assert.Equal(t, TurnEndClassQuestion, got)

	got, err = tb.Tiebreak(context.Background(), "ep2", "Is this what you had in mind?")
	require.NoError(t, err)
	assert.Equal(t, TurnEndClassQuestion, got)
	assert.EqualValues(t, 1, m.calls.Load(), "second episode with the same text must hit the cache")
}

func TestLLMTiebreak_Should_CoalesceConcurrentCallsForOneEpisode(t *testing.T) {
	t.Parallel()
	m := &fakeModel{answer: "FINISHED", gate: make(chan struct{})}
	tb := NewLLMTurnEndTiebreaker(m.call, 5*time.Second)

	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := tb.Tiebreak(context.Background(), "ep", "Does that make sense to you?")
			assert.NoError(t, err)
		}()
	}
	require.Eventually(t, func() bool { return m.calls.Load() >= 1 }, time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond) // let the other callers join the flight
	close(m.gate)
	wg.Wait()
	assert.EqualValues(t, 1, m.calls.Load())
}

func TestLLMTiebreak_Should_RedactSecretsAndAnsiBeforeSending(t *testing.T) {
	t.Parallel()
	m := &fakeModel{answer: "FINISHED"}
	tb := NewLLMTurnEndTiebreaker(m.call, time.Second)

	_, err := tb.Tiebreak(context.Background(), "ep", "\x1b[31mtoken: abcdef123456\x1b[0m key ghp_abcdefghijklmnopqrstuvwxyz0123 ok?")
	require.NoError(t, err)
	m.mu.Lock()
	defer m.mu.Unlock()
	assert.NotContains(t, m.prompt, "ghp_")
	assert.NotContains(t, m.prompt, "abcdef123456")
	assert.NotContains(t, m.prompt, "\x1b")
	assert.Contains(t, m.prompt, "[redacted]")
}

func TestLLMTiebreak_Should_RejectOutputThatIsNotExactlyAnEnum(t *testing.T) {
	t.Parallel()
	for _, out := range []string{"QUESTION because it asks", "I think it is a question", "", "NOT_A_QUESTION"} {
		tb := NewLLMTurnEndTiebreaker((&fakeModel{answer: out}).call, time.Second)
		got, err := tb.Tiebreak(context.Background(), "ep", "Should I go ahead?")
		assert.Error(t, err, out)
		assert.Equal(t, TurnEndClassFinished, got, out)
	}
}

func TestLLMTiebreak_Should_FailOpenToFinished_When_CallTimesOut(t *testing.T) {
	t.Parallel()
	m := &fakeModel{gate: make(chan struct{})}
	tb := NewLLMTurnEndTiebreaker(m.call, 20*time.Millisecond)

	got, err := tb.Tiebreak(context.Background(), "ep", "Should I go ahead?")
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, TurnEndClassFinished, got)
}

func TestLLMTiebreak_Should_NotCacheFailures(t *testing.T) {
	t.Parallel()
	m := &fakeModel{err: errors.New("boom")}
	tb := NewLLMTurnEndTiebreaker(m.call, time.Second)
	_, _ = tb.Tiebreak(context.Background(), "ep", "Should I go ahead?")
	_, _ = tb.Tiebreak(context.Background(), "ep2", "Should I go ahead?")
	assert.EqualValues(t, 2, m.calls.Load())
}

func TestLLMTiebreak_Should_SendOnlyTheTailOfALongMessage(t *testing.T) {
	t.Parallel()
	m := &fakeModel{answer: "FINISHED"}
	tb := NewLLMTurnEndTiebreaker(m.call, time.Second)
	_, err := tb.Tiebreak(context.Background(), "ep", strings.Repeat("x ", 5000)+"Last sentence here?")
	require.NoError(t, err)
	m.mu.Lock()
	defer m.mu.Unlock()
	assert.LessOrEqual(t, len(m.prompt), turnEndTiebreakMaxInputBytes+64)
	assert.Contains(t, m.prompt, "Last sentence here?")
}

type fakeTiebreaker struct {
	class TurnEndClass
	err   error
	gate  chan struct{}
	calls atomic.Int32
}

func (f *fakeTiebreaker) Tiebreak(context.Context, EpisodeID, string) (TurnEndClass, error) {
	f.calls.Add(1)
	if f.gate != nil {
		<-f.gate
	}
	return f.class, f.err
}

func TestRunTurnEndTiebreakAsync_Should_NotBlockTheCaller_And_SkipCallbackOnError(t *testing.T) {
	t.Parallel()
	blocked := &fakeTiebreaker{class: TurnEndClassQuestion, gate: make(chan struct{})}
	var got atomic.Value
	done := RunTurnEndTiebreakAsync(blocked, "s", "ep", "text", func(c TurnEndClass) { got.Store(c) })
	select {
	case <-done:
		t.Fatal("returned before the tiebreak finished: it ran inline")
	default:
	}
	close(blocked.gate)
	<-done
	assert.Equal(t, TurnEndClassQuestion, got.Load())

	failing := &fakeTiebreaker{err: errors.New("timeout")}
	called := false
	<-RunTurnEndTiebreakAsync(failing, "s", "ep", "text", func(TurnEndClass) { called = true })
	assert.False(t, called, "a failed tiebreak must leave the Finished verdict alone")
}
