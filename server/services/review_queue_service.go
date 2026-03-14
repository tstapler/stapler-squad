package services

import (
	"context"
	"fmt"
	"time"

	sessionv1 "claude-squad/gen/proto/go/session/v1"
	"claude-squad/log"
	"claude-squad/server/adapters"
	"claude-squad/server/events"
	"claude-squad/session"

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

	protoQueue := adapters.ReviewQueueToProto(queue)

	// Enrich APPROVAL_PENDING items with their pending_approval_id so the
	// frontend can show Approve/Deny buttons directly in the review queue.
	if rqs.approvalStore != nil && protoQueue != nil {
		for _, item := range protoQueue.Items {
			if item.Reason == sessionv1.AttentionReason_ATTENTION_REASON_APPROVAL_PENDING {
				approvals := rqs.approvalStore.GetBySession(item.SessionId)
				if len(approvals) > 0 {
					if item.Metadata == nil {
						item.Metadata = make(map[string]string)
					}
					item.Metadata["pending_approval_id"] = approvals[0].ID
				}
			}
		}
	}

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

	instances, err := rqs.storage.LoadInstances()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load instances: %w", err))
	}

	var instance *session.Instance
	var instanceIndex int
	for i, inst := range instances {
		if inst.Title == req.Msg.Id {
			instance = inst
			instanceIndex = i
			break
		}
	}

	if instance == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
	}

	instance.LastAcknowledged = time.Now()
	instances[instanceIndex] = instance

	if err := rqs.storage.SaveInstances(instances); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to save instance: %w", err))
	}

	// CRITICAL: Update the ReviewQueuePoller's instance references.
	// When we LoadInstances() above, we create brand new instance objects.
	// The poller still has references to the OLD objects from initialization.
	// If we don't update the poller's references, it will continue checking
	// stale objects with outdated LastAddedToQueue timestamps, causing
	// notification spam even after the user acknowledges sessions.
	if rqs.reviewQueuePoller != nil {
		rqs.reviewQueuePoller.SetInstances(instances)
		log.InfoLog.Printf("[ReviewQueue] Updated poller instance references after AcknowledgeSession for '%s'", instance.Title)
	}

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
