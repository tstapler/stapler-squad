package services

import (
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// stuckBurst equals the harness in-flight cap: the burst that used to exhaust it.
const stuckBurst = 6

// startStuck sends a ctx-ignoring call and waits until its handler is running.
func startStuck(t *testing.T, h *bridgeHarness, c *websocket.Conn, id uint32, meta unaryRequestMeta) string {
	t.Helper()
	key := "stuck:" + strconv.Itoa(int(id))
	sendRequest(t, c, id, listFilesMethod, meta, mustProto(t, &sessionv1.ListFilesRequest{SessionId: key}))
	require.Equal(t, key, <-h.stub.started)
	return key
}

func sendCancel(t *testing.T, c *websocket.Conn, id uint32) {
	t.Helper()
	require.NoError(t, c.WriteMessage(websocket.BinaryMessage, encodeUnaryControl(unaryFrameCancel, id)))
}

func requireHealthyCall(t *testing.T, c *websocket.Conn, id uint32) {
	t.Helper()
	sendListFiles(t, c, id, "healthy")
	f := readFrame(t, c)
	require.Equal(t, unaryFrameResponse, f.Type, "healthy call %d must be answered, got id=%d %+v", id, f.ID, f.Error)
}

func TestUnaryWsBridge_should_FreeSlots_When_CtxIgnoringHandlersAreCanceled(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal, WithUnaryMaxInFlight(stuckBurst))
	c := h.connect(nil)
	var keys []string
	for i := 0; i < stuckBurst; i++ {
		keys = append(keys, startStuck(t, h, c, uint32(2*i+1), unaryRequestMeta{}))
	}
	for i := range keys {
		sendCancel(t, c, uint32(2*i+1))
	}
	requireHealthyCall(t, c, 1<<20)
	assert.EqualValues(t, stuckBurst, h.bridge.Metrics().Zombies(), "abandoned handlers are tracked as zombies")

	for _, k := range keys {
		h.stub.release(k)
	}
	require.Eventually(t, func() bool { return h.bridge.Metrics().Zombies() == 0 }, readWait, time.Millisecond)
	assert.Zero(t, h.bridge.Metrics().Inflight())
}

func TestUnaryWsBridge_should_ReplyDeadlineExceeded_And_FreeSlots_When_HandlerIgnoresCtx(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal, WithUnaryMaxInFlight(stuckBurst))
	c := h.connect(nil)
	var keys []string
	for i := 0; i < stuckBurst; i++ {
		keys = append(keys, startStuck(t, h, c, uint32(2*i+1), unaryRequestMeta{TimeoutMs: 30}))
	}
	for range keys {
		f := readFrame(t, c)
		require.Equal(t, unaryFrameError, f.Type)
		assert.Equal(t, "deadline_exceeded", f.Error.Code)
	}
	requireHealthyCall(t, c, 1<<20)
	assert.EqualValues(t, stuckBurst, h.bridge.Metrics().Abandoned(abandonDeadline))
	for _, k := range keys {
		h.stub.release(k)
	}
	require.Eventually(t, func() bool { return h.bridge.Metrics().Zombies() == 0 }, readWait, time.Millisecond)
}

func TestUnaryWsBridge_should_CapZombies_And_HoldSlotsBeyondCap(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal, WithUnaryMaxInFlight(4), WithUnaryMaxZombies(2))
	c := h.connect(nil)
	var keys []string
	for i := 0; i < 4; i++ {
		keys = append(keys, startStuck(t, h, c, uint32(2*i+1), unaryRequestMeta{}))
	}
	for i := range keys {
		sendCancel(t, c, uint32(2*i+1))
	}
	m := h.bridge.Metrics()
	require.Eventually(t, func() bool { return m.Zombies() == 2 && m.Inflight() == 2 }, readWait, time.Millisecond,
		"two zombies freed their slots; the rest keep theirs as backpressure")
	assert.EqualValues(t, 2, m.Zombies(), "zombie count never exceeds the cap")

	for _, k := range keys {
		h.stub.release(k)
	}
	require.Eventually(t, func() bool { return m.Zombies() == 0 && m.Inflight() == 0 }, readWait, time.Millisecond)
	requireHealthyCall(t, c, 1<<20)
}

func TestUnaryWsBridge_should_NotLeakGoroutines_Over100StuckCancelCycles(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())
	h := newBridgeHarness(t, ListenerLocal, WithUnaryMaxZombies(1000))
	c := h.connect(nil)
	var keys []string
	for i := 0; i < 100; i++ {
		id := uint32(2*i + 1)
		keys = append(keys, startStuck(t, h, c, id, unaryRequestMeta{}))
		sendCancel(t, c, id)
		if i%2 == 0 {
			requireHealthyCall(t, c, id+1000001)
		}
	}
	for _, k := range keys {
		h.stub.release(k)
	}
	require.Eventually(t, func() bool { return h.bridge.Metrics().Zombies() == 0 }, readWait, time.Millisecond)
	_ = c.Close()
	h.cancel()
	h.srv.Close()
}
