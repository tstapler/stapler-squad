package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/tokens"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// wireAutoArchiveCallback registers a lifecycle listener that auto-archives a
// workflow-spawned or backlog-spawned session when it exits. Backlog sessions are
// included because BacklogService's own archiveItemWorkSessions only fires when
// the owning item reaches a terminal status or gets reworked — a session whose
// item never reaches either (stuck, orphaned, or manually abandoned) would
// otherwise never get ArchivedAt set and would accumulate forever, since
// SessionRetentionSweeper only ever considers archived sessions.
func (s *SessionService) wireAutoArchiveCallback(inst *session.Instance) {
	if inst == nil {
		return
	}
	if inst.WorkflowID == "" && !inst.IsBacklogOriginatedSession() {
		return
	}
	inst.RegisterLifecycleListener(&autoArchiveListener{svc: s, inst: inst})
}

// autoArchiveListener implements session.LifecycleListener to archive workflow sessions on exit.
type autoArchiveListener struct {
	svc  *SessionService
	inst *session.Instance
}

func (l *autoArchiveListener) OnLifecycleEvent(event session.LifecycleEvent, _ string) {
	if event == session.EventExited {
		go l.svc.maybeAutoArchive(l.inst)
	}
}

// wireColdRestoreOutcomeListener registers a lifecycle listener that notifies
// the user when a cold restore was forced fresh despite the session having
// previously captured conversation history (session-revive-uuid-loss AC3).
func (s *SessionService) wireColdRestoreOutcomeListener(inst *session.Instance) {
	if inst == nil {
		return
	}
	inst.RegisterLifecycleListener(&coldRestoreOutcomeListener{svc: s, inst: inst})
}

// coldRestoreOutcomeListener implements session.LifecycleListener to surface
// session.ReasonColdRestoreLostHistory as a durable, user-visible notification.
type coldRestoreOutcomeListener struct {
	svc  *SessionService
	inst *session.Instance
}

func (l *coldRestoreOutcomeListener) OnLifecycleEvent(event session.LifecycleEvent, reason string) {
	if event == session.EventStarted && reason == session.ReasonColdRestoreLostHistory {
		l.svc.onColdRestoreLostHistory(l.inst)
	}
}

// onColdRestoreLostHistory publishes a durable WARNING notification for inst
// when a cold restore could not recover its previous conversation history.
// Hidden instances (e.g. headless review sessions) never surface this.
func (s *SessionService) onColdRestoreLostHistory(inst *session.Instance) {
	if inst.Hidden {
		return
	}
	linkedItemID := s.rateLimitLinkedItemID(inst)
	notifID := fmt.Sprintf("cold-restore-lost-history-%s", inst.UUID)
	s.eventBus.Publish(events.NewNotificationEvent(
		inst.UUID, inst.Title, notifID,
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
		derivePriority(false, true), // urgent, important — real context loss, but not a drop-everything alert
		fmt.Sprintf("Session %q started fresh — previous conversation could not be resumed", inst.Title),
		"The session's tmux pane restarted and the previous conversation history could not be found on disk. Earlier context is not available.",
		events.SessionScopedMetadata(nil, linkedItemID),
	))
}

// wireSessionExitedPublisher registers a lifecycle listener that publishes a
// SessionUpdatedEvent whenever a session exits unexpectedly (PTY EOF, process
// crash, reconcile). Without this, the frontend WatchSessions stream never
// learns the session stopped, leaving the "Thinking…" chip visible indefinitely.
func (s *SessionService) wireSessionExitedPublisher(inst *session.Instance) {
	if inst == nil {
		return
	}
	inst.RegisterLifecycleListener(&sessionExitedPublisher{svc: s, inst: inst})
}

// sessionExitedPublisher publishes a SessionUpdatedEvent and saves the instance
// when a session exits so the frontend receives the updated Stopped status.
type sessionExitedPublisher struct {
	svc  *SessionService
	inst *session.Instance
}

func (l *sessionExitedPublisher) OnLifecycleEvent(event session.LifecycleEvent, _ string) {
	if event != session.EventExited {
		return
	}
	go func() {
		_ = l.svc.storage.SaveInstances([]*session.Instance{l.inst})
		l.svc.eventBus.Publish(events.NewSessionUpdatedEvent(l.inst, []string{"status"}))
	}()
}

// wireStatusChangeCallback registers a ReactiveQueueManager callback on inst so that
// ClaudeController status transitions immediately trigger a CheckSession call, bypassing
// the poll cycle. Also publishes a session update event so WatchSessions clients receive
// the detection state change without waiting for the next poll cycle.
// Safe to call before or after the controller is started.
func (s *SessionService) wireStatusChangeCallback(inst *session.Instance) {
	if inst == nil || s.reviewQueueSvc == nil {
		return
	}
	mgr := s.reviewQueueSvc.GetReactiveQueueManager()
	if mgr == nil {
		return
	}
	inst.SetStatusChangeCallback(func(newStatus detection.DetectedStatus, context string) {
		mgr.OnControllerStatusChange(inst, newStatus)
		s.eventBus.Publish(events.NewSessionUpdatedEventWithDetection(
			inst, []string{"detected_status"},
			newStatus, context,
		))
	})
}

// rateLimitLookupTimeout bounds the ItemSession lookup performed by
// onRateLimitDetected/onRateLimitRecovery to resolve a Hidden, backlog-linked
// session's item_id for notification metadata. These callbacks run from
// goroutines in the ratelimit package, so an unbounded lookup could otherwise
// hang that goroutine indefinitely on a slow/stuck storage backend.
const rateLimitLookupTimeout = 2 * time.Second

// wireRateLimitCallbacks registers server-level callbacks on an Instance so that
// rate-limit detection and recovery events are published to the event bus and
// trigger desktop push notifications.
func (s *SessionService) wireRateLimitCallbacks(inst *session.Instance) {
	if inst == nil {
		return
	}
	inst.SetRateLimitCallbacks(
		func(sessionID string, resetTime time.Time) {
			s.onRateLimitDetected(inst, sessionID, resetTime)
		},
		func(sessionID string, success bool, errMsg string) {
			if success {
				s.onRateLimitRecoverySucceeded(inst, sessionID)
			} else {
				s.onRateLimitRecoveryFailed(inst, sessionID, errMsg)
			}
		},
	)
}

// rateLimitLinkedItemID looks up the backlog item ID linked to inst's session
// (via concStorage), bounded by rateLimitLookupTimeout. Returns "" when the
// instance isn't backlog-linked, when concStorage is nil (fake InstanceStore
// backing, e.g. in some test setups), or when the lookup fails/times out.
func (s *SessionService) rateLimitLinkedItemID(inst *session.Instance) string {
	if s.concStorage == nil {
		return ""
	}
	lookupCtx, cancel := context.WithTimeout(context.Background(), rateLimitLookupTimeout)
	defer cancel()
	itemSession, err := s.concStorage.GetItemSessionBySessionUUID(lookupCtx, inst.UUID)
	if err != nil {
		if !errors.Is(err, session.ErrNotFound) {
			log.Warn("wireRateLimitCallbacks: ItemSession lookup failed", "session", inst.UUID, "err", err)
		}
		return ""
	}
	return itemSession.BacklogItemID
}

// onRateLimitDetected publishes the rate-limit-detected notification for inst.
// Hidden instances (e.g. headless review sessions) never surface a
// notification for this.
func (s *SessionService) onRateLimitDetected(inst *session.Instance, sessionID string, resetTime time.Time) {
	if !inst.Hidden {
		linkedItemID := s.rateLimitLinkedItemID(inst)

		var resetMsg string
		if !resetTime.IsZero() {
			resetMsg = fmt.Sprintf(" — resumes at %s", resetTime.Format("3:04 PM"))
		}
		title := fmt.Sprintf("Session \"%s\" rate limited%s", inst.Title, resetMsg)
		notifID := fmt.Sprintf("rl-detect-%s", sessionID)
		s.eventBus.Publish(events.NewNotificationEvent(
			sessionID, inst.Title, notifID,
			int32(8),                   // NotificationType_WARNING
			derivePriority(true, true), // urgent, important — the session just stopped making progress right now
			title,
			fmt.Sprintf("Session hit the usage limit%s.", resetMsg),
			events.SessionScopedMetadata(nil, linkedItemID),
		))
	}
	// Session state sync (rate_limit_state/rate_limit_reset_time) must fire
	// regardless of Hidden — only the Notifications-page entry above is gated.
	s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"rate_limit_state", "rate_limit_reset_time"}))

	// Feed the account-wide quota gate's hard/reactive override signal. Not
	// gated on inst.Hidden (unlike the notification above) — a rate limit hit
	// by a hidden/headless session still consumes real account quota.
	if s.quotaGate != nil {
		s.quotaGate.recordRateLimitEvent(time.Now())
	}
}

// onRateLimitRecoverySucceeded publishes the rate-limit-recovery notification
// for inst's successful auto-resume. Hidden instances (e.g. headless review
// sessions) never surface a notification for this, but the session-state
// sync below still fires regardless of Hidden.
//
// Split from onRateLimitRecoveryFailed below (Fowler's Remove Flag Argument
// -- the two used to be one onRateLimitRecovery(..., success bool, ...)
// function branching on success internally); the wireRateLimitCallbacks
// dispatch closure picks the function that matches the outcome instead of
// passing a flag.
func (s *SessionService) onRateLimitRecoverySucceeded(inst *session.Instance, sessionID string) {
	if !inst.Hidden {
		linkedItemID := s.rateLimitLinkedItemID(inst)
		s.eventBus.Publish(events.NewNotificationEvent(
			sessionID, inst.Title, fmt.Sprintf("rl-recover-%s", sessionID),
			int32(10), // NotificationType_INFO
			derivePriority(false, false),
			fmt.Sprintf("Session \"%s\" resumed after rate limit", inst.Title),
			"Session auto-resumed after rate limit expiry.",
			events.SessionScopedMetadata(nil, linkedItemID),
		))
	}
	s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"rate_limit_state"}))
}

// onRateLimitRecoveryFailed is onRateLimitRecoverySucceeded's failure
// counterpart -- see its doc comment. errMsg is the auto-resume failure
// reason, surfaced in the notification body.
func (s *SessionService) onRateLimitRecoveryFailed(inst *session.Instance, sessionID, errMsg string) {
	if !inst.Hidden {
		linkedItemID := s.rateLimitLinkedItemID(inst)
		s.eventBus.Publish(events.NewNotificationEvent(
			sessionID, inst.Title, fmt.Sprintf("rl-recover-%s", sessionID),
			int32(9), // NotificationType_FAILURE
			derivePriority(true, true),
			fmt.Sprintf("Session \"%s\" failed to resume after rate limit", inst.Title),
			fmt.Sprintf("Auto-resume failed: %s", errMsg),
			events.SessionScopedMetadata(nil, linkedItemID),
		))
	}
	s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"rate_limit_state"}))
}

// wireClaudeSessionIDCallback registers a callback on inst so that when the
// session driver captures a Claude session_id, the instance is persisted.
func (s *SessionService) wireClaudeSessionIDCallback(inst *session.Instance) {
	if inst == nil {
		return
	}
	inst.SetClaudeSessionIDSavedCallback(func() {
		_ = s.storage.SaveInstances([]*session.Instance{inst})
	})
}

// logClientEntry writes a single browser log entry to the server log.
func logClientEntry(e *sessionv1.ClientLogEntry) {
	msg := sanitizeClientLogField(e.GetMessage(), 200)
	ua := sanitizeClientLogField(e.GetUserAgent(), 80)
	sid := sanitizeClientLogField(e.GetSessionId(), 64)
	lvl := sanitizeClientLogField(e.GetLevel(), 16)
	url := sanitizeClientLogField(e.GetUrl(), 256)

	args := []any{"level", lvl, "session", sid, "url", url, "ua", ua}
	if lvl == "error" {
		log.Error("[client-log] "+msg, args...)
	} else {
		log.Info("[client-log] "+msg, args...)
	}
}

// sanitizeClientLogField strips control characters and truncates to maxLen runes.
func sanitizeClientLogField(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "…"
	}
	return s
}

// +api: errors:list
// ListErrors returns persisted RPC error events from SQLite, ordered by last_seen desc.
func (s *SessionService) ListErrors(
	ctx context.Context,
	req *connect.Request[sessionv1.ListErrorsRequest],
) (*connect.Response[sessionv1.ListErrorsResponse], error) {
	if s.errorRegistry == nil {
		return connect.NewResponse(&sessionv1.ListErrorsResponse{}), nil
	}
	events, err := s.errorRegistry.List(ctx, req.Msg.GetIncludeAcknowledged())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	records := make([]*sessionv1.ErrorEventRecord, 0, len(events))
	for _, e := range events {
		rec := &sessionv1.ErrorEventRecord{
			Fingerprint:     e.Fingerprint,
			ErrorType:       e.ErrorType,
			Message:         e.Message,
			StackTrace:      e.StackTrace,
			RpcProcedure:    e.RPCProcedure,
			OccurrenceCount: int32(e.OccurrenceCount), //#nosec G115 -- error occurrence count, bounded well under int32 max
			Acknowledged:    e.Acknowledged,
		}
		if !e.FirstSeen.IsZero() {
			rec.FirstSeen = timestamppb.New(e.FirstSeen)
		}
		if !e.LastSeen.IsZero() {
			rec.LastSeen = timestamppb.New(e.LastSeen)
		}
		records = append(records, rec)
	}
	return connect.NewResponse(&sessionv1.ListErrorsResponse{Errors: records}), nil
}

// +api: errors:acknowledge
// AcknowledgeError marks a persisted error event as acknowledged.
func (s *SessionService) AcknowledgeError(
	ctx context.Context,
	req *connect.Request[sessionv1.AcknowledgeErrorRequest],
) (*connect.Response[sessionv1.AcknowledgeErrorResponse], error) {
	if s.errorRegistry == nil {
		return connect.NewResponse(&sessionv1.AcknowledgeErrorResponse{}), nil
	}
	if err := s.errorRegistry.Acknowledge(ctx, req.Msg.GetFingerprint()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&sessionv1.AcknowledgeErrorResponse{}), nil
}

// +api: feature-flags:list
// GetFeatureFlags returns all known feature flags and their current state.
func (s *SessionService) GetFeatureFlags(
	ctx context.Context,
	req *connect.Request[sessionv1.GetFeatureFlagsRequest],
) (*connect.Response[sessionv1.GetFeatureFlagsResponse], error) {
	return s.featureFlagSvc.GetFeatureFlags(ctx, req)
}

// +api: feature-flags:update
// UpdateFeatureFlag enables or disables a named feature flag and persists the change.
func (s *SessionService) UpdateFeatureFlag(
	ctx context.Context,
	req *connect.Request[sessionv1.UpdateFeatureFlagRequest],
) (*connect.Response[sessionv1.UpdateFeatureFlagResponse], error) {
	return s.featureFlagSvc.UpdateFeatureFlag(ctx, req)
}

// SetWorkflowService injects the workflow sub-service using deferred setter injection.
// Must be called after both SessionService and WorkflowService are constructed.
func (s *SessionService) SetWorkflowService(svc *WorkflowService) {
	s.workflowSvc = svc
}

// SetWorkflowRepository injects the workflow repository used to populate the meta cache.
// Must be called after both SessionService and WorkflowRepository are constructed.
func (s *SessionService) SetWorkflowRepository(repo session.WorkflowRepository) {
	s.workflowRepo = repo
	s.refreshWorkflowMetaCache(context.Background())
}

// refreshWorkflowMetaCache reloads all workflow names and archiveAfterHours from the repo.
func (s *SessionService) refreshWorkflowMetaCache(ctx context.Context) {
	if s.workflowRepo == nil {
		return
	}
	wfs, err := s.workflowRepo.ListAll(ctx)
	if err != nil {
		log.Warn("[SessionService] failed to refresh workflow meta cache", "err", err)
		return
	}
	cache := make(map[string]workflowMeta, len(wfs))
	for _, wf := range wfs {
		cache[wf.ID.String()] = workflowMeta{
			name:              wf.Name,
			archiveAfterHours: wf.ArchiveAfterHours,
		}
	}
	s.workflowMetaMu.Lock()
	s.workflowMetaCache = cache
	s.workflowMetaMu.Unlock()
}

// workflowNames returns a snapshot of the workflow ID→name map for use in InstanceToProto.
func (s *SessionService) workflowNames() map[string]string {
	s.workflowMetaMu.RLock()
	defer s.workflowMetaMu.RUnlock()
	if len(s.workflowMetaCache) == 0 {
		return nil
	}
	m := make(map[string]string, len(s.workflowMetaCache))
	for id, meta := range s.workflowMetaCache {
		m[id] = meta.name
	}
	return m
}

// +api: workflow:create
// CreateWorkflow delegates to WorkflowService.
func (s *SessionService) CreateWorkflow(ctx context.Context, req *connect.Request[sessionv1.CreateWorkflowRequest]) (*connect.Response[sessionv1.CreateWorkflowResponse], error) {
	if s.workflowSvc == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow service not available"))
	}
	return s.workflowSvc.CreateWorkflow(ctx, req)
}

// +api: workflow:update
// UpdateWorkflow delegates to WorkflowService.
func (s *SessionService) UpdateWorkflow(ctx context.Context, req *connect.Request[sessionv1.UpdateWorkflowRequest]) (*connect.Response[sessionv1.UpdateWorkflowResponse], error) {
	if s.workflowSvc == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow service not available"))
	}
	return s.workflowSvc.UpdateWorkflow(ctx, req)
}

// +api: workflow:delete
// DeleteWorkflow delegates to WorkflowService.
func (s *SessionService) DeleteWorkflow(ctx context.Context, req *connect.Request[sessionv1.DeleteWorkflowRequest]) (*connect.Response[sessionv1.DeleteWorkflowResponse], error) {
	if s.workflowSvc == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow service not available"))
	}
	return s.workflowSvc.DeleteWorkflow(ctx, req)
}

// +api: workflow:list
// ListWorkflows delegates to WorkflowService.
func (s *SessionService) ListWorkflows(ctx context.Context, req *connect.Request[sessionv1.ListWorkflowsRequest]) (*connect.Response[sessionv1.ListWorkflowsResponse], error) {
	if s.workflowSvc == nil {
		return connect.NewResponse(&sessionv1.ListWorkflowsResponse{
			Workflows: []*sessionv1.WorkflowProto{},
		}), nil
	}
	return s.workflowSvc.ListWorkflows(ctx, req)
}

// +api: workflow:run
// RunWorkflow delegates to WorkflowService.
func (s *SessionService) RunWorkflow(ctx context.Context, req *connect.Request[sessionv1.RunWorkflowRequest]) (*connect.Response[sessionv1.RunWorkflowResponse], error) {
	if s.workflowSvc == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow service not available"))
	}
	return s.workflowSvc.RunWorkflow(ctx, req)
}

// +api: workflow:watch
// WatchWorkflows delegates to WorkflowService.
func (s *SessionService) WatchWorkflows(ctx context.Context, req *connect.Request[sessionv1.WatchWorkflowsRequest], stream *connect.ServerStream[sessionv1.WorkflowEvent]) error {
	if s.workflowSvc == nil {
		return connect.NewError(connect.CodeUnavailable, fmt.Errorf("workflow service not available"))
	}
	return s.workflowSvc.WatchWorkflows(ctx, req, stream)
}

// +api: workflow:list-trigger-fire-events
// ListTriggerFireEvents delegates to WorkflowService.
func (s *SessionService) ListTriggerFireEvents(ctx context.Context, req *connect.Request[sessionv1.ListTriggerFireEventsRequest]) (*connect.Response[sessionv1.ListTriggerFireEventsResponse], error) {
	if s.workflowSvc == nil {
		return connect.NewResponse(&sessionv1.ListTriggerFireEventsResponse{
			Events: []*sessionv1.TriggerFireEventProto{},
		}), nil
	}
	return s.workflowSvc.ListTriggerFireEvents(ctx, req)
}

// GetDetectionEvents returns recent status-detection events for a session's Claude controller.
// Used by the debug panel (FR-8) — returns an empty list when the session has no active controller.
func (s *SessionService) GetDetectionEvents(ctx context.Context, req *connect.Request[sessionv1.GetDetectionEventsRequest]) (*connect.Response[sessionv1.GetDetectionEventsResponse], error) {
	inst := s.findInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session %q not found", req.Msg.SessionId))
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	if s.statusManager == nil {
		return connect.NewResponse(&sessionv1.GetDetectionEventsResponse{}), nil
	}

	controller, ok := s.statusManager.GetController(inst.Title)
	if !ok || controller == nil {
		return connect.NewResponse(&sessionv1.GetDetectionEventsResponse{}), nil
	}

	events := controller.GetStatusDetector().RecentEvents(limit)
	protoEvents := make([]*sessionv1.DetectionEventProto, 0, len(events))
	for _, e := range events {
		protoEvents = append(protoEvents, &sessionv1.DetectionEventProto{
			SessionId:       e.SessionID,
			Timestamp:       timestamppb.New(e.Timestamp),
			MatchedPattern:  e.MatchedPattern,
			MatchedCategory: e.MatchedCategory,
			ResultStatus:    int32(e.ResultStatus), //#nosec G115 -- small enum value (DetectedStatus)
		})
	}
	return connect.NewResponse(&sessionv1.GetDetectionEventsResponse{Events: protoEvents}), nil
}

// +api: session:archive
// ArchiveSession soft-archives a session by setting archived_at.
// Archived sessions are excluded from the default ListSessions response.
func (s *SessionService) ArchiveSession(
	ctx context.Context,
	req *connect.Request[sessionv1.ArchiveSessionRequest],
) (*connect.Response[sessionv1.ArchiveSessionResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}
	inst := s.FindLiveInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}
	now := time.Now()
	// ArchiveWithStop also transitions Status to Stopped (best-effort — archiving
	// previously left ArchivedAt set while Status stayed Active/Paused/Hibernated,
	// which the retention sweep and other Stopped-gated logic depend on being in sync).
	if err := inst.ArchiveWithStop(now); err != nil {
		log.Warn("failed to transition archived session to Stopped", "session", req.Msg.SessionId, "err", err)
	}
	if err := s.storage.SaveInstances([]*session.Instance{inst}); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save session: %w", err))
	}
	// Notifies event-driven cleanup (ReactiveQueueManager evicting any stale review-queue entry), mirroring ArchiveSessionByUUID.
	s.eventBus.Publish(events.NewSessionArchivedEvent(inst.UUID))
	return connect.NewResponse(&sessionv1.ArchiveSessionResponse{}), nil
}

// GetProviderLimits returns the rate limit and usage details for a session.
func (s *SessionService) GetProviderLimits(
	ctx context.Context,
	req *connect.Request[sessionv1.GetProviderLimitsRequest],
) (*connect.Response[sessionv1.GetProviderLimitsResponse], error) {
	if req.Msg.SessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session_id is required"))
	}

	inst := s.findInstance(req.Msg.SessionId)
	if inst == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.SessionId))
	}

	provider := "anthropic"
	program := strings.ToLower(inst.Program)
	if strings.Contains(program, "agy") || strings.Contains(program, "antigravity") || strings.Contains(program, "gemini") {
		provider = "google"
	} else if strings.Contains(program, "openai") || strings.Contains(program, "opencode") {
		provider = "openai"
	}

	var limits ProviderLimits
	var found bool
	if s.capacityMonitor != nil {
		limits, found = s.capacityMonitor.GetSessionLimits(inst.Title)
		if !found {
			s.capacityMonitor.mu.RLock()
			globalLimits := s.capacityMonitor.current[provider]
			client, ok := s.capacityMonitor.clients[provider]
			s.capacityMonitor.mu.RUnlock()

			limits = globalLimits
			limits.Provider = provider
			limits.Model = inst.Program
			if ok {
				limits.ContextTokensMax = client.ModelContextWindow(inst.Program)
			}
		}
	} else {
		limits = ProviderLimits{
			Provider:  provider,
			Model:     inst.Program,
			Available: true,
		}
	}

	protoLimits := &sessionv1.ProviderLimitsProto{
		Provider:            limits.Provider,
		Model:               limits.Model,
		RequestsLimit:       int32(limits.RequestsLimit),       //#nosec G115 -- provider rate-limit header value, realistic range is far below int32 max
		RequestsRemaining:   int32(limits.RequestsRemaining),   //#nosec G115 -- provider rate-limit header value, realistic range is far below int32 max
		TokensLimit:         int32(limits.TokensLimit),         //#nosec G115 -- provider token-limit header value, realistic range is far below int32 max
		TokensRemaining:     int32(limits.TokensRemaining),     //#nosec G115 -- provider token-limit header value, realistic range is far below int32 max
		ContextTokensUsed:   int32(limits.ContextTokensUsed),   //#nosec G115 -- model context window size, realistic range is far below int32 max
		ContextTokensMax:    int32(limits.ContextTokensMax),    //#nosec G115 -- model context window size, realistic range is far below int32 max
		SessionInputTokens:  int32(limits.SessionInputTokens),  //#nosec G115 -- per-session token count, realistic range is far below int32 max
		SessionOutputTokens: int32(limits.SessionOutputTokens), //#nosec G115 -- per-session token count, realistic range is far below int32 max
		EstimatedCostUsd:    limits.EstimatedCostUSD,
		Available:           limits.Available,
		LastErrorCode:       limits.LastErrorCode,
	}

	if !limits.RequestsReset.IsZero() {
		protoLimits.RequestsReset = timestamppb.New(limits.RequestsReset)
	}
	if !limits.TokensReset.IsZero() {
		protoLimits.TokensReset = timestamppb.New(limits.TokensReset)
	}
	if !limits.FetchedAt.IsZero() {
		protoLimits.FetchedAt = timestamppb.New(limits.FetchedAt)
	}

	return connect.NewResponse(&sessionv1.GetProviderLimitsResponse{
		Limits: protoLimits,
	}), nil
}

// publishSessionUpdatedEvent publishes a SessionUpdated event for instance covering
// updatedFields, using the statusManager-aware NewSessionUpdatedEventWithDetection variant
// when a controller is actively running (so clients see live ClaudeStatus/StatusContext),
// and falling back to the plain NewSessionUpdatedEvent otherwise. Shared by UpdateSession's
// end-of-handler publish and UpdateSessionProgram (the capacity-monitor auto-fallback path)
// so the two program-switch entry points publish identically instead of drifting.
func (s *SessionService) publishSessionUpdatedEvent(instance *session.Instance, updatedFields []string) {
	if s.statusManager != nil {
		statusInfo := s.statusManager.GetStatus(instance)
		if statusInfo.IsControllerActive {
			s.eventBus.Publish(events.NewSessionUpdatedEventWithDetection(
				instance, updatedFields,
				statusInfo.ClaudeStatus, statusInfo.StatusContext,
			))
			return
		}
	}
	s.eventBus.Publish(events.NewSessionUpdatedEvent(instance, updatedFields))
}

// UpdateSessionProgram handles switching programs for a session, doing the history
// porting, DB save, and PTY restart. Shares its implementation with the UpdateSession RPC
// handler via Instance.SwitchProgram (see session/instance_program.go) and
// publishSessionUpdatedEvent above so the two program-switch entry points — this
// auto-fallback path and the manual RPC — can't drift.
func (s *SessionService) UpdateSessionProgram(ctx context.Context, sessionID string, newProgram string) error {
	inst := s.findInstance(sessionID)
	if inst == nil {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	changed, _, err := inst.SwitchProgram(ctx, newProgram, func() error {
		return s.storage.SaveInstances([]*session.Instance{inst})
	})
	if !changed {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to restart session: %w", err)
	}

	s.publishSessionUpdatedEvent(inst, []string{"program"})

	return nil
}

// SetResolveConversationUUID wires the tmux-UUID → Claude-UUID resolver into the search service.
func (s *SessionService) SetResolveConversationUUID(fn func(ctx context.Context, tmuxUUID string) (string, error)) {
	s.searchSvc.SetResolveConversationUUID(fn)
}

// SetTokenStoreReader wires the global parsed token store into the capacity monitor.
func (s *SessionService) SetTokenStoreReader(store tokens.TokenStoreReader) {
	if s.capacityMonitor != nil {
		s.capacityMonitor.tokenStore = store
	}
}

// SetQuotaGate wires the account-wide quota gate so onRateLimitDetected can
// feed it the hard/reactive override signal.
func (s *SessionService) SetQuotaGate(g *QuotaGate) {
	s.quotaGate = g
}

// GetInstances returns all managed (poller-tracked) live instances, satisfying InstancePoller.
func (s *SessionService) GetInstances() []*session.Instance {
	return s.allInstances()
}

// GetConfigFileRules delegates to RulesService.
func (s *SessionService) GetConfigFileRules(
	ctx context.Context,
	req *connect.Request[sessionv1.GetConfigFileRulesRequest],
) (*connect.Response[sessionv1.GetConfigFileRulesResponse], error) {
	return s.rulesSvc.GetConfigFileRules(ctx, req)
}

// SaveRulesToConfigFile delegates to RulesService.
func (s *SessionService) SaveRulesToConfigFile(
	ctx context.Context,
	req *connect.Request[sessionv1.SaveRulesToConfigFileRequest],
) (*connect.Response[sessionv1.SaveRulesToConfigFileResponse], error) {
	return s.rulesSvc.SaveRulesToConfigFile(ctx, req)
}
