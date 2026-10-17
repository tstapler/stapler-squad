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
