package services

// backlog_service_stuck.go — RPC handlers for the stuck-backlog-item read
// surface (ListStuckBacklogItems, SnoozeStuckItem), Phase 3 (Epic 3.1) of the
// backlog-stuck-item-visibility feature. ListStuckBacklogItems is a pure read
// (mirrors backlog_service_query.go patterns); SnoozeStuckItem is a small
// state mutation (mirrors backlog_service_lifecycle.go patterns). Both are
// kept together in this file since they were added as a single RPC surface.

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
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
	default:
		return ""
	}
}

// stuckBacklogItemToProto maps an OpenStuckStateData projection (already
// filtered to open + un-snoozed by FindOpenStuckStates) to its proto
// representation. allow_auto_merge is intentionally left unset here — Phase 4
// (Story 4.1.4) owns fetching and populating the per-repo, TTL-cached value.
func stuckBacklogItemToProto(row session.OpenStuckStateData) *sessionv1.StuckBacklogItem {
	return &sessionv1.StuckBacklogItem{
		ItemId:          row.ItemID,
		Title:           row.ItemTitle,
		Status:          string(row.ItemStatus),
		Reason:          toProtoStuckReason(row.Reason),
		FirstDetectedAt: timestamppb.New(row.FirstDetectedAt),
		LastCheckedAt:   timestamppb.New(row.LastCheckedAt),
		PrNumber:        int32(row.PrNumber),
		PrUrl:           row.PrURL,
		Context:         row.Context,
	}
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
