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

func TestUnaryWsBridge_should_CloseSocket_And_Recover_When_ZombieCapReached(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal, WithUnaryMaxInFlight(4), WithUnaryMaxZombies(2))
	c := h.connect(nil)
	var keys []string
	for i := 0; i < 4; i++ {
		keys = append(keys, startStuck(t, h, c, uint32(2*i+1), unaryRequestMeta{}))
	}
	for i := 0; i < 4; i++ {
		sendCancel(t, c, uint32(2*i+1)) // the third CANCEL finds the cap full
	}
	require.NoError(t, c.SetReadDeadline(time.Now().Add(readWait)))
	_, _, err := c.ReadMessage()
	require.Error(t, err, "the server closes the socket so stuck handlers cannot pin its slots")

	m := h.bridge.Metrics()
	assert.EqualValues(t, 1, m.Abandoned(abandonCapped), "counted once per socket, not per repeated CANCEL")
	for _, k := range keys {
		h.stub.release(k)
	}
	require.Eventually(t, func() bool { return m.Zombies() == 0 && m.Inflight() == 0 }, readWait, time.Millisecond)

	c2 := h.connect(nil) // a fresh socket is healthy
	requireHealthyCall(t, c2, 1)
}

func TestUnaryWsBridge_should_RefuseUpgrade_When_SocketOrZombieLimitReached(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal, WithUnaryMaxSockets(1))
	_ = h.connect(nil)
	_, _, err := h.dial(nil)
	require.Error(t, err)
	assert.EqualValues(t, 1, h.bridge.Metrics().UpgradeDenied(ListenerLocal, "socket_limit"))

	h2 := newBridgeHarness(t, ListenerLocal)
	h2.bridge.Metrics().zombieDelta(wsUnaryMaxZombiesTotal)
	_, _, err = h2.dial(nil)
	require.Error(t, err)
	assert.EqualValues(t, 1, h2.bridge.Metrics().UpgradeDenied(ListenerLocal, "zombie_limit"))
	h2.bridge.Metrics().zombieDelta(-wsUnaryMaxZombiesTotal)
}

func TestParseMaxInFlight_should_ClampAndDefault(t *testing.T) {
	assert.Equal(t, wsUnaryMaxInFlight, parseMaxInFlight(""))
	assert.Equal(t, wsUnaryMaxInFlight, parseMaxInFlight("nope"))
	assert.Equal(t, wsUnaryMaxInFlight, parseMaxInFlight("-3"))
	assert.Equal(t, 100, parseMaxInFlight("100"))
	assert.Equal(t, wsUnaryMaxInFlightLimit, parseMaxInFlight("100000"))
}

func TestUnaryWsBridge_should_ReturnResourceExhausted_And_ReleaseBudget_When_BufferBudgetExceeded(t *testing.T) {
	const budget = 1 << 20
	h := newBridgeHarness(t, ListenerLocal, WithUnaryBufferBudget(budget))
	c := h.connect(nil)

	sendRequest(t, c, 1, fileContentMethod, unaryRequestMeta{}, mustProto(t, &sessionv1.GetFileContentRequest{Path: strconv.Itoa(2 * budget)}))
	f := readFrame(t, c)
	require.Equal(t, unaryFrameError, f.Type)
	assert.Equal(t, codeResourceExhausted, f.Error.Code)
	require.Eventually(t, func() bool { return h.bridge.buffered.Load() == 0 }, readWait, time.Millisecond, "failed charge is refunded")

	sendRequest(t, c, 3, fileContentMethod, unaryRequestMeta{}, mustProto(t, &sessionv1.GetFileContentRequest{Path: strconv.Itoa(budget / 2)}))
	assert.Equal(t, unaryFrameResponse, readFrame(t, c).Type, "a call within budget still succeeds")
	require.Eventually(t, func() bool { return h.bridge.buffered.Load() == 0 }, readWait, time.Millisecond, "budget released on completion")
}

func TestUnaryWsBridge_should_ReleaseBudget_When_ZombieHandlerReturns(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)
	key := startStuck(t, h, c, 1, unaryRequestMeta{TimeoutMs: 20})
	assert.Equal(t, "deadline_exceeded", readFrame(t, c).Error.Code)
	h.stub.release(key) // zombie returns with a ~KB response buffered
	require.Eventually(t, func() bool { return h.bridge.Metrics().Zombies() == 0 && h.bridge.buffered.Load() == 0 }, readWait, time.Millisecond)
}

func TestUnarySession_should_NotAccountZombie_When_HandlerAlreadyFinished(t *testing.T) {
	s := &unarySession{inflight: map[uint32]*inflightCall{5: {cancel: func() {}, handlerDone: true}}, b: &UnaryWsBridge{metrics: newUnaryMetrics(), maxZombies: 1}}
	s.mu.Lock()
	ok := s.detachLocked(5, s.inflight[5], abandonShutdown, true)
	s.mu.Unlock()
	assert.True(t, ok)
	assert.Zero(t, s.zombies, "a handler that already returned must not leak a zombie")
	assert.Zero(t, s.b.metrics.Zombies())
}

func TestUnaryWsBridge_should_AnswerOnce_When_CancelFollowsDeadline(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)
	key := startStuck(t, h, c, 1, unaryRequestMeta{TimeoutMs: 20})
	f := readFrame(t, c)
	require.EqualValues(t, 1, f.ID)
	assert.Equal(t, "deadline_exceeded", f.Error.Code)
	sendCancel(t, c, 1)
	require.NoError(t, c.WriteMessage(websocket.BinaryMessage, encodeUnaryControl(unaryFramePing, 8)))
	next := readFrame(t, c)
	assert.Equal(t, unaryFramePong, next.Type, "no second reply for id 1 after the CANCEL")
	h.stub.release(key)
	require.Eventually(t, func() bool { return h.bridge.Metrics().Zombies() == 0 }, readWait, time.Millisecond)
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
