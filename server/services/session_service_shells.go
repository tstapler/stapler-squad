package services

// session_service_shells.go — ConnectRPC handlers for the custom-shell RPCs:
// SpawnShell, StopShell, RestartShell, ListShells, DeleteShell.
//
// All handlers follow the same pattern:
//  1. Validate required fields.
//  2. Resolve the session title (FindLiveInstance accepts a title).
//  3. Look up the live Instance via FindLiveInstance.
//  4. Delegate to the Instance method (instance_shells.go).
//  5. Convert Go types to proto types and return the response.

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// goShellStatusToProto maps the Go-side ShellStatus string constant to the proto enum.
func goShellStatusToProto(s session.ShellStatus) sessionv1.ShellStatus {
	switch s {
	case session.ShellStatusRunning:
		return sessionv1.ShellStatus_SHELL_STATUS_RUNNING
	case session.ShellStatusStopped:
		return sessionv1.ShellStatus_SHELL_STATUS_STOPPED
	case session.ShellStatusError:
		return sessionv1.ShellStatus_SHELL_STATUS_ERROR
	default:
		return sessionv1.ShellStatus_SHELL_STATUS_UNSPECIFIED
	}
}

// shellToProto converts an in-memory session.Shell to the proto Shell message.
func shellToProto(sh *session.Shell) *sessionv1.Shell {
	if sh == nil {
		return nil
	}
	p := &sessionv1.Shell{
		Id:         sh.ID,
		Name:       sh.Name,
		Command:    sh.Command,
		WorkingDir: sh.WorkingDir,
		Status:     goShellStatusToProto(sh.Status),
		ExitCode:   int32(sh.ExitCode),   //#nosec G115 -- POSIX process exit code, always small
		OrderIndex: int32(sh.OrderIndex), //#nosec G115 -- position within a session's shell list, always small
	}
	if !sh.StartedAt.IsZero() {
		p.StartedAt = timestamppb.New(sh.StartedAt)
	}
	return p
}

// resolveSessionTitle resolves a session title from the supplied session_id.
// session_id may be a Title or a UUID; we scan instance data to find the canonical title.
// Returns (title, nil) on success or (_, connect.Error) on failure.
func (s *SessionService) resolveSessionTitle(sessionID string) (string, error) {
	dataSlice, err := s.storage.ListInstanceData()
	if err != nil {
		return "", connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list sessions: %w", err))
	}
	for _, d := range dataSlice {
		if d.Title == sessionID || d.UUID == sessionID {
			return d.Title, nil
		}
	}
	return "", connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", sessionID))
}

// SpawnShell creates and starts a new custom shell attached to a session.
// +api: SpawnShell
func (s *SessionService) SpawnShell(
	ctx context.Context,
	req *connect.Request[sessionv1.SpawnShellRequest],
) (*connect.Response[sessionv1.SpawnShellResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}

	title, err := s.resolveSessionTitle(req.Msg.SessionId)
	if err != nil {
		return nil, err
	}

	inst := s.FindLiveInstance(title)
	if inst == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("session %q is not running", req.Msg.SessionId))
	}

	shell, spawnErr := inst.SpawnShell(ctx, session.SpawnShellRequest{
		Name:       req.Msg.Name,
		Command:    req.Msg.Command,
		WorkingDir: req.Msg.WorkingDir,
	})
	if spawnErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("SpawnShell failed: %w", spawnErr))
	}

	// Stamp StartedAt if not set (defensive; SpawnShell always sets it).
	if shell.StartedAt.IsZero() {
		shell.StartedAt = time.Now()
	}

	return connect.NewResponse(&sessionv1.SpawnShellResponse{
		Shell: shellToProto(shell),
	}), nil
}

// StopShell stops a running custom shell.
func (s *SessionService) StopShell(
	ctx context.Context,
	req *connect.Request[sessionv1.StopShellRequest],
) (*connect.Response[sessionv1.StopShellResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	if req.Msg.ShellId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("shell_id is required"))
	}

	title, err := s.resolveSessionTitle(req.Msg.SessionId)
	if err != nil {
		return nil, err
	}

	inst := s.FindLiveInstance(title)
	if inst == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("session %q is not running", req.Msg.SessionId))
	}

	// Capture the shell's tmux session name before stopping so a stale streamer
	// for it can be evicted below; StopShell doesn't return this itself.
	tmuxSessionName, hasTmuxName := inst.GetShellTmuxSessionName(req.Msg.ShellId)

	if stopErr := inst.StopShell(ctx, req.Msg.ShellId); stopErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("StopShell failed: %w", stopErr))
	}

	// Evict this shell's ExternalTmuxStreamer so a reopened shell with the same ID
	// gets a fresh streamer instead of replaying a degraded/stale one indefinitely.
	if hasTmuxName && s.tmuxStreamerManager != nil {
		s.tmuxStreamerManager.Remove(tmuxSessionName)
	}

	return connect.NewResponse(&sessionv1.StopShellResponse{
		Success: true,
		Message: fmt.Sprintf("Shell %q stopped", req.Msg.ShellId),
	}), nil
}

// RestartShell stops a shell (if running) and relaunches it with the same command.
func (s *SessionService) RestartShell(
	ctx context.Context,
	req *connect.Request[sessionv1.RestartShellRequest],
) (*connect.Response[sessionv1.RestartShellResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	if req.Msg.ShellId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("shell_id is required"))
	}

	title, err := s.resolveSessionTitle(req.Msg.SessionId)
	if err != nil {
		return nil, err
	}

	inst := s.FindLiveInstance(title)
	if inst == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("session %q is not running", req.Msg.SessionId))
	}

	// shellTmuxSessionName is deterministic per shell ID, so RestartShell reuses the
	// same tmux session name — evict any existing streamer first so the new session
	// gets a fresh one rather than a streamer still caching content from before the restart.
	tmuxSessionName, hasTmuxName := inst.GetShellTmuxSessionName(req.Msg.ShellId)
	if hasTmuxName && s.tmuxStreamerManager != nil {
		s.tmuxStreamerManager.Remove(tmuxSessionName)
	}

	if restartErr := inst.RestartShell(ctx, req.Msg.ShellId); restartErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("RestartShell failed: %w", restartErr))
	}

	return connect.NewResponse(&sessionv1.RestartShellResponse{
		Success: true,
		Message: fmt.Sprintf("Shell %q restarted", req.Msg.ShellId),
	}), nil
}

// ListShells returns all custom shells for a session, sorted by order_index.
func (s *SessionService) ListShells(
	ctx context.Context,
	req *connect.Request[sessionv1.ListShellsRequest],
) (*connect.Response[sessionv1.ListShellsResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}

	title, err := s.resolveSessionTitle(req.Msg.SessionId)
	if err != nil {
		return nil, err
	}

	inst := s.FindLiveInstance(title)
	if inst == nil {
		// Session exists in storage but has no live instance — return empty list rather than
		// an error, because the frontend polls this on load before the session is fully running.
		return connect.NewResponse(&sessionv1.ListShellsResponse{
			Shells: []*sessionv1.Shell{},
		}), nil
	}

	goShells := inst.ListShellsInMemory()
	protoShells := make([]*sessionv1.Shell, 0, len(goShells))
	for _, sh := range goShells {
		protoShells = append(protoShells, shellToProto(sh))
	}

	return connect.NewResponse(&sessionv1.ListShellsResponse{
		Shells: protoShells,
	}), nil
}

// DeleteShell stops a shell and removes it from storage.
func (s *SessionService) DeleteShell(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteShellRequest],
) (*connect.Response[sessionv1.DeleteShellResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	if req.Msg.ShellId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("shell_id is required"))
	}

	title, err := s.resolveSessionTitle(req.Msg.SessionId)
	if err != nil {
		return nil, err
	}

	inst := s.FindLiveInstance(title)
	if inst == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("session %q is not running", req.Msg.SessionId))
	}

	tmuxSessionName, hasTmuxName := inst.GetShellTmuxSessionName(req.Msg.ShellId)

	if delErr := inst.DeleteShell(ctx, req.Msg.ShellId); delErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("DeleteShell failed: %w", delErr))
	}

	if hasTmuxName && s.tmuxStreamerManager != nil {
		s.tmuxStreamerManager.Remove(tmuxSessionName)
	}

	return connect.NewResponse(&sessionv1.DeleteShellResponse{
		Success: true,
		Message: fmt.Sprintf("Shell %q deleted", req.Msg.ShellId),
	}), nil
}
