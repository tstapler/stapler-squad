package backlog

import (
	"context"

	"github.com/tstapler/stapler-squad/session"
)

// StuckDetector is one panic-isolated sweep of ReconcileStuck. A new stuck
// condition is a new detector added to defaultStuckDetectors; the ReconcileStuck loop and runStuckDetector stay closed.
type StuckDetector interface {
	Name() string
	Run(ctx context.Context, er *session.EntRepository)
}

type funcStuckDetector struct {
	name string
	run  func(ctx context.Context, er *session.EntRepository)
}

func (d funcStuckDetector) Name() string { return d.name }

func (d funcStuckDetector) Run(ctx context.Context, er *session.EntRepository) { d.run(ctx, er) }

// defaultStuckDetectors is the ordered detector list. Order is load-bearing and
// pinned by TestDefaultStuckDetectors_Order.
func (l *BacklogLifecycleListener) defaultStuckDetectors() []StuckDetector {
	return []StuckDetector{
		// Re-drive recorded transition denials first, so an approved gate releases
		// its item before the spawn-point guards below look at it (Story 1.5.8).
		funcStuckDetector{"transition_denied_redrive", func(ctx context.Context, _ *session.EntRepository) {
			l.storage.RedriveTransitionDenials(ctx)
		}},
		// Re-read each live work session's actual HEAD into its ItemSession row, so
		// LastCommitSha means "the session's latest commit" rather than the value it
		// was seeded with at spawn. Registered first so every detector below in this
		// same tick — notably pr_ready+merge_detection's closeIfSupersededByMain —
		// sees fresh data rather than lagging a full tick behind (BUG-047).
		funcStuckDetector{"work_commit_refresh", func(ctx context.Context, _ *session.EntRepository) {
			l.refreshWorkSessionGitActivity(ctx)
		}},
		// Flag in_progress work sessions that have gone quiet for too long. Detection +
		// notification only — a slow-but-alive agent should not be force-stopped.
		funcStuckDetector{"stale_work", func(ctx context.Context, er *session.EntRepository) {
			l.reconcileStaleWorkSessions(ctx, er)
		}},
		// Resolve-only pass for rework_blocked_stale: the mark side
		// (notifyIfActiveWorkSessionStale) has no periodic tick of its own, so
		// this closes an open row once its blocking session recovers, ends, or
		// the item leaves review, mirroring reconcileStaleWorkSessions' own
		// resolve half for the structurally similar in_progress case.
		funcStuckDetector{"rework_blocked_stale", func(ctx context.Context, er *session.EntRepository) {
			l.reconcileReworkBlockedStaleResolution(ctx, er)
		}},
		// Resolve-only pass for respawn_blocked_active: load-bearing (not merely
		// convenient) for AutoRespawnReview, whose only caller gates the respawn
		// behind a backoff that eventually parks and stops re-invoking it — see
		// reconcileRespawnBlockedActiveResolution's doc comment for the full
		// orphaned-row scenario this closes.
		funcStuckDetector{"respawn_blocked_active", func(ctx context.Context, er *session.EntRepository) {
			l.reconcileRespawnBlockedActiveResolution(ctx, er)
		}},
		// Flag review-status items that already have a review verdict but nothing
		// active in flight (AutoReopenAfterFailedReview's spawn failed and rolled
		// back, or a legacy review session exited without a verdict), plus
		// zombie-session review items (pre-mortem F3). Detection + notification
		// only, same rationale as reconcileStaleWorkSessions above: these items
		// are otherwise invisible to every other reconciler.
		funcStuckDetector{"abandoned_review", func(ctx context.Context, er *session.EntRepository) {
			l.reconcileStuckReviewItems(ctx, er)
		}},
		// Reconciliation backstop (Epic 3.2, "PR Metadata Capture Fix"): review-status
		// items with no live session but a real, unreported GitHub PR for their
		// branch — an agent that shipped via /backlog:ship but crashed before calling
		// report_pr_created. Self-heals immediately on a match; see
		// reconcileOrphanedAgentPRs' doc comment for why this is a backstop, not a
		// new StuckReason.
		funcStuckDetector{"orphaned_agent_pr", func(ctx context.Context, er *session.EntRepository) {
			l.reconcileOrphanedAgentPRs(ctx, er)
		}},
		// Apply a review verdict that was recorded but never actioned because the
		// review session died before its exit event fired — distinct from the
		// zombie detection above (see reconcileUnprocessedReviewVerdicts doc comment).
		funcStuckDetector{"unprocessed_review_verdict", func(ctx context.Context, er *session.EntRepository) {
			l.reconcileUnprocessedReviewVerdicts(ctx, er)
		}},
		// Bouncing (non-converging in_progress<->review cycle) detector — wired
		// before merge detection per Task 2.1.4b so a panic here can't skip it
		// (also guarded by its own recover() below regardless of ordering).
		funcStuckDetector{"bouncing", func(ctx context.Context, er *session.EntRepository) {
			l.reconcileBouncingItems(ctx, er)
		}},
		// PASS-verdict items whose last N work sessions all ended with no commits
		// (the 70+ report_duplicate no-op dispatch loop) — flags them so the
		// dispatcher gate in BacklogService.spawnSessionAfterGates stops respawning.
		funcStuckDetector{"repeated_noop_dispatch", func(ctx context.Context, er *session.EntRepository) {
			l.reconcileRepeatedNoopDispatch(ctx, er)
		}},
		// Retry the push+PR flow for items with an open push_failed row (Phase B
		// of docs/tasks/backlog-stuck-item-auto-remediation.md). This is the
		// periodic counterpart to pushAndCreatePR's own event-driven attempt:
		// nothing else ever re-invokes pushAndCreatePR for an item that has no
		// active session, so without this detector a push_failed item sits stuck
		// forever once nothing is left running (the exact shape live-repro'd
		// 2026-07-20: c2ad7bf3-91bf-4d47-8654-0f2f20869080).
		funcStuckDetector{"push_failed", func(ctx context.Context, er *session.EntRepository) {
			l.reconcilePushFailedItems(ctx, er)
		}},
		// Flag idea-status items whose triage session crashed, was killed, or never
		// reached the completion goroutine (e.g. a server restart mid-triage) —
		// previously only caught on a manual re-trigger (tombstoneOrphanTriageSessions),
		// now a standing detector so it doesn't require a human to notice and retry.
		funcStuckDetector{"orphaned_triage", func(ctx context.Context, er *session.EntRepository) {
			l.reconcileOrphanedTriageItems(ctx, er)
		}},
		// Retry triage for every open orphaned_triage row still anchored at
		// "idea" (Phase B of docs/tasks/backlog-stuck-item-auto-remediation.md) —
		// the periodic counterpart to reconcileOrphanedTriageItems' one-time
		// MarkStuck+notify above, which never retries on its own once the
		// orphaned triage session is tombstoned (that session's EndedAt is no
		// longer nil, so the detector above never re-fires for the same item).
		// Closes the exact gap confirmed live 2026-07-27
		// (docs/tasks/backlog-feature-improvement.md): items 4f03de7b and
		// 505fb733 sat in "idea" for 2 days with only the one notification ever
		// sent.
		funcStuckDetector{"orphaned_triage_remediation", func(ctx context.Context, er *session.EntRepository) {
			l.reconcileOrphanedTriageRemediation(ctx, er)
		}},
		// Flag a custom-transition gate check (Epic 2.4, Task 2.4.4c) whose
		// invocation has been open longer than its bound LivenessDefinition's
		// StalenessThreshold — the same LivenessEngine-consulting sweep pattern as
		// reconcileOrphanedTriageItems above, applied to InvokeCustomGateCheck's
		// in-flight GateSatisfactionRecord rows instead of ItemSession rows.
		funcStuckDetector{"gate_timeout", func(ctx context.Context, er *session.EntRepository) {
			l.reconcileCustomGateChecks(ctx, er)
		}},
		// Flag queued items DequeueNextQueuedItems' planning gate refuses to ever
		// claim (plan not approved, skip_planning not set) — otherwise silent
		// forever except for a per-tick WARNING log. See reconcilePlanNotApprovedItems'
		// doc comment (BUG-038).
		funcStuckDetector{"plan_not_approved", func(ctx context.Context, er *session.EntRepository) {
			l.reconcilePlanNotApprovedItems(ctx, er)
		}},
		// Self-heal sweep: resolve any open stuck row whose reason's expected
		// status no longer matches the item's current status (Task 2.1.5d).
		funcStuckDetector{"self_heal", func(ctx context.Context, er *session.EntRepository) {
			l.selfHealStuck(ctx, er)
		}},
		// Multi-reason escalation detector (Signal 1, Epic 1.2): marks/refreshes a
		// durable multiple_reasons row for any item with 2+ simultaneously open
		// non-escalation stuck reasons, and dwell-gated-notifies once. Registered
		// immediately after self_heal (not before it) so a terminal-status item's
		// stale non-escalation rows have already been cleared this tick before
		// being counted — see reconcileMultiReasonEscalation's doc comment.
		funcStuckDetector{"multi_reason_escalation", func(ctx context.Context, er *session.EntRepository) {
			l.reconcileMultiReasonEscalation(ctx, er)
		}},
		// Auto-archive items that have sat in "done" for longer than maxDoneAge.
		// Registered immediately before archive_terminal_sessions so a freshly
		// archived item's work sessions are swept in the same tick — see
		// archiveStaleDoneItems' doc comment.
		funcStuckDetector{"auto_archive_done", func(ctx context.Context, _ *session.EntRepository) {
			l.archiveStaleDoneItems(ctx)
		}},
		// Safety-net sweep: archive work sessions for items already in done/archived
		// status that TransitionBacklogItemStatus's archival hook missed (pre-existing
		// terminal items from before that hook existed, or a race/crash mid-transition).
		// See SessionArchiver's doc comment. No-op when unwired.
		funcStuckDetector{"archive_terminal_sessions", func(ctx context.Context, _ *session.EntRepository) {
			l.reconcileTerminalItemSessions(ctx)
		}},
		// Safety-net sweep: re-run worktree cleanup for items already in done/archived
		// status whose synchronous WorktreeCleaner call (transitionBouncingItemToDone, the
		// PR-merge-detected done path, the manual archive RPC) was missed or never fired
		// (pre-existing terminal items from before those call sites existed, or a
		// race/crash mid-transition). Unlike archive_terminal_sessions above, worktree
		// cleanup previously had no periodic re-scan at all. See
		// reconcileTerminalItemWorktrees' doc comment. No-op when unwired.
		funcStuckDetector{"cleanup_terminal_worktrees", func(ctx context.Context, _ *session.EntRepository) {
			l.reconcileTerminalItemWorktrees(ctx)
		}},
		// Self-heal items whose real, cached PR reference (prNumber/prUrl) has
		// drifted out of ReconcilePRPending's status=="pr_pending" view — see
		// FindDriftedPRItems' doc comment. Registered immediately before
		// pr_ready+merge_detection so a recovered item is picked up by the merge/
		// CI polling sweep below in this same tick rather than an extra cycle
		// later.
		funcStuckDetector{"pr_drift_recovery", func(ctx context.Context, er *session.EntRepository) {
			l.reconcileDriftedPRItems(ctx, er)
		}},
		// Flag pr_pending items with no PR reference at all (pr_number == 0) — a
		// permanent dead end otherwise invisible to FindPRPendingItems'
		// PrNumberGT(0) filter and everything downstream of it, including
		// ReconcilePRPending itself (BUG-040). Registered immediately before
		// pr_ready+merge_detection, mirroring pr_drift_recovery's placement, so a
		// drift-recovered item that still somehow lacks a pr_number is caught in
		// the same tick.
		funcStuckDetector{"pr_pending_no_pr", func(ctx context.Context, er *session.EntRepository) {
			l.reconcilePRPendingWithoutPRItems(ctx, er)
		}},
		// Poll pr_pending items: auto-transition to done when the PR is merged,
		// and (Story 2.1.1) flag/resolve pr_ready_unmerged.
		funcStuckDetector{"pr_ready+merge_detection", func(ctx context.Context, er *session.EntRepository) {
			l.ReconcilePRPending(ctx, er)
		}},
		// Resolve-only counterpart to notifyBlockedByDependency
		// (server/services/backlog_service_triage.go), which marks
		// StuckReasonBlockedByDependency but has no periodic tick of its own to
		// notice when the blocker reaches a resolved status — that only happens
		// on the next DequeueNextQueuedItems sweep, which won't re-run for an
		// item that's already been skipped once this tick.
		funcStuckDetector{"blocked_by_dependency", func(ctx context.Context, er *session.EntRepository) {
			l.reconcileBlockedByDependencyResolution(ctx, er)
		}},
		// Safety net for the backlog work-item queue: dequeues queued items whose
		// exit-hook trigger was missed (server restart mid-transition, panic in the
		// hook's own goroutine) or whose slot was freed by the concurrency limit
		// being raised via Settings while items were queued (not itself a session
		// exit, so onSessionExited never fires for it).
		funcStuckDetector{"dequeue_backlog_items", func(ctx context.Context, _ *session.EntRepository) {
			l.triggerDequeue(ctx)
		}},
		// Restart-recovery for pipeline chain-fires interrupted by a crash between
		// the "done" transition committing and the chained session actually being
		// created (webhook-triggers AC5). See reconcileTriggerChains' doc comment.
		funcStuckDetector{"trigger_chain_reconcile", func(ctx context.Context, er *session.EntRepository) {
			l.reconcileTriggerChains(ctx, er)
		}},
	}
}
