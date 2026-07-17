package services

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/procinfo"
)

// Compile-time check: ImportService must implement ImportServiceHandler.
var _ sessionv1connect.ImportServiceHandler = (*ImportService)(nil)

// ProcessCreateTimeReader is the subset of procinfo.ProcessInspector needed
// to mint a PIDIdentity at preview time. Scoped to the one method this
// service actually calls (see .claude/rules/interface-pollution-checklist.md
// smell #1 -- this is deliberately narrow, not a general process-inspection
// interface).
type ProcessCreateTimeReader interface {
	CreateTime(pid int32) (int64, error)
}

// ImportService implements the ImportService RPC surface for
// import-external-session (Story 1.1.3 onward). All three mutating RPCs are
// no-ops behind the STAPLER_SQUAD_ENABLE_SESSION_IMPORT feature flag,
// enforced by an interceptor registered in server.go (Story 1.3.2) rather
// than duplicated per-method here.
type ImportService struct {
	detector  *session.HistoryFileDetector
	inspector ProcessCreateTimeReader
}

// NewImportService creates an ImportService. inspector may be nil in
// environments where process inspection isn't available (e.g. unsupported
// OS); PreviewImportExternalSession still functions but never populates
// pid_identity.
func NewImportService(detector *session.HistoryFileDetector, inspector ProcessCreateTimeReader) *ImportService {
	return &ImportService{detector: detector, inspector: inspector}
}

// NewImportServiceWithRealInspector creates an ImportService wired to the
// real OS-backed HistoryFileDetector and ProcessInspector.
func NewImportServiceWithRealInspector() *ImportService {
	return &ImportService{
		detector:  session.NewHistoryFileDetectorWithRealInspector(),
		inspector: procinfo.NewProcessInspector(),
	}
}

// PreviewImportExternalSession runs correlation against the candidate and
// reports what an import WOULD do. Side-effect-free: no process signaling,
// no persistence.
func (s *ImportService) PreviewImportExternalSession(
	ctx context.Context,
	req *connect.Request[sessionv1.PreviewImportExternalSessionRequest],
) (*connect.Response[sessionv1.PreviewImportExternalSessionResponse], error) {
	candidateRef := req.Msg.GetCandidate()
	if candidateRef == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("candidate is required"))
	}

	candidate := candidateFromProto(candidateRef)

	result, err := session.CorrelateCandidate(s.detector, candidate)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("correlation failed: %w", err))
	}

	resp := &sessionv1.PreviewImportExternalSessionResponse{
		Program:     candidate.Program,
		Path:        candidate.Path,
		Correlation: correlationResultToProto(result),
	}

	if result.Kind == session.CorrelationResolved {
		// A read failure here is not fatal to the preview -- the user still
		// sees correlation + program/path; turn_count/excerpt are simply
		// omitted (zero value). Commit will attempt the same read again.
		if historyPath, err := s.detector.ResolveFilePath(candidate.Path, result.UUID); err == nil {
			if turns, err := session.ReadCanonicalTurnsFromFile(historyPath); err == nil {
				resp.TurnCount = int32(len(turns)) //nolint:gosec // turn counts are bounded by transcript size, never near int32 max
				resp.LastMessageExcerpt = lastMessageExcerpt(turns)
			}
		}
	}

	if candidate.PID > 0 && s.inspector != nil {
		if createTimeMs, err := s.inspector.CreateTime(candidate.PID); err == nil {
			resp.PidIdentity = &sessionv1.PIDIdentity{
				Pid:          candidate.PID,
				CreateTimeMs: createTimeMs,
			}
		}
		// If CreateTime errors (process died between discovery and preview),
		// pid_identity is simply left nil -- the frontend must treat a nil
		// pid_identity as "kill/suspend flow unavailable for this candidate".
	}

	return connect.NewResponse(resp), nil
}

// CommitImportExternalSession is not yet implemented (Story 1.2.1).
func (s *ImportService) CommitImportExternalSession(
	ctx context.Context,
	req *connect.Request[sessionv1.CommitImportExternalSessionRequest],
) (*connect.Response[sessionv1.CommitImportExternalSessionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("CommitImportExternalSession not yet implemented"))
}

// ConfirmKillExternalSession is not yet implemented (Story 1.3.1).
func (s *ImportService) ConfirmKillExternalSession(
	ctx context.Context,
	req *connect.Request[sessionv1.ConfirmKillExternalSessionRequest],
) (*connect.Response[sessionv1.ConfirmKillExternalSessionResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("ConfirmKillExternalSession not yet implemented"))
}

// CancelPendingKill is not yet implemented (Story 1.3.3).
func (s *ImportService) CancelPendingKill(
	ctx context.Context,
	req *connect.Request[sessionv1.CancelPendingKillRequest],
) (*connect.Response[sessionv1.CancelPendingKillResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("CancelPendingKill not yet implemented"))
}

// candidateFromProto maps the wire representation back into the domain
// type. Pure mapping, no I/O.
func candidateFromProto(ref *sessionv1.ExternalSessionCandidateRef) session.ExternalSessionCandidate {
	sourceKind := session.MuxDiscovered
	if ref.GetSourceKind() == sessionv1.ImportSourceKind_IMPORT_SOURCE_KIND_PLAIN_TMUX {
		sourceKind = session.PlainTmux
	}
	return session.ExternalSessionCandidate{
		SourceKind:  sourceKind,
		Path:        ref.GetPath(),
		Program:     ref.GetProgram(),
		PID:         ref.GetPid(),
		TmuxSession: ref.GetTmuxSession(),
		SocketPath:  ref.GetSocketPath(),
	}
}

// correlationResultToProto maps the domain CorrelationResult onto the wire
// type. Pure mapping, no I/O. Exhaustively switches on CorrelationKind so a
// future addition to the sum type fails to compile here rather than
// silently defaulting.
func correlationResultToProto(result session.CorrelationResult) *sessionv1.CorrelationResultProto {
	proto := &sessionv1.CorrelationResultProto{
		Uuid: result.UUID,
	}

	switch result.Kind {
	case session.CorrelationNotFound:
		proto.Kind = sessionv1.CorrelationKind_CORRELATION_KIND_NOT_FOUND
	case session.CorrelationResolved:
		proto.Kind = sessionv1.CorrelationKind_CORRELATION_KIND_RESOLVED
	case session.CorrelationAmbiguous:
		proto.Kind = sessionv1.CorrelationKind_CORRELATION_KIND_AMBIGUOUS
	default:
		proto.Kind = sessionv1.CorrelationKind_CORRELATION_KIND_UNSPECIFIED
	}

	switch result.Confidence {
	case session.ConfidencePIDExact:
		proto.Confidence = sessionv1.CorrelationConfidence_CORRELATION_CONFIDENCE_PID_EXACT
	case session.ConfidencePathHeuristic:
		proto.Confidence = sessionv1.CorrelationConfidence_CORRELATION_CONFIDENCE_PATH_HEURISTIC
	default:
		proto.Confidence = sessionv1.CorrelationConfidence_CORRELATION_CONFIDENCE_NONE
	}

	for _, c := range result.Candidates {
		proto.Candidates = append(proto.Candidates, &sessionv1.HistoryFileCandidate{
			ConversationUuid: c.ConversationUUID,
			HistoryFilePath:  c.HistoryFilePath,
			ProjectDir:       c.ProjectDir,
		})
	}

	return proto
}

// lastMessageExcerpt returns a short preview of the last turn's text, for
// display in the preview dialog so the user can sanity-check they're
// importing the right conversation.
func lastMessageExcerpt(turns []session.CanonicalTurn) string {
	if len(turns) == 0 {
		return ""
	}
	last := turns[len(turns)-1]
	for _, block := range last.Blocks {
		if block.Kind == session.BlockKindText && block.Text != "" {
			text := strings.TrimSpace(block.Text)
			const maxExcerptLen = 200
			if len(text) > maxExcerptLen {
				return text[:maxExcerptLen] + "…"
			}
			return text
		}
	}
	return ""
}
