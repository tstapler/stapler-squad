package session

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
)

// maxChainDepth caps how many hops a pipeline chain may take before ChainFirer
// refuses to fire the next link and marks the chain as terminated
// (webhook-triggers Epic 6.3 — a runaway/amplifying-fan-out backstop
// independent of the WIP-limit admission gate, see pitfalls §3). Compile-time
// constant, not config: resolved during /pm:triad-review's Engineering pass —
// no real-world usage data yet to justify per-deployment tuning (see plan.md's
// Unresolved Questions).
const maxChainDepth = 5

// maxChainWaitDuration bounds how long a chain-fire that keeps getting
// rejected by the WIP admission gate is retried before TriggerChainReconciler
// gives up (pre-mortem P2 #5). Without this ceiling, a chain stuck behind a
// saturated WIP cap (which exists precisely because saturation happens — see
// the 2026-07-12 OOM incident) would retry forever, once per 60s reconcile
// tick, accumulating pending chains without bound.
const maxChainWaitDuration = time.Hour

// chainReconcileScanLimit bounds ReconcileChains's ListBacklogItems query.
// Previously ReconcileChains relied on ListBacklogItems's default 1000-row
// safety cap (no filter pushed into SQL) and filtered NextWorkflowID/ChainFired
// in a Go loop afterward — past 1000 "done" items, a pending unfired chain
// outside that window was silently never reconciled, with no error/log signal
// (sdd:6-verify finding, AC5). Now that the query is narrowed to exactly the
// crash-recovery access pattern (status=done AND next_workflow_id IS NOT NULL
// AND chain_fired=false, backed by index.Fields("status", "chain_fired") in
// session/ent/schema/backlog_item.go), the result set is expected to stay
// small in steady state — a pending unfired chain is a transient
// crash-recovery case, not an accumulating backlog. This cap is a safety
// valve against pathological growth, not a meaningful business limit.
const chainReconcileScanLimit = 100_000

// maxInFlightChainFires bounds concurrent ChainFirer.Dispatch goroutines —
// same shape/rationale as server/services/callback_dispatcher.go's
// maxInFlightCallbacks (Task 5.2.1a): a burst of simultaneous "done"
// transitions (or a single TriggerChainReconciler tick re-dispatching many
// pending chains) must not fan out an unbounded number of goroutines, each of
// which drives an expensive CreateSession (tmux+git-worktree) call.
const maxInFlightChainFires = 20

// chainFireTimeout bounds a single ChainFirer.Fire attempt (workflow lookup +
// prompt build + FireTriggerChained's CreateSession call). Generous relative
// to a plain HTTP callback (callbackAttemptTimeout is 5s) because CreateSession
// itself provisions a tmux session and git worktree — matches createSessionTimeout's
// order of magnitude in server/services/session_service.go.
const chainFireTimeout = 5 * time.Minute

// TriggerFirer is the narrow interface ChainFirer needs to fire the next
// workflow in a pipeline chain. Defined here (consumer-defined), not in
// server/workflows, to avoid a session -> server/workflows import cycle
// (server/workflows already imports session for WorkflowRepository,
// TriggerFireEventRepository, etc. — see server/workflows/scheduler.go).
// Satisfied by *workflows.Scheduler's FireTriggerChained method — per
// the `interface-pollution-checklist` skill, this is a genuine
// cross-package boundary (unlike ChainFirer's other collaborators below,
// which live in this same package and are referenced by concrete type).
type TriggerFirer interface {
	// FireTriggerChained fires wf as the next hop in a pipeline chain:
	// priorItemSummary (built via BuildSessionInitialPrompt over the
	// just-completed item) is interpolated into wf's own prompt template, and
	// chainDepth is threaded onto the created session's
	// TriggeredByChainDepth attribution field. Returns the created session ID.
	FireTriggerChained(ctx context.Context, wf *ent.Workflow, priorItemSummary string, chainDepth int32) (sessionID string, err error)
}

// ChainFirer fires the next workflow in a pipeline chain once a BacklogItem
// reaches BacklogStatusDone with NextWorkflowID set (webhook-triggers FR10/
// AC5). Both the happy-path async dispatch
// (EntRepository.dispatchChainFire, called immediately after
// TransitionBacklogItemStatus's own DB write returns — AC9) and
// TriggerChainReconciler's restart-recovery sweep call the same Fire method,
// so "fire exactly once per item" logic lives in exactly one place.
//
// Concrete type, not an interface — repo/workflows/fireEvents are this
// package's own types (EntRepository, WorkflowRepository,
// TriggerFireEventRepository), referenced directly per
// the `interface-pollution-checklist` skill; only firer crosses a real
// package boundary and needs one (TriggerFirer, above).
type ChainFirer struct {
	repo       *EntRepository
	workflows  WorkflowRepository
	fireEvents TriggerFireEventRepository
	firer      TriggerFirer
	// cfg is nil-safe (config.Config.GetFeatureFlag tolerates a nil receiver) —
	// gates every Dispatch/ReconcileChains call on the webhook_triggers feature
	// flag, defense in depth beyond route-registration gating, mirroring
	// server/services/callback_dispatcher.go's CallbackDispatcher.Dispatch.
	cfg *config.Config

	inFlight chan struct{}
}

// NewChainFirer constructs a ChainFirer. repo is the exact *EntRepository
// instance the rest of the backlog write path uses (so ChainFirer's own
// UpdateBacklogItem calls share the same callbackDispatcher/
// itemChangePublisher wiring as every other backlog mutation) — callers
// should obtain it via Storage.WireChainFirer rather than constructing a
// second EntRepository around the same ent.Client.
func NewChainFirer(repo *EntRepository, workflows WorkflowRepository, fireEvents TriggerFireEventRepository, firer TriggerFirer, cfg *config.Config) *ChainFirer {
	return &ChainFirer{
		repo:       repo,
		workflows:  workflows,
		fireEvents: fireEvents,
		firer:      firer,
		cfg:        cfg,
		inFlight:   make(chan struct{}, maxInFlightChainFires),
	}
}

// Dispatch reserves a semaphore slot and fires item's chain in a new
// goroutine — same non-blocking shape as
// server/services/callback_dispatcher.go's CallbackDispatcher.Dispatch
// (Task 5.2.1a): a non-blocking select on a semaphore-sized channel either
// reserves a slot immediately or drops the dispatch and logs a warning. A
// dropped dispatch loses no work — ChainFired stays false, so
// TriggerChainReconciler retries the item on its next 60s tick. Never blocks
// the caller. No-op when the webhook_triggers feature flag is off (Task
// 8.2.1b — defense in depth beyond route-registration gating).
//
// Takes no ctx parameter — matching CallbackDispatcher.Dispatch's identical
// signature choice — because the spawned goroutine always derives its own
// context.WithTimeout(context.Background(), chainFireTimeout) below rather
// than propagating a caller's ctx: by the time that goroutine's
// FireTriggerChained (CreateSession) call actually runs, a caller-supplied ctx
// (e.g. an RPC handler's request-scoped context, or dispatchChainFire's
// deliberately-Background() context) may already be cancelled or long gone —
// see dispatchChainFire's doc comment (session/ent_repository_backlog.go) for
// the same rationale applied one layer up.
func (c *ChainFirer) Dispatch(item *BacklogItemData) {
	if c == nil || item == nil {
		return
	}
	if !c.cfg.GetFeatureFlag("webhook_triggers") {
		return
	}

	select {
	case c.inFlight <- struct{}{}:
	default:
		log.WarningLog().Printf("[ChainFirer] dispatch dropped, at capacity, item=%s", item.ID)
		return
	}

	itemCopy := *item
	go func() {
		defer func() { <-c.inFlight }()
		defer func() {
			if rec := recover(); rec != nil {
				log.WarningLog().Printf("[ChainFirer] Fire panicked (recovered): %v", rec)
			}
		}()
		fireCtx, cancel := context.WithTimeout(context.Background(), chainFireTimeout)
		defer cancel()
		if _, err := c.Fire(fireCtx, &itemCopy); err != nil {
			log.WarningLog().Printf("[ChainFirer] fire failed, item=%s: %v", itemCopy.ID, err)
		}
	}()
}

// Fire attempts the chain-fire for item, which must already have
// NextWorkflowID set (callers filter on ChainFired == false before calling —
// Fire itself re-checks both as a defense-in-depth no-op guard). Returns
// fired=true only when a new session was actually created.
//
// Double-fire race (webhook-triggers Task 6.2.1d): EntRepository's happy-path
// dispatch (right after TransitionBacklogItemStatus's done write) and
// TriggerChainReconciler's periodic sweep can both observe the same item with
// ChainFired==false and call Fire concurrently — a genuine risk once a chain
// sits pending for even one 60s tick. Fire closes this the same way this
// package already closed an identically-shaped TOCTOU race in
// TransitionBacklogItemStatus itself (see that function's doc comment,
// BUG-026): claimIfUnfired below performs a genuine SQL-level compare-and-
// swap (ChainFired=true, WHERE updated_at = <the value item was read at>)
// *before* calling TriggerFirer.FireTriggerChained, not after. Exactly one
// concurrent caller's claim UPDATE affects a row; every other caller's claim
// affects zero rows (ErrPreconditionFailed) and backs off without ever
// reaching FireTriggerChained — so at most one goroutine can ever call
// CreateSession for a given item's chain.
func (c *ChainFirer) Fire(ctx context.Context, item *BacklogItemData) (fired bool, err error) {
	if item == nil || item.NextWorkflowID == nil || item.ChainFired {
		return false, nil
	}

	if item.TriggeredByChainDepth >= maxChainDepth {
		log.WarningLog().Printf("[ChainFirer] chain depth exceeded for item %s (depth=%d, max=%d)", item.ID, item.TriggeredByChainDepth, maxChainDepth)
		if !c.claimIfUnfired(ctx, item) {
			return false, nil // another goroutine already claimed/handled this item
		}
		c.recordFireEvent(ctx, item.NextWorkflowID, "fired_failed", "chain depth exceeded")
		return false, nil
	}

	if item.ChainedAt != nil && time.Since(*item.ChainedAt) > maxChainWaitDuration {
		log.WarningLog().Printf("[ChainFirer] chain expired waiting for WIP capacity, item=%s (chained_at=%s)", item.ID, item.ChainedAt)
		if !c.claimIfUnfired(ctx, item) {
			return false, nil
		}
		c.recordFireEvent(ctx, item.NextWorkflowID, "fired_failed", "chain expired waiting for WIP capacity")
		return false, nil
	}

	wf, err := c.workflows.GetByID(ctx, *item.NextWorkflowID)
	if err != nil {
		// Left as a retryable failure (ChainFired stays false) rather than an
		// immediate give-up: a workflow lookup failure here could be transient
		// (e.g. a momentary DB hiccup), and maxChainWaitDuration above already
		// bounds how long a permanently-missing workflow (e.g. deleted after
		// the chain was configured) gets retried before the reconciler gives
		// up on its own. No claim attempted — nothing to revert.
		return false, fmt.Errorf("get next workflow %s for item %s: %w", *item.NextWorkflowID, item.ID, err)
	}

	var priorSessions []ItemSessionSummary
	if c.repo != nil {
		priorSessions, err = c.repo.ListItemSessions(ctx, item.ID)
		if err != nil {
			log.WarningLog().Printf("[ChainFirer] ListItemSessions failed for item %s: %v", item.ID, err)
			priorSessions = nil // BuildSessionInitialPrompt handles nil priorSessions fine
		}
	}
	priorItemSummary := BuildSessionInitialPrompt(item, priorSessions)
	chainDepth := int32(item.TriggeredByChainDepth + 1) //nolint:gosec // bounded by maxChainDepth above

	// Claim BEFORE firing (see doc comment above) — must happen after the
	// depth/expiry checks (which read item fields only) but before the one
	// call with a real, non-idempotent side effect (CreateSession via
	// FireTriggerChained).
	if !c.claimIfUnfired(ctx, item) {
		return false, nil // lost the claim race to another concurrent Fire call
	}

	sessionID, fireErr := c.firer.FireTriggerChained(ctx, wf, priorItemSummary, chainDepth)
	if fireErr != nil {
		// Revert the claim so TriggerChainReconciler retries on its next tick
		// (bounded by maxChainWaitDuration). Safe unconditionally (no
		// precondition needed): only the goroutine that just won the claim
		// above can reach this line, so nothing else can be racing this
		// specific revert.
		c.revertClaim(ctx, item.ID)
		return false, fmt.Errorf("fire chained workflow %s for item %s: %w", wf.ID, item.ID, fireErr)
	}

	log.InfoLog().Printf("[ChainFirer] fired chain for item %s -> workflow %s (session %s, depth %d)", item.ID, wf.Slug, sessionID, chainDepth)
	return true, nil
}

// claimIfUnfired atomically flips item's ChainFired flag from false to true
// via EntRepository.ClaimChainFire — a genuine SQL-level CAS, conditioned on
// item.UpdatedAt still matching the value item was read/copied at. Returns
// true iff this call won the claim. A lost claim is the expected, common
// outcome of the double-fire race described on Fire's doc comment, not an
// error — callers must not treat it as one.
func (c *ChainFirer) claimIfUnfired(ctx context.Context, item *BacklogItemData) bool {
	if c.repo == nil {
		return false
	}
	claimed, err := c.repo.ClaimChainFire(ctx, item.ID, item.UpdatedAt)
	if err != nil {
		log.WarningLog().Printf("[ChainFirer] claim attempt failed for item %s: %v", item.ID, err)
		return false
	}
	return claimed
}

// revertClaim resets ChainFired back to false after a claim winner's
// subsequent FireTriggerChained call failed — see Fire's doc comment for why
// no precondition is needed here.
func (c *ChainFirer) revertClaim(ctx context.Context, itemID string) {
	if c.repo == nil {
		return
	}
	if err := c.repo.RevertChainFireClaim(ctx, itemID); err != nil {
		log.WarningLog().Printf("[ChainFirer] failed to revert claim for item %s: %v", itemID, err)
	}
}

// recordFireEvent persists a TriggerFireEvent audit row for a chain-fire
// outcome ChainFirer itself rejects before ever calling
// TriggerFirer.FireTriggerChained (depth-cap / expired-wait) — mirrors
// Scheduler.recordFireEvent's shape (server/workflows/scheduler.go), best-
// effort: a failed audit write is logged, never propagated.
func (c *ChainFirer) recordFireEvent(ctx context.Context, workflowID *uuid.UUID, outcome, errMsg string) {
	if c.fireEvents == nil {
		return
	}
	if err := c.fireEvents.Create(ctx, TriggerFireEventInput{
		WorkflowID:   workflowID,
		Outcome:      outcome,
		ErrorMessage: errMsg,
	}); err != nil {
		log.WarningLog().Printf("[ChainFirer] failed to record fire event: %v", err)
	}
}

// TriggerChainReconciler completes pipeline chain-fires interrupted by a
// crash between the "done" transition committing
// (EntRepository.dispatchChainFire) and the chained session actually being
// created — AC5's restart-recovery scenario. ReconcileChains is invoked from
// BacklogLifecycleListener's existing 60s reconcile tick (ReconcileStuck),
// mirroring reconcileStaleWorkSessions' idiom exactly: no dedicated
// goroutine/ticker of its own (plan.md's explicit instruction — reuse the
// existing ticker infra, don't create a second one).
type TriggerChainReconciler struct {
	firer *ChainFirer
}

// NewTriggerChainReconciler constructs a TriggerChainReconciler bound to
// firer — the same ChainFirer instance wired as the happy-path dispatcher
// (EntRepository.SetChainFirer), so both paths share one semaphore and one
// "fire exactly once" implementation (ChainFirer.Fire).
func NewTriggerChainReconciler(firer *ChainFirer) *TriggerChainReconciler {
	return &TriggerChainReconciler{firer: firer}
}

// ReconcileChains scans done items for a pending, unfired chain
// (NextWorkflowID != nil && !ChainFired) and re-dispatches each through
// ChainFirer.Dispatch — the same async, semaphore-bounded path the happy-path
// caller uses, so a tick with many pending chains can't block the reconcile
// sweep itself on a burst of expensive CreateSession calls.
func (r *TriggerChainReconciler) ReconcileChains(ctx context.Context, er *EntRepository) {
	if r == nil || r.firer == nil || er == nil {
		return
	}
	if !r.firer.cfg.GetFeatureFlag("webhook_triggers") {
		// Skip the list query entirely when the feature is off (Task 8.2.1b) —
		// Dispatch below re-checks the same flag anyway, but there's no reason
		// to pay for the scan when every Dispatch call would immediately no-op.
		return
	}
	chainFired := false
	nextWorkflowSet := true
	items, err := er.ListBacklogItems(ctx, BacklogItemFilter{
		Statuses:          []string{string(BacklogStatusDone)},
		ChainFired:        &chainFired,
		NextWorkflowIDSet: &nextWorkflowSet,
		Limit:             chainReconcileScanLimit,
	})
	if err != nil {
		log.WarningLog().Printf("[TriggerChainReconciler] list error: %v", err)
		return
	}
	for i := range items {
		item := items[i]
		// Defense-in-depth only: the query above already restricts to
		// next_workflow_id IS NOT NULL AND chain_fired=false, so this should
		// never trigger — kept in case a future filter regression reintroduces
		// the bug this replaced (the old code relied on this Go-side check as
		// its ONLY filter, past a 1000-row cap that silently dropped items).
		if item.NextWorkflowID == nil || item.ChainFired {
			continue
		}
		r.firer.Dispatch(&item)
	}
}
