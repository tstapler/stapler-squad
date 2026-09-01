package services

// jules_dispatch_service.go — Epic 2.2: the one guarded, idempotent path from
// "user clicks Dispatch to Jules" to "a billed Jules session exists and a
// local ItemSession records it". See
// project_plans/google-jules-integration/implementation/plan.md's Epic 2.2
// for the full story/task breakdown this file implements.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	connect "connectrpc.com/connect"
	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/config"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/jules"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// julesSessionUUIDPrefix is prepended to a jules.JulesSessionName to form
// ItemSession.session_uuid once CreateSession confirms a real session.
// julesPendingUUIDPrefix is the reservation UUID written *before* the
// CreateSession POST (ADR-004's idempotency reservation), replaced with the
// real name after.
//
// Declared independently here and in session/jules_session_poller.go
// (Task 2.3.1a) — session/ cannot import server/services (import-direction
// constraint), mirroring headlessTriageUUIDPrefix's existing duplication
// (server/services/backlog_service_triage.go:423 /
// session/backlog_lifecycle_triage.go:43-50). Keep both copies byte-identical
// if either ever changes.
const (
	julesSessionUUIDPrefix = "jules-"
	julesPendingUUIDPrefix = "jules-pending-"
)

// ErrJulesDispatchInFlight indicates this item already has an open (or
// racing) Jules dispatch — either a persisted jules_work ItemSession with no
// end time (Task 2.2.1b step 2) or a concurrent call already holding the
// item's in-process mutex (Task 2.2.1a). Both mean the same thing to the
// user ("a dispatch is already in flight for this item"); which guard caught
// it is distinguishable in code, not by a second sentinel.
var ErrJulesDispatchInFlight = errors.New("jules: dispatch already in flight for this item")

// errJulesConcurrencyCapReached / errJulesDailyCapReached let DispatchToJules
// classify a checkSpendGuards failure into the right "jules dispatch
// rejected" reason (concurrency_cap vs daily_cap) without checkSpendGuards
// itself needing an itemID parameter to log with (Task 2.2.2b).
var (
	errJulesConcurrencyCapReached = errors.New("jules: concurrency cap reached")
	errJulesDailyCapReached       = errors.New("jules: daily cap reached")
)

// julesSessionCreator is the narrow, locally-declared dependency
// JulesDispatchService uses instead of holding a concrete *jules.Client —
// satisfied structurally by it (Dependency Inversion: the consumer owns the
// interface it depends on). A test fake implements this directly, which is
// what lets Task 2.2.1c inject one without a real HTTP round trip.
type julesSessionCreator interface {
	CreateSession(ctx context.Context, req jules.CreateSessionRequest) (*jules.JulesSession, error)
}

// julesTransitionGuard is the narrow, locally-declared dependency
// JulesDispatchService uses for the guarded status transition and blocker
// check, instead of holding a concrete *BacklogService or duplicating its
// storage/engine fields. *BacklogService (backlog_service_triage.go:720,738)
// satisfies this structurally — both methods live in this same package, so
// the unexported names resolve. Mirrors AutonomousStuckRespawner
// (autonomous_orchestration_service.go:30), this codebase's existing
// convention for one server/services type reaching BacklogService behavior
// via a consumer-owned interface rather than a concrete sibling pointer.
type julesTransitionGuard interface {
	transitionWithGuard(ctx context.Context, item *session.BacklogItemData, to session.BacklogStatus, precondition *session.BacklogItemPrecondition, triggeredBy string, hasUnresolvedBlockers bool) (*session.BacklogItemData, error)
	hasUnresolvedBlockers(ctx context.Context, itemID string) (bool, error)
}

// JulesDispatchRequest is the dispatch input: ItemID, Branch, Prompt.
// Validated once at construction (NewJulesDispatchRequest) — parse, don't
// validate. Deliberately carries no EgressAcknowledged field (pre-mortem
// P1 #3): consent is a durable, server-side fact
// (config.JulesConfig.EgressAcknowledgedRepos) that only ConfirmEgressConsent
// can write, never a caller-supplied boolean DispatchToJules could trust.
type JulesDispatchRequest struct {
	ItemID string
	Branch jules.GitHubBranchRef
	Prompt string
}

// NewJulesDispatchRequest validates branch and prompt at construction time
// (parse, don't validate) — a JulesDispatchRequest that exists is already
// known-valid. No egressAck parameter; see the type's doc comment.
func NewJulesDispatchRequest(itemID, branch, prompt string) (JulesDispatchRequest, error) {
	if itemID == "" {
		return JulesDispatchRequest{}, fmt.Errorf("jules: item id must not be empty")
	}
	branchRef, err := jules.ParseGitHubBranchRef(branch)
	if err != nil {
		return JulesDispatchRequest{}, err
	}
	if prompt == "" {
		return JulesDispatchRequest{}, fmt.Errorf("jules: prompt must not be empty")
	}
	return JulesDispatchRequest{ItemID: itemID, Branch: branchRef, Prompt: prompt}, nil
}

// JulesDispatcher is the consumer-defined interface RPC handlers (Epic 2.4)
// depend on. Deliberately not a widening of SessionCreator (ADR-001): a
// Jules dispatch has no tmux ProcessManager and no *session.Instance.
type JulesDispatcher interface {
	DispatchToJules(ctx context.Context, item *session.BacklogItemData, req JulesDispatchRequest) (jules.JulesSessionName, error)
}

// JulesDispatchService is the JulesDispatcher implementation: guards
// (in-flight mutex -> persisted-open-session check -> egress -> spend caps
// -> blocker check) -> reserve ItemSession -> CreateSession -> confirm ->
// transitionWithGuard to in_progress.
//
// counters (JulesUsageCounter, Task 4.1.1a) is not yet wired — that type
// belongs to Epic 4.1 (Observability), which has not landed as of this
// service's construction. The Observability Plan's required log lines
// (jules dispatch requested/rejected, jules session created) are emitted
// unconditionally below; Epic 4.1 will add the counter increments alongside
// them once JulesUsageCounter exists.
type JulesDispatchService struct {
	storage         *session.Storage
	transitionGuard julesTransitionGuard
	client          julesSessionCreator
	sources         *jules.JulesSourceRegistry
	cfg             *config.Config

	// itemLocks holds one *sync.Mutex per item ID, lazily created on first
	// use — the in-process double-click guard (Task 2.2.1a). A per-item
	// TryLock, not singleflight.Group: singleflight hands the *first*
	// caller's result to every waiter, which would give a racing second
	// caller a fabricated success instead of ErrJulesDispatchInFlight.
	itemLocks sync.Map
}

// NewJulesDispatchService constructs a JulesDispatchService. transitionGuard
// is passed as the already-constructed *BacklogService at the call site
// (Task 2.4.4a) — no late-binding setter is needed, unlike
// AutonomousStuckRespawner.
func NewJulesDispatchService(storage *session.Storage, transitionGuard julesTransitionGuard, client julesSessionCreator, sources *jules.JulesSourceRegistry, cfg *config.Config) *JulesDispatchService {
	return &JulesDispatchService{
		storage:         storage,
		transitionGuard: transitionGuard,
		client:          client,
		sources:         sources,
		cfg:             cfg,
	}
}

// itemMutex returns (creating if necessary) the per-item mutex backing the
// double-click guard.
func (s *JulesDispatchService) itemMutex(itemID string) *sync.Mutex {
	lockVal, _ := s.itemLocks.LoadOrStore(itemID, &sync.Mutex{})
	return lockVal.(*sync.Mutex)
}

// logDispatchRejected emits the Observability Plan's "jules dispatch
// rejected" line. reason is one of: not_configured, no_egress_ack,
// source_not_registered, in_flight, concurrency_cap, daily_cap, no_branch
// (emitted by NewJulesDispatchRequest's caller, not this service),
// unresolved_blockers. Logged at Info, not Error — these are expected
// user-facing outcomes, not failures.
func (s *JulesDispatchService) logDispatchRejected(itemID, reason string) {
	log.InfoLog().Printf("jules dispatch rejected item_id=%s reason=%s", itemID, reason)
}

// DispatchToJules implements the reserve -> create -> confirm sequence
// (Task 2.2.1b). Each guard returns immediately on failure; none of it
// reaches client.CreateSession (the only billed call) unless every guard
// before it passed.
func (s *JulesDispatchService) DispatchToJules(ctx context.Context, item *session.BacklogItemData, req JulesDispatchRequest) (jules.JulesSessionName, error) {
	// 1. In-process mutex guard (double-click). Released via defer once the
	// whole sequence below finishes — see the type's itemLocks doc comment.
	mu := s.itemMutex(item.ID)
	if !mu.TryLock() {
		s.logDispatchRejected(item.ID, "in_flight")
		return "", fmt.Errorf("jules: dispatch already in flight for item %s: %w", item.ID, ErrJulesDispatchInFlight)
	}
	defer mu.Unlock()

	// 2. Persisted-state duplicate check (Gap 3) — the guard that actually
	// enforces "at most one open Jules session per item" against durable
	// state, since the mutex above is released the instant each call
	// returns. Fetched once here and reused by checkSpendGuards' concurrency
	// count below, rather than querying ListOpenJulesItemSessions twice.
	openSessions, err := s.storage.ListOpenJulesItemSessions(ctx)
	if err != nil {
		return "", fmt.Errorf("jules: listing open sessions: %w", err)
	}
	for _, open := range openSessions {
		if open.ItemID == item.ID {
			s.logDispatchRejected(item.ID, "in_flight")
			return "", fmt.Errorf("jules: item %s already has an open dispatch: %w", item.ID, ErrJulesDispatchInFlight)
		}
	}

	// 3. Egress consent — read-only membership check, never a write path.
	if err := s.checkEgressConsent(item); err != nil {
		reason := "no_egress_ack"
		if errors.Is(err, jules.ErrJulesNotConfigured) {
			reason = "not_configured"
		}
		s.logDispatchRejected(item.ID, reason)
		return "", err
	}

	// 4. Spend guards — concurrency ceiling + daily cap, enforced before any
	// billed call (Risk Control's blast-radius guard, ADR-004).
	if err := s.checkSpendGuards(ctx, openSessions); err != nil {
		reason := "concurrency_cap"
		if errors.Is(err, errJulesDailyCapReached) {
			reason = "daily_cap"
		}
		s.logDispatchRejected(item.ID, reason)
		return "", err
	}

	// 5. Blocker gate (Gap 2) — the exact same storage.UnresolvedBlockerItemIDs-
	// backed check every other single-item transition in this codebase uses,
	// run before any reservation is written or any billed call is made.
	hasBlockers, err := s.transitionGuard.hasUnresolvedBlockers(ctx, item.ID)
	if err != nil {
		return "", fmt.Errorf("jules: checking blockers for item %s: %w", item.ID, err)
	}
	if hasBlockers {
		s.logDispatchRejected(item.ID, "unresolved_blockers")
		return "", fmt.Errorf("jules: item %s has unresolved blockers: %w", item.ID, session.ErrUnresolvedBlockers)
	}

	// 6. Resolve owner/repo -> JulesSourceName, reserve, create, confirm.
	ref, err := resolveJulesOwnerRepo(item.RepoPath)
	if err != nil {
		return "", fmt.Errorf("jules: %w", err)
	}
	sourceName, err := s.sources.Resolve(ctx, ref.Owner(), ref.Repo())
	if err != nil {
		s.logDispatchRejected(item.ID, "source_not_registered")
		return "", err
	}

	log.InfoLog().Printf("jules dispatch requested item_id=%s repo=%s branch=%s source_name=%s", item.ID, ref.String(), req.Branch, sourceName)

	reservationUUID := julesPendingUUIDPrefix + uuid.New().String()
	reservation, err := s.storage.CreateItemSession(ctx, session.ItemSessionData{
		ItemID:      item.ID,
		SessionUUID: reservationUUID,
		SessionRole: session.SessionRoleJulesWork,
	})
	if err != nil {
		return "", fmt.Errorf("jules: creating item session reservation: %w", err)
	}

	julesSession, createErr := s.client.CreateSession(ctx, jules.CreateSessionRequest{
		Prompt:         req.Prompt,
		Source:         sourceName,
		StartingBranch: req.Branch,
	})
	if createErr != nil {
		s.endFailedReservation(ctx, reservation.ID, item.ID, createErr)
		return "", fmt.Errorf("jules: creating session: %w", createErr)
	}

	sessionUUID := julesSessionUUIDPrefix + string(julesSession.Name)
	if err := s.storage.UpdateItemSessionSessionUUID(ctx, reservation.ID, sessionUUID); err != nil {
		return "", fmt.Errorf("jules: recording session name: %w", err)
	}

	log.InfoLog().Printf("jules session created item_id=%s jules_session=%s item_session_id=%s", item.ID, julesSession.Name, reservation.ID)

	precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusReady)}
	// hasBlockers is guaranteed false here — step 5 already returned early
	// otherwise — so this call site can never drift back out of sync with
	// the real check above if the guard sequence is ever reordered.
	if _, err := s.transitionGuard.transitionWithGuard(ctx, item, session.BacklogStatusInProgress, precondition, session.TriggeredByUser, hasBlockers); err != nil {
		return "", fmt.Errorf("jules: transitioning item %s to in_progress: %w", item.ID, err)
	}

	return julesSession.Name, nil
}

// endFailedReservation ends a reservation row whose CreateSession call
// failed, so no orphan claim is left behind, and appends a visible progress
// note so the failure shows up in the UI rather than silently (per
// feedback_document_ai_decisions_in_edge_cases). Best-effort: a failure to
// record the failure is logged, not propagated — the caller already has a
// createErr to return.
func (s *JulesDispatchService) endFailedReservation(ctx context.Context, itemSessionID, itemID string, createErr error) {
	if endErr := s.storage.UpdateItemSessionEndedWithReason(ctx, itemSessionID, time.Now(), "dispatch_failed"); endErr != nil {
		log.WarningLog().Printf("[JulesDispatchService] failed to end orphaned reservation item_session=%s: %v", itemSessionID, endErr)
	}
	note := fmt.Sprintf("Jules dispatch failed: %v", createErr)
	if noteErr := s.storage.AppendProgressNote(ctx, itemID, -1, note, "blocked"); noteErr != nil {
		log.WarningLog().Printf("[JulesDispatchService] failed to append dispatch-failure note item=%s: %v", itemID, noteErr)
	}
}

// checkEgressConsent is a pure, read-only membership check — it has no code
// path that can write to cfg.Jules.EgressAcknowledgedRepos (Story 2.2.3's
// pre-mortem P1 #3 redesign). Deliberately takes no request-shaped argument,
// so it structurally cannot read a caller-supplied consent flag.
//
// Checks, in order: Enabled, then repo membership. "Key resolvable" (Risk
// Control's third AND condition) is not re-probed here — JulesDispatchService
// is deliberately not given a credential-source dependency (see
// julesSessionCreator's narrow, CreateSession-only shape); an unresolvable
// key surfaces from the real *jules.Client at the CreateSession call itself
// (classifyJulesResponse maps a 401/403 to jules.ErrJulesNotConfigured).
func (s *JulesDispatchService) checkEgressConsent(item *session.BacklogItemData) error {
	if s.cfg == nil || !s.cfg.Jules.Enabled {
		return fmt.Errorf("jules dispatch is not enabled: %w", jules.ErrJulesNotConfigured)
	}
	for _, acked := range s.cfg.Jules.EgressAcknowledgedRepos {
		if acked == item.RepoPath {
			return nil
		}
	}
	display := item.RepoPath
	if ref, err := resolveJulesOwnerRepo(item.RepoPath); err == nil {
		display = ref.String()
	}
	return fmt.Errorf(
		"the contents of %s have not been acknowledged for Jules dispatch — dispatching would send its contents to Google's cloud VM to run this session; confirm this in the Jules dispatch dialog before continuing",
		display,
	)
}

// checkSpendGuards enforces the concurrency ceiling and daily cap before any
// billed call (Story 2.2.2). openSessions is the slice DispatchToJules
// already fetched for the persisted-duplicate check (Task 2.2.1b step 2),
// reused here for the concurrency count instead of a second
// ListOpenJulesItemSessions call.
func (s *JulesDispatchService) checkSpendGuards(ctx context.Context, openSessions []session.ItemSessionBacklogEntry) error {
	limit := s.cfg.MaxConcurrentJulesSessionsOrDefault()
	if len(openSessions) >= limit {
		return connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("%w: %d Jules sessions are already running (limit %d)", errJulesConcurrencyCapReached, len(openSessions), limit))
	}

	dailyLimit := s.cfg.MaxJulesSessionsPerDayOrDefault()
	since := time.Now().Add(-24 * time.Hour)
	count, err := s.storage.CountJulesItemSessionsSince(ctx, since)
	if err != nil {
		return fmt.Errorf("jules: counting sessions dispatched in the last 24h: %w", err)
	}
	if count >= dailyLimit {
		return connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("%w: %d Jules sessions have already been dispatched in the last 24 hours (limit %d)", errJulesDailyCapReached, count, dailyLimit))
	}
	return nil
}

// resolveJulesOwnerRepo resolves repoPath's GitHub owner/repo from its git
// remote — the same github.GetOwnerRepoFromRemote helper
// defaultOrphanedPRFinder/defaultPRByNumberFinder already use for this exact
// purpose (session/backlog_lifecycle_pr.go).
func resolveJulesOwnerRepo(repoPath string) (githubpkg.RepoRef, error) {
	ref, err := githubpkg.GetOwnerRepoFromRemote(repoPath)
	if err != nil {
		return githubpkg.RepoRef{}, fmt.Errorf("resolving GitHub owner/repo for %s: %w", repoPath, err)
	}
	if !ref.IsValid() {
		return githubpkg.RepoRef{}, fmt.Errorf("could not resolve a GitHub owner/repo from the git remote at %s", repoPath)
	}
	return ref, nil
}
