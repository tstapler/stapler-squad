# Cross-Artifact Consistency Review: backlog-flow-observability

Date: 2026-11-05. Artifacts read in full: `requirements.md`, `implementation/plan.md`, `design/ux.md`, `decisions/ADR-001..005`. Method: read and grep; DB-backed numbers were not re-queried (INFERRED from the artifacts' own figures). Plan task/story/CU arithmetic was recounted with awk and holds (44 stories, 50 tasks, 19 M / 26 S / 5 XS, 4,000k raw, 10,494k CU).

Summary: 23 findings (6 blockers, 12 concerns, 5 nitpicks).

## BLOCKER

**B1. Size band contradicts.** `requirements.md:30` says "Medium (0.5-3M CU)"; `plan.md` (Effort estimate) says about 10.5M CU, "Large (3-15M)", "noted and proceeding". Resolution: update requirements Size to Large (about 10.5M CU) so the two artifacts agree.

**B2. Deadlock definition contradicts.** `requirements.md:25` success metric (d) defines candidates as "repeated FAIL/PARTIAL verdicts and agent-claimed completion, or cap exhausted". ADR-005 and plan Story 1.3.2 deliberately make `agent_claimed_done` informational (true for 95.7% of non-PASS) and gate on 3+ judgment non-PASS verdicts plus a no-progress signal. Resolution: rewrite requirements (d) and the In Scope deadlock bullet to the ADR-005 definition.

**B3. Coverage gap: CI failure vs merge conflict as separate rework causes.** In Scope (`requirements.md:51`) lists "PR CI failure, merge conflict" as separate causes. Plan Phases 1-4 ship one `PRFix` bucket (`pr_fix_cause:unavailable`); the split exists only from Story 5.2.3 forward and is still an open owner decision in plan Unresolved Questions. Requirements do not acknowledge the gap. Resolution: get the owner to confirm 5.2.3, and record the single-bucket limitation in requirements (or move the split to a stated follow-up).

**B4. Coverage gap: merge-conflict and CI-failure dwell.** `requirements.md:53` (PR-health analysis) requires "merge-conflict and CI-failure dwell". No plan story computes dwell per PR-fix trigger; `pr_needs_fix` stuck time (Story 1.4.1) is the nearest signal and is not mapped to it. Resolution: add an AC to 4.2.3 or 5.2.3 for per-trigger dwell, or state that stuck time for `pr_needs_fix` stands in for it.

**B5. Adjusted FAIL rate in UX contradicts plan and ADR-005.** `ux.md:105,142` show adjusted `84/426 (19.7%)` next to "98 no committed changes and 34 self-check failures excluded". 186-84 = 102, and 528-426 = 102, which is the "102" figure the plan repair log says was removed (Repair log C4). Per plan Story 1.3.1 (NoCommits, SelfCheckFailed and AutoVerified excluded), FAIL falls by 98 to 88, and the denominator is 528-98-34-23 = 373 (88/373 = 23.6%) or 396 if auto-verified stays in (22.2%). Computed from the artifacts' own counts, not re-queried. Resolution: recompute the mockup values (and say whether AutoVerified is excluded) in S5 and S8.

**B6. Verdict totals disagree across artifacts.** `requirements.md:13` has 526 rows (PASS 149, UNVERIFIABLE 109); plan, ADR-005 and fixtures have 528 (PASS 150, UNVERIFIABLE 110). `ux.md:143` prints `UNVERIFIABLE 109/528 (20.6%)`, mixing the two; its verdict counts then sum to 527 (186+82+150+109). `ux.md:231` logs `rows=526`. Resolution: use one dated snapshot (528 / 150 / 110, 110/528 = 20.8%) in requirements and UX, or label requirements' figures as a superseded earlier read.

## CONCERN

**C1. Requirements still cite `analytics.db`/`stage_transitions` as a reuse target.** `requirements.md:68` (Rabbit Holes) says "reuse ... `analytics.db` `stage_transitions`", while Open Question 1 (`:87`) resolves that `stage_transitions` is pipeline config and `analytics.db` holds no backlog rows. Resolution: reword line 68 to `backlog_status_events` plus `backlog_stuck_states`, with the pipeline definition used only for stage order (as the plan does).

**C2. Requirements evidence numbers are stale or wrong against the plan.** (a) `:11` "Two items account for ~175k events" is unnamed; the plan names them (`d91477d5` 87,435 and `8a53ce39` 87,122, archived) and says `0791f5b4` (about 330 events) is not one of them, while Open Question 3 still presents `0791f5b4` as the dequeue-loop example. (b) `:11` has 257,973 event rows; ADR-004 says 257,988. (c) UX S11 and plan Story 6.1.1 depend on those names. Resolution: refresh requirements evidence to the plan's recount or date-stamp it.

**C3. "61 episodes" is used as the PRFix count, but the plan derived it as all-cause.** Plan Story 1.2.1 and ADR-002: 61 is all thrash episodes after the 1h merge across all causes and 35 items (dequeue and PR-fix). Plan 4.2.2/4.2.3 ACs and `ux.md:117,131` use it as "PRFix 61 episodes" and "pr_pending -> in_progress 4,437 -> 61 episodes". Resolution: re-derive a PRFix-only episode count (or label the mock values illustrative); keep one definition.

**C4. UX rework-cause mock contradicts the plan's cohort fixture.** `ux.md:131` shows Review FAIL rework 425 across 31 items. Plan Story 1.2.2's bounce distribution has 66 of 105 merged items with at least one bounce (sum of the non-zero buckets), so 31 items is impossible if the window includes the merged cohort. 31 also equals the deadlock-item count. Resolution: fix the mock or state it is illustrative (UX section 7 covers only thrash and deadlock numbers).

**C5. Three spellings of the heuristic flag in `estimated[]`.** UX S15 `deadlock_candidates`; plan Story 1.3.2 and 3.2.2 `deadlock_candidates:heuristic`; ADR-005 `estimated: heuristic`. Resolution: standardize on `deadlock_candidates:heuristic` in UX and ADR-005.

**C6. Section status enum mismatch.** UX S15 (`ux.md:223`) and S3 use `ok/failed/partial/empty` (and "timeout"); plan Glossary `SectionStatus` is `OK, Partial, Failed, Skipped`. `empty` is a UX requirement ("Empty sections return status:empty") with no plan equivalent. Resolution: add `Empty` to the plan enum (and Story 3.1.1/1.4.2 ACs) or drop it from UX.

**C7. MCP JSON field casing is unspecified.** Plan Story 3.2.1/3.2.3 require `as_of`, `row_counts`, `query_ms` in JSON produced by `protojson`; `protojson.Marshal` emits lowerCamelCase (`asOf`) unless `UseProtoNames: true`. The parity AC ("MCP JSON equals `protojson.Marshal` of the RPC response") and UX S15's snake_case example cannot both hold with default options. INFERRED from protojson defaults, not run. Resolution: state `UseProtoNames: true` in 3.2.1 and make the parity test use the same marshal options.

**C8. No defined input for an "all-time" window.** Plan Story 6.1.1, UX S4 and S11 rely on an all-time view (35 thrash items, `slow_path:true` in ADR-002), but the UI offers only 7/30/90 and `NewFlowWindow(clock, 0)` is an error (Story 1.1.1). MCP `window_days` has no all-time value. Resolution: define the all-time request (for example `window_days=0` meaning all-time via an explicit constructor) and say whether the UI exposes it.

**C9. "Bounce" vs "Review FAIL rework" vs `review>in_progress`.** Plan Glossary `ReworkCause` has both `ReviewFail` and `Bounce`, yet FPY and the bounce distribution (Story 1.2.2) count `review>in_progress` events, which is also what "Review FAIL rework 425" is (`requirements.md:14` "rework"). `bouncing` is also a stuck reason. Resolution: define Bounce versus ReviewFail precisely (which events feed each) in the glossary, or merge them.

**C10. `PRFixTrigger` enum differs.** Glossary lists `ci, conflict, review, feedback, unknown`; Story 5.2.3 records `ci|conflict|review|feedback|mixed`; UX S9 lists CI/conflict/review/feedback. `mixed` is missing from the glossary and UX. Resolution: add `mixed` to all three.

**C11. UX "Median lead time" tile has no backing metric in the plan.** `ux.md:40,103` show a lead-time tile with delta; no plan story computes lead time (creation or first in_progress to first merge). Funnel dwell and throughput exist, lead time does not. Resolution: add lead time to Story 1.2.2 (and a drill key) or remove the tile.

**C12. Debug log line format differs.** UX S16: `flow aggregate section=review window=30d rows=526 ms=41 status=ok`; plan Observability Plan: `window=<..> section=<..> rows=<n> query_ms=<n> cache=<hit|miss>`. Field names (`ms` vs `query_ms`) and `status` vs `cache` differ. Resolution: pick the plan's line and update UX S16 (and add `status` to the plan's warn path if wanted).

## NITPICK

**N1. UX header and page-state surfaces lack plan ACs.** S2 (window/compare RadioGroups, Refresh, jump list, LiveRegion, unsupported-window fallback), S1's error boundary link, and S3's "Widen to 30d" and `unavailable` states map to Story 4.1.2, whose ACs cover only no-fake-zero, stale/partial, URL state and server freshness (4.3.1 lists fixtures for most states but not the controls). Resolution: add ACs to 4.1.2 for the selectors, refresh, jump list and Widen button.

**N2. UX S9 inventory omits Story 5.2.2.** `ux.md:19` maps S9 to 4.2.3 and 5.2.3; the outcomes block (steered/respawned/skipped_active, `pr_fix_outcome:since_capture`, "not yet recording") is produced by 5.2.2, and 4.2.3 has no AC for the "not yet recording" no-numbers state. Resolution: add 5.2.2 to the S9 row and an AC for the pre-capture state.

**N3. UX S11 "View events" has no UI story.** The button implies `GetItemFlowTimeline`, which no Phase 4 story consumes (the RPC and MCP tool exist). Resolution: add it to 4.2.3b or drop the button.

**N4. Tool and RPC naming asymmetry, and UX names no tools.** Plan has six tools (`get_flow_summary`, `compare_flow_windows`, `list_flow_drilldown`, `list_flow_deadlock_candidates`, `list_flow_thrash_episodes`, `get_item_flow_timeline`) against six RPCs (`GetFlowSummary`, `CompareFlowWindows`, `ListFlowDrilldown`, `ListDeadlockCandidates`, `ListThrashEpisodes`, `GetItemFlowTimeline`); the list RPCs drop "Flow" while the tools add it. UX S15 describes the JSON but names no tool. The mapping is one-to-one, so there is no functional mismatch.

**N5. `capture_since` naming.** ADR-003 says `stuck_capture_since`; plan `FlowMeta.CaptureSince`/proto `capture_since`; UX S10 `capture_since`. Resolution: use `capture_since` in ADR-003.

## Checks that passed

- All In Scope items other than B3/B4 have at least one plan story (collapse: 1.2.1; metrics: 1.2.2/1.4.1; review gate: 1.3.x; UI: 4.x; MCP: 3.2.x; capture: 5.x; before/after: 1.4.2/3.2.1).
- Every plan story has a requirement or an ADR/review-derived reason; 1.1.3 (depguard), 4.1.3/4.2.5 (FlowChart) trace to ux/research, not requirements (no scope creep beyond ADR-backed work).
- Non-judgment counts 132 = 98 + 34 and 18 degraded are consistent in plan, ADR-005 and UX S8; 31/27 deadlock figures consistent in plan and ADR-005; 103/61/35 episode and item figures consistent in plan, ADR-002 and UX S4.
- UX inventory story IDs 4.1.1-4.3.3 all exist in plan Phase 4; drill keys and `estimated` tags `stuck_history:lower_bound`, `pr_fix_cause:*`, `pr_fix_outcome:since_capture` match.
- Requirements Open Questions 1, 2 and 4 agree with plan (dwell source, 95.7% claim, `/backlog/flow`).
