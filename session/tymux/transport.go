package tymux

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
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
