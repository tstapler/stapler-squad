package server

import (
	"claude-squad/log"
	"claude-squad/server/adapters"
	"claude-squad/server/events"
	"claude-squad/session"
	"context"
	"sync"
	"time"

	sessionv1 "claude-squad/gen/proto/go/session/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ReactiveQueueManager manages the review queue with immediate reactivity to user interactions.
// It listens to interaction events and immediately re-evaluates the queue instead of waiting
// for the next poll cycle, providing <100ms feedback to users.
type ReactiveQueueManager struct {
	queue         *session.ReviewQueue
	poller        *session.ReviewQueuePoller
	eventBus      *events.EventBus
	statusManager *session.InstanceStatusManager
	storage       *session.Storage // For persisting timestamps

	// Streaming clients for WatchReviewQueue
	streamClients     map[string]*reviewQueueStreamClient
	streamClientsMu   sync.RWMutex
	nextClientID      int
	nextClientIDMu    sync.Mutex

	// Subscription channels
	eventCh   <-chan *events.Event
	subID     string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// reviewQueueStreamClient represents a client streaming review queue events
type reviewQueueStreamClient struct {
	id       string
	filters  *WatchReviewQueueFilters
	eventCh  chan *sessionv1.ReviewQueueEvent
	ctx      context.Context
	cancel   context.CancelFunc
}

// WatchReviewQueueFilters contains filters for review queue event streaming
type WatchReviewQueueFilters struct {
	PriorityFilter      []session.Priority
	ReasonFilter        []session.AttentionReason
	SessionIDs          []string
	IncludeStatistics   bool
	InitialSnapshot     bool
}

// NewReactiveQueueManager creates a new reactive queue manager.
func NewReactiveQueueManager(
	queue *session.ReviewQueue,
	poller *session.ReviewQueuePoller,
	eventBus *events.EventBus,
	statusManager *session.InstanceStatusManager,
	storage *session.Storage,
) *ReactiveQueueManager {
	return &ReactiveQueueManager{
		queue:         queue,
		poller:        poller,
		eventBus:      eventBus,
		statusManager: statusManager,
		storage:       storage,
		streamClients: make(map[string]*reviewQueueStreamClient),
	}
}

// Start initializes the reactive queue manager and subscribes to events.
func (rqm *ReactiveQueueManager) Start(ctx context.Context) {
	rqm.ctx, rqm.cancel = context.WithCancel(ctx)

	// Subscribe to event bus for immediate queue updates
	if rqm.eventBus != nil {
		eventCh, subID := rqm.eventBus.Subscribe(rqm.ctx)
		rqm.eventCh = eventCh
		rqm.subID = subID

		// Start event processing loop
		rqm.wg.Add(1)
		go rqm.processEvents()
	}

	// Subscribe to review queue changes to publish to streaming clients
	rqm.queue.Subscribe(rqm)

	// Start the background poller (for safety and periodic checks)
	rqm.poller.Start(ctx)

	log.InfoLog.Printf("[ReactiveQueueManager] Started with event-driven updates")
}

// processEvents processes events from the event bus.
func (rqm *ReactiveQueueManager) processEvents() {
	defer rqm.wg.Done()

	for {
		select {
		case event := <-rqm.eventCh:
			rqm.handleEvent(event)
		case <-rqm.ctx.Done():
			return
		}
	}
}

// handleEvent dispatches events to the appropriate handler.
func (rqm *ReactiveQueueManager) handleEvent(event *events.Event) {
	switch event.Type {
	case events.EventUserInteraction:
		rqm.handleUserInteraction(event)
	case events.EventSessionAcknowledged:
		rqm.handleSessionAcknowledged(event)
	case events.EventApprovalResponse:
		rqm.handleApprovalResponse(event)
	}
}

// Stop stops the reactive queue manager.
func (rqm *ReactiveQueueManager) Stop() {
	if rqm.cancel != nil {
		rqm.cancel()
	}

	// Stop the poller
	rqm.poller.Stop()

	// Close all streaming clients
	rqm.streamClientsMu.Lock()
	for _, client := range rqm.streamClients {
		client.cancel()
		close(client.eventCh)
	}
	rqm.streamClients = make(map[string]*reviewQueueStreamClient)
	rqm.streamClientsMu.Unlock()

	rqm.wg.Wait()
	log.InfoLog.Printf("[ReactiveQueueManager] Stopped")
}

// handleUserInteraction handles user interaction events and immediately re-evaluates the queue.
func (rqm *ReactiveQueueManager) handleUserInteraction(event *events.Event) {
	sessionID := event.SessionID
	if sessionID == "" {
		return
	}

	log.DebugLog.Printf("[ReactiveQueueManager] User interaction on '%s' (type: %s)",
		sessionID, event.InteractionType)

	// Find the instance
	inst := rqm.poller.FindInstance(sessionID)
	if inst == nil {
		log.DebugLog.Printf("[ReactiveQueueManager] Instance '%s' not found", sessionID)
		return
	}

	// Update LastUserResponse timestamp
	inst.LastUserResponse = time.Now()
	log.InfoLog.Printf("[ReactiveQueueManager] Updated LastUserResponse for '%s' to %v",
		sessionID, inst.LastUserResponse)

	// Persist timestamp (critical for restart scenarios)
	if rqm.storage != nil {
		if err := rqm.storage.UpdateInstanceLastUserResponse(inst.Title, inst.LastUserResponse); err != nil {
			log.ErrorLog.Printf("Failed to persist LastUserResponse for '%s': %v", sessionID, err)
		}
	}

	// Immediate re-evaluation using exported method
	rqm.poller.CheckSession(inst)
}

// handleSessionAcknowledged handles session acknowledged events.
func (rqm *ReactiveQueueManager) handleSessionAcknowledged(event *events.Event) {
	sessionID := event.SessionID
	if sessionID == "" {
		return
	}

	log.InfoLog.Printf("[ReactiveQueueManager] Session '%s' acknowledged - removing from queue", sessionID)

	// Immediate removal from queue
	removed := rqm.queue.Remove(sessionID)
	if removed {
		log.DebugLog.Printf("[ReactiveQueueManager] Session '%s' removed from queue", sessionID)
	}
}

// handleApprovalResponse handles approval response events.
func (rqm *ReactiveQueueManager) handleApprovalResponse(event *events.Event) {
	sessionID := event.SessionID
	if sessionID == "" {
		return
	}

	approved := event.Approved
	log.InfoLog.Printf("[ReactiveQueueManager] Approval %s for '%s' - removing from queue",
		map[bool]string{true: "given", false: "denied"}[approved], sessionID)

	// Find the instance and update status
	inst := rqm.poller.FindInstance(sessionID)
	if inst != nil && approved {
		inst.SetStatus(session.Running)
	}

	// Immediate removal from queue
	rqm.queue.Remove(sessionID)
}

// ReviewQueueObserver implementation - publishes events to streaming clients

// OnItemAdded is called when an item is added to the queue.
func (rqm *ReactiveQueueManager) OnItemAdded(item *session.ReviewItem) {
	event := &sessionv1.ReviewQueueEvent{
		Timestamp: timestamppb.Now(),
		Event: &sessionv1.ReviewQueueEvent_ItemAdded{
			ItemAdded: &sessionv1.ReviewQueueItemAddedEvent{
				Item:       rqm.reviewItemToProto(item),
				Trigger:    "reactive_manager",
				IsSnapshot: false, // Real-time event - frontend SHOULD fire notifications
			},
		},
	}
	rqm.publishToClients(event)
}

// OnItemRemoved is called when an item is removed from the queue.
func (rqm *ReactiveQueueManager) OnItemRemoved(sessionID string) {
	event := &sessionv1.ReviewQueueEvent{
		Timestamp: timestamppb.Now(),
		Event: &sessionv1.ReviewQueueEvent_ItemRemoved{
			ItemRemoved: &sessionv1.ReviewQueueItemRemovedEvent{
				SessionId: sessionID,
				Reason:    "user_action",
			},
		},
	}
	rqm.publishToClients(event)
}

// OnQueueUpdated is called when the queue is updated.
func (rqm *ReactiveQueueManager) OnQueueUpdated(items []*session.ReviewItem) {
	// Optionally publish statistics update
	stats := rqm.queue.GetStatistics()
	event := &sessionv1.ReviewQueueEvent{
		Timestamp: timestamppb.Now(),
		Event: &sessionv1.ReviewQueueEvent_Statistics{
			Statistics: &sessionv1.ReviewQueueStatisticsEvent{
				TotalItems:   int32(stats.TotalItems),
				ByPriority:   rqm.priorityMapToProto(stats.ByPriority),
				ByReason:     rqm.reasonMapToProto(stats.ByReason),
				AverageAgeMs: stats.AverageAge.Milliseconds(),
			},
		},
	}
	rqm.publishToClients(event)
}

// FilterProvider is an interface that provides filter values for type-safe conversion
type FilterProvider interface {
	GetPriorityFilter() []session.Priority
	GetReasonFilter() []session.AttentionReason
	GetSessionIDs() []string
	GetIncludeStatistics() bool
	GetInitialSnapshot() bool
}

// AddStreamClient adds a new streaming client for WatchReviewQueue.
func (rqm *ReactiveQueueManager) AddStreamClient(ctx context.Context, filtersInterface interface{}) (<-chan *sessionv1.ReviewQueueEvent, string) {
	// Convert interface to our filters type
	var filters *WatchReviewQueueFilters

	if filtersInterface == nil {
		filters = nil
	} else if filterProvider, ok := filtersInterface.(FilterProvider); ok {
		// Use the interface to extract values
		filters = &WatchReviewQueueFilters{
			PriorityFilter:    filterProvider.GetPriorityFilter(),
			ReasonFilter:      filterProvider.GetReasonFilter(),
			SessionIDs:        filterProvider.GetSessionIDs(),
			IncludeStatistics: filterProvider.GetIncludeStatistics(),
			InitialSnapshot:   filterProvider.GetInitialSnapshot(),
		}
	} else if f, ok := filtersInterface.(*WatchReviewQueueFilters); ok {
		filters = f
	}

	// Original implementation continues

	rqm.nextClientIDMu.Lock()
	clientID := rqm.generateClientID()
	rqm.nextClientIDMu.Unlock()

	clientCtx, cancel := context.WithCancel(ctx)
	eventCh := make(chan *sessionv1.ReviewQueueEvent, 100) // Buffered channel

	client := &reviewQueueStreamClient{
		id:      clientID,
		filters: filters,
		eventCh: eventCh,
		ctx:     clientCtx,
		cancel:  cancel,
	}

	rqm.streamClientsMu.Lock()
	rqm.streamClients[clientID] = client
	rqm.streamClientsMu.Unlock()

	log.InfoLog.Printf("[ReactiveQueueManager] Added streaming client %s", clientID)

	// Send initial snapshot if requested
	if filters != nil && filters.InitialSnapshot {
		rqm.sendInitialSnapshot(client)
	}

	return eventCh, clientID
}

// RemoveStreamClient removes a streaming client.
func (rqm *ReactiveQueueManager) RemoveStreamClient(clientID string) {
	rqm.streamClientsMu.Lock()
	defer rqm.streamClientsMu.Unlock()

	if client, exists := rqm.streamClients[clientID]; exists {
		client.cancel()
		close(client.eventCh)
		delete(rqm.streamClients, clientID)
		log.InfoLog.Printf("[ReactiveQueueManager] Removed streaming client %s", clientID)
	}
}

// publishToClients publishes an event to all streaming clients that match filters.
func (rqm *ReactiveQueueManager) publishToClients(event *sessionv1.ReviewQueueEvent) {
	rqm.streamClientsMu.RLock()
	defer rqm.streamClientsMu.RUnlock()

	for _, client := range rqm.streamClients {
		if rqm.eventMatchesFilters(event, client.filters) {
			select {
			case client.eventCh <- event:
				// Event sent successfully
			case <-client.ctx.Done():
				// Client disconnected
			default:
				// Channel full, drop event (could log this)
				log.DebugLog.Printf("[ReactiveQueueManager] Dropped event for client %s (channel full)", client.id)
			}
		}
	}
}

// eventMatchesFilters checks if an event matches the client's filters.
func (rqm *ReactiveQueueManager) eventMatchesFilters(event *sessionv1.ReviewQueueEvent, filters *WatchReviewQueueFilters) bool {
	if filters == nil {
		return true // No filters, accept all
	}

	// Check statistics filter
	if _, isStats := event.Event.(*sessionv1.ReviewQueueEvent_Statistics); isStats {
		return filters.IncludeStatistics
	}

	// Extract session ID from event
	var sessionID string
	var priority sessionv1.Priority
	var reason sessionv1.AttentionReason

	switch e := event.Event.(type) {
	case *sessionv1.ReviewQueueEvent_ItemAdded:
		if e.ItemAdded.Item != nil {
			sessionID = e.ItemAdded.Item.SessionId
			priority = e.ItemAdded.Item.Priority
			reason = e.ItemAdded.Item.Reason
		}
	case *sessionv1.ReviewQueueEvent_ItemRemoved:
		sessionID = e.ItemRemoved.SessionId
	case *sessionv1.ReviewQueueEvent_ItemUpdated:
		if e.ItemUpdated.Item != nil {
			sessionID = e.ItemUpdated.Item.SessionId
			priority = e.ItemUpdated.Item.Priority
			reason = e.ItemUpdated.Item.Reason
		}
	}

	// Check session ID filter
	if len(filters.SessionIDs) > 0 {
		found := false
		for _, id := range filters.SessionIDs {
			if id == sessionID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check priority filter
	if len(filters.PriorityFilter) > 0 {
		found := false
		for _, p := range filters.PriorityFilter {
			if rqm.priorityToProto(p) == priority {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check reason filter
	if len(filters.ReasonFilter) > 0 {
		found := false
		for _, r := range filters.ReasonFilter {
			if rqm.reasonToProto(r) == reason {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// sendInitialSnapshot sends the current queue state to a new client.
func (rqm *ReactiveQueueManager) sendInitialSnapshot(client *reviewQueueStreamClient) {
	// Recover from panic if channel is closed (race condition with RemoveStreamClient)
	defer func() {
		if r := recover(); r != nil {
			log.DebugLog.Printf("[ReactiveQueueManager] Recovered from panic in sendInitialSnapshot for client %s: %v (client likely disconnected)", client.id, r)
		}
	}()

	items := rqm.queue.List()

	for _, item := range items {
		event := &sessionv1.ReviewQueueEvent{
			Timestamp: timestamppb.Now(),
			Event: &sessionv1.ReviewQueueEvent_ItemAdded{
				ItemAdded: &sessionv1.ReviewQueueItemAddedEvent{
					Item:       rqm.reviewItemToProto(item),
					Trigger:    "initial_snapshot",
					IsSnapshot: true, // Mark as snapshot - frontend should NOT fire notifications
				},
			},
		}

		select {
		case client.eventCh <- event:
			// Sent successfully
		case <-client.ctx.Done():
			return
		}
	}

	// Send statistics if requested
	if client.filters != nil && client.filters.IncludeStatistics {
		stats := rqm.queue.GetStatistics()
		event := &sessionv1.ReviewQueueEvent{
			Timestamp: timestamppb.Now(),
			Event: &sessionv1.ReviewQueueEvent_Statistics{
				Statistics: &sessionv1.ReviewQueueStatisticsEvent{
					TotalItems:   int32(stats.TotalItems),
					ByPriority:   rqm.priorityMapToProto(stats.ByPriority),
					ByReason:     rqm.reasonMapToProto(stats.ByReason),
					AverageAgeMs: stats.AverageAge.Milliseconds(),
				},
			},
		}

		select {
		case client.eventCh <- event:
			// Sent successfully
		case <-client.ctx.Done():
			return
		}
	}
}

// generateClientID generates a unique client ID.
func (rqm *ReactiveQueueManager) generateClientID() string {
	rqm.nextClientID++
	return time.Now().Format("20060102150405") + "-" + string(rune('A'+rqm.nextClientID%26))
}

// Helper methods to convert between internal types and proto types

func (rqm *ReactiveQueueManager) reviewItemToProto(item *session.ReviewItem) *sessionv1.ReviewItem {
	return adapters.ReviewItemToProto(item)
}

func (rqm *ReactiveQueueManager) priorityToProto(p session.Priority) sessionv1.Priority {
	switch p {
	case session.PriorityUrgent:
		return sessionv1.Priority_PRIORITY_URGENT
	case session.PriorityHigh:
		return sessionv1.Priority_PRIORITY_HIGH
	case session.PriorityMedium:
		return sessionv1.Priority_PRIORITY_MEDIUM
	case session.PriorityLow:
		return sessionv1.Priority_PRIORITY_LOW
	default:
		return sessionv1.Priority_PRIORITY_UNSPECIFIED
	}
}

func (rqm *ReactiveQueueManager) reasonToProto(r session.AttentionReason) sessionv1.AttentionReason {
	switch r {
	case session.ReasonApprovalPending:
		return sessionv1.AttentionReason_ATTENTION_REASON_APPROVAL_PENDING
	case session.ReasonInputRequired:
		return sessionv1.AttentionReason_ATTENTION_REASON_INPUT_REQUIRED
	case session.ReasonErrorState:
		return sessionv1.AttentionReason_ATTENTION_REASON_ERROR_STATE
	case session.ReasonIdleTimeout:
		return sessionv1.AttentionReason_ATTENTION_REASON_IDLE_TIMEOUT
	case session.ReasonTaskComplete:
		return sessionv1.AttentionReason_ATTENTION_REASON_TASK_COMPLETE
	case session.ReasonUncommittedChanges:
		return sessionv1.AttentionReason_ATTENTION_REASON_UNCOMMITTED_CHANGES
	case session.ReasonIdle:
		return sessionv1.AttentionReason_ATTENTION_REASON_IDLE
	case session.ReasonStale:
		return sessionv1.AttentionReason_ATTENTION_REASON_STALE
	case session.ReasonWaitingForUser:
		return sessionv1.AttentionReason_ATTENTION_REASON_WAITING_FOR_USER
	case session.ReasonTestsFailing:
		return sessionv1.AttentionReason_ATTENTION_REASON_TESTS_FAILING
	default:
		return sessionv1.AttentionReason_ATTENTION_REASON_UNSPECIFIED
	}
}

func (rqm *ReactiveQueueManager) priorityMapToProto(m map[session.Priority]int) map[int32]int32 {
	result := make(map[int32]int32)
	for k, v := range m {
		result[int32(rqm.priorityToProto(k))] = int32(v)
	}
	return result
}

func (rqm *ReactiveQueueManager) reasonMapToProto(m map[session.AttentionReason]int) map[int32]int32 {
	result := make(map[int32]int32)
	for k, v := range m {
		result[int32(rqm.reasonToProto(k))] = int32(v)
	}
	return result
}
