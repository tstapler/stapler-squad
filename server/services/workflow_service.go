package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/workflows"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	entsession "github.com/tstapler/stapler-squad/session/ent/session"
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
	// storage is used by ArchiveWorkflowSessions and DeleteWorkflowFailedSessions to
	// access the ent client for bulk updates. Wired at construction time.
	storage session.InstanceStore
	// poller is wired after construction via SetPoller, forwarded from SessionService.
	poller *session.ReviewQueuePoller
	// fireEventRepo backs ListTriggerFireEvents (Epic 1.2, Task 1.2.1d). Optional — nil
	// makes the RPC return an empty list rather than erroring, matching this file's
	// other nil-degradation conventions (e.g. ListWorkflows with a nil repo).
	fireEventRepo session.TriggerFireEventRepository
}

// NewWorkflowService creates a new WorkflowService.
// storage is required for ArchiveWorkflowSessions / DeleteWorkflowFailedSessions; pass nil
// to disable those RPCs (they will return CodeUnavailable).
func NewWorkflowService(repo session.WorkflowRepository, scheduler WorkflowSchedulerInterface, storage session.InstanceStore) *WorkflowService {
	return &WorkflowService{repo: repo, scheduler: scheduler, storage: storage}
}

// SetPoller wires the live-instance poller so ArchiveWorkflowSessions can update
// in-memory instance state. Forwarded from SessionService.SetReviewQueuePoller.
func (ws *WorkflowService) SetPoller(p *session.ReviewQueuePoller) {
	ws.poller = p
}

// SetTriggerFireEventRepo wires the trigger-fire audit trail repository used by
// ListTriggerFireEvents. Optional — see fireEventRepo's doc comment.
func (ws *WorkflowService) SetTriggerFireEventRepo(repo session.TriggerFireEventRepository) {
	ws.fireEventRepo = repo
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
	// Trigger fields (webhook-triggers Epic 1.1). webhook_secret_encrypted is
	// deliberately never copied onto the proto — never returned in plaintext (or
	// ciphertext) by any RPC.
	proto.TriggerType = w.TriggerType
	proto.GithubRepo = w.GithubRepo
	proto.GithubBranch = w.GithubBranch
	proto.WebhookSlug = w.WebhookSlug
	proto.EventFilter = w.EventFilter
	proto.LabelFilter = w.LabelFilter
	proto.PromptTemplate = w.PromptTemplate
	if w.LastFiredAt != nil {
		proto.LastFiredAt = timestamppb.New(*w.LastFiredAt)
	}
	return proto
}

// validateTriggerTypeFieldConsistency rejects a trigger_type / populated-match-field
// combination that would let a Workflow register as more than one trigger mechanism at
// once — e.g. a row that is simultaneously a valid cron entry AND has a webhook_slug
// set (Task 1.1.1e, pre-mortem P1 #2). Called with the *effective* (already-defaulted
// and, for updates, already-merged-with-the-existing-row) values.
func validateTriggerTypeFieldConsistency(triggerType string, cronEnabled bool, webhookSlug, githubRepo string) error {
	if triggerType != "cron" && cronEnabled {
		return fmt.Errorf("cron_enabled requires trigger_type=%q, got trigger_type=%q", "cron", triggerType)
	}
	if triggerType != "webhook" && webhookSlug != "" {
		return fmt.Errorf("webhook_slug requires trigger_type=%q, got trigger_type=%q", "webhook", triggerType)
	}
	if triggerType != "github_push" && githubRepo != "" {
		return fmt.Errorf("github_repo requires trigger_type=%q, got trigger_type=%q", "github_push", triggerType)
	}
	return nil
}

// resolveTriggerType applies the same "cron if cron_enabled else manual" default the
// Task 1.1.1d backfill uses, for a request that left trigger_type unspecified — so an
// existing cron-only client that has never heard of trigger_type keeps working exactly
// as before, with a correctly self-consistent row on disk (not one that relies on the
// ent schema's own "manual" column default, which would be wrong for a cron-enabled row).
func resolveTriggerType(triggerType string, cronEnabled bool) string {
	if triggerType != "" {
		return triggerType
	}
	if cronEnabled {
		return "cron"
	}
	return "manual"
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

	// Trigger-type validation (Task 1.1.1e): resolve trigger_type (defaulting for
	// backward-compat cron-only clients that never send it) and reject any request
	// whose populated match-criteria fields don't match the declared trigger_type.
	triggerType := resolveTriggerType(req.Msg.TriggerType, req.Msg.CronEnabled)
	if err := validateTriggerTypeFieldConsistency(triggerType, req.Msg.CronEnabled, req.Msg.WebhookSlug, req.Msg.GithubRepo); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
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
		TriggerType:     triggerType,
		GitHubRepo:      req.Msg.GithubRepo,
		GitHubBranch:    req.Msg.GithubBranch,
		WebhookSlug:     req.Msg.WebhookSlug,
		EventFilter:     req.Msg.EventFilter,
		LabelFilter:     req.Msg.LabelFilter,
		PromptTemplate:  req.Msg.PromptTemplate,
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

	// Trigger-type validation (Task 1.1.1e): UpdateWorkflow is a partial update, so
	// validate the *effective* post-update state — the existing row's values merged
	// with whatever this request is actually changing — not just the request's own
	// (possibly-nil) fields in isolation. Without fetching the existing row first, a
	// request that only changes an unrelated field (e.g. description) could either be
	// spuriously rejected or, worse, let a mismatched combination through unnoticed.
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("workflow %s not found", req.Msg.Id))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get workflow: %w", err))
	}
	effectiveCronEnabled := existing.CronEnabled
	if req.Msg.CronEnabled != nil {
		effectiveCronEnabled = *req.Msg.CronEnabled
	}
	effectiveWebhookSlug := existing.WebhookSlug
	if req.Msg.WebhookSlug != nil {
		effectiveWebhookSlug = *req.Msg.WebhookSlug
	}
	effectiveGitHubRepo := existing.GithubRepo
	if req.Msg.GithubRepo != nil {
		effectiveGitHubRepo = *req.Msg.GithubRepo
	}
	effectiveTriggerType := existing.TriggerType
	if req.Msg.TriggerType != nil {
		effectiveTriggerType = *req.Msg.TriggerType
	}
	effectiveTriggerType = resolveTriggerType(effectiveTriggerType, effectiveCronEnabled)
	if err := validateTriggerTypeFieldConsistency(effectiveTriggerType, effectiveCronEnabled, effectiveWebhookSlug, effectiveGitHubRepo); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
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
		TriggerType:     &effectiveTriggerType,
		GitHubRepo:      req.Msg.GithubRepo,
		GitHubBranch:    req.Msg.GithubBranch,
		WebhookSlug:     req.Msg.WebhookSlug,
		EventFilter:     req.Msg.EventFilter,
		LabelFilter:     req.Msg.LabelFilter,
		PromptTemplate:  req.Msg.PromptTemplate,
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

// +api: workflow:list-trigger-fire-events
// ListTriggerFireEvents returns the trigger-fire audit trail for a workflow, newest
// first (Epic 1.2, Task 1.2.1d). Query-only, shipped ahead of the Phase 7 UI so
// existing cron-workflow users can observe fired_failed rejections from the Epic 1.3
// admission-gate fix.
func (s *WorkflowService) ListTriggerFireEvents(
	ctx context.Context,
	req *connect.Request[sessionv1.ListTriggerFireEventsRequest],
) (*connect.Response[sessionv1.ListTriggerFireEventsResponse], error) {
	if s.fireEventRepo == nil {
		return connect.NewResponse(&sessionv1.ListTriggerFireEventsResponse{
			Events: []*sessionv1.TriggerFireEventProto{},
		}), nil
	}
	if req.Msg.WorkflowId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("workflow_id is required"))
	}
	workflowID, err := uuid.Parse(req.Msg.WorkflowId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid workflow_id: %w", err))
	}

	events, err := s.fireEventRepo.ListByWorkflow(ctx, workflowID, int(req.Msg.Limit))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list trigger fire events: %w", err))
	}

	protos := make([]*sessionv1.TriggerFireEventProto, len(events))
	for i, ev := range events {
		p := &sessionv1.TriggerFireEventProto{
			Id:           ev.ID.String(),
			Outcome:      ev.Outcome,
			DeliveryId:   ev.DeliveryID,
			SessionId:    ev.SessionID,
			ErrorMessage: ev.ErrorMessage,
			CreatedAt:    timestamppb.New(ev.CreatedAt),
		}
		if ev.WorkflowID != nil {
			p.WorkflowId = ev.WorkflowID.String()
		}
		protos[i] = p
	}

	return connect.NewResponse(&sessionv1.ListTriggerFireEventsResponse{
		Events: protos,
	}), nil
}

// +api: session:archive-workflow-sessions
// ArchiveWorkflowSessions archives all non-active sessions for a given workflow.
// Active, Creating, and Paused sessions are silently skipped.
// Extracted from SessionService per ADR-001.
func (ws *WorkflowService) ArchiveWorkflowSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.ArchiveWorkflowSessionsRequest],
) (*connect.Response[sessionv1.ArchiveWorkflowSessionsResponse], error) {
	if req.Msg.WorkflowId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("workflow_id is required"))
	}

	// Get the ent client via the concrete storage implementation.
	concreteStorage, ok := ws.storage.(*session.Storage)
	if !ok || concreteStorage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("bulk archive requires ent-backed storage"))
	}
	entClient := concreteStorage.GetEntClient()
	if entClient == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("bulk archive requires ent client"))
	}

	// Query all non-active, non-archived sessions for this workflow.
	// Status guard: use Go-layer DB values (Creating=0, Active=1, Paused=2), NOT proto wire values.
	now := time.Now()
	updated, err := entClient.Session.Update().
		Where(
			entsession.WorkflowID(req.Msg.WorkflowId),
			entsession.ArchivedAtIsNil(),
			entsession.StatusNotIn(int(session.Active), int(session.Creating), int(session.Paused)),
		).
		SetArchivedAt(now).
		Save(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("bulk archive workflow sessions: %w", err))
	}

	// Update in-memory instances for any that are still in the poller.
	if ws.poller != nil {
		for _, inst := range ws.poller.GetInstances() {
			if inst.WorkflowID == req.Msg.WorkflowId && inst.ArchivedAt == nil {
				if !inst.IsActive() && !inst.IsCreating() && !inst.IsPaused() {
					inst.ArchivedAt = &now
				}
			}
		}
	}

	log.Info("[WorkflowService] ArchiveWorkflowSessions completed",
		"workflow_id", req.Msg.WorkflowId, "archived_count", updated)

	return connect.NewResponse(&sessionv1.ArchiveWorkflowSessionsResponse{
		ArchivedCount: int32(updated),
	}), nil
}

// +api: session:delete-workflow-failed-sessions
// DeleteWorkflowFailedSessions archives (soft-deletes) sessions that appear to have
// failed — Stopped sessions with no meaningful terminal output for the given workflow.
// Extracted from SessionService per ADR-001.
func (ws *WorkflowService) DeleteWorkflowFailedSessions(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteWorkflowFailedSessionsRequest],
) (*connect.Response[sessionv1.DeleteWorkflowFailedSessionsResponse], error) {
	if req.Msg.WorkflowId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("workflow_id is required"))
	}

	concreteStorage, ok := ws.storage.(*session.Storage)
	if !ok || concreteStorage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("bulk delete requires ent-backed storage"))
	}
	entClient := concreteStorage.GetEntClient()
	if entClient == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("bulk delete requires ent client"))
	}

	// "Failed" = Stopped sessions with no meaningful output.
	// last_meaningful_output IS NULL indicates the session never produced useful work.
	now := time.Now()
	updated, err := entClient.Session.Update().
		Where(
			entsession.WorkflowID(req.Msg.WorkflowId),
			entsession.StatusIn(int(session.Stopped)),
			entsession.ArchivedAtIsNil(),
			entsession.LastMeaningfulOutputIsNil(),
		).
		SetArchivedAt(now).
		Save(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("delete workflow failed sessions: %w", err))
	}

	log.Info("[WorkflowService] DeleteWorkflowFailedSessions completed",
		"workflow_id", req.Msg.WorkflowId, "archived_count", updated)

	return connect.NewResponse(&sessionv1.DeleteWorkflowFailedSessionsResponse{
		DeletedCount: int32(updated),
	}), nil
}
