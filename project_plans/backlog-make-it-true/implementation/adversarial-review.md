# Adversarial Review: backlog-make-it-true

**Date**: 2026-10-17
**Verdict**: BLOCKED

Evidence labels: VERIFIED = I opened the file or ran the command in this worktree. INFERRED = reasoning from verified facts.

## Blockers

- [ ] **B1. The shadow divergence definition makes the soak criterion unsatisfiable, and it contradicts the plan's own harness.**
  - ADR-001 and Story 1.1.3 define `gate_missing` as "Default blocks, Configured passes". They compare Configured-alone against Default on identical input.
  - The seed writes zero `TransitionGate` rows (ADR-001, citing `session/workflow_stage_seed.go`). `DefaultWorkflowEngine.PendingGates` reports an unsatisfied structural gate whenever `TransitionGuard` fails (VERIFIED, `session/workflow_engine.go` PendingGates/ValidateGates). So every legitimate user or automation denial by a built-in guard counts as `gate_missing` under `config=seeded`. Examples: `idea->ready` with no acceptance criteria, `review->done` with no PASS verdict. Story 1.1.1 itself calls this "the documented gap".
  - The alert and exit rule is "`gate_missing=0` and `graph=0` under `config=seeded`" (Observability Plan, Story 1.4.1). `make workflow-engine-soak-status` therefore exits 1 on any ordinary guard denial. The READY verdict can only be reached by a window in which nobody is ever blocked, so the flip is gated on noise, or the owner learns to ignore the alarm.
  - This also defeats the requirements' alert condition "any shadow divergence >0 on a default-seeded config is a bug".
  - Recommendation: before Task 1.1.3a, change what shadow compares to **Default vs. the would-be-enforced composition** (Configured graph ∪ system-required edges, AND Default guards AND Configured gates). That is exactly what Enforce will return. Report Configured-alone gate coverage separately as informational (`gate_builtin_absent_in_config`), never as an alert class. Rewrite the Story 1.1.3 and 1.1.2 acceptance criteria and the runbook criteria to match. Reconcile the 1.1.1 "expected-failing gap" wording with this definition.

## Concerns

- [ ] **C1. Metric 1 is honestly met only at the handler/RPC/MCP/SyncLoop level, and only after the owner flips enforce.**
  - What holds: Story 1.2.4 proves allow->deny->allow through a real `BacklogService` with the selector. The scope-out is stated in ADR-002 and on the Stages page.
  - Shipped default: shadow on, enforce off, so editing stages still changes nothing at runtime until the owner flips after the soak. The plan's Definition of Done should say "Metric 1 is satisfied by the test under enforce. It is satisfied in production only after the owner-gated flip."
  - Wiring gap: Story 1.2.4 builds `BacklogService` in the test with the selector. It does not exercise `dependencies.go:1477`. The `:678` vs `:773` bug class is pinned only by Story 1.1.4 (wiring test). That is acceptable, but record explicitly that Metric 1 is proven by 1.2.4 plus 1.1.4 together.
  - Recommendation: add that sentence to the plan and to the PR template.

- [ ] **C2. The bypass surface is about 3x larger than the plan inventories, so tasks 1.2.3b/c and the ADR-002 narrative are understated.**
  - VERIFIED: `grep` for non-test `.TransitionBacklogItemStatus(` finds about 38 sites across 15 files, excluding `tools/lint`. Counts: `backlog_service_triage.go` 11, `backlog_lifecycle_pr.go` 5, `tools_backlog.go` 4, `backlog_lifecycle.go` 3, `jules_session_poller.go` 3, `backlog_service_lifecycle.go` 3, `backlog_service_trigger_triage.go` 2, plus single sites in `autonomous_orchestration_service.go`, `backlog_service_sync.go`, `backlog_debug_mutate_handler.go`, `backlog_lifecycle_archive.go`, `backlog_sync.go`, `storage.go`.
  - Tasks 1.2.3b/c name only about 11 sites (each sized S). The triage file alone has 10 direct storage calls beyond `transitionWithGuard` (lines 881, 1268, 1324, 2041, 2115, 2418, 2471, 2637, 2906, 3348).
  - The inventory test is located in `server/services` but must walk `session/` and `server/mcp` too, which is not stated.
  - Recommendation: either resize 1.2.3b/c to M, or make the counter a single wrapper at the storage layer with a `BypassCaller` argument. State in Task 1.2.3d that the `go/parser` walk is repo-wide.

- [ ] **C3. Stale or wrong `file:line` citations (spot-check results below).**
  - Task 1.2.2b and the Tech Debt table cite `backlog_service_triage.go:801-810`, `:2313-2321` and `session/backlog_lifecycle.go:971-984` as the "log-and-continue" `TransitionBacklogItemStatus` wrappers. At those lines the code is, respectively, a candidate-ID loop, the tail of `RemediateStaleWorkSession`, and an `alreadyEndedByOtherPath` early return. None is a transition call.
  - Only `backlog_lifecycle.go:1971-1975` matches (`transitionBouncingItemToDone`).
  - Story 1.2.2 therefore has no verified target list. This is the whole basis of "four call sites".
  - Recommendation: re-derive the call sites (candidates: triage 1268, 1324, 2041, 2418, 3348; lifecycle sites that wrap and only `log.Warn` on `transErr`). Fix the citations in `plan.md` and ADR-002 before dispatching 1.2.2.

- [ ] **C4. Soak evidence is thin by construction.**
  - Shadow data exists only on the live service (manual instance has its own DB, ADR-001 and the plan agree). The workspace has about 21 backlog items and only about 23 minutes of retained logs (VERIFIED in `research/build-vs-buy.md:250,266,280`).
  - Most real transitions bypass the engine (see C2), so "at least N shadowed calls on every used edge" cannot be met for lifecycle edges such as `review->pr_pending` and `pr_pending->done`. Reads such as `AllowedTransitions`/`GetPendingGates` on UI render inflate `shadow_calls_total` without being transitions.
  - With a seeded config, Configured≈Default is already proven offline across all 81 pairs by Story 1.1.1. The only live-only unknown is whether the live DB's stage config equals the seed.
  - Recommendation: add a one-shot task, run in the first deployed hour, that reads the live config and reports `SeedFingerprint` match plus the diff. Define the soak criterion over engine-routed edges only. Separate transition calls from read-path calls in the counters (`method` label is a start; add `path=transition|read`). Make the window and N an owner decision up front, not "INFERRED 7 days".

- [ ] **C5. `override_reason` vs Configured operator gates is left open, and ADR-001 says "unchanged".**
  - VERIFIED: the handler always calls `ValidateGates` with `OverrideReason` set (`backlog_service_lifecycle.go:~930-933`). Override is honored only inside `TransitionGuard` (`session/domain/backlog.go:693,714`). `ConfiguredWorkflowEngine.ValidateGates` (`:219-230`) never reads it.
  - In Enforce, an operator-added gate on an edge blocks even with `override_reason`, which is a regression of the "Manual Override" escape hatch (the gate's own `ActionHint` at `:210` promises it).
  - The plan lists this as an Unresolved Question blocking 1.1.2a. Resolve it as a decision (recommend: override bypasses Default guards as today, and operator gates also yield to override with the same audit note), then pin it in the characterization test, before implementation starts.

- [ ] **C6. ConfigSurfaceProof entries are registered before the framework exists.**
  - Tasks 1.2.4a and 2.1.1b register `ConfigSurfaceProof` entries in waves W2-W4. Story 5.1.1 (the registry, helper `assertConfigChangesBehavior`) is scheduled "after 1.1.2, 2.1.1 shapes" and in the wave list under W7 ("Epic 5.1 finish").
  - Either build the minimal registry and helper in W1/W2 (5.1.1 earlier) or drop the registration language from the early tasks.

- [ ] **C7. Wave and WIP-cap arithmetic is inconsistent.**
  - "Dependency Visualization" lists W0-W7 (eight waves). The effort section says "seven serial waves" W0-W6 with different story assignments.
  - W4 is "Stories 1.1.4 + 1.2.x || 3.1.1" (at least three stories, four counting 1.2.x as several). W5 is "1.3.x || 2.2.1 || ..." (three). The plan states max 2 concurrent stories and WIP cap 2.
  - Files that must serialize (`proto/session/v1/backlog.proto` has four editors: 1.2.2a, 1.3.1a, 3.1.2a, 4.1.2b) are in different waves. This is partly addressed but not reflected in wave contents.
  - Recommendation: regenerate waves from the dependency graph with the cap enforced and one number of waves. The CU total is unaffected, but wall-clock critical path is.

- [ ] **C8. Effort numbers.**
  - Arithmetic checks out (VERIFIED): 61 task headings (21 M, 31 S, 9 XS), 59 in the sum, 4.54M raw, x1.03 = 4.68M CU, x1.75 = 8.18M. Within the Large band.
  - Per-task sizing looks optimistic for: 1.1.2a (selector with mode, composition, fallback, override semantics, system edges and attribution is one M), 1.2.3b/c (C2), 4.1.4a/b (accessibility rewrite of a special-case page; two M), and 3.1.1b (cache with ETag retention, singleflight, negative caching, token cases is one M).
  - The x1.75 multiplier absorbs some of this. Say so, and keep the overrun checkpoint (25%) as the real control.

- [ ] **C9. Scope drift (moderate).**
  - Story 4.1.4 (switch semantics, 44px hit area, focus ring, `flagExtras` registry, source line) is about 315k raw, 7% of the project. The requirements ask only that "UI/docs must say" the AND-dependency of the nudge flags. The a11y rework is justified as collateral debt, which is defensible per the global memory, but it is not tied to a requirement or a metric.
  - Two persisted ledgers (workflow-engine soak, nudge outcomes), two actuator endpoints and a Make target are heavy for a single-operator service with about 21 items.
  - Recommendation: keep, but mark 4.1.4a/b as the "collateral debt" commit and cut it first if the overrun checkpoint trips (owner decides; I am not proposing a cut).

- [ ] **C10. Soft-budget "alters warning behavior" is log-only.**
  - `[BudgetWarning]` is a `log.WarningLog().Printf` (`server/services/backlog_service.go:1467,1489`). Metric 2 is about an operator-settable threshold and proven warning behavior. A log line is a weak operator-visible effect.
  - Recommendation: say so in Story 2.1.3, or route the crossing to an existing notification/stuck-panel surface.

## Minors

- Spot-check of cross-references (36 `file:line` claims opened; 27 correct, 4 wrong/stale, 5 point at code that is planned-new):
  - Correct (VERIFIED): `dependencies.go:678`, `:773-780`, `:1477`, `:1544`; `backlog_sync.go:57,69,449`; `workflow_engine.go:14`; `configured_workflow_engine.go:78-85,188,219`; `backlog_service_lifecycle.go:870,892,933`; `backlog_service_gates.go:148`; `actuator.go:31`; `config/types.go:743`; `config/config.go:375`; `autonomous_orchestration_service.go:586`; `tools_backlog.go:1507,1515,1682,1821`; `backlog_lifecycle_pr.go:601,819,848,1531,2183` (all five exact); `backlog_lifecycle.go:271-274` (stale comment), `:1925-1935` (comment says repo has `allow_auto_merge` disabled), `:1971-1975`; `StuckItemDetail.tsx:54-59`; `backlog_stuck_rpc_test.go:179`; `domain/backlog.go:599-606` sentinels.
  - Wrong or stale (VERIFIED): `backlog_service_triage.go:801-810`, `:2313-2321`, `backlog_lifecycle.go:971-984` (see C3). `session/gitwiring/soak.go:207` is `SoakReport`, not the "ledger-write" pattern the task implies (the ledger helper is elsewhere in that package).
  - Not yet existing, as expected for new work: `proto/session/v1/quota.proto`, `backlog_service_engine_status.go`.
  - ADR cross-references (ADR-001..006) all resolve to files that exist; ADR-001 "Spike results" are still `_pending`, as intended.
- The plan says `ent_repository_backlog.go:1721-1760` for the storage CAS; the function begins at `:1727`. Harmless.
- `knownFeatureFlags` exists (used in `completion_reviewer_test.go`), and `defaultValue` fields exist in `feature_flag_service.go`, so Story 4.1.3's premise holds. The new `docs/adr` number is "next free" with a mixed numbering scheme; pick the number at write time.
- `session/backlog_lifecycle.go:1092,1116` type-assert `getWorkflowEngine().(*ConfiguredWorkflowEngine)`. This is why the listener must keep `stageConfigEngine` and not receive the selector (the plan does this). Add a comment or test so a future "inject the selector everywhere" change fails loudly rather than silently degrading gate-context resolution.
- Quota gate gap is real (VERIFIED): `Reconcile` is a no-op when `!cfg.Enabled` (`quota_gate.go:~202`), and `StatusDetail` text still says "set Quota.AssumedWindowTokenBudget in config.json" (`:~168`). Story 2.1.2 targets exactly these.
- Mixed-mode within one request (flag flip between `CanTransition` and `ValidateGates`) is accepted in ADR-001. Fine, but state the TTL (about 1s) in the test so the race is bounded.

## Verdict

BLOCKED on B1 only. B1 is a design defect in the shadow comparison that would make the soak and alert criteria unmeetable and is cheap to fix before any code is written. Everything else is CONCERNS to resolve in the plan text (C2, C3, C5, C6 before dispatching the affected stories).

## Re-review

**Date**: 2026-10-17 (fresh-eyes re-reviewer, against the repaired `plan.md` and ADR-001..006)
**Verdict**: CONCERNS (B1 is resolved; no remaining BLOCKERs)

Evidence labels as above. Counting and wave checks were run by script (`python3 -I`, regexes over `plan.md` headings and the wave table); cites were opened with `sed -n` in this worktree.

### B1: resolved

- **(a) Is the soak criterion now meetable?** Yes on a seeded config. VERIFIED by reading `session/domain/backlog.go:652-692` (`TransitionGuard`): the guard is a pure function of the item, so Default and `enforceAnswer` run the same call and an ordinary denial (`ErrACRequired`, `ErrPlanRequired`) is identical on both sides. The seed has zero gate rows, so under `config=seeded` the only live sources of divergence are graph, `cache_empty` and composition defects. The Story 1.1.3 AC (500 calls, 40 legitimate denials, all divergence series 0) is satisfiable.
- **Is "divergence" now tautological?** Partly, and the plan should say so. The guard half is compared against itself by construction, so `gate_missing` is a composition self-test, not a parity measurement; it is only proven by the injected-defect test in Story 1.1.3. It is not tautological where it matters: the graph half compares Default's static map against the DB-backed Configured graph plus system edges (independent, so real graph bugs surface as `graph`); operator gates surface as `gate_extra`; sentinel mismatch surfaces as `error_kind`. Residual: guard-parity regressions are caught offline (Story 1.1.2d harness over 81 pairs plus the sentinel corpus), not live.
- **(b) Does `not_compared` hide the gates that matter?** On the seeded config no (there are no stateful gates). On an edited config yes, and this is the main remaining weakness (R2).

### Remaining concerns (none block dispatch of stories before the soak)

- [ ] **R1. READY is undefined for a live config that is `edited`.** The alert classes fire only under `config=seeded`, but the READY rule (Story 1.1.3 soak AC, Story 1.4.1) is "at least N transition calls on every observed edge with zero divergence". If the live DB differs from the seed (the C4 unknown, and the whole point of the feature), `graph` and `gate_extra` divergences are expected by design, so either READY is unreachable or the owner must ignore it, which is B1 again for the edited case. Recommendation: for `config=edited`, define READY as "every divergence maps to an edge in the `--config-diff` output the owner has acknowledged; zero divergence on edges outside the diff". Also state which `config` label a window that spans an edit belongs to. Resolve before Task 1.4.1a.
- [ ] **R2. Shadow's `stateful_gate` skip is not in any exit criterion.** `workflow_engine_shadow_not_compared_total{reason="stateful_gate"}` is exposed but absent from the soak exit rule and the runbook criteria (checked `plan.md` lines 259, 111, 497-503). A `human_approval` gate on `queued->in_progress` would block every automated claim only after the flip; shadow reports no `gate_extra` for it. Recommendation: `make workflow-engine-soak-status` lists edges with stateful gates (from `--config-diff`) and prints "UNVERIFIED in shadow" for them, or exits non-zero while `not_compared>0` on a transition-path edge. Story 1.2.2 limits the blast radius but does not detect it beforehand.
- [ ] **R3. Internal inconsistency on `path`.** Story 1.1.3 says exit 1 on divergence "(either path)"; READY counts `path=transition` only. State that a `read`-path divergence alarms but never counts as coverage, or drop `read` from the alarm.
- [ ] **R4. Dependency graph is only ASCII.** There is no per-story prerequisite list. I encoded 27 edges from the diagram and sequencing rules (including 5.1.1 before 2.1.1/1.2.4, 4.1.4 before 1.3.3/4.1.2/4.1.3, every UI story before 6.1.1): all 27 go strictly forward in wave order, so the graph is acyclic and the waves are consistent. Add a `Prerequisites:` line per story so this stays checkable.

### (c) Waves: VERIFIED by script

- 16 waves W0-W15, 30 distinct stories, each appears exactly once, none missing, none extra, max 2 per wave.
- Same-wave file collisions (`dependencies.go`, `backlog.proto`, `feature_flag_service.go`, `features/page.tsx`, `server/actuator.go`, `config/config.go`, `backlog_service_triage.go`) from each story's `Files:` lines: none. `backlog.proto` editors land in W6, W7, W8, W9, W10, as the plan claims.
- Minor inaccuracy: the Effort text says the A chain "occupies one slot in waves W1 through W13". W0 holds 0.1.1 and W12 holds no A story (4.1.1 || 5.1.2); `1.4.1` waits behind `1.3.3` in W13. Cosmetic.

### (d) Counts: VERIFIED by script, all match the repair's claims

| Claim | Recomputed |
|---|---|
| 11 epics | 11 |
| 30 stories | 30 |
| 67 task headings | 67 |
| 65 sized (26 M, 34 S, 5 XS) after excluding 1.2.5a and 4.1.5a | 65 (26 M, 34 S, 5 XS); the two excluded are 75k raw |
| 5.35M raw | 5,350k |
| CU 5.51M, x1.75 = 9.64M | 5,510.5k; 9,643k |
| Per-story table | 30 rows; every row's task count and raw tokens equal the headings; row CU sums to 5,513k (per-row rounding) |

### (e) Cites in the rewritten sections: 60 opened, 0 wrong

Checked in 1.1.2, 1.2.1/1.2.2, 1.2.3, 2.1.1c and ADR-002. All exact: `backlog_service_triage.go:680,683,684,689,692,699,716,837,839,872,873,881,1268,1324,2041,2115,2418,2471,2637,2906,3348`; `backlog_sync.go:449,450,543`; `backlog_service_sync.go:157,158`; `jules_dispatch_service.go:361`; `storage.go:1104`; `tools_backlog_dependencies.go:163`; `backlog_service_lifecycle.go:929,933,959,1446`; `configured_workflow_engine.go:188,284,374`; `config/config.go:593,702,1475-1482`; `backlog_lifecycle.go:1004,1866,1873`; `backlog_lifecycle_archive.go:87`; `jules_session_poller.go:432,518,533`; `backlog_service_trigger_triage.go:290,772`; `backlog_debug_mutate_handler.go:172`; `autonomous_orchestration_service.go:586`; `tools_backlog.go:1507,1515,1682,1821`; `dependencies.go:1429,1436,1477`. The `git grep` recount reproduces 38 sites in 14 files with the plan's per-file counts (triage 11 = 10 direct + `:692`; `backlog_service_lifecycle.go` 3 = `:959`, `:1332`, `:1446`), and 1+1+5+31 = 38. The `claimAndSpawnCandidate` `default:` arm and its four named sentinels (`ErrPreconditionFailed`, `ErrUnresolvedBlockers`, `ErrPlanRequired`/`ErrPlanArtifactsRequired`, `ErrPlanArtifactsMissing`) match the Story 1.2.2 text.
- Nit: Story 1.2.3's scope paragraph lists `OverrideVerdict` as `:1332` right after triage cites; the file is `backlog_service_lifecycle.go` (ADR-002 has it right).

### Remaining BLOCKERs

None. Fix R1 and R2 in the plan text before Task 1.4.1a (and ideally before 1.1.3b, since they change the soak exit rule).

---

## Re-review: Epic 1.5

**Date**: 2026-10-17 (fresh-eyes re-review of the plan after "Repair round 3": Epic 1.5, ADR-007, four live flags, strong Metric 1)
**Verdict**: BLOCKED (3 blockers, 5 concerns, 4 minors)

### Blockers

- [ ] **RB1. The strict guard (Task 1.5.9a) keys on `Raw*` names, but the repository-level primitives stay unrenamed and reachable, so "no raw call anywhere" is not enforced.** ADR-007 D1: "The repository interface keeps its method names; only the exported `Storage` wrappers are renamed." Verified: `ReconcileStuckItems` is a method on `*EntRepository` (`session/storage_backlog.go:967`) with no `Storage` wrapper; the only caller is `er := l.storage.repo; er.ReconcileStuckItems(ctx)` (`session/backlog_lifecycle.go:1255-1256`). Renaming Storage wrappers cannot make that a compile error, and the AC "a stray call fails to compile" is false for it. Any file in package `session` can also write `s.repo.TransitionBacklogItemStatus`, `.ArchiveBacklogItem`, `.UnarchiveBacklogItem` or `.TransitionBacklogItemStatusWithPRFields` (`session/storage.go:1089,1094,1104,1218`) and the strict test (a) "reference to a `Raw*` primitive" does not see it. `chain_firer_test.go:143` already calls the repo method directly. The 45-site inventory also has no entry for `TransitionBacklogItemStatusWithPRFields`. Recommendation: make the inventory test match the repo-level method names too (selector calls to those five names outside `transition_gate.go`, `storage*.go` wrappers and `_test.go` fail), or rename them on `Repository`/`EntRepository`; give `ReconcileStuckItems` its own explicit migration line (Task 1.5.4c already splits it, but the guard must cover the `storage.repo` path); fix the "compile error" claim in ADR-007 D1 and Story 1.5.4.
  - Same blocker, second path: `CreateBacklogItem` accepts an arbitrary `data.Status` (`session/ent_repository_backlog.go:435-437,452`). Production callers all pass `idea` (`tools_backlog.go:2221,2315`, `import_tracker.go:37`, `backlog_service_lifecycle.go:221`, `backlog_service_sync.go:319`), but the debug seed handler inserts items directly at other statuses (`backlog_debug_seed_handler.go:100-102,149-151,227-229,572-574`). Not in the inventory, not in the allow-list, not mentioned in ADR-007. Recommendation: add `create` as an explicit non-transition entry (assert `Status` is an entry stage, `builtInEntryStages`) and list the seed handler as `exempt: debug` with a reason, so the strict guard states it rather than silently missing it.
  - Checked and clean: no other `SetStatus` on `BacklogItem` (only `ent_repository_backlog.go:1484,1559,1753,1977` and `storage_backlog.go:1006`), `BacklogItemUpdate` has no status field, no raw SQL against the table, stage deletion refuses when live items exist (`ent_stage_config_repository.go:173`).
- [ ] **RB2. Adding `in_progress|review|pr_pending->archived` to `validTransitions` (Task 1.5.1c) falsifies "all four flags off equals today", is not an owner-confirmed change, and does not fix the live database.**
  - It changes the Default engine, which every Off and Shadow path asks. `session/workflow_engine_test.go:22-26` pins the opposite on purpose ("in_progress to archived has no such edge", `want: false`); the plan never mentions that test (1.5.1c lists only `workflow_engine_equivalence_test.go`). `DefaultWorkflowEngine.AllowedTransitions` feeds the UI's `allowed_transitions` (`server/services/backlog_service.go:1102-1109`), so Archive appears on in-progress, review and PR-pending items the moment the binary deploys, flags or no flags. The Migration Plan, ADR-007 D11 and the Rollback procedure all state Off equals today exactly.
  - It is also unnecessary for Enforce: the selector already unions `SystemRequiredEdges()` into the enforced graph. And it is not sufficient: the seed is create-if-count-zero and never re-run (`workflow_stage_seed.go` header), so an existing live DB's Configured graph lacks the edge. ADR-007 D4 makes only `agent` and `system` registry edges system-required, so under operator Enforce the human Archive RPC on an in-progress item (`backlog_service_lifecycle.go:516`) would be denied on the live DB, while the Story 1.5.5 AC ("archiving an `in_progress` item is allowed (edge now in the table)") passes on a fresh seed. The shadow `graph` divergence would show it, but only as an ack row.
  - Recommendation: do not change `validTransitions`. Register the archive/unarchive edges in `SystemRequiredEdges()` (or a "storage-direct" Group 2 that the glossary already names but ADR-007 D4 does not state) and relax the audit to "in `ValidTransitions()` or `SystemRequiredEdges()`". If the owner wants the table changed, put it in Unresolved Questions, update the pinned test, and add an existing-DB (stale seed) test for the operator archive RPC.
- [ ] **RB3. The agent-class proof contradicts ADR-007 Decision 4.** D4: every `(from,to)` in any registry entry with class `agent` or `system` is system-required and cannot be disabled. Stories 1.5.5 and 1.5.9 prove the agent class by disabling `done->refining` and calling MCP `transition_backlog_item_status`, which delegates to the handler with actor `agent` (`tools_backlog_dependencies.go:163`) and can request any valid edge. Either that entry's `edges` are every valid edge (then by D4 all of them become un-disableable, the editor rejects every edit, and the proof cannot be written), or `done->refining` is not in its edges (then the 1.5.1b edge-audit "every declared edge" is vacuous for the one agent entry whose edge set is unbounded, and the DoD test denies something D4 says is un-deniable by graph). Recommendation: decide explicitly whether the generic delegate is exempt from D4 (agent callers with bounded edges such as `request_review` are system-required; the generic delegate is governed like `operator`), state it in the registry rules and the editor rejection list, and keep the proof on that basis.

### Concerns

- [ ] **RC1. Per-class soak evidence is thin; the system-class READY is largely an acknowledgement, not evidence (question b).** The ledger persists per minute so "short log retention" is not the constraint; volume is. ADR-007 D10 says "tens per day, not per request". Requirements as written: N=20 samples on every observed expected edge plus a floor of 100, within W=7 days for operator and agent and 14 days for system. A single human operator will not produce 20 samples on each of archive, unarchive, PR association, `forceResetItem`, manual review and trigger-triage in a week; `autoReopenForPRFix`, `respawnTriage`, the Jules poller and `archiveStaleDoneItems` with about 21 live items will not reach 20 in 14 days either. The rule falls back to "or owner acknowledgement", so READY is reachable, but by acknowledgement. More important, seeded config has no gates (the seed never creates gates), so under `config=seeded` the shadow can only ever show graph/guard parity. The system class's real risk, an operator gate stranding automation, is not observable in shadow until the config is edited, and an `edited` window is excluded from the zero-divergence rule and covered only by `--config-diff` plus ack. Recommendation: state in the runbook that the system (and operator) flip is justified by the Story 1.5.9b per-caller table, the `--config-diff` stranded-item preview and the replay of the registry against a copy of the live DB, with shadow as a parity check only; do not present the sample-count thresholds as the evidence. Consider a synthetic canary edge (a no-op gate toggled on a scratch stage) so the edited path is exercised once before the flip.
- [ ] **RC2. W12 runs two stories that edit the same files (question c).** Story 1.5.5 (Task 1.5.5b: `forceResetItem` `triage.go:1324`, `TriggerTriage` `trigger_triage.go:290`) and Story 1.5.6 (Tasks 1.5.6b/c/d) both list `server/services/backlog_service_triage.go` and `server/services/backlog_service_trigger_triage.go` in `Files:` (computed by script over the `Files:` lines). The plan says "disjoint files" and its serialization list gives `backlog_service_triage.go` as W6 and W12 for 1.5.6b only. `forceResetItem` is owned twice (1.5.5b as operator, 1.5.6d as a system nil-precondition site; 1.5.1a is told to confirm its class). Recommendation: move `forceResetItem` and `trigger_triage.go:290` out of 1.5.5 into a wave-disjoint story, or put 1.5.5 and 1.5.6 in different waves (adds one wave, W21).
- [ ] **RC3. The post-1.5.3 overrun checkpoint is prose, not a gate (question c).** It exists only as one sentence in "Effort estimate" and is "informational, not a cut line". No task, wave row, acceptance criterion or pre-mortem item owns it, so no worker brief will contain it. It is the right place (after W10, about 28% of CU spent, before the 3 migration stories that are 1.5.5 to 1.5.7 = 1.11M raw), but nothing makes it fire. Recommendation: add it to the W10 row ("exit: record cumulative CU versus table, owner informed") and to the Worker brief block.
- [ ] **RC4. Metric 1 "met" still depends on an open owner question.** The Definition of Done says "met in tests for all three classes", but the carve-out (system-required edges cannot be disabled, Unresolved Question 1) is listed as blocking only the 1.2.1 acceptance wording. With RB3 unresolved the plan cannot say which agent/system edges Metric 1 covers. The PR template "records production status per class" but should also record the carve-out list generated from `SystemRequiredEdges()`.
- [ ] **RC5. Test rename scope is understated.** Story 1.5.4 counts 27 files / 177 calls of `.TransitionBacklogItemStatus(` (reproduced: 27 files, 177 calls) but those include `fx.repo.` calls that a Storage rename does not touch, while the 23 test lines calling `ArchiveBacklogItem`, `UnarchiveBacklogItem` or `SetBacklogItemPRAndTransition` also need the rename and are not in the "27 test files" figure. Size of Task 1.5.4a (M) may be light; the checkpoint should include it.

### Minors

- Epic 1.5 text still says "all 45 sites" (Task 1.5.1a) while Metric 1 text says 40 plus 5 with 5 exempt: consistent arithmetic (31+9+5), but the registry has fewer entries than sites (exempt ones are not registered); say so once.
- Task 1.5.4a lists "production call sites (rename only)" without file names; W11's disjointness with 4.1.2 (touches `dependencies.go` only) is therefore asserted, not checked.
- Story 1.5.4's AC "a `Raw*` name that the inventory lists as pending" lets a green build coexist with ungated paths for waves W11 to W14; fine for rollout, but the banner and PR template should state the live-coverage percentage per wave.
- `validation.md` M1 row still names the pre-1.5 startup line without `class_modes`; align with Task 1.5.3c.

### Answers to the six questions

- **(a) Metric 1 met?** Not yet provable. Every named primitive is routed, and my grep of `SetStatus`/ent update builders/SQL found no other production status writer, but the guard that is supposed to keep it true misses repo-level methods and `CreateBacklogItem` (RB1), the archive edges break the Off invariant (RB2) and the agent proof contradicts D4 (RB3).
- **(b) Soak.** READY is reachable only through acknowledgements for operator and system classes; shadow under a seeded config cannot exercise gates (RC1).
- **(c) Schedule: counts VERIFIED by script.** 12 epics, 39 stories, 102 task headings, 101 sized (43 M, 52 S, 6 XS; 4.1.5a excluded), 21 waves, every story once, max 2 per wave, 0 prerequisite-versus-wave violations over 39 `Prerequisites:` lines, raw 8,570k, CU 8,827k, x1.75 = 15,447k (above the 15M Large ceiling, as the plan says), Epic 1.5 = 32 tasks, 3,035k raw (claimed about 3.04M). One same-wave file collision: W12 (RC2). 38 `TransitionBacklogItemStatus` sites recounted by `git grep` (matches ADR-007).
- **(d) Archive edges.** Out of scope as a Default-table change and unsafe (RB2); the scope-correct fix is the system-required union.
- **(e) Cites: 22 opened, 0 wrong** (`domain/backlog.go:514-574`, `ent_repository_backlog.go:1457,1542,1721`, `storage.go:1089,1094,1104,1164,1218`, `storage_backlog.go:967`, `backlog_remediation.go:332`, `backlog_service_gates.go:148`, `dependencies.go:678,1477,1544`, `backlog_service_lifecycle.go:470,516,547,870,892,933,959`, `configured_workflow_engine.go:284,374`, `backlog_service_triage.go:872-873`, `backlog_debug_mutate_handler.go:253`, `tools_backlog_pr.go:375`, plus the 38-site list). Two cite-adjacent errors are in RB1 (the "compile error" claim for `ReconcileStuckItems`).
- **(f) Contradictions with the owner decision.** None of "measure, don't govern", "counted, not governed", `DefaultFlagsChangeNothing`, `BypassSites_RemainOnBuiltInGraph` survives outside superseded/historical notes (ADR-002 D1 title, repair logs, `consistency-review.md`, `adversarial-review.md` history, `research/architecture.md:78`). `requirements.md:40` carries the owner note. The only residual tension is the explicit carve-out (RC4, RB3).

---

## Re-review round 2: Epic 1.5

**Date**: 2026-10-17 (fresh-eyes round 2, scope: RB1-RB3 against "Repair round 4" in `plan.md`, ADR-007 and `validation.md`)
**Verdict**: CONCERNS (0 blockers; RB1, RB2, RB3 are resolved in substance; 5 concerns, 3 minors)

Method: `git grep` over the tree for every status writer; `python3 -I` scripts over `plan.md` headings, the wave table, `Prerequisites:` and `Files:` lines; 24 cites opened with `sed -n`.

### (a) Is the strict guard airtight? Mostly; one lexical gap

- VERIFIED the tree has exactly the six writers the plan names. `git grep -n 'SetStatus('` on `BacklogItem` builders: `session/ent_repository_backlog.go:452` (create), `:1484`, `:1559`, `:1753`, `:1977`, and `session/storage_backlog.go:1006`; `:2650` is a progress note. No `OnConflict`/upsert, `CreateBulk`, `Modify` or raw SQL touches `backlog_items` (the only `OnConflict` hits are the dependency and stuck-state tables, `ent_repository_backlog.go:628,2304`). No reflection or `MethodByName` in the backlog/storage/services files. Other `BacklogItem` `UpdateOneID` calls (`backlog_item_*migration*.go`) set no status.
- VERIFIED no exported ent-client accessor exists on `*EntRepository` or `*Storage` (grep of `func (...) Client|EntClient|DB`), and the only other `*ent.Client` consumers are `server/analytics` (a separate DB). So after Task 1.5.4c unexports the writers, a raw status write is only possible inside package `session`.
- The `create` class with computed `Status`: the four in-tree `MapToBacklogItem` plugins set `idea` or nothing (VERIFIED `backlog_plugin_github.go:~425`, `backlog_plugin_github_prs.go`; jira and linear set none), and the production creators pass `idea` (`backlog_service_lifecycle.go:221-235`). But `session/backlog_sync.go:353` passes `data` returned by the open `ExternalPlugin.MapToBacklogItem` interface, so there is no same-function composite literal for Task 1.5.4g to resolve; it needs the allow-list reason the task already permits. Not a hole today.
- [ ] **N3 (CONCERN). The strict test's rule (a) matches only six method NAMES.** A seventh writer written as a new ent builder (`client.BacklogItem.UpdateOneID(id).SetStatus(...)`) in any `session` file is invisible to it, which is exactly how the missed sixth writer (`TransitionBacklogItemStatusWithPRFields`) went unnoticed twice. Add a rule (d): any `.SetStatus(` call in package `session` outside the five allow-listed writer functions fails (lexical is enough; the six sites above are the allow-list). Put it in Task 1.5.9a, and in the Task 1.5.4g `create` rule too, because `:452` is the creator's `SetStatus`.

### (b) Archive edges as system-required: claim holds, with one fallback hole and one UI gap

- Stale-seed claim: HOLDS AS DESIGN, but note that `SystemRequiredEdges()` and `IsSystemRequired` do not exist in the tree (`git grep` returns zero hits); round-1's statement that "the selector already unions" them was wrong, and Task 1.1.2b creates the union. As specified (`plan.md:273`, Task 1.1.2b: "add the set and the union in `CanTransition`/`AllowedTransitions`"), a live database whose seed lacks `in_progress->archived` still archives under operator Enforce, because `ConfiguredWorkflowEngine.CanTransition` (`configured_workflow_engine.go:100-114`) is queried and then OR-ed with the code-defined set. `TransitionGuard` has no archive case and does not check the graph (`session/domain/backlog.go:648-692`), and `PendingGates` returns `nil, nil` for an edge absent from the cache (`:~173-176`), so no phantom gate denies it. `workflow_engine_test.go:22-26` still passes because `validTransitions` is untouched (VERIFIED, table at `domain/backlog.go:514-574` has no `in_progress|review|pr_pending->archived`).
- [ ] **N1 (CONCERN). The union is dropped on the fallback path.** `plan.md:273` and the glossary row at `plan.md:48` say "Graph = Configured UNION `SystemRequiredEdges()`; if Configured is nil or its cache is empty/unloaded, graph = **Default graph**". The Default graph lacks the archive edges, so on `cache_empty`, `cfg_nil` or `load_error` an Enforce-mode archive of an `in_progress`, `review` or `pr_pending` item (and `archived->archived`) is denied for every class, with `legacy=Unchecked` having allowed it. That is the opposite of the "fail-open to Default on graph" intent (`plan.md:89`) and contradicts the ADR-007 D7 claim that archive succeeds "under every class". The stale-seed test in `validation.md:69` uses a populated cache, so it would not catch this. Fix: the fallback graph is Default UNION `SystemRequiredEdges()`; add a case to `TestSwitchableWorkflowEngine_FallsBackToDefaultGraph_WhenConfiguredCacheEmptyOrNil` (`validation.md:44`) for `in_progress->archived`.
- [ ] **N4 (CONCERN). "Gates on archive edges stay editable" is not reachable from the UI for the added edges.** The edges `in_progress|review|pr_pending->archived` and `archived->archived` have no row in the stage-config tables (the seed is create-if-count-zero, and `AllowedTransitions` is deliberately unchanged), so the Stages editor neither shows them nor offers a place to attach a gate; PendingGates looks up `cache.Get(from,to)` and finds nothing. The Story 1.5.5 AC ("an operator `human_approval` gate on `done->archived`") only covers `done->archived`, which does have a seeded row. For the other four statuses the strong-metric claim is true only if the editor can create the edge row. State which: (i) the editor lists system-required edges as virtual rows that persist a real edge when a gate is attached (then the union is redundant for that edge but harmless), or (ii) archive from active statuses is a documented UI-ungateable part of the carve-out. Add it to Unresolved Question 1 so the owner sees it.

### (c) Forwarding MCP delegate exemption: not a hole for Metric 1, but the class mechanism is unspecified

- The exemption itself is sound. The union is edge-level, not caller-level: an edge another fixed caller needs (`in_progress->review`, `pr_pending->done`, every `*->archived`) is un-disableable for the delegate too, and that is the declared carve-out; every other edge the delegate can request (`done->refining` and so on) is governed by the graph, Default guards and gates, with `override_reason` omitted by design (`tools_backlog_dependencies.go:143-146`). The registry audit stays non-vacuous (`plan.md:599`).
- [ ] **N2 (CONCERN). Nothing says how a request that reaches the SHARED handler becomes class `agent`.** The delegate calls the same RPC handler as the UI (`tools_backlog_dependencies.go:163` into `backlog_service_lifecycle.go:870`, which reads `transitionActorFromContext`), the handler is the only `GatedTransition` call site, and registry `class` is static per entry. Yet the plan wants one registry entry `mcp_transition_delegate` (class `agent`, `forwarding`) AND the operator handler, pins "every registry entry has a call site" (rule (c), `plan.md:477-478`, `:891`) and the delegate makes no `Gated*` call (it is an allow-listed passthrough). As written an implementer either gives the handler one `operator` caller (agents then ride the operator flag and `-enforce-agent` never governs the generic delegate, which is the main agent path, making Metric 1 untrue for agents) or invents a mapping. Specify in Task 1.5.5a: the handler resolves the registry caller from the context actor (`agent` gives `mcp_transition_delegate`, otherwise `operator_transition_rpc`), and exempt the delegate from rule (c) or make the handler pass both names as literals. The existing test `TestMcpTransitionDelegate_UsesAgentClassAndGraph_WhenActorAgentAndEdgeDisabled` (`validation.md:89`) would catch the wrong choice, which is why this is a CONCERN and not a BLOCKER.
- Related, minor: MCP `trigger_triage` calls the `TriggerTriage` RPC (`tools_backlog_dependencies.go:~125`), which Task 1.5.6e classes as `system`. An agent's action is therefore governed by `-enforce-system`, not `-enforce-agent`. Say so in the runbook.

### (d) Schedule and counts: all VERIFIED by script (`python3 -I`)

| Claim | Recomputed |
|---|---|
| 12 epics, 39 stories | 12, 39 |
| 114 task headings, 113 sized | 114; 113 sized (49 M, 57 S, 7 XS) after excluding 4.1.5a |
| 9,645k raw | 9,645k (49x130 + 57x55 + 7x20) |
| 9,934k CU, 17,385k with x1.75 | 9,934k; 17,384k (the one-unit difference is per-row rounding; cosmetic) |
| Epic 1.5 = 44 tasks, 4,110k raw | 44, 4,110k; 7,408k CU after x1.75 = 42.6% of total (plan says about 43%) |
| above the 15M ceiling by about 16% | 17,384k vs 15,000k = +15.9% |
| 21 waves, max 2 per wave, every story once | 21 waves (W0-W20), max 2, 39 distinct, 0 missing, 0 extra, 0 duplicate |
| 0 prerequisite-versus-wave violations | 0 over 39 `Prerequisites:` lines (every prerequisite is in a strictly earlier wave) |
| 0 same-wave `Files:` collisions | 0 (exact-path comparison of union of `**Files**` and task `- Files:` lines per paired wave; note the check is by exact path string, so a directory or glob entry would not collide) |

Note for anyone reproducing: task headings `1.5.5e`, `4.1.4a`, `4.1.4d` and `4.1.5a` have prose between the ID and the size, so a naive `Task [\d.]+[a-z]?:` regex drops 4 tasks (110 instead of 114).

### (e) Cites: 24 opened, 0 materially wrong, 1 drift

Exact: `storage.go:1103-1104,1218`; `ent_repository_backlog.go:1951,1977`; `storage_backlog.go:967,1006`; `backlog_lifecycle.go:1255-1256`; `jules_session_poller.go:86-87`; `silenttransition/analyzer.go:63`; `autonomous_orchestration_service.go:453,604` (the `toStatus == Review` branch is real and `toStatus` is assigned `Ready` at `:453`); `tools_backlog_dependencies.go:163`; `workflow_engine_test.go:22-26`; `backlog_remediation.go:332`; `backlog_item_event_publisher_test.go:258`; `workflow_stage_seed.go:49`; `repository.go:730`; `backlog_lifecycle_triage.go:445`; `backlog_service_triage.go:2608`; `backlog_debug_seed_handler.go:100`; `jules_session_poller_test.go:178`; `jules_config_service_test.go:78`; `domain/backlog.go:514-574,648-692`.
- Drift (minor): ADR-007 D12 cites `backlog_lifecycle_triage.go:384-399` for "shape 2 (ended, item still in idea)". Shape 2 is at `:374`; `:385` starts Shape 3. The graceful-shutdown branch at `:352-366` is correct.

### (f) Consistency with the strong Metric 1 decision: consistent

`git grep` for "measure, don't govern", "counted, not governed", "scope out", `DefaultFlagsChangeNothing` and `BypassSites_Remain` hits only superseded or historical text (ADR-002 title and D1 marked SUPERSEDED, repair logs, `ADR-007` context, `plan.md:1352` which records their removal). `requirements.md` keeps Metric 1 text unchanged with the owner note. The only retained weakening is the single explicit carve-out (Unresolved Question 1), and N4 shows it is slightly larger than the text says (archive-from-active gates are not UI-reachable). Minor: `validation.md:15` M1 row and `:46` use "UnionsSystemRequiredEdges" wording that is fine; `validation.md` still never names the `class_modes` startup line in M1 (round-1 minor, now present in row `:62`, so closed).

### Minors

- `plan.md` repair log says "114 task headings ... 49 M, 57 S, 7 XS sized" correctly; the `17,385k` figure is 1k high (cosmetic).
- Round-1 R-notes said "the selector already unions `SystemRequiredEdges()`"; it does not exist yet. Harmless because RB2's fix is a plan, but any reader assuming existing code should not.
- Story 1.5.8's two "confirm" rows (`rework_blocked_stale`, `orphaned_agent_pr`) are explicitly deferred to Tasks 1.5.8c-d; acceptable, but the completeness test must fail until they are classified.

### Honest view: is Epic 1.5 proportionate, and is there a simpler design?

- **Size.** 44 tasks, 4.11M raw, about 7.4M CU after the multiplier, 43% of the whole plan, for one of five metrics. It is proportionate to the requirement as the owner stated it (any edit changes any path) only if you accept all three of: three caller classes, four flags with per-class soak, and enumerating every automated detector. It is not proportionate to the stated risk profile (about 21 live items, tens of transitions a day, which the plan itself concedes makes the soak thresholds unreachable).
- **What is essential (about 1.2-1.5M raw, INFERRED):** the choke point (1.5.3), closing the six raw writers with the strict guard plus the N3 rule (1.5.4, 1.5.9a), the registry and edge audit (1.5.1), and migration of the 45 sites (1.5.5-1.5.7). Without these Metric 1 is false.
- **What buys policy, not truth, and could be simplified without weakening the metric (all INFERRED, none required for the metric):**
  1. Per-class flags and soak (1.5.2 partly, 1.5.9c, the 4-row UI in 1.3.3, per-class runbook, `-enforce-agent`/`-enforce-system` degrade logic): replace four flags with the existing shadow/enforce pair plus a per-caller revert list in config (`caller -> shadow`). Revert granularity becomes finer than per class; the registry already names every caller. Class semantics (override honored, gates, guards) stay in the registry as policy, not rollout.
  2. Denied-transition recovery (1.5.8, 705k raw): the real stranding path today is one edge (`idea->ready` from triage, `autonomous_orchestration_service.go:453`) plus `ReconcileStuck` and the PR edges. If the shared respawn/nudge dispatch (`runStuckDetector` and `remediationActionByReason`) can host a single `HasOpenTransitionDenied` check, six per-detector tasks collapse to one plus the completeness test. I have not verified the dispatchers are a single choke; Task 1.5.8 should say so before it sizes six guards.
  3. The replay harness (1.5.9d) and the "four-artifact acknowledgement" substitute for evidence the volume cannot produce; keep the replay (it is the only test of the edited path on live data) and drop the sample-count machinery for the operator and system classes instead of printing thresholds the plan admits are unreachable.
- **Is there a simpler design meeting the strong metric?** The minimal faithful design is: one `GatedTransition` that decides graph (Configured UNION system-required) plus gates for every caller, Default guards only where the caller declares them, one enforce flag with a per-caller revert list, and the strict inventory guard. That keeps every property the metric needs (edits change results on all paths; no raw writer reachable) and drops the class-keyed rollout and most of the soak apparatus. The owner has said not to downscale, so this is offered as an option for the 1.5.3e overrun checkpoint, not as a cut line; no scope is removed from the metric by it.

### Remaining BLOCKERs

None. Fix N1 (one sentence in Story 1.1.2 and the glossary), N2 (Task 1.5.5a) and N3 (Task 1.5.9a) before Story 1.1.2 and Story 1.5.5 are dispatched; N4 goes to the owner with Unresolved Question 1.
