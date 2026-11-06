package services

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
)

// WriteGuard is the session-level write gate: hidden and archived sessions are
// refused. It only reads state and never starts or revives a session
// (.claude/rules/noarchivedrevival.md).
type WriteGuard interface {
	CheckWriteAccess(sessionID string) error
}

// StatusInvalidator drops cached VCS status for a directory after a save changed it.
type StatusInvalidator interface {
	InvalidateVCSStatus(workDir string)
}

var (
	errSessionHidden   = errors.New("session is a background session and is read-only")
	errSessionArchived = errors.New("session is archived")
)

// readOnlyError builds a Connect error carrying the typed ReadOnlyReasonDetail.
func reasonRefusal(code connect.Code, reason sessionv1.ReadOnlyReason, cause error) *connect.Error {
	ce := connect.NewError(code, cause)
	if d, err := connect.NewErrorDetail(&sessionv1.ReadOnlyReasonDetail{Reason: reason}); err == nil {
		ce.AddDetail(d)
	}
	return ce
}

func hiddenSessionRefusal() error {
	return reasonRefusal(connect.CodePermissionDenied, sessionv1.ReadOnlyReason_READ_ONLY_REASON_HIDDEN_SESSION, errSessionHidden)
}

// archivedSessionRefusal reuses WorktreeMissing: an archived session has no editable workspace.
func archivedSessionRefusal() error {
	return reasonRefusal(connect.CodeFailedPrecondition, sessionv1.ReadOnlyReason_READ_ONLY_REASON_WORKTREE_MISSING, errSessionArchived)
}

// FileWriter is the write-side collaborator of FileOps (ADR-036): the flag, guard,
// workspace and telemetry ports and the backend are FileOps's; the writer adds the
// status invalidator and the per-path mutex registry.
type FileWriter struct {
	ops         *FileOps
	invalidator StatusInvalidator
	locks       *pathMutexRegistry
	// preRenameHook runs just before the pre-rename re-check; tests use it to play the agent.
	preRenameHook func()
}

// NewFileWriter builds the writer over ops; a nil invalidator skips cache eviction.
func NewFileWriter(ops *FileOps, invalidator StatusInvalidator) *FileWriter {
	return &FileWriter{ops: ops, invalidator: invalidator, locks: pathLocks}
}

// WriteFile atomically replaces a file after verifying the caller's version token.
// Exactly one slog line, one metric sample and one telemetry event describe each call.
func (w *FileWriter) WriteFile(
	ctx context.Context,
	req *connect.Request[sessionv1.WriteFileRequest],
) (*connect.Response[sessionv1.WriteFileResponse], error) {
	start := time.Now()
	call := newWriteCall(req.Msg)
	resp, err := w.write(ctx, call)
	w.finish(ctx, call, resp, err, time.Since(start))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// finish records the outcome; nothing here can change the response.
func (w *FileWriter) finish(ctx context.Context, call *writeCall, resp *sessionv1.WriteFileResponse, err error, took time.Duration) {
	outcome, reason := call.classify(resp, err)
	log.Info("file write", "session", call.req.SessionId, "path", call.req.Path,
		"bytes", len(call.req.Content), "outcome", string(outcome))
	recordWriteMetrics(ctx, outcome, took)
	w.ops.policy.Telemetry.RecordWrite(ctx, call.req.SessionId, WriteOutcomeEvent{
		Outcome: outcome, Reason: reason, Size: BucketSize(int64(len(call.req.Content))),
		EOL: call.eol, BOM: call.bom, Duration: took,
	})
}
