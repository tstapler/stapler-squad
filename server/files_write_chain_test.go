package server

import (
	"context"
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

// writeFileStub counts the bytes WriteFile and WriteToSession handlers receive.
type writeFileStub struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	writeBytes   atomic.Int64
	writeCalls   atomic.Int32
	sessionBytes atomic.Int64
}

func (s *writeFileStub) WriteFile(_ context.Context, req *connect.Request[sessionv1.WriteFileRequest]) (*connect.Response[sessionv1.WriteFileResponse], error) {
	s.writeCalls.Add(1)
	s.writeBytes.Store(int64(len(req.Msg.Content)))
	return connect.NewResponse(&sessionv1.WriteFileResponse{Outcome: &sessionv1.WriteFileResponse_Saved{
		Saved: &sessionv1.WriteFileSaved{Token: "t"},
	}}), nil
}

func (s *writeFileStub) WriteToSession(_ context.Context, req *connect.Request[sessionv1.WriteToSessionRequest]) (*connect.Response[sessionv1.WriteToSessionResponse], error) {
	s.sessionBytes.Store(int64(len(req.Msg.Input)))
	return connect.NewResponse(&sessionv1.WriteToSessionResponse{Success: true}), nil
}

// newWriteFileChainServer mounts the real Connect handler with the same wrapper
// server.go applies (withFileBodyLimits), so both chains share one bounded handler.
func newWriteFileChainServer(t *testing.T) (*Server, *writeFileStub) {
	t.Helper()
	srv, _ := newServerBase("localhost:8543")
	t.Cleanup(srv.connCtxCancel)
	stub := &writeFileStub{}
	path, h := sessionv1connect.NewSessionServiceHandler(stub, ConnectOptions(nil)...)
	h = withFileBodyLimits(h)
	srv.RegisterConnectHandler("/api"+path, http.StripPrefix("/api", h))
	return srv, stub
}

// jsonClient speaks the browser's encoding: content is base64 inside JSON.
func jsonClient(baseURL, token string) sessionv1connect.SessionServiceClient {
	opts := []connect.ClientOption{connect.WithProtoJSON()}
	if token != "" {
		opts = append(opts, connect.WithInterceptors(connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Authorization", "Bearer "+token)
				return next(ctx, req)
			}
		})))
	}
	return sessionv1connect.NewSessionServiceClient(http.DefaultClient, baseURL+"/api", opts...)
}

func writeFileOf(c sessionv1connect.SessionServiceClient, contentBytes int) error {
	_, err := c.WriteFile(context.Background(), connect.NewRequest(&sessionv1.WriteFileRequest{
		SessionId: "s", Path: "a.go", Content: []byte(strings.Repeat("a", contentBytes)), ExpectedToken: "t",
	}))
	return err
}

func TestWriteFile_ShouldRefuseOversizeBodyWithResourceExhausted_On_LocalAndRemoteChains(t *testing.T) {
	srv, stub := newWriteFileChainServer(t)
	admit := func(next http.Handler) http.Handler { return next }
	for name, chain := range map[string]http.Handler{"local": srv.localChain(), "remote": srv.remoteChain(admit, false)} {
		t.Run(name, func(t *testing.T) {
			ts := httptest.NewServer(chain)
			defer ts.Close()
			c := jsonClient(ts.URL, "")
			stub.writeCalls.Store(0)

			// 1.2 MiB of content is about 1.6 MB of base64 JSON: over the 1.5 MiB body limit.
			err := writeFileOf(c, 1200*1024)
			require.Error(t, err)
			assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(err))
			var ce *connect.Error
			require.ErrorAs(t, err, &ce)
			assert.Empty(t, ce.Details(), "no typed detail can be attached")
			assert.Zero(t, stub.writeCalls.Load(), "handler never ran")

			require.NoError(t, writeFileOf(c, 1<<20), "1 MiB of content is accepted")
			assert.EqualValues(t, 1<<20, stub.writeBytes.Load())

			// 1.1 MiB is under the body limit, so it reaches the handler (which then answers TooLarge).
			require.NoError(t, writeFileOf(c, 1100*1024))
			assert.EqualValues(t, 1100*1024, stub.writeBytes.Load())

			// Another procedure's large body is unaffected.
			_, err = c.WriteToSession(context.Background(), connect.NewRequest(&sessionv1.WriteToSessionRequest{
				SessionId: "s", Input: strings.Repeat("a", 4<<20),
			}))
			require.NoError(t, err)
			assert.EqualValues(t, 4<<20, stub.sessionBytes.Load())
		})
	}
}

// ADR-031 "Spike outcomes" (Task 0.1.3b): remote access has a single admit decision
// (authenticated or not), no read-only tier, so WriteFile needs no per-RPC tier deny.
func TestWriteFile_ShouldMatchRecordedTierOutcome_When_RemoteAccessCallerPerTier(t *testing.T) {
	srv, stub := newWriteFileChainServer(t)
	ts := httptest.NewServer(srv.remoteChain(middleware.Auth(reconValidator{}), true))
	defer ts.Close()

	t.Run("unauthenticated caller is refused before the handler", func(t *testing.T) {
		err := writeFileOf(jsonClient(ts.URL, ""), 10)
		require.Error(t, err)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
		assert.Zero(t, stub.writeCalls.Load())
	})
	t.Run("authenticated caller reaches WriteFile (the only tier that exists)", func(t *testing.T) {
		require.NoError(t, writeFileOf(jsonClient(ts.URL, reconGoodToken), 10))
		assert.EqualValues(t, 1, stub.writeCalls.Load())
	})
}
