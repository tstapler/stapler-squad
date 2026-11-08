package tymux

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A frame dropped for a full subscriber leaves a hole in the byte stream the browser parses
// statefully; the fanout must say so, and the repaint must replace the stale queue for exactly
// the subscribers that lost something.
func TestClientFanout_RepaintDropped_ReplacesStaleQueueOfOverflowedSubscriberOnly(t *testing.T) {
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

	assert.Equal(t, 1, f.RepaintDropped([]byte("REPAINT")))
	assert.Equal(t, 0, f.RepaintDropped([]byte("again")), "the dropped flag clears once repainted")

	require.Len(t, slow, 1)
	assert.Equal(t, []byte("REPAINT"), <-slow)
	assert.Empty(t, healthy)
}
