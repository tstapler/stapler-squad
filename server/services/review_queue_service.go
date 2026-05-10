package services

import (
	"context"
	"fmt"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"

	"connectrpc.com/connect"
)

// ReviewQueueService handles all review-queue-related RPC methods, extracted
// from the monolithic SessionService for separation of concerns.
//
// Dependencies it owns (moved out of SessionService):
//   - reviewQueue:      stateful queue managed by ReviewQueuePoller
//   - reactiveQueueMgr: streams live review queue events to clients
//
// Dependencies it borrows (still on SessionService, passed via setters):
//   - storage:           needed by AcknowledgeSession to persist ack timestamps
//   - reviewQueuePoller: needed by AcknowledgeSession to refresh poller refs
//   - eventBus:          needed by AcknowledgeSession and LogUserInteraction
type ReviewQueueService struct {
	reviewQueue      *session.ReviewQueue
	reactiveQueueMgr ReactiveQueueManager

	// Borrowed from SessionService — set via Set* methods after construction.
	storage           *session.Storage
	reviewQueuePoller *session.ReviewQueuePoller
	eventBus          *events.EventBus
	approvalStore     *ApprovalStore
}

// NewReviewQueueService creates a ReviewQueueService with the required state.
func NewReviewQueueService(
	reviewQueue *session.ReviewQueue,
	storage *session.Storage,
	eventBus *events.EventBus,
) *ReviewQueueService {
	return &ReviewQueueService{
		reviewQueue: reviewQueue,
		storage:     storage,
		eventBus:    eventBus,
	}
}

// SetReactiveQueueManager injects the ReactiveQueueManager (dependency injection).
// Must be called before WatchReviewQueue is used.
func (rqs *ReviewQueueService) SetReactiveQueueManager(mgr ReactiveQueueManager) {
	rqs.reactiveQueueMgr = mgr
}

// GetReactiveQueueManager returns the injected ReactiveQueueManager, or nil if not set.
func (rqs *ReviewQueueService) GetReactiveQueueManager() ReactiveQueueManager {
	return rqs.reactiveQueueMgr
}

// SetReviewQueuePoller injects the ReviewQueuePoller used to refresh instance
// references after acknowledgement.
func (rqs *ReviewQueueService) SetReviewQueuePoller(poller *session.ReviewQueuePoller) {
	rqs.reviewQueuePoller = poller
}

// SetApprovalStore injects the ApprovalStore for enriching APPROVAL_PENDING items
// with their pending_approval_id metadata.
func (rqs *ReviewQueueService) SetApprovalStore(store *ApprovalStore) {
	rqs.approvalStore = store
}

// GetQueue returns the underlying ReviewQueue for wiring reactive components.
func (rqs *ReviewQueueService) GetQueue() *session.ReviewQueue {
	return rqs.reviewQueue
}

// ---------------------------------------------------------------------------
// RPC methods
// ---------------------------------------------------------------------------

// GetReviewQueue returns sessions needing user attention with priority ordering.
// Uses the global stateful queue managed by ReviewQueuePoller, with optional filtering.
func (rqs *ReviewQueueService) GetReviewQueue(
	ctx context.Context,
	req *connect.Request[sessionv1.GetReviewQueueRequest],
) (*connect.Response[sessionv1.GetReviewQueueResponse], error) {
	allItems := rqs.reviewQueue.List()

	filteredItems := make([]*session.ReviewItem, 0, len(allItems))
	for _, item := range allItems {
		if req.Msg.PriorityFilter != nil {
			targetPriority := adapters.ProtoToPriority(*req.Msg.PriorityFilter)
			if item.Priority != targetPriority {
				continue
			}
		}
		if req.Msg.ReasonFilter != nil {
			targetReason := adapters.ProtoToAttentionReason(*req.Msg.ReasonFilter)
			if item.Reason != targetReason {
				continue
			}
		}
		filteredItems = append(filteredItems, item)
	}

	queue := session.NewReviewQueue()
	for _, item := range filteredItems {
		queue.Add(item)
	}

	// Build approvalID enrichment map before converting so the adapter can
	// inject pending_approval_id at construction time — no post-conversion
	// mutation needed (which would race across concurrent RPC calls).
	var approvalIDs map[string]string
	if rqs.approvalStore != nil {
		for _, item := range filteredItems {
			if item.Reason == session.ReasonApprovalPending {
				if pending := rqs.approvalStore.GetBySession(item.SessionID); len(pending) > 0 {
					if approvalIDs == nil {
						approvalIDs = make(map[string]string)
					}
					approvalIDs[item.SessionID] = pending[0].ID
				}
			}
		}
	}

	protoQueue := adapters.ReviewQueueToProto(queue, approvalIDs)

	return connect.NewResponse(&sessionv1.GetReviewQueueResponse{
		ReviewQueue: protoQueue,
	}), nil
}

// AcknowledgeSession marks a session as acknowledged in the review queue.
// The session won't reappear in the queue until it receives an update.
func (rqs *ReviewQueueService) AcknowledgeSession(
	ctx context.Context,
	req *connect.Request[sessionv1.AcknowledgeSessionRequest],
) (*connect.Response[sessionv1.AcknowledgeSessionResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	// Always remove from the in-memory review queue immediately.
	// This handles sessions with IDs that don't match any stored instance
	// (e.g., external sessions, or sessions with corrupt IDs from the TTY detection bug).
	rqs.reviewQueue.Remove(req.Msg.Id)

	// Look up the live in-memory instance from the poller first. This avoids calling
	// LoadInstances(), which constructs fresh Instance objects from the database and
	// discards the live PTY/controller state held by instances already in the poller.
	var instance *session.Instance
	if rqs.reviewQueuePoller != nil {
		instance = rqs.reviewQueuePoller.FindInstance(req.Msg.Id)
	}

	if instance == nil {
		// Session is not in the poller (external session, unknown ID, or poller not wired).
		// Fall back to a direct storage lookup so we can still persist the ack timestamp.
		dataSlice, err := rqs.storage.ListInstanceData()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list instances: %w", err))
		}
		sessionTitle := ""
		for _, data := range dataSlice {
			if data.Title == req.Msg.Id || data.UUID == req.Msg.Id {
				sessionTitle = data.Title
				break
			}
		}
		if sessionTitle == "" {
			log.InfoLog.Printf("[ReviewQueue] AcknowledgeSession: session '%s' not found in storage, removed from queue", req.Msg.Id)
			return connect.NewResponse(&sessionv1.AcknowledgeSessionResponse{
				Success: true,
				Message: fmt.Sprintf("Session '%s' removed from review queue", req.Msg.Id),
			}), nil
		}
		// Persist the acknowledged timestamp using Title (the storage key), not the client-supplied ID.
		if err := rqs.storage.UpdateInstanceAcknowledged(sessionTitle); err != nil {
			log.WarningLog.Printf("[ReviewQueue] AcknowledgeSession: failed to persist ack for '%s': %v", req.Msg.Id, err)
		}
		rqs.eventBus.Publish(events.NewSessionAcknowledgedEvent(req.Msg.Id, "user_acknowledged"))
		return connect.NewResponse(&sessionv1.AcknowledgeSessionResponse{
			Success: true,
			Message: fmt.Sprintf("Session '%s' acknowledged and removed from review queue", req.Msg.Id),
		}), nil
	}

	// Update the live instance in-place — no LoadInstances() required.
	instance.MarkAcknowledged()

	// Persist only this instance. UpdateInstance does a targeted UPDATE by title.
	if err := rqs.storage.UpdateInstance(instance); err != nil {
		log.WarningLog.Printf("[ReviewQueue] AcknowledgeSession: failed to persist ack for '%s': %v", instance.Title, err)
	}

	log.InfoLog.Printf("[ReviewQueue] AcknowledgeSession: acknowledged live instance '%s'", instance.Title)
	rqs.eventBus.Publish(events.NewSessionAcknowledgedEvent(instance.Title, "user_acknowledged"))

	return connect.NewResponse(&sessionv1.AcknowledgeSessionResponse{
		Success: true,
		Message: fmt.Sprintf("Session '%s' acknowledged and removed from review queue", req.Msg.Id),
	}), nil
}

// WatchReviewQueue streams real-time review queue events.
func (rqs *ReviewQueueService) WatchReviewQueue(
	ctx context.Context,
	req *connect.Request[sessionv1.WatchReviewQueueRequest],
	stream *connect.ServerStream[sessionv1.ReviewQueueEvent],
) error {
	if rqs.reactiveQueueMgr == nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("reactive queue manager not initialized"))
	}

	filters := &WatchReviewQueueFilters{
		PriorityFilter:    convertProtoPriorities(req.Msg.PriorityFilter),
		ReasonFilter:      convertProtoReasons(req.Msg.ReasonFilter),
		SessionIDs:        req.Msg.SessionIds,
		IncludeStatistics: req.Msg.IncludeStatistics,
		InitialSnapshot:   req.Msg.InitialSnapshot,
	}

	eventCh, clientID := rqs.reactiveQueueMgr.AddStreamClient(ctx, filters)
	defer rqs.reactiveQueueMgr.RemoveStreamClient(clientID)

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-eventCh:
			if !ok {
				return nil
			}
			if err := stream.Send(event); err != nil {
				return err
			}
		}
	}
}

// LogUserInteraction logs a user interaction event for audit trail and analytics.
func (rqs *ReviewQueueService) LogUserInteraction(
	ctx context.Context,
	req *connect.Request[sessionv1.LogUserInteractionRequest],
) (*connect.Response[sessionv1.LogUserInteractionResponse], error) {
	sessionID := ""
	if req.Msg.SessionId != nil {
		sessionID = *req.Msg.SessionId
	}
	interactionType := req.Msg.InteractionType
	interactionCtx := ""
	if req.Msg.Context != nil {
		interactionCtx = *req.Msg.Context
	}
	notificationID := ""
	if req.Msg.NotificationId != nil {
		notificationID = *req.Msg.NotificationId
	}

	fields := map[string]interface{}{
		"interaction_type": interactionType.String(),
		"timestamp":        time.Now().Format(time.RFC3339),
	}
	if sessionID != "" {
		fields["session_id"] = sessionID
	}
	if interactionCtx != "" {
		fields["context"] = interactionCtx
	}
	if notificationID != "" {
		fields["notification_id"] = notificationID
	}
	if req.Msg.Metadata != nil {
		for key, value := range req.Msg.Metadata {
			fields["meta_"+key] = value
		}
	}

	log.InfoS("User Interaction", fields)

	if rqs.eventBus != nil {
		rqs.eventBus.Publish(events.NewUserInteractionEvent(sessionID, interactionType.String(), interactionCtx))
	}

	return connect.NewResponse(&sessionv1.LogUserInteractionResponse{
		Success: true,
	}), nil
}

// ---------------------------------------------------------------------------
// Review queue filter types and helpers
// ---------------------------------------------------------------------------

// WatchReviewQueueFilters contains filters for review queue event streaming.
type WatchReviewQueueFilters struct {
	PriorityFilter    []session.Priority
	ReasonFilter      []session.AttentionReason
	SessionIDs        []string
	IncludeStatistics bool
	InitialSnapshot   bool
}

// Implement FilterProvider interface for type-safe conversion.
func (f *WatchReviewQueueFilters) GetPriorityFilter() []session.Priority {
	return f.PriorityFilter
}

func (f *WatchReviewQueueFilters) GetReasonFilter() []session.AttentionReason {
	return f.ReasonFilter
}

func (f *WatchReviewQueueFilters) GetSessionIDs() []string {
	return f.SessionIDs
}

func (f *WatchReviewQueueFilters) GetIncludeStatistics() bool {
	return f.IncludeStatistics
}

func (f *WatchReviewQueueFilters) GetInitialSnapshot() bool {
	return f.InitialSnapshot
}

// convertProtoPriorities converts proto Priority values to internal session.Priority.
func convertProtoPriorities(protoPriorities []sessionv1.Priority) []session.Priority {
	result := make([]session.Priority, 0, len(protoPriorities))
	for _, p := range protoPriorities {
		switch p {
		case sessionv1.Priority_PRIORITY_URGENT:
			result = append(result, session.PriorityUrgent)
		case sessionv1.Priority_PRIORITY_HIGH:
			result = append(result, session.PriorityHigh)
		case sessionv1.Priority_PRIORITY_MEDIUM:
			result = append(result, session.PriorityMedium)
		case sessionv1.Priority_PRIORITY_LOW:
			result = append(result, session.PriorityLow)
		}
	}
	return result
}

// convertProtoReasons converts proto AttentionReason values to internal session.AttentionReason.
func convertProtoReasons(protoReasons []sessionv1.AttentionReason) []session.AttentionReason {
	result := make([]session.AttentionReason, 0, len(protoReasons))
	for _, r := range protoReasons {
		switch r {
		case sessionv1.AttentionReason_ATTENTION_REASON_APPROVAL_PENDING:
			result = append(result, session.ReasonApprovalPending)
		case sessionv1.AttentionReason_ATTENTION_REASON_INPUT_REQUIRED:
			result = append(result, session.ReasonInputRequired)
		case sessionv1.AttentionReason_ATTENTION_REASON_ERROR_STATE:
			result = append(result, session.ReasonErrorState)
		case sessionv1.AttentionReason_ATTENTION_REASON_IDLE_TIMEOUT:
			result = append(result, session.ReasonIdleTimeout)
		case sessionv1.AttentionReason_ATTENTION_REASON_TASK_COMPLETE:
			result = append(result, session.ReasonTaskComplete)
		}
	}
	return result
}
