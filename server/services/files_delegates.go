package services

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
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

// WriteFile atomically replaces a file in the session's worktree, guarded by a version token.
// +api: WriteFile
func (s *SessionService) WriteFile(
	ctx context.Context,
	req *connect.Request[sessionv1.WriteFileRequest],
) (*connect.Response[sessionv1.WriteFileResponse], error) {
	return s.fileSvc.WriteFile(ctx, req)
}

// WriteFile is the wire adapter over FileWriter.
func (fs *FileService) WriteFile(
	ctx context.Context,
	req *connect.Request[sessionv1.WriteFileRequest],
) (*connect.Response[sessionv1.WriteFileResponse], error) {
	return fs.writer.WriteFile(ctx, req)
}

// ReadFileRange returns one line-aligned or byte-anchored window of a file.
// +api: ReadFileRange
func (s *SessionService) ReadFileRange(
	ctx context.Context,
	req *connect.Request[sessionv1.ReadFileRangeRequest],
) (*connect.Response[sessionv1.ReadFileRangeResponse], error) {
	return s.fileSvc.ReadFileRange(ctx, req)
}

// ReadFileRange is the wire adapter over FileOps.ReadRange: peer address in, the
// range limiter, one slog line and one metric around the call.
func (fs *FileService) ReadFileRange(
	ctx context.Context,
	req *connect.Request[sessionv1.ReadFileRangeRequest],
) (*connect.Response[sessionv1.ReadFileRangeResponse], error) {
	start := time.Now()
	release, refused := fs.ranges.Acquire(req.Peer().Addr, req.Msg.SessionId)
	if refused != "" {
		err := withDetail(connect.NewError(connect.CodeResourceExhausted, errors.New("too many range reads are running")),
			&sessionv1.RangeBusyDetail{Reason: string(refused)})
		logRangeCall(ctx, req.Msg, nil, err, time.Since(start))
		return nil, err
	}
	defer release()
	resp, err := fs.ops.ReadRange(ctx, req.Msg)
	logRangeCall(ctx, req.Msg, resp, err, time.Since(start))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// rangeOutcome is the closed outcome set shared by the log line and the metric.
func rangeOutcome(resp *sessionv1.ReadFileRangeResponse, err error) string {
	switch {
	case err == nil && resp.Torn:
		return "torn"
	case err == nil:
		return "ok"
	}
	switch connect.CodeOf(err) {
	case connect.CodeResourceExhausted:
		return "busy"
	case connect.CodePermissionDenied:
		return "denied"
	case connect.CodeInvalidArgument, connect.CodeNotFound:
		return "invalid"
	case connect.CodeFailedPrecondition:
		return "disabled"
	default:
		return "error"
	}
}

// logRangeCall records the one slog line and the metric; it never logs file content.
func logRangeCall(ctx context.Context, req *sessionv1.ReadFileRangeRequest, resp *sessionv1.ReadFileRangeResponse, err error, took time.Duration) {
	outcome := rangeOutcome(resp, err)
	kind, bytes, lines := "lines", int64(0), int64(0)
	if req.GetBytes() != nil {
		kind = "bytes"
	}
	if resp != nil {
		bytes = resp.EndByte - resp.StartByte
		lines = max(resp.EndLine-resp.StartLine+1, 0)
	}
	log.Info("file range read", "session", req.SessionId, "path", req.Path, "kind", kind,
		"bytes", bytes, "lines", lines, "duration_ms", took.Milliseconds(), "outcome", outcome)
	recordRangeMetrics(ctx, outcome, took)
}
