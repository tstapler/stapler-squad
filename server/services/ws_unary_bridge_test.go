package services

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
)

const (
	listFilesMethod   = "/session.v1.SessionService/ListFiles"
	fileContentMethod = "/session.v1.SessionService/GetFileContent"
	writeFileMethod   = "/session.v1.SessionService/WriteFile"
	readWait          = 10 * time.Second
)

// bridgeStub is a real connect handler whose ListFiles behavior is chosen by
// SessionId: "gate:<k>" blocks until released or its context ends.
type bridgeStub struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	mu       sync.Mutex
	gates    map[string]chan struct{}
	started  chan string
	canceled chan string
	hits     atomic.Int32
	seen     chan seenRequest
}

type seenRequest struct {
	host, origin, remote, cookie, contentType, protoVersion, timeoutMs string
}

func newBridgeStub() *bridgeStub {
	return &bridgeStub{
		gates:    map[string]chan struct{}{},
		started:  make(chan string, 64),
		canceled: make(chan string, 64),
		seen:     make(chan seenRequest, 64),
	}
}

func (s *bridgeStub) gate(key string) chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gates[key] == nil {
		s.gates[key] = make(chan struct{})
	}
	return s.gates[key]
}

func (s *bridgeStub) release(key string) { close(s.gate(key)) }

func (s *bridgeStub) ListFiles(ctx context.Context, req *connect.Request[sessionv1.ListFilesRequest]) (*connect.Response[sessionv1.ListFilesResponse], error) {
	s.hits.Add(1)
	id := req.Msg.SessionId
	if strings.HasPrefix(id, "stuck:") { // ignores ctx, like a git subprocess under a mutex
		s.started <- id
		<-s.gate(id)
	}
	if strings.HasPrefix(id, "gate:") {
		s.started <- id
		select {
		case <-s.gate(id):
		case <-ctx.Done():
			s.canceled <- id
			return nil, ctx.Err()
		}
	}
	resp := connect.NewResponse(&sessionv1.ListFilesResponse{BasePath: "ok:" + id})
	resp.Header().Set("X-Test-Header", "h1")
	resp.Trailer().Set("X-Test-Trailer", "t1")
	return resp, nil
}

func (s *bridgeStub) GetFileContent(_ context.Context, req *connect.Request[sessionv1.GetFileContentRequest]) (*connect.Response[sessionv1.GetFileContentResponse], error) {
	n, _ := strconv.Atoi(req.Msg.Path)
	return connect.NewResponse(&sessionv1.GetFileContentResponse{Content: strings.Repeat("x", n)}), nil
}

type bridgeHarness struct {
	t      *testing.T
	stub   *bridgeStub
	bridge *UnaryWsBridge
	srv    *httptest.Server
	cancel context.CancelFunc
	dialer *websocket.Dialer
}

func newBridgeHarness(t *testing.T, listener string, opts ...UnaryWsOption) *bridgeHarness {
	t.Helper()
	return newBridgeHarnessTLS(t, listener, false, opts...)
}

// newBridgeHarnessTLS serves the bridge over wss:// (httptest TLS) when useTLS is set.
func newBridgeHarnessTLS(t *testing.T, listener string, useTLS bool, opts ...UnaryWsOption) *bridgeHarness {
	t.Helper()
	stub := newBridgeStub()
	_, h := sessionv1connect.NewSessionServiceHandler(stub)
	dispatcher := http.StripPrefix("/api", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case stub.seen <- seenRequest{
			host: r.Host, origin: r.Header.Get("Origin"), remote: r.RemoteAddr, cookie: r.Header.Get("Cookie"),
			contentType: r.Header.Get("Content-Type"), protoVersion: r.Header.Get("Connect-Protocol-Version"),
			timeoutMs: r.Header.Get("Connect-Timeout-Ms"),
		}:
		default:
		}
		h.ServeHTTP(w, r)
	}))
	bridge := NewUnaryWsBridge(func(l string) http.Handler {
		if l == listener {
			return dispatcher
		}
		return nil
	}, opts...)
	baseCtx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewUnstartedServer(WithRequestRecord(listener, listener == ListenerRemote)(bridge))
	srv.Config.BaseContext = func(_ net.Listener) context.Context { return baseCtx }
	dialer := websocket.DefaultDialer
	if useTLS {
		srv.StartTLS()
		dialer = &websocket.Dialer{TLSClientConfig: srv.Client().Transport.(*http.Transport).TLSClientConfig}
	} else {
		srv.Start()
	}
	t.Cleanup(func() { cancel(); srv.Close() })
	return &bridgeHarness{t: t, stub: stub, bridge: bridge, srv: srv, cancel: cancel, dialer: dialer}
}

func (h *bridgeHarness) dial(header http.Header) (*websocket.Conn, *http.Response, error) {
	u := "ws" + strings.TrimPrefix(h.srv.URL, "http") + UnaryWsPath
	return h.dialer.Dial(u, header)
}

func (h *bridgeHarness) connect(header http.Header) *websocket.Conn {
	h.t.Helper()
	c, _, err := h.dial(header)
	require.NoError(h.t, err)
	h.t.Cleanup(func() { _ = c.Close() })
	return c
}

func mustProto(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(m)
	require.NoError(t, err)
	return b
}

func sendRequest(t *testing.T, c *websocket.Conn, id uint32, method string, meta unaryRequestMeta, body []byte) {
	t.Helper()
	frame, err := encodeUnaryRequest(id, method, meta, body)
	require.NoError(t, err)
	require.NoError(t, c.WriteMessage(websocket.BinaryMessage, frame))
}

func sendListFiles(t *testing.T, c *websocket.Conn, id uint32, sessionID string) {
	t.Helper()
	sendRequest(t, c, id, listFilesMethod, unaryRequestMeta{}, mustProto(t, &sessionv1.ListFilesRequest{SessionId: sessionID}))
}

func readFrame(t *testing.T, c *websocket.Conn) unaryFrame {
	t.Helper()
	require.NoError(t, c.SetReadDeadline(time.Now().Add(readWait)))
	_, msg, err := c.ReadMessage()
	require.NoError(t, err)
	f, err := decodeUnaryFrame(msg)
	require.NoError(t, err)
	return f
}

func decodeListFiles(t *testing.T, f unaryFrame) *sessionv1.ListFilesResponse {
	t.Helper()
	require.Equal(t, unaryFrameResponse, f.Type, "frame: %+v", f)
	require.True(t, f.Fin)
	var out sessionv1.ListFilesResponse
	require.NoError(t, proto.Unmarshal(f.Body, &out))
	return &out
}

func TestUnaryWsBridge_should_DispatchAllowedRead_with_UpgradeIdentity(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	origin := "http://" + h.srv.Listener.Addr().String()
	c := h.connect(http.Header{"Origin": {origin}, "Cookie": {"cs_auth=tok"}, "X-Forwarded-For": {"203.0.113.9"}})

	sendRequest(t, c, 1, listFilesMethod, unaryRequestMeta{Headers: map[string]string{"Connect-Protocol-Version": "1"}, TimeoutMs: 30000},
		mustProto(t, &sessionv1.ListFilesRequest{SessionId: "echo"}))
	f := readFrame(t, c)

	assert.Equal(t, "ok:echo", decodeListFiles(t, f).BasePath, "decoded message equals the handler's")
	assert.Equal(t, "h1", f.ResponseMeta.Headers["X-Test-Header"])
	assert.Equal(t, "t1", f.ResponseMeta.Trailers["X-Test-Trailer"])
	assert.Empty(t, f.ResponseMeta.Headers["Set-Cookie"])

	seen := <-h.stub.seen
	assert.Equal(t, h.srv.Listener.Addr().String(), seen.host)
	assert.Equal(t, origin, seen.origin)
	assert.Equal(t, "cs_auth=tok", seen.cookie)
	assert.NotEmpty(t, seen.remote)
	assert.Equal(t, "application/proto", seen.contentType)
	assert.Equal(t, "1", seen.protoVersion)
	assert.Equal(t, "30000", seen.timeoutMs)
	assert.EqualValues(t, 1, h.bridge.Metrics().Calls(listFilesMethod, "ok"))
}

func TestUnaryWsBridge_should_AnswerOutOfOrder_When_FirstCallIsSlow(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)

	sendListFiles(t, c, 1, "gate:slow")
	require.Equal(t, "gate:slow", <-h.stub.started)
	sendListFiles(t, c, 3, "fast")

	f3 := readFrame(t, c)
	assert.EqualValues(t, 3, f3.ID)
	assert.Equal(t, "ok:fast", decodeListFiles(t, f3).BasePath)

	h.stub.release("gate:slow")
	f1 := readFrame(t, c)
	assert.EqualValues(t, 1, f1.ID)
	assert.Equal(t, "ok:gate:slow", decodeListFiles(t, f1).BasePath)
}

func TestUnaryWsBridge_should_DenyMethodsOutsideAllowlist(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)

	for i, m := range []string{writeFileMethod, "/session.v1.SessionService/UpdateFeatureFlag", "/nope.Svc/Whatever", "/../etc/passwd"} {
		sendRequest(t, c, uint32(2*i+1), m, unaryRequestMeta{}, nil)
		f := readFrame(t, c)
		require.Equal(t, unaryFrameError, f.Type, m)
		assert.Equal(t, codePermissionDenied, f.Error.Code, m)
		assert.JSONEq(t, `{"marker":"ws_method_not_allowed"}`, string(f.Error.Details), m)
	}
	assert.Zero(t, h.stub.hits.Load(), "denied methods never reach a handler")
	assert.EqualValues(t, 4, h.bridge.Metrics().Denied(wsUnaryMethodOther))
}

func TestUnaryWsBridge_should_RejectForbiddenMetaHeaders_Without_InvokingHandler(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)

	for i, key := range []string{"Origin", "Host", "X-Forwarded-For", "Cookie", "Authorization", "Connect-Timeout-Ms", "origin"} {
		sendRequest(t, c, uint32(2*i+1), listFilesMethod,
			unaryRequestMeta{Headers: map[string]string{key: "http://evil.example"}}, mustProto(t, &sessionv1.ListFilesRequest{SessionId: "x"}))
		f := readFrame(t, c)
		require.Equal(t, unaryFrameError, f.Type, key)
		assert.Equal(t, codeInvalidArgument, f.Error.Code, key)
	}
	assert.Zero(t, h.stub.hits.Load())
}

func TestUnaryWsBridge_should_ReturnResourceExhausted_When_InFlightCapExceeded(t *testing.T) {
	const limit = 6
	h := newBridgeHarness(t, ListenerLocal, WithUnaryMaxInFlight(limit))
	c := h.connect(nil)

	for i := 1; i <= limit; i++ {
		sendListFiles(t, c, uint32(2*i-1), "gate:"+strconv.Itoa(i))
		<-h.stub.started
	}
	sendListFiles(t, c, 99, "gate:7")
	f := readFrame(t, c)
	assert.EqualValues(t, 99, f.ID)
	assert.Equal(t, unaryFrameError, f.Type)
	assert.Equal(t, codeResourceExhausted, f.Error.Code)
	assert.EqualValues(t, limit, h.bridge.Metrics().Inflight())

	h.stub.release("gate:1")
	assert.EqualValues(t, 1, readFrame(t, c).ID, "a freed slot completes normally")
}

func TestUnaryWsBridge_should_CancelHandler_And_WriteNoResponse_When_CancelReceived(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)

	sendListFiles(t, c, 5, "gate:c")
	<-h.stub.started
	require.NoError(t, c.WriteMessage(websocket.BinaryMessage, encodeUnaryControl(unaryFrameCancel, 5)))
	assert.Equal(t, "gate:c", <-h.stub.canceled)
	require.Eventually(t, func() bool { return h.bridge.Metrics().Inflight() == 0 }, readWait, time.Millisecond)

	require.NoError(t, c.WriteMessage(websocket.BinaryMessage, encodeUnaryControl(unaryFramePing, 8)))
	f := readFrame(t, c)
	assert.Equal(t, unaryFramePong, f.Type, "the next frame is the PONG, so no RESPONSE or ERROR was written for id 5")
	assert.EqualValues(t, 8, f.ID)
}

func TestUnaryWsBridge_should_ReturnDeadlineExceeded_When_TimeoutMsElapses(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)

	sendRequest(t, c, 1, listFilesMethod, unaryRequestMeta{TimeoutMs: 50}, mustProto(t, &sessionv1.ListFilesRequest{SessionId: "gate:d"}))
	f := readFrame(t, c)
	assert.Equal(t, unaryFrameError, f.Type)
	assert.Equal(t, "deadline_exceeded", f.Error.Code)
}

func TestUnaryWsBridge_should_CancelAllInFlight_When_ServerContextEnds(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)
	for i := 1; i <= 3; i++ {
		sendListFiles(t, c, uint32(2*i-1), "gate:s"+strconv.Itoa(i))
		<-h.stub.started
	}
	h.cancel()
	got := map[string]bool{}
	for i := 0; i < 3; i++ {
		select {
		case id := <-h.stub.canceled:
			got[id] = true
		case <-time.After(readWait):
			t.Fatal("handler did not observe cancellation")
		}
	}
	assert.Len(t, got, 3)
}

func TestUnaryWsBridge_should_ReassembleLargeResponse(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)
	const size = 2 << 20

	sendRequest(t, c, 1, fileContentMethod, unaryRequestMeta{}, mustProto(t, &sessionv1.GetFileContentRequest{Path: strconv.Itoa(size)}))
	head := readFrame(t, c)
	require.Equal(t, unaryFrameResponse, head.Type)
	require.False(t, head.Fin)
	payload := append([]byte(nil), head.Body...)
	for {
		ch := readFrame(t, c)
		require.Equal(t, unaryFrameChunk, ch.Type)
		payload = append(payload, ch.Body...)
		if ch.Fin {
			break
		}
	}
	assert.EqualValues(t, head.TotalLen, len(payload))
	var out sessionv1.GetFileContentResponse
	require.NoError(t, proto.Unmarshal(payload, &out))
	assert.Len(t, out.Content, size)
}

func TestUnaryWriter_should_SendSmallResponseBeforeLargeChunks_And_InterleaveChunks(t *testing.T) {
	w := newUnaryWriter(nil)
	big := make([]byte, 3*unaryMaxFrameBytes)
	_, err := w.enqueueResponse(1, unaryResponseMeta{}, big, nil)
	require.NoError(t, err)
	_, err = w.enqueueResponse(3, unaryResponseMeta{}, big, nil)
	require.NoError(t, err)
	_, err = w.enqueueResponse(5, unaryResponseMeta{}, []byte("small"), nil)
	require.NoError(t, err)

	var order []string
	for f := w.next(); f != nil; f = w.next() {
		require.LessOrEqual(t, len(f), unaryMaxFrameBytes)
		d, err := decodeUnaryFrame(f)
		require.NoError(t, err)
		order = append(order, d.Type.String()+":"+strconv.Itoa(int(d.ID)))
	}
	assert.Equal(t, "RESPONSE:1 RESPONSE:3 RESPONSE:5", strings.Join(order[:3], " "), "whole frames go first")
	chunkIDs := order[3:]
	for i := 1; i < len(chunkIDs); i++ {
		assert.NotEqual(t, chunkIDs[i-1], chunkIDs[i], "chunks of two ids alternate: %v", chunkIDs)
	}
}

func TestUnaryWriter_should_DropQueuedChunks_When_Canceled(t *testing.T) {
	w := newUnaryWriter(nil)
	_, err := w.enqueueResponse(1, unaryResponseMeta{}, make([]byte, 2*unaryMaxFrameBytes), nil)
	require.NoError(t, err)
	w.drop(1)
	head := w.next()
	require.NotNil(t, head)
	assert.Nil(t, w.next(), "no CHUNK follows the head after drop")
}

func TestUnaryWriter_should_Refuse_When_ReadyQueueFull(t *testing.T) {
	w := newUnaryWriter(nil)
	for i := 0; i < unaryReadyLimit; i++ {
		require.NoError(t, w.enqueue(encodeUnaryControl(unaryFramePong, 0)))
	}
	assert.ErrorIs(t, w.enqueue(encodeUnaryControl(unaryFramePong, 0)), errUnaryWriterOverflow)
}

func TestUnaryWsBridge_should_AnswerOversizeFrame_And_StayOpen(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)

	sendRequest(t, c, 7, listFilesMethod, unaryRequestMeta{}, make([]byte, 300*1024))
	f := readFrame(t, c)
	assert.EqualValues(t, 7, f.ID)
	assert.Equal(t, codeResourceExhausted, f.Error.Code)

	sendListFiles(t, c, 9, "after")
	assert.Equal(t, "ok:after", decodeListFiles(t, readFrame(t, c)).BasePath)
}

func TestUnaryWsBridge_should_Close1009_When_FrameExceedsHardLimit(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)
	_ = c.WriteMessage(websocket.BinaryMessage, make([]byte, unaryReadLimitBytes+1024))
	require.NoError(t, c.SetReadDeadline(time.Now().Add(readWait)))
	_, _, err := c.ReadMessage()
	assert.True(t, websocket.IsCloseError(err, websocket.CloseMessageTooBig), "got %v", err)
}

func TestUnaryWsBridge_should_Close1003_When_FrameMalformedOrText(t *testing.T) {
	for name, send := range map[string]func(*websocket.Conn){
		"malformed": func(c *websocket.Conn) { _ = c.WriteMessage(websocket.BinaryMessage, []byte{1, 0}) },
		"unknown":   func(c *websocket.Conn) { _ = c.WriteMessage(websocket.BinaryMessage, []byte{9, 0, 0, 0, 1}) },
		"text":      func(c *websocket.Conn) { _ = c.WriteMessage(websocket.TextMessage, []byte("hi")) },
		"server-only type": func(c *websocket.Conn) {
			_ = c.WriteMessage(websocket.BinaryMessage, encodeUnaryChunk(1, true, nil))
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newBridgeHarness(t, ListenerLocal)
			c := h.connect(nil)
			send(c)
			require.NoError(t, c.SetReadDeadline(time.Now().Add(readWait)))
			_, _, err := c.ReadMessage()
			assert.True(t, websocket.IsCloseError(err, websocket.CloseUnsupportedData), "got %v", err)
		})
	}
}

func TestUnaryWsBridge_should_CloseSocket_When_PongDeadlineMissed(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal, WithUnaryKeepalive(5*time.Millisecond, 40*time.Millisecond))
	c := h.connect(nil)
	c.SetPingHandler(func(string) error { return nil }) // never answer pings
	require.NoError(t, c.SetReadDeadline(time.Now().Add(readWait)))
	_, _, err := c.ReadMessage()
	require.Error(t, err, "server closes a silent socket")
}

func TestUnaryWsBridge_should_AnswerPingWithPong(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	c := h.connect(nil)
	require.NoError(t, c.WriteMessage(websocket.BinaryMessage, encodeUnaryControl(unaryFramePing, 0)))
	assert.Equal(t, unaryFramePong, readFrame(t, c).Type)
}

func TestUnaryWsBridge_should_FailClosed_When_DispatcherUnset(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	h.bridge.dispatchers = func(string) http.Handler { return nil }
	c := h.connect(nil)
	sendListFiles(t, c, 1, "x")
	f := readFrame(t, c)
	assert.Equal(t, codeUnavailable, f.Error.Code)
	assert.Zero(t, h.stub.hits.Load())
}

func TestUnaryWsBridge_should_RefuseUpgrade_When_Query_Origin_or_PolicyMissing(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	u := "ws" + strings.TrimPrefix(h.srv.URL, "http") + UnaryWsPath

	_, resp, err := websocket.DefaultDialer.Dial(u+"?token=abc", nil)
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	for _, origin := range []string{"http://evil.example", "https://evil.example", "null"} {
		_, resp, err = websocket.DefaultDialer.Dial(u, http.Header{"Origin": {origin}})
		require.Error(t, err, origin)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, origin)
	}
	assert.EqualValues(t, 1, h.bridge.Metrics().UpgradeDenied(ListenerLocal, "query"))
	assert.EqualValues(t, 3, h.bridge.Metrics().UpgradeDenied(ListenerLocal, "origin"))

	remote := newBridgeHarness(t, ListenerRemote) // no remote policy installed: fail closed
	ru := "ws" + strings.TrimPrefix(remote.srv.URL, "http") + UnaryWsPath
	_, resp, err = websocket.DefaultDialer.Dial(ru, http.Header{"Origin": {"https://anything.example"}})
	require.Error(t, err)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	// remote harness stamps AuthModeRequired; the gate itself is covered over the real chain.
	remoteOK := newBridgeHarness(t, ListenerRemote, WithUnaryOriginPolicy(ListenerRemote, func(*http.Request) bool { return true }))
	remoteOK.connect(nil)
}

func TestUnaryWsBridge_should_Return426_When_NotAnUpgrade(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	resp, err := http.Get(h.srv.URL + UnaryWsPath)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUpgradeRequired, resp.StatusCode)
}

func TestWsUnaryAllowlist_should_MatchSharedFixture(t *testing.T) {
	var a unaryAllowlist
	loadFixture(t, "ws-unary-allowlist.json", &a)
	bare := make([]string, 0, len(wsUnaryAllowed))
	for _, m := range WsUnaryAllowedMethods() {
		bare = append(bare, m[strings.LastIndex(m, "/")+1:])
	}
	assert.ElementsMatch(t, a.WsUnaryAllowed, bare)
	assert.Equal(t, a.MaxInFlight, wsUnaryMaxInFlight)
	assert.Equal(t, a.ClientHeaders, wsUnaryClientHeaders)
}

func TestUnaryBridge_ShouldIncrementActuatorCounters_WhenCallsDeniedAndInflight(t *testing.T) {
	h := newBridgeHarness(t, ListenerLocal)
	m := h.bridge.Metrics()
	c := h.connect(nil)

	sendListFiles(t, c, 1, "counted")
	require.Equal(t, unaryFrameResponse, readFrame(t, c).Type)
	assert.EqualValues(t, 1, m.Calls(listFilesMethod, "ok"), "ws_unary_calls_total{method,code=ok}")

	sendRequest(t, c, 3, writeFileMethod, unaryRequestMeta{}, nil)
	require.Equal(t, unaryFrameError, readFrame(t, c).Type)
	assert.EqualValues(t, 1, m.Denied(wsUnaryMethodOther), "ws_unary_denied_total{method}")

	sendListFiles(t, c, 5, "gate:inflight")
	<-h.stub.started
	assert.EqualValues(t, 1, m.Inflight(), "ws_unary_inflight while a call executes")
	h.stub.release("gate:inflight")
	require.Equal(t, unaryFrameResponse, readFrame(t, c).Type)
	require.Eventually(t, func() bool { return m.Inflight() == 0 }, readWait, time.Millisecond)
	assert.EqualValues(t, 2, m.Calls(listFilesMethod, "ok"))

	u := "ws" + strings.TrimPrefix(h.srv.URL, "http") + UnaryWsPath
	_, _, err := websocket.DefaultDialer.Dial(u, http.Header{"Origin": {"https://evil.example"}})
	require.Error(t, err)
	assert.EqualValues(t, 1, m.UpgradeDenied(ListenerLocal, "origin"), "ws_unary_upgrade_denied_total{listener,reason}")

	// The same tallies, as the /actuator/ws-unary endpoint serves them.
	rep := h.bridge.Report()
	assert.Equal(t, []UnaryWsCallCount{
		{Method: listFilesMethod, Code: "ok", Count: 2},
		{Method: wsUnaryMethodOther, Code: "permission_denied", Count: 1},
	}, rep.CallsTotal)
	assert.Equal(t, []UnaryWsDeniedCount{{Method: wsUnaryMethodOther, Count: 1}}, rep.DeniedTotal)
	assert.Equal(t, []UnaryWsUpgradeDeniedCount{{Listener: ListenerLocal, Reason: "origin", Count: 1}}, rep.UpgradeDeniedTotal)
	assert.Zero(t, rep.Inflight)
}

func TestUnarySession_should_KeepIDReserved_UntilLastChunkWritten(t *testing.T) {
	s := &unarySession{inflight: map[uint32]*inflightCall{7: {cancel: func() {}}}, writer: newUnaryWriter(nil), b: &UnaryWsBridge{metrics: newUnaryMetrics()}}
	require.False(t, s.finishCall(7))
	streamed, canceled, err := s.enqueueUnlessCanceled(7, unaryResponseMeta{}, make([]byte, 2*unaryMaxFrameBytes), nil)
	require.NoError(t, err)
	require.True(t, streamed)
	require.False(t, canceled)

	has := func() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.inflight[7] != nil }
	for f := s.writer.next(); f != nil && has(); f = s.writer.next() {
		d, derr := decodeUnaryFrame(f)
		require.NoError(t, derr)
		if d.Type == unaryFrameChunk && d.Fin {
			break
		}
		assert.True(t, has(), "id stays reserved while chunks remain")
	}
	assert.False(t, has(), "id released once the last chunk is popped")
}

func TestUnarySession_should_SkipEnqueue_When_CancelArrivesAfterFinish(t *testing.T) {
	s := &unarySession{inflight: map[uint32]*inflightCall{9: {cancel: func() {}}}, writer: newUnaryWriter(nil), b: &UnaryWsBridge{metrics: newUnaryMetrics()}}
	s.finishCall(9)
	s.cancelCall(9) // lands between finishCall and enqueue
	streamed, canceled, err := s.enqueueUnlessCanceled(9, unaryResponseMeta{}, make([]byte, 2*unaryMaxFrameBytes), nil)
	require.NoError(t, err)
	assert.True(t, canceled)
	assert.False(t, streamed)
	assert.Nil(t, s.writer.next(), "cancelled response never reaches the writer")
}

func TestUnaryWriter_should_ReleaseStream_When_Dropped(t *testing.T) {
	w := newUnaryWriter(nil)
	released := 0
	streamed, err := w.enqueueResponse(1, unaryResponseMeta{}, make([]byte, 2*unaryMaxFrameBytes), func() { released++ })
	require.NoError(t, err)
	require.True(t, streamed)
	w.drop(1)
	assert.Equal(t, 1, released)
}
