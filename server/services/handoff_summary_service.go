package services

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/session"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time check: HandoffSummaryService must implement the generated handler.
var _ sessionv1connect.HandoffSummaryServiceHandler = (*HandoffSummaryService)(nil)

// HandoffSummaryService implements the ConnectRPC HandoffSummaryServiceHandler.
// Reads/writes the HandoffSummary ent table via generator.FindRowBySessionID,
// mirroring SessionSummaryService's shape (session_summary_service.go) — ent
// access stays confined to the session package (.golangci.yml's
// no_ent_in_services/forbidigo rules), translated to the session.ErrNotFound
// sentinel at the boundary.
type HandoffSummaryService struct {
	generator *session.HandoffSummaryGenerator
}

// NewHandoffSummaryService creates a HandoffSummaryService.
func NewHandoffSummaryService(generator *session.HandoffSummaryGenerator) *HandoffSummaryService {
	return &HandoffSummaryService{generator: generator}
}

// GetHandoffSummary returns the current handoff summary for a session, if one
// exists. The response's summary field is unset (nil) when no row exists yet,
// which is a valid state, not an error.
//
// GetHandoffSummary is a read RPC that may perform a side-effecting write: on
// a stale GENERATING row (see HandoffSummaryGenerator.ReconcileStaleness), it
// upserts the row to ERROR before returning it, as a lazy restart-recovery
// mechanism — mirrors SessionSummaryService.GetSessionSummary's identical
// pattern; see that method's doc comment for why a caller polling this every
// few seconds can otherwise observe GENERATING forever.
// +api: handoff-summary:get
func (s *HandoffSummaryService) GetHandoffSummary(
	ctx context.Context,
	req *connect.Request[sessionv1.GetHandoffSummaryRequest],
) (*connect.Response[sessionv1.GetHandoffSummaryResponse], error) {
	row, err := s.generator.FindRowBySessionID(ctx, req.Msg.SessionId)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return connect.NewResponse(&sessionv1.GetHandoffSummaryResponse{Summary: nil}), nil
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query handoff summary: %w", err))
	}

	row = s.generator.ReconcileStaleness(ctx, row)

	return connect.NewResponse(&sessionv1.GetHandoffSummaryResponse{Summary: toHandoffSummaryProto(row)}), nil
}

// TriggerHandoffSummary triggers generation of a session's handoff summary and
// returns the resulting summary. Generation runs asynchronously — this method
// dispatches it and returns immediately with the current (possibly
// still-generating) row; the client is expected to poll GetHandoffSummary
// until status leaves GENERATING. The dedup guard inside
// HandoffSummaryGenerator.GenerateAndPersist (not this handler) is what
// prevents a second overlapping pipeline when one is already in flight for
// this session.
// +api: handoff-summary:trigger
func (s *HandoffSummaryService) TriggerHandoffSummary(
	ctx context.Context,
	req *connect.Request[sessionv1.TriggerHandoffSummaryRequest],
) (*connect.Response[sessionv1.TriggerHandoffSummaryResponse], error) {
	if !config.LoadConfig().HandoffSummary.EnabledOrDefault() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("handoff summary generation is disabled"))
	}

	sessionID := req.Msg.SessionId

	row, err := s.generator.FindRowBySessionID(ctx, sessionID)
	if err != nil && !errors.Is(err, session.ErrNotFound) {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query handoff summary: %w", err))
	}

	// Resolve a display title for the generator: this service has no direct
	// access to live-instance/storage session metadata (unlike
	// SessionSummaryService, which is handed a liveInstanceFinder), so fall
	// back to a prior persisted row's title (set by an earlier generation for
	// this session) and, failing that, to the session_id itself — a session_id
	// often *is* the session's title in this codebase's ID scheme (see
	// SessionService.resolveSessionTitle). sourceSessionTitle is a display
	// value threaded into the LLM prompt, not a correctness-critical field.
	sessionTitle := sessionID
	if row != nil && row.SessionTitle != "" {
		sessionTitle = row.SessionTitle
	}

	// Critical: detached goroutine with context.Background(), never the
	// request-scoped ctx — ConnectRPC cancels ctx the moment this handler
	// returns, but the pipeline (including its own LLM-call timeout) must keep
	// running after that. Mirrors
	// SessionSummaryService.RegenerateSessionSummary's identical dispatch shape.
	go s.generator.GenerateAndPersist(context.Background(), sessionID, sessionTitle)

	// Look up the row again: GenerateAndPersist's interim upsert should have
	// already written it as GENERATING by the time the goroutine above starts
	// running. There is a narrow race where this read lands before that
	// goroutine has been scheduled at all — in that case FindRowBySessionID
	// returns not-found, so synthesize a PENDING response rather than erroring,
	// since the row is about to exist.
	current, err := s.generator.FindRowBySessionID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return connect.NewResponse(&sessionv1.TriggerHandoffSummaryResponse{
				Summary: &sessionv1.HandoffSummaryProto{
					SessionId:    sessionID,
					SessionTitle: sessionTitle,
					Status:       sessionv1.HandoffSummaryStatus_HANDOFF_SUMMARY_STATUS_PENDING,
				},
			}), nil
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query handoff summary: %w", err))
	}

	return connect.NewResponse(&sessionv1.TriggerHandoffSummaryResponse{Summary: toHandoffSummaryProto(current)}), nil
}

// toHandoffSummaryProto maps a *session.HandoffSummary domain row to its
// proto representation. Returns nil for a nil row (mirrors
// GetHandoffSummaryResponse.Summary's "unset when no row exists" contract).
func toHandoffSummaryProto(row *session.HandoffSummary) *sessionv1.HandoffSummaryProto {
	if row == nil {
		return nil
	}

	p := &sessionv1.HandoffSummaryProto{
		SessionId:                row.SessionID,
		SessionTitle:             row.SessionTitle,
		Status:                   toHandoffSummaryStatusProto(row.Status),
		ActiveTask:               row.ActiveTask,
		SummaryText:              row.SummaryText,
		MiddleMessagesSummarized: int32(row.MiddleMessagesSummarized),
		ErrorMessage:             row.ErrorMessage,
		ErrorStage:               row.ErrorStage,
	}
	if row.GeneratedAt != nil {
		p.GeneratedAt = timestamppb.New(*row.GeneratedAt)
	}
	if row.GenerationStartedAt != nil {
		p.GenerationStartedAt = timestamppb.New(*row.GenerationStartedAt)
	}
	return p
}

// toHandoffSummaryStatusProto maps the ent row's plain-string status column
// onto the proto enum, defaulting to UNSPECIFIED for an unrecognized value
// rather than panicking (defensive against a future status value the proto
// hasn't caught up to yet).
func toHandoffSummaryStatusProto(status string) sessionv1.HandoffSummaryStatus {
	switch session.HandoffSummaryStatus(status) {
	case session.HandoffSummaryStatusPending:
		return sessionv1.HandoffSummaryStatus_HANDOFF_SUMMARY_STATUS_PENDING
	case session.HandoffSummaryStatusGenerating:
		return sessionv1.HandoffSummaryStatus_HANDOFF_SUMMARY_STATUS_GENERATING
	case session.HandoffSummaryStatusReady:
		return sessionv1.HandoffSummaryStatus_HANDOFF_SUMMARY_STATUS_READY
	case session.HandoffSummaryStatusError:
		return sessionv1.HandoffSummaryStatus_HANDOFF_SUMMARY_STATUS_ERROR
	default:
		return sessionv1.HandoffSummaryStatus_HANDOFF_SUMMARY_STATUS_UNSPECIFIED
	}
}
