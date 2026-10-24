# Requirements: attention-inbox-consolidation

**Date**: 2026-10-23
**Type**: feature addition (with a decision on consolidating three existing surfaces)
**Complexity**: 3 — system design (multiple surfaces: detection pipeline, notification tray, Up Next page, review-queue page, session list; includes a recommendation/ADR on page consolidation)

## Problem Statement

The Review Queue page was meant to be the one place the operator goes to see "sessions that need me or have information for me." It is under-used because the pipeline that feeds it misses sessions that genuinely need attention, so the operator cannot trust it as the source of truth and falls back to scanning the sessions list. Three concrete gaps, all confirmed by the operator:

Clarified by the operator: "paused" means a still-running session whose agent **finished its work and is waiting for more context, or asked a question in plain prose instead of using a question tool** (AskUserQuestion / hook-based question). It does not mean `Status=Paused`. Terminology (kept in sync with the plan, triad repair iteration 1): "finished and waiting for more context" / "finished-awaiting-context" is the reason `awaiting_direction` (UI label "Finished, waiting for direction"), and "asked a question in prose" is the reason `prose_question` (UI label "Asked a question"); see `implementation/plan.md` Domain Glossary.

1. **Plain-prose questions are invisible** — the agent asks "which approach do you want?" as ordinary text. Detection keys off explicit prompts (approval dialog, input-required patterns, `StatusSuccess`), so a free-text question at an idle prompt falls to generic `ReasonIdle` (Low priority) or nothing, and may be suppressed by acknowledgment (`suppressedByAck`), the `basicIdleThreshold`, or `StatusWaitingForAgent` (e.g. the auto-mode footer) — `session/review_queue_determiner.go:166-288`.
2. **Finished and waiting for more context** — completion/idle items are Low priority, can be suppressed (hidden sessions, ack-after-output, background-task footer for up to 30 min), and likely produce weak or no notification compared with approvals.
3. **Missed notifications are unrecoverable** — if a completion/question notification is missed or dismissed, there is no durable place to find the session later or to see what happened when it finished.

Out of the operator's stated meaning (kept only as a secondary check): `shouldSkipSession` (`session/review_queue_poller.go:822-835`) skips Status Paused/Stopped/Crashed/Hibernated sessions; research should confirm whether any session the operator considers "paused" is actually in one of those statuses (e.g. auto-pause) before treating it as in scope.

Two adjacent problems:

4. **No per-session event log** — the operator cannot see "what was going on with a session when it completed" (completion, pause, error, approval, idle transitions) to decide what to look at next, which sessions have not changed in a while, or which just finished.
5. **Session "age/updated" timer resets on browser reload** — `SessionCard.tsx:1272` renders `Updated {formatTimeAgo(session.updatedAt)}`, but the displayed value restarts after a tab reload. Hypothesis to confirm in research: `UpdatedAt` is bumped by non-semantic events (`touchUpdatedAt()` on every status transition, pin and note; `session/instance_serialization.go:110` overwrites it with `time.Now()` on each persist; several DB partial updates also set `updated_at`), so it measures "last touched by the server" rather than "last meaningful change" or "session age".

Plus a product-direction question the operator asked explicitly: should the Review Queue page be replaced by the notification tray/bar (added after the queue) and its information consolidated into the Up Next page (`web-app/src/app/unfinished/`)?

## Baseline

- Review Queue (`/review-queue`, `ReviewQueuePanel`) lists sessions flagged by `DefaultStatusDeterminer` (reasons: approval_pending, input_required, error_state, tests_failing, task_complete, uncommitted_changes, idle, stale, waiting_for_user). Suspended, hidden, archived and not-started sessions are never evaluated. Hidden-session task-complete/idle/stale are suppressed.
- A notification tray (`/notifications`, `NotificationItem`, hidden-session delivery gate) and an Up Next page (`/unfinished`: PR tabs, backlog queue) exist as separate surfaces with overlapping purpose.
- Session cards show `Updated <relative time>` from `updatedAt`; no per-session event timeline is exposed; `/history` and `/logs` exist but are not session-event-oriented.
- Operator workaround today: scan the sessions list and open sessions one by one; rely on desktop/push/Slack notifications that may be missed.

## Users / Consumers

- Primary: the single operator (Tyler), on desktop and mobile web (both form factors required).
- System consumers: notification delivery (tray, web push, Slack), MCP/agent callers of `get_notification_history`, session list/WatchSessions streams.

## Success Metrics

Tied to the baseline above; all measurable without self-report:

- **Coverage**: 100% of running sessions whose agent is waiting on the operator (approval, tool-based question, prose question, finished awaiting direction, error, stale) appear in the single attention surface within one poll/event interval, verified by a fixture corpus of real terminal tails per program, including prose-question and finished-awaiting-context cases; measured false-negative rate against operator-labelled samples. **Baseline B1** (Phase 0, Story 0.1.6a): the share of needs-operator episodes in the last 30 days that were missed (no operator input or session view within 30 minutes), split into detected-but-missed (from notification history, logs and the session store) and a sampled estimate of undetected prose questions (>= 100 owner-labelled transcripts); the method and its limitations are written to `docs/reference/attention-inbox.md`. **Target (proposed, owner confirms once B1 is recorded)**: `/actuator/attention-coverage` gap = 0 in the soak, and the missed share falls to at most half of B1 (or at most 5%, whichever is larger) over the 14 days after `attention_prose_question_detection` is on. **B1 measured 2026-10-24 (`docs/reference/attention-inbox.md`)**: (a) detected-but-missed = 63 of 81 needs-operator episodes (77.8%, upper bound), over the actual retained window 2026-10-18 to 2026-10-24 (5.8 days, not 30; notification history prunes at 7 days); (b) undetected-question sample: pending owner labelling (100 transcripts harvested, 0 labelled), so the combined B1 is not yet computed and the target above stays a placeholder. **If B1 < 5%** the "or at most 5%" floor would pass vacuously, so the target becomes: missed share does not exceed B1 AND prose-question recall on the owner-labelled corpus is >= 0.80. **Gap = 0 is necessary but not sufficient**: the actuator's ground truth is detector status or a persisted question, so it cannot see a prose question the classifier never flagged; recall on the owner-labelled corpus (Story 3.1.2 and the soak sample) is the falsifier of "100% coverage", and a gap of 0 with recall below 0.80 is a failed Coverage metric. **Scope per program**: prose-question detection is Claude-only (Stop hook or transcript). Aider, agy and pi get finished-awaiting-direction (screen fallback) plus the approvals and errors they already produce, so for them "100%" covers those reasons only and prose questions are reported "Finished-only, cannot detect"; Coverage, the gap and recall are computed and reported per program, never as one blended figure.
- **Durability**: an item the operator did not act on remains visible until acknowledged or the state changes; a missed notification is recoverable from the surface, not only from the tray.
- **Event log**: for any session, the last N state transitions (what, when, why) are viewable in ≤ 2 clicks from the surface and from the session card, and survive page reload and server restart.
- **Timer correctness**: the "last changed"/"age" value on a session card is identical before and after a browser reload and after a server restart (stable timestamp, not a client/mount-relative value).
- **Usage**: review/attention surface opens per day vs. the measured baseline increases; secondary, observational. **Baseline U1** (Phase 0, Story 0.1.6b): mean daily `page_view` events for `/review-queue` over the trailing 14 days from the analytics sink behind `usePageView` (`web-app/src/lib/analytics/usePageView.ts`); if no sink persists them, a 14-day collection window starts at PR A's merge. **Target (proposed)**: >= 1.5x U1 over the 14 days after PR B's flag flip. **U1 measured 2026-10-24**: 5.14 `/review-queue` page_view events per day over 2026-10-10 to 2026-10-23 (72 events; persisted in `analytics.db` `analytics_events`); 69.71/day if the trailing-slash path form is included, which stopped being emitted around 2026-10-21 (see the doc); includes any automation traffic. The target stays a placeholder. Opens alone do not prove trust, so this stays secondary to Coverage and Alert guarantee.
- **Noise guardrail (alert fatigue; thresholds proposed, owner to confirm)**: more alerts must not be bought with a flood. Over the 14 days after `attention_prose_question_detection` is on: (a) the "Not a question" dismiss rate (`attention_prose_question_dismissed_total` / `attention_items_created_total{reason="prose_question"}`) is < 10% over the first 100 items (the pre-mortem's bar; the > 20% trip of the runbook still forces the flag off); (b) mute usage stays at most 3 mute actions and 1 "Stop flagging questions" opt-out per week after week 1; (c) push-cap hits: the global `awaiting_direction` cap fires on at most 2 of the 14 days and the digest averages at most 1 per day. A breach means the alerts are being tuned out, which defeats Coverage.
- **Alert guarantee**: for every needs-human item and every finished-awaiting-direction episode on a visible session, at least one operator-facing channel fires: tray record, toast, web push, Slack, or the non-alerting "N new finished" nav cue (which counts every unseen finished session, whether or not push can reach the device: ADR-007 #1). Verified by an integration test with a negative control (`server/review_queue_alert_guarantee_test.go`) and a browser spec, not only by a row existing in the list. Hidden sessions are asserted silent by design. **Limitation**: this proves an alert or cue was produced, not that the operator perceived it; perception is only sampled by the manual usability script (Story 7.3.3).

## Size

Large to XL (~24.6M CU per `implementation/plan.md` Effort estimate after triad repair iteration 2; INFERRED, bands per ESTIMATION.md): detection-pipeline change, durable event log (storage + RPC + UI), delivery policy and mute, surface consolidation, timer fix, baselines and usability script. Full scope is the default; not a gate.

**Wall-clock blockers**: owner decision on page consolidation recommendation (ADR) before implementing removal/redirect of `/review-queue`; a soak window on an isolated instance (`STAPLER_SQUAD_INSTANCE` + private tmux socket, per CLAUDE.md) to observe the new prose-question and finished-awaiting-context reasons; CI runs (e2e, axe).

## Roadmap Fit

- **Why now**: the operator cannot trust the Review Queue (three confirmed gaps), so the work removes a daily manual scan of the sessions list. Reach: one operator, every session, every day. Impact: high (restores the surface's purpose). Confidence: medium (the Stop-hook premise is unverified until the Story 0.1.5 gate; the missed-rate baseline B1 is not yet measured). Effort: ~24.6M CU across four soaked PRs. RICE (relative, INFERRED, no absolute score claimed): Reach 1 operator with high frequency, Impact 3 (massive for the stated complaint), Confidence 50-70% until Phase 0 completes, Effort high; it ranks above polish work because every other attention feature depends on the list being trusted, and below any outstanding production-breaking bug.
- **Interaction with the backlog WIP limit of 2** (memory `feedback_backlog_wip_limit`; 2026-07-12 OOM incident): this project is one backlog item delivered as four sequential PRs. Within a PR at most 2 workers run concurrently on the serialized Go files (plan.md Worker briefing), so it never takes more than the 2 slots and no second backlog item starts beside two active workers. The owner-gated waits (ADR ratification, Story 0.1.5 threshold, each soak, the usability script) free the slots for other items.
- **Kill/continue signal**: if the Story 0.1.5 gate returns F2 (neither hook nor transcript usable), prose-question detection is not built and the owner is told the main complaint is not fixed in this release; the remaining phases (finished-waiting, event log, timer) still ship.

## Constraints

- Follow `.claude/rules/instance-lock-free-reads.md`, `noarchivedrevival.md` (never revive archived sessions as a side effect of surfacing them), `nav-facade.md` (RouteMeta rows, `@/lib/nav`).
- New rollout behavior is gated by live-settable feature flags (no env vars), per project memory.
- Must not use `make install-service` to trial; test on a separate instance.
- Mobile + desktop UX both required; e2e conventions apply; proto changes via `make proto-gen`; if a new RPC is added, run `make registry-generate` and update route tables.
- Bun only for web-app.

## Non-functional Requirements

- **Performance SLO**: attention state changes surface within the existing poll/event latency (no new tight polling loop; prefer the event-driven path — see `project_plans/review-queue-event-driven/`).
- **Scalability**: tens to low hundreds of sessions; event log bounded (retention cap per session and global).
- **Security classification**: internal (local operator tool; log excerpts may contain terminal content — avoid persisting secrets from output).
- **Data residency**: no special requirements.
- **Security (write surface)**: the new write RPCs (`UnacknowledgeSession`, `AcknowledgeSessions`, `SetNotificationMute`, the per-session question opt-out) follow the existing protection model, verified in `server/guarded_procedures.go` and `server/middleware/auth.go`: every route sits behind `HostGuard` and, when configured, the auth middleware (`Auth`); the sibling write `AcknowledgeSession` is not in `guardedProcedures`, so the pure state writes match it, while the alert-silencing writes (`SetNotificationMute`, the opt-out) are added to `guardedProcedures` with `ProfileRebinding` like `UpdateFeatureFlag`; reads (`ListSessionEvents`, `GetNotificationMutes`) stay unguarded. Abuse limits: `ListSessionEvents` limit capped at 500, `AcknowledgeSessions` at 100 items per call, mute duration clamped to 24 h, session-scoped writes reject an unknown session. Event and tray excerpts are rendered only as React text nodes (no HTML, no markdown, no `dangerouslySetInnerHTML`).

## Riskiest assumption

That the Claude Stop hook (or, failing that, the transcript tail) reliably identifies that a turn ended on a prose question, for most of the sessions the operator runs. Everything in the main complaint (gap 1) rests on it, and it is unverified until Spikes 0.1.1 and 0.1.4 run. Story 0.1.5 is the falsifier: GO needs Stop (or a readable transcript) for >= 80% of the operator's session-creation paths; otherwise fallback F1 or F2 applies and the owner is told the prose-question complaint is not fixed in this release. The classifier bar (precision >= 0.90, recall >= 0.80 on the owner-labelled corpus, Story 3.1.2) is the second test of the same assumption.

## Scope

### In Scope

- Root-cause analysis of why prose-question and finished-awaiting-context sessions are not producing review items/notifications (pipeline audit from detection → queue → notification → UI).
- Detecting "finished, waiting for context" and "asked a question in prose (no question tool)" as first-class attention reasons with a priority above generic idle, and a reliable notification for each.
- Secondary, only if research shows it matters: surfacing unexpectedly suspended (crashed/auto-paused) sessions without reviving them.
- A durable per-session event log (state transitions with reason and timestamp) and a UI to browse it per session and across sessions (recently finished, unchanged for a while).
- Diagnose and fix the timer-resets-on-reload behavior; define what the timer should show (last meaningful change vs. age).
- Research of comparable solutions (inbox/triage UIs, agent-orchestrator dashboards, CI/on-call attention queues) and a recommendation, recorded as an ADR, on: replace the Review Queue page with the notification tray, consolidate into Up Next, or keep and fix.
- Migration/redirect plan for whichever surface is retired.

### Out of Scope

- Changing the agent state-detection heuristics for terminal parsing beyond what the coverage gap requires.
- Auto-reviving, auto-resuming or auto-answering sessions.
- New external notification channels.
- Rewriting the backlog Up Next PR tabs (only integrating the attention feed with them).

## Rabbit Holes

- Hidden/system sessions: current rule suppresses complete/idle/stale for hidden sessions; changing it risks notification floods.
- Distinguishing a prose question from a status narration in terminal text (heuristic vs. LLM classifier vs. a stop-hook signal from the agent) risks false positives that re-flood the queue; the existing hook/transcript channel may give a more reliable "turn ended" signal than screen scraping.
- Deciding "unchanged for a while" requires a defined clock (`LastMeaningfulOutput` vs. `UpdatedAt`); UpdatedAt is bumped by non-semantic touches.
- Event-log storage growth and where it lives (ent schema vs. existing notification history store); ent generated code must not be committed.
- Dedup against existing notification dedup (`smart-notification-dedup`) and the hidden-session delivery gate so the log does not double-count.
- Consolidation touches nav, deep links (`/review-queue` URLs, keyboard shortcuts, onboarding modal, e2e specs, feature registry).

## Alternatives Considered

- A. Keep `/review-queue`, fix pipeline gaps, add event log and timer fix.
- B. Retire `/review-queue`; the notification tray becomes the attention surface (durable, grouped, actionable).
- C. Merge attention items into Up Next (`/unfinished`) as the single "what next" page, with the tray as transient alerts and a per-session event timeline.
- D. Hybrid: Up Next as the home for "needs me", tray for real-time toasts, event log as a drawer reachable from both.
- Existing-solution survey (research phase): PagerDuty/Opsgenie incident inbox, GitHub notifications inbox, Linear/Superhuman inbox-zero triage, CI dashboards, agent-orchestrator UIs.

## Feasibility Risks

- The detector may not distinguish "agent waiting on me" from "agent done" reliably for non-Claude programs (aider, agy, pi); coverage test must be per program.
- If `UpdatedAt` semantics are relied on by other consumers, changing it risks regressions (sort order, stale detection, backlog gates).
- Dropping a page can break e2e specs, analytics, deep links, and mobile shortcuts.

## Observability Requirements

- Counters: attention items created/cleared by reason (including the new prose-question and finished-awaiting-context reasons); notifications emitted vs. delivered vs. suppressed (hidden gate) per reason; event-log writes and retention drops.
- Log line when a session in a needs-operator state does not produce an item, with the skip reason (today `shouldSkipSession` skips silently).
- Dashboard/query to compute coverage: sessions in needs-operator states with no active item (should be 0).
- Alert condition: none paging (single-operator tool); a self-check on `/actuator` style endpoint reporting the gap count is sufficient.

## Risk Control

- **Post-ship decision rule**: at 14 days after the `attention_prose_question_detection` flag flip (Usage: 14 days after the `attention_inbox_consolidation` flip) the owner (Tyler) reviews one report of Coverage, Usage, Alert guarantee and the noise guardrail (Story 7.3.2) and records the outcome in `docs/reference/attention-inbox.md`. A miss maps to one action: Coverage miss or guardrail (a) at 10-20% -> tune the classifier and corpus and re-measure (flip `attention_prose_question_detection` off if the miss repeats); guardrail (a) above 20% or any alert-guarantee failure -> flip the offending flag off now; guardrail (b) or (c) breach -> tune the delivery constants first; a Usage miss alone -> no rollback, one more 14-day window, then reopen the consolidation decision (ADR-001).

- Feature flags (live-settable via the feature-flag RPC/panel, default off until soak; six, per plan.md Risk Control): `attention_prose_question_detection`, `session_event_log`, `attention_ack_bypass_fix`, `attention_idle_clock_output`, `attention_prose_question_llm_tiebreak`, and `attention_inbox_consolidation` (controls the redirect from `/review-queue`).
- Rollback: flip flags off; `/review-queue` is never removed (ADR-001: it stays as a permanent flag-gated redirect, so Slack, bookmarks and shortcuts keep working); event-log table is additive (no destructive migration).
- Staged rollout: isolated instance soak → live instance with flags on for the new detection reasons → consolidation last.

## Open Questions

- What exactly does the notification pipeline emit for a session that completes or pauses, and where does it drop (audit `ReviewQueuePoller` → queue → notification gate → tray/push)? Is there an existing event/transition store we can reuse (`get_notification_history`, activity-log work in `project_plans/backlog-item-activity-log/`)?
- Which timestamp should the session card show, and what is mutating `UpdatedAt` on reload/reconnect (research found no reload-only path: see research/stack.md; confirm `touchUpdatedAt` callers and DB partial updates)?
- What reliable signal marks "agent turn ended and last message ends in a question" (Claude Code Stop hook + transcript last message vs. terminal heuristics), and does it work for non-Claude programs?
- What do comparable tools do for "recently finished" and "gone quiet" views, and does a single inbox with filters beat separate pages for a one-operator workflow?
- Does the operator's Up Next page have room (information architecture) for session attention items alongside PR and backlog tabs on mobile?

## Unresolved after Phase 2 research
*(carried into plan.md as spikes, per owner decision 2026-10-23)*
- Does the Claude Stop-hook payload carry the last assistant message, or only `transcript_path`?
- What makes the session timer reset on a plain browser reload? Research found only server-side `UpdatedAt` bumps, no reload-only path; reproduce live.
- notification-revamp ADR-002 hid idle items from the Review Queue; this work needs a new visible reason or an explicit supersede.
