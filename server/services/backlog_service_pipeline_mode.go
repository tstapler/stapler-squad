package services

// backlog_service_pipeline_mode.go — CRUD RPC handlers for PipelineMode
// (Epic 2.2 of project_plans/backlog-configurable-pipeline). Structural
// pattern mirrors workflow_service.go's Workflow CRUD handlers (closest
// existing precedent: a slug-addressed, ent-backed entity with Create/
// Update/Delete/Get/List RPCs and a downstream cache that must be kept in
// sync on every write).
//
// Every write handler (Create/Update/Delete) invalidates
// CachingPipelineEngine's in-process cache synchronously before returning,
// via invalidatePipelineCache. Per Story 2.2.1's acceptance criteria, a
// cache-invalidation failure AFTER a successful DB write must never fail the
// RPC — the row is correctly persisted, and reporting an error would be
// inaccurate. invalidatePipelineCache logs a Warn and returns nothing; it
// deliberately has no error return so call sites cannot accidentally start
// propagating this failure into the RPC response.
//
// Story 2.3.1 (structural validation of content-template fields — slug
// format, shell-metacharacter blocklist, and placeholder allow-list) is
// enforced by session.ValidatePipelineModeContent, called from both
// CreatePipelineMode and UpdatePipelineMode before any repository write.

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
)

// pipelineModeToProto converts an ent.PipelineMode to its proto representation,
// deriving content_hash on read from the row's live 9 content-template fields
// (in the same fixed field order session.ComputeContentHash's callers must
// always use — see pipeline_engine.go's pipelineModeCache.refresh). This is
// computed independently of pipelineModeCache/PipelineEngine.ContentHashFor
// so it is correct for disabled modes too, which never enter the
// ListEnabled-backed cache.
func pipelineModeToProto(pm *ent.PipelineMode) *sessionv1.PipelineMode {
	return &sessionv1.PipelineMode{
		Id:                    pm.ID.String(),
		Slug:                  pm.Slug,
		Name:                  pm.Name,
		Description:           pm.Description,
		Enabled:               pm.Enabled,
		StatusCommandTemplate: pm.StatusCommandTemplate,
		DoneCommandTemplate:   pm.DoneCommandTemplate,
		FailCommandTemplate:   pm.FailCommandTemplate,
		ReviewCommandTemplate: pm.ReviewCommandTemplate,
		ShipCommandTemplate:   pm.ShipCommandTemplate,
		HelpCommandTemplate:   pm.HelpCommandTemplate,
		TriagePromptTemplate:  pm.TriagePromptTemplate,
		ReviewPromptTemplate:  pm.ReviewPromptTemplate,
		InitialPromptTemplate: pm.InitialPromptTemplate,
		CreatedAt:             timestamppb.New(pm.CreatedAt),
		UpdatedAt:             timestamppb.New(pm.UpdatedAt),
		ContentHash: session.ComputeContentHash(
			pm.StatusCommandTemplate,
			pm.DoneCommandTemplate,
			pm.FailCommandTemplate,
			pm.ReviewCommandTemplate,
			pm.ShipCommandTemplate,
			pm.HelpCommandTemplate,
			pm.TriagePromptTemplate,
			pm.ReviewPromptTemplate,
			pm.InitialPromptTemplate,
		),
	}
}

// pipelineCacheInvalidator is a narrow, consumer-defined interface (per
// the `interface-pollution-checklist` skill — defined where consumed,
// not next to CachingPipelineEngine) matched via duck typing against
// s.pipelineEngine. CachingPipelineEngine.InvalidateCache satisfies it;
// PipelineEngine itself intentionally does NOT declare this method (it is a
// write-side operational concern, not part of the read-side resolution
// contract every PipelineEngine implementation must offer).
type pipelineCacheInvalidator interface {
	InvalidateCache(ctx context.Context) error
}

// invalidatePipelineCache re-fetches enabled pipeline modes into
// s.pipelineEngine's cache after a successful Create/Update/Delete. id is
// used only for the Warn log line if invalidation fails. No-op (silently) if
// pipelineEngine is unwired or doesn't support cache invalidation — neither
// case is an error worth surfacing here.
//
// Deliberately returns nothing: per Story 2.2.1's acceptance criteria, an
// invalidation failure after a successful DB write must never fail the RPC,
// so there is no error for callers to (mis)propagate.
func (s *BacklogService) invalidatePipelineCache(ctx context.Context, id string) {
	invalidator, ok := s.pipelineEngine.(pipelineCacheInvalidator)
	if !ok {
		return
	}
	if err := invalidator.InvalidateCache(ctx); err != nil {
		log.WarningLog().Printf("[PipelineEngine] cache invalidation failed after successful write id=%s: %v — cache may be stale until next successful invalidation", id, err)
	}
}

// CreatePipelineMode registers a new runtime-definable pipeline mode.
// +api: backlog:create-pipeline-mode
func (s *BacklogService) CreatePipelineMode(
	ctx context.Context,
	req *connect.Request[sessionv1.CreatePipelineModeRequest],
) (*connect.Response[sessionv1.CreatePipelineModeResponse], error) {
	if s.pipelineModeRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("pipeline mode storage not available"))
	}

	if err := session.ValidatePipelineModeContent(session.PipelineModeContentFields{
		Slug:                  req.Msg.Slug,
		ValidateSlug:          true,
		StatusCommandTemplate: req.Msg.StatusCommandTemplate,
		DoneCommandTemplate:   req.Msg.DoneCommandTemplate,
		FailCommandTemplate:   req.Msg.FailCommandTemplate,
		ReviewCommandTemplate: req.Msg.ReviewCommandTemplate,
		ShipCommandTemplate:   req.Msg.ShipCommandTemplate,
		HelpCommandTemplate:   req.Msg.HelpCommandTemplate,
		TriagePromptTemplate:  req.Msg.TriagePromptTemplate,
		ReviewPromptTemplate:  req.Msg.ReviewPromptTemplate,
		InitialPromptTemplate: req.Msg.InitialPromptTemplate,
	}); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	input := session.PipelineModeCreateInput{
		Slug:                  req.Msg.Slug,
		Name:                  req.Msg.Name,
		Description:           req.Msg.Description,
		Enabled:               req.Msg.Enabled,
		StatusCommandTemplate: req.Msg.StatusCommandTemplate,
		DoneCommandTemplate:   req.Msg.DoneCommandTemplate,
		FailCommandTemplate:   req.Msg.FailCommandTemplate,
		ReviewCommandTemplate: req.Msg.ReviewCommandTemplate,
		ShipCommandTemplate:   req.Msg.ShipCommandTemplate,
		HelpCommandTemplate:   req.Msg.HelpCommandTemplate,
		TriagePromptTemplate:  req.Msg.TriagePromptTemplate,
		ReviewPromptTemplate:  req.Msg.ReviewPromptTemplate,
		InitialPromptTemplate: req.Msg.InitialPromptTemplate,
	}

	pm, err := s.pipelineModeRepo.Create(ctx, input)
	if err != nil {
		if errors.Is(err, session.ErrConflict) {
			return nil, connect.NewError(connect.CodeAlreadyExists,
				fmt.Errorf("pipeline mode with slug %q already exists", req.Msg.Slug))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create pipeline mode: %w", err))
	}

	s.invalidatePipelineCache(ctx, pm.ID.String())

	return connect.NewResponse(&sessionv1.CreatePipelineModeResponse{
		Item: pipelineModeToProto(pm),
	}), nil
}

// UpdatePipelineMode modifies an existing pipeline mode's fields via partial update.
// +api: backlog:update-pipeline-mode
func (s *BacklogService) UpdatePipelineMode(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdatePipelineModeRequest],
) (*connect.Response[sessionv1.UpdatePipelineModeResponse], error) {
	if s.pipelineModeRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("pipeline mode storage not available"))
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid pipeline mode id: %w", err))
	}

	// Slug is immutable and has no field on UpdatePipelineModeRequest, so
	// ValidateSlug is left false here — there is nothing for an Update call
	// to validate. Unset (nil) optional fields are left as "" in
	// contentFields, which trivially passes validation (see
	// PipelineModeContentFields' doc comment) so a partial update is never
	// rejected over a field it isn't touching.
	contentFields := session.PipelineModeContentFields{}
	if req.Msg.StatusCommandTemplate != nil {
		contentFields.StatusCommandTemplate = *req.Msg.StatusCommandTemplate
	}
	if req.Msg.DoneCommandTemplate != nil {
		contentFields.DoneCommandTemplate = *req.Msg.DoneCommandTemplate
	}
	if req.Msg.FailCommandTemplate != nil {
		contentFields.FailCommandTemplate = *req.Msg.FailCommandTemplate
	}
	if req.Msg.ReviewCommandTemplate != nil {
		contentFields.ReviewCommandTemplate = *req.Msg.ReviewCommandTemplate
	}
	if req.Msg.ShipCommandTemplate != nil {
		contentFields.ShipCommandTemplate = *req.Msg.ShipCommandTemplate
	}
	if req.Msg.HelpCommandTemplate != nil {
		contentFields.HelpCommandTemplate = *req.Msg.HelpCommandTemplate
	}
	if req.Msg.TriagePromptTemplate != nil {
		contentFields.TriagePromptTemplate = *req.Msg.TriagePromptTemplate
	}
	if req.Msg.ReviewPromptTemplate != nil {
		contentFields.ReviewPromptTemplate = *req.Msg.ReviewPromptTemplate
	}
	if req.Msg.InitialPromptTemplate != nil {
		contentFields.InitialPromptTemplate = *req.Msg.InitialPromptTemplate
	}
	if err := session.ValidatePipelineModeContent(contentFields); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	update := session.PipelineModeUpdateInput{
		Name:                  req.Msg.Name,
		Description:           req.Msg.Description,
		Enabled:               req.Msg.Enabled,
		StatusCommandTemplate: req.Msg.StatusCommandTemplate,
		DoneCommandTemplate:   req.Msg.DoneCommandTemplate,
		FailCommandTemplate:   req.Msg.FailCommandTemplate,
		ReviewCommandTemplate: req.Msg.ReviewCommandTemplate,
		ShipCommandTemplate:   req.Msg.ShipCommandTemplate,
		HelpCommandTemplate:   req.Msg.HelpCommandTemplate,
		TriagePromptTemplate:  req.Msg.TriagePromptTemplate,
		ReviewPromptTemplate:  req.Msg.ReviewPromptTemplate,
		InitialPromptTemplate: req.Msg.InitialPromptTemplate,
	}

	pm, err := s.pipelineModeRepo.Update(ctx, id, update)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("pipeline mode %s not found", req.Msg.Id))
		}
		if errors.Is(err, session.ErrConflict) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("pipeline mode slug already exists"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update pipeline mode: %w", err))
	}

	// Cache-invalidation failure after this point must NOT fail the RPC — the
	// row above is already correctly persisted (Story 2.2.1 acceptance
	// criteria). invalidatePipelineCache logs a Warn internally and never
	// returns an error for this reason.
	s.invalidatePipelineCache(ctx, req.Msg.Id)

	return connect.NewResponse(&sessionv1.UpdatePipelineModeResponse{
		Item: pipelineModeToProto(pm),
	}), nil
}

// DeletePipelineMode removes a pipeline mode definition. Does NOT block on
// existing BacklogItemData.PipelineMode references — per the plan's
// Unresolved Questions default, a deleted-but-still-referenced mode relies on
// PipelineEngine's fail-closed resolution (Story 1.3.3: an unresolved slug
// falls back to the default pipeline with a Warn log, it never errors).
// +api: backlog:delete-pipeline-mode
func (s *BacklogService) DeletePipelineMode(
	ctx context.Context,
	req *connect.Request[sessionv1.DeletePipelineModeRequest],
) (*connect.Response[sessionv1.DeletePipelineModeResponse], error) {
	if s.pipelineModeRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("pipeline mode storage not available"))
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid pipeline mode id: %w", err))
	}

	if err := s.pipelineModeRepo.Delete(ctx, id); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("pipeline mode %s not found", req.Msg.Id))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("delete pipeline mode: %w", err))
	}

	s.invalidatePipelineCache(ctx, req.Msg.Id)

	return connect.NewResponse(&sessionv1.DeletePipelineModeResponse{}), nil
}

// GetPipelineMode retrieves a single pipeline mode by slug.
// +api: backlog:get-pipeline-mode
func (s *BacklogService) GetPipelineMode(
	ctx context.Context,
	req *connect.Request[sessionv1.GetPipelineModeRequest],
) (*connect.Response[sessionv1.GetPipelineModeResponse], error) {
	if s.pipelineModeRepo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("pipeline mode storage not available"))
	}

	pm, err := s.pipelineModeRepo.GetBySlug(ctx, req.Msg.Slug)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("pipeline mode %q not found", req.Msg.Slug))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get pipeline mode: %w", err))
	}

	return connect.NewResponse(&sessionv1.GetPipelineModeResponse{
		Item: pipelineModeToProto(pm),
	}), nil
}

// ListPipelineModes returns all pipeline modes, including disabled ones — the
// management UI must be able to see (and re-enable) disabled modes, unlike
// PipelineEngine's cache, which is backed by ListEnabled only.
// +api: backlog:list-pipeline-modes
func (s *BacklogService) ListPipelineModes(
	ctx context.Context,
	_ *connect.Request[sessionv1.ListPipelineModesRequest],
) (*connect.Response[sessionv1.ListPipelineModesResponse], error) {
	if s.pipelineModeRepo == nil {
		return connect.NewResponse(&sessionv1.ListPipelineModesResponse{}), nil
	}

	pms, err := s.pipelineModeRepo.ListAll(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list pipeline modes: %w", err))
	}

	items := make([]*sessionv1.PipelineMode, len(pms))
	for i, pm := range pms {
		items[i] = pipelineModeToProto(pm)
	}

	return connect.NewResponse(&sessionv1.ListPipelineModesResponse{
		Items: items,
	}), nil
}
