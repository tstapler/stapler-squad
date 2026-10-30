package services

import (
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func abandonGoroutines() int {
	var sb strings.Builder
	_ = pprof.Lookup("goroutine").WriteTo(&sb, 2)
	return strings.Count(sb.String(), "(*unarySession).abandon(")
}

// A deadline hit while the zombie cap is full parks abandon waiting for the handler;
// a later CANCEL that finds zombie room must wake it instead of leaking it.
func TestUnaryWsBridge_should_NotLeakAbandonWaiter_When_CancelFreesSlotAfterCapRefusal(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal, WithUnaryMaxInFlight(4), WithUnaryMaxZombies(1))
	c := h.connect(nil)
	m := h.bridge.Metrics()

	keyA := startStuck(t, h, c, 1, unaryRequestMeta{})
	keyB := startStuck(t, h, c, 3, unaryRequestMeta{TimeoutMs: 30})
	sendCancel(t, c, 1) // A becomes the one allowed zombie
	require.Eventually(t, func() bool { return m.Zombies() == 1 }, readWait, time.Millisecond)

	f := readFrame(t, c) // B's deadline: reply sent, detach refused at cap
	require.EqualValues(t, 3, f.ID)
	assert.Equal(t, "deadline_exceeded", f.Error.Code)
	require.Eventually(t, func() bool { return abandonGoroutines() == 1 }, readWait, time.Millisecond, "abandon waits for B's handler")

	h.stub.release(keyA) // frees zombie room
	require.Eventually(t, func() bool { return m.Zombies() == 0 }, readWait, time.Millisecond)
	sendCancel(t, c, 3) // now succeeds in detaching B
	require.Eventually(t, func() bool { return abandonGoroutines() == 0 }, readWait, time.Millisecond,
		"the waiter must exit once B is detached, not stay parked until socket close")

	h.stub.release(keyB)
	require.Eventually(t, func() bool { return m.Zombies() == 0 && m.Inflight() == 0 }, readWait, time.Millisecond)
}
