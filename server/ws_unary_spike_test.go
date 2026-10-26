package server

// Throwaway spike for Task 5.1.1a (project_plans/frontend-responsiveness,
// research/ws-unary-seams.md). Task 5.2.1d promotes this into
// ws_unary_chain_test.go or deletes it in the same PR.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/services"
)

const (
	spikeVCSPath   = "/api" + sessionv1connect.SessionServiceGetVCSStatusProcedure
	spikeProbePath = "/api" + sessionv1connect.SessionServiceProbeProgramProcedure
	spikeToken     = "spike-token"
)

type spikeVCSStub struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	hits        atomic.Int32
	gotListener atomic.Value
}

func (s *spikeVCSStub) GetVCSStatus(ctx context.Context, req *connect.Request[sessionv1.GetVCSStatusRequest]) (*connect.Response[sessionv1.GetVCSStatusResponse], error) {
	s.hits.Add(1)
	if rec, ok := services.RequestRecordFrom(ctx); ok {
		s.gotListener.Store(rec.Listener)
	}
	return connect.NewResponse(&sessionv1.GetVCSStatusResponse{Error: "id=" + req.Msg.Id}), nil
}

func newSpikeServer(t *testing.T) (*Server, *spikeVCSStub) {
	t.Helper()
	srv, _ := newServerBase("localhost:8543")
	t.Cleanup(srv.connCtxCancel)
	stub := &spikeVCSStub{}
	path, h := sessionv1connect.NewSessionServiceHandler(stub, ConnectOptions(nil)...)
	srv.RegisterConnectHandler("/api"+path, http.StripPrefix("/api", h))
	return srv, stub
}

// spikeLocalDispatcher is localChainWith minus Logging/CORS/Compress: the
// composition the bridge re-enters (stamp -> HostGuard -> LocalWriteGuard -> mux).
func spikeLocalDispatcher(s *Server) http.Handler {
	inner := middleware.LocalWriteGuard(guardedProcedures, middleware.LocalWriteGuardConfig{
		Probe:     s.probeGuardConfig(),
		Rebinding: s.LocalWriteVerdictConfig(),
	})(http.Handler(s))
	inner = middleware.HostGuard(s.hostGuardConfig())(inner)
	return s.stampRequest(services.ListenerLocal, s.requiresAuth)(inner)
}

// spikeRemoteDispatcher is remoteChain minus Logging/CORS/Compress.
func spikeRemoteDispatcher(s *Server, authMW func(http.Handler) http.Handler) http.Handler {
	return s.stampRequest(services.ListenerRemote, true)(authMW(http.Handler(s)))
}

// upgradeRequest is what the bridge sees at handshake time; synthesize copies
// only these fields into each per-call request.
type upgradeRequest struct {
	host, origin, remoteAddr, cookie string
}

func (u upgradeRequest) synthesize(t *testing.T, path string, body []byte) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
	r.Host = u.host
	r.RemoteAddr = u.remoteAddr
	r.Header.Set("Content-Type", "application/proto")
	r.Header.Set("Accept-Encoding", "identity")
	r.Header.Set("Connect-Protocol-Version", "1")
	if u.origin != "" {
		r.Header.Set("Origin", u.origin)
	}
	if u.cookie != "" {
		r.AddCookie(&http.Cookie{Name: "cs_auth", Value: u.cookie})
	}
	return r
}

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func spikeBody(t *testing.T) []byte {
	t.Helper()
	b, err := proto.Marshal(&sessionv1.GetVCSStatusRequest{Id: "s1"})
	require.NoError(t, err)
	return b
}

func TestSpike_LocalDispatcher_ShouldReachRealUnaryHandler_AndMatchHTTPGuardVerdicts(t *testing.T) {
	srv, stub := newSpikeServer(t)
	body := spikeBody(t)
	up := upgradeRequest{host: "localhost:8543", origin: "http://localhost:8543", remoteAddr: "127.0.0.1:50000"}

	for name, h := range map[string]http.Handler{
		"split dispatcher":  spikeLocalDispatcher(srv),
		"full localChain()": srv.localChain(),
	} {
		t.Run(name+"/read reaches the handler with the local listener stamp", func(t *testing.T) {
			before := stub.hits.Load()
			w := serve(h, up.synthesize(t, spikeVCSPath, body))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Equal(t, before+1, stub.hits.Load())
			assert.Equal(t, services.ListenerLocal, stub.gotListener.Load())
			var resp sessionv1.GetVCSStatusResponse
			require.NoError(t, proto.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, "id=s1", resp.Error)
			assert.Empty(t, w.Header().Values("Set-Cookie"))
		})

		t.Run(name+"/guarded procedure verdict equals the HTTP verdict", func(t *testing.T) {
			foreign := upgradeRequest{host: "localhost:8543", origin: "http://evil.example", remoteAddr: "127.0.0.1:50000"}
			rebound := upgradeRequest{host: "evil.example:8543", remoteAddr: "127.0.0.1:50000"}
			for label, u := range map[string]upgradeRequest{"foreign origin": foreign, "rebinding host": rebound, "same origin": up} {
				viaWS := serve(h, u.synthesize(t, spikeProbePath, body)).Code
				ts := httptest.NewServer(srv.localChain())
				req := u.synthesize(t, spikeProbePath, body)
				req.RequestURI = ""
				req.URL, _ = req.URL.Parse(ts.URL + spikeProbePath)
				req.Host = u.host
				resp, err := http.DefaultClient.Do(req)
				ts.Close()
				require.NoError(t, err)
				_ = resp.Body.Close()
				assert.Equal(t, resp.StatusCode, viaWS, "%s: WS-synthesized vs real HTTP", label)
				t.Logf("%s/%s ProbeProgram: status %d (HTTP %d)", name, label, viaWS, resp.StatusCode)
			}
			assert.GreaterOrEqual(t, serve(h, foreign.synthesize(t, spikeProbePath, body)).Code, 400)
		})
	}
}

// spikeValidator expires tokens on demand (fake clock stand-in for the fixed TTL).
type spikeValidator struct{ expired atomic.Bool }

func (v *spikeValidator) ValidateAuthSession(tok string) bool {
	return tok == spikeToken && !v.expired.Load()
}

func TestSpike_RemoteDispatcher_ShouldAuthenticatePerRequest_AndNeverRefreshCookie(t *testing.T) {
	srv, stub := newSpikeServer(t)
	v := &spikeValidator{}
	authMW := middleware.Auth(v)
	body := spikeBody(t)
	up := upgradeRequest{host: "box.tail.ts.net:8444", origin: "https://box.tail.ts.net:8444", remoteAddr: "100.64.0.5:41000", cookie: spikeToken}

	for name, h := range map[string]http.Handler{
		"split dispatcher":   spikeRemoteDispatcher(srv, authMW),
		"full remoteChain()": srv.remoteChain(authMW, true),
	} {
		v.expired.Store(false)
		t.Run(name, func(t *testing.T) {
			w := serve(h, up.synthesize(t, spikeVCSPath, body))
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Equal(t, services.ListenerRemote, stub.gotListener.Load())
			assert.Empty(t, w.Header().Values("Set-Cookie"), "auth middleware never refreshes the cookie")

			anon := up
			anon.cookie = ""
			before := stub.hits.Load()
			w = serve(h, anon.synthesize(t, spikeVCSPath, body))
			assert.Equal(t, http.StatusUnauthorized, w.Code, "unauthenticated is answered before the handler")
			assert.Equal(t, before, stub.hits.Load())

			v.expired.Store(true)
			w = serve(h, up.synthesize(t, spikeVCSPath, body))
			assert.Equal(t, http.StatusUnauthorized, w.Code, "request past the TTL re-runs auth")
		})
	}
}

func TestSpike_UnauthenticatedUpgrade_ShouldBeAnswered401BeforeBridge_AndKeepHijacker(t *testing.T) {
	srv, _ := newSpikeServer(t)
	var bridgeRan atomic.Int32
	var hijackable atomic.Bool
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv.mux.HandleFunc("/api/ws/unary", func(w http.ResponseWriter, r *http.Request) {
		bridgeRan.Add(1)
		_, ok := w.(http.Hijacker)
		hijackable.Store(ok)
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.WriteMessage(websocket.BinaryMessage, []byte("ok"))
	})
	v := &spikeValidator{}
	// StartRemote wraps otelhttp around remoteChain; mirror it.
	remote := otelhttp.NewHandler(srv.remoteChain(middleware.Auth(v), true), "spike")
	ts := httptest.NewServer(remote)
	defer ts.Close()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws/unary"

	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	assert.EqualValues(t, 0, bridgeRan.Load(), "bridge never ran for an unauthenticated upgrade")

	hdr := http.Header{"Cookie": {"cs_auth=" + spikeToken}}
	c, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	require.NoError(t, err)
	defer c.Close()
	_, msg, err := c.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, "ok", string(msg))
	assert.True(t, hijackable.Load(), "otelhttp -> Logging -> CORS -> Compress -> Auth preserve http.Hijacker")
}

func TestSpike_UnarySeams_ShouldRecordOriginsAndHostnames(t *testing.T) {
	srv, _ := newSpikeServer(t)
	srv.SetHostnames([]string{"box.tail.ts.net"})
	t.Logf("GetOrigins() on a fresh server (before main.go:1626 SetOrigins) = %v", srv.GetOrigins())
	t.Logf("GetHostnames() after SetHostnames = %v", srv.GetHostnames())
	assert.Equal(t, []string{"box.tail.ts.net"}, srv.GetHostnames())
}
