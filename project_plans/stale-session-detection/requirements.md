# Requirements: stale-session-detection

**Date**: 2026-08-06
**Source**: Migrated from https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/41 (backlog item 477d0170-3baf-4907-80ea-1a2ec30d57f9)
**Type**: feature — mostly wiring/UI/config on top of already-shipped data plumbing
**Complexity**: 2 — no new data model required; scoped additions to existing config, UI, and approval-rule surfaces

## Problem Statement

Running 5-10 parallel agent sessions makes it easy to miss one that silently stopped
producing output (stuck agent, waiting prompt, crashed process). The original request
asks for: a configurable staleness threshold, a visual indicator + optional notification
when a session goes stale, a "Stale" grouping/filter option, and a `session_age_minutes`
condition usable in auto-approval rules.

**Pre-implementation research finding (this triage pass, 2026-08-06):** most of the
underlying plumbing this request needs already exists in the codebase, built for adjacent
features:

- **Timestamps already tracked and wire-exposed.** `ReviewState.LastTerminalUpdate` /
  `LastMeaningfulOutput` (`session/review_state.go:50-76`) are populated per session and
  already on the proto wire (`proto/session/v1/types.proto:41,44`). The frontend already
  reads them (`web-app/src/components/sessions/SessionCard.tsx:679-683`) but only to gate
  snapshot fetching, not to render a staleness indicator.
- **Two hardcoded, uncoordinated staleness thresholds already exist**, both Go constants,
  neither config-driven: the Review Queue "Stale" badge
  (`session/review_queue_poller.go:38,49`, `DefaultReviewQueuePollerConfig().StalenessThreshold`,
  currently 5min) and `reconcileStaleWorkSessions`'s durable stuck-item detector
  (`session/backlog_lifecycle.go:2098`, `maxWorkSessionStaleness = 2h`, feeding
  `StuckReasonStaleWork` and the `backlog-stuck` UI components). The adjacent
  `project_plans/review-gate-stale-session-rework/` plan already root-caused these two
  thresholds conflicting and recommends decoupling/recalibrating them — this project
  should build on that recommendation, not duplicate the investigation.
- **Notification plumbing already exists and is wired end-to-end**
  (`server/services/notification_service.go`, `eventBus`, frontend
  `NotificationContext.tsx`/`NotificationsNavBadge.tsx`/`NotificationPanel.tsx`).
  `notifyIfActiveWorkSessionStale` (`server/services/backlog_service_triage.go:882-967`)
  is an existing caller of the same pipe for a narrower scenario (rework-blocked).

**What's genuinely net-new** (confirmed absent from the codebase):
1. A visual stale indicator on regular session cards in the main session list (today,
   "Stale" only exists inside the review-queue-specific `ReasonStale` logic and the
   backlog-stuck panel — not on `SessionCard.tsx` in the normal grouped session view).
2. A "Stale" grouping/filter strategy in `web-app/src/lib/grouping/strategies.ts` (9
   strategies exist today: Category, Tag, Branch, Path, Program, Status, SessionType,
   Project, Workflow — no Stale option).
3. Config-driven staleness thresholds (`stale_session_threshold_minutes`,
   `stale_notify` or equivalent) — today both existing thresholds are hardcoded Go
   constants with zero `config/` surface.
4. A `session_age_minutes`-style time-based condition on `ApprovalRuleProto`
   (`proto/session/v1/types.proto:1076-1108` has only tool/command/file/risk matching
   fields today — no duration-based condition of any kind).
5. A stale-session notification event wired onto the existing notification bus (the
   plumbing exists; no such event is emitted for "a session in the main list went stale"
   today — only the narrower rework-block case is).

## Users / Consumers

Single user (Tyler), via the stapler-squad web UI (desktop and mobile), the backlog
automation/reconciliation loop, and the auto-approval rule engine.

## Success Metrics

**Outcome metric (the actual thing this feature is for, distinct from the build
checklist below):** the user notices a silently-stalled `ACTIVE` session (stuck agent,
waiting prompt, crashed process) by glancing at the main session list, without having
to open each card individually or discover it hours later by accident. There is no
pre-existing incident count to baseline against (unlike `review-gate-stale-session-rework`'s
cited 37/41 false-positive incident) — this is a net-new capability, not a fix to a
measured failure rate. The falsifiable proxy for "it worked": after Phase 4 ships, a
week of real single-user usage produces zero "I didn't notice my session died for
hours" incidents that a working badge/notification would have caught, self-observed
and logged informally (no telemetry infrastructure exists for this — see Constraints).
If the 30-minute default threshold (an explicitly-acknowledged best-effort estimate,
not empirically derived — see ADR-001 Consequences) proves too noisy or too slow
against real usage, that is itself a Success Metric failure signal requiring
recalibration, not a separate concern.

**Build checklist (acceptance-criteria-level, necessary but not sufficient for the
outcome above):**
- A session that has produced no output for longer than the configured threshold is
  visibly flagged in the main session list (not just the review queue / stuck-items
  panel) without the user having to open each session.
- The staleness threshold is configurable (not a recompile) and the review-queue /
  active-session-blocking-rework use cases can use independently appropriate values,
  consistent with `review-gate-stale-session-rework`'s recalibration work.
- An auto-approval rule can be written that denies/allows based on session idle time,
  and it is exercised by at least one test.
- No duplicate threshold/detector is introduced where an existing one
  (`StalenessThreshold`, `maxWorkSessionStaleness`, `StuckReasonStaleWork`) already
  covers the same signal — new work extends/config-drives existing mechanisms rather
  than forking them.

**Prioritization**: single-developer roadmap, no competing-team RICE/ICE scoring
needed — justified directly by the Problem Statement (running 5-10 parallel sessions
today has zero mechanism for catching a silently-dead one short of manually opening
each card) and by Complexity 2 (mostly wiring atop already-shipped plumbing, per
Phase 2 research) making the cost low relative to the gap it closes.

## Constraints

- Must not conflict with, or duplicate the investigation already done in,
  `project_plans/review-gate-stale-session-rework/` (threshold recalibration + durable
  stuck-state surfacing for the rework-blocked case) or
  `project_plans/review-queue-state-detection/` (working/idle/stuck classification
  accuracy) or `project_plans/review-queue-event-driven/` (event-driven
  `LastMeaningfulOutput` updates replacing polling). Phase 2 research must re-confirm
  current implementation status of all three against `main` before this plan assumes
  anything about them.
- Reuse `ReviewState.LastMeaningfulOutput`/`LastTerminalUpdate` as the staleness signal
  source — do not add a new per-session timestamp/event-bus tracking mechanism when one
  already exists.
- Reuse the existing `StuckReasonStaleWork` / `backlog-stuck` UI pattern where the
  scenario is "work session considered stuck," rather than inventing a parallel
  "Stale" concept with different semantics for the same underlying condition. The
  main-session-list card indicator and grouping option are a distinct, narrower UI
  surface (session-level "no recent output," independent of backlog stuck-state) and
  are in scope as new work.
- `docs/registry/` feature-registry rule applies to any new RPC/UI component
  (`.claude/rules/feature-registry.md`); new omnibar/session-creation touchpoints are
  not implicated (this is a filter/grouping/indicator addition, not a new session
  creation mode).
- Single-developer, self-hosted instance — no multi-tenant/auth considerations.

## Scope

### In Scope

- Config-driven staleness threshold(s) surfaced in `config/` (e.g.
  `stale_session_threshold_minutes`), read by the existing detectors instead of (or in
  addition to, per review-gate-stale-session-rework's split-threshold recommendation)
  their current hardcoded constants.
- Visual stale indicator (icon/badge) on `SessionCard.tsx` in the main session list,
  driven by `LastMeaningfulOutput`/`LastTerminalUpdate` vs. the configured threshold.
- New "Stale" grouping/filter strategy in `web-app/src/lib/grouping/strategies.ts` and
  its UI wiring.
- Optional stale-session notification (`stale_notify` config flag) emitted onto the
  existing notification bus when a session crosses the threshold.
- `session_age_minutes` (or equivalent) condition added to `ApprovalRuleProto` and the
  approval-rule evaluation logic, usable in rules like "deny approvals from sessions
  stale > 60 minutes."
- Feature registry entries for any new RPC/UI component per
  `.claude/rules/feature-registry.md`.

### Out of Scope

- Re-deriving the threshold recalibration analysis and durable-stuck-state UI already
  covered by `project_plans/review-gate-stale-session-rework/` — this project should
  depend on / extend that work's outcome for the review-queue and rework-block
  thresholds specifically, not redo it.
- Working/idle/stuck classification accuracy improvements — `project_plans/review-queue-state-detection/`'s
  territory.
- Replacing polling with event-driven output tracking —
  `project_plans/review-queue-event-driven/`'s territory.
- Separate review-queue-vs-active-session threshold *values* research — deferred to
  whatever `review-gate-stale-session-rework` lands on; this project consumes that
  output rather than re-deciding it.

## Open Questions — resolved by Phase 2 research

- **Threshold reuse vs. new threshold**: *Resolved.* `review-gate-stale-session-rework`
  has since shipped (PR #219, verified live on `main`): Review Queue badge threshold is
  now 5min (`session/review_queue_poller.go:49`), a new `maxReworkBlockStaleness`=15min
  exists (`server/services/backlog_service_triage.go:1030`), and the stuck-detector
  `maxWorkSessionStaleness`=2h (`session/backlog_lifecycle.go:2098`) is unchanged — three
  thresholds already live, not two. Both the architecture and UX research agents
  independently recommend the main-session-list card indicator use its own fourth,
  distinct, config-driven threshold (default 30min per the original issue) rather than
  silently inheriting one of the other three, which serve different purposes (queue
  triage priority, an automation safety gate, a durable stuck-item detector). Phase 3
  must document this as a fourth named constant with explicit rationale, following the
  `review-gate-stale-session-rework`/ADR-001 precedent, to avoid uncoordinated threshold
  proliferation flagged as the top risk in `research/pitfalls.md`.
- **Config surface shape**: *Resolved.* A new `StaleSessionConfig` struct in
  `config/types.go`, threaded to the frontend via the existing
  `SessionDefaultsConfig`/`sessionDefaultsToProto` RPC pattern (no new RPC needed) — see
  `research/architecture.md`.
- **`session_age_minutes` semantics**: *Resolved.* Measures idle-since-last-output
  (reusing `GetTimeSinceLastMeaningfulOutput()`, `session/instance_approval.go:112`),
  not session creation time, for consistency with the rest of the feature. The proto
  field name should be reconsidered (e.g. `session_idle_minutes`) to avoid the
  "age" vs. "idle" ambiguity the features research flagged — final naming left to
  Phase 3 planning.

See `project_plans/stale-session-detection/research/*.md` for full detail behind each
resolution.

## Riskiest Assumption

The 30-minute default threshold (`ADR-001` Decision 1) is a **best-effort estimate**,
not derived from measured turn-duration data — the single riskiest assumption in this
project, per `implementation/pre-mortem.md` Failure #1 (P1-adjacent P2: likely, not
catastrophic, but the whole feature's value depends on this number being roughly
right). If it's too short, legitimately-active sessions get flagged and the badge gets
mentally filtered out (the same failure mode `review-gate-stale-session-rework`'s
37/41 incident demonstrated for a different threshold); if too long, the feature
misses genuinely-stalled sessions it exists to catch. Validation gate:
`implementation/plan.md` Task 4.2.1b's manual smoke test, extended per pre-mortem.md's
Prevention column to run against a real multi-session working day (not just one
session) before Phase 4 merges, with the false-positive/false-negative read recorded
back into ADR-001.
