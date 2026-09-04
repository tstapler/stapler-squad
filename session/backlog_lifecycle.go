package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
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

// QueueDequeuer claims and spawns as many queued (and, by default, "ready" —
// config.Config.AutoSpawnReadyItemsOrDefault) backlog items as there are free WIP
// slots, highest-priority first. Called the moment a slot frees up (onSessionExited)
// and by the periodic ReconcileStuck sweep as a safety net for a missed exit hook, a
// concurrency limit raised while items were waiting, or an item reaching "ready"
// between ticks.
type QueueDequeuer interface {
	DequeueNextQueuedItems(ctx context.Context) error
}

// BacklogLifecycleListener drives backlog item state transitions in response to
// session lifecycle events. It must be registered via Instance.RegisterLifecycleListener.
//
// OnLifecycleEvent is non-blocking; all DB work is dispatched to a goroutine.
// Call SetEnabled(false) to make all callbacks no-ops without unwiring.
type BacklogLifecycleListener struct {
	storage *Storage

	// sessionCreatorMu guards sessionCreator for concurrent Set/get access, same
	// pattern as autoReopener/notifier/prFixSpawner below. Needed because
	// production wiring (server/dependencies.go) constructs this listener before
	// SessionService exists, so the spawner is wired post-construction via
	// SetSessionCreator.
	sessionCreatorMu sync.RWMutex
	sessionCreator   ReviewGateSpawner

	// poolMu guards headlessPool for concurrent Set/get access.
	poolMu       sync.RWMutex
	headlessPool *headless.Pool

	// autoReopenMu guards autoReopener for concurrent Set/get access.
	autoReopenMu sync.RWMutex
	autoReopener AutoReopenSpawner

	// prFixMu guards prFixSpawner for concurrent Set/get access.
	prFixMu      sync.RWMutex
	prFixSpawner PRFixSpawner

	// staleWorkRemediatorMu guards staleWorkRemediator for concurrent Set/get access.
	staleWorkRemediatorMu sync.RWMutex
	staleWorkRemediator   StaleWorkRemediator

	// reworkBlockStaleResolverMu guards reworkBlockStaleResolver for concurrent Set/get access.
	reworkBlockStaleResolverMu sync.RWMutex
	reworkBlockStaleResolver   ReworkBlockStaleResolver

	// reviewRespawnMu guards reviewRespawner for concurrent Set/get access.
	reviewRespawnMu sync.RWMutex
	reviewRespawner ReviewRespawner

	// triageRespawnMu guards triageRespawner for concurrent Set/get access.
	triageRespawnMu sync.RWMutex
	triageRespawner TriageRespawner

	// dequeuerMu guards dequeuer for concurrent Set/get access.
	dequeuerMu sync.RWMutex
	dequeuer   QueueDequeuer

	// dashboardBaseURLFnMu guards dashboardBaseURLFn for concurrent Set/get
	// access. Resolves the base URL used to build a clickable deep link back
	// to a backlog item from a PR body (see backlogItemLink in
	// backlog_lifecycle_pr.go); defaults to localhost:8543 and is overridden
	// at startup via SetDashboardBaseURLFn with the real bound address.
	dashboardBaseURLFnMu sync.RWMutex
	dashboardBaseURLFn   func() string

	// oneShotShipRunnerMu guards oneShotShipRunner for concurrent Set/get access.
	oneShotShipRunnerMu sync.RWMutex
	// oneShotShipRunner runs the agent-driven ship flow (see agentShipPrompt)
	// against an ended work session's worktree. nil (the default, and the
	// case for every constructor except production wiring via
	// SetOneShotShipRunner) makes shipViaAgentOrFallback skip straight to the
	// mechanical pushAndCreatePR path — preserves pre-existing behavior for
	// any test/caller that hasn't wired it.
	oneShotShipRunner OneShotShipRunner

	// prPendingCheckerMu guards prPendingCheckerFactory for concurrent Set/get access.
	prPendingCheckerMu      sync.RWMutex
	prPendingCheckerFactory func(repoPath string) prPendingChecker

	// prCreatorMu guards prCreatorFactory for concurrent Set/get access.
	prCreatorMu      sync.RWMutex
	prCreatorFactory func(repoPath, worktreePath, sessionName, branchName, baseCommitSHA string) prCreator

	// orphanedPRFinderMu guards orphanedPRFinder for concurrent Set/get access.
	orphanedPRFinderMu sync.RWMutex
	// orphanedPRFinder looks up an existing PR for repoPath's branch — used by
	// reconcileOrphanedAgentPRs (Epic 3.2's reconciliation backstop). Defaults
	// to defaultOrphanedPRFinder via newListenerBase (resolves owner/repo from
	// repoPath's git remote, then queries GitHub); overridable in tests to
	// avoid real GitHub API calls or needing a real git remote on disk.
	orphanedPRFinder func(ctx context.Context, repoPath, branch string) (*github.PRInfo, error)

	// prByNumberFinderMu guards prByNumberFinder for concurrent Set/get access.
	prByNumberFinderMu sync.RWMutex
	// prByNumberFinder looks up a PR by its immutable number — used by
	// verifyPRHeadBranchMatchesTracked to re-verify, via a live GitHub lookup,
	// that item.PrNumber's real head branch still matches the item's
	// currently-tracked branch before an automated reconciliation call site
	// treats that PR number as ground truth (Story 6, adversarial-review.md's
	// Blocker). Defaults to defaultPRByNumberFinder via newListenerBase
	// (resolves owner/repo from repoPath's git remote, then queries GitHub by
	// PR number); overridable in tests to avoid real GitHub API calls or
	// needing a real git remote on disk.
	prByNumberFinder func(ctx context.Context, repoPath string, prNumber int) (*github.PRInfo, error)

	// branchReconcilerMu guards branchReconciler for concurrent Set/get access.
	branchReconcilerMu sync.RWMutex
	// branchReconciler fetches+merges a branch's remote ref into whatever is
	// currently checked out in a worktree — the push_failed remediation
	// mechanism (attemptPushRemediation). Same signature as
	// git.MergeMainIntoWorktree (the production default installed by
	// newListenerBase), which despite its "main" name just fetches+merges
	// whatever branch name is passed; here that's the item's OWN branch,
	// reconciling a non-fast-forward push rejection by combining local and
	// remote history.
	branchReconciler func(worktreePath, branchName string) (*git.MergeMainResult, error)

	// notifierMu guards notifier for concurrent Set/get access.
	notifierMu sync.RWMutex
	notifier   Notifier

	// sessionArchiverMu guards sessionArchiver for concurrent Set/get access.
	sessionArchiverMu sync.RWMutex
	sessionArchiver   SessionArchiver

	// sessionLivenessCheckerMu guards sessionLivenessChecker for concurrent
	// Set/get access.
	sessionLivenessCheckerMu sync.RWMutex
	// sessionLivenessChecker reports whether the tmux/CLI process backing a
	// session UUID is still alive. Injected via SetSessionLivenessChecker;
	// wired in production to session.Registry (Instance.TmuxSessionExists) —
	// reused, not reinvented, per pre-mortem F3 / Task 2.1.3d. Nil-safe: when
	// unset, the zombie-session detector treats liveness as unknown and does
	// not flag (conservative default for tests that don't wire this).
	sessionLivenessChecker func(sessionUUID string) bool

	// reviewSem limits concurrent review gate goroutines.
	reviewSem chan struct{}

	// shutdownCtx is cancelled by Shutdown(); used by long-running review gate calls.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	// runner encapsulates the spawnReviewGate logic so it can be tested independently.
	runner *ReviewGateRunner

	// pipelineEngine resolves mode-specific slash-command sets and prompts (Epic 1.5).
	// Optional — nil (the default for every constructor except NewBacklogLifecycleListenerWithPool,
	// which is production's only caller) falls back to the built-in default pipeline. Set once at
	// construction and forwarded unchanged into runner — never mutated afterward.
	pipelineEngine PipelineEngine

	// livenessEngine resolves per-stage/per-pipeline-mode stuck-detection thresholds (Epic 1.4 of
	// backlog-custom-workflow-stages), replacing the flat maxHeadlessTriageSessionStaleness/
	// maxWorkSessionStaleness/bounceThreshold/bounceLookback constants at their reconcile* call
	// sites. Optional — nil (the default for every constructor except
	// NewBacklogLifecycleListenerWithPool, production's only caller) falls back to each call
	// site's literal constant, unchanged from pre-Epic-1.4 behavior. Set once at construction,
	// same pattern as pipelineEngine above — never mutated afterward.
	livenessEngine LivenessEngine

	// chainReconcilerMu guards chainReconciler for concurrent Set/get access.
	chainReconcilerMu sync.RWMutex
	// chainReconciler completes pipeline chain-fires interrupted by a crash
	// (webhook-triggers Phase 6, AC5's restart-recovery scenario). nil (the
	// default) makes reconcileTriggerChains a no-op — matches every other
	// optional-dependency detector's nil-safe convention in this file. Wired
	// via SetChainReconciler.
	chainReconciler *TriggerChainReconciler

	enabled atomic.Bool
}

// PipelineEngine returns the PipelineEngine injected at construction (nil if none was
// wired). Exported for the pointer-equality integration test proving BacklogService and
// BacklogLifecycleListener share a single PipelineEngine instance (Story 1.5.1).
func (l *BacklogLifecycleListener) PipelineEngine() PipelineEngine {
	return l.pipelineEngine
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

// SetDashboardBaseURLFn overrides the base URL used by backlogItemLink to
// build a deep link back to a backlog item in agent-created PR bodies.
func (l *BacklogLifecycleListener) SetDashboardBaseURLFn(fn func() string) {
	l.dashboardBaseURLFnMu.Lock()
	defer l.dashboardBaseURLFnMu.Unlock()
	l.dashboardBaseURLFn = fn
}

// SetAutoReopener wires in the spawner used to automatically reopen items for
// rework when a review verdict is FAIL or PARTIAL.
func (l *BacklogLifecycleListener) SetAutoReopener(r AutoReopenSpawner) {
	l.autoReopenMu.Lock()
	defer l.autoReopenMu.Unlock()
	l.autoReopener = r
}

// SetSessionCreator wires in the spawner used to create review-gate sessions
// after construction. Needed because production wiring
// (server/dependencies.go) constructs this listener before SessionService
// exists.
func (l *BacklogLifecycleListener) SetSessionCreator(s ReviewGateSpawner) {
	l.sessionCreatorMu.Lock()
	defer l.sessionCreatorMu.Unlock()
	l.sessionCreator = s
}

// getSessionCreator returns the current review-gate session spawner under a read lock.
func (l *BacklogLifecycleListener) getSessionCreator() ReviewGateSpawner {
	l.sessionCreatorMu.RLock()
	defer l.sessionCreatorMu.RUnlock()
	return l.sessionCreator
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

// SetStaleWorkRemediator wires in the remediator used to clean up and
// respawn stale (but not zombie) work sessions — the "stale_work" reason's
// automated remediation action.
func (l *BacklogLifecycleListener) SetStaleWorkRemediator(r StaleWorkRemediator) {
	l.staleWorkRemediatorMu.Lock()
	defer l.staleWorkRemediatorMu.Unlock()
	l.staleWorkRemediator = r
}

// getStaleWorkRemediator returns the current stale-work remediator under a read lock.
func (l *BacklogLifecycleListener) getStaleWorkRemediator() StaleWorkRemediator {
	l.staleWorkRemediatorMu.RLock()
	defer l.staleWorkRemediatorMu.RUnlock()
	return l.staleWorkRemediator
}

// SetReworkBlockStaleResolver wires in the resolver used to re-check and clear
// an open StuckReasonReworkBlockedStale row once its blocking work session
// recovers, ends, or the item leaves review — same pattern as
// AutoReopenSpawner/PRFixSpawner/SetStaleWorkRemediator.
func (l *BacklogLifecycleListener) SetReworkBlockStaleResolver(r ReworkBlockStaleResolver) {
	l.reworkBlockStaleResolverMu.Lock()
	defer l.reworkBlockStaleResolverMu.Unlock()
	l.reworkBlockStaleResolver = r
}

// getReworkBlockStaleResolver returns the current rework-block-stale resolver under a read lock.
func (l *BacklogLifecycleListener) getReworkBlockStaleResolver() ReworkBlockStaleResolver {
	l.reworkBlockStaleResolverMu.RLock()
	defer l.reworkBlockStaleResolverMu.RUnlock()
	return l.reworkBlockStaleResolver
}

// SetReviewRespawner wires in the spawner used to automatically re-trigger the
// review gate for items abandoned in review with no active session.
func (l *BacklogLifecycleListener) SetReviewRespawner(r ReviewRespawner) {
	l.reviewRespawnMu.Lock()
	defer l.reviewRespawnMu.Unlock()
	l.reviewRespawner = r
}

// getReviewRespawner returns the current review respawner under a read lock.
func (l *BacklogLifecycleListener) getReviewRespawner() ReviewRespawner {
	l.reviewRespawnMu.RLock()
	defer l.reviewRespawnMu.RUnlock()
	return l.reviewRespawner
}

// SetTriageRespawner wires in the spawner used to automatically re-trigger
// triage for idea-status items whose triage session orphaned.
func (l *BacklogLifecycleListener) SetTriageRespawner(r TriageRespawner) {
	l.triageRespawnMu.Lock()
	defer l.triageRespawnMu.Unlock()
	l.triageRespawner = r
}

// getTriageRespawner returns the current triage respawner under a read lock.
func (l *BacklogLifecycleListener) getTriageRespawner() TriageRespawner {
	l.triageRespawnMu.RLock()
	defer l.triageRespawnMu.RUnlock()
	return l.triageRespawner
}

// SetDequeuer wires in the spawner used to dequeue queued backlog items once a
// WIP slot frees up.
func (l *BacklogLifecycleListener) SetDequeuer(d QueueDequeuer) {
	l.dequeuerMu.Lock()
	defer l.dequeuerMu.Unlock()
	l.dequeuer = d
}

// getDequeuer returns the current dequeuer under a read lock.
func (l *BacklogLifecycleListener) getDequeuer() QueueDequeuer {
	l.dequeuerMu.RLock()
	defer l.dequeuerMu.RUnlock()
	return l.dequeuer
}

// triggerDequeue best-effort dequeues queued items on the current goroutine. It
// swallows and logs errors — a failed dequeue attempt must never block or fail
// the caller (a session-exit transition or the periodic stuck sweep).
func (l *BacklogLifecycleListener) triggerDequeue(ctx context.Context) {
	d := l.getDequeuer()
	if d == nil {
		return
	}
	if err := d.DequeueNextQueuedItems(ctx); err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] DequeueNextQueuedItems error: %v", err)
	}
}

// SetChainReconciler wires in the pipeline-chain restart-recovery reconciler
// (webhook-triggers Phase 6). Called via server/dependencies.go once a
// ChainFirer has been constructed (see Storage.WireChainFirer).
func (l *BacklogLifecycleListener) SetChainReconciler(r *TriggerChainReconciler) {
	l.chainReconcilerMu.Lock()
	defer l.chainReconcilerMu.Unlock()
	l.chainReconciler = r
}

// getChainReconciler returns the current chain reconciler under a read lock.
func (l *BacklogLifecycleListener) getChainReconciler() *TriggerChainReconciler {
	l.chainReconcilerMu.RLock()
	defer l.chainReconcilerMu.RUnlock()
	return l.chainReconciler
}

// reconcileTriggerChains delegates to TriggerChainReconciler.ReconcileChains
// — the periodic counterpart to EntRepository.dispatchChainFire's happy-path
// fire, both funneling through the same ChainFirer.Fire so "fire exactly
// once" logic lives in one place. No-op when no reconciler is wired.
func (l *BacklogLifecycleListener) reconcileTriggerChains(ctx context.Context, er *EntRepository) {
	r := l.getChainReconciler()
	if r == nil {
		return
	}
	r.ReconcileChains(ctx, er)
}

// SetOneShotShipRunner wires in the runner used by shipViaAgentOrFallback to
// attempt an agent-driven PR ship (see agentShipPrompt) before falling back
// to the mechanical pushAndCreatePR path. Optional — nil means every PASS
// verdict with an ended work session goes straight to the mechanical path,
// matching this fix's pre-existing behavior.
func (l *BacklogLifecycleListener) SetOneShotShipRunner(r OneShotShipRunner) {
	l.oneShotShipRunnerMu.Lock()
	defer l.oneShotShipRunnerMu.Unlock()
	l.oneShotShipRunner = r
}

// getOneShotShipRunner returns the current one-shot ship runner under a read lock.
func (l *BacklogLifecycleListener) getOneShotShipRunner() OneShotShipRunner {
	l.oneShotShipRunnerMu.RLock()
	defer l.oneShotShipRunnerMu.RUnlock()
	return l.oneShotShipRunner
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

// SetOrphanedPRFinder overrides the function used to look up an existing PR
// for a repo path's branch, used by reconcileOrphanedAgentPRs (Epic 3.2).
// Overridable in tests to avoid real GitHub API calls or needing a real git
// remote on disk; production code never needs to call this, since
// newListenerBase installs defaultOrphanedPRFinder.
func (l *BacklogLifecycleListener) SetOrphanedPRFinder(f func(ctx context.Context, repoPath, branch string) (*github.PRInfo, error)) {
	l.orphanedPRFinderMu.Lock()
	defer l.orphanedPRFinderMu.Unlock()
	l.orphanedPRFinder = f
}

// getOrphanedPRFinder returns the current orphaned-PR finder under a read lock.
func (l *BacklogLifecycleListener) getOrphanedPRFinder() func(ctx context.Context, repoPath, branch string) (*github.PRInfo, error) {
	l.orphanedPRFinderMu.RLock()
	defer l.orphanedPRFinderMu.RUnlock()
	return l.orphanedPRFinder
}

// SetPRByNumberFinder overrides the function used to look up a PR by its
// immutable number, used by verifyPRHeadBranchMatchesTracked (Story 6).
// Overridable in tests to avoid real GitHub API calls or needing a real git
// remote on disk; production code never needs to call this, since
// newListenerBase installs defaultPRByNumberFinder.
func (l *BacklogLifecycleListener) SetPRByNumberFinder(f func(ctx context.Context, repoPath string, prNumber int) (*github.PRInfo, error)) {
	l.prByNumberFinderMu.Lock()
	defer l.prByNumberFinderMu.Unlock()
	l.prByNumberFinder = f
}

// getPRByNumberFinder returns the current PR-by-number finder under a read lock.
func (l *BacklogLifecycleListener) getPRByNumberFinder() func(ctx context.Context, repoPath string, prNumber int) (*github.PRInfo, error) {
	l.prByNumberFinderMu.RLock()
	defer l.prByNumberFinderMu.RUnlock()
	return l.prByNumberFinder
}

// SetBranchReconciler overrides the function used to fetch+merge a branch's
// remote ref into its worktree for push_failed remediation
// (attemptPushRemediation). Overridable in tests to avoid needing a real git
// repo on disk; production code never needs to call this, since
// newListenerBase installs git.MergeMainIntoWorktree.
func (l *BacklogLifecycleListener) SetBranchReconciler(f func(worktreePath, branchName string) (*git.MergeMainResult, error)) {
	l.branchReconcilerMu.Lock()
	defer l.branchReconcilerMu.Unlock()
	l.branchReconciler = f
}

// getBranchReconciler returns the current branch reconciler under a read lock.
func (l *BacklogLifecycleListener) getBranchReconciler() func(worktreePath, branchName string) (*git.MergeMainResult, error) {
	l.branchReconcilerMu.RLock()
	defer l.branchReconcilerMu.RUnlock()
	return l.branchReconciler
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

// notifyTransitionFailed publishes an operator-facing notification when a
// status-transition write fails AFTER its side effects have already happened
// — e.g. a PR was confirmed merged, or a commit was confirmed shipped to
// main — leaving the item's status silently out of sync with reality.
// Mirrors server/services/backlog_service_triage.go's identical helper (same
// shape, different package: this package must not import the server layer —
// see .golangci.yml's no_server_in_core depguard rule). No-op if no notifier
// is wired. Part of the fix for the recurring "silent status-transition
// failure" bug shape — see that helper's doc comment for the full history
// (BUG-030, BUG-040, BUG-041, BUG-046, BUG-048, and this fix's sibling call
// sites in reconcileBouncingItems/ReconcilePRPending).
func (l *BacklogLifecycleListener) notifyTransitionFailed(itemID, itemTitle, failureContext string, writeErr error) {
	l.notify(itemID,
		"Status update failed after work completed",
		fmt.Sprintf("%s — %s: %v. The item's status may not reflect reality; check manually.", itemTitle, failureContext, writeErr),
		7, // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
		3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
	)
}

// SetSessionArchiver wires in the archiver used to soft-archive backlog work
// sessions belonging to done/archived items that the transition hook missed
// (see the archive_terminal_sessions detector in ReconcileStuck). Optional —
// nil means the detector no-ops.
func (l *BacklogLifecycleListener) SetSessionArchiver(a SessionArchiver) {
	l.sessionArchiverMu.Lock()
	defer l.sessionArchiverMu.Unlock()
	l.sessionArchiver = a
}

// getSessionArchiver returns the current session archiver under a read lock.
func (l *BacklogLifecycleListener) getSessionArchiver() SessionArchiver {
	l.sessionArchiverMu.RLock()
	defer l.sessionArchiverMu.RUnlock()
	return l.sessionArchiver
}

// SetSessionLivenessChecker wires the function used by the zombie-session
// review detector (pre-mortem F3) to confirm whether a session's underlying
// tmux/CLI process is actually still alive, rather than trusting the DB's
// EndedAt IS NULL row alone. Optional — nil means the zombie detector never
// flags (conservative: unknown liveness is treated as "assume alive").
func (l *BacklogLifecycleListener) SetSessionLivenessChecker(f func(sessionUUID string) bool) {
	l.sessionLivenessCheckerMu.Lock()
	defer l.sessionLivenessCheckerMu.Unlock()
	l.sessionLivenessChecker = f
}

// getSessionLivenessChecker returns the current liveness checker under a read lock.
func (l *BacklogLifecycleListener) getSessionLivenessChecker() func(sessionUUID string) bool {
	l.sessionLivenessCheckerMu.RLock()
	defer l.sessionLivenessCheckerMu.RUnlock()
	return l.sessionLivenessChecker
}

// getHeadlessPool returns the current headless pool under a read lock.
func (l *BacklogLifecycleListener) getHeadlessPool() *headless.Pool {
	l.poolMu.RLock()
	defer l.poolMu.RUnlock()
	return l.headlessPool
}

func (l *BacklogLifecycleListener) getDashboardBaseURL() string {
	l.dashboardBaseURLFnMu.RLock()
	defer l.dashboardBaseURLFnMu.RUnlock()
	return l.dashboardBaseURLFn()
}

// Shutdown cancels in-flight review gate calls. Safe to call concurrently.
func (l *BacklogLifecycleListener) Shutdown() {
	if l.shutdownCancel != nil {
		l.shutdownCancel()
	}
}

// newListenerBase initialises fields common to all BacklogLifecycleListener constructors.
// pipelineEngine and livenessEngine may both be nil — see their field doc comments for the
// fallback behavior.
func newListenerBase(storage *Storage, pipelineEngine PipelineEngine, livenessEngine LivenessEngine) *BacklogLifecycleListener {
	ctx, cancel := context.WithCancel(context.Background())
	l := &BacklogLifecycleListener{
		storage:                 storage,
		pipelineEngine:          pipelineEngine,
		livenessEngine:          livenessEngine,
		reviewSem:               make(chan struct{}, maxConcurrentReviewGates),
		shutdownCtx:             ctx,
		shutdownCancel:          cancel,
		prPendingCheckerFactory: defaultPRPendingCheckerFactory,
		prCreatorFactory:        defaultPRCreatorFactory,
		branchReconciler:        git.MergeMainIntoWorktree,
		orphanedPRFinder:        defaultOrphanedPRFinder,
		prByNumberFinder:        defaultPRByNumberFinder,
		dashboardBaseURLFn:      func() string { return "http://localhost:8543" },
	}
	l.runner = NewReviewGateRunner(storage, l.getAutoReopener, l.getNotifier, l.getSessionCreator, pipelineEngine)
	return l
}

// NewBacklogLifecycleListener creates a listener backed by the given storage.
// The review gate is disabled (sessionCreator=nil, headlessPool=nil). No PipelineEngine
// is wired (nil) — callers needing one should use NewBacklogLifecycleListenerWithPool.
func NewBacklogLifecycleListener(storage *Storage) *BacklogLifecycleListener {
	return newListenerBase(storage, nil, nil)
}

// NewBacklogLifecycleListenerWithSpawner creates a listener that will spawn a
// review gate session when a work session exits and SkipReviewGate is false.
func NewBacklogLifecycleListenerWithSpawner(storage *Storage, spawner ReviewGateSpawner) *BacklogLifecycleListener {
	l := newListenerBase(storage, nil, nil)
	l.SetSessionCreator(spawner)
	return l
}

// NewBacklogLifecycleListenerWithPool creates a listener that uses a headless.Pool
// for review gate calls instead of spawning a tmux session. pipelineEngine is the
// shared PipelineEngine instance (Epic 1.5, Story 1.5.1) — pass nil to fall back to
// the built-in default pipeline for every item. livenessEngine is the shared
// LivenessEngine instance (Epic 1.4) — pass nil to fall back to each stuck-detection
// sweep's literal constant for every item.
func NewBacklogLifecycleListenerWithPool(storage *Storage, pool *headless.Pool, pipelineEngine PipelineEngine, livenessEngine LivenessEngine) *BacklogLifecycleListener {
	l := newListenerBase(storage, pipelineEngine, livenessEngine)
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
	case EventExited, EventStopped:
		// A deliberate operator stop (stop_session, DeleteSession, backlog
		// stale-work remediation) ends the session exactly as much as an
		// unexpected exit does — the ItemSession bookkeeping and downstream
		// reconciliation must not depend on which one happened. See BUG-027.
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
		log.ErrorLog().Printf("[BacklogLifecycle] GetItemSessionBySessionUUID(%s) error: %v", sessionUUID, err)
		return
	}
	if err := l.storage.UpdateItemSessionStarted(ctx, is.ID, time.Now()); err != nil {
		log.ErrorLog().Printf("[BacklogLifecycle] UpdateItemSessionStarted(%s) error: %v", is.ID, err)
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
		log.ErrorLog().Printf("[BacklogLifecycle] GetItemSessionBySessionUUID(%s) error: %v", sessionUUID, err)
		return
	}

	// Snapshot before this call's own bookkeeping (below) overwrites it: a
	// non-nil EndedAt here means some OTHER code path already deliberately
	// closed this session out before this exit event even arrived, and that
	// path — not this one — owns whatever should happen next. See BUG-064:
	// RemediateStaleWorkSession ends a stale work session's ItemSession row
	// and kills its tmux pane (KillTmuxPaneOnly -> Instance.KillSession) before
	// calling AutoRespawnAutonomousWork to give the item a fresh work-session
	// turn. Killing the pane fires this exact onSessionExited path
	// asynchronously, in its own goroutine (instanceBacklogListener.
	// OnLifecycleEvent), racing AutoRespawnAutonomousWork's synchronous
	// respawn. This handler's DB-only work is reliably faster than
	// AutoRespawnAutonomousWork's (GetBacklogItem + ListItemSessions +
	// tombstone + SpawnSessionFromItem), so without this guard it wins nearly
	// every time — flipping status to review out from under
	// AutoRespawnAutonomousWork, whose own "already moved on" guard then
	// silently no-ops (no error, no log line), permanently discarding the
	// intended fresh work session and instead re-reviewing the exact same
	// stale, already-rejected diff. Confirmed live (item 2d7fac56,
	// 2026-08-06T00:44:12): staplersquad.log shows "ended stale work
	// session=...respawning" immediately followed by "transitioned to review"
	// for the very same session, with no AutoRespawnAutonomousWork log line
	// ever appearing.
	alreadyEndedByOtherPath := is.EndedAt != nil

	// Record end time for all session roles (triage, review, work).
	now := time.Now()
	if err := l.storage.UpdateItemSessionEnded(ctx, is.ID, now); err != nil { //nolint:silenttransition bookkeeping timestamp; the zombie-session detector (reconcileStuckReviewItems) falls back to SessionLivenessChecker rather than relying solely on EndedAt, so a failed write here doesn't fully hide a dead session
		log.ErrorLog().Printf("[BacklogLifecycle] UpdateItemSessionEnded(%s) error: %v", is.ID, err)
	}

	// Review sessions are handled by a dedicated post-verdict path: they don't
	// drive an in_progress→review/done transition themselves, they process the
	// verdict submitted (or not) by the review session that just exited. Other
	// non-work roles (e.g. triage) have nothing further to do here.
	switch is.Role {
	case SessionRoleReview:
		// forcePush=false: this is the normal, real-time exit-event path — defer to
		// a still-live work session's own /backlog/ship instead of pushing
		// mechanically. See handleReviewSessionExited's doc comment.
		l.handleReviewSessionExited(ctx, is, false)
		return
	case SessionRoleWork:
		// fall through to the in_progress→review/done logic below.
	default:
		return
	}

	if alreadyEndedByOtherPath {
		// Whoever closed this session out ahead of this exit event already
		// owns the follow-up (e.g. AutoRespawnAutonomousWork deciding whether
		// to spawn a fresh work session) — driving our own status transition
		// here would race it and, per BUG-064, reliably win, silently
		// discarding that follow-up. Nothing further to do.
		log.DebugLog().Printf("[BacklogLifecycle] onSessionExited item=%s session=%s: already ended by another code path before this exit event; skipping status transition", is.BacklogItemID, sessionUUID)
		return
	}

	// Look up the BacklogItem via storage (no longer an eager-loaded edge).
	item, err := l.storage.GetBacklogItem(ctx, is.BacklogItemID)
	if err != nil {
		log.ErrorLog().Printf("[BacklogLifecycle] GetBacklogItem for session %s (item %s): %v", sessionUUID, is.BacklogItemID, err)
		return
	}

	if BacklogStatus(item.Status) != BacklogStatusInProgress {
		log.DebugLog().Printf("[BacklogLifecycle] item %s is %s (not in_progress); skipping", item.ID, item.Status)
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
	if _, err := l.storage.TransitionBacklogItemStatus(ctx, item.ID, toStatus, precondition, TriggeredBySystem); err != nil {
		log.ErrorLog().Printf("[BacklogLifecycle] TransitionBacklogItemStatus item=%s to=%s: %v", item.ID, toStatus, err)
		return
	}

	// The item is leaving in_progress — any open stale_work row is stale by
	// definition now. Resolve immediately rather than waiting for the
	// self-heal sweep's next tick (Task 2.1.5a).
	l.resolveStuckLogged(ctx, l.storage.repo, item.ID, domain.StuckReasonStaleWork, "onSessionExited")

	log.InfoLog().Printf("[BacklogLifecycle] item %s transitioned to %s (session %s exited)", item.ID, toStatus, sessionUUID)

	// The item just left in_progress, freeing a WIP slot — dequeue immediately
	// rather than waiting for the next ReconcileStuck tick (safety-net only).
	go l.triggerDequeue(context.Background())

	// Spawn review gate if the item moved to review and a review mechanism is configured.
	if toStatus == BacklogStatusReview && !item.SkipReviewGate && l.getSessionCreator() != nil {
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

// BackfillStuckStates seeds durable BacklogStuckState rows for items that are
// already stuck at startup, with notified_at pre-set, so the first genuine
// reconcile tick after a restart/deploy does not re-notify for conditions
// that were already known (and already surfaced) before the restart. Intended
// to be called once, before the reconcile ticker goroutine starts. Idempotent
// via MarkStuck's (item_id, reason) unique-constraint upsert — safe to call on
// every startup, not just the first one. Best-effort throughout: query/write
// failures are logged, never returned — backfill must never block startup.
//
// Scope note: only the two DB-derivable reasons that already have a queryable
// detection surface as of this Epic are seeded — abandoned_review (via the
// existing FindStuckReviewItems query) and stale_work (mirroring
// reconcileStaleWorkSessions' maxWorkSessionStaleness check, without
// modifying that function). rework_cap, bouncing, and push_failed are
// deliberately NOT seeded here: their detection logic is introduced by Phase
// 2 (Stories 2.1.2, 2.1.4, 2.1.6 respectively) and does not exist yet in this
// Epic — seeding them now would mean fabricating that not-yet-built detection
// logic ahead of schedule. Once Phase 2 ships those detectors, their own
// MarkStuck/MarkStuckNotified notify-once dedup naturally covers the "first
// tick after shipping" storm-suppression case for those three reasons at that
// time, the same way this backfill does for the two reasons seeded today.
//
// pr_ready_unmerged is excluded for a different reason: detecting it needs a
// GetPRStatus/IsPRMerged GitHub call per pr_pending item, which would burst
// the GitHub API on every one of the 15+ daily boots. The first genuine tick
// after startup surfaces it via its own notified_at IS NULL + 30-min gate —
// a one-tick delay, not a startup API burst.
func (l *BacklogLifecycleListener) BackfillStuckStates(ctx context.Context) {
	er := l.storage.repo

	seeded := 0

	// abandoned_review: review-status items with a verdict on record but
	// nothing active in flight.
	reviewItems, err := er.FindStuckReviewItems(ctx)
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] BackfillStuckStates FindStuckReviewItems error: %v", err)
	} else {
		for _, item := range reviewItems {
			if l.backfillMarkAndNotify(ctx, er, item.ID.String(), domain.StuckReasonAbandonedReview, BacklogStatusReview,
				"backfilled at startup: stuck in review with nothing in flight") {
				seeded++
			}
		}
	}

	// stale_work: in_progress items whose active work session has gone quiet
	// past maxWorkSessionStaleness.
	inProgressItems, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{string(BacklogStatusInProgress)},
	})
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] BackfillStuckStates ListBacklogItems error: %v", err)
	} else {
		for _, item := range inProgressItems {
			sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
			if sessErr != nil {
				log.WarningLog().Printf("[BacklogLifecycle] BackfillStuckStates ListItemSessions item=%s: %v", item.ID, sessErr)
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
			if l.backfillMarkAndNotify(ctx, er, item.ID, domain.StuckReasonStaleWork, BacklogStatusInProgress,
				fmt.Sprintf("backfilled at startup: no progress since %s", lastProgress)) {
				seeded++
			}
		}
	}

	log.InfoLog().Printf("[BacklogLifecycle] BackfillStuckStates: seeded %d stuck row(s) at startup", seeded)
}

// backfillMarkAndNotify marks a stuck row and immediately pre-sets
// notified_at so the first genuine reconcile tick after startup does not
// re-notify for a condition already known before the restart. Returns
// whether a row was newly opened by this call. Best-effort: errors are
// logged, never returned.
func (l *BacklogLifecycleListener) backfillMarkAndNotify(ctx context.Context, er *EntRepository, itemID string, reason domain.StuckReason, expectedStatus BacklogStatus, stuckContext string) bool {
	applied, err := er.MarkStuck(ctx, itemID, reason, expectedStatus, stuckContext)
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] BackfillStuckStates MarkStuck item=%s reason=%s: %v", itemID, reason, err)
		return false
	}
	if !applied {
		return false
	}
	if _, notifyErr := er.MarkStuckNotified(ctx, itemID, reason); notifyErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] BackfillStuckStates MarkStuckNotified item=%s reason=%s: %v", itemID, reason, notifyErr)
	}
	return true
}

// runStuckDetector invokes fn with its own recover(), so a panic in one
// detector cannot skip the others or merge detection (Story 2.1.5, pre-mortem
// P3). The existing whole-tick recover() in server/dependencies.go stays as
// the outer net. Every panic is logged at WARNING (never silent — pre-mortem
// F5) and the detector name is appended to okNames/panickedNames for the
// per-tick self-check summary line.
func (l *BacklogLifecycleListener) runStuckDetector(name string, okNames, panickedNames *[]string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.WarningLog().Printf("[BacklogLifecycle] stuck detector %q panicked (recovered): %v", name, r)
			*panickedNames = append(*panickedNames, name)
			return
		}
		*okNames = append(*okNames, name)
	}()
	fn()
}

// ReconcileStuck calls ReconcileStuckItems and logs the result.
// Intended to be called on a periodic ticker as a safety net for abnormal session exits.
// No-op when the listener is disabled.
func (l *BacklogLifecycleListener) ReconcileStuck(ctx context.Context) {
	if !l.enabled.Load() {
		return
	}
	er := l.storage.repo
	n, err := er.ReconcileStuckItems(ctx)
	if err != nil {
		log.ErrorLog().Printf("[BacklogLifecycle] ReconcileStuckItems error: %v", err)
		return
	}
	if n > 0 {
		log.InfoLog().Printf("[BacklogLifecycle] ReconcileStuckItems: transitioned %d stuck items to review", n)
	} else {
		log.DebugLog().Printf("[BacklogLifecycle] ReconcileStuckItems: no stuck items found")
	}

	// Re-spawn review gates for items stuck in "review" with no review session.
	// Occurs when the headless pool was unavailable at the time of the work session exit.
	// Scoped to this block only (not an early return) — PR-pending polling and staleness
	// detection below must still run even when no review mechanism is configured.
	if l.getSessionCreator() != nil {
		items, gateErr := er.FindReviewItemsWithoutGate(ctx)
		if gateErr != nil {
			log.ErrorLog().Printf("[BacklogLifecycle] FindReviewItemsWithoutGate error: %v", gateErr)
		} else {
			for _, item := range items {
				var workSession *ItemSessionSummary
				if len(item.Edges.ItemSessions) > 0 {
					s := itemSessionToSummary(item.Edges.ItemSessions[0])
					workSession = &s
				}
				if workSession == nil {
					log.DebugLog().Printf("[BacklogLifecycle] ReconcileStuckReviewGates: item %s has no work session, skipping", item.ID)
					continue
				}
				log.InfoLog().Printf("[BacklogLifecycle] ReconcileStuckReviewGates: re-spawning review gate for item %s", item.ID)
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

	// Self-heal pr_pending items whose pr_number is missing (0) despite having a
	// pr_url — otherwise permanently invisible to FindPRPendingItems' PrNumberGT(0)
	// filter below, so they'd never get polled. See BackfillMissingPRNumbers doc.
	if n, backfillErr := er.BackfillMissingPRNumbers(ctx); backfillErr != nil {
		log.ErrorLog().Printf("[BacklogLifecycle] BackfillMissingPRNumbers error: %v", backfillErr)
	} else if n > 0 {
		log.InfoLog().Printf("[BacklogLifecycle] BackfillMissingPRNumbers: backfilled pr_number for %d item(s)", n)
	}

	// Durable stuck-reason detectors, each panic-isolated (Story 2.1.5e) so one
	// detector's panic cannot skip the others or merge detection below.
	var okNames, panickedNames []string

	// Re-read each live work session's actual HEAD into its ItemSession row, so
	// LastCommitSha means "the session's latest commit" rather than the value it
	// was seeded with at spawn. Registered first so every detector below in this
	// same tick — notably pr_ready+merge_detection's closeIfSupersededByMain —
	// sees fresh data rather than lagging a full tick behind (BUG-047).
	l.runStuckDetector("work_commit_refresh", &okNames, &panickedNames, func() {
		l.refreshWorkSessionGitActivity(ctx)
	})

	// Flag in_progress work sessions that have gone quiet for too long. Detection +
	// notification only — a slow-but-alive agent should not be force-stopped.
	l.runStuckDetector("stale_work", &okNames, &panickedNames, func() {
		l.reconcileStaleWorkSessions(ctx, er)
	})

	// Resolve-only pass for rework_blocked_stale: the mark side
	// (notifyIfActiveWorkSessionStale) has no periodic tick of its own, so
	// this closes an open row once its blocking session recovers, ends, or
	// the item leaves review, mirroring reconcileStaleWorkSessions' own
	// resolve half for the structurally similar in_progress case.
	l.runStuckDetector("rework_blocked_stale", &okNames, &panickedNames, func() {
		l.reconcileReworkBlockedStaleResolution(ctx, er)
	})

	// Resolve-only pass for respawn_blocked_active: load-bearing (not merely
	// convenient) for AutoRespawnReview, whose only caller gates the respawn
	// behind a backoff that eventually parks and stops re-invoking it — see
	// reconcileRespawnBlockedActiveResolution's doc comment for the full
	// orphaned-row scenario this closes.
	l.runStuckDetector("respawn_blocked_active", &okNames, &panickedNames, func() {
		l.reconcileRespawnBlockedActiveResolution(ctx, er)
	})

	// Flag review-status items that already have a review verdict but nothing
	// active in flight (AutoReopenAfterFailedReview's spawn failed and rolled
	// back, or a legacy review session exited without a verdict), plus
	// zombie-session review items (pre-mortem F3). Detection + notification
	// only, same rationale as reconcileStaleWorkSessions above: these items
	// are otherwise invisible to every other reconciler.
	l.runStuckDetector("abandoned_review", &okNames, &panickedNames, func() {
		l.reconcileStuckReviewItems(ctx, er)
	})

	// Reconciliation backstop (Epic 3.2, "PR Metadata Capture Fix"): review-status
	// items with no live session but a real, unreported GitHub PR for their
	// branch — an agent that shipped via /backlog:ship but crashed before calling
	// report_pr_created. Self-heals immediately on a match; see
	// reconcileOrphanedAgentPRs' doc comment for why this is a backstop, not a
	// new StuckReason.
	l.runStuckDetector("orphaned_agent_pr", &okNames, &panickedNames, func() {
		l.reconcileOrphanedAgentPRs(ctx, er)
	})

	// Apply a review verdict that was recorded but never actioned because the
	// review session died before its exit event fired — distinct from the
	// zombie detection above (see reconcileUnprocessedReviewVerdicts doc comment).
	l.runStuckDetector("unprocessed_review_verdict", &okNames, &panickedNames, func() {
		l.reconcileUnprocessedReviewVerdicts(ctx, er)
	})

	// Bouncing (non-converging in_progress<->review cycle) detector — wired
	// before merge detection per Task 2.1.4b so a panic here can't skip it
	// (also guarded by its own recover() below regardless of ordering).
	l.runStuckDetector("bouncing", &okNames, &panickedNames, func() {
		l.reconcileBouncingItems(ctx, er)
	})

	// Retry the push+PR flow for items with an open push_failed row (Phase B
	// of docs/tasks/backlog-stuck-item-auto-remediation.md). This is the
	// periodic counterpart to pushAndCreatePR's own event-driven attempt:
	// nothing else ever re-invokes pushAndCreatePR for an item that has no
	// active session, so without this detector a push_failed item sits stuck
	// forever once nothing is left running (the exact shape live-repro'd
	// 2026-07-20: c2ad7bf3-91bf-4d47-8654-0f2f20869080).
	l.runStuckDetector("push_failed", &okNames, &panickedNames, func() {
		l.reconcilePushFailedItems(ctx, er)
	})

	// Flag idea-status items whose triage session crashed, was killed, or never
	// reached the completion goroutine (e.g. a server restart mid-triage) —
	// previously only caught on a manual re-trigger (tombstoneOrphanTriageSessions),
	// now a standing detector so it doesn't require a human to notice and retry.
	l.runStuckDetector("orphaned_triage", &okNames, &panickedNames, func() {
		l.reconcileOrphanedTriageItems(ctx, er)
	})

	// Retry triage for every open orphaned_triage row still anchored at
	// "idea" (Phase B of docs/tasks/backlog-stuck-item-auto-remediation.md) —
	// the periodic counterpart to reconcileOrphanedTriageItems' one-time
	// MarkStuck+notify above, which never retries on its own once the
	// orphaned triage session is tombstoned (that session's EndedAt is no
	// longer nil, so the detector above never re-fires for the same item).
	// Closes the exact gap confirmed live 2026-07-27
	// (docs/tasks/backlog-feature-improvement.md): items 4f03de7b and
	// 505fb733 sat in "idea" for 2 days with only the one notification ever
	// sent.
	l.runStuckDetector("orphaned_triage_remediation", &okNames, &panickedNames, func() {
		l.reconcileOrphanedTriageRemediation(ctx, er)
	})

	// Flag queued items DequeueNextQueuedItems' planning gate refuses to ever
	// claim (plan not approved, skip_planning not set) — otherwise silent
	// forever except for a per-tick WARNING log. See reconcilePlanNotApprovedItems'
	// doc comment (BUG-038).
	l.runStuckDetector("plan_not_approved", &okNames, &panickedNames, func() {
		l.reconcilePlanNotApprovedItems(ctx, er)
	})

	// Self-heal sweep: resolve any open stuck row whose reason's expected
	// status no longer matches the item's current status (Task 2.1.5d).
	l.runStuckDetector("self_heal", &okNames, &panickedNames, func() {
		l.selfHealStuck(ctx, er)
	})

	// Multi-reason escalation detector (Signal 1, Epic 1.2): marks/refreshes a
	// durable multiple_reasons row for any item with 2+ simultaneously open
	// non-escalation stuck reasons, and dwell-gated-notifies once. Registered
	// immediately after self_heal (not before it) so a terminal-status item's
	// stale non-escalation rows have already been cleared this tick before
	// being counted — see reconcileMultiReasonEscalation's doc comment.
	l.runStuckDetector("multi_reason_escalation", &okNames, &panickedNames, func() {
		l.reconcileMultiReasonEscalation(ctx, er)
	})

	// Auto-archive items that have sat in "done" for longer than maxDoneAge.
	// Registered immediately before archive_terminal_sessions so a freshly
	// archived item's work sessions are swept in the same tick — see
	// archiveStaleDoneItems' doc comment.
	l.runStuckDetector("auto_archive_done", &okNames, &panickedNames, func() {
		l.archiveStaleDoneItems(ctx)
	})

	// Safety-net sweep: archive work sessions for items already in done/archived
	// status that TransitionBacklogItemStatus's archival hook missed (pre-existing
	// terminal items from before that hook existed, or a race/crash mid-transition).
	// See SessionArchiver's doc comment. No-op when unwired.
	l.runStuckDetector("archive_terminal_sessions", &okNames, &panickedNames, func() {
		l.reconcileTerminalItemSessions(ctx)
	})

	// Self-heal items whose real, cached PR reference (prNumber/prUrl) has
	// drifted out of ReconcilePRPending's status=="pr_pending" view — see
	// FindDriftedPRItems' doc comment. Registered immediately before
	// pr_ready+merge_detection so a recovered item is picked up by the merge/
	// CI polling sweep below in this same tick rather than an extra cycle
	// later.
	l.runStuckDetector("pr_drift_recovery", &okNames, &panickedNames, func() {
		l.reconcileDriftedPRItems(ctx, er)
	})

	// Flag pr_pending items with no PR reference at all (pr_number == 0) — a
	// permanent dead end otherwise invisible to FindPRPendingItems'
	// PrNumberGT(0) filter and everything downstream of it, including
	// ReconcilePRPending itself (BUG-040). Registered immediately before
	// pr_ready+merge_detection, mirroring pr_drift_recovery's placement, so a
	// drift-recovered item that still somehow lacks a pr_number is caught in
	// the same tick.
	l.runStuckDetector("pr_pending_no_pr", &okNames, &panickedNames, func() {
		l.reconcilePRPendingWithoutPRItems(ctx, er)
	})

	// Poll pr_pending items: auto-transition to done when the PR is merged,
	// and (Story 2.1.1) flag/resolve pr_ready_unmerged.
	l.runStuckDetector("pr_ready+merge_detection", &okNames, &panickedNames, func() {
		l.ReconcilePRPending(ctx, er)
	})

	// Resolve-only counterpart to notifyBlockedByDependency
	// (server/services/backlog_service_triage.go), which marks
	// StuckReasonBlockedByDependency but has no periodic tick of its own to
	// notice when the blocker reaches a resolved status — that only happens
	// on the next DequeueNextQueuedItems sweep, which won't re-run for an
	// item that's already been skipped once this tick.
	l.runStuckDetector("blocked_by_dependency", &okNames, &panickedNames, func() {
		l.reconcileBlockedByDependencyResolution(ctx, er)
	})

	// Safety net for the backlog work-item queue: dequeues queued items whose
	// exit-hook trigger was missed (server restart mid-transition, panic in the
	// hook's own goroutine) or whose slot was freed by the concurrency limit
	// being raised via Settings while items were queued (not itself a session
	// exit, so onSessionExited never fires for it).
	l.runStuckDetector("dequeue_backlog_items", &okNames, &panickedNames, func() {
		l.triggerDequeue(ctx)
	})

	// Restart-recovery for pipeline chain-fires interrupted by a crash between
	// the "done" transition committing and the chained session actually being
	// created (webhook-triggers AC5). See reconcileTriggerChains' doc comment.
	l.runStuckDetector("trigger_chain_reconcile", &okNames, &panickedNames, func() {
		l.reconcileTriggerChains(ctx, er)
	})

	openRows, countErr := er.FindOpenStuckStates(ctx)
	openCount := -1
	if countErr == nil {
		openCount = len(openRows)
	}
	log.InfoLog().Printf("[BacklogLifecycle] stuck sweep tick: detectors ok=%v panicked=%v openRows=%d", okNames, panickedNames, openCount)
}

// resolveStuckLogged resolves an open BacklogStuckState row for (itemID, reason),
// logging (not returning) any error. Centralizes the resolve-and-log idiom that was
// previously repeated verbatim across every detector's resolve path (10 call sites —
// backlog-feature-improvement audit finding). caller identifies the calling
// function/branch for the log line; reason is included automatically, so callers no
// longer need to encode it in caller too (except to distinguish multiple resolve call
// sites for the same reason within one function, e.g. "ReconcilePRPending/closed").
func (l *BacklogLifecycleListener) resolveStuckLogged(ctx context.Context, er *EntRepository, itemID string, reason domain.StuckReason, caller string) {
	if _, err := er.ResolveStuck(ctx, itemID, reason); err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] %s ResolveStuck(%s) item=%s: %v", caller, reason, itemID, err)
	}
}

// resolveBouncingAndCapExhausted resolves both domain.StuckReasonBouncing and
// domain.StuckReasonBounceCapExhausted for itemID via resolveStuckLogged.
// bounce_cap_exhausted (Signal 2, plan.md Epic 1.3) can only ever coexist
// with an open bouncing row, so every site that resolves bouncing must
// resolve bounce_cap_exhausted alongside it or the marker would outlive the
// condition it describes. Centralizes reconcileBouncingItems' two identical
// resolve-pairs (merged and shipped-without-PR branches).
func (l *BacklogLifecycleListener) resolveBouncingAndCapExhausted(ctx context.Context, er *EntRepository, itemID, caller string) {
	l.resolveStuckLogged(ctx, er, itemID, domain.StuckReasonBouncing, caller)
	l.resolveStuckLogged(ctx, er, itemID, domain.StuckReasonBounceCapExhausted, caller)
}

// findOpenStuckStateFor returns the row for (itemID, reason) from rows, if present.
func findOpenStuckStateFor(rows []OpenStuckStateData, itemID string, reason domain.StuckReason) (OpenStuckStateData, bool) {
	for _, row := range rows {
		if row.ItemID == itemID && row.Reason == reason {
			return row, true
		}
	}
	return OpenStuckStateData{}, false
}

// reconcileStaleWorkSessions notifies once per item when an in_progress backlog item's
// active work session has gone longer than maxWorkSessionStaleness without progress,
// then (docs/tasks/backlog-stuck-item-auto-remediation.md Phase B) drives the
// "stale_work" reason's automated remediation for every subsequent tick the
// condition still holds. Notify-once dedup and "since when" are DB-backed
// (durable BacklogStuckState row), not an in-memory map. Best-effort:
// query/notify failures are logged, never returned.
//
// The very first tick a given item is observed stale only marks+notifies —
// the open row's NotifiedAt (nil vs already-set) distinguishes "just
// notified this tick" from "notified on a prior tick" (mirrors
// reconcileBouncingItems/reconcilePushFailedItems' architectural split
// between detection, which only ever notifies once per open row, and
// remediation, which is a repeatable action gated purely by
// RemediationDue's own backoff). remediateStaleWorkWithBackoffGate is only
// reachable from the row.NotifiedAt != nil branch below, so the 2-hour
// staleness threshold that already gated the first MarkStuck call remains
// the sole "give it a chance" window before any automated action —
// remediation only starts on the tick AFTER that first notification, the
// same lag autoReopenWithBackoffGate/retryPushFailedWithBackoffGate get for
// free from being invoked out-of-band from their reason's own MarkStuck call
// site.
// refreshWorkSessionGitActivity re-reads the real current tip commit of each
// non-terminal item's most recent work session and writes it back to that
// session's LastCommitSha/LastCommitMessage/LastCommitAt/CommitCountSinceSpawn.
//
// Before this existed, LastCommitSha was written exactly once — at spawn, with
// the worktree's pre-work base HEAD (see SetItemSessionBaseCommit) — and never
// again, so a field named "last commit" permanently held a commit the session
// had not authored. Every consumer that asked "has this session's work landed
// on main?" therefore got an unconditional yes, because a branch's own base
// commit is by construction already an ancestor of main. That is how
// closeIfSupersededByMain closed PR #342 — a real, reviewed, CI-green fix — as
// "superseded" and marked its item done (BUG-047).
//
// Deliberately reuses the sweep loop every other stuck detector already runs on
// rather than adding a poller of its own; it is registered first in that sweep
// so same-tick consumers read fresh values.
//
// Best-effort throughout: an unresolvable HEAD leaves the stored value alone
// rather than clearing it, because the last known real tip is still the most
// accurate answer available once a merged branch has been deleted. Writes are
// skipped entirely when the tip has not moved, so an idle session costs one
// HEAD read per tick and no DB write or change event.
func (l *BacklogLifecycleListener) refreshWorkSessionGitActivity(ctx context.Context) {
	items, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{
			string(BacklogStatusInProgress),
			string(BacklogStatusReview),
			string(BacklogStatusPRPending),
		},
	})
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] refreshWorkSessionGitActivity list error: %v", err)
		return
	}

	for _, item := range items {
		if item.RepoPath == "" {
			continue
		}
		sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
		if sessErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] refreshWorkSessionGitActivity ListItemSessions item=%s: %v", item.ID, sessErr)
			continue
		}
		var lastWork *ItemSessionSummary
		for i := range sessions {
			// Ascending by CreatedAt (ListItemSessions' query order) — keep
			// overwriting so this ends up holding the *most recent* work session.
			if sessions[i].Role == SessionRoleWork {
				lastWork = &sessions[i]
			}
		}
		if lastWork == nil {
			continue
		}

		head := l.resolveLatestWorkCommit(ctx, lastWork.SessionUUID, item.RepoPath)
		if head == "" || head == lastWork.LastCommitSha {
			continue
		}
		// The base commit is not a commit this session authored — recording it
		// as the latest is precisely the bug this function exists to prevent.
		if head == lastWork.BaseCommitSha {
			continue
		}

		info, infoErr := git.CommitInfo(item.RepoPath, head)
		if infoErr != nil {
			log.DebugLog().Printf("[BacklogLifecycle] refreshWorkSessionGitActivity CommitInfo item=%s sha=%s: %v", item.ID, head, infoErr)
			continue
		}

		commitCount := lastWork.CommitCountSinceSpawn
		if lastWork.BaseCommitSha != "" {
			if shipped, _, listErr := git.ListShippedCommits(ctx, item.RepoPath, lastWork.BaseCommitSha, head); listErr == nil {
				commitCount = len(shipped)
			} else {
				log.DebugLog().Printf("[BacklogLifecycle] refreshWorkSessionGitActivity ListShippedCommits item=%s: %v", item.ID, listErr)
			}
		}

		if updErr := l.storage.UpdateItemSessionGitActivity(ctx, lastWork.ID, head, info.Summary, info.AuthorAt, commitCount); updErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] refreshWorkSessionGitActivity update item=%s session=%s: %v", item.ID, lastWork.SessionUUID, updErr)
			continue
		}
		log.DebugLog().Printf("[BacklogLifecycle] refreshWorkSessionGitActivity item=%s session=%s: last commit %s → %s (%d since base)",
			item.ID, lastWork.SessionUUID, lastWork.LastCommitSha, head, commitCount)
	}
}

// planApprovalStaleness is how long a queued item may sit blocked by
// DequeueNextQueuedItems' planning gate before this detector flags it — a
// short buffer (not the multi-hour/day thresholds used elsewhere in this
// file) since, unlike a running session, there is no legitimate "still
// working" explanation for this condition: it is a pure configuration gap
// that will never self-resolve without a human action.
const planApprovalStaleness = 5 * time.Minute

// reconcilePlanNotApprovedItems flags queued items DequeueNextQueuedItems'
// planning gate (SkipPlanning=false, PlanApproved=false) has refused to claim
// — by that function's own design, this happens silently forever: the gate
// logs one WARNING per 60s tick and leaves the item queued, with no durable,
// human-visible signal and (as of this detector's introduction) no "Approve
// Plan" UI action reachable for items using the default pipeline. Confirmed
// live 2026-07-22: three items (including the one this fix was written
// against) sat queued for days this way, invisible on the kanban board
// (BUG-037, fixed alongside this) and structurally un-unblockable by a user.
// Detection + notification only, mirroring reconcileOrphanedTriageItems —
// resolving *how* an item should get its plan approved (auto-approve for
// items with prior completed work sessions? build the missing UI action? is
// the gate even correct for the "default" pipeline mode?) is a product/
// architecture question out of scope for this fix; see BUG-038.
func (l *BacklogLifecycleListener) reconcilePlanNotApprovedItems(ctx context.Context, er *EntRepository) {
	items, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{string(BacklogStatusQueued)},
	})
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] reconcilePlanNotApprovedItems list error: %v", err)
		return
	}

	for _, item := range items {
		if item.SkipPlanning || item.PlanApproved {
			continue
		}
		if item.QueuedAt == nil || time.Since(*item.QueuedAt) <= planApprovalStaleness {
			continue // still plausibly about to be approved/dequeued
		}

		// An item whose most recent triage session never left a usable plan
		// behind isn't "awaiting human review of a real plan" — it's the
		// generalized orphaned-triage shape reconcileOrphanedTriageItems now
		// also covers (docs/tasks/backlog-feature-improvement.md's 2026-08-03
		// entry, item be676dab). Defer to that detector instead of flagging the
		// same item under two differently-worded stuck reasons at once.
		sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
		if sessErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcilePlanNotApprovedItems ListItemSessions item=%s: %v", item.ID, sessErr)
			// Fail open (still flag as plan-not-approved below) — losing session
			// visibility for one tick shouldn't suppress the pre-existing signal.
		} else if latest := latestTriageSession(sessions); latest != nil && latest.EndedAt != nil && latest.TriageResult == "" {
			continue
		}

		applied, markErr := er.MarkStuck(ctx, item.ID, domain.StuckReasonPlanNotApproved, BacklogStatusQueued,
			"queued item blocked by DequeueNextQueuedItems' planning gate (plan not approved, skip_planning not set)")
		if markErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcilePlanNotApprovedItems MarkStuck item=%s: %v", item.ID, markErr)
			continue
		}
		if !applied {
			continue
		}
		rows, findErr := er.FindOpenStuckStates(ctx)
		if findErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcilePlanNotApprovedItems FindOpenStuckStates item=%s: %v", item.ID, findErr)
			continue
		}
		row, ok := findOpenStuckStateFor(rows, item.ID, domain.StuckReasonPlanNotApproved)
		if !ok || row.NotifiedAt != nil {
			continue
		}

		log.WarningLog().Printf("[BacklogLifecycle] item %s queued but blocked by unapproved plan", item.ID)
		l.notify(item.ID,
			"Queued item blocked by unapproved plan",
			fmt.Sprintf("%s — this item cannot be dequeued until its plan is approved (or skip_planning is set). Approve the plan or update the item to unblock it.", item.Title),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
		)
		if _, notifyErr := er.MarkStuckNotified(ctx, item.ID, domain.StuckReasonPlanNotApproved); notifyErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcilePlanNotApprovedItems MarkStuckNotified item=%s: %v", item.ID, notifyErr)
		}
	}
	// No resolve pass needed here: selfHealStuck (status-anchored) clears this
	// reason once the item leaves 'queued' (dequeued, manually reopened, etc.).
}

// resolveLatestWorkCommit returns the true current tip commit of the work
// session identified by sessionUUID — never ItemSessionSummary.LastCommitSha,
// which is only ever seeded once at session spawn with the pre-work base SHA
// (see the UpdateItemSessionGitActivity calls in
// backlog_service_triage.go/backlog_service_sync.go, all of which pass
// baseSHA) and is never updated afterward as the agent commits real work.
// Treating that field as "the agent's latest commit" made
// mostRecentWorkCommitShippedToMain trivially true for almost any PR-less
// item, because a branch's own base commit is — by construction — always an
// ancestor of main. Confirmed live 2026-07-21: items 635a373d, e99d3f4a, and
// 54e5aa1f were all auto-marked done in a single reconciliation tick despite
// each having real, unmerged work (an open PR for 635a373d, and unpushed
// branches for the other two).
//
// Prefers the session's own worktree HEAD; falls back to resolving the
// branch's tip directly in repoPath if the worktree directory is gone, since
// worktrees of the same repo share one object store — the same fallback
// shape getWorkSessionDiff/GetGitDiffRef already rely on for the review-diff
// path. Returns "" if neither resolves.
//
// Before trusting the worktree HEAD, confirms the directory still has
// wt.BranchName checked out. Worktree paths are recycled across sessions
// once a session ends, so a directory existing at wt.WorktreePath does not
// mean it still holds *this* session's branch. Confirmed live 2026-08-12:
// item 0f5d760b's ended work session still pointed at a worktree path that
// had since been reassigned to a later item's branch (0f127033's, then
// a3ca3918's — same shape recurred across items), so its HEAD resolved to
// that other item's legitimately-merged commit instead of "no commits",
// falsely marking 0f5d760b (and the others) shipped. A branch mismatch falls
// through to the branch-name lookup below, same as the worktree-gone case.
func (l *BacklogLifecycleListener) resolveLatestWorkCommit(ctx context.Context, sessionUUID, repoPath string) string {
	wt, err := l.storage.GetWorktreeDataBySessionUUID(ctx, sessionUUID)
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] resolveLatestWorkCommit: no worktree data for session %s: %v", sessionUUID, err)
		return ""
	}
	if wt.WorktreePath != "" {
		if info, statErr := os.Stat(wt.WorktreePath); statErr == nil && info.IsDir() {
			branch, branchErr := git.GetCurrentBranchName(wt.WorktreePath)
			if branchErr == nil && (wt.BranchName == "" || branch == wt.BranchName) {
				sha, headErr := GetGitHeadSHA(wt.WorktreePath)
				if headErr == nil && sha != "" {
					return sha
				}
			} else if branchErr == nil {
				log.WarningLog().Printf("[BacklogLifecycle] resolveLatestWorkCommit: worktree path %s now holds branch %q, not session's %q (path recycled?) — falling back to repo-wide branch lookup", wt.WorktreePath, branch, wt.BranchName)
			}
		}
	}
	if wt.BranchName == "" {
		return ""
	}
	cmd := safeexec.CommandContext(ctx, "git", "rev-parse", "--verify", wt.BranchName)
	cmd.Dir = repoPath
	out, revErr := cmd.Output()
	if revErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] resolveLatestWorkCommit: rev-parse %s in %s: %v", wt.BranchName, repoPath, revErr)
		return ""
	}
	return strings.TrimSpace(string(out))
}

// transitionBouncingItemToDone moves an item reconcileBouncingItems has
// externally verified as converged (its PR merged, or its commit shipped to
// main) to done. It does this via the state machine's own legal edges —
// recording a genuine PASS verdict via recordTerminalReviewVerdict (documenting
// the external verification as the justification), then in_progress->review
// (only if the item isn't already at review) followed by review->done —
// rather than calling the raw storage-layer TransitionBacklogItemStatus
// directly from item.Status straight to done. That direct hop is not even a
// legal edge in validTransitions when item.Status is in_progress, and more
// importantly it bypasses TransitionGuard's ErrVerdictRequired gate entirely,
// since the raw storage layer has no knowledge of WorkflowEngine/TransitionGuard
// (see session/domain/backlog.go's validTransitions and TransitionGuard, and
// the guarded RPC path in server/services/backlog_service_lifecycle.go that
// this internal caller was bypassing).
func (l *BacklogLifecycleListener) transitionBouncingItemToDone(ctx context.Context, item BacklogItemData, verdictSummary string) error {
	if _, err := recordTerminalReviewVerdict(l.storage, item.ID, item.AcceptanceCriteria, "bounce-reconcile-"+uuid.New().String(), ReviewVerdictPass, verdictSummary); err != nil {
		return fmt.Errorf("record PASS verdict: %w", err)
	}

	status := item.Status
	if status == string(BacklogStatusInProgress) {
		precondition := &BacklogItemPrecondition{ExpectedStatus: status}
		if _, err := l.storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusReview, precondition, TriggeredBySystem); err != nil {
			return fmt.Errorf("in_progress->review: %w", err)
		}
		status = string(BacklogStatusReview)
	}

	precondition := &BacklogItemPrecondition{ExpectedStatus: status}
	if _, err := l.storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, precondition, TriggeredBySystem); err != nil {
		return fmt.Errorf("review->done: %w", err)
	}
	return nil
}

// reconcileBouncingItems flags items that have crossed in_progress->review
// >= bounceThreshold times within bounceLookback with no recorded PASS
// verdict — a non-converging rework cycle that never hits the rework cap
// (root cause #4). Before flagging, it first checks whether the item's
// linked PR has already merged (including a manual merge outside the app's
// own ship flow) — a merged item isn't bouncing, it's done, and is
// transitioned to done instead of being marked stuck. For an item with no PR
// number at all (real work committed and merged/pushed straight to main
// without ever going through a PR — see item 93565fa1, 2026-07-21), it falls
// back to checking the item's own most recent work-session commit directly
// against main via mostRecentWorkCommitShippedToMain, so an item isn't left
// bouncing forever just because it never had a PR to check. Best-effort:
// query/notify failures are logged, never returned.
func (l *BacklogLifecycleListener) reconcileBouncingItems(ctx context.Context, er *EntRepository) {
	// Scan items in the two statuses a bouncing cycle spans; a converged item
	// (done) is handled by the self-heal sweep, not re-flagged here.
	items, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{string(BacklogStatusInProgress), string(BacklogStatusReview)},
	})
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] reconcileBouncingItems list error: %v", err)
		return
	}

	for _, item := range items {
		// Resolve this item's Shape-C liveness definition (CycleThreshold/CycleLookback)
		// per item, inside the loop — a per-mode override means these can legitimately
		// differ between items in the same tick (Epic 1.4, Story 1.4.3). Keyed to
		// BacklogStatusReview, NOT BacklogStatusInProgress: DefaultLivenessEngine's table
		// (Epic 1.2) already occupies BacklogStatusInProgress with the Shape-B stale-work
		// definition, and a LivenessDefinition is a tagged union with exactly one Kind per
		// stage — see this file's Epic 1.4 plan-correction note in
		// project_plans/backlog-custom-workflow-stages/implementation/plan.md's Story 1.4.3.
		cycleThreshold := bounceThreshold
		cycleLookback := bounceLookback
		if l.livenessEngine != nil {
			if def, defErr := l.livenessEngine.LivenessFor(BacklogStatusReview, PipelineMode(item.PipelineMode)); defErr == nil && !def.IsNoTimeout() {
				cycleThreshold = def.CycleThreshold
				cycleLookback = def.CycleLookback
			}
		}
		since := time.Now().Add(-cycleLookback)

		// Before treating this item as failing, check whether its linked PR
		// already merged — including a PR merged manually, outside the app's
		// own ship flow (allow_auto_merge is disabled at the repo-settings
		// level). Without this check, an item whose code already landed on
		// main keeps bouncing and accumulates further remediation attempts
		// (worktrees, sessions, tokens) on work that's already done. Reuses
		// the same prPendingChecker/TransitionBacklogItemStatus path
		// ReconcilePRPending already uses for its own merge->done transition,
		// rather than inventing a new one.
		if item.PrNumber > 0 && item.RepoPath != "" {
			checker := l.getPRPendingCheckerFactory()(item.RepoPath)
			merged, mergedErr := checker.IsPRMerged(item.PrNumber)
			if mergedErr != nil {
				log.DebugLog().Printf("[BacklogLifecycle] reconcileBouncingItems IsPRMerged item=%s pr=%d: %v", item.ID, item.PrNumber, mergedErr)
			} else if merged {
				// Story 6 guard (adversarial-review.md's Blocker): re-verify,
				// via a live GitHub lookup, that PR #item.PrNumber's head
				// branch still matches this item's currently-tracked branch
				// before auto-completing it on the strength of item.PrNumber
				// alone. Fails closed identically to
				// verifyPRAssociationForFixSpawn's own contract.
				if !l.verifyPRAssociationForFixSpawn(ctx, item.ID, item.RepoPath, item.PrNumber) {
					log.WarningLog().Printf("[BacklogLifecycle] reconcileBouncingItems item=%s: PR #%d head branch no longer verifiably matches the tracked branch — skipping auto-done transition (was this item's PR attached via report_pr_created's override_reason path?)", item.ID, item.PrNumber)
					continue
				}
				summary := fmt.Sprintf("Auto-verified by reconcileBouncingItems: PR #%d for this item is confirmed merged on GitHub, so the item's rework cycle is treated as converged rather than bouncing.", item.PrNumber)
				if transErr := l.transitionBouncingItemToDone(ctx, item, summary); transErr != nil {
					log.WarningLog().Printf("[BacklogLifecycle] reconcileBouncingItems done transition item=%s: %v", item.ID, transErr)
					// The PR is already confirmed merged — the item is left
					// bouncing between in_progress/review with nothing else
					// surfacing this until the next tick retries it.
					l.notifyTransitionFailed(item.ID, item.Title, fmt.Sprintf("PR #%d was confirmed merged but the item's transition to done failed", item.PrNumber), transErr)
				} else {
					log.InfoLog().Printf("[BacklogLifecycle] reconcileBouncingItems item=%s → done (PR #%d already merged)", item.ID, item.PrNumber)
					// Best-effort: clear any bouncing (+ bounce_cap_exhausted,
					// Signal 2) row from a prior tick immediately, rather than
					// waiting for the next selfHealStuck sweep to notice the
					// terminal status.
					l.resolveBouncingAndCapExhausted(ctx, er, item.ID, "reconcileBouncingItems/merged")
				}
				continue
			}
		} else if item.RepoPath != "" {
			// No PR was ever linked to this item — check whether the item's own
			// most recent work-session commit landed on main anyway (a direct
			// merge/push to main outside the app's ship flow entirely, so
			// item.PrNumber was never set). Only that specific commit is
			// checked, never an arbitrary one, so an unrelated commit merged to
			// main elsewhere can't produce a false positive.
			if sha, shipped := l.mostRecentWorkCommitShippedToMain(ctx, item.ID, item.RepoPath); shipped {
				summary := fmt.Sprintf("Auto-verified by reconcileBouncingItems: this item's most recent work-session commit (%s) is confirmed shipped to %s without ever going through a PR, so the item's rework cycle is treated as converged rather than bouncing.", sha, bounceMainBranch)
				if transErr := l.transitionBouncingItemToDone(ctx, item, summary); transErr != nil {
					log.WarningLog().Printf("[BacklogLifecycle] reconcileBouncingItems done transition (shipped without PR) item=%s: %v", item.ID, transErr)
					// The commit is already confirmed shipped to main — same
					// silent-stranding risk as the merged-PR branch above.
					l.notifyTransitionFailed(item.ID, item.Title, fmt.Sprintf("commit %s was confirmed shipped to %s but the item's transition to done failed", sha, bounceMainBranch), transErr)
				} else {
					log.InfoLog().Printf("[BacklogLifecycle] reconcileBouncingItems item=%s → done (commit %s shipped to %s without a PR)", item.ID, sha, bounceMainBranch)
					// Best-effort: clear any bouncing (+ bounce_cap_exhausted,
					// Signal 2) row from a prior tick — see the identical
					// comment at the merged-PR branch above.
					l.resolveBouncingAndCapExhausted(ctx, er, item.ID, "reconcileBouncingItems/shipped-no-pr")
				}
				continue
			}
		}

		count, countErr := er.CountReviewCyclesSince(ctx, item.ID, since)
		if countErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcileBouncingItems CountReviewCyclesSince item=%s: %v", item.ID, countErr)
			continue
		}
		// Fetch the full most-recent verdict (outcome + reviewer summary), not
		// just the outcome, so a bouncing item's stuck-state context can
		// surface *why* the last attempt failed instead of only that it did
		// (BUG-060, the same discard-after-fetch shape BUG-059 fixed for
		// orphaned_triage's EndReason). Across a multi-cycle bounce there may
		// be several different verdicts; only the single most recent one is
		// surfaced here — proportional to a diagnostic string, not a full
		// verdict history. GetRecentReviewVerdictSummaries runs the identical
		// "most recent ItemSession with a verdict" query
		// GetMostRecentReviewVerdictForItem uses, so limit 1 returns the same
		// verdict either would.
		recentVerdicts, verdictErr := er.GetRecentReviewVerdictSummaries(ctx, item.ID, 1)
		if verdictErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcileBouncingItems GetRecentReviewVerdictSummaries item=%s: %v", item.ID, verdictErr)
		}
		var latestOutcome, latestSummary string
		if len(recentVerdicts) > 0 {
			latestOutcome = recentVerdicts[0].OverallOutcome
			latestSummary = recentVerdicts[0].Summary
		}
		hasPass := latestOutcome == string(ReviewOutcomePass)

		if !isBouncing(count, cycleThreshold, hasPass) {
			continue
		}

		reasonDetail := fmt.Sprintf("bounced in_progress<->review %d times in the last %s with no PASS verdict", count, cycleLookback)
		if latestOutcome != "" {
			// sanitizeField at 500 matches the existing convention for
			// rendering a ReviewVerdict.Summary into operator/agent-facing
			// text (see backlog_context.go, backlog_review.go).
			reasonDetail = fmt.Sprintf("%s (most recent verdict: %s — %s)", reasonDetail, latestOutcome, sanitizeField(latestSummary, 500))
		}

		applied, markErr := er.MarkStuck(ctx, item.ID, domain.StuckReasonBouncing, BacklogStatus(item.Status), reasonDetail)
		if markErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcileBouncingItems MarkStuck item=%s: %v", item.ID, markErr)
			continue
		}
		if !applied {
			continue
		}
		rows, findErr := er.FindOpenStuckStates(ctx)
		if findErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcileBouncingItems FindOpenStuckStates item=%s: %v", item.ID, findErr)
			continue
		}
		row, ok := findOpenStuckStateFor(rows, item.ID, domain.StuckReasonBouncing)
		if !ok || row.NotifiedAt != nil {
			continue
		}
		log.WarningLog().Printf("[BacklogLifecycle] item %s bouncing (%d cycles in %s, no PASS)", item.ID, count, cycleLookback)
		notifyBody := fmt.Sprintf("%s — bounced between in_progress and review %d times in the last %s with no PASS verdict. It may be stuck in a non-converging rework loop.", item.Title, count, cycleLookback)
		if latestOutcome != "" {
			notifyBody = fmt.Sprintf("%s Most recent verdict: %s — %s", notifyBody, latestOutcome, sanitizeField(latestSummary, 500))
		}
		l.notify(item.ID,
			"Item is thrashing between work and review",
			notifyBody,
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
		)
		if _, notifyErr := er.MarkStuckNotified(ctx, item.ID, domain.StuckReasonBouncing); notifyErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcileBouncingItems MarkStuckNotified item=%s: %v", item.ID, notifyErr)
		}
	}
}

// selfHealStuck resolves any open BacklogStuckState row whose reason's
// expected item-status no longer matches the item's current status (Task
// 2.1.5d, adversarial concern C1). This backstops racing MarkStuck writes
// (the best-effort precondition in MarkStuck is not atomic with the write)
// and any un-stick call site that was missed.
//
// The sweep applies ONE blanket rule up front, before any reason-specific
// logic: an open stuck row on an item that has reached a genuine terminal
// status (done or archived) is always resolved, regardless of which reason
// it is for. An item that has truly finished has nothing left needing
// operator attention, no matter what it was ever stuck for — so this check
// runs first and short-circuits the rest of the loop body for that row.
// This is what closes the recurring bug shape fixed one reason at a time in
// PR #200 (autonomous_stuck) and PR #203 (push_failed): rather than adding a
// fourth near-identical case for the next reason that turns out to need it
// (rework_cap, fixed here), any reason gets this behavior for free, forever,
// with no further PRs required.
//
// For a row on an item that has NOT yet reached a terminal status, the sweep
// falls through to the per-reason anchor-set table below — most reasons
// anchor on a single non-terminal status and resolve as soon as the item
// LEAVES it:
//
//	pr_ready_unmerged  -> anchor {pr_pending}          resolve when status not in anchor
//	abandoned_review   -> anchor {review}               resolve when status not in anchor
//	stale_work         -> anchor {in_progress}          resolve when status not in anchor
//	bouncing           -> anchor {in_progress, review}  resolve ONLY on leaving both (never mid-cycle)
//	orphaned_triage    -> anchor {idea}                 resolve when status not in anchor
//
// autonomous_stuck, push_failed, and rework_cap have no non-terminal anchor
// at all — before the item reaches done/archived they stay open, relying
// entirely on the blanket terminal rule above (or, for autonomous_stuck and
// push_failed, their own faster event-driven resolution paths — see
// resolveAutonomousStuck in server/services/autonomous_orchestration_service.go
// and resolveToPRPending respectively — which typically resolve the row
// before this sweep's next tick even runs). Any future StuckReason added
// without an explicit case here behaves the same way: excluded from
// non-terminal anchoring, covered by the blanket rule once the item finishes.
//
// Why the blanket terminal rule is safe for every reason, not just the ones
// it was originally proven safe for: the concern that motivated excluding
// autonomous_stuck from anchoring in the first place (documented in the
// original PR #200 investigation) was specifically an "in_progress" false-resolve
// risk — pre-PR #180, onAutonomousDriverComplete's SessionRoleWork case could
// force an in_progress->review transition on a turn-cap stop even while the
// item was genuinely still stuck, which would have made a naive {in_progress}
// anchor resolve the row before an operator ever saw it. That risk is
// specific to anchoring on a NON-terminal, mid-cycle status simply being
// "left" (in_progress, or push_failed's "review"), which a routine, expected
// state transition can trigger while real work is still incomplete.
// done/archived are not reachable that way: nothing in this codebase
// transitions an item to done or archived as a side effect of a retry,
// respawn, or turn-cap stop — reaching either status is only ever the result
// of the item's work being genuinely, verifiably finished (a successful
// pipeline run, a human merging the PR directly, or the auto-archive sweep
// acting on an item already done). This holds for every existing reason
// (autonomous_stuck, push_failed) and for rework_cap: a rework_cap row is
// marked when an item exhausts its retry budget, but nothing about hitting
// that cap can itself force the item to done/archived — those transitions
// only happen through the same genuinely-finished paths as every other
// reason, so the same safety argument PR #200 made for autonomous_stuck and
// PR #203 re-derived for push_failed applies unconditionally to rework_cap
// and to any future reason: this settles the question for the blanket rule
// as a whole, not per-reason, and no future reason should need to re-derive
// it again.
//
// Same-status clears (e.g. a pr_pending item whose PR stops being ready
// while it's still pr_pending) are NOT this sweep's job — they are handled
// by each detector's own poll-shaped else-branch (pre-mortem F2), since the
// sweep structurally cannot observe a same-status transition.
func (l *BacklogLifecycleListener) selfHealStuck(ctx context.Context, er *EntRepository) {
	open, err := er.FindOpenStuckStates(ctx)
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] selfHealStuck FindOpenStuckStates error: %v", err)
		return
	}
	for _, row := range open {
		if IsTerminalStatus(row.ItemStatus) {
			// Blanket terminal rule — see doc comment above. An item that has
			// truly finished has nothing left needing operator attention,
			// regardless of which reason its stuck row is for.
			l.resolveStuckLogged(ctx, er, row.ItemID, row.Reason, "selfHealStuck/terminal")
			continue
		}
		resolve := false
		switch row.Reason {
		case domain.StuckReasonPRReadyUnmerged:
			resolve = row.ItemStatus != BacklogStatusPRPending
		case domain.StuckReasonAbandonedReview:
			resolve = row.ItemStatus != BacklogStatusReview
		case domain.StuckReasonStaleWork:
			resolve = row.ItemStatus != BacklogStatusInProgress
		case domain.StuckReasonBouncing, domain.StuckReasonBounceCapExhausted:
			// bounce_cap_exhausted (Signal 2, plan.md Epic 1.3) can only ever
			// coexist with an open bouncing row, so it shares bouncing's exact
			// non-terminal anchor — this is a backstop for any status
			// transition that bypasses reconcileBouncingItems' explicit
			// resolve-alongside-bouncing call sites above.
			resolve = row.ItemStatus != BacklogStatusInProgress && row.ItemStatus != BacklogStatusReview
		case domain.StuckReasonOrphanedTriage:
			// Generalized 2026-08-03 to also anchor at queued (see
			// reconcileOrphanedTriageItems' doc comment): a retry that
			// successfully re-triages moves the item queued->idea->ready,
			// landing outside both anchor statuses, so this still resolves
			// correctly on genuine success. A retry still in flight (queued
			// reset to idea) or a repeat failure (back to idea) keeps the row
			// open, matching the pre-existing idea-only behavior.
			resolve = row.ItemStatus != BacklogStatusIdea && row.ItemStatus != BacklogStatusQueued
		case domain.StuckReasonPlanNotApproved:
			resolve = row.ItemStatus != BacklogStatusQueued
		case domain.StuckReasonPRPendingNoPR:
			resolve = row.ItemStatus != BacklogStatusPRPending
		case domain.StuckReasonPRNeedsFix:
			resolve = row.ItemStatus != BacklogStatusPRPending
		default:
			// autonomous_stuck, push_failed, rework_cap, multiple_reasons, and
			// any future reason with no non-terminal anchor: stays open until
			// the blanket terminal rule above catches it, or its own
			// event-site resolves it first. multiple_reasons specifically is
			// resolved by its own detector (reconcileMultiReasonEscalation's
			// de-escalate branch), not a status-anchor case here — its
			// "resolved" condition is "count of other open reasons dropped
			// below multiReasonThreshold", which has no single item-status
			// anchor to check.
			continue
		}
		if !resolve {
			continue
		}
		l.resolveStuckLogged(ctx, er, row.ItemID, row.Reason, "selfHealStuck")
	}
}

// reconcileMultiReasonEscalation groups every open BacklogStuckState row by
// item and, for any item with multiReasonThreshold or more simultaneously
// open *non-escalation* stuck reasons, marks/refreshes a durable
// domain.StuckReasonMultipleReasons row (Signal 1 — see plan.md Epic 1.2).
// Computed fresh from FindOpenStuckStates every tick (never a cached count,
// per research/pitfalls.md §2), so the signal can never drift from the live
// set of open reasons. Registered immediately after self_heal in
// ReconcileStuck so a terminal-status item's stale rows have already been
// cleared this tick before they're counted.
//
// Two exclusions apply before counting (see ADR-001 and Task 1.2.2a):
//  1. domain.StuckReasonMultipleReasons and domain.StuckReasonBounceCapExhausted
//     themselves never count toward their own trigger — otherwise the
//     escalation row would be self-reinforcing.
//  2. domain.StuckReasonAbandonedReview is excluded when the same item also
//     has an open domain.StuckReasonBouncing row whose remediation gate is
//     currently blocked (parked or mid-backoff) — abandoned_review and
//     bouncing are structurally coupled in that state, not two independent
//     signals: markAbandonedReview (backlog_lifecycle_review.go) already
//     skips its own respawn for exactly this condition
//     (TestMarkAbandonedReview_SkipsRespawn_WhenBouncingGateNotDue). Without
//     this exclusion, bouncing+abandoned_review would co-occur on nearly
//     every bouncing item, degrading "2 simultaneous reasons" from a
//     distinguishing signal to "most bouncing items" (pre-mortem.md
//     Failure #1, P1).
//
// Notification is dwell-gated and notify-once per row lifetime
// (multiReasonEscalationNotifyReady, keyed off the row's FirstDetectedAt) so
// a single-tick threshold crossing doesn't notify immediately. De-escalation
// (ResolveStuck) is NOT dwell-gated — it fires the same tick the count first
// drops below threshold (see plan.md Pattern Decisions' "Flap control"
// rows and Unresolved Questions for why no hysteresis is applied here yet).
// Best-effort throughout: errors are logged, never returned.
func (l *BacklogLifecycleListener) reconcileMultiReasonEscalation(ctx context.Context, er *EntRepository) {
	open, err := er.FindOpenStuckStates(ctx)
	if err != nil {
		log.WarningLog().Printf("[BacklogLifecycle] reconcileMultiReasonEscalation FindOpenStuckStates error: %v", err)
		return
	}

	byItem := make(map[string][]OpenStuckStateData)
	for _, row := range open {
		byItem[row.ItemID] = append(byItem[row.ItemID], row)
	}

	for itemID, rows := range byItem {
		l.reconcileMultiReasonEscalationForItem(ctx, er, itemID, rows)
	}
}

// multiReasonRowSet is the per-item categorization of open stuck-state rows
// that reconcileMultiReasonEscalation needs to decide whether the
// multi-reason escalation should be raised, cleared, or left alone.
type multiReasonRowSet struct {
	nonEscalation      []OpenStuckStateData
	bouncingRow        OpenStuckStateData
	hasBouncing        bool
	hasAbandonedReview bool
}

// categorizeOpenStuckRows partitions an item's open stuck-state rows into
// the ones eligible to count toward multi-reason escalation (excluding
// StuckReasonMultipleReasons and StuckReasonBounceCapExhausted, which are
// derived signals rather than independent reasons) and tracks whether a
// bouncing/abandoned-review pair is present for the structural-coupling
// exclusion below.
func categorizeOpenStuckRows(rows []OpenStuckStateData) multiReasonRowSet {
	var set multiReasonRowSet
	for _, row := range rows {
		if row.Reason == domain.StuckReasonMultipleReasons || row.Reason == domain.StuckReasonBounceCapExhausted {
			continue
		}
		if row.Reason == domain.StuckReasonBouncing {
			set.hasBouncing = true
			set.bouncingRow = row
		}
		if row.Reason == domain.StuckReasonAbandonedReview {
			set.hasAbandonedReview = true
		}
		set.nonEscalation = append(set.nonEscalation, row)
	}
	return set
}

// excludeStructurallyCoupledAbandonedReview applies the ADR-001 exclusion
// (plan.md Task 1.2.2a): a bouncing item's abandoned-review row is expected
// structural coupling, not an independent signal, while remediation for the
// bouncing reason is still blocked (parked or mid-backoff). Evaluated
// in-process from set.bouncingRow (already fetched via FindOpenStuckStates)
// rather than via l.storage.RemediationBlocked, which would re-query every
// open stuck row across the whole system again just to look up the one row
// already in hand. Mirrors RemediationBlocked's own decision set
// (session/backlog_remediation.go): blocked iff the gate is parked or
// mid-backoff, not eligible/granted.
func excludeStructurallyCoupledAbandonedReview(set multiReasonRowSet) []OpenStuckStateData {
	if !set.hasBouncing || !set.hasAbandonedReview {
		return set.nonEscalation
	}
	switch evaluateRemediation(set.bouncingRow, time.Now(), serverStartTime) {
	case remediationSkippedParked, remediationSkippedNotDue:
		filtered := make([]OpenStuckStateData, 0, len(set.nonEscalation))
		for _, row := range set.nonEscalation {
			if row.Reason != domain.StuckReasonAbandonedReview {
				filtered = append(filtered, row)
			}
		}
		return filtered
	}
	return set.nonEscalation
}

// deescalateMultiReasonIfNeeded resolves an item's open StuckReasonMultipleReasons
// row once fewer than the escalation threshold of independent reasons remain
// open. It reports whether de-escalation was the outcome (true) so the
// caller can stop processing the item, or whether escalation should still be
// evaluated (false).
func (l *BacklogLifecycleListener) deescalateMultiReasonIfNeeded(ctx context.Context, er *EntRepository, itemID string, hasExistingRow bool, openReasonsCount int) bool {
	if isMultiReasonEscalated(openReasonsCount) {
		return false
	}
	if hasExistingRow {
		if _, resolveErr := er.ResolveStuck(ctx, itemID, domain.StuckReasonMultipleReasons); resolveErr != nil {
			log.WarningLog().Printf("[BacklogLifecycle] reconcileMultiReasonEscalation ResolveStuck item=%s: %v", itemID, resolveErr)
		} else {
			log.InfoLog().Printf("[BacklogLifecycle] de-escalated item=%s open_reasons=%d", itemID, openReasonsCount)
		}
	}
	return true
}

// notifyMultiReasonEscalationIfReady sends the multi-reason-escalation
// notification once, using notify-readiness derived from the pre-MarkStuck
// row (if one was already open this tick): MarkStuck does not change
// FirstDetectedAt or NotifiedAt for a row that was already open (only for
// one it reopens from a resolved state), so the pre-fetched values are still
// accurate post-MarkStuck. A freshly-created row (no existingRow) was just
// opened this tick, so it is never notify-ready yet.
func (l *BacklogLifecycleListener) notifyMultiReasonEscalationIfReady(ctx context.Context, er *EntRepository, itemID, itemTitle, contextString string, nonEscalationCount int, existingRow OpenStuckStateData, hasExistingRow bool) {
	if !hasExistingRow {
		return
	}
	if existingRow.NotifiedAt != nil {
		return
	}
	if !multiReasonEscalationNotifyReady(existingRow.FirstDetectedAt, time.Now()) {
		return
	}
	l.notify(itemID,
		"Multiple stuck reasons open",
		fmt.Sprintf("%s — %d stuck reasons currently open simultaneously (%s). This combination is a stronger signal than any single reason alone.", itemTitle, nonEscalationCount, contextString),
		7, // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
		4, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT
	)
	if _, notifyErr := er.MarkStuckNotified(ctx, itemID, domain.StuckReasonMultipleReasons); notifyErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] reconcileMultiReasonEscalation MarkStuckNotified item=%s: %v", itemID, notifyErr)
	}
}

// reconcileMultiReasonEscalationForItem applies the multi-reason escalation
// decision (de-escalate, leave alone, or escalate + notify) for a single
// item's open stuck-state rows. Split out of reconcileMultiReasonEscalation
// to keep the per-item decision tree — categorize, exclude structurally
// coupled rows, de-escalate-or-escalate, notify — independently readable and
// under the complexity gate.
func (l *BacklogLifecycleListener) reconcileMultiReasonEscalationForItem(ctx context.Context, er *EntRepository, itemID string, rows []OpenStuckStateData) {
	set := categorizeOpenStuckRows(rows)
	nonEscalation := excludeStructurallyCoupledAbandonedReview(set)

	existingRow, hasExistingRow := findOpenStuckStateFor(rows, itemID, domain.StuckReasonMultipleReasons)

	if l.deescalateMultiReasonIfNeeded(ctx, er, itemID, hasExistingRow, len(nonEscalation)) {
		return
	}

	reasonLabels := make([]string, 0, len(nonEscalation))
	for _, row := range nonEscalation {
		reasonLabels = append(reasonLabels, string(row.Reason))
	}
	contextString := strings.Join(reasonLabels, ", ")

	applied, markErr := er.MarkStuck(ctx, itemID, domain.StuckReasonMultipleReasons, rows[0].ItemStatus, contextString)
	if markErr != nil {
		log.WarningLog().Printf("[BacklogLifecycle] reconcileMultiReasonEscalation MarkStuck item=%s: %v", itemID, markErr)
		return
	}
	if !applied {
		return
	}
	log.InfoLog().Printf("[BacklogLifecycle] escalated item=%s open_reasons=%d", itemID, len(nonEscalation))

	l.notifyMultiReasonEscalationIfReady(ctx, er, itemID, rows[0].ItemTitle, contextString, len(nonEscalation), existingRow, hasExistingRow)
}

// hasActiveSession reports whether any of the provided ItemSessions is an
// open (not yet ended) work-, review-, or Jules-role session. Package-local
// equivalent of server/services' hasActiveWorkSession/hasActiveReviewSession
// (not reusable directly — that package imports session, not the other way
// around) used by recoverDriftedPRItem/reconcileDriftedPRItems to avoid
// stealing an item away from a still-legitimately-running session, mirroring
// AutoReopenForPRFix's/AutoRespawnReview's identical guard.
func hasActiveSession(sessions []ItemSessionSummary) bool {
	for _, s := range sessions {
		if s.EndedAt == nil && (s.Role == SessionRoleWork || s.Role == SessionRoleReview) {
			return true
		}
	}
	return HasActiveJulesSession(sessions)
}
