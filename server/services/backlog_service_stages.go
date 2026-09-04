package services

// backlog_service_stages.go — CRUD RPC handlers for BacklogStage (Story
// 2.7.1 of backlog-custom-workflow-stages). Structural pattern mirrors
// backlog_service_pipeline_mode.go's PipelineMode CRUD handlers — a
// slug-addressed, ent-backed entity with Create/Update/Delete/Get/List RPCs
// and a downstream cache (ConfiguredWorkflowEngine's stageConfigCache) kept
// in sync on every write.
//
// This file must NOT import session/ent directly (depguard's
// no_ent_in_services rule, .golangci.yml) — session.StageCRUDRepository
// hands back plain session.StageData DTOs instead.

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

// stageToProto converts a session.StageData to its proto representation.
func stageToProto(sd *session.StageData) *sessionv1.BacklogStage {
	return &sessionv1.BacklogStage{
		Id:          sd.ID.String(),
		Slug:        sd.Slug,
		Name:        sd.Name,
		Description: sd.Description,
		IsEntry:     sd.IsEntry,
		IsTerminal:  sd.IsTerminal,
		Enabled:     sd.Enabled,
		CreatedAt:   timestamppb.New(sd.CreatedAt),
		UpdatedAt:   timestamppb.New(sd.UpdatedAt),
	}
}

// CreateStage registers a new workflow stage.
// +api: backlog:create-stage
func (s *BacklogService) CreateStage(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateStageRequest],
) (*connect.Response[sessionv1.CreateStageResponse], error) {
	if s.stageCRUDRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("stage storage not available"))
	}

	stage, err := s.stageCRUDRepo.CreateStage(ctx, session.StageCreateInput{
		Slug:        req.Msg.Slug,
		Name:        req.Msg.Name,
		Description: req.Msg.Description,
		IsEntry:     req.Msg.IsEntry,
		IsTerminal:  req.Msg.IsTerminal,
		Enabled:     req.Msg.Enabled,
	})
	if err != nil {
		if errors.Is(err, session.ErrConflict) {
			return nil, connect.NewError(connect.CodeAlreadyExists,
				fmt.Errorf("stage with slug %q already exists", req.Msg.Slug))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create stage: %w", err))
	}

	s.invalidateStageConfigCache(ctx, stage.ID.String())

	return connect.NewResponse(&sessionv1.CreateStageResponse{
		Item: stageToProto(stage),
	}), nil
}

// UpdateStage modifies an existing stage's fields via partial update.
// +api: backlog:update-stage
func (s *BacklogService) UpdateStage(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateStageRequest],
) (*connect.Response[sessionv1.UpdateStageResponse], error) {
	if s.stageCRUDRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("stage storage not available"))
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid stage id: %w", err))
	}

	stage, err := s.stageCRUDRepo.UpdateStage(ctx, id, session.StageUpdateInput{
		Name:        req.Msg.Name,
		Description: req.Msg.Description,
		IsEntry:     req.Msg.IsEntry,
		IsTerminal:  req.Msg.IsTerminal,
		Enabled:     req.Msg.Enabled,
	})
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("stage %s not found", req.Msg.Id))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update stage: %w", err))
	}

	// Cache-invalidation failure after this point must NOT fail the RPC — the
	// row above is already correctly persisted, mirroring
	// invalidatePipelineCache's Story 2.2.1 rationale.
	s.invalidateStageConfigCache(ctx, req.Msg.Id)

	return connect.NewResponse(&sessionv1.UpdateStageResponse{
		Item: stageToProto(stage),
	}), nil
}

// DeleteStage removes a stage definition. Per Story 2.7.1's acceptance
// criterion ("disable, don't delete" — research/ux.md §4), a stage with >=1
// live item is rejected with CodeFailedPrecondition naming the item count,
// unless force=true.
// +api: backlog:delete-stage
func (s *BacklogService) DeleteStage(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteStageRequest],
) (*connect.Response[sessionv1.DeleteStageResponse], error) {
	if s.stageCRUDRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("stage storage not available"))
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid stage id: %w", err))
	}

	if !req.Msg.Force {
		stage, err := s.stageCRUDRepo.GetStageByID(ctx, id)
		if err != nil {
			if errors.Is(err, session.ErrNotFound) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("stage %s not found", req.Msg.Id))
			}
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get stage: %w", err))
		}
		liveCount, err := s.stageCRUDRepo.LiveItemCountForStage(ctx, stage.Slug)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("count live items for stage %q: %w", stage.Slug, err))
		}
		if liveCount > 0 {
			return nil, connect.NewError(connect.CodeFailedPrecondition,
				fmt.Errorf("stage %q has %d live item(s); disable it instead, or pass force=true to delete anyway", stage.Slug, liveCount))
		}
	}

	if err := s.stageCRUDRepo.DeleteStage(ctx, id); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("stage %s not found", req.Msg.Id))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("delete stage: %w", err))
	}

	s.invalidateStageConfigCache(ctx, req.Msg.Id)

	return connect.NewResponse(&sessionv1.DeleteStageResponse{}), nil
}

// GetStage retrieves a single stage by slug.
// +api: backlog:get-stage
func (s *BacklogService) GetStage(
	ctx context.Context,
	req *connect.Request[sessionv1.GetStageRequest],
) (*connect.Response[sessionv1.GetStageResponse], error) {
	if s.stageCRUDRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("stage storage not available"))
	}

	stage, err := s.stageCRUDRepo.GetStageBySlug(ctx, req.Msg.Slug)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("stage %q not found", req.Msg.Slug))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get stage: %w", err))
	}

	return connect.NewResponse(&sessionv1.GetStageResponse{
		Item: stageToProto(stage),
	}), nil
}

// ListStages returns all stages, including disabled ones — the settings UI
// must be able to see (and re-enable) disabled stages.
// +api: backlog:list-stages
func (s *BacklogService) ListStages(
	ctx context.Context,
	_ *connect.Request[sessionv1.ListStagesRequest],
) (*connect.Response[sessionv1.ListStagesResponse], error) {
	if s.stageCRUDRepo == nil {
		return connect.NewResponse(&sessionv1.ListStagesResponse{}), nil
	}

	stages, err := s.stageCRUDRepo.ListAllStages(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list stages: %w", err))
	}

	items := make([]*sessionv1.BacklogStage, len(stages))
	for i, st := range stages {
		items[i] = stageToProto(st)
	}

	return connect.NewResponse(&sessionv1.ListStagesResponse{
		Items: items,
	}), nil
}
