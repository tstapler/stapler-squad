package streamhub

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func queued(s *subscriber) []string {
	var out []string
	for len(s.outbound) > 0 {
		out = append(out, string(<-s.outbound))
	}
	return out
}

// overflowed returns a writerless subscriber (buffer 3) whose queue held a, b, c when d was
// dropped, with the writer having delivered one frame since.
func overflowed(t *testing.T) *subscriber {
	t.Helper()
	s := newSubscriber("s", nil, SubscriberCapability{}, 3)
	for _, f := range []string{"a", "b", "c"} {
		s.trySend([]byte(f))
	}
	delivered, _ := s.trySend([]byte("d"))
	require.False(t, delivered)
	<-s.outbound // the writer takes a ...
	s.sentFrames.Add(1)
	return s
}

func TestSubscriber_finishResync_should_PutRepaintAheadOfFramesHeldDuringCapture(t *testing.T) {
	t.Parallel()
	s := overflowed(t)
	require.True(t, s.beginResync())
	s.trySend([]byte("late1"))
	<-s.outbound // the writer keeps draining during the capture
	s.trySend([]byte("late2"))

	s.finishResync([]byte("R"))
	require.Equal(t, []string{"R", "late1", "late2"}, queued(s))
	require.False(t, s.hasDropped())
}

func TestSubscriber_finishResync_should_KeepQueueAndHeldFramesAndReflag_When_CaptureFailed(t *testing.T) {
	t.Parallel()
	s := overflowed(t)
	require.True(t, s.beginResync())
	<-s.outbound // b
	s.trySend([]byte("late"))

	s.finishResync(nil)
	require.Equal(t, []string{"c", "late"}, queued(s))
	require.True(t, s.hasDropped())
}

func TestSubscriber_finishResync_should_Reflag_When_HeldFramesOverflow(t *testing.T) {
	t.Parallel()
	s := overflowed(t) // buffer 3: at most 2 frames can be held beside the repaint
	require.True(t, s.beginResync())
	for _, f := range []string{"l1", "l2", "l3"} {
		s.trySend([]byte(f))
	}

	s.finishResync([]byte("R"))
	require.Equal(t, []string{"R", "l1", "l2"}, queued(s))
	require.True(t, s.hasDropped(), "the lost frame is covered by another repaint")
}

func TestSubscriber_beginResync_should_Wait_When_WriterHasNotDrainedSinceTheDrop(t *testing.T) {
	t.Parallel()
	s := newSubscriber("s", nil, SubscriberCapability{}, 1)
	s.trySend([]byte("a"))
	s.trySend([]byte("b")) // dropped; the writer never delivers anything

	require.False(t, s.beginResync(), "a repaint would only queue behind the stalled write")
	require.True(t, s.hasDropped(), "the flag is kept")

	s.sentFrames.Add(1)
	require.True(t, s.beginResync())
}
