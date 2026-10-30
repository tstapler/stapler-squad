package session

import "github.com/linkdata/deadlock"

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// StatusProvider is the interface ReviewQueuePoller uses to fetch session status.
// Defined at the consumption point (the poller), not the production point.
type StatusProvider interface {
	GetStatus(inst *Instance) InstanceStatusInfo
	GetController(instanceTitle string) (*ClaudeController, bool)
}

// ContentProvider fetches terminal content for a session.
// Defined at the consumption point so tests can inject fakes without tmux.
type ContentProvider interface {
	GetContent(inst *Instance, statusInfo InstanceStatusInfo, paneActivity map[string]time.Time) string
	EvictInstance(title string)
}

// ReviewQueuePollerConfig contains configuration for the review queue poller.
type ReviewQueuePollerConfig struct {
	PollInterval       time.Duration // How often to check sessions (fast path, default 2s)
	SlowPollInterval   time.Duration // Interval when review queue is empty (default 8s); 0 = no backoff
	IdleThreshold      time.Duration // Duration before considering session idle and adding to queue
	InputWaitDuration  time.Duration // Time waiting for input before flagging
	StalenessThreshold time.Duration // Duration since last meaningful output before considering stale
	ReconcileInterval  time.Duration // How often to reconcile in-memory state against tmux reality (0 = disabled)
}

// DefaultReviewQueuePollerConfig returns sensible defaults for polling.
func DefaultReviewQueuePollerConfig() ReviewQueuePollerConfig {
	return ReviewQueuePollerConfig{
		PollInterval:       2 * time.Second,  // Poll every 2 seconds for immediate detection
		SlowPollInterval:   8 * time.Second,  // Back off to 8s when queue is empty
		IdleThreshold:      5 * time.Second,  // Add to queue after 5s idle for immediate user notifications
		InputWaitDuration:  3 * time.Second,  // Flag if waiting for input > 3s (reduced from 5s)
		StalenessThreshold: 5 * time.Minute,  // Flag if no meaningful output for 5 minutes (reverted from an undocumented 2min reduction — a single slow LLM turn routinely exceeds 2min without being stuck; see project_plans/review-gate-stale-session-rework/decisions/ADR-001-staleness-threshold-recalibration.md)
		ReconcileInterval:  30 * time.Second, // Reconcile against tmux reality every 30 seconds
	}
}

// ApprovalMetadata holds metadata about a pending approval for enriching review queue items.
type ApprovalMetadata struct {
	ApprovalID string
	ToolName   string
	ToolInput  map[string]interface{}
	Cwd        string
	Orphaned   bool

	// EscalationReason and EscalationCategory explain why this request was
	// escalated for manual review (no-match/explicit-rule/domain-age/
	// unclassifiable/unexpected). Copied from PendingApproval via
	// ApprovalStore.GetApprovalMetadataBySession.
	EscalationReason   string
	EscalationCategory string

	// RiskLevel is the classifier-assigned risk level ("low"/"medium"/"high"/"critical"),
	// copied from PendingApproval.RiskLevel. "" means not recorded — never treated as "low".
	RiskLevel string
}

// ApprovalMetadataProvider provides approval metadata for enriching review queue items.
// This interface decouples the poller (session package) from the ApprovalStore (services package).
type ApprovalMetadataProvider interface {
	// GetApprovalMetadataBySession returns approval metadata for the given session ID.
	// Returns nil if no approvals exist for the session.
	GetApprovalMetadataBySession(sessionID string) []ApprovalMetadata
}

// riskLevelRankTable orders RiskLevel strings by severity, highest first. Unrecorded ("")
// ranks alongside "high" — fail-safe, since an unclassified request must never be treated as
// safe. Never read this map directly — use riskLevelRank(), which falls back to the
// unrecorded rank for any key (including a future/unrecognized RiskLevel string) absent from
// this table, so an unknown value never silently ranks *below* "low" (the Go zero value for
// a missing map key). Mirrors web-app/src/lib/sessions/riskLevel.ts's riskLevelRank().
var riskLevelRankTable = map[string]int{
	"critical": 4,
	"high":     3,
	"":         3,
	"medium":   2,
	"low":      1,
}

// riskLevelRank returns level's fail-safe rank, falling back to the unrecorded ("") rank for
// any value not in riskLevelRankTable — see that table's doc comment for why.
func riskLevelRank(level string) int {
	if rank, ok := riskLevelRankTable[level]; ok {
		return rank
	}
	return riskLevelRankTable[""]
}

// highestRiskApproval returns the most dangerous of a session's concurrent pending approvals
// (GAP-004, docs/bugs/open/review-queue-gaps.md) so the review queue surfaces the item most
// in need of attention rather than an arbitrary one. Ties keep the earliest (first-inserted)
// approval, since approvals is already in creation order.
//
// Panics if approvals is empty — the only call site guards with len(approvals) > 0 first.
func highestRiskApproval(approvals []ApprovalMetadata) ApprovalMetadata {
	best := approvals[0]
	for _, a := range approvals[1:] {
		if riskLevelRank(a.RiskLevel) > riskLevelRank(best.RiskLevel) {
			best = a
		}
	}
	return best
}

// ReviewQueuePoller automatically monitors sessions and adds them to the review queue
// when they become idle or need attention.
type ReviewQueuePoller struct {
	queue            *ReviewQueue
	statusManager    StatusProvider
	storage          *Storage
	instances        []*Instance
	config           ReviewQueuePollerConfig
	statusDetector   detection.TerminalDetector // For detecting status in sessions without ClaudeController
	approvalProvider ApprovalMetadataProvider   // Optional: enriches approval items with hook metadata
	contentProvider  ContentProvider            // Fetches and caches terminal content
	statusDeterminer StatusDeterminer           // Evaluates whether session should be in queue
	tmuxSocket       TmuxSocketQuerier          // Tmux server-socket queries for reconcileSessions; fakeable in tests
	optionsSource    FlagOptionsSource          // Attention flags, read once per poll cycle; nil = all off
	explain          suppressionExplainer       // Why-suppressed and why-skipped visibility
	recorder         SessionEventRecorder       // Attention raised/cleared sink; nil = no-op
	instanceWirer    func(*Instance)            // Registers per-instance collaborators (turn-end sink, recorder); nil = none

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     deadlock.RWMutex

	// activityCh is an optional channel that signals the poll loop to snap back to
	// the fast interval. Wired by ReactiveQueueManager on EventApprovalResponse and
	// EventUserInteraction. A nil channel is never selected (safe in select statements).
	activityCh <-chan struct{}

	// archivedLivePaneWarned throttles warnArchivedLivePaneOnce to one record
	// per session title per process; RemoveInstance drops the entry so the map
	// tracks live instances, not every title seen. See that method's doc comment.
	archivedLivePaneWarned sync.Map

	// Backoff state: tracks consecutive poll errors to apply exponential delay.
	consecutiveErrors int
	// tickCount counts poll loop iterations; atomic because pollLoop writes and tests read concurrently.
	tickCount atomic.Int64
}

// contentCacheEntry bundles all per-session cache fields into one value so
// reads and writes can be done with a single lock-free map operation.
type contentCacheEntry struct {
	cachedContent        string
	lastSeenActivity     time.Time // last IdleDetector.lastActivity seen (controller path)
	lastSeenPaneActivity time.Time // last #{pane_last_activity} seen (no-controller path)
	lastPreviewTime      time.Time // fallback TTL timestamp (no-controller path)
}

// pollerContentProvider is the default ContentProvider implementation that owns all
// content caching state. It is created by NewReviewQueuePoller and can be replaced
// in tests with a fake implementation.
type pollerContentProvider struct {
	// ponytail: xsync.Map replaces 4 map[string]* + RWMutex — lock-free reads across sessions
	cache *xsync.Map[string, contentCacheEntry]
}

// NewPollerContentProvider creates a new pollerContentProvider.
// It is exported so server/dependencies.go can pass it to NewStartupScanner.
func NewPollerContentProvider() ContentProvider {
	return &pollerContentProvider{
		cache: xsync.NewMap[string, contentCacheEntry](),
	}
}

// EvictInstance removes all cache entries for the given session title.
func (p *pollerContentProvider) EvictInstance(title string) {
	p.cache.Delete(title)
}

// NewReviewQueuePoller creates a new poller for automatically managing the review queue.
// The storage parameter is optional (can be nil) but required for persisting LastAddedToQueue timestamps.
func NewReviewQueuePoller(queue *ReviewQueue, statusManager StatusProvider, storage *Storage) *ReviewQueuePoller {
	return NewReviewQueuePollerWithConfig(queue, statusManager, storage, DefaultReviewQueuePollerConfig())
}

// NewReviewQueuePollerWithConfig creates a poller with custom configuration.
// The storage parameter is optional (can be nil) but required for persisting LastAddedToQueue timestamps.
func NewReviewQueuePollerWithConfig(queue *ReviewQueue, statusManager StatusProvider, storage *Storage, config ReviewQueuePollerConfig) *ReviewQueuePoller {
	return &ReviewQueuePoller{
		queue:            queue,
		statusManager:    statusManager,
		storage:          storage,
		instances:        make([]*Instance, 0),
		config:           config,
		statusDetector:   detection.NewStatusDetector(),
		contentProvider:  NewPollerContentProvider(),
		statusDeterminer: NewDefaultStatusDeterminer(config),
		tmuxSocket:       realTmuxSocketQuerier{},
	}
}

// SetInstances sets the list of instances to monitor.
func (rqp *ReviewQueuePoller) SetInstances(instances []*Instance) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	rqp.instances = instances
	rqp.wireInstances(instances)
}

// SetInstanceWirer sets the registration hook applied to every instance the poller tracks,
// now and on later SetInstances/AddInstance calls, so no creation path has to remember it.
func (rqp *ReviewQueuePoller) SetInstanceWirer(wire func(*Instance)) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	rqp.instanceWirer = wire
	rqp.wireInstances(rqp.instances)
}

// wireInstances requires rqp.mu held.
func (rqp *ReviewQueuePoller) wireInstances(instances []*Instance) {
	if rqp.instanceWirer == nil {
		return
	}
	for _, inst := range instances {
		rqp.instanceWirer(inst)
	}
}

// AddInstance adds a single instance to monitor.
func (rqp *ReviewQueuePoller) AddInstance(instance *Instance) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	// Stagger this session's dirty-cache TTL so sessions added in a burst
	// (e.g. bulk startup) don't all run git-status subprocesses simultaneously.
	instance.gitManager.PrimeDirtyCacheJitter()
	rqp.instances = append(rqp.instances, instance)
	rqp.wireInstances([]*Instance{instance})
}

// RemoveInstance removes an instance from monitoring.
func (rqp *ReviewQueuePoller) RemoveInstance(instanceTitle string) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()

	filtered := make([]*Instance, 0, len(rqp.instances))
	var removedTitle string
	for _, inst := range rqp.instances {
		if inst.MatchesID(instanceTitle) {
			removedTitle = inst.Title
		} else {
			filtered = append(filtered, inst)
		}
	}
	rqp.instances = filtered

	// Evict content cache using the resolved title (MatchesID may have matched by UUID).
	evictKey := instanceTitle
	if removedTitle != "" {
		evictKey = removedTitle
	}
	rqp.contentProvider.EvictInstance(evictKey)
	rqp.archivedLivePaneWarned.Delete(evictKey)
	rqp.explain.forget(evictKey)
}

// SetApprovalProvider sets the approval metadata provider for enriching review queue items.
func (rqp *ReviewQueuePoller) SetApprovalProvider(provider ApprovalMetadataProvider) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	rqp.approvalProvider = provider
}

// SetActivityChannel wires an external signal channel to the poll loop. When a signal
// arrives on ch, the loop snaps back to the fast interval (PollInterval). Must be called
// before Start(); subsequent calls have no effect once the loop is running.
func (rqp *ReviewQueuePoller) SetActivityChannel(ch <-chan struct{}) {
	rqp.mu.Lock()
	rqp.activityCh = ch
	rqp.mu.Unlock()
}

// Start begins polling for idle sessions.
func (rqp *ReviewQueuePoller) Start(ctx context.Context) {
	rqp.mu.Lock()
	if rqp.ctx != nil {
		rqp.mu.Unlock()
		log.Info("ReviewQueuePoller already started")
		return
	}

	rqp.ctx, rqp.cancel = context.WithCancel(ctx)
	rqp.mu.Unlock()

	// STARTUP CLEANUP: Remove orphaned queue items with invalid timestamps
	// This handles items that were persisted before the LastMeaningfulOutput migration
	rqp.cleanupOrphanedItems()

	// Perform initial queue population immediately on startup
	// This ensures the queue is populated without waiting for the first poll interval
	rqp.checkSessions()

	rqp.wg.Add(1)
	go rqp.pollLoop()

	log.Info("ReviewQueuePoller started", "poll_interval", rqp.config.PollInterval)
}

// Wait blocks until the poll loop launched by Start has exited. Unlike Stop it
// does not cancel anything; call it after cancelling Start's ctx.
func (rqp *ReviewQueuePoller) Wait() {
	rqp.wg.Wait()
}

// Stop stops the poller.
func (rqp *ReviewQueuePoller) Stop() {
	rqp.mu.Lock()
	if rqp.cancel != nil {
		rqp.cancel()
	}
	rqp.mu.Unlock()

	rqp.wg.Wait()
	log.Info("ReviewQueuePoller stopped")
}

// cleanupOrphanedItems removes queue items with zero or invalid LastActivity timestamps.
// This handles orphaned items that were persisted before the LastMeaningfulOutput migration
// and never got cleaned up. Should be called once during startup.
func (rqp *ReviewQueuePoller) cleanupOrphanedItems() {
	// Get all items currently in queue
	allItems := rqp.queue.List()

	// Timestamp validation threshold - any timestamp before this is considered invalid
	minValidTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	removedCount := 0
	for _, item := range allItems {
		// Remove items with zero or invalid LastActivity timestamps
		if item.LastActivity.IsZero() || item.LastActivity.Before(minValidTime) {
			log.Info("startup cleanup: removing orphaned item with invalid LastActivity", "session", item.SessionID, "last_activity", item.LastActivity)
			rqp.removeFromQueue(item.SessionID)
			removedCount++
		}
	}

	if removedCount > 0 {
		log.Info("startup cleanup: removed orphaned items with invalid timestamps", "count", removedCount)
	} else {
		log.Info("startup cleanup: no orphaned items found")
	}
}

// pollLoop is the main polling loop that runs in the background.
//
// Adaptive interval: when an activityCh is wired and the review queue becomes empty,
// the loop backs off to SlowPollInterval (8s by default). Any signal on activityCh
// snaps the interval back to PollInterval (2s) immediately.
func (rqp *ReviewQueuePoller) pollLoop() {
	defer rqp.wg.Done()

	fastInterval := rqp.config.PollInterval
	slowInterval := rqp.config.SlowPollInterval
	if slowInterval <= 0 {
		slowInterval = fastInterval
	}

	// Capture activityCh once; must be set before Start() for the snap behavior to work.
	// A nil channel is never selected in a select statement, so the adaptive path is
	// simply skipped when no activity channel is wired.
	rqp.mu.RLock()
	actCh := rqp.activityCh
	rqp.mu.RUnlock()

	interval := fastInterval
	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-rqp.ctx.Done():
			return

		case <-actCh:
			// Snap back to fast interval on external activity signal.
			if interval != fastInterval {
				interval = fastInterval
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(interval)
			}

		case <-timer.C:
			rqp.tickCount.Add(1)

			if err := rqp.checkSessionsSafe(); err != nil {
				rqp.consecutiveErrors++
				backoff := rqp.backoffDuration(rqp.consecutiveErrors)
				log.Warn("ReviewQueuePoller checkSessions error", "consecutive_errors", rqp.consecutiveErrors, "err", err, "backoff", backoff)
				select {
				case <-rqp.ctx.Done():
					return
				case <-time.After(backoff):
				}
				timer.Reset(interval)
				continue
			}
			rqp.consecutiveErrors = 0

			// Adaptive interval: back off when queue is empty and activity channel is wired.
			if actCh != nil && rqp.queue.Count() == 0 {
				interval = slowInterval
			} else {
				interval = fastInterval
			}
			timer.Reset(interval)

			// Periodic reconciliation: verify in-memory state matches tmux reality.
			// Runs every ReconcileInterval ticks; skipped when ReconcileInterval is 0.
			if rqp.config.ReconcileInterval > 0 {
				ticksPerReconcile := int(rqp.config.ReconcileInterval / rqp.config.PollInterval)
				if ticksPerReconcile < 1 {
					ticksPerReconcile = 1
				}
				if rqp.tickCount.Load()%int64(ticksPerReconcile) == 0 {
					rqp.reconcileSessions()
				}
			}
		}
	}
}

// checkSessionsSafe wraps checkSessions with panic recovery, returning an error on panic.
func (rqp *ReviewQueuePoller) checkSessionsSafe() (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("panic in checkSessions: %v", r)
			log.Error("ReviewQueuePoller panic recovered", "panic", r)
		}
	}()
	rqp.checkSessions()
	return nil
}

// backoffDuration returns the exponential backoff duration for the given error count.
// Caps at 30 seconds: 2s, 4s, 8s, 16s, 30s, 30s, ...
func (rqp *ReviewQueuePoller) backoffDuration(consecutiveErrors int) time.Duration {
	const base = 2 * time.Second
	const maxBackoff = 30 * time.Second
	if consecutiveErrors <= 0 {
		return base
	}
	shift := consecutiveErrors - 1
	if shift > 10 {
		shift = 10 // prevent overflow
	}
	backoff := base * (1 << uint(shift))
	if backoff > maxBackoff {
		backoff = maxBackoff
	}
	return backoff
}

// ForceReconcile immediately runs session reconciliation outside the normal 30s cadence.
// Safe to call concurrently; typically used by the fork pressure monitor to rapidly clean
// up dead sessions when subprocess failures indicate stale Active states.
func (rqp *ReviewQueuePoller) ForceReconcile() {
	log.Info("ReviewQueuePoller ForceReconcile triggered")
	rqp.reconcileSessions()
}

// reconcileSessions compares in-memory instances against live tmux sessions.
// - Active instances not found in tmux are transitioned to Stopped.
// - Stopped instances whose tmux session is found alive are revived to Active.
func (rqp *ReviewQueuePoller) reconcileSessions() {
	rqp.mu.RLock()
	instances := make([]*Instance, 0, len(rqp.instances))
	for _, inst := range rqp.instances {
		// Filtered before grouping/querying so a socket populated only by
		// push-liveness instances triggers no ListSessions call at all --
		// see ProcessManagerBackend.SkipsPollBasedLiveness's doc comment.
		if inst.Snapshot().Backend.SkipsPollBasedLiveness() {
			continue
		}
		instances = append(instances, inst)
	}
	rqp.mu.RUnlock()

	if len(instances) == 0 {
		return
	}

	// Group instances by their own tmux server socket -- see groupInstancesBySocket
	// for why querying a single assumed-shared socket is wrong. Unmanaged instances
	// are skipped inside the per-socket loop below, same as before.
	bySocket := groupInstancesBySocket(instances)

	for serverSocket, socketInstances := range bySocket {
		liveSessions, err := rqp.tmuxSocket.ListSessions(serverSocket)
		if err != nil {
			if err == tmux.ErrServerDown {
				log.Warn("reconcileSessions: tmux server is down, skipping reconciliation", "socket", serverSocket)
			} else {
				log.Warn("reconcileSessions: ListAllSessions error", "socket", serverSocket, "err", err)
			}
			continue
		}

		for _, inst := range socketInstances {
			if !inst.IsManaged {
				continue
			}
			sessionName := inst.GetTmuxSessionName()
			if sessionName == "" {
				continue
			}

			// Status(inst.GetStatus()) reads the lock-free published snapshot
			// (Instance.snapshot, an atomic.Pointer[InstanceSnapshot]) instead of
			// inst.Status directly. This switch runs on the poller's own goroutine,
			// not the actor's, so a raw field read here races with
			// transitionToLocked's write under -race. GetStatus() is the correct,
			// zero-lock way to read status from outside an in-flight actor command --
			// it's only unsafe to call *inside* one (see the s.inst.mu.RLock() reads
			// below). Tracked flake: https://github.com/tstapler/stapler-squad/issues/271

			// Never revive an archived session to Active (see ADR-001,
			// superseded-rework-session-retirement). The Active case below is
			// deliberately NOT guarded — an archived row must still converge to
			// Stopped.
			archived := inst.IsArchived()

			switch Status(inst.GetStatus()) {
			case Active:
				// Active but tmux session gone — mark Stopped.
				if !liveSessions[sessionName] {
					log.Warn("reconcileSessions: managed session not found in live sessions, transitioning to Stopped", "session", inst.Title, "tmux", sessionName, "socket", serverSocket)
					ctx2s, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					_ = inst.sendCtx(ctx2s, func(s *instanceState) {
						// Can't use GetStatus()/Snapshot() here: this closure runs
						// *inside* an in-flight actor command, and the published
						// snapshot is only republished by runActor after the command
						// returns (session/actor.go) -- Snapshot() would read the stale
						// pre-command value. Fall back to the same i.mu.RLock()-guarded
						// direct read transitionToLocked uses for its own status read
						// (session/instance_state.go), which defends against legacy
						// setters writing i.Status directly under i.mu from other
						// goroutines while this actor command runs.
						s.inst.mu.RLock()
						status := s.inst.Status
						s.inst.mu.RUnlock()
						if status == Active {
							if err := transitionToLocked(s, context.Background(), Stopped); err != nil {
								log.Warn("reconcileSessions: transition to Stopped failed, using loadStatus", "session", inst.Title, "err", err)
								s.inst.loadStatus(Stopped)
							}
						}
					})
					cancel()
					rqp.removeFromQueue(inst.Title)
					inst.fireLifecycleEvent(EventExited, "reconcile-session-missing")
				}
			case Stopped:
				// Stopped but tmux session is alive — revive to Active only if
				// the wrapped program is genuinely still running. liveSessions
				// only proves the tmux pane object exists; remain-on-exit keeps
				// that true even after the wrapped program has exited (a dead
				// "Pane is dead (signal N, ...)" placeholder, see
				// PaneProcessDead's doc comment) — blindly reviving on pane
				// existence alone previously left such sessions stuck showing
				// Active with no live process behind them (frozen terminal, no
				// response to input, confirmed 2026-09-06 via a restart that
				// raced a controller's PTY-EOF transition to Stopped against
				// this poller's next tick).
				if liveSessions[sessionName] {
					if archived {
						rqp.warnArchivedLivePaneOnce(inst, sessionName, serverSocket)
						continue
					}
					if dead, _, _ := inst.paneExitInfoIgnoringStatus(); dead {
						log.Info("reconcileSessions: stopped session's pane exists but wrapped program has exited, leaving Stopped", "session", inst.Title, "tmux", sessionName, "socket", serverSocket)
						continue
					}
					log.Info("reconcileSessions: stopped session found alive, reviving to Active", "session", inst.Title, "tmux", sessionName, "socket", serverSocket)
					ctx2s, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					_ = inst.sendCtx(ctx2s, func(s *instanceState) {
						s.inst.mu.RLock()
						status := s.inst.Status
						s.inst.mu.RUnlock()
						if status == Stopped {
							if err := transitionToLocked(s, context.Background(), Active); err != nil {
								log.Warn("reconcileSessions: revival to Active failed", "session", inst.Title, "err", err)
							}
						}
					})
					cancel()
					inst.fireLifecycleEvent(EventStarted, "reconcile-session-revived")
				}
			case Hibernated:
				// Hibernated sessions intentionally have no tmux session (Hibernate()
				// explicitly kills it) -- that's the expected steady state, so do
				// nothing when the tmux session is also gone. But if one is found
				// alive anyway (e.g. a code path resurrected it without checking
				// IsHibernated() first), bring the instance back in sync via the same
				// transition streamTerminal's hibernation guard uses deliberately:
				// this restarts the controller/session driver, instead of leaving
				// Status stuck at Hibernated forever while a real tmux session runs
				// unmanaged underneath it (Preview() short-circuits for Hibernated,
				// so such a session would otherwise look permanently dead).
				if liveSessions[sessionName] {
					if archived {
						rqp.warnArchivedLivePaneOnce(inst, sessionName, serverSocket)
						continue
					}
					log.Warn("reconcileSessions: hibernated session found alive in tmux, resuming to Active", "session", inst.Title, "tmux", sessionName, "socket", serverSocket)
					ctx2s, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					_ = inst.sendCtx(ctx2s, func(s *instanceState) {
						s.inst.mu.RLock()
						status := s.inst.Status
						s.inst.mu.RUnlock()
						if status == Hibernated {
							if err := transitionToLocked(s, context.Background(), Active); err != nil {
								log.Warn("reconcileSessions: resume from hibernation failed", "session", inst.Title, "err", err)
							}
						}
					})
					cancel()
					inst.fireLifecycleEvent(EventStarted, "reconcile-session-hibernated-but-alive")
				}
			case Crashed:
				// Crashed sessions intentionally have no tmux session (MarkCrashed
				// kills it before setting this status, session/instance_crash.go) --
				// that's the expected steady state until an explicit resume. But if
				// one is found alive anyway, bring the instance back in sync rather
				// than leaving it stuck showing the Crashed banner over a live pane,
				// mirroring the Hibernated case just above.
				if liveSessions[sessionName] {
					if archived {
						rqp.warnArchivedLivePaneOnce(inst, sessionName, serverSocket)
						continue
					}
					log.Warn("reconcileSessions: crashed session found alive in tmux, reviving to Active", "session", inst.Title, "tmux", sessionName, "socket", serverSocket)
					ctx2s, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					_ = inst.sendCtx(ctx2s, func(s *instanceState) {
						if s.inst.Status == Crashed {
							if err := transitionToLocked(s, context.Background(), Active); err != nil {
								log.Warn("reconcileSessions: revival from crashed failed", "session", inst.Title, "err", err)
							}
						}
					})
					cancel()
					inst.fireLifecycleEvent(EventStarted, "reconcile-session-crashed-but-alive")
				}
			case PermanentlyFailed:
				// Deliberately not auto-revived even if tmux is alive, unlike
				// Stopped/Hibernated/Crashed above — PermanentlyFailed means the
				// configurable retry policy already exhausted its attempt budget
				// for this failure episode; only an explicit RetryNow() call
				// (the "Retry now" UI action / RetrySession RPC) should bring it
				// back to Active. See ADR-001.
			}
		}
	}
}

// warnArchivedLivePaneOnce logs, at most once per session title per process,
// that an archived session still has a tmux pane object. Such a row is skipped
// by every reconciler and hidden from ListSessions, so an orphaned claude
// process behind it would otherwise surface nowhere (see ADR-001,
// superseded-rework-session-retirement); this is the detector for it, not the
// reaper. Keyed on Title, the codebase's conventional instance key, so
// RemoveInstance can drop the entry alongside the content-cache eviction; a
// rename re-arms the warning once.
func (rqp *ReviewQueuePoller) warnArchivedLivePaneOnce(inst *Instance, sessionName, serverSocket string) {
	snap := inst.Snapshot()
	if _, dup := rqp.archivedLivePaneWarned.LoadOrStore(snap.Title, struct{}{}); dup {
		return
	}
	// Probed after the de-dup: paneExitInfoIgnoringStatus runs several tmux
	// subprocesses, so it must not fire on every tick.
	dead, _, _ := inst.paneExitInfoIgnoringStatus()
	log.Warn("reconcileSessions: archived session still has a live tmux pane object; not reviving",
		"session", snap.Title, "tmux", sessionName, "socket", serverSocket,
		"status", snap.Status, "pane_process_dead", dead,
		"hint", "if pane_process_dead=false this is an orphaned process — `tmux kill-session -t <tmux>`")
}

// checkSessionsConcurrency caps the number of sessions checked simultaneously,
// limiting concurrent subprocess (capture-pane) calls to avoid fork exhaustion
// on macOS (kern.maxprocperuid).
const checkSessionsConcurrency = 5

// checkSessions checks all instances and updates the review queue.
func (rqp *ReviewQueuePoller) checkSessions() {
	rqp.mu.RLock()
	instances := make([]*Instance, len(rqp.instances))
	copy(instances, rqp.instances)
	rqp.mu.RUnlock()

	if len(instances) == 0 {
		return
	}

	opts, ruleOpts := rqp.currentOptions()

	// Fetch pane activity timestamps once for all sessions. This single subprocess call
	// replaces per-session capture-pane calls when content hasn't changed.
	paneActivity := batchPaneActivity("")

	sem := make(chan struct{}, checkSessionsConcurrency)
	var wg sync.WaitGroup
	for _, inst := range instances {
		sem <- struct{}{}
		wg.Add(1)
		go func(i *Instance) {
			defer wg.Done()
			defer func() { <-sem }()
			rqp.evaluateSession(i, paneActivity, opts, ruleOpts)
		}(inst)
	}
	wg.Wait()
}

// previewCacheTTL is the fallback maximum age of a cached Preview() result when
// pane activity timestamps are unavailable (e.g. tmux not running). The primary
// invalidation mechanism is #{pane_last_activity} from batchPaneActivity(); this
// TTL is only a safety net and is intentionally long.
const previewCacheTTL = 30 * time.Second

// GetContent returns the terminal content for inst, using a cache to avoid
// spawning a subprocess when no new output has arrived since the last poll.
//
// For sessions with an active ClaudeController: the idle detector's lastActivity
// timestamp (driven by PTY reads, no subprocess) is the change signal.
//
// For sessions without a ClaudeController: #{pane_last_activity} from the
// paneActivity snapshot (one `tmux list-panes -a` call shared across all sessions)
// is the change signal. capture-pane is only called when that timestamp advances.
// A 30s TTL acts as a fallback when paneActivity is nil (tmux unavailable).
//
// On error, the last cached content is returned so callers see empty string only
// on the very first poll for a session.
func (p *pollerContentProvider) GetContent(inst *Instance, statusInfo InstanceStatusInfo, paneActivity map[string]time.Time) string {
	// Lock-free read: load the single bundled entry for this session.
	entry, _ := p.cache.Load(inst.Title)

	if statusInfo.IsControllerActive {
		lastActivity := statusInfo.IdleState.LastActivity
		if !lastActivity.IsZero() && lastActivity.Equal(entry.lastSeenActivity) {
			return entry.cachedContent
		}
	} else {
		if paneActivity != nil {
			tmuxName := inst.GetTmuxSessionName()
			if currentActivity, ok := paneActivity[tmuxName]; ok {
				if !currentActivity.IsZero() && currentActivity.Equal(entry.lastSeenPaneActivity) {
					return entry.cachedContent
				}
			} else if !entry.lastPreviewTime.IsZero() && time.Since(entry.lastPreviewTime) < previewCacheTTL {
				// The session's pane is absent from the batch snapshot entirely (its
				// tmux session has died or never existed). Without this fallback, a
				// dead session's capture-pane gets retried on every single poll cycle
				// forever instead of backing off -- observed as thousands of failed
				// subprocess spawns per hour for a handful of orphaned sessions.
				return entry.cachedContent
			}
		} else if !entry.lastPreviewTime.IsZero() && time.Since(entry.lastPreviewTime) < previewCacheTTL {
			return entry.cachedContent
		}
	}

	content, err := inst.Preview()
	if err != nil {
		log.Debug("Preview() error", "session", inst.Title, "err", err)
		// Record the attempt even on failure. Without this, lastPreviewTime stays
		// at its zero value forever for a session whose Preview() never succeeds,
		// which defeats the previewCacheTTL backoff above and re-triggers a fresh
		// capture-pane subprocess on every single poll cycle indefinitely.
		p.cache.Compute(inst.Title, func(e contentCacheEntry, _ bool) (contentCacheEntry, xsync.ComputeOp) {
			e.lastPreviewTime = time.Now()
			return e, xsync.UpdateOp
		})
		return entry.cachedContent
	}

	// Update LastMeaningfulOutput when new terminal content is detected.
	// This ensures sessions resurface in the review queue after producing new output,
	// even when the user hasn't visited them via WebSocket streaming.
	// The content-signature dedup in UpdateTimestamps() (persisted to DB) prevents
	// false positives: if content is cosmetically changed but semantically the same
	// as when the user last acknowledged, LastMeaningfulOutput is not updated and
	// the acknowledgment snooze is preserved.
	if content != "" {
		inst.UpdateTerminalTimestamps(content, false)
	}

	// Atomic write: Compute replaces the entry under a per-key shard lock (no I/O inside).
	newActivity := statusInfo.IdleState.LastActivity
	isController := statusInfo.IsControllerActive
	var newPaneActivity time.Time
	if !isController && paneActivity != nil {
		tmuxName := inst.GetTmuxSessionName()
		if t, ok := paneActivity[tmuxName]; ok && !t.IsZero() {
			newPaneActivity = t
		}
	}
	now := time.Now()
	p.cache.Compute(inst.Title, func(e contentCacheEntry, _ bool) (contentCacheEntry, xsync.ComputeOp) {
		e.cachedContent = content
		if isController && !newActivity.IsZero() {
			e.lastSeenActivity = newActivity
		} else {
			if !newPaneActivity.IsZero() {
				e.lastSeenPaneActivity = newPaneActivity
			}
			e.lastPreviewTime = now
		}
		return e, xsync.UpdateOp
	})

	return content
}

// shouldSkipSession returns true for sessions the poller should not evaluate.
// Don't check sessions that are not running or are explicitly paused.
// Hidden (system/background) sessions are never shown in the review queue.
// Archived sessions (ArchivedAt set) are deliberately superseded/retired — e.g. by
// archiveItemWorkSessions during a backlog item reopen, which sets ArchivedAt and kills
// the tmux pane but does not change Hidden or Status — so they must be excluded here too,
// or a dead pane never produces new activity and the session sits in the queue as a
// permanent false ATTENTION_REASON_STALE entry (see docs/tasks/backlog-feature-improvement.md,
// 2026-08-02 entry).
// All other states proceed to status detection regardless of controller state.
func (rqp *ReviewQueuePoller) shouldSkipSession(inst *Instance) bool {
	// Lock-free snapshot read for Hidden, Status, and ArchivedAt; Started() reads
	// inst.started (set once during construction, not in the snapshot).
	// Status.IsSuspended() covers Paused/Hibernated/Stopped/Crashed/PermanentlyFailed:
	// none of these have a live tmux pane to check (SessionHealthChecker already
	// tore it down, or it was never created), so there is no pane content to
	// evaluate, and each surfaces to the user via its own distinct status/banner
	// rather than a review-queue attention reason.
	snap := inst.Snapshot()
	reason := skipReasonOf(snap, inst.Started())
	rqp.explain.noteSkip(snap.Title, reason)
	return reason != SkipNone
}

func skipReasonOf(snap *InstanceSnapshot, started bool) SkipReason {
	switch {
	case snap.Hidden:
		return SkipHidden
	case snap.Status.IsSuspended():
		return SkipSuspended
	case snap.ArchivedAt != nil:
		return SkipArchived
	case !started:
		return SkipNotStarted
	}
	return SkipNone
}

// SetEventRecorder wires the sink for AttentionRaised/Cleared events; nil restores the no-op.
func (rqp *ReviewQueuePoller) SetEventRecorder(r SessionEventRecorder) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	rqp.recorder = r
}

// EventRecorder returns the poller's recorder (never nil) for the other single-owner event callers.
func (rqp *ReviewQueuePoller) EventRecorder() SessionEventRecorder { return rqp.eventRecorder() }

func (rqp *ReviewQueuePoller) eventRecorder() SessionEventRecorder {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()
	return orNoopRecorder(rqp.recorder)
}

// removeFromQueue is the poller's single removal path; it records AttentionCleared for
// the reason the removed item carried.
func (rqp *ReviewQueuePoller) removeFromQueue(title string) {
	existing, had := rqp.queue.Get(title)
	if rqp.queue.Remove(title) && had {
		rqp.eventRecorder().RecordAttentionCleared(title, existing.Reason)
	}
}

// addToQueue is the poller's single add path; it records AttentionRaised when the item is
// new or its reason differs from the queued one.
func (rqp *ReviewQueuePoller) addToQueue(item, prior *ReviewItem) {
	rqp.queue.Add(item)
	if prior == nil || prior.Reason != item.Reason {
		rqp.eventRecorder().RecordAttentionRaised(item.SessionID, item.Reason)
	}
}

// SetOptionsSource wires the attention-flag source read once per poll cycle.
func (rqp *ReviewQueuePoller) SetOptionsSource(src FlagOptionsSource) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	rqp.optionsSource = src
}

func (rqp *ReviewQueuePoller) currentOptions() (DeterminerOptions, RuleOptions) {
	rqp.mu.RLock()
	src := rqp.optionsSource
	rqp.mu.RUnlock()
	return src.resolve()
}

// evaluateSession checks a single session and adds/removes from queue as needed, using the
// cycle's already-resolved flag options. paneActivity is the snapshot from
// batchPaneActivity(); nil falls back to the TTL cache.
func (rqp *ReviewQueuePoller) evaluateSession(inst *Instance, paneActivity map[string]time.Time, opts DeterminerOptions, ruleOpts RuleOptions) {
	// Skip paused, stopped, or unstarted sessions
	if rqp.shouldSkipSession(inst) {
		return
	}

	// Lock-free snapshot for all read-only field accesses below.
	// Write paths (inst.LastAddedToQueue = ...) and method calls (IsAcknowledgedAfterOutput,
	// GetDiffStats, etc.) are left unchanged.
	snap := inst.Snapshot()

	// Get comprehensive status
	statusInfo := rqp.statusManager.GetStatus(inst)

	// STEP 1: Get terminal content for prompt detection.
	// Uses cached content when the controller reports no new activity since the last
	// poll — avoids a subprocess spawn on every tick for idle controller-managed sessions.
	content := rqp.contentProvider.GetContent(inst, statusInfo, paneActivity)

	// Status determination: pure evaluation, no side effects.
	// Handles controller-based and terminal-content detection, idle/staleness checks.
	result := rqp.statusDeterminer.Determine(inst, content, statusInfo, rqp.statusDetector, opts)

	if result.Action == DetectionActionRemove {
		rqp.handleDetectedRemove(snap.Title, statusInfo)
		return
	}

	// If the determiner saw a clean worktree, remove any stale UncommittedChanges entry.
	if result.CleanWorktree {
		if existing, exists := rqp.queue.Get(snap.Title); exists && existing.Reason == ReasonUncommittedChanges {
			log.Debug("changes committed, removing UncommittedChanges entry", "session", snap.Title)
			rqp.removeFromQueue(snap.Title)
		}
	}

	// LastMeaningfulOutput is updated by GetContent() above via UpdateTerminalTimestamps();
	// the persisted content-signature dedup keeps acknowledged sessions snoozed unless
	// output genuinely changes. The rules in suppressionPolicy decide what happens next.
	decision := suppressionPolicy(snap, result, statusInfo, rqp.queue, ruleOpts)
	rqp.explain.noteDecision(snap.Title, decision)
	switch decision.kind {
	case suppressionRemove:
		rqp.removeFromQueue(snap.Title)
		return
	case suppressionSuppress:
		return
	}

	// Add or update in queue, or leave unchanged.
	// DetectionActionSkip means the determiner saw no actionable condition — the queue
	// entry (if any) should remain; only DetectionActionAdd and DetectionActionRemove
	// (handled above) mutate the queue.
	if result.Action == DetectionActionSkip {
		return
	}

	log.Debug("final decision", "session", snap.Title, "reason", result.Reason.String(), "priority", result.Priority.String(), "context", result.Context)

	rqp.addDetectedItem(inst, snap, result, statusInfo)
}

// handleDetectedRemove applies a determiner Remove action.
func (rqp *ReviewQueuePoller) handleDetectedRemove(title string, statusInfo InstanceStatusInfo) {
	// Without a controller the raw terminal scanner can false-positive Active during a resize
	// redraw, so keep prompt-reason items there; controller-active sessions are trusted.
	if !statusInfo.IsControllerActive {
		if existing, exists := rqp.queue.Get(title); exists && isPromptReason(existing.Reason) {
			return
		}
	}
	rqp.removeFromQueue(title)
}

// addDetectedItem builds, enriches and enqueues the review item for an Add action.
func (rqp *ReviewQueuePoller) addDetectedItem(inst *Instance, snap *InstanceSnapshot, result DetectionResult, statusInfo InstanceStatusInfo) {
	reason, priority, context := result.Reason, result.Priority, result.Context

	detectedAt := time.Now()
	isUpdate := false
	var prior *ReviewItem
	if existingItem, exists := rqp.queue.Get(snap.Title); exists {
		isUpdate = true
		prior = existingItem
		// Preserve original timestamp if meaningful fields haven't changed
		if existingItem.Reason == reason &&
			existingItem.Priority == priority &&
			existingItem.Context == context {
			detectedAt = existingItem.DetectedAt
		}
	}

	// DO NOT update LastMeaningfulOutput here - it must reflect actual terminal output time
	// Updating it would defeat staleness detection by making the session appear fresh

	// Use CreatedAt as fallback LastActivity when LastMeaningfulOutput hasn't been set yet
	// (new sessions, sessions where StartController failed before the migration ran).
	lastActivity := snap.LastMeaningfulOutput
	if lastActivity.IsZero() {
		lastActivity = snap.CreatedAt
	}

	item := &ReviewItem{
		SessionID:   snap.Title,
		SessionName: snap.Title,
		Reason:      reason,
		Priority:    priority,
		DetectedAt:  detectedAt,
		Context:     context,
		// Populate session details for rich display
		Program:         snap.Program,
		Branch:          snap.Branch,
		Path:            snap.Path,
		WorkingDir:      snap.WorkingDir,
		Status:          snap.Status.String(),
		Tags:            snap.Tags,
		Category:        snap.Category,
		DiffStats:       inst.GetDiffStats(),
		LastActivity:    lastActivity,
		HasCommitsAhead: inst.GetHasCommitsAhead(),
		// Populate idle state and raw detected status for WorkingState mapping.
		IdleState:    statusInfo.IdleState.State,
		ClaudeStatus: result.ClaudeStatus,
	}

	rqp.enrichApprovalMetadata(item, snap)
	setEpisodeMetadata(item, result.Episode)

	log.Debug("adding to queue", "session", snap.Title, "reason", reason.String(), "priority", priority.String(), "context", context)
	rqp.addToQueue(item, prior)

	rqp.persistLastAddedToQueue(inst, snap.Title)

	if !isUpdate {
		log.Debug("successfully added to queue", "session", snap.Title, "reason", reason.String(), "priority", priority.String(), "context", context)
	}
}

// enrichApprovalMetadata adds hook metadata from ApprovalStore (Story 3, Task 3.2).
//
// ApprovalHandler.resolveSessionID stores PendingApproval.SessionID as the session's stable
// ID (UUID when present, Title only as a fallback), but ReviewItem is keyed by Title, so
// look up by UUID first and fall back to Title for sessions without a UUID.
func (rqp *ReviewQueuePoller) enrichApprovalMetadata(item *ReviewItem, snap *InstanceSnapshot) {
	rqp.mu.RLock()
	provider := rqp.approvalProvider
	rqp.mu.RUnlock()
	if item.Reason != ReasonApprovalPending || provider == nil {
		return
	}
	approvals := provider.GetApprovalMetadataBySession(snap.UUID)
	if len(approvals) == 0 && snap.UUID != snap.Title {
		approvals = provider.GetApprovalMetadataBySession(snap.Title)
	}
	if len(approvals) == 0 {
		return
	}
	a := highestRiskApproval(approvals) // GAP-004: surface the most dangerous concurrent approval, not just the first
	if item.Metadata == nil {
		item.Metadata = make(map[string]string)
	}
	item.Metadata["pending_approval_id"] = a.ApprovalID
	item.Metadata["tool_name"] = a.ToolName
	setIfNonEmpty(item.Metadata, "tool_input_command", toolInputString(a.ToolInput, "command"))
	setIfNonEmpty(item.Metadata, "tool_input_file", toolInputString(a.ToolInput, "file_path"))
	setIfNonEmpty(item.Metadata, "cwd", a.Cwd)
	if a.Orphaned {
		item.Metadata["orphaned"] = "true"
	}
	setIfNonEmpty(item.Metadata, "escalation_reason", a.EscalationReason)
	setIfNonEmpty(item.Metadata, "escalation_reason_category", a.EscalationCategory)
	setIfNonEmpty(item.Metadata, "risk_level", a.RiskLevel)
	log.Debug("enriched approval item with hook metadata", "session", snap.Title, "tool", a.ToolName, "approval_id", a.ApprovalID, "escalation_category", item.Metadata["escalation_reason_category"], "risk_level", item.Metadata["risk_level"])
}

func toolInputString(input map[string]interface{}, key string) string {
	v, _ := input[key].(string)
	return v
}

func setIfNonEmpty(m map[string]string, key, val string) {
	if val != "" {
		m[key] = val
	}
}

// persistLastAddedToQueue stamps the spam-prevention timestamp via the actor command (so the
// write is serialised with buildSnapshot) and persists it so restarts don't re-add sessions.
// Uses UpdateInstanceLastAddedToQueue, not SaveInstances, to avoid the merge logic that would
// restore deleted instances from disk.
func (rqp *ReviewQueuePoller) persistLastAddedToQueue(inst *Instance, title string) {
	now := time.Now()
	inst.SetLastAddedToQueue(now)
	log.Debug("updated LastAddedToQueue timestamp", "session", title, "timestamp", now)
	if rqp.storage == nil {
		return
	}
	if err := rqp.storage.UpdateInstanceLastAddedToQueue(title, now); err != nil {
		log.Error("failed to persist LastAddedToQueue", "session", title, "err", err)
	}
}

// UpdateConfig updates the poller configuration.
func (rqp *ReviewQueuePoller) UpdateConfig(config ReviewQueuePollerConfig) {
	rqp.mu.Lock()
	defer rqp.mu.Unlock()
	rqp.config = config
	log.Info("ReviewQueuePoller config updated", "poll_interval", config.PollInterval, "idle_threshold", config.IdleThreshold)
}

// GetConfig returns the current configuration.
func (rqp *ReviewQueuePoller) GetConfig() ReviewQueuePollerConfig {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()
	return rqp.config
}

// IsRunning returns true if the poller is currently running.
func (rqp *ReviewQueuePoller) IsRunning() bool {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()
	return rqp.ctx != nil && rqp.ctx.Err() == nil
}

// GetMonitoredCount returns the number of instances being monitored.
func (rqp *ReviewQueuePoller) GetMonitoredCount() int {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()
	return len(rqp.instances)
}

// CheckSession checks a single session immediately (exported for ReactiveQueueManager).
// This allows external components to trigger immediate re-evaluation without waiting for
// the next poll cycle, providing <100ms feedback on user interactions.
// Fetches a fresh pane activity snapshot for accurate cache invalidation.
func (rqp *ReviewQueuePoller) CheckSession(inst *Instance) {
	opts, ruleOpts := rqp.currentOptions()
	rqp.evaluateSession(inst, batchPaneActivity(""), opts, ruleOpts)
}

// FindInstance finds an instance by session ID (exported for ReactiveQueueManager).
// Returns nil if the instance is not found in the monitored list.
func (rqp *ReviewQueuePoller) FindInstance(sessionID string) *Instance {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()

	for _, inst := range rqp.instances {
		if inst.MatchesID(sessionID) {
			return inst
		}
	}
	return nil
}

// injectCachedContent is a test helper that seeds the content cache directly,
// bypassing tmux. Only the pollerContentProvider implementation supports this;
// custom ContentProvider implementations ignore the call.
func (rqp *ReviewQueuePoller) injectCachedContent(title, content string) {
	if p, ok := rqp.contentProvider.(*pollerContentProvider); ok {
		now := time.Now()
		p.cache.Compute(title, func(e contentCacheEntry, _ bool) (contentCacheEntry, xsync.ComputeOp) {
			e.cachedContent = content
			e.lastPreviewTime = now
			return e, xsync.UpdateOp
		})
	}
}

// GetInstances returns a snapshot of all live in-memory instances held by the poller.
// Use this instead of LoadInstances() for read-only operations to avoid the side effect
// of FromInstanceData() calling Start() on every non-paused instance.
func (rqp *ReviewQueuePoller) GetInstances() []*Instance {
	rqp.mu.RLock()
	defer rqp.mu.RUnlock()
	result := make([]*Instance, len(rqp.instances))
	copy(result, rqp.instances)
	return result
}
