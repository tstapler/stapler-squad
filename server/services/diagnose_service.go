package services

// diagnose_service.go — Phase 7, Epic 7.1: the ConnectRPC surface for the
// backlog-diagnose-and-nudge feature. DiagnoseBacklogItem delegates dispatch
// to DiagnoseDispatcher (diagnose_dispatcher.go); ListDiagnoseDispatches
// reads history straight from DiagnoseDispatchStore (diagnose_dispatch_store.go).
// Mirrors HandoffSummaryService's shape (handoff_summary_service.go) -- a
// thin proto<->domain translation layer with no business logic of its own.

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/session/diagnose"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time check: DiagnoseService must implement the generated handler.
var _ sessionv1connect.DiagnoseServiceHandler = (*DiagnoseService)(nil)

// DiagnoseService implements the ConnectRPC DiagnoseServiceHandler.
type DiagnoseService struct {
	dispatcher *DiagnoseDispatcher
	store      DiagnoseDispatchStore
}

// NewDiagnoseService constructs a DiagnoseService. dispatcher drives
// DiagnoseBacklogItem; store (typically the same DiagnoseDispatchStore
// dispatcher was built with) backs ListDiagnoseDispatches.
func NewDiagnoseService(dispatcher *DiagnoseDispatcher, store DiagnoseDispatchStore) *DiagnoseService {
	return &DiagnoseService{dispatcher: dispatcher, store: store}
}

// DiagnoseBacklogItem requests a new diagnose dispatch for a backlog item.
// Returns CodeFailedPrecondition when a dispatch is already in flight for
// this item (RequestDiagnosisResult.AlreadyInFlight) -- a genuine dispatch
// that goes on to fail synchronously (DispatchFailed) still returns
// successfully here, since the DiagnoseDispatch row was durably recorded and
// is visible via ListDiagnoseDispatches with outcome_kind "dispatch_failed".
// +api: backlog:diagnose
func (s *DiagnoseService) DiagnoseBacklogItem(
	ctx context.Context,
	req *connect.Request[sessionv1.DiagnoseBacklogItemRequest],
) (*connect.Response[sessionv1.DiagnoseBacklogItemResponse], error) {
	itemID := req.Msg.ItemId
	if itemID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id must not be empty"))
	}

	result, err := s.dispatcher.RequestDiagnosis(ctx, itemID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to request diagnosis for item %s: %w", itemID, err))
	}
	if result.AlreadyInFlight {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("a diagnosis is already in flight for item %s", itemID))
	}

	return connect.NewResponse(&sessionv1.DiagnoseBacklogItemResponse{
		DispatchId:          result.DispatchID,
		DiagnosticSessionId: result.DiagnosticSessionUUID,
	}), nil
}

// ListDiagnoseDispatches returns item_id's diagnose dispatch history,
// chronological oldest-first, straight from DiagnoseDispatchStore -- an
// empty list (not an error) when the item has no dispatches yet. A row still
// Pending (or since marked Stalled by Story 6.1.5's reconciler) is returned
// exactly as stored, which is what makes a page-refresh-mid-dispatch call
// return the same row unchanged: this handler adds no caching or mutation of
// its own.
// +api: backlog:list-diagnose-dispatches
func (s *DiagnoseService) ListDiagnoseDispatches(
	ctx context.Context,
	req *connect.Request[sessionv1.ListDiagnoseDispatchesRequest],
) (*connect.Response[sessionv1.ListDiagnoseDispatchesResponse], error) {
	itemID := req.Msg.ItemId
	if itemID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id must not be empty"))
	}

	records, err := s.store.ListByItem(ctx, itemID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list diagnose dispatches for item %s: %w", itemID, err))
	}

	dispatches := make([]*sessionv1.DiagnoseDispatchProto, len(records))
	for i, record := range records {
		dispatches[i] = toDiagnoseDispatchProto(record)
	}

	return connect.NewResponse(&sessionv1.ListDiagnoseDispatchesResponse{Dispatches: dispatches}), nil
}

// toDiagnoseDispatchProto maps a domain DiagnoseDispatchRecord onto its proto
// representation. Outcome-kind-specific fields (safety_gate_reason,
// bug_item_id, note_text, failure_reason) stay unset when record.Outcome is
// nil -- true for a Pending or Stalled row, matching DiagnoseOutcome's closed
// sum type (session/diagnose/outcome.go).
func toDiagnoseDispatchProto(record DiagnoseDispatchRecord) *sessionv1.DiagnoseDispatchProto {
	p := &sessionv1.DiagnoseDispatchProto{
		Id:                  record.ID,
		ItemId:              record.ItemID,
		TargetSessionUuid:   record.TargetSessionUUID,
		DiagnosticSessionId: record.DiagnosticSessionUUID,
		Status:              toDiagnoseDispatchStatusProto(record.Status),
		CreatedAt:           timestamppb.New(record.CreatedAt),
	}
	if record.CompletedAt != nil {
		p.CompletedAt = timestamppb.New(*record.CompletedAt)
	}
	if record.Outcome != nil {
		outcome := record.Outcome
		kind := string(outcome.Kind)
		p.OutcomeKind = &kind
		if outcome.GateReason != nil {
			reason := string(*outcome.GateReason)
			p.SafetyGateReason = &reason
		}
		p.BugItemId = outcome.BugItemID
		p.NoteText = outcome.NoteText
		p.FailureReason = outcome.FailureReason
	}
	return p
}

// toDiagnoseDispatchStatusProto maps the domain DiagnoseDispatchStatus onto
// the proto enum, defaulting to UNSPECIFIED for an unrecognized value rather
// than panicking (mirrors toHandoffSummaryStatusProto's convention,
// handoff_summary_service.go).
func toDiagnoseDispatchStatusProto(status diagnose.DiagnoseDispatchStatus) sessionv1.DiagnoseDispatchStatus {
	switch status {
	case diagnose.DiagnoseDispatchStatusPending:
		return sessionv1.DiagnoseDispatchStatus_DIAGNOSE_DISPATCH_STATUS_PENDING
	case diagnose.DiagnoseDispatchStatusCompleted:
		return sessionv1.DiagnoseDispatchStatus_DIAGNOSE_DISPATCH_STATUS_COMPLETED
	case diagnose.DiagnoseDispatchStatusStalled:
		return sessionv1.DiagnoseDispatchStatus_DIAGNOSE_DISPATCH_STATUS_STALLED
	default:
		return sessionv1.DiagnoseDispatchStatus_DIAGNOSE_DISPATCH_STATUS_UNSPECIFIED
	}
}
