package tymux

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
	"github.com/tstapler/tymux/clients/go/gen/tymux/v1/tymuxv1connect"
)

type connKey struct{}

// connRecorder records which accepted net.Conn each RPC arrived on.
type connRecorder struct {
	tymuxv1connect.UnimplementedTymuxServiceHandler
	mu       sync.Mutex
	infoConn net.Conn
	attached chan net.Conn
	release  chan struct{}
}

func (r *connRecorder) GetServerInfo(ctx context.Context, _ *connect.Request[v1.GetServerInfoRequest]) (*connect.Response[v1.ServerInfo], error) {
	r.mu.Lock()
	r.infoConn, _ = ctx.Value(connKey{}).(net.Conn)
	r.mu.Unlock()
	return connect.NewResponse(&v1.ServerInfo{Version: "test"}), nil
}

func (r *connRecorder) Attach(ctx context.Context, stream *connect.BidiStream[v1.AttachRequest, v1.AttachEvent]) error {
	if _, err := stream.Receive(); err != nil {
		return err
	}
	c, _ := ctx.Value(connKey{}).(net.Conn)
	r.attached <- c
	<-r.release
	return nil
}

// A CapturePane/resize/GetServerInfo call must not share an HTTP/2 connection with the standing
// Attach streams: one connection means one flow-control window and one reader goroutine for all
// of them, so a pane streaming a burst stalls every other pane's control RPCs behind it.
func TestRealTransport_AttachStreamsAndControlRPCsUseSeparateConnections(t *testing.T) {
	dir, err := os.MkdirTemp("", "ssq")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "d.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)

	rec := &connRecorder{attached: make(chan net.Conn, 1), release: make(chan struct{})}
	mux := http.NewServeMux()
	mux.Handle(tymuxv1connect.NewTymuxServiceHandler(rec))
	srv := &http.Server{
		Handler:     mux,
		Protocols:   new(http.Protocols),
		ConnContext: func(ctx context.Context, c net.Conn) context.Context { return context.WithValue(ctx, connKey{}, c) },
	}
	srv.Protocols.SetUnencryptedHTTP2(true)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { close(rec.release); _ = ln.Close(); _ = srv.Close() })

	addr := closedAddr(t)
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")
	t.Setenv("TYMUXD_ADDR", addr)
	t.Setenv("TYMUXD_SOCKET_PATH", sock)
	tr := NewRealTransport(addr)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	stream := tr.Attach(ctx)
	require.NoError(t, stream.Send(&v1.AttachRequest{Payload: &v1.AttachRequest_PaneId{PaneId: "p1"}}))
	attachConn := <-rec.attached

	_, err = tr.GetServerInfo(ctx, connect.NewRequest(&v1.GetServerInfoRequest{}))
	require.NoError(t, err)
	rec.mu.Lock()
	infoConn := rec.infoConn
	rec.mu.Unlock()

	require.NotNil(t, attachConn)
	require.NotNil(t, infoConn)
	require.NotSame(t, attachConn, infoConn, "Attach and control RPCs rode the same connection")
}
