package tymux

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
	"github.com/tstapler/tymux/clients/go/selecting"
)

// A socket that refuses us (untrusted path/owner, EACCES) belongs to a live daemon. Falling back to
// the unauthenticated TCP listener would hand it our traffic, so TCP must see zero requests on a
// call, a stream and a re-probe.

// recordingTCP is a healthy tymuxd TCP listener that counts every request it receives.
func recordingTCP(t *testing.T) (addr string, requests *atomic.Int32) {
	t.Helper()
	requests = new(atomic.Int32)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status")
		_, _ = w.Write([]byte{0, 0, 0, 0, 0})
		w.Header().Set("Grpc-Status", "0")
	}))
	srv.Config.Protocols = new(http.Protocols)
	srv.Config.Protocols.SetUnencryptedHTTP2(true)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return "http://" + ln.Addr().String(), requests
}

// hostileSocket names a socket path that, once armed, makes the client refuse it.
type hostileSocket struct {
	path string
	arm  func()
	want error
}

func hostileSockets(t *testing.T) map[string]func(*testing.T) hostileSocket {
	t.Helper()
	return map[string]func(*testing.T) hostileSocket{
		"untrusted (group-writable parent)": func(t *testing.T) hostileSocket {
			base, err := os.MkdirTemp(os.TempDir(), "ssq-nofb")
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.RemoveAll(base) })
			dir := filepath.Join(base, "loose")
			require.NoError(t, os.Mkdir(dir, 0o700))
			require.NoError(t, os.Chmod(dir, 0o770))
			sock := filepath.Join(dir, "tymuxd.sock")
			return hostileSocket{path: sock, want: selecting.ErrSocketUntrusted, arm: func() {
				ln, err := net.Listen("unix", sock)
				require.NoError(t, err)
				grpcOKServer(t, ln)
			}}
		},
		"EACCES (socket mode 000)": func(t *testing.T) hostileSocket {
			if os.Geteuid() == 0 {
				t.Skip("root bypasses socket file permissions")
			}
			dir, err := os.MkdirTemp("", "ssq")
			require.NoError(t, err)
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			sock := filepath.Join(dir, "tymuxd.sock")
			return hostileSocket{path: sock, want: ErrTymuxdSocketAccessDenied, arm: func() {
				ln, err := net.Listen("unix", sock)
				require.NoError(t, err)
				grpcOKServer(t, ln)
				require.NoError(t, os.Chmod(sock, 0))
			}}
		},
	}
}

func TestRefusedSocket_CallAndStreamNeverFallBackToTCP(t *testing.T) {
	for name, mk := range hostileSockets(t) {
		t.Run(name, func(t *testing.T) {
			hs := mk(t)
			addr, tcpRequests := recordingTCP(t)
			t.Setenv("STAPLER_SQUAD_INSTANCE", "")
			t.Setenv("TYMUXD_ADDR", addr)
			t.Setenv("TYMUXD_SOCKET_PATH", hs.path)
			hs.arm()
			tr := NewRealTransport(addr)

			_, err := tr.GetServerInfo(context.Background(), connect.NewRequest(&v1.GetServerInfoRequest{}))
			require.ErrorIs(t, err, hs.want, "call")

			stream := tr.Attach(context.Background())
			err = stream.Send(&v1.AttachRequest{})
			if err == nil {
				_, err = stream.Receive()
			}
			require.ErrorIs(t, err, hs.want, "stream")

			require.Zero(t, tcpRequests.Load(), "never fall back to the unauthenticated listener")
			require.EqualValues(t, 1, transportGauge(t, addr, string(selecting.Unix)))
		})
	}
}

func TestRefusedSocket_ReprobeNeverFallsBackToTCP(t *testing.T) {
	for name, mk := range hostileSockets(t) {
		t.Run(name, func(t *testing.T) {
			hs := mk(t)
			addr, tcpRequests := recordingTCP(t)
			unix, _ := newUnixTransport(hs.path)
			tcp, _ := newTCPTransport(addr)
			var now atomic.Int64
			now.Store(time.Unix(1_000_000, 0).UnixNano())
			sel := selecting.NewFromTransports(unix, tcp, selecting.Options{Now: func() time.Time { return time.Unix(0, now.Load()) }})
			info := func() error {
				_, err := sel.GetServerInfo(context.Background(), connect.NewRequest(&v1.GetServerInfoRequest{}))
				return err
			}

			require.NoError(t, info(), "socket missing: falls back to TCP")
			require.Equal(t, selecting.TCP, sel.Active())

			hs.arm() // the socket appears, and refuses us
			before := tcpRequests.Load()
			now.Add(int64(reprobeInterval))

			require.ErrorIs(t, info(), hs.want, "the re-probe sees the refusal and surfaces it")
			require.Equal(t, selecting.Unix, sel.Active())
			require.ErrorIs(t, info(), hs.want)
			require.Equal(t, before, tcpRequests.Load(), "no request reached TCP after the socket appeared")
		})
	}
}
