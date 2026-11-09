package tymux

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
	"github.com/tstapler/tymux/clients/go/gen/tymux/v1/tymuxv1connect"
	"golang.org/x/net/http2"
)

// closeOnlyStream models an HTTP/2 bidi stream whose headers have arrived: cancelling the
// Attach ctx does not wake a blocked Receive (verified against x/net v0.58 + connect v1.20),
// only CloseResponse does.
type closeOnlyStream struct {
	closed chan struct{}
	once   sync.Once
}

func newCloseOnlyStream() *closeOnlyStream { return &closeOnlyStream{closed: make(chan struct{})} }

func (c *closeOnlyStream) Send(*v1.AttachRequest) error { return nil }
func (c *closeOnlyStream) Receive() (*v1.AttachEvent, error) {
	<-c.closed
	return nil, errors.New("response body closed")
}
func (c *closeOnlyStream) CloseRequest() error { return nil }
func (c *closeOnlyStream) CloseResponse() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func TestTeardownStandingStream_should_NotWaitOutAReaderThatIgnoresContextCancel(t *testing.T) {
	dir := t.TempDir()
	stream := newCloseOnlyStream()
	transport := &fakeTransport{
		createSessionFn: func(context.Context, *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
			return connect.NewResponse(fakeSession("sess-1", "pane-1", dir, v1.Liveness_LIVENESS_LIVE)), nil
		},
	}
	transport.attachFn = func(context.Context) attachStream { return stream }
	sess := NewTymuxGRPCSession(transport)
	s := setTeardownWait(sess, 3*time.Second)
	require.NoError(t, sess.Start(dir))
	t.Cleanup(func() { _ = sess.Close() })

	start := time.Now()
	s.teardownStandingStream()

	assert.Less(t, time.Since(start), time.Second, "teardown must close the response instead of waiting the full bound")
}

type headersThenIdleSvc struct {
	tymuxv1connect.UnimplementedTymuxServiceHandler
}

func (headersThenIdleSvc) Attach(ctx context.Context, st *connect.BidiStream[v1.AttachRequest, v1.AttachEvent]) error {
	go func() {
		for {
			if _, err := st.Receive(); err != nil {
				return
			}
		}
	}()
	_ = st.Send(&v1.AttachEvent{}) // flushes response headers, like tymuxd's priming Snapshot
	<-ctx.Done()
	return ctx.Err()
}

// Against a real h2c connect server: once headers have arrived, CloseResponse must wake a
// blocked Receive. (Cancelling the ctx alone did not within 8s on x/net v0.58 + connect v1.20,
// which is why teardownStandingStream closes the response; not asserted here so an upstream fix
// does not break this test.)
func TestRealHTTP2Stream_should_WakeReceiveOnCloseResponseAfterHeaders(t *testing.T) {
	t.Parallel()
	_, h := tymuxv1connect.NewTymuxServiceHandler(headersThenIdleSvc{})
	srv := httptest.NewUnstartedServer(h)
	srv.Config.Protocols = new(http.Protocols)
	srv.Config.Protocols.SetUnencryptedHTTP2(true)
	srv.Start()
	t.Cleanup(srv.Close)
	hc := &http.Client{Transport: &http2.Transport{AllowHTTP: true, DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	st := tymuxv1connect.NewTymuxServiceClient(hc, srv.URL, connect.WithGRPC()).Attach(ctx)
	require.NoError(t, st.Send(&v1.AttachRequest{Payload: &v1.AttachRequest_PaneId{PaneId: "p"}}))
	_, err := st.Receive()
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { _, rerr := st.Receive(); done <- rerr }()
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, st.CloseResponse())

	select {
	case rerr := <-done:
		require.Error(t, rerr)
	case <-time.After(2 * time.Second):
		t.Fatal("Receive still blocked 2s after CloseResponse")
	}
}
