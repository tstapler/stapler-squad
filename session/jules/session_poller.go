package jules

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	julesapi "github.com/tstapler/stapler-squad/jules"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// julesSessionUUIDPrefix marks every jules_work ItemSession's session_uuid —
// both the pre-CreateSession reservation form (session.JulesPendingUUIDPrefix,
// already declared package-locally in storage_backlog.go by Epic 2.1 — NOT
// redeclared here, since a second `const session.JulesPendingUUIDPrefix = ...` in
// this same package would be a duplicate-declaration compile error) and the
// confirmed "jules-sessions/{id}" form once CreateSession succeeds.
// julesSessionUUIDPrefix itself mirrors the constant Epic 2.2's
// server/services/jules_dispatch_service.go declares (duplicated rather than
// imported: server/services imports session, so the reverse import would
// cycle) — same precedent as headlessTriageUUIDPrefix/
// headlessTriageSessionUUIDPrefix (session/backlog_lifecycle_triage.go:43-50,
// server/services/backlog_service_triage.go:423). Keep byte-identical with
// that copy; a future value change must update both.
const julesSessionUUIDPrefix = "jules-"

// julesPendingReservationMaxAge bounds how long a jules-pending- reservation
// row may sit unconfirmed before the poller treats the dispatch as abandoned
// (Story 2.3.3).
const julesPendingReservationMaxAge = 10 * time.Minute

// julesAuthBlockedNote / julesAuthRestoredNote are the exact, dedup'd
// progress-note strings Story 2.3.4's acceptance criteria assert verbatim.
const (
	julesAuthBlockedNote  = "Jules session needs reauthentication — update your API key in Settings."
	julesAuthRestoredNote = "Jules reconnected — resuming normal polling."
)

// julesStatusClient is the narrow slice of *julesapi.Client the poller needs —
// declared locally so tests can fake it without a real HTTP round trip.
type julesStatusClient interface {
	GetSession(ctx context.Context, name julesapi.JulesSessionName) (*julesapi.JulesSession, error)
	IsLimited() bool
}

// julesPollerStorage is the narrow slice of *Storage the poller needs.
// Beyond the five methods Epic 2.1/2.3.1's design named (ListOpenJulesItemSessions,
// TouchItemSessionProgress, SetBacklogItemPRAndTransition,
// UpdateItemSessionEndedWithReason, AppendProgressNote), this also needs
// GetItemSessionBySessionUUID (ListOpenJulesItemSessions' ItemSessionBacklogEntry
// carries only session_uuid/item_id/item metadata — not the ItemSession's own
// row ID, started_at, or created_at, all of which Story 2.3.2/2.3.3's mapper
// and reconciliation sweeps require; GetItemSessionBySessionUUID is the
// existing lookup that fills that gap, already used elsewhere in this package
// for the same reason, e.g. session/backlog_lifecycle.go:676) and
// TransitionBacklogItemStatus (SetBacklogItemPRAndTransition only accepts an
// item already in "review" or "pr_pending"; a Jules item sits in "in_progress"
// the whole time its session is open — Story 2.1.3 — so applyJulesState must
// transition in_progress -> review itself before recording a PR, mirroring
// what request_review does for a local agent session).
// julesUsageRecorder is the narrow slice of *services.JulesUsageCounter
// JulesSessionPoller needs (Task 4.1.1a) — declared locally, not imported,
// since session cannot import server/services (import-direction constraint,
// see julesSessionUUIDPrefix's doc comment above). *services.JulesUsageCounter
// satisfies this structurally; server/dependencies.go passes it in via
// SetUsageCounter after constructing both.
type julesUsageRecorder interface {
	IncSessionCompleted()
	IncSessionFailed()
	IncAPIRateLimited()
	IncAPIError()
}

type julesPollerStorage interface {
	ListOpenJulesItemSessions(ctx context.Context) ([]backlogdto.ItemSessionBacklogEntry, error)
	GetItemSessionBySessionUUID(ctx context.Context, sessionUUID string) (backlogdto.ItemSessionSummary, error)
	TouchItemSessionProgress(ctx context.Context, id string, at time.Time) error
	UpdateItemSessionEndedWithReason(ctx context.Context, id string, endedAt time.Time, reason string) error
	AppendProgressNote(ctx context.Context, itemID string, criterionIndex int, note, status string) error
	GatedSetPRAndTransition(ctx context.Context, req session.TransitionRequest, pr session.PRAssociation) error
	GatedTransition(ctx context.Context, req session.TransitionRequest) (*backlogdto.BacklogItemData, error)
	// AttachItemPRLink records one Outputs[] PR as an item-direct link; attached is
	// false when the item tombstoned that PR.
	AttachItemPRLink(ctx context.Context, itemID, prURL string, source domain.AttachSource) (attached bool, err error)
}

// JulesSessionPollerConfig controls polling cadence and staleness thresholds.
type JulesSessionPollerConfig struct {
	PollInterval time.Duration
	CallTimeout  time.Duration
	// MaxSessionAge bounds how long a session may stay open before the
	// poller ends it as timed out (Story 2.3.3), rather than polling it
	// indefinitely.
	MaxSessionAge time.Duration
	// NoChangeBackoff is reserved for a future per-session polling backoff
	// once a session has reported the same state for several consecutive
	// ticks. Not yet consumed by tick/applyJulesState — every open session
	// is polled every tick in this epic's implementation.
	NoChangeBackoff time.Duration
}

// DefaultJulesSessionPollerConfig returns sensible defaults, mirroring
// WorktreePRPollerConfig's shape (session/worktree_pr_poller.go:37-55).
func DefaultJulesSessionPollerConfig() JulesSessionPollerConfig {
	return JulesSessionPollerConfig{
		PollInterval:    60 * time.Second,
		CallTimeout:     20 * time.Second,
		MaxSessionAge:   24 * time.Hour,
		NoChangeBackoff: 5 * time.Minute,
	}
}

// JulesSessionPoller ticks over every open jules_work ItemSession, converting
// Jules' remote session state into backlog effects via applyJulesState. See
// the Epic 2.3 goal: fail soft, and never leave an item stuck in in_progress
// forever.
type JulesSessionPoller struct {
	client  julesStatusClient
	storage julesPollerStorage
	config  JulesSessionPollerConfig

	// usage (julesUsageRecorder, Task 4.1.1a) is nil until SetUsageCounter is
	// called — server/dependencies.go wires the process-wide
	// *services.JulesUsageCounter after constructing both. Every Inc call
	// site below is nil-guarded via incSessionCompleted/incSessionFailed/
	// incAPIRateLimited/incAPIError, so no existing test that never calls
	// SetUsageCounter needs updating.
	usage julesUsageRecorder

	// nowFn is overridable in tests to drive the age-based sweeps
	// (Story 2.3.3b) without sleeping.
	nowFn func() time.Time

	// authReconnectRequired is process-level (Story 2.3.4): a 401/403 mid-poll
	// is an account-wide condition (the key itself is invalid), not specific
	// to any one session, so it is tracked once here rather than per-row.
	authReconnectRequired atomic.Bool

	// authBlockedItems dedups the "needs reauthentication" note per item —
	// only items with an open Jules session that has actually hit
	// ErrJulesNotConfigured get an entry, cleared again once auth recovers.
	authBlockedMu    sync.Mutex
	authBlockedItems map[string]bool

	// lastState dedups the "state changed" progress note per session
	// (Story 2.3.2b): only a change from the last-observed state writes a
	// note. Reset on process restart — acceptable per the plan (at most one
	// duplicate note per session across a restart).
	lastStateMu sync.Mutex
	lastState   map[string]julesapi.JulesSessionState

	startOnce sync.Once
	cancel    context.CancelFunc
	done      chan struct{}
}

// NewJulesSessionPoller creates a JulesSessionPoller. client and storage are
// narrow, locally-declared interfaces so tests need no real Jules HTTP
// client or database.
func NewJulesSessionPoller(client julesStatusClient, storage julesPollerStorage, cfg JulesSessionPollerConfig) *JulesSessionPoller {
	return &JulesSessionPoller{
		client:           client,
		storage:          storage,
		config:           cfg,
		authBlockedItems: make(map[string]bool),
		lastState:        make(map[string]julesapi.JulesSessionState),
		nowFn:            time.Now,
	}
}

func (p *JulesSessionPoller) now() time.Time {
	if p.nowFn != nil {
		return p.nowFn()
	}
	return time.Now()
}

// SetUsageCounter wires the process-wide *services.JulesUsageCounter
// dependency post-construction (Task 4.1.1a) — needed because
// server/dependencies.go constructs the counter alongside, not before, this
// poller. nil (the zero value) is safe: every increment call site is
// nil-guarded via the incXxx helpers below.
func (p *JulesSessionPoller) SetUsageCounter(usage julesUsageRecorder) {
	p.usage = usage
}

func (p *JulesSessionPoller) incSessionCompleted() {
	if p.usage != nil {
		p.usage.IncSessionCompleted()
	}
}

func (p *JulesSessionPoller) incSessionFailed() {
	if p.usage != nil {
		p.usage.IncSessionFailed()
	}
}

func (p *JulesSessionPoller) incAPIRateLimited() {
	if p.usage != nil {
		p.usage.IncAPIRateLimited()
	}
}

func (p *JulesSessionPoller) incAPIError() {
	if p.usage != nil {
		p.usage.IncAPIError()
	}
}

// AuthReconnectRequired reports whether the most recent poll observed a
// 401/403 from the Jules API (an invalid or expired key) and has not yet
// seen a subsequent successful call. See Story 2.3.4.
func (p *JulesSessionPoller) AuthReconnectRequired() bool {
	return p.authReconnectRequired.Load()
}

// Start begins the polling loop. It is a no-op if already started, and
// returns once the goroutine has been launched (not once it exits) — the
// goroutine itself exits shortly after ctx is cancelled.
func (p *JulesSessionPoller) Start(ctx context.Context) {
	p.startOnce.Do(func() {
		tickCtx, cancel := context.WithCancel(ctx)
		p.cancel = cancel
		p.done = make(chan struct{})
		go p.run(tickCtx)
	})
}

// run drives the ticker loop until ctx is cancelled.
func (p *JulesSessionPoller) run(ctx context.Context) {
	defer close(p.done)
	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.tick(ctx)
		}
	}
}

// Wait blocks until the polling goroutine has exited, after Start's ctx is
// cancelled. It returns immediately if Start was never called.
func (p *JulesSessionPoller) Wait() {
	if p.done != nil {
		<-p.done
	}
}

// Stop cancels the polling loop and waits for the goroutine to exit. Safe to
// call even if Start was never called.
func (p *JulesSessionPoller) Stop() {
	if p.cancel != nil {
		p.cancel()
		<-p.done
	}
}

// tick performs one polling pass over every open jules_work ItemSession.
// Every per-session error is logged and swallowed (Task 2.3.1b) — a single
// failing session never aborts the tick.
func (p *JulesSessionPoller) tick(ctx context.Context) {
	if p.client.IsLimited() {
		log.Debug("jules poll tick", "open_sessions", 0, "polled", 0, "skipped_rate_limited", true)
		return
	}

	entries, err := p.storage.ListOpenJulesItemSessions(ctx)
	if err != nil {
		log.Warn("jules poll failed", "error", err)
		return
	}

	polled := 0
	sawSuccess := false
	for _, entry := range entries {
		attempted, success := p.processEntry(ctx, entry)
		if attempted {
			polled++
		}
		if success {
			sawSuccess = true
		}
	}

	if sawSuccess {
		p.maybeRecoverAuth(ctx)
	}

	log.Debug("jules poll tick", "open_sessions", len(entries), "polled", polled, "skipped_rate_limited", false)
}

// processEntry handles one open jules_work row: the pre-GetSession age/
// reservation sweeps (Story 2.3.3b), then the GetSession call and its error
// branches (Stories 2.3.3a/2.3.4), then applyJulesState on success.
// attempted reports whether a GetSession call was actually made; success
// reports whether it returned 200 OK (used to drive auth recovery).
func (p *JulesSessionPoller) processEntry(ctx context.Context, entry backlogdto.ItemSessionBacklogEntry) (attempted, success bool) {
	row, err := p.storage.GetItemSessionBySessionUUID(ctx, entry.SessionUUID)
	if err != nil {
		log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
		return false, false
	}

	// Abandoned-reservation sweep: a row still carrying the pre-CreateSession
	// placeholder UUID has no real Jules session to poll at all.
	if strings.HasPrefix(entry.SessionUUID, session.JulesPendingUUIDPrefix) {
		if p.now().Sub(row.CreatedAt) > julesPendingReservationMaxAge {
			p.failJulesSession(ctx, entry, row.ID, "dispatch_incomplete",
				"Jules dispatch did not complete — check jules.google.com in case a session was created but not recorded here, then retry dispatch if needed.")
		}
		return false, false
	}

	// Age sweep. Jules gets its own age model rather than reusing
	// session/backlog_lifecycle_stale.go: that sweep is tuned to local tmux
	// processes going quiet, and would misfire on a healthy long-running
	// cloud task with no local process to watch.
	if row.StartedAt != nil && p.now().Sub(*row.StartedAt) > p.config.MaxSessionAge {
		p.failJulesSession(ctx, entry, row.ID, "jules_timed_out",
			fmt.Sprintf("Jules session exceeded the %s maximum runtime and was ended automatically. Check jules.google.com for its final state.", p.config.MaxSessionAge))
		return false, false
	}

	name, err := julesSessionNameFromUUID(entry.SessionUUID)
	if err != nil {
		log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
		return false, false
	}

	callCtx, cancel := context.WithTimeout(ctx, p.config.CallTimeout)
	defer cancel()

	s, err := p.client.GetSession(callCtx, name)
	if err != nil {
		p.handleGetSessionError(ctx, entry, row, err)
		return true, false
	}

	if applyErr := p.applyJulesState(ctx, entry, row, s); applyErr != nil {
		log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", applyErr)
	}
	return true, true
}

// handleGetSessionError classifies a GetSession error into the reconnect-
// required branch (Story 2.3.4), the vanished-session branch (Story 2.3.3a),
// or the generic transient/logged-and-swallowed path.
func (p *JulesSessionPoller) handleGetSessionError(ctx context.Context, entry backlogdto.ItemSessionBacklogEntry, row backlogdto.ItemSessionSummary, err error) {
	switch {
	case errors.Is(err, julesapi.ErrJulesRateLimited):
		p.incAPIRateLimited()
		log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
	case errors.Is(err, julesapi.ErrJulesNotConfigured):
		// Account-wide: the key itself is invalid, not this session
		// specifically. Do not end the session, transition the item, or
		// touch progress — a stale key is not evidence the task failed.
		p.incAPIError()
		p.authReconnectRequired.Store(true)
		p.appendAuthBlockedNoteOnce(ctx, entry.ItemID)
		log.Warn("jules session auth invalid", "jules_session", entry.SessionUUID, "item_id", entry.ItemID)
	case errors.Is(err, julesapi.ErrJulesSessionNotFound):
		p.incAPIError()
		p.failJulesSession(ctx, entry, row.ID, "jules_session_missing",
			"Jules no longer reports this session — it may have expired or been removed on Jules' side. Check jules.google.com, then retry from here if the work still needs doing.")
	default:
		p.incAPIError()
		log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
	}
}

// maybeRecoverAuth clears authReconnectRequired and notifies every item with
// an outstanding blocked note the first time a tick observes a successful
// GetSession call after the flag was set (Story 2.3.4).
func (p *JulesSessionPoller) maybeRecoverAuth(ctx context.Context) {
	if !p.authReconnectRequired.CompareAndSwap(true, false) {
		return
	}

	p.authBlockedMu.Lock()
	items := make([]string, 0, len(p.authBlockedItems))
	for id := range p.authBlockedItems {
		items = append(items, id)
		delete(p.authBlockedItems, id)
	}
	p.authBlockedMu.Unlock()

	for _, id := range items {
		if err := p.storage.AppendProgressNote(ctx, id, -1, julesAuthRestoredNote, string(session.BacklogStatusInProgress)); err != nil {
			log.Warn("jules poll failed", "item_id", id, "error", err)
		}
	}
	log.Info("jules session auth restored")
}

// appendAuthBlockedNoteOnce writes the reauthentication note for itemID at
// most once per occurrence — repeated ticks while still blocked write
// nothing further.
func (p *JulesSessionPoller) appendAuthBlockedNoteOnce(ctx context.Context, itemID string) {
	p.authBlockedMu.Lock()
	if p.authBlockedItems[itemID] {
		p.authBlockedMu.Unlock()
		return
	}
	p.authBlockedItems[itemID] = true
	p.authBlockedMu.Unlock()

	if err := p.storage.AppendProgressNote(ctx, itemID, -1, julesAuthBlockedNote, "blocked"); err != nil {
		log.Warn("jules poll failed", "item_id", itemID, "error", err)
	}
}

// failJulesSession ends sessionID with reason, best-effort returns its owning
// item from in_progress to ready, and records note. Shared by Story 2.3.3's
// FAILED/not-found/timeout/abandoned-reservation branches. Every step is
// logged and swallowed individually (fail soft) — a failure here must not
// prevent the others from running, mirroring AppendProgressNote's own
// best-effort discipline elsewhere in this package.
func (p *JulesSessionPoller) failJulesSession(ctx context.Context, entry backlogdto.ItemSessionBacklogEntry, sessionID, reason, note string) {
	p.incSessionFailed()
	now := p.now()
	if err := p.storage.UpdateItemSessionEndedWithReason(ctx, sessionID, now, reason); err != nil {
		log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
	}
	precondition := &backlogdto.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusInProgress), Note: note}
	// Tolerated: an abandoned reservation's item may never have left "ready" (the
	// move to in_progress happens only after CreateSession confirms), so a lost
	// precondition is a harmless no-op, and a gate denial is reported by the gate.
	if _, err := p.storage.GatedTransition(ctx, session.TransitionRequest{
		Caller: session.CallerJulesFailSession, ItemID: entry.ItemID, To: session.BacklogStatusReady, Precondition: precondition, TriggeredBy: session.TriggeredBySystem,
	}); err != nil {
		log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
	}
	if err := p.storage.AppendProgressNote(ctx, entry.ItemID, -1, note, string(session.BacklogStatusReady)); err != nil {
		log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
	}
}

// applyJulesState is the one exhaustive mapper from a Jules session's
// reported state to a backlog effect (Story 2.3.2). Every declared
// julesapi.JulesSessionState constant gets its own named branch — no default
// that silently succeeds — so a new alpha-API enum value fails loudly
// (applyUnknownState) instead of leaving an item stuck in in_progress
// forever. See TestApplyJulesState_should_HandleEveryDeclaredState_When_StatesEnumerated.
func (p *JulesSessionPoller) applyJulesState(ctx context.Context, entry backlogdto.ItemSessionBacklogEntry, row backlogdto.ItemSessionSummary, s *julesapi.JulesSession) error {
	switch s.State {
	case julesapi.JulesStateQueued, julesapi.JulesStatePlanning, julesapi.JulesStateAwaitingPlanApproval, julesapi.JulesStateInProgress:
		return p.applyNonTerminalState(ctx, entry, row, s.State)
	case julesapi.JulesStateCompleted:
		return p.applyCompletedState(ctx, entry, row, s)
	case julesapi.JulesStateFailed:
		return p.applyFailedState(ctx, entry, row, s)
	case julesapi.JulesStateUnknown:
		return p.applyUnknownState(ctx, entry, row, s)
	default:
		// Any raw wire value IsKnown() doesn't recognize (e.g. a new
		// alpha-API state this package has never seen) falls here.
		return p.applyUnknownState(ctx, entry, row, s)
	}
}

// applyNonTerminalState touches progress and, only on an actual state
// change, appends a visible note (Story 2.3.2b's dedup).
func (p *JulesSessionPoller) applyNonTerminalState(ctx context.Context, entry backlogdto.ItemSessionBacklogEntry, row backlogdto.ItemSessionSummary, state julesapi.JulesSessionState) error {
	if err := p.storage.TouchItemSessionProgress(ctx, row.ID, p.now()); err != nil {
		return fmt.Errorf("touch progress: %w", err)
	}
	p.noteStateChangeIfNeeded(ctx, entry, state)
	return nil
}

// noteStateChangeIfNeeded appends a progress note only when state differs
// from the last state observed for this session (Story 2.3.2b).
func (p *JulesSessionPoller) noteStateChangeIfNeeded(ctx context.Context, entry backlogdto.ItemSessionBacklogEntry, state julesapi.JulesSessionState) {
	p.lastStateMu.Lock()
	prev, seen := p.lastState[entry.SessionUUID]
	changed := !seen || prev != state
	p.lastState[entry.SessionUUID] = state
	p.lastStateMu.Unlock()

	if !changed {
		return
	}

	note := fmt.Sprintf("Jules session is now %s.", julesStateNoteLabel(state))
	if err := p.storage.AppendProgressNote(ctx, entry.ItemID, -1, note, string(session.BacklogStatusInProgress)); err != nil {
		log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
	}
	log.Info("jules session state changed", "jules_session", entry.SessionUUID, "from", string(prev), "to", string(state))
}

// applyCompletedState records the PR and hands off to the existing review
// path when Jules produced one, or surfaces the no-PR case loudly otherwise
// (Story 2.3.2). SetBacklogItemPRAndTransition only accepts an item already
// in "review" or "pr_pending" (session/storage.go:868), so a Jules item —
// which sits in "in_progress" for the session's entire lifetime, Story
// 2.1.3 — is first moved in_progress -> review here, mirroring what
// request_review does for a local agent session, before either branch runs.
func (p *JulesSessionPoller) applyCompletedState(ctx context.Context, entry backlogdto.ItemSessionBacklogEntry, row backlogdto.ItemSessionSummary, s *julesapi.JulesSession) error {
	p.incSessionCompleted()
	prURLs := completedSessionPRURLs(s)
	prURL := ""
	if len(prURLs) > 0 {
		prURL = prURLs[0]
	}
	now := p.now()

	if prURL == "" {
		if err := p.storage.UpdateItemSessionEndedWithReason(ctx, row.ID, now, "jules_completed_no_pr"); err != nil {
			return fmt.Errorf("end session (no pr): %w", err)
		}
		note := fmt.Sprintf("Jules finished this session but did not open a pull request. Check the session at %s.", julesSessionWebURL(s))
		// Best-effort status advance: the progress note below still records the no-PR
		// outcome, and the session row is already closed so no sweep retries this.
		if _, err := p.storage.GatedTransition(ctx, session.TransitionRequest{
			Caller: session.CallerJulesApplyCompleted, ItemID: entry.ItemID, To: session.BacklogStatusReview,
			Precondition: &backlogdto.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusInProgress), Note: note}, TriggeredBy: session.TriggeredBySystem,
		}); err != nil {
			log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
		}
		if err := p.storage.AppendProgressNote(ctx, entry.ItemID, -1, note, string(session.BacklogStatusReview)); err != nil {
			log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
		}
		return nil
	}

	prNumber, err := session.ParsePRNumberFromURL(prURL)
	if err != nil {
		return fmt.Errorf("parse PR number from %q: %w", prURL, err)
	}

	// Links first: an upsert is idempotent, so a failure here retries cleanly on the
	// next poll before any transition has happened.
	for _, u := range prURLs {
		attached, linkErr := p.storage.AttachItemPRLink(ctx, entry.ItemID, u, domain.AttachSourceJules)
		if linkErr != nil {
			return fmt.Errorf("attach jules pr %s: %w", u, linkErr)
		}
		if !attached {
			log.Info("jules pr skipped: item tombstoned it", "jules_session", entry.SessionUUID, "url", u)
		}
	}

	reviewItem, err := p.storage.GatedTransition(ctx, session.TransitionRequest{
		Caller: session.CallerJulesApplyCompleted, ItemID: entry.ItemID, To: session.BacklogStatusReview,
		Precondition: &backlogdto.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusInProgress), Note: "Jules session completed"}, TriggeredBy: session.TriggeredBySystem,
	})
	if err != nil {
		return fmt.Errorf("transition to review: %w", err)
	}

	summary := fmt.Sprintf("Jules opened pull request %s.", prURL)
	if err := p.storage.GatedSetPRAndTransition(ctx,
		session.TransitionRequest{Caller: session.CallerJulesApplyCompleted, ItemID: entry.ItemID, Observed: reviewItem, TriggeredBy: session.TriggeredBySystem},
		session.PRAssociation{URL: prURL, Number: prNumber, Summary: summary, Source: domain.AttachSourceJules}); err != nil {
		return fmt.Errorf("record PR: %w", err)
	}
	if err := p.storage.UpdateItemSessionEndedWithReason(ctx, row.ID, now, "jules_completed"); err != nil {
		return fmt.Errorf("end session: %w", err)
	}
	return nil
}

// applyFailedState ends the session and returns the item to ready, quoting
// Jules' own failure text and session URL (Story 2.3.3).
func (p *JulesSessionPoller) applyFailedState(ctx context.Context, entry backlogdto.ItemSessionBacklogEntry, row backlogdto.ItemSessionSummary, s *julesapi.JulesSession) error {
	reasonText := s.Title
	if reasonText == "" {
		reasonText = "Jules reported no further detail."
	}
	note := fmt.Sprintf("Jules session failed: %s (%s)", reasonText, julesSessionWebURL(s))
	p.failJulesSession(ctx, entry, row.ID, "jules_failed", note)
	return nil
}

// applyUnknownState is the alpha-API-drift alarm (Story 2.3.2): logs loudly
// at Error, still touches progress so the session isn't independently swept
// as stale, and makes no transition.
func (p *JulesSessionPoller) applyUnknownState(ctx context.Context, entry backlogdto.ItemSessionBacklogEntry, row backlogdto.ItemSessionSummary, s *julesapi.JulesSession) error {
	log.Error("jules unknown session state", "jules_session", entry.SessionUUID, "raw_state", s.State.Raw())
	if err := p.storage.TouchItemSessionProgress(ctx, row.ID, p.now()); err != nil {
		return fmt.Errorf("touch progress: %w", err)
	}
	return nil
}

// completedSessionPRURLs returns every pull request URL in s.Outputs that passes
// isValidGitHubPRURL, in order and de-duplicated; empty if COMPLETED carried no
// (valid) PR output. The Jules API response is semi-trusted output (target-repo
// content the session processed can influence it via prompt injection, per this
// feature's egress-consent threat model), and a URL is persisted and rendered as
// an <a href> in GitHubBadge.tsx, so a value that isn't actually an
// https://github.com/... URL is rejected here rather than stored; a bare
// trailing-path regex match (e.g. "javascript:alert(1)//pull/1") is not enough.
func completedSessionPRURLs(s *julesapi.JulesSession) []string {
	var urls []string
	seen := map[string]bool{}
	for _, out := range s.Outputs {
		if out.PullRequest == nil || out.PullRequest.URL == "" {
			continue
		}
		if !isValidGitHubPRURL(out.PullRequest.URL) {
			log.Warn("jules rejected invalid pr url", "jules_session", s.Name, "url", out.PullRequest.URL)
			continue
		}
		if !seen[out.PullRequest.URL] {
			seen[out.PullRequest.URL] = true
			urls = append(urls, out.PullRequest.URL)
		}
	}
	return urls
}

// isValidGitHubPRURL reports whether rawURL is a genuine
// https://github.com/.../pull/<n> URL — scheme and host anchored, not just
// matched against the trailing /pull/(\d+)/? shape session.ParsePRNumberFromURL checks.
// Anchoring only the suffix would accept e.g. "javascript:alert(1)//pull/1",
// which the PR-number parse alone accepts.
func isValidGitHubPRURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" {
		return false
	}
	_, err = session.ParsePRNumberFromURL(rawURL)
	return err == nil
}

// julesSessionWebURL returns s.URL, or a generic pointer to the Jules web UI
// if the API response didn't carry one.
func julesSessionWebURL(s *julesapi.JulesSession) string {
	if s.URL != "" {
		return s.URL
	}
	return "https://jules.google.com"
}

// julesStateNoteLabel renders a non-terminal state as the lowercase
// participle used in "Jules session is now <label>." progress notes.
func julesStateNoteLabel(state julesapi.JulesSessionState) string {
	switch state {
	case julesapi.JulesStateQueued:
		return "queued"
	case julesapi.JulesStatePlanning:
		return "planning"
	case julesapi.JulesStateAwaitingPlanApproval:
		return "awaiting plan approval"
	case julesapi.JulesStateInProgress:
		return "in progress"
	default:
		return strings.ToLower(state.String())
	}
}

// julesSessionNameFromUUID converts a stored session_uuid ("jules-sessions/xyz")
// into the wire-format julesapi.JulesSessionName GetSession expects ("sessions/xyz").
func julesSessionNameFromUUID(sessionUUID string) (julesapi.JulesSessionName, error) {
	raw := strings.TrimPrefix(sessionUUID, julesSessionUUIDPrefix)
	return julesapi.ParseJulesSessionName(raw)
}
