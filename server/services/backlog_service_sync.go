package services

// backlog_service_sync.go — external source sync and session attachment handlers for BacklogService

import (
	"context"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
)

// defaultTriggerSyncTimeout bounds a single manual TriggerSync RPC call. The
// GitHub PRs plugin issues one extra HTTP call per open PR (for CI status), so
// this is generous relative to the "seconds, not minutes" expectation for a
// single page of items — without it, a slow/rate-limited GitHub response would
// block the RPC handler for however long the client's transport allows.
const defaultTriggerSyncTimeout = 2 * time.Minute

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

	if item.Status != string(session.BacklogStatusIdea) &&
		item.Status != string(session.BacklogStatusReady) &&
		item.Status != string(session.BacklogStatusInProgress) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item must be in %q, %q, or %q status to attach a session, got %q",
				session.BacklogStatusIdea, session.BacklogStatusReady, session.BacklogStatusInProgress, item.Status))
	}

	// 3. Snapshot current AC.
	acSnapshot := item.AcceptanceCriteria

	// 4. Load prior sessions BEFORE creating this attach's own ItemSession, so the
	// "prior sessions" list passed to WriteBacklogContextFile never transiently includes
	// the session being attached (mirrors SpawnSessionFromItem's ordering).
	attachPriorSessions, priorErr := s.storage.ListItemSessions(ctx, item.ID)
	if priorErr != nil {
		log.WarningLog.Printf("[AttachSessionToItem] failed to load prior sessions for item %s: %v", item.ID, priorErr)
		attachPriorSessions = nil
	}

	// 5. Create ItemSession.
	is, err := s.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: req.Msg.SessionUuid,
		SessionRole: session.SessionRoleWork,
		AcSnapshot:  acSnapshot,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create item session: %w", err))
	}

	// 6. Write slash commands to session worktree if instance is reachable.

	instances, loadErr := s.storage.LoadInstances()
	if loadErr == nil {
		for _, inst := range instances {
			if inst.UUID == req.Msg.SessionUuid && inst.Path != "" {
				worktreePath := inst.GetEffectiveRootDir()
				// Write synchronously under mutex to prevent concurrent write races.
				s.worktreeMu.Lock()
				if wErr := session.WriteSlashCommands(s.pipelineEngine, item, worktreePath); wErr != nil {
					s.worktreeMu.Unlock()
					return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("WriteSlashCommands: %w", wErr))
				}
				if wErr := session.WriteBacklogContextFile(item, attachPriorSessions, worktreePath); wErr != nil {
					s.worktreeMu.Unlock()
					return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("WriteBacklogContextFile: %w", wErr))
				}
				s.worktreeMu.Unlock()
				// Capture pre-work HEAD SHA so the review gate can diff base..HEAD
				// across all commits the agent makes (same as SpawnSessionFromItem step 12b).
				if baseSHA, shaErr := session.GetGitHeadSHA(worktreePath); shaErr == nil && baseSHA != "" {
					_ = s.storage.UpdateItemSessionGitActivity(ctx, is.ID, baseSHA, "", time.Now(), 0)
					inst.SetDirBaseSHA(baseSHA)
				}
				// Persist synchronously so the review gate's worktree lookup (by session
				// UUID) doesn't race the next periodic SaveInstances sweep — same fix as
				// SpawnSessionFromItem.
				if saveErr := s.storage.SaveInstances([]*session.Instance{inst}); saveErr != nil {
					log.WarningLog.Printf("[AttachSessionToItem] failed to persist instance immediately after attach item=%s session=%s: %v", item.ID, inst.UUID, saveErr)
				}
				break
			}
		}
	}

	// 7. Transition item to in_progress (only if the state machine permits it).
	if session.CanTransitionBacklog(session.BacklogStatus(item.Status), session.BacklogStatusInProgress) {
		if _, transErr := s.storage.TransitionBacklogItemStatus(ctx, item.ID, session.BacklogStatusInProgress, nil); transErr != nil {
			log.ErrorLog.Printf("[AttachSessionToItem] failed to transition item to in_progress: %v", transErr)
		}
	}

	return connect.NewResponse(&sessionv1.AttachSessionToItemResponse{
		ItemSession: itemSessionToProto(is, s.buildCostLookup()),
	}), nil
}

// TriggerSync initiates a synchronous, on-demand sync run for an external item
// source, regardless of its Enabled flag. Runs inline (not backgrounded like
// TriggerTriage) because a single external-API fetch is expected to complete
// in seconds, not the 7-15 minutes a headless LLM triage call takes.
// +api: backlog:trigger-sync
func (s *BacklogService) TriggerSync(
	ctx context.Context,
	req *connect.Request[sessionv1.TriggerSyncRequest],
) (*connect.Response[sessionv1.TriggerSyncResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if s.pluginRegistry == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("sync not configured — no plugin registry wired"))
	}
	if s.syncFeatureEnabled != nil && !s.syncFeatureEnabled() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("backlog sync is disabled"))
	}
	if req.Msg.SourceId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("source_id is required"))
	}
	if _, parseErr := uuid.Parse(req.Msg.SourceId); parseErr != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid source_id %q: %w", req.Msg.SourceId, parseErr))
	}

	var sl *session.SyncLoop
	if s.syncKeyFunc != nil {
		sl = session.NewSyncLoopWithKeyProvider(s.storage, s.pluginRegistry, s.syncKeyFunc)
	} else {
		sl = session.NewSyncLoop(s.storage, s.pluginRegistry)
	}

	syncCtx, cancel := context.WithTimeout(ctx, defaultTriggerSyncTimeout)
	defer cancel()

	if err := sl.SyncByID(syncCtx, req.Msg.SourceId); err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("item source %q not found", req.Msg.SourceId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("sync failed: %w", err))
	}

	return connect.NewResponse(&sessionv1.TriggerSyncResponse{}), nil
}

func (s *BacklogService) ImportGitHubIssue(ctx context.Context, req *connect.Request[sessionv1.ImportGitHubIssueRequest]) (*connect.Response[sessionv1.ImportGitHubIssueResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("ImportGitHubIssue not yet implemented"))
}
