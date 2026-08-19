package services

// backlog_service.go — core struct, constructor, setter methods, and shared helpers
// for BacklogService. RPC handlers are split across:
//   - backlog_service_query.go    (read-only handlers)
//   - backlog_service_lifecycle.go (state-mutation handlers)
//   - backlog_service_triage.go   (session spawning + triage orchestration)

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/scrollback"
	"github.com/tstapler/stapler-squad/session/tokens"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SessionCreator allows BacklogService to spawn sessions without importing handler internals.
type SessionCreator interface {
	CreateDirectorySession(ctx context.Context, title, path, prompt string, tags []string, oneShot bool, hidden bool) (*session.Instance, error)
	// CreateWorktreeSession spawns a session inside an already-created git worktree at
	// worktreePath. repoPath is the parent repo used for program resolution; worktreePath
	// must already exist on disk before this is called.
	CreateWorktreeSession(ctx context.Context, title, repoPath, worktreePath, prompt string, tags []string, oneShot bool, hidden bool) (*session.Instance, error)
}

// AutonomousDriverStarter allows BacklogService to start an AutonomousDriver on an existing instance.
// Wired via SetAutonomousDriverStarter from server.go after both services are constructed.
type AutonomousDriverStarter interface {
	StartAutonomousDriverForInstance(inst *session.Instance)
	StartAutonomousDriverWithTimeout(inst *session.Instance, startupTimeout time.Duration)
}

// SessionStopper allows BacklogService to kill live sessions.
// It is nil-safe: BacklogService degrades gracefully when not wired.
type SessionStopper interface {
	StopSessionByUUID(ctx context.Context, sessionUUID string) error
	// KillTmuxSessionByTitle kills a tmux session by its title, regardless of
	// whether the Instance is still tracked in memory. Used to clear stale tmux
	// sessions before re-triggering so the fresh session gets its --append-system-prompt.
	KillTmuxSessionByTitle(ctx context.Context, title string) error
	// IsSessionLive returns true if the session UUID is currently tracked in the
	// live in-memory poller. Used to distinguish genuinely-running sessions from
	// sessions that exited but whose DB records were not closed (e.g. after a
	// server restart that killed the underlying process).
	IsSessionLive(sessionUUID string) bool
	// KillTmuxPaneOnly closes the tmux pane for sessionUUID without touching its
	// worktree — unlike StopSessionByUUID/Instance.Kill, which also runs
	// CleanupWorktree. Rework rounds share one worktree/branch across their "-rN"
	// revisions (see buildRevisionTitle), so tearing down a finished round's
	// worktree would destroy the next round's checkout. No-op if the session
	// isn't tracked live (already gone).
	KillTmuxPaneOnly(ctx context.Context, sessionUUID string) error
	// ArchiveSessionByUUID soft-archives a session so it stops accumulating in
	// the default session list once its backlog item is done/superseded. No-op
	// (not an error) if the session isn't tracked live or is already archived.
	ArchiveSessionByUUID(ctx context.Context, sessionUUID string) error
	// TimeSinceLastMeaningfulOutput returns how long it has been since the live
	// Instance for sessionUUID last produced meaningful terminal output, backed
	// by the same Instance.GetTimeSinceLastMeaningfulOutput signal
	// review_queue_determiner.go's staleness detector uses — so "is this
	// session stale" has exactly one definition across the codebase instead of
	// each call site re-deriving its own. ok is false if the session isn't
	// currently tracked live (same "not live" cases as IsSessionLive); dur is
	// meaningless when ok is false.
	TimeSinceLastMeaningfulOutput(sessionUUID string) (dur time.Duration, ok bool)
}

// RepoWatchRemover lets BacklogService tell the background unfinished-changes
// scanner (session/unfinished.Scanner) to stop watching a repo path once its
// worktree has been removed from disk — see BUG-034. Without this, the
// scanner's watch list only ever grows: every worktree it was ever told about
// (via session auto-spider) keeps getting rescanned on every tick forever,
// even long after the session/item that created it finished and its worktree
// was deleted. Nil-safe: BacklogService degrades gracefully (the repo just
// stays watched a little longer, until the scanner's own self-pruning
// backstop catches it) when not wired.
type RepoWatchRemover interface {
	RemoveRepo(repoPath string)
}

// itemSourceBackend is a narrow interface for item source persistence; satisfied by *session.Storage.
type itemSourceBackend interface {
	CreateItemSource(ctx context.Context, data session.ItemSourceData) (*session.ItemSourceData, error)
	UpdateItemSource(ctx context.Context, id string, update session.ItemSourceUpdate) (*session.ItemSourceData, error)
}

// BacklogService handles Backlog RPCs.
type BacklogService struct {
	storage           *session.Storage
	sourceBackend     itemSourceBackend
	sessionCreator    SessionCreator
	sessionStopper    SessionStopper
	autonomousStarter AutonomousDriverStarter
	// repoWatchRemover tells the unfinished-changes scanner to stop watching a
	// worktree path once it's removed from disk (BUG-034). nil-safe — wired via
	// SetRepoWatchRemover.
	repoWatchRemover RepoWatchRemover
	// oneShotRunner drives TriggerShipPR (backlog_service_ship.go) — the
	// self-service "Ship PR" action on the item detail page. nil (the default)
	// makes TriggerShipPR return CodeUnimplemented; wired via SetOneShotRunner.
	oneShotRunner PRRunner
	cfg           *config.Config
	engine        session.WorkflowEngine
	// worktreeMu serializes context-file writes to the same worktree path so that
	// concurrent SpawnSessionFromItem / AttachSessionToItem calls cannot produce
	// a partially-written .claude/backlog-context.md.
	worktreeMu sync.Mutex

	// cfgMu guards concurrent reads of the two backlog-concurrency fields on cfg
	// (MaxConcurrentBacklogWorkItems, MaxAutoReworkIterations) against
	// DefaultsService.UpdateGlobalDefaults's writes to that SAME *config.Config
	// instance (wired via ConfigMu()/server/dependencies.go's
	// SetSharedBacklogConfig call) — see maxConcurrentBacklogWorkItems and
	// maxAutoReworkIterations. Previously cfg was a snapshot loaded once at
	// process start with no writer ever touching it, so raising the WIP cap via
	// Settings had zero runtime effect until a restart (PR #199 review F1).
	cfgMu sync.RWMutex

	// dequeueMu serializes the entire body of DequeueNextQueuedItems so the two
	// independent, unsynchronized call paths — BacklogLifecycleListener.
	// onSessionExited's `go l.triggerDequeue(...)` and the periodic
	// ReconcileStuck sweep — can never run concurrently and jointly overshoot
	// the WIP cap by each computing freeSlots from their own stale snapshot
	// (PR #199 review F2).
	dequeueMu sync.Mutex

	// spawnInFlight is a per-backlog-item "at most one work-session spawn in
	// flight" set, keyed by item ID, storing struct{} — the same LoadOrStore/
	// Delete atomic check-and-set idiom as review_queue_manager.go's
	// autoCreatePRInFlight field. SpawnSessionFromItem's read (ListItemSessions)
	// -> check (hasActiveWorkSession) -> write (CreateItemSession) sequence is
	// not otherwise atomic: two concurrent SpawnSessionFromItem calls for the
	// SAME item (e.g. the autonomous-driver respawn path racing a periodic
	// reconciliation sweep or a manual retrigger) can both read "no active work
	// session" before either has inserted its new ItemSession row, and both
	// proceed to spawn — confirmed live on 2026-07-19 (item d3227302 had two
	// literal overlapping "work" role ItemSessions). The isReopen path is the
	// most exposed: it skips SpawnSessionFromItem's own
	// TransitionBacklogItemStatus call entirely (only fresh, non-reopen spawns
	// transition ready->in_progress), so it has none of the optimistic-
	// concurrency protection (ExpectedUpdatedAt precondition) that already
	// protects AutoReopenAfterFailedReview / AutoReopenForPRFix's review/
	// pr_pending->in_progress transitions from double-firing.
	//
	// A sync.Map of per-item *sync.Mutex was considered and rejected: it would
	// leak one mutex per distinct item ID for the life of the process, whereas
	// this set is self-cleaning (LoadOrStore on entry, Delete via defer on
	// exit) and never grows past the number of spawns genuinely in flight at
	// once. This is a single-process server (see CLAUDE.md's architecture
	// overview) so an in-process guard is sufficient; a DB-level uniqueness
	// constraint was not needed. See SpawnSessionFromItem for the guarded
	// section.
	spawnInFlight sync.Map

	// headless triage pool and concurrency controls.
	headlessPool   headless.PoolClient
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	triageSem      chan struct{}

	// triageInFlight tracks, per item ID, whether a headless triage call this
	// process itself started is still genuinely running. tombstoneOrphanTriageSessions
	// has no other way to tell a still-running headless call apart from a dead one —
	// unlike a work/review session, there's no tmux session to query liveness against
	// (see BUG-054: before this field existed, tombstoneOrphanTriageSessions treated
	// every not-yet-ended headless triage session as dead unconditionally, so
	// retriggering triage for an item with a genuinely still-running call silently
	// orphaned that live call in the DB and started a fully redundant duplicate).
	// Same self-cleaning LoadOrStore-on-entry/Delete-via-defer-on-exit shape as
	// spawnInFlight above, for the same reason: a sync.Map here never leaks an entry
	// past the life of the call it tracks, and this is a single-process server so an
	// in-process guard is sufficient. Deliberately NOT persisted — on a fresh process
	// start after a restart, every item's entry is (correctly) absent, since no
	// goroutine in the new process could possibly still be running an old triage call.
	triageInFlight sync.Map

	// capabilityCheck gates the first codebase-read call per process lifetime (Story
	// 2.2.6). Defaults to headless.DefaultCapabilitySelfCheck (shared with
	// ReviewGateRunner so a failure discovered via either call site short-circuits
	// the other) but is a field — not a hardcoded package-var reference — so tests
	// can inject a fresh instance instead of fighting the singleton's sync.Once.
	capabilityCheck *headless.CodebaseReadCapabilitySelfCheck

	// triageCleanupTimeout bounds the post-LLM-call DB writes in TriggerTriage's
	// goroutine. An instance field (not a package var) so tests can override it on
	// their own *BacklogService without any shared global state or data-race risk
	// across concurrently running tests — see SetTriageCleanupTimeout and
	// TestTriggerTriage_SlowLLMCallDoesNotExpireCleanupContext, a regression test
	// for a bug where this timeout used to start counting down BEFORE the
	// (7-15 minute) LLM call instead of after it, so it was always already expired
	// by the time these persistence calls ran and every successful triage
	// silently failed to ever mark the item ready.
	triageCleanupTimeout time.Duration

	// pluginRegistry and syncKeyFunc back TriggerSync / GetSyncHistory. Both are
	// optional: if pluginRegistry is nil, TriggerSync degrades to CodeUnimplemented
	// the same way sessionCreator-dependent RPCs degrade when unwired.
	pluginRegistry *session.PluginRegistry
	syncKeyFunc    func() ([]byte, error)

	// syncFeatureEnabled reports whether the backlog feature (and therefore its
	// sync capability) is currently enabled. Optional: if nil, TriggerSync is
	// never gated by feature state (matches the other ItemSource RPCs, which
	// also don't self-gate). Wired to BacklogController.IsEnabled in production
	// so a manually-triggered sync can't run while the feature is toggled off.
	syncFeatureEnabled func() bool

	// resolveGitHubInput resolves a GitHub URL/shorthand to a local clone path,
	// cloning it if necessary. Defaults to session.ResolveGitHubInput; overridable
	// via SetGitHubResolver so tests don't need real network/git access.
	resolveGitHubInput func(input string) (string, *session.GitHubRef, error)

	// tokenStore and pricing power per-session cost estimates surfaced in the UI.
	tokenStore tokens.TokenStoreReader
	pricing    *tokens.PricingTable

	// eventBus publishes operator-facing notifications (e.g. rework-iteration-cap hit).
	// Optional — nil means those notifications are disabled.
	eventBus *events.EventBus

	// scrollbackMu guards scrollbackManager for concurrent Set/get access. Previously
	// unguarded, on the (false) assumption that SetScrollbackManager was always
	// called during single-threaded startup wiring before any concurrent RPC
	// handling began. In production, server/dependencies.go wires
	// SetScrollbackManager at Step 9+ (backlogSvc.SetScrollbackManager) while the
	// HTTP server can already be serving TriggerReReview RPCs that read
	// s.scrollbackManager concurrently — the field is genuinely racy and must be
	// mutex-guarded like every other optional-dependency field on this struct.
	scrollbackMu sync.RWMutex
	// scrollbackManager backs the "## Session Transcript" prompt section on the
	// empty-diff codebase-read re-review path (session.WriteReviewTranscriptFile).
	// Optional — nil (the default, until SetScrollbackManager is called) simply omits
	// that section. Guarded by scrollbackMu — see its doc comment.
	scrollbackManager *scrollback.ScrollbackManager

	// pipelineEngine resolves a BacklogItemData.PipelineMode's slash-command
	// set / prompts, and (via ContentHashFor) the content hash snapshotted
	// onto a new ItemSession at session-start (Epic 1.6). Wired by
	// NewBacklogService's constructor (Epic 1.5); may still be nil in tests
	// that don't pass one, so every call site that reads it must nil-check
	// and degrade to the built-in default pipeline rather than panic. See
	// triagePromptFor/reviewPromptFor/initialPromptFor below and
	// SpawnSessionFromItem/TriggerTriage's nil-guarded ContentHashFor reads
	// in backlog_service_triage.go.
	pipelineEngine session.PipelineEngine

	// pipelineModeRepo backs the PipelineMode CRUD RPCs (Epic 2.2):
	// CreatePipelineMode/UpdatePipelineMode/DeletePipelineMode/
	// GetPipelineMode/ListPipelineModes. Wired by NewBacklogService's
	// constructor from the same repository instance server/dependencies.go
	// uses to construct pipelineEngine (Epic 1.5.1a). May be nil in tests
	// that don't pass one; handlers nil-check and return CodeUnavailable.
	pipelineModeRepo session.PipelineModeRepository
}

// PipelineEngine returns the PipelineEngine injected at construction (nil if none was
// wired). Exported for the pointer-equality integration test proving BacklogService and
// BacklogLifecycleListener share a single PipelineEngine instance (Story 1.5.1).
func (s *BacklogService) PipelineEngine() session.PipelineEngine {
	return s.pipelineEngine
}

// ConfigMu exposes the mutex guarding cfg's backlog-concurrency fields so
// server/dependencies.go can wire the exact same mutex into
// DefaultsService.SetSharedBacklogConfig — see cfgMu's doc comment on the
// BacklogService struct for why this must be shared, not merely the same
// *config.Config pointer.
func (s *BacklogService) ConfigMu() *sync.RWMutex {
	return &s.cfgMu
}

// EnterpriseHosts returns the statically-configured GitHub Enterprise
// hostnames (normalized, github.com excluded), for callers that need to
// recognize GHE PR/issue URLs — e.g. server/mcp/tools_backlog.go's
// reportPRCreated and importGitHubIssue, which otherwise fall back to
// session.ParseGitHubURL's github.com-only matching and silently fail to
// recognize any GHE host. Mirrors SessionService.enterpriseHosts, minus that
// method's additional union with cached-account hosts (BacklogService has no
// UserPRCache dependency) — extend this if/when that's needed here too.
// Read under cfgMu's read lock for the same reason as
// maxConcurrentBacklogWorkItems below.
func (s *BacklogService) EnterpriseHosts() []string {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	configuredHosts := s.cfg.GetGitHubEnterpriseHosts()
	hosts := make([]string, 0, len(configuredHosts))
	seen := make(map[string]bool, len(configuredHosts))
	for _, h := range configuredHosts {
		host := githubpkg.NormalizeHost(h.Host)
		if host == "" || githubpkg.IsGitHubCom(host) || seen[host] {
			continue
		}
		seen[host] = true
		hosts = append(hosts, host)
	}
	return hosts
}

// maxConcurrentBacklogWorkItems reads cfg.MaxConcurrentBacklogWorkItemsOrDefault()
// under cfgMu's read lock so a concurrent DefaultsService.UpdateGlobalDefaults
// write (propagated via SetSharedBacklogConfig) is always observed by the next
// call rather than requiring a process restart (PR #199 review F1).
func (s *BacklogService) maxConcurrentBacklogWorkItems() int {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg.MaxConcurrentBacklogWorkItemsOrDefault()
}

// maxAutoReworkIterations reads cfg.MaxAutoReworkIterationsOrDefault() under
// cfgMu's read lock — see maxConcurrentBacklogWorkItems's doc comment.
func (s *BacklogService) maxAutoReworkIterations() int {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg.MaxAutoReworkIterationsOrDefault()
}

// autoSpawnReadyItemsEnabled reads cfg.AutoSpawnReadyItemsOrDefault() under
// cfgMu's read lock — see maxConcurrentBacklogWorkItems's doc comment.
func (s *BacklogService) autoSpawnReadyItemsEnabled() bool {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg.AutoSpawnReadyItemsOrDefault()
}

// Admit reports whether a new trigger-fired session may be created right now, per the
// same MaxConcurrentBacklogWorkItems WIP cap SpawnSessionFromItem's own gate enforces
// (backlog_service_triage.go). Implements server/workflows.AdmissionGate — wired into
// Scheduler at construction (server/dependencies.go) so Scheduler.FireNow/FireTrigger
// can no longer bypass this cap (webhook-triggers Epic 1.3, closing the collateral
// debt the 2026-07-12 OOM incident's WIP limit was meant to prevent everywhere).
func (s *BacklogService) Admit(ctx context.Context) (bool, error) {
	liveCount, err := s.countLiveBacklogWorkSessions(ctx)
	if err != nil {
		return false, fmt.Errorf("count live work sessions: %w", err)
	}
	return liveCount < s.maxConcurrentBacklogWorkItems(), nil
}

// SetEventBus wires in the event bus used to publish operator-facing notifications.
func (s *BacklogService) SetEventBus(b *events.EventBus) {
	s.eventBus = b
}

// NewBacklogService creates a BacklogService with all optional dependencies.
// storage and sourceBackend are typically the same (*session.Storage).
// sessionCreator and cfg may be nil; handlers degrade gracefully when absent.
//
// Degradation contract: If creator is nil, RPCs that spawn sessions will return
// CodeUnimplemented. This is expected in test environments where a real session
// manager is unavailable.
func NewBacklogService(storage *session.Storage, creator SessionCreator, cfg *config.Config, engine session.WorkflowEngine, pipelineEngine session.PipelineEngine, pipelineModeRepo session.PipelineModeRepository) *BacklogService {
	if engine == nil {
		engine = session.NewDefaultWorkflowEngine()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &BacklogService{
		storage:              storage,
		sourceBackend:        storage,
		sessionCreator:       creator,
		cfg:                  cfg,
		engine:               engine,
		pipelineEngine:       pipelineEngine,
		pipelineModeRepo:     pipelineModeRepo,
		shutdownCtx:          ctx,
		shutdownCancel:       cancel,
		triageSem:            make(chan struct{}, 8),
		triageCleanupTimeout: defaultTriageCleanupTimeout,
		resolveGitHubInput:   session.ResolveGitHubInput,
		capabilityCheck:      headless.DefaultCapabilitySelfCheck,
	}
}

// SetHeadlessPool wires the headless pool for autonomous triage calls.
func (s *BacklogService) SetHeadlessPool(pool headless.PoolClient) {
	s.headlessPool = pool
}

// claimantHostID returns the stable per-host/per-instance identifier for the
// process performing a backlog claim or attach, for cross-host provenance on
// ItemSession rows. It is distinct from STAPLER_SQUAD_INSTANCE (a config
// namespace, not an identity) and from session/contexts.go's cloud InstanceID
// (cloud-only, not populated for local/dev). Returns "" on any failure so a
// claim/attach never fails just because host-identity persistence did.
func (s *BacklogService) claimantHostID() string {
	if s.cfg == nil {
		return ""
	}
	id, err := s.cfg.GetOrCreateClaimantHostID()
	if err != nil {
		log.WarningLog.Printf("[claimantHostID] failed to resolve claimant host id: %v", err)
		return ""
	}
	return id
}

// SetScrollbackManager wires in the scrollback manager used to write a searchable
// session transcript file on the empty-diff codebase-read re-review path. Optional —
// nil (the default) simply omits the "## Session Transcript" prompt section. Safe to
// call concurrently with RPC handlers that read the scrollback manager.
func (s *BacklogService) SetScrollbackManager(sm *scrollback.ScrollbackManager) {
	s.scrollbackMu.Lock()
	defer s.scrollbackMu.Unlock()
	s.scrollbackManager = sm
}

// getScrollbackManager returns the current scrollback manager under a read lock.
func (s *BacklogService) getScrollbackManager() *scrollback.ScrollbackManager {
	s.scrollbackMu.RLock()
	defer s.scrollbackMu.RUnlock()
	return s.scrollbackManager
}

// SetCapabilityCheck overrides the codebase-read capability self-check instance.
// Exposed for tests, which need a fresh (non-shared) instance to avoid the
// package-level singleton's sync.Once making later tests observe an earlier test's
// cached result. Production callers should rely on the default.
func (s *BacklogService) SetCapabilityCheck(c *headless.CodebaseReadCapabilitySelfCheck) {
	s.capabilityCheck = c
}

// SetTriageCleanupTimeout overrides the default timeout for TriggerTriage's
// post-LLM-call DB writes. Exposed for tests; production callers should rely
// on the default.
func (s *BacklogService) SetTriageCleanupTimeout(d time.Duration) {
	s.triageCleanupTimeout = d
}

// SetPluginRegistry wires the item-source plugin registry, enabling TriggerSync.
func (s *BacklogService) SetPluginRegistry(registry *session.PluginRegistry) {
	s.pluginRegistry = registry
}

// SetSyncKeyFunc wires the encryption key provider used to decrypt item source
// tokens during a manual sync. May be left nil if no sources use encrypted
// tokens; SyncByID degrades gracefully (see session.SyncLoop.decryptConfigToken).
func (s *BacklogService) SetSyncKeyFunc(keyFunc func() ([]byte, error)) {
	s.syncKeyFunc = keyFunc
}

// SetSyncFeatureEnabledCheck wires a callback TriggerSync uses to refuse
// running while the backlog feature is disabled. Pass nil (the default) to
// leave TriggerSync ungated.
func (s *BacklogService) SetSyncFeatureEnabledCheck(check func() bool) {
	s.syncFeatureEnabled = check
}

// Shutdown cancels the service's background context, unblocking any goroutines
// waiting on the triage semaphore.
func (s *BacklogService) Shutdown() {
	s.shutdownCancel()
}

// SetSessionStopper wires the optional session stopper used to kill orphaned sessions on re-triage.
func (s *BacklogService) SetSessionStopper(stopper SessionStopper) {
	s.sessionStopper = stopper
}

// SetRepoWatchRemover wires the optional unfinished-changes scanner hook used
// to stop watching a worktree path once it's cleaned up (BUG-034).
func (s *BacklogService) SetRepoWatchRemover(remover RepoWatchRemover) {
	s.repoWatchRemover = remover
}

// SetAutonomousDriverStarter wires the optional autonomous driver starter.
// When set, SpawnSessionFromItem with autonomous=true will start an AutonomousDriver on the spawned instance.
func (s *BacklogService) SetAutonomousDriverStarter(starter AutonomousDriverStarter) {
	s.autonomousStarter = starter
}

// SetTokenStore wires cost-estimation data. Optional: if not set, cost fields remain 0.
func (s *BacklogService) SetTokenStore(ts tokens.TokenStoreReader, pt *tokens.PricingTable) {
	s.tokenStore = ts
	s.pricing = pt
}

// buildCostLookup returns a function that maps a tmux session UUID to its estimated
// USD cost. TokenStore keys by Claude conversation UUID (JSONL filename), so we
// resolve via session records. Returns a no-op func when token data is unavailable.
func (s *BacklogService) buildCostLookup() func(tmuxUUID string) float64 {
	if s.tokenStore == nil || s.pricing == nil || s.storage == nil {
		return nil
	}
	convIDByTmux := make(map[string]string)
	for _, rec := range s.storage.ListSessionRecords() {
		if rec.SessionID != "" && rec.ConversationID != "" {
			convIDByTmux[rec.SessionID] = rec.ConversationID
		}
	}
	ts := s.tokenStore
	pt := s.pricing
	return func(tmuxUUID string) float64 {
		convID := convIDByTmux[tmuxUUID]
		if convID == "" {
			return 0
		}
		r := ts.GetByUUID(convID)
		if r == nil {
			return 0
		}
		cost, _ := pt.EstimateCost(r)
		return cost
	}
}

// SetGitHubResolver overrides how GitHub URLs are resolved to local clone paths.
// Used by tests to avoid real network/git access; production wiring uses the
// session.ResolveGitHubInput default set in NewBacklogService.
func (s *BacklogService) SetGitHubResolver(fn func(input string) (string, *session.GitHubRef, error)) {
	s.resolveGitHubInput = fn
}

// resolveRepoPathInput resolves a GitHub URL/shorthand repo_path to a local clone
// path, cloning it if necessary (mirrors SessionService.CreateSession's handling
// of GitHub URLs). Plain filesystem paths pass through unchanged.
func (s *BacklogService) resolveRepoPathInput(input string) (string, error) {
	if input == "" || !session.IsGitHubURL(input) {
		return input, nil
	}
	localPath, _, err := s.resolveGitHubInput(input)
	if err != nil {
		return "", fmt.Errorf("failed to resolve GitHub URL %q: %w", input, err)
	}
	return localPath, nil
}

// itemSessionToProto converts an ItemSessionSummary to its proto representation.
// costFor, if non-nil, is called with the tmux session UUID to populate EstimatedCostUsd.
func itemSessionToProto(is session.ItemSessionSummary, costFor func(tmuxUUID string) float64) *sessionv1.ItemSession {
	p := &sessionv1.ItemSession{
		Id:                       is.ID,
		SessionUuid:              is.SessionUUID,
		SessionRole:              is.Role,
		CommitCountSinceSpawn:    int32(is.CommitCountSinceSpawn),
		LastCommitMessage:        is.LastCommitMessage,
		CreatedAt:                timestamppb.New(is.CreatedAt),
		PipelineModeSnapshot:     is.PipelineModeSnapshot,
		PipelineModeSnapshotHash: is.PipelineModeSnapshotHash,
		EndReason:                is.EndReason,
		FailureCapturePath:       is.FailureCapturePath,
		ClaimantHostId:           is.ClaimantHostID,
	}
	if is.StartedAt != nil {
		p.StartedAt = timestamppb.New(*is.StartedAt)
	}
	if is.EndedAt != nil {
		p.EndedAt = timestamppb.New(*is.EndedAt)
	}
	if is.LastCommitAt != nil {
		p.LastCommitAt = timestamppb.New(*is.LastCommitAt)
	}
	if is.LastFileTouchAt != nil {
		p.LastFileTouchAt = timestamppb.New(*is.LastFileTouchAt)
	}
	// Populate the review verdict when it was eagerly loaded.
	if rv := is.ReviewVerdict; rv != nil {
		p.ReviewVerdict = &sessionv1.ReviewVerdict{
			Id:             rv.ID,
			OverallOutcome: rv.OverallOutcome,
			Summary:        rv.Summary,
			DiffTokenCount: int32(rv.DiffTokenCount),
			DiffTruncated:  rv.DiffTruncated,
			OverrideBy:     rv.OverrideBy,
			OverrideReason: rv.OverrideReason,
			CreatedAt:      timestamppb.New(rv.CreatedAt),
		}
		if rv.OverrideAt != nil {
			p.ReviewVerdict.OverrideAt = timestamppb.New(*rv.OverrideAt)
		}
		// Deserialize per-criterion verdicts from JSON storage.
		if rv.PerCriterion != "" {
			var cvs []session.CriterionVerdict
			if jsonErr := json.Unmarshal([]byte(rv.PerCriterion), &cvs); jsonErr == nil {
				p.ReviewVerdict.PerCriterion = make([]*sessionv1.CriterionVerdict, len(cvs))
				for i, cv := range cvs {
					p.ReviewVerdict.PerCriterion[i] = &sessionv1.CriterionVerdict{
						CriterionIndex: int32(cv.CriterionIndex),
						Outcome:        string(cv.Outcome),
						Evidence:       cv.Evidence,
					}
				}
			}
		}
	}
	// Populate triage result when stored as JSON in ItemSession.TriageResult.
	if is.TriageResult != "" {
		var tr triageResultJSON
		if jsonErr := json.Unmarshal([]byte(is.TriageResult), &tr); jsonErr != nil {
			log.WarningLog.Printf("[itemSessionToProto] invalid triage_result JSON for session %s: %v", is.ID, jsonErr)
		} else {
			suggs := make([]*sessionv1.TriageSuggestion, len(tr.Suggestions))
			for i, sg := range tr.Suggestions {
				suggs[i] = &sessionv1.TriageSuggestion{Text: sg.Text, Rationale: sg.Rationale}
			}
			tasks := make([]*sessionv1.TriageTask, len(tr.Tasks))
			for i, t := range tr.Tasks {
				tasks[i] = &sessionv1.TriageTask{Text: t.Text, Estimate: t.Estimate, Category: t.Category}
			}
			// ClarifyingQuestions: MCP-submitted results store them as a top-level field.
			// Headless triage embeds questions as suggestions with rationale="question".
			// Derive from both sources so both paths populate the proto field correctly.
			clarifying := tr.ClarifyingQuestions
			for _, sg := range tr.Suggestions {
				if sg.Rationale == "question" {
					clarifying = append(clarifying, sg.Text)
				}
			}
			p.TriageResult = &sessionv1.TriageResult{
				Summary:             tr.Summary,
				Suggestions:         suggs,
				ClarifyingQuestions: clarifying,
				Tasks:               tasks,
				Iteration:           int32(tr.Iteration),
				Feedback:            tr.Feedback,
			}
		}
	}
	if costFor != nil && is.SessionUUID != "" {
		p.EstimatedCostUsd = costFor(is.SessionUUID)
	}
	// Fall back to the persisted cost for headless sessions where live lookup returns 0.
	if p.EstimatedCostUsd == 0 && is.EstimatedCostUsd > 0 {
		p.EstimatedCostUsd = is.EstimatedCostUsd
	}
	return p
}

// triageResultJSON is the JSON shape stored by submit_triage_result and read back
// by itemSessionToProto. Uses canonical session.TriageSuggestion / session.TriageTask
// to keep the schema in sync across the MCP tool, headless path, and proto conversion.
type triageResultJSON struct {
	Summary             string                     `json:"summary"`
	Suggestions         []session.TriageSuggestion `json:"suggestions"`
	ClarifyingQuestions []string                   `json:"clarifying_questions,omitempty"`
	Tasks               []session.TriageTask       `json:"tasks,omitempty"`
	Iteration           int                        `json:"iteration,omitempty"`
	Feedback            string                     `json:"feedback,omitempty"`
}

// backlogItemSummaryToProto maps a BacklogItemSummary to the proto BacklogItem message.
// Used by ListBacklogItems to avoid over-hydrating description/plan fields.
func backlogItemSummaryToProto(item *session.BacklogItemSummary, costFor func(tmuxUUID string) float64) *sessionv1.BacklogItem {
	p := &sessionv1.BacklogItem{
		Id:         item.ID,
		Title:      item.Title,
		Priority:   int32(item.Priority),
		Status:     string(item.Status),
		RepoPath:   item.RepoPath,
		Notes:      item.Notes,
		ExternalId: item.ExternalID,
		Labels:     item.Labels,
		PrUrl:      item.PrURL,
		PrNumber:   int32(item.PrNumber),
		CreatedAt:  timestamppb.New(item.CreatedAt),
		UpdatedAt:  timestamppb.New(item.UpdatedAt),
	}
	if item.ExternalURL != "" {
		p.ExternalUrl = &item.ExternalURL
	}
	if item.ArchivedAt != nil {
		p.ArchivedAt = timestamppb.New(*item.ArchivedAt)
	}
	if item.AcceptanceCriteria != "" {
		criteria, err := session.ParseAcCriteria(item.AcceptanceCriteria)
		if err == nil {
			protoAC := make([]*sessionv1.AcCriterion, len(criteria))
			for i, c := range criteria {
				protoAC[i] = &sessionv1.AcCriterion{
					Index:  int32(c.Index),
					Text:   c.Text,
					Status: string(c.Status),
				}
			}
			p.AcceptanceCriteria = protoAC
		}
	}
	if len(item.ItemSessions) > 0 {
		protoSessions := make([]*sessionv1.ItemSession, len(item.ItemSessions))
		var totalCost float64
		for i, is := range item.ItemSessions {
			ps := itemSessionToProto(is, costFor)
			protoSessions[i] = ps
			totalCost += ps.EstimatedCostUsd
		}
		p.ItemSessions = protoSessions
		p.TotalEstimatedCostUsd = totalCost
	}
	return p
}

// protoWorkflowEngine is a stateless, read-only WorkflowEngine used only to
// surface AllowedTransitions on the wire (backlogItemToProto below) — package
// state is safe here since the underlying transitions map is never mutated
// after construction. Not s.engine: backlogItemToProto is a free function
// called from many BacklogService methods, and threading an engine parameter
// through every call site would be a much larger change for the same result.
var protoWorkflowEngine = session.NewDefaultWorkflowEngine()

// allowedTransitionStrings returns the string form of
// protoWorkflowEngine.AllowedTransitions(from), for BacklogItem.allowed_transitions.
func allowedTransitionStrings(from session.BacklogStatus) []string {
	targets := protoWorkflowEngine.AllowedTransitions(from)
	out := make([]string, len(targets))
	for i, t := range targets {
		out[i] = string(t)
	}
	return out
}

// backlogItemToProto maps a BacklogItemData to the proto BacklogItem message.
func backlogItemToProto(item *session.BacklogItemData, costFor func(tmuxUUID string) float64) *sessionv1.BacklogItem {
	p := &sessionv1.BacklogItem{
		Id:                  item.ID,
		Title:               item.Title,
		Description:         item.Description,
		Priority:            int32(item.Priority),
		Status:              item.Status,
		RepoPath:            item.RepoPath,
		SkipReviewGate:      item.SkipReviewGate,
		SkipPlanning:        item.SkipPlanning,
		AutoSpawnSession:    item.AutoSpawnSession,
		AutoCreatePr:        item.AutoCreatePR,
		PipelineMode:        &item.PipelineMode,
		Category:            &item.Category,
		PlanApproved:        item.PlanApproved,
		PlanArtifactsPath:   item.PlanArtifactsPath,
		PlanRejectionReason: item.PlanRejectionReason,
		Notes:               item.Notes,
		ExternalId:          item.ExternalID,
		Labels:              item.Labels,
		SourceId:            item.SourceID,
		PrUrl:               item.PrURL,
		PrNumber:            int32(item.PrNumber),
		CreatedAt:           timestamppb.New(item.CreatedAt),
		UpdatedAt:           timestamppb.New(item.UpdatedAt),
		AllowedTransitions:  allowedTransitionStrings(session.BacklogStatus(item.Status)),
	}
	if item.ExternalURL != "" {
		p.ExternalUrl = &item.ExternalURL
	}
	if item.PlanApprovedAt != nil {
		p.PlanApprovedAt = timestamppb.New(*item.PlanApprovedAt)
	}
	if item.PlanRejectedAt != nil {
		p.PlanRejectedAt = timestamppb.New(*item.PlanRejectedAt)
	}
	if item.ArchivedAt != nil {
		p.ArchivedAt = timestamppb.New(*item.ArchivedAt)
	}
	if item.ReworkCapOverride != nil {
		override := int32(*item.ReworkCapOverride)
		p.ReworkCapOverride = &override
	}

	// Parse acceptance criteria JSON into repeated AcCriterion.
	if item.AcceptanceCriteria != "" {
		criteria, err := session.ParseAcCriteria(item.AcceptanceCriteria)
		if err == nil {
			protoAC := make([]*sessionv1.AcCriterion, len(criteria))
			for i, c := range criteria {
				protoAC[i] = &sessionv1.AcCriterion{
					Index:  int32(c.Index),
					Text:   c.Text,
					Status: string(c.Status),
				}
			}
			p.AcceptanceCriteria = protoAC
		}
	}

	// Populate item sessions when they were eagerly loaded.
	if len(item.ItemSessions) > 0 {
		protoSessions := make([]*sessionv1.ItemSession, len(item.ItemSessions))
		var totalCost float64
		for i, is := range item.ItemSessions {
			ps := itemSessionToProto(is, costFor)
			protoSessions[i] = ps
			totalCost += ps.EstimatedCostUsd
		}
		p.ItemSessions = protoSessions
		p.TotalEstimatedCostUsd = totalCost
	}

	// Populate status events when they were eagerly loaded.
	if len(item.StatusEvents) > 0 {
		protoEvents := make([]*sessionv1.BacklogStatusEvent, len(item.StatusEvents))
		for i, ev := range item.StatusEvents {
			protoEvents[i] = &sessionv1.BacklogStatusEvent{
				Id:          ev.ID,
				FromStatus:  ev.FromStatus,
				ToStatus:    ev.ToStatus,
				TriggeredBy: ev.TriggeredBy,
				CreatedAt:   timestamppb.New(ev.CreatedAt),
				Note:        ev.Note,
			}
		}
		p.StatusEvents = protoEvents
	}

	// Populate progress notes (the implementer's report_progress audit trail)
	// when they were eagerly loaded.
	if len(item.ProgressNotes) > 0 {
		protoNotes := make([]*sessionv1.BacklogProgressNote, len(item.ProgressNotes))
		for i, n := range item.ProgressNotes {
			protoNotes[i] = &sessionv1.BacklogProgressNote{
				Id:             n.ID,
				CriterionIndex: int32(n.CriterionIndex),
				Note:           n.Note,
				Status:         n.Status,
				CreatedAt:      timestamppb.New(n.CreatedAt),
			}
		}
		p.ProgressNotes = protoNotes
	}

	// Populate activity notes (the ungated post_backlog_update log) when they
	// were eagerly loaded.
	if len(item.ActivityNotes) > 0 {
		protoActivityNotes := make([]*sessionv1.BacklogActivityNote, len(item.ActivityNotes))
		for i := range item.ActivityNotes {
			protoActivityNotes[i] = activityNoteDataToProto(&item.ActivityNotes[i])
		}
		p.ActivityNotes = protoActivityNotes
	}

	return p
}

// itemSourceToProto maps an ItemSourceData to the proto ItemSource message.
func itemSourceToProto(src *session.ItemSourceData) *sessionv1.ItemSource {
	p := &sessionv1.ItemSource{
		Id:                    src.ID,
		PluginId:              src.PluginID,
		DisplayName:           src.DisplayName,
		Enabled:               src.Enabled,
		ForwardSyncEnabled:    src.ForwardSyncEnabled,
		BackwardSyncEnabled:   src.BackwardSyncEnabled,
		ForwardSyncCloseLabel: src.ForwardSyncCloseLabel,
		TokenConfigured:       src.TokenConfigured,
		CreatedAt:             timestamppb.New(src.CreatedAt),
		UpdatedAt:             timestamppb.New(src.UpdatedAt),
	}
	if src.LastSyncedAt != nil {
		p.LastSyncedAt = timestamppb.New(*src.LastSyncedAt)
	}
	return p
}

// commitAndPushItemWorktrees commits any dirty work and pushes branches to the remote
// for all work-role item sessions. Called BEFORE status transitions to ensure changes
// are durable before the item is marked done. Errors are logged but not returned.
func (s *BacklogService) commitAndPushItemWorktrees(ctx context.Context, sessions []session.ItemSessionSummary) {
	for _, is := range sessions {
		if is.SessionUUID == "" || is.Role != string(session.SessionRoleWork) {
			continue
		}
		wt, err := s.storage.GetWorktreeDataBySessionUUID(ctx, is.SessionUUID)
		if err != nil || wt.WorktreePath == "" {
			continue
		}
		g := git.NewGitWorktreeFromStorage(wt.RepoPath, wt.WorktreePath, wt.SessionName, wt.BranchName, wt.BaseCommitSHA)
		commitMsg := fmt.Sprintf("[claudesquad] save work before done (session %s)", is.SessionUUID)
		if commitErr := g.CommitChanges(commitMsg); commitErr != nil {
			log.WarningLog.Printf("[commitAndPushItemWorktrees] commit failed path=%s: %v", wt.WorktreePath, commitErr)
		}
		if pushErr := g.PushBranch(); pushErr != nil {
			log.WarningLog.Printf("[commitAndPushItemWorktrees] push failed path=%s: %v", wt.WorktreePath, pushErr)
		}
	}
}

// cleanupItemWorktrees removes git worktrees for work-role item sessions.
// Call commitAndPushItemWorktrees first to ensure changes are durable.
// Errors are logged but do not fail the caller — cleanup is best-effort.
func (s *BacklogService) cleanupItemWorktrees(ctx context.Context, sessions []session.ItemSessionSummary) {
	s.cleanupItemWorktreesExcept(ctx, sessions, "")
}

// cleanupItemWorktreesExcept is cleanupItemWorktrees with one path exempted from
// removal. Reopen/rework spawns reuse the same "backlog/<item>" branch and worktree
// directory across revisions (see SpawnSessionFromItem step 10's comment) rather than
// creating a fresh one, so a prior work session's worktree row can point at the exact
// path the brand-new session just started using. Cleaning that up unconditionally —
// as every caller used to — deleted the directory out from under the session that
// just reused it, leaving a still-in_progress/review item with no worktree at all
// (diffs and re-review's codebase-read verification both came up empty). exceptPath
// lets the reopen call site keep that one path alive while still clearing out any
// genuinely stale worktree from an earlier, differently-named revision (e.g. the
// item's title changed between rework rounds).
func (s *BacklogService) cleanupItemWorktreesExcept(ctx context.Context, sessions []session.ItemSessionSummary, exceptPath string) {
	for _, is := range sessions {
		if is.SessionUUID == "" {
			continue
		}
		if is.Role != string(session.SessionRoleWork) {
			continue
		}
		wt, err := s.storage.GetWorktreeDataBySessionUUID(ctx, is.SessionUUID)
		if err != nil || wt.WorktreePath == "" {
			continue
		}
		if exceptPath != "" && wt.WorktreePath == exceptPath {
			continue
		}
		g := git.NewGitWorktreeFromStorage(wt.RepoPath, wt.WorktreePath, wt.SessionName, wt.BranchName, wt.BaseCommitSHA)
		if cleanErr := g.Cleanup(); cleanErr != nil {
			log.WarningLog.Printf("[cleanupItemWorktrees] failed to cleanup worktree path=%s: %v", wt.WorktreePath, cleanErr)
			continue
		}
		// The worktree directory is gone — stop the unfinished-changes scanner
		// from rescanning it forever (BUG-034). Only after Cleanup succeeds: a
		// failed cleanup means the path may still be a real worktree worth
		// watching.
		if s.repoWatchRemover != nil {
			s.repoWatchRemover.RemoveRepo(wt.WorktreePath)
		}
	}
}

// archiveItemWorkSessions soft-archives every work- or review-role session in
// sessions so it stops accumulating in the default session list, and kills its live
// tmux pane so the underlying claude process (and its MCP server subprocess fleet)
// doesn't keep running indefinitely — ArchiveSessionByUUID alone only hides the
// session from the UI, it does not stop it (root cause of the 2026-07-29 OOM:
// dozens of superseded/completed work AND review sessions still live). Worktree
// cleanup is handled separately by cleanupItemWorktreesExcept, so this uses
// KillTmuxPaneOnly (pane only), not StopSessionByUUID (which also destroys the
// worktree). Review sessions are included because they leak the same way work
// sessions do — a review session that already wrote its verdict (the precondition
// for reaching either call site below) has no further reason to stay alive, but
// nothing else stops it. Callers are: (1) terminal status transitions (done/
// archived), where every session for the item is superseded, and (2) rework
// respawns, where only the sessions loaded *before* the new spawn (i.e. every prior
// round, including the review session whose FAIL verdict triggered the reopen) are
// passed in — the brand-new session is never included. Nil-safe (sessionStopper may
// be unwired, e.g. in tests) and best-effort: archival/kill failures are logged, not
// returned, matching cleanupItemWorktreesExcept's contract.
func (s *BacklogService) archiveItemWorkSessions(ctx context.Context, sessions []session.ItemSessionSummary) {
	if s.sessionStopper == nil {
		return
	}
	for _, is := range sessions {
		if is.SessionUUID == "" || !session.IsTmuxBackedSessionRole(is.Role) {
			continue
		}
		if err := s.sessionStopper.ArchiveSessionByUUID(ctx, is.SessionUUID); err != nil {
			log.WarningLog.Printf("[archiveItemWorkSessions] failed to archive session=%s: %v", is.SessionUUID, err)
		}
		if err := s.sessionStopper.KillTmuxPaneOnly(ctx, is.SessionUUID); err != nil {
			log.WarningLog.Printf("[archiveItemWorkSessions] failed to kill tmux pane session=%s: %v", is.SessionUUID, err)
		}
	}
}
