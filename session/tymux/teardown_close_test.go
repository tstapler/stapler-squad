package tymux

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
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
