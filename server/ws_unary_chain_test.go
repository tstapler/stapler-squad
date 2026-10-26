package server

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/services"
)

func TestUnaryDispatcher_ShouldBeNil_When_ListenerNotStarted(t *testing.T) {
	srv, _ := newServerBase("localhost:8543")
	t.Cleanup(srv.connCtxCancel)
	assert.Nil(t, srv.unaryDispatcher(services.ListenerLocal))
	assert.Nil(t, srv.unaryDispatcher(services.ListenerRemote))
}

func TestUnaryDispatcher_ShouldReturnPublishedHandlerPerListener(t *testing.T) {
	srv, _ := newServerBase("localhost:8543")
	t.Cleanup(srv.connCtxCancel)
	local := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	srv.setUnaryDispatcher(services.ListenerLocal, local)

	require.NotNil(t, srv.unaryDispatcher(services.ListenerLocal))
	assert.Nil(t, srv.unaryDispatcher(services.ListenerRemote), "remote stays unset")
	w := httptest.NewRecorder()
	srv.unaryDispatcher(services.ListenerLocal).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	assert.Equal(t, http.StatusTeapot, w.Code)
}

func TestLocalDispatcher_ShouldStampListenerAndApplyHostGuard(t *testing.T) {
	srv, _ := newServerBase("localhost:8543")
	t.Cleanup(srv.connCtxCancel)
	d := srv.localDispatcherWith(guardedProcedures)

	r := httptest.NewRequest(http.MethodGet, "/api/nothing", nil)
	r.Host = "evil.example"
	w := httptest.NewRecorder()
	d.ServeHTTP(w, r)
	assert.Equal(t, http.StatusForbidden, w.Code, "HostGuard rejects a rebinding Host inside the dispatcher")
}

// Chain parity (Task 5.2.1d) and the remote origin/auth gate (Task 5.2.1g): the
// unary bridge over the real localChain() and remoteChain(), with HTTP as the
// reference. The remote harness installs the production origin policy and trusts
// exactly its own TLS origin.

const (
	chainToken       = "chain-token"
	chainListFiles   = "/session.v1.SessionService/ListFiles"
	chainWriteFile   = "/session.v1.SessionService/WriteFile"
	chainReadTimeout = 10 * time.Second
)

type chainStub struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	listHits  atomic.Int32
	writeHits atomic.Int32
}

func (s *chainStub) ListFiles(_ context.Context, req *connect.Request[sessionv1.ListFilesRequest]) (*connect.Response[sessionv1.ListFilesResponse], error) {
	s.listHits.Add(1)
	return connect.NewResponse(&sessionv1.ListFilesResponse{BasePath: "base:" + req.Msg.SessionId, TotalCount: 7}), nil
}

func (s *chainStub) WriteFile(context.Context, *connect.Request[sessionv1.WriteFileRequest]) (*connect.Response[sessionv1.WriteFileResponse], error) {
	s.writeHits.Add(1)
	return connect.NewResponse(&sessionv1.WriteFileResponse{}), nil
}

// chainValidator accepts one token until expire() is called.
type chainValidator struct{ expired atomic.Bool }

func (v *chainValidator) ValidateAuthSession(tok string) bool {
	return tok == chainToken && !v.expired.Load()
}

type chainCase struct {
	name     string
	listener string
	auth     bool
}

var chainCases = []chainCase{
	{"local", services.ListenerLocal, false},
	{"local-auth", services.ListenerLocal, true},
	{"remote", services.ListenerRemote, true},
}

type chainHarness struct {
	chainCase
	stub      *chainStub
	validator *chainValidator
	bridge    *services.UnaryWsBridge
	ts        *httptest.Server
	dialer    *websocket.Dialer
}

func newChainHarness(t *testing.T, cc chainCase) *chainHarness {
	t.Helper()
	return newChainHarnessWithRemoteAuth(t, cc, true)
}

// newChainHarnessWithRemoteAuth lets a test start the remote chain with no auth
// middleware (the nil-authMW posture); local cases ignore remoteAuth.
func newChainHarnessWithRemoteAuth(t *testing.T, cc chainCase, remoteAuth bool) *chainHarness {
	t.Helper()
	srv, _ := newServerBase("localhost:8543")
	t.Cleanup(srv.connCtxCancel)
	stub := &chainStub{}
	path, h := sessionv1connect.NewSessionServiceHandler(stub, ConnectOptions(nil)...)
	srv.RegisterConnectHandler("/api"+path, http.StripPrefix("/api", h))

	v := &chainValidator{}
	bridge := services.NewUnaryWsBridge(srv.unaryDispatcher,
		services.WithUnaryOriginPolicy(services.ListenerRemote, srv.wsUnaryRemoteOriginPolicy()))
	srv.mux.Handle(services.UnaryWsPath, bridge)

	hn := &chainHarness{chainCase: cc, stub: stub, validator: v, bridge: bridge}
	if cc.listener == services.ListenerLocal {
		if cc.auth {
			srv.SetupAuth(middleware.Auth(v), true)
		}
		srv.setUnaryDispatcher(services.ListenerLocal, srv.localDispatcherWith(guardedProcedures))
		hn.ts = httptest.NewServer(srv.localChain())
		hn.dialer = websocket.DefaultDialer
	} else {
		var authMW func(http.Handler) http.Handler
		if remoteAuth {
			authMW = middleware.Auth(v)
		}
		srv.setUnaryDispatcher(services.ListenerRemote, srv.remoteDispatcher(authMW, remoteAuth))
		hn.ts = httptest.NewUnstartedServer(srv.remoteChain(authMW, remoteAuth))
		// Origins are set before the server starts: SetOrigins is not synchronized.
		srv.SetOrigins([]string{"https://" + hn.ts.Listener.Addr().String()})
		hn.ts.StartTLS()
		hn.dialer = &websocket.Dialer{TLSClientConfig: hn.ts.Client().Transport.(*http.Transport).TLSClientConfig}
	}
	t.Cleanup(hn.ts.Close)
	return hn
}

func (h *chainHarness) host() string {
	return strings.TrimPrefix(strings.TrimPrefix(h.ts.URL, "https://"), "http://")
}

func (h *chainHarness) scheme() string {
	if h.listener == services.ListenerRemote {
		return "https"
	}
	return "http"
}

func (h *chainHarness) header() http.Header {
	hd := http.Header{"Origin": {h.scheme() + "://" + h.host()}}
	if h.auth {
		hd.Set("Cookie", "cs_auth="+chainToken)
	}
	return hd
}

func (h *chainHarness) wsURL() string {
	return "ws" + strings.TrimPrefix(h.ts.URL, "http") + services.UnaryWsPath
}

func (h *chainHarness) dial(t *testing.T, hd http.Header) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	c, resp, err := h.dialer.Dial(h.wsURL(), hd)
	if c != nil {
		t.Cleanup(func() { _ = c.Close() })
	}
	return c, resp, err
}

func (h *chainHarness) send(t *testing.T, c *websocket.Conn, id uint32, method string, headers map[string]string, msg proto.Message) unaryFrameLite {
	t.Helper()
	body, err := proto.Marshal(msg)
	require.NoError(t, err)
	frame := encodeTestRequest(t, id, method, headers, body)
	require.NoError(t, c.WriteMessage(websocket.BinaryMessage, frame))
	require.NoError(t, c.SetReadDeadline(time.Now().Add(chainReadTimeout)))
	_, raw, err := c.ReadMessage()
	require.NoError(t, err)
	return decodeTestFrame(t, raw)
}

func (h *chainHarness) httpListFiles(t *testing.T, session string) (*sessionv1.ListFilesResponse, error) {
	t.Helper()
	client := sessionv1connect.NewSessionServiceClient(h.ts.Client(), h.ts.URL+"/api")
	req := connect.NewRequest(&sessionv1.ListFilesRequest{SessionId: session})
	if h.auth {
		req.Header().Set("Cookie", "cs_auth="+chainToken)
	}
	resp, err := client.ListFiles(context.Background(), req)
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func TestWsUnaryChain_should_ReturnSameDecodedMessageAsHTTP(t *testing.T) {
	for _, cc := range chainCases {
		t.Run(cc.name, func(t *testing.T) {
			h := newChainHarness(t, cc)
			c, _, err := h.dial(t, h.header())
			require.NoError(t, err)

			viaWS := h.send(t, c, 1, chainListFiles, nil, &sessionv1.ListFilesRequest{SessionId: "same"})
			require.Equal(t, "RESPONSE", viaWS.Kind, viaWS.ErrorCode)
			var wsMsg sessionv1.ListFilesResponse
			require.NoError(t, proto.Unmarshal(viaWS.Body, &wsMsg))

			httpMsg, err := h.httpListFiles(t, "same")
			require.NoError(t, err)
			assert.True(t, proto.Equal(httpMsg, &wsMsg), "ws=%v http=%v", &wsMsg, httpMsg)
		})
	}
}

func TestWsUnaryChain_should_DenyEveryGuardedProcedureOverWS(t *testing.T) {
	for _, cc := range chainCases {
		t.Run(cc.name, func(t *testing.T) {
			h := newChainHarness(t, cc)
			c, _, err := h.dial(t, h.header())
			require.NoError(t, err)
			id := uint32(1)
			for path := range guardedProcedures {
				got := h.send(t, c, id, strings.TrimPrefix(path, "/api"), nil, &sessionv1.WriteFileRequest{})
				id += 2
				assert.Equal(t, "ERROR", got.Kind, path)
				assert.Equal(t, "permission_denied", got.ErrorCode, path)
			}
			assert.Zero(t, h.stub.writeHits.Load())
		})
	}
}

func TestWsUnaryChain_should_NeverAllowAGuardedProcedure(t *testing.T) {
	allowed := map[string]bool{}
	for _, m := range services.WsUnaryAllowedMethods() {
		allowed["/api"+m] = true
	}
	for path := range guardedProcedures {
		assert.False(t, allowed[path], "%s is in guardedProcedures and must not be in wsUnaryAllowed", path)
	}
}

func TestWsUnaryChain_should_RejectMetaHeaderOverrides(t *testing.T) {
	for _, cc := range chainCases {
		t.Run(cc.name, func(t *testing.T) {
			h := newChainHarness(t, cc)
			c, _, err := h.dial(t, h.header())
			require.NoError(t, err)
			for i, key := range []string{"Origin", "Host", "X-Forwarded-For"} {
				got := h.send(t, c, uint32(2*i+1), chainListFiles, map[string]string{key: "http://evil.example"}, &sessionv1.ListFilesRequest{})
				assert.Equal(t, "invalid_argument", got.ErrorCode, key)
			}
			assert.Zero(t, h.stub.listHits.Load())
		})
	}
}

func TestWsUnaryChain_should_AnswerUnauthenticated_When_SessionExpiresMidSocket(t *testing.T) {
	for _, cc := range chainCases {
		if !cc.auth {
			continue
		}
		t.Run(cc.name, func(t *testing.T) {
			h := newChainHarness(t, cc)
			c, _, err := h.dial(t, h.header())
			require.NoError(t, err)
			require.Equal(t, "RESPONSE", h.send(t, c, 1, chainListFiles, nil, &sessionv1.ListFilesRequest{}).Kind)

			h.validator.expired.Store(true)
			before := h.stub.listHits.Load()
			got := h.send(t, c, 3, chainListFiles, nil, &sessionv1.ListFilesRequest{})
			assert.Equal(t, "unauthenticated", got.ErrorCode)
			assert.Equal(t, before, h.stub.listHits.Load(), "handler not invoked")

			require.NoError(t, c.WriteMessage(websocket.BinaryMessage, []byte{5, 0, 0, 0, 0}))
			require.NoError(t, c.SetReadDeadline(time.Now().Add(chainReadTimeout)))
			_, raw, err := c.ReadMessage()
			require.NoError(t, err, "socket stays open")
			assert.Equal(t, "PONG", decodeTestFrame(t, raw).Kind)
		})
	}
}

func TestWsUnaryChain_should_RefuseUpgrade_When_HostOrOriginOrQueryIsBad(t *testing.T) {
	h := newChainHarness(t, chainCases[0])

	rebound := h.header()
	d := *websocket.DefaultDialer
	_, resp, err := (&d).Dial(h.wsURL(), func() http.Header { rebound.Set("Host", "evil.example"); return rebound }())
	require.Error(t, err)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "foreign Host is rejected by HostGuard")

	for _, origin := range []string{"http://evil.example", "https://evil.example", "http://127.0.0.1:1"} {
		_, resp, err = h.dial(t, http.Header{"Origin": {origin}})
		require.Error(t, err, origin)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, origin)
	}

	_, resp, err = h.dialer.Dial(h.wsURL()+"?x=1", h.header())
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// unaryFrameLite is the slice of the ADR-001 wire format these tests inspect;
// the codec itself is unexported in server/services and tested there.
type unaryFrameLite struct {
	Kind      string // RESPONSE, ERROR, PONG, ...
	Body      []byte // RESPONSE first slice
	ErrorCode string
}

func encodeTestRequest(t *testing.T, id uint32, method string, headers map[string]string, body []byte) []byte {
	t.Helper()
	if headers == nil {
		headers = map[string]string{}
	}
	meta, err := json.Marshal(map[string]any{"headers": headers, "timeoutMs": 0})
	require.NoError(t, err)
	b := []byte{1}
	b = binary.BigEndian.AppendUint32(b, id)
	b = binary.BigEndian.AppendUint16(b, uint16(len(method)))
	b = append(b, method...)
	b = binary.BigEndian.AppendUint16(b, uint16(len(meta)))
	b = append(b, meta...)
	return append(b, body...)
}

func decodeTestFrame(t *testing.T, raw []byte) unaryFrameLite {
	t.Helper()
	require.GreaterOrEqual(t, len(raw), 5)
	body := raw[5:]
	switch raw[0] {
	case 2:
		require.GreaterOrEqual(t, len(body), 7)
		metaLen := int(binary.BigEndian.Uint16(body[5:7]))
		return unaryFrameLite{Kind: "RESPONSE", Body: body[7+metaLen:]}
	case 3:
		var e struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.Unmarshal(body, &e))
		return unaryFrameLite{Kind: "ERROR", ErrorCode: e.Code}
	case 6:
		return unaryFrameLite{Kind: "PONG"}
	}
	return unaryFrameLite{Kind: "OTHER"}
}

// CSWSH and auth-gate rows over the real remoteChain (Task 5.2.1g).

var remoteChainCase = chainCase{"remote", services.ListenerRemote, true}

func (h *chainHarness) expectDeniedUpgrade(t *testing.T, hd http.Header, wantStatus int, why string) {
	t.Helper()
	c, resp, err := h.dial(t, hd)
	require.Error(t, err, why)
	require.Nil(t, c, why)
	require.NotNil(t, resp, why)
	assert.Equal(t, wantStatus, resp.StatusCode, why)
	assert.Zero(t, h.stub.listHits.Load(), "%s: handler never invoked", why)
}

func TestUnaryChain_ShouldReturn403AtUpgradeAndSkipHandler_WhenRemoteCookieValidButOriginEvil(t *testing.T) {
	h := newChainHarness(t, remoteChainCase)
	hd := h.header()
	hd.Set("Origin", "https://evil.example")

	h.expectDeniedUpgrade(t, hd, http.StatusForbidden, "evil origin")
	assert.EqualValues(t, 1, h.bridge.Metrics().UpgradeDenied(services.ListenerRemote, "origin"))

	// Control: the same credentials with the allowed origin upgrade.
	c, _, err := h.dial(t, h.header())
	require.NoError(t, err)
	assert.Equal(t, "RESPONSE", h.send(t, c, 1, chainListFiles, nil, &sessionv1.ListFilesRequest{}).Kind)
}

func TestUnaryChain_ShouldReturn401_WhenRemoteUpgradeHasNoCookie(t *testing.T) {
	h := newChainHarness(t, remoteChainCase)
	hd := http.Header{"Origin": {"https://" + h.host()}}

	h.expectDeniedUpgrade(t, hd, http.StatusUnauthorized, "no cookie")
	assert.Zero(t, h.bridge.Metrics().UpgradeDenied(services.ListenerRemote, "origin"), "bridge never ran")
}

func TestUnaryChain_ShouldReturn403_WhenRemoteOriginSameSiteOtherPortPlaintextNullOrHostnameNotVerified(t *testing.T) {
	h := newChainHarness(t, remoteChainCase)
	hostname, _, err := net.SplitHostPort(h.host())
	require.NoError(t, err)

	withOrigin := func(origin string) http.Header {
		hd := h.header()
		hd.Set("Origin", origin)
		return hd
	}
	cases := map[string]http.Header{
		"same site, other port": withOrigin("https://" + hostname + ":1"),
		"plaintext origin":      withOrigin("http://" + h.host()),
		"null origin":           withOrigin("null"),
		"missing origin":        {"Cookie": h.header()["Cookie"]},
		"rebinding shape":       {"Origin": {"https://evil.example"}, "Host": {"evil.example"}, "Cookie": h.header()["Cookie"]},
	}
	for name, hd := range cases {
		h.expectDeniedUpgrade(t, hd, http.StatusForbidden, name)
	}
}

func TestUnaryChain_ShouldUpgrade_WhenRemoteMissingOriginPresentsBearerOnly(t *testing.T) {
	h := newChainHarness(t, remoteChainCase)
	c, _, err := h.dial(t, http.Header{"Authorization": {"Bearer " + chainToken}})
	require.NoError(t, err)
	assert.Equal(t, "RESPONSE", h.send(t, c, 1, chainListFiles, nil, &sessionv1.ListFilesRequest{}).Kind)
}

func TestUnaryChain_ShouldRefuseQueryToken_WhenRemote(t *testing.T) {
	h := newChainHarness(t, remoteChainCase)

	_, resp, err := h.dialer.Dial(h.wsURL()+"?token="+chainToken, http.Header{"Origin": {"https://" + h.host()}})
	require.Error(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode, "a URL token never authenticates")

	_, resp, err = h.dialer.Dial(h.wsURL()+"?x=1", h.header())
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "any query is refused")
}

func TestUnaryChain_ShouldReturn403AuthRequired_WhenRemoteChainHasNoAuthMiddleware(t *testing.T) {
	h := newChainHarnessWithRemoteAuth(t, remoteChainCase, false)
	hd := http.Header{"Origin": {"https://" + h.host()}}

	h.expectDeniedUpgrade(t, hd, http.StatusForbidden, "open remote chain")
	assert.EqualValues(t, 1, h.bridge.Metrics().UpgradeDenied(services.ListenerRemote, "auth_required"))
}

func TestUnaryChain_ShouldReturn401_WhenRevokedCookieOpensSecondRemoteSocket(t *testing.T) {
	h := newChainHarness(t, remoteChainCase)
	_, _, err := h.dial(t, h.header())
	require.NoError(t, err)

	h.validator.expired.Store(true)
	h.expectDeniedUpgrade(t, h.header(), http.StatusUnauthorized, "revoked session")
}

func TestUnaryChain_ShouldReturn101_WhenUpgradeOnRealRemoteChainWithValidCookieAndAllowedOrigin(t *testing.T) {
	h := newChainHarness(t, remoteChainCase)
	c, resp, err := h.dial(t, h.header())
	require.NoError(t, err)
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	viaWS := h.send(t, c, 1, chainListFiles, nil, &sessionv1.ListFilesRequest{SessionId: "remote-ok"})
	require.Equal(t, "RESPONSE", viaWS.Kind, viaWS.ErrorCode)
	var wsMsg sessionv1.ListFilesResponse
	require.NoError(t, proto.Unmarshal(viaWS.Body, &wsMsg))
	httpMsg, err := h.httpListFiles(t, "remote-ok")
	require.NoError(t, err)
	assert.True(t, proto.Equal(httpMsg, &wsMsg), "ws=%v http=%v", &wsMsg, httpMsg)
}
