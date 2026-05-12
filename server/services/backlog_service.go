package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SessionCreator allows BacklogService to spawn sessions without importing handler internals.
type SessionCreator interface {
	CreateDirectorySession(ctx context.Context, title, path, appendSystemPrompt string, tags []string, oneShot bool) (*session.Instance, error)
}

// itemSourceBackend is a narrow interface for item source persistence; satisfied by *session.Storage.
type itemSourceBackend interface {
	CreateItemSource(ctx context.Context, data session.ItemSourceData) (*session.ItemSourceData, error)
	UpdateItemSource(ctx context.Context, id string, update session.ItemSourceUpdate) (*session.ItemSourceData, error)
}

// BacklogService handles Backlog RPCs.
type BacklogService struct {
	storage        *session.Storage
	sourceBackend  itemSourceBackend
	sessionCreator SessionCreator
	cfg            *config.Config
}

// NewBacklogService creates a BacklogService backed by Storage.
// cfg may be nil; when non-nil, tokens are encrypted before storage.
func NewBacklogService(storage *session.Storage) *BacklogService {
	return &BacklogService{storage: storage, sourceBackend: storage}
}

// NewBacklogServiceWithConfig creates a BacklogService with encryption support.
func NewBacklogServiceWithConfig(storage *session.Storage, cfg *config.Config) *BacklogService {
	return &BacklogService{storage: storage, sourceBackend: storage, cfg: cfg}
}

// NewBacklogServiceWithCreator creates a BacklogService with session spawning capability.
func NewBacklogServiceWithCreator(storage *session.Storage, creator SessionCreator) *BacklogService {
	return &BacklogService{storage: storage, sourceBackend: storage, sessionCreator: creator}
}

// NewBacklogServiceWithCreatorAndConfig creates a BacklogService with session spawning and encryption.
func NewBacklogServiceWithCreatorAndConfig(storage *session.Storage, creator SessionCreator, cfg *config.Config) *BacklogService {
	return &BacklogService{storage: storage, sourceBackend: storage, sessionCreator: creator, cfg: cfg}
}

// slugify converts s to a lowercase hyphen-delimited slug safe for file paths.
func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// itemSessionToProto converts an ent.ItemSession to its proto representation.
func itemSessionToProto(is *ent.ItemSession) *sessionv1.ItemSession {
	p := &sessionv1.ItemSession{
		Id:                    is.ID.String(),
		SessionUuid:           is.SessionUUID,
		SessionRole:           is.SessionRole,
		CommitCountSinceSpawn: int32(is.CommitCountSinceSpawn),
		LastCommitMessage:     is.LastCommitMessage,
		CreatedAt:             timestamppb.New(is.CreatedAt),
	}
	if is.StartedAt != nil {
		p.StartedAt = timestamppb.New(*is.StartedAt)
	}
	if is.EndedAt != nil {
		p.EndedAt = timestamppb.New(*is.EndedAt)
	}
	if is.LastCommitAt != nil {
		p.LastCommitAt = timestamppb.New(*is.LastCommitAt)
	}
	if is.LastFileTouchAt != nil {
		p.LastFileTouchAt = timestamppb.New(*is.LastFileTouchAt)
	}
	return p
}

// backlogItemToProto maps a BacklogItemData to the proto BacklogItem message.
func backlogItemToProto(item *session.BacklogItemData) *sessionv1.BacklogItem {
	p := &sessionv1.BacklogItem{
		Id:                item.ID,
		Title:             item.Title,
		Description:       item.Description,
		Priority:          int32(item.Priority),
		Status:            item.Status,
		RepoPath:          item.RepoPath,
		SkipReviewGate:    item.SkipReviewGate,
		SkipPlanning:      item.SkipPlanning,
		PlanApproved:      item.PlanApproved,
		PlanArtifactsPath: item.PlanArtifactsPath,
		Notes:             item.Notes,
		ExternalId:        item.ExternalID,
		SourceId:          item.SourceID,
		CreatedAt:         timestamppb.New(item.CreatedAt),
		UpdatedAt:         timestamppb.New(item.UpdatedAt),
	}
	if item.PlanApprovedAt != nil {
		p.PlanApprovedAt = timestamppb.New(*item.PlanApprovedAt)
	}
	if item.ArchivedAt != nil {
		p.ArchivedAt = timestamppb.New(*item.ArchivedAt)
	}

	// Parse acceptance criteria JSON into repeated AcCriterion.
	if item.AcceptanceCriteria != "" {
		criteria, err := session.ParseAcCriteria(item.AcceptanceCriteria)
		if err == nil {
			protoAC := make([]*sessionv1.AcCriterion, len(criteria))
			for i, c := range criteria {
				protoAC[i] = &sessionv1.AcCriterion{
					Index:  int32(c.Index),
					Text:   c.Text,
					Status: c.Status,
				}
			}
			p.AcceptanceCriteria = protoAC
		}
	}

	return p
}

// itemSourceToProto maps an ItemSourceData to the proto ItemSource message.
func itemSourceToProto(src *session.ItemSourceData) *sessionv1.ItemSource {
	p := &sessionv1.ItemSource{
		Id:              src.ID,
		PluginId:        src.PluginID,
		DisplayName:     src.DisplayName,
		Enabled:         src.Enabled,
		TokenConfigured: src.TokenConfigured,
		CreatedAt:       timestamppb.New(src.CreatedAt),
		UpdatedAt:       timestamppb.New(src.UpdatedAt),
	}
	if src.LastSyncedAt != nil {
		p.LastSyncedAt = timestamppb.New(*src.LastSyncedAt)
	}
	return p
}

// acCriteriaToJSON serializes proto AcCriterion slice to JSON string for storage.
func acCriteriaToJSON(protoAC []*sessionv1.AcCriterion) (string, error) {
	if len(protoAC) == 0 {
		return "", nil
	}
	criteria := make([]session.AcCriterion, len(protoAC))
	for i, c := range protoAC {
		criteria[i] = session.AcCriterion{
			Index:  int(c.Index),
			Text:   c.Text,
			Status: c.Status,
		}
	}
	b, err := json.Marshal(criteria)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// --- CreateBacklogItem ---

// CreateBacklogItem adds a new item to the backlog.
func (s *BacklogService) CreateBacklogItem(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateBacklogItemRequest],
) (*connect.Response[sessionv1.CreateBacklogItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.Title == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("title is required"))
	}

	acJSON, err := acCriteriaToJSON(req.Msg.AcceptanceCriteria)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid acceptance_criteria: %w", err))
	}

	priority := int(req.Msg.Priority)
	if priority == 0 {
		priority = 3 // default
	}

	data := session.BacklogItemData{
		Title:              req.Msg.Title,
		Description:        req.Msg.Description,
		AcceptanceCriteria: acJSON,
		Priority:           priority,
		Status:             string(session.BacklogStatusIdea),
		RepoPath:           req.Msg.RepoPath,
		SkipReviewGate:     req.Msg.SkipReviewGate,
		SkipPlanning:       req.Msg.SkipPlanning,
		Notes:              req.Msg.Notes,
	}

	created, err := s.storage.CreateBacklogItem(ctx, data)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create backlog item: %w", err))
	}

	return connect.NewResponse(&sessionv1.CreateBacklogItemResponse{
		Item: backlogItemToProto(created),
	}), nil
}

// --- GetBacklogItem ---

// GetBacklogItem retrieves a single backlog item by ID.
func (s *BacklogService) GetBacklogItem(
	ctx context.Context,
	req *connect.Request[sessionv1.GetBacklogItemRequest],
) (*connect.Response[sessionv1.GetBacklogItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	return connect.NewResponse(&sessionv1.GetBacklogItemResponse{
		Item: backlogItemToProto(item),
	}), nil
}

// --- ListBacklogItems ---

// ListBacklogItems returns backlog items with optional filtering and sorting.
func (s *BacklogService) ListBacklogItems(
	ctx context.Context,
	req *connect.Request[sessionv1.ListBacklogItemsRequest],
) (*connect.Response[sessionv1.ListBacklogItemsResponse], error) {
	if s.storage == nil {
		return connect.NewResponse(&sessionv1.ListBacklogItemsResponse{}), nil
	}

	filter := session.BacklogItemFilter{
		SortBy:          req.Msg.SortBy,
		ExcludeTerminal: !req.Msg.IncludeTerminal,
	}
	if len(req.Msg.Status) > 0 {
		filter.Statuses = req.Msg.Status
		filter.ExcludeTerminal = false // explicit status filter overrides default exclusion
	}
	if len(req.Msg.Priority) > 0 {
		priorities := make([]int, len(req.Msg.Priority))
		for i, p := range req.Msg.Priority {
			priorities[i] = int(p)
		}
		filter.Priorities = priorities
	}

	items, err := s.storage.ListBacklogItems(ctx, filter)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list backlog items: %w", err))
	}

	protoItems := make([]*sessionv1.BacklogItem, len(items))
	for i := range items {
		protoItems[i] = backlogItemToProto(&items[i])
	}

	return connect.NewResponse(&sessionv1.ListBacklogItemsResponse{
		Items: protoItems,
	}), nil
}

// --- UpdateBacklogItem ---

// UpdateBacklogItem modifies the properties of an existing backlog item.
func (s *BacklogService) UpdateBacklogItem(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateBacklogItemRequest],
) (*connect.Response[sessionv1.UpdateBacklogItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	acJSON, err := acCriteriaToJSON(req.Msg.AcceptanceCriteria)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid acceptance_criteria: %w", err))
	}

	update := session.BacklogItemUpdate{}
	if req.Msg.Title != "" {
		title := req.Msg.Title
		update.Title = &title
	}
	if req.Msg.Description != "" {
		desc := req.Msg.Description
		update.Description = &desc
	}
	if acJSON != "" {
		update.AcceptanceCriteria = &acJSON
	}
	if req.Msg.Priority != 0 {
		prio := int(req.Msg.Priority)
		update.Priority = &prio
	}
	if req.Msg.RepoPath != "" {
		rp := req.Msg.RepoPath
		update.RepoPath = &rp
	}
	skipRG := req.Msg.SkipReviewGate
	update.SkipReviewGate = &skipRG
	skipP := req.Msg.SkipPlanning
	update.SkipPlanning = &skipP
	if req.Msg.Notes != "" {
		notes := req.Msg.Notes
		update.Notes = &notes
	}

	var precondition *session.BacklogItemPrecondition
	if req.Msg.ExpectedStatus != "" || req.Msg.ExpectedUpdatedAt != nil {
		precondition = &session.BacklogItemPrecondition{
			ExpectedStatus: req.Msg.ExpectedStatus,
		}
		if req.Msg.ExpectedUpdatedAt != nil {
			t := req.Msg.ExpectedUpdatedAt.AsTime()
			precondition.ExpectedUpdatedAt = &t
		}
	}

	updated, err := s.storage.UpdateBacklogItem(ctx, req.Msg.ItemId, update, precondition)
	if err != nil {
		if errors.Is(err, session.ErrPreconditionFailed) {
			return nil, connect.NewError(connect.CodeAborted, err)
		}
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update backlog item: %w", err))
	}

	return connect.NewResponse(&sessionv1.UpdateBacklogItemResponse{
		Item: backlogItemToProto(updated),
	}), nil
}

// --- ArchiveBacklogItem ---

// ArchiveBacklogItem soft-deletes an item by setting its archived_at timestamp.
func (s *BacklogService) ArchiveBacklogItem(
	ctx context.Context,
	req *connect.Request[sessionv1.ArchiveBacklogItemRequest],
) (*connect.Response[sessionv1.ArchiveBacklogItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	archived, err := s.storage.ArchiveBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to archive backlog item: %w", err))
	}

	return connect.NewResponse(&sessionv1.ArchiveBacklogItemResponse{
		Item: backlogItemToProto(archived),
	}), nil
}

// --- TransitionBacklogItemStatus ---

// TransitionBacklogItemStatus moves an item through the status state machine.
func (s *BacklogService) TransitionBacklogItemStatus(
	ctx context.Context,
	req *connect.Request[sessionv1.TransitionBacklogItemStatusRequest],
) (*connect.Response[sessionv1.TransitionBacklogItemStatusResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	// Load current item to check CanTransitionBacklog.
	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	from := session.BacklogStatus(item.Status)
	to := session.BacklogStatus(req.Msg.TargetStatus)

	if !session.CanTransitionBacklog(from, to) {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid transition from %q to %q", from, to))
	}

	// Run transition guard for business rules.
	guardInput := session.BacklogItemTransitionInput{
		Status:         from,
		AcCriteriaJSON: item.AcceptanceCriteria,
		PlanApproved:   item.PlanApproved,
		SkipPlanning:   item.SkipPlanning,
		OverrideReason: req.Msg.OverrideReason,
	}
	if guardErr := session.TransitionGuard(guardInput, to); guardErr != nil {
		if errors.Is(guardErr, session.ErrACRequired) ||
			errors.Is(guardErr, session.ErrPlanRequired) ||
			errors.Is(guardErr, session.ErrVerdictRequired) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, guardErr)
		}
		return nil, connect.NewError(connect.CodeInvalidArgument, guardErr)
	}

	var precondition *session.BacklogItemPrecondition
	if req.Msg.ExpectedStatus != "" || req.Msg.ExpectedUpdatedAt != nil {
		precondition = &session.BacklogItemPrecondition{
			ExpectedStatus: req.Msg.ExpectedStatus,
		}
		if req.Msg.ExpectedUpdatedAt != nil {
			t := req.Msg.ExpectedUpdatedAt.AsTime()
			precondition.ExpectedUpdatedAt = &t
		}
	}

	updated, err := s.storage.TransitionBacklogItemStatus(ctx, req.Msg.ItemId, to, precondition)
	if err != nil {
		if errors.Is(err, session.ErrPreconditionFailed) {
			return nil, connect.NewError(connect.CodeAborted, err)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to transition backlog item: %w", err))
	}

	return connect.NewResponse(&sessionv1.TransitionBacklogItemStatusResponse{
		Item: backlogItemToProto(updated),
	}), nil
}

// --- ApprovePlan ---

// ApprovePlan marks the planning artifacts for an item as approved.
func (s *BacklogService) ApprovePlan(
	ctx context.Context,
	req *connect.Request[sessionv1.ApprovePlanRequest],
) (*connect.Response[sessionv1.ApprovePlanResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	if item.PlanArtifactsPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("no plan artifacts found — run TriggerTriage first"))
	}

	now := time.Now()
	approved := true
	update := session.BacklogItemUpdate{
		PlanApproved:   &approved,
		PlanApprovedAt: &now,
	}

	updated, err := s.storage.UpdateBacklogItem(ctx, req.Msg.ItemId, update, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to approve plan: %w", err))
	}

	return connect.NewResponse(&sessionv1.ApprovePlanResponse{
		Item: backlogItemToProto(updated),
	}), nil
}

// --- ItemSource handlers ---

// CreateItemSource registers a new external plugin source.
func (s *BacklogService) CreateItemSource(
	ctx context.Context,
	req *connect.Request[sessionv1.CreateItemSourceRequest],
) (*connect.Response[sessionv1.CreateItemSourceResponse], error) {
	if s.sourceBackend == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	data := session.ItemSourceData{
		PluginID:    req.Msg.PluginId,
		DisplayName: req.Msg.DisplayName,
		Enabled:     true,
		Config:      req.Msg.ConfigJson,
	}
	if req.Msg.Token != "" {
		data.TokenConfigured = true

		var tokenJSON string
		// Encrypt token if config is available.
		if s.cfg != nil {
			key, err := s.cfg.GetOrCreateEncryptionKey()
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get encryption key: %w", err))
			}
			encrypted, err := session.EncryptToken(key, req.Msg.Token)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("encrypt token: %w", err))
			}
			tokenJSON = fmt.Sprintf(`{"token":%q,"encrypted":true}`, encrypted)
		} else {
			// No config; store unencrypted (backwards compatibility).
			tokenJSON = fmt.Sprintf(`{"token":%q}`, req.Msg.Token)
		}

		if data.Config == "" {
			data.Config = tokenJSON
		} else {
			// Merge token into existing config JSON.
			var cfg map[string]interface{}
			if err := json.Unmarshal([]byte(data.Config), &cfg); err == nil {
				var tokCfg map[string]interface{}
				if err := json.Unmarshal([]byte(tokenJSON), &tokCfg); err == nil {
					for k, v := range tokCfg {
						cfg[k] = v
					}
					if merged, err := json.Marshal(cfg); err == nil {
						data.Config = string(merged)
					}
				}
			}
		}
	}

	created, err := s.sourceBackend.CreateItemSource(ctx, data)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create item source: %w", err))
	}

	return connect.NewResponse(&sessionv1.CreateItemSourceResponse{
		Source: itemSourceToProto(created),
	}), nil
}

// ListItemSources returns all registered external item sources.
func (s *BacklogService) ListItemSources(
	ctx context.Context,
	req *connect.Request[sessionv1.ListItemSourcesRequest],
) (*connect.Response[sessionv1.ListItemSourcesResponse], error) {
	if s.storage == nil {
		return connect.NewResponse(&sessionv1.ListItemSourcesResponse{}), nil
	}

	sources, err := s.storage.ListItemSources(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list item sources: %w", err))
	}

	protoSources := make([]*sessionv1.ItemSource, len(sources))
	for i := range sources {
		protoSources[i] = itemSourceToProto(&sources[i])
	}

	return connect.NewResponse(&sessionv1.ListItemSourcesResponse{
		Sources: protoSources,
	}), nil
}

// UpdateItemSource modifies configuration for an existing item source.
func (s *BacklogService) UpdateItemSource(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateItemSourceRequest],
) (*connect.Response[sessionv1.UpdateItemSourceResponse], error) {
	if s.sourceBackend == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	update := session.ItemSourceUpdate{}
	if req.Msg.DisplayName != "" {
		dn := req.Msg.DisplayName
		update.DisplayName = &dn
	}
	enabled := req.Msg.Enabled
	update.Enabled = &enabled
	if req.Msg.Token != "" {
		var tokenJSON string
		// Encrypt token if config is available.
		if s.cfg != nil {
			key, err := s.cfg.GetOrCreateEncryptionKey()
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get encryption key: %w", err))
			}
			encrypted, err := session.EncryptToken(key, req.Msg.Token)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("encrypt token: %w", err))
			}
			tokenJSON = fmt.Sprintf(`{"token":%q,"encrypted":true}`, encrypted)
		} else {
			// No config; store unencrypted (backwards compatibility).
			tokenJSON = fmt.Sprintf(`{"token":%q}`, req.Msg.Token)
		}
		update.Config = &tokenJSON
	}

	updated, err := s.sourceBackend.UpdateItemSource(ctx, req.Msg.SourceId, update)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("item source %q not found", req.Msg.SourceId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update item source: %w", err))
	}

	return connect.NewResponse(&sessionv1.UpdateItemSourceResponse{
		Source: itemSourceToProto(updated),
	}), nil
}

// DeleteItemSource removes an external item source registration.
func (s *BacklogService) DeleteItemSource(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteItemSourceRequest],
) (*connect.Response[sessionv1.DeleteItemSourceResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	if err := s.storage.DeleteItemSource(ctx, req.Msg.SourceId); err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("item source %q not found", req.Msg.SourceId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete item source: %w", err))
	}

	return connect.NewResponse(&sessionv1.DeleteItemSourceResponse{}), nil
}

// --- Session-linked handlers ---

// SpawnSessionFromItem creates a new AI agent session for a backlog item.
func (s *BacklogService) SpawnSessionFromItem(
	ctx context.Context,
	req *connect.Request[sessionv1.SpawnSessionFromItemRequest],
) (*connect.Response[sessionv1.SpawnSessionFromItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	// 1. Load item.
	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	// 2. Validate status is ready.
	if item.Status != string(session.BacklogStatusReady) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item must be in %q status to spawn a session, got %q", session.BacklogStatusReady, item.Status))
	}

	// 3. Planning gate.
	if !item.SkipPlanning && !item.PlanApproved {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("run TriggerTriage and approve the plan before spawning; set skip_planning=true to bypass"))
	}

	// 4. Repo path required.
	if item.RepoPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("set repo_path before spawning a session"))
	}

	// 5. Snapshot current AC.
	acSnapshot := item.AcceptanceCriteria

	// 6. Create an ItemSession record (UUID TBD — will be updated after spawn).
	is, err := s.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: "<pending>",
		SessionRole: session.SessionRoleWork,
		AcSnapshot:  acSnapshot,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create item session: %w", err))
	}

	// 7. Load prior sessions for context.
	priorSessions, err := s.storage.ListItemSessions(ctx, item.ID)
	if err != nil {
		log.WarningLog.Printf("[SpawnSessionFromItem] failed to load prior sessions for item %s: %v", item.ID, err)
		priorSessions = nil
	}

	// 8. Build agent prompt.
	// Parse item.ID as UUID for the ent struct (needed by BuildTokenBudgetedPrompt for logging).
	itemUUID, _ := uuid.Parse(item.ID)
	entItem := &ent.BacklogItem{
		ID:                 itemUUID,
		Title:              item.Title,
		Description:        item.Description,
		AcceptanceCriteria: item.AcceptanceCriteria,
		Priority:           item.Priority,
		Status:             item.Status,
		Notes:              item.Notes,
		PlanArtifactsPath:  item.PlanArtifactsPath,
		PlanApproved:       item.PlanApproved,
		SkipPlanning:       item.SkipPlanning,
	}
	prompt := session.BuildTokenBudgetedPrompt(entItem, priorSessions)
	if item.PlanArtifactsPath != "" {
		prompt += fmt.Sprintf("\nYour plan is at `%s/plan.md`. Read plan.md and validation.md before writing code.\n", item.PlanArtifactsPath)
	}

	// 9. Generate session title.
	title := "backlog:" + slugify(item.Title)

	// 10. Require SessionCreator.
	if s.sessionCreator == nil {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("SessionCreator not wired — contact admin"))
	}

	// Spawn session.
	inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, prompt,
		[]string{"backlog:work"}, false)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn session: %w", err))
	}

	// 11. Update ItemSession with real UUID.
	if updateErr := s.storage.UpdateItemSessionSessionUUID(ctx, is.ID.String(), inst.UUID); updateErr != nil {
		log.ErrorLog.Printf("[SpawnSessionFromItem] failed to update item session UUID: %v", updateErr)
	}

	// 12. Write slash commands and context file non-blocking.
	worktreePath := inst.Path
	go func() {
		if err := session.WriteSlashCommands(entItem, worktreePath); err != nil {
			log.ErrorLog.Printf("[SpawnSessionFromItem] WriteSlashCommands: %v", err)
		}
	}()
	go func() {
		if err := session.WriteBacklogContextFile(entItem, worktreePath); err != nil {
			log.ErrorLog.Printf("[SpawnSessionFromItem] WriteBacklogContextFile: %v", err)
		}
	}()

	// 13. Transition item to in_progress.
	if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusInProgress, nil); transErr != nil {
		log.ErrorLog.Printf("[SpawnSessionFromItem] failed to transition item to in_progress: %v", transErr)
	}

	// Reload updated item session.
	is.SessionUUID = inst.UUID

	return connect.NewResponse(&sessionv1.SpawnSessionFromItemResponse{
		SessionUuid: inst.UUID,
		ItemSession: itemSessionToProto(is),
	}), nil
}

// AttachSessionToItem links an existing session to a backlog item.
func (s *BacklogService) AttachSessionToItem(
	ctx context.Context,
	req *connect.Request[sessionv1.AttachSessionToItemRequest],
) (*connect.Response[sessionv1.AttachSessionToItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	// 1. Validate inputs.
	if req.Msg.ItemId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}
	if req.Msg.SessionUuid == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_uuid is required"))
	}

	// 2. Load and validate item.
	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	if item.Status != string(session.BacklogStatusIdea) && item.Status != string(session.BacklogStatusReady) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item must be in %q or %q status to attach a session, got %q",
				session.BacklogStatusIdea, session.BacklogStatusReady, item.Status))
	}

	// 3. Snapshot current AC.
	acSnapshot := item.AcceptanceCriteria

	// 4. Create ItemSession.
	is, err := s.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: req.Msg.SessionUuid,
		SessionRole: session.SessionRoleWork,
		AcSnapshot:  acSnapshot,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create item session: %w", err))
	}

	// 5. Write slash commands to session worktree if instance is reachable.
	attachItemUUID, _ := uuid.Parse(item.ID)
	entItem := &ent.BacklogItem{
		ID:                 attachItemUUID,
		Title:              item.Title,
		Description:        item.Description,
		AcceptanceCriteria: item.AcceptanceCriteria,
		Priority:           item.Priority,
		Status:             item.Status,
		Notes:              item.Notes,
	}
	instances, loadErr := s.storage.LoadInstances()
	if loadErr == nil {
		for _, inst := range instances {
			if inst.UUID == req.Msg.SessionUuid && inst.Path != "" {
				worktreePath := inst.Path
				go func() {
					if wErr := session.WriteSlashCommands(entItem, worktreePath); wErr != nil {
						log.ErrorLog.Printf("[AttachSessionToItem] WriteSlashCommands: %v", wErr)
					}
				}()
				go func() {
					if wErr := session.WriteBacklogContextFile(entItem, worktreePath); wErr != nil {
						log.ErrorLog.Printf("[AttachSessionToItem] WriteBacklogContextFile: %v", wErr)
					}
				}()
				break
			}
		}
	}

	// 6. Transition item to in_progress.
	if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusInProgress, nil); transErr != nil {
		log.ErrorLog.Printf("[AttachSessionToItem] failed to transition item to in_progress: %v", transErr)
	}

	return connect.NewResponse(&sessionv1.AttachSessionToItemResponse{
		ItemSession: itemSessionToProto(is),
	}), nil
}

// TriggerTriage kicks off a triage planning session for a backlog item.
func (s *BacklogService) TriggerTriage(
	ctx context.Context,
	req *connect.Request[sessionv1.TriggerTriageRequest],
) (*connect.Response[sessionv1.TriggerTriageResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	// 1. Load item.
	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	// 2. Repo path required.
	if item.RepoPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("set repo_path before triggering triage"))
	}

	// 3. Build slug and artifact dir path.
	slug := slugify(item.Title)
	artifactRelPath := filepath.Join("docs", "tasks", slug)
	artifactAbsPath := filepath.Join(item.RepoPath, artifactRelPath)

	// 4. Create artifact dir.
	if mkErr := os.MkdirAll(artifactAbsPath, 0o755); mkErr != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to create artifact dir %s: %w", artifactAbsPath, mkErr))
	}

	// 5. Require SessionCreator.
	if s.sessionCreator == nil {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("SessionCreator not wired — contact admin"))
	}

	// 6. Build triage prompt.
	triagePrompt := buildTriagePrompt(item, artifactRelPath, slug)

	// 7. Spawn one-shot triage session.
	title := "triage:" + slug
	inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, triagePrompt,
		[]string{"backlog:triage"}, true)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn triage session: %w", err))
	}

	// 8. Create ItemSession with role=triage.
	is, err := s.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleTriage,
		AcSnapshot:  item.AcceptanceCriteria,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create triage item session: %w", err))
	}

	log.InfoLog.Printf("[TriggerTriage] spawned triage session %s for item %s at %s", inst.UUID, item.ID, artifactAbsPath)

	return connect.NewResponse(&sessionv1.TriggerTriageResponse{
		ItemSession: itemSessionToProto(is),
	}), nil
}

// buildTriagePrompt builds the one-shot triage agent prompt.
func buildTriagePrompt(item *session.BacklogItemData, artifactRelPath, slug string) string {
	var sb strings.Builder

	sb.WriteString("You are a senior software architect performing pre-implementation triage.\n\n")
	fmt.Fprintf(&sb, "# Backlog Item: %s\n\n", item.Title)
	if item.Description != "" {
		fmt.Fprintf(&sb, "## Description\n%s\n\n", item.Description)
	}
	if item.AcceptanceCriteria != "" {
		criteria, _ := session.ParseAcCriteria(item.AcceptanceCriteria)
		if len(criteria) > 0 {
			sb.WriteString("## Acceptance Criteria\n")
			for _, c := range criteria {
				fmt.Fprintf(&sb, "%d. %s\n", c.Index, c.Text)
			}
			sb.WriteString("\n")
		}
	}

	researchDir := artifactRelPath + "/research"
	fmt.Fprintf(&sb, `## Your Task

Perform pre-implementation triage for this backlog item. Work in parallel:

### Step 1 — Research (run 4 subagents in parallel)
Each subagent writes one file:
- %s/stack.md       — Technology choices, versions, compatibility
- %s/features.md    — Similar existing features, patterns to reuse
- %s/architecture.md — Proposed architecture, component boundaries
- %s/pitfalls.md    — Known risks, gotchas, failure modes

### Step 2 — Synthesis (after research completes)
Write %s/plan.md containing:
- Executive summary (2-3 sentences)
- Implementation approach
- Task breakdown with time estimates
- Dependencies and blockers

### Step 3 — Validation
Write %s/validation.md containing:
- Test plan mapping each acceptance criterion to a specific test
- Edge cases and error scenarios

### Step 4 — Submit
After all files are written, call the update_backlog_item MCP tool:
- Set plan_artifacts_path to %q
- This notifies the operator that triage is complete and ready for review

Do not modify any source code. Only write planning documents.
`, researchDir, researchDir, researchDir, researchDir,
		artifactRelPath, artifactRelPath, artifactRelPath)

	_ = slug // used in title, kept for clarity
	return sb.String()
}

// SuggestNextItem recommends the highest-priority ready backlog item.
func (s *BacklogService) SuggestNextItem(
	ctx context.Context,
	_ *connect.Request[sessionv1.SuggestNextItemRequest],
) (*connect.Response[sessionv1.SuggestNextItemResponse], error) {
	if s.storage == nil {
		return connect.NewResponse(&sessionv1.SuggestNextItemResponse{}), nil
	}

	// Load ready items ordered by priority (lower number = higher priority).
	items, err := s.storage.ListBacklogItems(ctx, session.BacklogItemFilter{
		Statuses: []string{string(session.BacklogStatusReady)},
		SortBy:   "priority",
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list backlog items: %w", err))
	}

	if len(items) == 0 {
		// No ready items — return empty response.
		return connect.NewResponse(&sessionv1.SuggestNextItemResponse{}), nil
	}

	// Return the first (highest-priority) item wrapped in a minimal ItemSession-like container.
	// The proto response carries *ItemSession, so we synthesize a placeholder.
	top := &items[0]
	placeholder := &sessionv1.ItemSession{
		// Reuse the Id field to carry the suggested item ID for callers.
		Id:          top.ID,
		SessionRole: "suggestion",
	}

	return connect.NewResponse(&sessionv1.SuggestNextItemResponse{
		ItemSession: placeholder,
	}), nil
}

// OverrideVerdict manually overrides a review verdict for an item session.
func (s *BacklogService) OverrideVerdict(
	ctx context.Context,
	req *connect.Request[sessionv1.OverrideVerdictRequest],
) (*connect.Response[sessionv1.OverrideVerdictResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	// 1. Validate override reason.
	if req.Msg.OverrideReason == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("override_reason is required"))
	}

	// 2. Load the ItemSession to get the linked BacklogItem ID.
	is, err := s.storage.GetItemSessionBySessionUUID(ctx, req.Msg.ItemSessionId)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("item session %q not found", req.Msg.ItemSessionId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get item session: %w", err))
	}

	// Load the linked BacklogItem (edge must be loaded via GetItemSessionBySessionUUID).
	var itemID string
	if is.Edges.BacklogItem != nil {
		itemID = is.Edges.BacklogItem.ID.String()
	} else {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("item session %q has no linked backlog item", req.Msg.ItemSessionId))
	}

	// 3. Determine outcome based on to_status.
	outcome := session.ReviewVerdictPass
	if req.Msg.ToStatus == string(session.BacklogStatusInProgress) {
		outcome = session.ReviewVerdictFail
	}

	// 4. Save/upsert the ReviewVerdict with override fields.
	now := time.Now()
	if _, verdictErr := s.storage.SaveReviewVerdict(ctx, is.ID.String(), session.ReviewVerdictData{
		OverallOutcome: outcome,
		Summary:        fmt.Sprintf("Manual override: %s", req.Msg.OverrideReason),
		OverrideBy:     "user",
		OverrideReason: req.Msg.OverrideReason,
		OverrideAt:     &now,
	}); verdictErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save review verdict: %w", verdictErr))
	}

	// 5. Transition item to target status if valid.
	var updatedItem *session.BacklogItemData
	if req.Msg.ToStatus != "" {
		toStatus := session.BacklogStatus(req.Msg.ToStatus)
		updated, transErr := s.storage.TransitionBacklogItemStatus(ctx, itemID, toStatus, nil)
		if transErr != nil {
			log.ErrorLog.Printf("[OverrideVerdict] failed to transition item %s to %s: %v", itemID, toStatus, transErr)
		} else {
			updatedItem = updated
		}
	}

	// Fall back to loading item if transition was skipped or failed.
	if updatedItem == nil {
		updatedItem, err = s.storage.GetBacklogItem(ctx, itemID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to reload backlog item: %w", err))
		}
	}

	return connect.NewResponse(&sessionv1.OverrideVerdictResponse{
		Item: backlogItemToProto(updatedItem),
	}), nil
}

// TriggerReReview re-runs the review gate for a backlog item.
func (s *BacklogService) TriggerReReview(
	_ context.Context,
	_ *connect.Request[sessionv1.TriggerReReviewRequest],
) (*connect.Response[sessionv1.TriggerReReviewResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("TriggerReReview not yet implemented"))
}

// TriggerSync initiates a sync run for an external item source.
func (s *BacklogService) TriggerSync(
	_ context.Context,
	_ *connect.Request[sessionv1.TriggerSyncRequest],
) (*connect.Response[sessionv1.TriggerSyncResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("TriggerSync not yet implemented"))
}

// GetSyncHistory returns the sync event history for an item source.
func (s *BacklogService) GetSyncHistory(
	_ context.Context,
	_ *connect.Request[sessionv1.GetSyncHistoryRequest],
) (*connect.Response[sessionv1.GetSyncHistoryResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("GetSyncHistory not yet implemented"))
}
