package services

// backlog_service_transitions.go — CRUD RPC handlers for StageTransition and
// TransitionGate (Story 2.7.2 of backlog-custom-workflow-stages).
//
// CreateStageTransition/UpdateStageTransition wrap their validate-then-persist
// sequence in a single ent transaction via session.StageCRUDRepository.WithTx
// (Task 2.7.2h), re-running Epic 2.6's graph validator against the
// transaction's own read view — never a pre-transaction snapshot — so a
// concurrent commit between two callers can't defeat the validator's
// guarantee (architecture-review Concern 4 / TOCTOU).
//
// This file must NOT import session/ent directly (depguard's
// no_ent_in_services rule) — session.StageCRUDRepository/TransitionTxRepository
// hand back plain session.TransitionData/GateData DTOs instead.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
)

// stageTransitionToProto converts a session.TransitionData to its proto
// representation, including its attached gates.
func stageTransitionToProto(td *session.TransitionData) *sessionv1.StageTransition {
	gates := make([]*sessionv1.TransitionGate, len(td.Gates))
	for i := range td.Gates {
		gates[i] = transitionGateToProto(&td.Gates[i])
	}
	return &sessionv1.StageTransition{
		Id:            td.ID.String(),
		FromStageSlug: td.FromStageSlug,
		ToStageSlug:   td.ToStageSlug,
		Enabled:       td.Enabled,
		Gates:         gates,
		CreatedAt:     timestamppb.New(td.CreatedAt),
		UpdatedAt:     timestamppb.New(td.UpdatedAt),
	}
}

// transitionGateToProto converts a session.GateData to its proto representation.
func transitionGateToProto(g *session.GateData) *sessionv1.TransitionGate {
	return &sessionv1.TransitionGate{
		Id:           g.ID.String(),
		TransitionId: g.TransitionID.String(),
		Kind:         g.Kind,
		Config:       configMapToProto(g.Config),
		Stateful:     g.Stateful,
		OrderIndex:   int32(g.OrderIndex),
		Enabled:      g.Enabled,
		CreatedAt:    timestamppb.New(g.CreatedAt),
		UpdatedAt:    timestamppb.New(g.UpdatedAt),
	}
}

// configMapToProto stringifies a persisted gate config's values for the
// wire's map<string,string> shape. Every value this package itself ever
// writes (via configMapFromProto below) is already a string; a non-string
// value can only appear if something else wrote the row directly, so this
// stringifies defensively rather than dropping the key.
func configMapToProto(m map[string]interface{}) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
			continue
		}
		out[k] = fmt.Sprintf("%v", v)
	}
	return out
}

// configMapFromProto widens a wire map<string,string> into the
// map[string]interface{} shape session.GateCreateInput/GateUpdateInput and
// the ent JSON column expect.
func configMapFromProto(m map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// validateGateConfig marshals a wire config map to JSON and runs it through
// session.ParseGateConfig (Task 2.7.2g3) — the save-time structural boundary
// that rejects an unrecognized key or (for a "custom" gate) an unregistered
// skill before anything is persisted.
func validateGateConfig(kind string, config map[string]string) error {
	raw, err := json.Marshal(configMapFromProto(config))
	if err != nil {
		return fmt.Errorf("marshal gate config: %w", err)
	}
	if _, err := session.ParseGateConfig(session.GateKind(kind), raw); err != nil {
		return err
	}
	return nil
}

// mapTransitionGraphError translates graph_validator.go's sentinel errors
// (and the repository's ErrNotFound/ErrConflict) into the connect.Code an
// RPC caller should see.
func mapTransitionGraphError(err error) error {
	switch {
	case errors.Is(err, session.ErrNotFound):
		return connect.NewError(connect.CodeNotFound, err)
	case errors.Is(err, session.ErrConflict):
		return connect.NewError(connect.CodeAlreadyExists, err)
	case errors.Is(err, session.ErrStageDeadEnd),
		errors.Is(err, session.ErrStageUnreachable):
		return connect.NewError(connect.CodeInvalidArgument, err)
	case errors.Is(err, session.ErrDisableWouldStrandLiveItems):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	default:
		return connect.NewError(connect.CodeInternal, err)
	}
}

// CreateStageTransition adds a new (from_stage, to_stage) edge. Task 2.7.2h1:
// the insert and Epic 2.6's ValidateGraph re-check both run inside one ent
// transaction, against that transaction's own read view.
// +api: backlog:create-stage-transition
func (s *BacklogService) CreateStageTransition(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateStageTransitionRequest],
) (*connect.Response[sessionv1.CreateStageTransitionResponse], error) {
	if s.stageCRUDRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("stage storage not available"))
	}

	var created *session.TransitionData
	var warnings []string

	txErr := s.stageCRUDRepo.WithTx(ctx, func(tx session.TransitionTxRepository) error {
		var err error
		created, err = tx.CreateTransition(ctx, session.TransitionCreateInput{
			FromStageSlug: req.Msg.FromStageSlug,
			ToStageSlug:   req.Msg.ToStageSlug,
			Enabled:       req.Msg.Enabled,
		})
		if err != nil {
			return err
		}

		stages, transitions, err := tx.ListGraphForValidation(ctx)
		if err != nil {
			return fmt.Errorf("load graph for validation: %w", err)
		}
		w, verr := session.ValidateGraph(stages, transitions)
		if verr != nil {
			return verr
		}
		warnings = w
		return nil
	})
	if txErr != nil {
		return nil, mapTransitionGraphError(fmt.Errorf("create stage transition: %w", txErr))
	}

	s.invalidateStageConfigCache(ctx, created.ID.String())

	return connect.NewResponse(&sessionv1.CreateStageTransitionResponse{
		Item:     stageTransitionToProto(created),
		Warnings: warnings,
	}), nil
}

// UpdateStageTransition modifies an existing transition's enabled flag.
// Disabling an edge additionally invokes Task 2.6.1e's live-item-aware
// disable check (Task 2.7.2e) before persisting — distinct from
// CreateStageTransition's unconditional ValidateGraph re-check: per Story
// 2.6.1's "disabling any edge for a stage with zero live items is always
// allowed" acceptance criterion, a disable that would otherwise dead-end a
// stage is not blocked when nothing is currently sitting on it, so
// ValidateGraph's dead-end/reachability errors are surfaced here only as
// non-blocking warnings, never as a rejection. Both the disable check and
// the persist run inside one ent transaction (Task 2.7.2h2).
// +api: backlog:update-stage-transition
func (s *BacklogService) UpdateStageTransition(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateStageTransitionRequest],
) (*connect.Response[sessionv1.UpdateStageTransitionResponse], error) {
	if s.stageCRUDRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("stage storage not available"))
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid transition id: %w", err))
	}

	existing, err := s.stageCRUDRepo.GetTransition(ctx, id)
	if err != nil {
		return nil, mapTransitionGraphError(fmt.Errorf("get stage transition: %w", err))
	}

	var updated *session.TransitionData
	var warnings []string

	txErr := s.stageCRUDRepo.WithTx(ctx, func(tx session.TransitionTxRepository) error {
		isDisabling := req.Msg.Enabled != nil && !*req.Msg.Enabled && existing.Enabled
		if isDisabling {
			_, transitions, err := tx.ListGraphForValidation(ctx)
			if err != nil {
				return fmt.Errorf("load graph for validation: %w", err)
			}
			// candidateEdges must reflect the graph as it would exist AFTER
			// the proposed disable (ValidateDisableTransition's contract) —
			// the target edge is still enabled in tx's current read view, so
			// flip it here rather than in storage.
			candidateEdges := make([]session.TransitionDefinition, len(transitions))
			for i, t := range transitions {
				if t.FromSlug == existing.FromStageSlug && t.ToSlug == existing.ToStageSlug {
					t.Enabled = false
				}
				candidateEdges[i] = t
			}
			liveCount, err := tx.LiveItemCountForStage(ctx, existing.FromStageSlug)
			if err != nil {
				return fmt.Errorf("count live items for stage %q: %w", existing.FromStageSlug, err)
			}
			if err := session.ValidateDisableTransition(
				candidateEdges, existing.FromStageSlug, map[string]int{existing.FromStageSlug: liveCount},
			); err != nil {
				return err
			}
		}

		var updateErr error
		updated, updateErr = tx.UpdateTransition(ctx, id, session.TransitionUpdateInput{Enabled: req.Msg.Enabled})
		if updateErr != nil {
			return updateErr
		}

		stages, transitions, err := tx.ListGraphForValidation(ctx)
		if err != nil {
			// Warnings are informational only — never fail an otherwise-
			// successful update over a failure to compute them.
			return nil
		}
		if w, verr := session.ValidateGraph(stages, transitions); verr != nil {
			warnings = []string{verr.Error()}
		} else {
			warnings = w
		}
		return nil
	})
	if txErr != nil {
		return nil, mapTransitionGraphError(fmt.Errorf("update stage transition: %w", txErr))
	}

	s.invalidateStageConfigCache(ctx, req.Msg.Id)

	return connect.NewResponse(&sessionv1.UpdateStageTransitionResponse{
		Item:     stageTransitionToProto(updated),
		Warnings: warnings,
	}), nil
}

// DeleteStageTransition removes a transition edge. Mirrors
// UpdateStageTransition's disable path: deleting an enabled transition is
// exactly as capable of stranding live items on its from-stage as disabling
// one is (an enabled-edge-turned-absent is indistinguishable from
// enabled-edge-turned-disabled to ValidateDisableTransition's counting), so
// it gets the identical live-item safety check, run inside the same
// transaction as the delete (Task 2.7.2h2's pattern).
// +api: backlog:delete-stage-transition
func (s *BacklogService) DeleteStageTransition(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteStageTransitionRequest],
) (*connect.Response[sessionv1.DeleteStageTransitionResponse], error) {
	if s.stageCRUDRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("stage storage not available"))
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid transition id: %w", err))
	}

	existing, err := s.stageCRUDRepo.GetTransition(ctx, id)
	if err != nil {
		return nil, mapTransitionGraphError(fmt.Errorf("get stage transition: %w", err))
	}

	txErr := s.stageCRUDRepo.WithTx(ctx, func(tx session.TransitionTxRepository) error {
		if existing.Enabled {
			_, transitions, err := tx.ListGraphForValidation(ctx)
			if err != nil {
				return fmt.Errorf("load graph for validation: %w", err)
			}
			// candidateEdges reflects the graph as it would exist after this
			// transition is deleted — omitted entirely rather than marked
			// disabled, since ValidateDisableTransition only counts edges
			// present in the slice with Enabled==true either way.
			candidateEdges := make([]session.TransitionDefinition, 0, len(transitions))
			for _, t := range transitions {
				if t.FromSlug == existing.FromStageSlug && t.ToSlug == existing.ToStageSlug {
					continue
				}
				candidateEdges = append(candidateEdges, t)
			}
			liveCount, err := tx.LiveItemCountForStage(ctx, existing.FromStageSlug)
			if err != nil {
				return fmt.Errorf("count live items for stage %q: %w", existing.FromStageSlug, err)
			}
			if err := session.ValidateDisableTransition(
				candidateEdges, existing.FromStageSlug, map[string]int{existing.FromStageSlug: liveCount},
			); err != nil {
				return err
			}
		}
		return tx.DeleteTransition(ctx, id)
	})
	if txErr != nil {
		return nil, mapTransitionGraphError(fmt.Errorf("delete stage transition: %w", txErr))
	}

	s.invalidateStageConfigCache(ctx, req.Msg.Id)

	return connect.NewResponse(&sessionv1.DeleteStageTransitionResponse{}), nil
}

// GetStageTransition retrieves a single transition by id.
// +api: backlog:get-stage-transition
func (s *BacklogService) GetStageTransition(
	ctx context.Context,
	req *connect.Request[sessionv1.GetStageTransitionRequest],
) (*connect.Response[sessionv1.GetStageTransitionResponse], error) {
	if s.stageCRUDRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("stage storage not available"))
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid transition id: %w", err))
	}

	td, err := s.stageCRUDRepo.GetTransition(ctx, id)
	if err != nil {
		return nil, mapTransitionGraphError(fmt.Errorf("get stage transition: %w", err))
	}

	return connect.NewResponse(&sessionv1.GetStageTransitionResponse{
		Item: stageTransitionToProto(td),
	}), nil
}

// ListStageTransitions returns transitions, optionally filtered by
// from_stage_slug.
// +api: backlog:list-stage-transitions
func (s *BacklogService) ListStageTransitions(
	ctx context.Context,
	req *connect.Request[sessionv1.ListStageTransitionsRequest],
) (*connect.Response[sessionv1.ListStageTransitionsResponse], error) {
	if s.stageCRUDRepo == nil {
		return connect.NewResponse(&sessionv1.ListStageTransitionsResponse{}), nil
	}

	var fromStageSlug string
	if req.Msg.FromStageSlug != nil {
		fromStageSlug = *req.Msg.FromStageSlug
	}

	transitions, err := s.stageCRUDRepo.ListAllTransitions(ctx, fromStageSlug)
	if err != nil {
		return nil, mapTransitionGraphError(fmt.Errorf("list stage transitions: %w", err))
	}

	items := make([]*sessionv1.StageTransition, len(transitions))
	for i, td := range transitions {
		items[i] = stageTransitionToProto(td)
	}

	return connect.NewResponse(&sessionv1.ListStageTransitionsResponse{
		Items: items,
	}), nil
}

// CreateTransitionGate attaches a new gate to a transition. config is
// validated against kind (Task 2.7.2g3) before persisting — an unrecognized
// key, or (for a "custom" gate) a skill outside Story 2.4.4's pre-registered
// allowlist, is rejected here with CodeInvalidArgument and nothing is
// written.
// +api: backlog:create-transition-gate
func (s *BacklogService) CreateTransitionGate(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateTransitionGateRequest],
) (*connect.Response[sessionv1.CreateTransitionGateResponse], error) {
	if s.stageCRUDRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("stage storage not available"))
	}

	transitionID, err := uuid.Parse(req.Msg.TransitionId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid transition id: %w", err))
	}

	if err := validateGateConfig(req.Msg.Kind, req.Msg.Config); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	gate, err := s.stageCRUDRepo.CreateGate(ctx, session.GateCreateInput{
		TransitionID: transitionID,
		Kind:         req.Msg.Kind,
		Config:       configMapFromProto(req.Msg.Config),
		Stateful:     req.Msg.Stateful,
		OrderIndex:   int(req.Msg.OrderIndex),
		Enabled:      req.Msg.Enabled,
	})
	if err != nil {
		return nil, mapTransitionGraphError(fmt.Errorf("create transition gate: %w", err))
	}

	s.invalidateStageConfigCache(ctx, gate.ID.String())

	return connect.NewResponse(&sessionv1.CreateTransitionGateResponse{
		Item: transitionGateToProto(gate),
	}), nil
}

// UpdateTransitionGate modifies an existing gate. kind and config are always
// resubmitted together (see UpdateTransitionGateRequest's doc comment in
// backlog.proto) and revalidated atomically via ParseGateConfig before
// persisting, same as CreateTransitionGate.
// +api: backlog:update-transition-gate
func (s *BacklogService) UpdateTransitionGate(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateTransitionGateRequest],
) (*connect.Response[sessionv1.UpdateTransitionGateResponse], error) {
	if s.stageCRUDRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("stage storage not available"))
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid gate id: %w", err))
	}

	if err := validateGateConfig(req.Msg.Kind, req.Msg.Config); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	kind := req.Msg.Kind
	var orderIndex *int
	if req.Msg.OrderIndex != nil {
		v := int(*req.Msg.OrderIndex)
		orderIndex = &v
	}

	gate, err := s.stageCRUDRepo.UpdateGate(ctx, id, session.GateUpdateInput{
		Kind:       &kind,
		Config:     configMapFromProto(req.Msg.Config),
		Stateful:   req.Msg.Stateful,
		OrderIndex: orderIndex,
		Enabled:    req.Msg.Enabled,
	})
	if err != nil {
		return nil, mapTransitionGraphError(fmt.Errorf("update transition gate: %w", err))
	}

	s.invalidateStageConfigCache(ctx, req.Msg.Id)

	return connect.NewResponse(&sessionv1.UpdateTransitionGateResponse{
		Item: transitionGateToProto(gate),
	}), nil
}

// DeleteTransitionGate removes a gate from its transition.
// +api: backlog:delete-transition-gate
func (s *BacklogService) DeleteTransitionGate(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteTransitionGateRequest],
) (*connect.Response[sessionv1.DeleteTransitionGateResponse], error) {
	if s.stageCRUDRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("stage storage not available"))
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid gate id: %w", err))
	}

	if err := s.stageCRUDRepo.DeleteGate(ctx, id); err != nil {
		return nil, mapTransitionGraphError(fmt.Errorf("delete transition gate: %w", err))
	}

	s.invalidateStageConfigCache(ctx, req.Msg.Id)

	return connect.NewResponse(&sessionv1.DeleteTransitionGateResponse{}), nil
}

// GetTransitionGate retrieves a single gate by id.
// +api: backlog:get-transition-gate
func (s *BacklogService) GetTransitionGate(
	ctx context.Context,
	req *connect.Request[sessionv1.GetTransitionGateRequest],
) (*connect.Response[sessionv1.GetTransitionGateResponse], error) {
	if s.stageCRUDRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("stage storage not available"))
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid gate id: %w", err))
	}

	gate, err := s.stageCRUDRepo.GetGate(ctx, id)
	if err != nil {
		return nil, mapTransitionGraphError(fmt.Errorf("get transition gate: %w", err))
	}

	return connect.NewResponse(&sessionv1.GetTransitionGateResponse{
		Item: transitionGateToProto(gate),
	}), nil
}

// ListTransitionGates returns gates, optionally filtered by transition_id.
// +api: backlog:list-transition-gates
func (s *BacklogService) ListTransitionGates(
	ctx context.Context,
	req *connect.Request[sessionv1.ListTransitionGatesRequest],
) (*connect.Response[sessionv1.ListTransitionGatesResponse], error) {
	if s.stageCRUDRepo == nil {
		return connect.NewResponse(&sessionv1.ListTransitionGatesResponse{}), nil
	}

	var transitionID *uuid.UUID
	if req.Msg.TransitionId != nil {
		id, err := uuid.Parse(*req.Msg.TransitionId)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid transition id: %w", err))
		}
		transitionID = &id
	}

	gates, err := s.stageCRUDRepo.ListAllGates(ctx, transitionID)
	if err != nil {
		return nil, mapTransitionGraphError(fmt.Errorf("list transition gates: %w", err))
	}

	items := make([]*sessionv1.TransitionGate, len(gates))
	for i, g := range gates {
		items[i] = transitionGateToProto(g)
	}

	return connect.NewResponse(&sessionv1.ListTransitionGatesResponse{
		Items: items,
	}), nil
}
