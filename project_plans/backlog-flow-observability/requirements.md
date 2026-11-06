# Requirements: backlog-flow-observability

**Date**: 2026-11-05
**Type**: feature addition (observability: aggregation backend + web view + MCP tools)
**Complexity**: 3 — system design (multiple epics, spans DB queries, RPC, UI, MCP)

## Problem Statement
Backlog items do not flow through the pipeline like an autonomous software factory. Many park in states that need manual nudges: PR CI failures, merge conflicts, review-gate deadlocks where the reviewer rejects and the agent believes it is done (so it gives up or the rework cap trips). There is no way to see, at the system level, where items stall, why, how much rework occurs, or whether the tuning changes already shipped (stuck-item detection, bounce escalation, rework caps, PR-fix steering) actually help. Per-item stuck visibility exists (`project_plans/backlog-stuck-item-visibility`); aggregate flow analytics for the human **and for Claude via MCP** do not. Without that data, process tuning is guesswork.

Live evidence (workspace DB `~/.stapler-squad/workspaces/6eb0b580fa0331d5/sessions.db`; one authoritative snapshot, re-read with `sqlite3 -readonly` for the Phase 4 patch of 2026-11-05; live counts drift, so tests assert shapes and Story 6.1.1 matches within drift):
- `backlog_status_events`: 257,988 rows, of which 238,553 are `ready<->in_progress` flips with notes "dequeued: WIP slot freed" / "dequeue spawn failed". Two items account for ~175k events (`d91477d5` 87,435 and `8a53ce39` 87,122, both archived). Raw status history is therefore dominated by one thrash loop and is unusable for flow analysis without noise collapse. The thrash itself is a defect worth surfacing.
- `backlog_stuck_states` reasons seen (all-time): autonomous_stuck 97, multiple_reasons 90, respawn_blocked_active 80, pr_needs_fix 79, bouncing 71, abandoned_review 65, stale_work 55, orphaned_triage 53, pr_ready_unmerged 50, verdict_undelivered 16, push_failed 12, plan_not_approved 11, likely_flaky 11, bounce_cap_exhausted 7, rework_blocked_stale 6, and others. Only 8 currently unresolved.
- `review_verdicts`: 528 rows (FAIL 186, PARTIAL 82, PASS 150, UNVERIFIABLE 110); 5 human overrides. 378 are non-PASS; 132 of those are non-judgments (98 "no committed changes" FAILs plus 34 self-check UNVERIFIABLEs) and 18 more UNVERIFIABLEs are reviewer-degraded, so 150 of 378 are not real review judgments; 23 PASSes are synthetic auto-verifications. The raw 35.2% FAIL rate and 20.8% UNVERIFIABLE rate therefore overstate reviewer strictness: excluding all of those, FAIL is 88 of 355 judgment verdicts (24.8%). Per-item measurement is the point of this feature.
- Real transitions: `in_progress>review` 636, `review>in_progress` 426 (rework: 398 auto-reopened after a failed verdict, 28 other), `pr_pending>in_progress` 4,437 (auto-reopen for PR fix; 4,341 of them sit inside thrash episodes and 96 are real reopens), `pr_pending>done` 106, `done>archived` 160.
- Current items (161): archived 153, done 1, idea 2, ready 3, review 2. Thrash, all-cause, by simulation of the ADR-002 rule: 103 episodes across 35 of 160 items that have events at the 15-minute gap, 61 after the 1h merge (dequeue loops 54, PR-fix loop 7 across 3 items). Do not read 61 as the PR-fix count.

## Baseline
Today the owner reads raw sqlite tables, server logs, and per-item pages by hand (as done to produce the numbers above). The Board (`/backlog`) shows current state only. No aggregate funnel, no rework/cycle-time metrics, no cause attribution, and no MCP tool lets Claude query flow data (existing MCP backlog tools in `server/mcp/tools_backlog.go` are per-item: get/list/create/post_update/wait_for_event).

### Outcome baseline (the falsifiable "before")
Re-derived 2026-11-05 with `sqlite3 -readonly` on the workspace DB above (live counts drift; Story 6.1.1 re-derives them with the shipped definitions and records the delta). Rough definitions, not yet the plan's FPY (which also requires zero thrash episodes and keys on the first `pr_pending>done`):

| Outcome | Baseline | How derived |
|---|---|---|
| Done items with zero review rework | 48 of 137 items that ever reached `done` (35.0%) | items with a `to_status='done'` event and no `review>in_progress` event |
| Same, also counting PR-fix reopens | 40 of 137 (29.2%), a lower bound on the zero-rework share | adds `pr_pending>in_progress` events as rework; contaminated by thrash because the reopen note is one constant string, so it is a floor, not a measurement |
| Review bounces per done item | median 2 (mean 2.74, max 24); median 3 among the 89 items that bounced at least once | count of `review>in_progress` events per item |
| Non-judgment share of non-PASS verdicts | 150 of 378 (39.7%): 98 "no committed changes" FAILs (re-verified today) + 34 self-check UNVERIFIABLEs + 18 reviewer-degraded UNVERIFIABLEs (the 34 and 18 are from the Phase 4 derivation and were not re-run today) | `review_verdicts.summary` classes per ADR-005; total verdicts today are 529 (528 in the snapshot above, drift) |
| Manual nudges per week | UNMEASURED: no source records them (`triggered_by` is 99.8% `system`) | recorded as a gap; the feature does not claim to reduce it |

The problem statement is therefore falsified or confirmed on two measurable outcomes (zero-rework share of done items, non-judgment share of review failures), not only on "the owner can now answer a-f without SQL".

## Users / Consumers
1. Tyler, via a web UI view of system-level flow.
2. Claude sessions (including the improvement loop that tunes the factory), via MCP tools returning the same aggregates as structured JSON.

## Success Metrics
- Answers, from one view and one MCP call each, with no manual SQL: (a) where do items spend time per stage; (b) how many review/rework/PR-fix cycles per merged item; (c) top stuck reasons, current vs historical, with time-in-stuck; (d) reviewer-vs-agent deadlock candidates, defined per ADR-005 (items with a run of at least 3 consecutive judgment non-PASS verdicts plus a no-progress signal, the same failing criterion or an unchanged diff, or cap exhausted; `agent_claimed_done` is displayed as information only because it is true for 95.7% of non-PASS criterion verdicts and so does not discriminate; the filter is weak and is labeled heuristic, with a precision check before ship; the display name is "Review-fail streaks (heuristic)" and is promoted to "Deadlock candidates" only if the hand-labeled hit rate is at least 50%); (e) first-pass yield (share of items reaching done with zero rework); (f) thrash loops (items with abnormal transition rates).
- Re-running today's investigation (the queries above) via the new MCP tools reproduces those numbers, and the thrash items surface as anomalies rather than drowning the totals.
- After a tuning change ships, a before/after comparison over a time window is available in one call (the data needed to judge whether it worked), anchored on an absolute date, not only a day offset.
- **Outcome metric (measured after ship, not at build)**: the zero-rework share of done items (the shipped first-pass yield) and the non-judgment share of review failures, compared against the baseline table above. Direction wanted: first-pass yield up, non-judgment share down; no numeric target is set because the baseline sample is small (see minimum-sample rule).
- **Post-ship checkpoint (dated once its blockers resolve)**: 30 days after the later of the read-layer PR (PR-B) merging and the backoff item (plan Task 6.1.2a) landing; the backoff item gates the checkpoint, so there is no calendar date until both blockers are named as resolved. By then a real `compare_flow_windows` call around a shipped tuning change has been run and a tuning decision (act on hypothesis X, or explicitly "no change") has been recorded. Tracked as a real backlog item filed by plan Task 6.1.2c with the due date written in its body once both blockers have resolved; the feature counts as having worked only if that item has a recorded decision. **Window choice and fallback**: the compare anchors on the shipped tuning change's date T with 30-day windows A `[T-30d, T)` and B `[T, T+30d)`; if the primary metrics come back `incomparable` it widens both windows to 60 then 90 days, and if still `incomparable` it falls back to a single-window read of B against the baseline below. A decision that cites an `incomparable` compare plus that fallback read counts as evidence; only no recorded decision falsifies the meta-assumption, so sample size alone cannot falsify it. **Baseline for the checkpoint**: the shipped-definition numbers re-derived in plan Story 6.1.1 (zero-thrash first-pass yield, bounces, non-judgment share, each with N), not the rough SQL table in "Outcome baseline", which is thrash-contaminated. If it does not, the meta-assumption below is falsified and the owner decides whether to keep investing.
- **Minimum-sample rule for window comparisons**: a metric is compared as a number only when both windows have at least 20 items (per-item metrics: first-pass yield, lead time, bounces) or 30 verdicts (verdict rates). Below that, `compare_flow_windows` returns that metric as `incomparable` with reason `low sample (N_A=<n>, N_B=<n>, need 20)`, still prints the raw values and N, and never computes or implies significance (no "improved", "worse", p-value or confidence wording in UI or MCP output; deltas are descriptive). Evidence the rule bites on real data (approximate: string-ordered timestamps, `sqlite3 -readonly`, 2026-11-05): 26 items reached `pr_pending>done` in the last 30 days but only 8 in the 30 days before that, so a naive 30-vs-30 compare of first-pass yield would be `incomparable` today; verdict counts are 154 and 53.

## Size
Large (3-15M CU), about 12.4M CU (12,438k) per `implementation/plan.md` (Effort estimate, recounted in the Final triad patch) — INFERRED, see ESTIMATION.md. The request implied Medium; full scope is kept (this is a re-estimate, not a downscale) and the band is not a gate.

**Wall-clock blockers**: owner decision on which tuning hypotheses to act on after the first data read (second pass); a soak window of real item flow after instrumentation lands to accumulate post-change data. None block this pass's build.

## Constraints
- Single-user self-hosted instance; the live service on `:8543` must not be restarted for testing (use a manual instance per CLAUDE.md).
- Read-mostly aggregation over existing tables first; add new capture only where existing data cannot answer a question (e.g. agent "I'm done" claims vs verdict, PR-fix nudge outcomes).
- New RPCs and React pages need feature-registry entries and `tested:true` + testIds (CI gate); MCP tools need tests alongside `tools_backlog_test.go`.
- ent schema changes: use `--feature sql/upsert`, do not commit generated output.
- Web-app uses pnpm; vanilla-extract CSS; e2e conventions apply.
- Must tolerate high-volume event tables (260k rows and growing): aggregation must be indexed/bounded, not full scans per page load.

## Non-functional Requirements
- **Performance SLO**: flow summary for a 30-day window returns in < 1s on current data volume.
- **Scalability**: tens of items, hundreds of thousands of status events; thrash-collapse prevents event growth from degrading views.
- **Security classification**: internal.
- **Data residency**: local only.

## Scope
### In Scope
- Noise-collapsing event model: collapse repeated dequeue/spawn-fail flips into a single "thrash episode" with count and duration; flag as an anomaly.
- Aggregate flow metrics: stage dwell time, funnel/throughput, rework loop counts by cause (review FAIL, PR CI failure, merge conflict, bounce), first-pass yield, median lead time, time-in-stuck by reason. **Known gap**: status events do not record which PR-fix trigger fired, so Phases 1-4 report one combined PR-fix bucket (`estimated: pr_fix_cause:unavailable`); the CI-vs-conflict split is captured forward-only by plan Story 5.2.3 (in scope, no backfill).
- Review-gate analysis: verdict outcome mix over time, per-item verdict sequences, override rate, UNVERIFIABLE rate, and a "deadlock candidate" heuristic as defined in ADR-005 (judgment-run plus no-progress signal; default top-10 list with a stated weak-filter note; hand-labeled precision check before ship).
- PR-health analysis: auto-reopen-for-PR-fix counts, nudge-to-resolution outcomes, and PR-fix dwell. The requirement is merge-conflict and CI-failure dwell separately; Phases 1-4 deliver combined PR-fix dwell (plan Story 1.2.3, labeled estimated), and the split by trigger exists only for cycles after the Story 5.2.3 capture date (`pr_fix_dwell_split:since_capture`).
- Web view (new "Flow" view alongside Board) showing the above for a selectable time window, with drill-down to the items behind each number.
- MCP tools exposing the same aggregates (and per-item timelines with thrash collapsed) as JSON, plus a before/after window comparison.
- Capture gaps identified during research (only if existing data is insufficient).

### Out of Scope
- Changing pipeline behavior, thresholds, reviewer prompts, or rework caps (second pass, driven by this pass's data).
- Fixing the ready<->in_progress thrash root cause (surface it here; fix as a separate backlog item unless research shows it is trivial).
- Cross-instance / multi-workspace aggregation.
- Alerting/paging beyond what stuck-state notifications already do.

## Rabbit Holes
- Defining "reviewer too strict / agent thinks done" from data: verdicts exist, but agent completion claims may not be captured structurally. Research must find what signal exists (progress notes, activity notes, session end state) before inventing capture.
- Time-in-stage across the 238k-flip noise: dwell must be computed over collapsed episodes or numbers are meaningless.
- Multiple workspace DBs (`4a9afb0cab51f0cb`, `6eb0b580fa0331d5`, legacy root `sessions.db` which is empty for backlog): the view must read whichever is live for the instance.
- Overlap with existing `backlog_stuck_states` and `backlog_item_activity_log` plans: reuse `backlog_status_events` plus `backlog_stuck_states` (the pipeline definition is used only for stage order); do not build a parallel store. `analytics.db` and `stage_transitions` are not reuse targets (see Open Question 1).

## Alternatives Considered
- Export tables to an external BI/Grafana (repo has `observability-grafana-dashboards` skill): richer charts but not exposed to Claude via the app's MCP and adds infra. Keep as a later option.
- Compute everything on read vs. materialized rollups: decide in research/plan based on the SLO.
- A one-off analysis script/report: rejected as the primary deliverable because the stated goal is a standing view plus MCP access for iterative tuning.

## Feasibility Risks
- The `stapler-squad` MCP server failed to connect in this session (ECONNREFUSED), so MCP tool design must be verified against `server/mcp` code and tests, not the live endpoint.
- Event semantics may be ambiguous (`triggered_by` is 99.8% `system`); cause attribution may need the `note` text, which is free-form.
- Verdicts link via `item_sessions`, not directly to items; joins need care.

## Risky Assumptions
1. **Meta-assumption: flow data will change tuning decisions.** If the owner reads the view but no tuning decision ever cites it, the feature is a dashboard nobody acts on. Tested by the dated post-ship checkpoint (Success Metrics): a backlog item (plan Task 6.1.2c) must hold a recorded decision that cites a `compare_flow_windows` result or a decision-rule row below. Falsifier: no such decision by the checkpoint start rule's 30 days (not an `incomparable` result on its own).
2. **The deadlock heuristic discriminates** (ADR-005, pre-mortem P1 #1). Tested by the hand-labeled top-10 precision check in Story 6.1.1; below 50% the section keeps the name "Review-fail streaks (heuristic)", at or above 50% it is promoted to "Deadlock candidates".
3. **The sample is large enough to compare windows.** About 160 items and 35 thrash items; the minimum-sample rule above makes the tool refuse rather than overclaim.

## Second-pass decision rules (hypotheses, not conclusions)
Each row names the metric movement that should trigger a tuning hypothesis for the second pass. They are starting points to test with a before/after compare, not diagnoses.

| Observed movement | Second-pass hypothesis | First action |
|---|---|---|
| High review-bounce rate (median bounces per done item at or above 2) and high non-judgment share (above 30% of non-PASS) | The reviewer is failing for reasons unrelated to the diff (no commit, self-check, degraded) | Review reviewer inputs: commit-capture before review, diff computation, acceptance criteria presence |
| High bounce rate and low non-judgment share | Agents produce work reviewers legitimately reject | Review agent prompts and rework guidance; check same-criterion repeats in review-fail streaks |
| Many PR-fix reopens per merged item, or long combined PR-fix dwell | The PR-fix nudge or backoff policy is too eager or too slow | Review nudge and backoff policy; after Story 5.2.3 capture, split by CI vs conflict |
| Review-fail streaks of 3+ with the same criterion, rising or concentrated in few items | Reviewer criteria are unachievable or ambiguous for those items | Review the criteria text and caps for the top streak items |
| Thrash episodes persist or recur (anomalies section non-empty after the backoff fix) | Spawn or reopen backoff is incomplete | Reopen the spawn-failure backoff item |
| Open stuck reasons dominated by one reason with long P90 time-in-stuck | That stuck detector or its remediation lags | Look at that reason's episodes first |
| First-pass yield flat while non-judgment share falls | Non-judgment fixes did not translate to fewer bounces | Re-check classification (`unrecognized` count) before concluding |

## Ordering relative to other backlog-improvement work
1. The thrash and PR-fix backoff fix (filed by plan Task 6.1.2a) goes first among backlog-improvement items (owner: Tyler's backlog; it gates the checkpoint and carries no calendar date): until it lands, 8,691 events on one item and the dequeue loops keep inflating the event table and the single SQLite writer's load, and the thrash exclusion rather than the data carries every number. This feature's build does not depend on it (collapse and the row ceiling bound the impact), so the build proceeds in parallel.
2. The first before/after compare is taken after the backoff fix ships, so a tuning change is not confounded with the loop stopping.
3. The commit-capture item (Task 6.1.2b) follows, since it strengthens the deadlock heuristic's second signal.
4. Everything else in the decision-rule table is sequenced from the first real data read.

## Observability Requirements
The feature is itself observability. Additionally: log aggregation query durations and row counts at debug level; surface data freshness (as-of timestamp) in both UI and MCP responses; mark partial/estimated metrics explicitly.

## Risk Control
Read-only queries plus additive schema (if any); new RPCs and MCP tools are additive. No feature flag needed; rollback is reverting the PR. Index additions must be verified on the live-size dataset.

## Open Questions
1. ~~Dwell source~~ RESOLVED (research): `backlog_status_events` only; `stage_transitions` is pipeline config and `analytics.db` holds no backlog rows.
2. ~~Agent "believes done"~~ RESOLVED: latest `backlog_progress_notes` status `done` on 95.7% of non-PASS criterion verdicts; also `request_review` notes and `item_sessions.verification_notes`. `end_reason` is unusable.
3. ~~Thrash still occurring~~ RESOLVED: yes, far below peak (dequeue loop on `0791f5b4`, about 330 events, a small offender and not one of the two ~175k-event items; PR-fix reopen loop on `a90b99cb`, 8,691 events, same worktree-failure root cause, no backoff).
4. ~~Placement~~ RESOLVED: `/backlog/flow`, third tab.
5. Retention of `backlog_status_events` (pruned or not) *(unresolved after Phase 2 research)*; plan must not depend on it.
