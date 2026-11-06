# Requirements: backlog-make-it-true

**Date**: 2026-10-17
**Type**: feature addition (wiring + settings surface + default-policy decisions)
**Complexity**: 3 — system design (5 workstreams, A to E, across Go services, proto/config, and web-app settings; touches the state-machine gate)

## Problem Statement

The backlog feature advertises configurability and automation that does not take effect. Operators see settings UIs and status text that are disconnected from runtime behavior, and the automation that makes the "autonomous software factory" goal real ships disabled. Specifically (all from this session's cohesion audit; items marked VERIFIED were confirmed by reading code):

1. **Stage/transition/gate configuration is not enforced.** The settings UI (`web-app/src/app/settings/backlog-stages/*`) and CRUD RPCs persist stage config, but `BacklogService` receives `session.NewDefaultWorkflowEngine()` (static `validTransitions`) — `server/dependencies.go:678`, passed at `:1477`. The `ConfiguredWorkflowEngine` built at `:773` is used by `BacklogService` only for `InvalidateCache` (`backlog_service.go:417-461`), but it is NOT unused elsewhere: the lifecycle listener already holds it (`SetWorkflowEngine(stageConfigEngine)`, `dependencies.go:1544`) for automated-review/custom-check gate resolution, so three engines coexist today (BacklogService default, lifecycle listener configured, `SyncLoop` private default at `session/backlog_sync.go:57,69`). Corrected 2026-10-17 after research. VERIFIED. Traced (Open Question 1, resolved): no status-write path other than `BacklogService.TransitionBacklogItemStatus` consults any engine for graph or gates; 40 other direct status-write call sites exist (ADR-007), and the configured engine is consulted only by the lifecycle listener for automated-review/custom-check gate resolution, so config is ignored by those 40 sites.
2. **Quota gating and the soft cost budget have no settings surface.** `Quota.Enabled` defaults false and `AssumedWindowTokenBudget` defaults 0 (disables the proactive signal); the web app only displays `status_detail` (`settings/features/page.tsx:170`). VERIFIED 2026-10-17 by command: `grep -rn -i quota proto/session/v1/` excluding GitHub/rate-limit lines returns 0 hits (no quota RPC or message), the only config reader is the closure at `server/dependencies.go:1436`, and the soft budget exists only per item (`cost_budget_threshold_usd`, editable in `web-app/src/components/backlog/BacklogItemForm.tsx`); no global default exists (`session/budget_warning.go` is pure and takes a per-item pointer).
3. **Repo auto-merge status always renders "unknown".** `allow_auto_merge` exists in the proto (`backlog.proto:2144`) and is rendered (`StuckItemDetail.tsx:54-59`) but no Go code sets it (`backlog_service_stuck.go:142`). Meanwhile `EnablePRAutoMerge` is attempted on every PR (`session/backlog_lifecycle_pr.go:718`) and failures leave the operator to merge by hand.
4. **Flag state is an undocumented, un-soaked decision.** CORRECTION (verified 2026-10-17 via `GetFeatureFlags` on the live instance): `backlog:sdd-default-pipeline`, `diagnose_nudge_enabled` and `autonomous_stall_nudge_enabled` are all ENABLED live, while the code defaults are false (`config/config.go:555`, `:561`; `backlog_service_lifecycle.go:150`). So a fresh install/instance behaves differently from the operator's, no record says when/why they were enabled, and no queryable soak evidence exists (no nudge-outcome counter or notification type). The earlier audit statement that this automation "ships off / is off" described code defaults only.

## Baseline

- Editing stages in settings changes nothing about which transitions/gates the backlog enforces; operators must change Go code.
- Quota gate is opt-in via config file only; an operator cannot tell from the UI whether it is on or what headroom budget it uses.
- Stuck-item panel says "Repo auto-merge: unknown" for every item; a PR that cannot auto-merge is discovered only after it stalls (`pr_ready_unmerged` nudge, `backlog_lifecycle_pr.go:2205`).
- Stalled autonomous sessions are killed and respawned rather than nudged (`docs/explanation/autonomous-stall-causes.md` "Known gaps").
- Live evidence at audit time (2026-10-18 UTC): 6 distinct stuck items across 13 stuck-reason rows out of 21 items returned by `ListBacklogItems`; one item orphaned in triage since 2026-09-04 with 6 remediation attempts.

## Users / Consumers

- The owner/operator configuring the factory via `/settings/*`.
- Autonomous backlog sessions and the reconciliation loop (consumers of the workflow engine and flags).
- Backlog UI (`StuckItemDetail`, settings pages) reading the new/populated fields.

## Success Metrics

Each is a behavior change tied to the baseline above:

1. **Config changes runtime.** Changing a transition or gate in the stages UI changes the result of `TransitionBacklogItemStatus` for that edge, proven by an integration test that edits config then asserts allow→deny (and deny→allow) without a restart. Baseline: no effect.
2. **Quota/budget settable in UI.** Operator can enable the quota gate and set the headroom budget and soft-budget threshold from settings; a test proves a changed value alters gate/warning behavior. Baseline: config-file only / not settable.
3. **Auto-merge status is real.** `allow_auto_merge` is populated for items with a PR repo; when the lookup genuinely fails the UI says the check failed and why (the word "unknown" is retired in the UI). Baseline: always unknown.
4. **Every flag state is a recorded decision backed by evidence.** For each of the three flags, an ADR states the code default, the live setting, evidence for it, and the rollback. Add a durable nudge-outcome counter/log so soak evidence becomes queryable; code defaults flip to on only when stated exit criteria are met. Baseline: live-on, code-default-off, no record, no evidence.
5. **No config surface without a proof.** A repo check (test or lint) fails if a stage-config/quota setting is added to the UI/proto without a test that exercises its runtime effect (the "every config surface must demonstrably change runtime behavior" rule).

6. **Outcome: the factory needs fewer manual rescues.** Metrics 1-5 prove the settings are true; this one measures whether that made the factory more autonomous. Two numbers, both read from `ListStuckBacklogItems` (`proto/session/v1/backlog.proto:1784`, the stuck report; `/api` prefix, `docs/reference/backlog-dependencies.md`) plus `ListBacklogItems` for the denominator: (a) **stuck-item rate** = distinct items with at least one open stuck row / items returned by `ListBacklogItems`; (b) **manual interventions per week** = stuck-report items the operator resolved by hand (manual merge of a `pr_ready_unmerged` item, "Retry now" / `ResetStuckRemediation`, or a hand transition out of a stuck state), counted from the item event log. Baseline (recorded by the cohesion audit, copied from the Baseline section below, NOT re-measured by this planning pass because the live MCP was unreachable): 13 stuck rows covering 6 distinct items among 21 items returned by `ListBacklogItems`, so (a) = 6/21 = 29%; (b) baseline not recorded (the owner records it from the item event log at the first deploy; until then (b) has no baseline and the review below compares (a) only). Target: not a pass/fail gate for the build. After at least one class is enforced and `diagnose_nudge`/`autonomous_stall_nudge` have run for the soak, (a) does not rise above the baseline and the share of stuck rows that are `transition_denied` is reported separately (denials are intended friction, not failures). **Measured** by the runbook's "Outcome review" step (Story 1.4.1) with the same two reads at the same time of day. **Reviewed** on a fixed date: 14 days after the first deploy of Epic 1.5 (the same date as the soak review in the plan's Risk Control), recorded as a dated line in the runbook and as a calendar reminder; `make workflow-engine-soak-status` prints `REVIEW DUE <date>` and, once the date has passed with no class enforced, `REVIEW OVERDUE` (exit code unchanged) so "the flip never happens" cannot stay silent (pre-mortem #10). Round 7 makes the forcing function visible in the product as well: the same status carries `review_due_at` / `review_overdue`, and the Stages banner and the Features "Workflow engine" summary show "Review overdue: ..." (plan Story 1.3.1 Task 1.3.1c, Story 1.3.2 Task 1.3.2g, UX-55).

*Owner decision 2026-10-17 on Metric 1 (the metric text above is unchanged and binding):* it applies to ALL status-transition paths — UI/RPC, MCP-by-agent, and system-driven lifecycle, reconciliation, polling and sync — not only the engine-routed ones. The plan routes every status write through one choke point with per-caller-class rollout (`decisions/ADR-007-engine-gated-transition-choke-point.md`). Edges that automation depends on cannot be disabled (the editor rejects it with a visible reason); gates on them are editable.

## Size

Large (3–15M CU), INFERRED at planning time; the repaired plan estimates about 19.8M CU after the owner decision to route every status-write path through the engine and Repair rounds 4 to 7, i.e. above the Large ceiling by about 32% (`implementation/plan.md` Effort estimate) — five workstreams (A to E), one of which is the state-machine gate (`TransitionBacklogItemStatus` is a top reconciliation hotspot). Full scope is the default; not a gate.

**Wall-clock blockers**:
- Owner decision (Tyler): which of the three flags to flip on by default vs. leave off with exit criteria (workstream D). The code/doc work does not need the answer, but the flip does.
- Soak window: any default flipped on needs an observation window on the live instance before it is called safe. Soak parameters are owner-approved 2026-10-17 (W = 7 days operator/agent, 14 days system; N = 20 per engine-routed edge; floor 100 calls; low-volume READY = owner acknowledgement of `--config-diff`, the stranded-item preview, the Story 1.5.9b per-caller table and a replay on a COPY of the live DB), so they no longer block W0.
- Do NOT use `make install-service` to try changes — it kills live tmux sessions; use the separate manual-instance recipe in CLAUDE.md (ports 62871/62872).
- Dependency: ADR-013 S2/S3 (custom states) is the intended seam; confirm what `ConfiguredWorkflowEngine` already guarantees before planning.
- Open owner confirmations (Tyler) and what each blocks (the full table is in the plan's Unresolved Questions): the Metric 1 carve-out reading blocks Story 1.2.1 (W5); the operator-gate policy for the `system` class and the agent `override_reason` rule block Story 1.5.2 / Task 1.5.2a (W9) and, for `override_reason`, Task 1.5.5c (W12); the soak start date of the three autonomy flags blocks Task 4.1.1a only (W18); Q5 blocks the owner-gated Task 4.1.5a only. None blocks W0 through W4.

## Constraints

- Go: honor `.claude/rules/instance-lock-free-reads.md` (Snapshot reads), `noarchivedrevival`, `norawghrequest`; new GitHub calls go through `NewConditionalRequest*`.
- Proto changes require `make proto-gen`; generated output and `session/ent/*` generated code are not committed. Ent schema changes use `--feature sql/upsert`.
- Rollout flags must be live-settable via the feature-flag RPC/panel, never env vars (memory: rollout-flags-live-settable).
- web-app uses pnpm only; mobile and desktop layouts both required for new settings UI.
- New code must pass the blocking `dupl`/gocyclo/gocognit new-code gates (`make ready`).
- Add `// +feature:` / `// +api:` markers and run `make registry-generate` if RPCs/components change.
- WIP cap: at most 2 concurrent backlog work sessions on the live instance.

## Non-functional Requirements

- **Performance SLO**: transition checks stay on the hot path of `TransitionBacklogItemStatus`; the configured engine must be cache-backed (it already has a stage-config cache) with no per-call DB round trip for graph and built-in-guard decisions. Caveat: an operator-added stateful gate (`human_approval`, `automated_review`, `custom`) reads its satisfaction record from the DB, bounded by a 2s timeout and failing closed; the seeded config has none. No regression in existing benchmarks.
- **Scalability**: not applicable (single-operator service, tens of items).
- **Security classification**: internal. Settings RPCs must follow existing authz/middleware; no new external surface.
- **Data residency**: no special requirements.

## Scope

### In Scope
- A. Make `ConfiguredWorkflowEngine` the engine every status transition uses — `BacklogService` and every other status-write path (MCP tools, lifecycle, reconciliation, pollers, sync) — for transitions and gates, with a safe fallback to the default graph, cache invalidation on CRUD writes already in place, and per-caller-class shadow-to-enforce rollout (ADR-007).
- B. Settings UI + config plumbing for quota gating (enabled, thresholds, `AssumedWindowTokenBudget`) and the soft-budget threshold, live-settable.
- C. Populate `allow_auto_merge` (repo-level lookup, cached, rate-limit-aware) and surface lookup failure honestly; use it to warn before/at PR creation when auto-merge cannot be enabled.
- D. Decision record and (after owner approval) default changes for `backlog:sdd-default-pipeline`, `diagnose_nudge_enabled`, `autonomous_stall_nudge_enabled`, with exit criteria/rollback.
- E. The enforcement rule from Success Metric 5.

### Out of Scope
- Respawn/remediation audit trail (`session-lifecycle-ux` / `status-visibility`) — roadmap item 2.
- Backward GitHub→item sync, `github-autonomous-fix`, retry-escalation policy, adaptive turn budget, merge-conflict auto-resolution, bulk approve/retry — roadmap item 3.
- Plan-doc housekeeping and finishing `no_ent_in_services` — roadmap item 4 (except where touched files are already being edited).
- New stage types or a redesigned stage-graph UI.

## Rabbit Holes

- **Engine swap semantics.** `DefaultWorkflowEngine` uses a static map; swapping changes behavior for existing items whose current status is not in an operator-edited graph (dangling statuses, in-flight items). Phase 3 must define: seeded default config identical to `validTransitions`, behavior on config load failure, and migration for items in removed stages.
- **Gate enforcement coverage.** Gates (`GateChecklist`, `backlog_service_gates.go`) may be checked in paths other than `TransitionBacklogItemStatus` (autonomous completion, MCP `transition_backlog_item_status`, lifecycle auto-transitions). Each path must route through the same engine or the config is still partly fiction. (Resolved by the owner decision of 2026-10-17: every path routes through one choke point, `Storage.GatedTransition`; see ADR-007.)
- **Circular wiring.** `ConfiguredWorkflowEngine` takes `pipelineModeRepo`; `BacklogService` takes the engine and the repo — check construction order in `dependencies.go`.
- **`allow_auto_merge` accuracy.** GitHub exposes repo `allow_auto_merge`, but effective auto-merge also needs branch protection/required checks; the field may be true while enabling still fails. Decide what "known" means.
- **Quota semantics.** `AssumedWindowTokenBudget = 0` disabling the proactive signal is a hidden coupling; the UI must make that state legible, not just editable.
- **Flag interaction.** `autonomous_stall_nudge_enabled` is AND-ed with `diagnose_nudge_enabled`; flipping one alone is a no-op — UI/docs must say so.

## Riskiest Assumption

**Named assumption (round 7)**: routing every status write (the 40 direct sites plus the 5 engine-routed ones, owner decision 2026-10-17) through one choke point, `Storage.GatedTransition`, does not strand in-flight items and does not break automation that depends on edges the stage editor could otherwise disable. If it is false, Success Metric 1 on all paths either cannot be met or damages the factory: an automatic transition denied by an edited gate looks like one more stuck item, and the stuck-item rate (Metric 6, baseline 6 of 21) rises instead of falling.

**Why this one**: it is the only assumption whose failure is operationally quiet, and the seeded configuration equals the built-in graph, so nothing on live data exercises it until an operator edits a stage, edge or gate.

**How the plan tests it, cheapest first**: (1) characterization of today's handler (Story 0.1.1) and the edge audit over all 45 sites (Story 1.5.1); (2) edges that automation requests cannot be disabled and edits that would strand a populated status are rejected (Stories 1.2.1, 1.2.1c; ADR-007 D4); (3) shadow first, one enforce flag per caller class, all four flags off equals today (Stories 1.1.x, 1.5.2); (4) a denied automatic transition becomes a `transition_denied` stuck item and never loops or respawns (Story 1.5.8); (5) a replay on a COPY of the live database applies a synthetic gate to each fixed edge (Task 1.5.9d); (6) owner-gated flips per class after the soak, with Metric 6 reviewed on the dated schedule.

**Falsifier (stop and do not flip that class)**: any `graph`, `gate_missing`, `gate_extra` or `error_kind` divergence on a seeded configuration, any stranded item in the replay or the `--config-diff` preview, or, after a class is enforced, a stuck-item rate above the 29% baseline once `transition_denied` rows (intended friction) are counted separately. The response is a flag flip (live, about 1s), not a rollback of code.

## Alternatives Considered

- Remove the stages settings UI instead of wiring it (rejects the product intent of configurable stages; ADR-013 S2/S3 explicitly plans this seam).
- Leave defaults off and only document them (keeps the factory opt-in; acceptable only for flags lacking soak evidence).
- Environment variables for quota (rejected by standing preference: live-settable flags).
- Query `allow_auto_merge` on demand per render vs. persist on item (persist trades staleness for rate-limit safety; decide in research).

## Feasibility Risks

- Behavior drift for in-flight items on engine swap (see Rabbit Holes) — the highest risk; this is the state-machine gate in the file with complexity 41.
- Soak evidence for nudge flags may not exist in a queryable form (`diagnose_nudge_session` outcomes, notification history); D may end as "keep off + explicit exit criteria."
- Live-instance testing constraints (cannot restart the service).

## Observability Requirements

- Log (structured, slog) which engine is active at startup and each denied transition with `from`, `to`, `reason` (config-denied vs. gate-failed).
- Counter for transitions denied by configured graph vs. default graph divergence during a shadow period (see Risk Control), exposed under the existing actuator pattern (`/actuator/*` like git-backend).
- Quota gate decision log already exists; add the effective config values to the startup/`Using ...` log line.
- `allow_auto_merge` lookup: counter for success/failure/cached, so "unknown" rate is measurable.
- Alert condition: any shadow divergence >0 on a default-seeded config is a bug.

## Risk Control

- **Shadow mode first** for workstream A: run `ConfiguredWorkflowEngine` alongside `DefaultWorkflowEngine`, enforce the default, log/count divergences (mirrors the git-backend cohort shadow-soak pattern, `docs/how-to/flip-git-backend-cohort.md`). Flip enforcement via live-settable feature flags (one per caller class: operator, agent, system; global plus per-scope where the scope vocabulary allows), revert = flip back.
- Seed config must be byte-for-byte equivalent to `validTransitions`; a test asserts equivalence.
- Quota/budget and default flips (B, D) are live-settable; rollback is a flag flip, no deploy.
- C is additive (new populated field, no behavior change to PR creation unless warning is enabled).

## Roadmap fit

This project is roadmap item 1 of the four items produced by the backlog cohesion audit (the audit ran in the session that created this project; no roadmap document was committed, and none is created here: `grep -n -i roadmap docs/tasks/backlog-feature-improvement.md` returns 0 hits). The four items: (1) make the advertised configuration true (this project); (2) respawn/remediation audit trail (`session-lifecycle-ux`, `status-visibility`); (3) backward GitHub-to-item sync, `github-autonomous-fix`, retry-escalation policy, adaptive turn budget, merge-conflict auto-resolution, bulk approve/retry; (4) plan-doc housekeeping and finishing `no_ent_in_services`.

Why item 1 comes first (INFERRED, owner may overrule): items 2 and 3 each add automation or an audit view on top of status transitions and gates, so they are only trustworthy if a transition means what the stages UI says it means; item 2's audit trail has one place to hook only after the choke point (`Storage.GatedTransition`, ADR-007) exists, and item 3's retry-escalation and bulk approve would otherwise act on gates the engine ignores. Item 4 is debt with no dependency on 1 and is deliberately last. An informal ICE read (Impact high: settings that lie destroy trust; Confidence medium-high: code verified; Ease low: about 17.5M CU) is recorded here rather than a scored RICE, because Reach is one operator.

## Open Questions

1. Does any path other than `TransitionBacklogItemStatus` enforce transitions/gates (autonomous completion, MCP tool, lifecycle)? (research) — Answered: no; 40 other status-write call sites exist and none consults the engine (ADR-007). They are being routed through it.
2. What does `ConfiguredWorkflowEngine` do on missing/invalid config today, and does it already fall back to the default graph? (research)
3. Where does soak evidence for `diagnose_nudge`/`autonomous_stall_nudge` live (notification history, `diagnose_nudge_session` results, logs)? (research)
4. Is `quota_headroom` budget derivable from observed usage instead of a user-entered number? (research)
5. Owner: acceptable to flip any of the three flags to default-on once soak criteria are met, and what are the criteria? (owner decision, wall-clock blocker)

*Owner decisions recorded 2026-10-17 (no longer open):* soak parameters W, N, floor and the low-volume READY rule (see Wall-clock blockers); archive-from-active edges (`in_progress|review|pr_pending->archived`, `archived->archived`, custom-slug archive) are documented as UI-ungateable (option A, the default): an operator who needs a gate there attaches it to `done->archived`.
