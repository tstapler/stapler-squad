package backlog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/backlog/chains"
	"github.com/tstapler/stapler-squad/session/backlog/liveness"
	"github.com/tstapler/stapler-squad/session/backlog/pipeline"
	"github.com/tstapler/stapler-squad/session/backlog/reviewgate"
	"github.com/tstapler/stapler-squad/session/backlog/stuck"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/hostclaim"
)

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
	storage *session.Storage

	// triageParks tracks recent orphaned-triage retry-cap parkings so a burst can
	// escalate as one batch notification; see recordTriageParkAndMaybeEscalate.
	triageParks parkBurstTracker

	// sessionCreatorMu guards sessionCreator for concurrent Set/get access, same
	// pattern as autoReopener/notifier/prFixSpawner below. Needed because
	// production wiring (server/dependencies.go) constructs this listener before
	// SessionService exists, so the spawner is wired post-construction via
	// SetSessionCreator.
	sessionCreatorMu sync.RWMutex
	sessionCreator   reviewgate.ReviewGateSpawner

	// poolMu guards headlessPool for concurrent Set/get access.
	poolMu       sync.RWMutex
	headlessPool *headless.Pool
	// headlessClient, when set, takes precedence over headlessPool for gate and PR-draft
	// calls so they honor the LLM backend selector. Guarded by poolMu.
	headlessClient headless.PoolClient

	// autoReopenMu guards autoReopener for concurrent Set/get access.
	autoReopenMu sync.RWMutex
	autoReopener reviewgate.AutoReopenSpawner

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
	// backlog_lifecycle_pr.go). Defaults to "" (text-only footer); the server
	// wires a non-loopback address via SetDashboardBaseURLFn, and loopback
	// results are always dropped by getDashboardBaseURL.
	dashboardBaseURLFnMu sync.RWMutex
	dashboardBaseURLFn   func() string

	// hostRefFn names this instance (host ID and hostname) in the PR footer;
	// guarded by dashboardBaseURLFnMu. Nil means the footer carries no host.
	hostRefFn func() (hostclaim.HostID, string)

	// repoSettingsMu guards the pre-PR repo-settings seam (see
	// SetRepoSettingsSeam) and the per-repo preflight dedupe state.
	repoSettingsMu    sync.Mutex
	repoSettings      RepoSettingsSeam
	preflightSentAt   map[string]time.Time
	preflightNowFn    func() time.Time
	preflightWarmWG   sync.WaitGroup
	preflightInflight map[string]bool

	// noopThresholdMu guards noopThresholdFn (see SetNoopDispatchThresholdFn).
	noopThresholdMu sync.RWMutex
	noopThresholdFn func() int

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

	// backlogReaderOverride replaces the reader:backlog config lookup in tests.
	backlogReaderOverride func() bool

	// liveWork holds the shared live-work probe, its test clock and the per-item
	// hold bookkeeping (backlog_lifecycle_live_work.go).
	liveWork liveWorkState

	// terminalWork is the bounded worker behind PRStatesWentTerminal.
	terminalWork struct {
		once sync.Once
		ch   chan []int
	}

	// ciRerunner/ciReruns back maybeFlakyCheckRerun (ci_flaky_rerun.go); a nil
	// ciRerunner disables the free-rerun shortcut.
	ciRerunner ciRerunClient
	ciReruns   ciRerunLedger

	// conflictRetries throttles conflict-only PR fixes (see remediatePRFixWithBackoffGate).
	conflictRetries    session.ConflictRetryLedger
	conflictRetryClock func() time.Time // nil: time.Now

	// terminalSettled lets the 60s done/archived safety-net sweeps skip items they
	// already fully handled (see terminalSweepSettleTTL); sweepClock nil: time.Now.
	terminalSettled session.SettledTracker
	sweepClock      func() time.Time

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
	// verifyPRBelongsToItem to re-verify, via a live GitHub lookup,
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
	notifier   domain.Notifier

	// sessionArchiverMu guards sessionArchiver for concurrent Set/get access.
	sessionArchiverMu sync.RWMutex
	sessionArchiver   SessionArchiver

	// worktreeCleanerMu guards worktreeCleaner for concurrent Set/get access.
	worktreeCleanerMu sync.RWMutex
	worktreeCleaner   WorktreeCleaner

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
	// reviewGuard keeps at most one review spawn per item in flight (see ReviewSpawnGuard).
	reviewGuard *session.ReviewSpawnGuard

	// customCheckSem limits concurrent custom-gate-check goroutines
	// (runCustomGateCheck, session/backlog_lifecycle_gates.go), same pattern
	// as reviewSem above but independent — a transition may spawn both an
	// automated-review gate and a custom-check gate concurrently, and one
	// kind saturating its bound must not starve the other.
	customCheckSem chan struct{}

	// shutdownCtx is cancelled by Shutdown(); used by long-running review gate calls.
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	// runner encapsulates the spawnReviewGate logic so it can be tested independently.
	runner *reviewgate.ReviewGateRunner

	// pipelineEngine resolves mode-specific slash-command sets and prompts (Epic 1.5).
	// Optional — nil (the default for every constructor except NewBacklogLifecycleListenerWithPool,
	// which is production's only caller) falls back to the built-in default pipeline. Set once at
	// construction and forwarded unchanged into runner — never mutated afterward.
	pipelineEngine pipeline.PipelineEngine

	// livenessEngine resolves per-stage/per-pipeline-mode stuck-detection thresholds (Epic 1.4 of
	// backlog-custom-workflow-stages), replacing the flat liveness.MaxHeadlessTriageSessionStaleness/
	// liveness.MaxWorkSessionStaleness/stuck.BounceThreshold/stuck.BounceLookback constants at their reconcile* call
	// sites. Optional — nil (the default for every constructor except
	// NewBacklogLifecycleListenerWithPool, production's only caller) falls back to each call
	// site's literal constant, unchanged from pre-Epic-1.4 behavior. Set once at construction,
	// same pattern as pipelineEngine above — never mutated afterward.
	livenessEngine liveness.LivenessEngine

	// chainReconcilerMu guards chainReconciler for concurrent Set/get access.
	chainReconcilerMu sync.RWMutex
	// chainReconciler completes pipeline chain-fires interrupted by a crash
	// (webhook-triggers Phase 6, AC5's restart-recovery scenario). nil (the
	// default) makes reconcileTriggerChains a no-op — matches every other
	// optional-dependency detector's nil-safe convention in this file. Wired
	// via SetChainReconciler.
	chainReconciler *chains.TriggerChainReconciler

	// gateSatisfactionRepoMu guards gateSatisfactionRepo for concurrent Set/get access.
	gateSatisfactionRepoMu sync.RWMutex
	// gateSatisfactionRepo backs reconcileCustomGateChecks' scan for in-flight
	// custom-check invocations (Epic 2.4, Task 2.4.4c). nil (the default)
	// makes that sweep a no-op — matches every other optional-dependency
	// detector's nil-safe convention in this file. Wired via
	// SetGateSatisfactionRepository.
	gateSatisfactionRepo session.GateSatisfactionRepository

	// workflowEngineMu guards workflowEngine for concurrent Set/get access.
	workflowEngineMu sync.RWMutex
	// workflowEngine is consulted by transitionHasAutomatedReviewGate (Epic
	// 2.4, Story 2.4.3) to detect a custom transition's attached
	// GateKindAutomatedReview gate, generalizing the review-gate spawn
	// condition beyond the built-in review status literal. nil makes
	// transitionHasAutomatedReviewGate degrade to that literal comparison only.
	// server/dependencies.go wires the concrete *ConfiguredWorkflowEngine here,
	// never the SwitchableWorkflowEngine selector: the .(*ConfiguredWorkflowEngine)
	// assertions below would fail and silently disable gate-context resolution.
	// Wired via SetWorkflowEngine.
	workflowEngine domain.WorkflowEngine
	// wrongEngineWarn limits the "not a ConfiguredWorkflowEngine" WARN to once.
	wrongEngineWarn sync.Once

	enabled atomic.Bool
}

// SetWorkflowEngine wires the WorkflowEngine transitionHasAutomatedReviewGate
// consults for a custom transition's automated-review gate. nil (the
// default) is safe — see the field's doc comment.
func (l *BacklogLifecycleListener) SetWorkflowEngine(engine domain.WorkflowEngine) {
	l.workflowEngineMu.Lock()
	defer l.workflowEngineMu.Unlock()
	l.workflowEngine = engine
}

// configuredWorkflowEngine returns the wired engine as a *ConfiguredWorkflowEngine.
// Any other non-nil engine (notably the SwitchableWorkflowEngine selector)
// silently disables gate-context resolution, so that is logged once.
func (l *BacklogLifecycleListener) configuredWorkflowEngine() (*session.ConfiguredWorkflowEngine, bool) {
	eng := l.getWorkflowEngine()
	cwe, ok := eng.(*session.ConfiguredWorkflowEngine)
	if !ok && eng != nil {
		l.wrongEngineWarn.Do(func() {
			log.Warn("backlog lifecycle listener: workflow engine is not a *ConfiguredWorkflowEngine; automated-review and custom-check gate contexts will not resolve",
				"engine_type", fmt.Sprintf("%T", eng))
		})
	}
	return cwe, ok
}

// WorkflowEngine returns the wired engine (nil if none); exported for the
// production-wiring test in server/.
func (l *BacklogLifecycleListener) WorkflowEngine() domain.WorkflowEngine {
	return l.getWorkflowEngine()
}

// getWorkflowEngine returns the currently-wired WorkflowEngine (nil if none).
func (l *BacklogLifecycleListener) getWorkflowEngine() domain.WorkflowEngine {
	l.workflowEngineMu.RLock()
	defer l.workflowEngineMu.RUnlock()
	return l.workflowEngine
}

// SetGateSatisfactionRepository wires the repository reconcileCustomGateChecks
// (session/backlog_lifecycle_gates.go) scans for in-flight custom-check
// invocations. nil (the default) is safe — see the field's doc comment.
func (l *BacklogLifecycleListener) SetGateSatisfactionRepository(repo session.GateSatisfactionRepository) {
	l.gateSatisfactionRepoMu.Lock()
	defer l.gateSatisfactionRepoMu.Unlock()
	l.gateSatisfactionRepo = repo
}

// getGateSatisfactionRepo returns the currently-wired GateSatisfactionRepository
// (nil if none).
func (l *BacklogLifecycleListener) getGateSatisfactionRepo() session.GateSatisfactionRepository {
	l.gateSatisfactionRepoMu.RLock()
	defer l.gateSatisfactionRepoMu.RUnlock()
	return l.gateSatisfactionRepo
}

// SetReviewGateSatisfactionRepository wires the GateSatisfactionRepository the
// underlying ReviewGateRunner uses to record a configured automated_review
// gate's terminal outcome (Epic 2.4 follow-up — session/review_gate.go's
// recordGateSatisfaction). Distinct from SetGateSatisfactionRepository above
// (which wires reconcileCustomGateChecks' unrelated sweep): the runner holds
// its own copy of the repository, not a shared reference to this listener's.
func (l *BacklogLifecycleListener) SetReviewGateSatisfactionRepository(repo session.GateSatisfactionRepository) {
	l.runner.SetGateSatisfactionRepository(repo)
}

// PipelineEngine returns the PipelineEngine injected at construction (nil if none was
// wired). Exported for the pointer-equality integration test proving BacklogService and
// BacklogLifecycleListener share a single PipelineEngine instance (Story 1.5.1).
func (l *BacklogLifecycleListener) PipelineEngine() pipeline.PipelineEngine {
	return l.pipelineEngine
}

// SetEnabled toggles whether this listener processes lifecycle events.
// Safe to call concurrently.
func (l *BacklogLifecycleListener) SetEnabled(v bool) { l.enabled.Store(v) }

var _ session.BacklogListener = (*BacklogLifecycleListener)(nil)

// IsEnabled reports whether SetEnabled(true) was last called.
func (l *BacklogLifecycleListener) IsEnabled() bool { return l.enabled.Load() }

// SetHeadlessPool wires in the headless LLM pool after construction.
// Calling this enables the headless review gate path even when the listener was
// created via NewBacklogLifecycleListenerWithSpawner.
func (l *BacklogLifecycleListener) SetHeadlessPool(p *headless.Pool) {
	l.poolMu.Lock()
	defer l.poolMu.Unlock()
	l.headlessPool = p
}

// SetDashboardBaseURLFn overrides the base URL used by backlogItemLink to
// build a deep link back to a backlog item in agent-created PR bodies. fn must
// return a non-loopback URL or ""; loopback URLs are discarded.
func (l *BacklogLifecycleListener) SetDashboardBaseURLFn(fn func() string) {
	l.dashboardBaseURLFnMu.Lock()
	defer l.dashboardBaseURLFnMu.Unlock()
	l.dashboardBaseURLFn = fn
}

// RepoSettingsSeam gives the PR-create path a non-blocking view of GitHub repo
// settings: Cached reads the cache only, Fetch warms it. Both nil disables the
// pre-PR warning.
type RepoSettingsSeam struct {
	Cached func(ref github.RepoRef) (github.RepoSettingsResult, bool)
	Fetch  func(ctx context.Context, ref github.RepoRef) github.RepoSettingsResult
}

// SetRepoSettingsSeam wires the repo-settings lookups used by the pre-PR
// auto-merge warning.
func (l *BacklogLifecycleListener) SetRepoSettingsSeam(s RepoSettingsSeam) {
	l.repoSettingsMu.Lock()
	defer l.repoSettingsMu.Unlock()
	l.repoSettings = s
}

// SetAutoReopener wires in the spawner used to automatically reopen items for
// rework when a review verdict is FAIL or PARTIAL.
func (l *BacklogLifecycleListener) SetAutoReopener(r reviewgate.AutoReopenSpawner) {
	l.autoReopenMu.Lock()
	defer l.autoReopenMu.Unlock()
	l.autoReopener = r
}

// SetSessionCreator wires in the spawner used to create review-gate sessions
// after construction. Needed because production wiring
// (server/dependencies.go) constructs this listener before SessionService
// exists.
func (l *BacklogLifecycleListener) SetSessionCreator(s reviewgate.ReviewGateSpawner) {
	l.sessionCreatorMu.Lock()
	defer l.sessionCreatorMu.Unlock()
	l.sessionCreator = s
}

// getSessionCreator returns the current review-gate session spawner under a read lock.
func (l *BacklogLifecycleListener) getSessionCreator() reviewgate.ReviewGateSpawner {
	l.sessionCreatorMu.RLock()
	defer l.sessionCreatorMu.RUnlock()
	return l.sessionCreator
}

// getAutoReopener returns the current auto-reopener under a read lock.
func (l *BacklogLifecycleListener) getAutoReopener() reviewgate.AutoReopenSpawner {
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
		log.Warn("[BacklogLifecycle] DequeueNextQueuedItems error", "error", err)
	}
}

// SetChainReconciler wires in the pipeline-chain restart-recovery reconciler
// (webhook-triggers Phase 6). Called via server/dependencies.go once a
// ChainFirer has been constructed (see WireChainFirer).
func (l *BacklogLifecycleListener) SetChainReconciler(r *chains.TriggerChainReconciler) {
	l.chainReconcilerMu.Lock()
	defer l.chainReconcilerMu.Unlock()
	l.chainReconciler = r
}

// getChainReconciler returns the current chain reconciler under a read lock.
func (l *BacklogLifecycleListener) getChainReconciler() *chains.TriggerChainReconciler {
	l.chainReconcilerMu.RLock()
	defer l.chainReconcilerMu.RUnlock()
	return l.chainReconciler
}

// reconcileTriggerChains delegates to TriggerChainReconciler.ReconcileChains
// — the periodic counterpart to EntRepository.dispatchChainFire's happy-path
// fire, both funneling through the same ChainFirer.Fire so "fire exactly
// once" logic lives in one place. No-op when no reconciler is wired.
func (l *BacklogLifecycleListener) reconcileTriggerChains(ctx context.Context, er *session.EntRepository) {
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
// immutable number, used by verifyPRBelongsToItem (Story 6).
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
func (l *BacklogLifecycleListener) SetNotifier(n domain.Notifier) {
	l.notifierMu.Lock()
	defer l.notifierMu.Unlock()
	l.notifier = n
}

// getNotifier returns the current notifier under a read lock.
func (l *BacklogLifecycleListener) getNotifier() domain.Notifier {
	l.notifierMu.RLock()
	defer l.notifierMu.RUnlock()
	return l.notifier
}

// notify publishes a best-effort operator notification. No-op if no notifier is wired.
func (l *BacklogLifecycleListener) notify(itemID, title, message string, notificationType int32, urgent, important bool) {
	if n := l.getNotifier(); n != nil {
		n.Notify(itemID, title, message, notificationType, urgent, important)
	}
}

// AutoRemediatingNotifier is an optional Notifier extension for a WARNING whose
// automation is already acting. The implementation stamps it so it is informational
// (never a pending decision that pins and inflates the count); the escalation, an
// unstamped notification, is what asks the operator to act.
type AutoRemediatingNotifier interface {
	NotifyAutoRemediating(itemID, title, message string, notificationType int32, urgent, important bool)
}

// notifyAutoRemediating falls back to a plain Notify when the wired notifier does
// not implement AutoRemediatingNotifier, so older adapters keep working.
func (l *BacklogLifecycleListener) notifyAutoRemediating(itemID, title, message string, notificationType int32, urgent, important bool) {
	n := l.getNotifier()
	if n == nil {
		return
	}
	if ar, ok := n.(AutoRemediatingNotifier); ok {
		ar.NotifyAutoRemediating(itemID, title, message, notificationType, urgent, important)
		return
	}
	n.Notify(itemID, title, message, notificationType, urgent, important)
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
	if domain.IsGateDenial(writeErr) {
		// The gate's reporter already raised the one comment and notification.
		log.Debug("[BacklogLifecycle] transition denied by the workflow gate; not re-notifying", "item", itemID, "error", writeErr)
		return
	}
	l.notify(itemID,
		"Status update failed after work completed",
		fmt.Sprintf("%s — %s: %v. The item's status may not reflect reality; check manually.", itemTitle, failureContext, writeErr),
		7,          // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
		true, true, // urgent, important — a silent status/reality mismatch is a genuine correctness bug
	)
}

// logSystemTransitionFailure logs a failed gated system write: a denial or a
// lost compare-and-swap is expected and retried on the next tick (Debug); any
// other failure is an Error.
func logSystemTransitionFailure(where, itemID string, to session.BacklogStatus, err error) {
	if domain.IsGateDenial(err) || errors.Is(err, domain.ErrPreconditionFailed) {
		log.Debug("[BacklogLifecycle] "+where+": transition skipped", "item", itemID, "to", to, "error", err)
		return
	}
	log.Error("[BacklogLifecycle] "+where+": transition failed", "item", itemID, "to", to, "error", err)
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

// SetWorktreeCleaner wires in the cleaner used to synchronously remove git
// worktrees and archive work/review sessions when an internal transition
// path (not the manual RPC) drives an item straight to a terminal status.
// Optional — nil means those paths fall back to the 60s
// reconcileTerminalItemWorktrees safety-net sweep (cleanup_terminal_worktrees
// detector in ReconcileStuck), same as before this was wired.
func (l *BacklogLifecycleListener) SetWorktreeCleaner(c WorktreeCleaner) {
	l.worktreeCleanerMu.Lock()
	defer l.worktreeCleanerMu.Unlock()
	l.worktreeCleaner = c
}

// getWorktreeCleaner returns the current worktree cleaner under a read lock.
func (l *BacklogLifecycleListener) getWorktreeCleaner() WorktreeCleaner {
	l.worktreeCleanerMu.RLock()
	defer l.worktreeCleanerMu.RUnlock()
	return l.worktreeCleaner
}

// cleanupTerminalItemSync invokes the injected WorktreeCleaner, if wired, for
// itemID — the shared synchronous-cleanup call internal transition paths
// (transitionBouncingItemToDone, the PR-merge-detected done path) make right
// after a successful terminal transition so cleanup does not depend solely
// on the 60s sweep. No-op when unwired (matches WorktreeCleaner's nil-safety
// contract).
func (l *BacklogLifecycleListener) cleanupTerminalItemSync(ctx context.Context, itemID string) session.CleanupOutcome {
	if c := l.getWorktreeCleaner(); c != nil {
		if session.CleanupPathFrom(ctx) == session.CleanupPathUnknown {
			ctx = session.WithCleanupPath(ctx, session.CleanupPathHook)
		}
		return c.CleanupTerminalItem(ctx, itemID)
	}
	return session.CleanupDoneOutcome()
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

// SetHeadlessClient routes custom-gate and PR-description calls through c
// (normally a headless.SelectingClient) instead of the raw claude pool.
func (l *BacklogLifecycleListener) SetHeadlessClient(c headless.PoolClient) {
	l.poolMu.Lock()
	defer l.poolMu.Unlock()
	l.headlessClient = c
}

// getHeadlessCaller returns the selector client if wired, else the raw pool, else nil.
func (l *BacklogLifecycleListener) getHeadlessCaller() headless.PoolClient {
	l.poolMu.RLock()
	defer l.poolMu.RUnlock()
	if l.headlessClient != nil {
		return l.headlessClient
	}
	if l.headlessPool != nil {
		return l.headlessPool
	}
	return nil
}

func (l *BacklogLifecycleListener) getDashboardBaseURL() string {
	l.dashboardBaseURLFnMu.RLock()
	defer l.dashboardBaseURLFnMu.RUnlock()
	if l.dashboardBaseURLFn == nil {
		return ""
	}
	return NonLoopbackBaseURL(l.dashboardBaseURLFn())
}

// SetHostRefFn wires the resolver that names this instance (host ID and
// ssq:// hostname) in PR-body footers. Either value may be empty.
func (l *BacklogLifecycleListener) SetHostRefFn(fn func() (hostclaim.HostID, string)) {
	l.dashboardBaseURLFnMu.Lock()
	defer l.dashboardBaseURLFnMu.Unlock()
	l.hostRefFn = fn
}

// getPRFooterOrigin resolves who created a PR, for prFooter.
func (l *BacklogLifecycleListener) getPRFooterOrigin() PRFooterOrigin {
	origin := PRFooterOrigin{BaseURL: l.getDashboardBaseURL()}
	l.dashboardBaseURLFnMu.RLock()
	fn := l.hostRefFn
	l.dashboardBaseURLFnMu.RUnlock()
	if fn != nil {
		origin.HostID, origin.Hostname = fn()
	}
	return origin
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
func newListenerBase(storage *session.Storage, pipelineEngine pipeline.PipelineEngine, livenessEngine liveness.LivenessEngine) *BacklogLifecycleListener {
	ctx, cancel := context.WithCancel(context.Background())
	l := &BacklogLifecycleListener{
		storage:                 storage,
		pipelineEngine:          pipelineEngine,
		livenessEngine:          livenessEngine,
		reviewSem:               make(chan struct{}, maxConcurrentReviewGates),
		reviewGuard:             session.NewReviewSpawnGuard(),
		customCheckSem:          make(chan struct{}, maxConcurrentCustomGateChecks),
		shutdownCtx:             ctx,
		shutdownCancel:          cancel,
		prPendingCheckerFactory: defaultPRPendingCheckerFactory,
		ciRerunner:              ghCIRerunClient{},
		prCreatorFactory:        defaultPRCreatorFactory,
		branchReconciler:        git.MergeMainIntoWorktree,
		orphanedPRFinder:        defaultOrphanedPRFinder,
		prByNumberFinder:        defaultPRByNumberFinder,
		dashboardBaseURLFn:      func() string { return "" },
	}
	l.runner = reviewgate.NewReviewGateRunner(storage, l.getAutoReopener, l.getNotifier, l.getSessionCreator, pipelineEngine)
	return l
}

// NewBacklogLifecycleListener creates a listener backed by the given storage.
// The review gate is disabled (sessionCreator=nil, headlessPool=nil). No PipelineEngine
// is wired (nil) — callers needing one should use NewBacklogLifecycleListenerWithPool.
func NewBacklogLifecycleListener(storage *session.Storage) *BacklogLifecycleListener {
	return newListenerBase(storage, nil, nil)
}

// NewBacklogLifecycleListenerWithSpawner creates a listener that will spawn a
// review gate session when a work session exits and SkipReviewGate is false.
func NewBacklogLifecycleListenerWithSpawner(storage *session.Storage, spawner reviewgate.ReviewGateSpawner) *BacklogLifecycleListener {
	l := newListenerBase(storage, nil, nil)
	l.SetSessionCreator(spawner)
	return l
}

// NewBacklogLifecycleListenerWithPool creates a listener that uses a headless.Pool
// for review gate calls instead of spawning a tmux session. pipelineEngine is the
// shared PipelineEngine instance (Epic 1.5, Story 1.5.1) — pass nil to fall back to
// the built-in default pipeline for every item. livenessEngine is the shared
// liveness.LivenessEngine instance (Epic 1.4) — pass nil to fall back to each stuck-detection
// sweep's literal constant for every item.
func NewBacklogLifecycleListenerWithPool(storage *session.Storage, pool *headless.Pool, pipelineEngine pipeline.PipelineEngine, livenessEngine liveness.LivenessEngine) *BacklogLifecycleListener {
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

func (il *instanceBacklogListener) OnLifecycleEvent(event domain.LifecycleEvent, _ string) {
	if !il.parent.enabled.Load() {
		return
	}
	switch event {
	case domain.EventStarted:
		go il.parent.onSessionStarted(il.instanceUUID)
	case domain.EventExited, domain.EventStopped:
		// A deliberate operator stop (stop_session, DeleteSession, backlog
		// stale-work remediation) ends the session exactly as much as an
		// unexpected exit does — the ItemSession bookkeeping and downstream
		// reconciliation must not depend on which one happened. See BUG-027.
		go il.parent.onSessionExited(il.instanceUUID)
	}
}

// WireToInstance creates a per-instance listener shim and registers it on inst.
// Call this for every Instance that should participate in backlog lifecycle tracking.
func (l *BacklogLifecycleListener) WireToInstance(inst *session.Instance) {
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
		if errors.Is(err, domain.ErrNotFound) {
			return
		}
		log.Error("[BacklogLifecycle] GetItemSessionBySessionUUID error", "session", sessionUUID, "error", err)
		return
	}
	if err := l.storage.UpdateItemSessionStarted(ctx, is.ID, time.Now()); err != nil {
		log.Error("[BacklogLifecycle] UpdateItemSessionStarted error", "item_session", is.ID, "error", err)
	}
}

// onSessionExited drives the in_progress→review (or in_progress→done for skip_review_gate) transition.
func (l *BacklogLifecycleListener) onSessionExited(sessionUUID string) {
	ctx := context.Background()

	is, err := l.storage.GetItemSessionBySessionUUID(ctx, sessionUUID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return
		}
		log.Error("[BacklogLifecycle] GetItemSessionBySessionUUID error", "session", sessionUUID, "error", err)
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
		log.Error("[BacklogLifecycle] UpdateItemSessionEnded error", "item_session", is.ID, "error", err)
	}

	// Review sessions are handled by a dedicated post-verdict path: they don't
	// drive an in_progress→review/done transition themselves, they process the
	// verdict submitted (or not) by the review session that just exited. Other
	// non-work roles (e.g. triage) have nothing further to do here.
	switch is.Role {
	case session.SessionRoleReview:
		// forcePush=false: this is the normal, real-time exit-event path — defer to
		// a still-live work session's own /backlog/ship instead of pushing
		// mechanically. See handleReviewSessionExited's doc comment.
		l.handleReviewSessionExited(ctx, is, false)
		return
	case session.SessionRoleWork:
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
		log.Debug("[BacklogLifecycle] onSessionExited already ended by another code path before this exit event; skipping status transition", "item", is.BacklogItemID, "session", sessionUUID)
		return
	}

	// Look up the BacklogItem via storage (no longer an eager-loaded edge).
	item, err := l.storage.GetBacklogItem(ctx, is.BacklogItemID)
	if err != nil {
		log.Error("[BacklogLifecycle] GetBacklogItem failed", "session", sessionUUID, "item", is.BacklogItemID, "error", err)
		return
	}

	if session.BacklogStatus(item.Status) != session.BacklogStatusInProgress {
		log.Debug("[BacklogLifecycle] item is not in_progress; skipping", "item", item.ID, "status", item.Status)
		return
	}

	toStatus := session.BacklogStatusReview
	if item.SkipReviewGate {
		toStatus = session.BacklogStatusDone
	}

	updatedAt := item.UpdatedAt
	precondition := &backlogdto.BacklogItemPrecondition{
		ExpectedStatus:    string(session.BacklogStatusInProgress),
		ExpectedUpdatedAt: &updatedAt,
	}
	if _, err := l.storage.GatedTransition(ctx, backlogdto.TransitionRequest{
		Caller: domain.CallerSessionExited, ItemID: item.ID, To: toStatus, Precondition: precondition, TriggeredBy: session.TriggeredBySystem,
	}); err != nil {
		logSystemTransitionFailure("onSessionExited", item.ID, toStatus, err)
		return
	}

	// The item is leaving in_progress — any open stale_work row is stale by
	// definition now. Resolve immediately rather than waiting for the
	// self-heal sweep's next tick (Task 2.1.5a).
	l.resolveStuckLogged(ctx, l.storage.Repo, item.ID, domain.StuckReasonStaleWork, "onSessionExited")

	log.Info("[BacklogLifecycle] item transitioned (session exited)", "item", item.ID, "to_status", toStatus, "session", sessionUUID)

	// The item just left in_progress, freeing a WIP slot — dequeue immediately
	// rather than waiting for the next ReconcileStuck tick (safety-net only).
	go l.triggerDequeue(context.Background())

	// Spawn review gate if the item moved to review and a review mechanism is configured.
	if gateContext, ok := l.resolveReviewGateContext(session.BacklogStatusInProgress, toStatus); ok && !item.SkipReviewGate && l.getSessionCreator() != nil {
		go func() {
			// Acquire the bounded semaphore to prevent unbounded goroutine fan-out
			// when many sessions exit simultaneously.
			select {
			case l.reviewSem <- struct{}{}:
			case <-l.shutdownCtx.Done():
				return
			}
			defer func() { <-l.reviewSem }()
			l.spawnReviewGate(gateContext, item, is)
		}()
	}

	// Spawn a custom/pluggable check gate (Story 2.4.4's follow-up: this was
	// previously unreachable in production — see gate_custom_check.go's
	// InvokeCustomGateCheck doc comment) if this transition has one configured.
	// Independent of the automated-review gate above; a transition may have
	// either, both, or neither.
	if gateID, cfg, ok := l.resolveCustomCheckGateContext(session.BacklogStatusInProgress, toStatus); ok {
		targetStage := toStatus
		go func() {
			select {
			case l.customCheckSem <- struct{}{}:
			case <-l.shutdownCtx.Done():
				return
			}
			defer func() { <-l.customCheckSem }()
			l.runCustomGateCheck(l.shutdownCtx, gateID, cfg, targetStage, item)
		}()
	}
}

// builtInReviewGateContext is the GateContext (session/review_gate.go) passed
// to every review-gate spawn for the built-in review->pr_pending transition:
// no persisted TransitionGate backs it (GateID ""), and it always reviews a
// code diff (RequiresDiff: true) — the same behavior review_gate.go's Run had
// unconditionally before Story 2.4.3 generalized it to accept a GateContext.
var builtInReviewGateContext = reviewgate.GateContext{
	GateID:           "",
	TargetTransition: session.BacklogStatusReview,
	RequiresDiff:     true,
}

// resolveReviewGateContext resolves the GateContext spawnReviewGate should use
// for a from->to transition (Epic 2.4, Story 2.4.3's generalization of this
// call site's old hardcoded `toStatus == BacklogStatusReview` literal; formerly
// transitionHasAutomatedReviewGate, which only reported a bool and always
// spawned with builtInReviewGateContext regardless of which gate matched).
//
// The built-in review status always resolves to builtInReviewGateContext,
// unconditionally — never a configured gate's own fields, even when a
// configured automated_review gate additionally matches this edge: the
// built-in workflow-stage seed migration (Epic 2.2) does not itself seed an
// automated_review gate for this edge, and requiring one here would silently
// stop the review gate from firing for every install that hasn't separately
// configured a custom graph. This is the zero-regression guarantee Task
// 2.4.3d's test covers.
//
// Only a genuinely custom transition (to != BacklogStatusReview) backed by a
// wired *ConfiguredWorkflowEngine with a matching GateKindAutomatedReview gate
// resolves to that gate's own GateID/RequiresDiff/PipelineMode. ok is false
// when no automated-review gate applies at all — the caller must not spawn a
// review gate in that case. server/dependencies.go wires the concrete engine
// via SetWorkflowEngine; with none (or a non-concrete one) this degrades to
// the `to == BacklogStatusReview` case only.
func (l *BacklogLifecycleListener) resolveReviewGateContext(from, to session.BacklogStatus) (reviewgate.GateContext, bool) {
	if to == session.BacklogStatusReview {
		return builtInReviewGateContext, true
	}
	cwe, ok := l.configuredWorkflowEngine()
	if !ok {
		return reviewgate.GateContext{}, false
	}
	gateID, cfg, found := cwe.ResolveAutomatedReviewGateContext(from, to)
	if !found {
		return reviewgate.GateContext{}, false
	}
	return reviewgate.GateContext{
		GateID:           gateID,
		TargetTransition: to,
		RequiresDiff:     cfg.RequiresDiff,
		PipelineMode:     cfg.PipelineMode,
	}, true
}

// resolveCustomCheckGateContext resolves the GateID+CustomCheckConfig for a
// GateKindCustom gate configured on the from->to edge (Epic 2.4, Story
// 2.4.4's follow-up — see gate_custom_check.go's InvokeCustomGateCheck doc
// comment for why this previously had no production call site). Mirrors
// resolveReviewGateContext's shape for the sibling stateful gate kind. ok is
// false when no WorkflowEngine is wired, it isn't a *ConfiguredWorkflowEngine,
// or the edge carries no custom-check gate.
func (l *BacklogLifecycleListener) resolveCustomCheckGateContext(from, to session.BacklogStatus) (uuid.UUID, domain.CustomCheckConfig, bool) {
	cwe, ok := l.configuredWorkflowEngine()
	if !ok {
		return uuid.Nil, domain.CustomCheckConfig{}, false
	}
	return cwe.ResolveCustomCheckGateContext(from, to)
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
// reconcileStaleWorkSessions' liveness.MaxWorkSessionStaleness check, without
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
	er := l.storage.Repo

	seeded := 0

	// abandoned_review: review-status items with a verdict on record but
	// nothing active in flight.
	reviewItems, err := er.FindStuckReviewItems(ctx)
	if err != nil {
		log.Warn("[BacklogLifecycle] BackfillStuckStates FindStuckReviewItems error", "error", err)
	} else {
		for _, item := range reviewItems {
			if l.backfillMarkAndNotify(ctx, er, item.ID.String(), domain.StuckReasonAbandonedReview, session.BacklogStatusReview,
				"backfilled at startup: stuck in review with nothing in flight") {
				seeded++
			}
		}
	}

	// stale_work: in_progress items whose active work session has gone quiet
	// past liveness.MaxWorkSessionStaleness.
	inProgressItems, err := l.storage.ListBacklogItems(ctx, backlogdto.BacklogItemFilter{
		Statuses: []string{string(session.BacklogStatusInProgress)},
	})
	if err != nil {
		log.Warn("[BacklogLifecycle] BackfillStuckStates ListBacklogItems error", "error", err)
	} else {
		for _, item := range inProgressItems {
			sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
			if sessErr != nil {
				log.Warn("[BacklogLifecycle] BackfillStuckStates ListItemSessions failed", "item", item.ID, "error", sessErr)
				continue
			}
			var active *backlogdto.ItemSessionSummary
			for i := range sessions {
				if sessions[i].Role == session.SessionRoleWork && sessions[i].EndedAt == nil {
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
			if time.Since(lastProgress) < liveness.MaxWorkSessionStaleness {
				continue
			}
			if l.backfillMarkAndNotify(ctx, er, item.ID, domain.StuckReasonStaleWork, session.BacklogStatusInProgress,
				fmt.Sprintf("backfilled at startup: no progress since %s", lastProgress)) {
				seeded++
			}
		}
	}

	log.Info("[BacklogLifecycle] BackfillStuckStates: seeded stuck row(s) at startup", "seeded", seeded)
}

// backfillMarkAndNotify marks a stuck row and immediately pre-sets
// notified_at so the first genuine reconcile tick after startup does not
// re-notify for a condition already known before the restart. Returns
// whether a row was newly opened by this call. Best-effort: errors are
// logged, never returned.
func (l *BacklogLifecycleListener) backfillMarkAndNotify(ctx context.Context, er *session.EntRepository, itemID string, reason domain.StuckReason, expectedStatus session.BacklogStatus, stuckContext string) bool {
	applied, err := er.MarkStuck(ctx, itemID, reason, expectedStatus, stuckContext)
	if err != nil {
		log.Warn("[BacklogLifecycle] BackfillStuckStates MarkStuck failed", "item", itemID, "reason", reason, "error", err)
		return false
	}
	if !applied {
		return false
	}
	if _, notifyErr := er.MarkStuckNotified(ctx, itemID, reason); notifyErr != nil {
		log.Warn("[BacklogLifecycle] BackfillStuckStates MarkStuckNotified failed", "item", itemID, "reason", reason, "error", notifyErr)
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
			log.Warn("[BacklogLifecycle] stuck detector panicked (recovered)", "detector", name, "recovered", r)
			*panickedNames = append(*panickedNames, name)
			return
		}
		*okNames = append(*okNames, name)
	}()
	fn()
}

// heldByTransitionDenial is the detector-local form of the open-denial guard
// (Story 1.5.8): true, with a Debug line, while item's status has an open
// transition_denied row the engine still refuses. detector names the caller in
// the log.
func (l *BacklogLifecycleListener) heldByTransitionDenial(ctx context.Context, item *backlogdto.BacklogItemData, detector string) bool {
	if !l.storage.TransitionDenialHolds(ctx, item) {
		return false
	}
	log.Debug("[BacklogLifecycle] item waits on a denied transition, skipping", "detector", detector, "item", item.ID, "status", item.Status)
	return true
}

// respawnReviewGatesWithoutGate is ReconcileStuck's inline review-gate respawn:
// it spawns a gate for every review item that has none. It calls the
// listener-local spawnReviewGate rather than a BacklogService spawn point, so
// the open-denial guard (Story 1.5.8) lives here.
func (l *BacklogLifecycleListener) respawnReviewGatesWithoutGate(ctx context.Context, er *session.EntRepository) {
	items, gateErr := er.FindReviewItemsWithoutGate(ctx)
	if gateErr != nil {
		log.Error("[BacklogLifecycle] FindReviewItemsWithoutGate error", "error", gateErr)
		return
	}
	for _, item := range items {
		var workSession *backlogdto.ItemSessionSummary
		if len(item.Edges.ItemSessions) > 0 {
			s := session.ItemSessionToSummary(item.Edges.ItemSessions[0])
			workSession = &s
		}
		if workSession == nil {
			log.Debug("[BacklogLifecycle] ReconcileStuckReviewGates: item has no work session, skipping", "item", item.ID)
			continue
		}
		itemData := session.BacklogItemToData(item)
		if l.heldByTransitionDenial(ctx, &itemData, "ReconcileStuckReviewGates") {
			continue
		}
		log.Info("[BacklogLifecycle] ReconcileStuckReviewGates: re-spawning review gate", "item", item.ID)
		isCopy := *workSession
		go func(itemCopy *backlogdto.BacklogItemData, isCopy backlogdto.ItemSessionSummary) {
			select {
			case l.reviewSem <- struct{}{}:
			case <-l.shutdownCtx.Done():
				return
			}
			defer func() { <-l.reviewSem }()
			// itemCopy.Status is already "review" here (FindReviewItemsWithoutGate's
			// query scope), so resolveReviewGateContext's to==BacklogStatusReview
			// branch always applies — this always resolves to
			// builtInReviewGateContext, routed through the same resolver as
			// onSessionExited for a single source of truth rather than a second
			// hardcoded reference to that variable.
			gateContext, ok := l.resolveReviewGateContext(session.BacklogStatus(itemCopy.Status), session.BacklogStatusReview)
			if !ok {
				gateContext = builtInReviewGateContext
			}
			l.spawnReviewGate(gateContext, itemCopy, isCopy)
		}(&itemData, isCopy)
	}
}

// ReconcileStuck calls GatedReconcileStuck and logs the result.
// Intended to be called on a periodic ticker as a safety net for abnormal session exits.
// No-op when the listener is disabled.
func (l *BacklogLifecycleListener) ReconcileStuck(ctx context.Context) {
	if !l.enabled.Load() {
		return
	}
	er := l.storage.Repo
	n, err := l.storage.GatedReconcileStuck(ctx)
	if err != nil {
		log.Error("[BacklogLifecycle] ReconcileStuck error", "error", err)
		return
	}
	if n > 0 {
		log.Info("[BacklogLifecycle] ReconcileStuck: transitioned stuck items to review", "count", n)
	} else {
		log.Debug("[BacklogLifecycle] ReconcileStuck: no stuck items found")
	}

	// Re-spawn review gates for items stuck in "review" with no review session.
	// Occurs when the headless pool was unavailable at the time of the work session exit.
	// Scoped to this block only (not an early return) — PR-pending polling and staleness
	// detection below must still run even when no review mechanism is configured.
	if l.getSessionCreator() != nil {
		l.respawnReviewGatesWithoutGate(ctx, er)
	}

	// Self-heal pr_pending items whose pr_number is missing (0) despite having a
	// pr_url — otherwise permanently invisible to FindPRPendingItems' PrNumberGT(0)
	// filter below, so they'd never get polled. See BackfillMissingPRNumbers doc.
	if n, backfillErr := er.BackfillMissingPRNumbers(ctx); backfillErr != nil {
		log.Error("[BacklogLifecycle] BackfillMissingPRNumbers error", "error", backfillErr)
	} else if n > 0 {
		log.Info("[BacklogLifecycle] BackfillMissingPRNumbers: backfilled pr_number", "count", n)
	}

	// Durable stuck-reason detectors, each panic-isolated (Story 2.1.5e) so one
	// detector's panic cannot skip the others or merge detection below.
	var okNames, panickedNames []string
	for _, d := range l.defaultStuckDetectors() {
		l.runStuckDetector(d.Name(), &okNames, &panickedNames, func() { d.Run(ctx, er) })
	}

	openRows, countErr := er.FindOpenStuckStates(ctx)
	openCount := -1
	if countErr == nil {
		openCount = len(openRows)
	}
	log.Info("[BacklogLifecycle] stuck sweep tick", "ok", okNames, "panicked", panickedNames, "open_rows", openCount)
}

// resolveStuckLogged resolves an open BacklogStuckState row for (itemID, reason),
// logging (not returning) any error. Centralizes the resolve-and-log idiom that was
// previously repeated verbatim across every detector's resolve path (10 call sites —
// backlog-feature-improvement audit finding). caller identifies the calling
// function/branch for the log line; reason is included automatically, so callers no
// longer need to encode it in caller too (except to distinguish multiple resolve call
// sites for the same reason within one function, e.g. "ReconcilePRPending/closed").
func (l *BacklogLifecycleListener) resolveStuckLogged(ctx context.Context, er *session.EntRepository, itemID string, reason domain.StuckReason, caller string) {
	if _, err := er.ResolveStuck(ctx, itemID, reason); err != nil {
		log.Warn("[BacklogLifecycle] ResolveStuck failed", "caller", caller, "reason", reason, "item", itemID, "error", err)
	}
}

// resolveBouncingAndCapExhausted resolves both domain.StuckReasonBouncing and
// domain.StuckReasonBounceCapExhausted for itemID via resolveStuckLogged.
// bounce_cap_exhausted (Signal 2, plan.md Epic 1.3) can only ever coexist
// with an open bouncing row, so every site that resolves bouncing must
// resolve bounce_cap_exhausted alongside it or the marker would outlive the
// condition it describes. Centralizes reconcileBouncingItems' two identical
// resolve-pairs (merged and shipped-without-PR branches).
func (l *BacklogLifecycleListener) resolveBouncingAndCapExhausted(ctx context.Context, er *session.EntRepository, itemID, caller string) {
	l.resolveStuckLogged(ctx, er, itemID, domain.StuckReasonBouncing, caller)
	l.resolveStuckLogged(ctx, er, itemID, domain.StuckReasonBounceCapExhausted, caller)
}

// reconcileStaleWorkSessions notifies once per item when an in_progress backlog item's
// active work session has gone longer than liveness.MaxWorkSessionStaleness without progress,
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
	items, err := l.storage.ListBacklogItems(ctx, backlogdto.BacklogItemFilter{
		Statuses: []string{
			string(session.BacklogStatusInProgress),
			string(session.BacklogStatusReview),
			string(session.BacklogStatusPRPending),
		},
	})
	if err != nil {
		log.Warn("[BacklogLifecycle] refreshWorkSessionGitActivity list error", "error", err)
		return
	}

	for _, item := range items {
		if item.RepoPath == "" {
			continue
		}
		sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
		if sessErr != nil {
			log.Warn("[BacklogLifecycle] refreshWorkSessionGitActivity ListItemSessions failed", "item", item.ID, "error", sessErr)
			continue
		}
		var lastWork *backlogdto.ItemSessionSummary
		for i := range sessions {
			// Ascending by CreatedAt (ListItemSessions' query order) — keep
			// overwriting so this ends up holding the *most recent* work session.
			if sessions[i].Role == session.SessionRoleWork {
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
			log.Debug("[BacklogLifecycle] refreshWorkSessionGitActivity CommitInfo failed", "item", item.ID, "sha", head, "error", infoErr)
			continue
		}

		commitCount := lastWork.CommitCountSinceSpawn
		if lastWork.BaseCommitSha != "" {
			if shipped, _, listErr := git.ListShippedCommits(ctx, item.RepoPath, lastWork.BaseCommitSha, head); listErr == nil {
				commitCount = len(shipped)
			} else {
				log.Debug("[BacklogLifecycle] refreshWorkSessionGitActivity ListShippedCommits failed", "item", item.ID, "error", listErr)
			}
		}

		if updErr := l.storage.UpdateItemSessionGitActivity(ctx, lastWork.ID, head, info.Summary, info.AuthorAt, commitCount); updErr != nil {
			log.Warn("[BacklogLifecycle] refreshWorkSessionGitActivity update failed", "item", item.ID, "session", lastWork.SessionUUID, "error", updErr)
			continue
		}
		log.Debug("[BacklogLifecycle] refreshWorkSessionGitActivity: last commit updated",
			"item", item.ID, "session", lastWork.SessionUUID, "from_sha", lastWork.LastCommitSha, "to_sha", head, "commit_count", commitCount)
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
func (l *BacklogLifecycleListener) reconcilePlanNotApprovedItems(ctx context.Context, er *session.EntRepository) {
	items, err := l.storage.ListBacklogItems(ctx, backlogdto.BacklogItemFilter{
		Statuses: []string{string(session.BacklogStatusQueued)},
	})
	if err != nil {
		log.Warn("[BacklogLifecycle] reconcilePlanNotApprovedItems list error", "error", err)
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
			log.Warn("[BacklogLifecycle] reconcilePlanNotApprovedItems ListItemSessions failed", "item", item.ID, "error", sessErr)
			// Fail open (still flag as plan-not-approved below) — losing session
			// visibility for one tick shouldn't suppress the pre-existing signal.
		} else if latest := latestTriageSession(sessions); latest != nil && latest.EndedAt != nil && latest.TriageResult == "" {
			continue
		}

		applied, markErr := er.MarkStuck(ctx, item.ID, domain.StuckReasonPlanNotApproved, session.BacklogStatusQueued,
			"queued item blocked by DequeueNextQueuedItems' planning gate (plan not approved, skip_planning not set)")
		if markErr != nil {
			log.Warn("[BacklogLifecycle] reconcilePlanNotApprovedItems MarkStuck failed", "item", item.ID, "error", markErr)
			continue
		}
		if !applied {
			continue
		}
		rows, findErr := er.FindOpenStuckStates(ctx)
		if findErr != nil {
			log.Warn("[BacklogLifecycle] reconcilePlanNotApprovedItems FindOpenStuckStates failed", "item", item.ID, "error", findErr)
			continue
		}
		row, ok := backlogdto.FindOpenStuckStateFor(rows, item.ID, domain.StuckReasonPlanNotApproved)
		if !ok || row.NotifiedAt != nil {
			continue
		}

		log.Warn("[BacklogLifecycle] item queued but blocked by unapproved plan", "item", item.ID)
		l.notify(item.ID,
			"Queued item blocked by unapproved plan",
			fmt.Sprintf("%s — this item cannot be dequeued until its plan is approved (or skip_planning is set). Approve the plan or update the item to unblock it.", item.Title),
			8,           // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			false, true, // urgent, important — blocks progress but is a standing state, not time-critical
		)
		if _, notifyErr := er.MarkStuckNotified(ctx, item.ID, domain.StuckReasonPlanNotApproved); notifyErr != nil {
			log.Warn("[BacklogLifecycle] reconcilePlanNotApprovedItems MarkStuckNotified failed", "item", item.ID, "error", notifyErr)
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
	return l.storage.ResolveLatestWorkCommit(ctx, sessionUUID, repoPath)
}

// ResolveBranchTip resolves branch to its commit SHA in repoPath. Worktrees share one ref
// store, so repoPath may be any checkout of the repo. An absent branch is an error.
func ResolveBranchTip(ctx context.Context, repoPath, branch string) (string, error) {
	return session.ResolveBranchTip(ctx, repoPath, branch)
}

// transitionBouncingItemToDone moves an item reconcileBouncingItems has
// externally verified as converged (its PR merged, or its commit shipped to
// main) to done. It does this via the state machine's own legal edges —
// recording a genuine PASS verdict via RecordTerminalReviewVerdict (documenting
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
func (l *BacklogLifecycleListener) transitionBouncingItemToDone(ctx context.Context, item backlogdto.BacklogItemData, verdictSummary string) error {
	if _, err := pipeline.RecordTerminalReviewVerdict(l.storage, item.ID, item.AcceptanceCriteria, "bounce-reconcile-"+uuid.New().String(), session.ReviewVerdictPass, verdictSummary); err != nil {
		return fmt.Errorf("record PASS verdict: %w", err)
	}

	status := item.Status
	if status == string(session.BacklogStatusInProgress) {
		precondition := &backlogdto.BacklogItemPrecondition{ExpectedStatus: status}
		if _, err := l.storage.GatedTransition(ctx, backlogdto.TransitionRequest{
			Caller: domain.CallerBouncingToDone, ItemID: item.ID, To: session.BacklogStatusReview, Precondition: precondition, TriggeredBy: session.TriggeredBySystem,
		}); err != nil {
			return fmt.Errorf("in_progress->review: %w", err)
		}
		status = string(session.BacklogStatusReview)
	}

	precondition := &backlogdto.BacklogItemPrecondition{ExpectedStatus: status}
	if _, err := l.storage.GatedTransition(ctx, backlogdto.TransitionRequest{
		Caller: domain.CallerBouncingToDone, ItemID: item.ID, To: session.BacklogStatusDone, Precondition: precondition, TriggeredBy: session.TriggeredBySystem,
	}); err != nil {
		return fmt.Errorf("review->done: %w", err)
	}
	l.cleanupTerminalItemSync(ctx, item.ID)
	return nil
}

// otherPRsHoldItem reports, under reader:backlog, that the item's derived PR set
// has an open or unknown PR (or could not be read, which holds: later, never earlier).
func (l *BacklogLifecycleListener) otherPRsHoldItem(ctx context.Context, er *session.EntRepository, itemID string) (bool, string) {
	if !l.backlogReaderEnabled() {
		return false, ""
	}
	prs, err := er.ItemPRsByID(ctx, itemID)
	if err != nil {
		return true, "pr set unreadable: " + err.Error()
	}
	if session.ItemPRVerdict(prs) == session.ItemVerdictOpen {
		return true, "verdict open"
	}
	return false, ""
}

// liveHoldFromRows is reconcileBouncingItems' pre-network guard: true when the item
// PR rows already give verdict Done and a live work session holds it.
func (l *BacklogLifecycleListener) liveHoldFromRows(ctx context.Context, er *session.EntRepository, itemID, title string) bool {
	if l.LiveWorkProbe() == nil || !l.backlogReaderEnabled() {
		return false
	}
	prs, err := er.ItemPRsByID(ctx, itemID)
	if err != nil || session.ItemPRVerdict(prs) != session.ItemVerdictDone {
		return false
	}
	return l.holdMergedItemForLiveWork(ctx, er, itemID, title)
}

// reconcileBouncingItems flags items that have crossed in_progress->review
// >= stuck.BounceThreshold times within stuck.BounceLookback with no recorded PASS
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
func (l *BacklogLifecycleListener) reconcileBouncingItems(ctx context.Context, er *session.EntRepository) { //nolint:gocognit,gocyclo // moved verbatim by the package split; simplification is a separate change
	// Scan items in the two statuses a bouncing cycle spans; a converged item
	// (done) is handled by the self-heal sweep, not re-flagged here.
	items, err := l.storage.ListBacklogItems(ctx, backlogdto.BacklogItemFilter{
		Statuses: []string{string(session.BacklogStatusInProgress), string(session.BacklogStatusReview)},
	})
	if err != nil {
		log.Warn("[BacklogLifecycle] reconcileBouncingItems list error", "error", err)
		return
	}

	for _, item := range items {
		// Resolve this item's Shape-C liveness definition (CycleThreshold/CycleLookback)
		// per item, inside the loop — a per-mode override means these can legitimately
		// differ between items in the same tick (Epic 1.4, Story 1.4.3). Keyed to
		// BacklogStatusReview, NOT BacklogStatusInProgress: liveness.DefaultLivenessEngine's table
		// (Epic 1.2) already occupies BacklogStatusInProgress with the Shape-B stale-work
		// definition, and a liveness.LivenessDefinition is a tagged union with exactly one Kind per
		// stage — see this file's Epic 1.4 plan-correction note in
		// project_plans/backlog-custom-workflow-stages/implementation/plan.md's Story 1.4.3.
		cycleThreshold := stuck.BounceThreshold
		cycleLookback := stuck.BounceLookback
		if l.livenessEngine != nil {
			if def, defErr := l.livenessEngine.LivenessFor(session.BacklogStatusReview, backlogdto.PipelineMode(item.PipelineMode)); defErr == nil && !def.IsNoTimeout() {
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
			// Cheap local check first: rows already say the set is Done and a session
			// is live, so skip the GitHub call and the refresh for the whole hold.
			if l.liveHoldFromRows(ctx, er, item.ID, item.Title) {
				continue
			}
			checker := l.getPRPendingCheckerFactory()(item.RepoPath)
			merged, mergedErr := checker.IsPRMerged(ctx, item.PrNumber)
			if mergedErr != nil {
				log.Debug("[BacklogLifecycle] reconcileBouncingItems IsPRMerged failed", "item", item.ID, "pr", item.PrNumber, "error", mergedErr)
			} else if merged {
				// reader:backlog: the scalar PR merging is not enough while another PR of the
				// item's set is open or unknown; re-evaluated next pass, not flagged bouncing.
				if hold, why := l.otherPRsHoldItem(ctx, er, item.ID); hold {
					log.Info("[BacklogLifecycle] reconcileBouncingItems: tracked PR merged but the item PR set still holds it", "item", item.ID, "pr", item.PrNumber, "reason", why)
					continue
				}
				// Story 6 guard (adversarial-review.md's Blocker): re-verify,
				// via a live GitHub lookup, that PR #item.PrNumber's head
				// branch still matches this item's currently-tracked branch
				// before auto-completing it on the strength of item.PrNumber
				// alone. Fails closed identically to
				// verifyPRAssociationForFixSpawn's own contract.
				if !l.verifyPRAssociationForFixSpawn(ctx, item.ID, item.RepoPath, item.PrNumber) {
					log.Warn("[BacklogLifecycle] reconcileBouncingItems: PR head branch no longer verifiably matches the tracked branch — skipping auto-done transition (was this item's PR attached via report_pr_created's override_reason path?)", "item", item.ID, "pr", item.PrNumber)
					continue
				}
				if l.holdMergedItemForLiveWork(ctx, er, item.ID, item.Title) {
					continue
				}
				summary := fmt.Sprintf("Auto-verified by reconcileBouncingItems: PR #%d for this item is confirmed merged on GitHub, so the item's rework cycle is treated as converged rather than bouncing.", item.PrNumber)
				if transErr := l.transitionBouncingItemToDone(ctx, item, summary); transErr != nil {
					log.Warn("[BacklogLifecycle] reconcileBouncingItems done transition failed", "item", item.ID, "error", transErr)
					// The PR is already confirmed merged — the item is left
					// bouncing between in_progress/review with nothing else
					// surfacing this until the next tick retries it.
					l.notifyTransitionFailed(item.ID, item.Title, fmt.Sprintf("PR #%d was confirmed merged but the item's transition to done failed", item.PrNumber), transErr)
				} else {
					log.Info("[BacklogLifecycle] reconcileBouncingItems: item → done (PR already merged)", "item", item.ID, "pr", item.PrNumber)
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
				summary := fmt.Sprintf("Auto-verified by reconcileBouncingItems: this item's most recent work-session commit (%s) is confirmed shipped to %s without ever going through a PR, so the item's rework cycle is treated as converged rather than bouncing.", sha, domain.BounceMainBranch)
				if transErr := l.transitionBouncingItemToDone(ctx, item, summary); transErr != nil {
					log.Warn("[BacklogLifecycle] reconcileBouncingItems done transition (shipped without PR) failed", "item", item.ID, "error", transErr)
					// The commit is already confirmed shipped to main — same
					// silent-stranding risk as the merged-PR branch above.
					l.notifyTransitionFailed(item.ID, item.Title, fmt.Sprintf("commit %s was confirmed shipped to %s but the item's transition to done failed", sha, domain.BounceMainBranch), transErr)
				} else {
					log.Info("[BacklogLifecycle] reconcileBouncingItems: item → done (commit shipped without a PR)", "item", item.ID, "commit", sha, "branch", domain.BounceMainBranch)
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
			log.Warn("[BacklogLifecycle] reconcileBouncingItems CountReviewCyclesSince failed", "item", item.ID, "error", countErr)
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
			log.Warn("[BacklogLifecycle] reconcileBouncingItems GetRecentReviewVerdictSummaries failed", "item", item.ID, "error", verdictErr)
		}
		var latestOutcome, latestSummary string
		if len(recentVerdicts) > 0 {
			latestOutcome = recentVerdicts[0].OverallOutcome
			latestSummary = recentVerdicts[0].Summary
		}
		hasPass := latestOutcome == string(session.ReviewOutcomePass)

		if !stuck.IsBouncing(count, cycleThreshold, hasPass) {
			continue
		}

		reasonDetail := fmt.Sprintf("bounced in_progress<->review %d times in the last %s with no PASS verdict", count, cycleLookback)
		if latestOutcome != "" {
			// sanitizeField at 500 matches the existing convention for
			// rendering a ReviewVerdict.Summary into operator/agent-facing
			// text (see backlog_context.go, backlog_review.go).
			reasonDetail = fmt.Sprintf("%s (most recent verdict: %s — %s)", reasonDetail, latestOutcome, domain.SanitizeField(latestSummary, 500))
		}

		applied, markErr := er.MarkStuck(ctx, item.ID, domain.StuckReasonBouncing, session.BacklogStatus(item.Status), reasonDetail)
		if markErr != nil {
			log.Warn("[BacklogLifecycle] reconcileBouncingItems MarkStuck failed", "item", item.ID, "error", markErr)
			continue
		}
		if !applied {
			continue
		}
		rows, findErr := er.FindOpenStuckStates(ctx)
		if findErr != nil {
			log.Warn("[BacklogLifecycle] reconcileBouncingItems FindOpenStuckStates failed", "item", item.ID, "error", findErr)
			continue
		}
		row, ok := backlogdto.FindOpenStuckStateFor(rows, item.ID, domain.StuckReasonBouncing)
		if !ok || row.NotifiedAt != nil {
			continue
		}
		log.Warn("[BacklogLifecycle] item bouncing, no PASS", "item", item.ID, "cycles", count, "lookback", cycleLookback)
		notifyBody := fmt.Sprintf("%s — bounced between in_progress and review %d times in the last %s with no PASS verdict. It may be stuck in a non-converging rework loop.", item.Title, count, cycleLookback)
		if latestOutcome != "" {
			notifyBody = fmt.Sprintf("%s Most recent verdict: %s — %s", notifyBody, latestOutcome, domain.SanitizeField(latestSummary, 500))
		}
		l.notify(item.ID,
			"Item is thrashing between work and review",
			notifyBody,
			8,           // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			false, true, // urgent, important — non-converging loop matters, but not a drop-everything alert
		)
		if _, notifyErr := er.MarkStuckNotified(ctx, item.ID, domain.StuckReasonBouncing); notifyErr != nil {
			log.Warn("[BacklogLifecycle] reconcileBouncingItems MarkStuckNotified failed", "item", item.ID, "error", notifyErr)
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
//	autonomous_stuck   -> anchor {in_progress, review}  resolve ONLY on leaving both (never mid-cycle)
//	orphaned_triage    -> anchor {idea}                 resolve when status not in anchor
//	verdict_undelivered -> anchor {review}              resolve when status not in anchor
//
// autonomous_stuck shares bouncing's two-status anchor rather than a single
// {in_progress} one: onAutonomousDriverComplete's SessionRoleWork turn-cap-stop
// path can force an in_progress->review transition while the item is still
// genuinely stuck (the exact false-resolve risk the original PR #200
// investigation ruled out for a naive {in_progress} anchor — see below), so
// review has to stay IN the anchor, not outside it, for the same reason
// bouncing already treats it that way. Added here because resolveAutonomousStuck's
// point-of-success call (server/services/autonomous_orchestration_service.go) and
// the blanket terminal rule above only cover two of the row's actual close paths —
// confirmed live on item a0fb3466: its autonomous_stuck row stayed open after the
// item cycled all the way back to "ready" (via its abandoned_review resolving,
// not a driver success), a transition neither of those two paths observes.
//
// push_failed and rework_cap have no non-terminal anchor at all — before the
// item reaches done/archived they stay open, relying entirely on the blanket
// terminal rule above (or, for push_failed, its own faster event-driven
// resolution path — see resolveToPRPending — which typically resolves the row
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
func (l *BacklogLifecycleListener) selfHealStuck(ctx context.Context, er *session.EntRepository) {
	open, err := er.FindOpenStuckStates(ctx)
	if err != nil {
		log.Warn("[BacklogLifecycle] selfHealStuck FindOpenStuckStates error", "error", err)
		return
	}
	for _, row := range open {
		if session.IsTerminalStatus(row.ItemStatus) {
			// Blanket terminal rule — see doc comment above. An item that has
			// truly finished has nothing left needing operator attention,
			// regardless of which reason its stuck row is for.
			l.resolveStuckLogged(ctx, er, row.ItemID, row.Reason, "selfHealStuck/terminal")
			continue
		}
		resolve := false
		switch row.Reason {
		case domain.StuckReasonPRReadyUnmerged:
			resolve = row.ItemStatus != session.BacklogStatusPRPending
		case domain.StuckReasonAbandonedReview:
			resolve = row.ItemStatus != session.BacklogStatusReview
		case domain.StuckReasonVerdictUndelivered:
			// Point-of-success resolve is resolveVerdictUndeliveredLogged
			// (server/services/backlog_service_verdict_steer.go); this is the
			// backstop for every other way the item can leave review (ships,
			// reopens for rework, is manually moved, etc.) without that call
			// ever firing.
			resolve = row.ItemStatus != session.BacklogStatusReview
		case domain.StuckReasonStaleWork:
			resolve = row.ItemStatus != session.BacklogStatusInProgress
		case domain.StuckReasonBouncing, domain.StuckReasonBounceCapExhausted:
			// bounce_cap_exhausted (Signal 2, plan.md Epic 1.3) can only ever
			// coexist with an open bouncing row, so it shares bouncing's exact
			// non-terminal anchor — this is a backstop for any status
			// transition that bypasses reconcileBouncingItems' explicit
			// resolve-alongside-bouncing call sites above.
			resolve = row.ItemStatus != session.BacklogStatusInProgress && row.ItemStatus != session.BacklogStatusReview
		case domain.StuckReasonAutonomousStuck:
			// Shares bouncing's two-status anchor, not a single {in_progress}
			// one — see this function's doc comment for why review must stay
			// IN the anchor (a turn-cap-stop can force in_progress->review
			// while genuinely still stuck) and for the live item (a0fb3466)
			// this backstops: a row left open after the item cycled all the
			// way back to "ready" through a path resolveAutonomousStuck's
			// point-of-success call never observes.
			resolve = row.ItemStatus != session.BacklogStatusInProgress && row.ItemStatus != session.BacklogStatusReview
		case domain.StuckReasonOrphanedTriage:
			// Generalized 2026-08-03 to also anchor at queued (see
			// reconcileOrphanedTriageItems' doc comment): a retry that
			// successfully re-triages moves the item queued->idea->ready,
			// landing outside both anchor statuses, so this still resolves
			// correctly on genuine success. A retry still in flight (queued
			// reset to idea) or a repeat failure (back to idea) keeps the row
			// open, matching the pre-existing idea-only behavior.
			resolve = row.ItemStatus != session.BacklogStatusIdea && row.ItemStatus != session.BacklogStatusQueued
		case domain.StuckReasonPlanNotApproved:
			resolve = row.ItemStatus != session.BacklogStatusQueued
		case domain.StuckReasonPRPendingNoPR:
			resolve = row.ItemStatus != session.BacklogStatusPRPending
		case domain.StuckReasonPRNeedsFix, domain.StuckReasonMergedPRUnverified:
			resolve = row.ItemStatus != session.BacklogStatusPRPending
		case domain.StuckReasonRepeatedNoopDispatch:
			// Commit-landed resolution is reconcileRepeatedNoopDispatch's
			// else-branch (same-status, invisible to this sweep).
			resolve = row.ItemStatus != session.BacklogStatusInProgress && row.ItemStatus != session.BacklogStatusReview
		case domain.StuckReasonTransitionDenied:
			resolve = !session.TransitionDenialAnchored(row)
		case domain.StuckReasonSpawnFailed:
			// Only the dequeue-backoff use is anchored; the review-reopen use (item
			// stranded in_progress) keeps waiting for its own event site. in_progress
			// stays in the anchor: it is the transient claim state of a retry, and
			// resolving there would erase the backoff count. Rows written by #1460 lack the
			// prefix until the next failed retry rewrites the context (MarkStuck's upsert), so they
			// are skipped here only until then.
			if !domain.IsDequeueSpawnFailedContext(row.Context) {
				continue
			}
			resolve = row.ItemStatus != session.BacklogStatusReady && row.ItemStatus != session.BacklogStatusQueued &&
				row.ItemStatus != session.BacklogStatusInProgress
		default:
			// push_failed, rework_cap, multiple_reasons, and any future
			// reason with no non-terminal anchor: stays open until the
			// blanket terminal rule above catches it, or its own
			// event-site resolves it first. multiple_reasons specifically is
			// resolved by its own detector (reconcileMultiReasonEscalation's
			// de-escalate branch), not a status-anchor case here — its
			// "resolved" condition is "count of other open reasons dropped
			// below stuck.MultiReasonThreshold", which has no single item-status
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
// item and, for any item with stuck.MultiReasonThreshold or more simultaneously
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
// (stuck.MultiReasonEscalationNotifyReady, keyed off the row's FirstDetectedAt) so
// a single-tick threshold crossing doesn't notify immediately. De-escalation
// (ResolveStuck) is NOT dwell-gated — it fires the same tick the count first
// drops below threshold (see plan.md Pattern Decisions' "Flap control"
// rows and Unresolved Questions for why no hysteresis is applied here yet).
// Best-effort throughout: errors are logged, never returned.
func (l *BacklogLifecycleListener) reconcileMultiReasonEscalation(ctx context.Context, er *session.EntRepository) {
	open, err := er.FindOpenStuckStates(ctx)
	if err != nil {
		log.Warn("[BacklogLifecycle] reconcileMultiReasonEscalation FindOpenStuckStates error", "error", err)
		return
	}

	byItem := make(map[string][]backlogdto.OpenStuckStateData)
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
	nonEscalation      []backlogdto.OpenStuckStateData
	bouncingRow        backlogdto.OpenStuckStateData
	hasBouncing        bool
	hasAbandonedReview bool
}

// categorizeOpenStuckRows partitions an item's open stuck-state rows into
// the ones eligible to count toward multi-reason escalation (excluding
// StuckReasonMultipleReasons and StuckReasonBounceCapExhausted, which are
// derived signals rather than independent reasons, and the waiting reasons
// isWaitingStuckReason names) and tracks whether a
// bouncing/abandoned-review pair is present for the structural-coupling
// exclusion below.
func categorizeOpenStuckRows(rows []backlogdto.OpenStuckStateData) multiReasonRowSet {
	var set multiReasonRowSet
	for _, row := range rows {
		if row.Reason == domain.StuckReasonMultipleReasons || row.Reason == domain.StuckReasonBounceCapExhausted {
			continue
		}
		if isWaitingStuckReason(row.Reason) {
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

// isWaitingStuckReason reports whether reason only says "something else is
// already acting or the item awaits a merge" — respawn_blocked_active (its own
// notification is INFO, "no action needed") and pr_ready_unmerged — so it
// never counts toward multi-reason escalation. Both co-occur on every healthy
// item whose PR is waiting on review, which escalated them to ERROR.
func isWaitingStuckReason(reason domain.StuckReason) bool {
	return reason == domain.StuckReasonRespawnBlockedActive || reason == domain.StuckReasonPRReadyUnmerged
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
func excludeStructurallyCoupledAbandonedReview(set multiReasonRowSet) []backlogdto.OpenStuckStateData {
	if !set.hasBouncing || !set.hasAbandonedReview {
		return set.nonEscalation
	}
	switch session.EvaluateRemediation(set.bouncingRow, time.Now(), session.ServerStartTime) {
	case session.RemediationSkippedParked, session.RemediationSkippedNotDue:
		filtered := make([]backlogdto.OpenStuckStateData, 0, len(set.nonEscalation))
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
func (l *BacklogLifecycleListener) deescalateMultiReasonIfNeeded(ctx context.Context, er *session.EntRepository, itemID string, hasExistingRow bool, openReasonsCount int) bool {
	if stuck.IsMultiReasonEscalated(openReasonsCount) {
		return false
	}
	if hasExistingRow {
		if _, resolveErr := er.ResolveStuck(ctx, itemID, domain.StuckReasonMultipleReasons); resolveErr != nil {
			log.Warn("[BacklogLifecycle] reconcileMultiReasonEscalation ResolveStuck failed", "item", itemID, "error", resolveErr)
		} else {
			log.Info("[BacklogLifecycle] de-escalated", "item", itemID, "open_reasons", openReasonsCount)
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
func (l *BacklogLifecycleListener) notifyMultiReasonEscalationIfReady(ctx context.Context, er *session.EntRepository, itemID, itemTitle, contextString string, nonEscalationCount int, existingRow backlogdto.OpenStuckStateData, hasExistingRow bool) {
	if !hasExistingRow {
		return
	}
	if existingRow.NotifiedAt != nil {
		return
	}
	if !stuck.MultiReasonEscalationNotifyReady(existingRow.FirstDetectedAt, time.Now()) {
		return
	}
	l.notify(itemID,
		"Multiple stuck reasons open",
		fmt.Sprintf("%s — %d stuck reasons currently open simultaneously (%s). This combination is a stronger signal than any single reason alone.", itemTitle, nonEscalationCount, contextString),
		7,          // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
		true, true, // urgent, important
	)
	if _, notifyErr := er.MarkStuckNotified(ctx, itemID, domain.StuckReasonMultipleReasons); notifyErr != nil {
		log.Warn("[BacklogLifecycle] reconcileMultiReasonEscalation MarkStuckNotified failed", "item", itemID, "error", notifyErr)
	}
}

// reconcileMultiReasonEscalationForItem applies the multi-reason escalation
// decision (de-escalate, leave alone, or escalate + notify) for a single
// item's open stuck-state rows. Split out of reconcileMultiReasonEscalation
// to keep the per-item decision tree — categorize, exclude structurally
// coupled rows, de-escalate-or-escalate, notify — independently readable and
// under the complexity gate.
func (l *BacklogLifecycleListener) reconcileMultiReasonEscalationForItem(ctx context.Context, er *session.EntRepository, itemID string, rows []backlogdto.OpenStuckStateData) {
	set := categorizeOpenStuckRows(rows)
	nonEscalation := excludeStructurallyCoupledAbandonedReview(set)

	existingRow, hasExistingRow := backlogdto.FindOpenStuckStateFor(rows, itemID, domain.StuckReasonMultipleReasons)

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
		log.Warn("[BacklogLifecycle] reconcileMultiReasonEscalation MarkStuck failed", "item", itemID, "error", markErr)
		return
	}
	if !applied {
		return
	}
	log.Info("[BacklogLifecycle] escalated", "item", itemID, "open_reasons", len(nonEscalation))

	l.notifyMultiReasonEscalationIfReady(ctx, er, itemID, rows[0].ItemTitle, contextString, len(nonEscalation), existingRow, hasExistingRow)
}

// hasActiveSession reports whether any of the provided ItemSessions is an
// open (not yet ended) work-, review-, or Jules-role session. Package-local
// equivalent of server/services' hasActiveWorkSession/hasActiveReviewSession
// (not reusable directly — that package imports session, not the other way
// around) used by recoverDriftedPRItem/reconcileDriftedPRItems to avoid
// stealing an item away from a still-legitimately-running session, mirroring
// AutoReopenForPRFix's/AutoRespawnReview's identical guard.
func hasActiveSession(sessions []backlogdto.ItemSessionSummary) bool {
	for _, s := range sessions {
		if s.EndedAt == nil && (s.Role == session.SessionRoleWork || s.Role == session.SessionRoleReview) {
			return true
		}
	}
	return session.HasActiveJulesSession(sessions)
}
