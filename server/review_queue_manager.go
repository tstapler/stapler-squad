package server

import (
	"context"
	"errors"
	"fmt"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection"
	"sync"
	"time"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// OneShotPRCreator runs a one-shot LLM prompt against a session's worktree,
// returning the PR URL the prompt produced (or "" if none was created).
// Defined here — the consumer — rather than in server/services, per this
// repo's anti-interface-pollution convention; *services.SessionService
// satisfies it via RunOneShotForSession.
type OneShotPRCreator interface {
	RunOneShotForSession(ctx context.Context, sessionID, prompt string, timeoutSeconds int32) (string, error)
}

// autoCreatePRPrompt is the default one-shot prompt used when a backlog item's
// AutoCreatePR policy is enabled. Mirrors DEFAULT_PR_PROMPT in
// web-app/src/components/sessions/ReviewQueuePanel.tsx — the same prompt a
// human sees pre-filled in the manual "Create PR" modal. Kept in sync manually;
// TestMaybeAutoCreatePR_* tests only exercise the Go side, so a drift between
// the two literals would not be caught by CI today.
//
// Spells out Summary/What Changed/Test plan structure explicitly rather than
// leaving format to the agent's judgment — the vague predecessor ("descriptive
// title and a summary") produced bare one-line PR bodies with no test plan and
// no link back to why the change was made (see PRs #147/#148 on this repo).
const autoCreatePRPrompt = "Create a pull request for the changes in this session. Title: use Conventional Commits format (fix:, feat:, etc.). Body: structure as ## Summary (1-3 sentences on why this change was made, tied to the backlog item's problem statement in .backlog-context.md if present), ## What Changed (a short bullet list, not a line-by-line diff restatement), and ## Test plan (a checklist of concrete verification steps such as specific commands or manual checks — not an unqualified claim that tests pass). Keep it concise, no scratch notes."

// Timeouts for the AutoCreatePR trigger (server/review_queue_manager.go's
// maybeAutoCreatePR). autoCreatePRRunTimeout is passed explicitly as
// RunOneShotForSession's timeoutSeconds (rather than 0) so this value is the
// single source of truth — RunOneShot's own internal 900s default clamp
// (server/services/session_service.go) is a fallback for other callers, not a
// second definition this trigger depends on coincidentally matching.
const (
	autoCreatePRLookupTimeout = 20 * time.Second
	autoCreatePRRunTimeout    = 900 * time.Second

	// itemSessionLookupTimeout bounds the synchronous ItemSession lookup OnItemAdded
	// performs (see below) to resolve a backlog-linked item ID for notification
	// metadata. Kept short and separate from autoCreatePRLookupTimeout: this lookup
	// runs inline on the OnItemAdded call path (not in a background goroutine), so a
	// slow storage backend must not stall queue-add notification delivery.
	itemSessionLookupTimeout = 2 * time.Second
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

	// oneShotRunner is set via SetOneShotRunner and drives the opt-in AutoCreatePR
	// policy (see maybeAutoCreatePR). nil disables the feature entirely — safe
	// default for tests and any wiring path that doesn't call the setter.
	oneShotRunner OneShotPRCreator

	// callbackDispatcher is set via SetCallbackDispatcher and fires the
	// on_queue_item_created outbound callback (webhook-triggers Phase 5). nil
	// disables the feature entirely (Dispatch is also nil-receiver-safe, but
	// OnItemAdded nil-checks first to avoid the call entirely) — safe default
	// for tests and any wiring path that doesn't call the setter.
	callbackDispatcher *services.CallbackDispatcher

	// autoCreatePRInFlight tracks sessions with an in-progress AutoCreatePR
	// one-shot run, keyed by stable session UUID. Prevents a second concurrent
	// run for the same session when the review-queue item is removed and
	// re-added (acknowledgment, grace-period expiry, detection flicker) while
	// the first run — which can take up to autoCreatePRRunTimeout — is still
	// in flight and hasn't persisted GitHubPRURL yet. See maybeAutoCreatePR.
	autoCreatePRInFlight sync.Map

	// activityCh is sent to when EventApprovalResponse or EventUserInteraction arrives,
	// causing the poll loop to snap back to its fast interval immediately.
	activityCh chan struct{}

	// Streaming clients for WatchReviewQueue
	streamClients   map[string]*reviewQueueStreamClient
	streamClientsMu sync.RWMutex
	nextClientID    int
	nextClientIDMu  sync.Mutex

	// Subscription channels
	eventCh <-chan *events.Event
	subID   string

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// reviewQueueStreamClient represents a client streaming review queue events
type reviewQueueStreamClient struct {
	id      string
	filters *WatchReviewQueueFilters
	eventCh chan *sessionv1.ReviewQueueEvent
	ctx     context.Context
	cancel  context.CancelFunc
}

// WatchReviewQueueFilters contains filters for review queue event streaming
type WatchReviewQueueFilters struct {
	PriorityFilter    []session.Priority
	ReasonFilter      []session.AttentionReason
	SessionIDs        []string
	IncludeStatistics bool
	InitialSnapshot   bool
}

// NewReactiveQueueManager creates a new reactive queue manager.
func NewReactiveQueueManager(
	queue *session.ReviewQueue,
	poller *session.ReviewQueuePoller,
	eventBus *events.EventBus,
	statusManager *session.InstanceStatusManager,
	storage *session.Storage,
) *ReactiveQueueManager {
	// Buffered so senders never block even if the poll loop is momentarily busy.
	activityCh := make(chan struct{}, 1)
	poller.SetActivityChannel(activityCh)

	return &ReactiveQueueManager{
		queue:         queue,
		poller:        poller,
		eventBus:      eventBus,
		statusManager: statusManager,
		storage:       storage,
		activityCh:    activityCh,
		streamClients: make(map[string]*reviewQueueStreamClient),
	}
}

// SetOneShotRunner wires the one-shot PR-creation runner used by the opt-in
// AutoCreatePR policy (see maybeAutoCreatePR). Called post-construction from
// server/dependencies.go once SessionService is available — mirrors the
// existing SetHeadlessPool/SetStatusManager setter-injection pattern used
// elsewhere in this file's wiring to break a construction-order cycle.
func (rqm *ReactiveQueueManager) SetOneShotRunner(r OneShotPRCreator) {
	rqm.oneShotRunner = r
}

// SetCallbackDispatcher wires the outbound-callback dispatcher used to fire
// on_queue_item_created (see OnItemAdded, webhook-triggers Phase 5). Called
// post-construction from server/dependencies.go, same setter-injection pattern
// as SetOneShotRunner above.
func (rqm *ReactiveQueueManager) SetCallbackDispatcher(d *services.CallbackDispatcher) {
	rqm.callbackDispatcher = d
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

	log.Info("ReactiveQueueManager started with event-driven updates")
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
	if event == nil {
		return
	}
	switch event.Type {
	case events.EventUserInteraction:
		rqm.handleUserInteraction(event)
	case events.EventSessionAcknowledged:
		rqm.handleSessionAcknowledged(event)
	case events.EventApprovalResponse:
		rqm.handleApprovalResponse(event)
	case events.EventSessionDeleted:
		rqm.queue.Remove(rqm.resolveQueueKey(event.SessionID))
		rqm.signalActivity()
	}
}

// resolveQueueKey converts a session identifier (UUID or Title) to the queue key
// (Title). Queue items are keyed by inst.Title; events arrive with UUID.
func (rqm *ReactiveQueueManager) resolveQueueKey(id string) string {
	if inst := rqm.poller.FindInstance(id); inst != nil {
		return inst.Title
	}
	return id // fallback: already a Title, or instance no longer loaded
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
	log.Info("ReactiveQueueManager stopped")
}

// signalActivity non-blockingly notifies the poll loop to snap to its fast interval.
func (rqm *ReactiveQueueManager) signalActivity() {
	select {
	case rqm.activityCh <- struct{}{}:
	default:
	}
}

// OnControllerStatusChange is called by a ClaudeController's status-change goroutine
// when it detects a terminal status transition. Safe to call from any goroutine.
func (rqm *ReactiveQueueManager) OnControllerStatusChange(inst *session.Instance, _ detection.DetectedStatus) {
	rqm.signalActivity()
	go func() {
		select {
		case <-rqm.ctx.Done():
			return
		default:
		}
		rqm.poller.CheckSession(inst)
	}()
}

// handleUserInteraction handles user interaction events and immediately re-evaluates the queue.
func (rqm *ReactiveQueueManager) handleUserInteraction(event *events.Event) {
	sessionID := event.SessionID
	if sessionID == "" {
		return
	}

	log.Debug("ReactiveQueueManager user interaction", "session", sessionID, "type", event.InteractionType)

	// Find the instance
	inst := rqm.poller.FindInstance(sessionID)
	if inst == nil {
		log.Debug("ReactiveQueueManager instance not found", "session", sessionID)
		return
	}

	// Snap the poll loop back to fast interval so any new prompt surfaces quickly.
	rqm.signalActivity()

	// Immediate re-evaluation using exported method
	rqm.poller.CheckSession(inst)
}

// handleSessionAcknowledged handles session acknowledged events.
func (rqm *ReactiveQueueManager) handleSessionAcknowledged(event *events.Event) {
	sessionID := event.SessionID
	if sessionID == "" {
		return
	}

	log.Info("ReactiveQueueManager session acknowledged, removing from queue", "session", sessionID)

	// Immediate removal from queue — event SessionID may be a UUID; resolve to Title.
	removed := rqm.queue.Remove(rqm.resolveQueueKey(sessionID))
	if removed {
		log.Debug("ReactiveQueueManager session removed from queue", "session", sessionID)
	}
}

// handleApprovalResponse handles approval response events.
func (rqm *ReactiveQueueManager) handleApprovalResponse(event *events.Event) {
	sessionID := event.SessionID
	if sessionID == "" {
		return
	}

	approved := event.Approved
	log.Info("ReactiveQueueManager approval response, removing from queue", "session", sessionID, "approved", approved)

	// Find the instance and update status
	inst := rqm.poller.FindInstance(sessionID)
	if inst != nil && approved {
		if err := inst.Approve(); err != nil {
			log.Error("ReactiveQueueManager failed to approve session", "session", sessionID, "err", err)
		}
	}

	// Immediate removal from queue — event SessionID may be a UUID; resolve to Title.
	rqm.queue.Remove(rqm.resolveQueueKey(sessionID))

	// Snap the poll loop back to fast interval so any follow-up prompts surface quickly.
	rqm.signalActivity()
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

	// item.SessionID is the session title (the queue key). Resolve it to the
	// stable UUID so the web client can match the notification to a session, and
	// capture the resolved *session.Instance itself so its Hidden flag can gate
	// notification publishing below (a Hidden session — e.g. a headless
	// triage/review worker — should not surface routine TASK_COMPLETE/IDLE/STALE
	// notifications the way a normal user-facing session does).
	resolvedID := item.SessionID
	var inst *session.Instance
	if rqm.poller != nil {
		inst = rqm.poller.FindInstance(item.SessionID)
		if inst != nil {
			resolvedID = inst.GetStableID()
		}
	}
	hiddenSession := inst != nil && inst.Hidden

	// linkedItemID is the backlog item ID this session is linked to (if any),
	// resolved below via a bounded storage lookup. Kept as a local variable —
	// never written into item.Metadata, which is a pointer shared unlocked with
	// ReviewQueue.Add() and concurrently ranged over by WatchReviewQueue's
	// ReviewItemToProto goroutine (a data race / potential crash if mutated here).
	var linkedItemID string
	if rqm.storage != nil {
		lookupCtx, cancel := context.WithTimeout(rqm.baseContext(), itemSessionLookupTimeout)
		itemSession, err := rqm.storage.GetItemSessionBySessionUUID(lookupCtx, resolvedID)
		cancel()
		if err != nil {
			if !errors.Is(err, session.ErrNotFound) {
				log.Warn("OnItemAdded: ItemSession lookup failed", "session", resolvedID, "err", err)
			}
			// Not found (or any other lookup failure) — not a backlog-linked
			// session, or lookup unavailable. Silently proceed without item_id.
		} else {
			linkedItemID = itemSession.BacklogItemID
		}
	}

	// suppressForHidden narrows notification suppression to the routine reasons a
	// Hidden (headless/background) session churns through in the ordinary
	// course of automated work — TASK_COMPLETE/IDLE/STALE. ReasonErrorState and
	// ReasonTestsFailing must still publish even when Hidden: those indicate a
	// real problem an operator needs to see regardless of whether the session is
	// hidden from the default session list/review queue UI.
	suppressForHidden := hiddenSession && (item.Reason == session.ReasonTaskComplete || item.Reason == session.ReasonIdle || item.Reason == session.ReasonStale)

	// Publish an EventNotification to the EventBus so the notification history store
	// captures this event — but skip APPROVAL_PENDING items. The ApprovalHandler already
	// broadcasts a richer notification (with the actual command preview and approval UUID)
	// when the HTTP hook fires. Publishing again here would create a duplicate card in the
	// notification panel because APPROVAL_NEEDED records are never deduplicated server-side.
	if rqm.eventBus != nil && item.Reason != session.ReasonApprovalPending && !suppressForHidden {
		notifType, notifPriority := rqm.mapReviewItemToNotification(item)
		notifID := fmt.Sprintf("review-queue-%s-%d", item.SessionID, item.DetectedAt.UnixMilli())
		title := fmt.Sprintf("%s: %s", item.Reason.String(), item.SessionName)
		message := item.Context
		if message == "" {
			message = fmt.Sprintf("Session '%s' needs attention", item.SessionName)
		}

		metadata := events.SessionScopedMetadata(item.Metadata, linkedItemID)

		notifEvent := events.NewNotificationEvent(
			resolvedID,
			item.SessionName,
			notifID,
			notifType,
			notifPriority,
			title,
			message,
			metadata,
		)
		rqm.eventBus.Publish(notifEvent)
	}

	// Opt-in AutoCreatePR policy: if the backlog item behind this session has
	// enabled it, automatically run the same one-shot PR-creation prompt a human
	// would otherwise have to click "Create PR" + "Run" for manually. Runs async
	// so a slow/failing LLM call never blocks queue-add notification delivery.
	rqm.maybeAutoCreatePR(item)

	// AC4-shaped on_queue_item_created callback (webhook-triggers Phase 5, FR7):
	// Dispatch is itself non-blocking (bounded semaphore + go), so this adds no
	// latency here. rqm.callbackDispatcher is nil-checked rather than relying on
	// a nil *services.CallbackDispatcher receiver, matching this file's existing
	// nil-safety convention (e.g. oneShotRunner in maybeAutoCreatePR).
	if rqm.callbackDispatcher != nil {
		rqm.callbackDispatcher.Dispatch("queue_item_created", map[string]any{
			"event":       "queue_item_created",
			"session_id":  resolvedID,
			"session":     item.SessionName,
			"reason":      item.Reason.String(),
			"item_id":     linkedItemID,
			"occurred_at": time.Now(),
		})
	}
}

// maybeAutoCreatePR implements the opt-in "auto-create PR on Complete" policy
// (docs/tasks/backlog-feature-improvement.md, 2026-07-17 entry): when a session
// is at TASK_COMPLETE and the backlog item behind it has AutoCreatePR set, run
// the one-shot PR-creation prompt automatically instead of waiting for a human
// to click "Create PR" in the Review Queue. Off by default —
// SkipReviewGate/SkipPlanning/AutoSpawnSession precedent: opt-in per-item bool.
//
// Called from both OnItemAdded (item newly enters the queue already at
// TASK_COMPLETE) and OnQueueUpdated (item was already queued for a different
// reason — e.g. ReasonIdle while status detection lagged — and only later
// transitioned to TASK_COMPLETE; ReviewQueue.Add only fires OnItemAdded on the
// exists==false branch, so relying on OnItemAdded alone silently misses this
// transition). Safe to call repeatedly/redundantly for the same item — the
// in-flight guard and GitHubPRURL re-check below make it idempotent.
func (rqm *ReactiveQueueManager) maybeAutoCreatePR(item *session.ReviewItem) {
	if rqm.oneShotRunner == nil || rqm.storage == nil || rqm.poller == nil {
		return
	}
	if item.Reason != session.ReasonTaskComplete {
		return
	}

	inst := rqm.poller.FindInstance(item.SessionID)
	if inst == nil {
		return
	}
	if inst.GitHubPRURL != "" {
		return // already has a PR — nothing to do
	}
	stableID := inst.GetStableID()
	if stableID == "" {
		return
	}

	// Atomic check-and-set: only one in-flight run per session at a time.
	// Without this, a session removed and re-added to the queue (ack,
	// grace-period expiry, detection flicker) while a first run is still
	// executing — up to autoCreatePRRunTimeout, since GitHubPRURL isn't
	// persisted until the run completes — would pass the GitHubPRURL check
	// above a second time and launch a concurrent duplicate `claude -p` run
	// against the same worktree.
	if _, alreadyRunning := rqm.autoCreatePRInFlight.LoadOrStore(stableID, struct{}{}); alreadyRunning {
		return
	}

	rqm.wg.Add(1)
	go func() {
		defer rqm.wg.Done()
		defer rqm.autoCreatePRInFlight.Delete(stableID)

		lookupCtx, cancel := context.WithTimeout(rqm.baseContext(), autoCreatePRLookupTimeout)
		defer cancel()

		itemSession, err := rqm.storage.GetItemSessionBySessionUUID(lookupCtx, stableID)
		if err != nil || itemSession.BacklogItemID == "" {
			return // not a backlog-linked session — nothing to auto-create for
		}
		backlogItem, err := rqm.storage.GetBacklogItem(lookupCtx, itemSession.BacklogItemID)
		if err != nil || backlogItem == nil || !backlogItem.AutoCreatePR {
			return
		}

		// Re-check immediately before the expensive call: closes the TOCTOU
		// window between the synchronous check above and this goroutine
		// actually running — e.g. a human clicking the manual "Create PR"
		// button for this same session during the DB lookups just above.
		if inst.GitHubPRURL != "" {
			return
		}

		runCtx, runCancel := context.WithTimeout(rqm.baseContext(), autoCreatePRRunTimeout)
		defer runCancel()
		prURL, runErr := rqm.oneShotRunner.RunOneShotForSession(runCtx, stableID, autoCreatePRPrompt, int32(autoCreatePRRunTimeout.Seconds()))
		if runErr != nil {
			log.Warn("auto-create-PR: one-shot prompt failed", "session", inst.Title, "backlog_item", backlogItem.ID, "err", runErr)
			return
		}
		log.Info("auto-create-PR: PR created automatically", "session", inst.Title, "backlog_item", backlogItem.ID, "pr_url", prURL)
	}()
}

// baseContext returns rqm.ctx if the manager has been Start()ed, otherwise
// context.Background(). Guards maybeAutoCreatePR (and any other post-construction
// caller) against a nil rqm.ctx when invoked directly in tests without Start().
func (rqm *ReactiveQueueManager) baseContext() context.Context {
	if rqm.ctx != nil {
		return rqm.ctx
	}
	return context.Background()
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
	// AutoCreatePR: catch the transition-while-queued case OnItemAdded misses
	// (see maybeAutoCreatePR's doc comment) — a session already in the queue
	// for a different reason (e.g. ReasonIdle) that later reaches
	// ReasonTaskComplete fires OnQueueUpdated, not OnItemAdded. Safe to call
	// for every TASK_COMPLETE item on every update: maybeAutoCreatePR's
	// in-flight guard and GitHubPRURL check make repeated calls idempotent.
	for _, item := range items {
		if item.Reason == session.ReasonTaskComplete {
			rqm.maybeAutoCreatePR(item)
		}
	}

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

	log.Info("ReactiveQueueManager added streaming client", "client", clientID)

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
		log.Info("ReactiveQueueManager removed streaming client", "client", clientID)
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
				// Channel full, drop event
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
			log.Debug("ReactiveQueueManager recovered from panic in sendInitialSnapshot (client likely disconnected)", "client", client.id, "err", r)
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
	return adapters.ReviewItemToProto(item, nil)
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

// mapReviewItemToNotification maps a review item's attention reason and priority
// to notification type and priority int32 values (matching sessionv1 enums).
func (rqm *ReactiveQueueManager) mapReviewItemToNotification(item *session.ReviewItem) (int32, int32) {
	var notifType int32
	switch item.Reason {
	case session.ReasonApprovalPending:
		notifType = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_APPROVAL_NEEDED)
	case session.ReasonInputRequired, session.ReasonWaitingForUser:
		notifType = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED)
	case session.ReasonErrorState:
		notifType = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR)
	case session.ReasonTestsFailing:
		notifType = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_FAILURE)
	case session.ReasonTaskComplete:
		notifType = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE)
	case session.ReasonUncommittedChanges:
		notifType = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_STATUS_CHANGE)
	case session.ReasonIdle, session.ReasonStale, session.ReasonIdleTimeout:
		notifType = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO)
	default:
		notifType = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO)
	}

	var notifPriority int32
	switch item.Priority {
	case session.PriorityUrgent:
		notifPriority = int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT)
	case session.PriorityHigh:
		notifPriority = int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH)
	case session.PriorityMedium:
		notifPriority = int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM)
	case session.PriorityLow:
		notifPriority = int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW)
	default:
		notifPriority = int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM)
	}

	return notifType, notifPriority
}
