package services

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const muxTestWait = 5 * time.Second

type muxTestClient struct {
	t    *testing.T
	conn *websocket.Conn
	in   chan muxFrame

	partial map[uint32][]byte
	done    map[uint32][]byte
}

func newMuxTestClient(t *testing.T, serve muxServeFunc, poolSize int) (*muxTestClient, *TerminalMuxBridge) {
	t.Helper()
	bridge := NewTerminalMuxBridge(serve, poolSize)
	srv := httptest.NewServer(WithRequestRecord(ListenerLocal, false)(bridge))
	t.Cleanup(srv.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	c := &muxTestClient{t: t, conn: conn, in: make(chan muxFrame, 1024), partial: map[uint32][]byte{}, done: map[uint32][]byte{}}
	go func() {
		defer close(c.in)
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			f, derr := decodeMuxFrame(append([]byte(nil), msg...))
			if derr != nil {
				return
			}
			c.in <- f
		}
	}()
	t.Cleanup(func() { _ = conn.Close() })
	return c, bridge
}

func (c *muxTestClient) send(frame []byte) {
	c.t.Helper()
	require.NoError(c.t, c.conn.WriteMessage(websocket.BinaryMessage, frame))
}

// next returns the next frame matching want (other types are skipped), failing on timeout.
func (c *muxTestClient) next(want muxFrameType) muxFrame {
	c.t.Helper()
	timeout := time.After(muxTestWait)
	for {
		select {
		case f, ok := <-c.in:
			require.True(c.t, ok, "socket closed waiting for frame type %d", want)
			if f.Type == want {
				return f
			}
		case <-timeout:
			c.t.Fatalf("timed out waiting for frame type %d", want)
		}
	}
}

// collect reads DATA frames until stream id's message is complete. Other streams'
// frames are buffered, not dropped, so collect order does not matter.
func (c *muxTestClient) collect(id uint32) []byte {
	c.t.Helper()
	for {
		if msg, ok := c.done[id]; ok && len(msg) > 0 {
			delete(c.done, id)
			return msg
		}
		f := c.next(muxFrameData)
		c.partial[f.ID] = append(c.partial[f.ID], f.Payload...)
		if f.Fin {
			c.done[f.ID] = c.partial[f.ID]
			delete(c.partial, f.ID)
		}
	}
}

func echoServe(conn streamConn, _ string) {
	for {
		kind, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if conn.WriteMessage(kind, data) != nil {
			return
		}
	}
}

func TestMuxFrame_should_RoundTrip_When_EncodedThenDecoded(t *testing.T) {
	t.Parallel()
	cases := map[string][]byte{
		"data":   encodeMuxData(7, websocket.BinaryMessage, true, []byte("hi")),
		"credit": encodeMuxCredit(7, 4096),
		"close":  encodeMuxClose(7, 1013, "full"),
		"hello":  encodeMuxConfig(muxFrameHello, muxConfig{PoolSize: 2, Window: 10, Chunk: 5, MaxStreams: 3}),
		"open":   encodeMuxEmpty(muxFrameOpen, 7),
	}
	for name, raw := range cases {
		f, err := decodeMuxFrame(raw)
		require.NoError(t, err, name)
		assert.Equal(t, muxFrameType(raw[0]), f.Type, name)
	}
	d, _ := decodeMuxFrame(cases["data"])
	assert.Equal(t, uint32(7), d.ID)
	assert.True(t, d.Fin)
	assert.Equal(t, []byte("hi"), d.Payload)
	c, _ := decodeMuxFrame(cases["credit"])
	assert.Equal(t, uint32(4096), c.Credit)
	cl, _ := decodeMuxFrame(cases["close"])
	assert.Equal(t, uint16(1013), cl.Code)
	assert.Equal(t, "full", cl.Reason)
}

func TestMuxFrame_should_Reject_When_Malformed(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string][]byte{
		"empty":        nil,
		"short":        {1, 0, 0},
		"unknown":      {99, 0, 0, 0, 1},
		"data-no-kind": {byte(muxFrameData), 0, 0, 0, 1},
		"data-bad":     {byte(muxFrameData), 0, 0, 0, 1, 9, 0},
		"credit-short": {byte(muxFrameCredit), 0, 0, 0, 1, 0},
		"close-short":  {byte(muxFrameClose), 0, 0, 0, 1, 0},
		"hello-json":   {byte(muxFrameHello), 0, 0, 0, 0, '{'},
	} {
		_, err := decodeMuxFrame(raw)
		assert.ErrorIs(t, err, errMalformedMuxFrame, name)
	}
}

func TestMuxBridge_should_AdvertisePoolSize_When_SocketConnects(t *testing.T) {
	t.Parallel()
	c, bridge := newMuxTestClient(t, echoServe, 3)
	hello := c.next(muxFrameHello)
	assert.Equal(t, 3, hello.Config.PoolSize)
	assert.Equal(t, muxStreamWindow, hello.Config.Window)

	bridge.SetPoolSize(5)
	cfg := c.next(muxFrameConfig)
	assert.Equal(t, 5, cfg.Config.PoolSize)
}

func TestMuxBridge_should_Return404_When_PoolSizeIsZero(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(WithRequestRecord(ListenerLocal, false)(NewTerminalMuxBridge(echoServe, 0)))
	defer srv.Close()
	_, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestMuxBridge_should_EchoPerStream_When_TwoStreamsShareASocket(t *testing.T) {
	t.Parallel()
	c, _ := newMuxTestClient(t, echoServe, 2)
	c.send(encodeMuxEmpty(muxFrameOpen, 1))
	c.send(encodeMuxEmpty(muxFrameOpen, 2))
	c.send(encodeMuxData(1, websocket.TextMessage, true, []byte("one")))
	c.send(encodeMuxData(2, websocket.BinaryMessage, true, []byte("two")))
	assert.Equal(t, []byte("one"), c.collect(1))
	assert.Equal(t, []byte("two"), c.collect(2))
}

func TestMuxBridge_should_ChunkAndReassemble_When_MessageExceedsChunk(t *testing.T) {
	t.Parallel()
	payload := []byte(strings.Repeat("x", muxChunkBytes*2+10))
	c, _ := newMuxTestClient(t, echoServe, 1)
	c.send(encodeMuxEmpty(muxFrameOpen, 1))
	// client -> server in two chunks
	c.send(encodeMuxData(1, websocket.BinaryMessage, false, payload[:muxChunkBytes]))
	c.send(encodeMuxData(1, websocket.BinaryMessage, true, payload[muxChunkBytes:]))
	var got []byte
	frames := 0
	for {
		f := c.next(muxFrameData)
		frames++
		assert.LessOrEqual(t, len(f.Payload), muxChunkBytes)
		got = append(got, f.Payload...)
		if f.Fin {
			break
		}
	}
	assert.Equal(t, payload, got)
	assert.Equal(t, 3, frames)
}

// A stream whose client grants no credit parks its own queue; another stream's output
// still flows (no head-of-line blocking).
func TestMuxBridge_should_NotBlockOtherStreams_When_OneStreamHasNoCredit(t *testing.T) {
	t.Parallel()
	big := make([]byte, 4*muxStreamWindow)
	started := make(chan struct{})
	serve := func(conn streamConn, _ string) {
		kind, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if string(data) == "slow" {
			close(started)
			_ = conn.WriteMessage(kind, big) // far above the stream's window
			_, _, _ = conn.ReadMessage()     // hold open until the client closes
			return
		}
		_ = conn.WriteMessage(kind, []byte("fast-reply"))
	}
	c, _ := newMuxTestClient(t, serve, 1)
	c.send(encodeMuxEmpty(muxFrameOpen, 1))
	c.send(encodeMuxData(1, websocket.BinaryMessage, true, []byte("slow")))
	<-started

	// Drain exactly the window for stream 1, then it is out of credit.
	var drained int
	for drained < muxStreamWindow {
		f := c.next(muxFrameData)
		require.Equal(t, uint32(1), f.ID)
		drained += len(f.Payload)
	}
	require.Equal(t, muxStreamWindow, drained)

	c.send(encodeMuxEmpty(muxFrameOpen, 2))
	c.send(encodeMuxData(2, websocket.BinaryMessage, true, []byte("fast")))
	assert.Equal(t, []byte("fast-reply"), c.collect(2))

	// Granting credit as frames arrive (what the client pool does) resumes the parked stream.
	c.send(encodeMuxCredit(1, muxStreamWindow))
	var rest int
	for drained+rest < len(big) {
		f := c.next(muxFrameData)
		if f.ID == 1 {
			rest += len(f.Payload)
			c.send(encodeMuxCredit(1, len(f.Payload)))
		}
	}
	assert.Equal(t, len(big), drained+rest)
}

func TestMuxBridge_should_RefuseOpen_When_StreamCapReached(t *testing.T) {
	t.Parallel()
	hold := make(chan struct{})
	t.Cleanup(func() { close(hold) })
	serve := func(conn streamConn, _ string) { <-hold }
	c, _ := newMuxTestClient(t, serve, 1)
	for id := uint32(1); id <= muxMaxStreamsPerSocket; id++ {
		c.send(encodeMuxEmpty(muxFrameOpen, id))
	}
	c.send(encodeMuxEmpty(muxFrameOpen, muxMaxStreamsPerSocket+1))
	f := c.next(muxFrameClose)
	assert.Equal(t, uint32(muxMaxStreamsPerSocket+1), f.ID)
	assert.Equal(t, uint16(websocket.CloseTryAgainLater), f.Code)
}

func TestMuxStream_should_InterruptRead_When_ReadDeadlineSetToNow(t *testing.T) {
	t.Parallel()
	got := make(chan error, 1)
	serve := func(conn streamConn, _ string) {
		time.AfterFunc(10*time.Millisecond, func() { _ = conn.SetReadDeadline(time.Now()) })
		_, _, err := conn.ReadMessage()
		got <- err
	}
	c, _ := newMuxTestClient(t, serve, 1)
	c.send(encodeMuxEmpty(muxFrameOpen, 1))
	select {
	case err := <-got:
		assert.True(t, errors.Is(err, os.ErrDeadlineExceeded), "got %v", err)
	case <-time.After(muxTestWait):
		t.Fatal("ReadMessage was not interrupted by SetReadDeadline")
	}
}

func TestMuxStream_should_FlushThenClose_When_HandlerReturns(t *testing.T) {
	t.Parallel()
	serve := func(conn streamConn, _ string) {
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("goodbye"))
	}
	c, _ := newMuxTestClient(t, serve, 1)
	c.send(encodeMuxEmpty(muxFrameOpen, 1))
	assert.Equal(t, []byte("goodbye"), c.collect(1))
	f := c.next(muxFrameClose)
	assert.Equal(t, uint16(websocket.CloseNormalClosure), f.Code)
}

func TestMuxStream_should_SurfaceCloseError_When_ClientClosesStream(t *testing.T) {
	t.Parallel()
	got := make(chan error, 1)
	serve := func(conn streamConn, _ string) {
		_, _, err := conn.ReadMessage()
		got <- err
	}
	c, _ := newMuxTestClient(t, serve, 1)
	c.send(encodeMuxEmpty(muxFrameOpen, 1))
	c.send(encodeMuxClose(1, websocket.CloseNormalClosure, ""))
	err := <-got
	assert.True(t, websocket.IsCloseError(err, websocket.CloseNormalClosure), "got %v", err)
}

func TestMuxBridge_should_EndHandlers_When_SocketDrops(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	wg.Add(2)
	serve := func(conn streamConn, _ string) {
		defer wg.Done()
		_, _, _ = conn.ReadMessage()
	}
	c, _ := newMuxTestClient(t, serve, 1)
	c.send(encodeMuxEmpty(muxFrameOpen, 1))
	c.send(encodeMuxEmpty(muxFrameOpen, 2))
	require.NoError(t, c.conn.Close())
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(muxTestWait):
		t.Fatal("handlers did not exit after the socket dropped")
	}
}

func TestMuxStream_should_FailWrite_When_QueueFullAndDeadlinePasses(t *testing.T) {
	t.Parallel()
	res := make(chan error, 1)
	serve := func(conn streamConn, _ string) {
		// First message fills the queue far past the window; the second must wait for room.
		_ = conn.WriteMessage(websocket.BinaryMessage, make([]byte, 2*muxStreamQueueBytes))
		_ = conn.SetWriteDeadline(time.Now().Add(50 * time.Millisecond))
		res <- conn.WriteMessage(websocket.BinaryMessage, []byte("more"))
	}
	c, _ := newMuxTestClient(t, serve, 1)
	c.send(encodeMuxEmpty(muxFrameOpen, 1))
	select {
	case err := <-res:
		assert.ErrorIs(t, err, os.ErrDeadlineExceeded)
	case <-time.After(muxTestWait):
		t.Fatal("blocked write ignored its deadline")
	}
}

type muxVector struct {
	Name  string `json:"name"`
	Hex   string `json:"hex"`
	Frame struct {
		Type       string `json:"type"`
		ID         uint32 `json:"id"`
		Kind       int    `json:"kind"`
		Fin        bool   `json:"fin"`
		PayloadHex string `json:"payloadHex"`
		Credit     uint32 `json:"credit"`
		Code       uint16 `json:"code"`
		Reason     string `json:"reason"`
	} `json:"frame"`
}

// The same vectors drive web-app/src/lib/transport/terminal-mux-frame.test.ts, so the two
// codecs cannot drift apart.
func TestMuxFrame_should_MatchSharedVectors(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../web-app/src/lib/transport/__fixtures__/terminal-mux-vectors.json")
	require.NoError(t, err)
	var vectors []muxVector
	require.NoError(t, json.Unmarshal(raw, &vectors))
	require.NotEmpty(t, vectors)
	for _, v := range vectors {
		wire, err := hex.DecodeString(v.Hex)
		require.NoError(t, err, v.Name)
		f, err := decodeMuxFrame(wire)
		require.NoError(t, err, v.Name)
		assert.Equal(t, v.Frame.ID, f.ID, v.Name)
		switch v.Frame.Type {
		case "DATA":
			want, _ := hex.DecodeString(v.Frame.PayloadHex)
			assert.Equal(t, want, f.Payload, v.Name)
			assert.Equal(t, v.Frame.Fin, f.Fin, v.Name)
			assert.Equal(t, v.Frame.Kind, f.Kind, v.Name)
			assert.Equal(t, wire, encodeMuxData(f.ID, f.Kind, f.Fin, f.Payload), v.Name)
		case "CREDIT":
			assert.Equal(t, v.Frame.Credit, f.Credit, v.Name)
			assert.Equal(t, wire, encodeMuxCredit(f.ID, int(f.Credit)), v.Name)
		case "CLOSE":
			assert.Equal(t, v.Frame.Code, f.Code, v.Name)
			assert.Equal(t, v.Frame.Reason, f.Reason, v.Name)
			assert.Equal(t, wire, encodeMuxClose(f.ID, f.Code, f.Reason), v.Name)
		}
	}
}

// A client that reuses a stream id after closing it must get a working new stream even
// though the old handler is still unwinding.
func TestMuxBridge_should_KeepNewStream_When_IdReusedWhileOldHandlerUnwinds(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	serve := func(conn streamConn, p string) {
		kind, data, err := conn.ReadMessage()
		if err != nil { // the old stream: it sees the client CLOSE but lingers
			<-release
			return
		}
		_ = conn.WriteMessage(kind, data)
		echoServe(conn, p)
	}
	c, _ := newMuxTestClient(t, serve, 1)
	c.send(encodeMuxEmpty(muxFrameOpen, 1))
	c.send(encodeMuxClose(1, websocket.CloseNormalClosure, ""))
	c.send(encodeMuxEmpty(muxFrameOpen, 1))
	c.send(encodeMuxData(1, websocket.BinaryMessage, true, []byte("again")))
	assert.Equal(t, []byte("again"), c.collect(1))
	close(release) // old handler returns only now; it must not evict the new stream
	c.send(encodeMuxData(1, websocket.BinaryMessage, true, []byte("still")))
	assert.Equal(t, []byte("still"), c.collect(1))
}

func TestMuxStream_should_AbortWith1013_When_NoCreditForStallTimeout(t *testing.T) {
	t.Parallel()
	serve := func(conn streamConn, _ string) {
		_ = conn.WriteMessage(websocket.BinaryMessage, make([]byte, 2*muxStreamWindow))
		_, _, _ = conn.ReadMessage()
	}
	c, bridge := newMuxTestClient(t, serve, 1)
	bridge.stallTimeout = 50 * time.Millisecond // before any stream exists
	c.send(encodeMuxEmpty(muxFrameOpen, 1))
	f := c.next(muxFrameClose)
	assert.Equal(t, uint16(websocket.CloseTryAgainLater), f.Code)
}

func TestMuxBridge_should_RefuseUpgrade_When_RemoteListenerLacksAuthOrOriginMatches(t *testing.T) {
	t.Parallel()
	dial := func(listener string, requiresAuth bool, origin string) int {
		bridge := NewTerminalMuxBridge(echoServe, 1,
			WithTerminalMuxOriginPolicy(ListenerRemote, func(r *http.Request) bool { return r.Header.Get("Origin") == "https://ok.example" }))
		srv := httptest.NewServer(WithRequestRecord(listener, requiresAuth)(bridge))
		defer srv.Close()
		h := http.Header{}
		if origin != "" {
			h.Set("Origin", origin)
		}
		conn, resp, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), h)
		if err == nil {
			_ = conn.Close()
			return http.StatusSwitchingProtocols
		}
		return resp.StatusCode
	}
	assert.Equal(t, http.StatusForbidden, dial(ListenerRemote, false, "https://ok.example"), "remote without auth")
	assert.Equal(t, http.StatusForbidden, dial(ListenerRemote, true, "https://evil.example"), "remote bad origin")
	assert.Equal(t, http.StatusSwitchingProtocols, dial(ListenerRemote, true, "https://ok.example"))
	assert.Equal(t, http.StatusForbidden, dial(ListenerLocal, false, "https://evil.example"), "local non-loopback origin")
}
