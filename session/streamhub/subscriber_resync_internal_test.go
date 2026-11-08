package streamhub

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func queued(s *subscriber) []string {
	var out []string
	for len(s.outbound) > 0 {
		out = append(out, string((<-s.outbound).data))
	}
	return out
}

// overflowed returns a writerless subscriber (buffer 2) whose queue held a, b when c was
// dropped, with the writer having delivered one frame since.
func overflowed(t *testing.T) *subscriber {
	t.Helper()
	s := newSubscriber("s", nil, SubscriberCapability{}, 2)
	s.trySend([]byte("a"))
	s.trySend([]byte("b"))
	delivered, _ := s.trySend([]byte("c"))
	require.False(t, delivered)
	<-s.outbound // the writer takes a ...
	s.sentFrames.Add(1)
	return s
}

func TestSubscriber_supersede_should_KeepFramesQueuedAfterTheMark(t *testing.T) {
	t.Parallel()
	s := overflowed(t)
	mark, ok := s.beginResync()
	require.True(t, ok)
	s.trySend([]byte("late")) // queued after the capture began; b is still queued ahead of it

	require.True(t, s.supersede([]byte("R"), mark))
	require.Equal(t, []string{"R", "late"}, queued(s))
}

func TestSubscriber_supersede_should_RefuseAndReflag_When_WriterTookPostMarkFrame(t *testing.T) {
	t.Parallel()
	s := overflowed(t)
	mark, ok := s.beginResync()
	require.True(t, ok)
	s.trySend([]byte("late"))
	<-s.outbound // b
	<-s.outbound // late: the writer is already sending a frame newer than the capture

	require.False(t, s.supersede([]byte("R"), mark))
	require.Empty(t, queued(s))
	_, again := s.beginResync()
	require.True(t, again, "the subscriber is flagged for a fresher capture")
}

func TestSubscriber_beginResync_should_Wait_When_WriterHasNotDrainedSinceTheDrop(t *testing.T) {
	t.Parallel()
	s := newSubscriber("s", nil, SubscriberCapability{}, 1)
	s.trySend([]byte("a"))
	s.trySend([]byte("b")) // dropped; the writer never delivers anything

	_, ok := s.beginResync()
	require.False(t, ok, "a repaint would only queue behind the stalled write")
	require.True(t, s.hasDropped(), "the flag is kept")

	s.sentFrames.Add(1)
	_, ok = s.beginResync()
	require.True(t, ok)
}
