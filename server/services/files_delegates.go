package services

import (
	"context"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// StatFile returns a file's version token, size and read-only reason without its content.
// +api: StatFile
func (s *SessionService) StatFile(
	ctx context.Context,
	req *connect.Request[sessionv1.StatFileRequest],
) (*connect.Response[sessionv1.StatFileResponse], error) {
	return s.fileSvc.StatFile(ctx, req)
}

// GetFileTransportCapabilities reports which file operations and wires this server serves.
// +api: GetFileTransportCapabilities
func (s *SessionService) GetFileTransportCapabilities(
	ctx context.Context,
	req *connect.Request[sessionv1.GetFileTransportCapabilitiesRequest],
) (*connect.Response[sessionv1.GetFileTransportCapabilitiesResponse], error) {
	return s.fileSvc.GetFileTransportCapabilities(ctx, req)
}

// GetFileTransportCapabilities is the wire adapter over FileOps.Capabilities.
func (fs *FileService) GetFileTransportCapabilities(
	ctx context.Context,
	_ *connect.Request[sessionv1.GetFileTransportCapabilitiesRequest],
) (*connect.Response[sessionv1.GetFileTransportCapabilitiesResponse], error) {
	return connect.NewResponse(&sessionv1.GetFileTransportCapabilitiesResponse{
		Capabilities: fs.ops.Capabilities(ctx),
	}), nil
}

// SetTelemetry wires the Files outcome port; call at startup, before serving.
func (fs *FileService) SetTelemetry(t FilesTelemetry) {
	if t != nil {
		fs.ops.policy.Telemetry = t
	}
}

// SearchContent streams content-search matches for a session's worktree.
// +api: SearchContent
func (s *SessionService) SearchContent(
	ctx context.Context,
	req *connect.Request[sessionv1.SearchContentRequest],
	stream *connect.ServerStream[sessionv1.SearchContentResponse],
) error {
	return s.fileSvc.SearchContent(ctx, req, stream)
}

// SearchContent is the wire adapter over SearchHandler: peer address and stream in, policy out.
func (fs *FileService) SearchContent(
	ctx context.Context,
	req *connect.Request[sessionv1.SearchContentRequest],
	stream *connect.ServerStream[sessionv1.SearchContentResponse],
) error {
	return fs.search.Handle(ctx, req.Msg, req.Peer().Addr, stream.Send)
}
