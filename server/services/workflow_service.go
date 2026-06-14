package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/server/workflows"
)

// WorkflowSchedulerInterface is the interface WorkflowService uses to interact with the scheduler.
// Defined in this package to avoid a circular import with server/workflows.
type WorkflowSchedulerInterface interface {
	Reload(ctx context.Context, wf *ent.Workflow) error
	Remove(workflowID string) error
	// FireNow immediately fires a workflow job, returning the created session ID.
	FireNow(ctx context.Context, wf *ent.Workflow, arg string) (string, error)
}

// WorkflowService implements the workflow-related RPCs on SessionService.
type WorkflowService struct {
	repo      session.WorkflowRepository
	scheduler WorkflowSchedulerInterface
}

// NewWorkflowService creates a new WorkflowService.
func NewWorkflowService(repo session.WorkflowRepository, scheduler WorkflowSchedulerInterface) *WorkflowService {
	return &WorkflowService{repo: repo, scheduler: scheduler}
}

// entWorkflowToProto converts an ent.Workflow to its proto representation.
func entWorkflowToProto(w *ent.Workflow) *sessionv1.WorkflowProto {
	proto := &sessionv1.WorkflowProto{
		Id:              w.ID.String(),
		Slug:            w.Slug,
		Name:            w.Name,
		Description:     w.Description,
		Command:         w.Command,
		TargetDirectory: w.TargetDirectory,
		InputTemplate:   w.InputTemplate,
		SessionType:     w.SessionType,
		Model:           w.Model,
		AgentType:       w.AgentType,
		CronExpression:  w.CronExpression,
		CronEnabled:     w.CronEnabled,
		CreatedAt:       timestamppb.New(w.CreatedAt),
		UpdatedAt:       timestamppb.New(w.UpdatedAt),
	}
	// Optional retention fields — only set on proto when non-zero (zero = disabled).
	if w.KeepSessions != 0 {
		v := int32(w.KeepSessions)
		proto.KeepSessions = &v
	}
	if w.ArchiveAfterHours != 0 {
		v := int32(w.ArchiveAfterHours)
		proto.ArchiveAfterHours = &v
	}
	return proto
}

// validateTargetDirectory checks that dir is an absolute path with no traversal components.
func validateTargetDirectory(dir string) error {
	if !filepath.IsAbs(dir) {
		return errors.New("target_directory must be an absolute path")
	}
	if filepath.Clean(dir) != dir {
		return errors.New("target_directory contains path traversal components")
	}
	return nil
}

// CreateWorkflow handles the CreateWorkflow RPC.
func (s *WorkflowService) CreateWorkflow(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateWorkflowRequest],
) (*connect.Response[sessionv1.CreateWorkflowResponse], error) {
	if s.repo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("workflow storage not available"))
	}

	// Validate slug.
	if err := session.ValidateWorkflowSlug(req.Msg.Slug); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	// Validate required fields per ADR-9.
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("name is required"))
	}
	if req.Msg.Command == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("command is required"))
	}
	if req.Msg.TargetDirectory == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("target_directory is required"))
	}
	if err := validateTargetDirectory(req.Msg.TargetDirectory); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	// Validate cron fields.
	if req.Msg.CronEnabled && req.Msg.CronExpression == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New("cron_expression is required when cron_enabled is true"))
	}
	if req.Msg.CronExpression != "" {
		if err := workflows.ValidateCronExpression(req.Msg.CronExpression); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cron expression: %w", err))
		}
	}

	createInput := session.WorkflowCreateInput{
		Slug:            req.Msg.Slug,
		Name:            req.Msg.Name,
		Description:     req.Msg.Description,
		Command:         req.Msg.Command,
		TargetDirectory: req.Msg.TargetDirectory,
		InputTemplate:   req.Msg.InputTemplate,
		SessionType:     req.Msg.SessionType,
		Model:           req.Msg.Model,
		AgentType:       req.Msg.AgentType,
		CronExpression:  req.Msg.CronExpression,
		CronEnabled:     req.Msg.CronEnabled,
	}
	if req.Msg.KeepSessions != nil {
		v := int(*req.Msg.KeepSessions)
		createInput.KeepSessions = &v
	}
	if req.Msg.ArchiveAfterHours != nil {
		v := int(*req.Msg.ArchiveAfterHours)
		createInput.ArchiveAfterHours = &v
	}
	wf, err := s.repo.Create(ctx, createInput)
	if err != nil {
		if errors.Is(err, session.ErrConflict) {
			return nil, connect.NewError(connect.CodeAlreadyExists,
				fmt.Errorf("workflow with slug %q already exists", req.Msg.Slug))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create workflow: %w", err))
	}

	// Register cron if enabled.
	if req.Msg.CronEnabled && s.scheduler != nil {
		if reloadErr := s.scheduler.Reload(ctx, wf); reloadErr != nil {
			log.Warn("[WorkflowService] cron schedule registration failed after create",
				"slug", wf.Slug, "err", reloadErr)
			// Workflow persisted; cron will retry on next server start via Start().
		}
	}

	return connect.NewResponse(&sessionv1.CreateWorkflowResponse{
		Workflow: entWorkflowToProto(wf),
	}), nil
}

// UpdateWorkflow handles the UpdateWorkflow RPC.
func (s *WorkflowService) UpdateWorkflow(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateWorkflowRequest],
) (*connect.Response[sessionv1.UpdateWorkflowResponse], error) {
	if s.repo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("workflow storage not available"))
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid workflow id: %w", err))
	}

	// Validate target_directory if being updated.
	if req.Msg.TargetDirectory != nil {
		if err := validateTargetDirectory(*req.Msg.TargetDirectory); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}
	// Validate cron fields if being updated.
	// If cron_enabled is being set true while cron_expression is being set to empty, reject.
	if req.Msg.CronEnabled != nil && *req.Msg.CronEnabled &&
		req.Msg.CronExpression != nil && *req.Msg.CronExpression == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			errors.New("cron_expression is required when cron_enabled is true"))
	}
	if req.Msg.CronExpression != nil && *req.Msg.CronExpression != "" {
		if err := workflows.ValidateCronExpression(*req.Msg.CronExpression); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cron expression: %w", err))
		}
	}

	update := session.WorkflowUpdateInput{
		Name:            req.Msg.Name,
		Description:     req.Msg.Description,
		Command:         req.Msg.Command,
		TargetDirectory: req.Msg.TargetDirectory,
		InputTemplate:   req.Msg.InputTemplate,
		SessionType:     req.Msg.SessionType,
		Model:           req.Msg.Model,
		AgentType:       req.Msg.AgentType,
		CronExpression:  req.Msg.CronExpression,
		CronEnabled:     req.Msg.CronEnabled,
	}
	if req.Msg.KeepSessions != nil {
		v := int(*req.Msg.KeepSessions)
		update.KeepSessions = &v
	}
	if req.Msg.ArchiveAfterHours != nil {
		v := int(*req.Msg.ArchiveAfterHours)
		update.ArchiveAfterHours = &v
	}

	wf, err := s.repo.Update(ctx, id, update)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("workflow %s not found", req.Msg.Id))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("update workflow: %w", err))
	}

	// Reload cron entry (will add or remove based on cron_enabled).
	if s.scheduler != nil {
		if reloadErr := s.scheduler.Reload(ctx, wf); reloadErr != nil {
			log.Warn("[WorkflowService] cron schedule registration failed after update",
				"slug", wf.Slug, "err", reloadErr)
			// Workflow persisted; cron will retry on next server start via Start().
		}
	}

	return connect.NewResponse(&sessionv1.UpdateWorkflowResponse{
		Workflow: entWorkflowToProto(wf),
	}), nil
}

// DeleteWorkflow handles the DeleteWorkflow RPC.
func (s *WorkflowService) DeleteWorkflow(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteWorkflowRequest],
) (*connect.Response[sessionv1.DeleteWorkflowResponse], error) {
	if s.repo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("workflow storage not available"))
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid workflow id: %w", err))
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("workflow %s not found", req.Msg.Id))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("delete workflow: %w", err))
	}

	// Remove from cron scheduler.
	if s.scheduler != nil {
		_ = s.scheduler.Remove(req.Msg.Id)
	}

	return connect.NewResponse(&sessionv1.DeleteWorkflowResponse{}), nil
}

// ListWorkflows handles the ListWorkflows RPC.
func (s *WorkflowService) ListWorkflows(
	ctx context.Context,
	req *connect.Request[sessionv1.ListWorkflowsRequest],
) (*connect.Response[sessionv1.ListWorkflowsResponse], error) {
	if s.repo == nil {
		return connect.NewResponse(&sessionv1.ListWorkflowsResponse{
			Workflows: []*sessionv1.WorkflowProto{},
		}), nil
	}

	wfs, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list workflows: %w", err))
	}

	protos := make([]*sessionv1.WorkflowProto, len(wfs))
	for i, wf := range wfs {
		protos[i] = entWorkflowToProto(wf)
	}

	return connect.NewResponse(&sessionv1.ListWorkflowsResponse{
		Workflows: protos,
	}), nil
}

// RunWorkflow handles the RunWorkflow RPC.
// Delegates to scheduler.FireNow to avoid circular dependency with SessionService.
func (s *WorkflowService) RunWorkflow(
	ctx context.Context,
	req *connect.Request[sessionv1.RunWorkflowRequest],
) (*connect.Response[sessionv1.RunWorkflowResponse], error) {
	if s.repo == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("workflow storage not available"))
	}
	if s.scheduler == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("workflow scheduler not available"))
	}

	id, err := uuid.Parse(req.Msg.Id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid workflow id: %w", err))
	}

	wf, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("workflow %s not found", req.Msg.Id))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get workflow: %w", err))
	}

	sessionID, err := s.scheduler.FireNow(ctx, wf, req.Msg.Arg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("run workflow: %w", err))
	}

	return connect.NewResponse(&sessionv1.RunWorkflowResponse{
		SessionId: sessionID,
	}), nil
}
