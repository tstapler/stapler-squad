package services

import (
	"context"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// supersededSessionSweeperCheckInterval mirrors StaleCreationSweeper's cadence
// -- this is a correctness backstop, not a hot path, so 60s is fine-grained
// enough without meaningful I/O overhead.
const supersededSessionSweeperCheckInterval = 60 * time.Second

// supersededSessionStore is the narrow storage surface SupersededSessionSweeper
// needs -- ListBacklogItems to find every item whose rework rounds could have
// accumulated a superseded session, ListItemSessions to read each item's round
// history. GetBacklogItem and AppendActivityNote back Phase 6's additions:
// GetBacklogItem resolves a stalled dispatch's item title for its
// notification copy (Story 6.1.5), AppendActivityNote posts
// handoffThenCleanup's error/timeout-path diagnostic note via the same
// primitive server/mcp/tools_backlog.go's postBacklogUpdate uses (Story
// 6.1.2). Satisfied by *session.Storage.
type supersededSessionStore interface {
	ListBacklogItems(ctx context.Context, filter session.BacklogItemFilter) ([]session.BacklogItemData, error)
	ListItemSessions(ctx context.Context, itemID string) ([]session.ItemSessionSummary, error)
	GetBacklogItem(ctx context.Context, itemID string) (*session.BacklogItemData, error)
	AppendActivityNote(ctx context.Context, itemID, authorUUID, authorTitle, message string) error
}

// SupersededSessionSweeper is a ticker-driven defense-in-depth sweeper
// (modeled directly on StaleCreationSweeper) that archives any work/review
// ItemSession left behind by an older rework round once a newer round for the
// same item+role exists.
//
// It closes two gaps the spawn-time archive-on-supersede calls
// (TriggerReReview/spawnSessionAfterGates's archiveItemWorkSessions calls)
// cannot close on their own: (1) archiveItemWorkSessions is explicitly
// best-effort -- a transient failure there leaves an orphaned session with no
// retry, and this sweeper is the self-heal; (2) it converges on rows already
// persisted as stale, not just ones created after this sweeper exists.
//
// "Which round is current" is decided by findSupersededSessions, which
// reuses findMostRecentSessions' latest-CreatedAt-wins tie-break -- the same
// helper TriggerReReview itself already calls to find the prior review round
// to archive -- rather than this sweeper re-deriving its own, third
// definition of "current" (see findSupersededSessions' doc comment for why
// that's a distinct question from spawnSessionAfterGates' EndedAt/IsSessionLive-based
// "is a work session already open" guards, and why archiveIfNotLive's own
// liveness check, not tie-break agreement, is what protects a genuinely-live
// session from an incorrect archive).
//
// Never hard-kills: a superseded session that is still confirmed-live (a user
// may be mid-conversation or steering it) is skipped for this tick rather
// than archived out from under them -- see pitfalls.md #2. It re-evaluates on
// every tick, so a session that goes idle later is picked up on a later pass.
//
// Phase 6 (ADR-001: extend this sweeper rather than build a parallel
// mechanism) additionally reuses this same ticker for two more, deliberately
// disjoint predicates: findIdleStaleSessions (Story 6.1.1 -- a current round
// that has simply gone quiet, no newer round involved) and
// findStalledDiagnoseDispatches (Story 6.1.5 -- a diagnose dispatch whose
// session died without ever recording an outcome). handoffGenerator/instances
// and dispatchStore/notifier back those two additions respectively and are
// independently nil-safe: each sub-feature degrades to a no-op (or, for
// findIdleStaleSessions, a direct archive with no handoff summary) when its
// own dependencies are unwired, exactly like storage/stopper's own
// nil-safety convention below.
type SupersededSessionSweeper struct {
	storage supersededSessionStore
	stopper SessionStopper

	handoffGenerator staleSessionHandoffGenerator
	instances        InstanceLookup

	dispatchStore DiagnoseDispatchStore
	notifier      diagnoseEventNotifier

	staleThreshold   time.Duration
	stalledThreshold time.Duration
}

// SupersededSessionSweeperDeps groups Phase 6's additive, all-optional
// dependencies for SupersededSessionSweeper. Leaving a field unset degrades
// only the sub-feature that needs it to a no-op (or a plain archive, for
// HandoffGenerator/Instances) for that tick -- mirrors storage/stopper's own
// nil-safety convention. A zero StaleThreshold/StalledThreshold falls back to
// diagnoseIdleStaleThreshold/diagnoseDispatchStalledThreshold.
type SupersededSessionSweeperDeps struct {
	// HandoffGenerator and Instances back Story 6.1.2/6.1.3's
	// handoff-then-cleanup step. A findIdleStaleSessions match is archived
	// directly, with no handoff summary and no IsArchived() guard available,
	// when either is nil.
	HandoffGenerator staleSessionHandoffGenerator
	Instances        InstanceLookup
	// DispatchStore and Notifier back Story 6.1.5's stalled-dispatch sweep.
	// The whole stalled-dispatch check is skipped for a tick when
	// DispatchStore is nil.
	DispatchStore DiagnoseDispatchStore
	Notifier      diagnoseEventNotifier

	StaleThreshold   time.Duration
	StalledThreshold time.Duration
}

// NewSupersededSessionSweeper constructs a sweeper. storage and stopper are
// consulted every tick and may be nil during startup wiring -- sweep() is a
// no-op if either is unset, mirroring SessionStopper's own nil-safety
// convention used throughout BacklogService. deps' fields are independently
// optional -- see SupersededSessionSweeperDeps' doc comment.
func NewSupersededSessionSweeper(storage supersededSessionStore, stopper SessionStopper, deps SupersededSessionSweeperDeps) *SupersededSessionSweeper {
	staleThreshold := deps.StaleThreshold
	if staleThreshold <= 0 {
		staleThreshold = diagnoseIdleStaleThreshold
	}
	stalledThreshold := deps.StalledThreshold
	if stalledThreshold <= 0 {
		stalledThreshold = diagnoseDispatchStalledThreshold
	}
	return &SupersededSessionSweeper{
		storage:          storage,
		stopper:          stopper,
		handoffGenerator: deps.HandoffGenerator,
		instances:        deps.Instances,
		dispatchStore:    deps.DispatchStore,
		notifier:         deps.Notifier,
		staleThreshold:   staleThreshold,
		stalledThreshold: stalledThreshold,
	}
}

// Start runs the periodic sweep loop. Blocks until ctx is cancelled. Mirrors
// StaleCreationSweeper.Start's shape: run once immediately (so pre-existing
// stale rows are cleaned up on the very next restart, not just prevented
// going forward), then on every tick.
func (s *SupersededSessionSweeper) Start(ctx context.Context) {
	ticker := time.NewTicker(supersededSessionSweeperCheckInterval)
	defer ticker.Stop()

	log.Info("superseded session sweeper started", "check_interval", supersededSessionSweeperCheckInterval)

	s.sweep(ctx)

	for {
		select {
		case <-ctx.Done():
			log.Info("superseded session sweeper stopped")
			return
		case <-ticker.C:
			s.sweep(ctx)
		}
	}
}

// sweep scans every in_progress/review backlog item's session history,
// archives any tmux-backed session findSupersededSessions identifies as not
// the current round for its role (skipping any still confirmed-live), cleans
// up any current-round session findIdleStaleSessions identifies as gone quiet
// for too long (Story 6.1.1), and -- on the same tick -- flips any
// long-Pending, session-dead DiagnoseDispatch rows to Stalled (Story 6.1.5).
func (s *SupersededSessionSweeper) sweep(ctx context.Context) {
	if s.storage == nil || s.stopper == nil {
		return
	}

	items, err := s.storage.ListBacklogItems(ctx, session.BacklogItemFilter{
		Statuses: []string{string(session.BacklogStatusInProgress), string(session.BacklogStatusReview)},
	})
	if err != nil {
		log.Warn("superseded session sweeper: ListBacklogItems failed", "err", err)
		return
	}

	now := time.Now()
	for _, item := range items {
		sessions, err := s.storage.ListItemSessions(ctx, item.ID)
		if err != nil {
			log.Warn("superseded session sweeper: ListItemSessions failed", "item", item.ID, "err", err)
			continue
		}
		for _, is := range findSupersededSessions(sessions) {
			s.archiveIfNotLive(ctx, is)
		}
		for _, is := range findIdleStaleSessions(sessions, s.staleThreshold, now) {
			s.cleanupIdleStale(ctx, item.ID, is)
		}
	}

	s.sweepStalledDiagnoseDispatches(ctx)
}

// archiveIfNotLive soft-archives is unless it is still confirmed-live, in
// which case it's left alone for this tick (pitfalls.md #2: never hard-kill a
// session a user may be actively viewing or steering). ArchiveSessionByUUID
// is itself the actor-setter-routed, no-op-if-already-archived call
// (SetArchivedAtIfNilAndStop or its storage-only fallback) -- see its doc
// comment; this never performs a raw Status write.
func (s *SupersededSessionSweeper) archiveIfNotLive(ctx context.Context, is session.ItemSessionSummary) {
	if is.SessionUUID == "" {
		return
	}
	if s.stopper.IsSessionLive(is.SessionUUID) {
		return
	}
	if err := s.stopper.ArchiveSessionByUUID(ctx, is.SessionUUID); err != nil {
		log.Warn("superseded session sweeper: archive failed", "session", is.SessionUUID, "err", err)
	}
}

// cleanupIdleStale handles one findIdleStaleSessions match (Story 6.1.1):
// skips a confirmed-live session (same pitfalls.md #2 guard archiveIfNotLive
// uses above), then runs Story 6.1.2's handoff-then-cleanup step when a
// handoff generator and instance lookup are wired, falling back to a direct
// archive (no handoff summary) otherwise -- generating a summary is a
// best-effort enhancement to cleanup, never a precondition for it.
func (s *SupersededSessionSweeper) cleanupIdleStale(ctx context.Context, itemID string, is session.ItemSessionSummary) {
	if is.SessionUUID == "" {
		return
	}
	if s.stopper.IsSessionLive(is.SessionUUID) {
		return
	}
	if s.handoffGenerator == nil || s.instances == nil {
		s.archiveIfNotLive(ctx, is)
		return
	}
	inst, found := s.instances.FindInstance(is.SessionUUID)
	if !found {
		s.archiveIfNotLive(ctx, is)
		return
	}
	handoffThenCleanup(ctx, staleSessionCleanupDeps{
		generator: s.handoffGenerator,
		notes:     s.storage,
		stopper:   s.stopper,
	}, inst, itemID)
}

// sweepStalledDiagnoseDispatches implements Story 6.1.5's dispatch-stalled
// sweep on this same 60s tick: lists every Pending DiagnoseDispatch row
// across all items, flips each one findStalledDiagnoseDispatches identifies
// to Stalled, and publishes the same notifyDiagnoseEvent-adjacent live
// notification every other dispatch outcome uses (Story 5.2.2). No-op when
// dispatchStore is unwired.
func (s *SupersededSessionSweeper) sweepStalledDiagnoseDispatches(ctx context.Context) {
	if s.dispatchStore == nil {
		return
	}

	pending, err := s.dispatchStore.ListAllPending(ctx)
	if err != nil {
		log.Warn("superseded session sweeper: ListAllPending failed", "err", err)
		return
	}

	now := time.Now()
	for _, d := range findStalledDiagnoseDispatches(pending, s.stopper.IsSessionLive, s.stalledThreshold, now) {
		s.markDispatchStalled(ctx, d, now)
	}
}

// markDispatchStalled persists Story 6.1.5's Stalled transition, logs
// diagnose.dispatch.stalled, and publishes a live notification -- durability
// first, then the best-effort live-toast publish, mirroring
// notifyDiagnoseEvent's exact two-part ordering (diagnose_dispatcher.go).
func (s *SupersededSessionSweeper) markDispatchStalled(ctx context.Context, d DiagnoseDispatchRecord, now time.Time) {
	if err := s.dispatchStore.MarkStalled(ctx, d.ID); err != nil {
		log.Warn("superseded session sweeper: MarkStalled failed", "dispatch", d.ID, "item", d.ItemID, "err", err)
		return
	}

	log.Warn("diagnose.dispatch.stalled",
		"item_id", d.ItemID,
		"diagnostic_session_uuid", d.DiagnosticSessionUUID,
		"pending_duration", now.Sub(d.CreatedAt).String(),
	)

	if s.notifier == nil {
		return
	}
	itemTitle := d.ItemID
	if s.storage != nil {
		if item, err := s.storage.GetBacklogItem(ctx, d.ItemID); err == nil && item != nil {
			itemTitle = item.Title
		}
	}
	title, message, notificationType, urgent, important := DiagnoseStalledNotification(itemTitle)
	s.notifier.Notify(d.ItemID, title, message, notificationType, urgent, important)
}

// diagnoseIdleStaleThreshold is Story 6.1.1's "no progress for this long, no
// newer round exists" cleanup threshold -- reuses maxWorkSessionStaleness's
// exact magnitude (session/backlog_lifecycle_stale.go) since both answer the
// same underlying question ("has this session gone quiet for long enough
// that something should intervene") for two disjoint session populations: that
// sweep scans an in_progress item's active work session, this one scans the
// CURRENT round only (a superseded round is findSupersededSessions' job, not
// this one's -- see findIdleStaleSessions' doc comment). A plain const, not a
// live-settable flag, matching Task 6.1.5a's proportionality call for this
// phase's other reconciler thresholds.
const diagnoseIdleStaleThreshold = 2 * time.Hour

// findIdleStaleSessions returns every tmux-backed (work or review role),
// still-open (EndedAt == nil) ItemSession in sessions that IS the current
// round for its role (findMostRecentSessions' tie-break -- see
// findSupersededSessions' doc comment) but has produced no progress for
// longer than threshold, relative to now. A round a newer round already
// supersedes is left to findSupersededSessions instead -- this predicate and
// that one are deliberately disjoint (a session can appear in at most one of
// their results), so sweep() never attempts to archive the same session
// twice via two different code paths.
//
// A session's progress timestamp falls back to CreatedAt when LastProgressAt
// is unset, mirroring reconcileStaleWorkSessions' exact fallback idiom
// (session/backlog_lifecycle_stale.go) -- a session that has genuinely never
// produced output yet is "idle since it started," not exempt from staleness.
// Pure -- no I/O, no locking -- so it's table-testable without tmux/storage.
func findIdleStaleSessions(sessions []session.ItemSessionSummary, threshold time.Duration, now time.Time) []session.ItemSessionSummary {
	currentReview, currentWork := findMostRecentSessions(sessions)

	var stale []session.ItemSessionSummary
	for i := range sessions {
		is := &sessions[i]
		if !session.IsTmuxBackedSessionRole(is.Role) {
			continue
		}
		if is != currentReview && is != currentWork {
			continue
		}
		if is.EndedAt != nil {
			continue
		}
		lastProgress := is.CreatedAt
		if is.LastProgressAt != nil {
			lastProgress = *is.LastProgressAt
		}
		if now.Sub(lastProgress) > threshold {
			stale = append(stale, *is)
		}
	}
	return stale
}

// diagnoseDispatchStalledThreshold (Task 6.1.5a) bounds how long a Pending
// DiagnoseDispatch row may go with its diagnostic session confirmed dead
// before the sweep flips it to Stalled -- long enough that a genuinely
// thorough investigation isn't misdiagnosed as stalled, short enough that
// Tyler isn't staring at "Diagnosing..." for hours. A plain const, no
// live-settable config surface, matching the proportionality of this phase's
// other reconciler thresholds (e.g. triageCallBudget).
const diagnoseDispatchStalledThreshold = 30 * time.Minute

// findStalledDiagnoseDispatches returns every row in pending whose CreatedAt
// is older than threshold (relative to now) AND whose DiagnosticSessionUUID
// isLive reports as not live -- Story 6.1.5's "stopped without a completion
// signal" detection. A row still within threshold, or whose session is still
// live, is left alone: a genuinely long-running diagnosis must not be
// misdiagnosed as stalled just because it's slow. pending is assumed to
// already be status-filtered to Pending only (ListAllPending's job); this
// function never re-examines Status itself. Pure -- no I/O -- so it's
// table-testable with a fake isLive func.
func findStalledDiagnoseDispatches(pending []DiagnoseDispatchRecord, isLive func(sessionUUID string) bool, threshold time.Duration, now time.Time) []DiagnoseDispatchRecord {
	var stalled []DiagnoseDispatchRecord
	for _, d := range pending {
		if now.Sub(d.CreatedAt) <= threshold {
			continue
		}
		if isLive(d.DiagnosticSessionUUID) {
			continue
		}
		stalled = append(stalled, d)
	}
	return stalled
}
