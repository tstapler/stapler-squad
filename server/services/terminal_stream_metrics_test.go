package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTerminalCreation_should_TrackInFlightAndEndExactlyOnce(t *testing.T) {
	base := terminalCreationsInFlight.Load()
	clock := time.Unix(0, 0)
	now := func() time.Time { return clock }

	c := beginTerminalCreationAt(now)
	require.Equal(t, base+1, terminalCreationsInFlight.Load())

	clock = clock.Add(120 * time.Millisecond)
	require.Equal(t, 120*time.Millisecond, c.Mark(stageUpgrade))
	clock = clock.Add(30 * time.Millisecond)
	require.Equal(t, 30*time.Millisecond, c.Mark(stageHandshake), "each stage measures from the previous mark")

	c.End(true)
	c.End(false) // second End must not decrement again
	require.Equal(t, base, terminalCreationsInFlight.Load())
}

func TestTerminalCreation_should_BeNilSafe(t *testing.T) {
	var c *terminalCreation
	require.Zero(t, c.Mark(stageHubAttach))
	c.End(true)
}
