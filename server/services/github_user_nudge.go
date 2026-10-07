package services

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// NudgeSessionForPR is a placeholder so the proto change (Task 1.1.1b) compiles;
// the handler body lands with Story 2.3.1.
func (s *GitHubUserService) NudgeSessionForPR(_ context.Context, _ *connect.Request[sessionv1.NudgeSessionForPRRequest]) (*connect.Response[sessionv1.NudgeSessionForPRResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, errors.New("NudgeSessionForPR not implemented yet"))
}
