package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

// StaleWorkRemediator can clean up and respawn an in_progress backlog item
// whose active work session has gone stale (StuckReasonStaleWork — no
// progress reported for over maxWorkSessionStaleness) but is NOT a zombie:
// the underlying tmux session and pane process are still alive
// (Instance.TmuxAlive/PaneProcessDead), so the generic tmux health check
// never flags it — the agent inside simply finished its own work and is
// idle at an interactive prompt instead of properly closing out. Before this
// existed, reconcileStaleWorkSessions was detection-only (MarkStuck +
// notify), so such an item sat "in_progress" forever once the agent went
// idle (docs/tasks/backlog-stuck-item-auto-remediation.md Phase B; live
// repro 2026-07-20, item 9264efe7-b4c2-455a-9e2a-ab0196a63ecd, rework suffix
// -r14). Implemented outside this package (BacklogService owns the live
// Instance registry needed to kill the stale tmux pane) and wired via
// SetStaleWorkRemediator, same pattern as AutoReopenSpawner/PRFixSpawner.
type StaleWorkRemediator interface {
	// RemediateStaleWorkSession ends the item's current stale work session
	// (killing its tmux pane but keeping the worktree so uncommitted work
	// survives) and spawns a fresh one with a new turn budget. No-op (nil
	// error) if the item already moved off in_progress or its work session
	// already ended by the time this runs.
	RemediateStaleWorkSession(ctx context.Context, itemID string) error
}

// ReviewRespawner can automatically re-trigger the review gate for a backlog
// item stuck in review with no active session in flight (the
// StuckReasonAbandonedReview condition — see markAbandonedReview). Before this
// existed, such items were detected and notified but nothing ever respawned
// work on them, so they sat forever until a human noticed (see
// docs/tasks/backlog-feature-improvement.md, 2026-07-17 update — 4 real items
// went stale this way, several with nearly all acceptance criteria already
// marked complete, just never actually re-reviewed).
type ReviewRespawner interface {
	AutoRespawnReview(ctx context.Context, itemID string) error
}

// OneShotShipRunner runs a one-shot LLM prompt against a session's worktree,
// returning the PR URL the prompt produced (or "" if none was found in its
// output). Defined here — the consumer — per this repo's anti-interface-
// pollution convention (.claude/rules/interface-pollution-checklist.md);
// *services.SessionService satisfies it via RunOneShotForSession, wired in
// production via SetOneShotShipRunner from server/dependencies.go. Mirrors
// services.PRRunner (server/services/backlog_service_ship.go), which the same
// method also satisfies for the manual "Ship PR" self-service action —
// intentionally not shared/exported from that package, since importing it
// here would pull server/services (which imports this package) into an
// import cycle.
//
// Used by shipViaAgentOrFallback to close the gap flagged in PR #189's
// "deliberately out of scope" section: when the work session that earned a
// PASS verdict has already exited, the only PR-creation mechanism available
// was pushAndCreatePR's mechanical `git push` + `gh pr create` — no CI
// reaction, no merge-conflict resolution. RunOneShotForSession lets us run
// the same agent-driven ship flow a still-live session would have run itself
// (see /backlog/ship's ship.md, which drives /github:pr-ship) as a headless
// one-shot against the ended session's worktree — it only needs the
// session's Instance/worktree to still be resolvable, not a live tmux
// process (see RunOneShot's use of findInstance + GetEffectiveRootDir).
type OneShotShipRunner interface {
	RunOneShotForSession(ctx context.Context, sessionID, prompt string, timeoutSeconds int32) (string, error)
}

// agentShipPrompt is the one-shot prompt used by shipViaAgentOrFallback.
// Deliberately NOT the same literal as services.shipPRPrompt
// (server/services/backlog_service_ship.go) — that prompt is a plain-English
// "create a PR" ask with no conflict-resolution or CI-reaction instructions,
// fine for its own use case (a human clicking "Ship PR" on a review-status
// item that hasn't necessarily finished /backlog/review's protocol). Here we
// are resuming exactly the step a still-live work session would have taken
// next per taskProtocolBlock rules 8-9 (PASS -> run /backlog/ship), so we
// invoke that same slash command directly: WriteSlashCommands
// (session/backlog_commands.go) already wrote ship.md into this worktree at
// session-spawn time, and nothing cleans it up before the item leaves
// "review" (CleanupSlashCommands is not wired to fire on review exit — see
// its call sites), so it is still present. ship.md's own instructions run
// /github:pr-ship (local CI, code review, remote CI, and actual
// merge-conflict resolution — the whole reason this path was added) and
// already special-case "review already returned PASS" by skipping the
// redundant re-review step, exactly matching the state we call this in.
const agentShipPrompt = "/backlog/ship"

// oneShotShipTimeoutSeconds bounds shipViaAgentOrFallback's one-shot call.
// Set to the RunOneShot handler's own hard ceiling (server/services/
// session_service.go clamps TimeoutSeconds to max 1800s) rather than
// TriggerShipPR's shorter 900s: /github:pr-ship does more work than
// services.shipPRPrompt's plain PR-creation ask (it also waits on CI and
// resolves merge conflicts), so it needs more headroom, and this path runs
// unattended — there's no human waiting on an RPC response to bound it.
const oneShotShipTimeoutSeconds = 1800

// SessionArchiver soft-archives a session by UUID so it stops accumulating in the
// default session list. Implemented by server/services.SessionService (it owns the
// live in-memory Instance registry archival must go through — see ArchivedAt's
// doc comment on session.Instance); wired via SetSessionArchiver from
// server/dependencies.go, same pattern as SetNotifier/SetSessionCreator below.
// Used by the archive_terminal_sessions detector in ReconcileStuck as a periodic
// safety net for work sessions belonging to backlog items that reached done/archived
// without their sessions being archived by the (also newly added) transition hook —
// e.g. pre-existing terminal items from before this detector existed, or a race/crash
// mid-transition. Nil-safe: the detector no-ops when unset.
type SessionArchiver interface {
	// ArchiveSessionByUUID soft-archives the session, if found and not already
	// archived. No-op (not an error) if the session is not tracked.
	ArchiveSessionByUUID(ctx context.Context, sessionUUID string) error
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

// maxDoneAge is how long a backlog item remains in "done" status before the
// auto_archive_done detector (see archiveStaleDoneItems) transitions it to
// "archived". A fixed constant rather than a Settings/Defaults config knob
// (unlike e.g. MaxConcurrentBacklogWorkItems) — this matches the literal
// requirement ("archive 3 days after done") without adding configuration
// surface nothing has asked for; promote to a per-deployment setting if a
// real need for tuning this ever shows up.
const maxDoneAge = 3 * 24 * time.Hour

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

	// reviewRespawnMu guards reviewRespawner for concurrent Set/get access.
	reviewRespawnMu sync.RWMutex
	reviewRespawner ReviewRespawner

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

// Shutdown cancels in-flight review gate calls. Safe to call concurrently.
func (l *BacklogLifecycleListener) Shutdown() {
	if l.shutdownCancel != nil {
		l.shutdownCancel()
	}
}

// newListenerBase initialises fields common to all BacklogLifecycleListener constructors.
// pipelineEngine may be nil — see the field's doc comment for the fallback behavior.
func newListenerBase(storage *Storage, pipelineEngine PipelineEngine) *BacklogLifecycleListener {
	ctx, cancel := context.WithCancel(context.Background())
	l := &BacklogLifecycleListener{
		storage:                 storage,
		pipelineEngine:          pipelineEngine,
		reviewSem:               make(chan struct{}, maxConcurrentReviewGates),
		shutdownCtx:             ctx,
		shutdownCancel:          cancel,
		prPendingCheckerFactory: defaultPRPendingCheckerFactory,
		prCreatorFactory:        defaultPRCreatorFactory,
		branchReconciler:        git.MergeMainIntoWorktree,
	}
	l.runner = NewReviewGateRunner(storage, l.getAutoReopener, l.getNotifier, l.getSessionCreator, pipelineEngine)
	return l
}

// NewBacklogLifecycleListener creates a listener backed by the given storage.
// The review gate is disabled (sessionCreator=nil, headlessPool=nil). No PipelineEngine
// is wired (nil) — callers needing one should use NewBacklogLifecycleListenerWithPool.
func NewBacklogLifecycleListener(storage *Storage) *BacklogLifecycleListener {
	return newListenerBase(storage, nil)
}

// NewBacklogLifecycleListenerWithSpawner creates a listener that will spawn a
// review gate session when a work session exits and SkipReviewGate is false.
func NewBacklogLifecycleListenerWithSpawner(storage *Storage, spawner ReviewGateSpawner) *BacklogLifecycleListener {
	l := newListenerBase(storage, nil)
	l.SetSessionCreator(spawner)
	return l
}

// NewBacklogLifecycleListenerWithPool creates a listener that uses a headless.Pool
// for review gate calls instead of spawning a tmux session. pipelineEngine is the
// shared PipelineEngine instance (Epic 1.5, Story 1.5.1) — pass nil to fall back to
// the built-in default pipeline for every item.
func NewBacklogLifecycleListenerWithPool(storage *Storage, pool *headless.Pool, pipelineEngine PipelineEngine) *BacklogLifecycleListener {
	l := newListenerBase(storage, pipelineEngine)
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

	// The item is leaving in_progress — any open stale_work row is stale by
	// definition now. Resolve immediately rather than waiting for the
	// self-heal sweep's next tick (Task 2.1.5a).
	if er, ok := l.storage.repo.(*EntRepository); ok {
		l.resolveStuckLogged(ctx, er, item.ID, domain.StuckReasonStaleWork, "onSessionExited")
	}

	log.InfoLog.Printf("[BacklogLifecycle] item %s transitioned to %s (session %s exited)", item.ID, toStatus, sessionUUID)

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

// handleReviewSessionExited processes the outcome of a review session (Role ==
// SessionRoleReview) that has just exited. Review now always happens in a real,
// hidden session.Instance (see ReviewGateRunner.Run / SpawnReviewSession)
// instead of a synchronous in-process headless LLM call, so the verdict — if
// any — was submitted via the submit_review_verdict MCP tool while the review
// session was running (see server/mcp/tools_backlog.go) and is read back here
// from storage rather than computed inline.
//
// forcePush controls the PASS branch's behavior when the work session that
// earned the verdict is still alive (EndedAt nil): false (the normal,
// real-time exit-event path — see onSessionExited below) defers to that live
// session, which is expected to discover the PASS verdict on its own next
// poll and ship the PR itself via /backlog/ship (see taskProtocolBlock rules
// 8-9). true (used only by reconcileUnprocessedReviewVerdicts, the
// crash-recovery sweep for a review session that died before this function
// ever ran for it normally) routes to shipViaAgentOrFallback regardless of
// work-session liveness — that sweep cannot tell a genuinely-live,
// still-polling work session apart from a zombie that will never poll again,
// and its whole reason to exist is to make forward progress on a verdict
// nothing else is going to act on. See
// TestReconcileUnprocessedReviewVerdicts_should_applyPassVerdict_When_ReviewSessionDiedButWorkSessionStillAlive
// — that test's fixture has no worktree recorded at all, so it exercises
// shipViaAgentOrFallback -> pushAndCreatePR's pre-existing, unchanged
// fallbackToDone("no worktree") branch; this fix does not touch that branch.
func (l *BacklogLifecycleListener) handleReviewSessionExited(ctx context.Context, reviewIS ItemSessionSummary, forcePush bool) {
	item, err := l.storage.GetBacklogItem(ctx, reviewIS.BacklogItemID)
	if err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] handleReviewSessionExited GetBacklogItem item=%s: %v", reviewIS.BacklogItemID, err)
		return
	}

	// ListItemSessions (unlike GetItemSessionBySessionUUID, used by the caller)
	// eagerly loads the ReviewVerdict edge, which is what we need here.
	sessions, err := l.storage.ListItemSessions(ctx, item.ID)
	if err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] handleReviewSessionExited ListItemSessions item=%s: %v", item.ID, err)
		return
	}

	// Scan oldest-first: find the review ItemSession matching this exited
	// session, and keep overwriting workEntry so it ends up as the most recent
	// work session — the one whose worktree needs to be pushed on a PASS verdict.
	var reviewEntry *ItemSessionSummary
	var workEntry *ItemSessionSummary
	for i := range sessions {
		s := &sessions[i]
		if s.SessionUUID == reviewIS.SessionUUID && s.Role == SessionRoleReview {
			reviewEntry = s
		}
		if s.Role == SessionRoleWork {
			workEntry = s
		}
	}

	if reviewEntry == nil || reviewEntry.ReviewVerdict == nil {
		// The review session exited without ever calling submit_review_verdict —
		// crashed, killed, ran out of turns, etc. Treat it like a failed review so
		// the item doesn't sit stuck in "review" forever.
		log.WarningLog.Printf("[BacklogLifecycle] handleReviewSessionExited item=%s review session %s exited without a verdict", item.ID, reviewIS.SessionUUID)
		l.notify(item.ID,
			"Review session ended without a verdict",
			fmt.Sprintf("%s — the review session exited without calling submit_review_verdict. Treating as a failed review.", item.Title),
			7, // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
			3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		)
		l.autoReopenWithBackoffGate(ctx, item.ID, item.Title)
		return
	}

	verdict := reviewEntry.ReviewVerdict
	overall := ReviewOutcome(verdict.OverallOutcome)
	perCriterion, parseErr := parsePerCriterionVerdicts(verdict.PerCriterion)
	if parseErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] handleReviewSessionExited parsePerCriterionVerdicts item=%s: %v", item.ID, parseErr)
	}
	acSnapshot, _ := ParseAcCriteria(reviewIS.AcSnapshot)
	applyVerdictsToACs(ctx, l.storage, item, acSnapshot, perCriterion)

	log.InfoLog.Printf("[BacklogLifecycle] handleReviewSessionExited item=%s outcome=%s (review session %s)", item.ID, overall, reviewIS.SessionUUID)

	switch overall {
	case ReviewVerdictFail, ReviewVerdictPartial, ReviewVerdictUnverifiable:
		l.autoReopenWithBackoffGate(ctx, item.ID, item.Title)
	case ReviewVerdictPass:
		if workEntry == nil {
			log.ErrorLog.Printf("[BacklogLifecycle] handleReviewSessionExited item=%s: PASS verdict but no work session found — cannot push", item.ID)
			return
		}
		if workEntry.EndedAt == nil && !forcePush {
			// The work session that produced this PASS is still alive — it stays
			// running and polls get_backlog_item/backlog status after request_review
			// (see taskProtocolBlock rules 8-9, session/backlog_context.go). Per those
			// rules it will discover this PASS verdict on its next poll and run
			// /backlog/ship itself, which drives /github:pr-ship end to end (local CI,
			// code review, remote CI, and — unlike the mechanical push below — actual
			// merge-conflict resolution and reaction to failing checks). Leave the item
			// in review and let the live agent drive shipping; do not race it with the
			// mechanical push path. Mirrors AutoReopenAfterFailedReview's identical
			// hasActiveWorkSession guard on the FAIL/PARTIAL side of this same loop.
			log.InfoLog.Printf("[BacklogLifecycle] handleReviewSessionExited item=%s: PASS verdict with a live work session (%s) — leaving PR creation to the agent via /backlog/ship instead of the mechanical push path", item.ID, workEntry.SessionUUID)
			return
		}
		// Reached when either the work session that earned this PASS already exited
		// (crashed, was killed, or hit a turn cap — nothing will ever run
		// /backlog/ship for this item on its own) or forcePush is set
		// (reconcileUnprocessedReviewVerdicts' crash-recovery sweep, which cannot
		// distinguish a genuinely-live work session from a zombie). Ship the PR —
		// see shipViaAgentOrFallback's doc comment for the agent-driven-first,
		// mechanical-push-as-backstop policy.
		l.shipViaAgentOrFallback(ctx, item, *workEntry)
	}
}

// autoReopenWithBackoffGate dispatches AutoReopenAfterFailedReview through the
// shared remediation backoff gate (Storage.RemediationDue,
// session/backlog_remediation.go) — the "bouncing" reason's remediation
// action per docs/tasks/backlog-stuck-item-auto-remediation.md Phase A.
// Called on every failed/verdict-less review exit, same trigger points as
// before this gate existed; the gate itself is what makes repeated calls in
// rapid succession (the exact 2026-07-19 incident shape) stop consuming a
// fresh attempt every few minutes once a "bouncing" BacklogStuckState row is
// open. When no such row exists yet (this reason hasn't been detected as
// stuck), RemediationDue reports due=true unconditionally — the first few
// reopen attempts, before reconcileBouncingItems' bounceThreshold trips,
// behave exactly as they did before this gate existed. Best-effort: gate
// query/write errors are logged, never returned, and fail OPEN (still
// attempts the reopen) rather than silently stranding the item.
func (l *BacklogLifecycleListener) autoReopenWithBackoffGate(ctx context.Context, itemID, itemTitle string) {
	reopener := l.getAutoReopener()
	if reopener == nil {
		return
	}

	due, justParked, gateErr := l.storage.RemediationDue(ctx, itemID, domain.StuckReasonBouncing)
	if gateErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] autoReopenWithBackoffGate RemediationDue item=%s: %v", itemID, gateErr)
		due = true // fail open — see doc comment above
	}
	if justParked {
		l.notify(itemID,
			"Auto-rework paused",
			fmt.Sprintf("%s — automated rework has been retried %d times over an extended period without resolving. It now needs manual attention; use Reset to try again automatically.", itemTitle, MaxRemediationAttempts),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		)
	}
	if !due {
		log.InfoLog.Printf("[BacklogLifecycle] autoReopenWithBackoffGate item=%s: bouncing remediation backoff not yet due, skipping auto-reopen", itemID)
		return
	}

	go func() {
		if err := reopener.AutoReopenAfterFailedReview(ctx, itemID); err != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] autoReopenWithBackoffGate AutoReopenAfterFailedReview item=%s: %v", itemID, err)
		}
	}()
}

// TriggerReviewForSession immediately spawns a review gate for the work session
// identified by workSessionUUID. Used by the autonomous driver to trigger review
// as soon as the driver signals DONE, rather than waiting for ReconcileStuck.
// No-op if the listener is disabled or no review mechanism is configured.
func (l *BacklogLifecycleListener) TriggerReviewForSession(workSessionUUID string) {
	if !l.enabled.Load() {
		return
	}
	if l.getSessionCreator() == nil {
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
	er, ok := l.storage.repo.(*EntRepository)
	if !ok {
		return
	}

	seeded := 0

	// abandoned_review: review-status items with a verdict on record but
	// nothing active in flight.
	reviewItems, err := er.FindStuckReviewItems(ctx)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] BackfillStuckStates FindStuckReviewItems error: %v", err)
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
		log.WarningLog.Printf("[BacklogLifecycle] BackfillStuckStates ListBacklogItems error: %v", err)
	} else {
		for _, item := range inProgressItems {
			sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
			if sessErr != nil {
				log.WarningLog.Printf("[BacklogLifecycle] BackfillStuckStates ListItemSessions item=%s: %v", item.ID, sessErr)
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

	log.InfoLog.Printf("[BacklogLifecycle] BackfillStuckStates: seeded %d stuck row(s) at startup", seeded)
}

// backfillMarkAndNotify marks a stuck row and immediately pre-sets
// notified_at so the first genuine reconcile tick after startup does not
// re-notify for a condition already known before the restart. Returns
// whether a row was newly opened by this call. Best-effort: errors are
// logged, never returned.
func (l *BacklogLifecycleListener) backfillMarkAndNotify(ctx context.Context, er *EntRepository, itemID string, reason domain.StuckReason, expectedStatus BacklogStatus, stuckContext string) bool {
	applied, err := er.MarkStuck(ctx, itemID, reason, expectedStatus, stuckContext)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] BackfillStuckStates MarkStuck item=%s reason=%s: %v", itemID, reason, err)
		return false
	}
	if !applied {
		return false
	}
	if _, notifyErr := er.MarkStuckNotified(ctx, itemID, reason); notifyErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] BackfillStuckStates MarkStuckNotified item=%s reason=%s: %v", itemID, reason, notifyErr)
	}
	return true
}

// archiveStaleDoneItems is the auto_archive_done detector: it finds backlog
// items that have been in "done" status for longer than maxDoneAge (measured
// from the most recent transition into "done" — see FindDoneItemsOlderThan's
// doc comment for why UpdatedAt is not used) and transitions each to
// "archived". Registered before archive_terminal_sessions in ReconcileStuck
// so an item archived by this detector gets its work sessions swept by that
// detector in the very same tick, rather than waiting a full cycle.
//
// Idempotent by construction, not by precondition-failure suppression: an
// item only appears in FindDoneItemsOlderThan's result while its status is
// still "done", so a re-run after a successful archive naturally excludes it
// on the next tick — no double-transition, no error, on repeat runs.
func (l *BacklogLifecycleListener) archiveStaleDoneItems(ctx context.Context) {
	items, err := l.storage.FindDoneItemsOlderThan(ctx, time.Now().Add(-maxDoneAge))
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] archiveStaleDoneItems FindDoneItemsOlderThan error: %v", err)
		return
	}
	if len(items) == 0 {
		return
	}
	archived := 0
	for _, item := range items {
		precondition := &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusDone)}
		if _, transErr := l.storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusArchived, precondition); transErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] archiveStaleDoneItems transition item=%s: %v", item.ID, transErr)
			continue
		}
		archived++
	}
	if archived > 0 {
		log.InfoLog.Printf("[BacklogLifecycle] archiveStaleDoneItems: auto-archived %d item(s) done for more than %s", archived, maxDoneAge)
	}
}

// reconcileTerminalItemSessions is the archive_terminal_sessions safety-net detector:
// it finds every backlog item already in done/archived status and archives any of its
// work-role sessions that are not yet archived. This exists because
// TransitionBacklogItemStatus's archival hook only fires on a NEW transition into
// done/archived — items that were already terminal before that hook was added (or hit a
// race/crash mid-transition) would otherwise keep their work sessions unarchived forever.
// Idempotent and cheap to re-run every tick: SessionArchiver.ArchiveSessionByUUID is a
// CAS no-op for sessions that are already archived or no longer tracked.
func (l *BacklogLifecycleListener) reconcileTerminalItemSessions(ctx context.Context) {
	archiver := l.getSessionArchiver()
	if archiver == nil {
		return
	}
	items, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{string(BacklogStatusDone), string(BacklogStatusArchived)},
	})
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileTerminalItemSessions ListBacklogItems error: %v", err)
		return
	}
	processed := 0
	for _, item := range items {
		sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
		if sessErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileTerminalItemSessions ListItemSessions item=%s: %v", item.ID, sessErr)
			continue
		}
		for _, is := range sessions {
			if is.SessionUUID == "" || is.Role != SessionRoleWork {
				continue
			}
			if archErr := archiver.ArchiveSessionByUUID(ctx, is.SessionUUID); archErr != nil {
				log.WarningLog.Printf("[BacklogLifecycle] reconcileTerminalItemSessions failed to archive session=%s item=%s: %v", is.SessionUUID, item.ID, archErr)
				continue
			}
			processed++
		}
	}
	if processed > 0 {
		log.InfoLog.Printf("[BacklogLifecycle] reconcileTerminalItemSessions: processed %d work session(s) across %d terminal item(s)", processed, len(items))
	}
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
			log.WarningLog.Printf("[BacklogLifecycle] stuck detector %q panicked (recovered): %v", name, r)
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
	if l.getSessionCreator() != nil {
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

	// Self-heal pr_pending items whose pr_number is missing (0) despite having a
	// pr_url — otherwise permanently invisible to FindPRPendingItems' PrNumberGT(0)
	// filter below, so they'd never get polled. See BackfillMissingPRNumbers doc.
	if n, backfillErr := er.BackfillMissingPRNumbers(ctx); backfillErr != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] BackfillMissingPRNumbers error: %v", backfillErr)
	} else if n > 0 {
		log.InfoLog.Printf("[BacklogLifecycle] BackfillMissingPRNumbers: backfilled pr_number for %d item(s)", n)
	}

	// Durable stuck-reason detectors, each panic-isolated (Story 2.1.5e) so one
	// detector's panic cannot skip the others or merge detection below.
	var okNames, panickedNames []string

	// Flag in_progress work sessions that have gone quiet for too long. Detection +
	// notification only — a slow-but-alive agent should not be force-stopped.
	l.runStuckDetector("stale_work", &okNames, &panickedNames, func() {
		l.reconcileStaleWorkSessions(ctx, er)
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

	// Self-heal sweep: resolve any open stuck row whose reason's expected
	// status no longer matches the item's current status (Task 2.1.5d).
	l.runStuckDetector("self_heal", &okNames, &panickedNames, func() {
		l.selfHealStuck(ctx, er)
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

	// Poll pr_pending items: auto-transition to done when the PR is merged,
	// and (Story 2.1.1) flag/resolve pr_ready_unmerged.
	l.runStuckDetector("pr_ready+merge_detection", &okNames, &panickedNames, func() {
		l.ReconcilePRPending(ctx, er)
	})

	openRows, countErr := er.FindOpenStuckStates(ctx)
	openCount := -1
	if countErr == nil {
		openCount = len(openRows)
	}
	log.InfoLog.Printf("[BacklogLifecycle] stuck sweep tick: detectors ok=%v panicked=%v openRows=%d", okNames, panickedNames, openCount)
}

// reconcileStuckReviewItems notifies once per item when a review-status item
// has a review verdict on record but no active review or work session — i.e.
// it is not mid-cycle, it is simply abandoned — or when the item's only
// "active" session is confirmed dead (a zombie: pre-mortem F3). Notify-once
// dedup and "since when" are DB-backed (durable BacklogStuckState row), not
// an in-memory map, so both survive a restart. Best-effort: query/notify
// failures are logged, never returned.
func (l *BacklogLifecycleListener) reconcileStuckReviewItems(ctx context.Context, er *EntRepository) {
	seen := make(map[string]bool)

	items, err := er.FindStuckReviewItems(ctx)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileStuckReviewItems query error: %v", err)
	} else {
		for _, item := range items {
			seen[item.ID.String()] = true
			l.markAbandonedReview(ctx, er, item.ID.String(), item.Title, "stuck in review with no active session")
		}
	}

	// Zombie-session review items (pre-mortem F3): items FindStuckReviewItems
	// excludes because a review/work session row still looks active, but the
	// underlying tmux/CLI process is confirmed dead.
	checker := l.getSessionLivenessChecker()
	if checker != nil {
		zombieCandidates, zErr := er.FindZombieReviewItems(ctx)
		if zErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileStuckReviewItems FindZombieReviewItems error: %v", zErr)
		} else {
			for _, item := range zombieCandidates {
				if seen[item.ID.String()] {
					continue // already flagged via the abandoned path above
				}
				allDead := len(item.Edges.ItemSessions) > 0
				for _, is := range item.Edges.ItemSessions {
					if checker(is.SessionUUID) {
						allDead = false
						break
					}
				}
				if !allDead {
					continue // at least one active session is genuinely alive
				}
				// Tombstone the confirmed-dead rows now, not just flag them. Without
				// this, AutoRespawnReview's hasActiveWorkSession/hasActiveReviewSession
				// guard (server/services/backlog_service_triage.go) still sees these
				// EndedAt-nil rows as "active" and silently skips the respawn it was
				// just dispatched to perform — the zombie detection fired for nothing.
				for _, is := range item.Edges.ItemSessions {
					if endErr := l.storage.UpdateItemSessionEnded(ctx, is.ID.String(), time.Now()); endErr != nil {
						log.WarningLog.Printf("[BacklogLifecycle] reconcileStuckReviewItems UpdateItemSessionEnded item=%s session=%s: %v", item.ID, is.ID, endErr)
					}
				}
				seen[item.ID.String()] = true
				l.markAbandonedReview(ctx, er, item.ID.String(), item.Title, "review session process is gone (zombie)")
			}
		}
	}

	// Poll-shaped resolve (else-branch, pre-mortem F2): an item with an open
	// abandoned_review row whose condition no longer holds while it's still
	// "review" (the review gate came back in flight) must be resolved here —
	// the status-anchored self-heal sweep structurally cannot see a
	// same-status clear.
	open, openErr := er.FindOpenStuckStates(ctx)
	if openErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileStuckReviewItems FindOpenStuckStates error: %v", openErr)
		return
	}
	for _, row := range open {
		if row.Reason != domain.StuckReasonAbandonedReview {
			continue
		}
		if row.ItemStatus != BacklogStatusReview {
			continue // not this item's status anymore — self-heal sweep handles it
		}
		if seen[row.ItemID] {
			continue // still abandoned this tick
		}
		l.resolveStuckLogged(ctx, er, row.ItemID, domain.StuckReasonAbandonedReview, "reconcileStuckReviewItems")
	}
}

// reconcileUnprocessedReviewVerdicts closes the gap where a review session
// submitted its verdict (PASS/FAIL/PARTIAL/UNVERIFIABLE) but died — crash, OOM,
// server restart — before its exit event ever reached handleReviewSessionExited,
// the one place that acts on a verdict (push+PR on PASS, auto-reopen otherwise).
// The item is left stuck in "review" with a recorded verdict nothing ever
// processes.
//
// This is deliberately separate from reconcileStuckReviewItems' zombie detection:
// that path requires EVERY open review-or-work session on the item to be
// confirmed dead, but AutoReopenAfterFailedReview intentionally leaves a work
// session alive polling for the verdict once the item is back in "review" (see
// docs/tasks/backlog-feature-improvement.md's "WIP limit now undercounts live
// sessions" finding) — so the item never looks like a full zombie even though
// the review session itself is the one that died with unactioned output. Found
// live: a work session correctly detected its own item had an already-recorded
// PASS verdict and all criteria done, but had no way to force the review→done
// transition itself (by design — that's this function's job, not a work
// session's), so it looped forever re-requesting a review the backlog system
// correctly rejected (item already past "in_progress").
//
// Acts on the most recent review-role session only, once it is confirmed not
// still wrapping up on its own (EndedAt already set, or the liveness checker
// says it's dead) — a session that's merely slow to exit is left alone.
// Best-effort: query/tombstone failures are logged, never returned.
func (l *BacklogLifecycleListener) reconcileUnprocessedReviewVerdicts(ctx context.Context, er *EntRepository) {
	items, err := er.FindReviewItemsWithUnprocessedVerdict(ctx)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileUnprocessedReviewVerdicts query error: %v", err)
		return
	}
	checker := l.getSessionLivenessChecker()
	for _, item := range items {
		if len(item.Edges.ItemSessions) == 0 {
			continue
		}
		latest := item.Edges.ItemSessions[0] // most recent review-role session (query orders desc)
		if latest.Edges.ReviewVerdict == nil {
			continue // defensive: query already filters on HasReviewVerdict()
		}

		dead := latest.EndedAt != nil
		if !dead && checker != nil {
			dead = !checker(latest.SessionUUID)
		}
		if !dead {
			continue // still plausibly wrapping up on its own — leave it alone
		}

		// latest is only a genuinely *unprocessed* verdict if it belongs to the
		// item's current stay in "review" — i.e. it was created at or after the
		// most recent transition into "review". FindReviewItemsWithUnprocessedVerdict
		// has no notion of "already consumed"; it matches on "most recent
		// review-role session has a dead-and-verdicted state", full stop. A
		// review session whose verdict was already correctly applied once (via
		// the normal real-time handleReviewSessionExited path) stays matchable
		// by that query forever, because nothing marks the verdict as consumed
		// — so if the item later re-enters "review" for any other reason (a new
		// review cycle not yet represented by a new review-role session, or,
		// live 2026-07-20 on item 0fd4a940 (PR #176), a bug elsewhere that
		// force-reopened an already-"done" item), this sweep would treat that
		// stale, already-shipped verdict as fresh and reprocess it — reshipping
		// or reopening an item nothing here should be touching. Comparing
		// against the current review-entry timestamp catches exactly that: a
		// session created before the item's current review stay began cannot
		// be what that stay's outcome will be judged on.
		if reviewAt, found, evErr := er.GetMostRecentStatusEventAt(ctx, item.ID.String(), BacklogStatusReview); evErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileUnprocessedReviewVerdicts GetMostRecentStatusEventAt item=%s: %v", item.ID, evErr)
		} else if found && latest.CreatedAt.Before(reviewAt) {
			continue // verdict belongs to a prior, already-concluded review cycle
		}

		if latest.EndedAt == nil {
			if endErr := l.storage.UpdateItemSessionEnded(ctx, latest.ID.String(), time.Now()); endErr != nil {
				log.WarningLog.Printf("[BacklogLifecycle] reconcileUnprocessedReviewVerdicts tombstone item=%s session=%s: %v", item.ID, latest.ID, endErr)
			}
		}

		log.WarningLog.Printf("[BacklogLifecycle] item %s: review session %s has an unprocessed %s verdict — applying it now",
			item.ID, latest.SessionUUID, latest.Edges.ReviewVerdict.OverallOutcome)
		// forcePush=true: this is the crash-recovery sweep for a review session that
		// died before its exit event ever reached handleReviewSessionExited normally
		// — it cannot tell a genuinely-live work session apart from a zombie that will
		// never poll again, so it must make forward progress regardless. See
		// handleReviewSessionExited's doc comment and
		// TestReconcileUnprocessedReviewVerdicts_should_applyPassVerdict_When_ReviewSessionDiedButWorkSessionStillAlive.
		l.handleReviewSessionExited(ctx, ItemSessionSummary{
			ID:            latest.ID.String(),
			BacklogItemID: item.ID.String(),
			SessionUUID:   latest.SessionUUID,
			Role:          string(SessionRoleReview),
		}, true)
	}
}

// markAbandonedReview writes/refreshes the durable abandoned_review row for
// itemID and, once the condition has held past the 15-minute grace
// (abandonedReview pure fn, Story 2.1.0), notifies AND auto-respawns a review
// pass via the injected ReviewRespawner (if wired) — gives the 60s reconcile
// one or more ticks to re-spawn a review gate before flagging, avoiding a
// false positive on an item that just entered review. The row itself is
// mark/refreshed unconditionally so first_detected_at tracks the true onset
// even before the grace elapses. Respawn shares the exact same "notify once"
// gate as the notification (row.NotifiedAt IS NULL): it fires exactly once
// per stuck-row lifetime, not on every tick, so a genuinely-failing item
// doesn't spin the reconciler on repeated re-review attempts — the
// respawned call's own internal iteration cap (see
// BacklogService.AutoRespawnReview) is what actually stops runaway retries
// across separate abandoned_review occurrences. Best-effort: errors are
// logged, never returned.
func (l *BacklogLifecycleListener) markAbandonedReview(ctx context.Context, er *EntRepository, itemID, itemTitle, contextDesc string) {
	applied, err := er.MarkStuck(ctx, itemID, domain.StuckReasonAbandonedReview, BacklogStatusReview, contextDesc)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview MarkStuck item=%s: %v", itemID, err)
		return
	}
	if !applied {
		return
	}

	// 15-minute grace, keyed off the most recent to_status="review" transition
	// (falls back to the row's own first_detected_at if no event is on record,
	// e.g. an item seeded directly into review by a test or migration).
	lastReviewAt, found, evErr := er.GetMostRecentStatusEventAt(ctx, itemID, BacklogStatusReview)
	if evErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview GetMostRecentStatusEventAt item=%s: %v", itemID, evErr)
	}

	rows, findErr := er.FindOpenStuckStates(ctx)
	if findErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview FindOpenStuckStates item=%s: %v", itemID, findErr)
		return
	}
	row, ok := findOpenStuckStateFor(rows, itemID, domain.StuckReasonAbandonedReview)
	if !ok {
		return
	}
	if !found {
		lastReviewAt = row.FirstDetectedAt
	}
	if !abandonedReview(lastReviewAt, time.Now()) {
		return
	}

	// Notify-once dedup: the operator notification itself still fires exactly
	// once per stuck-row lifetime (row.NotifiedAt), independent of the
	// backoff-gated respawn below — otherwise every subsequent automated retry
	// (per the exponential schedule) would also re-notify, which would be
	// spam, not signal.
	if row.NotifiedAt == nil {
		log.WarningLog.Printf("[BacklogLifecycle] item %s stuck in review with nothing in flight (%s)", itemID, contextDesc)
		l.notify(itemID,
			"Review item needs attention",
			fmt.Sprintf("%s — stuck in review with no active session (%s). It may need manual re-review or rework.", itemTitle, contextDesc),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
		)
		if _, notifyErr := er.MarkStuckNotified(ctx, itemID, domain.StuckReasonAbandonedReview); notifyErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview MarkStuckNotified item=%s: %v", itemID, notifyErr)
			// Do NOT proceed to dispatch a respawn below on this tick: a sustained
			// MarkStuckNotified failure would otherwise re-notify (not just
			// respawn) every ~60s tick, breaking the "exactly once per
			// stuck-row lifetime" notification guarantee. The backoff gate below
			// gets its own chance on the NEXT tick regardless.
			return
		}
	}

	// Close the loop: a notification alone leaves the item stuck until a human
	// notices — the exact gap that let 4 real backlog items go stale, some for
	// multiple days (docs/tasks/backlog-feature-improvement.md). Backoff-gated
	// (session/backlog_remediation.go, Phase A of
	// docs/tasks/backlog-stuck-item-auto-remediation.md): fires on this first
	// grace-elapsed tick AND, unlike the notification above, again on each
	// later tick once the exponential schedule allows — up to
	// MaxRemediationAttempts before parking. Dispatched async, bounded by
	// reviewSem (same limiter the sibling review-gate-respawn path in
	// ReconcileStuck uses): a headless re-review call can take minutes, and
	// this runs inside a synchronous detector sweep that must not block.
	respawner := l.getReviewRespawner()
	if respawner == nil {
		log.DebugLog.Printf("[BacklogLifecycle] markAbandonedReview item=%s: no ReviewRespawner configured, notification only", itemID)
		return
	}

	due, justParked, gateErr := l.storage.RemediationDue(ctx, itemID, domain.StuckReasonAbandonedReview)
	if gateErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview RemediationDue item=%s: %v", itemID, gateErr)
		due = true // fail open — see autoReopenWithBackoffGate's identical rationale
	}
	if justParked {
		l.notify(itemID,
			"Auto-rework paused",
			fmt.Sprintf("%s — automated re-review has been retried %d times over an extended period without resolving. It now needs manual attention; use Reset to try again automatically.", itemTitle, MaxRemediationAttempts),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		)
	}
	if !due {
		log.InfoLog.Printf("[BacklogLifecycle] markAbandonedReview item=%s: abandoned_review remediation backoff not yet due, skipping respawn", itemID)
		return
	}

	go func(id string) {
		select {
		case l.reviewSem <- struct{}{}:
		case <-l.shutdownCtx.Done():
			return
		}
		defer func() { <-l.reviewSem }()
		if respawnErr := respawner.AutoRespawnReview(l.shutdownCtx, id); respawnErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] markAbandonedReview AutoRespawnReview item=%s: %v", id, respawnErr)
		}
	}(itemID)
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
		log.WarningLog.Printf("[BacklogLifecycle] %s ResolveStuck(%s) item=%s: %v", caller, reason, itemID, err)
	}
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

// maxWorkSessionStaleness is the longest an in_progress work session can go without
// reporting progress before ReconcileStuck flags it as stale. Mirrors the order of
// magnitude of maxTriageSessionAge (server/services/backlog_service_triage.go).
const maxWorkSessionStaleness = 2 * time.Hour

// headlessTriageSessionUUIDPrefix mirrors server/services/backlog_service_triage.go's
// headlessTriageUUIDPrefix constant (duplicated here rather than imported: server/services
// imports this package, so the reverse import would cycle). Headless triage sessions have
// no live in-memory Instance to check liveness against — per that file's
// tombstoneOrphanTriageSessions, an "open" (EndedAt nil) row found later means the call
// that would have closed it on completion already finished or crashed, not that it's
// genuinely still running — so they warrant a much shorter staleness threshold than the
// general 2h ceiling below.
const headlessTriageSessionUUIDPrefix = "headless-triage-"

// maxHeadlessTriageSessionStaleness bounds how long an open headless-triage session is
// trusted before reconcileOrphanedTriageItems flags it as orphaned. Headless triage calls
// routinely run 7-15 minutes (see that function's doc comment); 30 minutes gives 2x margin
// over that ceiling while closing the "triage session died before submit_triage_result,
// item silently stuck in idea" gap (docs/tasks/triage-validation-*/research/pitfalls.md,
// GAP-20/21) far faster than waiting out the general-purpose 2h threshold, which was tuned
// for interactive/foreground triage sessions where a liveness signal isn't available here.
const maxHeadlessTriageSessionStaleness = 30 * time.Minute

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
func (l *BacklogLifecycleListener) reconcileStaleWorkSessions(ctx context.Context, er *EntRepository) {
	items, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{string(BacklogStatusInProgress)},
	})
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileStaleWorkSessions list error: %v", err)
		return
	}

	stillStale := make(map[string]bool)

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
		if !staleWork(lastProgress, time.Now()) {
			continue
		}
		stillStale[item.ID] = true

		applied, markErr := er.MarkStuck(ctx, item.ID, domain.StuckReasonStaleWork, BacklogStatusInProgress,
			fmt.Sprintf("no progress since %s", lastProgress))
		if markErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileStaleWorkSessions MarkStuck item=%s: %v", item.ID, markErr)
			continue
		}
		if !applied {
			// Status precondition mismatch (item moved off in_progress between
			// the ListBacklogItems read above and this write) — nothing to mark
			// or remediate this tick.
			continue
		}
		rows, findErr := er.FindOpenStuckStates(ctx)
		if findErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileStaleWorkSessions FindOpenStuckStates item=%s: %v", item.ID, findErr)
			continue
		}
		row, ok := findOpenStuckStateFor(rows, item.ID, domain.StuckReasonStaleWork)
		if !ok {
			continue
		}
		if row.NotifiedAt != nil {
			// Already notified on a prior tick — not the first sighting.
			// Notify-once semantics already covered the "give it a chance"
			// window on the tick that opened this row; from here on,
			// automated remediation takes over, itself gated by the shared
			// backoff schedule (RemediationDue), independent of the
			// per-item rework cap (docs/tasks/backlog-stuck-item-auto-
			// remediation.md Phase B — a live item with reworkCapOverride=0
			// (unlimited) had bounced through this exact stale-agent-idle
			// shape 14 times with nothing ever unsticking it).
			l.remediateStaleWorkWithBackoffGate(ctx, item.ID, item.Title)
			continue
		}

		log.WarningLog.Printf("[BacklogLifecycle] item %s work session %s stale (no progress since %s)", item.ID, active.SessionUUID, lastProgress)
		l.notify(item.ID,
			"Work session may be stuck",
			fmt.Sprintf("%s — no progress reported in over %s. It may be hung or working silently.", item.Title, maxWorkSessionStaleness),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
		)
		if _, notifyErr := er.MarkStuckNotified(ctx, item.ID, domain.StuckReasonStaleWork); notifyErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileStaleWorkSessions MarkStuckNotified item=%s: %v", item.ID, notifyErr)
		}
	}

	// Poll-shaped resolve (else-branch, pre-mortem F2): an in_progress item
	// with an open stale_work row whose session resumed reporting progress
	// must be resolved here — same-status clears are invisible to the
	// status-anchored self-heal sweep.
	open, openErr := er.FindOpenStuckStates(ctx)
	if openErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileStaleWorkSessions FindOpenStuckStates(resolve pass) error: %v", openErr)
		return
	}
	for _, row := range open {
		if row.Reason != domain.StuckReasonStaleWork {
			continue
		}
		if row.ItemStatus != BacklogStatusInProgress {
			continue // self-heal sweep handles it once status has moved on
		}
		if stillStale[row.ItemID] {
			continue // still stale this tick
		}
		l.resolveStuckLogged(ctx, er, row.ItemID, domain.StuckReasonStaleWork, "reconcileStaleWorkSessions")
	}
}

// remediateStaleWorkWithBackoffGate dispatches StaleWorkRemediator.
// RemediateStaleWorkSession through the shared remediation backoff gate
// (Storage.RemediationDue, session/backlog_remediation.go) — the
// "stale_work" reason's remediation action per
// docs/tasks/backlog-stuck-item-auto-remediation.md Phase B. Mirrors
// retryPushFailedWithBackoffGate's shape (bare goroutine, no semaphore — see
// that function's doc comment for why the reviewSem review-gate respawns
// share is not needed here either: ending a stale ItemSession and
// respawning a fresh one is fast compared to a live headless LLM call).
// Best-effort: gate query/write errors are logged, never returned, and fail
// OPEN (still attempts the remediation) rather than silently stranding the
// item — same rationale as autoReopenWithBackoffGate/
// retryPushFailedWithBackoffGate.
//
// Deliberately does NOT add a second liveness check before dispatching
// (e.g. re-querying Instance.TmuxAlive/PaneProcessDead here) — the caller
// (reconcileStaleWorkSessions) already reconfirmed staleWork() true this
// tick, and RemediationDue's own backoff (minimum 30 minutes after the
// first notification) has independently elapsed by the time due=true. A
// second, independently-computed liveness heuristic here could disagree
// with that detector and cause flapping; trust the one signal already
// gating this call.
func (l *BacklogLifecycleListener) remediateStaleWorkWithBackoffGate(ctx context.Context, itemID, itemTitle string) {
	remediator := l.getStaleWorkRemediator()
	if remediator == nil {
		return
	}

	due, justParked, gateErr := l.storage.RemediationDue(ctx, itemID, domain.StuckReasonStaleWork)
	if gateErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] remediateStaleWorkWithBackoffGate RemediationDue item=%s: %v", itemID, gateErr)
		due = true // fail open — see retryPushFailedWithBackoffGate's identical rationale
	}
	if justParked {
		l.notify(itemID,
			"Auto-rework paused",
			fmt.Sprintf("%s — automated stale-session recovery has been retried %d times over an extended period without resolving. It now needs manual attention; use Reset to try again automatically.", itemTitle, MaxRemediationAttempts),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		)
	}
	if !due {
		log.InfoLog.Printf("[BacklogLifecycle] remediateStaleWorkWithBackoffGate item=%s: stale_work remediation backoff not yet due, skipping", itemID)
		return
	}

	go func() {
		if err := remediator.RemediateStaleWorkSession(l.shutdownCtx, itemID); err != nil {
			log.ErrorLog.Printf("[BacklogLifecycle] remediateStaleWorkWithBackoffGate RemediateStaleWorkSession item=%s: %v", itemID, err)
		}
	}()
}

// reconcileOrphanedTriageItems flags idea-status items whose most recent triage-role
// ItemSession never ended and has gone stale — the triage process crashed, was killed,
// or a server restart happened mid-triage before the completion goroutine ever ran.
// Previously this class of failure was only caught by tombstoneOrphanTriageSessions
// (same package, server/services/backlog_service_triage.go), and only when a human
// manually re-triggered triage on the item; this is the standing-sweep equivalent.
// Pure staleness gate — no liveness checker — matching reconcileStaleWorkSessions'
// established pattern for the closest analogous detector in this file: a headless
// triage call routinely runs 7-15 minutes, so per-tick liveness signals are noisy
// here; staleness alone is the reliable signal. Headless-triage sessions (the common
// case) get the much shorter maxHeadlessTriageSessionStaleness (30m) rather than the
// general-purpose maxWorkSessionStaleness (2h): an open headless row found later
// reliably means dead, not slow (see that constant's doc comment). Best-effort:
// query/notify failures are logged, never returned.
func (l *BacklogLifecycleListener) reconcileOrphanedTriageItems(ctx context.Context, er *EntRepository) {
	items, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{string(BacklogStatusIdea)},
	})
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileOrphanedTriageItems list error: %v", err)
		return
	}

	for _, item := range items {
		sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID)
		if sessErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileOrphanedTriageItems ListItemSessions item=%s: %v", item.ID, sessErr)
			continue
		}
		var latestTriage *ItemSessionSummary
		for i := range sessions {
			if sessions[i].Role == SessionRoleTriage && sessions[i].EndedAt == nil {
				latestTriage = &sessions[i]
			}
		}
		if latestTriage == nil {
			continue // no open triage session
		}
		staleness := maxWorkSessionStaleness
		if strings.HasPrefix(latestTriage.SessionUUID, headlessTriageSessionUUIDPrefix) {
			staleness = maxHeadlessTriageSessionStaleness
		}
		if time.Since(latestTriage.CreatedAt) <= staleness {
			continue // still plausibly running
		}

		// Tombstone the dead row now rather than leaving it open until a human
		// manually re-triggers triage (the only other path that closes it, via
		// tombstoneOrphanTriageSessions in server/services). Staleness past
		// maxWorkSessionStaleness IS the confirmed-dead signal for this detector
		// (see doc comment above: no liveness checker, headless calls don't run
		// this long) — nothing left to preserve by keeping the row open.
		if endErr := l.storage.UpdateItemSessionEnded(ctx, latestTriage.ID, time.Now()); endErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileOrphanedTriageItems UpdateItemSessionEnded item=%s session=%s: %v", item.ID, latestTriage.ID, endErr)
		}

		applied, markErr := er.MarkStuck(ctx, item.ID, domain.StuckReasonOrphanedTriage, BacklogStatusIdea,
			fmt.Sprintf("triage session %s still open after %s", latestTriage.SessionUUID, maxWorkSessionStaleness))
		if markErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileOrphanedTriageItems MarkStuck item=%s: %v", item.ID, markErr)
			continue
		}
		if !applied {
			continue
		}
		rows, findErr := er.FindOpenStuckStates(ctx)
		if findErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileOrphanedTriageItems FindOpenStuckStates item=%s: %v", item.ID, findErr)
			continue
		}
		row, ok := findOpenStuckStateFor(rows, item.ID, domain.StuckReasonOrphanedTriage)
		if !ok || row.NotifiedAt != nil {
			continue
		}

		log.WarningLog.Printf("[BacklogLifecycle] item %s triage session %s orphaned (stale)", item.ID, latestTriage.SessionUUID)
		l.notify(item.ID,
			"Triage may be stuck",
			fmt.Sprintf("%s — its triage session ended without finishing and nothing is running. Re-trigger triage or investigate.", item.Title),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
		)
		if _, notifyErr := er.MarkStuckNotified(ctx, item.ID, domain.StuckReasonOrphanedTriage); notifyErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileOrphanedTriageItems MarkStuckNotified item=%s: %v", item.ID, notifyErr)
		}
	}
	// No resolve pass needed here: selfHealStuck (status-anchored) clears this
	// reason once the item leaves 'idea' — i.e. once triage is re-triggered and succeeds.
}

// reconcileBouncingItems flags items that have crossed in_progress->review
// >= bounceThreshold times within bounceLookback with no recorded PASS
// verdict — a non-converging rework cycle that never hits the rework cap
// (root cause #4). Before flagging, it first checks whether the item's
// linked PR has already merged (including a manual merge outside the app's
// own ship flow) — a merged item isn't bouncing, it's done, and is
// transitioned to done instead of being marked stuck. Best-effort:
// query/notify failures are logged, never returned.
func (l *BacklogLifecycleListener) reconcileBouncingItems(ctx context.Context, er *EntRepository) {
	// Scan items in the two statuses a bouncing cycle spans; a converged item
	// (done) is handled by the self-heal sweep, not re-flagged here.
	items, err := l.storage.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses: []string{string(BacklogStatusInProgress), string(BacklogStatusReview)},
	})
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileBouncingItems list error: %v", err)
		return
	}

	since := time.Now().Add(-bounceLookback)
	for _, item := range items {
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
				log.DebugLog.Printf("[BacklogLifecycle] reconcileBouncingItems IsPRMerged item=%s pr=%d: %v", item.ID, item.PrNumber, mergedErr)
			} else if merged {
				precondition := &BacklogItemPrecondition{ExpectedStatus: item.Status}
				if _, transErr := l.storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusDone, precondition); transErr != nil {
					log.WarningLog.Printf("[BacklogLifecycle] reconcileBouncingItems done transition item=%s: %v", item.ID, transErr)
				} else {
					log.InfoLog.Printf("[BacklogLifecycle] reconcileBouncingItems item=%s → done (PR #%d already merged)", item.ID, item.PrNumber)
					// Best-effort: clear any bouncing row from a prior tick
					// immediately, rather than waiting for the next
					// selfHealStuck sweep to notice the terminal status.
					l.resolveStuckLogged(ctx, er, item.ID, domain.StuckReasonBouncing, "reconcileBouncingItems/merged")
				}
				continue
			}
		}

		count, countErr := er.CountReviewCyclesSince(ctx, item.ID, since)
		if countErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileBouncingItems CountReviewCyclesSince item=%s: %v", item.ID, countErr)
			continue
		}
		outcome, verdictErr := er.GetMostRecentReviewVerdictForItem(ctx, item.ID)
		if verdictErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileBouncingItems GetMostRecentReviewVerdictForItem item=%s: %v", item.ID, verdictErr)
		}
		hasPass := outcome == ReviewOutcomePass

		if !isBouncing(count, hasPass) {
			continue
		}

		applied, markErr := er.MarkStuck(ctx, item.ID, domain.StuckReasonBouncing, BacklogStatus(item.Status),
			fmt.Sprintf("bounced in_progress<->review %d times in the last %s with no PASS verdict", count, bounceLookback))
		if markErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileBouncingItems MarkStuck item=%s: %v", item.ID, markErr)
			continue
		}
		if !applied {
			continue
		}
		rows, findErr := er.FindOpenStuckStates(ctx)
		if findErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileBouncingItems FindOpenStuckStates item=%s: %v", item.ID, findErr)
			continue
		}
		row, ok := findOpenStuckStateFor(rows, item.ID, domain.StuckReasonBouncing)
		if !ok || row.NotifiedAt != nil {
			continue
		}
		log.WarningLog.Printf("[BacklogLifecycle] item %s bouncing (%d cycles in %s, no PASS)", item.ID, count, bounceLookback)
		l.notify(item.ID,
			"Item is thrashing between work and review",
			fmt.Sprintf("%s — bounced between in_progress and review %d times in the last %s with no PASS verdict. It may be stuck in a non-converging rework loop.", item.Title, count, bounceLookback),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
		)
		if _, notifyErr := er.MarkStuckNotified(ctx, item.ID, domain.StuckReasonBouncing); notifyErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] reconcileBouncingItems MarkStuckNotified item=%s: %v", item.ID, notifyErr)
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
		log.WarningLog.Printf("[BacklogLifecycle] selfHealStuck FindOpenStuckStates error: %v", err)
		return
	}
	for _, row := range open {
		if row.ItemStatus == BacklogStatusDone || row.ItemStatus == BacklogStatusArchived {
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
		case domain.StuckReasonBouncing:
			resolve = row.ItemStatus != BacklogStatusInProgress && row.ItemStatus != BacklogStatusReview
		case domain.StuckReasonOrphanedTriage:
			resolve = row.ItemStatus != BacklogStatusIdea
		default:
			// autonomous_stuck, push_failed, rework_cap, and any future reason
			// with no non-terminal anchor: stays open until the blanket
			// terminal rule above catches it, or its own event-site resolves
			// it first.
			continue
		}
		if !resolve {
			continue
		}
		l.resolveStuckLogged(ctx, er, row.ItemID, row.Reason, "selfHealStuck")
	}
}

// buildFallbackPRBody composes a PR body from the backlog item's own data —
// used when no headless pool is configured, GetGitDiff fails, or
// headless.DraftPRDescription errors out. Previously this fallback was a bare
// "Automated PR for backlog item: X\n\nItem ID: Y" one-liner (see PRs #147/#148
// on this repo), which explains nothing about why the change was made and gives
// a reviewer no verification checklist. Description ties the PR back to the
// item's own problem statement (the "why"); the item's acceptance criteria
// double as a test plan checklist since they are the only verification steps
// this code path has available without an LLM call.
func buildFallbackPRBody(item *BacklogItemData) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Summary\n%s\n\n(Backlog item: %s)\n", sanitizeField(item.Description, 1000), item.ID)

	if criteria, _ := ParseAcCriteria(item.AcceptanceCriteria); len(criteria) > 0 {
		sb.WriteString("\n## Test plan\n")
		for _, c := range criteria {
			box := "[ ]"
			if c.Status == domain.AcStatusDone {
				box = "[x]"
			}
			fmt.Fprintf(&sb, "- %s %s\n", box, sanitizeField(c.Text, 300))
		}
	}
	return sb.String()
}

// shipViaAgentOrFallback is handleReviewSessionExited's PASS-verdict entry
// point for shipping a PR when the work session that earned the verdict is
// no longer live to run /backlog/ship itself (or forcePush is set — see
// handleReviewSessionExited's doc comment). It tries the agent-driven path
// first — running agentShipPrompt (/backlog/ship, which drives
// /github:pr-ship: local CI, code review, remote CI, and actual
// merge-conflict resolution) as a headless one-shot via the wired
// OneShotShipRunner — and only falls back to the mechanical pushAndCreatePR
// (bare `git push` + `gh pr create`, no CI reaction, no conflict resolution)
// when that isn't available or didn't work. This mirrors the design already
// used for AutoCreatePR/TriggerShipPR-style flows: prefer the agent, keep
// the mechanical path as a backstop rather than deleting it outright, since
// pushAndCreatePR is still the right tool for attemptPushRemediation (a
// purely mechanical retry after a purely mechanical fetch+merge — no LLM
// judgment involved there) and remains a working fallback of last resort
// here when the agent-driven attempt itself fails to produce a PR (e.g. the
// session's Instance is no longer tracked live, or the one-shot call itself
// errors/times out).
//
// Deliberately does NOT special-case "no worktree at all" before trying the
// one-shot: if OneShotShipRunner is wired but there is genuinely nothing to
// point it at, RunOneShotForSession fails fast (its own findInstance lookup
// misses) and this falls through to pushAndCreatePR, which performs the
// exact same worktree-presence check it always has and reaches the exact
// same two pre-existing outcomes depending on what's actually missing:
//   - No worktree recorded in storage at all: pushAndCreatePR's
//     fallbackToDone("no worktree") transitions straight to done, unchanged
//     from before this fix (see
//     TestReconcileUnprocessedReviewVerdicts_should_applyPassVerdict_When_ReviewSessionDiedButWorkSessionStillAlive).
//   - A worktree is recorded but the directory itself is gone from disk
//     (e.g. cleanupItemWorktreesExcept already ran): the mechanical git
//     commands fail with a filesystem error, which pushAndCreatePR's
//     existing stayInReviewAndNotify path already turns into a durable
//     StuckReasonPushFailed row and an operator notification — the item
//     stays in review rather than silently becoming done, and the PASS
//     verdict is not dropped. No new StuckReason was added for this case:
//     from an operator's perspective it is the same actionable signal
//     ("PASS verdict, no PR, needs a manual look") pushAndCreatePR's push/PR
//     failures already surface, and both remediation paths are identical
//     (investigate manually, or retry).
func (l *BacklogLifecycleListener) shipViaAgentOrFallback(ctx context.Context, item *BacklogItemData, is ItemSessionSummary) {
	runner := l.getOneShotShipRunner()
	if runner == nil {
		log.InfoLog.Printf("[BacklogLifecycle] shipViaAgentOrFallback item=%s: no OneShotShipRunner wired, using mechanical push directly", item.ID)
		l.pushAndCreatePR(ctx, item, is)
		return
	}

	prURL, err := runner.RunOneShotForSession(ctx, is.SessionUUID, agentShipPrompt, oneShotShipTimeoutSeconds)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] shipViaAgentOrFallback item=%s session=%s: agent-driven ship failed (%v), falling back to mechanical push", item.ID, is.SessionUUID, err)
		l.pushAndCreatePR(ctx, item, is)
		return
	}
	if prURL == "" {
		log.WarningLog.Printf("[BacklogLifecycle] shipViaAgentOrFallback item=%s session=%s: agent-driven ship ran but produced no PR URL, falling back to mechanical push", item.ID, is.SessionUUID)
		l.pushAndCreatePR(ctx, item, is)
		return
	}

	log.InfoLog.Printf("[BacklogLifecycle] shipViaAgentOrFallback item=%s session=%s: agent shipped PR via one-shot /backlog/ship: %s", item.ID, is.SessionUUID, prURL)

	// Persist the PR fields and transition explicitly rather than relying
	// solely on the RunOneShot -> RecordPRCreatedOutOfBand side effect the
	// production *services.SessionService implementation performs as part of
	// RunOneShotForSession — that side effect only fires because
	// SessionService happens to hold a pointer back to this same listener
	// (server/dependencies.go's sessionService.SetBacklogLifecycleListener).
	// A test fake — or any future OneShotShipRunner implementation — has no
	// such obligation, so this path must be self-sufficient.
	// resolveToPRPending's transition is guarded by an ExpectedStatus
	// precondition, so if the side effect already made this transition, the
	// call below simply no-ops with a (deliberately ignored) precondition
	// error rather than double-applying anything.
	prNumber := 0
	if ref, parseErr := ParseGitHubURL(prURL); parseErr == nil {
		prNumber = ref.PRNumber
	}
	if prNumber > 0 {
		prURLCopy, prNumCopy := prURL, prNumber
		if _, updateErr := l.storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
			PrURL:    &prURLCopy,
			PrNumber: &prNumCopy,
		}, nil); updateErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] shipViaAgentOrFallback store PR fields item=%s: %v", item.ID, updateErr)
		}
	}
	if transErr := l.resolveToPRPending(ctx, item.ID, "agent-driven ship via one-shot /backlog/ship", "shipViaAgentOrFallback"); transErr != nil {
		// May just be a harmless race with RunOneShot's own RecordPRCreatedOutOfBand
		// side effect already landing the same transition — handlePRPendingTransitionFailed
		// re-checks the item's current status before doing anything, so that case is a
		// silent no-op there. Anything else (a genuine drift) gets recovered immediately
		// if safe, or picked up by the next reconcileDriftedPRItems tick.
		l.handlePRPendingTransitionFailed(ctx, item.ID, "shipViaAgentOrFallback", transErr)
	}
}

// pushAndCreatePR commits any dirty state, pushes the branch, creates a GitHub PR,
// stores the PR URL and number on the item, then transitions to pr_pending.
// Falls back to a direct done transition only when there was genuinely nothing to
// ship (no worktree). If code was committed but push/PR-creation fails, the item
// stays in review and a notification is published — see stayInReviewAndNotify.
// Used both as shipViaAgentOrFallback's mechanical backstop (agent-driven ship
// unavailable or failed) and directly by attemptPushRemediation (a push retry
// after a purely mechanical fetch+merge — no LLM judgment needed there, so it
// skips shipViaAgentOrFallback and calls this directly).
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

		notifyToast := func() {
			l.notify(item.ID,
				"PR creation failed",
				fmt.Sprintf("%s — %s: %v. Code is committed locally but not pushed; retry or investigate manually.", item.Title, reason, err),
				7, // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
				3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
			)
		}

		// Durable push_failed row (Story 2.1.6). Also doubles as the ephemeral
		// toast's dedup key below — without a durable repo to gate on, fall back
		// to the old always-notify behavior rather than silently dropping the toast.
		er, ok := l.storage.repo.(*EntRepository)
		if !ok {
			notifyToast()
			return
		}
		applied, markErr := er.MarkStuck(ctx, item.ID, domain.StuckReasonPushFailed, BacklogStatusReview,
			fmt.Sprintf("%s: %v", reason, err))
		if markErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] pushAndCreatePR MarkStuck(push_failed) item=%s: %v", item.ID, markErr)
			return
		}
		if !applied {
			return
		}

		// Notify-once dedup (same pattern as markAbandonedReview and the other
		// stuck reasons): MarkStuckNotified only flips notified_at nil -> now
		// once per open stuck-state row, so repeated calls for the same
		// still-open failure (e.g. a non-fast-forward push retried every
		// reconciliation tick) skip the ephemeral ERROR toast after the first —
		// this is what was previously firing a fresh "PR creation failed" toast
		// every few seconds with no dedup. The toast fires again only once the
		// row is resolved (push/PR succeeds) and later reopens on a new failure.
		notifiedNow, notifyErr := er.MarkStuckNotified(ctx, item.ID, domain.StuckReasonPushFailed)
		if notifyErr != nil {
			log.WarningLog.Printf("[BacklogLifecycle] pushAndCreatePR MarkStuckNotified(push_failed) item=%s: %v", item.ID, notifyErr)
			return
		}
		if !notifiedNow {
			return
		}
		notifyToast()
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
		prBody := buildFallbackPRBody(item)
		if pool := l.getHeadlessPool(); pool != nil {
			diff, _, diffErr := GetGitDiff(ctx, wt.WorktreePath, wt.BaseCommitSHA)
			if diffErr != nil {
				log.WarningLog.Printf("[BacklogLifecycle] pushAndCreatePR GetGitDiff for description item=%s: %v; using fallback body", item.ID, diffErr)
			} else if drafted, draftErr := headless.DraftPRDescription(ctx, pool, item.Title, item.Description, diff, wt.BranchName); draftErr != nil {
				log.WarningLog.Printf("[BacklogLifecycle] pushAndCreatePR DraftPRDescription item=%s: %v; using fallback body", item.ID, draftErr)
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
	// Best-effort: repos without branch protection or auto-merge enabled will fail here.
	// ReconcilePRPending still polls and will detect the merge if one happens some other
	// way, but nothing will ever *initiate* the merge for this PR without auto-merge — the
	// operator must merge it manually, so this needs a notification, not just a log line
	// (same silent-failure pattern found and fixed elsewhere in this codebase — see
	// docs/tasks/backlog-feature-improvement.md).
	if autoErr := g.EnablePRAutoMerge(prNumber); autoErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] pushAndCreatePR auto-merge item=%s pr=%d: %v", item.ID, prNumber, autoErr)
		l.notify(item.ID,
			"Auto-merge not enabled",
			fmt.Sprintf("%s — PR #%d could not be set to auto-merge (%v). It will need to be merged manually once checks pass.", item.Title, prNumber, autoErr),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
		)
	} else {
		log.InfoLog.Printf("[BacklogLifecycle] pushAndCreatePR item=%s PR #%d auto-merge enabled", item.ID, prNumber)
	}

	// Transition to pr_pending.
	if transErr := l.resolveToPRPending(ctx, item.ID, "", "pushAndCreatePR"); transErr != nil {
		l.handlePRPendingTransitionFailed(ctx, item.ID, "pushAndCreatePR", transErr)
		return
	}
	log.InfoLog.Printf("[BacklogLifecycle] pushAndCreatePR item=%s → pr_pending (PR #%d %s)", item.ID, prNumber, prURL)
}

// resolveToPRPending performs the transition+resolve tail shared by every
// path that moves a backlog item from review to pr_pending because a PR now
// exists: the status transition itself, then — on success — resolving any
// open push_failed/abandoned_review rows immediately rather than waiting for
// the self-heal sweep's next tick (Task 2.1.5a). note is attached to the
// transition's audit event; caller identifies the log prefix used by the
// (always best-effort) stuck-resolution calls. Returns the transition error,
// if any, so callers can apply their own logging/fallback behavior.
func (l *BacklogLifecycleListener) resolveToPRPending(ctx context.Context, itemID, note, caller string) error {
	precondition := &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusReview), Note: note}
	if _, transErr := l.storage.TransitionBacklogItemStatus(ctx, itemID, BacklogStatusPRPending, precondition); transErr != nil {
		return transErr
	}
	if er, ok := l.storage.repo.(*EntRepository); ok {
		l.resolveStuckLogged(ctx, er, itemID, domain.StuckReasonPushFailed, caller)
		l.resolveStuckLogged(ctx, er, itemID, domain.StuckReasonAbandonedReview, caller)
	}
	return nil
}

// hasActiveSession reports whether any of the provided ItemSessions is an
// open (not yet ended) work- or review-role session. Package-local
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
	return false
}

// recoverDriftedPRItem attempts to recover a single item whose real, cached
// PR reference (prNumber/prUrl) has drifted out of ReconcilePRPending's view
// — see FindDriftedPRItems' doc comment for the drift mechanism. Recovery is
// a single CAS transition back to pr_pending, scoped to the item's own
// currently-observed status/updated_at so a genuine concurrent transition
// (e.g. a fresh work/review session starting between the caller's read and
// this write) simply loses the CAS and is left alone rather than clobbered —
// the same "anchor on reality, never force" discipline BUG-026's fix
// established for TransitionBacklogItemStatus itself. Callers must have
// already confirmed no active work/review session exists for this item
// (hasActiveSession) before calling — this function does not re-check.
// Returns true if the item was recovered. Best-effort: errors are logged,
// never returned.
func (l *BacklogLifecycleListener) recoverDriftedPRItem(ctx context.Context, item *BacklogItemData, caller string) bool {
	updatedAt := item.UpdatedAt
	precondition := &BacklogItemPrecondition{
		ExpectedStatus:    item.Status,
		ExpectedUpdatedAt: &updatedAt,
		Note: fmt.Sprintf("self-heal (%s): recovered from drift — item has PR #%d (%s) cached but status was %q, not pr_pending",
			caller, item.PrNumber, item.PrURL, item.Status),
	}
	if _, transErr := l.storage.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusPRPending, precondition); transErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] recoverDriftedPRItem(%s) item=%s: recovery transition failed (likely a concurrent legitimate transition, will retry next tick): %v", caller, item.ID, transErr)
		return false
	}
	log.WarningLog.Printf("[BacklogLifecycle] recoverDriftedPRItem(%s) item=%s: recovered from status drift — PR #%d (%s) was stranded at status %q with no active session; transitioned back to pr_pending", caller, item.ID, item.PrNumber, item.PrURL, item.Status)
	l.notify(item.ID,
		"Backlog item recovered from stuck state",
		fmt.Sprintf("%s — had an open PR (#%d) but its status had drifted away from tracking; automatically recovered and resumed polling.", item.Title, item.PrNumber),
		10, // sessionv1.NotificationType_NOTIFICATION_TYPE_INFO
		1,  // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW
	)
	return true
}

// handlePRPendingTransitionFailed is called when resolveToPRPending fails
// after prNumber/prUrl were already durably persisted on the item — the
// exact drift mechanism FindDriftedPRItems' doc comment describes (a
// concurrent legitimate event, e.g. markAbandonedReview's grace period
// respawning a review pass while an agent-driven ship is still mid-flight,
// wins the race and moves status away from "review" before this call's own
// CAS-gated transition to pr_pending lands). Rather than silently leaving
// the item stranded until the periodic reconcileDriftedPRItems sweep's next
// tick (ReconcileStuck runs every 60s — server/dependencies.go), this
// attempts the same recovery immediately: if nothing is actively working the
// item right now, transition it straight back to pr_pending so it re-enters
// ReconcilePRPending's view in this same tick. If something IS actively
// working it (a legitimate concurrent event genuinely owns the item now),
// recovery correctly declines — the periodic sweep remains the backstop for
// whenever that session later ends without itself resolving to pr_pending.
// Also correctly no-ops when the "failure" was actually a harmless race with
// another writer that already landed the same transition (e.g.
// RecordPRCreatedOutOfBand beating shipViaAgentOrFallback to it) — the
// re-fetched item's status is checked before attempting anything.
func (l *BacklogLifecycleListener) handlePRPendingTransitionFailed(ctx context.Context, itemID, caller string, transErr error) {
	log.WarningLog.Printf("[BacklogLifecycle] %s pr_pending transition item=%s failed after PR fields were already persisted — item may be stranded with a real PR outside pr_pending tracking until self-heal recovers it: %v", caller, itemID, transErr)

	sessions, sessErr := l.storage.ListItemSessions(ctx, itemID)
	if sessErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] %s handlePRPendingTransitionFailed ListItemSessions item=%s: %v", caller, itemID, sessErr)
		return
	}
	if hasActiveSession(sessions) {
		log.InfoLog.Printf("[BacklogLifecycle] %s handlePRPendingTransitionFailed item=%s: active session found, leaving recovery to the next reconcileDriftedPRItems tick", caller, itemID)
		return
	}
	item, getErr := l.storage.GetBacklogItem(ctx, itemID)
	if getErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] %s handlePRPendingTransitionFailed GetBacklogItem item=%s: %v", caller, itemID, getErr)
		return
	}
	if item.PrNumber <= 0 || item.PrURL == "" ||
		item.Status == string(BacklogStatusPRPending) || item.Status == string(BacklogStatusDone) || item.Status == string(BacklogStatusArchived) {
		return // already recovered, or resolved to a terminal state, by the time we got here
	}
	l.recoverDriftedPRItem(ctx, item, caller)
}

// reconcileDriftedPRItems is the periodic self-heal detector for the drift
// class FindDriftedPRItems queries: items with a real, cached PR reference
// whose status has fallen out of ReconcilePRPending's view with nothing left
// actively working on them. Registered immediately before ReconcilePRPending
// (Task: PR-lifecycle drift self-heal) so a recovered item is picked up by
// the merge/CI polling sweep in the very same tick rather than waiting an
// extra cycle. Best-effort: query failures are logged, never returned.
func (l *BacklogLifecycleListener) reconcileDriftedPRItems(ctx context.Context, er *EntRepository) {
	items, err := er.FindDriftedPRItems(ctx)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcileDriftedPRItems query error: %v", err)
		return
	}
	for _, item := range items {
		itemData := backlogItemToData(item)
		l.recoverDriftedPRItem(ctx, &itemData, "reconcileDriftedPRItems")
	}
}

// reconcilePushFailedItems retries the push+PR flow for every open
// push_failed stuck row still anchored at "review" — see the doc comment on
// its ReconcileStuck call site for why this periodic sweep is needed at all
// (pushAndCreatePR itself only ever runs in response to a review-session
// event, so an item with no active session would otherwise never get a
// second attempt). While the item remains at "review", resolution happens
// through resolveToPRPending once a retried push succeeds — selfHealStuck's
// terminal-anchor case only backstops the item reaching done/archived some
// other way (see its doc comment), so this loop's own row-status filter
// below still only needs to consider "review" as an active retry target.
// Best-effort: query failures are logged, never returned.
func (l *BacklogLifecycleListener) reconcilePushFailedItems(ctx context.Context, er *EntRepository) {
	open, err := er.FindOpenStuckStates(ctx)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] reconcilePushFailedItems FindOpenStuckStates error: %v", err)
		return
	}
	for _, row := range open {
		if row.Reason != domain.StuckReasonPushFailed {
			continue
		}
		if row.ItemStatus != BacklogStatusReview {
			continue // no longer applicable to this item's current state
		}
		l.retryPushFailedWithBackoffGate(ctx, row.ItemID, row.ItemTitle)
	}
}

// retryPushFailedWithBackoffGate dispatches attemptPushRemediation through
// the shared remediation backoff gate (Storage.RemediationDue,
// session/backlog_remediation.go) — the "push_failed" reason's remediation
// action per docs/tasks/backlog-stuck-item-auto-remediation.md Phase B.
// Mirrors autoReopenWithBackoffGate's shape (bare goroutine, no semaphore —
// a git fetch+merge+push is seconds, not the minutes a headless LLM respawn
// can take, so the reviewSem markAbandonedReview/ReconcileStuck's
// review-gate respawns share is not needed here). Best-effort: gate
// query/write errors are logged, never returned, and fail OPEN (still
// attempts the retry) rather than silently stranding the item — same
// rationale as autoReopenWithBackoffGate.
func (l *BacklogLifecycleListener) retryPushFailedWithBackoffGate(ctx context.Context, itemID, itemTitle string) {
	due, justParked, gateErr := l.storage.RemediationDue(ctx, itemID, domain.StuckReasonPushFailed)
	if gateErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] retryPushFailedWithBackoffGate RemediationDue item=%s: %v", itemID, gateErr)
		due = true // fail open — see autoReopenWithBackoffGate's identical rationale
	}
	if justParked {
		l.notify(itemID,
			"Auto-rework paused",
			fmt.Sprintf("%s — automated push retry has been attempted %d times over an extended period without resolving. It now needs manual attention; use Reset to try again automatically.", itemTitle, MaxRemediationAttempts),
			8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
			3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		)
	}
	if !due {
		log.InfoLog.Printf("[BacklogLifecycle] retryPushFailedWithBackoffGate item=%s: push_failed remediation backoff not yet due, skipping retry", itemID)
		return
	}

	go func() {
		l.attemptPushRemediation(l.shutdownCtx, itemID, itemTitle)
	}()
}

// attemptPushRemediation is the push_failed remediation action dispatched by
// retryPushFailedWithBackoffGate once RemediationDue grants an attempt. It
// fetches the branch's current remote ref and merges it into the worktree
// (via the injected branchReconciler — git.MergeMainIntoWorktree in
// production, despite the "main" name it fetches+merges whatever branch
// name is passed; here that's the item's OWN branch, which reconciles the
// exact non-fast-forward-rejection shape live-repro'd on 2026-07-20:
// c2ad7bf3-91bf-4d47-8654-0f2f20869080's stelekit branch was rejected
// because something else advanced origin's copy of the same branch name).
// On a clean merge (or if the branch was already up to date — e.g. a
// previous remediation attempt already fixed it but the push itself failed
// for an unrelated transient reason), retries the full push+PR flow via
// pushAndCreatePR, which resolves the push_failed row on success through its
// existing resolveToPRPending call. A real content conflict is NOT
// auto-resolved — per the task scope, merge conflicts need a human; the
// item is left stuck (still governed by the normal backoff schedule, so it
// eventually parks after MaxRemediationAttempts) with a notification naming
// the conflicting files.
func (l *BacklogLifecycleListener) attemptPushRemediation(ctx context.Context, itemID, itemTitle string) {
	item, err := l.storage.GetBacklogItem(ctx, itemID)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] attemptPushRemediation GetBacklogItem item=%s: %v", itemID, err)
		return
	}
	if item.Status != string(BacklogStatusReview) {
		log.DebugLog.Printf("[BacklogLifecycle] attemptPushRemediation item=%s: status is now %s, not review — skipping", itemID, item.Status)
		return
	}

	sessions, err := l.storage.ListItemSessions(ctx, itemID)
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] attemptPushRemediation ListItemSessions item=%s: %v", itemID, err)
		return
	}
	var lastWork *ItemSessionSummary
	for i := range sessions {
		// Ascending by CreatedAt (ListItemSessions' query order) — keep
		// overwriting so this ends up holding the *most recent* work
		// session, mirroring the identical pattern in ReconcilePRPending's
		// ship-snapshot path.
		if sessions[i].Role == SessionRoleWork {
			s := sessions[i]
			lastWork = &s
		}
	}
	if lastWork == nil {
		log.WarningLog.Printf("[BacklogLifecycle] attemptPushRemediation item=%s: no work session found, cannot retry push", itemID)
		return
	}

	wt, wtErr := l.storage.GetWorktreeDataBySessionUUID(ctx, lastWork.SessionUUID)
	if wtErr != nil || wt.WorktreePath == "" {
		log.WarningLog.Printf("[BacklogLifecycle] attemptPushRemediation item=%s: no worktree available (%v), cannot retry push", itemID, wtErr)
		return
	}

	result, mergeErr := l.getBranchReconciler()(wt.WorktreePath, wt.BranchName)
	if mergeErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] attemptPushRemediation item=%s: fetch/merge of origin/%s failed: %v — will retry on next backoff window", itemID, wt.BranchName, mergeErr)
		return
	}
	if result.Conflicted {
		log.WarningLog.Printf("[BacklogLifecycle] attemptPushRemediation item=%s: origin/%s conflicts with the local worktree in %v — cannot auto-resolve", itemID, wt.BranchName, result.ConflictedFiles)
		l.notify(itemID,
			"Manual rebase needed",
			fmt.Sprintf("%s — the remote branch has diverged in a way that conflicts with this item's committed work (%s). Automated retry cannot resolve real content conflicts; resolve manually and push, or use Reset to try again automatically after fixing it.", itemTitle, strings.Join(result.ConflictedFiles, ", ")),
			7, // sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR
			3, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH
		)
		return
	}

	log.InfoLog.Printf("[BacklogLifecycle] attemptPushRemediation item=%s: origin/%s reconciled (upToDate=%v merged=%v), retrying push", itemID, wt.BranchName, result.UpToDate, result.Merged)
	l.pushAndCreatePR(ctx, item, *lastWork)
}

// RecordPRCreatedOutOfBand records a PR that was created for workSessionUUID
// through a path other than pushAndCreatePR and transitions the linked
// backlog item straight to pr_pending via the shared resolveToPRPending tail.
// (Named "Record", not "Notify", to avoid confusion with l.notify — the
// user-facing toast helper used elsewhere in this file; this method mutates
// backlog-item state, it doesn't just surface a message.)
//
// Why this exists: pushAndCreatePR is the *only* place that ever writes
// pr_pending, but it is reached exclusively via the automated
// handleReviewSessionExited(PASS) → pushAndCreatePR call chain. The Review
// Queue's manual "Create PR" button (web-app/src/components/sessions/
// ReviewQueuePanel.tsx) drives a completely separate path —
// SessionService.RunOneShot (server/services/session_service.go) — that runs
// an ad hoc `claude -p <prompt>` in the worktree and only ever persists the
// resulting PR URL onto the *session* record (inst.SetGitHubPR). It has no
// knowledge of backlog items at all, so a backlog-linked item whose PR was
// created this way never left "review" — ReconcilePRPending's FindPRPendingItems
// query structurally cannot find it, since it only looks at items already in
// pr_pending. Left in "review", the item instead accumulates
// in_progress↔review bounce churn from unrelated reconciliation and eventually
// reports stuck-reason BOUNCING instead of the correct pr_ready_unmerged. This
// is the root cause traced in docs/tasks/backlog-feature-improvement.md's
// "second, compounding root cause" note for PR #157.
//
// No-op if the listener is disabled, the caller has no PR info, the session
// isn't backlog-linked, or the item isn't currently "review" (avoids
// clobbering any other in-flight transition). That guard narrows, but does
// not eliminate, a race with a concurrent pushAndCreatePR call on the same
// item: TransitionBacklogItemStatus's precondition check is a read-then-write
// (Get, check in memory, then Save) rather than a true atomic compare-and-
// swap, so both calls can observe "review" and both succeed. That's harmless
// here — both write the same target status and equivalent PR fields — but it
// means two BacklogStatusEvent audit rows can be written instead of one, not
// that exactly one call is guaranteed to win.
//
// Known limitation (not fixed here — see PR description): unlike
// pushAndCreatePR, this does not attempt EnablePRAutoMerge, since it has no
// worktree/git handle to call it with; a PR created via this path currently
// requires a manual merge. extractPRURL's freeform-text parsing also means
// prURL/prNumber are not independently verified against GitHub before being
// persisted — acceptable for this single-operator tool's threat model, but
// worth knowing if RunOneShot's trust boundary ever changes.
func (l *BacklogLifecycleListener) RecordPRCreatedOutOfBand(ctx context.Context, workSessionUUID, prURL string, prNumber int) {
	if !l.enabled.Load() || prURL == "" || prNumber <= 0 {
		return
	}
	is, err := l.storage.GetItemSessionBySessionUUID(ctx, workSessionUUID)
	if err != nil {
		// Not backlog-linked (or lookup failed) — nothing to reconcile. Debug,
		// not Error: the overwhelming majority of RunOneShot callers are
		// non-backlog sessions, so this is the expected common case.
		log.DebugLog.Printf("[BacklogLifecycle] RecordPRCreatedOutOfBand GetItemSessionBySessionUUID(%s): %v", workSessionUUID, err)
		return
	}
	item, err := l.storage.GetBacklogItem(ctx, is.BacklogItemID)
	if err != nil {
		log.ErrorLog.Printf("[BacklogLifecycle] RecordPRCreatedOutOfBand GetBacklogItem session=%s item=%s: %v", workSessionUUID, is.BacklogItemID, err)
		return
	}
	if item.Status != string(BacklogStatusReview) {
		// Only review→pr_pending is a valid transition here. If the item is
		// already pr_pending (e.g. pushAndCreatePR beat us to it) or anywhere
		// else, leave it alone rather than fighting the item's real owner.
		return
	}

	prURLCopy, prNumCopy := prURL, prNumber
	if _, updateErr := l.storage.UpdateBacklogItem(ctx, item.ID, BacklogItemUpdate{
		PrURL:    &prURLCopy,
		PrNumber: &prNumCopy,
	}, nil); updateErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] RecordPRCreatedOutOfBand store PR fields item=%s: %v", item.ID, updateErr)
	}

	note := "PR created via manual Review Queue Create-PR flow (RunOneShot), not the automated pushAndCreatePR path"
	if transErr := l.resolveToPRPending(ctx, item.ID, note, "RecordPRCreatedOutOfBand"); transErr != nil {
		l.handlePRPendingTransitionFailed(ctx, item.ID, "RecordPRCreatedOutOfBand", transErr)
		return
	}
	log.InfoLog.Printf("[BacklogLifecycle] RecordPRCreatedOutOfBand item=%s session=%s → pr_pending (PR #%d %s, via manual RunOneShot flow)", item.ID, workSessionUUID, prNumber, prURL)
}

// CaptureShipSnapshot durably captures the GitHub PR/review/CI state and the
// per-file diff stats for item at the moment its PR merges, so that data
// survives worktree cleanup once the item reaches "done" — the core
// unified-vcs-widget requirement. It is a free function, not a method on
// BacklogLifecycleListener: it needs no state from that type beyond
// *Storage, which is passed explicitly here (per
// .claude/rules/interface-pollution-checklist.md, a method only earns its
// receiver when it genuinely needs the type's other state).
//
// Two data groups are captured independently — a failure in one must never
// discard a success in the other:
//   - Group A (GitHub): mapped from the already-fetched prStatus.
//     CaptureShipSnapshot makes no GitHub call of its own. prStatus == nil
//     means group A already failed before this function was even called
//     (e.g. the caller's own GetPRStatus errored) — that's a valid input,
//     not a bug. PRStatus does not expose a raw CI-conclusion string
//     (worktree_git.go:330-345's field list), so ShippedCheckConclusion is
//     derived from CIFailing as "failure"/"success" — a minor, accepted
//     fidelity gap versus Session.githubCheckConclusion.
//   - Group B (file stats): computed independently via
//     git.FileStatsBetween(item.RepoPath, wt.BaseCommitSHA, lastWork.LastCommitSha),
//     JSON-encoded into ShippedFileStats.
//
// Whichever group(s) succeed are written via one storage.UpdateBacklogItem
// call. ShippedSnapshotCaptureFailed is set true whenever either group
// failed; ShippedSnapshotAt is set whenever at least one group succeeded.
// ShippedCheckConclusion is never written as "failed" — that field holds
// only genuine CI-conclusion values; ShippedSnapshotCaptureFailed is the
// dedicated signal for a capture failure.
//
// CaptureShipSnapshot always returns nil: it never blocks the pr_pending →
// done transition, regardless of how many groups failed. Blocking done on a
// GitHub API hiccup or a pruned base SHA would leave a genuinely-merged item
// stuck in pr_pending forever, so this fails closed on data completeness,
// not on the workflow itself.
//
// No in-process cache/memoization is introduced here — every call is a
// direct write-through via UpdateBacklogItem. If a future caching layer is
// added on top of this function, it must return the locally-computed
// snapshot value rather than re-reading a cache slot after a lock is
// released, per .claude/rules/go-double-checked-locking.md.
func CaptureShipSnapshot(ctx context.Context, storage *Storage, item *BacklogItemData, prStatus *git.PRStatus, lastWork *ItemSessionSummary, wt *GitWorktreeData) error {
	var update BacklogItemUpdate
	groupAFailed := false
	groupBFailed := false
	anySucceeded := false

	// Group A: GitHub PR/CI/review state, from the already-fetched prStatus.
	if prStatus != nil {
		approvedCount := prStatus.ApprovedCount
		changesReqCount := prStatus.ChangesRequestedCount
		conclusion := "success"
		if prStatus.CIFailing {
			conclusion = "failure"
		}
		update.ShippedApprovedCount = &approvedCount
		update.ShippedChangesReqCount = &changesReqCount
		update.ShippedCheckConclusion = &conclusion
		anySucceeded = true
	} else {
		groupAFailed = true
		log.WarningLog.Printf("[BacklogLifecycle] CaptureShipSnapshot item=%s pr=%d group=github: prStatus unavailable", item.ID, item.PrNumber)
	}

	// Group B: per-file diff stats, independent of group A's outcome.
	if lastWork != nil && wt != nil {
		stats, statsErr := git.FileStatsBetween(item.RepoPath, wt.BaseCommitSHA, lastWork.LastCommitSha)
		if statsErr != nil {
			groupBFailed = true
			log.WarningLog.Printf("[BacklogLifecycle] CaptureShipSnapshot item=%s pr=%d group=file-stats: %v", item.ID, item.PrNumber, statsErr)
		} else if encoded, jsonErr := json.Marshal(stats); jsonErr != nil {
			groupBFailed = true
			log.WarningLog.Printf("[BacklogLifecycle] CaptureShipSnapshot item=%s pr=%d group=file-stats: marshal: %v", item.ID, item.PrNumber, jsonErr)
		} else {
			encodedStr := string(encoded)
			update.ShippedFileStats = &encodedStr
			anySucceeded = true
		}
	} else {
		groupBFailed = true
		log.WarningLog.Printf("[BacklogLifecycle] CaptureShipSnapshot item=%s pr=%d group=file-stats: worktree/last-work data unavailable", item.ID, item.PrNumber)
	}

	if groupAFailed || groupBFailed {
		captureFailed := true
		update.ShippedSnapshotCaptureFailed = &captureFailed
	}
	if anySucceeded {
		now := time.Now()
		update.ShippedSnapshotAt = &now
	}

	if _, updateErr := storage.UpdateBacklogItem(ctx, item.ID, update, nil); updateErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] CaptureShipSnapshot item=%s pr=%d: UpdateBacklogItem failed: %v", item.ID, item.PrNumber, updateErr)
	}

	return nil
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
			// Capture the durable ship snapshot (GitHub PR/CI/review state +
			// per-file diff stats) synchronously, before the done transition —
			// never as a background goroutine — so the data is written before
			// the worktree is eligible for cleanup (Story 3.3.1). prStatus is
			// fetched here at the merge-detection point specifically for the
			// snapshot; a fetch error is passed through as prStatus == nil
			// rather than skipping capture entirely, since CaptureShipSnapshot
			// treats a nil prStatus as "group A already failed" and still
			// captures group B (file stats) independently.
			snapshotPRStatus, snapshotStatusErr := g.GetPRStatus(item.PrNumber)
			if snapshotStatusErr != nil {
				log.WarningLog.Printf("[BacklogLifecycle] ReconcilePRPending GetPRStatus (ship snapshot) item=%s pr=%d: %v", item.ID, item.PrNumber, snapshotStatusErr)
				snapshotPRStatus = nil
			}

			itemData := backlogItemToData(item)

			var lastWork *ItemSessionSummary
			if sessions, sessErr := l.storage.ListItemSessions(ctx, item.ID.String()); sessErr != nil {
				log.WarningLog.Printf("[BacklogLifecycle] ReconcilePRPending ListItemSessions (ship snapshot) item=%s: %v", item.ID, sessErr)
			} else {
				for i := range sessions {
					// Ascending by CreatedAt (ListItemSessions' query order) —
					// keep overwriting so this ends up holding the *most
					// recent* work session, mirroring
					// backlog_service_ship_status.go:51-58.
					if sessions[i].Role == SessionRoleWork {
						lastWork = &sessions[i]
					}
				}
			}

			var wt *GitWorktreeData
			if lastWork != nil {
				if wtData, wtErr := l.storage.GetWorktreeDataBySessionUUID(ctx, lastWork.SessionUUID); wtErr != nil {
					log.WarningLog.Printf("[BacklogLifecycle] ReconcilePRPending GetWorktreeDataBySessionUUID (ship snapshot) item=%s session=%s: %v", item.ID, lastWork.SessionUUID, wtErr)
				} else {
					wt = &wtData
				}
			}

			if capErr := CaptureShipSnapshot(ctx, l.storage, &itemData, snapshotPRStatus, lastWork, wt); capErr != nil {
				// CaptureShipSnapshot always returns nil today; this branch
				// exists defensively in case that contract ever changes, and
				// must never block the done transition below.
				log.WarningLog.Printf("[BacklogLifecycle] ReconcilePRPending CaptureShipSnapshot item=%s pr=%d: %v", item.ID, item.PrNumber, capErr)
			}

			precondition := &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusPRPending)}
			if _, transErr := l.storage.TransitionBacklogItemStatus(ctx, item.ID.String(), BacklogStatusDone, precondition); transErr != nil {
				log.ErrorLog.Printf("[BacklogLifecycle] ReconcilePRPending done transition item=%s: %v", item.ID, transErr)
			} else {
				log.InfoLog.Printf("[BacklogLifecycle] ReconcilePRPending item=%s → done (PR #%d merged)", item.ID, item.PrNumber)
				// The item just reached done — resolve pr_ready_unmerged
				// immediately (Task 2.1.5a) rather than waiting for the
				// self-heal sweep's next tick.
				l.resolveStuckLogged(ctx, er, item.ID.String(), domain.StuckReasonPRReadyUnmerged, "ReconcilePRPending")
				// The PR is merged, so ship.md's "must still exist for a
				// possible one-shot /backlog/ship re-invocation" constraint
				// (see CleanupSlashCommands' doc comment) no longer applies —
				// this is the first point in the lifecycle where scaffolding
				// cleanup is safe. Best-effort: the worktree directory is
				// often already gone by now (Instance.Kill/Pause deletes it
				// independently), in which case these are no-ops.
				if wt != nil && wt.WorktreePath != "" {
					if cleanupErr := CleanupBacklogContextFile(wt.WorktreePath); cleanupErr != nil {
						log.WarningLog.Printf("[BacklogLifecycle] ReconcilePRPending CleanupBacklogContextFile item=%s: %v", item.ID, cleanupErr)
					}
					if cleanupErr := CleanupSlashCommands(wt.WorktreePath); cleanupErr != nil {
						log.WarningLog.Printf("[BacklogLifecycle] ReconcilePRPending CleanupSlashCommands item=%s: %v", item.ID, cleanupErr)
					}
				}
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
			// A closed-without-merging PR can never be pr_ready_unmerged again
			// under this pr_number; resolve immediately (self-heal would also
			// catch this once/if the status moves off pr_pending, but that may
			// not happen if no PRFixSpawner is configured below).
			l.resolveStuckLogged(ctx, er, item.ID.String(), domain.StuckReasonPRReadyUnmerged, "ReconcilePRPending/closed")
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
			// PR is open and healthy — wait for merge. Story 2.1.1: flag it
			// pr_ready_unmerged once it's been solo-ready (prReadyToMergeSolo)
			// past the threshold, using ONLY the already-fetched prStatus — no
			// second GitHub API call. Deliberately NOT gated on
			// github.DerivePRPriority(info)==PRPriorityReady, which requires
			// ApprovedCount>0 and is a permanent false-negative on a
			// self-authored single-user PR (pre-mortem F1; see
			// session/stuck_decisions.go prReadyToMergeSolo doc).
			info := &github.PRInfo{
				State:                 "open",
				IsDraft:               prStatus.IsDraft,
				ChangesRequestedCount: prStatus.ChangesRequestedCount,
				Mergeable:             prStatus.Mergeable,
				ApprovedCount:         prStatus.ApprovedCount,
			}
			if prStatus.CIFailing {
				info.CheckConclusion = "failure"
			}

			if prReadyToMergeSolo(info) {
				l.markPRReadyUnmerged(ctx, er, item.ID.String(), item.Title)
			} else {
				l.resolveStuckLogged(ctx, er, item.ID.String(), domain.StuckReasonPRReadyUnmerged, "ReconcilePRPending")
			}
			continue
		}

		// Poll-shaped resolve (else-branch, pre-mortem F2): the PR just
		// became CI-failing/blocked/conflicting while the item is still
		// pr_pending — a same-status clear the status-anchored self-heal
		// sweep structurally cannot see.
		l.resolveStuckLogged(ctx, er, item.ID.String(), domain.StuckReasonPRReadyUnmerged, "ReconcilePRPending/unhealthy")

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

// markPRReadyUnmerged marks/refreshes the durable pr_ready_unmerged row for
// itemID and notifies once it has been solo-ready (stuckPRReady) past
// prReadyThreshold — DB-backed notify-once dedup via notified_at, and
// first_detected_at survives restarts (unlike a process-uptime timer).
// Best-effort: errors are logged, never returned.
func (l *BacklogLifecycleListener) markPRReadyUnmerged(ctx context.Context, er *EntRepository, itemID, itemTitle string) {
	applied, err := er.MarkStuck(ctx, itemID, domain.StuckReasonPRReadyUnmerged, BacklogStatusPRPending,
		"PR is green, mergeable, and unmerged")
	if err != nil {
		log.WarningLog.Printf("[BacklogLifecycle] markPRReadyUnmerged MarkStuck item=%s: %v", itemID, err)
		return
	}
	if !applied {
		return
	}
	rows, findErr := er.FindOpenStuckStates(ctx)
	if findErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] markPRReadyUnmerged FindOpenStuckStates item=%s: %v", itemID, findErr)
		return
	}
	row, ok := findOpenStuckStateFor(rows, itemID, domain.StuckReasonPRReadyUnmerged)
	if !ok || row.NotifiedAt != nil || !stuckPRReady(row.FirstDetectedAt, time.Now()) {
		return
	}
	log.InfoLog.Printf("[BacklogLifecycle] item %s PR #%d ready to merge (unmerged past threshold)", itemID, row.PrNumber)
	l.notify(itemID,
		"PR ready to merge",
		fmt.Sprintf("%s — PR #%d is green, mergeable, and has been ready to merge for over %s. Merge it on GitHub.", itemTitle, row.PrNumber, prReadyThreshold),
		8, // sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING
		2, // sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM
	)
	if _, notifyErr := er.MarkStuckNotified(ctx, itemID, domain.StuckReasonPRReadyUnmerged); notifyErr != nil {
		log.WarningLog.Printf("[BacklogLifecycle] markPRReadyUnmerged MarkStuckNotified item=%s: %v", itemID, notifyErr)
	}
}
