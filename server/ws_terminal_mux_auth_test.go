package server

import (
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/server/services/wsconn"
	"github.com/tstapler/stapler-squad/session"
)

// Mux frame types and layout (docs/reference/terminal-ws-pool.md): type u8 | id u32 | body.
const (
	muxTOpen  byte = 2
	muxTData  byte = 3
	muxTClose byte = 5
	muxTHello byte = 1
)

// tokenValidator is a revocable set of sessions: two principals, each with its own cookie.
type tokenValidator struct{ live sync.Map }

func (v *tokenValidator) ValidateAuthSession(tok string) bool { _, ok := v.live.Load(tok); return ok }

func muxTestFrame(typ byte, id uint32, body ...byte) []byte {
	f := append([]byte{typ, 0, 0, 0, 0}, body...)
	binary.BigEndian.PutUint32(f[1:5], id)
	return f
}

type muxAuthHarness struct {
	ts     *httptest.Server
	dialer *websocket.Dialer
	v      *tokenValidator
}

func muxAuthOptions(srv *Server) []wsconn.TerminalMuxOption {
	return []wsconn.TerminalMuxOption{
		wsconn.WithTerminalMuxOriginPolicy(services.ListenerRemote, srv.wsUnaryRemoteOriginPolicy()),
		wsconn.WithTerminalMuxDispatchers(srv.unaryDispatcher),
	}
}

func newMuxAuthHarness(t *testing.T) *muxAuthHarness {
	t.Helper()
	srv, _ := newServerBase("localhost:8543")
	t.Cleanup(srv.connCtxCancel)
	h := &muxAuthHarness{v: &tokenValidator{}}
	storage, err := session.NewStorageWithRepository(session.NewTestEntRepository(t))
	require.NoError(t, err)
	bus := events.NewEventBus(16)
	t.Cleanup(bus.Close)
	svc := services.NewSessionService(storage, bus)
	t.Cleanup(svc.Shutdown)
	// The real StreamTerminal handler: a stream it serves answers the Connect handshake.
	wsHandler := wsconn.NewConnectRPCWebSocketHandler(svc, nil, nil)
	bridge := wsconn.NewTerminalMuxBridgeForHandler(wsHandler, 2, muxAuthOptions(srv)...)
	srv.mux.Handle(wsconn.TerminalMuxPath, bridge)
	authMW := middleware.Auth(h.v)
	srv.setUnaryDispatcher(services.ListenerRemote, srv.remoteDispatcher(authMW, true))
	h.ts = httptest.NewUnstartedServer(srv.remoteChain(authMW, true))
	srv.SetOrigins([]string{"https://" + h.ts.Listener.Addr().String()})
	h.ts.StartTLS()
	t.Cleanup(h.ts.Close)
	h.dialer = &websocket.Dialer{TLSClientConfig: h.ts.Client().Transport.(*http.Transport).TLSClientConfig}
	return h
}

// connect dials the mux as the principal holding token and waits for HELLO.
func (h *muxAuthHarness) connect(t *testing.T, token string) *websocket.Conn {
	t.Helper()
	hd := http.Header{
		"Origin": {"https://" + h.ts.Listener.Addr().String()},
		"Cookie": {"cs_auth=" + token},
	}
	c, _, err := h.dialer.Dial("wss"+strings.TrimPrefix(h.ts.URL, "https")+wsconn.TerminalMuxPath, hd)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	f := readMuxFrame(t, c)
	require.Equal(t, muxTHello, f[0])
	return c
}

func readMuxFrame(t *testing.T, c *websocket.Conn) []byte {
	t.Helper()
	require.NoError(t, c.SetReadDeadline(time.Now().Add(10*time.Second)))
	_, msg, err := c.ReadMessage()
	require.NoError(t, err)
	return msg
}

// openStream opens stream id with the Connect header and request envelope for an unknown
// session, then waits for the handler's handshake reply ("Status-Code: 200" text DATA): proof
// the real StreamTerminal handler ran for this stream.
func openStream(t *testing.T, c *websocket.Conn, id uint32) {
	t.Helper()
	req, err := proto.Marshal(&sessionv1.TerminalData{SessionId: "no-such-session"})
	require.NoError(t, err)
	env := append([]byte{0, 0, 0, 0, 0}, req...)
	binary.BigEndian.PutUint32(env[1:5], uint32(len(req)))
	hdr := "Content-Type: application/connect+proto\r\n\r\n"
	for _, m := range [][]byte{
		muxTestFrame(muxTOpen, id),
		muxTestFrame(muxTData, id, append([]byte{1, 1}, hdr...)...),
		muxTestFrame(muxTData, id, append([]byte{2, 1}, env...)...),
	} {
		require.NoError(t, c.WriteMessage(websocket.BinaryMessage, m))
	}
	for {
		f := readMuxFrame(t, c)
		if binary.BigEndian.Uint32(f[1:5]) != id {
			continue // an earlier stream's trailing frames
		}
		if f[0] == muxTData && f[5] == 1 && strings.HasPrefix(string(f[7:]), "Status-Code: 200") {
			return
		}
		require.NotEqual(t, muxTClose, f[0], "stream closed before the handshake: %v", f)
	}
}

// A pooled socket is admitted once, but each plain StreamTerminal upgrade re-runs the auth
// chain; a revoked principal must not keep opening streams on its earlier-admitted socket,
// while another principal's socket is unaffected.
func TestTerminalMuxOpen_should_DenyOpen_When_SessionRevokedAfterConnect(t *testing.T) {
	h := newMuxAuthHarness(t)
	h.v.live.Store("alice", true)
	h.v.live.Store("bob", true)
	alice, bob := h.connect(t, "alice"), h.connect(t, "bob")
	openStream(t, alice, 1)
	openStream(t, bob, 1)

	h.v.live.Delete("alice") // logout / expiry

	require.NoError(t, alice.WriteMessage(websocket.BinaryMessage, muxTestFrame(muxTOpen, 3)))
	f := readMuxFrame(t, alice)
	for binary.BigEndian.Uint32(f[1:5]) != 3 { // skip stream 1's trailing frames
		f = readMuxFrame(t, alice)
	}
	require.Equal(t, muxTClose, f[0], "revoked principal's OPEN must be refused with CLOSE, not served")
	assert.EqualValues(t, 3, binary.BigEndian.Uint32(f[1:5]))
	assert.EqualValues(t, websocket.ClosePolicyViolation, binary.BigEndian.Uint16(f[5:7]))
	assert.Equal(t, "unauthorized", string(f[7:]), "fixed text: no session or credential detail")

	openStream(t, bob, 3) // other principal still works
}

func TestTerminalMuxOpen_should_CloseSocket_When_OpenKeepsBeingDenied(t *testing.T) {
	h := newMuxAuthHarness(t)
	h.v.live.Store("alice", true)
	alice := h.connect(t, "alice")
	h.v.live.Delete("alice")
	for id := uint32(1); id <= 3; id++ {
		require.NoError(t, alice.WriteMessage(websocket.BinaryMessage, muxTestFrame(muxTOpen, id)))
	}
	require.NoError(t, alice.SetReadDeadline(time.Now().Add(10*time.Second)))
	for {
		_, _, err := alice.ReadMessage()
		if err != nil {
			assert.True(t, websocket.IsCloseError(err, websocket.ClosePolicyViolation), "got %v", err)
			break
		}
	}
}
