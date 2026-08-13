package services

// backlog_service_stuck.go — RPC handlers for the stuck-backlog-item read
// surface (ListStuckBacklogItems, SnoozeStuckItem), Phase 3 (Epic 3.1) of the
// backlog-stuck-item-visibility feature. ListStuckBacklogItems is a pure read
// (mirrors backlog_service_query.go patterns); SnoozeStuckItem is a small
// state mutation (mirrors backlog_service_lifecycle.go patterns). Both are
// kept together in this file since they were added as a single RPC surface.

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// toProtoStuckReason maps a domain.StuckReason to its proto enum value. An
// unknown/invalid reason string maps to STUCK_REASON_UNSPECIFIED rather than
// panicking. MarkStuck only ever writes validated compile-time constants, but
// this mapping stays defensive against any row a future migration or direct
// DB write could introduce.
func toProtoStuckReason(reason domain.StuckReason) sessionv1.StuckReason {
	switch reason {
	case domain.StuckReasonPRReadyUnmerged:
		return sessionv1.StuckReason_STUCK_REASON_PR_READY_UNMERGED
	case domain.StuckReasonReworkCap:
		return sessionv1.StuckReason_STUCK_REASON_REWORK_CAP
	case domain.StuckReasonAbandonedReview:
		return sessionv1.StuckReason_STUCK_REASON_ABANDONED_REVIEW
	case domain.StuckReasonStaleWork:
		return sessionv1.StuckReason_STUCK_REASON_STALE_WORK
	case domain.StuckReasonBouncing:
		return sessionv1.StuckReason_STUCK_REASON_BOUNCING
	case domain.StuckReasonPushFailed:
		return sessionv1.StuckReason_STUCK_REASON_PUSH_FAILED
	case domain.StuckReasonOrphanedTriage:
		return sessionv1.StuckReason_STUCK_REASON_ORPHANED_TRIAGE
	case domain.StuckReasonAutonomousStuck:
		return sessionv1.StuckReason_STUCK_REASON_AUTONOMOUS_STUCK
	case domain.StuckReasonSpawnFailed:
		return sessionv1.StuckReason_STUCK_REASON_SPAWN_FAILED
	case domain.StuckReasonPlanNotApproved:
		return sessionv1.StuckReason_STUCK_REASON_PLAN_NOT_APPROVED
	case domain.StuckReasonPRPendingNoPR:
		return sessionv1.StuckReason_STUCK_REASON_PR_PENDING_NO_PR
	case domain.StuckReasonReworkBlockedStale:
		return sessionv1.StuckReason_STUCK_REASON_REWORK_BLOCKED_STALE
	case domain.StuckReasonPRNeedsFix:
		return sessionv1.StuckReason_STUCK_REASON_PR_NEEDS_FIX
	case domain.StuckReasonRespawnBlockedActive:
		return sessionv1.StuckReason_STUCK_REASON_RESPAWN_BLOCKED_ACTIVE
	case domain.StuckReasonLikelyFlaky:
		return sessionv1.StuckReason_STUCK_REASON_LIKELY_FLAKY
	default:
		return sessionv1.StuckReason_STUCK_REASON_UNSPECIFIED
	}
}

// fromProtoStuckReason maps a proto StuckReason enum value back to the domain
// type for repository calls. STUCK_REASON_UNSPECIFIED and any unrecognized
// value map to the empty StuckReason (""), which fails IsValid() — callers
// must check IsValid() before using the result.
func fromProtoStuckReason(reason sessionv1.StuckReason) domain.StuckReason {
	switch reason {
	case sessionv1.StuckReason_STUCK_REASON_PR_READY_UNMERGED:
		return domain.StuckReasonPRReadyUnmerged
	case sessionv1.StuckReason_STUCK_REASON_REWORK_CAP:
		return domain.StuckReasonReworkCap
	case sessionv1.StuckReason_STUCK_REASON_ABANDONED_REVIEW:
		return domain.StuckReasonAbandonedReview
	case sessionv1.StuckReason_STUCK_REASON_STALE_WORK:
		return domain.StuckReasonStaleWork
	case sessionv1.StuckReason_STUCK_REASON_BOUNCING:
		return domain.StuckReasonBouncing
	case sessionv1.StuckReason_STUCK_REASON_PUSH_FAILED:
		return domain.StuckReasonPushFailed
	case sessionv1.StuckReason_STUCK_REASON_ORPHANED_TRIAGE:
		return domain.StuckReasonOrphanedTriage
	case sessionv1.StuckReason_STUCK_REASON_AUTONOMOUS_STUCK:
		return domain.StuckReasonAutonomousStuck
	case sessionv1.StuckReason_STUCK_REASON_SPAWN_FAILED:
		return domain.StuckReasonSpawnFailed
	case sessionv1.StuckReason_STUCK_REASON_PLAN_NOT_APPROVED:
		return domain.StuckReasonPlanNotApproved
	case sessionv1.StuckReason_STUCK_REASON_PR_PENDING_NO_PR:
		return domain.StuckReasonPRPendingNoPR
	case sessionv1.StuckReason_STUCK_REASON_REWORK_BLOCKED_STALE:
		return domain.StuckReasonReworkBlockedStale
	case sessionv1.StuckReason_STUCK_REASON_PR_NEEDS_FIX:
		return domain.StuckReasonPRNeedsFix
	case sessionv1.StuckReason_STUCK_REASON_RESPAWN_BLOCKED_ACTIVE:
		return domain.StuckReasonRespawnBlockedActive
	case sessionv1.StuckReason_STUCK_REASON_LIKELY_FLAKY:
		return domain.StuckReasonLikelyFlaky
	default:
		return ""
	}
}

// stuckBacklogItemToProto maps an OpenStuckStateData projection (already
// filtered to open + un-snoozed by FindOpenStuckStates) to its proto
// representation. allow_auto_merge is intentionally left unset here — Phase 4
// (Story 4.1.4) owns fetching and populating the per-repo, TTL-cached value.
func stuckBacklogItemToProto(row session.OpenStuckStateData) *sessionv1.StuckBacklogItem {
	item := &sessionv1.StuckBacklogItem{
		ItemId:              row.ItemID,
		Title:               row.ItemTitle,
		Status:              string(row.ItemStatus),
		Reason:              toProtoStuckReason(row.Reason),
		FirstDetectedAt:     timestamppb.New(row.FirstDetectedAt),
		LastCheckedAt:       timestamppb.New(row.LastCheckedAt),
		PrNumber:            int32(row.PrNumber),
		PrUrl:               row.PrURL,
		Context:             row.Context,
		RemediationAttempts: row.RemediationAttempts,
		PlanArtifactsPath:   row.PlanArtifactsPath,
	}
	if row.NextRemediationAt != nil {
		item.NextRemediationAt = timestamppb.New(*row.NextRemediationAt)
	}
	return item
}

// ListStuckBacklogItems returns open (unresolved, un-snoozed) stuck backlog
// items — items that have stopped progressing toward merge, with a reason,
// since-when, and PR context.
// +api: backlog:list-stuck
func (s *BacklogService) ListStuckBacklogItems(
	ctx context.Context,
	_ *connect.Request[sessionv1.ListStuckBacklogItemsRequest],
) (*connect.Response[sessionv1.ListStuckBacklogItemsResponse], error) {
	if s.storage == nil {
		return connect.NewResponse(&sessionv1.ListStuckBacklogItemsResponse{}), nil
	}

	rows, err := s.storage.FindOpenStuckStates(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list stuck backlog items: %w", err))
	}

	items := make([]*sessionv1.StuckBacklogItem, len(rows))
	for i, row := range rows {
		items[i] = stuckBacklogItemToProto(row)
	}

	return connect.NewResponse(&sessionv1.ListStuckBacklogItemsResponse{Items: items}), nil
}

// SnoozeStuckItem suppresses a stuck row from the active view and from
// re-notification until the given time.
// +api: backlog:snooze-stuck
func (s *BacklogService) SnoozeStuckItem(
	ctx context.Context,
	req *connect.Request[sessionv1.SnoozeStuckItemRequest],
) (*connect.Response[sessionv1.SnoozeStuckItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.ItemId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}
	reason := fromProtoStuckReason(req.Msg.Reason)
	if !reason.IsValid() {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid or unspecified reason"))
	}
	if req.Msg.Until == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("until is required"))
	}

	applied, err := s.storage.SnoozeStuckState(ctx, req.Msg.ItemId, reason, req.Msg.Until.AsTime())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to snooze stuck item: %w", err))
	}

	return connect.NewResponse(&sessionv1.SnoozeStuckItemResponse{Applied: applied}), nil
}

// ResetStuckRemediation clears the automated-remediation counters on a
// single open stuck row (docs/tasks/backlog-stuck-item-auto-remediation.md
// Phase A) — the per-item admin escape hatch for an attempt budget spuriously
// consumed by e.g. an OOM-restart storm. Never itself invokes a remediation
// action; it only un-parks the row for the NEXT automated (or
// TriggerRemediationNow-triggered) attempt.
// +api: backlog:reset-stuck-remediation
func (s *BacklogService) ResetStuckRemediation(
	ctx context.Context,
	req *connect.Request[sessionv1.ResetStuckRemediationRequest],
) (*connect.Response[sessionv1.ResetStuckRemediationResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.ItemId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}
	reason := fromProtoStuckReason(req.Msg.Reason)
	if !reason.IsValid() {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid or unspecified reason"))
	}

	applied, err := s.storage.ResetStuckRemediation(ctx, req.Msg.ItemId, reason)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to reset stuck remediation: %w", err))
	}

	return connect.NewResponse(&sessionv1.ResetStuckRemediationResponse{Applied: applied}), nil
}

// BulkResetStuckRemediation applies ResetStuckRemediation's reset to every
// open stuck row matching the optional reason filter — see
// only_parked_explicitly_set's doc comment in the proto for why only_parked
// defaults to true (the safer, more targeted reset) rather than proto3's
// natural false zero value.
// +api: backlog:bulk-reset-stuck-remediation
func (s *BacklogService) BulkResetStuckRemediation(
	ctx context.Context,
	req *connect.Request[sessionv1.BulkResetStuckRemediationRequest],
) (*connect.Response[sessionv1.BulkResetStuckRemediationResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	var reasonFilter *domain.StuckReason
	if req.Msg.Reason != sessionv1.StuckReason_STUCK_REASON_UNSPECIFIED {
		reason := fromProtoStuckReason(req.Msg.Reason)
		if !reason.IsValid() {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid reason"))
		}
		reasonFilter = &reason
	}

	onlyParked := true
	if req.Msg.OnlyParkedExplicitlySet {
		onlyParked = req.Msg.OnlyParked
	}

	n, err := s.storage.BulkResetStuckRemediation(ctx, reasonFilter, onlyParked)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to bulk reset stuck remediation: %w", err))
	}

	return connect.NewResponse(&sessionv1.BulkResetStuckRemediationResponse{ResetCount: int32(n)}), nil
}

// remediationActionByReason maps a stuck reason to the BacklogService method
// TriggerRemediationNow invokes directly (bypassing the interface
// indirection the periodic sweep uses — this RPC handler IS a *BacklogService,
// so it can call its own remediation methods without going through
// AutoReopenSpawner/ReviewRespawner/AutonomousStuckRespawner/
// StaleWorkRemediator). Wired reasons are the ones with a working respawn
// action as of this Epic; every other reason returns
// connect.CodeUnimplemented until a future phase adds it here.
func (s *BacklogService) remediationActionByReason(reason domain.StuckReason) func(ctx context.Context, itemID string) error {
	switch reason {
	case domain.StuckReasonBouncing:
		return s.AutoReopenAfterFailedReview
	case domain.StuckReasonAbandonedReview:
		return s.AutoRespawnReview
	case domain.StuckReasonAutonomousStuck:
		return s.AutoRespawnAutonomousWork
	case domain.StuckReasonStaleWork:
		return s.RemediateStaleWorkSession
	case domain.StuckReasonOrphanedTriage:
		return s.AutoRespawnTriage
	case domain.StuckReasonPRPendingNoPR:
		return func(ctx context.Context, itemID string) error {
			return s.AutoReopenForPRFix(ctx, itemID, "Manually triggered reopen — item was stuck in pr_pending with no PR reference (BUG-040)")
		}
	case domain.StuckReasonPRNeedsFix:
		return func(ctx context.Context, itemID string) error {
			return s.AutoReopenForPRFix(ctx, itemID, "Manually triggered PR fix retry — PR has failing CI, blocking reviews, or a merge conflict")
		}
	default:
		return nil
	}
}

// TriggerRemediationNow immediately runs the reason-specific remediation
// action for a single open stuck row (docs/tasks/backlog-stuck-item-auto-remediation.md
// addendum) — the operator "Retry now" escape hatch. Bypasses only the
// next_remediation_at backoff timer: RecordManualRemediationAttempt still
// rejects a parked row (ErrRemediationParked) rather than un-parking it, and
// this attempt still increments remediation_attempts exactly like a normal
// dispatcher-triggered one, so it counts toward the same 5-attempt cap. The
// wrapped action's own circuit breaker (IsRepeatedFailure/
// IsRepeatedNoVerdictFailure inside AutoReopenAfterFailedReview, for example)
// still applies — this RPC does not bypass it.
// +api: backlog:trigger-remediation-now
func (s *BacklogService) TriggerRemediationNow(
	ctx context.Context,
	req *connect.Request[sessionv1.TriggerRemediationNowRequest],
) (*connect.Response[sessionv1.TriggerRemediationNowResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.ItemId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}
	reason := fromProtoStuckReason(req.Msg.Reason)
	if !reason.IsValid() {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid or unspecified reason"))
	}

	action := s.remediationActionByReason(reason)
	if action == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("no automated remediation action is available for reason %q yet", reason))
	}

	justParked, err := s.storage.RecordManualRemediationAttempt(ctx, req.Msg.ItemId, reason)
	if err != nil {
		switch {
		case errors.Is(err, session.ErrRemediationParked):
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("remediation attempts are exhausted for this item/reason — call ResetStuckRemediation first: %w", err))
		case errors.Is(err, session.ErrNoOpenStuckState):
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("no open stuck state for this item/reason: %w", err))
		default:
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to record manual remediation attempt: %w", err))
		}
	}

	if actionErr := action(ctx, req.Msg.ItemId); actionErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("remediation action failed: %w", actionErr))
	}

	if justParked {
		log.InfoLog.Printf("[BacklogService] TriggerRemediationNow item=%s reason=%s: this was the final attempt before parking", req.Msg.ItemId, reason)
	}

	return connect.NewResponse(&sessionv1.TriggerRemediationNowResponse{Triggered: true}), nil
}
