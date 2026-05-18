package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	// worktreeMu serializes context-file writes to the same worktree path so that
	// concurrent SpawnSessionFromItem / AttachSessionToItem calls cannot produce
	// a partially-written .claude/backlog-context.md.
	worktreeMu sync.Mutex
}

// NewBacklogService creates a BacklogService with all optional dependencies.
// storage and sourceBackend are typically the same (*session.Storage).
// sessionCreator and cfg may be nil; handlers degrade gracefully when absent.
//
// Degradation contract: If creator is nil, RPCs that spawn sessions will return
// CodeUnimplemented. This is expected in test environments where a real session
// manager is unavailable.
func NewBacklogService(storage *session.Storage, creator SessionCreator, cfg *config.Config) *BacklogService {
	return &BacklogService{
		storage:        storage,
		sourceBackend:  storage,
		sessionCreator: creator,
		cfg:            cfg,
	}
}

// encryptAndMergeToken produces a token config JSON string suitable for storage.
// If key is non-nil the token is AES-GCM encrypted and the result is
// `{"token":"<ciphertext>","encrypted":true}`. Otherwise the token is stored
// unencrypted (backwards-compat). The returned string can be stored as-is when
// the existing config is empty. When existingConfig is non-empty the token JSON
// is merged into it (token fields win). Returns the merged JSON or an error.
func encryptAndMergeToken(cfg *config.Config, token, existingConfig string) (string, error) {
	var tokenJSON string
	if cfg != nil {
		key, err := cfg.GetOrCreateEncryptionKey()
		if err != nil {
			return "", fmt.Errorf("get encryption key: %w", err)
		}
		encrypted, err := session.EncryptToken(key, token)
		if err != nil {
			return "", fmt.Errorf("encrypt token: %w", err)
		}
		tokenJSON = fmt.Sprintf(`{"token":%q,"encrypted":true}`, encrypted)
	} else {
		// No config available; store unencrypted (backwards compatibility).
		tokenJSON = fmt.Sprintf(`{"token":%q}`, token)
	}

	if existingConfig == "" {
		return tokenJSON, nil
	}

	// Merge token fields into the existing config JSON.
	var cfgMap map[string]interface{}
	if err := json.Unmarshal([]byte(existingConfig), &cfgMap); err != nil {
		return "", fmt.Errorf("unmarshal existing config: %w", err)
	}
	var tokMap map[string]interface{}
	if err := json.Unmarshal([]byte(tokenJSON), &tokMap); err != nil {
		return "", fmt.Errorf("unmarshal token json: %w", err)
	}
	for k, v := range tokMap {
		cfgMap[k] = v
	}
	merged, err := json.Marshal(cfgMap)
	if err != nil {
		return "", fmt.Errorf("marshal merged config: %w", err)
	}
	return string(merged), nil
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
	// Populate the review verdict when it was eagerly loaded.
	if rv := is.Edges.ReviewVerdict; rv != nil {
		p.ReviewVerdict = &sessionv1.ReviewVerdict{
			Id:             rv.ID.String(),
			OverallOutcome: rv.OverallOutcome,
			Summary:        rv.Summary,
			DiffTokenCount: int32(rv.DiffTokenCount),
			DiffTruncated:  rv.DiffTruncated,
			OverrideBy:     rv.OverrideBy,
			OverrideReason: rv.OverrideReason,
			CreatedAt:      timestamppb.New(rv.CreatedAt),
		}
		if rv.OverrideAt != nil {
			p.ReviewVerdict.OverrideAt = timestamppb.New(*rv.OverrideAt)
		}
		// Deserialize per-criterion verdicts from JSON storage.
		if rv.PerCriterion != "" {
			var cvs []session.CriterionVerdict
			if jsonErr := json.Unmarshal([]byte(rv.PerCriterion), &cvs); jsonErr == nil {
				p.ReviewVerdict.PerCriterion = make([]*sessionv1.CriterionVerdict, len(cvs))
				for i, cv := range cvs {
					p.ReviewVerdict.PerCriterion[i] = &sessionv1.CriterionVerdict{
						CriterionIndex: int32(cv.CriterionIndex),
						Outcome:        cv.Outcome,
						Evidence:       cv.Evidence,
					}
				}
			}
		}
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

	// Populate item sessions when they were eagerly loaded.
	if len(item.ItemSessions) > 0 {
		protoSessions := make([]*sessionv1.ItemSession, len(item.ItemSessions))
		for i, is := range item.ItemSessions {
			protoSessions[i] = itemSessionToProto(is)
		}
		p.ItemSessions = protoSessions
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
// +api: backlog:create-item
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
		priority = session.DefaultBacklogPriority
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
// +api: backlog:get-item
func (s *BacklogService) GetBacklogItem(
	ctx context.Context,
	req *connect.Request[sessionv1.GetBacklogItemRequest],
) (*connect.Response[sessionv1.GetBacklogItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	// Load item sessions with review verdicts so the detail panel can show gate results.
	isSessions, isErr := s.storage.ListItemSessions(ctx, req.Msg.ItemId)
	if isErr != nil {
		log.ErrorLog.Printf("[GetBacklogItem] failed to load item sessions for %s: %v", req.Msg.ItemId, isErr)
		// Non-fatal: return item without sessions.
	} else {
		item.ItemSessions = isSessions
	}

	return connect.NewResponse(&sessionv1.GetBacklogItemResponse{
		Item: backlogItemToProto(item),
	}), nil
}

// --- ListBacklogItems ---

// ListBacklogItems returns backlog items with optional filtering and sorting.
// +api: backlog:list-items
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
// +api: backlog:update-item
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
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
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
// +api: backlog:archive-item
func (s *BacklogService) ArchiveBacklogItem(
	ctx context.Context,
	req *connect.Request[sessionv1.ArchiveBacklogItemRequest],
) (*connect.Response[sessionv1.ArchiveBacklogItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	archived, err := s.storage.ArchiveBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
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
// +api: backlog:transition-status
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
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
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

	// Load the most recent ReviewVerdict for this item so TransitionGuard can
	// evaluate the review→done guard (ErrVerdictRequired).
	overallOutcome, verdictErr := s.storage.GetMostRecentReviewVerdictForItem(ctx, req.Msg.ItemId)
	if verdictErr != nil {
		log.WarningLog.Printf("[TransitionBacklogItemStatus] failed to load review verdict for item %s: %v", req.Msg.ItemId, verdictErr)
		// Non-fatal: proceed with empty outcome; TransitionGuard will block review→done if needed.
	}

	// Run transition guard for business rules.
	guardInput := session.BacklogItemTransitionInput{
		Status:            from,
		AcCriteriaJSON:    item.AcceptanceCriteria,
		PlanApproved:      item.PlanApproved,
		SkipPlanning:      item.SkipPlanning,
		PlanArtifactsPath: item.PlanArtifactsPath,
		OverallOutcome:    overallOutcome,
		OverrideReason:    req.Msg.OverrideReason,
	}
	if guardErr := session.TransitionGuard(guardInput, to); guardErr != nil {
		if errors.Is(guardErr, session.ErrACRequired) ||
			errors.Is(guardErr, session.ErrPlanRequired) ||
			errors.Is(guardErr, session.ErrPlanArtifactsRequired) ||
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
// +api: backlog:approve-plan
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
	if _, statErr := os.Stat(item.PlanArtifactsPath); statErr != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("plan artifacts path %q does not exist on disk — re-run TriggerTriage", item.PlanArtifactsPath))
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
// +api: backlog:create-source
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
		merged, mergeErr := encryptAndMergeToken(s.cfg, req.Msg.Token, data.Config)
		if mergeErr != nil {
			return nil, connect.NewError(connect.CodeInternal, mergeErr)
		}
		data.Config = merged
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
// +api: backlog:list-sources
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
// +api: backlog:update-source
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
		// UpdateItemSource replaces the config wholesale (no prior config to merge).
		tokenJSON, mergeErr := encryptAndMergeToken(s.cfg, req.Msg.Token, "")
		if mergeErr != nil {
			return nil, connect.NewError(connect.CodeInternal, mergeErr)
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
// +api: backlog:delete-source
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
// +api: backlog:spawn-session
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

	// 5. Require SessionCreator before doing any DB writes.
	// degraded: sessionCreator unavailable — return CodeUnimplemented so callers can detect the gap.
	if s.sessionCreator == nil {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("SessionCreator not wired — contact admin"))
	}

	// 6. Snapshot current AC.
	acSnapshot := item.AcceptanceCriteria

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

	// 10. Spawn session first so we have the real UUID before creating the ItemSession record.
	inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, prompt,
		[]string{"backlog:work"}, false)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn session: %w", err))
	}

	// 11. Create ItemSession with the real session UUID (avoids "<pending>" orphan records on failure).
	is, err := s.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleWork,
		AcSnapshot:  acSnapshot,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create item session: %w", err))
	}

	// 12. Write slash commands and context file synchronously under a mutex so
	// concurrent spawn calls cannot interleave writes to the same worktree path.
	worktreePath := inst.Path
	s.worktreeMu.Lock()
	if wErr := session.WriteSlashCommands(entItem, worktreePath); wErr != nil {
		s.worktreeMu.Unlock()
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("WriteSlashCommands: %w", wErr))
	}
	if wErr := session.WriteBacklogContextFile(entItem, worktreePath); wErr != nil {
		s.worktreeMu.Unlock()
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("WriteBacklogContextFile: %w", wErr))
	}
	s.worktreeMu.Unlock()

	// 13. Transition item to in_progress.
	if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusInProgress, nil); transErr != nil {
		log.ErrorLog.Printf("[SpawnSessionFromItem] failed to transition item to in_progress: %v", transErr)
	}

	return connect.NewResponse(&sessionv1.SpawnSessionFromItemResponse{
		SessionUuid: inst.UUID,
		ItemSession: itemSessionToProto(is),
	}), nil
}

// AttachSessionToItem links an existing session to a backlog item.
// +api: backlog:attach-session
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

	if item.Status != string(session.BacklogStatusIdea) &&
		item.Status != string(session.BacklogStatusReady) &&
		item.Status != string(session.BacklogStatusInProgress) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item must be in %q, %q, or %q status to attach a session, got %q",
				session.BacklogStatusIdea, session.BacklogStatusReady, session.BacklogStatusInProgress, item.Status))
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
				// Write synchronously under mutex to prevent concurrent write races.
				s.worktreeMu.Lock()
				if wErr := session.WriteSlashCommands(entItem, worktreePath); wErr != nil {
					s.worktreeMu.Unlock()
					return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("WriteSlashCommands: %w", wErr))
				}
				if wErr := session.WriteBacklogContextFile(entItem, worktreePath); wErr != nil {
					s.worktreeMu.Unlock()
					return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("WriteBacklogContextFile: %w", wErr))
				}
				s.worktreeMu.Unlock()
				break
			}
		}
	}

	// 6. Transition item to in_progress (only if the state machine permits it).
	if session.CanTransitionBacklog(session.BacklogStatus(item.Status), session.BacklogStatusInProgress) {
		if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusInProgress, nil); transErr != nil {
			log.ErrorLog.Printf("[AttachSessionToItem] failed to transition item to in_progress: %v", transErr)
		}
	}

	return connect.NewResponse(&sessionv1.AttachSessionToItemResponse{
		ItemSession: itemSessionToProto(is),
	}), nil
}

// TriggerTriage kicks off a triage planning session for a backlog item.
// +api: backlog:trigger-triage
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

	// 2. Status guard — triage is only valid for idea or ready items.
	if item.Status != string(session.BacklogStatusIdea) && item.Status != string(session.BacklogStatusReady) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item must be in %q or %q status to trigger triage, got %q",
				session.BacklogStatusIdea, session.BacklogStatusReady, item.Status))
	}

	// 3. Repo path required.
	if item.RepoPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("set repo_path before triggering triage"))
	}

	// 4. Build slug and artifact dir path.
	slug := slugify(item.Title)
	artifactRelPath := filepath.Join("docs", "tasks", slug)
	artifactAbsPath := filepath.Join(item.RepoPath, artifactRelPath)

	// 5. Create artifact dir.
	if mkErr := os.MkdirAll(artifactAbsPath, 0o755); mkErr != nil {
		return nil, connect.NewError(connect.CodeInternal,
			fmt.Errorf("failed to create artifact dir %s: %w", artifactAbsPath, mkErr))
	}

	// 6. Require SessionCreator.
	// degraded: sessionCreator unavailable — return CodeUnimplemented so callers can detect the gap.
	if s.sessionCreator == nil {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("SessionCreator not wired — contact admin"))
	}

	// 7. Build triage prompt (use absolute path so agent submits a path os.Stat can verify).
	triagePrompt := buildTriagePrompt(item, artifactAbsPath, slug)

	// 8. Spawn one-shot triage session.
	title := "triage:" + slug
	inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, triagePrompt,
		[]string{"backlog:triage"}, true)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn triage session: %w", err))
	}

	// 9. Create ItemSession with role=triage.
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
// artifactAbsPath is the absolute path to the artifact directory on disk.
func buildTriagePrompt(item *session.BacklogItemData, artifactAbsPath, slug string) string {
	var sb strings.Builder

	sb.WriteString("You are a senior software architect performing pre-implementation triage.\n\n")
	fmt.Fprintf(&sb, "# Backlog Item: %s\n\n", item.Title)
	fmt.Fprintf(&sb, "item_id (pass this as item_id to submit_triage_result): %s\n\n", item.ID)
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

	researchDir := artifactAbsPath + "/research"
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
After all files are written, call the submit_triage_result MCP tool with:
- item_id: the backlog item UUID you were given
- plan_artifact_path: %q
- summary: a 2-3 sentence executive summary of your triage findings
- suggestions: (optional) array of improvement suggestions, each with text and rationale
This notifies the operator that triage is complete and ready for review.

Do not modify any source code. Only write planning documents.
`, researchDir, researchDir, researchDir, researchDir,
		artifactAbsPath, artifactAbsPath, artifactAbsPath)

	_ = slug // used in title, kept for clarity
	return sb.String()
}

// SuggestNextItem recommends the highest-priority ready backlog item.
// +api: backlog:suggest-next
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

	top := &items[0]
	return connect.NewResponse(&sessionv1.SuggestNextItemResponse{
		Item: backlogItemToProto(top),
	}), nil
}

// OverrideVerdict manually overrides a review verdict for an item session.
// +api: backlog:override-verdict
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

	// 2. Load the ItemSession by entity UUID to get the linked BacklogItem ID.
	is, err := s.storage.GetItemSession(ctx, req.Msg.ItemSessionId)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("item session %q not found", req.Msg.ItemSessionId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get item session: %w", err))
	}

	// Load the linked BacklogItem (edge is loaded via GetItemSession).
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

	// 5. Transition item to target status if valid (validate via state machine).
	var updatedItem *session.BacklogItemData
	if req.Msg.ToStatus != "" {
		toStatus := session.BacklogStatus(req.Msg.ToStatus)
		currentItem, currentErr := s.storage.GetBacklogItem(ctx, itemID)
		if currentErr != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load item for transition: %w", currentErr))
		}
		from := session.BacklogStatus(currentItem.Status)
		if !session.CanTransitionBacklog(from, toStatus) {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("cannot transition item from %q to %q", from, toStatus))
		}
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
// +api: backlog:trigger-re-review
func (s *BacklogService) TriggerReReview(
	ctx context.Context,
	req *connect.Request[sessionv1.TriggerReReviewRequest],
) (*connect.Response[sessionv1.TriggerReReviewResponse], error) {
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

	// 2. Validate item is in review status.
	if item.Status != string(session.BacklogStatusReview) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item must be in %q status to re-trigger review, got %q", session.BacklogStatusReview, item.Status))
	}

	// 3. Repo path required.
	if item.RepoPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("set repo_path before triggering re-review"))
	}

	// 4. Find the most recent review and work ItemSessions for this item.
	sessions, err := s.storage.ListItemSessions(ctx, item.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list item sessions: %w", err))
	}

	var mostRecentReviewSession *ent.ItemSession
	var mostRecentWorkSession *ent.ItemSession
	for _, is := range sessions {
		switch is.SessionRole {
		case session.SessionRoleReview:
			if mostRecentReviewSession == nil || is.CreatedAt.After(mostRecentReviewSession.CreatedAt) {
				mostRecentReviewSession = is
			}
		case session.SessionRoleWork:
			if mostRecentWorkSession == nil || is.CreatedAt.After(mostRecentWorkSession.CreatedAt) {
				mostRecentWorkSession = is
			}
		}
	}

	// 5. Note: We don't need to delete the old verdict; a new one will overwrite it when the re-review
	// session submits its findings via the MCP tool.

	// 6. Get git diff from the most recent work session (use HEAD~1..HEAD if no commit SHA tracked).
	var workSessionDiff string
	if mostRecentWorkSession != nil {
		fromSHA := mostRecentWorkSession.LastCommitSha
		if fromSHA == "" {
			fromSHA = "HEAD~1"
		}
		diff, _, diffErr := session.GetGitDiff(ctx, item.RepoPath, fromSHA)
		if diffErr != nil {
			log.WarningLog.Printf("[TriggerReReview] GetGitDiff failed: %v", diffErr)
		} else {
			workSessionDiff = diff
		}
	}

	// 7. Deserialize AC snapshot (from most recent work session or item AC).
	var acSnapshot []session.AcCriterion
	if mostRecentWorkSession != nil && mostRecentWorkSession.AcSnapshot != "" {
		acSnapshot, _ = session.ParseAcCriteria(mostRecentWorkSession.AcSnapshot)
	}
	if len(acSnapshot) == 0 {
		acSnapshot, _ = session.ParseAcCriteria(item.AcceptanceCriteria)
	}

	// 8. Build re-review prompt.
	acSnapshotJSON, _ := json.Marshal(acSnapshot)

	priorVerdictSection := ""
	if mostRecentReviewSession != nil && mostRecentReviewSession.Edges.ReviewVerdict != nil {
		rv := mostRecentReviewSession.Edges.ReviewVerdict
		priorVerdictSection = fmt.Sprintf("\n## Prior Review Verdict\nOutcome: %s\nSummary: %s\n", rv.OverallOutcome, rv.Summary)
	}

	reReviewPrompt := fmt.Sprintf(`You are re-reviewing a backlog item that previously entered the review state.

# Item: %s

## Description
%s
%s
## Acceptance Criteria (at time of work session)
`, item.Title, item.Description, priorVerdictSection)

	for _, ac := range acSnapshot {
		reReviewPrompt += fmt.Sprintf("%d. %s (status: %s)\n", ac.Index, ac.Text, ac.Status)
	}

	reReviewPrompt += fmt.Sprintf(`
## Recent Changes
The work session made the following changes to the codebase:

%s

## Your Task
Perform a comprehensive review and submit your verdict using the submit_review_verdict MCP tool:
- Assess each acceptance criterion listed above
- Evaluate the diff against the requirements
- For each criterion provide: criterion_index, outcome (PASS/FAIL/PARTIAL), evidence

Call submit_review_verdict with:
  item_id: "%s"
  summary: "<overall summary of your findings>"
  verdicts: [{"criterion_index": N, "outcome": "PASS|FAIL|PARTIAL", "evidence": "<specific evidence>"}]

Do not modify the code. Only write the review verdict.
`, workSessionDiff, item.ID)

	// 9. Require SessionCreator to spawn review session.
	// degraded: sessionCreator unavailable — return a placeholder response so the
	// caller knows re-review was acknowledged, even without a live session spawner.
	if s.sessionCreator == nil {
		// No spawner configured; just return a placeholder indicating re-review was triggered.
		log.InfoLog.Printf("[TriggerReReview] triggered for item %s but no SessionCreator available", item.ID)
		return connect.NewResponse(&sessionv1.TriggerReReviewResponse{
			ItemSession: &sessionv1.ItemSession{
				Id:          item.ID,
				SessionRole: "re-review-triggered",
			},
		}), nil
	}

	// 10. Spawn one-shot re-review session.
	slug := slugify(item.Title)
	title := "re-review:" + slug
	inst, spawnErr := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, reReviewPrompt,
		[]string{"backlog:review"}, true)
	if spawnErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn re-review session: %w", spawnErr))
	}

	// 11. Create ItemSession with role=review.
	is, err := s.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: inst.UUID,
		SessionRole: session.SessionRoleReview,
		AcSnapshot:  string(acSnapshotJSON),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create re-review item session: %w", err))
	}

	log.InfoLog.Printf("[TriggerReReview] spawned re-review session %s for item %s", inst.UUID, item.ID)

	return connect.NewResponse(&sessionv1.TriggerReReviewResponse{
		ItemSession: itemSessionToProto(is),
	}), nil
}

// TriggerSync initiates a sync run for an external item source.
// +api: backlog:trigger-sync
func (s *BacklogService) TriggerSync(
	_ context.Context,
	_ *connect.Request[sessionv1.TriggerSyncRequest],
) (*connect.Response[sessionv1.TriggerSyncResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("TriggerSync not yet implemented"))
}

// GetSyncHistory returns the sync event history for an item source.
// +api: backlog:get-sync-history
func (s *BacklogService) GetSyncHistory(
	_ context.Context,
	_ *connect.Request[sessionv1.GetSyncHistoryRequest],
) (*connect.Response[sessionv1.GetSyncHistoryResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("GetSyncHistory not yet implemented"))
}
