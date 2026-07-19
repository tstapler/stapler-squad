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

	// headless triage pool and concurrency controls.
	headlessPool   headless.PoolClient
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	triageSem      chan struct{}

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
		return pt.EstimateCost(r)
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
		PrUrl:      item.PrURL,
		PrNumber:   int32(item.PrNumber),
		CreatedAt:  timestamppb.New(item.CreatedAt),
		UpdatedAt:  timestamppb.New(item.UpdatedAt),
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

// backlogItemToProto maps a BacklogItemData to the proto BacklogItem message.
func backlogItemToProto(item *session.BacklogItemData, costFor func(tmuxUUID string) float64) *sessionv1.BacklogItem {
	p := &sessionv1.BacklogItem{
		Id:                item.ID,
		Title:             item.Title,
		Description:       item.Description,
		Priority:          int32(item.Priority),
		Status:            item.Status,
		RepoPath:          item.RepoPath,
		SkipReviewGate:    item.SkipReviewGate,
		SkipPlanning:      item.SkipPlanning,
		AutoSpawnSession:  item.AutoSpawnSession,
		AutoCreatePr:      item.AutoCreatePR,
		PipelineMode:      &item.PipelineMode,
		PlanApproved:      item.PlanApproved,
		PlanArtifactsPath: item.PlanArtifactsPath,
		Notes:             item.Notes,
		ExternalId:        item.ExternalID,
		SourceId:          item.SourceID,
		PrUrl:             item.PrURL,
		PrNumber:          int32(item.PrNumber),
		CreatedAt:         timestamppb.New(item.CreatedAt),
		UpdatedAt:         timestamppb.New(item.UpdatedAt),
	}
	if item.PlanApprovedAt != nil {
		p.PlanApprovedAt = timestamppb.New(*item.PlanApprovedAt)
	}
	if item.ArchivedAt != nil {
		p.ArchivedAt = timestamppb.New(*item.ArchivedAt)
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

	return p
}

// itemSourceToProto maps an ItemSourceData to the proto ItemSource message.
func itemSourceToProto(src *session.ItemSourceData) *sessionv1.ItemSource {
	p := &sessionv1.ItemSource{
		Id:              src.ID,
		PluginId:        src.PluginID,
		DisplayName:     src.DisplayName,
		Enabled:         src.Enabled,
		TokenConfigured: src.TokenConfigured,
		CreatedAt:       timestamppb.New(src.CreatedAt),
		UpdatedAt:       timestamppb.New(src.UpdatedAt),
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
		}
	}
}
