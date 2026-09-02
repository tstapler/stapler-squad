package session

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/jules"
	"github.com/tstapler/stapler-squad/log"
)

// julesSessionUUIDPrefix marks every jules_work ItemSession's session_uuid —
// both the pre-CreateSession reservation form (julesPendingUUIDPrefix,
// already declared package-locally in storage_backlog.go by Epic 2.1 — NOT
// redeclared here, since a second `const julesPendingUUIDPrefix = ...` in
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

// julesStatusClient is the narrow slice of *jules.Client the poller needs —
// declared locally so tests can fake it without a real HTTP round trip.
type julesStatusClient interface {
	GetSession(ctx context.Context, name jules.JulesSessionName) (*jules.JulesSession, error)
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
	ListOpenJulesItemSessions(ctx context.Context) ([]ItemSessionBacklogEntry, error)
	GetItemSessionBySessionUUID(ctx context.Context, sessionUUID string) (ItemSessionSummary, error)
	TouchItemSessionProgress(ctx context.Context, id string, at time.Time) error
	UpdateItemSessionEndedWithReason(ctx context.Context, id string, endedAt time.Time, reason string) error
	AppendProgressNote(ctx context.Context, itemID string, criterionIndex int, note, status string) error
	SetBacklogItemPRAndTransition(ctx context.Context, observed *BacklogItemData, prURL string, prNumber int, summary string, guard *PRReassignmentGuard) error
	TransitionBacklogItemStatus(ctx context.Context, id string, toStatus BacklogStatus, precondition *BacklogItemPrecondition, triggeredBy string) (*BacklogItemData, error)
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
	lastState   map[string]jules.JulesSessionState

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
		lastState:        make(map[string]jules.JulesSessionState),
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
func (p *JulesSessionPoller) processEntry(ctx context.Context, entry ItemSessionBacklogEntry) (attempted, success bool) {
	row, err := p.storage.GetItemSessionBySessionUUID(ctx, entry.SessionUUID)
	if err != nil {
		log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
		return false, false
	}

	// Abandoned-reservation sweep: a row still carrying the pre-CreateSession
	// placeholder UUID has no real Jules session to poll at all.
	if strings.HasPrefix(entry.SessionUUID, julesPendingUUIDPrefix) {
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
func (p *JulesSessionPoller) handleGetSessionError(ctx context.Context, entry ItemSessionBacklogEntry, row ItemSessionSummary, err error) {
	switch {
	case errors.Is(err, jules.ErrJulesRateLimited):
		p.incAPIRateLimited()
		log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
	case errors.Is(err, jules.ErrJulesNotConfigured):
		// Account-wide: the key itself is invalid, not this session
		// specifically. Do not end the session, transition the item, or
		// touch progress — a stale key is not evidence the task failed.
		p.incAPIError()
		p.authReconnectRequired.Store(true)
		p.appendAuthBlockedNoteOnce(ctx, entry.ItemID)
		log.Warn("jules session auth invalid", "jules_session", entry.SessionUUID, "item_id", entry.ItemID)
	case errors.Is(err, jules.ErrJulesSessionNotFound):
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
		if err := p.storage.AppendProgressNote(ctx, id, -1, julesAuthRestoredNote, string(BacklogStatusInProgress)); err != nil {
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
func (p *JulesSessionPoller) failJulesSession(ctx context.Context, entry ItemSessionBacklogEntry, sessionID, reason, note string) {
	p.incSessionFailed()
	now := p.now()
	if err := p.storage.UpdateItemSessionEndedWithReason(ctx, sessionID, now, reason); err != nil {
		log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
	}
	precondition := &BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusInProgress), Note: note}
	if _, err := p.storage.TransitionBacklogItemStatus(ctx, entry.ItemID, BacklogStatusReady, precondition, TriggeredBySystem); err != nil {
		// Tolerated: an abandoned reservation's item may never have left
		// "ready" in the first place (Epic 2.2's transition to in_progress
		// happens only after CreateSession confirms) — in that case this is
		// a harmless no-op, not a real failure.
		log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
	}
	if err := p.storage.AppendProgressNote(ctx, entry.ItemID, -1, note, string(BacklogStatusReady)); err != nil {
		log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
	}
}

// applyJulesState is the one exhaustive mapper from a Jules session's
// reported state to a backlog effect (Story 2.3.2). Every declared
// jules.JulesSessionState constant gets its own named branch — no default
// that silently succeeds — so a new alpha-API enum value fails loudly
// (applyUnknownState) instead of leaving an item stuck in in_progress
// forever. See TestApplyJulesState_should_HandleEveryDeclaredState_When_StatesEnumerated.
func (p *JulesSessionPoller) applyJulesState(ctx context.Context, entry ItemSessionBacklogEntry, row ItemSessionSummary, s *jules.JulesSession) error {
	switch s.State {
	case jules.JulesStateQueued, jules.JulesStatePlanning, jules.JulesStateAwaitingPlanApproval, jules.JulesStateInProgress:
		return p.applyNonTerminalState(ctx, entry, row, s.State)
	case jules.JulesStateCompleted:
		return p.applyCompletedState(ctx, entry, row, s)
	case jules.JulesStateFailed:
		return p.applyFailedState(ctx, entry, row, s)
	case jules.JulesStateUnknown:
		return p.applyUnknownState(ctx, entry, row, s)
	default:
		// Any raw wire value IsKnown() doesn't recognize (e.g. a new
		// alpha-API state this package has never seen) falls here.
		return p.applyUnknownState(ctx, entry, row, s)
	}
}

// applyNonTerminalState touches progress and, only on an actual state
// change, appends a visible note (Story 2.3.2b's dedup).
func (p *JulesSessionPoller) applyNonTerminalState(ctx context.Context, entry ItemSessionBacklogEntry, row ItemSessionSummary, state jules.JulesSessionState) error {
	if err := p.storage.TouchItemSessionProgress(ctx, row.ID, p.now()); err != nil {
		return fmt.Errorf("touch progress: %w", err)
	}
	p.noteStateChangeIfNeeded(ctx, entry, state)
	return nil
}

// noteStateChangeIfNeeded appends a progress note only when state differs
// from the last state observed for this session (Story 2.3.2b).
func (p *JulesSessionPoller) noteStateChangeIfNeeded(ctx context.Context, entry ItemSessionBacklogEntry, state jules.JulesSessionState) {
	p.lastStateMu.Lock()
	prev, seen := p.lastState[entry.SessionUUID]
	changed := !seen || prev != state
	p.lastState[entry.SessionUUID] = state
	p.lastStateMu.Unlock()

	if !changed {
		return
	}

	note := fmt.Sprintf("Jules session is now %s.", julesStateNoteLabel(state))
	if err := p.storage.AppendProgressNote(ctx, entry.ItemID, -1, note, string(BacklogStatusInProgress)); err != nil {
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
func (p *JulesSessionPoller) applyCompletedState(ctx context.Context, entry ItemSessionBacklogEntry, row ItemSessionSummary, s *jules.JulesSession) error {
	p.incSessionCompleted()
	prURL := completedSessionPRURL(s)
	now := p.now()

	if prURL == "" {
		if err := p.storage.UpdateItemSessionEndedWithReason(ctx, row.ID, now, "jules_completed_no_pr"); err != nil {
			return fmt.Errorf("end session (no pr): %w", err)
		}
		note := fmt.Sprintf("Jules finished this session but did not open a pull request. Check the session at %s.", julesSessionWebURL(s))
		if _, err := p.storage.TransitionBacklogItemStatus(ctx, entry.ItemID, BacklogStatusReview,
			&BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusInProgress), Note: note}, TriggeredBySystem); err != nil {
			log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
		}
		if err := p.storage.AppendProgressNote(ctx, entry.ItemID, -1, note, string(BacklogStatusReview)); err != nil {
			log.Warn("jules poll failed", "jules_session", entry.SessionUUID, "error", err)
		}
		return nil
	}

	prNumber, err := ParsePRNumberFromURL(prURL)
	if err != nil {
		return fmt.Errorf("parse PR number from %q: %w", prURL, err)
	}

	reviewItem, err := p.storage.TransitionBacklogItemStatus(ctx, entry.ItemID, BacklogStatusReview,
		&BacklogItemPrecondition{ExpectedStatus: string(BacklogStatusInProgress), Note: "Jules session completed"}, TriggeredBySystem)
	if err != nil {
		return fmt.Errorf("transition to review: %w", err)
	}

	summary := fmt.Sprintf("Jules opened pull request %s.", prURL)
	if err := p.storage.SetBacklogItemPRAndTransition(ctx, reviewItem, prURL, prNumber, summary, nil); err != nil {
		return fmt.Errorf("record PR: %w", err)
	}
	if err := p.storage.UpdateItemSessionEndedWithReason(ctx, row.ID, now, "jules_completed"); err != nil {
		return fmt.Errorf("end session: %w", err)
	}
	return nil
}

// applyFailedState ends the session and returns the item to ready, quoting
// Jules' own failure text and session URL (Story 2.3.3).
func (p *JulesSessionPoller) applyFailedState(ctx context.Context, entry ItemSessionBacklogEntry, row ItemSessionSummary, s *jules.JulesSession) error {
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
func (p *JulesSessionPoller) applyUnknownState(ctx context.Context, entry ItemSessionBacklogEntry, row ItemSessionSummary, s *jules.JulesSession) error {
	log.Error("jules unknown session state", "jules_session", entry.SessionUUID, "raw_state", s.State.Raw())
	if err := p.storage.TouchItemSessionProgress(ctx, row.ID, p.now()); err != nil {
		return fmt.Errorf("touch progress: %w", err)
	}
	return nil
}

// completedSessionPRURL returns the first pull request URL in s.Outputs, or
// "" if COMPLETED carried no PR output.
func completedSessionPRURL(s *jules.JulesSession) string {
	for _, out := range s.Outputs {
		if out.PullRequest != nil && out.PullRequest.URL != "" {
			return out.PullRequest.URL
		}
	}
	return ""
}

// julesSessionWebURL returns s.URL, or a generic pointer to the Jules web UI
// if the API response didn't carry one.
func julesSessionWebURL(s *jules.JulesSession) string {
	if s.URL != "" {
		return s.URL
	}
	return "https://jules.google.com"
}

// julesStateNoteLabel renders a non-terminal state as the lowercase
// participle used in "Jules session is now <label>." progress notes.
func julesStateNoteLabel(state jules.JulesSessionState) string {
	switch state {
	case jules.JulesStateQueued:
		return "queued"
	case jules.JulesStatePlanning:
		return "planning"
	case jules.JulesStateAwaitingPlanApproval:
		return "awaiting plan approval"
	case jules.JulesStateInProgress:
		return "in progress"
	default:
		return strings.ToLower(state.String())
	}
}

// julesSessionNameFromUUID converts a stored session_uuid ("jules-sessions/xyz")
// into the wire-format jules.JulesSessionName GetSession expects ("sessions/xyz").
func julesSessionNameFromUUID(sessionUUID string) (jules.JulesSessionName, error) {
	raw := strings.TrimPrefix(sessionUUID, julesSessionUUIDPrefix)
	return jules.ParseJulesSessionName(raw)
}

// ParsePRNumberFromURL extracts the trailing PR number from a GitHub PR URL,
// reusing the same regex BackfillMissingPRNumbers already uses
// (session/storage_backlog.go:1040) so both parses agree exactly.
func ParsePRNumberFromURL(prURL string) (int, error) {
	m := prNumberFromURLRe.FindStringSubmatch(prURL)
	if m == nil {
		return 0, fmt.Errorf("no PR number found in %q", prURL)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("parsing PR number from %q: %w", prURL, err)
	}
	return n, nil
}
