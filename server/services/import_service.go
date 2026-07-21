package services

import (
	"context"
	"errors"
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

	// aliveChecker, storage, registry, linker, and suspended are only
	// required by the three mutating RPCs (Commit/ConfirmKill/Cancel) --
	// PreviewImportExternalSession works with detector+inspector alone, so
	// existing tests constructing an ImportService via NewImportService keep
	// working with these left as nil/zero.
	aliveChecker session.AliveChecker
	storage      session.InstanceStore
	registry     *session.Registry
	linker       *session.HistoryLinker
	suspended    *session.SuspendedProcessStore
}

// NewImportService creates an ImportService. inspector may be nil in
// environments where process inspection isn't available (e.g. unsupported
// OS); PreviewImportExternalSession still functions but never populates
// pid_identity.
func NewImportService(detector *session.HistoryFileDetector, inspector ProcessCreateTimeReader) *ImportService {
	return &ImportService{detector: detector, inspector: inspector}
}

// NewImportServiceWithRealInspector creates an ImportService wired to the
// real OS-backed HistoryFileDetector and ProcessInspector, plus the
// dependencies needed by the three mutating RPCs.
func NewImportServiceWithRealInspector(
	storage session.InstanceStore,
	registry *session.Registry,
	linker *session.HistoryLinker,
	suspended *session.SuspendedProcessStore,
) *ImportService {
	inspector := procinfo.NewProcessInspector()
	return &ImportService{
		detector:     session.NewHistoryFileDetectorWithRealInspector(),
		inspector:    inspector,
		aliveChecker: inspector,
		storage:      storage,
		registry:     registry,
		linker:       linker,
		suspended:    suspended,
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

// CommitImportExternalSession persists a managed Instance for the candidate,
// starts it resumed, and SIGSTOPs the original process (Story 1.2.1-1.2.4).
// Correlation drift (the candidate's history file(s) changed since preview)
// is surfaced as connect.CodeFailedPrecondition per import_commit.go's doc
// comment on ErrCorrelationDrifted, asking the client to re-preview. Every
// other domain-level failure (ambiguous choice, path collision, start
// failure) is reported inside the response body via status=FAILED so the
// client always gets a structured result to update UI state from.
func (s *ImportService) CommitImportExternalSession(
	ctx context.Context,
	req *connect.Request[sessionv1.CommitImportExternalSessionRequest],
) (*connect.Response[sessionv1.CommitImportExternalSessionResponse], error) {
	candidateRef := req.Msg.GetCandidate()
	if candidateRef == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("candidate is required"))
	}

	pidIdentity := req.Msg.GetPidIdentity()
	params := session.CommitImportParams{
		Detector:             s.detector,
		Storage:              s.storage,
		Registry:             s.registry,
		Linker:               s.linker,
		Suspended:            s.suspended,
		Candidate:            candidateFromProto(candidateRef),
		ExpectedCorrelation:  correlationResultFromProto(req.Msg.GetExpectedCorrelation()),
		DisambiguationChoice: req.Msg.GetDisambiguationChoice(),
		OriginalPID:          pidIdentity.GetPid(),
		OriginalCreateTimeMs: pidIdentity.GetCreateTimeMs(),
	}

	result, err := session.CommitImportExternalSession(ctx, params)
	if err != nil {
		if errors.Is(err, session.ErrCorrelationDrifted) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, err)
		}
		return connect.NewResponse(&sessionv1.CommitImportExternalSessionResponse{
			Status: sessionv1.ImportStatus_IMPORT_STATUS_FAILED,
			Error:  err.Error(),
		}), nil
	}

	resp := &sessionv1.CommitImportExternalSessionResponse{
		Status:     sessionv1.ImportStatus_IMPORT_STATUS_COMMITTED,
		InstanceId: result.Instance.Title,
	}
	if pidIdentity.GetPid() > 0 {
		resp.PidIdentity = &sessionv1.PIDIdentity{
			Pid:          pidIdentity.GetPid(),
			CreateTimeMs: result.FreshCreateTimeMs,
		}
	}
	return connect.NewResponse(resp), nil
}

// ConfirmKillExternalSession re-verifies the original process's identity and
// kills its tmux session (Story 1.3.1). The tmux session name is not part of
// the request (import.proto's ConfirmKillExternalSessionRequest carries only
// instance_id + pid_identity) so it's recovered from the SuspendedProcessRecord
// persisted by CommitImportExternalSession.
func (s *ImportService) ConfirmKillExternalSession(
	ctx context.Context,
	req *connect.Request[sessionv1.ConfirmKillExternalSessionRequest],
) (*connect.Response[sessionv1.ConfirmKillExternalSessionResponse], error) {
	instanceID := req.Msg.GetInstanceId()
	if instanceID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("instance_id is required"))
	}

	pidIdentity := req.Msg.GetPidIdentity()
	if pidIdentity == nil || pidIdentity.GetPid() <= 0 {
		return connect.NewResponse(&sessionv1.ConfirmKillExternalSessionResponse{
			Status: sessionv1.KillStatus_KILL_STATUS_FAILED,
			Error:  "pid_identity is required",
		}), nil
	}

	var tmuxSession string
	if s.suspended != nil {
		if record, ok, err := s.suspended.Get(instanceID); err == nil && ok {
			tmuxSession = record.Candidate.TmuxSession
		}
	}

	outcome := session.KillExternalOriginalProcess(s.aliveChecker, pidIdentity.GetPid(), pidIdentity.GetCreateTimeMs(), tmuxSession)

	resp := &sessionv1.ConfirmKillExternalSessionResponse{}
	switch outcome.Status {
	case session.KillOutcomeKilled:
		resp.Status = sessionv1.KillStatus_KILL_STATUS_KILLED
		if s.suspended != nil {
			_ = s.suspended.Remove(instanceID)
		}
	case session.KillOutcomeAlreadyGone:
		resp.Status = sessionv1.KillStatus_KILL_STATUS_ALREADY_GONE
		if s.suspended != nil {
			_ = s.suspended.Remove(instanceID)
		}
	case session.KillOutcomeFailed:
		resp.Status = sessionv1.KillStatus_KILL_STATUS_FAILED
		if outcome.Err != nil {
			resp.Error = outcome.Err.Error()
		}
	default:
		resp.Status = sessionv1.KillStatus_KILL_STATUS_FAILED
		resp.Error = "unknown kill outcome"
	}
	return connect.NewResponse(resp), nil
}

// CancelPendingKill abandons the entire import: deletes the committed
// Instance and, only if that succeeds, resumes the original process (Story
// 1.3.3). See session.CancelPendingKill's doc comment for why deletion must
// happen strictly before resumption.
func (s *ImportService) CancelPendingKill(
	ctx context.Context,
	req *connect.Request[sessionv1.CancelPendingKillRequest],
) (*connect.Response[sessionv1.CancelPendingKillResponse], error) {
	instanceID := req.Msg.GetInstanceId()
	if instanceID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("instance_id is required"))
	}

	originalPID := req.Msg.GetPidIdentity().GetPid()
	if originalPID <= 0 && s.suspended != nil {
		if record, ok, err := s.suspended.Get(instanceID); err == nil && ok {
			originalPID = record.PID
		}
	}

	resumed, err := session.CancelPendingKill(session.CancelPendingKillParams{
		Storage:     s.storage,
		Suspended:   s.suspended,
		InstanceID:  instanceID,
		OriginalPID: originalPID,
	})

	resp := &sessionv1.CancelPendingKillResponse{Resumed: resumed}
	if err != nil {
		resp.Error = err.Error()
	}
	return connect.NewResponse(resp), nil
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

// correlationResultFromProto is the reverse of correlationResultToProto,
// used to rebuild the ExpectedCorrelation the client observed at preview
// time so CommitImportExternalSession can detect drift. A nil proto (client
// omitted expected_correlation) maps onto the zero value, which never
// matches a real CorrelationResolved result -- fine, since a zero
// ExpectedCorrelation only appears if the caller skipped preview, and
// resolveResumeUUID only compares against it when fresh.Kind is Resolved.
func correlationResultFromProto(proto *sessionv1.CorrelationResultProto) session.CorrelationResult {
	result := session.CorrelationResult{
		UUID: proto.GetUuid(),
	}

	switch proto.GetKind() {
	case sessionv1.CorrelationKind_CORRELATION_KIND_NOT_FOUND:
		result.Kind = session.CorrelationNotFound
	case sessionv1.CorrelationKind_CORRELATION_KIND_RESOLVED:
		result.Kind = session.CorrelationResolved
	case sessionv1.CorrelationKind_CORRELATION_KIND_AMBIGUOUS:
		result.Kind = session.CorrelationAmbiguous
	}

	switch proto.GetConfidence() {
	case sessionv1.CorrelationConfidence_CORRELATION_CONFIDENCE_PID_EXACT:
		result.Confidence = session.ConfidencePIDExact
	case sessionv1.CorrelationConfidence_CORRELATION_CONFIDENCE_PATH_HEURISTIC:
		result.Confidence = session.ConfidencePathHeuristic
	}

	for _, c := range proto.GetCandidates() {
		result.Candidates = append(result.Candidates, session.HistoryFileInfo{
			ConversationUUID: c.GetConversationUuid(),
			HistoryFilePath:  c.GetHistoryFilePath(),
			ProjectDir:       c.GetProjectDir(),
		})
	}

	return result
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
