package services

// backlog_service_jules.go — Epic 2.4.3: the DispatchToJules Connect
// handler. A thin wrapper around JulesDispatchService (jules_dispatch_service.go,
// Epic 2.2) — matching how TriggerShipPR (backlog_service_ship.go) sits
// beside its own guard-clause logic rather than living inside it — so this
// file adds no new guard logic of its own, only request validation and
// sentinel-to-Connect-code classification. See
// project_plans/google-jules-integration/implementation/plan.md's Epic 2.4
// for the full story/task breakdown this file implements.

import (
	"context"
	"errors"
	"fmt"

	connect "connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/jules"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// SetJulesDispatcher wires the Jules dispatch backend used by DispatchToJules.
// Called post-construction from server/dependencies.go once JulesDispatchService
// is available (Task 2.4.4a) — nil (the default) makes DispatchToJules return
// CodeFailedPrecondition, since a nil dispatcher and a disabled/unconfigured
// Jules feature look identical from the RPC's perspective.
func (s *BacklogService) SetJulesDispatcher(d JulesDispatcher) {
	s.julesDispatcher = d
}

// julesUnconfiguredMessage is returned verbatim (wrapped in
// CodeFailedPrecondition) both when julesDispatcher is nil (feature disabled
// or its dependencies unresolvable at startup, Task 2.4.4a) and when the
// dispatcher itself reports ErrJulesNotConfigured — the two are
// indistinguishable to a caller and should read the same way.
const julesUnconfiguredMessage = "jules dispatch is not enabled — configure it in Settings → Jules"

// DispatchToJules dispatches a ready backlog item to Google Jules — the one
// guarded entry point the web UI (and later an MCP tool) uses. See
// JulesDispatchService.DispatchToJules for the full guard sequence
// (in-flight -> persisted-duplicate -> egress consent -> spend caps ->
// blockers -> reserve/create/confirm); this handler only validates the
// request shape and classifies the result into a Connect code.
// +api: backlog:dispatch-to-jules
func (s *BacklogService) DispatchToJules(
	ctx context.Context,
	req *connect.Request[sessionv1.DispatchToJulesRequest],
) (*connect.Response[sessionv1.DispatchToJulesResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.ItemId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}
	if req.Msg.Branch == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("branch is required — Jules can only start a session from a branch already pushed to GitHub"))
	}

	julesReq, err := NewJulesDispatchRequest(req.Msg.ItemId, req.Msg.Branch, req.Msg.Prompt)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if s.julesDispatcher == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("%s", julesUnconfiguredMessage))
	}

	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	sessionName, dispatchErr := s.julesDispatcher.DispatchToJules(ctx, item, julesReq)
	if dispatchErr != nil {
		return nil, classifyJulesDispatchError(dispatchErr)
	}

	sessionUUID := julesSessionUUIDPrefix + string(sessionName)
	summary, err := s.storage.GetItemSessionBySessionUUID(ctx, sessionUUID)
	if err != nil {
		// The dispatch itself already succeeded (item transitioned, billed
		// session created) — a failure to re-read it back is a read-side
		// problem, not a dispatch failure, so log and still report success
		// with what we know rather than erroring an already-successful call.
		log.WarningLog().Printf("[DispatchToJules] item=%s session=%s: failed to read back item session: %v", item.ID, sessionName, err)
		return connect.NewResponse(&sessionv1.DispatchToJulesResponse{
			ItemSession: &sessionv1.ItemSession{SessionUuid: sessionUUID, SessionRole: string(session.SessionRoleJulesWork)},
		}), nil
	}

	return connect.NewResponse(&sessionv1.DispatchToJulesResponse{
		ItemSession: itemSessionToProto(summary, nil),
	}), nil
}

// classifyJulesDispatchError maps a JulesDispatchService.DispatchToJules
// error into the Connect code the UI needs to react correctly: guard
// rejections (in-flight, unregistered source, spend caps, unresolved
// blockers, egress not acknowledged, not configured) as FailedPrecondition
// so the UI can show the reason inline; a transient Jules API failure as
// Unavailable; anything else as Internal.
func classifyJulesDispatchError(err error) error {
	// checkSpendGuards (jules_dispatch_service.go) already returns a
	// *connect.Error with the correct code — pass it through unchanged
	// rather than re-wrapping it in another layer of "failed_precondition:".
	var connectErr *connect.Error
	if errors.As(err, &connectErr) {
		return connectErr
	}

	switch {
	case errors.Is(err, jules.ErrJulesNotConfigured):
		return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("%s: %w", julesUnconfiguredMessage, err))
	case errors.Is(err, jules.ErrJulesSourceNotRegistered),
		errors.Is(err, ErrJulesDispatchInFlight),
		errors.Is(err, session.ErrUnresolvedBlockers):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, jules.ErrJulesRateLimited), errors.Is(err, jules.ErrJulesTransient):
		return connect.NewError(connect.CodeUnavailable, err)
	case errors.Is(err, ErrJulesEgressNotAcknowledged):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}
