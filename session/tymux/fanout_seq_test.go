package tymux

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func drain(ch chan []byte) (out []string) {
	for {
		select {
		case b := <-ch:
			out = append(out, string(b))
		default:
			return out
		}
	}
}

func seqFanout(base uint64) (*ClientFanout, chan []byte) {
	f := NewClientFanout()
	f.ResetSeq(base)
	_, ch := f.Subscribe()
	return f, ch
}

func TestFanoutResyncAt_ReplacesQueueWithRedrawThenOutputAfterSnapshot(t *testing.T) {
	t.Parallel()
	f, ch := seqFanout(0)
	for i, s := range []string{"a", "b", "c", "d"} {
		f.BroadcastSeq(uint64(i+1), []byte(s))
	}

	n, ok := f.ResyncAt(2, f.SeqGeneration(), []byte("R"), ScopeAll) // snapshot already contains a, b

	require.True(t, ok)
	assert.Equal(t, 1, n)
	assert.Equal(t, []string{"Rcd"}, drain(ch), "queue holds the redraw plus c, d, nothing before the snapshot")
}

// Output the snapshot already contains can still be in flight toward the fanout when the
// snapshot is spliced in; it must be discarded when it lands, not applied twice.
func TestFanoutResyncAt_DiscardsLaterArrivalsThatTheSnapshotContains(t *testing.T) {
	t.Parallel()
	f, ch := seqFanout(0)
	f.BroadcastSeq(1, []byte("a"))

	_, ok := f.ResyncAt(3, f.SeqGeneration(), []byte("R"), ScopeAll) // snapshot is already at seq 3, stream is at 1
	require.True(t, ok)
	f.BroadcastSeq(2, []byte("b"))
	f.BroadcastSeq(3, []byte("c"))
	f.BroadcastSeq(4, []byte("d"))

	assert.Equal(t, []string{"R", "d"}, drain(ch))
}

func TestFanoutResyncAt_ScopeDroppedLeavesHealthySubscribersAlone(t *testing.T) {
	t.Parallel()
	f := NewClientFanout()
	f.ResetSeq(0)
	_, slow := f.Subscribe()
	_, healthy := f.Subscribe()
	for i := 1; i <= subscriberBufferSize+5; i++ {
		f.BroadcastSeq(uint64(i), []byte("x"))
		<-healthy // the healthy subscriber keeps up; the slow one overflows
	}

	n, ok := f.ResyncAt(uint64(subscriberBufferSize), f.SeqGeneration(), []byte("R"), ScopeDropped)

	require.True(t, ok)
	assert.Equal(t, 1, n)
	assert.Len(t, drain(slow), 1, "overflowed subscriber is repaired in one frame")
	assert.Empty(t, drain(healthy))
}

func TestFanoutResyncAt_FailsWhenOutputAfterSnapshotWasEvicted(t *testing.T) {
	t.Parallel()
	f, ch := seqFanout(0)
	chunk := make([]byte, seqRingBudgetBytes/2+1)
	for i := 1; i <= 4; i++ {
		f.BroadcastSeq(uint64(i), chunk)
		<-ch
	}

	_, ok := f.ResyncAt(1, f.SeqGeneration(), []byte("R"), ScopeAll)

	assert.False(t, ok, "frames 2.. no longer fit the ring; caller must capture again")
	assert.Empty(t, drain(ch), "a failed resync changes nothing")
}

func TestFanoutBroadcastSeq_DropsDuplicatesAndReportsUnannouncedGaps(t *testing.T) {
	t.Parallel()
	f, ch := seqFanout(5)

	assert.False(t, f.BroadcastSeq(5, []byte("dup")), "seq already covered by the priming snapshot")
	assert.False(t, f.BroadcastSeq(6, []byte("a")))
	assert.True(t, f.BroadcastSeq(9, []byte("b")), "7, 8 vanished with no OutputGap")
	f.NoteGapSignaled()
	assert.False(t, f.BroadcastSeq(12, []byte("c")), "this jump was announced")

	assert.Equal(t, []string{"a", "b", "c"}, drain(ch))
	assert.EqualValues(t, 2, f.GapGeneration())
}

func TestFanoutDisableSeq_MakesResyncAtDecline(t *testing.T) {
	t.Parallel()
	f, _ := seqFanout(0)
	f.DisableSeq()

	_, ok := f.ResyncAt(0, f.SeqGeneration(), []byte("R"), ScopeAll)

	assert.False(t, ok)
	assert.False(t, f.SeqValid())
}

// After a reseed (daemon restart, reattach) the numbering may restart; a capture taken under the
// old numbering must not be spliced, or its high seq would make every later chunk look stale.
func TestFanoutResyncAt_RejectsASnapshotFromBeforeAReseed(t *testing.T) {
	t.Parallel()
	f, ch := seqFanout(0)
	gen := f.SeqGeneration()
	f.ResetSeq(0)

	_, ok := f.ResyncAt(900, gen, []byte("R"), ScopeAll)

	assert.False(t, ok)
	f.BroadcastSeq(1, []byte("a"))
	assert.Equal(t, []string{"a"}, drain(ch), "output after the reseed still flows")
}
