package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/scrollback"
)

// Notifier publishes an operator-facing notification. Implemented outside this
// package (typically a thin adapter over the event bus) since this package cannot
// import pkg/events directly — pkg/events imports session, so the reverse import
// would be a cycle. notificationType and priority are int32 values matching
// sessionv1.NotificationType / sessionv1.NotificationPriority; this package stays
// free of the proto dependency and just passes the raw values through.
type Notifier interface {
	Notify(itemID, title, message string, notificationType, priority int32)
}

// ReviewGateSpawner can create a short-lived review session for a backlog item.
// Deprecated: use headless.Pool via NewBacklogLifecycleListenerWithSpawner instead.
// Retained for backward compatibility with existing tests and callers.
type ReviewGateSpawner interface {
	// SpawnReviewSession creates a one-shot review session for item using prompt.
	// itemSessionID is the UUID of the work ItemSession being reviewed.
	SpawnReviewSession(ctx context.Context, item *BacklogItemData, itemSessionID string, prompt string) (*Instance, error)
}

// AutoReopenSpawner can automatically reopen a backlog item for rework after a
// failed review verdict (FAIL or PARTIAL). It transitions the item back to
// in_progress and spawns a new work session so the review→rework cycle is
// fully automated.
type AutoReopenSpawner interface {
	AutoReopenAfterFailedReview(ctx context.Context, itemID string) error
}

// PRFixSpawner can reopen a pr_pending item for rework when CI checks fail or
// reviewers request changes. The fixContext string contains a summary of the
// failures/comments to pass as context to the new work session.
type PRFixSpawner interface {
	AutoReopenForPRFix(ctx context.Context, itemID string, fixContext string) error
}

// prPendingChecker is the subset of GitWorktree's PR-status behavior that
// ReconcilePRPending depends on. Defined here (the consumer) rather than in
// package git, scoped to exactly what's called.
type prPendingChecker interface {
	IsPRMerged(prNumber int) (bool, error)
	GetPRStatus(prNumber int) (*git.PRStatus, error)
}

// prCreator is the subset of GitWorktree's push/PR-creation behavior that
// pushAndCreatePR depends on. Defined here (the consumer), scoped to exactly
// what's called, mirroring prPendingChecker below.
type prCreator interface {
	CommitChanges(commitMessage string) error
	PushBranch() error
	CreatePR(title, body string) (prURL string, prNumber int, err error)
	EnablePRAutoMerge(prNumber int) error
}

// defaultPRCreatorFactory constructs the push/PR-creation client for a given
// worktree. This is the production default installed by newListenerBase;
// SetPRCreatorFactory overrides it in tests.
func defaultPRCreatorFactory(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
	return git.NewGitWorktreeFromStorage(repoPath, worktreePath, sessionName, branchName, baseCommitSHA)
}

// defaultPRPendingCheckerFactory constructs the PR-status checker for a given
// repo path. This is the production default installed by newListenerBase;
// SetPRPendingCheckerFactory overrides it in tests.
func defaultPRPendingCheckerFactory(repoPath string) prPendingChecker {
	return git.NewGitWorktreeFromStorage(repoPath, repoPath, "", "", "")
}

// maxConcurrentReviewGates is the maximum number of review gates that can run
// concurrently. This caps goroutine fan-out when many sessions exit simultaneously.
const maxConcurrentReviewGates = 8

// BacklogLifecycleListener drives backlog item state transitions in response to
// session lifecycle events. It must be registered via Instance.RegisterLifecycleListener.
//
// OnLifecycleEvent is non-blocking; all DB work is dispatched to a goroutine.
// Call SetEnabled(false) to make all callbacks no-ops without unwiring.
type BacklogLifecycleListener struct {
	storage        *Storage
	sessionCreator ReviewGateSpawner

	// poolMu guards headlessPool for concurrent Set/get access.
	poolMu       sync.RWMutex
	headlessPool *headless.Pool

	// autoReopenMu guards autoReopener for concurrent Set/get access.
	autoReopenMu sync.RWMutex
	autoReopener AutoReopenSpawner

	// prFixMu guards prFixSpawner for concurrent Set/get access.
	prFixMu      sync.RWMutex
	prFixSpawner PRFixSpawner

	// prPendingCheckerMu guards prPendingCheckerFactory for concurrent Set/get access.
	prPendingCheckerMu      sync.RWMutex
	prPendingCheckerFactory func(repoPath string) prPendingChecker

	// prCreatorMu guards prCreatorFactory for concurrent Set/get access.
	prCreatorMu      sync.RWMutex
	prCreatorFactory func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator

	// notifierMu guards notifier for concurrent Set/get access.
	notifierMu sync.RWMutex
	notifier   Notifier

	// staleWorkNotifiedMu guards staleWorkNotified for concurrent access.
	// Tracks which in_progress item IDs have already been flagged as stale by
	// ReconcileStuck so repeat ticks don't re-notify every 60s. In-memory only —
	// resets on server restart, which is an acceptable tradeoff for avoiding a
	// schema migration for a best-effort notification.
	staleWorkNotifiedMu sync.Mutex
	staleWorkNotified   map[string]bool

	// stuckReviewNotifiedMu guards stuckReviewNotified for concurrent access.
	// Tracks which review-status item IDs have already been flagged by
	// ReconcileStuck as "abandoned" (has a review verdict, nothing active in
	// flight) so repeat ticks don't re-notify every 60s. In-memory only, same
	// tradeoff as staleWorkNotified above.
	stuckReviewNotifiedMu sync.Mutex
	stuckReviewNotified   map[string]bool

	// reviewSem limits concurrent review gate goroutines.
	reviewSem chan struct{}

	// shutdownCtx is cancelled by Shutdown(); used by long-running review gate calls.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	// runner encapsulates the spawnReviewGate logic so it can be tested independently.
	runner *ReviewGateRunner

	enabled atomic.Bool
}

// SetEnabled toggles whether this listener processes lifecycle events.
// Safe to call concurrently.
func (l *BacklogLifecycleListener) SetEnabled(v bool) { l.enabled.Store(v) }

// SetHeadlessPool wires in the headless LLM pool after construction.
// Calling this enables the headless review gate path even when the listener was
// created via NewBacklogLifecycleListenerWithSpawner.
func (l *BacklogLifecycleListener) SetHeadlessPool(p *headless.Pool) {
	l.poolMu.Lock()
	defer l.poolMu.Unlock()
	l.headlessPool = p
}

// SetAutoReopener wires in the spawner used to automatically reopen items for
// rework when a review verdict is FAIL or PARTIAL.
func (l *BacklogLifecycleListener) SetAutoReopener(r AutoReopenSpawner) {
	l.autoReopenMu.Lock()
	defer l.autoReopenMu.Unlock()
	l.autoReopener = r
}

// SetScrollbackManager wires in the scrollback manager used to write a searchable
// session transcript file on the empty-diff codebase-read review path. Delegates to
// the underlying ReviewGateRunner, which owns the field. Optional — nil (the default)
// simply omits the "## Session Transcript" prompt section.
func (l *BacklogLifecycleListener) SetScrollbackManager(sm *scrollback.ScrollbackManager) {
	l.runner.SetScrollbackManager(sm)
}

// getAutoReopener returns the current auto-reopener under a read lock.
func (l *BacklogLifecycleListener) getAutoReopener() AutoReopenSpawner {
	l.autoReopenMu.RLock()
	defer l.autoReopenMu.RUnlock()
	return l.autoReopener
}

// SetPRFixSpawner wires in the spawner used to automatically reopen pr_pending
// items for rework when CI checks fail or reviewers request changes.
func (l *BacklogLifecycleListener) SetPRFixSpawner(s PRFixSpawner) {
	l.prFixMu.Lock()
	defer l.prFixMu.Unlock()
	l.prFixSpawner = s
}

// getPRFixSpawner returns the current PR fix spawner under a read lock.
func (l *BacklogLifecycleListener) getPRFixSpawner() PRFixSpawner {
	l.prFixMu.RLock()
	defer l.prFixMu.RUnlock()
	return l.prFixSpawner
}

// SetPRPendingCheckerFactory overrides the factory used to construct the
// PR-status checker for ReconcilePRPending. Overridable in tests (mirrors the
// timeNow seam in instance_workspace.go:581); production code never needs to
// call this, since newListenerBase installs defaultPRPendingCheckerFactory.
func (l *BacklogLifecycleListener) SetPRPendingCheckerFactory(f func(repoPath string) prPendingChecker) {
	l.prPendingCheckerMu.Lock()
	defer l.prPendingCheckerMu.Unlock()
	l.prPendingCheckerFactory = f
}

// getPRPendingCheckerFactory returns the current PR-pending-checker factory under a read lock.
func (l *BacklogLifecycleListener) getPRPendingCheckerFactory() func(repoPath string) prPendingChecker {
	l.prPendingCheckerMu.RLock()
	defer l.prPendingCheckerMu.RUnlock()
	return l.prPendingCheckerFactory
}

// SetPRCreatorFactory overrides the factory used to construct the push/PR-creation
// client for pushAndCreatePR. Overridable in tests; production code never needs to
// call this, since newListenerBase installs defaultPRCreatorFactory.
func (l *BacklogLifecycleListener) SetPRCreatorFactory(f func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator) {
	l.prCreatorMu.Lock()
	defer l.prCreatorMu.Unlock()
	l.prCreatorFactory = f
}

// getPRCreatorFactory returns the current PR-creator factory under a read lock.
func (l *BacklogLifecycleListener) getPRCreatorFactory() func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator {
	l.prCreatorMu.RLock()
	defer l.prCreatorMu.RUnlock()
	return l.prCreatorFactory
}

// SetNotifier wires in the notifier used to publish operator-facing notifications
// (PR creation failures, security blocks, stale work sessions, rework-cap hits).
// Optional — nil means notifications are disabled.
func (l *BacklogLifecycleListener) SetNotifier(n Notifier) {
	l.notifierMu.Lock()
	defer l.notifierMu.Unlock()
	l.notifier = n
}

// getNotifier returns the current notifier under a read lock.
func (l *BacklogLifecycleListener) getNotifier() Notifier {
	l.notifierMu.RLock()
	defer l.notifierMu.RUnlock()
	return l.notifier
}

// notify publishes a best-effort operator notification. No-op if no notifier is wired.
func (l *BacklogLifecycleListener) notify(itemID, title, message string, notificationType, priority int32) {
	if n := l.getNotifier(); n != nil {
		n.Notify(itemID, title, message, notificationType, priority)
	}
}

// getHeadlessPool returns the current headless pool under a read lock.
func (l *BacklogLifecycleListener) getHeadlessPool() *headless.Pool {
	l.poolMu.RLock()
	defer l.poolMu.RUnlock()
	return l.headlessPool
}

// Shutdown cancels in-flight review gate calls. Safe to call concurrently.
func (l *BacklogLifecycleListener) Shutdown() {
	if l.shutdownCancel != nil {
		l.shutdownCancel()
	}
}

// newListenerBase initialises fields common to all BacklogLifecycleListener constructors.
func newListenerBase(storage *Storage) *BacklogLifecycleListener {
	ctx, cancel := context.WithCancel(context.Background())
	l := &BacklogLifecycleListener{
		storage:                 storage,
		reviewSem:               make(chan struct{}, maxConcurrentReviewGates),
		shutdownCtx:             ctx,
		shutdownCancel:          cancel,
		prPendingCheckerFactory: defaultPRPendingCheckerFactory,
		prCreatorFactory:        defaultPRCreatorFactory,
		staleWorkNotified:       make(map[string]bool),
		stuckReviewNotified:     make(map[string]bool),
	}
	l.runner = NewReviewGateRunner(storage, l.getHeadlessPool, l.getAutoReopener, l.getNotifier, nil)
	return l
}

// NewBacklogLifecycleListener creates a listener backed by the given storage.
// The review gate is disabled (sessionCreator=nil, headlessPool=nil).
func NewBacklogLifecycleListener(storage *Storage) *BacklogLifecycleListener {
	return newListenerBase(storage)
}

// NewBacklogLifecycleListenerWithSpawner creates a listener that will spawn a
// review gate session when a work session exits and SkipReviewGate is false.
func NewBacklogLifecycleListenerWithSpawner(storage *Storage, spawner ReviewGateSpawner) *BacklogLifecycleListener {
	l := newListenerBase(storage)
	l.sessionCreator = spawner
	l.runner.sessionCreator = spawner
	return l
}

// NewBacklogLifecycleListenerWithPool creates a listener that uses a headless.Pool
// for review gate calls instead of spawning a tmux session.
func NewBacklogLifecycleListenerWithPool(storage *Storage, pool *headless.Pool) *BacklogLifecycleListener {
	l := newListenerBase(storage)
	l.headlessPool = pool
	return l
}

// instanceBacklogListener is a per-instance shim that binds the instance UUID into
// every lifecycle callback. Created and registered via WireToInstance.
type instanceBacklogListener struct {
	parent       *BacklogLifecycleListener
	instanceUUID string
}

func (il *instanceBacklogListener) OnLifecycleEvent(event LifecycleEvent, _ string) {
	if !il.parent.enabled.Load() {
		return
	}
	switch event {
	case EventStarted:
		go il.parent.onSessionStarted(il.instanceUUID)
	case EventExited:
		go il.parent.onSessionExited(il.instanceUUID)
	}
}

// WireToInstance creates a per-instance listener shim and registers it on inst.
// Call this for every Instance that should participate in backlog lifecycle tracking.
func (l *BacklogLifecycleListener) WireToInstance(inst *Instance) {
	inst.RegisterLifecycleListener(&instanceBacklogListener{
		parent:       l,
		instanceUUID: inst.UUID,
	})
}

// onSessionStarted records the start time for the ItemSession linked to sessionUUID.
func (l *BacklogLifecycleListener) onSessionStarted(sessionUUID string) {
	ctx := context.Background()
	is, err := l.storage.GetItemSessionBySessionUUID(ctx, sessionUUID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return
		}
		log.ErrorLog.Printf("[BacklogLifecycle] GetItemSessionBySessionUUID(%s) error: %v", sessionUUID, err)
		return
	}
	if err := l.storage.UpdateItemSessionStarted(ctx, is.ID, time.Now()); err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] UpdateItemSessionStarted(%s) error: %v", is.ID, err)
	}
}

// onSessionExited drives the in_progress→review (or in_progress→done for skip_review_gate) transition.
func (l *BacklogLifecycleListener) onSessionExited(sessionUUID string) {
	ctx := context.Background()

	is, err := l.storage.GetItemSessionBySessionUUID(ctx, sessionUUID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return
		}
		log.ErrorLog.Printf("[BacklogLifecycle] GetItemSessionBySessionUUID(%s) error: %v", sessionUUID, err)
		return
	}

	// Record end time for all session roles (triage, review, work).
	now := time.Now()
	if err := l.storage.UpdateItemSessionEnded(ctx, is.ID, now); err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] UpdateItemSessionEnded(%s) error: %v", is.ID, err)
	}

	// Only drive in_progress→review/done transitions for work sessions.
	if is.Role != SessionRoleWork {
		return
	}

	// Look up the BacklogItem via storage (no longer an eager-loaded edge).
	item, err := l.storage.GetBacklogItem(ctx, is.BacklogItemID)
	if err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] GetBacklogItem for session %s (item %s): %v", sessionUUID, is.BacklogItemID, err)
		return
	}

	if BacklogStatus(item.Status) != BacklogStatusInProgress {
		log.DebugLog.Printf("[BacklogLifecycle] item %s is %s (not in_progress); skipping", item.ID, item.Status)
		return
	}

	toStatus := BacklogStatusReview
	if item.SkipReviewGate {
		toStatus = BacklogStatusDone
	}

	updatedAt := item.UpdatedAt
	precondition := &BacklogItemPrecondition{
		ExpectedStatus:    string(BacklogStatusInProgress),
		ExpectedUpdatedAt: &updatedAt,
	}
	if _, err := l.storage.TransitionBacklogItemStatus(ctx, item.ID, toStatus, precondition); err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] TransitionBacklogItemStatus item=%s to=%s: %v", item.ID, toStatus, err)
		return
	}

	log.InfoLog.Printf("[BacklogLifecycle] item %s transitioned to %s (session %s exited)", item.ID, toStatus, sessionUUID)

	// Spawn review gate if the item moved to review and a review mechanism is configured.
	if toStatus == BacklogStatusReview && !item.SkipReviewGate && (l.getHeadlessPool() != nil || l.sessionCreator != nil) {
		go func() {
			// Acquire the bounded semaphore to prevent unbounded goroutine fan-out
			// when many sessions exit simultaneously.
			select {
			case l.reviewSem <- struct{}{}:
			case <-l.shutdownCtx.Done():
				return
			}
			defer func() { <-l.reviewSem }()
			l.spawnReviewGate(item, is)
		}()
	}
}

// TriggerReviewForSession immediately spawns a review gate for the work session
// identified by workSessionUUID. Used by the autonomous driver to trigger review
// as soon as the driver signals DONE, rather than waiting for ReconcileStuck.
// No-op if the listener is disabled or no review mechanism is configured.
func (l *BacklogLifecycleListener) TriggerReviewForSession(workSessionUUID string) {
	if !l.enabled.Load() {
		return
	}
	if l.getHeadlessPool() == nil && l.sessionCreator == nil {
		return
	}
	go func() {
		select {
		case l.reviewSem <- struct{}{}:
		case <-l.shutdownCtx.Done():
			return
		}
		defer func() { <-l.reviewSem }()

		ctx := l.shutdownCtx
		is, err := l.storage.GetItemSessionBySessionUUID(ctx, workSessionUUID)
		if err != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] TriggerReviewForSession GetItemSessionBySessionUUID(%s): %v", workSessionUUID, err)
			return
		}
		item, err := l.storage.GetBacklogItem(ctx, is.BacklogItemID)
		if err != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] TriggerReviewForSession GetBacklogItem session=%s item=%s: %v", workSessionUUID, is.BacklogItemID, err)
			return
		}
		if item.SkipReviewGate {
			return
		}
		log.InfoLog.Printf("[BacklogLifecycle] TriggerReviewForSession: spawning immediate review gate item=%s session=%s", item.ID, workSessionUUID)
		l.spawnReviewGate(item, is)
	}()
}

// applyVerdictsToACs updates the acceptance criteria status fields on a backlog
// item to reflect the review verdict for each criterion:
//
//	PASS  → "done"
//	PARTIAL → "in_progress"
//	FAIL / UNVERIFIABLE → unchanged (stay "pending")
//
// Best-effort: errors are logged but do not block the caller.
func applyVerdictsToACs(ctx context.Context, storage *Storage, item *BacklogItemData, acSnapshot []AcCriterion, verdicts []CriterionVerdict) {
	if len(verdicts) == 0 || len(acSnapshot) == 0 {
		return
	}

	outcomeByIdx := make(map[int]ReviewOutcome, len(verdicts))
	for _, v := range verdicts {
		outcomeByIdx[v.CriterionIndex] = v.Outcome
	}

	updated := make([]AcCriterion, len(acSnapshot))
	copy(updated, acSnapshot)
	changed := false
	for i, ac := range updated {
		outcome, ok := outcomeByIdx[ac.Index]
		if !ok {
			continue
		}
		var newStatus AcStatus
		switch outcome {
		case ReviewOutcomePass:
			newStatus = AcStatusDone
		case ReviewOutcomePartial:
			newStatus = AcStatusInProgress
		default:
			continue // FAIL / UNVERIFIABLE: leave as-is
		}
		if newStatus != ac.Status {
			updated[i].Status = newStatus
			changed = true
		}
	}

	if !changed {
		return
	}

	newJSON, err := SerializeAcCriteria(updated)
	if err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] applyVerdictsToACs serialize item=%s: %v", item.ID, err)
		return
	}
	acj := newJSON
	if _, err := storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{AcceptanceCriteria: &acj}, nil); err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] applyVerdictsToACs update item=%s: %v", item.ID, err)
		return
	}
	log.InfoLog.Printf("[BacklogLifecycle] applyVerdictsToACs: updated AC statuses for item=%s (%d criteria)", item.ID, len(updated))
}

// spawnReviewGate creates a one-shot review session for item, using the diff
// from the work session's worktree.
func (l *BacklogLifecycleListener) spawnReviewGate(item *BacklogItemData, is ItemSessionSummary) {
	l.runner.Run(l.shutdownCtx, item, is, l.pushAndCreatePR)
}

// ReconcileStuck calls ReconcileStuckItems and logs the result.
// Intended to be called on a periodic ticker as a safety net for abnormal session exits.
// No-op when the listener is disabled.
func (l *BacklogLifecycleListener) ReconcileStuck(ctx context.Context) {
	if !l.enabled.Load() {
		return
	}
	er, ok := l.storage.repo.(*EntRepository)
	if !ok {
		return
	}
	n, err := er.ReconcileStuckItems(ctx)
	if err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] ReconcileStuckItems error: %v", err)
		return
	}
	if n > 0 {
		log.InfoLog.Printf("[BacklogLifecycle] ReconcileStuckItems: transitioned %d stuck items to review", n)
	} else {
		log.DebugLog.Printf("[BacklogLifecycle] ReconcileStuckItems: no stuck items found")
	}

	// Re-spawn review gates for items stuck in "review" with no review session.
	// Occurs when the headless pool was unavailable at the time of the work session exit.
	// Scoped to this block only (not an early return) — PR-pending polling and staleness
	// detection below must still run even when no review mechanism is configured.
	if l.getHeadlessPool() != nil || l.sessionCreator != nil {
		items, gateErr := er.FindReviewItemsWithoutGate(ctx)
		if gateErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] FindReviewItemsWithoutGate error: %v", gateErr)
		} else {
			for _, item := range items {
				var workSession *ItemSessionSummary
				if len(item.Edges.ItemSessions) > 0 {
					s := itemSessionToSummary(item.Edges.ItemSessions[0])
					workSession = &s
				}
				if workSession == nil {
					log.DebugLog.Printf("[BacklogLifecycle] ReconcileStuckReviewGates: item %s has no work session, skipping", item.ID)
					continue
				}
				log.InfoLog.Printf("[BacklogLifecycle] ReconcileStuckReviewGates: re-spawning review gate for item %s", item.ID)
				itemData := backlogItemToData(item)
				isCopy := *workSession
				go func(itemCopy *BacklogItemData, isCopy ItemSessionSummary) {
					select {
					case l.reviewSem <- struct{}{}:
					case <-l.shutdownCtx.Done():
						return
					}
					defer func() { <-l.reviewSem }()
					l.spawnReviewGate(itemCopy, isCopy)
				}(&itemData, isCopy)
			}
		}
	}

	// Flag in_progress work sessions that have gone quiet for too long. Detection +
	// notification only — a slow-but-alive agent should not be force-stopped.
	l.reconcileStaleWorkSessions(ctx)

	// Self-heal pr_pending items whose pr_number is missing (0) despite having a
	// pr_url — otherwise permanently invisible to FindPRPendingItems' PrNumberGT(0)
	// filter below, so they'd never get polled. See BackfillMissingPRNumbers doc.
	if n, backfillErr := er.BackfillMissingPRNumbers(ctx); backfillErr != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] BackfillMissingPRNumbers error: %v", backfillErr)
	} else if n > 0 {
		log.InfoLog.Printf("[BacklogLifecycle] BackfillMissingPRNumbers: backfilled pr_number for %d item(s)", n)
	}

	// Flag review-status items that already have a review verdict but nothing
	// active in flight (AutoReopenAfterFailedReview's spawn failed and rolled
	// back, or a legacy review session exited without a verdict). Detection +
	// notification only, same rationale as reconcileStaleWorkSessions above:
	// these items are otherwise invisible to every other reconciler.
	l.reconcileStuckReviewItems(ctx, er)

	// Poll pr_pending items: auto-transition to done when the PR is merged.
	l.ReconcilePRPending(ctx, er)
}

// reconcileStuckReviewItems notifies once per item when a review-status item
// has a review verdict on record but no active review or work session — i.e.
// it is not mid-cycle, it is simply abandoned. See FindStuckReviewItems for
// the two known causes. Best-effort: query/notify failures are logged, never
// returned.
func (l *BacklogLifecycleListener) reconcileStuckReviewItems(ctx context.Context, er *EntRepository) {
	items, err := er.FindStuckReviewItems(ctx)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileStuckReviewItems query error: %v", err)
		return
	}
	for _, item := range items {
		l.stuckReviewNotifiedMu.Lock()
		alreadyNotified := l.stuckReviewNotified[item.ID.String()]
		if !alreadyNotified {
			l.stuckReviewNotified[item.ID.String()] = true
		}
		l.stuckReviewNotifiedMu.Unlock()
		if alreadyNotified {
			continue
		}

		outcome, verdictErr := er.GetMostRecentReviewVerdictForItem(ctx, item.ID.String())
		if verdictErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileStuckReviewItems GetMostRecentReviewVerdictForItem item=%s: %v", item.ID, verdictErr)
		}
		outcomeDesc := "no verdict recorded"
		if outcome != "" {
			outcomeDesc = fmt.Sprintf("last verdict: %s", outcome)
		}

		log.WarningLog.Printf("[BacklogLifecycle] item %s stuck in review with nothing in flight (%s)", item.ID, outcomeDesc)
		l.notify(item.ID.String(),
			"Review item needs attention",
			fmt.Sprintf("%s — stuck in review with no active session (%s). It may need manual re-review or rework.", item.Title, outcomeDesc),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
		)
	}
}

// maxWorkSessionStaleness is the longest an in_progress work session can go without
// reporting progress before ReconcileStuck flags it as stale. Mirrors the order of
// magnitude of maxTriageSessionAge (server/services/backlog_service_triage.go).
const maxWorkSessionStaleness = 2 * time.Hour

// reconcileStaleWorkSessions notifies once per item when an in_progress backlog item's
// active work session has gone longer than maxWorkSessionStaleness without progress.
// Best-effort: query/notify failures are logged, never returned.
func (l *BacklogLifecycleListener) reconcileStaleWorkSessions(ctx context.Context) {
	items, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{string(BacklogStatusInProgress)},
	})
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileStaleWorkSessions list error: %v", err)
		return
	}
	for _, item := range items {
		sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
		if sessErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileStaleWorkSessions ListItemSessions item=%s: %v", item.ID, sessErr)
			continue
		}
		var active *ItemSessionSummary
		for i := range sessions {
			if sessions[i].Role == SessionRoleWork && sessions[i].EndedAt == nil {
				active = &sessions[i]
				break
			}
		}
		if active == nil {
			continue
		}
		lastProgress := active.CreatedAt
		if active.LastProgressAt != nil {
			lastProgress = *active.LastProgressAt
		}
		if time.Since(lastProgress) < maxWorkSessionStaleness {
			continue
		}

		l.staleWorkNotifiedMu.Lock()
		alreadyNotified := l.staleWorkNotified[item.ID]
		if !alreadyNotified {
			l.staleWorkNotified[item.ID] = true
		}
		l.staleWorkNotifiedMu.Unlock()
		if alreadyNotified {
			continue
		}

		log.WarningLog.Printf("[BacklogLifecycle] item %s work session %s stale (no progress since %s)", item.ID, active.SessionUUID, lastProgress)
		l.notify(item.ID,
			"Work session may be stuck",
			fmt.Sprintf("%s — no progress reported in over %s. It may be hung or working silently.", item.Title, maxWorkSessionStaleness),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
		)
	}
}

// pushAndCreatePR commits any dirty state, pushes the branch, creates a GitHub PR,
// stores the PR URL and number on the item, then transitions to pr_pending.
// Falls back to a direct done transition only when there was genuinely nothing to
// ship (no worktree). If code was committed but push/PR-creation fails, the item
// stays in review and a notification is published — see stayInReviewAndNotify.
func (l *BacklogLifecycleListener) pushAndCreatePR(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
	fallbackToDone := func(reason string) {
		log.InfoLog.Printf("[BacklogLifecycle] pushAndCreatePR item=%s falling back to done: %s", item.ID, reason)
		// No status precondition: item may be at review or ready depending on when
		// the PASS verdict was delivered relative to other transitions.
		if _, transErr := l.storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, nil); transErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] pushAndCreatePR fallback done item=%s: %v", item.ID, transErr)
		}
	}

	// stayInReviewAndNotify handles push/PR-creation failures. Unlike fallbackToDone,
	// this must NOT transition the item to done: code was committed to the worktree but
	// never reached GitHub, so marking it done would silently discard that fact. The
	// item stays in review — a human can retry via TriggerReReview, or fix the underlying
	// issue (auth, network, branch protection) and let the next review pass retry.
	stayInReviewAndNotify := func(reason string, err error) {
		log.WarningLog.Printf("[BacklogLifecycle] pushAndCreatePR item=%s: %s: %v — leaving in review, code is committed but not shipped", item.ID, reason, err)
		l.notify(item.ID,
			"PR creation failed",
			fmt.Sprintf("%s — %s: %v. Code is committed locally but not pushed; retry or investigate manually.", item.Title, reason, err),
			7, // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
			3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		)
	}

	wt, wtErr := l.storage.GetWorktreeDataBySessionUUID(ctx, is.SessionUUID)
	if wtErr != nil || wt.WorktreePath == "" {
		fallbackToDone("no worktree")
		return
	}

	g := l.getPRCreatorFactory()(wt.RepoPath, wt.WorktreePath, wt.SessionName, wt.BranchName, wt.BaseCommitSHA)

	// Commit any remaining dirty state.
	commitMsg := fmt.Sprintf("[claudesquad] work complete for %q (pre-PR)", item.Title)
	if commitErr := g.CommitChanges(commitMsg); commitErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] pushAndCreatePR commit item=%s: %v", item.ID, commitErr)
	}

	// Push branch to origin.
	if pushErr := g.PushBranch(); pushErr != nil {
		stayInReviewAndNotify("push failed", pushErr)
		return
	}

	// Create (or locate existing) PR.
	var prURL string
	var prNumber int
	if item.PrNumber > 0 && item.PrURL != "" {
		// PR already exists from a previous attempt — just use it.
		prURL = item.PrURL
		prNumber = item.PrNumber
		log.InfoLog.Printf("[BacklogLifecycle] pushAndCreatePR item=%s reusing existing PR #%d", item.ID, prNumber)
	} else {
		prTitle := item.Title
		prBody := fmt.Sprintf("Automated PR for backlog item: %s\n\nItem ID: %s", item.Title, item.ID)
		if pool := l.getHeadlessPool(); pool != nil {
			diff, _, diffErr := GetGitDiff(ctx, wt.WorktreePath, wt.BaseCommitSHA)
			if diffErr != nil {
				log.WarningLog.Printf("[BacklogLifecycle] pushAndCreatePR GetGitDiff for description item=%s: %v; using boilerplate body", item.ID, diffErr)
			} else if drafted, draftErr := headless.DraftPRDescription(ctx, pool, diff, wt.BranchName); draftErr != nil {
				log.WarningLog.Printf("[BacklogLifecycle] pushAndCreatePR DraftPRDescription item=%s: %v; using boilerplate body", item.ID, draftErr)
			} else if drafted != "" {
				prBody = drafted
			}
		}
		var prErr error
		prURL, prNumber, prErr = g.CreatePR(prTitle, prBody)
		if prErr != nil {
			stayInReviewAndNotify("PR creation failed", prErr)
			return
		}
		// Cache PR URL + number on the item so the reconciler and UI can use them.
		prURLCopy := prURL
		prNumCopy := prNumber
		if _, updateErr := l.storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
			PrURL:    &prURLCopy,
			PrNumber: &prNumCopy,
		}, nil); updateErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] pushAndCreatePR store PR fields item=%s: %v", item.ID, updateErr)
		}
	}

	// Enable GitHub auto-merge so the PR merges automatically once CI passes.
	// Best-effort: repos without branch protection or auto-merge enabled will fail here,
	// and ReconcilePRPending will detect the merge via polling instead.
	if autoErr := g.EnablePRAutoMerge(prNumber); autoErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] pushAndCreatePR auto-merge item=%s pr=%d: %v", item.ID, prNumber, autoErr)
	} else {
		log.InfoLog.Printf("[BacklogLifecycle] pushAndCreatePR item=%s PR #%d auto-merge enabled", item.ID, prNumber)
	}

	// Transition to pr_pending.
	precondition := &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusReview)}
	if _, transErr := l.storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusPRPending, precondition); transErr != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] pushAndCreatePR pr_pending transition item=%s: %v", item.ID, transErr)
	} else {
		log.InfoLog.Printf("[BacklogLifecycle] pushAndCreatePR item=%s → pr_pending (PR #%d %s)", item.ID, prNumber, prURL)
	}
}

// ReconcilePRPending polls items in pr_pending status. It transitions to done
// when the PR is merged, and spawns a fix session when CI fails or reviewers
// request changes.
func (l *BacklogLifecycleListener) ReconcilePRPending(ctx context.Context, er *EntRepository) {
	items, err := er.FindPRPendingItems(ctx)
	if err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] ReconcilePRPending query error: %v", err)
		return
	}
	for _, item := range items {
		if item.PrNumber == 0 || item.PrURL == "" {
			continue
		}
		repoPath := item.RepoPath
		if repoPath == "" {
			continue
		}
		g := l.getPRPendingCheckerFactory()(repoPath)

		// 1. Check if the PR has been merged → done.
		merged, mergedErr := g.IsPRMerged(item.PrNumber)
		if mergedErr != nil {
			log.DebugLog.Printf("[BacklogLifecycle] ReconcilePRPending IsPRMerged item=%s pr=%d: %v", item.ID, item.PrNumber, mergedErr)
			continue
		}
		if merged {
			precondition := &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusPRPending)}
			if _, transErr := l.storage.TransitionBacklogItemStatus(ctx, item.ID.String(), BacklogStatusDone, precondition); transErr != nil {
				log.ErrorLog.Printf("[BacklogLifecycle] ReconcilePRPending done transition item=%s: %v", item.ID, transErr)
			} else {
				log.InfoLog.Printf("[BacklogLifecycle] ReconcilePRPending item=%s → done (PR #%d merged)", item.ID, item.PrNumber)
			}
			continue
		}

		// 2. PR still open — check CI status and reviews.
		prStatus, statusErr := g.GetPRStatus(item.PrNumber)
		if statusErr != nil {
			log.DebugLog.Printf("[BacklogLifecycle] ReconcilePRPending GetPRStatus item=%s pr=%d: %v", item.ID, item.PrNumber, statusErr)
			continue
		}

		fixSpawner := l.getPRFixSpawner()

		// 2b. Closed without merging (human rejected it) — IsPRMerged already returned
		// false above, and without this check a closed PR reads identically to a
		// healthy open one (no failing CI, no blocking review, no conflict), so the
		// loop below would poll it forever. Clear the cached PR fields so the next
		// pushAndCreatePR call creates a fresh PR instead of reusing the closed one.
		if prStatus.IsClosed {
			closedPrURL, closedPrNum := item.PrURL, item.PrNumber
			emptyURL, zeroNum := "", 0
			if _, updateErr := l.storage.UpdateBacklogItem(ctx, item.ID.String(), BacklogItemUpdate{
				PrURL:    &emptyURL,
				PrNumber: &zeroNum,
			}, nil); updateErr != nil {
				log.ErrorLog.Printf("[BacklogLifecycle] ReconcilePRPending clear closed PR fields item=%s: %v", item.ID, updateErr)
			}
			if fixSpawner == nil {
				log.WarningLog.Printf("[BacklogLifecycle] ReconcilePRPending item=%s: PR #%d closed without merging but no PRFixSpawner configured", item.ID, closedPrNum)
				continue
			}
			fixCtx := fmt.Sprintf("PR #%d (%s) was closed without merging. Investigate why, address any concerns, and open a fresh PR.", closedPrNum, closedPrURL)
			log.InfoLog.Printf("[BacklogLifecycle] ReconcilePRPending item=%s → in_progress: PR #%d closed without merging", item.ID, closedPrNum)
			if fixErr := fixSpawner.AutoReopenForPRFix(ctx, item.ID.String(), fixCtx); fixErr != nil {
				log.ErrorLog.Printf("[BacklogLifecycle] ReconcilePRPending AutoReopenForPRFix (closed) item=%s: %v", item.ID, fixErr)
			}
			continue
		}

		if !prStatus.CIFailing && !prStatus.HasBlockingReviews && !prStatus.HasConflicts {
			continue // PR is open and healthy — wait for merge.
		}

		// 3. CI failure, review changes requested, or merge conflict → spawn fix session.
		if fixSpawner == nil {
			log.WarningLog.Printf("[BacklogLifecycle] ReconcilePRPending item=%s: CI/review issues found but no PRFixSpawner configured", item.ID)
			continue
		}
		fixCtx := fmt.Sprintf("PR #%d (%s) needs fixes:\n\n%s", item.PrNumber, item.PrURL, prStatus.FeedbackText)
		log.InfoLog.Printf("[BacklogLifecycle] ReconcilePRPending item=%s → in_progress for PR fix (CI=%v, reviews=%v, conflict=%v)",
			item.ID, prStatus.CIFailing, prStatus.HasBlockingReviews, prStatus.HasConflicts)
		if fixErr := fixSpawner.AutoReopenForPRFix(ctx, item.ID.String(), fixCtx); fixErr != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] ReconcilePRPending AutoReopenForPRFix item=%s: %v", item.ID, fixErr)
		}
	}
}
