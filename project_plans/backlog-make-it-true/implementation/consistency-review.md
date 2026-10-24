# Cross-Artifact Consistency Review: backlog-make-it-true

**Date**: 2026-10-17
**Artifacts checked**: `requirements.md`, `implementation/plan.md`, `design/ux.md`, `decisions/ADR-001` through `ADR-006`.
**Result**: 21 findings: 3 BLOCKER, 6 CONCERN, 12 NITPICK. No artifact was edited.

## Recounts (by command, all consistent)

| Quantity | Value | Command basis |
|---|---|---|
| Stories | 30 | `grep -cE '^#### Story '` on plan.md; effort table has 30 rows; wave table lists the same 30 once each |
| Task headings | 67 (6 XS, 35 S, 26 M); all carry a size | `grep -cE '^##### Task '`; the plan's "65 sized + 2 unlisted (1.2.5a S, 4.1.5a XS)" matches (26 M, 34 S, 5 XS) |
| Per-story task counts | Match the effort table for every story | awk over headings |
| Waves | 16 (W0 to W15), max 2 stories each | wave table rows |
| Bypass inventory | 38 sites = 1 wrapper + 1 MCP delegate + 5 engine-routed + 31 bypass in 10 files; per-file counts sum to 31; task split 7 + 12 + 12 = 31 | plan Story 1.2.3, ADR-002 |
| UX acceptance criteria | 34 distinct `UX-nn`, "Total: 34" is correct | grep on ux.md |
| Prerequisites vs waves | Every story's prerequisites are in an earlier wave | read against the wave table |
| Terminology checked with no drift | `enforceAnswer` (single name; no "enforce-composed" variant anywhere), `gate_missing` / `gate_extra` meanings (plan glossary = ADR-001 = Stories 1.1.2/1.1.3), mode names off/shadow/enforce, flag names `backlog:workflow-engine-shadow` / `-enforce`, `AutoMergeLookupStatus` values (plan = ADR-003 = ux.md), `SystemRequiredEdges()` / `SystemRequiredEdge` (the `system_required` string appears only as the rejected DB-column alternative) |

## BLOCKER

**B1. requirements.md Success Metric 1 contradicts plan "Success-metric scoping" and ADR-002.**
- requirements.md line 34: "Changing a transition or gate in the stages UI changes the result of `TransitionBacklogItemStatus` ... without a restart." The plan (lines 14-17) and ADR-002 say editing changes nothing by default (shadow on, enforce off), the effect holds only on engine-routed paths and only after the owner flips enforce, and 31 direct-storage sites never follow config.
- Status: the plan already flags it and lists it under Unresolved Questions awaiting owner approval, so the artifact set is contradictory until that wording is approved.
- Fix: owner approves the restated Metric 1 given in plan line 17; then update requirements.md (not done here).

**B2. requirements.md Scope A and Rabbit Hole "Gate enforcement coverage" contradict ADR-002 and the shipped default.**
- requirements.md line 70: "Make `ConfiguredWorkflowEngine` the engine `BacklogService` uses for transitions and gates." Plan/ADR-001: `BacklogService` holds a `SwitchableWorkflowEngine` that returns the Default engine's answer in the shipped default (shadow).
- requirements.md line 85: "Each path must route through the same engine or the config is still partly fiction." ADR-002 Decision 1 deliberately scopes out 31 direct-storage sites (counted, not governed).
- Fix: owner-approved rewording of Scope A ("selector over Default and Configured, enforce behind a flag; direct-storage sites measured, not governed") and the Rabbit Hole bullet; cite ADR-001/002.

**B3. ADR-006 / Story 5.1.1 sequencing contradicts the wave plan, leaving the proof test red mid-plan.**
- ADR-006 Consequences: "Each new surface ... lands with its registry entry in the same story." Plan: the proof test enumerates `QuotaConfig` JSON tags, `knownFeatureFlags` and request fields by reflection and fails on any field without an entry, and exists from W1 (Story 5.1.1). But Story 5.1.1 pre-seeds only stage-config CRUD fields as "pending" (line 767); entries for pre-existing `QuotaConfig` fields land in W2 (2.1.1b); the two new mode flags (added W3, Task 1.1.3c) and `DefaultCostBudgetThreshold` (added W4, Task 2.1.3a) get entries only in Story 5.1.2 (W12). Existing `knownFeatureFlags` names have no entry at all.
- Effect: `TestConfigSurfaceProofs` fails on `make test` from W1 (existing quota fields/flags) and W3-W11 (new surfaces), contradicting "green from day one" and the same-story rule.
- Fix: either (a) give 5.1.1 a ratcheted `pending` list that includes all existing quota fields, existing flags and (explicitly) the new surfaces, shrunk by each registering story, or (b) move the entry tasks into the story that adds each surface (1.1.3, 2.1.3, 3.1.2) and fix the wave table.

## CONCERN

**C1. Field name drift: `DefaultCostBudgetThreshold` vs `DefaultCostBudgetThresholdUSD`.** plan glossary, Story 2.1.3, Migration Plan use `DefaultCostBudgetThreshold`; ADR-004 (decision 4, consequences) and ADR-006 (enumeration list) use `DefaultCostBudgetThresholdUSD`; the proto field is `default_cost_budget_threshold_usd`. ADR-006's reflection test keys on names. Fix: pick one Go field name (suggest `DefaultCostBudgetThresholdUSD`, matching the existing `cost_budget_threshold_usd` unit suffix) and update the plan glossary and stories.

**C2. UX validation range contradicts server validation.** ux.md section 4: "thresholds 0-100". Plan Story 2.1.1 and ADR-004 decision 2: `PauseBelowHeadroomPct=0` is rejected (zero would be silently replaced by the default). Fix: ux.md range for the pause threshold becomes 1-100 (resume margin per `QuotaConfigOrDefault` semantics), with matching message text.

**C3. UX-07 and the StageForm failure states have no plan task.** ux.md sections 2 and 6 and UX-07 require a system-required edge rejection shown with edge name and reason (`role="alert"`, values kept, Cancel exit) and a save-failure alert with Retry in StageForm. Story 1.2.1 covers only the server rejection; Story 1.3.2 and Task 1.3.2b cover only the success message. Fix: add ACs/a task under Story 1.3.2 (or 1.2.1) for the rejection and save-failure rendering.

**C4. PR-create warning has no cache-warming path.** requirements.md Scope C: warn "before/at PR creation". Story 3.1.4 reads only the cache with no synchronous call, and the only cache populator in the plan is `GetBacklogStuckReport` (Story 3.1.2). A PR created before anyone views the stuck panel never warns. Fix: add a task to start an async `RepoSettings` fetch at PR creation (warms the cache for the next PR) or state in Story 3.1.4 that the warning only fires for repos already looked up.

**C5. UX acceptance depends on cuttable tasks.** UX-09, 10, 13, 14, 17 and the UX-32/UX-34 Axe/touch-target gates come from collateral-debt Tasks 4.1.4a and 4.1.4d, which the plan marks "optional, cut first". If cut, ux.md criteria and the CI Axe gate on the Features page are unmet, and ux.md does not say they are conditional. Also Story 6.1.1 has no Playwright spec for Features-page a11y (UX-09 to UX-17 beyond the engine flag toggle). Fix: mark those UX criteria conditional in ux.md, or promote the tasks.

**C6. Requirements Observability item has no task.** requirements.md line 108 and plan Observability Plan promise the quota startup line extended with effective `QuotaConfig` values and `QuotaMode`; no story or task implements it. Fix: add a one-line task (for example under Story 2.1.2).

## NITPICK

1. **"unknown" wording.** requirements.md Metric 3 says "unknown appears only when the lookup genuinely fails"; ux.md/Story 3.1.3 retire the word. ADR-003 also uses `unknown` as an internal short-backoff cache label. Align Metric 3 and relabel the ADR-003 cache state.
2. **Per-call DB round trip.** requirements.md NFR says no per-call DB round trip; ADR-001 Enforce evaluates stateful gates with DB reads (bounded by 2s, none in the seeded config). Add a one-line caveat to the NFR.
3. **Prerequisites omit `flagExtras`.** Stories 1.3.3 and 2.2.1 depend on Task 4.1.4b (`flagExtras.tsx`, W0) but list only 1.3.2 and 2.1.3 as prerequisites.
4. **ADR status vs content.** ADR-001/003/004 are "Accepted (planning)" while ADR-001 "Spike results" are `_pending_` and ADR-003 carries an UNVERIFIED item. Consider "Accepted, spike results pending". ADR-005 "Proposed" correctly matches Task 4.1.1a.
5. **ux.md names "Story 1.3.2b".** Section 6 cites it; 1.3.2b is a Task (Story 1.3.2). All other ux.md story IDs (1.3.2, 1.3.3, 2.2.1, 3.1.3, 3.1.4, 4.1.4) and Tasks 2.2.1b, 1.3.2a exist.
6. **ADR-003 task cross-reference.** ADR-003 points to the token-without-push test in "Task 3.1.1b"; plan places it in Task 3.1.1c.
7. **Duplicate Unresolved Questions.** plan lines 132/134 (quota unsaved-changes) and 131/135 (soft-budget routing) are each listed twice.
8. **Workstream count.** requirements.md says "4 workstreams" (lines 5, 42) but In Scope lists A to E (5); the plan uses 5 plus cross-cutting Phase 6.
9. **Mode name casing.** ADRs use `off/shadow/enforce`, plan `Off/Shadow/Enforce`, proto `SHADOW`, ux.md/banner "Enforced". Acceptable, but say once in the glossary that the banner label "Enforced" is the display name for `Enforce`.
10. **Exported-looking field in AC.** Story 3.1.1 AC says `AllowAutoMerge=false`, while the glossary makes the bool unexported behind `AutoMerge()`. Reword the AC to `AutoMerge() == (false, true)`.
11. **Sentinel package.** ADR-002 and glossary say `session.ErrTransitionNotAllowed`; Task 1.2.2a places the sentinel in `session/domain/backlog.go`. Name the package consistently.
12. **Preflight copy not asserted.** ux.md section 7 requires "PR will still be created" and the repo name; Story 3.1.4's ACs assert only count and dedupe key.

## ADR status summary

ADR-001, 002, 003, 004, 006: Accepted (planning). ADR-005: Proposed (promotion in Task 4.1.1a; owner decision for default flips). Plan header cites all six consistently.
