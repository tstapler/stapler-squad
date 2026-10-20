package server

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/middleware"
)

// Story 0.1.1 regression: a short-lived server-streaming RPC under /api/ must
// reach the client incrementally through the real chains (otelhttp included),
// not as one burst at the end. SearchContent will ride this path.

const (
	streamStepDelay  = 100 * time.Millisecond
	streamMinGap     = 80 * time.Millisecond
	streamMessageCnt = 4 // 3 batches 100 ms apart, then a final message
)

type streamStub struct {
	sessionv1connect.UnimplementedSessionServiceHandler
}

func (streamStub) WatchSessions(ctx context.Context, _ *connect.Request[sessionv1.WatchSessionsRequest], st *connect.ServerStream[sessionv1.SessionEvent]) error {
	for i := 0; i < streamMessageCnt; i++ {
		if i > 0 {
			select {
			case <-time.After(streamStepDelay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err := st.Send(&sessionv1.SessionEvent{}); err != nil {
			return err
		}
	}
	return nil
}

func newStreamChainServer(t *testing.T) *Server {
	t.Helper()
	srv, _ := newServerBase("localhost:8543")
	t.Cleanup(srv.connCtxCancel)
	path, h := sessionv1connect.NewSessionServiceHandler(streamStub{}, ConnectOptions(nil)...)
	srv.RegisterConnectHandler("/api"+path, http.StripPrefix("/api", h))
	return srv
}

// arrivalGaps opens the stream with the encodings a browser offers and returns
// the delay of each message since the previous one (first: since the request).
func arrivalGaps(t *testing.T, baseURL string) []time.Duration {
	t.Helper()
	c := sessionv1connect.NewSessionServiceClient(http.DefaultClient, baseURL+"/api",
		connect.WithInterceptors(connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Accept-Encoding", "gzip, zstd")
				return next(ctx, req)
			}
		})))
	req := connect.NewRequest(&sessionv1.WatchSessionsRequest{})
	req.Header().Set("Accept-Encoding", "gzip, zstd")
	start := time.Now()
	st, err := c.WatchSessions(context.Background(), req)
	require.NoError(t, err)
	defer st.Close()
	var gaps []time.Duration
	last := start
	for st.Receive() {
		now := time.Now()
		gaps = append(gaps, now.Sub(last))
		last = now
	}
	require.NoError(t, st.Err())
	return gaps
}

func isIncremental(gaps []time.Duration) bool {
	if len(gaps) != streamMessageCnt {
		return false
	}
	for _, g := range gaps[1:] {
		if g < streamMinGap {
			return false
		}
	}
	return true
}

func assertIncremental(t *testing.T, gaps []time.Duration) {
	t.Helper()
	require.True(t, isIncremental(gaps), "buffered burst or missing messages (gaps %v)", gaps)
}

// bufferingLayer holds the whole response until the handler returns, the failure
// the chain test must be able to see.
func bufferingLayer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		next.ServeHTTP(rec, r)
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
	})
}

func TestStreamingChain_ShouldFlagBurstDelivery_When_ALayerBuffersTheResponse(t *testing.T) {
	srv := newStreamChainServer(t)
	ts := httptest.NewServer(bufferingLayer(srv.localChain()))
	defer ts.Close()
	require.False(t, isIncremental(arrivalGaps(t, ts.URL)))
}

func TestStreamingChain_ShouldDeliverMessagesIncrementally_ThroughLocalAndRemoteChains(t *testing.T) {
	srv := newStreamChainServer(t)
	admit := func(next http.Handler) http.Handler { return next }
	for name, chain := range map[string]http.Handler{
		"local":  srv.localChain(),
		"remote": srv.remoteChain(admit, false),
	} {
		t.Run(name, func(t *testing.T) {
			wrapped := otelhttp.NewHandler(chain, "stapler-squad-http",
				otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents))
			ts := httptest.NewServer(wrapped)
			defer ts.Close()
			gaps := arrivalGaps(t, ts.URL)
			t.Logf("%s arrival gaps: %v", name, gaps)
			if !isIncremental(gaps) {
				bisectBufferingLayer(t, srv)
			}
			assertIncremental(t, gaps)
		})
	}
}

// bisectBufferingLayer rebuilds the local chain with one layer removed at a
// time and logs which removal restores incremental delivery.
func bisectBufferingLayer(t *testing.T, srv *Server) {
	t.Helper()
	layers := map[string]func(http.Handler) http.Handler{
		"otelhttp": func(h http.Handler) http.Handler { return h },
		"logging":  middleware.Logging,
		"cors":     middleware.CORSWithOrigins(srv.origins),
		"compress": middleware.Compress,
	}
	build := func(skip string) http.Handler {
		var h http.Handler = srv
		h = middleware.HostGuard(srv.hostGuardConfig())(h)
		for _, n := range []string{"compress", "cors", "logging"} {
			if n != skip {
				h = layers[n](h)
			}
		}
		if skip != "otelhttp" {
			h = otelhttp.NewHandler(h, "bisect", otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents))
		}
		return h
	}
	for _, skip := range []string{"otelhttp", "logging", "cors", "compress"} {
		ts := httptest.NewServer(build(skip))
		gaps := arrivalGaps(t, ts.URL)
		ts.Close()
		t.Logf("without %s: gaps %v", skip, gaps)
	}
}

type protoSpy struct {
	rt    http.RoundTripper
	proto atomic.Value
}

func (p *protoSpy) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := p.rt.RoundTrip(r)
	if resp != nil {
		p.proto.Store(resp.Proto)
	}
	return resp, err
}

// startRealRemoteListener runs the production StartRemote (TLS, HTTP/2 via
// ServeTLS, otelhttp, remoteChain) on a free loopback port.
func startRealRemoteListener(t *testing.T, srv *Server) (baseURL string, spy *protoSpy) {
	t.Helper()
	dummy := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(dummy.Close)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	admit := func(next http.Handler) http.Handler { return next }
	require.NoError(t, srv.StartRemote(ctx, addr, &tls.Config{Certificates: dummy.TLS.Certificates}, admit, false))

	spy = &protoSpy{rt: &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // throwaway self-signed listener
		ForceAttemptHTTP2: true,
	}}
	return "https://" + addr, spy
}

func TestStreamingChain_ShouldDeliverIncrementally_OverRealRemoteTLSListener(t *testing.T) {
	srv := newStreamChainServer(t)
	base, spy := startRealRemoteListener(t, srv)
	c := sessionv1connect.NewSessionServiceClient(&http.Client{Transport: spy}, base+"/api")
	st, err := c.WatchSessions(context.Background(), connect.NewRequest(&sessionv1.WatchSessionsRequest{}))
	require.NoError(t, err)
	defer st.Close()
	var gaps []time.Duration
	last := time.Now()
	for st.Receive() {
		gaps = append(gaps, time.Since(last))
		last = time.Now()
	}
	require.NoError(t, st.Err())
	t.Logf("real StartRemote listener, negotiated %v, arrival gaps %v", spy.proto.Load(), gaps)
	assertIncremental(t, gaps)
}
