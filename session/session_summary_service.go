package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/sessionsummary"
	"github.com/tstapler/stapler-squad/session/headless"
	"github.com/tstapler/stapler-squad/session/tokens"
)

// narrativeFallbackTrivial is substituted for the narrative when isTrivialSession
// skips the LLM call entirely (FR-5/FR-6/cost-control).
const narrativeFallbackTrivial = "This session ended before any work was recorded."

// narrativeFallbackLLMFailure is substituted for the narrative when
// GenerateSessionCompletionNarrative fails or times out — the pipeline still
// reaches READY rather than aborting (FR-5's graceful degradation).
const narrativeFallbackLLMFailure = "This session made changes across the files listed in Changes below; a narrative summary could not be generated."

// llmNarrativeTimeout bounds the GenerateSessionCompletionNarrative call
// (research/pitfalls.md §3's "no explicit LLM timeout" gap). Declared as a var
// (not const) so integration tests can temporarily lower it to exercise the
// timeout path without waiting the full 60s.
var llmNarrativeTimeout = 60 * time.Second

// staleGenerationTimeout is the read-time staleness threshold: a row stuck in
// GENERATING older than this is treated as interrupted (e.g. a server restart)
// and flipped to ERROR on next read, unless the in-memory guard is still held by
// this process (see ReconcileStaleness).
const staleGenerationTimeout = 5 * time.Minute

// regenerateCooldown rate-limits repeated manual-regenerate clicks, independent of
// the in-flight dedup guard (research/pitfalls.md §7).
const regenerateCooldown = 30 * time.Second

// reasonManualRegenerate is the reason value RegenerateSessionSummary (Phase 2)
// passes to GenerateAndPersist to bypass the sequential-duplicate short-circuit and
// apply the cooldown check instead.
const reasonManualRegenerate = "manual-regenerate"

// SessionSummaryGenerator is the domain-level orchestrator that owns the headless
// pool, the ent client, and the FR-7 in-process dedup map. GenerateAndPersist runs
// the full async pipeline (assemble deterministic snapshots, call the LLM for a
// narrative, render markdown, persist) and satisfies the summaryGenerator interface
// consumed by sessionSummaryListener structurally — no explicit "implements"
// declaration needed.
type SessionSummaryGenerator struct {
	entClient    *ent.Client
	pool         headless.PoolClient
	reviewLookup ReviewQueueLookup

	// lateBindMu guards notifLister/tokenStore, mirroring
	// BacklogLifecycleListener's poolMu/sessionCreatorMu convention
	// (session/backlog_lifecycle.go) for fields wired post-construction.
	// server/dependencies.go calls WireSessionSummaryListener on every existing
	// Instance at construction time, but SetNotificationLister/SetTokenStore
	// aren't called until later during server startup — a lifecycle event
	// firing in that window dispatches GenerateAndPersist as a goroutine that
	// races the field writes without this lock.
	lateBindMu  sync.RWMutex
	notifLister NotificationDecisionLister
	tokenStore  tokens.TokenStoreReader

	// inFlight is a sync.Map[string]*sync.Mutex keyed by session_id, mirroring
	// UnfinishedWorkService.aiMu (server/services/unfinished_work_service.go).
	// Collapses concurrent/duplicate triggers per session.
	inFlight sync.Map
}

// compile-time check that *SessionSummaryGenerator satisfies summaryGenerator
// (session/session_summary_listener.go) structurally — no explicit "implements"
// declaration needed, per the `interface-pollution-checklist` skill.
var _ summaryGenerator = (*SessionSummaryGenerator)(nil)

// NewSessionSummaryGenerator creates a SessionSummaryGenerator. notifLister and
// reviewLookup may be nil in a degraded/partial deployment — BuildDecisionsSnapshot
// nil-checks both.
func NewSessionSummaryGenerator(entClient *ent.Client, pool headless.PoolClient, notifLister NotificationDecisionLister, tokenStore tokens.TokenStoreReader, reviewLookup ReviewQueueLookup) *SessionSummaryGenerator {
	return &SessionSummaryGenerator{
		entClient:    entClient,
		pool:         pool,
		notifLister:  notifLister,
		tokenStore:   tokenStore,
		reviewLookup: reviewLookup,
	}
}

// FindRowBySessionID queries the SessionSummary row for sessionID directly — never
// via the Session-keyed live-instance machinery (AC-3: a summary must remain
// retrievable after its Session row is gone). Wraps ent's not-found error as
// ErrNotFound so callers in server/services (which must not import session/ent's
// error-handling helpers directly — see .golangci.yml's no_ent_in_services/forbidigo
// rules) can check it with errors.Is instead.
func (g *SessionSummaryGenerator) FindRowBySessionID(ctx context.Context, sessionID string) (*ent.SessionSummary, error) {
	row, err := g.entClient.SessionSummary.Query().Where(sessionsummary.SessionID(sessionID)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: session summary for session=%s", ErrNotFound, sessionID)
		}
		return nil, err
	}
	return row, nil
}

// SetNotificationLister wires notifLister after construction. Needed because
// server/dependencies.go constructs SessionSummaryGenerator early (alongside the
// headless pool, so it can be wired to every instance in the same loop that wires
// backlogLifecycleListener) but the NotificationHistoryStore it needs isn't built
// until later, in server.go's RunServer — the same "Set* called long after
// construction" ordering constraint documented on SessionService.SetHeadlessPool.
// Safe to call with nil; BuildDecisionsSnapshot nil-checks notifLister. Guarded by
// lateBindMu because GenerateAndPersist (always dispatched via a goroutine) reads
// notifLister concurrently with this call.
func (g *SessionSummaryGenerator) SetNotificationLister(l NotificationDecisionLister) {
	g.lateBindMu.Lock()
	defer g.lateBindMu.Unlock()
	g.notifLister = l
}

// SetTokenStore wires tokenStore after construction, for the same reason and
// timing as SetNotificationLister — the token store is also constructed after
// SessionSummaryGenerator during server startup. Safe to call with nil;
// BuildCostSnapshot nil-checks tokenStore. Guarded by lateBindMu because
// GenerateAndPersist (always dispatched via a goroutine) reads tokenStore
// concurrently with this call.
func (g *SessionSummaryGenerator) SetTokenStore(t tokens.TokenStoreReader) {
	g.lateBindMu.Lock()
	defer g.lateBindMu.Unlock()
	g.tokenStore = t
}

// notificationLister returns the currently-wired notifLister under lateBindMu's
// read lock, for use inside GenerateAndPersist.
func (g *SessionSummaryGenerator) notificationLister() NotificationDecisionLister {
	g.lateBindMu.RLock()
	defer g.lateBindMu.RUnlock()
	return g.notifLister
}

// currentTokenStore returns the currently-wired tokenStore under lateBindMu's read
// lock, for use inside GenerateAndPersist.
func (g *SessionSummaryGenerator) currentTokenStore() tokens.TokenStoreReader {
	g.lateBindMu.RLock()
	defer g.lateBindMu.RUnlock()
	return g.tokenStore
}

// tryAcquire attempts to acquire the in-process per-session guard for sessionUUID.
// Returns (release, true) on success — the caller must call release() exactly once,
// typically via defer. Returns (nil, false) if a generation is already in flight for
// this session.
func (g *SessionSummaryGenerator) tryAcquire(sessionUUID string) (release func(), ok bool) {
	muI, _ := g.inFlight.LoadOrStore(sessionUUID, &sync.Mutex{})
	m := muI.(*sync.Mutex)
	if !m.TryLock() {
		return nil, false
	}
	return func() {
		m.Unlock()
		// Remove the map entry once this generation is done, so a session that
		// triggers generation once doesn't leave a permanent *sync.Mutex behind
		// for the process's lifetime (unbounded growth over the server's life).
		// CompareAndDelete only removes it if it's still the same mutex we
		// acquired — if a concurrent tryAcquire already raced in a fresh
		// LoadOrStore between our Unlock and this line, that entry belongs to a
		// different in-flight generation and must be left alone.
		g.inFlight.CompareAndDelete(sessionUUID, m)
	}, true
}

// isInFlight is a non-blocking probe used by ReconcileStaleness to distinguish
// "still actively generating in this process" from "genuinely stuck (e.g. the
// process restarted mid-generation)".
func (g *SessionSummaryGenerator) isInFlight(sessionUUID string) bool {
	muI, ok := g.inFlight.Load(sessionUUID)
	if !ok {
		return false
	}
	m := muI.(*sync.Mutex)
	if m.TryLock() {
		m.Unlock()
		return false
	}
	return true
}

// ReconcileStaleness flips a row stuck in GENERATING for longer than
// staleGenerationTimeout to ERROR, unless this process's in-memory guard is still
// held for that session (a long-running call, not a genuinely stuck row). Called
// from the RPC read path (Phase 2's GetSessionSummary), not a background sweep —
// see plan.md's Pattern Decisions "FR-7 restart-survival dedup" row for the
// accepted v1 gap (a never-revisited session stays stuck in GENERATING forever).
// Exported so server/services.SessionSummaryService (a different package) can call
// it directly from the RPC handler.
func (g *SessionSummaryGenerator) ReconcileStaleness(ctx context.Context, row *ent.SessionSummary) *ent.SessionSummary {
	if row == nil || row.Status != string(SessionSummaryStatusGenerating) || row.GenerationStartedAt == nil {
		return row
	}
	if time.Since(*row.GenerationStartedAt) <= staleGenerationTimeout {
		return row
	}
	if g.isInFlight(row.SessionID) {
		return row
	}

	const restartInterruptedMessage = "generation did not complete, possibly due to a server restart"
	// Predicated bulk update instead of UpdateOne(row): a fresh GenerateAndPersist
	// call can acquire the in-flight guard and write a READY row in the narrow
	// window between the isInFlight check above and this write landing — an
	// unconditional UpdateOne(row) would stomp that fresh READY row back to
	// ERROR (TOCTOU). Condition the write on the row still being in exactly the
	// state we read (same status, same generation_started_at); if 0 rows match,
	// someone else already transitioned it and this is a no-op, not an error.
	affected, err := g.entClient.SessionSummary.Update().
		Where(
			sessionsummary.SessionID(row.SessionID),
			sessionsummary.Status(string(SessionSummaryStatusGenerating)),
			sessionsummary.GenerationStartedAtEQ(*row.GenerationStartedAt),
		).
		SetStatus(string(SessionSummaryStatusError)).
		SetErrorStage("restart-interrupted").
		SetErrorMessage(restartInterruptedMessage).
		Save(ctx)
	if err != nil {
		log.ForSession(row.SessionID).Warn("[SessionSummary] failed to reconcile stale GENERATING row", "err", err)
		return row
	}
	if affected == 0 {
		// Someone else (a fresh GenerateAndPersist call, or a concurrent
		// reconcile) already transitioned this row out of the state we read.
		// Return the row as originally read, not a freshly-error'd one.
		log.ForSession(row.SessionID).Info("[SessionSummary] skipped stale-GENERATING reconcile: row already transitioned")
		return row
	}

	updated, err := g.entClient.SessionSummary.Query().Where(sessionsummary.SessionID(row.SessionID)).Only(ctx)
	if err != nil {
		log.ForSession(row.SessionID).Warn("[SessionSummary] reconciled stale GENERATING row but failed to re-read it", "err", err)
		return row
	}
	log.ForSession(row.SessionID).Warn("[SessionSummary] reconciled stale GENERATING row to ERROR", "generation_started_at", row.GenerationStartedAt)
	return updated
}

// GenerateAndPersist runs the full session-completion-summary pipeline: build
// deterministic snapshots, generate (or skip/fallback) a narrative, render markdown,
// and persist via a single final status-transitioning upsert. Always invoked as a
// detached goroutine (`go g.GenerateAndPersist(...)`) by sessionSummaryListener or
// (Phase 2) RegenerateSessionSummary — never call this synchronously.
//
// diff/diffContent/sessionGoal are synchronous, in-memory-only reads captured by the
// caller at dispatch time (no I/O) — see sessionSummaryListener.OnLifecycleEvent.
// diff is the already-derived DiffSnapshot (callers that have a live *git.DiffStats
// build it via BuildDiffSnapshot; RegenerateSessionSummary's no-live-instance
// fallback builds it directly from the persisted row's diff_* columns, since
// BuildDiffSnapshot's FilesChanged derivation depends on diff Content, which isn't
// persisted). diffContent is the raw diff text forwarded into the LLM narrative
// prompt — empty when unavailable (e.g. the no-live-instance fallback).
func (g *SessionSummaryGenerator) GenerateAndPersist(ctx context.Context, sessionUUID, sessionTitle string, createdAt time.Time, diff DiffSnapshot, diffContent string, sessionGoal *SessionGoalData, reason string) {
	release, ok := g.tryAcquire(sessionUUID)
	if !ok {
		return
	}
	defer release()

	// Panic safety (mirrors BacklogLifecycleListener.runStuckDetector,
	// session/backlog_lifecycle.go): this method always runs as a detached
	// goroutine, so an unrecovered panic here would crash the entire server
	// process, taking every live tmux session down with it. Go runs deferred
	// functions LIFO, so this recover (registered second) fires before
	// release() (registered first) — the guard is never left permanently
	// locked by a panic.
	defer func() {
		if r := recover(); r != nil {
			log.WarningLog().Printf("[SessionSummary] GenerateAndPersist panicked (recovered) for session=%s: %v", sessionUUID, r)
		}
	}()

	dispatchTime := time.Now()
	incomingDiff := diff

	existingRow, err := g.entClient.SessionSummary.Query().Where(sessionsummary.SessionID(sessionUUID)).Only(ctx)
	if err != nil {
		if !ent.IsNotFound(err) {
			log.ForSession(sessionUUID).Warn("[SessionSummary] failed to query existing row", "err", err)
			return
		}
		existingRow = nil
	}

	if reason == reasonManualRegenerate {
		// Cooldown check (Task 1.5.3c): independent of the in-flight guard —
		// rate-limits rapid repeated Regenerate clicks.
		if existingRow != nil && existingRow.GeneratedAt != nil && time.Since(*existingRow.GeneratedAt) < regenerateCooldown {
			return
		}
	} else if existingRow != nil && existingRow.Status == string(SessionSummaryStatusReady) {
		// Sequential-duplicate short-circuit (Task 1.5.2b step 1b, closes
		// pre-mortem P1 #5): only short-circuit if this generation's diff data
		// is unchanged from the persisted row — a resumed session that did
		// substantial new work before exiting again must NOT be short-circuited.
		if incomingDiff.FilesChanged == existingRow.DiffFilesChanged &&
			incomingDiff.Added == existingRow.DiffAdded &&
			incomingDiff.Removed == existingRow.DiffRemoved {
			return
		}
	}

	now := time.Now()

	// Interim GENERATING upsert. Deliberately field-scoped (NOT UpdateNewValues())
	// — session/ent/sessionsummary_create.go's generated defaults() method applies
	// every Default()-bearing field (diff_*, decisions_*, narrative_fallback_used,
	// cost_data_unavailable) to the mutation even when not explicitly Set(), so
	// UpdateNewValues() here would reset those columns to their zero values on
	// every re-trigger — clobbering a prior successful generation's diff/decisions
	// counts while this new generation is still in flight, which would violate the
	// "never clears a prior successful generation's fields" requirement (this
	// file's own Pattern Decisions "Error-path field preservation" row) before the
	// error path even runs. This deviates from plan.md Task 1.5.2b's literal "same
	// shape as step 6" wording for that reason.
	if err := g.entClient.SessionSummary.Create().
		SetID(uuid.New().String()).
		SetSessionID(sessionUUID).
		SetSessionTitle(sessionTitle).
		SetStatus(string(SessionSummaryStatusGenerating)).
		SetGenerationStartedAt(now).
		OnConflictColumns(sessionsummary.FieldSessionID).
		Update(func(u *ent.SessionSummaryUpsert) {
			u.SetSessionTitle(sessionTitle)
			u.SetStatus(string(SessionSummaryStatusGenerating))
			u.SetGenerationStartedAt(now)
		}).
		Exec(ctx); err != nil {
		log.ForSession(sessionUUID).Error("[SessionSummary] failed to write interim GENERATING row", "err", err)
		return
	}
	log.ForSession(sessionUUID).Info("[SessionSummary] PENDING -> GENERATING", "reason", reason)

	timeline := BuildTimelineSnapshot(createdAt, dispatchTime)

	decisions, err := BuildDecisionsSnapshot(ctx, sessionUUID, g.notificationLister(), g.reviewLookup)
	if err != nil {
		if upsertErr := g.entClient.SessionSummary.Create().
			SetID(uuid.New().String()).
			SetSessionID(sessionUUID).
			SetSessionTitle(sessionTitle).
			SetStatus(string(SessionSummaryStatusError)).
			SetGenerationStartedAt(now).
			OnConflictColumns(sessionsummary.FieldSessionID).
			Update(func(u *ent.SessionSummaryUpsert) {
				u.SetStatus(string(SessionSummaryStatusError))
				u.SetErrorStage("decisions")
				u.SetErrorMessage(err.Error())
				u.SetGenerationStartedAt(now)
				u.SetDiffFilesChanged(incomingDiff.FilesChanged)
				u.SetDiffAdded(incomingDiff.Added)
				u.SetDiffRemoved(incomingDiff.Removed)
				u.SetSessionStartedAt(timeline.StartedAt)
				u.SetSessionStoppedAt(timeline.StoppedAt)
				u.SetDurationMs(timeline.Duration().Milliseconds())
			}).
			Exec(ctx); upsertErr != nil {
			log.ForSession(sessionUUID).Error("[SessionSummary] failed to persist decisions-stage error row", "err", upsertErr)
		}
		log.ForSession(sessionUUID).Warn("[SessionSummary] GENERATING -> ERROR (decisions stage)", "err", err)
		return
	}

	cost := BuildCostSnapshot(sessionUUID, g.currentTokenStore())

	var narrative string
	var fallbackUsed bool
	if isTrivialSession(incomingDiff, decisions, timeline.Duration()) {
		narrative = narrativeFallbackTrivial
		fallbackUsed = true
		log.ForSession(sessionUUID).Info("[SessionSummary] skipping LLM narrative call: trivial session")
	} else {
		goalText := ""
		if sessionGoal != nil {
			goalText = sessionGoal.Goal
		}
		narrCtx, cancel := context.WithTimeout(ctx, llmNarrativeTimeout)
		text, narrCostUSD, narrErr := headless.GenerateSessionCompletionNarrative(narrCtx, g.pool, sessionTitle, goalText, diffContent, formatDecisionsSummary(decisions))
		cancel()
		// Folded into the same EstimatedCostUSD the token-usage-log snapshot above
		// populates: this narrative call is a separate cost channel (a headless pool
		// call, not part of the interactive session's own JSONL token log) that would
		// otherwise never be counted anywhere.
		cost.EstimatedCostUSD += narrCostUSD
		if narrErr != nil {
			log.ForSession(sessionUUID).Warn("[SessionSummary] narrative generation failed, using fallback", "err", narrErr)
			narrative = narrativeFallbackLLMFailure
			fallbackUsed = true
		} else {
			narrative = text
		}
	}

	diffLink := fmt.Sprintf("/sessions/%s/summary", sessionUUID)
	markdown := RenderSessionSummaryMarkdown(sessionTitle, narrative, fallbackUsed, incomingDiff, decisions, timeline, cost, diffLink)

	finalNow := time.Now()
	create := g.entClient.SessionSummary.Create().
		SetID(uuid.New().String()).
		SetSessionID(sessionUUID).
		SetSessionTitle(sessionTitle).
		SetStatus(string(SessionSummaryStatusReady)).
		SetNarrative(narrative).
		SetNarrativeFallbackUsed(fallbackUsed).
		SetDiffFilesChanged(incomingDiff.FilesChanged).
		SetDiffAdded(incomingDiff.Added).
		SetDiffRemoved(incomingDiff.Removed).
		SetDecisionsAutoApproved(decisions.AutoApproved).
		SetDecisionsManuallyApproved(decisions.ManuallyApproved).
		SetDecisionsDenied(decisions.Denied).
		SetDecisionsReviewQueueResolved(decisions.ReviewQueueResolved).
		SetDecisionsStillOpen(decisions.StillOpen).
		SetSessionStartedAt(timeline.StartedAt).
		SetSessionStoppedAt(timeline.StoppedAt).
		SetDurationMs(timeline.Duration().Milliseconds()).
		SetCostDataUnavailable(cost.DataUnavailable).
		SetMarkdown(markdown).
		SetGenerationStartedAt(now).
		SetGeneratedAt(finalNow)
	if !cost.DataUnavailable {
		create = create.SetTotalTokens(cost.TotalTokens).SetEstimatedCostUsd(cost.EstimatedCostUSD)
	}

	upsert := create.OnConflictColumns(sessionsummary.FieldSessionID).UpdateNewValues().
		Update(func(u *ent.SessionSummaryUpsert) {
			// error_message/error_stage are Optional() fields with no Default(),
			// so UpdateNewValues() (which only sets columns explicitly Set() on
			// the create mutation) never clears them here. A prior failed
			// generation for this session may have left both set — unconditionally
			// clear them so a successful (READY) row never carries forward stale
			// error state from an earlier failure.
			u.ClearErrorMessage()
			u.ClearErrorStage()
			if cost.DataUnavailable {
				// UpdateNewValues() only sets columns explicitly Set() on the
				// create mutation — total_tokens/estimated_cost_usd are skipped
				// above when cost data is unavailable, which otherwise leaves a
				// prior generation's stale values in place alongside
				// cost_data_unavailable=true. Explicitly clear them so the two
				// columns stay consistent with the unavailable flag.
				u.ClearTotalTokens()
				u.ClearEstimatedCostUsd()
			}
		})

	if err := upsert.Exec(ctx); err != nil {
		// LLM cost already spent, row still says GENERATING — attempt a
		// best-effort fallback write rather than silently leaving it stuck
		// (research/pitfalls.md's concern about a misleading "possibly due to
		// a server restart" message papering over a real persist failure).
		if fallbackErr := g.entClient.SessionSummary.Create().
			SetID(uuid.New().String()).
			SetSessionID(sessionUUID).
			SetSessionTitle(sessionTitle).
			SetStatus(string(SessionSummaryStatusError)).
			SetGenerationStartedAt(now).
			OnConflictColumns(sessionsummary.FieldSessionID).
			Update(func(u *ent.SessionSummaryUpsert) {
				u.SetStatus(string(SessionSummaryStatusError))
				u.SetErrorStage("persist")
				u.SetErrorMessage(err.Error())
			}).
			Exec(ctx); fallbackErr != nil {
			log.ForSession(sessionUUID).Error("[SessionSummary] failed to persist fallback ERROR row after final write failure", "err", fallbackErr)
		}
		log.ForSession(sessionUUID).Error("[SessionSummary] GENERATING -> ERROR (persist stage)", "err", err)
		return
	}

	log.ForSession(sessionUUID).Info("[SessionSummary] GENERATING -> READY", "fallback_used", fallbackUsed)
}

// formatDecisionsSummary renders a plain-text summary of decisions for the
// narrative LLM prompt (GenerateSessionCompletionNarrative's decisionsSummary
// input).
func formatDecisionsSummary(d DecisionsSnapshot) string {
	if d.Total() == 0 {
		return "No approval decisions were recorded."
	}
	return fmt.Sprintf("%d auto-approved, %d manually approved, %d denied, %d review-queue items resolved, %d still open",
		d.AutoApproved, d.ManuallyApproved, d.Denied, d.ReviewQueueResolved, d.StillOpen)
}
