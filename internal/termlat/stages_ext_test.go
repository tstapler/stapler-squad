package termlat

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeStage_should_AcceptEveryDecompositionStage(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		StageOutputQueueWait, StageFanoutWait, StageFanoutDepth, StageFrameHandle, StagePTYWrite,
		StageCaptureRender, StageCaptureRevive, StageStreamLockWait, StageStreamSend, StagePMLockWait,
		StageInputQueue, StageEchoRender, StageWSBufferedKiB, StageEventLoopLag,
	} {
		assert.Equal(t, s, NormalizeStage(s))
	}
	assert.Equal(t, "unknown", NormalizeStage("made_up"))
}

func TestCountDrop_should_BoundLabelsAndSortRows(t *testing.T) {
	t.Parallel()
	r, _ := newTest()
	r.CountDrop(DropHubQueueFull, "tymux")
	r.CountDrop(DropHubQueueFull, "tymux")
	r.CountDrop(DropFanoutFull, "tymux")
	r.CountDrop("client-chosen", "weird")

	got := r.DropCounts()
	require.Len(t, got, 3)
	assert.Equal(t, DropCount{Cause: "hub_queue_full", Backend: "tymux", Total: 2}, got[0])
	assert.Equal(t, DropCount{Cause: "tymux_fanout_full", Backend: "tymux", Total: 1}, got[1])
	assert.Equal(t, DropCount{Cause: "unknown", Backend: "unknown", Total: 1}, got[2])
}

func TestHotSessions_should_CapDistinctLabelsAndFoldTheRestIntoOther(t *testing.T) {
	t.Parallel()
	c := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	h := NewHotSessions(HotSessionsConfig{Max: 2, Lifetime: 100, Window: time.Minute}, c.now)

	assert.Equal(t, "a", h.Label("a"))
	assert.Equal(t, "b", h.Label("b"))
	assert.Equal(t, "other", h.Label("c"), "a third session with fewer hits than the members is folded")

	// c becomes busier than the least active member and takes its slot.
	for range 5 {
		h.Label("c")
	}
	assert.Equal(t, "c", h.Label("c"))
}

func TestHotSessions_should_StopMintingLabelsAtTheLifetimeCap(t *testing.T) {
	t.Parallel()
	c := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	h := NewHotSessions(HotSessionsConfig{Max: 5, Lifetime: 3, Window: time.Minute}, c.now)
	for _, id := range []string{"a", "b", "c"} {
		assert.Equal(t, id, h.Label(id))
	}
	c.advance(2 * time.Minute) // window rolls: membership resets, lifetime set does not
	assert.Equal(t, "other", h.Label("d"))
	assert.Equal(t, "a", h.Label("a"), "an already-minted label is still usable")
}

func TestHotSessions_should_NotEvictAMemberForALabelItCannotMint(t *testing.T) {
	t.Parallel()
	h := NewHotSessions(HotSessionsConfig{Max: 2, Lifetime: 2, Window: time.Minute}, nil)
	h.Label("a")
	h.Label("b")
	for range 5 {
		assert.Equal(t, "other", h.Label("new"))
	}
	assert.Equal(t, "a", h.Label("a"))
	assert.Equal(t, "b", h.Label("b"))
}

func TestHotSessions_should_TruncateLongIDs(t *testing.T) {
	t.Parallel()
	h := NewHotSessions(HotSessionsConfig{Max: 2, Lifetime: 10, Window: time.Minute}, nil)
	got := h.Label("0123456789012345678901234567890123456789")
	assert.LessOrEqual(t, len(got), maxSessionLabelLen)
}
