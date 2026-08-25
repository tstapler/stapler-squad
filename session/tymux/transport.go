package tymux

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"os"

	"connectrpc.com/connect"
	"golang.org/x/net/http2"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
	"github.com/tstapler/tymux/clients/go/gen/tymux/v1/tymuxv1connect"
)

// rpcTransport is the narrow seam between tymuxGRPCSession and the generated
// Connect-Go client (tymuxv1connect.TymuxServiceClient). It exposes only the
// methods tymuxGRPCSession actually calls, rather than the full generated
// client, so tests can substitute a hand-driven fake and unit-test
// Start/Close/IsAlive/exit-callback-ordering without a live tymuxd daemon.
//
// tymuxGRPCSession is architecturally the only real TymuxManager implementation
// (BackendTymux always wraps one), so this interface — not TymuxManager — is the
// actual testability seam for the RPC-calling logic; see plan.md Task 2.1.2d.
type rpcTransport interface {
	CreateSession(context.Context, *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error)
	KillSession(context.Context, *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error)
	ListSessions(context.Context, *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error)
	ReviveSession(context.Context, *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error)
	CapturePane(context.Context, *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error)
	// Attach opens the standing bidi stream. The returned attachStream exposes
	// only Send/Receive/CloseRequest/CloseResponse — the subset of
	// *connect.BidiStreamForClient[v1.AttachRequest, v1.AttachEvent] that
	// tymuxGRPCSession's Attach/ReconnectLoop logic (Epic 2.3/2.5) actually uses.
	Attach(context.Context) attachStream
}

// attachStream is the narrow surface of
// *connect.BidiStreamForClient[v1.AttachRequest, v1.AttachEvent] that
// tymuxGRPCSession uses, letting a fake rpcTransport substitute a hand-driven
// implementation without a live daemon.
type attachStream interface {
	Send(*v1.AttachRequest) error
	Receive() (*v1.AttachEvent, error)
	CloseRequest() error
	CloseResponse() error
}

// compile-time check that the generated bidi stream type satisfies attachStream.
var _ attachStream = (*connect.BidiStreamForClient[v1.AttachRequest, v1.AttachEvent])(nil)

// --- Real (non-fake) rpcTransport ---
//
// Epic 2.2's completion report flagged that no real rpcTransport existed
// in-tree — backend_factory.go could only ever construct
// tymux.NewTymuxGRPCSession(nil), and plan.md never explicitly assigns
// building one to any task in Epics 2.1-2.3. Epic 2.3 needs it: the
// standing Attach stream (Story 2.3.1) is the first place in this whole
// implementation a live RPC actually has to work, so a standing stream
// with no real transport underneath it would be meaningless. Closing that
// gap here, in Epic 2.1's own file, since this is the seam it defined.

// grpcTransport adapts the generated Connect-Go client
// (tymuxv1connect.TymuxServiceClient) to the narrower rpcTransport
// interface above. A plain type assertion/embedding can't do this
// directly: rpcTransport.Attach returns attachStream (the narrow seam),
// while the generated client's Attach returns the concrete
// *connect.BidiStreamForClient — satisfying attachStream (see the
// compile-time check above) but not identical to it, so Go's interface
// satisfaction rules need an explicit adapter method to convert one
// return type to the other.
type grpcTransport struct {
	client tymuxv1connect.TymuxServiceClient
}

func (t *grpcTransport) CreateSession(ctx context.Context, req *connect.Request[v1.CreateSessionRequest]) (*connect.Response[v1.Session], error) {
	return t.client.CreateSession(ctx, req)
}

func (t *grpcTransport) KillSession(ctx context.Context, req *connect.Request[v1.KillSessionRequest]) (*connect.Response[v1.KillSessionResponse], error) {
	return t.client.KillSession(ctx, req)
}

func (t *grpcTransport) ListSessions(ctx context.Context, req *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
	return t.client.ListSessions(ctx, req)
}

func (t *grpcTransport) ReviveSession(ctx context.Context, req *connect.Request[v1.ReviveSessionRequest]) (*connect.Response[v1.ReviveSessionResponse], error) {
	return t.client.ReviveSession(ctx, req)
}

func (t *grpcTransport) CapturePane(ctx context.Context, req *connect.Request[v1.CapturePaneRequest]) (*connect.Response[v1.PaneSnapshot], error) {
	return t.client.CapturePane(ctx, req)
}

func (t *grpcTransport) Attach(ctx context.Context) attachStream {
	return t.client.Attach(ctx)
}

// compile-time check that *grpcTransport satisfies rpcTransport.
var _ rpcTransport = (*grpcTransport)(nil)

// defaultTymuxdAddr matches clients/go/examples/list-sessions/main.go's
// default and tymuxd's documented loopback-only listen address/port.
const defaultTymuxdAddr = "http://127.0.0.1:7419"

// tymuxdAddr resolves the base URL for a real tymuxd connection: the
// TYMUXD_ADDR environment variable when set, else defaultTymuxdAddr.
// stapler-squad does not start or supervise tymuxd itself (Story 2.2.6's
// documented scope decision — no ensureServerRunning-equivalent), so this
// assumes an out-of-band, already-running daemon at a fixed, configurable
// address rather than any service-discovery mechanism.
func tymuxdAddr() string {
	if v := os.Getenv("TYMUXD_ADDR"); v != "" {
		return v
	}
	return defaultTymuxdAddr
}

// newH2CClient builds an http.Client able to speak plaintext HTTP/2 (h2c)
// to tymuxd, which listens on loopback-only plain HTTP/2 with no TLS (the
// loopback-trust security model) and is a strict gRPC server (tonic) that
// requires connect.WithGRPC() below — mirrors
// clients/go/examples/list-sessions/main.go's newClient exactly, since
// that example exists precisely to prove this transport shape works
// end-to-end against a real daemon (Task 1.6.2b).
func newH2CClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
		},
	}
}

// NewRealTransport constructs an rpcTransport backed by a real Connect-Go
// gRPC client against a live tymuxd. addr is the tymuxd base URL (e.g.
// "http://127.0.0.1:7419"); pass "" to use tymuxdAddr()'s
// TYMUXD_ADDR-or-default resolution.
func NewRealTransport(addr string) rpcTransport {
	if addr == "" {
		addr = tymuxdAddr()
	}
	client := tymuxv1connect.NewTymuxServiceClient(newH2CClient(), addr, connect.WithGRPC())
	return &grpcTransport{client: client}
}
