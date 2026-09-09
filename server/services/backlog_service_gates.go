package services

// backlog_service_gates.go — RecordGateApproval RPC handler (Story 2.4.1 of
// backlog-custom-workflow-stages). Structural pattern mirrors
// backlog_service_stages.go's CRUD handlers: a thin translation layer over
// session.RecordGateApproval, returning connect error codes for the
// nil-repository and duplicate-approval cases.
//
// This file must NOT import session/ent directly (depguard's
// no_ent_in_services rule, .golangci.yml) — session.GateSatisfactionRepository
// hands back a plain session.GateSatisfactionData DTO instead.

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// gateSatisfactionToProto converts a session.GateSatisfactionData to its
// proto representation.
func gateSatisfactionToProto(rec *session.GateSatisfactionData) *sessionv1.GateSatisfactionRecord {
	out := &sessionv1.GateSatisfactionRecord{
		Id:          rec.ID.String(),
		ItemId:      rec.ItemID.String(),
		GateId:      rec.GateID.String(),
		Satisfied:   rec.Satisfied,
		SatisfiedBy: rec.SatisfiedBy,
		CreatedAt:   timestamppb.New(rec.CreatedAt),
	}
	if rec.SatisfiedAt != nil {
		out.SatisfiedAt = timestamppb.New(*rec.SatisfiedAt)
	}
	return out
}

// RecordGateApproval records one explicit human-approval action for a
// human_approval-kind gate (Epic 2.4, Story 2.4.1). A duplicate call for the
// same (item_id, gate_id) pair — the gate was already approved — is rejected
// with connect.CodeAlreadyExists rather than silently succeeding a second
// time, per the schema's UNIQUE(item_id, gate_id) index (defense in depth:
// the one-shot guarantee is enforced at the DB layer, not just by this
// handler's own logic).
// +api: backlog:record-gate-approval
func (s *BacklogService) RecordGateApproval(
	ctx context.Context,
	req *connect.Request[sessionv1.RecordGateApprovalRequest],
) (*connect.Response[sessionv1.RecordGateApprovalResponse], error) {
	if s.gateSatisfactionRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("gate satisfaction storage not available"))
	}

	itemID, err := uuid.Parse(req.Msg.ItemId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid item id: %w", err))
	}
	gateID, err := uuid.Parse(req.Msg.GateId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid gate id: %w", err))
	}

	record, err := session.RecordGateApproval(ctx, s.gateSatisfactionRepo, itemID, gateID, req.Msg.SatisfiedBy)
	if err != nil {
		if errors.Is(err, session.ErrConflict) {
			return nil, connect.NewError(connect.CodeAlreadyExists,
				fmt.Errorf("gate %s for item %s is already satisfied", gateID, itemID))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("record gate approval: %w", err))
	}

	return connect.NewResponse(&sessionv1.RecordGateApprovalResponse{
		Record: gateSatisfactionToProto(record),
	}), nil
}
