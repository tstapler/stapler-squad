package sessioncreate

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/git"
	instancepkg "github.com/tstapler/stapler-squad/session/instance"
)

// PreviewDestinationPath computes where a session's checkout/worktree would land
// without performing any git or filesystem mutation. Used by the Omnibar to show a
// live destination hint before the user submits session creation.
// +api: session:preview-destination-path
func (h *Handler) PreviewDestinationPath(
	ctx context.Context,
	req *connect.Request[sessionv1.PreviewDestinationPathRequest],
) (*connect.Response[sessionv1.PreviewDestinationPathResponse], error) {
	cfg := config.LoadConfig()

	switch req.Msg.Mode {
	case "github_url":
		ref, err := instancepkg.ParseGitHubURLWithHosts(req.Msg.Input, h.EnterpriseHosts(cfg))
		if err != nil {
			return connect.NewResponse(&sessionv1.PreviewDestinationPathResponse{
				UnresolvedReason: "not a recognized GitHub URL",
			}), nil
		}
		path := instancepkg.DefaultRepoPathManager.GetRepoPath(ref)
		return connect.NewResponse(&sessionv1.PreviewDestinationPathResponse{
			Path:    path,
			IsExact: true,
		}), nil

	case "new_worktree":
		if req.Msg.RepoPath == "" || req.Msg.SessionName == "" {
			return connect.NewResponse(&sessionv1.PreviewDestinationPathResponse{
				UnresolvedReason: "repo_path and session_name are required",
			}), nil
		}
		prefix, err := git.PreviewWorktreePath(req.Msg.RepoPath, req.Msg.SessionName)
		if err != nil {
			return connect.NewResponse(&sessionv1.PreviewDestinationPathResponse{
				UnresolvedReason: fmt.Sprintf("could not resolve repo path: %v", err),
			}), nil
		}
		return connect.NewResponse(&sessionv1.PreviewDestinationPathResponse{
			Path:    prefix,
			IsExact: false,
		}), nil

	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown mode: %q", req.Msg.Mode))
	}
}
