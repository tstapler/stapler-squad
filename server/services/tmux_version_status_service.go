package services

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// +api: tmux:version-status
// GetTmuxVersionStatus reports any tmux client/server version mismatch this
// process has detected (session/tmux/version_check.go) -- a mismatch disables
// control mode for the affected socket, degrading terminal input/output to
// slower per-command subprocess calls.
func (s *SessionService) GetTmuxVersionStatus(
	_ context.Context,
	_ *connect.Request[sessionv1.GetTmuxVersionStatusRequest],
) (*connect.Response[sessionv1.GetTmuxVersionStatusResponse], error) {
	mismatches := tmux.GetVersionMismatches()
	resp := &sessionv1.GetTmuxVersionStatusResponse{
		Mismatches: make([]*sessionv1.TmuxVersionMismatch, 0, len(mismatches)),
	}
	for _, m := range mismatches {
		count := 0
		if sessions, err := tmux.ListAllSessions(m.ServerSocket); err == nil {
			count = len(sessions)
		}
		resp.Mismatches = append(resp.Mismatches, &sessionv1.TmuxVersionMismatch{
			ServerSocket:         m.ServerSocket,
			ClientVersion:        m.ClientVersion,
			ServerVersion:        m.ServerVersion,
			AffectedSessionCount: int32(count), //#nosec G115 -- live session count, bounded well under int32 max
		})
	}
	return connect.NewResponse(resp), nil
}

// +api: tmux:restart-server
// RestartTmuxServer kills the tmux server on the given socket so it restarts
// on this process's own tmux binary, resolving a version mismatch.
// DESTRUCTIVE: every session on that socket loses its live terminal.
// The frontend must obtain explicit user confirmation before calling this;
// this handler performs no additional confirmation of its own.
func (s *SessionService) RestartTmuxServer(
	_ context.Context,
	req *connect.Request[sessionv1.RestartTmuxServerRequest],
) (*connect.Response[sessionv1.RestartTmuxServerResponse], error) {
	socket := req.Msg.GetServerSocket()
	affected := 0
	if sessions, err := tmux.ListAllSessions(socket); err == nil {
		// A non-nil err (e.g. ErrServerDown) means the server is already gone
		// -- nothing was affected by this call, so 0 is the correct count.
		affected = len(sessions)
	}

	if err := tmux.RestartTmuxServer(socket); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("restart tmux server: %w", err))
	}
	log.Warn("[RestartTmuxServer] tmux server restarted to resolve a client/server version mismatch", "server_socket", socket, "sessions_affected", affected)

	return connect.NewResponse(&sessionv1.RestartTmuxServerResponse{
		SessionsAffected: int32(affected), //#nosec G115 -- live session count, bounded well under int32 max
	}), nil
}
