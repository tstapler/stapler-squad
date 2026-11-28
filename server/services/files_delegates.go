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

// SearchContent streams content-search matches for a session's worktree.
// +api: SearchContent
func (s *SessionService) SearchContent(
	ctx context.Context,
	req *connect.Request[sessionv1.SearchContentRequest],
	stream *connect.ServerStream[sessionv1.SearchContentResponse],
) error {
	return s.fileSvc.SearchContent(ctx, req, stream)
}

// WriteFile atomically replaces a file in the session's worktree, guarded by a version token.
// +api: WriteFile
func (s *SessionService) WriteFile(
	ctx context.Context,
	req *connect.Request[sessionv1.WriteFileRequest],
) (*connect.Response[sessionv1.WriteFileResponse], error) {
	return s.fileSvc.WriteFile(ctx, req)
}

// ReadFileRange returns one line-aligned or byte-anchored window of a file.
// +api: ReadFileRange
func (s *SessionService) ReadFileRange(
	ctx context.Context,
	req *connect.Request[sessionv1.ReadFileRangeRequest],
) (*connect.Response[sessionv1.ReadFileRangeResponse], error) {
	return s.fileSvc.ReadFileRange(ctx, req)
}
