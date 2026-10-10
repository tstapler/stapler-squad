package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/prompts"
	"github.com/tstapler/stapler-squad/session/search"
	"github.com/tstapler/stapler-squad/session/sshremote"
	"github.com/tstapler/stapler-squad/session/streamhub"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// Compile-time interface check: SessionService must implement the full ConnectRPC handler.
var _ sessionv1connect.SessionServiceHandler = (*SessionService)(nil)

// Compile-time interface check: SessionService satisfies BacklogService's
// consumer-defined SessionSteerer interface.
var _ SessionSteerer = (*SessionService)(nil)

// resumeIDRe validates the client-supplied resume_id field: must be a standard UUID.
// createSessionTimeout bounds the synchronous portion of CreateSession (fast-fail
// validation, config/remote/restart-source resolution, alias-existence check,
// title-uniqueness check, and instance construction/persist/publish). As of
// Epic 2.1 (async-session-creation), GitHub URL resolution (`git clone`, up to
// ~120s for large repos per research) no longer runs on this synchronous path
// -- it is deferred to the Background Resolution Pipeline (Epic 2.2, see
// runBackgroundResolutionPipeline in session_creation_pipeline.go and its
// own maxCreationResolutionTimeout), same as the tmux startup poll (~10s)
// that already ran there. This value stays generous rather than being
// tightened immediately, since other synchronous sub-operations (e.g. remote
// SSH dial/worktree setup) can still be slow.
const createSessionTimeout = 150 * time.Second

var resumeIDRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// testTmuxServerSocketCounter gives each SessionService created within a test binary run a
// unique tmux -L socket suffix (combined with os.Getpid() for cross-process uniqueness), so
// concurrent tests never contend on the same isolated socket. See SessionService.testTmuxServerSocket.
var testTmuxServerSocketCounter uint64

// ReactiveQueueManager is an interface to avoid circular dependencies.
// The actual implementation is in server/review_queue_manager.go
type ReactiveQueueManager interface {
	AddStreamClient(ctx context.Context, filters interface{}) (<-chan *sessionv1.ReviewQueueEvent, string)
	RemoveStreamClient(clientID string)
	OnControllerStatusChange(inst *session.Instance, newStatus detection.DetectedStatus)
}

// FeatureController is implemented by components that can be enabled/disabled at runtime.
// Used by GetFeatureFlags/UpdateFeatureFlag to toggle named subsystems.
type FeatureController interface {
	Enable(ctx context.Context) error
	Disable() error
	IsEnabled() bool
}

// SessionService implements the SessionServiceHandler interface for ConnectRPC.
type SessionService struct {
	storage           session.InstanceStore
	eventBus          *events.EventBus
	statusManager     *session.InstanceStatusManager
	reviewQueuePoller *session.ReviewQueuePoller

	// guardedSteer holds the per-session nudge guard and its test seams; see
	// session_service_guarded_steer.go. The zero value is production-ready.
	guardedSteer guardedSteerState

	// deliveryGate is the hidden-session delivery gate installed as the event bus
	// publish filter at construction (nil when built through NewSessionService).
	deliveryGate *deliverygate.Gate

	// tapRegistry backs SetCaptureTap/GetCaptureTap. nil means the process-wide
	// streamhub.DefaultTapRegistry, which is what the terminal streams use.
	tapRegistry *streamhub.TapRegistry

	// sessionTagPoller drives the Phase 4 LLM fallback tag classification
	// (session-classifier-pipeline Epic 4.4). nil when HeadlessPool is nil (no
	// claude binary found) — every AddInstance/RemoveInstance call site below
	// guards for nil the same way reviewQueuePoller does.
	sessionTagPoller *session.SessionTagClassificationPoller

	// concStorage is the concrete backing store, used for operations (like
	// ListWorkspacePeers) not part of the InstanceStore interface. nil when storage is a
	// fake InstanceStore (tests) — callers must nil-check.
	concStorage *session.Storage

	// inFlightWorktreeSpawns claims a canonicalized worktree path (-> claiming session
	// UUID) for the duration of startLocked's check-through-spawn window, closing the
	// TOCTOU race between two concurrent CreateSession calls resolving the same
	// worktree path before either has spawned (worktree-envvars-hijack Story 3.3.2c).
	// Wired into each Instance via SetWorktreeSpawnReservation in wireCallbacks.
	inFlightWorktreeSpawns sync.Map

	// Extracted domain services.
	reviewQueueSvc  *ReviewQueueService
	searchSvc       *SearchService
	githubSvc       *GitHubService
	workspaceSvc    *WorkspaceService
	configSvc       *ConfigService
	notificationSvc *NotificationService
	approvalSvc     *ApprovalService
	utilitySvc      *UtilityService
	rulesSvc        *RulesService
	// taggingRulesSvc is exposed over ListTaggingRules/UpsertTaggingRule/DeleteTaggingRule
	// (Phase 5 of the session-classifier-pipeline project). nil-safe: those RPC handlers
	// guard with `if s.taggingRulesSvc == nil` and return CodeUnimplemented, mirroring the
	// pattern documented for rulesSvc's own nil-safe accessors below.
	taggingRulesSvc *TaggingRulesService
	// taggingEngine is the same live engine taggingRulesSvc mutates on CRUD — injected into
	// every Instance (wireCallbacks) so session/instance_actor_setters.go's reclassifyTagsLocked
	// fixpoint hook has a real rule set to evaluate (session-classifier-pipeline Epic 3.3/Task
	// 2.3.3d). nil-safe like taggingRulesSvc; SetTaggingEngine(nil) just disables auto-tagging.
	taggingEngine *classifier.TaggingEngine

	// External session discovery (for mux-enabled sessions from external terminals)
	externalDiscovery *session.ExternalSessionDiscovery

	// historyLinker tracks JSONL conversation files per session.
	// Must be kept in sync with deletions so the shutdown hook does not
	// re-persist sessions the user has deleted.
	historyLinker *session.HistoryLinker

	// approvalStore holds pending Claude Code hook approval requests.
	approvalStore *ApprovalStore

	// deleteSessionCleanupTimeout bounds DeleteSession's cleanup-timeout warning;
	// see defaultDeleteSessionCleanupTimeout's doc comment. Overridable via
	// SetDeleteSessionCleanupTimeout for tests.
	deleteSessionCleanupTimeout time.Duration

	// databaseSvc handles workspace/database switcher RPCs.
	databaseSvc *DatabaseService

	// tmuxStreamerManager caches ExternalTmuxStreamer instances per tmux session
	// name (main sessions and shell siblings alike). Wired in so StopShell can
	// evict a shell's streamer on close instead of letting a stale/degraded one
	// persist across shell restarts.
	tmuxStreamerManager *session.ExternalTmuxStreamerManager

	// userPRCache supplies enterprise hosts from dynamically-added GitHub
	// accounts (gh CLI import, device auth) for CreateSession's GitHub URL
	// detection — mirrors ListGitHubAccounts' host union in github_user_service.go.
	userPRCache *githubpkg.UserPRCache

	// fileSvc handles file tree browsing RPCs (ListFiles, GetFileContent).
	fileSvc *FileService

	// pathCompletionSvc handles filesystem path completion RPCs.
	pathCompletionSvc *PathCompletionService

	// slashCommandSvc resolves slash commands from disk and built-ins.
	slashCommandSvc *SlashCommandService

	// defaultsSvc handles session defaults configuration RPCs.
	defaultsSvc *DefaultsService

	// slackConfigSvc handles GetSlackConfig/UpdateSlackConfig/TestSlackWebhook RPCs.
	slackConfigSvc *SlackConfigService

	// julesConfigSvc handles GetJulesConfig/UpdateJulesConfig/
	// TestJulesConnection/ConfirmEgressConsent RPCs (google-jules-integration
	// Epic 2.4). Constructed with nil dependencies here — real ones (keychain,
	// source registry, poller) are wired post-construction via
	// SetJulesConfigDependencies once server/dependencies.go builds them
	// (Task 2.4.4a), since Jules-enablement is only known after config load.
	julesConfigSvc *JulesConfigService

	// credChain resolves AI-provider credentials (Anthropic, Google, Jules)
	// for capacityMonitor's limits clients. Its JulesCredentialSource starts
	// with its own *jules.KeyringTokenSource (built by NewDefaultChain, which
	// runs before server/dependencies.go has one to share); SetJulesKeyringTokenSource
	// swaps that for the process-wide instance once dependencies.go builds it,
	// so this chain never resolves the Jules OS-keychain entry through a
	// second, independent cache/circuit-breaker.
	credChain *CredentialChain

	// callbackConfigSvc handles GetCallbackConfig/UpdateCallbackConfig RPCs
	// (webhook-triggers Phase 5, FR7).
	callbackConfigSvc *CallbackConfigService

	// streamHubRolloutSvc handles the stream-hub staged-rollout RPCs
	// (terminal-multi-connection-streaming Story 3.3).
	streamHubRolloutSvc *StreamHubRolloutService

	// launcherPresetsSvc handles the GetLauncherPresets RPC.
	launcherPresetsSvc *LauncherPresetsService

	// projectSvc handles Project CRUD RPCs.
	projectSvc *ProjectService

	// checkpointSvc handles CreateCheckpoint/ListCheckpoints/ClearConversationState RPCs.
	checkpointSvc *CheckpointService

	// prCreationSvc handles DraftPullRequest/CreatePullRequest RPCs.
	prCreationSvc *PRCreationService

	// featureFlagSvc handles GetFeatureFlags/UpdateFeatureFlag RPCs.
	featureFlagSvc *FeatureFlagService

	// terminalSvc handles GetTerminalSnapshot/WriteToSession RPCs.
	terminalSvc *TerminalService

	// promptStore persists prompt history for the "initial prompt" dropdown.
	promptStore *prompts.PromptStore

	// scrollbackMgr provides access to per-session scrollback sequence numbers
	// for checkpoint creation. May be nil if not wired (seq defaults to 0).
	scrollbackMgr ScrollbackSequencer

	// mcpServerURLFn lazily resolves the URL of the stapler-squad HTTP MCP
	// endpoint. It is invoked (not read as a stored string) at the point of
	// use so it always reflects the server's real bound address, even when
	// the listener was constructed before Start() resolved an OS-assigned
	// port (PORT=0). When non-nil, its result is passed to new sessions via
	// InstanceOptions.MCPServerURL.
	mcpServerURLFn func() string

	// errorRegistry persists deduplicated RPC errors to SQLite.
	// May be nil when wired without an ent-backed storage (e.g. in tests).
	errorRegistry *ErrorRegistry

	// backlogLifecycleListener is wired to each newly created session so that
	// backlog item state transitions fire when the session exits.
	backlogLifecycleListener *session.BacklogLifecycleListener

	// sessionSummaryGenerator is wired to each newly created session (alongside
	// backlogLifecycleListener, at the same call sites) so that session-completion-
	// summary generation fires on exit/stop. Nil until SetSessionSummaryGenerator is
	// called (session/session_summary_service.go's ent-client/headless-pool wiring
	// happens after SessionService construction).
	sessionSummaryGenerator *session.SessionSummaryGenerator

	// capacityMonitor tracks rate limits and triggers transitions.
	capacityMonitor *CapacityMonitor

	// quotaGate feeds account-wide rate-limit detections into the quota
	// headroom gate that pauses/resumes backlog automation. Late-wired via
	// SetQuotaGate since QuotaGate needs backlogCtrl, which doesn't exist yet
	// inside NewSessionService.
	quotaGate *QuotaGate

	// analyticsClient is the ent client for the analytics database (escape events, etc.).
	// May be nil when escape analytics is disabled or in tests that don't need it.
	analyticsClient *ent.Client

	// memoryCacheReader provides per-session RSS and system memory percentage.
	// Wired to the HibernationSweeper after startup. May be nil (fields default to 0).
	memoryCacheReader session.MemoryCacheReader

	// headlessPool is the shared LLM pool for non-interactive AI calls (RunOneShot, etc.).
	// May be nil when the claude binary is not found at startup.
	headlessPool *headless.Pool
	// headlessClient routes RunOneShot's custom-prompt calls through the backend
	// selector; nil falls back to headlessPool.
	headlessClient headless.PoolClient

	// autonomousSvc manages the lifecycle of AutonomousDriver instances.
	autonomousSvc *AutonomousOrchestrationService

	// workflowSvc handles workflow CRUD and RunWorkflow RPC delegation.
	// Injected after construction via SetWorkflowService to avoid bootstrapping cycle.
	workflowSvc *WorkflowService

	// workflowRepo is used to populate the workflow meta cache.
	// Injected via SetWorkflowRepository to avoid bootstrapping cycle.
	workflowRepo session.WorkflowRepository

	// workflowMetaCache provides workflow name and retention settings keyed by workflow UUID.
	// Populated on startup and refreshed every minute. Protected by workflowMetaMu.
	workflowMetaCache map[string]workflowMeta
	workflowMetaMu    sync.RWMutex

	// registry is the live-handle map for all running sessions. Wired after construction
	// via SetRegistry so that NewSessionService callers don't need to supply it at build time.
	registry *session.Registry

	// deleteCleanupWG tracks DeleteSession's background tmux/worktree cleanup
	// goroutines so Shutdown can await them instead of letting them outlive the
	// process (or, in tests, outlive the test that spawned them).
	deleteCleanupWG sync.WaitGroup

	// deleteCleanupMu guards deleteCleanupClosed and serializes it against
	// deleteCleanupWG.Add via trackCleanup, so Add never races Shutdown's Wait —
	// see deleteCleanupClosed's doc comment.
	deleteCleanupMu sync.Mutex

	// deleteCleanupClosed is set once Shutdown begins draining deleteCleanupWG.
	// sync.WaitGroup requires that any Add with a positive delta happen before
	// the matching Wait call is invoked (or after a prior Wait returns) —
	// calling Add concurrently with Wait when the counter may be zero is a
	// documented misuse that can panic or let Wait return before the new work
	// finishes. trackCleanup checks this flag under deleteCleanupMu before
	// calling Add; Shutdown sets it under the same mutex before calling Wait.
	// That makes the two mutually exclusive: any Add() that wins the lock race
	// happens-before closed is set true (and thus happens-before Wait()), and
	// any Add() that loses sees closed=true and never calls Add at all — it
	// runs the cleanup untracked instead, which is fine because Shutdown no
	// longer needs to wait for it.
	deleteCleanupClosed bool

	// testTmuxServerSocket, when non-empty, is applied to every Instance this
	// service creates via TmuxServerSocket, isolating the tmux -L socket used
	// by test runs from the shared production default. Set automatically by
	// NewSessionServiceWithSearchEngine under config.IsTestMode() — see
	// docs/bugs/fixed/BUG-075 for why this is a service-wide test hook rather
	// than a per-call-site change across the package's ~90 NewSessionService
	// test call sites.
	testTmuxServerSocket string

	// githubResolver performs the actual GitHub-URL-to-local-clone resolution
	// (network clone/fetch). Defaults to session.ResolveGitHubInputCtxWithHosts;
	// overridable in tests to simulate a slow/instant resolution without
	// depending on real network I/O (see
	// TestCreateSession_should_ReturnWithinSLO_When_GithubURLResolutionIsSlow).
	// Epic 2.1 (async-session-creation): CreateSession no longer calls this
	// synchronously on the RPC path -- it runs in the trackCleanup-dispatched
	// background goroutine, ahead of Start(). See project_plans/
	// async-session-creation/implementation/plan.md Epic 2.1 Story 2.1.1.
	githubResolver func(ctx context.Context, input string, enterpriseHosts []string) (localPath string, ref *session.GitHubRef, err error)

	// creationResolutionTimeout bounds the Background Resolution Pipeline's
	// (Epic 2.2) entire run end to end -- the Background Resolution
	// Context is context.WithTimeout(context.WithoutCancel(rpcCtx),
	// creationResolutionTimeout). Defaults to maxCreationResolutionTimeout;
	// overridable directly in tests (same package, same pattern as
	// githubResolver/testTmuxServerSocket above) to exercise the
	// timeout-exceeded path without waiting the real 10 minutes.
	creationResolutionTimeout time.Duration

	// creationPhaseHook, when non-nil, is invoked synchronously by the
	// Background Resolution Pipeline (Epic 2.2) with each Creation Phase's
	// progress message, immediately before it is published/persisted. Test
	// seam only (nil in production): lets a test observe the exact ordered
	// phase sequence without racing the shared, mutable
	// session.Instance.CreationProgress field against the pipeline's own
	// goroutine advancing to a later phase before a slower test consumer
	// (e.g. an eventBus subscriber) gets scheduled to read it.
	creationPhaseHook func(msg string)

	// testSSHClientPool, when non-nil, is shared by every tmux.SSHRunner and
	// sshremote.RemoteApprovalRelay this service constructs for a remote
	// session, in place of the process-wide tmux.DefaultSSHClientPool().
	// tmux.SSHClientPool keys pooled *ssh.Client connections by remote NAME
	// alone, not by dialed address (see SSHTarget's doc comment) -- so tests
	// across this package that all name their fixture remote "test-remote"
	// but spin up a fresh in-process sshd on a new ephemeral port each time
	// would otherwise share ONE process-wide pool entry and intermittently
	// reuse a stale *ssh.Client left over from a previous test's
	// already-torn-down server, surfacing as a garbled "ssh: unexpected
	// packet in response to channel open" failure. Set automatically by
	// NewSessionServiceWithSearchEngine under config.IsTestMode(), same
	// service-wide test-hook rationale as testTmuxServerSocket above.
	testSSHClientPool *tmux.SSHClientPool

	// remoteKeyStore and remoteKnownHosts back CreateSession's remote-target
	// mode-specific block (ssh-remote-workspaces Phase 4, Epic 4.2): resolving
	// a remote's stored SSH identity and verifying its host key, mirroring
	// RemoteService's own fields of the same purpose. nil until SetRemoteDeps
	// is called (server.go wires the same *sshremote.KnownHostsStore/*sshremote.KeyStore
	// instances RemoteService uses) -- CreateSession returns CodeFailedPrecondition
	// for any request naming a remote when either is nil, rather than nil-pointer
	// panicking.
	remoteKeyStore   *sshremote.KeyStore
	remoteKnownHosts *sshremote.KnownHostsStore

	// permissionRequestHandler backs setupRemoteApprovalHooks's per-remote-
	// session *sshremote.RemoteApprovalRelay construction (ssh-remote-
	// workspaces Phase 5 correction, ADR-003's addendum): every relay is
	// wired to drive its requests through this handler, exactly the way the
	// real local `/api/hooks/permission-request` HTTP endpoint does. nil
	// until SetPermissionRequestHandler is called (server.go wires the same
	// *ApprovalHandler registered at that endpoint) -- setupRemoteApprovalHooks
	// returns an error (non-fatal, logged and swallowed by its caller) rather
	// than nil-pointer panicking when unset.
	permissionRequestHandler sshremote.PermissionRequestHandler
}

// trackCleanup runs fn in a goroutine tracked by deleteCleanupWG so Shutdown
// can await it, unless Shutdown has already begun draining the WaitGroup — see
// deleteCleanupClosed's doc comment for why Add can't be allowed to race Wait.
func (s *SessionService) trackCleanup(fn func()) {
	s.deleteCleanupMu.Lock()
	if s.deleteCleanupClosed {
		s.deleteCleanupMu.Unlock()
		go fn()
		return
	}
	s.deleteCleanupWG.Add(1)
	s.deleteCleanupMu.Unlock()
	go func() {
		defer s.deleteCleanupWG.Done()
		fn()
	}()
}

// deleteSessionCleanupTimeout bounds how long DeleteSession's background
// liveInst.Destroy() cleanup (tmux kill, git diff stats, and worktree
// filesystem cleanup) runs before a warning is logged that it's taking
// longer than expected. Destroy() takes no context and cannot be forcibly
// cancelled (see BUG-072 for threading a real context.Context through the
// cleanup chain so this could cancel the work itself), so this only bounds
// *when a warning fires* — deleteCleanupWG.Done() (and thus
// Shutdown/deleteCleanupWG.Wait()) still waits for Destroy() to actually
// finish regardless of this timeout. That matters for tests: an early Done()
// at this timeout would let Destroy() (and the SessionDriver goroutine it
// stops) outlive Shutdown() and the test that called it, free to interfere
// with whatever state (e.g. a later -count iteration's t.Setenv-scoped
// STAPLER_SQUAD_TEST_DIR) reuses the process next. Matches
// KillTmuxSessionByTitle's 5s cap on the same kill-session subprocess.
//
// defaultDeleteSessionCleanupTimeout is the production default; SessionService
// stores its own copy in deleteSessionCleanupTimeout so tests can shrink it via
// SetDeleteSessionCleanupTimeout instead of waiting out the real 5 seconds to
// exercise the timeout-warning branch below (mirrors
// BacklogService.triageCleanupTimeout / SetTriageCleanupTimeout).
const defaultDeleteSessionCleanupTimeout = 5 * time.Second

// SetDeleteSessionCleanupTimeout overrides the default timeout for DeleteSession's
// cleanup-timeout warning branch. Test-only seam — see
// defaultDeleteSessionCleanupTimeout's doc comment for why this needs to be
// overridable rather than a fixed const.
func (s *SessionService) SetDeleteSessionCleanupTimeout(d time.Duration) {
	s.deleteSessionCleanupTimeout = d
}

// destroyWithTimeout runs destroy() (normally inst.Destroy) and waits up to
// timeout for it to finish. If it doesn't finish in time, this returns a
// timeout error immediately while destroy() continues running in the
// background goroutine it was started in — see deleteSessionCleanupTimeout's
// doc comment for why the bound is on the wait, not the work itself.
//
// destroy is a func() error rather than a *session.Instance so the
// timeout-exceeded branch can be exercised directly in tests with a
// deliberately slow callback, without needing a real Instance whose Destroy()
// can be made to hang on demand.
func destroyWithTimeout(destroy func() error, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- destroy()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("timed out after %s waiting for session cleanup to finish (still running in background)", timeout)
	}
}

// waitForDestroyLoggingSlowCleanup runs destroy() (normally liveInst.Destroy) in a
// goroutine and waits for it to finish. Unlike destroyWithTimeout, it never abandons
// the goroutine: if destroy() takes longer than timeout, onSlow is invoked (to log a
// warning) but this function keeps waiting for the real result instead of returning
// early. That matters because the caller (DeleteSession) only signals
// deleteCleanupWG.Done() after this returns — an early return at the timeout would let
// destroy() outlive Shutdown()/the caller and interfere with whatever reuses that state
// next (e.g. the next -count iteration's t.Setenv-scoped STAPLER_SQUAD_TEST_DIR); see
// defaultDeleteSessionCleanupTimeout's doc comment.
//
// destroy is a func() error (rather than a *session.Instance) for the same testability
// reason as destroyWithTimeout: onSlow lets a test observe the timeout firing without
// needing a real Instance whose Destroy() can be made to hang on demand.
func waitForDestroyLoggingSlowCleanup(destroy func() error, timeout time.Duration, onSlow func()) error {
	done := make(chan error, 1)
	go func() {
		done <- destroy()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		if onSlow != nil {
			onSlow()
		}
		return <-done
	}
}

// workflowMeta holds cached metadata about a workflow used at session-list time.
type workflowMeta struct {
	name              string
	archiveAfterHours int
}

// ScrollbackSequencer is the minimal interface SessionService needs from ScrollbackManager.
// Exported so server/dependencies.go can use warren.Set to validate this wiring at startup.
type ScrollbackSequencer interface {
	CurrentSequence(sessionID string) uint64
}

// Dependency-injection audit (ADR-001, Story 1.3):
//
// All Set* wiring methods on SessionService inject Phase 2 or Phase 3 dependencies —
// none can be moved to constructor injection. Rationale by group:
//
//   Phase 2 (from server/dependencies.go after NewSessionService returns):
//     SetErrorRegistry      — built from storage.GetEntClient() after storage is opened
//     SetAnalyticsClient    — same ent client, pre-existing connection path
//     SetNotificationStore  — built from config dir (lazy path resolution)
//     SetConfigService      — thin wrapper, wired for test-overridability
//     SetMCPServerURL       — env-var / config value, not available inside package
//     SetBacklogLifecycleListener — depends on headless.Pool built after this service
//     SetHistoryLinker      — depends on storage + ent client both resolved
//     SetHeadlessPool       — constructed by BuildDependencies, not NewSessionService
//
//   Phase 3 (via warren.Set in BuildDependencies, after all Phase 2 deps):
//     SetReviewQueuePoller   — circular: poller takes SessionService as param
//     SetStatusManager       — built after reviewQueuePoller is available
//     SetExternalDiscovery   — built after headless pool
//     SetScrollbackManager   — built after storage is opened + log dir resolved
//     SetMemoryCacheReader   — optional, provided by HibernationSweeper
//     SetReactiveQueueManager — built after reviewQueuePoller + statusManager
//     SetLifecycleContext    — application-level ctx not available at pkg init
//     SetFeatureController   — one call per named flag (backlog), after backlog ctrl built
//     SetWorkflowService     — WorkflowService depends on SessionService (circular)
//     SetWorkflowRepository  — same cycle; repo available after workflowSvc constructed
//     SetTokenStoreReader    — built after storage open, wired late by warren
//
// Conclusion: the two-param constructor (storage, eventBus) is the correct boundary.
// All other deps are external to this package or have construction-order constraints.

// NewSessionService creates a new SessionService with the given storage and event bus,
// using a disk-backed search engine (or an in-memory one under config.IsTestMode(), to
// keep the ~78 existing test call sites working without change — see
// NewSessionServiceWithSearchEngine's doc comment for why IsTestMode() is only the
// default, not the seam itself).
// NOTE: Instances are NOT loaded here to prevent double-loading and initialization timing issues.
// Instances will be loaded in server.go after dependencies (statusManager, reviewQueue) are wired.
func NewSessionService(storage session.InstanceStore, eventBus *events.EventBus) *SessionService {
	return NewSessionServiceWithSearchEngine(storage, eventBus, newDefaultSearchEngine())
}

// newDefaultSearchEngine builds the search engine NewSessionService uses when the caller
// doesn't inject one: disk-backed with incremental persistence in production, in-memory
// under config.IsTestMode(). Under go test, skipping disk avoids a shared per-process test
// directory that every NewSessionService call in a test binary would otherwise persist
// into — the index grows across the whole package run and gob-decoding it gets slow enough
// under -race to blow CI's timeout budget, independent of what each test is exercising.
func newDefaultSearchEngine() *search.SearchEngine {
	if config.IsTestMode() {
		return search.NewSearchEngine()
	}
	indexStore, err := search.NewIndexStore()
	if err != nil {
		log.Warn("failed to create index store, using in-memory search", "err", err)
		return search.NewSearchEngine()
	}
	searchEngine := search.NewSearchEngineWithPersistence(indexStore)
	if loadErr := searchEngine.LoadIndex(); loadErr != nil {
		var versionErr *search.IndexVersionMismatchError
		if errors.As(loadErr, &versionErr) {
			// The on-disk index predates a schema change (see CurrentIndexVersion)
			// and was rejected rather than mis-decoded. This self-heals: the next
			// search request finds syncMetadata unset and triggers a full rebuild
			// (SearchEngine.IncrementalSync -> buildIndexLocked) from live session
			// history, so results are only empty until that first search — not
			// until new session activity occurs.
			log.Warn("search index format changed; discarding incompatible on-disk index",
				"old_version", versionErr.Got, "new_version", versionErr.Want, "path", versionErr.Path,
				"impact", "search results empty until next search request triggers an automatic full reindex")
		} else {
			log.Warn("failed to load persisted search index", "err", loadErr)
		}
	} else if meta := searchEngine.GetSyncMetadata(); meta != nil {
		log.Info("loaded persisted search index", "sessions", meta.TotalSessions, "documents", meta.TotalDocuments)
	}
	return searchEngine
}

// claudeSettingsNotificationContent groups publishClaudeSettingsNotification's
// three string fields — kibitzer's primitive-obsession check flags 3
// consecutive same-typed string params as accident-prone (easy to pass in
// the wrong order); naming each field here makes a mis-order a compile-time
// mismatch instead of a silently-wrong notification.
type claudeSettingsNotificationContent struct {
	title   string
	message string
	origin  string
}

// publishClaudeSettingsNotification is the shared publish call for both claude-settings
// notification sites (startup activation and watcher reload) — see NewNotificationEvent.
func publishClaudeSettingsNotification(eventBus *events.EventBus, content claudeSettingsNotificationContent) {
	if eventBus == nil {
		return
	}
	eventBus.Publish(events.NewNotificationEvent(
		"", "System", uuid.New().String(),
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_INFO),
		derivePriority(false, false), // urgent, important — informational config-reload notice
		content.title, content.message,
		map[string]string{"type": "claude_settings_reload", "origin": content.origin},
	))
}

// loadClaudeSettingsRulesAtStartup parses ~/.claude/settings.json (and project-level
// equivalents under cwd) and merges the resulting rules into classifierObj, publishing a
// one-time notification if any were found. Extracted from NewSessionServiceWithSearchEngine
// so it can be unit-tested directly without needing config.IsTestMode() to be false.
func loadClaudeSettingsRulesAtStartup(classifierObj *classifier.RuleBasedClassifier, cwd string, eventBus *events.EventBus) {
	var claudeSettingsRules []classifier.Rule
	for _, result := range LoadClaudeSettingsRulesDetailed(cwd) {
		claudeSettingsRules = append(claudeSettingsRules, result.Rules...)
	}
	if len(claudeSettingsRules) == 0 {
		return
	}
	classifierObj.AddRules(claudeSettingsRules)
	log.Info("[ClaudeSettings] activated claude-settings rules at startup", "rule_count", len(claudeSettingsRules))
	publishClaudeSettingsNotification(eventBus, claudeSettingsNotificationContent{
		title:   "Claude Settings Rules Activated",
		message: fmt.Sprintf("%d rule(s) from your Claude settings are now active as stapler-squad auto-approval rules.", len(claudeSettingsRules)),
		origin:  "startup",
	})
}

// testModeGitHubResolver wraps session.ResolveGitHubInputCtxWithHosts with a
// small artificial floor on how fast it can return, but ONLY under
// config.IsIsolatedInstance() -- a no-op (returns the real resolver
// directly) in every production/dev run. Deliberately IsIsolatedInstance(),
// not the narrower IsTestMode(): the e2e harness's `stapler-squad --test-mode
// --test-dir ...` process is a real compiled binary (STAPLER_SQUAD_TEST_DIR
// set, not a `go test` binary), which IsTestMode() alone does not detect --
// confirmed the hard way, this wrapper was a silent no-op against the e2e
// server on the first attempt. Fixes a genuine e2e flake: the async creation
// pipeline's Creating -> Failed transition for a GitHub-404 (accessibility.spec.ts's
// "Cancel and Retry controls are keyboard-reachable" test, session-creation-async.spec.ts's
// Epic 5.3 toast test) is a REAL network round trip, and on a fast/warm
// connection it can complete in well under a second -- faster than a
// Playwright test can reliably observe the Creating state and drive real
// keyboard interaction against it (repro: 0-2 of 3 runs failed with the
// Cancel button already gone by the time the test got to use it, even after
// several rounds of test-side ordering/timing fixes). Those tests' own doc
// comments already flagged "no backend test-mode hook exists yet to simulate
// an actually-slow GitHub host resolution end-to-end" as a known gap -- this
// is that hook. Safe for `go test` runs too: every existing unit test that
// exercises this path overrides svc.githubResolver directly after
// construction (see session_creation_pipeline_test.go, session_service_test.go,
// session_service_create_test.go), which replaces this default outright, so
// none of them see the added delay. STAPLER_SQUAD_TEST_GITHUB_RESOLVE_MIN_MS
// overrides the floor (0 disables it entirely); unset defaults to 3000ms,
// comfortably inside every existing e2e timeout that waits on this
// transition (all >= 10s, most 30s).
func testModeGitHubResolver() func(ctx context.Context, input string, enterpriseHosts []string) (string, *session.GitHubRef, error) {
	if !config.IsIsolatedInstance() {
		return session.ResolveGitHubInputCtxWithHosts
	}
	minDelay := 3 * time.Second
	if raw := os.Getenv("STAPLER_SQUAD_TEST_GITHUB_RESOLVE_MIN_MS"); raw != "" {
		if ms, err := strconv.Atoi(raw); err == nil && ms >= 0 {
			minDelay = time.Duration(ms) * time.Millisecond
		}
	}
	if minDelay == 0 {
		return session.ResolveGitHubInputCtxWithHosts
	}
	return func(ctx context.Context, input string, enterpriseHosts []string) (string, *session.GitHubRef, error) {
		start := time.Now()
		localPath, ref, err := session.ResolveGitHubInputCtxWithHosts(ctx, input, enterpriseHosts)
		if elapsed := time.Since(start); elapsed < minDelay {
			select {
			case <-time.After(minDelay - elapsed):
			case <-ctx.Done():
			}
		}
		return localPath, ref, err
	}
}

// NewSessionServiceWithSearchEngine is the dependency-injection seam for the search engine:
// pass an explicit *search.SearchEngine (e.g. search.NewSearchEngine() for in-memory)
// instead of relying on NewSessionService's config.IsTestMode() default. Full migration of
// the ~78 existing NewSessionService(storage, eventBus) call sites to explicit injection is
// a separate, larger mechanical refactor — out of scope here; this seam exists so new or
// updated tests can opt in without waiting on that migration.
func NewSessionServiceWithSearchEngine(storage session.InstanceStore, eventBus *events.EventBus, searchEngine *search.SearchEngine) *SessionService {
	reviewQueue := session.NewReviewQueue()

	// concStorage is the concrete backing store used by sub-services that haven't migrated to
	// InstanceStore yet (ReviewQueueService, GitHubService, WorkspaceService). In tests using a
	// fake InstanceStore, concStorage will be nil — those sub-services degrade gracefully to nil storage.
	var concStorage *session.Storage
	if cs, ok := storage.(*session.Storage); ok {
		concStorage = cs
	}

	// Build approval store with disk persistence path
	approvalFilePath := ""
	configDir, configErr := config.GetConfigDir()
	if configErr == nil {
		approvalFilePath = configDir + "/pending_approvals.json"
	} else {
		log.Warn("failed to get config dir for approval persistence", "err", configErr)
	}
	approvalStore := NewApprovalStore(approvalFilePath)
	reviewQueueSvc := NewReviewQueueService(reviewQueue, concStorage, eventBus)
	reviewQueueSvc.SetApprovalStore(approvalStore)

	notificationSvc := NewNotificationService(NewNotificationRateLimiter(10, 20), eventBus)
	approvalSvc := NewApprovalService(approvalStore)
	approvalSvc.SetEventBus(eventBus)
	approvalSvc.SetReviewQueueRemover(reviewQueue)
	utilitySvc := NewUtilityService(approvalStore)

	// Build rules store, analytics store, and classifier for approval rules service.
	rulesStore, rulesErr := NewRulesStore(concStorage)
	if rulesErr != nil {
		log.Warn("failed to load rules store, using empty store", "err", rulesErr)
		rulesStore = &RulesStore{storage: concStorage}
	}
	analyticsStore := NewAnalyticsStore(concStorage)
	analyticsStore.Start(context.Background())
	classifierObj := classifier.NewRuleBasedClassifier()
	// Merge user rules into the classifier. Safe to call AddRules directly (bypassing
	// RulesService.rebuildMu) only because this and the claude-settings load below both run
	// before ClaudeSettingsWatcher.Start() is ever invoked (that happens later, in
	// wireDepsIntoServer) and before the server accepts RPCs — do not add a third AddRules
	// call site here without going through rebuildMu once the watcher may already be running.
	if userRules := rulesStore.ToRules(); len(userRules) > 0 {
		classifierObj.AddRules(userRules)
	}

	// Build tagging rules store/service. The live TaggingEngine itself is wired into
	// Instance construction by a later phase of this project (session-classifier-pipeline
	// Phase 3) — constructing it here just gives TaggingRulesService's CRUD/rebuild
	// plumbing a real engine to target ahead of that wiring.
	taggingRulesStore, taggingRulesErr := NewTaggingRulesStore(concStorage)
	if taggingRulesErr != nil {
		log.Warn("failed to load tagging rules store, using empty store", "err", taggingRulesErr)
		taggingRulesStore = &TaggingRulesStore{storage: concStorage}
	}
	taggingEngine := classifier.NewTaggingEngine()
	if userTaggingRules := taggingRulesStore.ToRules(); len(userTaggingRules) > 0 {
		taggingEngine.AddRules(userTaggingRules)
	}
	taggingRulesSvc := NewTaggingRulesService(taggingRulesStore, taggingEngine, analyticsStore)

	// Determine the server process's own working directory for claude-settings
	// project-level scope. Empty cwd makes LoadClaudeSettingsRulesDetailed skip
	// project-level paths gracefully. Global-only for v1 — this is always the
	// server's own cwd, never a per-session git worktree path (see ADR-001).
	cwd, cwdErr := os.Getwd()
	if cwdErr != nil {
		log.Warn("failed to determine working directory for claude-settings project scope", "err", cwdErr)
		cwd = ""
	}

	// Loads claude-settings rules (previously dead code — LoadClaudeSettingsRules had zero
	// call sites). Gated on IsTestMode() so tests don't read the developer's real
	// ~/.claude/settings.json and become machine-dependent (same guard newDefaultSearchEngine
	// uses two lines above).
	if !config.IsTestMode() {
		loadClaudeSettingsRulesAtStartup(classifierObj, cwd, eventBus)
	}

	// Wire AI rule generation. NewBestAvailableAIClient selects the highest-priority
	// available backend: Anthropic HTTP API (if ANTHROPIC_API_KEY is set) → claude CLI
	// → gemini CLI → opencode CLI. Returns nil when no backend is available.
	var promptBuilder RulePromptBuilder
	var aiClientImpl AIClient
	{
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if c, backend := NewBestAvailableAIClient(apiKey, knownCLIAgents); c != nil {
			promptBuilder = &DefaultRulePromptBuilder{}
			aiClientImpl = c
			log.Info("[SessionService] AI rule generation enabled", "backend", backend)
		} else {
			log.Info("[SessionService] AI rule generation unavailable: set ANTHROPIC_API_KEY or install claude/gemini/opencode CLI")
		}
	}
	rulesSvc := NewRulesService(rulesStore, nil, analyticsStore, classifierObj, promptBuilder, WrapRulesAIClient(aiClientImpl))

	// Wire the claude-settings file watcher: fsnotify-driven or manually-triggered
	// (ReloadClaudeSettingsRules RPC) reloads both flow through this one callback, which
	// hot-swaps the classifier's claude-settings rules and emits a visible reload event.
	claudeSettingsWatcher := NewClaudeSettingsWatcher(cwd, func(rules []classifier.Rule, origin string, notify bool) {
		rulesSvc.rebuildClaudeSettingsRules(rules)
		log.Info("[ClaudeSettingsWatcher] reloaded claude-settings rules", "rule_count", len(rules), "origin", origin)
		// notify is false for Start()'s initial priming reload — the operator already saw a
		// startup activation notification if any rules exist, and nothing meaningful
		// happened for this callback to report otherwise. A zero-rule result is also
		// never notification-worthy, on any call: "0 rules reloaded" is a no-op from the
		// operator's perspective whether it's the priming reload or a later real one.
		if !notify || len(rules) == 0 {
			return
		}
		publishClaudeSettingsNotification(eventBus, claudeSettingsNotificationContent{
			title:   "Claude Settings Reloaded",
			message: fmt.Sprintf("%d claude-settings rule(s) reloaded (%s).", len(rules), origin),
			origin:  origin,
		})
	})
	rulesSvc.SetClaudeSettingsWatcher(claudeSettingsWatcher)
	rulesSvc.SetApprovalService(approvalSvc)
	// Started after SetApprovalService: the first load reconciles pending approvals, which reads
	// rs.approvalSvc unsynchronised.
	// Hot-reload shared_rules.yaml (gated on IsTestMode like the claude-settings load above, so
	// tests never read the developer's real home directory).
	if home, homeErr := os.UserHomeDir(); homeErr == nil && !config.IsTestMode() {
		rulesSvc.StartConfigFileRulesReload(context.Background(), classifier.ConfigFileRulesPath(home), configFileRulesPollInterval)
	}

	// Initialize capacity monitor.
	var capCfg config.CapacityConfig
	if dir, err := config.GetConfigDir(); err == nil {
		if c, err := config.LoadConfigFromPath(filepath.Join(dir, "config.json")); err == nil {
			capCfg = c.Capacity
		}
	}
	capCfg = capCfg.CapacityConfigOrDefault()

	directCfg := &config.Config{}
	if dir, err := config.GetConfigDir(); err == nil {
		if c, err := config.LoadConfigFromPath(filepath.Join(dir, "config.json")); err == nil {
			directCfg = c
		}
	}
	credChain := NewDefaultChain(directCfg)

	var capRoleResolver SessionRoleResolver
	var capTerminator SessionTerminator
	if concStorage != nil {
		capRoleResolver = func(ctx context.Context, sessionUUID string) string {
			is, err := concStorage.GetItemSessionBySessionUUID(ctx, sessionUUID)
			if err != nil {
				if !errors.Is(err, session.ErrNotFound) {
					log.Warn("capacity: role lookup failed; applying default (non-pipeline) limits", "uuid", sessionUUID, "err", err)
				}
				return ""
			}
			return is.Role
		}
		capTerminator = func(ctx context.Context, inst *session.Instance, reason string) error {
			return stopGuardrailSession(ctx, concStorage, inst, reason)
		}
	}
	capacityMonitor := NewCapacityMonitor(CapacityMonitorParams{
		Config:       capCfg,
		EventBus:     eventBus,
		RoleResolver: capRoleResolver,
		Terminator:   capTerminator,
	})
	capacityMonitor.RegisterClient("anthropic", NewAnthropicLimitsClient(credChain, ""))
	capacityMonitor.RegisterClient("google", NewGeminiLimitsClient(credChain, ""))

	if anthropicClient, ok := aiClientImpl.(*AnthropicAIClient); ok {
		anthropicClient.OnResponseHeaders = func(h http.Header) {
			capacityMonitor.UpdateFromResponseHeaders("anthropic", h)
		}
	}

	workspaceSvc := NewWorkspaceService(concStorage, eventBus)

	svc := &SessionService{
		storage:                     storage,
		concStorage:                 concStorage,
		eventBus:                    eventBus,
		reviewQueueSvc:              reviewQueueSvc,
		searchSvc:                   NewSearchService(searchEngine, search.NewSnippetGenerator(), 5*time.Minute),
		githubSvc:                   NewGitHubService(concStorage),
		workspaceSvc:                workspaceSvc,
		configSvc:                   NewConfigService(),
		notificationSvc:             notificationSvc,
		approvalSvc:                 approvalSvc,
		utilitySvc:                  utilitySvc,
		rulesSvc:                    rulesSvc,
		taggingRulesSvc:             taggingRulesSvc,
		taggingEngine:               taggingEngine,
		approvalStore:               approvalStore,
		databaseSvc:                 NewDatabaseService(),
		fileSvc:                     NewFileService(workspaceSvc),
		pathCompletionSvc:           NewPathCompletionService(),
		slashCommandSvc:             NewSlashCommandService(),
		defaultsSvc:                 NewDefaultsService(),
		slackConfigSvc:              NewSlackConfigService(NewSlackNotifier()),
		julesConfigSvc:              NewJulesConfigService(nil, nil, nil),
		callbackConfigSvc:           NewCallbackConfigService(),
		streamHubRolloutSvc:         NewStreamHubRolloutService(nil),
		launcherPresetsSvc:          NewLauncherPresetsService(),
		projectSvc:                  NewProjectService(concStorage),
		checkpointSvc:               NewCheckpointService(storage, eventBus),
		featureFlagSvc:              NewFeatureFlagService(),
		terminalSvc:                 NewTerminalService(),
		promptStore:                 newPromptStore(),
		capacityMonitor:             capacityMonitor,
		deleteSessionCleanupTimeout: defaultDeleteSessionCleanupTimeout,
		credChain:                   credChain,
		githubResolver:              testModeGitHubResolver(),
		creationResolutionTimeout:   maxCreationResolutionTimeout,
	}
	capacityMonitor.sessionSwitcher = svc
	capacityMonitor.poller = svc

	if config.IsTestMode() {
		svc.testTmuxServerSocket = fmt.Sprintf("test_server_services_%d_%d", os.Getpid(), atomic.AddUint64(&testTmuxServerSocketCounter, 1))
		svc.testSSHClientPool = tmux.NewSSHClientPool()
	}

	// Wire the fast-path live-instance lookup so WorkspaceService read-only RPCs
	// (GetVCSStatus, GetWorkspaceInfo, ListWorkspaceTargets) bypass LoadInstances.
	workspaceSvc.SetLiveFinder(svc)
	// Wire the live-instance lookup into ApprovalService's block-on-red-CI guard (AC5).
	// GitHubCheckConclusion is not persisted (see plan.md's Implementation Deviations),
	// so this must be the live registry, not storage.
	approvalSvc.SetLiveInstanceFinder(svc)
	// Wire the live-instance provider so ListClaudeHistory can populate
	// session_status on history entries without a separate storage call.
	svc.searchSvc.SetInstanceProvider(svc.allInstances)
	// Wire CheckpointService's instance-load fallback to this service's own
	// loadInstancesWithWiring so ClearConversationState gets properly-wired instances.
	svc.checkpointSvc.SetLoadInstancesFn(svc.loadInstancesWithWiring)

	// Construct prCreationSvc with the shared storage/eventBus and a bound
	// findInstance method value (narrow dependency, not the whole SessionService —
	// see interface-pollution-checklist.md). headlessPool and
	// backlogLifecycleListener are wired onto SessionService after construction
	// (see the Set* wiring doc comment above NewSessionService), so they are nil
	// here at construction time.
	svc.prCreationSvc = NewPRCreationService(storage, eventBus, svc.headlessPool, svc.backlogLifecycleListener, svc.findInstance)
	svc.streamHubRolloutSvc = NewStreamHubRolloutService(svc.findInstance)

	// Wire the autonomous orchestration service with a storage getter closure.
	autonomousSvc := NewAutonomousOrchestrationService(nil, eventBus)
	autonomousSvc.SetStorageGetter(func() *session.Storage {
		if cs, ok := storage.(*session.Storage); ok {
			return cs
		}
		return nil
	})
	svc.autonomousSvc = autonomousSvc

	return svc
}

// newPromptStore creates a PromptStore backed by ~/.stapler-squad/prompts.json.
func newPromptStore() *prompts.PromptStore {
	dir, err := config.GetConfigDir()
	if err != nil {
		log.Warn("[PromptStore] failed to get config dir", "err", err)
		return prompts.NewPromptStore(os.TempDir() + "/stapler-squad-prompts.json")
	}
	return prompts.NewPromptStore(dir + "/prompts.json")
}

// loadInstancesWithWiring loads instances from storage and wires up dependencies.
// This ensures instances have reviewQueue and statusManager set properly.
func (s *SessionService) loadInstancesWithWiring() ([]*session.Instance, error) {
	instances, err := s.storage.LoadInstances()
	if err != nil {
		return nil, fmt.Errorf("loadInstancesWithWiring: %w", err)
	}

	// Wire up dependencies on loaded instances
	for _, inst := range instances {
		inst.SetReviewQueue(s.reviewQueueSvc.GetQueue())
		inst.SetNotifier(&EventBusNotifier{Bus: s.eventBus})
		if s.statusManager != nil {
			inst.SetStatusManager(s.statusManager)
		}
		s.wireCallbacks(inst)
	}

	return instances, nil
}

// NewSessionServiceFromConfig creates a SessionService using EntRepository as storage backend.
// On first startup, if the legacy state.json exists and Ent DB is empty, sessions are
// auto-migrated from JSON to Ent.
func NewSessionServiceFromConfig() (*SessionService, error) {
	// Write workspace metadata on startup so the workspace switcher can discover this workspace.
	config.EnsureWorkspaceMeta()

	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to determine config directory: %w", err)
	}
	dbPath := configDir + "/sessions.db"

	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	if err != nil {
		return nil, fmt.Errorf("failed to initialize EntRepository: %w", err)
	}

	// Auto-migrate from state.json if Ent DB is empty and legacy data exists
	if migrateErr := maybeAutoMigrateToEnt(repo); migrateErr != nil {
		log.Warn("auto-migration to Ent skipped or failed", "err", migrateErr)
	}

	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage with EntRepository: %w", err)
	}

	return newGatedSessionService(storage), nil
}

// newGatedSessionService builds the production SessionService: the delivery
// gate is created with the bus and installed as its publish filter before any
// producer can publish, so there is no un-gated window and no later bind step.
func newGatedSessionService(storage session.InstanceStore) *SessionService {
	gate := deliverygate.NewGate(deliverygate.WithInstanceLister(storage))
	eventBus := events.NewEventBus(100)
	eventBus.SetPublishFilter(gate.PublishFilter())
	svc := NewSessionService(storage, eventBus)
	svc.deliveryGate = gate
	svc.notificationSvc.SetDeliveryGate(gate)
	svc.autonomousSvc.SetLegacyHiddenCounter(gate.CountLegacySuppressedType)
	return svc
}

// DeliveryGate returns the delivery gate, or nil when the service was built
// without one (unit tests that call NewSessionService directly).
func (s *SessionService) DeliveryGate() *deliverygate.Gate {
	return s.deliveryGate
}

// NewSessionServiceWithEntClient creates a SessionService from a pre-existing *ent.Client.
// Use this when the caller already opened a database (e.g. in tests or when sharing a
// connection) and wants to bypass the config-based path discovery in NewSessionServiceFromConfig.
func NewSessionServiceWithEntClient(entClient *ent.Client) (*SessionService, error) {
	config.EnsureWorkspaceMeta()
	repo := session.NewEntRepositoryFromClient(entClient)
	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage with provided ent client: %w", err)
	}
	return newGatedSessionService(storage), nil
}

// GetStorage returns the concrete *session.Storage for components that haven't migrated to InstanceStore yet.
// Returns nil when SessionService was constructed with a fake InstanceStore (e.g., in unit tests).
// Prefer using the session.InstanceStore interface via GetInstanceStore() for new code.
func (s *SessionService) GetStorage() *session.Storage {
	if cs, ok := s.storage.(*session.Storage); ok {
		return cs
	}
	return nil
}

// GetInstanceStore returns the InstanceStore interface, suitable for both production and test code.
func (s *SessionService) GetInstanceStore() session.InstanceStore {
	return s.storage
}

// sshClientPool returns the SSHClientPool every remote-session SSHRunner/
// RemoteApprovalRelay this service constructs must share -- s.testSSHClientPool
// under test (see its doc comment), otherwise the process-wide production
// default. Never construct a tmux.SSHRunner or sshremote.RemoteApprovalRelay
// for a remote session by calling tmux.DefaultSSHClientPool() directly; use
// this instead so tests stay isolated from each other and from production.
func (s *SessionService) sshClientPool() *tmux.SSHClientPool {
	if s.testSSHClientPool != nil {
		return s.testSSHClientPool
	}
	return tmux.DefaultSSHClientPool()
}

// maybeAutoMigrateToEnt checks whether state.json exists in the config directory and the
// Ent repository is empty. If both conditions hold, it migrates all sessions from state.json
// to Ent automatically. This is a one-shot migration: once data is in Ent the check is a no-op.
func maybeAutoMigrateToEnt(repo *session.EntRepository) error {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("could not determine config dir: %w", err)
	}

	stateJSONPath := configDir + "/state.json"
	if _, statErr := os.Stat(stateJSONPath); os.IsNotExist(statErr) {
		return nil // nothing to migrate
	}

	// Check if Ent DB is already populated — skip migration if so
	ctx := context.Background()
	existing, listErr := repo.List(ctx)
	if listErr != nil {
		return fmt.Errorf("failed to list Ent sessions: %w", listErr)
	}
	if len(existing) > 0 {
		return nil // already has data, skip
	}

	// state.json stores instances inside a wrapper: {"instances": [...], ...}
	type stateFileFormat struct {
		Instances []session.InstanceData `json:"instances"`
	}
	// #nosec G304 -- stateJSONPath is configDir+"/state.json", built from the internal
	// config dir and a literal filename; never network/RPC input.
	rawData, readErr := os.ReadFile(stateJSONPath)
	if readErr != nil {
		return fmt.Errorf("failed to read state.json: %w", readErr)
	}

	var stateFile stateFileFormat
	if unmarshalErr := json.Unmarshal(rawData, &stateFile); unmarshalErr != nil {
		return fmt.Errorf("failed to parse state.json: %w", unmarshalErr)
	}

	if len(stateFile.Instances) == 0 {
		return nil // nothing to migrate
	}

	log.Info("auto-migrating sessions from state.json to Ent repository", "count", len(stateFile.Instances))

	for _, inst := range stateFile.Instances {
		if createErr := repo.Create(ctx, inst); createErr != nil {
			log.Warn("auto-migrate: failed to create session", "session", inst.Title, "err", createErr)
		}
	}

	log.Info("auto-migration to Ent complete")
	return nil
}
