package services

// backlog_service_liveness.go — CRUD RPC handlers for LivenessDefinition
// (Epic 1.3, Story 1.3.2 of project_plans/backlog-custom-workflow-stages).
// Structural pattern mirrors backlog_service_pipeline_mode.go's PipelineMode
// CRUD handlers (closest existing precedent: a slug/mode-addressed, ent-backed
// entity with Create/Update/Delete/Get/List RPCs and a downstream in-process
// cache that must be kept in sync on every write).
//
// Every write handler (Create/Update/Delete) invalidates the injected
// LivenessEngine's cache synchronously before returning, via
// invalidateLivenessCache. As with invalidatePipelineCache, a
// cache-invalidation failure AFTER a successful DB write must never fail the
// RPC — the row is correctly persisted, and reporting an error would be
// inaccurate. invalidateLivenessCache logs a Warn and returns nothing.
//
// No UI consumes these RPCs in Phase 1 (per the plan's Milestone Structure
// note) — operator-callable only.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// livenessDefinitionToProto converts a session.LivenessDefinitionRecord
// (the no_ent_in_services-lint-compliant DTO — see that type's doc comment)
// to its proto representation. Nullable ms/count fields not applicable to
// the record's Kind are left at their proto zero value.
func livenessDefinitionToProto(row *session.LivenessDefinitionRecord) *sessionv1.LivenessDefinition {
	msg := &sessionv1.LivenessDefinition{
		Id:        row.ID.String(),
		StageSlug: row.StageSlug,
		Kind:      row.Kind,
		Enabled:   row.Enabled,
		CreatedAt: timestamppb.New(row.CreatedAt),
		UpdatedAt: timestamppb.New(row.UpdatedAt),
	}
	if row.PipelineMode != nil {
		msg.PipelineMode = row.PipelineMode
	}
	if row.ExpectedDurationMs != nil {
		msg.ExpectedDurationMs = *row.ExpectedDurationMs
	}
	if row.StalenessMarginMs != nil {
		msg.StalenessMarginMs = *row.StalenessMarginMs
	}
	if row.MaxNoProgressDurationMs != nil {
		msg.MaxNoProgressDurationMs = *row.MaxNoProgressDurationMs
	}
	if row.CycleThreshold != nil {
		msg.CycleThreshold = *row.CycleThreshold
	}
	if row.CycleLookbackMs != nil {
		msg.CycleLookbackMs = *row.CycleLookbackMs
	}
	return msg
}

// livenessCacheInvalidator is a narrow, consumer-defined interface (per the
// `interface-pollution-checklist` skill) matched via duck typing against
// s.livenessEngine. *session.CachingLivenessEngine.InvalidateCache satisfies
// it; the session.LivenessEngine interface itself intentionally does NOT
// declare this method — it's a write-side operational concern, not part of
// the read-side resolution contract every LivenessEngine implementation must
// offer. Mirrors pipelineCacheInvalidator (backlog_service_pipeline_mode.go)
// exactly.
type livenessCacheInvalidator interface {
	InvalidateCache(ctx context.Context) error
}

// invalidateLivenessCache re-fetches liveness definitions into
// s.livenessEngine's cache after a successful Create/Update/Delete. id is
// used only for the Warn log line if invalidation fails. No-op (silently) if
// livenessEngine is unwired or doesn't support cache invalidation.
//
// Deliberately returns nothing: an invalidation failure after a successful
// DB write must never fail the RPC (mirrors invalidatePipelineCache's
// contract), so there is no error for callers to (mis)propagate.
func (s *BacklogService) invalidateLivenessCache(ctx context.Context, id string) {
	invalidator, ok := s.livenessEngine.(livenessCacheInvalidator)
	if !ok {
		return
	}
	if err := invalidator.InvalidateCache(ctx); err != nil {
		log.WarningLog().Printf("[LivenessEngine] cache invalidation failed after successful write id=%s: %v — cache may be stale until next successful invalidation", id, err)
	}
}

// livenessDefinitionFromCreateRequest builds a session.LivenessDefinition
// from req's kind + ms/count fields, deferring shape validation to
// session.NewLivenessDefinition (a field set outside req.Kind's shape is
// rejected there — e.g. expected_duration_ms set alongside kind="heartbeat").
// Only non-zero fields are passed as options: the proto request has no
// optional/presence tracking for these (unlike UpdateLivenessDefinitionRequest,
// which does), so a zero value is indistinguishable from "not set" here —
// exactly matching how LivenessDefinition.validate() already treats zero
// kind-specific fields as "not set" for shape-mismatch detection.
func livenessDefinitionFromCreateRequest(req *sessionv1.CreateLivenessDefinitionRequest) (*session.LivenessDefinition, error) {
	var opts []session.LivenessDefinitionOption
	if req.ExpectedDurationMs != 0 {
		opts = append(opts, session.WithExpectedDuration(time.Duration(req.ExpectedDurationMs)*time.Millisecond))
	}
	if req.StalenessMarginMs != 0 {
		opts = append(opts, session.WithStalenessMargin(time.Duration(req.StalenessMarginMs)*time.Millisecond))
	}
	if req.MaxNoProgressDurationMs != 0 {
		opts = append(opts, session.WithMaxNoProgressDuration(time.Duration(req.MaxNoProgressDurationMs)*time.Millisecond))
	}
	if req.CycleThreshold != 0 {
		opts = append(opts, session.WithCycleThreshold(int(req.CycleThreshold)))
	}
	if req.CycleLookbackMs != 0 {
		opts = append(opts, session.WithCycleLookback(time.Duration(req.CycleLookbackMs)*time.Millisecond))
	}
	return session.NewLivenessDefinition(session.LivenessKind(req.Kind), opts...)
}

// CreateLivenessDefinition registers a new stage/pipeline-mode liveness
// override.
// +api: backlog:create-liveness-definition
func (s *BacklogService) CreateLivenessDefinition(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateLivenessDefinitionRequest],
) (*connect.Response[sessionv1.CreateLivenessDefinitionResponse], error) {
	if s.livenessRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("liveness definition storage not available"))
	}

	mode := session.PipelineModeDefault
	if req.Msg.PipelineMode != nil {
		mode = session.PipelineMode(*req.Msg.PipelineMode)
	}

	// Reject a request whose (stage_slug, pipeline_mode) pair already has an
	// enabled row (Story 1.3.2 acceptance criterion) — checked explicitly
	// here (rather than relying solely on the DB's UNIQUE constraint) so the
	// "enabled" qualifier is honored precisely as specified.
	if existing, err := s.livenessRepo.GetByStageAndMode(ctx, req.Msg.StageSlug, mode); err == nil {
		if existing.Enabled {
			return nil, connect.NewError(connect.CodeAlreadyExists,
				fmt.Errorf("liveness definition for stage %q pipeline_mode %q already exists", req.Msg.StageSlug, mode))
		}
	} else if !errors.Is(err, session.ErrNotFound) {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("check existing liveness definition: %w", err))
	}

	def, err := livenessDefinitionFromCreateRequest(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	var pipelineMode *string
	if req.Msg.PipelineMode != nil {
		pipelineMode = req.Msg.PipelineMode
	}
	enabled := req.Msg.Enabled

	row, err := s.livenessRepo.Create(ctx, session.LivenessCreateInput{
		StageSlug:    req.Msg.StageSlug,
		PipelineMode: pipelineMode,
		Definition:   *def,
		Enabled:      &enabled,
	})
	if err != nil {
		if errors.Is(err, session.ErrConflict) {
			return nil, connect.NewError(connect.CodeAlreadyExists,
				fmt.Errorf("liveness definition for stage %q pipeline_mode %q already exists", req.Msg.StageSlug, mode))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create liveness definition: %w", err))
	}

	s.invalidateLivenessCache(ctx, row.ID.String())

	return connect.NewResponse(&sessionv1.CreateLivenessDefinitionResponse{
		Item: livenessDefinitionToProto(row),
	}), nil
}

// UpdateLivenessDefinition modifies an existing liveness definition's fields
// via partial update. StageSlug/PipelineMode/Kind are immutable — see
// session.LivenessRepository.Update's doc comment.
// +api: backlog:update-liveness-definition
func (s *BacklogService) UpdateLivenessDefinition(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateLivenessDefinitionRequest],
) (*connect.Response[sessionv1.UpdateLivenessDefinitionResponse], error) {
	if s.livenessRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("liveness definition storage not available"))
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid liveness definition id: %w", err))
	}

	update := session.LivenessUpdateInput{
		Enabled: req.Msg.Enabled,
	}
	if req.Msg.ExpectedDurationMs != nil {
		d := time.Duration(*req.Msg.ExpectedDurationMs) * time.Millisecond
		update.ExpectedDuration = &d
	}
	if req.Msg.StalenessMarginMs != nil {
		d := time.Duration(*req.Msg.StalenessMarginMs) * time.Millisecond
		update.StalenessMargin = &d
	}
	if req.Msg.MaxNoProgressDurationMs != nil {
		d := time.Duration(*req.Msg.MaxNoProgressDurationMs) * time.Millisecond
		update.MaxNoProgressDuration = &d
	}
	if req.Msg.CycleThreshold != nil {
		v := int(*req.Msg.CycleThreshold)
		update.CycleThreshold = &v
	}
	if req.Msg.CycleLookbackMs != nil {
		d := time.Duration(*req.Msg.CycleLookbackMs) * time.Millisecond
		update.CycleLookback = &d
	}

	row, err := s.livenessRepo.Update(ctx, id, update)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("liveness definition %s not found", req.Msg.Id))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update liveness definition: %w", err))
	}

	// Cache-invalidation failure after this point must NOT fail the RPC — the
	// row above is already correctly persisted. invalidateLivenessCache logs
	// a Warn internally and never returns an error for this reason.
	s.invalidateLivenessCache(ctx, req.Msg.Id)

	return connect.NewResponse(&sessionv1.UpdateLivenessDefinitionResponse{
		Item: livenessDefinitionToProto(row),
	}), nil
}

// DeleteLivenessDefinition removes a liveness definition override.
// +api: backlog:delete-liveness-definition
func (s *BacklogService) DeleteLivenessDefinition(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteLivenessDefinitionRequest],
) (*connect.Response[sessionv1.DeleteLivenessDefinitionResponse], error) {
	if s.livenessRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("liveness definition storage not available"))
	}
	if err := deleteByIDAndInvalidateCache(ctx, req.Msg.Id, "liveness definition", s.livenessRepo.Delete, s.invalidateLivenessCache); err != nil {
		return nil, err
	}
	return connect.NewResponse(&sessionv1.DeleteLivenessDefinitionResponse{}), nil
}

// deleteByIDAndInvalidateCache is the shared shape behind every "delete a
// slug/mode-addressed, ent-backed entity, then invalidate its in-process
// cache" RPC handler in this package (DeleteLivenessDefinition here;
// DeletePipelineMode's identical inline version in
// backlog_service_pipeline_mode.go predates this helper and is left as-is —
// dupl's new-code-only gate only requires the new copy to stop being a
// literal duplicate, not a retroactive rewrite of already-shipped code).
func deleteByIDAndInvalidateCache(
	ctx context.Context,
	rawID string,
	entityLabel string,
	deleteFn func(ctx context.Context, id uuid.UUID) error,
	invalidateCache func(ctx context.Context, rawID string),
) error {
	id, err := uuid.Parse(rawID)
	if err != nil {
		return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid %s id: %w", entityLabel, err))
	}
	if err := deleteFn(ctx, id); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return connect.NewError(connect.CodeNotFound, fmt.Errorf("%s %s not found", entityLabel, rawID))
		}
		return connect.NewError(connect.CodeInternal, fmt.Errorf("delete %s: %w", entityLabel, err))
	}
	invalidateCache(ctx, rawID)
	return nil
}

// GetLivenessDefinition retrieves a single liveness definition by
// (stage_slug, pipeline_mode) exact match — no fallback (that logic lives
// exclusively in livenessCache.Get, consulted only via LivenessEngine.LivenessFor).
// +api: backlog:get-liveness-definition
func (s *BacklogService) GetLivenessDefinition(
	ctx context.Context,
	req *connect.Request[sessionv1.GetLivenessDefinitionRequest],
) (*connect.Response[sessionv1.GetLivenessDefinitionResponse], error) {
	if s.livenessRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("liveness definition storage not available"))
	}

	mode := session.PipelineModeDefault
	if req.Msg.PipelineMode != nil {
		mode = session.PipelineMode(*req.Msg.PipelineMode)
	}

	row, err := s.livenessRepo.GetByStageAndMode(ctx, req.Msg.StageSlug, mode)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("liveness definition for stage %q pipeline_mode %q not found", req.Msg.StageSlug, mode))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get liveness definition: %w", err))
	}

	return connect.NewResponse(&sessionv1.GetLivenessDefinitionResponse{
		Item: livenessDefinitionToProto(row),
	}), nil
}

// ListLivenessDefinitions returns all liveness definitions, including
// disabled ones — mirrors ListPipelineModes' management-visibility posture.
// +api: backlog:list-liveness-definitions
func (s *BacklogService) ListLivenessDefinitions(
	ctx context.Context,
	_ *connect.Request[sessionv1.ListLivenessDefinitionsRequest],
) (*connect.Response[sessionv1.ListLivenessDefinitionsResponse], error) {
	if s.livenessRepo == nil {
		return connect.NewResponse(&sessionv1.ListLivenessDefinitionsResponse{}), nil
	}

	rows, err := s.livenessRepo.ListAll(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list liveness definitions: %w", err))
	}

	items := make([]*sessionv1.LivenessDefinition, len(rows))
	for i, row := range rows {
		items[i] = livenessDefinitionToProto(row)
	}

	return connect.NewResponse(&sessionv1.ListLivenessDefinitionsResponse{
		Items: items,
	}), nil
}
