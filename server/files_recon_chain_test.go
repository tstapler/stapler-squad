package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/middleware"
)

// Recon for Task 0.1.3b (ADR-031 / ADR-036 "Spike outcomes"): what the real
// localChain/remoteChain and Connect handler options do to a write RPC and to a
// file-bytes GET. The outcomes recorded in the ADRs are these assertions.

const (
	reconRawFilesPath = "/api/files/raw"
	reconGoodToken    = "good"
	patchSizedBody    = 2 << 20 // ADR-036 patch cap
	wholeFileCapPlus1 = 1536*1024 + 1
)

type reconValidator struct{}

func (reconValidator) ValidateAuthSession(token string) bool { return token == reconGoodToken }

type reconWriteStub struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	gotBytes atomic.Int64
}

func (s *reconWriteStub) WriteToSession(_ context.Context, req *connect.Request[sessionv1.WriteToSessionRequest]) (*connect.Response[sessionv1.WriteToSessionResponse], error) {
	s.gotBytes.Store(int64(len(req.Msg.Input)))
	return connect.NewResponse(&sessionv1.WriteToSessionResponse{Success: true}), nil
}

// newReconServer mounts the real Connect handler (real ConnectOptions, no
// WithReadMaxBytes, exactly as server.go does) and a stand-in for ServeFileRaw.
func newReconServer(t *testing.T) (*Server, *reconWriteStub, *int) {
	t.Helper()
	srv, _ := newServerBase("localhost:8543")
	t.Cleanup(srv.connCtxCancel)
	stub := &reconWriteStub{}
	path, h := sessionv1connect.NewSessionServiceHandler(stub, ConnectOptions(nil)...)
	srv.RegisterConnectHandler("/api"+path, http.StripPrefix("/api", h))
	rawHits := new(int)
	srv.mux.HandleFunc(reconRawFilesPath, func(w http.ResponseWriter, _ *http.Request) {
		*rawHits++
		_, _ = w.Write([]byte("file-bytes"))
	})
	return srv, stub, rawHits
}

func writeCall(t *testing.T, baseURL string, token string, bytes int) error {
	t.Helper()
	opts := []connect.ClientOption{}
	if token != "" {
		opts = append(opts, connect.WithInterceptors(connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Authorization", "Bearer "+token)
				return next(ctx, req)
			}
		})))
	}
	c := sessionv1connect.NewSessionServiceClient(http.DefaultClient, baseURL+"/api", opts...)
	_, err := c.WriteToSession(context.Background(), connect.NewRequest(&sessionv1.WriteToSessionRequest{
		SessionId: "s", Input: strings.Repeat("a", bytes),
	}))
	return err
}

func TestReconRemoteTier_ShouldHaveNoReadScopedTier_And_AcceptPatchSizedWriteBody(t *testing.T) {
	srv, stub, _ := newReconServer(t)
	ts := httptest.NewServer(srv.remoteChain(middleware.Auth(reconValidator{}), true))
	defer ts.Close()

	t.Run("unauthenticated remote caller is refused", func(t *testing.T) {
		err := writeCall(t, ts.URL, "", 10)
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})
	t.Run("any valid session reaches a write RPC with a patch-sized body", func(t *testing.T) {
		require.NoError(t, writeCall(t, ts.URL, reconGoodToken, patchSizedBody))
		assert.EqualValues(t, patchSizedBody, stub.gotBytes.Load())
	})
	t.Run("middleware.Auth only knows authenticated or not (no scope parameter)", func(t *testing.T) {
		// The only inputs are a cookie or bearer token validated by AuthValidator.ValidateAuthSession(string) bool.
		var v middleware.AuthValidator = reconValidator{}
		assert.True(t, v.ValidateAuthSession(reconGoodToken))
	})
}

func TestReconConnectLimits_ShouldAcceptBodiesAboveWholeFileCap_When_NoReadMaxBytes(t *testing.T) {
	srv, stub, _ := newReconServer(t)
	ts := httptest.NewServer(srv.localChain())
	defer ts.Close()

	require.NoError(t, writeCall(t, ts.URL, "", wholeFileCapPlus1))
	assert.EqualValues(t, wholeFileCapPlus1, stub.gotBytes.Load())
	require.NoError(t, writeCall(t, ts.URL, "", patchSizedBody))
	assert.EqualValues(t, patchSizedBody, stub.gotBytes.Load())
}

func TestReconLocalChain_ShouldKeepWriteRPCsOutOfGuardedProcedures(t *testing.T) {
	for _, p := range []string{
		"/api" + sessionv1connect.SessionServiceWriteToSessionProcedure,
		reconRawFilesPath,
	} {
		_, guarded := guardedProcedures[p]
		assert.False(t, guarded, "%s is not in guardedProcedures", p)
	}
}

func rawGet(h http.Handler, host, origin string) int {
	r := httptest.NewRequest(http.MethodGet, reconRawFilesPath+"?sessionId=s&path=a", nil)
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestReconLocalChain_ShouldRejectForeignOriginAndRebindingHost_For_FileBytesGET(t *testing.T) {
	srv, _, hits := newReconServer(t)
	chain := srv.localChain()

	assert.Equal(t, http.StatusOK, rawGet(chain, "localhost:8543", ""), "no Origin (img/nav) is served")
	assert.Equal(t, http.StatusOK, rawGet(chain, "localhost:8543", "http://localhost:8543"), "same-origin")
	before := *hits
	assert.Equal(t, http.StatusForbidden, rawGet(chain, "localhost:8543", "https://evil.example"), "foreign Origin")
	assert.Equal(t, http.StatusForbidden, rawGet(chain, "evil.example:8543", ""), "rebinding Host")
	assert.Equal(t, http.StatusForbidden, rawGet(chain, "localhost:8543", "http://localhost:9999"), "other loopback port")
	assert.Equal(t, before, *hits, "handler never reached on a rejected request")
}

func TestReconLocalChain_ShouldRejectForeignOriginAndRebindingHost_For_WriteRPC(t *testing.T) {
	srv, stub, _ := newReconServer(t)
	chain := srv.localChain()
	post := func(host, origin string) int {
		r := httptest.NewRequest(http.MethodPost, "/api"+sessionv1connect.SessionServiceWriteToSessionProcedure, strings.NewReader("{}"))
		r.Host = host
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		chain.ServeHTTP(w, r)
		_, _ = io.Copy(io.Discard, w.Body)
		return w.Code
	}
	assert.Equal(t, http.StatusForbidden, post("evil.example:8543", ""))
	assert.Equal(t, http.StatusForbidden, post("localhost:8543", "https://evil.example"))
	assert.Zero(t, stub.gotBytes.Load())
}

func TestReconRemoteChain_ShouldRequireAuthForFileBytesGET(t *testing.T) {
	srv, _, _ := newReconServer(t)
	chain := srv.remoteChain(middleware.Auth(reconValidator{}), true)
	assert.Equal(t, http.StatusUnauthorized, rawGet(chain, "onyx.staplerhome.internal:8444", ""), "unauthenticated /api/ GET is a 401")
}
