package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time check: SessionSummaryService must implement the generated handler.
var _ sessionv1connect.SessionSummaryServiceHandler = (*SessionSummaryService)(nil)

// liveInstanceFinder is the narrow interface RegenerateSessionSummary needs to check
// whether a session is still live, so it can refresh diff/goal data straight from the
// running Instance rather than stale persisted fields. Defined here, next to its
// consumer, per the `interface-pollution-checklist` skill — *SessionService
// satisfies this structurally, and a trivial fake can stand in for tests without
// constructing a full SessionService.
type liveInstanceFinder interface {
	FindLiveInstance(sessionID string) *session.Instance
}

// SessionSummaryService implements the ConnectRPC SessionSummaryServiceHandler.
// Reads/writes the SessionSummary ent table directly, via generator.FindRowBySessionID
// (never via SessionService's live-instance machinery), so a summary remains
// retrievable after its Session row is gone (AC-3). Queries are delegated to
// SessionSummaryGenerator rather than issuing them here because server/services must
// not import session/ent's query/error-handling helpers directly (.golangci.yml's
// no_ent_in_services/forbidigo rules) — ent access stays confined to the session
// package, translated to the session.ErrNotFound sentinel at the boundary.
type SessionSummaryService struct {
	generator *session.SessionSummaryGenerator
	instances liveInstanceFinder
}

// NewSessionSummaryService creates a SessionSummaryService.
func NewSessionSummaryService(generator *session.SessionSummaryGenerator, instances liveInstanceFinder) *SessionSummaryService {
	return &SessionSummaryService{generator: generator, instances: instances}
}

// GetSessionSummary returns the current summary for a session, if one exists. The
// response's summary field is unset (nil) when no row exists yet — e.g. the session
// is still running, or was never eligible for summary generation — which is a valid
// state, not an error.
//
// GetSessionSummary is a read RPC that may perform a side-effecting write: on a
// stale GENERATING row (see SessionSummaryGenerator.ReconcileStaleness), it upserts
// the row to ERROR before returning it, as a lazy restart-recovery mechanism
// (deliberately chosen over a background sweep — see plan.md's Pattern Decisions
// table). A caller polling this every 2s otherwise has no way to discover from the
// method's name/contract alone that it can mutate state.
// +api: GetSessionSummary
func (s *SessionSummaryService) GetSessionSummary(
	ctx context.Context,
	req *connect.Request[sessionv1.GetSessionSummaryRequest],
) (*connect.Response[sessionv1.GetSessionSummaryResponse], error) {
	row, err := s.generator.FindRowBySessionID(ctx, req.Msg.SessionId)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return connect.NewResponse(&sessionv1.GetSessionSummaryResponse{Summary: nil}), nil
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query session summary: %w", err))
	}

	row = s.generator.ReconcileStaleness(ctx, row)

	return connect.NewResponse(&sessionv1.GetSessionSummaryResponse{Summary: toSessionSummaryProto(row)}), nil
}

// RegenerateSessionSummary triggers regeneration of a session's summary and returns
// the resulting summary. The regeneration pipeline runs asynchronously — this method
// dispatches it and returns the current (possibly stale/still-generating) row
// immediately rather than waiting for the pipeline to finish; the client is expected
// to poll GetSessionSummary until status leaves GENERATING (Story 3.1.1).
//
// The dedup guard inside SessionSummaryGenerator.GenerateAndPersist (not this
// handler) is what prevents a second overlapping pipeline when one is already
// in flight for this session (AC-8) — this handler always dispatches unconditionally
// and lets that guard reject the duplicate.
// +api: RegenerateSessionSummary
func (s *SessionSummaryService) RegenerateSessionSummary(
	ctx context.Context,
	req *connect.Request[sessionv1.RegenerateSessionSummaryRequest],
) (*connect.Response[sessionv1.RegenerateSessionSummaryResponse], error) {
	sessionID := req.Msg.SessionId

	row, err := s.generator.FindRowBySessionID(ctx, sessionID)
	if err != nil {
		if !errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query session summary: %w", err))
		}
		row = nil // no prior row — this is the first-ever generation for this session
	}

	var (
		sessionTitle string
		createdAt    time.Time
		diffSnapshot session.DiffSnapshot
		diffContent  string
		sessionGoal  *session.SessionGoalData
	)

	var liveInst *session.Instance
	if s.instances != nil {
		liveInst = s.instances.FindLiveInstance(sessionID)
	}

	switch {
	case liveInst != nil:
		// Live instance still around — refresh diff snapshot and session goal the
		// same way sessionSummaryListener.OnLifecycleEvent does.
		sessionTitle = liveInst.Title
		createdAt = liveInst.CreatedAt
		diffStats := liveInst.GetDiffStats()
		diffSnapshot = session.BuildDiffSnapshot(diffStats)
		if diffStats != nil {
			diffContent = diffStats.Content
		}
		sessionGoal = liveInst.GetSessionGoal()
	case row != nil:
		// No live instance — fall back to the previously-persisted fields. No live
		// instance to read the goal from, so it stays nil (the persisted row doesn't
		// store the goal itself, only its effect on a prior narrative).
		//
		// diffSnapshot is built directly from the row's three persisted diff
		// columns rather than via session.BuildDiffSnapshot(&git.DiffStats{...}):
		// the raw diff Content isn't persisted (and shouldn't be re-fetched from a
		// possibly-gone worktree), so deriving FilesChanged from an empty Content
		// string would silently zero it even though DiffAdded/DiffRemoved are
		// correct. diffContent stays "" — there is no diff text to forward into
		// the LLM narrative prompt on this path.
		sessionTitle = row.SessionTitle
		if row.SessionStartedAt != nil {
			createdAt = *row.SessionStartedAt
		}
		diffSnapshot = session.DiffSnapshot{
			FilesChanged: row.DiffFilesChanged,
			Added:        row.DiffAdded,
			Removed:      row.DiffRemoved,
		}
	default:
		// Neither a live instance nor a persisted row — nothing to regenerate from.
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no session summary or live session found for %q", sessionID))
	}

	// Critical: detached goroutine with context.Background(), never the
	// request-scoped ctx — ConnectRPC cancels ctx the moment this handler returns,
	// but the pipeline (including its own LLM-call timeout) must keep running after
	// that. See session/session_summary_listener.go's identical dispatch shape.
	go s.generator.GenerateAndPersist(context.Background(), sessionID, sessionTitle, createdAt, diffSnapshot, diffContent, sessionGoal, "manual-regenerate")

	if row == nil {
		// Nothing persisted yet — synthesize a minimal PENDING summary so the client
		// has something to poll against until the goroutine's first write lands.
		return connect.NewResponse(&sessionv1.RegenerateSessionSummaryResponse{
			Summary: &sessionv1.SessionSummaryProto{
				SessionId:    sessionID,
				SessionTitle: sessionTitle,
				Status:       sessionv1.SessionSummaryStatus_SESSION_SUMMARY_STATUS_PENDING,
			},
		}), nil
	}

	return connect.NewResponse(&sessionv1.RegenerateSessionSummaryResponse{Summary: toSessionSummaryProto(row)}), nil
}

// toSessionSummaryProto maps a *ent.SessionSummary row to its proto representation.
// Returns nil for a nil row (mirrors GetSessionSummaryResponse.Summary's "unset when
// no row exists" contract).
func toSessionSummaryProto(row *ent.SessionSummary) *sessionv1.SessionSummaryProto {
	if row == nil {
		return nil
	}

	p := &sessionv1.SessionSummaryProto{
		SessionId:             row.SessionID,
		SessionTitle:          row.SessionTitle,
		Status:                toSessionSummaryStatusProto(row.Status),
		Narrative:             row.Narrative,
		NarrativeFallbackUsed: row.NarrativeFallbackUsed,
		Diff: &sessionv1.SessionSummaryProto_Diff{
			FilesChanged: int32(row.DiffFilesChanged), //#nosec G115 -- changed-file count, bounded well under int32 max
			Added:        int32(row.DiffAdded),        //#nosec G115 -- diff line count, bounded well under int32 max
			Removed:      int32(row.DiffRemoved),      //#nosec G115 -- diff line count, bounded well under int32 max
		},
		Decisions: &sessionv1.SessionSummaryProto_Decisions{
			AutoApproved:        int32(row.DecisionsAutoApproved),        //#nosec G115 -- per-session decision count, always small
			ManuallyApproved:    int32(row.DecisionsManuallyApproved),    //#nosec G115 -- per-session decision count, always small
			Denied:              int32(row.DecisionsDenied),              //#nosec G115 -- per-session decision count, always small
			ReviewQueueResolved: int32(row.DecisionsReviewQueueResolved), //#nosec G115 -- per-session decision count, always small
			StillOpen:           int32(row.DecisionsStillOpen),           //#nosec G115 -- per-session decision count, always small
		},
		Timeline: &sessionv1.SessionSummaryProto_Timeline{},
		Cost: &sessionv1.SessionSummaryProto_Cost{
			DataUnavailable: row.CostDataUnavailable,
		},
		Markdown:     row.Markdown,
		ErrorMessage: row.ErrorMessage,
		ErrorStage:   row.ErrorStage,
	}
	if row.SessionStartedAt != nil {
		p.Timeline.StartedAt = timestamppb.New(*row.SessionStartedAt)
	}
	if row.SessionStoppedAt != nil {
		p.Timeline.StoppedAt = timestamppb.New(*row.SessionStoppedAt)
	}
	if row.DurationMs != nil {
		p.Timeline.DurationMs = *row.DurationMs
	}
	if row.TotalTokens != nil {
		p.Cost.TotalTokens = *row.TotalTokens
	}
	if row.EstimatedCostUsd != nil {
		p.Cost.EstimatedCostUsd = *row.EstimatedCostUsd
	}
	if row.GeneratedAt != nil {
		p.GeneratedAt = timestamppb.New(*row.GeneratedAt)
	}
	return p
}

// toSessionSummaryStatusProto maps the ent row's plain-string status column onto the
// proto enum, defaulting to UNSPECIFIED for an unrecognized value rather than
// panicking (defensive against a future status value the proto hasn't caught up to
// yet).
func toSessionSummaryStatusProto(status string) sessionv1.SessionSummaryStatus {
	switch session.SessionSummaryStatus(status) {
	case session.SessionSummaryStatusPending:
		return sessionv1.SessionSummaryStatus_SESSION_SUMMARY_STATUS_PENDING
	case session.SessionSummaryStatusGenerating:
		return sessionv1.SessionSummaryStatus_SESSION_SUMMARY_STATUS_GENERATING
	case session.SessionSummaryStatusReady:
		return sessionv1.SessionSummaryStatus_SESSION_SUMMARY_STATUS_READY
	case session.SessionSummaryStatusError:
		return sessionv1.SessionSummaryStatus_SESSION_SUMMARY_STATUS_ERROR
	default:
		return sessionv1.SessionSummaryStatus_SESSION_SUMMARY_STATUS_UNSPECIFIED
	}
}
