# Implementation Plan: backlog-make-it-true

**Feature**: Make the backlog's advertised configuration true: wire the configured workflow engine behind a shadow/enforce seam with full gate parity, give quota and soft budget a settings surface, populate repo `allow_auto_merge` honestly, record and instrument the autonomy-flag defaults, and fail the build when a config surface has no runtime-effect proof.
**Date**: 2026-10-17
**Status**: W0 errata applied 2026-10-18 (see the last Repair log entry; W0 stories 0.1.1 and 4.1.4 are implemented and merged). Repaired after adversarial, architecture, pre-mortem and consistency review (see "Repair log" at the end). Owner decision 2026-10-17 applied: Success Metric 1 stays strong and ALL status-write paths are routed through the engine (Epic 1.5, ADR-007). Repair round 4 (2026-10-17) closed the Epic 1.5 re-review findings (six raw writers, `silenttransition` analyzer, real autonomous-completion mapping, archive edges, forwarding delegate, detector guards); re-review recommended before dispatch. Repair round 5 (2026-10-17) closed the Epic 1.5 re-review round 2 findings (gate-reports-denial test instead of an analyzer rule, complete spawn-point detector table, no `RecoveryReady()` degrade, caller in the denial key, fallback graph keeps the archive edges, shared-handler caller resolution, `.SetStatus(` rule, archive-edge UI choice for the owner). Repair round 6 (2026-10-17, triad review: product, UX, engineering) added the outcome metric with a dated review (requirements Metric 6), an Authn/authz section with guarded-procedure tasks, a coordination rule for the triage files, the new `AlertConfirmDialog` with readiness evidence, UX-35 to UX-42, and made Tasks 4.1.4a and 4.1.4d mandatory. Owner decisions of 2026-10-17 are recorded as approved: soak parameters (W, N, floor, low-volume READY rule) and archive-from-active edges documented as UI-ungateable (option A). Repair round 7 (2026-10-17, triad 2: UX, engineering, product) closed the triad-2 gaps: the `AlertConfirmDialog` contract and readiness re-check, live-region ownership, deep-link, leave-guard and pending-state behavior, quota source and mismatch handling, test-infra corrections (the app applies themes as an `<html>` class, not `data-theme`), a declared exception for mechanical renames, the dispatch preconditions, the riskiest-assumption statement, an in-product review-overdue line, and the per-confirmation wave blockers.
**ADRs**: [ADR-001](../decisions/ADR-001-switchable-workflow-engine-seam.md) (engine seam, enforce composition shared by shadow and enforce, guards stay code, fallback, override_reason), [ADR-002](../decisions/ADR-002-bypass-sites-and-system-required-edges.md) (inventory test, system-required edges, three engines; its "scope out, but measure" Decision 1 is superseded), [ADR-007](../decisions/ADR-007-engine-gated-transition-choke-point.md) (engine-gated choke point for every status write, caller classes, per-class rollout), [ADR-003](../decisions/ADR-003-allow-auto-merge-lookup.md) (auto-merge lookup), [ADR-004](../decisions/ADR-004-quota-settings-and-soft-budget.md) (quota RPC, soft budget), [ADR-005](../decisions/ADR-005-autonomy-flag-defaults-record.md) (flag defaults), [ADR-006](../decisions/ADR-006-config-surface-runtime-proof-registry.md) (config-surface proof). UX: [design/ux.md](../design/ux.md).

Evidence labels: VERIFIED = read from the research files in `../research/` (which cite `file:line`); INFERRED / UNVERIFIED as marked. Complexity 3, so every section below is present.

---

## Success Metric 1 stays strong (owner decision, binding)

On 2026-10-17 the owner rejected rewording Success Metric 1 in `requirements.md`. It reads as written: editing a stage, transition or gate in the stages UI changes the transition result for ALL status-transition paths, not only the engine-routed ones. This replaces the earlier "met for engine-routed paths only" scoping.

- **What changed**: ADR-002 "scope out, but measure" is superseded by [ADR-007](../decisions/ADR-007-engine-gated-transition-choke-point.md). All 40 direct status-write sites (31 direct `TransitionBacklogItemStatus` callers plus 9 callers of `ArchiveBacklogItem`, `UnarchiveBacklogItem`, `SetBacklogItemPRAndTransition` and `ReconcileStuckItems`) and the 5 engine-routed sites go through one choke point, `Storage.GatedTransition` (Epic 1.5). There are six raw writers in all (also `TransitionBacklogItemStatusWithPRFields`, reached only through `SetBacklogItemPRAndTransition`); the five `EntRepository` writers are unexported and the `Storage` wrappers renamed `Raw*`. A repo-wide test fails on any call, under old or new names on any receiver, outside an explicit allow-list (the choke point file, the `Storage` wrapper bodies, the MCP delegate, 3 rollback writes plus 2 debug sites). Creation (`CreateBacklogItem`) is not a transition: it is an exempt, constrained `create` class (non-entry statuses rejected outside the named debug entry).
- **Caller classes** `operator`, `agent`, `system` have different policies (ADR-007 Decision 3) and roll out through the same shadow to enforce selector with one enforce flag per class. Metric 1 is met in production for a class when that class's enforce flag is on, and for every path once all three are on. Until then the Stages banner names which classes are enforced; before any flip, edits are shadow-compared and visible, not applied.
- **Honest carve-out the owner should see** (the single carve-out; Unresolved Question 1): an edge that a purpose-specific agent tool or a system caller can request is `SystemRequiredEdges()`, and the editor rejects disabling it with a visible reason. This includes every `*->archived` edge (archive is allowed from any status). The generic MCP `transition_backlog_item_status` delegate is NOT in that set (it is `forwarding` and governed by the graph like an operator move). Gates on all of these edges are fully editable, and denials of automated transitions surface as `transition_denied` stuck items. The PR template lists the carve-out edges generated from `SystemRequiredEdges()`. If Metric 1 is read as "any edge can be disabled", this is an explicit exception.
- **Proof**: operator class by Story 1.2.4 (handler, real `BacklogService`); agent and system classes and the "every registered caller" table by Story 1.5.9b; no raw call anywhere by Story 1.5.9a; production wiring by Stories 1.1.4 and 1.5.3c. The weak-metric tests `DefaultFlagsChangeNothing` and `BypassSites_RemainOnBuiltInGraph` are removed from `validation.md`.
- **Definition of done for Metric 1**: "met in tests for all three classes (Stories 1.2.4, 1.5.9); met in production per class when that class's enforce flag is on after its soak". The PR template records production status per class as pending the owner's flips.
- **Size**: Epic 1.5 pushes the project to the top of or just beyond the Large band; see "Effort estimate". Full scope per owner; there is no cut line.

---

## Worker brief standard block (paste verbatim into every dispatched worker brief)

- **Forbidden commands** (restart the live service, kill live tmux sessions and the workers themselves): `make install-service`, `make uninstall-service`, `systemctl restart|stop stapler-squad` (user or system), killing the live service PID, `tmux kill-server` without `-L <your own socket>`, binding `:8543` or `:8444`, writing under the live `~/.stapler-squad/` state, `git add -A`, `git add .`, `git add -f`, force push.
- **Only allowed way to try a running build**: `scripts/verify-manual-instance.sh <N>` (Task 0.1.1d) with the manual slot `N` assigned to the story in the wave table. It builds to `~/.stapler-squad/manual-builds/manual-<N>/`, requires distinct `PORT`, `--remote-port`, `STAPLER_SQUAD_INSTANCE` and `STAPLER_SQUAD_TMUX_SOCKET`, runs the `ss -ltnp` precheck, and tears down only its own tmux socket.
- **Manual slot cap (W0 errata)**: `CLAUDE.md`'s manual dev port block is 62871-62880 (10 ports, one `PORT`/`--remote-port` pair per slot), so slots are 1 to 5 only; the wave table uses 1 and 2.
- **Fresh worktree build setup (W0 errata)**: `gen/` and `session/ent/*` are gitignored, so run `make proto-gen ent-gen` first, and create the gitignored placeholder `server/web/dist/.placeholder` so `go build` finds its embed. Do NOT run full `make build`: the web build makes it take 10+ minutes; use targeted `go build`/`go test` and `pnpm` commands instead.
- **Worktree base and plan files (W0 errata)**: Agent isolation worktrees branch from CURRENT `main`, not from the feature branch; the dispatcher merges each story branch back. The ADRs and plan files are NOT in the worker's HEAD: read them from the feature-branch checkout at `/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/stapler-squad-backlog-cohesion_18df794146eb4b25/project_plans/backlog-make-it-true/` (absolute path), and do NOT commit copies of plan or ADR files (the dispatcher records spike results in the ADRs).
- Stage explicit paths only. Before push, `git diff --name-only origin/main... | grep -E '^(gen/|session/ent/[^s])'` must print nothing.
- At the end of every story (not only W20) run `make ready-complexity-gate` and the jscpd check (`pnpm run lint:duplicates` in `web-app/`).
- Use a separate worktree per concurrent story (memory: parallel workers sweep each other).
- Estimates are cost classes and waves, never hours.
- **Overrun checkpoint (Task 1.5.3e)**: when Story 1.5.3 merges, record actual versus table CU for every story completed in W0-W10 before dispatching W11. If cumulative actual exceeds the table by more than 25%, Story 1.5.3 alone by more than 25%, any one story by more than 50%, or actual usage cannot be read, post the delta and its cause and wait for the owner's reply; otherwise record and continue.
- **Overlap check before dispatching any story that edits `server/services/backlog_service_triage*.go`, `backlog_service_trigger_triage.go`, `autonomous_orchestration_service.go` or `session/backlog_lifecycle*.go`** (Stories 1.2.2, 1.5.4, 1.5.6, 1.5.8): the dispatcher (1) lists open backlog items on the live instance and open PRs (`gh pr list --repo tstapler/stapler-squad --state open`, then `gh pr diff <n> --name-only` per PR) that touch those files, (2) runs `git fetch origin && git log origin/main --oneline -- <those files>` since the branch point, and (3) if another session or PR edits the same files (notably the open item "Investigate false triage artifact does not exist error", reported by the engineering triad review but NOT verified here because the live backlog MCP was unreachable, ECONNREFUSED, during review), does not run the stories concurrently: wait for that work to merge, merge `main` into the story branch, and re-run `make ready-complexity-gate`. The check and its result are recorded in the story PR.
- File-disjoint waves are verified by script over every story's `Files:` lines AND every task's `- Files:` line (resolved to full paths, test files included) before a wave is dispatched (see "Waves" and "Task sizing rule and exceptions"); a worker that needs a file outside its story's list stops and reports.

---

## Task sizing rule and exceptions (round 7)

Rule: a task edits 3-5 files (production plus its tests). Exceptions are declared here, not left implicit.

- **Mechanical rename exception.** A task may exceed five files when its change is one recorded `ast-grep` (or `gofmt -r`) rewrite with no behavior change, the command is pasted into the task and the PR, `go build ./...` (and `go vet ./...`, which compiles tests, where tests are renamed) is the verification, and the recount command's before and after output is attached to the PR. Applies to **Task 1.5.4a** (14 production files, the `Storage` wrapper rename) and **Task 1.5.4e** (31 test files, the same rename in tests). Splitting them by package was considered and rejected: the wrapper rename breaks every caller at once, so each half would leave the module not compiling; one atomic task costs less than two non-compiling halves. They land as consecutive commits of one PR (after 1.5.4a, `go build ./...` is green; `go vet ./...` is green only after 1.5.4e) and CI runs on the PR head.
- **Justified, not split: Task 1.5.3c (7 files).** Gate and reporter injection, the `-enforce-agent` flag registration, its proof entry and its truth-table rows must be one change so the flag can never exist without the reporter (ARCH-C3); each file edit is a few lines.
- **Justified, not split: Task 2.2.1a (8 files).** The Settings-index `<Link>` entry, `routes.settingsBacklogQuota`, the `nav-pages.ts` registry entry (with its pinning test) and the `flagExtras` link are each a one-to-three line edit around the new page; splitting would create tasks with no testable behavior of their own.
- **Task `- Files:` lines name real paths.** A token is a full repo path or a suffix of a path in the task's story `**Files**` line. The check unions every task's tokens into its story's set (a task may add its own test file that the story line lacks, and it then counts), and fails on a bare `test`/`tests` or "remaining files" in a task line, so the same-wave disjointness check covers test files. The only exception is the mechanical-rename Task 1.5.4e, whose set is a command output; it must sit in a wave with a single story (W11 holds only Story 1.5.4), which the script checks.

## Dispatch preconditions (the dispatcher checks these; W0 and before every wave)

W0 start:
1. **Planning artifacts committed.** `project_plans/backlog-make-it-true/` is untracked at `b416f5576` (`git status` shows `??`). Commit it on its branch per `docs/how-to/commit-sdd-planning-artifacts.md` before any worker is dispatched, or worker worktrees cannot read the plan and ADRs.
2. **WIP cap free.** At most 2 concurrent backlog work sessions (memory: backlog WIP limit; the wave table already schedules at most 2 stories). Before every wave the dispatcher lists the `in_progress` items on the live backlog and confirms two slots are free of non-project work; if another session holds one, it runs one story or waits. UNVERIFIED for W0: the live backlog MCP was unreachable (ECONNREFUSED) during this repair, so this is the dispatcher's first action.
3. **Triage-file overlap check** before W6 (Story 1.2.2), W11 (1.5.4), W12 (1.5.6) and W14 (1.5.8): the Worker brief's overlap check, naming the open item "Investigate false triage artifact does not exist error" (UNVERIFIED while the MCP is down). The result is recorded in the story PR.
4. The deployed-unit `--tmux-keep-server` check belongs to the owner before the first deploy; it is not a W0 condition.

Nothing else blocks W0: Stories 0.1.1 and 4.1.4 have no prerequisites.

---

## Domain Glossary
*(Exact names must be used in code, tests and comments.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `WorkflowEngine` | Existing interface: `CanTransition`, `AllowedTransitions`, `PendingGates`, `ValidateGates` (`session/workflow_engine.go:14`) | Unchanged |
| `DefaultWorkflowEngine` | Static-map engine wrapping `TransitionGuard` | Source of built-in guards |
| `ConfiguredWorkflowEngine` | Cache-backed engine over operator stage config | Never falls back by itself |
| `SwitchableWorkflowEngine` | New engine selecting off / shadow / enforce per call | `session/workflow_engine_selector.go` |
| `WorkflowEngineMode` | Sum type `Off`, `Shadow`, `Enforce` with unexported constructor and exhaustive switch; resolved per caller class by `ModeFor(class)` | Not a string. The banner label "Enforced" is the display name for `Enforce`; proto uses `ENFORCE`/`SHADOW`/`OFF` |
| `enforceAnswer` | The single private implementation of the enforced answer: Configured graph UNION `SystemRequiredEdges()` (on cfg_nil/cache_empty/load_error the fallback graph is Default UNION `SystemRequiredEdges()`, never plain Default), guards = Default `TransitionGuard` (per the caller's `guards` policy; input forwarded UNCHANGED so its own override semantics are preserved exactly) AND Configured operator gates; a non-empty `OverrideReason` additionally skips the Configured operator gates for class `operator` only, never the graph and never a Default guard that ignores override today. Takes the caller class. Used by `Enforce` and by `Shadow` | `session/workflow_engine_enforce.go` |
| `ShadowRecorder` | Counts and logs divergence between the legacy answer (Default engine answer, or "allow" for `Unchecked` callers) and `enforceAnswer` (never the raw Configured engine); read-only; does no stateful-gate I/O; wrapped in `recover()` | `session/workflow_engine_shadow.go` |
| `DivergenceClass` | Enum `graph`, `gate_missing`, `gate_extra`, `guard_new`, `error_kind`, `cache_empty` (`guard_new` = a caller whose legacy behavior was unchecked would be denied by a Default built-in guard under enforce) | Candidate is `enforceAnswer`. `gate_missing` = Default blocks, `enforceAnswer` passes, i.e. a composition defect (cannot occur from an ordinary guard denial); `gate_extra` = `enforceAnswer` blocks, Default passes (operator gate). `gate_builtin_absent_in_config` is an offline-harness-only informational label for the raw Configured engine |
| `DivergencePath` | Label `transition` or `read` | Soak criteria use `transition` only |
| `ConfigProvenance` | Enum `seeded`, `edited`: whether the live stage config equals the seed fingerprint | Labels divergence |
| `SeedFingerprint` | Hash of the seeded edges, enabled stage set, per-edge gate count and liveness settings | Compared to live config; any difference makes `config=edited` |
| `SystemRequiredEdge` | A `(from,to)` pair declared by a non-`forwarding` registry entry of class `agent` or `system`: engine-checked callers (`ready->queued`, `queued->in_progress`, `ready->in_progress`, SyncLoop `idea|refining|ready|queued->archived`, `AttachSessionToItem` `->in_progress`), storage-direct writers (`in_progress->review`, `review->pr_pending`, `pr_pending->done`, ...) and the archive edges (`X->archived` for every status X including custom stage slugs, and `archived->archived`) that `validTransitions` lacks. Cannot be disabled (gates stay editable) and is unioned into the enforced graph. The generic MCP delegate contributes none | Code-defined set, `session.SystemRequiredEdges()`; `validTransitions` and the seed are not changed. NOT in the tree today (`git grep -n 'SystemRequiredEdges\|IsSystemRequired' -- '*.go'` returns zero hits); Task 1.1.2b creates it and Task 1.5.1c adds the archive group |
| `EngineFallbackReason` | Enum `cfg_nil`, `cache_empty`, `load_error` | Label on `workflow_engine_fallback_total` |
| `TransitionCaller` | Registered entry `{name, class, legacy, guards, edges, forwarding}` for every call into the gate (`autonomous_complete`, `mcp_request_review`, ...); `forwarding` is true only for `mcp_transition_delegate` (pinned singleton) | `session/transition_callers.go`; the `caller` label on `workflow_engine_bypass_total` and `workflow_engine_gate_calls_total`. Replaces the earlier `BypassCaller` newtype |
| `TransitionCallerClass` | Sum type `operator` (human via UI/RPC), `agent` (AI session via MCP), `system` (reconciliation, lifecycle, pollers, sync, auto-reopen, triage pipeline, autonomous completion) | ADR-007 Decision 3 |
| `LegacyPolicy` | `Engine` (asks the engine today) or `Unchecked` (bare storage write today); Off and Shadow reproduce the legacy answer, so Off equals today's behavior exactly | Per `TransitionCaller` |
| `EngineGatedTransition` | Concept name for `Storage.GatedTransition` plus the sibling `GatedArchive`, `GatedUnarchive`, `GatedSetPRAndTransition`, `GatedReconcileStuck`, and the exempt `Rollback`, `DebugForce`; the only code allowed to call the six raw writers (`Raw*` on `Storage`, unexported `raw*` on `EntRepository`) |
| `HasOpenTransitionDenied` | `Storage.HasOpenTransitionDenied(ctx, itemID, status)`: true iff an open `transition_denied` row exists whose recorded `from` equals the item's current status; the one predicate the spawn points and the detector-local guards call before acting (ADR-007 D12) | `session/transition_gate.go` |
| `TransitionDeniedReporter` | Interface in `session`, `Report(item, caller, from, to, reason)`, implemented in `server/services` by adapting `transitionOrNotify`; the gate calls it on every `agent`/`system` denial; the gate unit test `TestGatedTransition_EveryAgentOrSystemDenial_ReachesReporter` is the guarantee that none is skipped | ADR-007 Decision 5 |
| `transitionOrNotify` | Service-layer helper (package `server/services`) that turns a denied automatic transition into a stuck reason, comment and `notify()`, idempotent per `(item, caller, from, to, reason)` (the caller name keeps two denial sources of the same edge, for example `autonomous_complete` and `maybe_trigger_triage` on `idea->ready`, from collapsing into one report). Also the implementation behind `TransitionDeniedReporter` so `session` callers reach it by injection | Replaces the `default:` log-and-continue arm of `claimAndSpawnCandidate` |
| `ErrTransitionNotAllowed` | Typed sentinel, defined in `session/domain/backlog.go` and re-exported as `session.ErrTransitionNotAllowed`; returned by the gate (and wrapped at `transitionWithGuard`) on a graph denial | Lets callers use `errors.Is` |
| `TransitionDeniedStuckReason` | New stuck reason `transition_denied` | Distinct from `orphaned_triage`, `bouncing` |
| `QuotaConfig` | Existing config struct for the quota gate | `config/types.go:743` |
| `QuotaMode` | Derived read-only enum `Disabled`, `ReactiveOnly`, `Proactive` | Computed from `Enabled` and `AssumedWindowTokenBudget`; never stored |
| `DefaultCostBudgetThresholdUSD` | New global soft-budget default, `*float64` USD, nil = off (the one Go name used in plan, ADR-004 and ADR-006) | `0.0` is a valid threshold |
| `CostBudgetThreshold` | Existing per-item soft budget (`cost_budget_threshold_usd`) | Overrides the default |
| `AutoMergeLookupStatus` | Proto enum `UNSPECIFIED`, `OK`, `RATE_LIMITED`, `NO_PERMISSION`, `NOT_FOUND`, `ERROR` | `UNSPECIFIED` = not applicable, UI omits line |
| `RepoSettings` | Result `{Status, Reason, FetchedAt}` plus accessors `AutoMerge() (allowed, ok bool)` and `SquashMerge() (allowed, ok bool)` that return `ok=true` only when `Status==OK`; the raw bools are unexported so `AllowAutoMerge=false` cannot coexist with a failed lookup | Package `github` |
| `RepoSettingsCache` | In-memory TTL + ETag cache keyed `(host, owner, repo)` with singleflight | Separate from PR `ETagCache` |
| `NudgeOutcome` | Enum `delivered`, `uncertain`, `slot_unavailable`, `fallback_respawn`, `skipped_cap`, `skipped_not_idle` | Label on `nudge_outcome_total` |
| `NudgeKind` | Enum `stall`, `diagnose` | Label |
| `FlagSource` | Enum `default`, `explicit` for an effective flag value | Shown in UI |
| `InertReason` | Text explaining why an On flag has no effect (for example prerequisite off) | Served in `statusDetail` |
| `ConfigSurface` | A settable field, flag, or RPC request field that an operator can change | Unit of the proof registry |
| `ConfigSurfaceProof` | Registry entry mapping a `ConfigSurface` to a runtime-effect test and entry point | `server/services/config_surface_proof_test.go` |
| `EngineModeBanner` | Always-rendered UI banner showing Enforced / Shadow / Off / Unknown on the Stages page | Fails closed |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Engine swap | Facade / Strategy selected per call (`SwitchableWorkflowEngine`) | GoF Strategy + Proxy | Direct constructor swap | Drops guards, denies all on load failure |
| Rollout of A | Shadow-then-enforce with live flags | git-backend cohort precedent | Big-bang flip | State-machine hotspot, no oracle for in-flight items |
| Guard composition | Decorator: Default guards always, Configured gates added | GoF Decorator | Seed guards as DB rows (arch option B) | Needs ~5 new check IDs and per-edge migration; lets operators delete safety rails |
| Graph/guard fallback | Fail-open to Default on graph, fail-closed on gates | Resilience patterns | One generic fallback | Wrong for one of the two (`pitfalls` A5) |
| `WorkflowEngineMode` | Sum type with exhaustive switch | type-driven-design | `bool` pair / string | Four flag combinations collapse to three legal modes |
| `AutoMergeLookupStatus` | Sum type / enum with reason | type-driven-design | `optional bool` | Cannot express "lookup failed" vs "not asked" |
| `QuotaMode` | Derived value, not stored field | Make illegal states unrepresentable | Stored `proactive_enabled` bool | No schema, cannot drift from budget |
| `DefaultCostBudgetThresholdUSD` | `*float64` pointer as option | Go idiom + proto comment | `0 = off` | `0.0` is a valid threshold |
| Quota settings service | Service Layer over config read-modify-write (Transaction Script inside) | PoEAA | Domain Model | Simple CRUD-like settings; no invariants beyond validation |
| Repo settings lookup | Gateway + cache-aside + singleflight | PoEAA Gateway | Persist on item | Staleness, ent schema change |
| Transition-denied handling | Shared helper (Extract Function) in `server/services`, one real adopter plus extraction of the claim-error switch | Refactoring | Patch four cited sites; place the helper in `session` | Three of four cited sites were not transition sites; `session` code does not call it, so no cycle |
| Shadow candidate | Compute the exact enforce answer once (`enforceAnswer`), share it between modes | DRY / Strategy | Compare Default to raw Configured | Raw Configured has no built-in guards, so every guard denial would count as divergence |
| Status-write governance | Single choke point (`Storage.GatedTransition`) + closed caller registry + six raw writers unexported/renamed + repo-wide inventory test (strict at 1.5.9a, including a `.SetStatus(` rule) + a gate unit test that every `agent`/`system` denial reaches the reporter + the existing `silenttransition` analyzer extended to `Raw*` and to `Gated*` calls of `operator`-class callers only | ADR-007; owner decision | Counter-only measurement (ADR-002 D1); a new lexical analyzer as the primary mechanism | Metric 1 must hold on all paths; unexport makes a stray repo-level call a compile error and the rename breaks every old `Storage` call (`build-vs-buy` E2 rejects a lexical analyzer as the sole guard) |
| Per-class rollout | One enforce flag per caller class composed with a global shadow flag; each flag is registered in the task that makes it safe (agent with the reporter wiring, system with the recovery guards), so no runtime degrade clause is needed | git-backend cohort precedent | One flag; one flag with class scope | Asymmetric risk and independent revert; flag RPC is bool-only; scope vocabulary unverified for class keys |
| Denial visibility | Reporter injected into the gate (`TransitionDeniedReporter`) | Dependency inversion; avoids `session` importing `server/services` | Each caller remembers to call `transitionOrNotify` | Recurring bug shape "swallowed status-transition"; the choke point cannot forget |
| Config-surface proof | Registry + reflection test | ADR-006 | Analyzer, proto-diff grep | Gameable / lexical |
| System-required edges | Code-defined set | Single source of truth | `system_required` DB column | Schema churn for what code already knows |
| Mode flags | Two bool flags composed to a mode | Existing flag RPC is bool-only | New tri-state flag type | Reuses RPC, panel, scopes, audit |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `server/services/backlog_service_lifecycle.go` `TransitionBacklogItemStatus` (`:870`, about 41 complexity, 1,465-line file) | Hotspot; guard-input assembly duplicated with `buildPendingGatesGuardInput` (`backlog_service_gates.go:148-170`) | **Isolate via seam** through Epic 1.1 (handler untouched); then **Refactor-first** in Story 1.5.5a: Task 1.5.5e (guard-input extraction) becomes a precondition and lands as its own commit before the handler's three engine/storage calls are replaced by one `Storage.GatedTransition` call | The handler must route through the gate (ADR-007). Extraction lowers its complexity; `make ready-complexity-gate` runs on both commits |
| Three coexisting engines (BacklogService, lifecycle listener `dependencies.go:1544`, `SyncLoop` `backlog_sync.go:57,69`) | Config can disagree across holders | **Isolate via seam** (selector injected to BacklogService and SyncLoop; listener scoped by comment and test) | Task 1.1.4b-c |
| Direct status writers: 31 `storage.TransitionBacklogItemStatus` sites in 10 files plus 9 sites of `ArchiveBacklogItem`/`UnarchiveBacklogItem`/`SetBacklogItemPRAndTransition`/`ReconcileStuckItems` (recounted by `git grep`, see Story 1.2.3 and ADR-007), including 10 direct transition sites in `backlog_service_triage.go` | No graph/gate check | **Route through the engine** (owner decision): inventory test first with a ratcheted pending list, then one choke point, then migrate by caller group per wave, then strict guard (Epic 1.5); the 5 exempt sites (3 rollbacks, 2 debug) stay exempt via named entry points and creation is the constrained `create` class | ADR-007 supersedes ADR-002 D1; risk managed by per-class shadow to enforce rollout |
| Log-and-continue in `claimAndSpawnCandidate` (`backlog_service_triage.go:837`, `default:` arm `:872-873`) | An operator gate or graph denial logs a Warn every sweep and strands the item queued | **Refactor-first**: Extract `handleClaimError`, then adopt `transitionOrNotify` (Story 1.2.2); complexity gate checked on both commits | Gate applies to modified existing functions; earlier cites (`:801-810`, `:2313-2321`, `backlog_lifecycle.go:971-984,1971-1975`) were not engine-deniable sites |
| `ConfiguredWorkflowEngine` swallowing load errors (`:78-85`) and nil `stageConfigEngine` (`dependencies.go:773-778`) | UI says saved, runtime unchanged | **Refactor-first (small)**: `HasAnyStage()` accessor, startup log, `invalidator_wired` in status RPC | Tasks 1.1.2a, 1.1.4a |
| `settings/features/page.tsx` toggle (`aria-pressed`, 24px height) and per-flag `name ===` special cases | A11y defect and special-case pile; we add flags to it | **Refactor-first** (Story 4.1.4: switch semantics, `flagExtras` registry) before adding consumers | Collateral debt directly in the touched component |
| `no_ent_in_services` (16 files import `session/ent`) | Out-of-scope debt | **Extend as-is** | Constraint only: add no new ent import in `server/services`; do not remove exclusions here |
| `session/workflow_stage_seed.go` create-if-count-zero | Seed drifts from code; gates never seeded | **Extend as-is** with `SeedFingerprint` for comparison; no re-seed | Guards stay code (ADR-001); drift is detected, not repaired |
| Stale comment `session/backlog_lifecycle.go:271-274` | Says no ConfiguredWorkflowEngine is wired | Fix in Task 1.1.4b | Same files being edited |

---

## Migration Plan

- **Code additions without schema impact**: package helper `config.UpdateConfig` (Task 2.1.1c) for read-modify-write of `config.json`, adopted by `FeatureFlagService` (Task 2.1.1d); `Storage.GatedTransition` and the `Raw*` renames (Epic 1.5) are source-level only (no data change).
- **Schema/data**: no ent schema change. No change to `validTransitions`, the seed or any row: the legitimate off-graph archive edges (`in_progress|review|pr_pending->archived`, `archived->archived`) live in the code-defined `SystemRequiredEdges()` that the selector unions at read time (Task 1.5.1c, ADR-007 D7), so all four flags off equals today exactly and an existing database needs no migration (a stale-seed test pins it). System-required edges are a code-defined set (ADR-002 D3, ADR-007 D4); `allow_auto_merge` is in-memory (ADR-003); quota/flags persist in the existing `config.json`.
- **Config additions** (additive, absent = old behavior): `DefaultCostBudgetThresholdUSD` in `config.json`; four new feature flags (`backlog:workflow-engine-shadow`, `backlog:workflow-engine-enforce` for class operator, `backlog:workflow-engine-enforce-agent`, `backlog:workflow-engine-enforce-system`). **Proto additions** (additive fields/RPCs, `make proto-gen`, generated output not committed): `AutoMergeLookupStatus` and `auto_merge_checked_at` on the stuck item, `GetWorkflowEngineStatus` (per-class modes, `gate_wired`), `GetQuotaConfig`/`UpdateQuotaConfig`, and stuck-reason fields `from`/`to`/`denied_reason` for `transition_denied`.
- **Reversibility**: every change is revertable by PR revert; flags revert live. Ledger files (`workflow-engine-soak.json`, `nudge-outcomes.json`) are advisory and safe to delete.
- **Zero-downtime**: shadow defaults on but read-only; every enforce flag defaults off; Off and Shadow reproduce today's behavior for every caller (engine-checked callers get the Default answer, unchecked callers get allow). No restart of the live service is performed by this project (see the Worker brief standard block). The owner deploys on their own schedule and first confirms the deployed unit passes `--tmux-keep-server` (`docs/explanation/tmux-keep-server-on-restart.md`); otherwise the restart kills every live tmux session, including worker sessions.
- **Rollback procedure**: (1) flip the enforce flag of the affected class off (live, about 1s); (2) if the selector or gate misbehaves, flip all four flags off (every class Off equals today's behavior exactly); (3) code-level: revert the construction at `dependencies.go:1477` and the gate injection (Task 1.5.3c); the `Raw*` renames make a partial revert fail to compile, so revert Epic 1.5 as a unit.

## Observability Plan

- **Logs** (slog, structured): startup line `workflow engine: mode=<m> configured_loaded=<bool> invalidator_wired=<bool> binary_started_at=<ts>`; denied transition `from,to,reason=config_graph|operator_gate|default_guard`; fallback WARN rate-limited to one per minute per reason; quota startup/`Using` line extended with effective `QuotaConfig` values and `QuotaMode`; `transition_denied` stuck-reason events; nudge outcome line per attempt; repo-settings lookup outcome at Debug, failures at Warn.
- **Metrics / actuator**: `workflow_engine_shadow_calls_total{class,method,path}`, `workflow_engine_shadow_divergence_total{class,method,divergence,config,path}` (candidate = `enforceAnswer`, never the raw Configured engine; baseline = the caller's legacy answer), `workflow_engine_shadow_not_compared_total{reason}`, `workflow_engine_shadow_errors_total` (recovered Shadow panics), `workflow_engine_fallback_total{reason}`, `workflow_engine_enforce_denied_total{class,source}`, `workflow_engine_gate_calls_total{class,caller,mode}`, `workflow_engine_gate_unwired_total`, `workflow_engine_bypass_total{caller}` (the exempt `rollback`, `debug` and `debug_create` entry points only), exposed at `GET /actuator/workflow-engine[?format=text]` with `config_diff` and `stranded_items` previews; `nudge_outcome_total{kind,outcome}` at `GET /actuator/nudge-outcomes`; `github_repo_settings_lookup_total{outcome,reason}` on the existing counter surface. `path` is `transition` (a real or attempted state change through the gate) or `read` (UI `AllowedTransitions`/`GetPendingGates`, sampled 1 in 10 in Shadow). Counters are atomic or sharded (no mutex on the call path). Ledgers persist per minute so a restart does not reset a soak window and record binary start time and every earlier binary start so history cannot be read from one young binary.
- **Ledger rules (pre-mortem #6)**: ledgers store aggregates over closed-enum labels plus a ring buffer of the last 200 repro records in total (not per run); a session id is a log field, never a label; the ledger path resolves via `config.GetConfigDir()` (instance and workspace aware, so a manual instance or `go test` never overwrites the live soak window; tests use `t.TempDir()`); a corrupt or oversized ledger starts fresh with a WARN.
- **Alerts**: no pager. `make workflow-engine-soak-status` is evaluated per class. It exits 1 (NOT READY) when any of: (a) `graph`, `gate_missing`, `gate_extra`, `error_kind` is greater than 0 under `config=seeded` for any class, or, for a class whose callers were `Unchecked`, any divergence (including `guard_new`) on a `(caller,from,to)` the owner has not acknowledged in the runbook ack table; (b) total `path=transition` samples for the class are below the floor; (c) ledger history plus `binary_started_at` span less than W; (d) any named expected edge of the class (from the caller registry) has fewer than N samples and no owner acknowledgement (so an empty or silently-off soak can never read READY); (e) the mode resolver ever returned Off or Shadow-disabled during the window (shadow flag unset). Exit 0 ready, 1 not ready, 2 unreachable. Because the candidate is the exact enforce composition, an ordinary guard denial (no AC, no PASS verdict) is not a divergence for an `Engine` legacy caller; `gate_missing` can only mean a composition defect. Raw-Configured gate coverage (`gate_builtin_absent_in_config`) is informational in the offline harness only. For an edge or class that the ledger marks low-volume, (b) and (d) are satisfied by the four-part owner acknowledgement defined in Risk Control instead of sample counts; (a), (c) and (e) always apply.
- **Soft-budget crossing (C10)**: `[BudgetWarning]` is a `log.WarningLog().Printf` line (`server/services/backlog_service.go:1467,1489`, verified by reading the function bodies) in `~/.stapler-squad/logs/staplersquad.log`. Nothing else surfaces it; Story 2.1.3 and the quota page copy say so. Routing the crossing to `notify()` or the stuck panel is an owner decision (Unresolved Questions) and is not in this plan.

## Authn/authz

VERIFIED by reading `server/server.go` and `server/middleware/` at `b416f5576`; nothing here is inferred from the live service.

- **What exists**. The local listener (`:8543`) chain is `Logging -> CORS -> Compress -> HostGuard -> [authMiddleware | LocalWriteGuard] -> mux` (`localChainWith`, `server/server.go:1927-1945`). `HostGuard` covers every route. If `authMiddleware` is configured it enforces a passkey session cookie (`cs_auth`) or `Authorization: Bearer` token on every non-exempt path (`server/middleware/auth.go:149-170`; exempt prefixes are only `/auth/`, `/login`, `/health`, `/_next/`, `/favicon` plus three `/internal/*` peer paths and generic webhooks, `auth.go:95-130`). If it is not configured, the only per-procedure protection is `LocalWriteGuard` over `guardedProcedures`, which has exactly five members: `ProbeProgram`, `NudgeSessionForPR`, `PruneHiddenSessionNotifications`, `UpdateFeatureFlag` and `ReplyToPendingQuestion` (`server/server.go:1907-1914`). The remote listener (`:8444`, `remoteChain`) applies the auth middleware (the code comment says `main.go` always passes `middleware.Auth`; a nil one only occurs in tests).
- **Consequence for this project**. `UpdateFeatureFlag` (so every new mode flag) is already guarded with `ProfileRebinding`. The stage, edge and gate write RPCs (`CreateStage`, `UpdateStage`, `DeleteStage`, `CreateStageTransition`, `UpdateStageTransition`, `DeleteStageTransition`, `CreateTransitionGate`, `UpdateTransitionGate`, `DeleteTransitionGate`, `proto/session/v1/backlog.proto:1668-1710`) are NOT in `guardedProcedures`; today they are protected only by `HostGuard` on the local listener and by auth on the remote one. Whether their handlers check the caller in-handler was not checked (UNVERIFIED). That gap becomes consequential once Epic 1.5 lands, because those writes then decide which transitions the whole factory may make. `/actuator/*` routes are registered on `s.mux` (`server/actuator.go:29-31`), so they sit behind the same chain and are not in the exempt list.
- **Required (Task 2.1.1e, tested)**: (1) `UpdateQuotaConfig` and the nine engine-affecting stage/edge/gate write RPCs above join `guardedProcedures` with `ProfileRebinding`, the same pattern as `UpdateFeatureFlag`; (2) the new read-only RPCs `GetQuotaConfig` and `GetWorkflowEngineStatus` and the endpoints `/actuator/workflow-engine` and `/actuator/nudge-outcomes` add NO exemption: with auth configured they need a session, and the actuator `config_diff` carries item id hashes, never titles; (3) the flag writes need no new code (same RPC) but a test pins the existing membership; (4) `RecordGateApproval` is left as is (it is an approval by a human or agent, not a configuration write) and recorded as a known pre-existing exposure, not widened. Risk: `ProfileRebinding` rejects non-POST and foreign-Host/Origin requests, so Task 2.1.1e's manual-instance check confirms the Stages UI still saves and the MCP stage tools (if any call these RPCs) still work.
- **Test ACs** are under Story 2.1.1 (guard membership, foreign-Host POST rejected with config unchanged), Story 1.3.1 (read RPC needs a session on the remote chain) and Story 1.1.3 (actuator routes not exempt); rows in `validation.md` NFR-2.

## Risk Control

- **Feature flags**: `backlog:workflow-engine-shadow` (default **on**, read-only), `backlog:workflow-engine-enforce` (class `operator`), `backlog:workflow-engine-enforce-agent` (registered in Task 1.5.3c with the reporter wiring), `backlog:workflow-engine-enforce-system` (registered in Task 1.5.8g, after the recovery guards; all default **off**), all live-settable via the flag RPC/panel. No env vars. Per-class flags rather than one flag or class scope: ADR-007 Decision 8. Per-from-stage scope for the operator flag remains subject to Task 0.1.1b (global-only is the stated fallback). Quota gate stays `Enabled=false` by default. The three autonomy flags keep code default off (ADR-005).
- **Staged rollout (per class)**: shadow on merge for all classes; enforce one class at a time in the order operator, agent, system, each only after its own soak criteria in `docs/how-to/flip-workflow-engine-enforcement.md` hold. Criteria are per class over that class's registered expected edges (`path=transition`). **Parameters, owner-approved 2026-10-17 exactly as proposed (pre-mortem #10; no longer an open decision)**: W = 7 days (operator, agent) and 14 days (system), N = 20 samples per observed expected edge, a minimum total-sample floor of 100 transition calls per class, and a review date = 14 days after the first deploy of Epic 1.5 (the soak-progress sizing is INFERRED from the git soak, not measured). **Forcing function against "the flip never happens"**: the review date is a dated task, not a hope. (a) The runbook (Story 1.4.1) opens with a "Review dates" block that the owner fills in at deploy (`first deploy: <date>`, `review due: <date + 14d>`) and a ready-to-paste calendar reminder line; (b) `make workflow-engine-soak-status` prints `REVIEW DUE <date>` computed from the earliest `binary_started_at` in the ledger and, once that date has passed with no class enforced, `REVIEW OVERDUE: no class is enforced; run the Outcome review` (exit code unchanged, advisory); (c) the runbook's "Outcome review" step records requirements Metric 6 (stuck-item rate and manual interventions) beside the soak verdicts, so the review is about the factory outcome and not only the counters. A one-shot `--config-diff` (with stranded-item preview) runs in the first deployed hour. The owner acknowledges each previewed stranded item, each `UNVERIFIED in shadow` stateful-gate edge and, for newly governed unchecked callers, each divergent `(caller,from,to)` before flipping that class.
  - **N and the floor are unreachable for the operator and system classes at the live volume** (about 21 live items, INFERRED from the re-review; tens of transitions per day at most, ADR-007 D10), and seeded config carries no gates, so shadow can only prove graph and guard parity until the config is edited. They stay as targets and are printed as evidence, but they are not what justifies a flip. **READY for a low-volume edge or class** means the zero-divergence rule under `config=seeded`, span W, no vacuity (every expected edge either has samples or is named in the ack file as not exercised) AND the owner's acknowledgement of four artifacts: (1) the `--config-diff` output, (2) the stranded-item preview, (3) the Story 1.5.9b every-registered-caller table, (4) the Story 1.5.9d replay report run against a COPY of the live database, which applies a synthetic `human_approval` gate to each system-required edge so the `edited` path is exercised once before any flip.
  - **The `system` flag cannot be flipped early because it does not exist yet**: `-enforce-system` is registered by Task 1.5.8g, the last task of Story 1.5.8, so the wave order (not a code degrade clause) controls the hazard and every flip is owner-gated after soak (ADR-007 D8). The runbook states the dependency: the system-class step requires the Story 1.5.8 completeness test green on the deployed binary.
- **Test environment**: manual instance recipe through `scripts/verify-manual-instance.sh <N>` (Task 0.1.1d; ports 62871-62874, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-<N>`, one slot per concurrently running story as listed in the wave table); it has its own DB, so production-row shadow data comes only from the live service behind the flag.
- **Latency and safety of the gate**: `recover()` around the Shadow branch; `read` path sampled; microbenchmark gate in Task 1.5.3d (ADR-007 Decision 10).
- **WIP cap**: at most 2 concurrent backlog work sessions; the wave plan below schedules at most 2 stories per wave.

## Unresolved Questions

- [x] **CONFIRMED by owner 2026-10-18** **Metric 1 carve-out for system-required edges (the single explicit carve-out)**: the editor rejects disabling any edge that a purpose-specific `agent` tool or a `system` caller can request (ADR-007 Decision 4), including every `*->archived` edge. For those edges Metric 1 is satisfied by a defined rejection plus fully editable gates, not by allowing the disable. The generic MCP `transition_backlog_item_status` delegate is exempt from the carve-out (`forwarding`). The list is generated from `SystemRequiredEdges()` and recorded in the PR template. Confirm this reading of "editing a transition must change the result". Alternative A (higher risk): allow the disable and let denied automation surface as `transition_denied`. Alternative B (archive only): add `in_progress|review|pr_pending->archived` to `validTransitions` and the seed instead; not chosen because it breaks the "all four flags off equals today" invariant (`session/workflow_engine_test.go:22` pins the opposite), leaks into the UI's `allowed_transitions`, and does not reach an already-seeded database. Owner: Tyler. Needed before W5 starts (Story 1.2.1 implements the rejection and Alternative A would reverse it); blocks nothing in W0-W4.
  - **Archive-from-active sub-choice: RESOLVED by the owner 2026-10-17 as option (ii)/A**: archive from `in_progress`, `review`, `pr_pending` (and re-archive `archived->archived`, and custom stage slugs) is documented as UI-ungateable; an operator who needs a gate there attaches it to `done->archived`. Carve-out width: the edges generated by `SystemRequiredEdges()` (queueing edges, SyncLoop archives, the group-2 PR/review edges, and the archive group), of which exactly those four plus the custom-slug `to=archived` predicate cannot carry a gate from the UI. The Story 1.5.5 gate AC is therefore only reachable on `done->archived`. Option (i) (virtual rows in the editor) is not built. The Stages page note and `design/ux.md` carry the fixed wording.
- [ ] **Operator-gate policy for the `system` class (new)**: ADR-007 Decision 3 applies Configured operator gates to automation and skips Default built-in guards. Confirm. Owner: Tyler. Blocks Story 1.5.2.
- [ ] **Agent `override_reason`**: agents get none and cannot bypass operator gates (ADR-007). Confirm. Owner: Tyler.
- [x] **Soak parameters and review date** (W, N, floor, review date): RESOLVED, owner-approved 2026-10-17 as proposed in Risk Control. Removed from the pre-W0 blockers.
- [ ] **Soak start date** of the three autonomy flags (not recorded). Blocks the evidence claim in the promoted ADR-005 (Task 4.1.1a). Owner: Tyler, or derive from flag-service audit lines.
- [ ] **Q5**: exit criteria to flip any code default for `backlog:sdd-default-pipeline`, `diagnose_nudge_enabled`, `autonomous_stall_nudge_enabled` (ADR-005 §5 holds INFERRED placeholders). Blocks only owner-gated Task 4.1.5a. Owner: Tyler.
- [ ] **Q4**: derive `AssumedWindowTokenBudget` from observed usage (future enhancement; mitigated by an explicit budget and a visible warning at 0). Blocks nothing. Owner: Tyler.
- [ ] **"Expected off" repo list**: this repo has `allow_auto_merge` disabled (`session/backlog_lifecycle.go:1925-1935`, comment, UNVERIFIED live), so the lookup reports not-allowed for every item here. Plan ships dedupe per repo per 24h plus neutral copy; decide whether an operator-maintained list is wanted. Does not block Story 3.1.4. Owner: Tyler.
- [ ] **Per-from-stage scope for the operator enforce flag**: depends on Task 0.1.1b. Blocks Task 1.1.3e only; global-only ships regardless.
- [ ] **Soft-budget crossing surface (C10)**: keep log-only (as shipped) or route to `notify()`/stuck panel. Does not block Story 2.1.3. Owner: Tyler.
- [x] **Quota page unsaved-changes handling**: RESOLVED by the triad UX repair: ship a navigation guard (`beforeunload` plus an in-app link and back-navigation guard using `AlertConfirmDialog`) in addition to the visible "Unsaved changes" line (Task 2.2.1c, UX-38).
- [ ] **Remaining `config.SaveConfig` callers** (`defaults_service.go`, `jules_config_service.go`, `remote_service.go`, `Config` setters) still do unlocked read-modify-write; migrating them onto `config.UpdateConfig` is a follow-up outside this project.
- [ ] **Pre-existing gap kept, not widened**: autonomous completion skips the AC-required gate on triage to ready (`autonomous_orchestration_service.go:586`); under ADR-007 the `system` class skips Default built-in guards by default, so this stays. Owner decides whether to give that one caller `guards=BuiltIn`.

**What each open owner confirmation blocks (round 7).** None of the open confirmations blocks W0 through W4.

| Confirmation | Blocks | Ask by |
|---|---|---|
| Metric 1 carve-out reading | Story 1.2.1 (W5): the validator rejection is the carve-out | before W5 is dispatched |
| System-class operator-gate policy (ADR-007 D3) | Story 1.5.2 (W9), Task 1.5.2a (class policy in `enforceAnswer`); W10-W15 build on it | before W9 is dispatched |
| Agent `override_reason` (agents get none) | Task 1.5.2a (W9) agent policy; MCP writers, Task 1.5.5c (W12) | with the system-class ask, before W9 |
| Soak start date of the three autonomy flags | Task 4.1.1a only (W18) | before W18 |
| Q5 flag-default exit criteria | Task 4.1.5a only (owner-gated, off the critical path) | not scheduled |
| "Expected off" repo list; soft-budget surface (C10); Q4; per-from-stage scope | nothing (per-from-stage scope blocks Task 1.1.3e only, global-only ships regardless) | n/a |

**Resolved by owner decision 2026-10-17 (round 6)**: soak parameters and low-volume READY rule; archive-from-active edges UI-ungateable (option A). **Resolved by earlier repair rounds**: `override_reason` versus Configured gates (ADR-001); `SaveConfig` locking (ADR-004). **Resolved by the owner decision of 2026-10-17**: whether to reword Success Metric 1 (no: keep it strong, ADR-007); whether the 31 direct-storage sites are governed (yes).

## Dependency Visualization

```
Phase 0 spikes: 0.1.1 (a b c)
        |
        v
 [A] 1.1.1 harness -> 1.1.2 selector + enforceAnswer -> 1.1.3 shadow/actuator/flags -> 1.1.4 wiring
                           |                                                              |
                           v                                                              v
                    1.2.1 validator -> 1.2.2 transitionOrNotify -> 1.2.3 counters + inventory test (pending list)
                                                                        |
                    Epic 1.5 (ALL status-write paths, owner decision):  v
                    (1.2.1 + 1.2.3) -> 1.5.1 registry + edge audit -> 1.5.2 class-aware selector (shadow + operator flags) -> 1.5.3 gate + reporter + agent flag (choke point; exit = overrun checkpoint)
                      -> 1.5.4 close the six raw writers + silenttransition (operator sites) + tests -> { 1.5.5 operator+agent writers, 1.5.6 system writers (services),
                                                         1.5.7 system writers (session) }
                      -> 1.2.4 integration proofs (needs 1.5.5) ; 1.5.8 denied-transition recovery + system flag (needs 1.5.6, 1.5.7)
                      -> 1.5.9 strict guard + per-class Metric 1 proofs
                    1.5.2 -> 1.3.1 status RPC -> 1.3.2 banner UI -> 1.3.3 flags UI ; 1.3.3 + 1.5.9 -> 1.4.1 runbook
                                                       (soak: wall-clock, owner, per class) -> enforce flips (operator, agent, system)
 [E] 5.1.1 proof framework + ratcheted pending list (BEFORE any entry is registered) -> 5.1.2 entries -> 5.1.3 backfill (list empty)
 [D] 4.1.4 features-page legibility + a11y + AlertConfirmDialog + contrast (all mandatory) -> 4.1.2 nudge counter -> 4.1.3 single-source defaults -> 4.1.1 ADR promote
 [B] 2.1.1 quota RPC (+2.1.1d flag writes) -> 2.1.2 gate resume + startup log -> 2.1.3 soft-budget default -> 2.2.1 quota page
 [C] 3.1.1 github lookup -> 3.1.2 proto+service -> 3.1.3 UI line, 3.1.4 PR preflight + cache warm
 [Z] 6.1.1 e2e -> 6.1.2 registry + make ready + manual-instance script (last, alone)
```

### Waves (one count, cap enforced)

Twenty-one waves, W0 to W20. Each wave runs at most 2 stories concurrently (WIP cap), generated from the dependency graph above. A wave starts when the previous wave's stories have merged. The "Manual slot" column assigns `manual-<N>` (ports 62871/62872 for N=1, 62873/62874 for N=2) to each concurrently running story so two stories never share a port, binary or tmux socket (pre-mortem #9).

| Wave | Stories (max 2) | Manual slot | Why this pairing |
|---|---|---|---|
| W0 | 0.1.1 / 4.1.4 | 1 / 2 | Spikes, characterization and the guarded manual-instance script (Task 0.1.1d); 4.1.4 has no backend dependency and is mandatory (legibility, switch a11y, `AlertConfirmDialog`, contrast fix; see C9) |
| W1 | 1.1.1 / 5.1.1 | 1 / 2 | Harness and proof framework; 5.1.1 needs only existing shapes and ships a ratcheted pending list covering every existing and planned surface |
| W2 | 1.1.2 / 2.1.1 | 1 / 2 | Selector; quota RPC plus the `UpdateConfig` adoption by `FeatureFlagService` |
| W3 | 1.1.3 / 2.1.2 | 1 / 2 | Shadow recorder and flags; quota gate resume and startup log |
| W4 | 1.1.4 / 2.1.3 | 1 / 2 | Wiring; soft-budget default |
| W5 | 1.2.1 / 2.2.1 | 1 / 2 | Validator (with stranded-item guard); quota page |
| W6 | 1.2.2 / 3.1.1 | 1 / 2 | `transitionOrNotify` and stuck-reason proto; github lookup |
| W7 | 1.2.3 / 3.1.2 | 1 / 2 | Counters and inventory test with ratcheted pending list; populate service layer |
| W8 | 1.5.1 / 3.1.3 | 1 / 2 | Caller registry and edge audit; auto-merge UI line |
| W9 | 1.5.2 / 3.1.4 | 1 / 2 | Class-aware selector (`ModeFor(class)`, shadow and operator flags); pre-PR warning |
| W10 | 1.5.3 | 1 | The choke point, reporter wiring and the `-enforce-agent` flag (touches `server/dependencies.go`, `feature_flag_service.go`; alone). **Exit gate: Task 1.5.3e overrun checkpoint**, record actual versus table CU for W0-W10; if a trigger fires (cumulative +25%, Story 1.5.3 +25%, any story +50%, or usage unreadable) W11 waits for the owner's reply |
| W11 | 1.5.4 | 1 | Close the six raw writers; alone: it renames production callers in 14 files and 31 test files, including `autonomous_orchestration_service.go` (which Story 4.1.2 also edits, so 4.1.2 moved to W16) |
| W12 | 1.5.5 / 1.5.6 | 1 / 2 | Operator and agent writers (lifecycle file, gates, MCP); system writers in services (autonomous, triage, trigger-triage, sync, debug). `forceResetItem` and the `TriggerTriage` RPC writes are owned by 1.5.6 only, so the pair is file-disjoint (script check below). **Before dispatch: Task 1.5.6h overlap check** (open backlog items and PRs touching the triage files; do not run concurrently with another session editing them) |
| W13 | 1.5.7 / 1.2.4 | 1 / 2 | System writers in `session`; operator-class integration proofs (needs 1.5.5) |
| W14 | 1.5.8 / 1.3.1 | 1 / 2 | Denied-transition recovery with the shared predicate at the spawn points and the detector-local sites, then the `-enforce-system` flag (touches `feature_flag_service.go`, `config.go`); status RPC (touches `dependencies.go`, `backlog.proto`) |
| W15 | 1.5.9 / 1.3.2 | 1 / 2 | Strict guard, per-class proofs and the live-database replay; banner |
| W16 | 1.3.3 / 4.1.2 | 1 / 2 | Mode flags UI (4 rows); nudge counter (`dependencies.go`, `backlog.proto`, `autonomous_orchestration_service.go`; Epic 1.5 is done, so no collision) |
| W17 | 1.4.1 / 4.1.3 | 1 / 2 | Runbook (per class); single-source defaults |
| W18 | 5.1.2 / 4.1.1 | 1 / 2 | Per-story proof entries; ADR promote (needs 4.1.3) |
| W19 | 5.1.3 / 6.1.1 | 1 / 2 | Backfill (pending list must reach empty); Playwright specs (all UI stories done) |
| W20 | 6.1.2 | 1 | `make registry-generate`, `make ready`, manual-instance verification script, alone because it verifies the whole branch |

Sequencing rules enforced by the table (the pairing is re-checked by script, `plancheck`: every story appears once, at most 2 per wave, every `Prerequisites:` story is in an earlier wave, and the union of the `Files:` lines of the two stories in a wave is disjoint; run after any change to a `Files:` line or a wave row; round 7: task-level `- Files:` tokens are resolved to full paths and included, and a bare `test`/`tests`, an unresolved name or "remaining files" fails the check unless the task is the mechanical-rename Task 1.5.4e in a one-story wave):
- `server/dependencies.go` touches, one per wave: 2.1.1 (W2, quota closure/registration), 1.1.4a and 1.1.4b (W4), 3.1.2 (W7, `RepoSettingsCache` injection), 1.5.3c (W10, gate injection and startup line), 1.3.1 (W14, status RPC), 4.1.2 (W16, nudge recorder). No two stories in the same wave touch it.
- `proto/session/v1/backlog.proto` editors: 1.2.2a (W6, includes the `transition_denied` `caller`/`from`/`to`/`denied_reason` fields so Story 1.5.8 needs no proto change), 3.1.2a (W7), 3.1.3a (W8), 1.3.1a (W14, no `recovery_wired` field), 4.1.2b (W16), each in a different wave.
- `features/page.tsx`: 4.1.4 (W0), 1.3.3a (W16); 2.2.1 adds its Features-to-Quota link through the `flagExtras` registry (`flagExtras.tsx`), not `page.tsx`.
- `feature_flag_service.go`: 4.1.4c (W0), 2.1.1d (W2), 1.1.3c (W3), 1.5.3c (W10, `-enforce-agent`), 1.5.8g (W14, `-enforce-system`), 4.1.3a (W17).
- `config/config.go`: 2.1.1c/d (W2), 1.1.3c (W3), 2.1.3a (W4, `types.go`), 1.5.3c (W10), 1.5.8g (W14), 4.1.3a (W17).
- `server/services/backlog_service_triage.go`: 1.2.2b (W6), 1.5.4a (W11), 1.5.6 (W12, sole owner in its wave), 1.5.8b (W14). `server/services/backlog_service_trigger_triage.go`: 1.5.4a (W11), 1.5.6e (W12). `server/services/backlog_service_lifecycle.go`: 1.5.4a (W11), 1.5.5 (W12), 4.1.3a (W17, flag read only), different waves.
- `server/services/autonomous_orchestration_service.go`: 1.5.4a (W11), 1.5.6a (W12), 4.1.2b (W16).
- `session/backlog_lifecycle_pr.go`: 3.1.4 (W9), 1.5.4a (W11), 1.5.7b (W13), 1.5.8d (W14).
- `tools/lint/silenttransition/*`: 1.5.4f (W11), 1.5.9a (W15).
- `Makefile`: 0.1.1d (W0), 1.1.3b/1.1.3f (W3), 1.5.2d (W9), 1.5.9d (W15). `server/server.go`: 2.1.1e (W2) only. `tests/e2e/a11y-route-sweep.spec.ts`: 4.1.4f (W0), 6.1.1c (W19). `web-app/src/app/settings/page.tsx` and `web-app/src/lib/routes.ts`: 2.2.1a (W5) only. `web-app/src/components/settings/AlertConfirmDialog*`: created by 4.1.4e (W0), consumed (not edited) by 1.3.3a (W16) and 2.2.1c (W5).
- Story 5.1.1 (W1) precedes every task that registers a `ConfigSurfaceProof` entry (2.1.1b in W2, 1.5.3c in W10, 1.5.8g in W14, 1.2.4a in W13, 1.5.9c in W15, 5.1.2 in W18, 5.1.3 in W19); the pending list shrinks as they land and is empty at 5.1.3.

---

## Phase 0: Spikes (riskiest unknowns first)

### Epic 0.1: Resolve unknowns that gate engine design
**Goal**: Replace UNVERIFIED claims with characterization tests and measured numbers before writing the selector. The `override_reason` semantics are no longer a spike question: they are a decision (ADR-001 "override_reason"), derived from code that was read and cited in the repair log.

#### Story 0.1.1: Characterize current behavior
**Prerequisites:** None
**As a** maintainer, **I want** the handler's override and error behavior pinned by tests and the remaining unknowns measured, **so that** the selector reproduces today's semantics in `Off` mode and implements the decided semantics in `Enforce`.
**Acceptance Criteria**:
- Handler semantics of `override_reason` against `ValidateGates` and error codes (Default engine) are pinned by a passing characterization test.
  - *Given* a `BacklogItem` in `review` with no `PASS` verdict, *When* `TransitionBacklogItemStatus` is called to `done` with `override_reason="ship per owner"`, *Then* the transition succeeds and a progress note plus `notifyManualOverride` are recorded; with an empty reason the call returns `FailedPrecondition` (`ErrVerdictRequired`).
  - *Given* any `(from,to)` not in `ValidTransitions()`, *When* called with a non-empty `override_reason`, *Then* the call is still rejected (`CanTransition` at `backlog_service_lifecycle.go:892` runs before and independent of the override).
- Spike notes recorded in the ADRs.
  - *Given* ADR-001 "Spike results", *When* the spike finishes, *Then* it states measured `config.LoadConfig()` microseconds per call, the scope-key validation rule for a non-gate flag such as `backlog:workflow-engine-enforce`, and whether `stage:idea` is accepted.
  - *Given* ADR-004 and ADR-003, *When* spike 0.1.1c finishes, *Then* they name the settings-nav registration file, the file where per-item `cost_budget_threshold_usd` is stored, and how an item's `RepoPath` becomes `RepoRef`.
**Files**: `server/services/backlog_transition_characterization_test.go`, `project_plans/backlog-make-it-true/decisions/ADR-001-*.md`, `ADR-003-*.md`, `ADR-004-*.md`, `scripts/verify-manual-instance.sh`, `Makefile`

##### Task 0.1.1a: Handler characterization test, Default-engine behavior only (S)
- Read `backlog_service_lifecycle.go:870-996`; table test over override / no override / each `Err*` sentinel recording gRPC code, and that a non-empty override never bypasses `CanTransition`. Does not characterize Configured gates (decided, not observed; covered by Story 1.1.2 ACs).
- Files: `server/services/backlog_transition_characterization_test.go`

##### Task 0.1.1b: Spike LoadConfig cost and flag scope validation (XS)
- Benchmark `config.LoadConfig()` (run with `&` per benchmark rule); read `config.FeatureFlagScopes` (`config/config.go:375`) and `feature_flag_service.go:172-299`; record results in ADR-001.
- Files: `ADR-001-*.md` (no code)

##### Task 0.1.1c: Spike settings-placement facts (XS)
- Read how the Settings index links to sub-pages (`web-app/src/app/settings/page.tsx` plain `<Link>` entries, not tiles (`styles.externalTabLink` `:60`, `styles.helpLink` `:89-129`) using `routes.*` from `web-app/src/lib/routes.ts`, plus the second registry `web-app/src/lib/nav-pages.ts` (Feature Flags at `:73`) pinned by `web-app/src/lib/__tests__/nav-pages.test.ts`; VERIFIED `grep -n -i nav web-app/src/app/settings/layout.tsx` returns no nav list), storage of `cost_budget_threshold_usd` (`session/repository.go:365-369`) and how an item's `RepoPath` becomes `RepoRef` (`github/repo_ref.go`); record in ADR-004 and ADR-003. The `config.json` locking question is decided in ADR-004 (package RMW helper), not spiked.
- Files: `ADR-003-*.md`, `ADR-004-*.md` (no code)

##### Task 0.1.1d: `scripts/verify-manual-instance.sh` guarded verification script (S)
- Moved here from Story 6.1.2 (it was Task 6.1.2c in W20, but the Worker brief standard block makes it the only way for any worker to run a build, and the runbook dry-run in Task 1.4.1b needs it in W17). Implements the CLAUDE.md manual-instance recipe as the only sanctioned way for a worker to run a build: slot `N` maps to ports `62871+2(N-1)` and `62872+2(N-1)`, binary dir `manual-builds/manual-N`, `STAPLER_SQUAD_INSTANCE=claude-manual-test-N`, `STAPLER_SQUAD_TMUX_SOCKET=ssq-manual-N`; `--check` dry run; `shellcheck` clean (`make ready` runs shellcheck over scripts); referenced from the Worker brief standard block.
- Files: `scripts/verify-manual-instance.sh`, `Makefile` (target `verify-manual-instance`)

---

## Phase 1: Workstream A — make stage configuration true (shadow, then enforce)

### Epic 1.1: Engine seam with gate parity
**Goal**: A `SwitchableWorkflowEngine` that runs Default, shadow-compares the *exact enforce-composed answer* against Default, or enforces it, wired into every engine holder, with the handler untouched in this epic (Story 1.5.5a later routes it through the gate; Story 1.5.2 adds the caller-class dimension to what is built here).

**Core design (decides blockers B1 and B2)**: the enforce answer is implemented exactly once, as a private `enforceAnswer(...)` (file `session/workflow_engine_enforce.go`):
- Graph = Configured graph UNION `SystemRequiredEdges()`; if Configured is nil or its cache is empty/unloaded, graph = **Default graph UNION `SystemRequiredEdges()`** (counted `fallback_total{reason}`). The union applies on the fallback path too: plain Default lacks `in_progress|review|pr_pending->archived` and `archived->archived`, so a fallback to plain Default would make Enforce deny archive on exactly the path meant to fail open (ADV-N1). `SystemRequiredEdges()` does not exist in the tree yet (zero `git grep` hits); Task 1.1.2b creates it before any selector code uses it.
- Guards = Default `TransitionGuard` (input forwarded UNCHANGED; its own narrower override semantics preserved) AND Configured operator gates, except that a non-empty `OverrideReason` additionally skips the Configured operator gates (it never skips the graph check and never a Default guard that ignores override: AC, plan, plan-artifacts and blocker guards still deny).
- `Enforce` returns `enforceAnswer`. `Shadow` returns the Default answer, computes `enforceAnswer` on the identical input, and records Default-vs-`enforceAnswer` divergence. The raw `ConfiguredWorkflowEngine` is compared to Default only in the offline harness (Story 1.1.1), as information.

#### Story 1.1.1: Gate-equivalence harness (written first)
**Prerequisites:** 0.1.1
**As a** maintainer, **I want** a parity test over edges and gates, **so that** a swap can never silently drop a guard and the raw-Configured gap is documented, not alarmed on.
**Acceptance Criteria**:
- Edge parity of the raw Configured engine on the seeded config.
  - *Given* a `ConfiguredWorkflowEngine` built over the seeded 9 stages, *When* `CanTransition` and `AllowedTransitions` are called for all 81 `(from,to)` pairs, *Then* results equal `DefaultWorkflowEngine` for every pair.
- Raw-Configured gate gap is measured as information, never as a failure.
  - *Given* an item fixture `idea` with zero acceptance criteria, *When* both raw engines evaluate `idea->ready`, *Then* Default returns `ErrACRequired`, raw Configured returns no pending gate, and the harness prints that fixture under `gate_builtin_absent_in_config` and passes.
- The harness takes the engine under test as a parameter so Story 1.1.2 reuses it for `enforceAnswer` without copying it.
  - *Given* the harness, *When* it is called with `DefaultWorkflowEngine` as both sides, *Then* it passes (sanity), and the composed-engine assertion is added by Task 1.1.2d (no skipped placeholder test is committed here).
- Seed fingerprint is stable and covers more than edges.
  - *Given* `domain.ValidTransitions()` and the seed's stage set, liveness settings and gate rows, *When* `SeedFingerprint()` is computed twice, *Then* both hashes are equal; removing one edge, disabling one stage, changing one liveness value, or adding one gate each yields a different hash.
**Files**: `session/workflow_engine_equivalence_test.go`, `session/workflow_stage_seed.go`, `session/workflow_stage_seed_test.go`

##### Task 1.1.1a: Equivalence harness over 81 pairs and gate fixture corpus (M)
- Corpus: one fixture per `Err*` sentinel in `domain/backlog.go:599-606` (AC, plan, plan-artifacts, blockers, verdict-clear, verdict-required, code-not-on-main); compare `CanTransition`, sorted `AllowedTransitions`, unsatisfied-gate set by `GateID/Kind`, and `errors.Is` identity (never `Description`). Engine-under-test is a parameter.
- Files: `session/workflow_engine_equivalence_test.go`

##### Task 1.1.1b: `SeedFingerprint()` over edges, stages, gates, liveness (S)
- Use the same sources as the seed; the fingerprint is what labels `config=seeded|edited` everywhere, so an operator who adds a gate or changes liveness without touching an edge is correctly `edited`.
- Files: `session/workflow_stage_seed.go`, `session/workflow_stage_seed_test.go`

#### Story 1.1.2: SwitchableWorkflowEngine and the enforce composition
**Prerequisites:** 1.1.1 (and 0.1.1)
**As an** operator, **I want** the engine selected by live flags with safe fallback and an override that still works, **so that** I can verify before enforcing and revert instantly.
**Acceptance Criteria**:
- Off and shadow return the Default answer.
  - *Given* mode `Shadow` and an edited config that disables `done->refining`, *When* `CanTransition(done, refining)` is called, *Then* it returns `true` (Default) and `workflow_engine_shadow_divergence_total{method="CanTransition",class="graph",config="edited",path="transition"}` increments by 1.
- Enforce keeps built-in guards (guard parity through the composition).
  - *Given* mode `Enforce`, seeded config and an `idea` item with 0 acceptance criteria, *When* `PendingGates`/`ValidateGates` run for `idea->ready`, *Then* `ErrACRequired` is returned (same sentinel as Default).
- Shadow on a seeded config never diverges on an ordinary guard denial (B1).
  - *Given* mode `Shadow`, seeded config and an `idea` item with 0 acceptance criteria, *When* `ValidateGates(idea->ready)` is shadowed, *Then* Default and `enforceAnswer` both return `ErrACRequired`, no divergence series increments, and `shadow_calls_total` increments by 1.
  - *Given* mode `Shadow` and an operator-added `human_approval` gate on `ready->in_progress`, *When* shadowed, *Then* the call is counted `gate_extra` with `config="edited"`.
- Shadow does not do hidden I/O.
  - *Given* mode `Shadow` and an edge carrying a `human_approval`, `automated_review` or `custom` gate, *When* shadowed, *Then* the stateful gate is NOT evaluated, `workflow_engine_shadow_not_compared_total{reason="stateful_gate"}` increments, and no `GateSatisfactionRepository` call is made (asserted with a fake repo that fails the test if called).
- Enforce honors operator graph edits without restart.
  - *Given* mode `Enforce`, *When* an operator disables the non-system-required edge `done->refining` via the CRUD RPC and the cache invalidates, *Then* `CanTransition(done, refining)` returns `false` on the next call and `CanTransition(ready, in_progress)` is still `true`.
- Fallback instead of deny-all.
  - *Given* mode `Enforce` and a `ConfiguredWorkflowEngine` whose `Load` failed (empty cache), *When* `CanTransition(idea, ready)` is called, *Then* it returns `true` (Default graph), `workflow_engine_fallback_total{reason="cache_empty"}` increments, and one WARN is logged per minute.
  - *Given* mode `Enforce` and an empty cache (and, separately, a nil `Configured` and a `load_error`), *When* `CanTransition(in_progress, archived)`, `CanTransition(review, archived)`, `CanTransition(pr_pending, archived)` and `CanTransition(archived, archived)` are called, *Then* all four return `true` (fallback graph = Default UNION `SystemRequiredEdges()`), each with the matching `fallback_total{reason}` increment; `AllowedTransitions` on the fallback path still does not list `archived` for an `in_progress` item (Story 1.5.1 AC 3: the archive group never reaches the UI-facing list).
- Shadow can never fail or slow a transition (pre-mortem #5).
  - *Given* mode `Shadow` and a `ShadowRecorder` whose method panics (and, separately, a nil snapshot), *When* `CanTransition` is called, *Then* the Default answer is returned unchanged, `workflow_engine_shadow_errors_total` is 1, and the panic is logged once with its stack.
  - *Given* mode `Shadow` and 1,000 `path=read` calls, *When* they are counted, *Then* only about 1 in 10 are shadow-evaluated (sampling is deterministic from an injected counter, not random), while every `path=transition` call is evaluated.
- Typed-nil safety and system-required union.
  - *Given* a selector built with a nil `*ConfiguredWorkflowEngine`, *When* any method is called in `Enforce`, *Then* Default answers with `reason="cfg_nil"` and no panic; *Given* a legacy DB where `in_progress->review` is disabled, *When* enforce evaluates it, *Then* it is allowed because it is a `SystemRequiredEdge`.
- Denials are attributed.
  - *Given* `Enforce` and a denied `review->done` with no `PASS` verdict and no override, *When* the transition is checked, *Then* a log line has `from=review to=done reason=default_guard` and `workflow_engine_enforce_denied_total{source="default_guard"}` is 1.
- `override_reason` under Enforce (B2; decision: Default guards keep their own override semantics unchanged; a non-empty reason additionally bypasses Configured operator gates including the synthetic deleted-stage gate, never the graph, and never `ErrACRequired`, `ErrPlanRequired`, `ErrPlanArtifactsRequired`, `ErrPlanArtifactsMissing` or `ErrUnresolvedBlockers`).
  - *Given* mode `Enforce`, an operator-added `human_approval` gate on `review->done` that is unsatisfied, and `override_reason="ship per owner"`, *When* `ValidateGates` runs, *Then* it returns nil and the handler's existing progress note and `notifyManualOverride` fire; with an empty reason it returns `ErrGateNotSatisfied`.
  - *Given* mode `Enforce` and an item whose stage was deleted but whose `StageConfigSnapshot` still lists the target, *When* `PendingGates` would return the synthetic `stage-config-unresolvable` gate, *Then* with a non-empty override `ValidateGates` returns nil, and with an empty reason it blocks (so the gate's own ActionHint "force this transition via Manual Override", `configured_workflow_engine.go` `stageConfigUnresolvableGateStatus`, is true).
  - *Given* mode `Enforce`, a non-empty override and an item that Default denies with `ErrACRequired` (`idea->ready`, no AC), `ErrUnresolvedBlockers` or `ErrPlanRequired` (`ready->in_progress`), *When* `ValidateGates` runs, *Then* it still returns that sentinel (override never bypasses these Default guards; pinned today by `backlog_transition_characterization_test.go`); with a satisfiable Default guard path, the `review->done` no-PASS case still succeeds under override exactly as under Default.
  - *Given* mode `Enforce`, a non-empty override and a `(from,to)` absent from both the live graph and the snapshot, *When* `CanTransition` runs, *Then* it returns `false` (override never bypasses the graph).
**Files**: `session/workflow_engine_selector.go`, `session/workflow_engine_enforce.go`, `session/workflow_engine_selector_test.go`, `session/workflow_system_edges.go`, `session/configured_workflow_engine.go`

##### Task 1.1.2a: Selector type, modes, fallback, denial attribution, `HasAnyStage()` (M)
- Implement `SwitchableWorkflowEngine` per ADR-001 as a thin dispatcher over `Default`, `enforceAnswer` and the `ShadowRecorder` interface (injected, nil-safe); add `HasAnyStage()` on `ConfiguredWorkflowEngine`; typed-nil guard; fallback WARN rate limit; denial log and `enforce_denied_total`; `defer recover()` around the entire Shadow branch (log with stack, `shadow_errors_total`, return the Default answer); atomic or sharded counters with no mutex on the call path; `path=read` evaluation sampled 1 in 10 from an injected counter; mode reader coalesced with singleflight.
- Files: `session/workflow_engine_selector.go`, `session/configured_workflow_engine.go`

##### Task 1.1.2b: `SystemRequiredEdges()` set and union (S)
- Derive the list per ADR-002 §3 (both groups: engine-checked automation callers and storage-direct writers), CREATE the set (it is not in the tree) and apply the union in `CanTransition`/`AllowedTransitions` on BOTH the configured path and the fallback path (fallback graph = Default UNION the set); the archive group is excluded from `AllowedTransitions` (Story 1.5.1 AC 3); test that the set is a subset of `ValidTransitions()` and contains `ready->queued`, `queued->in_progress`, `ready->in_progress`, and that the fallback path allows the archive edges once Task 1.5.1c adds them.
- Files: `session/workflow_system_edges.go`, `session/workflow_engine_selector.go`

##### Task 1.1.2c: `enforceAnswer` composition with override semantics and structural-only shadow evaluation (M)
- One private implementation used by both `Enforce` and `Shadow`. Add `ConfiguredWorkflowEngine.PendingGatesStructuralOnly(...)` returning `(statuses, skippedStateful bool)` for shadow so no DB read occurs. Default guards get the input unchanged; override additionally skips Configured operator gates only, never the graph.
- Files: `session/workflow_engine_enforce.go`, `session/configured_workflow_engine.go`

##### Task 1.1.2d: Selector table tests, modes x cases, and composed-engine parity (S)
- Table: {Off, Shadow, Enforce} x {seeded, edited, cache-empty, cfg-nil, snapshot-fallback item, override, stateful-gate edge}; run the Story 1.1.1 harness with `enforceAnswer` as engine-under-test on the seeded config, all 81 pairs plus the gate corpus, asserting equality with Default including `errors.Is` identity. Add `BenchmarkSwitchableWorkflowEngine_*` Off vs Shadow vs Enforce (run with `&`) with an agreed ceiling recorded in the task (INFERRED: Shadow adds under 20 microseconds per call, no allocation growth on `path=read` when sampled out).
- Files: `session/workflow_engine_selector_test.go`

##### Task 1.1.2e: Configured-engine hygiene: Info log to Debug, bound stateful-gate DB reads (S)
- `configured_workflow_engine.go:188` logs at Info per `PendingGates` call (flood risk). `evaluateHumanApprovalGate`/`evaluateRecordedGate` read `gateSatisfactionRepo.GetByItemAndGate(context.Background(), ...)` (`configured_workflow_engine.go:284,374`): wrap in a 2s `context.WithTimeout`; a timeout or error leaves the gate unsatisfied (existing fail-closed behavior) and increments a counter.
- Files: `session/configured_workflow_engine.go`, `session/configured_workflow_engine_test.go`

#### Story 1.1.3: Shadow recorder, flags, actuator, soak target
**Prerequisites:** 1.1.2
**As an** operator, **I want** divergence counts and a READY/NOT READY verdict that measures what enforce will actually do, **so that** I flip enforcement on evidence.
**Acceptance Criteria**:
- Divergence is Default vs `enforceAnswer`, classified and labeled.
  - *Given* the seeded config and mode `Shadow`, *When* 500 transitions are shadowed including 40 legitimate guard denials (no AC, no PASS verdict), *Then* `/actuator/workflow-engine` shows `shadow_calls_total{method="CanTransition",path="transition"}=500` and every `divergence_total` series is 0.
  - *Given* an operator-added extra gate on `ready->in_progress`, *When* shadowed, *Then* it is counted as `gate_extra` with `config="edited"` and does not set the alert condition.
  - *Given* an injected composition defect (a test double whose `enforceAnswer` drops one Default guard), *When* shadowed under the seeded config, *Then* `gate_missing` increments and the soak command exits 1 (`gate_missing` now means "composition defect", the only way it can occur).
- Transitions and reads are counted separately.
  - *Given* the UI calls `AllowedTransitions`/`GetPendingGates` while rendering, *When* shadowed, *Then* they increment `path="read"` series only, and the soak criterion uses `path="transition"`.
- Persistence across restart.
  - *Given* a ledger written at minute N, *When* the process restarts, *Then* counters resume from the ledger and the new `binary_started_at` is shown.
- Soak status exit codes and criterion.
  - *Given* any of `graph`, `gate_missing`, `gate_extra`, `error_kind` greater than 0 under `config="seeded"` (either path), *When* `make workflow-engine-soak-status` runs, *Then* it exits 1 and prints the redacted repro path; when the actuator is down it exits 2. READY (exit 0) requires ALL of: (a) zero `graph|gate_missing|gate_extra|error_kind` under `config=seeded`; (b) when live config is `edited`, every observed divergence maps to an edge in the `--config-diff` output that the owner has acknowledged (recorded in the runbook), otherwise NOT READY; (c) total `path=transition` samples at or above the floor (proposed 100); (d) ledger history plus `binary_started_at` spanning at least W; (e) every edge on the named expected-edge list (the engine-routed callers plus the system-required Group 1 edges) has at least N `path=transition` samples or an owner acknowledgement. Edges outside the list that were never observed are listed "covered offline by harness" (non-blocking).
  - *Given* a fresh ledger with zero shadowed calls (or shadow silently off), *When* the command runs, *Then* it exits 1 with "no evidence", never 0 (vacuous-truth test, pre-mortem #1).
  - *Given* total samples below the floor, or a history span shorter than W, or one expected edge with fewer than N samples and no acknowledgement, *When* the command runs, *Then* each case exits 1 and names the failing condition.
  - *Given* an empty config with no flag set, *When* the production mode resolver is asked, *Then* it returns `Shadow` (the registered default true), not `Off` (a test of the production reader, not only the registry; plus a test that no non-test code reads these flag names with plain `GetFeatureFlag`).
  - *Given* the live config was edited at any point inside the soak window, *When* the command labels the window, *Then* a window spanning an edit is labeled `edited` (never `seeded`), so rule (b) applies to the whole window.
  - *Given* an edge whose gate is stateful (`human_approval`, `automated_review`, `custom`), *When* `make workflow-engine-soak-status` runs, *Then* it lists that edge as `UNVERIFIED in shadow (not_compared{stateful_gate})` with its skip count, and the edge is not counted toward READY evidence; the runbook (Story 1.4.1) and exit criterion require the owner to acknowledge each such edge before flipping.
  - *Given* a divergence on `path=read` only, *When* alarms are evaluated, *Then* it still fires the alarm (exit 1 under `config=seeded`), but it does not count toward READY: only `path=transition` samples satisfy criterion (c).
- One-shot live-config check.
  - *Given* a freshly deployed binary, *When* the operator runs `make workflow-engine-soak-status ARGS=--config-diff` in the first hour, *Then* it reports `config=seeded|edited` from `SeedFingerprint` and, if edited, the diff of stages, edges, gates and liveness versus the seed.
  - *Given* live items where `enforceAnswer.AllowedTransitions(item.status)` would be empty or would lose an edge compared with Default (an operator disabled an outgoing edge or a stage), *When* `--config-diff` runs, *Then* a "stranded item preview" lists each such item (id hash, status, lost edges, whether `override_reason` would be the only way out) and the JSON `stranded_items` carries the same; the runbook blocks the flip until the owner acknowledges every previewed item (pre-mortem #2).
- Review-date forcing function (requirements Metric 6, Risk Control).
  - *Given* the earliest `binary_started_at` in the ledger is T, *When* `make workflow-engine-soak-status` runs, *Then* it prints `REVIEW DUE <T+14d>`; *Given* that date has passed and no class is enforced, *Then* it also prints `REVIEW OVERDUE: no class is enforced; run the Outcome review`, and the exit code is unchanged (advisory, never turns a READY into a failure).
- Access (see Authn/authz).
  - *Given* the remote chain with an auth middleware that rejects every token, *When* `/actuator/workflow-engine` or `/actuator/nudge-outcomes` is requested without credentials, *Then* the response is 401 and the handler is not reached; *Given* the local chain and a foreign `Host`, *Then* `HostGuard` rejects it; neither path is added to the auth exempt prefixes.
- Flags are live.
  - *Given* `backlog:workflow-engine-enforce` toggled on via `UpdateFeatureFlag`, *When* the next transition is checked after at most the mode TTL (about 1s), *Then* mode is `Enforce` with no restart; both flag defaults are declared once in `config` and read by `knownFeatureFlags`.
**Files**: `session/workflow_engine_shadow.go`, `session/workflow_engine_shadow_test.go`, `session/workflow_engine_soak.go`, `session/workflow_engine_soak_test.go`, `server/actuator.go`, `server/actuator_test.go`, `server/services/feature_flag_service.go`, `config/config.go`, `Makefile`

##### Task 1.1.3a: ShadowRecorder, classes, ledger, repro records (M)
- Counters in-memory plus once-a-minute ledger (follow the ledger-write helper in `session/gitwiring/` rather than `soak.go:207`, which is `SoakReport`, not the writer); `config=seeded|edited` via `SeedFingerprint`; `path=transition|read`; `not_compared{reason}`; repro records redacted (item id hash, from, to, both answers) kept in a ring of the last 200 in total, aggregates over closed-enum labels only, no session id as a label; ledger path via `config.GetConfigDir()` (tests use `t.TempDir()`); corrupt or oversized ledger starts fresh with a WARN; records every binary start time; read-only (no side effects).
- Files: `session/workflow_engine_shadow.go`, `session/workflow_engine_shadow_test.go`

##### Task 1.1.3b: Actuator endpoint and `make workflow-engine-soak-status` (S)
- Register `/actuator/workflow-engine[?format=text]` beside `/actuator/git-backend` (`server/actuator.go:31`); script modeled on `make git-soak-status`; implements the exit-code criterion above including the vacuity guards (zero calls exits 1, floor, span W, expected-edge list with N or acknowledgement); alarms fire on divergence on either path; READY counts only `path=transition`; alert classes `graph|gate_missing|gate_extra|error_kind` under `config=seeded`; stateful-gate edges listed as `UNVERIFIED in shadow`; a window spanning a config edit is labeled `edited`. Story 1.5.2d extends it per caller class.
- The evaluation (`EvaluateSoak(class, ledger, params) Verdict`, including `READY`/`NOT_READY`/`NEEDS_OWNER_ACK`) lives in `session/workflow_engine_soak.go`, not in `server/actuator.go`, so the actuator, the make target and Story 1.3.1's readiness evidence call one function (no `server/services` import of `server`). Also prints `REVIEW DUE`/`REVIEW OVERDUE` (see the AC above).
- Files: `server/actuator.go`, `Makefile`, `server/actuator_test.go`, `session/workflow_engine_soak.go`, `session/workflow_engine_soak_test.go`

##### Task 1.1.3c: Mode flags, TTL-cached mode reader, status detail (S)
- Add two `knownFeatureFlags` entries with `defaultValue` (shadow, enforce for class operator; Story 1.5.2b adds the other two); mode resolver reads through `GetFeatureFlagWithDefault` with the registered default (never plain `GetFeatureFlag`, which returns false when unset), caches about 1s (atomic) using the spike's TTL result; test: empty config resolves to `Shadow`; status-detail provider "Shadow: N divergences/24h".
- Files: `server/services/feature_flag_service.go`, `config/config.go`, `session/workflow_engine_selector.go`

##### Task 1.1.3d: Flag-default parity test for the two new flags (XS)
- Assert registry default equals `config` constant (feeds Story 4.1.3's generalized test) AND that the production mode resolver, run against an empty config, returns the registry default (pre-mortem #8); add a grep-style test that no non-test code calls plain `GetFeatureFlag` with these flag names.
- Files: `server/services/feature_flag_service_test.go`

##### Task 1.1.3e: Per-from-stage scope for the enforce flag (S)
- Only if Task 0.1.1b shows the scope vocabulary extension is cheap; otherwise record "global-only" in ADR-001 and the runbook and skip.
- Files: `config/config.go`, `server/services/feature_flag_service.go`, `session/workflow_engine_selector.go`

##### Task 1.1.3f: `--config-diff` live-config-versus-seed report with stranded-item preview (M)
- Reads the stage config through the existing repository, compares with `SeedFingerprint` components, prints the diff; exposed by the actuator JSON as `config_diff`. The stranded-item computation is a `session` function (`StrandedItemPreview`, in `workflow_stage_seed.go`) that the actuator only formats and Story 1.3.1 reuses for its count. Also computes, over live items, the set whose `enforceAnswer.AllowedTransitions(status)` is empty or smaller than Default's and prints it as the stranded-item preview (`stranded_items`), including items hosted in a disabled stage. Read-only.
- Files: `server/actuator.go`, `Makefile`, `session/workflow_stage_seed.go`

#### Story 1.1.4: Wiring and wiring test
**Prerequisites:** 1.1.3
**As a** maintainer, **I want** every engine holder pinned by a test, **so that** the `dependencies.go:678` vs `:773` class of bug cannot recur.
**Acceptance Criteria**:
- Selector is what BacklogService holds.
  - *Given* a narrow `ServerDependencies` built with ent storage, *When* the wiring test inspects `BacklogService`, *Then* its engine is `*SwitchableWorkflowEngine` and `SyncLoop` holds the same pointer.
- The listener keeps the concrete engine.
  - *Given* the same dependencies, *When* the test inspects `backlogLifecycleListener`, *Then* it holds `stageConfigEngine` (`*ConfiguredWorkflowEngine`), and a comment at `session/backlog_lifecycle.go:1092,1116` (the `.(*ConfiguredWorkflowEngine)` assertions) says that injecting the selector there silently degrades gate-context resolution; a unit test sets a selector on the listener and asserts the degrade is logged, so "inject the selector everywhere" fails loudly.
- Nil invalidator is visible.
  - *Given* `stageConfigEngine` construction returns an error, *When* the server starts, *Then* a WARN with `invalidator_wired=false` is logged and `GetWorkflowEngineStatus.invalidator_wired` is `false`.
- Startup line.
  - *Given* default flags, *When* the server starts, *Then* the log contains `workflow engine: mode=shadow configured_loaded=true invalidator_wired=true`.
**Files**: `server/dependencies.go`, `session/backlog_sync.go`, `session/backlog_lifecycle.go`, `server/dependencies_workflow_engine_test.go`

##### Task 1.1.4a: Construct selector and pass at `dependencies.go:1477` (S)
- Build after `stageConfigEngine` (`:773-780`); recorder registered with actuator; startup log; nil-engine WARN. `dependencies.go` touches are serialized across stories (see Critical path).
- Files: `server/dependencies.go`

##### Task 1.1.4b: `SyncLoop.SetWorkflowEngine` and stale-comment fix (S)
- Add setter, wire in `dependencies.go`, correct `session/backlog_lifecycle.go:271-274`; unit test that SyncLoop uses the injected engine at `backlog_sync.go:449`.
- Files: `session/backlog_sync.go`, `session/backlog_sync_test.go`, `session/backlog_lifecycle.go`, `server/dependencies.go`

##### Task 1.1.4c: Wiring test and listener-misuse guard (S)
- Narrow `ServerDependencies` (no `BuildDependencies()`); asserts selector, SyncLoop pointer equality, listener holds `stageConfigEngine`; adds the comment and degrade-logging test above.
- Files: `server/dependencies_workflow_engine_test.go`, `session/backlog_lifecycle.go`

### Epic 1.2: Do not strand items; inventory every status writer
**Goal**: Make a stricter graph safe for the automation that assumes edges are legal, and build the repo-wide inventory that Epic 1.5 drives to zero.

#### Story 1.2.1: System-required edges cannot be disabled
**Prerequisites:** 1.1.4
**As an** operator, **I want** the editor to refuse changes that would break automation, **so that** my edits cannot strand live sessions.
**Acceptance Criteria**:
- *Given* the seeded config, *When* `UpdateStageTransition(from=in_progress, to=review, enabled=false)` is called, *Then* it returns `FailedPrecondition` naming the edge as system-required and the stored edge stays enabled.
- *Given* the seeded config, *When* `UpdateStageTransition(from=ready, to=queued, enabled=false)` (dequeue path) or `(from=queued, to=in_progress)` is called, *Then* each is rejected the same way.
- *Given* `DeleteStage(review)` with live items, *When* called, *Then* it is rejected as today (unchanged behavior) and the message does not suggest `force` for a stage hosting system-required edges.
- *Given* live items in status `S` and an edit (`UpdateStageTransition enabled=false`, `UpdateStage enabled=false` or `DeleteStage force`) that would leave `S` with zero allowed outgoing transitions under the enforced graph, *When* called, *Then* it returns `FailedPrecondition` naming `S` and up to 5 affected item ids (`override_reason` would be the only exit); an edit that merely removes one of several outgoing edges is allowed and shown in the stranded-item preview (pre-mortem #2).
**Files**: `server/services/backlog_service_transitions.go`, `session/graph_validator.go`, `session/graph_validator_test.go`, `server/services/graph_validator_stranded_test.go`

##### Task 1.2.1a: Fix the edge list against call sites (S)
- Derive from both groups and record which group each edge came from: (1) engine-checked automation callers: `transitionWithGuard` (`backlog_service_triage.go:680`, callers `queueBacklogItem` `:699`/`:716` for `ready->queued`, `claimAndSpawnCandidate` `:837`/`:839` for `queued->in_progress` and `ready->in_progress`), `SyncLoop` backward targets (`session/backlog_sync.go:543` `determineBackwardSyncTarget`: `idea|refining|ready|queued -> archived`), `AttachSessionToItem` (`backlog_service_sync.go:157`, `->in_progress`); (2) storage-direct writers (the site list in Story 1.2.3's scope paragraph; the generated inventory in Task 1.2.3b later cross-checks it), kept system-required for UI honesty, plus the archive edges `X->archived` and `archived->archived` that `validTransitions` lacks (Task 1.5.1c). The generic MCP delegate contributes no edge (ADR-007 D4), which is what keeps `done->refining` a non-system edge the Story 1.2.4 and 1.5.9 allow/deny proofs can disable; confirm that here.
- Files: `session/workflow_system_edges.go`

##### Task 1.2.1b: Validator rejection plus tests (S)
- Files: `session/graph_validator.go`, `server/services/backlog_service_transitions.go`, `session/graph_validator_test.go`

##### Task 1.2.1c: Reject edits that strand live items (S)
- Validator takes a live-item status histogram (count of items per status, one query) and rejects an edge/stage disable or delete that leaves a populated status with no allowed outgoing edge; shares the "allowed outgoing edges under the enforced graph" function with the `--config-diff` preview (Task 1.1.3f).
- Files: `session/graph_validator.go`, `server/services/backlog_service_transitions.go`, `server/services/graph_validator_stranded_test.go`

#### Story 1.2.2: A denied automatic claim produces a visible stuck reason
**Prerequisites:** 1.2.1
**As an** operator, **I want** a denied automatic transition to produce a visible stuck reason, **so that** items do not silently sit queued while the dequeue sweep logs a Warn forever.
**Scope (B3, re-derived from current code)**: the engine-deniable AUTOMATED callers are `transitionWithGuard` (`server/services/backlog_service_triage.go:680`, engine check `:683`, gates `:689`) and its callers `queueBacklogItem` (`:699`, call `:716`), `claimAndSpawnCandidate` (`:837`, call `:839`) and `JulesDispatchService` (`jules_dispatch_service.go:361`), plus `SyncLoop` (`session/backlog_sync.go:449`). Of these only `claimAndSpawnCandidate` swallows a denial: any denial that is not one of its four named sentinels (an operator-gate `ErrGateNotSatisfied`, or the untyped `invalid transition` error at `:684`) falls to its `default:` arm (`backlog_service_triage.go:872-873`), which does `log.Warn("claim failed")` on every sweep. `queueBacklogItem` and `JulesDispatchService` return the error to a user-initiated caller; `SyncLoop` skips and retries (its denials are counted by the selector). The earlier cites `backlog_service_triage.go:801-810`, `:2313-2321` and `session/backlog_lifecycle.go:971-984` were not transition sites; the storage-direct lifecycle sites (`backlog_lifecycle.go:1004,1866,1873`) are pure CAS writes that the graph can never deny, so they are out of this story. **Placement**: the helper stays in package `server/services` and is scoped to service-layer callers (no import cycle: `session` code does not call it).
**Acceptance Criteria**:
- *Given* `Enforce` and an operator-added `human_approval` gate on `queued->in_progress` that is unsatisfied for item `BI-42` (the practical trigger: `queued->in_progress` itself is system-required, so a pure graph denial is reachable only on a stub engine), *When* `DequeueNextQueuedItems` sweeps and `claimAndSpawnCandidate` receives `ErrGateNotSatisfied`, *Then* `BI-42` gets stuck reason `transition_denied`, a progress comment states `from=queued to=in_progress reason=operator_gate`, and exactly one notification is sent. The same assertions hold with a stub engine returning `ErrTransitionNotAllowed` and `reason=config_graph`.
- *Given* the same item and a second sweep while the gate is still unsatisfied, *When* `claimAndSpawnCandidate` is called again, *Then* no second comment or notification is created (dedupe on the existing stuck reason) and the `default:` Warn is not logged again.
- *Given* the gate is satisfied (or the edge allowed) again, *When* the next sweep runs, *Then* the claim succeeds and the `transition_denied` stuck reason is cleared (no permanent park).
- *Given* a gate denial (`ErrPlanRequired`, `ErrUnresolvedBlockers`, `ErrPlanArtifactsMissing`), *When* the claim runs, *Then* the existing `case` arms behave exactly as before (regression-pinned).
**Files**: `server/services/backlog_transition_notify.go`, `server/services/backlog_service_triage.go`, `session/storage_backlog.go`, `proto/session/v1/backlog.proto`, `server/services/backlog_service_stuck.go`

##### Task 1.2.2a: Helper, typed denial error, stuck reason (M)
- New file with `transitionOrNotify` (comment + stuck reason + `notify()`, idempotent per `(item, caller, from, to, reason)`); introduce `session.ErrTransitionNotAllowed` and wrap it at `transitionWithGuard`'s `invalid transition` return (`backlog_service_triage.go:684`, one-line change) so callers can `errors.Is` instead of matching text; register `TransitionDeniedStuckReason` through proto wiring (a new stuck reason also needs proto per memory) with the remediation-due rule; the stuck row carries structured `caller`, `from`, `to` and `denied_reason` fields (added to the proto here, in W6, so Story 1.5.8 needs no proto change); a claim denial records caller `dequeue_claim`. The helper keeps a signature that Task 1.5.3b can adapt to the `TransitionDeniedReporter` interface declared in `session`, so `session` callers reach it by injection.
- Files: `server/services/backlog_transition_notify.go`, `session/storage_backlog.go`, `proto/session/v1/backlog.proto`, `server/services/backlog_service_stuck.go`, `session/domain/backlog.go` (sentinel)

##### Task 1.2.2b: Extract claim-error classification, then adopt (M)
- Complexity guard: `claimAndSpawnCandidate` (`:837`) already has a 5-arm switch and the gocognit/gocyclo gate applies to modified existing functions. First commit: Extract Function `handleClaimError(ctx, item, fromStatus, claimErr)` with no behavior change; second commit: add one arm there covering `ErrTransitionNotAllowed` and `ErrGateNotSatisfied`. Run `make ready-complexity-gate` on both commits; do not grow `claimAndSpawnCandidate` or `backlog_service_lifecycle.go`.
- Files: `server/services/backlog_service_triage.go`

##### Task 1.2.2c: Dedupe, recovery and regression tests (S)
- Files: `server/services/backlog_transition_denied_recovery_test.go`

#### Story 1.2.3: Status-writer inventory and counters (guard written first)
**Prerequisites:** 1.2.2
**As a** maintainer, **I want** every status-write call site inventoried by a failing-then-green repo-wide test with a ratcheted pending list, **so that** Epic 1.5 can migrate them wave by wave with `make test` green throughout and a new raw call can never slip in.
**Scope (recounted 2026-10-17 by `git grep`; owner decision reverses "measure, don't govern", ADR-007)**: `git grep -n "\.TransitionBacklogItemStatus(" -- '*.go' ':!*_test.go' ':!tools/lint'` returns 38 call sites in 14 files: 1 primitive wrapper (`session/storage.go:1104`), 1 delegate to the handler (`server/mcp/tools_backlog_dependencies.go:163`), 5 engine-routed (handler `backlog_service_lifecycle.go:959`, `transitionWithGuard` `backlog_service_triage.go:692`, `OverrideVerdict` `backlog_service_lifecycle.go:1332`, `AttachSessionToItem` `backlog_service_sync.go:158`, `SyncLoop` `backlog_sync.go:450`) and 31 direct-storage sites in 10 files (`tools_backlog.go` 4, `autonomous_orchestration_service.go` 1, `backlog_debug_mutate_handler.go` 1, `backlog_service_lifecycle.go` 1 `SubmitManualReview` `:1446`, `backlog_service_triage.go` 10 `:881,1268,1324,2041,2115,2418,2471,2637,2906,3348`, `backlog_service_trigger_triage.go` 2, `session/backlog_lifecycle.go` 3 `:1004,1866,1873`, `backlog_lifecycle_archive.go` 1, `backlog_lifecycle_pr.go` 5, `jules_session_poller.go` 3). Five more status-write primitives were missing from the earlier inventory (the fifth, `TransitionBacklogItemStatusWithPRFields`, `ent_repository_backlog.go:1951`, is only called inside `Storage.SetBacklogItemPRAndTransition` at `storage.go:1218`, so it adds a primitive to the registry and guard but no call site): `ArchiveBacklogItem` (3 callers: `tools_backlog.go:2525`, `backlog_debug_mutate_handler.go:253`, `backlog_service_lifecycle.go:516`), `UnarchiveBacklogItem` (`backlog_service_lifecycle.go:547`), `SetBacklogItemPRAndTransition` (4: `tools_backlog_pr.go:375`, `backlog_service_lifecycle.go:470`, `backlog_lifecycle_pr.go:308`, `jules_session_poller.go:540`) and `ReconcileStuckItems` (`backlog_lifecycle.go:1256`, which updates status inside a transaction at `storage_backlog.go:1006`). That is 40 sites to migrate (31 + 9) plus 5 engine-routed to re-point = 45 call sites into the gate. Exempt by design and routed through named entry points: three rollback writes (`backlog_service_triage.go:881,2115,2471`, restoring `ExpectedStatus`) and the debug handler's two sites (`:172,:253`): 3 + 2 = 5 exempt. Creation is a separate exempt `create` class: `CreateBacklogItem` has no `from` status; its non-entry statuses come only from the debug seed/mutate handlers and are constrained in Task 1.5.4g. Re-verified that no other status writer exists: `SetStatus(` on `BacklogItem` builders appears only in the six writers, every other `BacklogItem` update builder sets no status, and there is no raw SQL against `backlog_items`.
**Acceptance Criteria**:
- *Given* a new non-test call, anywhere in the module (`server/`, `session/`, `server/mcp/`, other packages), to any status-write primitive (`TransitionBacklogItemStatus`, `ArchiveBacklogItem`, `UnarchiveBacklogItem`, `SetBacklogItemPRAndTransition`, `ReconcileStuckItems`, `TransitionBacklogItemStatusWithPRFields`), on any receiver, in a function that is neither in the ratcheted `pending` list nor on the allow-list, *When* `go test` runs the inventory test, *Then* it fails naming file and function; the inventory walks the whole module with `go/parser`, not one package.
- *Given* the `pending` list, *When* a site is migrated and its entry is not deleted, *Then* the test fails (stale pending entry); *When* the list is non-empty, *Then* the test still passes, so `make test` is green at every wave. Story 1.5.9a makes the list empty.
- *Given* the inventory, *When* read, *Then* every entry is exactly one of `engine-routed`, `pending: migrate in Story 1.5.x`, `gated(caller)` or `exempt: <reason>` (rollback, debug, wrapper, the gate file) and none lacks a reason.
- *Given* the counter API, *When* `RecordEngineBypass(caller)` is called for an exempt entry point (`rollback`, `debug`; wired in Story 1.5.6), *Then* `workflow_engine_bypass_total{caller}` increments and the actuator shows it. Pending sites are not instrumented (the inventory test, not a counter, tracks them), so no behavior or control flow of the 40 sites changes before Epic 1.5.
**Files**: `session/workflow_engine_bypass.go`, `server/actuator.go`, `server/services/workflow_bypass_inventory_test.go`

##### Task 1.2.3a: `RecordEngineBypass(TransitionCaller)` counter and actuator exposure (S)
- Files: `session/workflow_engine_bypass.go`, `server/actuator.go`

##### Task 1.2.3b: Repo-wide inventory test with ratcheted pending list, written first (M)
- `go/parser` walk of all non-test files in the module (run from the module root, excluding `tools/lint`); matches calls by selector name, on any receiver, for the six primitives (old and, once renamed, `Raw*`/`raw*` forms); allowlist table with classification and reason per `(file, function)`; the pending list holds the 40 + 5 sites with the Story that migrates each; a stale pending entry fails. Generated first so Epic 1.5 migrates against a green-but-shrinking list.
- Files: `server/services/workflow_bypass_inventory_test.go`

#### Story 1.2.4: Integration proofs (Success Metric 1)
**Prerequisites:** 1.5.5, 5.1.1
**As a** maintainer, **I want** end-to-end proofs through the production handler (now routed through the gate), **so that** config edits are demonstrably load-bearing for the operator class; the agent and system classes are proven in Story 1.5.9b.
**Acceptance Criteria**:
- *Given* `Enforce` and a real `BacklogService` with the selector, *When* the test disables the non-system-required edge `done->refining` (confirmed by Task 1.2.1a) via the CRUD RPC then calls `TransitionBacklogItemStatus(done->refining)`, *Then* the call is denied; *When* the edge is re-enabled, *Then* the same call succeeds, with no restart in between (deny to allow and allow to deny).
- *Given* `Enforce` and a system-required edge (`ready->queued`), *When* an operator attempts to disable it, *Then* the CRUD RPC rejects it (Story 1.2.1) and the transition still succeeds.
- *Given* `Enforce` and an `idea` item with no acceptance criteria, *When* `TransitionBacklogItemStatus(idea->ready)` is called, *Then* it returns `FailedPrecondition` with `ErrACRequired` (gate parity through the handler).
- *Given* `Enforce` with a forced cache load failure, *When* `TransitionBacklogItemStatus(idea->refining)` is called, *Then* it succeeds via fallback and `fallback_total` is 1.
- *Given* `Enforce`, a failing operator gate and a non-empty `override_reason`, *When* the handler runs, *Then* it succeeds (B2 through the handler, operator gate only); with an empty reason it returns `FailedPrecondition`; a Default `ErrACRequired` or `ErrUnresolvedBlockers` denial is NOT bypassed by the override.
- *Given* mode `Shadow` with divergent config, *When* any transition is called, *Then* the handler result equals the Off-mode result.
- Metric 1 for the operator class is proven by this story plus Story 1.1.4 and Story 1.5.3c together: this story builds `BacklogService` and `Storage` with the gate in the test; the wiring tests prove production construction wires the same selector and gate. Agent and system classes: Story 1.5.9b. The earlier tests asserting that defaults change nothing and that bypass sites stay on the built-in graph are removed (they assert the weak metric the owner rejected); the Shadow-equals-Off assertion above stays because it describes rollout safety, not scope.
**Files**: `server/services/backlog_engine_enforcement_test.go`

##### Task 1.2.4a: Enforcement integration tests (M)
- Use narrow dependencies; registered as `ConfigSurfaceProof` entries for the stage-config surfaces using the Story 5.1.1 framework (which lands earlier, see waves).
- Files: `server/services/backlog_engine_enforcement_test.go`

### Epic 1.3: Honest status in the product
**Goal**: The operator can always see whether Stages edits are enforced.

#### Story 1.3.1: `GetWorkflowEngineStatus` RPC
**Prerequisites:** 1.5.2 (and 1.2.3)
**As a** UI, **I want** the mode of each caller class, divergence and wiring health, **so that** the banner is truthful.
**Acceptance Criteria**:
- *Given* shadow mode for all classes with 3 `graph` divergences in the last 24h, *When* the RPC is called, *Then* it returns `class_modes={operator:SHADOW, agent:SHADOW, system:SHADOW}`, `divergence_24h=3`, `config_provenance=EDITED`, `invalidator_wired=true`, `gate_wired=true`, `fallback_active=false`.
- *Given* operator enforced and the other classes in shadow, *When* the RPC is called, *Then* `class_modes` reports each class separately (the UI derives Mixed); `gate_wired=false` is reported when `Storage` has no gate injected (Story 1.5.3c), and the UI treats that as Unknown.
- *Given* the recorder is unavailable, *When* the RPC is called, *Then* it returns an error (the UI treats it as Unknown), never a default of ENFORCE.
- The mode TTL (about 1s) means the banner may lag a flag flip by that much; the runbook says so.
- Readiness evidence at decision time (UX-35): *Given* any caller class, *When* the RPC is called, *Then* `class_readiness[class]` carries `divergence_24h` (that class only), `soak_verdict` (`READY`, `NOT_READY`, `NEEDS_OWNER_ACK` for a low-volume class, or `UNAVAILABLE` when the ledger cannot be read; the value comes from `session.EvaluateSoak`, Task 1.1.3b, so it cannot disagree with `make workflow-engine-soak-status`) and `stranded_item_count` (from `StrandedItemPreview`, Task 1.1.3f); *Given* the evaluator errors, *Then* `soak_verdict=UNAVAILABLE` (never READY).
- Review-due fields (UX-55): *Given* a ledger whose earliest `binary_started_at` is T and a fake clock after T+14d with no class enforced, *When* the RPC is called, *Then* `review_due_at = T+14d` and `review_overdue = true`; before T+14d `review_overdue = false`; with any class enforced `review_overdue = false`; *Given* the ledger cannot be read, *Then* `review_state = UNAVAILABLE`, no date is returned and the UI omits the line. The fields come from the evaluator shared with `make workflow-engine-soak-status`, so the two cannot disagree.
- Access: *Given* the remote chain with an auth middleware that rejects every token, *When* `GetWorkflowEngineStatus` is called without credentials, *Then* it is 401 and the handler is not reached; the procedure is not added to any exempt list (read-only, same pattern as the sibling read RPCs; see Authn/authz).
**Files**: `proto/session/v1/backlog.proto`, `server/services/backlog_service_engine_status.go`, `server/services/backlog_service_engine_status_test.go`, `docs/registry/features/`

##### Task 1.3.1a: Proto + handler + registry markers (M)
- `// +api:` marker; `make proto-gen`; `make registry-generate`; commit only feature JSON and sources.
- Files: `proto/session/v1/backlog.proto`, `server/services/backlog_service_engine_status.go`, `docs/registry/features/*.json`

##### Task 1.3.1b: Per-class readiness evidence and the unauthenticated-remote test (S)
- Add `class_readiness` (message `ClassReadiness{divergence_24h, soak_verdict, stranded_item_count}` per class, same proto edit window as 1.3.1a, so no extra proto wave) filled from `session.EvaluateSoak` and `StrandedItemPreview`; table test over READY, NOT_READY, NEEDS_OWNER_ACK and evaluator error (UNAVAILABLE); the 401 test on the remote chain.
- Files: `server/services/backlog_service_engine_status.go`, `server/services/backlog_service_engine_status_test.go`

##### Task 1.3.1c: Review-due fields on the status RPC (S)
- Add `review_due_at`, `review_overdue` and `review_state` to the status response (same proto edit window as 1.3.1a), computed from `session.EvaluateSoak`'s ledger read plus the injected clock; table test over before the date, after the date with no class enforced, after the date with a class enforced, and ledger error.
- Files: `server/services/backlog_service_engine_status.go`, `server/services/backlog_service_engine_status_test.go`

#### Story 1.3.2: Stages page mode banner
**Prerequisites:** 1.3.1, 1.2.1
**As an** operator, **I want** a banner that says Enforced / Partly enforced / Shadow / Off / Unknown, **so that** I never believe unenforced edits are live.
**Acceptance Criteria**:
- *Given* the RPC returns `SHADOW` for every class, *When* `/settings/backlog-stages` renders, *Then* an element `data-testid="engine-mode-banner"` (no `role`; it contains one `role="status"` region `engine-mode-banner-live` that is empty on first render, UX-01) reads "Shadow: edits are compared against the built-in rules but are not enforced. 3 differences in the last 24h." and appears before the stage list in DOM order.
- *Given* `class_modes` with operator `ENFORCE` and agent/system `SHADOW`, *When* the page renders, *Then* the banner reads "Partly enforced. Operators (this UI): enforced. Agents: shadow, not enforced. Automation: shadow, not enforced." (never the plain Enforced wording unless all three classes are enforced).
- *Given* the RPC fails, *When* the page renders, *Then* the banner reads "Couldn't determine whether these settings are enforced. Treat edits as not applied." with a Retry button (`data-testid="engine-mode-banner-retry"`), never the Enforced wording.
- *Given* viewport 360x800 and 1280x800, *When* the banner has the Shadow text, *Then* no horizontal scroll and the phrase "not enforced" is fully visible.
- *Given* a stage save in Shadow mode, *When* it succeeds, *Then* `data-testid="stage-save-status"` (`role="status"`) reads "Saved. Not enforced yet (shadow)"; in Enforce "Saved and enforced"; in Off "Saved. Not in use (off)"; with `invalidator_wired=false` it adds "Saved, but the running engine was not refreshed."
- *Given* a stage-form save that the server rejects because the edge is system-required (UX-07), *When* the response returns, *Then* a `role="alert"` names the edge and says "Automation relies on this transition", the entered values are kept and Cancel closes the form.
- *Given* a stage-form save that fails (network or server error), *When* the response returns, *Then* a `role="alert"` reads "Couldn't save stage: <reason>" with a Retry button, the values are kept, and Save is re-enabled.
- *Given* the banner is on screen and the operator flips a class flag on the Features page in another tab or device, *When* the Stages tab regains focus or becomes visible (`visibilitychange`/`focus`), *Then* the status RPC is re-fetched once and the banner updates in place; *Given* that refetch fails, *Then* the last value stays with "Last checked <ago>; couldn't refresh" (the Stale row of the vocabulary) and never reads Enforced if the last good value was not all-three-enforced (UX-36).
- *Given* the banner link "Change in Feature Flags", *When* followed, *Then* it targets `/settings/features#flag-row-backlog-workflow-engine-enforce` for the first class not yet enforced (operator when none is), and the link text is "Change in Feature Flags" (UX-37; the focus behavior is Task 1.3.3b).
- *Given* the stage form at 360px, *When* it is open, *Then* a one-line mode reminder ("Mode: Shadow. Saved edits are not enforced yet." / "Mode: Enforced." / "Mode: Off. Edits are saved but have no effect.") sits immediately above the Save button, not only in the banner that has scrolled out of view (UX-41).
- *Given* the banner has resolved once, *When* a visibility refetch or Retry changes the mode (or enters or leaves the Unknown or Stale state), *Then* the new mode sentence appears in `engine-mode-banner-live` exactly once; *Given* the first resolved render in any mode, *Then* that region is empty (announces changes, not the initial render; a save-triggered refetch that does not change the mode text announces nothing) (UX-01).
- *Given* the stage form at 360px in each of Enforced, Partly enforced, Shadow, Off, Loading, Unknown and Stale, *When* it is open, *Then* the one-line reminder above Save shows that state's copy from `design/ux.md` section 2, read from the same status object as the banner through one shared hook (changing the mocked status changes both in one render), never renders an empty line, and never shows Enforced-only wording for Unknown or for a Stale value that was not all-three-enforced; `stage-save-status` uses the matching confirmation ("Saved. Enforced for operators only; not enforced for agents and automation." / "Saved. Couldn't confirm whether it is enforced.") (UX-52).
- *Given* the status carries `review_due_at` and `review_overdue`, *When* the banner renders, *Then* a static line `data-testid="engine-mode-review"` reads "Review overdue: the soak review date (<date>) has passed and no caller class is enforced. Review the soak evidence in docs/how-to/flip-workflow-engine-enforcement.md." when overdue, "Soak review due <date>." before the date, "Soak review date <date>." when any class is enforced, and is absent when `review_state=UNAVAILABLE` (UX-55).
**Files**: `web-app/src/app/settings/backlog-stages/page.tsx`, `web-app/src/app/settings/backlog-stages/EngineModeBanner.tsx`, `web-app/src/app/settings/backlog-stages/EngineModeBanner.test.tsx`, `web-app/src/app/settings/backlog-stages/StageForm.tsx`, `web-app/src/app/settings/backlog-stages/StageForm.test.tsx`, `web-app/src/lib/hooks/useEngineModeStatus.ts`, `web-app/src/lib/hooks/useEngineModeStatus.test.tsx`, `web-app/src/app/settings/backlog-stages/page.css.ts`, `web-app/src/app/settings/backlog-stages/page.test.tsx`

##### Task 1.3.2a: Banner component, shared status hook, nested live region, states, fail-closed, copy fix (`page.tsx:151-154`) (M)
- `useEngineModeStatus` (one fetch, shared by the banner and `StageForm`, also consumed by the Features page in Story 1.3.3); banner container without `role` plus the nested empty-at-first-render `engine-mode-banner-live` region; the live-region policy in `design/ux.md` section 1.
- Files: `web-app/src/app/settings/backlog-stages/EngineModeBanner.tsx`, `web-app/src/app/settings/backlog-stages/EngineModeBanner.test.tsx`, `web-app/src/app/settings/backlog-stages/page.tsx`, `web-app/src/app/settings/backlog-stages/page.css.ts`, `web-app/src/app/settings/backlog-stages/page.test.tsx`, `web-app/src/lib/hooks/useEngineModeStatus.ts`, `web-app/src/lib/hooks/useEngineModeStatus.test.tsx`

##### Task 1.3.2b: StageForm save message carries mode, mode reminder above Save for all seven states, `stage-save-status` testid (S)
- Files: `web-app/src/app/settings/backlog-stages/StageForm.tsx`, `web-app/src/app/settings/backlog-stages/StageForm.test.tsx`

##### Task 1.3.2c: Documented carve-out note ("automation edges are fixed; automation gates are editable"; "Archive from active stages cannot be gated here; gate `done->archived` instead", the owner-approved option A wording) (XS)
- Files: `page.tsx`

##### Task 1.3.2d: System-required edge rejection alert (UX-07) (S)
- Render the `FailedPrecondition` from Story 1.2.1 as a `role="alert"` with the edge name and "Automation relies on this transition"; keep form values; Cancel is the exit.
- Files: `web-app/src/app/settings/backlog-stages/StageForm.tsx`, `web-app/src/app/settings/backlog-stages/StageForm.test.tsx`

##### Task 1.3.2f: Banner freshness: refetch on focus/visibility, Stale row, hash link (S)
- `EngineModeBanner` re-fetches on `visibilitychange`/`focus` (debounced, at most once per 5 s) and after a successful stage save; Stale copy when a refetch fails; builds the hash link described above. No polling timer (no extra RPC load while the tab is hidden).
- Files: `EngineModeBanner.tsx`, `EngineModeBanner.test.tsx`

##### Task 1.3.2e: StageForm save-failure alert with Retry (S)
- "Couldn't save stage: <reason>" `role="alert"` plus Retry; pending state `aria-disabled` with "Saving...".
- Files: `StageForm.tsx`, `StageForm.test.tsx`

##### Task 1.3.2g: Review-due line in the banner (S)
- Static `engine-mode-review` line under the mode text from `review_due_at` / `review_overdue` / `review_state` (Task 1.3.1c); copy and omission rule per `design/ux.md` section 2 (UX-55); the Features-page mirror is Task 1.3.3b.
- Files: `web-app/src/app/settings/backlog-stages/EngineModeBanner.tsx`, `web-app/src/app/settings/backlog-stages/EngineModeBanner.test.tsx`

#### Story 1.3.3: Mode flags on the Features page
**Prerequisites:** 1.3.2, 1.3.1, 4.1.4 (the `flagExtras` registry, Task 4.1.4b, and `AlertConfirmDialog`, Task 4.1.4e)
**As an** operator, **I want** one place to flip shadow and each caller class's enforce flag with a confirm that shows the readiness evidence, **so that** rollback is as easy as enable, I can enforce one class at a time, and I never enforce blind.
**Acceptance Criteria**:
- *Given* the Features page, *When* the page renders, *Then* a group "Workflow engine" shows the shadow flag and three enforce rows labelled "Enforce for operators (this UI)", "Enforce for agents (MCP tools)" and "Enforce for automation (lifecycle, sync, pollers)", and above the rows a summary line (`data-testid="workflow-engine-summary"`, static text with no `role`; a toggle's change is announced by that row's confirmation, which appends the new count) in plain words: "0 of 3 caller classes enforced. Shadow: edits are compared with the built-in rules but not applied." / "1 of 3 caller classes enforced (Partly enforced): operators enforced; agents and automation still in shadow." / "All 3 caller classes enforced." / "Mode unavailable: couldn't read the engine status" (Unknown); the wording matches the Stages banner states exactly (UX-39).
- *Given* any enforce row, *When* it is toggled on, *Then* an `AlertConfirmDialog` (`role="alertdialog"`, NOT `PiDisableWarningDialog`) opens with initial focus on Cancel, a focus trap, Escape and overlay click cancel, focus returns to the switch whether the dialog was opened by mouse click, touch tap or keyboard (UX-43: the hook restores focus, the component owns overlay click and `returnFocusRef`), and the body states "Stage edits will change which transitions and gates are allowed" for that class (the automation row adds "Blocked automated transitions appear as stuck items (transition_denied) until the gate is satisfied"); the flag changes only after "Enforce".
- *Given* the enforce dialog, *When* it opens, *Then* it shows the readiness evidence for that class at decision time from `GetWorkflowEngineStatus.class_readiness` (UX-35): "Shadow differences in the last 24h: N", "Soak verdict: READY / NOT READY / needs your acknowledgement / unavailable", "Items that would lose an exit: M" and the plain-text pointer "Rollout checklist: docs/how-to/flip-workflow-engine-enforcement.md". *Given* `divergence_24h > 0`, or `soak_verdict` is not `READY`, or the evidence failed to load, *Then* the Enforce button is disabled (`aria-disabled`) until the operator ticks "I have reviewed this evidence and the rollout checklist" (`data-testid="feature-flag-enforce-ack"`); *Given* the evidence is clean (0 differences, READY, M=0), *Then* Enforce is enabled immediately. The dialog never hides a bad number to look reassuring, and a failed evidence load reads "Couldn't load readiness evidence" with Retry, not zeros.
- *Given* an enforce row on, *When* the row renders, *Then* it shows "Revert to Off" (`data-testid="feature-flag-revert"`) within the same row and an "Edit stages" link (`data-testid="feature-flag-edit-stages"`), and the Stages banner links back.
- *Given* keyboard-only use (Tab, Enter/Space), *When* the enforce row is focused, *Then* the tab order is switch, then "Revert to Off", then "Edit stages"; both controls are real `<button>`/`<a>` elements with accessible names, at least 44px tall at 360px, with a visible `:focus-visible` ring, and Revert works without a dialog (UX-16, UX-33).
- *Given* the URL `/settings/features#flag-row-backlog-workflow-engine-enforce` (or the `-agent`/`-system` row id), *When* the page loads, *Then* that row exists with `id`, `tabIndex={-1}`, `scroll-margin-top: calc(var(--header-height) + 16px)` and `scroll-margin-bottom: calc(var(--bottom-nav-height) + 16px)` (the existing CSS custom properties, so the offset is a token), receives programmatic focus once the flag list reaches `loaded` (not before), is scrolled into view and is not obscured (WCAG 2.4.11); *Given* the hash names a row that does not exist, *Then* the page loads normally with no error; *Given* the list load fails and a Retry then succeeds, *Then* the row is focused once; *Given* the list is still loading after 10 s, *Then* the attempt ends with no focus change and no error; *Given* the user pressed a key or pointer button, or focus is on any element other than `body` or the page container, before the list renders, *Then* the row is NOT focused and the page is not scrolled; *Given* a `hashchange` or `popstate` to another row id, *Then* that row is focused once the list is loaded; the page reads `window.location.hash` on mount, `hashchange` and `popstate` (Next `pushState` same-page hash links fire neither; none exist) (UX-37, UX-47).
- *Given* a flag flipped elsewhere (another tab or device), *When* this tab regains focus or becomes visible, *Then* the flag list is re-fetched once and the rows and summary update in place (UX-36); the engine rows' note reads "Takes effect within a few seconds, without reload" instead of `LIVE_FLAG_NOTE`'s "without reload" alone, because the mode reader caches about 1s (`GetWorkflowEngineStatus` TTL; Story 1.3.1).
- *Given* the enforce dialog is open, *When* readiness evidence is loading, resolves clean, resolves not clean, or fails, *Then* (UX-44) while loading: Enforce is `aria-disabled`, `feature-flag-enforce-ack` is absent and `feature-flag-enforce-evidence` (the dialog's only `role="status"` region) has `aria-busy="true"`; on resolution the region announces the evidence once ("Readiness evidence: ..."; "Acknowledgement required." appended when not clean), drops `aria-busy`, and programmatic focus does not move (it stays on Cancel); the evidence block keeps its three-line height so nothing jumps; the acknowledgement row, when present, sits between the evidence and the buttons, the whole label (at least 44px tall, full dialog content width) toggles the checkbox (UX-54); an `aria-disabled` Enforce has `aria-describedby` to the visible hint `feature-flag-enforce-hint` ("Waiting for readiness evidence." / "Tick the acknowledgement above to enable Enforce.") and activating it changes nothing; on a failed load, the Retry button (`feature-flag-enforce-evidence-retry`) keeps focus while pending and after a repeat failure, and focus moves to Cancel after a success.
- *Given* the dialog showed evidence at open, *When* the operator clicks an enabled Enforce, *Then* (UX-45) the dialog re-fetches `GetWorkflowEngineStatus` first; if `class_readiness[class]` is unchanged or not worse than what was acknowledged, `UpdateFeatureFlag` is called exactly once; if it is worse (divergence up, verdict moved away from READY, stranded count up, or clean-now-unclean), `UpdateFeatureFlag` is NOT called, the evidence updates in place with the announcement "Readiness evidence changed since you opened this dialog: <what changed>. Review it and confirm again.", a ticked acknowledgement is cleared, Enforce returns to `aria-disabled` and focus moves to the acknowledgement checkbox; if the re-check itself fails, `UpdateFeatureFlag` is NOT called and `feature-flag-enforce-recheck-alert` reads "Couldn't re-check readiness: <reason>" with Retry; the check is client-side because `UpdateFeatureFlag` is bool-only and shared (accepted residual: one RPC round trip).
- *Given* the three enforce rows, *When* rendered, *Then* (UX-46) the group intro reads "These three switches are independent. Recommended order: operators, then agents, then automation.", each row shows its step line ("Step 1 of 3 (recommended first).", "Step 2 of 3. Enforce after operators have soaked.", "Step 3 of 3. Enforce last; it needs the denied-transition recovery in the deployed binary."), no row is disabled by another row's state, and enabling a class whose recommended predecessor is not enforced adds the non-blocking advisory `feature-flag-enforce-order-advisory` in the dialog (no acknowledgement); the Stages banner link targets the first not-yet-enforced class in this order.
- *Given* a toggle in flight, *When* Space or Enter is pressed on the switch or "Revert to Off" is clicked in the same row, *Then* no second request is sent, the switch keeps focus and stays `aria-disabled`, "Revert to Off" is `aria-disabled`, and the row's status region announces "Saving <flag>..." once followed by the result (UX-48).
- *Given* the status carries the review fields, *When* the "Workflow engine" group renders, *Then* its summary block shows `workflow-engine-review` with the same states and copy as the Stages banner (UX-55).
**Files**: `web-app/src/app/settings/features/page.tsx`, `web-app/src/app/settings/features/flagExtras.tsx`, `web-app/src/app/settings/features/EnforceReadinessEvidence.tsx`, `web-app/src/app/settings/features/EnforceReadinessEvidence.test.tsx`, `web-app/src/app/settings/features/page.test.tsx`, `web-app/src/app/settings/features/page.css.ts`, `web-app/src/app/settings/features/useEnforceReadiness.ts`, `web-app/src/app/settings/features/useEnforceReadiness.test.tsx`, `web-app/src/app/settings/features/useHashRowFocus.ts`, `web-app/src/app/settings/features/useHashRowFocus.test.tsx`

##### Task 1.3.3a: Enforce confirm via `AlertConfirmDialog` with readiness evidence, Revert/Edit-stages controls and cross-links (M)
- New `EnforceReadinessEvidence` content passed into the dialog's body slot; acknowledgement checkbox gating; per-class copy; rows come from the `flagExtras` registry; the switch handler focuses the switch and passes `returnFocusRef` before opening. Depends on Story 4.1.4 (done in Wave 0) and Story 1.3.1.
- Files: `web-app/src/app/settings/features/page.tsx`, `web-app/src/app/settings/features/flagExtras.tsx`, `web-app/src/app/settings/features/EnforceReadinessEvidence.tsx`, `web-app/src/app/settings/features/EnforceReadinessEvidence.test.tsx`, `web-app/src/app/settings/features/page.test.tsx`

##### Task 1.3.3b: Workflow-engine summary and review lines, hash-focus hook, engine-row live note, refetch on visibility (M)
- `id`/`tabIndex`/`scroll-margin-top`/`scroll-margin-bottom` (tokens `--header-height`, `--bottom-nav-height`) on each flag row (`flag-row-<flag name with ':' replaced by '-'>`); `useHashRowFocus` implements the terminal-state, 10 s ceiling, no-steal (interaction and `activeElement`) and `hashchange`/`popstate` rules of the hash AC; `flagExtras` supplies the per-flag note override (engine rows), the static summary and the `workflow-engine-review` line; `visibilitychange`/`focus` refetch.
- Files: `web-app/src/app/settings/features/page.tsx`, `web-app/src/app/settings/features/page.css.ts`, `web-app/src/app/settings/features/flagExtras.tsx`, `web-app/src/app/settings/features/useHashRowFocus.ts`, `web-app/src/app/settings/features/useHashRowFocus.test.tsx`, `web-app/src/app/settings/features/page.test.tsx`

##### Task 1.3.3c: Enforce-dialog evidence state machine and readiness re-check on confirm (M)
- `useEnforceReadiness` hook: loading, clean, not-clean, failed, re-checking, changed-since-open, re-check-failed states; fetches on open, re-fetches on an enabled Enforce click, compares `class_readiness[class]` with the acknowledged view, and only then calls `UpdateFeatureFlag`; drives the single live region, the hint element, focus handoffs (Cancel after Retry success, acknowledgement checkbox after a worse re-check) and the non-blocking order advisory. Jest: every transition above with a recorded RPC counter (the write is never called on a worse or failed re-check).
- Files: `web-app/src/app/settings/features/useEnforceReadiness.ts`, `web-app/src/app/settings/features/useEnforceReadiness.test.tsx`, `web-app/src/app/settings/features/EnforceReadinessEvidence.tsx`, `web-app/src/app/settings/features/EnforceReadinessEvidence.test.tsx`

##### Task 1.3.3d: Independent-flags note, step lines and the order advisory text (XS)
- Group intro, per-row step lines and the advisory element through `flagExtras` (copy in `design/ux.md` section 3, UX-46); the banner link target helper (first not-yet-enforced class in recommended order) is shared with Task 1.3.2f.
- Files: `web-app/src/app/settings/features/flagExtras.tsx`, `web-app/src/app/settings/features/page.test.tsx`

### Epic 1.4: Soak and flip runbook
**Goal**: A repeatable, owner-gated path from shadow to enforce.

#### Story 1.4.1: Runbook
**Prerequisites:** 1.3.3, 1.1.3, 1.5.9, 0.1.1 (the verification script, Task 0.1.1d); W, N, floor and review date are owner-approved 2026-10-17
**As the** owner, **I want** a how-to with exact criteria and commands per caller class, **so that** each flip (operator, then agent, then system) is evidence-based and revertable.
**Acceptance Criteria**:
- *Given* `docs/how-to/flip-workflow-engine-enforcement.md`, *When* an operator follows it, *Then* it names the actuator URL, `make workflow-engine-soak-status` (and `ARGS=--config-diff` for the first-hour live-config check), the criteria (zero `graph`, `gate_missing`, `gate_extra`, `error_kind` under `config=seeded`; `cache_empty` fallbacks 0; at least N shadowed `path=transition` calls on every observed engine-routed edge; window of at least W days, with W and N set by the owner before the soak starts, see Unresolved Questions), requires the owner to acknowledge every edge listed `UNVERIFIED in shadow` (stateful gate) and every `--config-diff` edge with a divergence when config is `edited` (a window spanning an edit counts as edited), states that alarms fire on either path but READY counts `path=transition` only, does not claim lifecycle edges are exempt (under ADR-007 they are gated per class and the `system` class soak is its own gate), the per-class flip order and flags (`backlog:workflow-engine-enforce`, `-enforce-agent`, `-enforce-system`), the vacuity guards (zero calls is NOT READY; span W; per-edge N or the low-volume acknowledgement), the low-volume READY rule (zero divergence under seeded config plus the owner's acknowledgement of the `--config-diff` output, the stranded preview, the Story 1.5.9b per-caller table and the Story 1.5.9d replay report on a copy of the live database; N=20 and the floor of 100 are printed as targets but are not reachable for the operator and system classes at the live volume), the statement that the `-enforce-system` flag does not exist until Story 1.5.8 ships (it is registered by Task 1.5.8g, after the spawn-point guards, detector-local guards and re-drive sweep), so the system-class step requires the Story 1.5.8 completeness test green on the deployed binary, and that an agent's `trigger_triage` MCP call is a `TriggerTriage` RPC classed `system` (Task 1.5.6e), so it is governed by `-enforce-system`, not `-enforce-agent`, the stranded-item preview from `--config-diff` and a requirement that the owner acknowledges each previewed item before enforce, the acknowledgement table for newly governed unchecked callers (each divergent `(caller,from,to)`, including `guard_new`), the roughly 1s mode-lag, rollback (toggle that class off; all four off equals today), and a first line "Before deploying the binary that contains this change, confirm the deployed unit passes `--tmux-keep-server` (`docs/explanation/tmux-keep-server-on-restart.md`); otherwise the restart kills every live tmux session".
- *Dated review and outcome (requirements Metric 6)*: *Given* the runbook, *When* read, *Then* it opens with a "Review dates" block (`first deploy: <date>`, `review due: <first deploy + 14 days>`, a ready-to-paste calendar reminder line) and ends with an "Outcome review" step that records, beside the per-class soak verdicts, the stuck-item rate (distinct items with an open stuck row from `ListStuckBacklogItems` over `ListBacklogItems` count) and the manual interventions of the week, compares them with the baseline (6 of 21 on 2026-10-18), and says what to do when no class is enforced at the review date (decide per class, or record why not).
- *Dry run on the manual instance (mandatory, an AC and not only a reviewer row)*: *Given* the finished runbook, *When* the story worker follows it step by step against `scripts/verify-manual-instance.sh <N>` (the slot assigned in the wave table), *Then* every command and URL in it was executed with its output and exit code pasted into the PR (actuator URL, `make workflow-engine-soak-status` per class including the exit-1 NOT READY case on a fresh ledger, `ARGS=--config-diff`, one enforce flip with the readiness-evidence dialog, the Revert to Off), every step that failed or needed a fact the runbook lacked was fixed in the runbook before merge, and no `:8543`/`:8444` process was touched.
**Files**: `docs/how-to/flip-workflow-engine-enforcement.md`, `CLAUDE.md` reference-index row

##### Task 1.4.1a: Write how-to and index row (S)
- Files: `docs/how-to/flip-workflow-engine-enforcement.md`, `CLAUDE.md`

##### Task 1.4.1b: Dry-run the runbook on the manual instance and fix it (S)
- Execute the AC above; evidence in the PR. Cannot be done by reading the document.
- Files: `docs/how-to/flip-workflow-engine-enforcement.md`

### Epic 1.5: Every status-write path goes through the engine (Success Metric 1, all paths)
**Goal**: Editing a stage, transition or gate in the stages UI changes the result of every status transition, whichever caller class makes it. One choke point (`Storage.GatedTransition`), a closed caller registry, one enforce flag per caller class, and a repo-wide guard that fails on any raw status write outside the allow-list. Design and risks: [ADR-007](../decisions/ADR-007-engine-gated-transition-choke-point.md). Supersedes the "measure, don't govern" scope of ADR-002 D1.

**Caller classes and policy** (ADR-007 Decision 3):

| Class | Who | Graph | Default built-in guards | Operator gates | `override_reason` | Denial surfaces as |
|---|---|---|---|---|---|---|
| `operator` | A human through UI/RPC: handler, `OverrideVerdict`, `SubmitManualReview`, archive/unarchive RPCs, `UpdateBacklogItem` PR association, `TriggerTriage` RPC, `forceResetItem` (classification confirmed by Task 1.5.1a) | Configured UNION system-required | yes | yes | Default guards keep their own override semantics (input unchanged); additionally bypasses Configured operator gates; never the graph | error to the human |
| `agent` | An AI session through MCP. Purpose-specific tools with fixed edges: `request_review`, `report_blocked`, `resume_work`, `report_duplicate`, `report_pr_created`. The generic `transition_backlog_item_status` delegate (actor `agent`) is `forwarding` and EXEMPT from system-required edges (ADR-007 D4): it forwards the edge the agent requests, so the graph can deny it | same | yes | yes | none honored | error with next-step hint, plus reporter |
| `system` | Reconciliation, lifecycle, PR/Jules pollers, `SyncLoop`, auto-reopen/respawn, triage pipeline, autonomous completion (triage-role `idea->ready` only), `transitionWithGuard` callers, `AttachSessionToItem`, `ReconcileStuck` | same | per registry (`BuiltIn` only for `transitionWithGuard` callers and `AttachSessionToItem`, which ask for guards today) | yes | none exists | reporter (`transition_denied`), never swallowed |

Legacy baseline for Off and Shadow: callers that ask the engine today (`Engine`: handler, `OverrideVerdict`, `transitionWithGuard` callers, `AttachSessionToItem`, `SyncLoop`) keep the Default answer; callers that write storage directly today (`Unchecked`) keep "allow". So Off equals today's behavior exactly, and Shadow's divergence series measure what enforcing would newly deny. The registry has fewer entries than the 45 sites: the 5 exempt sites (3 rollbacks plus 2 debug) and the `create` class are listed in the inventory with a reason but are not registered callers.

#### Story 1.5.1: Caller registry and edge audit
**Prerequisites:** 1.2.3, 1.2.1 (`SystemRequiredEdges()` does not exist in the tree today, `git grep` returns zero hits; Task 1.1.2b creates it in W2, Story 1.2.1 fixes its edge list, and this story adds the archive group)
**As a** maintainer, **I want** every status-write caller registered with its class, legacy policy, guard policy and the edges it can request, **so that** the choke point, the flags and the system-required set are derived from one table and cannot drift from the call sites.
**Acceptance Criteria**:
- *Given* the module, *When* the registry test runs, *Then* every call site in the inventory's pending list maps to exactly one `TransitionCaller` entry `{name, class, legacy, guards, edges, forwarding}`, and an unregistered or duplicate site fails naming file and function.
- *Given* each registry entry, *When* the audit test runs, *Then* (a) every non-`forwarding` `agent` or `system` entry has a non-empty edge set that is a subset of `SystemRequiredEdges()`; (b) every `operator` entry's edges are in `ValidTransitions()` or `SystemRequiredEdges()`; (c) the set of `forwarding` entries is exactly `{mcp_transition_delegate}` (pinned, so a second generic forwarder fails the test and needs a deliberate registry change), and the forwarding entry contributes no edge to `SystemRequiredEdges()`; an off-graph edge fails naming the caller and edge. The audit is therefore not vacuous for the one agent entry whose edge set is unbounded: that entry is governed by the graph like an operator (ADR-007 D4).
- *Given* the known gap (VERIFIED: `validTransitions`, `session/domain/backlog.go:514-574`, has no `in_progress|review|pr_pending->archived` and no `archived->archived`, while `ArchiveBacklogItem` archives from any status), *When* Task 1.5.1c finishes, *Then* those edges are system-required (`SystemRequiredEdges()`, including `*->archived` for custom stage slugs) and **`validTransitions`, the seed, `SeedFingerprint` and `AllowedTransitions` output are unchanged**: `session/workflow_engine_test.go:22` ("in_progress to archived has no such edge") still passes and an `in_progress` item's `allowed_transitions` does not gain `archived`.
- *Given* a live database whose seeded config lacks those edges (the seed is create-if-count-zero and never re-run), *When* the operator Archive RPC, `reportDuplicate` and `archiveStaleDoneItems` run on `in_progress`, `review`, `pr_pending` and `done` items under Enforce for their class, *Then* they succeed (stale-seed test). *Given* the same four archive edges and an EMPTY stage-config cache (or nil `Configured`, or `load_error`) under Enforce, *When* `GatedArchive` runs on an `in_progress`, `review`, `pr_pending` and already-`archived` item, *Then* it succeeds, because the fallback graph is Default UNION `SystemRequiredEdges()` (ADV-N1; case added to `TestSwitchableWorkflowEngine_FallsBackToDefaultGraph_WhenConfiguredCacheEmptyOrNil`).
- *Given* a caller that computes `to` dynamically, *When* the audit runs, *Then* its registry entry lists the finite set of values the variable can take, derived from the code, and a unit test pins that set. VERIFIED for `autonomous_complete`: `toStatus` is assigned exactly once (`autonomous_orchestration_service.go:453`, triage role, `ready`, with `ExpectedStatus=idea`), so the pinned set is `{ready}` and the single edge is `idea->ready`; the work and review roles perform no status write and the `toStatus == Review` branch at `:604` is dead code (removed in Task 1.5.6a).
**Files**: `session/transition_callers.go`, `session/transition_callers_test.go`, `session/workflow_system_edges.go`, `session/workflow_system_edges_test.go`, `project_plans/backlog-make-it-true/decisions/ADR-007-engine-gated-transition-choke-point.md`

##### Task 1.5.1a: Registry types and table for all 45 sites (M)
- `TransitionCaller` (with `forwarding`), `TransitionCallerClass` (sum type), `LegacyPolicy`, `GuardPolicy`; table of every caller keyed by name; the wildcard `*->archived` for archive callers; derives `SystemRequiredEdges()` group membership from non-forwarding `agent` and `system` entries; `ready->queued` etc. stay. Proposed classification in the Epic 1.5 table is confirmed or corrected here against each function (including `forceResetItem` and `TriggerTriage`).
- Files: `session/transition_callers.go`, `session/workflow_system_edges.go`

##### Task 1.5.1b: Edge-audit test and off-graph edge decisions (M)
- Walk the registry, diff every declared edge against `ValidTransitions()` and `SystemRequiredEdges()` per the ACs above; record each off-graph edge and its decision in the ADR-007 text; assert every `(from,to)` literal passed to `transitionWithGuard`, `GuardedTransitionAllowed` and `AttachSessionToItem` is registered (closes pre-mortem #13); pin the `forwarding` singleton and the `autonomous_complete` set `{ready}`.
- Files: `session/transition_callers_test.go`, `project_plans/backlog-make-it-true/decisions/ADR-007-engine-gated-transition-choke-point.md`

##### Task 1.5.1c: Archive edges join the system-required set; table and seed untouched; stale-seed test (S)
- Add `X->archived` for every built-in status X, `archived->archived`, and the custom-stage `to=archived` predicate to `session.SystemRequiredEdges()` / `IsSystemRequired`; do NOT edit `validTransitions`, `workflow_stage_seed.go` or the equivalence tests; add the stale-seed test (live database without the edges still archives) and a test that the archive edges are absent from `ValidTransitions()` and from `AllowedTransitions` for an `in_progress` item, plus the empty-cache case from the AC above. The Story 1.2.1 validator treats them like every system-required edge (an attempt to disable one returns `FailedPrecondition` naming it). If the owner prefers a table change, that is Unresolved Question 1's alternative and would also require updating `workflow_engine_test.go:22` and the seed fingerprint.
- Files: `session/workflow_system_edges.go`, `session/workflow_system_edges_test.go`

#### Story 1.5.2: Class-aware selector, `ModeFor(class)` and shadow baselines
**Prerequisites:** 1.5.1
**As an** operator, **I want** each caller class to have its own mode, **so that** I can enforce operator moves first, agents next and automation last, and revert one class without touching the others.
**Acceptance Criteria**:
- *Given* a class and mode, *When* `enforceAnswer(class, guardPolicy)` runs, *Then* it follows the class table above: operator: Default guards keep their own override semantics and Configured operator gates are bypassed by a non-empty `override_reason`; agent ignores it, system has none, and the graph check is never bypassed.
- *Given* a registered `Unchecked` caller in `Off` or `Shadow`, *When* the gate decides, *Then* the answer is "allow"; in `Shadow` the recorder computes `enforceAnswer` and counts `graph`, `gate_extra` or `guard_new` divergence labelled with `class` and `caller`; under the seeded config and each caller's declared edges the expected divergences are zero except `guard_new` for agent callers whose edge has a Default guard, each of which is listed for owner acknowledgement.
- *Given* a class and its enforce flag, *When* `ModeFor(class)` runs, *Then* the truth table holds (enforce flag of the class wins over shadow; shadow on, enforce off gives Shadow; both off gives Off; empty config gives Shadow), implemented as a class-to-flag table read through the production reader. Only the operator enforce flag exists at this point; the agent flag is registered in Task 1.5.3c (the task that wires the reporter) and the system flag in Task 1.5.8g (the task that completes the recovery guards). A class whose enforce flag is not registered yet resolves from the shadow flag alone, so it cannot enforce, and no flip is possible for it; this is a wave-order property, not a runtime state (ADR-007 D8). The final four-flag truth table per class is asserted by `TestModeFor_ComposesFourFlagsPerClass_AndDefaultsToShadow` once Task 1.5.8g lands, and each task that registers a flag adds its class's rows.
- There is deliberately no runtime degrade clause (no `RecoveryReady()` read, no `reporter_unwired` branch, no `recovery_wired` proto field, no `enforce_degraded_total` metric): the hazards they would guard (a flag flipped before the reporter or the recovery guards exist) cannot occur because the flag is registered in the task that closes the hazard and every flip is owner-gated after soak.
- *Given* `make workflow-engine-soak-status`, *When* it runs, *Then* it evaluates each class separately with that class's expected-edge list taken from the registry, the same vacuity guards (zero calls, span W, per-edge evidence) and per-class exit codes, and prints one verdict line per class. READY for a low-volume edge or class follows the rule in Risk Control (owner acknowledgement of four artifacts), not the sample counts alone.
**Files**: `session/workflow_engine_selector.go`, `session/workflow_engine_enforce.go`, `session/workflow_engine_shadow.go`, `server/services/feature_flag_service_test.go`, `server/actuator.go`, `Makefile`

##### Task 1.5.2a: `ForClass` view and class/guard policy in `enforceAnswer` (M)
- Class-bound `WorkflowEngine` view over the selector so the gate and legacy callers use the same interface; operator gets override semantics (operator-gate bypass only; Default guards unchanged), agent does not, system uses registry guard policy; `forwarding` callers use the graph without a system-required union contribution.
- Files: `session/workflow_engine_selector.go`, `session/workflow_engine_enforce.go`, `session/workflow_engine_selector_test.go`

##### Task 1.5.2b: `ModeFor(class)` over a class-to-flag table, truth-table and parity tests (S)
- Class-to-flag table (operator bound to the existing `backlog:workflow-engine-enforce`; agent and system rows are added by Tasks 1.5.3c and 1.5.8g, which also register the flags, their defaults in `config` and their `ConfigSurfaceProof` entries); no global atomic, no degrade branch; registry-versus-production-reader parity test for the operator row; a table test that a class with no registered flag resolves from the shadow flag alone.
- Files: `session/workflow_engine_selector.go`, `server/services/feature_flag_service_test.go`

##### Task 1.5.2c: Shadow baseline for `Unchecked` callers and the `guard_new` class (M)
- Recorder takes `(legacyAnswer, enforceAnswer)` where legacy for `Unchecked` is allow; add `guard_new` divergence; labels `class`, `caller`; same `recover()` and sampling rules as Task 1.1.2a.
- Files: `session/workflow_engine_shadow.go`, `session/workflow_engine_shadow_test.go`

##### Task 1.5.2d: Per-class ledger, actuator and soak-status (S)
- Per-class counters and ledger sections; `soak-status` per-class verdicts and the acknowledgement input (`--ack-file`, carrying the four-part acknowledgement of Risk Control per class); extends Task 1.1.3b.
- Files: `server/actuator.go`, `Makefile`, `session/workflow_engine_shadow.go`, `server/actuator_test.go`

#### Story 1.5.3: `Storage.GatedTransition`, the choke point
**Prerequisites:** 1.5.2
**As a** maintainer, **I want** one function that decides and writes a status change, **so that** no path can reach the database without the engine.
**Acceptance Criteria**:
- *Given* a request with no precondition for an item observed in `ready`, *When* the gate allows, *Then* it writes with `ExpectedStatus=ready`; *Given* the item moved to `queued` between decision and write, *Then* it returns `ErrPreconditionFailed`, not an engine denial, and nothing is written.
- *Given* a caller-supplied `ExpectedStatus` that differs from the observed status, *When* called, *Then* it returns `ErrPreconditionFailed` without an engine call.
- *Given* a request carrying a `BacklogItemPrecondition` with `ExpectedStatus`, `ExpectedUpdatedAt` and `Note`, *When* the gate writes, *Then* all three reach the raw writer unchanged (`ExpectedUpdatedAt` is never dropped, `Note` lands in the status event), and `TriggeredBy` is recorded; *Given* a request with prebuilt `Guard` input and `Observed` but no `ExpectedUpdatedAt`, *Then* the gate sets `ExpectedUpdatedAt = Observed.UpdatedAt`, and a concurrent field edit that bumps `updated_at` yields `ErrPreconditionFailed`; *Given* `guards=None`, *Then* the CAS is status-only.
- *Given* `Unconditional=true` and a concurrent mover, *When* the first attempt fails the CAS, *Then* the gate re-reads and re-decides once more (maximum two attempts) and never writes a decision made for a different `from`.
- *Given* the engine denies an `agent` or `system` request, *When* the gate returns, *Then* `TransitionDeniedReporter.Report(item, caller, from, to, reason)` was called exactly once for that `(item, caller, from, to, reason)` before the error was returned, and a second identical denial does not report again; the same edge denied for a different caller (for example `autonomous_complete` versus `maybe_trigger_triage` on `idea->ready`) reports separately; an `operator` denial returns the error only.
- **The guarantee that no denial is swallowed is this gate test, not a lint rule** (ARCH-C1): *Given* every denial source (`config_graph`, `default_guard`, `operator_gate`, `target_stage_removed`) crossed with every non-`operator` class, *When* `TestGatedTransition_EveryAgentOrSystemDenial_ReachesReporter` drives the gate with a recording reporter, *Then* each case records exactly one `Report` before the gate returns and nothing is written; a mutation that skips the call for any class or source fails it. Because the gate itself calls the reporter, callers of an `agent`/`system` `Gated*` method have nothing further to surface, so Task 1.5.4f does not lint them.
- *Given* `BeforeCommit` returns an error, *When* the engine allowed, *Then* nothing is written and the error is returned; *Given* it succeeds, *Then* it ran after the engine decision and before the CAS (the handler's push-before-done order).
- *Given* `Storage` has no gate injected, *When* any caller runs, *Then* it behaves as `Off` (legacy answer), increments `workflow_engine_gate_unwired_total`, logs a WARN once per minute, and `GetWorkflowEngineStatus.gate_wired` is false; the production wiring test asserts the gate is injected and is the same selector the handler uses.
- *Given* the benchmark (INFERRED budget: under 50 microseconds per call excluding I/O, no regression in existing `Transition*` benchmarks), *When* run for Off, Shadow and Enforce, *Then* the numbers are recorded in the PR.
- *Given* the overrun checkpoint (Task 1.5.3e), *When* Story 1.5.3 merges, *Then* the checkpoint record exists before any W11 story is dispatched.
**Files**: `session/transition_gate.go`, `session/transition_gate_test.go`, `session/storage.go`, `server/services/backlog_transition_notify.go`, `server/dependencies.go`, `server/dependencies_workflow_engine_test.go`, `server/services/feature_flag_service.go`, `server/services/feature_flag_service_test.go`, `config/config.go`, `server/services/config_surface_proof_test.go`

##### Task 1.5.3a: `TransitionRequest`, `GatedTransition`, CAS derivation, `BeforeCommit`, bounded `Unconditional` retry, `Rollback` and `DebugForce` entry points (M)
- Request carries `Caller`, `ItemID` or `Observed`, `To`, `Precondition *BacklogItemPrecondition` (whole struct: `ExpectedStatus`, `ExpectedUpdatedAt`, `Note`), `TriggeredBy`, `OverrideReason`, `Guard *BacklogItemTransitionInput` (prebuilt by callers that already assemble it), `Unconditional`, `BeforeCommit`. Derivation rules in ADR-007 D6 (`repository.go:730` shape). Uses `ForClass` from Task 1.5.2a. Records `gate_calls_total{class,caller,mode}`.
- Files: `session/transition_gate.go`, `session/transition_gate_test.go`

##### Task 1.5.3b: `TransitionDeniedReporter` interface and `server/services` adapter (S)
- Interface in `session` with `Report(item, caller, from, to, reason)`; adapter wraps `transitionOrNotify` (Story 1.2.2); idempotence per `(item, caller, from, to, reason)` is the helper's; the gate calls it for classes `agent` and `system`. Includes `TestGatedTransition_EveryAgentOrSystemDenial_ReachesReporter` (the Story 1.5.3 guarantee above).
- Files: `session/transition_gate.go`, `server/services/backlog_transition_notify.go`

##### Task 1.5.3c: Inject the gate and reporter, register the `-enforce-agent` flag in the same task, startup line, wiring test (S)
- Construct after the selector; `storage.SetTransitionGate(...)` and the reporter in one change; the same task adds `backlog:workflow-engine-enforce-agent` to `knownFeatureFlags` (default declared once in `config`, off), the agent row of the class-to-flag table, its `ConfigSurfaceProof` entry (deleting its pending line) and its truth-table rows, so the flag can never exist without the reporter (ARCH-C3: no `reporter_unwired` degrade); startup line gains `transition_gate=wired reporter=wired class_modes=operator:shadow,agent:shadow,system:shadow` (the system class has no enforce flag until Task 1.5.8g and shows `shadow`); wiring test with narrow `ServerDependencies`; touches `server/dependencies.go` (serialized, W10).
- Files: `server/dependencies.go`, `session/storage.go`, `server/dependencies_workflow_engine_test.go`, `server/services/feature_flag_service.go`, `config/config.go`, `server/services/feature_flag_service_test.go`, `server/services/config_surface_proof_test.go`

##### Task 1.5.3d: Gate microbenchmarks and ceiling (S)
- Run with `&`; compare Off, Shadow, Enforce; record ceiling in the test comment.
- Files: `session/transition_gate_bench_test.go`

##### Task 1.5.3e: Overrun checkpoint after the gate story (XS)
- A real gate, not an aside. **Trigger** (any one): (1) cumulative actual CU of every story completed in W0 to W10 exceeds the sum of their table values in "Effort estimate" by more than 25%; (2) Story 1.5.3's own actual exceeds its table value by more than 25%; (3) any single story in W0 to W10 exceeds its table value by more than 50%; (4) actual usage cannot be read per story (counts as a trigger, reported UNKNOWN). **Action**: the dispatcher records actual versus table CU per story (session cost view or per-session token totals), appends a dated "Overrun checkpoint" line under the plan's Repair log, and, when a trigger fires, does not dispatch W11 until the delta report (wrong estimate versus real surprise) is posted and the owner replies continue or re-baselines. When no trigger fires it still records the numbers and proceeds. Informational, not a cut line: full scope stands (the owner decision binds), but the 1.5.4 and 1.5.8 estimates are re-based from the measured error. The W10 wave row and the Worker brief standard block repeat this rule.
- Files: `project_plans/backlog-make-it-true/implementation/plan.md`

#### Story 1.5.4: Close the raw writers
**Prerequisites:** 1.5.3
**As a** maintainer, **I want** the ungated writers unreachable except through the gate, **so that** a stray call is a compile error or a failing guard, not a review catch.
**Acceptance Criteria**:
- *Given* the module after this story, *When* `go build ./...` runs, *Then* a call to any old method name on `*EntRepository` (`TransitionBacklogItemStatus`, `ArchiveBacklogItem`, `UnarchiveBacklogItem`, `ReconcileStuckItems`, `TransitionBacklogItemStatusWithPRFields`) fails to compile from any package, because the five repo writers are unexported (`raw*`, ADR-007 D1); `Storage` exports no old-named writer: the wrappers are `RawTransitionBacklogItemStatus`, `RawArchiveBacklogItem`, `RawUnarchiveBacklogItem`, `RawSetBacklogItemPRAndTransition`, and every previous caller either compiles against a `Gated*` entry point or uses a `Raw*` name that the inventory lists as `pending: migrate in Story 1.5.5/1.5.6/1.5.7` (so the build is green while migration proceeds). `ReconcileStuckItems` has no `Storage` wrapper and is covered by `GatedReconcileStuck` plus the unexport.
- *Given* `GatedArchive`, `GatedUnarchive`, `GatedSetPRAndTransition` and `GatedReconcileStuck`, *When* each is called by a registered caller, *Then* it runs the same decision function as `GatedTransition`, with `to=archived`, `to=idea`, `to=pr_pending` and `to=review` respectively; `GatedSetPRAndTransition` keeps `PRReassignmentGuard` semantics and its single UPDATE through the unexported `rawTransitionBacklogItemStatusWithPRFields`.
- *Given* `ReconcileStuck` with two items stuck in `in_progress` and an operator `human_approval` gate on `in_progress->review`, *When* it runs under `system` Enforce, *Then* the ungated item moves to `review`, the gated item stays with stuck reason `transition_denied`, and the transaction is per item (one denial cannot roll back the other). *And* for every moved item: the item's `Notes` still ends with the line "[auto] Transitioned to review: all work sessions ended." (as today, `storage_backlog.go:996-1001`), the status event is recorded once with `triggeredBy=system`, exactly one `ChangeStatusTransition` event is published (`OldStatus in_progress`, `NewStatus review`), and `ReconcileStuck` still returns the moved count. The old single-transaction atomicity (all items commit or none) becomes per-item atomicity; this is stated in the PR.
- *Given* the test files that call the six names (31 files: 199 selector calls in 29 files, plus the 4 fake method definitions in `session/jules_session_poller_test.go` and `server/services/jules_config_service_test.go` (the 29 files include `server/services/backlog_item_event_publisher_test.go`); recount with `git grep -l -E '(TransitionBacklogItemStatusWithPRFields|TransitionBacklogItemStatus|ArchiveBacklogItem|UnarchiveBacklogItem|SetBacklogItemPRAndTransition|ReconcileStuckItems)\(' -- '*_test.go'`), *When* the rename lands, *Then* `Storage` calls use `Raw*`, the 28 calls on `repo`/`er`/`fx.repo` receivers (7 files in package `session`) use the unexported names, and the one call on an `*EntRepository` from another package (`server/services/backlog_item_event_publisher_test.go:258`, `repoA.TransitionBacklogItemStatus`) is rewritten to go through a `Storage` built on `repoA` (it would no longer compile), the `julesPollerStorage` interface (`jules_session_poller.go:80-88`) and both fakes are re-pointed to `Raw*` (and later to `Gated*` in Task 1.5.7c), and no test semantics change.
- *Given* `tools/lint/silenttransition`, *When* Task 1.5.4f lands, *Then* the analyzer keeps its existing question (does the CALLER surface the denial: `return`, `notify`, `Publish`, or a `//nolint:silenttransition <reason>`) and is applied only where a caller must handle the returned error: (a) the `Raw*` names (no reporter exists on that path), and (b) `Gated*` calls whose `Caller` is an `operator`-class registry constant or is not a constant. It is NOT applied to `Gated*` calls of `agent`/`system` callers, because the gate itself calls the reporter and the Story 1.5.3 gate test is the guarantee; there is no "selector contains `Report`" rule. The operator caller-name set the analyzer uses is pinned to the registry by a test; fixtures: a log-only operator-class `Gated*` denial is flagged, the same call with `return` passes, a log-only agent-class `Gated*` call is not flagged, a log-only `Raw*` call is flagged; `make lint-custom` is green; the 8 `//nolint:silenttransition` annotations on status-write calls stay on their calls.
- *Given* the inventory test, *When* it walks every non-test `CreateBacklogItem` call, *Then* each sets no `Status`, sets `BacklogStatusIdea`, or is a `CreateBacklogItemForDebug` call on the allow-list with a reason; a new call that sets any other status fails naming file and function. There is deliberately no runtime rejection in `Storage.CreateBacklogItem`, because about 300 test fixtures in 93 test files create items at non-entry statuses (ADR-007 D1).
**Files**: `session/storage.go`, `session/storage_backlog.go`, `session/ent_repository_backlog.go`, `session/transition_gate.go`, `session/jules_session_poller.go`, `tools/lint/silenttransition/analyzer.go`, `tools/lint/silenttransition/analyzer_test.go`, `tools/lint/silenttransition/testdata/src/a/a.go`, `tools/lint/silenttransition/testdata/src/github.com/tstapler/stapler-squad/session/storage.go`, `server/mcp/tools_backlog.go`, `server/mcp/tools_backlog_pr.go`, `server/services/autonomous_orchestration_service.go`, `server/services/backlog_debug_mutate_handler.go`, `server/services/backlog_debug_seed_handler.go`, `server/services/backlog_service_lifecycle.go`, `server/services/backlog_service_sync.go`, `server/services/backlog_service_triage.go`, `server/services/backlog_service_trigger_triage.go`, `session/backlog_lifecycle.go`, `session/backlog_lifecycle_archive.go`, `session/backlog_lifecycle_pr.go`, `session/backlog_sync.go`, `server/services/jules_config_service_test.go`, `session/jules_session_poller_test.go`, `server/services/backlog_item_event_publisher_test.go`, `server/services/workflow_bypass_inventory_test.go`, and the remaining test files found by the recount command above

##### Task 1.5.4a: Rename the four `Storage` wrappers to `Raw*` and fix every production caller (M)
- Production files (recounted by `git grep -l`): `server/mcp/tools_backlog.go`, `server/mcp/tools_backlog_pr.go`, `server/services/autonomous_orchestration_service.go`, `server/services/backlog_debug_mutate_handler.go`, `server/services/backlog_service_lifecycle.go`, `server/services/backlog_service_sync.go`, `server/services/backlog_service_triage.go`, `server/services/backlog_service_trigger_triage.go`, `session/backlog_lifecycle.go`, `session/backlog_lifecycle_archive.go`, `session/backlog_lifecycle_pr.go`, `session/backlog_sync.go`, `session/jules_session_poller.go`, `session/storage.go` (the MCP delegate `tools_backlog_dependencies.go:163` calls the `BacklogService` handler, not `Storage`, and is not renamed). Production callers keep compiling via the `Raw*` names and are listed pending in the inventory until 1.5.5-1.5.7.
- Files: `session/storage.go`, `server/mcp/tools_backlog.go`, `server/mcp/tools_backlog_pr.go`, `server/services/autonomous_orchestration_service.go`, `server/services/backlog_debug_mutate_handler.go`, `server/services/backlog_service_lifecycle.go`, `server/services/backlog_service_sync.go`, `server/services/backlog_service_triage.go`, `server/services/backlog_service_trigger_triage.go`, `session/backlog_lifecycle.go`, `session/backlog_lifecycle_archive.go`, `session/backlog_lifecycle_pr.go`, `session/backlog_sync.go`, `session/jules_session_poller.go`

##### Task 1.5.4b: `GatedArchive`, `GatedUnarchive`, `GatedSetPRAndTransition` entry points (M)
- Thin wrappers over the decision function; `GatedSetPRAndTransition` keeps `PRReassignmentGuard` semantics (`session/storage.go:1164`) and runs the engine check for `review->pr_pending` before the single-UPDATE write. `GatedArchive` runs the caller's `BeforeCommit` (push work branches) after the allow and before the CAS; post-write worktree cleanup and link-session ending stay in the caller and run only after a successful write, never on a denial. `GatedArchive` on an already archived item decides on the system-required `archived->archived` edge and, in Off and Shadow, reproduces today's re-archive write.
- Files: `session/transition_gate.go`, `session/storage.go`, `session/transition_gate_test.go`

##### Task 1.5.4c: Split `ReconcileStuckItems` into find plus per-item gated transition (M)
- Repository keeps the existing query (`storage_backlog.go:967-980`) as exported read-only `FindStuckInProgressCandidates`; `GatedReconcileStuck` transitions each through the gate with CAS (`ExpectedStatus=in_progress`), `Precondition.Note` and `TriggeredBy=system`, then appends the "[auto]" line to `Notes` (a follow-up write after the CAS succeeds, or an `AppendNotes` option on the raw write in the same UPDATE, chosen here; the AC pins the result) and relies on the raw transition's own status event and `ChangeStatusTransition` publish, so an event is emitted exactly once per item; denied items go to the reporter. The caller `backlog_lifecycle.go:1255-1265` keeps its count/log behavior.
- Files: `session/storage_backlog.go`, `session/ent_repository_backlog.go`, `session/transition_gate.go`, `session/backlog_lifecycle.go`, `session/storage_backlog_test.go`, `session/ent_repository_backlog_test.go`

##### Task 1.5.4d: Unexport the five `EntRepository` writers, including the missed `TransitionBacklogItemStatusWithPRFields` (S)
- Rename to `rawTransitionBacklogItemStatus`, `rawArchiveBacklogItem`, `rawUnarchiveBacklogItem`, `rawTransitionBacklogItemStatusWithPRFields` (`ent_repository_backlog.go:1727,1457,1542,1951`) and update the `Storage` wrappers and `backlog_lifecycle.go:1256`; confirm by `go build ./...` that no other package referenced them; the registry and inventory gain `TransitionBacklogItemStatusWithPRFields` as a primitive (its one call is inside `GatedSetPRAndTransition`).
- Files: `session/ent_repository_backlog.go`, `session/storage.go`, `session/backlog_lifecycle.go`

##### Task 1.5.4e: Rename in tests and fakes; re-point `julesPollerStorage` (M)
- `ast-grep` rewrite across the 31 test files (command in the AC); `fx.repo.`/`er.` calls to the unexported names; the interface `julesPollerStorage` methods at `jules_session_poller.go:86-87` and the four fake methods (`fakeJulesPollerStorage` in both test files) are declarations, handled explicitly, not by the call-site rewrite; `jules_usage_counter_test.go` wraps a real `*session.Storage` and is covered by the rewrite.
- Also rewrite `server/services/backlog_item_event_publisher_test.go:258` (an `*EntRepository` receiver in another package) to call `Storage.RawTransitionBacklogItemStatus` over a `Storage` built on `repoA`.
- Mechanical-rename exception (see "Task sizing rule and exceptions"): the rewrite is one recorded `ast-grep` command, verified by `go vet ./...`; the file set is the output of the recount command in the AC (31 files at `b416f5576`, `git grep -l ... -- '*_test.go' | wc -l` prints 31). Story 1.5.4 is alone in W11, so a set that is not listed statically cannot collide with a same-wave story.
- Files: `session/jules_session_poller.go`, `session/jules_session_poller_test.go`, `server/services/jules_config_service_test.go`, `server/services/backlog_item_event_publisher_test.go`, and the 31 test files printed by the recount command

##### Task 1.5.4f: Extend the `silenttransition` analyzer to `Raw*` and to operator-class `Gated*` call sites (M)
- `tools/lint/silenttransition/analyzer.go:63` keys on method names that disappear after migration, and its question ("does the caller surface the denial?") has no answer for `agent`/`system` `Gated*` calls because the gate reports them itself (ARCH-C1; the earlier "selector containing `Report` counts as surfacing" rule would either false-positive about 40 migrated sites or be vacuous, and is dropped). Add the `Raw*` names to `targetMethods`; add `Gated*` names restricted to calls whose `TransitionRequest.Caller` (or the caller argument of the variant) resolves to an `operator`-class constant in a small name set, treating a non-constant `Caller` as in scope; a test (`TestSilentTransition_OperatorCallerSetMatchesRegistry`) fails when that set differs from the registry's `operator` entries; update `testdata/src/github.com/tstapler/stapler-squad/session/storage.go` and `testdata/src/a/a.go` (BAD: operator-class `Gated*` denial only logged, log-only `Raw*`; GOOD: `return`, `notify`, `//nolint:silenttransition <reason>`, and an agent-class `Gated*` call that only logs, which is intentionally not flagged); keep the old names until the last caller moves (removed with Task 1.5.9a); fix the `TransitionBacklogItemStatus` mentions in the analyzer doc and in `tools/lint/cmd/linter/main.go:14`. Acceptance: `go -C tools/lint test ./silenttransition/...` and `make lint-custom` green; the 8 annotations on status-write calls are re-read as each migration story moves the call.
- Files: `tools/lint/silenttransition/analyzer.go`, `tools/lint/silenttransition/analyzer_test.go`, `tools/lint/silenttransition/testdata/src/a/a.go`, `tools/lint/silenttransition/testdata/src/github.com/tstapler/stapler-squad/session/storage.go`, `tools/lint/cmd/linter/main.go`

##### Task 1.5.4g: Classify creation as an exempt `create` class; add `CreateBacklogItemForDebug` (S)
- Add `Storage.CreateBacklogItemForDebug` (any status, counted `bypass{caller=debug_create}`); the debug seed (`backlog_debug_seed_handler.go:100,149,227,572`) and mutate (`backlog_debug_mutate_handler.go:115`) handlers switch to it; extend the inventory test with the `create` rule (status absent, `BacklogStatusIdea`, or allow-listed debug call; the two non-literal callers `backlog_service_lifecycle.go:235` and `session/backlog_sync.go:353` are resolved through their same-function composite literal or carry an allow-list reason); every creator is listed as `create` or `exempt: debug` (ADR-007 D1). The `create` rule also covers the creator's own write: `ent_repository_backlog.go:452` (`SetStatus` in `CreateBacklogItem`) is an allow-list entry of the `.SetStatus(` rule in Task 1.5.9a, reason `create`, so the creator is the one status writer that rule permits outside the five transition writers. No runtime rejection (see the AC).
- Files: `session/storage.go`, `session/ent_repository_backlog.go`, `server/services/backlog_debug_seed_handler.go`, `server/services/backlog_debug_mutate_handler.go`, `server/services/workflow_bypass_inventory_test.go`

#### Story 1.5.5: Migrate operator-class and agent-class writers
**Prerequisites:** 1.5.4
**As an** operator, **I want** my UI actions and my agents' MCP actions to obey my stage configuration, **so that** editing the Stages page changes what they can do.
**Acceptance Criteria**:
- *Given* `Enforce` for `operator`, *When* the handler RPC is called after an edge edit, *Then* results equal Story 1.2.4's expectations and the error codes are unchanged (`InvalidArgument` for graph, `FailedPrecondition` for guards, `Aborted` for `ErrPreconditionFailed`); the handler's `CanTransition`, `ValidateGates` and storage calls are replaced by one `GatedTransition` call with `BeforeCommit = commitAndPushItemWorktrees` and the whole `ExpectedStatus`/`ExpectedUpdatedAt` precondition built from the request.
- *Given* `Enforce` for `agent` and a `human_approval` gate on `in_progress->review`, *When* MCP `request_review` runs, *Then* it returns a structured error naming the gate with the hint "ask the operator to approve this transition" and does not retry; a `transition_denied` stuck reason exists; satisfying the gate lets the next call succeed. (The edge itself is system-required and cannot be disabled; the gate is the lever.)
- *Given* `Enforce` for `agent` and `done->refining` disabled, *When* MCP `transition_backlog_item_status` runs, *Then* it is denied: the generic delegate is `forwarding` and exempt from system-required edges (ADR-007 D4), so `done->refining` is a non-system edge that stays disableable and the delegate reaches the handler with actor `agent`, which the handler resolves to caller `mcp_transition_delegate` (class `agent`) per Task 1.5.5a.
- *Given* `OverrideVerdict`, `SubmitManualReview`, `ArchiveBacklogItem`, `UnarchiveBacklogItem` and `UpdateBacklogItem` (PR association) RPC writes, *When* each runs under `Off` or `Shadow`, *Then* the result equals today's (regression-pinned), and under `Enforce` a denial returns the error to the caller. (`TriggerTriage` and `forceResetItem` are migrated in Story 1.5.6 because they live in files that story owns.)
- *Given* `reportDuplicate` and the Archive RPC (archive from any status) with the archive edges system-required, *When* run under `Enforce` for their class against a live database whose seeded graph lacks `in_progress->archived`, *Then* archiving an `in_progress`, `review`, `pr_pending` and `done` item is allowed, an already archived item (`archived->archived`) is idempotently allowed, and an operator `human_approval` gate on `done->archived` (the edge cannot be disabled, only gated) denies both with the structured error; `validTransitions` is unchanged.
- *Given* the Archive RPC through `GatedArchive`, *When* the gate allows, *Then* work branches are pushed before the write (`BeforeCommit`), and worktrees are cleaned up and `reportDuplicate`'s own link sessions are ended only after the write succeeds; *When* the gate denies, *Then* none of those side effects runs and no status changes (new behavior: archive can now be denied).
**Files**: `server/services/backlog_service_lifecycle.go`, `server/services/backlog_service_gates.go`, `server/mcp/tools_backlog.go`, `server/mcp/tools_backlog_pr.go`, `server/mcp/tools_backlog_dependencies.go`

##### Task 1.5.5a: Handler, `OverrideVerdict`, `SubmitManualReview` onto the gate (M)
- Two commits: (1) Extract the guard-input assembly (`backlog_service_lifecycle.go:897-932` with `buildPendingGatesGuardInput`, Task 1.5.5e, now the first commit) with no behavior change; (2) replace the three engine/storage calls with one `GatedTransition` call. `make ready-complexity-gate` on both commits; do not grow the function.
- **Acceptance criteria for how a shared-handler request becomes class `agent` (ADV-N2; VERIFIED: the MCP generic delegate `server/mcp/tools_backlog_dependencies.go:163` calls the same `BacklogService.TransitionBacklogItemStatus` handler as the UI, with `services.WithTransitionActor(ctx, session.TriggeredByAgent)`; the handler reads `transitionActorFromContext(ctx)` at `backlog_service_lifecycle.go:874`; the context key is an unexported type, so only in-process code can set it, never a request field)**:
  - *Given* the handler is the only `GatedTransition` call site for this RPC, *When* the gate call is built, *Then* the caller is resolved from the context actor: actor `agent` resolves to the registered `mcp_transition_delegate` (class `agent`, `forwarding`), the default actor `user` (`session.TriggeredByUser`, what `transitionActorFromContext` returns when unset) resolves to `operator_transition_rpc` (class `operator`); both names appear as literals in the handler's branches so the registry audit sees them, and any other actor value fails closed to `mcp_transition_delegate` (the class that ignores `override_reason` is the stricter one, so an unknown actor never gains the operator override; a test pins this).
  - *Given* the rule "every registry entry has a call site" (Task 1.5.9a rule (c)), *When* it runs, *Then* `forwarding` entries are exempt from it, because the delegate itself makes no `Gated*` call (it is an allow-listed passthrough); its coverage is instead `TestTransitionHandler_ResolvesCallerClassFromContextActor` (agent actor gives `mcp_transition_delegate`/`agent`, absent or `user` actor gives `operator_transition_rpc`/`operator`, an unknown actor value gives `mcp_transition_delegate`/`agent`) and the existing `TestMcpTransitionDelegate_UsesAgentClassAndGraph_WhenActorAgentAndEdgeDisabled`, which fails if an implementer routes agents through the operator caller (agents would then ride the operator flag and ignore `-enforce-agent`).
  - *Given* the delegate request with `override_reason` omitted by design (`tools_backlog_dependencies.go:143-146`), *When* class `agent` applies, *Then* any `override_reason` is ignored (ADR-007 D3) and an `agent` denial also reaches the reporter.
- Files: `server/services/backlog_service_lifecycle.go`, `server/services/backlog_service_gates.go`

##### Task 1.5.5b: Other operator RPC writers in the lifecycle file (S)
- `ArchiveBacklogItem` `:516`, `UnarchiveBacklogItem` `:547`, `UpdateBacklogItem` PR association `:470`. (`TriggerTriage` and `forceResetItem` moved to Task 1.5.6e.)
- Files: `server/services/backlog_service_lifecycle.go`

##### Task 1.5.5c: MCP agent writers (M)
- `requestReview` (2 sites), `reportBlocked`, `resumeWork`, `reportDuplicate`, `reportPRCreated`; the delegate already passes actor `agent` and is registered `forwarding`; structured denial result with `blocked_by`, `hint`, and no automatic retry.
- Files: `server/mcp/tools_backlog.go`, `server/mcp/tools_backlog_pr.go`, `server/mcp/tools_backlog_dependencies.go`

##### Task 1.5.5d: Per-entry-point denial and regression tests (M)
- For each migrated entry point: Off/Shadow equals today; Enforce denial by graph edit and by gate; concurrent-mover case for the nil-precondition sites; error-code mapping; archive side-effect ordering; stale-seed archive case; `archived->archived`.
- Files: `server/services/backlog_gate_operator_test.go`, `server/mcp/tools_backlog_gate_test.go`

##### Task 1.5.5e (first commit of the story; formerly Task 1.2.5a): Merge guard-input assembly duplicates (S)
- Extract `backlog_service_lifecycle.go:897-932` and `buildPendingGatesGuardInput` (`backlog_service_gates.go:148-170`) into one helper; separate commit with no behavior change, before the handler is routed through the gate in Task 1.5.5a (it was optional before the owner decision; it is now the Extract-first step). Listed last only for ID stability; execute first.
- Files: `server/services/backlog_service_lifecycle.go`, `server/services/backlog_service_gates.go`

#### Story 1.5.6: Migrate system writers in `server/services` (and the two operator writers in the triage files)
**Prerequisites:** 1.5.4
**As an** operator, **I want** automation inside the service layer to obey my gates, **so that** a `human_approval` gate stops auto-completion, auto-reopen and respawn too.
**Ownership note**: this story alone owns `backlog_service_triage.go` and `backlog_service_trigger_triage.go` in its wave (Story 1.5.5 no longer lists them), so W12 is file-disjoint by the script check in "Waves". `forceResetItem` (`triage.go:1324`) and the `TriggerTriage` RPC writes (`trigger_triage.go:290,772`) are operator-class entries (classification confirmed in Task 1.5.1a) migrated here.
**Acceptance Criteria**:
- *Given* `onAutonomousDriverComplete` (`autonomous_orchestration_service.go`), whose only status write is the triage-role `idea->ready` with `ExpectedStatus=idea` (VERIFIED: `toStatus` is assigned once, `:453`; the work role does not transition, `:455-499`; the review role only resolves a stuck row or ends the `ItemSession`, `:505-575`), *When* run under `Off` or `Shadow`, *Then* the result is today's; under `Enforce` for `system` with an operator gate on `idea->ready`, *Then* the transition is denied, `statusTransitionErr` carries the denial (the existing "status update failed" notification fires once), the item gets `transition_denied` and stays `idea`, and no triage session is respawned for it (Story 1.5.8 AC 1). The work and review roles are pinned by a test to perform no status write, and the dead `toStatus == Review` branch (`:604`) and the stale comment that says the work role "still transitions in_progress->review" (`:596-598`) are removed.
- *Given* the triage-pipeline writers (`spawnSessionAfterGates` `:1268`, `AutoReopenAfterFailedReview` `:2041`, `autoReopenForPRFix` `:2418`, `respawnTriage` `:2637`, `MaybeTriggerTriage` `:2906`, `TriggerReReview` `:3348`, trigger-triage `:772`), *When* each runs under `Off` or `Shadow`, *Then* it behaves as today; under `Enforce` with a blocking gate its denial is reported, not swallowed, and its existing error path still runs. `MaybeTriggerTriage` is a creation-time one-shot (five callers: `tools_backlog.go:2251,2329`, `tools_backlog_import_tracker.go:50`, `backlog_service_lifecycle.go:240`, `backlog_service_sync.go:330`): a denied skip-rule `idea->ready` is reported once under caller `maybe_trigger_triage` (a different dedupe key from `autonomous_complete`'s denial of the same edge), returns false, and does not start triage.
- *Given* `forceResetItem` (`Unconditional`) and the `TriggerTriage` RPC writes under `Off` or `Shadow`, *Then* results equal today's; under `Enforce` a denial returns the error to the operator.
- *Given* the three rollback writes (`:881,:2115,:2471`) and the debug handler (`:172,:253`), *When* they run, *Then* they use `Rollback` or `DebugForce`, are never engine-denied, and are counted in `workflow_engine_bypass_total{caller}` with class `rollback` or `debug`.
- *Given* `AttachSessionToItem` and `transitionWithGuard` callers (queue, dequeue claim, Jules dispatch), *When* re-pointed at the gate with class `system` and `guards=BuiltIn`, *Then* Off/Shadow results equal today's and the Story 1.2.2 `claimAndSpawnCandidate` behavior is unchanged (regression-pinned).
- **Coordination risk and sequencing rule (triad round 6)**: this story rewrites about 10 sites and extracts three functions inside `backlog_service_triage.go`, a 3,300-line hotspot that other backlog work also edits. The engineering triad review reported an open backlog item "Investigate false triage artifact does not exist error" that concerns triage and may touch `backlog_service_triage*.go`; its state and file overlap are UNVERIFIED here because the live backlog MCP was unreachable (ECONNREFUSED) during review. *Given* W12 is about to be dispatched, *When* the dispatcher runs the Worker brief's overlap check (Task 1.5.6h), *Then* if that item, or any other open item or PR, edits `backlog_service_triage*.go`, `backlog_service_trigger_triage.go` or `autonomous_orchestration_service.go`, Story 1.5.6 is NOT run concurrently with it: it waits for that work to merge, merges `main`, re-runs the three extract-first complexity gates, and the result is recorded in the PR. The same rule covers Stories 1.2.2 (W6), 1.5.4 (W11) and 1.5.8 (W14).
**Files**: `server/services/autonomous_orchestration_service.go`, `server/services/backlog_service_triage.go`, `server/services/backlog_service_trigger_triage.go`, `server/services/backlog_service_sync.go`, `server/services/backlog_debug_mutate_handler.go`

##### Task 1.5.6a: Autonomous completion: the real mapping (S)
- Replace the single direct call (`:586`) with `GatedTransition` keeping the `ExpectedStatus=idea` precondition; table test over all roles and outcomes asserting exactly one status write (triage `idea->ready`) and none for work or review; delete the dead `toStatus == Review` block and the stale comment; the `silenttransition` suppression reason stays valid (the denial is carried by `statusTransitionErr`).
- Files: `server/services/autonomous_orchestration_service.go`, `server/services/autonomous_orchestration_service_test.go`

##### Task 1.5.6b: Extract-first `AutoReopenAfterFailedReview`, then route its write (S)
- Commit 1: Extract Function from `AutoReopenAfterFailedReview` (`backlog_service_triage.go:1887`) with no behavior change; commit 2: replace the `:2041` write with `GatedTransition`. `make ready-complexity-gate` on both commits.
- Files: `server/services/backlog_service_triage.go`

##### Task 1.5.6c: Extract-first `autoReopenForPRFix`, then route its write and rollback (S)
- Commit 1: Extract Function from `autoReopenForPRFix` (`:2357`); commit 2: replace the `:2418` write and move the `:2471` rollback to `gate.Rollback`. Complexity gate on both commits.
- Files: `server/services/backlog_service_triage.go`

##### Task 1.5.6d: Extract-first `MaybeTriggerTriage`, then route it and the other triage writers (M)
- Commit 1: Extract Function from `MaybeTriggerTriage` (`:2895`, skip-rule block `:2897-2912`) with no behavior change; commit 2: route its `:2906` write, `respawnTriage` `:2637`, `spawnSessionAfterGates` `:1268`, `TriggerReReview` `:3348`; the rollbacks `:881,:2115` to `gate.Rollback`. `make ready-complexity-gate` per commit; any other modified function that fails the gate is extracted too.
- Files: `server/services/backlog_service_triage.go`

##### Task 1.5.6e: `TriggerTriage` RPC writes and `forceResetItem` (S)
- `trigger_triage.go:290,772` and `triage.go:1324` (marked `Unconditional`); moved from Task 1.5.5b so one story owns each file in the wave.
- Files: `server/services/backlog_service_trigger_triage.go`, `server/services/backlog_service_triage.go`

##### Task 1.5.6f: `AttachSessionToItem`, `transitionWithGuard` re-point and debug handler (S)
- `transitionWithGuard` becomes a thin call to `GatedTransition` with the caller-supplied guard input; debug handler to `DebugForce`.
- Files: `server/services/backlog_service_sync.go`, `server/services/backlog_service_triage.go`, `server/services/backlog_debug_mutate_handler.go`

##### Task 1.5.6h: Pre-dispatch overlap check for the triage files (XS)
- A gate like Task 1.5.3e, not an aside: before W12 (and before W6, W11, W14 for the stories that edit the same files) the dispatcher runs the Worker brief's overlap check, appends a dated "Overlap check" line (items and PRs found, decision: run, or wait for merge) under the plan's Repair log, and does not dispatch the story while an overlapping edit is in flight.
- Files: `project_plans/backlog-make-it-true/implementation/plan.md`

##### Task 1.5.6g: Concurrent-mover and denial tests for the nil-precondition sites (S)
- `spawnSessionAfterGates`, `forceResetItem` (`Unconditional`), `Attach`, rollbacks, the autonomous mapping table.
- Files: `server/services/backlog_gate_system_test.go`

#### Story 1.5.7: Migrate system writers in `session`
**Prerequisites:** 1.5.4
**As an** operator, **I want** the lifecycle listener, PR reconciler, archiver, Jules poller and `SyncLoop` to obey my configuration, **so that** no automatic move ignores a gate or stage I edited.
**Acceptance Criteria**:
- *Given* `onSessionExited` (`backlog_lifecycle.go:1004`), `transitionBouncingItemToDone` (`:1866,:1873`), `ReconcileStuck` (`:1255`, via `GatedReconcileStuck`), `archiveStaleDoneItems` (`backlog_lifecycle_archive.go:87`), the PR paths (`backlog_lifecycle_pr.go:308,601,819,848,1531,2183`), the Jules poller (`jules_session_poller.go:432,518,533,540`) and `SyncLoop` (`backlog_sync.go:450`), *When* run under `Off` or `Shadow`, *Then* each behaves exactly as today (existing tests pass unchanged).
- *Given* `Enforce` for `system` and an operator `human_approval` gate on `pr_pending->done`, *When* `reconcilePRPendingItem` sees the PR merged, *Then* the transition is denied, the item stays `pr_pending` with `transition_denied` (one comment, one notification), the PR-merge reconcile keeps polling without error spam, and after the gate is satisfied the next tick moves it to `done`.
- *Given* `Enforce` for `system` and an operator `human_approval` gate on `done->archived` (the edge is system-required through `archiveStaleDoneItems` and cannot be disabled, only gated), *When* `archiveStaleDoneItems` runs, *Then* items stay `done`, one report per item is created, and the sweep does not log a Warn per tick (dedupe).
- *Given* the `session` package cannot import `server/services`, *When* these callers are denied, *Then* they reach the reporter through the injected `TransitionDeniedReporter` (Task 1.5.3b), asserted with a fake reporter in `session` tests.
- *Given* the nil-precondition sites (`backlog_lifecycle_pr.go:601`, `SyncLoop` `:450`), *When* a concurrent mover changes the status, *Then* the result is a skip with a Debug log and a retry on the next tick, not an error log.
- *Given* the Jules poller, *When* Task 1.5.7c lands, *Then* `julesPollerStorage` lists `GatedTransition` and `GatedSetPRAndTransition` (the `Raw*` lines at `:86-87` are gone) and both `fakeJulesPollerStorage` types implement the new names.
**Files**: `session/backlog_lifecycle.go`, `session/backlog_lifecycle_archive.go`, `session/backlog_lifecycle_pr.go`, `session/backlog_sync.go`, `session/jules_session_poller.go`, `session/jules_session_poller_test.go`, `server/services/jules_config_service_test.go`, `session/backlog_lifecycle_test.go`, `session/backlog_lifecycle_archive_test.go`, `session/backlog_lifecycle_pr_test.go`, `session/backlog_sync_test.go`

##### Task 1.5.7a: Lifecycle listener, `ReconcileStuck` call and archive sweep (M)
- Files: `session/backlog_lifecycle.go`, `session/backlog_lifecycle_archive.go`, `session/backlog_lifecycle_test.go`, `session/backlog_lifecycle_archive_test.go`

##### Task 1.5.7b: PR paths including `SetBacklogItemPRAndTransition` callers (M)
- Files: `session/backlog_lifecycle_pr.go`, `session/backlog_lifecycle_pr_test.go`

##### Task 1.5.7c: Jules poller, its interface and fakes, and `SyncLoop` (S)
- Files: `session/jules_session_poller.go`, `session/jules_session_poller_test.go`, `server/services/jules_config_service_test.go`, `session/backlog_sync.go`, `session/backlog_sync_test.go`

#### Story 1.5.8: Denied automatic transitions neither loop nor strand
**Prerequisites:** 1.5.7, 1.5.6
**As an** operator, **I want** an automation step that is waiting on my gate to sit quietly and finish by itself once I approve, **so that** enforcement does not create respawn churn or a stuck item that needs manual rescue.
**The real stranding path (VERIFIED, replaces the earlier role-mapping assumption)**: `onAutonomousDriverComplete` writes exactly one status, triage-role `idea->ready`. If an operator gate denies it, the triage `ItemSession` has already ended and the item stays `idea`; `reconcileOrphanedTriageItems` shape 2 ("ended, item still in idea", `session/backlog_lifecycle_triage.go:374`) then marks `orphaned_triage`, `reconcileOrphanedTriageRemediation` (`:445`) retries it through `AutoRespawnTriage` to `respawnTriage` (`backlog_service_triage.go:2608`) on every backoff tick, and the graceful-shutdown branch (`:352-366`) respawns with no backoff. `RemediationBlocked(item, reason)` (`backlog_remediation.go:332`) gates only the caller's own reason, so an open `transition_denied` row stops none of these on its own. The same hazard applies to `ReconcileStuck` `in_progress->review`, the PR paths and archive.

**Shared predicate**: `Storage.HasOpenTransitionDenied(ctx, itemID, status)` (ADR-007 D12): true iff an open `transition_denied` row exists whose recorded `from` equals the item's current status.

**Dispatch check, done first (Task 1.5.8a; result recorded here, VERIFIED by reading the code in Repair round 5)**: there is NO single item-level dispatch that hosts the guard once.
- `runStuckDetector` (`session/backlog_lifecycle.go:1236`) takes only `(name, okNames, panickedNames, fn func())`: a per-detector panic-recover and name-bookkeeping wrapper with no item in scope. `git grep -c 'l.runStuckDetector("' session/backlog_lifecycle.go` returns 24 detector names (the architecture review's "25" was one high). Each detector iterates its own items inside `fn`.
- `remediationActionByReason` (`server/services/backlog_service_stuck.go:353`) is only the operator "Retry now" map (`TriggerRemediationNow`, `:389`) from 7 stuck reasons to 6 `BacklogService` methods (`AutoReopenAfterFailedReview`, `AutoRespawnReview`, `AutoRespawnAutonomousWork`, `RemediateStaleWorkSession`, `AutoRespawnTriage`, `AutoReopenForPRFix`).
- BUT the periodic sweep reaches the SAME six methods through interfaces implemented by the one `*BacklogService`, wired at `server/dependencies.go:1524` (`SetReviewRespawner`), `:1551` (`SetTriageRespawner`), `:1572` (`SetStaleWorkRemediator`), `:1605` (`SetAutonomousStuckRespawner`) and the `AutoReopenSpawner` parameter of `server/mcp/server.go:42`. So the common layer is the **spawn point**, not the detector. **Consequence (collapse)**: the guard lives once inside each spawn-point method below, and that single call covers the periodic path and the "Retry now" path together; `TriggerRemediationNow` maps the guard's sentinel to `FailedPrecondition` in one place. Per-detector guards are written only for detectors that act WITHOUT going through a spawn point (the inline `FindReviewItemsWithoutGate` respawn, which calls the listener-local `spawnReviewGate`, `backlog_lifecycle.go:1272-1306`, VERIFIED; and the `orphaned_triage` shape-2 marker, which would otherwise label a gate-held item `orphaned_triage`). Detection-only detectors (they only `MarkStuck`/notify for their own reason, idempotent per row) are not guarded; their AC is "no respawn, one row, one notification".

**Detector and spawn-point table** (all 24 `runStuckDetector` names, the 7 `remediationActionByReason` reasons, the service-level respawners and the inline respawn; "VERIFIED" = read in code, "confirm" = Task 1.5.8c reads the body or `Find*` query and records the result in the test table). Guard kinds: **S** = guard inside a spawn-point method (one call covers every caller); **D** = detector-local guard; **W** = the detector is itself the status writer (a denial goes through the gate and reporter; the sweep retries each tick without a Warn); **R** = resolve-only, writes no status and spawns nothing; **DO** = detection-only (`MarkStuck`/notify, idempotent); **N** = not applicable.

| Detector / respawner | Where | Status scope | Kind and guard |
|---|---|---|---|
| spawn point `respawnTriage` via `AutoRespawnTriage` / `ResumeTriage`; reached by `orphaned_triage_remediation` (`backlog_lifecycle_triage.go:445`), the graceful-shutdown immediate goroutine (`:352-366`) and operator "Retry now" for `orphaned_triage` (`backlog_service_stuck.go:363`) | `backlog_service_triage.go:2575,2608` | idea, queued (VERIFIED) | **S**: skip when the item has an open denial from its status; "Retry now" returns `FailedPrecondition` naming the denial |
| `orphaned_triage` shapes 1-3 (marker) | `backlog_lifecycle_triage.go:264,374` | idea, queued (VERIFIED) | **D** for shape 2 only (the stranding path: do not mark `orphaned_triage` while a denial from `idea` is open); shapes 1 and 3 are **DO** |
| spawn point `MaybeTriggerTriage` (creation-time one-shot, five callers) | `backlog_service_triage.go:2895` | idea | **S**; its own denied skip-rule `idea->ready` is reported once under caller `maybe_trigger_triage`, returns false and starts no triage; the test asserts exactly one report for that caller and no `TriggerTriage` call (a different dedupe key from `autonomous_complete`'s denial of the same edge) |
| `plan_not_approved` | `backlog_lifecycle.go:1445` | queued (VERIFIED) | **DO** |
| spawn point `RemediateStaleWorkSession` (`stale_work`; the `ReconcileStuck` `in_progress->review` case) | `backlog_service_triage.go:2196`, `backlog_lifecycle_stale.go:64` | in_progress (VERIFIED) | **S** |
| `rework_blocked_stale` | `backlog_lifecycle.go:1345`, `backlog_lifecycle_stale.go:205` | review | **R** (VERIFIED: the call-site comment says "Resolve-only pass") |
| `respawn_blocked_active` | `backlog_lifecycle.go:1354`, `backlog_lifecycle_review.go:801` | review | **R** (VERIFIED: "Resolve-only pass") |
| spawn point `AutoRespawnAutonomousWork` (`autonomous_stuck`) | `backlog_service_triage.go:2128` | in_progress | **S** |
| spawn point `AutoReopenAfterFailedReview` (`bouncing`) | `backlog_service_triage.go:1887`, `backlog_lifecycle.go:1388` | in_progress, review (VERIFIED) | **S** |
| `repeated_noop_dispatch` | `backlog_lifecycle_noop.go:35` | in_progress, review (VERIFIED) | **DO** (it flags so the dispatcher gate stops respawning) |
| spawn point `AutoRespawnReview` (`abandoned_review`) | `backlog_service_triage.go:2488`, `backlog_lifecycle_review.go:397` | review (FindStuckReviewItems, FindZombieReviewItems) | **S** |
| `unprocessed_review_verdict` | `backlog_lifecycle_review.go:514` | review (FindReviewItemsWithUnprocessedVerdict) | confirm: **S** if it calls `AutoReopenAfterFailedReview`, otherwise **W** (Task 1.5.8c) |
| inline `FindReviewItemsWithoutGate` respawn inside `ReconcileStuck` | `backlog_lifecycle.go:1272-1306` | review (VERIFIED by the query comment and the body) | **D**: skip when an open denial from `review` exists; it calls the listener-local `spawnReviewGate`, so no spawn-point guard covers it |
| `orphaned_agent_pr` | `backlog_lifecycle_pr.go:253` | review | confirm: **W** (reports the PR through `SetBacklogItemPRAndTransition`) (Task 1.5.8c) |
| `push_failed` | `backlog_lifecycle_pr.go:935` | review | confirm: **W** (retries `pushAndCreatePR`) (Task 1.5.8c) |
| spawn point `AutoReopenForPRFix` and `AutoReopenForPRFixWithKnownSession` (`pr_pending_no_pr`, `pr_needs_fix`) | `backlog_lifecycle_pr.go:25-39,1333-1402`, `backlog_service_stuck.go:365-371` | review, pr_pending (confirm the scope) | **S** |
| `pr_drift_recovery` (drifted-PR reconcile) | `backlog_lifecycle.go:1487`, `backlog_lifecycle_pr.go:912,840` | drifted items to `pr_pending` | **W** (VERIFIED: `recoverDriftedPRItem` writes `->pr_pending`, `:819,848`): denial reported once, retried each tick without a Warn |
| `pr_ready+merge_detection` (`ReconcilePRPending`, the `pr_pending->done` writer) | `backlog_lifecycle.go:1504`, `backlog_lifecycle_pr.go:1775` | pr_pending | **W**, grouped with `auto_archive_done` as "is itself the writer" (VERIFIED: it writes `done` at `:1531,:2183`) |
| `auto_archive_done` | `backlog_lifecycle_archive.go`, `backlog_lifecycle.go:1469` | done | **W**: denial reported once, retried each tick without a Warn |
| `dequeue_backlog_items` calling `DequeueNextQueuedItems` | `backlog_lifecycle.go:1523`, `backlog_service_triage.go:764,873` | queued, ready | **S** (spawn point): before a claim, skip an item with an open denial from its status; the claim-denied `claim failed` Warn (logged on every tick, then `continue`) is demoted to Debug once the denial is reported, so a held claim neither blocks the queue head nor repeats a Warn |
| `blocked_by_dependency` | `backlog_lifecycle.go:1514`, `backlog_lifecycle_stale.go:239` | queued, ready | **R** (VERIFIED: the call-site comment calls it the "Resolve-only counterpart to notifyBlockedByDependency") |
| `trigger_chain_reconcile` | `backlog_lifecycle.go:1530,510`, `session/chain_firer.go` | done (chained fires) | **N**: it fires a chained session and writes no status (VERIFIED: `git grep` finds no status writer in `chain_firer.go`, only comments); Task 1.5.8d re-confirms and registers the caller if one appears |
| `gate_timeout`, `self_heal`, `multi_reason_escalation`, `archive_terminal_sessions`, `work_commit_refresh` | `backlog_lifecycle.go:1330,1437,1451,1461,1477` | n/a | **N**, each with an explicit "not applicable: <reason>" entry; `multi_reason_escalation` is tested to treat a lone `transition_denied` row as non-escalating |

**Acceptance Criteria**:
- *Given* an item with an open `transition_denied` row whose `from` equals its current status, *When* each **S**, **D** and **W** row above runs, *Then* none respawns a session, nudges or starts triage, no **D** row marks its own reason, and **DO** rows create at most one stuck row and one notification across many ticks; *Given* the row is absent, *Then* the code behaves as today (regression); *Given* the row's `from` differs from the item's current status (stale row), *Then* it proceeds. One required test per **S**, **D** and **W** row (parameterized over the production function with a fake-backed harness), and a completeness test fails when any `runStuckDetector` name or `remediationActionByReason` reason is in none of the kinds above. The test also pins that every **S** guard is the SAME predicate call (a spy on `HasOpenTransitionDenied` records the caller), so the collapse cannot regress into per-detector copies.
- *Given* autonomous triage completion denied on `idea->ready` (operator gate), *When* the operator satisfies the gate, *Then* the re-drive sweep moves the item to `ready` through the gate without a new triage session, the `transition_denied` row clears, `onAutonomousDriverComplete` is not re-invoked and `orphaned_triage` is never marked; with the denial persisting across many ticks, exactly one comment and one notification exist.
- *Given* `ReconcileStuck` `in_progress->review` denied by an operator gate while the item's work sessions have ended, *When* `stale_work`, `bouncing` and `autonomous_stuck` reach their spawn points, *Then* none respawns a work session while the only blocker is the gate; after approval the re-drive moves the item to `review`.
- *Given* a recorded denial `(item, caller, from, to, reason)`, *When* the reconcile tick runs and the gate has been satisfied or the edge re-enabled, *Then* the sweep re-attempts the transition through the gate once, it succeeds, the stuck reason clears and no second notification is sent.
- *Given* the denial persists, *When* many ticks run, *Then* exactly one comment and one notification exist (idempotence) and the sweep's cost is one indexed query per tick.
- *Given* a held `queued->in_progress` claim, *When* `DequeueNextQueuedItems` runs many ticks, *Then* the claim is skipped before it is attempted, no `claim failed` Warn repeats, and the next queued item is still claimed (no head-of-line blocking).
- *Given* the operator deletes the target stage under a stuck item, *When* the sweep runs, *Then* the stuck item is re-reported with reason `target_stage_removed` (not silently dropped) and `override_reason` remains the documented exit.
- *Given* Task 1.5.8g, *When* the process starts with the guards and the sweep installed, *Then* `backlog:workflow-engine-enforce-system` is registered (default off) with its `ConfigSurfaceProof` entry and `class_modes` can report `system` as `enforce`; before that task the flag does not exist (ADR-007 D8), so there is no runtime degrade state, no `RecoveryReady()` and no `recovery_wired` field.
**Files**: `session/backlog_remediation.go`, `session/backlog_lifecycle.go`, `session/backlog_lifecycle_triage.go`, `session/backlog_lifecycle_stale.go`, `session/backlog_lifecycle_review.go`, `session/backlog_lifecycle_pr.go`, `session/backlog_lifecycle_noop.go`, `session/backlog_lifecycle_archive.go`, `server/services/backlog_service_triage.go`, `server/services/backlog_service_stuck.go`, `server/services/backlog_transition_notify.go`, `server/services/backlog_transition_denied_recovery_test.go`, `session/backlog_denied_guard_test.go`, `server/services/feature_flag_service.go`, `server/services/feature_flag_service_test.go`, `config/config.go`, `server/services/config_surface_proof_test.go`

##### Task 1.5.8a: Dispatch check, `HasOpenTransitionDenied` and the completeness test (M)
- First step: confirm in code the dispatch finding above (no item-level choke in `runStuckDetector`; the six spawn-point methods are the common layer) and record it in the PR; if a shared item-level dispatch has appeared since, collapse further and say so. Then the predicate over the structured `caller`/`from`/`to`/`denied_reason` (Task 1.2.2a) with one indexed query, a typed skip sentinel that spawn points return and periodic callers treat as a quiet skip, and the completeness test over `runStuckDetector` names and `remediationActionByReason` reasons with the kinds above.
- Files: `session/backlog_remediation.go`, `session/backlog_denied_guard_test.go`, `session/backlog_lifecycle.go`

##### Task 1.5.8b: Guard the spawn points once each (M)
- `respawnTriage`/`AutoRespawnTriage`/`ResumeTriage`, `MaybeTriggerTriage`, `RemediateStaleWorkSession`, `AutoRespawnAutonomousWork`, `AutoReopenAfterFailedReview`, `AutoRespawnReview`, `AutoReopenForPRFix` and `AutoReopenForPRFixWithKnownSession`, and the `DequeueNextQueuedItems` pre-claim skip with the demoted Warn; `TriggerRemediationNow` maps the guard's sentinel to `FailedPrecondition` in one place; one test per spawn point (three cases each: open denial skips, no row behaves as today, stale row proceeds). Extract-first where a modified function fails the complexity gate.
- Files: `server/services/backlog_service_triage.go`, `server/services/backlog_service_stuck.go`, `session/backlog_lifecycle_pr.go`

##### Task 1.5.8c: Detector-local guards and the "confirm" rows (M)
- The inline `FindReviewItemsWithoutGate` respawn and the `orphaned_triage` shape-2 marker; read and classify `unprocessed_review_verdict`, `orphaned_agent_pr` and `push_failed` (each becomes **S**, **D** or **W**, recorded in the test table with its `Find*` scope); one test per row.
- Files: `session/backlog_lifecycle.go`, `session/backlog_lifecycle_triage.go`, `session/backlog_lifecycle_review.go`, `session/backlog_lifecycle_pr.go`, `session/backlog_lifecycle_noop.go`

##### Task 1.5.8d: Writer rows and the explicit "not applicable" entries (M)
- `auto_archive_done`, `pr_ready+merge_detection`, `pr_drift_recovery` (denial reported once through the injected reporter, retried each tick, no Warn per tick); the resolve-only rows `rework_blocked_stale`, `respawn_blocked_active`, `blocked_by_dependency` and the **N** entries (`trigger_chain_reconcile` re-confirmed against `chain_firer.go`), each with its reason; the completeness test lists them.
- Files: `session/backlog_lifecycle_archive.go`, `session/backlog_lifecycle_pr.go`, `session/backlog_lifecycle_stale.go`, `session/backlog_lifecycle.go`

##### Task 1.5.8e: Re-drive sweep for recorded denials (S)
- Reads the structured `caller`/`from`/`to`/`denied_reason` (added in Task 1.2.2a, no proto change here); re-attempts through the gate; clears on success; `target_stage_removed` re-report.
- Files: `session/backlog_lifecycle.go`, `server/services/backlog_transition_notify.go`

##### Task 1.5.8f: End-to-end regression tests: autonomous triage completion, `ReconcileStuck`, PR merge, archive, held dequeue claim (M)
- Files: `server/services/backlog_transition_denied_recovery_test.go`, `session/backlog_lifecycle_pr_test.go`

##### Task 1.5.8g: Register the `-enforce-system` flag after the recovery guards (S)
- Last task of the story (depends on 1.5.8a-f): add `backlog:workflow-engine-enforce-system` to `knownFeatureFlags` (default declared once in `config`, off), the system row of the class-to-flag table, its `ConfigSurfaceProof` entry (deleting its pending line), the final `TestModeFor_ComposesFourFlagsPerClass_AndDefaultsToShadow` rows, and the runbook dependency statement for Story 1.4.1 (the system-class step requires the Story 1.5.8 completeness test green). Replaces the earlier `RecoveryReady()` degrade clause (ARCH-C3).
- Files: `server/services/feature_flag_service.go`, `server/services/feature_flag_service_test.go`, `config/config.go`, `server/services/config_surface_proof_test.go`

#### Story 1.5.9: Strict guard and Metric 1 proofs for every caller class
**Prerequisites:** 1.5.8, 1.2.4
**As the** owner, **I want** a test that fails on any status write that does not go through the engine and tests that prove config edits change each class's result, **so that** Success Metric 1 is true for all paths and stays true.
**Acceptance Criteria**:
- *Given* the inventory test after this story, *When* it runs, *Then* the `pending` list is empty and the test fails on (a) any call, on any receiver, whose selector is one of the six old names or its `Raw*`/`raw*` form, outside the allow-list; (b) any `Gated*`, `Rollback` or `DebugForce` call whose `Caller` is not in the registry; (c) any non-`forwarding` registry entry with no call site (the `forwarding` `mcp_transition_delegate` is exempt: its coverage is the handler caller-resolution test, Task 1.5.5a); (d) any `.SetStatus(` call on a `BacklogItem` ent builder in package `session` outside the allow-listed writer functions (see Task 1.5.9a); the allow-list is exactly the gate file, the `Storage` wrapper bodies by function, the MCP delegate, the 3 `Rollback` sites, the 2 debug sites and the `create`/`exempt: debug` creators, each with a reason. The old names are removed from the `silenttransition` target set at the same time.
- *Given* class `operator`, *When* the operator disables the non-system edge `done->refining` then calls the RPC, *Then* denied; re-enabled, allowed (Story 1.2.4, linked here).
- *Given* class `agent`, *When* a `human_approval` gate is added to `in_progress->review` (a fixed `request_review` edge; the edge cannot be disabled but its gate can), *Then* `request_review` is denied; *When* `done->refining` is disabled, *Then* the generic `transition_backlog_item_status` delegate (`forwarding`, exempt from system-required edges) is denied; after approval or re-enable they succeed; *When* an operator tries to disable `in_progress->review`, *Then* the editor rejects it (the carve-out).
- *Given* class `system`, *When* a `human_approval` gate is added to `pr_pending->done`, *Then* the PR-merge reconcile is denied and reported; after approval the next tick succeeds; *When* the gate is removed, *Then* no denial occurs.
- *Given* the registry, *When* `TestEveryRegisteredCaller_DeniedByOperatorGate_IsReportedNotSwallowed` runs, *Then* for every non-forwarding caller with class `agent` or `system` it installs a blocking gate on one declared edge, drives the production function through a fake-backed harness, and asserts the denial reached the reporter (or the caller's own surfaced error) and nothing was written; the forwarding delegate is covered by a separate disable-an-edge case.
- *Given* the four mode flags, *When* the proof test runs, *Then* each has a `ConfigSurfaceProof` entry (agent flag registered in Task 1.5.3c, system flag in Task 1.5.8g, shadow and operator-enforce in Task 1.5.9c) flipping it through the production reader and asserting a different observable.
- *Given* a COPY of the live database (never the live file), *When* `make workflow-engine-replay DB=<copy>` runs (Task 1.5.9d), *Then* it prints, per registered caller and edge, the enforced decision under the live stage config and again with a synthetic `human_approval` gate added to each system-required edge one at a time on the copy, exits non-zero if a fixed system or agent edge is denied by the live config without any gate, and refuses a path that resolves to the live `~/.stapler-squad` database.
**Files**: `server/services/workflow_bypass_inventory_test.go`, `server/services/backlog_engine_enforcement_test.go`, `server/services/backlog_gate_classes_test.go`, `server/services/config_surface_proof_test.go`, `session/workflow_engine_replay.go`, `session/workflow_engine_replay_test.go`, `Makefile`

##### Task 1.5.9a: Flip the inventory test to strict and empty the pending list (S)
- Old and new names on any receiver; allow-list with reasons; remove the old names from the `silenttransition` target set.
- **Rule (d), the `.SetStatus(` guard (ADV-N3)**: rules (a)-(c) match six method NAMES, so a seventh writer written as a new ent builder (`client.BacklogItem.UpdateOneID(id).SetStatus(...)`) in any `session` file is invisible to them, which is how the missed sixth writer (`TransitionBacklogItemStatusWithPRFields`) went unnoticed twice. Add a lexical rule: any `.SetStatus(` call whose receiver type resolves to an ent `BacklogItem` create/update builder, in package `session` (the only package that can import the ent client), fails unless it sits inside one of the allow-listed functions: the five transition writers (`rawTransitionBacklogItemStatus`, `rawArchiveBacklogItem`, `rawUnarchiveBacklogItem`, `rawTransitionBacklogItemStatusWithPRFields` at `ent_repository_backlog.go:1753,1484,1559,1977`) and the `ReconcileStuckItems` write (`storage_backlog.go:1006`, now a per-item gated write), plus the creator `CreateBacklogItem` (`ent_repository_backlog.go:452`, reason `create`, Task 1.5.4g). Progress-note `SetStatus` calls (`ent_repository_backlog.go:2650`) are on a different builder type and are not matched. Fixture: a new file with `client.BacklogItem.UpdateOneID(id).SetStatus(...)` is flagged naming file and function. Test: `TestBacklogItemSetStatus_OnlyInsideAllowListedWriters`.
- Files: `server/services/workflow_bypass_inventory_test.go`, `tools/lint/silenttransition/analyzer.go`

##### Task 1.5.9b: Per-class config-edit proofs and the every-registered-caller table (M)
- Files: `server/services/backlog_gate_classes_test.go`, `server/services/backlog_engine_enforcement_test.go`

##### Task 1.5.9c: `ConfigSurfaceProof` entries for the shadow and operator-enforce flags, and the mode-behavior proof for all four (S)
- Files: `server/services/config_surface_proof_test.go`

##### Task 1.5.9d: Replay the registry against a copy of the live database (M)
- A read-only harness: opens a path given by `DB=<copy>` (made by the owner with `sqlite3 <live> ".backup <copy>"`, not by the agent against the live file), loads the live stage config from it, builds the selector, and for every registered caller and declared edge evaluates the `enforceAnswer` with and without a synthetic `human_approval` gate on that edge (gates applied to the in-memory copy only), reporting denials, would-be stranded items per status from the real item table, and the detector-guard table result. It is the evidence for low-volume edges where shadow sample counts cannot be reached (Risk Control), and exercises the `edited` path once before any flip. Run by the owner before each flip and attached to the PR.
- Files: `session/workflow_engine_replay.go`, `session/workflow_engine_replay_test.go`, `Makefile`

---

## Phase 2: Workstream B — quota and soft budget settings

### Epic 2.1: Backend
**Goal**: An operator can set and read quota and the soft-budget default with validation, and disabling the gate cannot leave backlog paused.

#### Story 2.1.1: Quota config RPC
**Prerequisites:** 5.1.1
**As an** operator, **I want** to save quota settings from the UI, **so that** I do not edit `config.json`.
**Acceptance Criteria**:
- *Given* stored `Enabled=false`, `AssumedWindowTokenBudget=0`, *When* `UpdateQuotaConfig{enabled:true, assumed_window_token_budget:5000000}` is called, *Then* `GetQuotaConfig` returns `enabled=true`, `mode=PROACTIVE`, effective values, and the quota gate's next `Reconcile` uses 5,000,000 without restart.
- *Given* `PauseBelowHeadroomPct=60` and `ResumeMarginPct=50`, *When* update is called, *Then* it returns `InvalidArgument` "pause_below_headroom_pct + resume_margin_pct must be at most 100" and nothing is saved.
- *Given* `PauseBelowHeadroomPct=0`, *When* update is called, *Then* `InvalidArgument` says zero would be silently replaced by the default 20 (VERIFIED `config/types.go:777-782`: pause default 20, resume margin default 15, and the same zero-replaced rule applies to `ResumeMarginPct`, tick counts, window and grace minutes and throttle seconds).
- *Given* `Enabled=true` and budget 0, *When* `GetQuotaConfig` is called, *Then* `mode=REACTIVE_ONLY` and `warnings` contains "no budget is set, so the proactive signal never fires".
- *Given* `default_cost_budget_threshold_usd` (Go field `DefaultCostBudgetThresholdUSD`) is `-1`, `NaN` or `+Inf`, *When* update is called, *Then* `InvalidArgument` and nothing is saved; `0.0` is accepted and an absent value stores nil (off).
- *Given* two concurrent writers, one `UpdateQuotaConfig` (field A) and one other `config.UpdateConfig` caller (field B), *When* both run 50 times in parallel, *Then* neither field's final value is lost (lost-update test against the package helper; ADR-004).
- *Given* `FeatureFlagService` flips `backlog:workflow-engine-enforce` while `UpdateQuotaConfig` saves `Enabled=true` (50 interleavings each, under `-race`), *When* both finish, *Then* both values hold (pre-mortem #3); `FeatureFlagService` writes go through `config.UpdateConfig`, and `saveConfigLocked` syncs the temp file before the rename.
- Source (UX-50): *Given* a `config.json` whose stored `quota` object is the zero value (never saved from the page), *When* `GetQuotaConfig` is called, *Then* `quota_source=NOT_CONFIGURED` while the effective values are the defaults; *Given* the page saved default-equal values (`Enabled=false`, budget 0, pause 20, resume 15), *Then* `quota_source=CONFIGURED`. It is derived from the stored value before `QuotaConfigOrDefault()` runs (`config/config.go:823,1673`; `Quota` is a non-pointer struct at `:390`, so the loaded value alone cannot tell the two apart) and the test pins both cases.
- Access (see Authn/authz): *Given* the local chain, *When* `UpdateQuotaConfig` or any of the nine stage/edge/gate write RPCs is POSTed with a foreign `Host` or `Origin`, *Then* `LocalWriteGuard` rejects it and `config.json` and the stage tables are unchanged; *Given* `guardedProcedures`, *Then* it contains `UpdateQuotaConfig` and those nine RPCs with `ProfileRebinding` and still contains `UpdateFeatureFlag` (the new mode flags use that RPC); *Given* the remote chain with an auth middleware that rejects every token, *When* `GetQuotaConfig` or `UpdateQuotaConfig` is called without credentials, *Then* 401 and the handler is not reached.
**Files**: `proto/session/v1/quota.proto`, `server/services/quota_settings_service.go`, `server/services/quota_settings_service_test.go`, `config/config.go`, `config/types.go`, `server/server.go`, `server/quota_listener_test.go`, `server/services/feature_flag_service.go`, `server/services/feature_flag_service_test.go`

##### Task 2.1.1a: Proto + service + validation + QuotaMode (M)
- Copy `unfinished.proto` pattern; read-modify-write through `config.UpdateConfig` (Task 2.1.1c); markers; registry; add `quota_source` to the response (stored-value accessor in `config/`, UNVERIFIED whether an accessor or a raw-file check is cheaper, the Source AC pins the behavior). Touches `dependencies.go` only for the shared quota closure (`:1429-1436`), serialized per the wave rules.
- Files: `proto/session/v1/quota.proto`, `server/services/quota_settings_service.go`, `config/types.go`, `server/server.go`

##### Task 2.1.1b: Validation unit tests and ConfigSurfaceProof entries (S)
- Registers into the registry from Story 5.1.1 (Wave 1, already merged).
- Files: `server/services/quota_settings_service_test.go`

##### Task 2.1.1e: Guard the engine-affecting writes (S)
- Add `UpdateQuotaConfig` and `CreateStage`, `UpdateStage`, `DeleteStage`, `CreateStageTransition`, `UpdateStageTransition`, `DeleteStageTransition`, `CreateTransitionGate`, `UpdateTransitionGate`, `DeleteTransitionGate` to `guardedProcedures` (`server/server.go:1907`) with `ProfileRebinding`, following the `updateFlagProcedurePath` constant pattern; tests in `server/quota_listener_test.go` modeled on `server/gatestats_listener_test.go:40-70` (membership test, foreign-Host POST rejected with state unchanged, remote chain 401). Manual-instance check (Task 0.1.1d script): the Stages UI still saves a stage and an edge from a browser and the MCP stage tools, if any call these RPCs, still work; a regression is fixed here, not by dropping a procedure from the set.
- Files: `server/server.go`, `server/quota_listener_test.go`

##### Task 2.1.1c: `config.UpdateConfig` read-modify-write helper (S)
- `UpdateConfig(fn func(*Config) error) error` holding a separate outer mutex (a new `sync.Map` keyed per config path, acquired before `LoadConfig`) across `LoadConfig`, `fn`, `SaveConfig`. It must never reuse `saveConfigLockFor` (`config/config.go:1475-1482`): `SaveConfig` takes that lock itself (`:1502`) and `sync.Mutex` is not reentrant, so reuse self-deadlocks. The inner lock only serializes the file write, not the read-modify-write.
- *Given* two goroutines each calling `UpdateConfig` 50 times, one mutating field A and one field B (different fields, same path), *When* both finish, *Then* both fields hold their last written values (no lost update) and the test completes under `-race` without deadlock; a second case calls `UpdateConfig` while `SaveConfig` runs on the same path to prove the two locks nest without self-deadlock. Residual risk, stated: other services (`defaults_service.go`, `jules_config_service.go`, `remote_service.go`, ...) and the `Config` setters (`config/config.go:593-702`) still do unlocked Load-modify-`SaveConfig`; migrating them is out of scope and noted as a follow-up (the flag service moves in Task 2.1.1d).
- Files: `config/config.go`, `config/config_test.go`

##### Task 2.1.1d: Move `FeatureFlagService` writes onto `config.UpdateConfig`; atomic-write hardening (M)
- `feature_flag_service.go` loads and saves `config.json` itself (VERIFIED `config.LoadConfig()` at `:453,:549,:574`) and writes the two-plus-two engine mode flags and the three autonomy flags, so it races with quota saves. Route every write (flag value and flag-scope writes) through `config.UpdateConfig`; the service reads back the stored value in its response. `SaveConfig` is already atomic (unique temp file in the same directory plus `os.Rename`, VERIFIED `config/config.go:1514-1555`); add `tmpFile.Sync()` before the rename so a power loss cannot leave a zero-length file. Test: flag flip versus `UpdateQuotaConfig`, 50 interleavings each, under `-race`; update ADR-004 consequence text (done in this repair).
- Files: `server/services/feature_flag_service.go`, `server/services/feature_flag_service_test.go`, `config/config.go`, `config/config_test.go`

#### Story 2.1.2: Gate resumes on disable; legible status text
**Prerequisites:** 2.1.1
**As an** operator, **I want** disabling the gate to un-pause backlog, **so that** a UI toggle cannot leave the factory stopped.
**Acceptance Criteria**:
- *Given* `QuotaGate` with `pausedByQuota=true`, *When* config flips to `Enabled=false` and `Reconcile` runs, *Then* `BacklogController.Enable` is called once, `pausedByQuota` becomes false, and a log line states "quota gate disabled while paused; resuming backlog".
- *Given* `StatusDetail()` for an enabled gate with budget 0, *When* rendered, *Then* the text no longer says "set in config.json" and points to Settings > Backlog quota.
- *Given* server start with `Enabled=true`, `PauseBelowHeadroomPct=30` and budget 5,000,000, *When* the quota gate is constructed, *Then* the startup `Using ...` line includes every effective `QuotaConfig` value and `QuotaMode=Proactive` (defaults are labelled `(default)` when the config is absent); the same line is logged on a live change through `UpdateQuotaConfig` (requirements Observability).
**Files**: `server/services/quota_gate.go`, `server/services/quota_gate_test.go`

##### Task 2.1.2a: Resume-on-disable and StatusDetail text (S)
- Files: `server/services/quota_gate.go`, `server/services/quota_gate_test.go`

##### Task 2.1.2b: Quota startup log line with effective values and `QuotaMode` (XS)
- Extend the existing `Using ...` log at gate construction (`dependencies.go:1429-1436` closure site) via a small formatter on `QuotaConfig`; touches `dependencies.go` only through the formatter call, so it rides Story 2.1.1's slot if the closure moves.
- Files: `server/services/quota_gate.go`, `server/services/quota_gate_test.go`

#### Story 2.1.3: Global soft-budget default
**Prerequisites:** 2.1.2
**As an** operator, **I want** a default soft budget for items without their own, **so that** requirement 2's threshold is settable.
**Acceptance Criteria**:
- *Given* `DefaultCostBudgetThresholdUSD=5.00` and an item with no `cost_budget_threshold_usd`, *When* cumulative spend is 5.20, *Then* `[BudgetWarning]` is logged by both evaluation sites (`logBudgetWarningIfCrossed` at `backlog_service.go:1457-1460` and the work-stage evaluation at `:1482`, whose callers include `backlog_service_triage.go:3220` and `backlog_service_trigger_triage.go:537`); *Given* the item has its own threshold 10.00, *When* spend is 5.20, *Then* no warning.
- The warning is a log line only (`[BudgetWarning]` at `backlog_service.go:1467,1489`, `~/.stapler-squad/logs/staplersquad.log`); the quota page help text says "Crossing is written to the server log; no notification is sent" (C10). Routing it to `notify()` is an owner decision, not in scope.
- *Given* `DefaultCostBudgetThresholdUSD=0.0`, *When* spend is 0.01 on an item with no threshold, *Then* the warning fires (0 is a valid threshold, nil is off).
**Files**: `config/types.go`, `server/services/backlog_service.go`, `server/services/backlog_service_budget_test.go`, `proto/session/v1/quota.proto`

##### Task 2.1.3a: Config field, resolution in service layer at both evaluation sites, RPC field (S)
- Resolution happens in the service layer via one helper `effectiveBudgetThreshold(item, cfg)` used by every caller; `session/budget_warning.go` stays pure.
- Files: `config/types.go`, `server/services/backlog_service.go`, `proto/session/v1/quota.proto`

### Epic 2.2: UI
**Goal**: A mobile- and desktop-usable quota page whose states are legible.

#### Story 2.2.1: Backlog quota settings page
**Prerequisites:** 2.1.3, 4.1.4 (the `flagExtras` registry, Task 4.1.4b, and `AlertConfirmDialog`, Task 4.1.4e)
**As an** operator, **I want** a page showing effective state and editable fields, **so that** I know what 0 means before saving.
**Acceptance Criteria**:
- *Given* `Enabled=true`, budget 0, *When* the page loads, *Then* a static warning (`data-testid="backlog-quota-warning"`, no `role`; the budget input's `aria-describedby` points at it; its appearance after an edit is announced once through `backlog-quota-status`) shows "Gate is on but no budget is set, so the proactive signal never fires" and the helper line "0 = proactive headroom signal disabled" is always visible text.
- *Given* a pause threshold of 0 or a resume margin of 0, *When* Save is pressed, *Then* the inline error says the value must be between 1 and 100 and that 0 would be replaced by the default (20 and 15), matching the server rule; range hints read 1-100 for both fields.
- *Given* the form with `pause=60`, `resume=50`, *When* Save is pressed, *Then* the inline error with `aria-invalid` and `aria-describedby` says "Pause threshold plus resume margin must be at most 100", focus is not lost, and entered values are kept.
- *Given* a successful save, *When* the read-back differs from the entered values, *Then* a `role="alert"` mismatch message replaces "Applied".
- *Given* a viewport of 360px, *When* the page renders, *Then* inputs are at least 44px tall, `inputMode="numeric"`, labels above inputs, no horizontal scroll; at 1280px fields use a two-column grid.
- *Given* the on-screen keyboard at 360px, *When* a field is focused, *Then* every field except the last has `enterKeyHint="next"` (Enter moves to the next field) and the last has `enterKeyHint="done"` (Enter submits), and the focused field is scrolled into view above the keyboard (`scrollIntoView({block:"center"})` when `visualViewport.height` is smaller than the window height, with `scroll-margin-bottom` for the inline Save row) so it is not hidden (UX-41).
- *Given* the gate was paused by quota and the user disables it, *When* the save returns, *Then* the page shows "Backlog resumed".
- *Given* the quota RPC fails on load, *When* the page renders, *Then* a `role="alert"` message "Couldn't load quota settings: <reason>." with a Retry button shows and the form is hidden (defaults never shown as truth); while loading, the region has `aria-busy="true"` and no field renders (UX-23).
- *Given* the USD default-budget field, *When* inspected, *Then* it has `inputMode="decimal"` while the token and percentage fields have `inputMode="numeric"` and `pattern="[0-9]*"` (UX-24).
- *Given* the Features page, *When* the quota flag row renders, *Then* it contains a link to `/settings/backlog-quota` supplied by a `flagExtras` registry entry; *Given* the Settings index (`/settings`), *Then* a Link tile "Backlog Quota" with `data-testid="settings-backlog-quota-link"` using `routes.settingsBacklogQuota` is present (VERIFIED: `settings/layout.tsx` has no nav list; sub-pages are reached from Link tiles in `settings/page.tsx`, e.g. `data-testid="settings-remotes-link"`); both navigate (UX-18).
- *Given* unsaved edits, *When* the user has changed any field, *Then* a visible "Unsaved changes" status line (`data-testid="backlog-quota-status"`, shared with Applied/errors and announced once per transition) appears and Revert is enabled.
- *Given* unsaved edits, *When* the user tries to leave (tab close or reload via `beforeunload`, which is registered while the form is dirty OR a save is pending; a click on any same-origin in-app link, intercepted in the capture phase; or browser Back or the phone back-swipe, handled with a history sentinel), *Then* an `AlertConfirmDialog` "Discard unsaved changes?" opens with initial focus on "Keep editing" (Cancel role) and "Discard" as the confirm action, and navigation proceeds only on Discard; *Given* no unsaved edits and no pending save, *Then* no guard is active and no `beforeunload` handler is registered (UX-38). Mechanics (UX-51): when the form becomes dirty the hook calls `history.pushState({ssqLeaveGuard: true}, "", location.href)`; Back pops the sentinel with the page and URL unchanged and opens the dialog; Keep editing pushes a new sentinel (history length unchanged, URL unchanged, entries never double-stack); Discard disarms and calls `history.back()` once more; a link click's Discard navigates with `router.replace(href)` while the sentinel is armed (`router.push` otherwise); if the form becomes clean with a sentinel in history, the next Back lands on it and the hook immediately calls `history.back()`. Limit, stated: the Next.js app router exposes no route-change veto, so a programmatic `router.push` from other code is not covered.
- *Given* any sequence of edit, save-pending, applied, warning-appearing and mismatch states, *When* the page is inspected, *Then* an unscoped `getByRole("status")` matches exactly one element (`backlog-quota-status`, one message at a time: Saving..., Applied (with "Backlog resumed"), Unsaved changes announced on the clean-to-dirty transition only, the warning appearing or disappearing), the "Effective now" line, help text and warning are static, and at most one `role="alert"` exists at a time (validation summary, save-failed and mismatch replace each other) (UX-49).
- *Given* a save whose read-back differs from the entered values, *When* the mismatch appears, *Then* focus moves to `backlog-quota-mismatch` (`tabIndex={-1}`), which lists each differing field as "Entered <x>, effective now <y>" with two buttons: "Reload effective values" (replaces the fields with the read-back, clears the dirty state, announces "Reloaded effective values", focuses the first differing field) and "Save again" (re-submits the entered values, focus stays on the button while "Saving..." shows) (UX-49).
- *Given* `GetQuotaConfig` returns `quota_source=NOT_CONFIGURED`, *When* the page renders, *Then* it shows "Quota is not configured. Defaults apply: gate Off, no budget." (`backlog-quota-not-configured`) with the defaults labelled as defaults; *Given* `quota_source=CONFIGURED` with default-equal values, *Then* that sentence is absent; *Given* a response without the field, *Then* the load-failed state shows (UX-50).
- *Given* viewport 360px, *When* the page renders, *Then* every input row (field, switch and label) has a bounding box at least 44px tall and clicking a label focuses or toggles its control (UX-54).
**Files**: `web-app/src/app/settings/backlog-quota/page.tsx`, `web-app/src/app/settings/backlog-quota/page.css.ts`, `web-app/src/app/settings/backlog-quota/page.test.tsx`, `web-app/src/app/settings/backlog-quota/useUnsavedChangesGuard.ts`, `web-app/src/app/settings/backlog-quota/useUnsavedChangesGuard.test.tsx`, `web-app/src/app/settings/page.tsx`, `web-app/src/app/settings/__tests__/page.backlog-quota-link.test.tsx`, `web-app/src/lib/routes.ts`, `web-app/src/lib/nav-pages.ts`, `web-app/src/lib/__tests__/nav-pages.test.ts`, `web-app/src/app/settings/features/flagExtras.tsx`

##### Task 2.2.1a: Page, form, validation, read-back, Settings-index `<Link>`, `routes.settingsBacklogQuota`, `nav-pages.ts` entry and Features link (M)
- `// +feature:` marker in first 10 lines; pnpm only; copy `LivenessRowEditor` field shape; add the Features-to-Quota link as a `flagExtras` entry (does not edit `features/page.tsx`, which is serialized elsewhere); add a `<Link>` entry in `settings/page.tsx` beside the other `helpLink` links (`:89-129`), the route constant in `lib/routes.ts`, and a `NAV_PAGES` entry in `web-app/src/lib/nav-pages.ts` (group `settings`, beside Feature Flags at `:73`) with `web-app/src/lib/__tests__/nav-pages.test.ts` updated, since that test pins the list; `enterKeyHint` and scroll-into-view per the AC.
- Files: `settings/backlog-quota/page.tsx`, `page.css.ts`, `settings/page.tsx`, `lib/routes.ts`, `lib/nav-pages.ts`, `lib/__tests__/nav-pages.test.ts`, `settings/features/flagExtras.tsx`, `settings/__tests__/page.backlog-quota-link.test.tsx`

##### Task 2.2.1c: Unsaved-changes navigation guard with history sentinel (M)
- `useUnsavedChangesGuard({dirty, savePending})` hook: `beforeunload` registered while dirty or a save is pending; capture-phase click interception for same-origin anchors; the history-sentinel design of the AC (`pushState` on dirty, re-arm on Keep editing, `history.back()` on Discard and on a clean-state Back, `router.replace` for link Discard); opens `AlertConfirmDialog` (Cancel = "Keep editing", initial focus). Jest with a fake `history`: not registered when clean, registered when dirty and when save-pending, Keep editing leaves URL and `history.length` unchanged, a second Back re-opens the dialog, Discard completes the Back, clean-state Back skips the sentinel, link Discard uses `replace`, modifier-key and external-link clicks pass through.
- Files: `settings/backlog-quota/useUnsavedChangesGuard.ts`, `settings/backlog-quota/useUnsavedChangesGuard.test.tsx`, `settings/backlog-quota/page.tsx`

##### Task 2.2.1d: Single live region, mismatch recovery, not-configured state, 44px rows (S)
- `backlog-quota-status` as the only status region with the priority order of the AC; static warning, effective and help lines; mismatch alert with focus handoff and the Reload effective values / Save again buttons; `quota_source` rendering; input rows at least 44px tall at 360px.
- Files: `web-app/src/app/settings/backlog-quota/page.tsx`, `web-app/src/app/settings/backlog-quota/page.css.ts`, `web-app/src/app/settings/backlog-quota/page.test.tsx`

##### Task 2.2.1b: Jest tests for the state matrix (loading, load-failed+retry, save-pending, save-failed, mismatch, unsaved-changes, decimal input mode, enterKeyHint) (S)
- Files: `settings/backlog-quota/page.test.tsx`

---

## Phase 3: Workstream C — real `allow_auto_merge`

### Epic 3.1: Lookup, surface, warn
**Goal**: "Unknown" disappears; each state is real and the noise from this repo's disabled setting is contained.

#### Story 3.1.1: `RepoSettings` in package `github`
**Prerequisites:** None (placed in W6 by the WIP cap only)
**As a** maintainer, **I want** a host-aware, cached, rate-limit-safe lookup, **so that** no per-render fetch or wrong-host query occurs.
**Acceptance Criteria**:
- *Given* an `httptest` server returning `{"allow_auto_merge":false,"allow_squash_merge":true}` for `GET /repos/acme/web`, *When* `RepoSettings(ctx, ref)` is called, *Then* it returns `Status=OK` with `AutoMerge()` equal to `(false, true)`, `lookup_total{outcome="success"}=1`.
- *Given* the same repo within TTL, *When* called again, *Then* no HTTP request occurs and `outcome="cached"` increments; *Given* the server replies 304 after TTL, *When* called, *Then* the cached parsed value (not zero value) is returned and `outcome="not_modified"` increments.
- *Given* 20 concurrent callers for one uncached repo, *When* called, *Then* exactly 1 HTTP request is made (singleflight).
- *Given* the server returns 429, *When* called, *Then* `Status=RATE_LIMITED`, the result is not cached as `false`, and a later call after back-off re-fetches.
- *Given* a GHE `RepoRef{Host:"ghe.example.com"}` and `SetEnterpriseBaseURLOverride`, *When* called, *Then* the request hits the GHE test server and none reaches github.com.
- *Given* a 200 body lacking `allow_auto_merge` (read-only token), *When* parsed, *Then* `Status=NO_PERMISSION`, never `false`.
- *Given* a lookup that hangs, *When* it exceeds the 2s context timeout, *Then* it returns `ERROR` (reason `timeout`) and counts `failure`; *Given* 3 consecutive failures for one host, *When* a 4th lookup for any repo on that host is requested, *Then* it returns `ERROR` immediately without an HTTP call until the 60s half-open probe (fake clock), so one failing GHES host cannot stall `GetBacklogStuckReport` (pre-mortem #7).
- *Given* any result with `Status != OK`, *When* `AutoMerge()` is called, *Then* it returns `(false, false)`; only `Status==OK` returns `ok=true` (the raw bool is unexported, so "false" and "not asked" cannot be conflated).
**Files**: `github/repo_settings.go`, `github/repo_settings_cache.go`, `github/repo_settings_test.go`

##### Task 3.1.1a: Fetch, parse, classify via `newGHRequestForHostWithToken` (M)
- Account-aware token; deliberate call-origin tag; lint rule `norawghrequest` satisfied by construction.
- Files: `github/repo_settings.go`, `github/repo_settings_test.go`

##### Task 3.1.1b: Cache keyed `(host, owner, repo)`, ETag value retention, singleflight (M)
- Files: `github/repo_settings_cache.go`, `github/repo_settings_test.go`

##### Task 3.1.1c: Negative-cache cap, back-off, 2s lookup timeout, per-host circuit breaker, lookup counters, token-without-push case (S)
- Files: `github/repo_settings_cache.go`, `github/repo_settings.go`, `github/repo_settings_test.go`

#### Story 3.1.2: Populate in the service layer
**Prerequisites:** 3.1.1
**As an** operator, **I want** the stuck report to carry real status, **so that** the panel stops saying unknown.
**Acceptance Criteria**:
- *Given* `StuckItem` for `BI-9` whose PR repo is `tstapler/stapler-squad` and `RepoSettings` returns `OK, false`, *When* `GetBacklogStuckReport` runs, *Then* the proto has `allow_auto_merge=false`, `auto_merge_lookup_status=OK`, `auto_merge_checked_at` set.
- *Given* an item with no GitHub repo (local-only), *When* the report runs, *Then* `auto_merge_lookup_status=UNSPECIFIED` and no GitHub call is made.
- *Given* 12 stuck items from one repo, *When* the report runs, *Then* one `RepoSettings` fetch occurs.
- *Given* any item whose `auto_merge_lookup_status` is not `OK`, *When* the proto is built, *Then* `allow_auto_merge` is unset (a test asserts it is nil for every non-OK status, so `false` never rides with a failed lookup). The proto comment states `allow_auto_merge` is meaningful only when status is `OK`; a `oneof` is the stronger form if proto churn is acceptable.
- *Given* a lookup that failed (`RATE_LIMITED`, `NO_PERMISSION`, `NOT_FOUND` or `ERROR`), *When* the proto is built, *Then* `auto_merge_checked_at` is the time of the attempt (not unset, not the zero time), which is what the UI shows as "Last tried" (UX-53); for `UNSPECIFIED` it is unset.
**Files**: `proto/session/v1/backlog.proto`, `server/services/backlog_service_stuck.go`, `server/services/backlog_stuck_rpc_test.go`

##### Task 3.1.2a: Proto enum/fields, populate at caller (not mapper), update test pinned at `backlog_stuck_rpc_test.go:179` (M)
- Files: `proto/session/v1/backlog.proto`, `server/services/backlog_service_stuck.go`, `server/services/backlog_stuck_rpc_test.go`

#### Story 3.1.3: StuckItemDetail states
**Prerequisites:** 3.1.2
**As an** operator, **I want** honest copy per state, **so that** I know why a PR may not auto-merge.
**Acceptance Criteria**:
- *Given* status OK and `allow_auto_merge=false`, *When* a `pr_ready_unmerged` item renders, *Then* the `stuck-item-auto-merge` element reads "Repo auto-merge: not allowed on this repo" with "Merge this PR by hand, or enable Allow auto-merge in the repo settings." and a `rel="noopener"` link whose visible text ends "(opens in new tab)" (WCAG 3.2.5, UX-40).
- *Given* status RATE_LIMITED, *When* rendered, *Then* "Repo auto-merge: couldn't check (rate limited). Last tried 2m ago." with a Retry button; the word "unknown" appears nowhere and the debug suffix `(allow_auto_merge: ...)` is gone.
- *Given* status UNSPECIFIED, *When* rendered, *Then* the line is omitted entirely.
- *Given* status OK and `allow_auto_merge=true`, *When* rendered, *Then* the line reads "Repo auto-merge: allowed" with the caveat "GitHub setting only. Branch protection or required checks can still block it." (UX-27).
- *Given* a Retry in flight, *When* the check is pending, *Then* the line reads "Repo auto-merge: checking..." with `aria-busy="true"`, and on failure the failed line returns with a new "Last tried" time (UX-28).
- *Given* a cached value older than its TTL whose refresh failed, *When* rendered, *Then* the line shows the value with "(as of <ago>, may be out of date)".
- *Given* viewport 360px, *When* rendered, *Then* text wraps with no truncation and Retry and the settings link are at least 44px tall.
- *Given* a failed status with `auto_merge_checked_at` set, *When* rendered, *Then* "Last tried <ago>" uses that timestamp as a relative time recomputed at most every 30 s without announcement; *Given* the field is absent, *Then* the "Last tried" sentence is omitted (UX-53). *Given* the item's PR URL or `RepoRef` yields host, owner and name, *Then* the settings link points at that repo (the GitHub Enterprise host when the host is not github.com); *Given* they cannot be derived, *Then* the link is absent and the guidance text does not refer to it (UX-53).
- *Given* a Retry that fails again, *When* the failed line re-renders, *Then* the Retry button is the same mounted element (stable `key`, `aria-disabled` and "Checking..." while pending) and still has focus; *Given* the Retry succeeds, *Then* the button unmounts and focus is on `stuck-item-auto-merge` (`tabIndex={-1}`), never `body` (UX-53).
**Files**: `web-app/src/components/backlog-stuck/StuckItemDetail.tsx`, `web-app/src/components/backlog-stuck/StuckItemDetail.test.tsx`, `web-app/src/components/backlog-stuck/StuckItemDetail.css.ts`, `proto/session/v1/backlog.proto`

##### Task 3.1.3a: Render states (OK true/false, failed, checking, stale, UNSPECIFIED), remove debug suffix, Retry via interactive refresh (M)
- Files: `web-app/src/components/backlog-stuck/StuckItemDetail.tsx`, `web-app/src/components/backlog-stuck/StuckItemDetail.css.ts`, `web-app/src/components/backlog-stuck/StuckItemDetail.test.tsx`, `proto/session/v1/backlog.proto` (refresh flag)

##### Task 3.1.3b: "Last tried" source, missing-repo-URL case and Retry focus retention (S)
- Relative-time hook with a 30 s tick, the repo-URL derivation with the omit rule, the stable-key Retry element and the focus handoff to `stuck-item-auto-merge`; Jest for each branch (present or absent timestamp, GitHub or GHE or unknown host, repeat failure, success).
- Files: `web-app/src/components/backlog-stuck/StuckItemDetail.tsx`, `web-app/src/components/backlog-stuck/StuckItemDetail.test.tsx`

#### Story 3.1.4: Pre-PR warning without noise
**Prerequisites:** 3.1.2
**As an** operator, **I want** one warning per repo, **so that** this repo's disabled setting does not spam me.
**Acceptance Criteria**:
- *Given* `RepoSettings` cached `OK,false` for `tstapler/stapler-squad` and 5 PRs created in one day, *When* each runs `pushAndCreatePR`, *Then* PR creation is never blocked and exactly 1 notification with key `automerge-off:github.com/tstapler/stapler-squad` is sent in 24h.
- *Given* `EnablePRAutoMerge` later fails for a PR, *When* the existing "Auto-merge not enabled" notification fires, *Then* it is the single message for that PR (preflight suppressed for that PR).
- *Given* a cold cache, *When* `pushAndCreatePR` runs, *Then* no synchronous GitHub call is made on the PR-create path, and an asynchronous `RepoSettings` fetch is started (background origin, never blocking) so the cache is warm for the next PR and the warning can fire (C4).
- *Given* a cached `OK,false` result, *When* the preflight notification is sent, *Then* its text contains the repo name and "PR will still be created" and contains neither "unknown" nor a raw field name (ux.md section 7).
- *Surface and dismissal (UX-42)*: *Given* the preflight fires, *Then* it is sent through `BacklogLifecycleListener.notify` (`session/backlog_lifecycle.go:640`), the same path as the existing "Auto-merge not enabled" notification (`session/backlog_lifecycle_pr.go:721`), with `notificationType=10` (INFO, not WARNING), `urgent=false`, `important=false`, tied to the item id, so it appears in the app's notification list (`/notifications`) without interrupting; it is dismissed with that list's existing per-notification control, VERIFIED in repair round 7 by reading the code: the `Remove notification` button on the row (`web-app/src/components/ui/NotificationItem.tsx:215`, rendered when the list passes `removeFromHistory`, which every list except the Needs-decision section does), `x` on a focused row (`TrayList.tsx`), the session-level `Dismiss N informational` button and the `Clear informational (N)` menu item; `INFO` maps to the informational bucket (`web-app/src/lib/utils/notificationMapping.ts:36-43`, Recent activity) and `tests/e2e/notification-a11y-matrix.spec.ts` has no INFO-specific expectation (it seeds `ERROR` and `CUSTOM`), so no matrix coupling is added; no new UI is added. A Jest/Go assertion pins type, urgent and important.
**Files**: `session/backlog_lifecycle_pr.go`, `session/backlog_lifecycle_pr_test.go`

##### Task 3.1.4a: Cached-only preflight, dedupe key, copy assertions, test (S)
- Runs FIRST in the story (the surface and level it fixes are what the UX-42 e2e in Story 6.1.1 asserts). Step 1, before any code: re-confirm at HEAD that `Remove notification` (`NotificationItem.tsx:215`) still dismisses an `INFO` row in the informational bucket and record that in the PR; step 2: the preflight itself.
- Files: `session/backlog_lifecycle_pr.go`, `session/backlog_lifecycle_pr_test.go`

##### Task 3.1.4b: Warm the repo-settings cache asynchronously at PR creation (S)
- On `pushAndCreatePR`, if the cache has no entry for the repo, start a bounded background fetch (singleflight, the 2s timeout and breaker from Task 3.1.1c) so later PRs can warn; a failed fetch sends nothing.
- Files: `session/backlog_lifecycle_pr.go`, `session/backlog_lifecycle_pr_test.go`

---

## Phase 4: Workstream D — autonomy flags: record, instrument, legibility

### Epic 4.1: Decision record and evidence
**Goal**: Every default is a recorded decision with a queryable evidence path; code defaults stay off.

#### Story 4.1.1: Promote ADR-005
**Prerequisites:** 4.1.3
**As the** owner, **I want** the state and rationale in the ADR index, **so that** a fresh install and the live instance are explained.
**Acceptance Criteria**:
- *Given* `docs/adr/`, *When* the task finishes, *Then* a new ADR carries the table of code default / live state / evidence / rollback for all three flags with the real enablement date or an explicit "not recorded" line, and cites the commands run.
**Files**: `docs/adr/0NN-autonomy-flag-defaults.md` (next free number), `docs/explanation/autonomous-stall-causes.md`

##### Task 4.1.1a: Promote and complete evidence lines (S)
- Run `GetFeatureFlags` read-back and record output; link `docs/explanation/autonomous-stall-causes.md`.
- Files: `docs/adr/`, `docs/explanation/autonomous-stall-causes.md`

#### Story 4.1.2: Durable nudge-outcome counter
**Prerequisites:** 4.1.4
**As the** owner, **I want** nudge outcomes queryable, **so that** exit criteria are measurable.
**Acceptance Criteria**:
- *Given* a stall nudge delivered to session `S1`, *When* it completes, *Then* a log line `nudge outcome kind=stall outcome=delivered session=S1` is written and `nudge_outcome_total{kind="stall",outcome="delivered"}` is 1 on `/actuator/nudge-outcomes`.
- *Given* the process restarts, *When* the endpoint is read, *Then* the ledger-backed totals persist and `since` shows the first-record time.
- *Given* the nudge cap is hit, *When* a stall occurs, *Then* `outcome="skipped_cap"` is counted.
- *Given* an `ErrNudgeSubmitUncertain`, *When* handled, *Then* `outcome="uncertain"`.
**Files**: `session/nudge_outcomes.go`, `server/services/autonomous_orchestration_service.go`, `server/mcp/tools_diagnose.go`, `server/actuator.go`, `proto/session/v1/backlog.proto`

##### Task 4.1.2a: Recorder, ledger, actuator (S)
- Files: `session/nudge_outcomes.go`, `server/actuator.go`

##### Task 4.1.2b: Instrument stall path and diagnose handler (M)
- `autonomous_orchestration_service.go:~727`, `tools_diagnose.go:~361`, `session_driver.go:933` path; expose `DiagnoseNudgeCount` on the item proto.
- Files: `server/services/autonomous_orchestration_service.go`, `server/mcp/tools_diagnose.go`, `proto/session/v1/backlog.proto`

#### Story 4.1.3: One declaration per default
**Prerequisites:** 4.1.2
**As a** maintainer, **I want** the registry to read the default from `config`, **so that** UI and runtime cannot disagree.
**Acceptance Criteria**:
- *Given* the three flags, *When* the parity test runs, *Then* for each, `featureFlagDefault(name)` equals the `config` constant and all three are `false`; `backlog:sdd-default-pipeline` reads via `GetFeatureFlagWithDefault` and an explicit `false` still opts out.
- *Given* a new flag added with mismatched defaults, *When* the test runs, *Then* it fails naming the flag.
**Files**: `config/config.go`, `server/services/feature_flag_service.go`, `server/services/backlog_service_lifecycle.go`, `server/services/feature_flag_service_test.go`

##### Task 4.1.3a: Single-source defaults and generalized parity test (S)
- Touch only `backlog_service_lifecycle.go:136-150` (flag read), nothing near the transition handler. Complexity guard: the gocognit/gocyclo gate applies to modified existing functions, so Extract the flag read into a small helper (own commit, no behavior change) before changing it, and run `make ready-complexity-gate` on the touched functions.
- Files: `config/config.go`, `server/services/feature_flag_service.go`, `server/services/backlog_service_lifecycle.go`, `server/services/feature_flag_service_test.go`

#### Story 4.1.4: Features page legibility, accessibility rework, shared confirm dialog and contrast
**Prerequisites:** None
**As an** operator, **I want** accurate On/Off/Inert states, operable switches and a safe confirm dialog, **so that** an On flag that does nothing says so and a dangerous flip cannot be clicked through.
**Traceability (C9, revised in round 6)**: all of this story is mandatory (the owner pursues full scope; nothing here is a cut candidate). The legibility criteria implement `requirements.md` Rabbit Hole "Flag interaction: ... UI/docs must say so" and Success Metric 4; the accessibility criteria (switch semantics, hit area, focus ring, load/toggle failure states) fix defects in a component this project edits anyway and are required by UX-09, UX-10, UX-13, UX-14, UX-17 and the Features-page part of UX-32/UX-34, which Story 1.3.3's four new rows depend on (inconsistent rows would result otherwise). They stay in separate tasks and commits (4.1.4a, 4.1.4d) for reviewability only.
**Acceptance Criteria (Legibility)**:
- *Given* `diagnose_nudge_enabled=false` and `autonomous_stall_nudge_enabled=true`, *When* the Features page renders, *Then* the stall row shows badge "On (inactive)" and "Has no effect until Diagnose & Nudge is On." and sits indented under the diagnose row in a group headed "Autonomous backlog" (UX-11).
- *Given* a flag set only by default, *When* rendered, *Then* "Default: Off (not set explicitly)" is shown; *Given* explicitly set, *Then* "Set explicitly: On" (UX-12).
- *Given* `FEATURE_META`, *When* the nudge flags render, *Then* human labels, not raw keys, appear.
**Acceptance Criteria (Accessibility, mandatory)**:
- *Given* any flag row, *When* inspected, *Then* the control has `role="switch"`, `aria-checked`, a stable label (not flipping Enable/Disable), `aria-describedby` to its description, a hit area of at least 44x44px, a visible `:focus-visible` ring, and no `aria-pressed` remains (UX-09, UX-10).
- *Given* viewport 360px and keyboard-only use, *When* toggling, *Then* the row is operable and `role="status"` confirms the change without moving focus (UX-13).
- *Given* the flag list request fails, *When* the page renders, *Then* a `role="alert"` "Couldn't load feature flags: <reason>" with a Retry button shows and no switch renders (a failure never renders as Off); *Given* the request is in flight, *Then* `aria-busy="true"` skeleton rows show with no switch rendered as Off (UX-14, UX-17).
- *Given* a toggle call fails, *When* the response returns, *Then* the switch reverts to the server value and the row shows a `role="alert"` "Couldn't change <flag>: <reason>" with a Retry button; while pending the switch uses `aria-disabled` and the row reads "Saving..." (UX-14).
- *Given* a toggle in flight, *When* Space or Enter is pressed on the switch or "Revert to Off" is clicked in that row, *Then* no second request is sent, focus stays on the switch, the switch and "Revert to Off" are `aria-disabled`, and the row's status region announces "Saving <flag>..." once and then the result (UX-48).
**Acceptance Criteria (Confirm dialog)**:
- *Given* `AlertConfirmDialog`, *When* opened, *Then* it renders `role="alertdialog"` with `aria-modal="true"`, `aria-labelledby` the heading and `aria-describedby` the body, takes initial focus on the Cancel button (via `useDialogFocusTrap` `initialFocusRef`, not the primary button), traps Tab/Shift+Tab, cancels on Escape and on overlay click, and returns focus to the invoking element (the hook restores focus to the element focused when the dialog first rendered; because Safari and Firefox on macOS do not focus a button on click, the component also accepts `returnFocusRef` and the caller focuses the switch before opening; the component, not the hook, implements overlay click, following the `HostKeyTrustDialog` precedent, which also focuses Cancel; the background is not made `inert`, a stated limit); it accepts a body slot, a `confirmLabel`, a `cancelLabel`, `confirmDisabled`, `confirmDescribedById` (the id of a visible hint, applied as `aria-describedby` on the confirm button while it is `aria-disabled`) and `returnFocusRef`, and activating a `confirmDisabled` confirm does nothing (UX-43); DOM and tab order are Cancel then confirm; at 360px the buttons stack full width, Cancel on top, confirm below with a 16px gap so a thumb mis-tap does not land on confirm, each at least 44px tall; at 1280px they sit right-aligned, Cancel then confirm (UX-15, UX-41). `PiDisableWarningDialog` is NOT reused or changed: it is `role="dialog"` with initial focus on its primary "I understand" button (`PiDisableWarningDialog.tsx:27-34`), which is the unsafe default for a dangerous action.
**Acceptance Criteria (Contrast, UX-32)**:
- *Given* `tests/e2e/a11y-route-sweep.spec.ts` `KNOWN`, *When* this story merges, *Then* the `color-contrast` exemptions for `/settings`, `/settings/backlog-stages` and `/settings/features` are removed and the sweep passes on those three routes in the default theme because the underlying contrast defect (cause not yet established; research/ux.md says verify the `textMuted` token first, shared via `web-app/src/styles/theme.css.ts`) is fixed, not hidden; *Given* the fix is a shared token change, *Then* the entries of other routes that it also fixes are removed too (the file says "only remove entries, never add"). *Given* Task 4.1.4g, *When* it runs before Task 4.1.4f, *Then* the axe baseline with the three exemptions OFF (including the shared tiles on `/settings`) is recorded in the PR with each violating node, computed ratio and owning component, and Task 4.1.4f fixes every recorded violation at its source; no exemption is narrowed to a selector, and if the baseline shows violations outside the shared token the fix covers those components too.
**Files**: `web-app/src/app/settings/features/page.tsx`, `web-app/src/app/settings/features/page.css.ts`, `web-app/src/app/settings/features/flagExtras.tsx`, `web-app/src/app/settings/features/page.test.tsx`, `web-app/src/lib/contexts/FeatureFlagsContext.tsx`, `server/services/feature_flag_service.go`, `web-app/src/components/settings/AlertConfirmDialog.tsx`, `web-app/src/components/settings/AlertConfirmDialog.css.ts`, `web-app/src/components/settings/AlertConfirmDialog.test.tsx`, `web-app/src/styles/theme.css.ts`, `web-app/src/app/globals.css`, `web-app/src/app/settings/settings.css.ts`, `web-app/src/styles/settingsListPage.css.ts`, `tests/e2e/a11y-route-sweep.spec.ts`, `server/services/feature_flag_service_test.go`

##### Task 4.1.4a: Switch semantics, hit area, focus ring, pending semantics, test update (`aria-pressed` coupling) (M)
- Mandatory (round 6). Round 7 adds the pending rules: Space/Enter ignored while `aria-disabled`, "Revert to Off" `aria-disabled` while the row's change is in flight, "Saving <flag>..." announced once (UX-48).
- Files: `web-app/src/app/settings/features/page.tsx`, `web-app/src/app/settings/features/page.css.ts`, `web-app/src/app/settings/features/page.test.tsx`

##### Task 4.1.4b: `flagExtras` registry, grouping, FEATURE_META, source line, per-flag live-note override (M)
- Requirement-tied; also the extension point used by Tasks 1.3.3a and 2.2.1a; `LIVE_FLAG_NOTE` (`features/page.tsx:36`) becomes the default that a `flagExtras` entry can override.
- Files: `web-app/src/app/settings/features/page.tsx`, `web-app/src/app/settings/features/flagExtras.tsx`, `web-app/src/lib/contexts/FeatureFlagsContext.tsx`

##### Task 4.1.4c: Server `statusDetail` for inert state and flag source (S)
- Status-detail provider: "inactive: requires diagnose_nudge_enabled", source default vs explicit.
- **DONE (W0, `b48a6a119`, errata 2026-10-18)**: both nudge flags were already registered in `knownFeatureFlags` (`feature_flag_service.go` ~`:286-292`), so only the status provider (`autonomyFlagStatusDetail`) was added. NOT implemented: the "Rollback line" and a default `LIVE_FLAG_NOTE` on every row (the existing Jest test `fg1` asserts exactly one note). Residual item for Story 1.3.3 (Task 1.3.3a) and Task 4.1.3a to decide.
- Files: `server/services/feature_flag_service.go`, `server/services/feature_flag_service_test.go`

##### Task 4.1.4d: Load-failed, loading and toggle-failed states (S)
- Mandatory (round 6). Files: `web-app/src/app/settings/features/page.tsx`, `web-app/src/app/settings/features/page.test.tsx`

##### Task 4.1.4e: `AlertConfirmDialog` component (M)
- New reusable component per the Confirm dialog AC, built on `useDialogFocusTrap` (`web-app/src/lib/hooks/useDialogFocusTrap`, accepts `initialFocusRef` and `onEscape`) and the portal pattern of `PiDisableWarningDialog`; Jest: initial focus is Cancel, Tab trap, Escape, overlay click cancels while a click inside does not, focus return after Cancel, Escape, overlay click and Confirm for an opener focused by keyboard, by click and by an opener that did not take focus (`returnFocusRef`), `confirmDisabled` with `confirmDescribedById` wiring and a no-op activation, role and aria wiring (UX-43). Used by Tasks 1.3.3a and 2.2.1c.
- Files: `web-app/src/components/settings/AlertConfirmDialog.tsx`, `AlertConfirmDialog.css.ts`, `AlertConfirmDialog.test.tsx`

##### Task 4.1.4g: Axe baseline with the color-contrast exemptions OFF on `/settings`, `/settings/backlog-stages`, `/settings/features` (S)
- Runs BEFORE Task 4.1.4f (the heading sits first only to keep the file's lettering; the story orders 4.1.4g, then 4.1.4f). Temporarily drop the three `KNOWN` entries in `tests/e2e/a11y-route-sweep.spec.ts`, run the sweep in the default theme and in `light` and `dark`, and paste into the PR every violating node, its computed contrast ratio, the failing foreground and background tokens and the owning component, including the shared tiles on `/settings`. This is measurement, not a fix, and it decides 4.1.4f's scope: a single shared-token defect (change the token) or several component defects (fix each at its source). UNVERIFIED today which it is; research/ux.md only says to check `textMuted` first.
- Files: `tests/e2e/a11y-route-sweep.spec.ts`

##### Task 4.1.4f: Fix the color-contrast defect on `/settings`, `/settings/backlog-stages`, `/settings/features` and remove their exemptions (S)
- Prerequisite step: Task 4.1.4g's recorded baseline; fix every recorded violation, token or style, at its source (not by darkening one page); remove the three `KNOWN` entries (plus any that the fix also clears). Verified present at `tests/e2e/a11y-route-sweep.spec.ts:31-37`.
- Files: `web-app/src/styles/theme.css.ts`, `web-app/src/app/globals.css`, `web-app/src/app/settings/settings.css.ts`, `web-app/src/styles/settingsListPage.css.ts`, `tests/e2e/a11y-route-sweep.spec.ts`
- **DONE (W0, commit `b48a6a119`, errata 2026-10-18)**: the baseline found five separate component defects, not one token: `globals.css` (toggle tracks: `--text-muted` is `#7d8ea8`, 3.32:1 against white, now mixed toward black), `settings.css.ts` and `settingsListPage.css.ts` (one line each), `theme.css.ts` `primaryText` (`#ffffff` to `#0a0a0a`, 3.04:1 to 6.50:1 on `#2d9cdb`) and `accentText` (`#6366f1` to `#7c83f7`, 4.22:1 to 5.79:1 on `#0f1117`). The three `KNOWN` entries are removed. `/settings/backlog-quota` does not exist yet and still needs its sweep entry in Task 6.1.1c.

##### Task 4.1.5a (owner-gated, not on critical path): Flip a code default (XS)
- Only after Q5 criteria hold with data from Story 4.1.2; change `config` constant (registry follows) and re-run the parity test.
- Files: `config/config.go`

---

## Phase 5: Workstream E — no config surface without a proof

### Epic 5.1: Registry-driven proof test
**Goal**: Adding a settable field without a runtime-effect test fails `make test`.

#### Story 5.1.1: Proof framework
**Prerequisites:** None
**As a** maintainer, **I want** reflection-enumerated surfaces with anti-gaming checks, **so that** metric 5 is structural.
**Ratcheted pending list (B3)**: the test starts green and stays green every wave because it carries a `pending` list naming EVERY existing and planned surface: all existing `QuotaConfig` fields, all existing `knownFeatureFlags` names, the four engine mode flags, `DefaultCostBudgetThresholdUSD`, the auto-merge status field, and the stage-config CRUD request fields. A story that registers a real entry deletes the matching pending line in the same change; a pending line whose surface already has an entry fails; a surface that is neither registered nor pending fails (this is what catches a new knob); the list must be empty at Story 5.1.3. ADR-006 is amended to match.
**Sequencing (C6)**: scheduled in Wave 1, before every task that registers an entry (2.1.1b in W2, 1.2.4a in W8, 5.1.2 in W12). It needs only the `QuotaConfig` struct and the `knownFeatureFlags` registry shapes, both of which exist; it starts with an entry table seeded from the pending list and the reflection enumerators for fields that already exist, so the suite is green from day one.
**Acceptance Criteria**:
- *Given* a new field `QuotaConfig.NewKnob` with no registry entry, *When* `go test ./server/services -run TestConfigSurfaceProofs` runs, *Then* it fails with "ConfigSurface quota.new_knob has no ConfigSurfaceProof".
- *Given* an entry naming `TestDoesNotExist`, *When* the test runs, *Then* it fails "proof test not found".
- *Given* an entry whose test never calls `assertConfigChangesBehavior`, *When* the test runs, *Then* it fails.
- *Given* an entry with `exempt:""`, *When* the test runs, *Then* it fails requiring a reason.
- *Given* a surface that has a real entry and also a pending line, *When* the test runs, *Then* it fails "remove stale pending entry"; *Given* a surface that is neither registered nor pending, *Then* it fails naming it; *Given* the pending list is non-empty at the Story 5.1.3 gate test, *Then* that gate test fails.
**Files**: `server/services/config_surface_proof_test.go`, `server/services/config_surface_helpers_test.go`

##### Task 5.1.1a: Registry, reflection (struct tags, `knownFeatureFlags`, `protoreflect` request fields), parser check, helper, ratcheted pending list (M)
- Files: `server/services/config_surface_proof_test.go`, `server/services/config_surface_helpers_test.go`

#### Story 5.1.2: Entries for surfaces this project adds
**Prerequisites:** 5.1.1, 1.2.4, 1.5.9
**As a** maintainer, **I want** each remaining new surface registered, **so that** nothing lands unproven.
**Acceptance Criteria**:
- *Given* quota fields, mode flags, `DefaultCostBudgetThresholdUSD`, and the auto-merge status field, *When* the proof test runs, *Then* each has an entry whose named test flips the value through the production entry point and asserts a different observable (for example `QuotaGate.Reconcile` pause decision for `PauseBelowHeadroomPct` 30 vs 70 at 40% headroom).
**Files**: `server/services/config_surface_proof_test.go`, per-story test files

##### Task 5.1.2a: Quota and budget proofs (S)
- Files: `server/services/quota_gate_test.go`, `server/services/backlog_service_budget_test.go`, registry

##### Task 5.1.2b: Auto-merge status and mode-flag proofs (S)
- Files: `server/services/backlog_stuck_rpc_test.go`, `server/services/backlog_engine_enforcement_test.go`, registry

#### Story 5.1.3: Backfill the existing stage-config surfaces
**Prerequisites:** 5.1.2
**As a** maintainer, **I want** existing CRUD fields registered or exempted with reasons and the pending list emptied, **so that** the framework ends truthful with no placeholders.
**Acceptance Criteria**:
- *Given* stage-config CRUD request fields (stage enabled/liveness, transition enabled, gate kind/config), *When* the proof test runs, *Then* each is either mapped to an enforcement test from Story 1.2.4 (edge enable/disable, gate add) or exempt with a written reason (for example display-only color), and no entry is exempt without a reason.
**Files**: `server/services/config_surface_proof_test.go`, `server/services/backlog_engine_enforcement_test.go`

##### Task 5.1.3a: Backfill mapping and any missing enforcement tests (liveness, gate kinds) (M)
- Files: `server/services/config_surface_proof_test.go`, `server/services/backlog_engine_enforcement_test.go`

---

## Phase 6: Cross-cutting verification

### Epic 6.1: E2E, registry, gates
**Goal**: The full change passes repo gates and has browser proof on both form factors without touching the live instance.

#### Story 6.1.1: Playwright specs
**Prerequisites:** 1.3.2, 1.3.3, 2.2.1, 3.1.3, 4.1.4
**As a** maintainer, **I want** specs per surface at mobile and desktop widths, **so that** UI claims are proven.
**Acceptance Criteria**:
- *Given* the isolated e2e server (global-setup, ephemeral port), *When* `tests/e2e/backlog-engine-mode.spec.ts` runs at 1280x800 and 360x800, *Then* the banner shows Shadow by default, shows "Partly enforced" after only the operator enforce flag is toggled via the Features page, changes to Enforced after all three class flags are on, and shows the Unknown copy when the status RPC is blocked.
- *Given* `tests/e2e/backlog-features-flags.spec.ts` and `tests/e2e/backlog-settings-a11y.spec.ts` (the two extra specs that cover UX-09 to UX-17 and UX-31 to UX-34), *When* run at both viewports, *Then* the switch semantics, hit areas, inert row, keyboard toggle, failure/revert, enforce dialog with readiness evidence, no-dead-end and Axe checks in `validation.md` pass; no criterion is skipped (Tasks 4.1.4a and 4.1.4d are mandatory).
- *Given* `tests/e2e/a11y-route-sweep.spec.ts`, *When* run, *Then* `ROUTES` includes `/settings/backlog-quota` with no `KNOWN` entry, and `/settings`, `/settings/backlog-stages`, `/settings/features` have no `color-contrast` exemption (Task 4.1.4f); the four-page Axe test in `backlog-settings-a11y.spec.ts` runs in light and dark through the theme helper.
- *Given* the mock helper `tests/e2e/helpers/backlog-settings-mocks.ts`, *When* a spec needs a state the real server cannot produce, *Then* it uses the helper's `page.route` fulfilments (matrix in `validation.md` UX section; route pattern `**/session.v1.<Service>/<Method>` as in `tests/e2e/pages/InsightsPage.ts:58`): status RPC per mode and `gate_wired=false`, deferred (pending) responses, a sequence mock that returns different bodies on consecutive calls (UX-45), per-procedure request counters (UX-45, UX-48), readiness-evidence variants, quota save with differing read-back, `GetQuotaConfig` with `quota_source` variants, stuck report per `AutoMergeLookupStatus` with `auto_merge_checked_at` present or absent, flag-update failure, cross-tab change.
- *Given* `backlog-quota.spec.ts`, *When* run, *Then* saving budget 0 with the gate on shows the warning, and an invalid pause/resume pair shows the inline error.
- *Given* `stuck-auto-merge.spec.ts`, *When* run with a stubbed `RepoSettings`, *Then* each of OK/true, OK/false, RATE_LIMITED, UNSPECIFIED renders its row.
- Specs start with `// @feature ...`, use `data-testid` or ARIA only, and contain no `waitForTimeout`.
**Files**: `tests/e2e/backlog-engine-mode.spec.ts`, `tests/e2e/backlog-quota.spec.ts`, `tests/e2e/stuck-auto-merge.spec.ts`, `tests/e2e/backlog-features-flags.spec.ts`, `tests/e2e/backlog-settings-a11y.spec.ts`, `tests/e2e/a11y-route-sweep.spec.ts`, `tests/e2e/helpers/backlog-settings-mocks.ts`, `tests/e2e/pages/ThemeToggle.ts`, `tests/e2e/pages/BacklogStagesSettingsPage.ts`, `tests/e2e/pages/FeaturesSettingsPage.ts`, `tests/e2e/pages/BacklogQuotaSettingsPage.ts`, `tests/e2e/pages/StuckItemsPage.ts`

##### Task 6.1.1c: Shared mocks, theme-toggle helper and sweep route (S)
- `tests/e2e/helpers/backlog-settings-mocks.ts` (Connect JSON `page.route` fulfilments for the procedures listed in the AC, including deferred responses that a test resolves explicitly, the sequence mock and request counters, no sleeps), `tests/e2e/pages/ThemeToggle.ts` (drives the real control: on `/settings` it clicks the `ThemePicker` radio whose name starts with "Light" or "Dark" inside `getByRole("radiogroup", {name: "Theme selection"})`, because the picker has no `data-testid`, then asserts `aria-checked="true"` on it and that the `<html>` class list changed; the app applies themes as a vanilla-extract class on `<html>` (`ThemeContext.tsx:223-229`), NOT as `data-theme`, so no `data-theme` assertion exists; used for UX-02/21/28/32), and the `/settings/backlog-quota` route added to `ROUTES` in `a11y-route-sweep.spec.ts`. Lands first in the story; 6.1.1a/b import it.
- Files: `tests/e2e/helpers/backlog-settings-mocks.ts`, `tests/e2e/pages/ThemeToggle.ts`, `tests/e2e/a11y-route-sweep.spec.ts`

##### Task 6.1.1d: Page objects and the keyboard/history helpers (S)
- Lands right after 6.1.1c and before 6.1.1a/b. Creates `tests/e2e/pages/BacklogStagesSettingsPage.ts`, `FeaturesSettingsPage.ts` and `BacklogQuotaSettingsPage.ts` (and extends the existing `StuckItemsPage.ts`), each locating by `data-testid` or ARIA role only (convention 4), exposing intent-level methods (open the page, read the banner text and live region, open the enforce dialog, tick the acknowledgement, save the quota form, press Back) and no assertions about counts. The on-screen keyboard is `setKeyboardOpen` from `tests/e2e/helpers/viewport-profiles.ts:56` (VERIFIED existing: overrides `visualViewport.height` and fires `resize`); history behavior uses `page.goBack()` and `page.evaluate(() => history.length)`. A real soft keyboard cannot be raised under Playwright; the manual phone leg is in `validation.md`.
- Files: `tests/e2e/pages/BacklogStagesSettingsPage.ts`, `tests/e2e/pages/FeaturesSettingsPage.ts`, `tests/e2e/pages/BacklogQuotaSettingsPage.ts`, `tests/e2e/pages/StuckItemsPage.ts`

##### Task 6.1.1a: Engine-mode and Features-flag specs (M)
- Includes the UX-43 to UX-48 and UX-55 cases (dialog contract, evidence states, re-check, independent flags, deep-link robustness, pending semantics, review line).
- Files: `tests/e2e/backlog-engine-mode.spec.ts`, `tests/e2e/backlog-features-flags.spec.ts`

##### Task 6.1.1b: Quota, auto-merge and cross-surface accessibility specs (M)
- Includes the UX-49 to UX-54 cases (quota live regions and mismatch, not-configured, leave-guard history, StageForm reminder states, auto-merge details, hit areas); UX-26 is asserted by outcome (Applied, no dialog or navigation, persisted after reload), not by counting helper internals.
- Files: `tests/e2e/backlog-quota.spec.ts`, `tests/e2e/stuck-auto-merge.spec.ts`, `tests/e2e/backlog-settings-a11y.spec.ts`

#### Story 6.1.2: Repo gates and manual verification
**Prerequisites:** 6.1.1, 5.1.3, 4.1.1, 1.4.1 (everything else merged)
**As a** maintainer, **I want** `make ready` green and a scripted manual-instance walkthrough, **so that** nothing ships on assertion and no agent restarts the live service to verify.
**Acceptance Criteria**:
- *Given* the branch, *When* `make registry-generate` then `make ready` run, *Then* both succeed (lint, dupl, gocyclo/gocognit, jscpd within 0.14%), and the output is pasted into the PR.
- *Given* `scripts/verify-manual-instance.sh 1`, *When* run, *Then* it builds to `~/.stapler-squad/manual-builds/manual-1/stapler-squad`, refuses to start unless `PORT`, `--remote-port`, `STAPLER_SQUAD_INSTANCE` and `STAPLER_SQUAD_TMUX_SOCKET` are set and differ from the live values (`:8543`, `:8444`, the default socket), refuses when `ss -ltnp` shows 62871/62872 taken, starts the instance with `--tmux-keep-server`, and on exit kills only its own socket (`tmux -L ssq-manual-1 kill-server`) and its own process. *When* the operator edits a stage and toggles flags there, *Then* the Stages banner and `/actuator/workflow-engine` agree; the live `:8543` service is never restarted.
- *Given* the script's dry-run mode (`--check`), *When* run with a missing variable, a variable equal to a live value, or an occupied port, *Then* each case exits non-zero with the reason (tested without starting a server).
**Files**: `docs/registry/features/*.json`, `web-app/.jscpd.json` (only if the owner approves a ratchet change)

##### Task 6.1.2a: `make registry-generate`, `make ready`, fix collateral lint/test debt found (M)
- Files: `docs/registry/features/*.json`

##### Task 6.1.2b: Manual-instance verification notes in the PR (XS)
- No new files; evidence pasted into the PR description.

---

## Effort estimate

*(Bands and weights INFERRED; recalibrate after the first implementer run. Raw tokens use class midpoints XS=20k, S=55k, M=130k. Assumed mix for implementation runs: output 12%, cache read 50%, fresh input 38%, so CU per raw token = 0.38 + 0.1x0.5 + 5x0.12 = 1.03. Table regenerated by script from the task headings in this file on the repair date; rerun the script after any task is added, split or resized.)*

| Story | Tasks | Cost classes | Est. raw tokens | Output share | Est. CU | Wall-clock blocker |
|---|---|---|---|---|---|---|
| 0.1.1 Characterize current behavior | 4 | XS/S | 150k | 12% | 154k |  |
| 1.1.1 Gate-equivalence harness (written first) | 2 | S/M | 185k | 12% | 191k |  |
| 1.1.2 SwitchableWorkflowEngine and the enforce composition | 5 | S/M | 425k | 12% | 438k |  |
| 1.1.3 Shadow recorder, flags, actuator, soak target | 6 | XS/S/M | 445k | 12% | 458k |  |
| 1.1.4 Wiring and wiring test | 3 | S | 165k | 12% | 170k |  |
| 1.2.1 System-required edges cannot be disabled | 3 | S | 165k | 12% | 170k |  |
| 1.2.2 A denied automatic claim produces a visible stuck reason | 3 | S/M | 315k | 12% | 324k |  |
| 1.2.3 Status-writer inventory and counters (guard written first) | 2 | S/M | 185k | 12% | 191k |  |
| 1.2.4 Integration proofs (Success Metric 1) | 1 | M | 130k | 12% | 134k |  |
| 1.3.1 `GetWorkflowEngineStatus` RPC | 3 | S/M | 240k | 12% | 247k |  |
| 1.3.2 Stages page mode banner | 7 | XS/S/M | 425k | 12% | 438k |  |
| 1.3.3 Mode flags on the Features page | 4 | XS/M | 410k | 12% | 422k |  |
| 1.4.1 Runbook | 2 | S | 110k | 12% | 113k | soak/owner (see blockers) |
| 1.5.1 Caller registry and edge audit | 3 | S/M | 315k | 12% | 324k |  |
| 1.5.2 Class-aware selector, `ModeFor(class)` and shadow baselines | 4 | S/M | 370k | 12% | 381k |  |
| 1.5.3 `Storage.GatedTransition`, the choke point | 5 | XS/S/M | 315k | 12% | 324k |  |
| 1.5.4 Close the raw writers | 7 | S/M | 760k | 12% | 783k |  |
| 1.5.5 Migrate operator-class and agent-class writers | 5 | S/M | 500k | 12% | 515k |  |
| 1.5.6 Migrate system writers in `server/services` (and the two operator writers in the triage files) | 8 | XS/S/M | 480k | 12% | 494k |  |
| 1.5.7 Migrate system writers in `session` | 3 | S/M | 315k | 12% | 324k |  |
| 1.5.8 Denied automatic transitions neither loop nor strand | 7 | S/M | 760k | 12% | 783k |  |
| 1.5.9 Strict guard and Metric 1 proofs for every caller class | 4 | S/M | 370k | 12% | 381k |  |
| 2.1.1 Quota config RPC | 5 | S/M | 425k | 12% | 438k |  |
| 2.1.2 Gate resumes on disable; legible status text | 2 | XS/S | 75k | 12% | 77k |  |
| 2.1.3 Global soft-budget default | 1 | S | 55k | 12% | 57k |  |
| 2.2.1 Backlog quota settings page | 4 | S/M | 370k | 12% | 381k |  |
| 3.1.1 `RepoSettings` in package `github` | 3 | S/M | 315k | 12% | 324k |  |
| 3.1.2 Populate in the service layer | 1 | M | 130k | 12% | 134k |  |
| 3.1.3 StuckItemDetail states | 2 | S/M | 185k | 12% | 191k |  |
| 3.1.4 Pre-PR warning without noise | 2 | S | 110k | 12% | 113k |  |
| 4.1.1 Promote ADR-005 | 1 | S | 55k | 12% | 57k |  |
| 4.1.2 Durable nudge-outcome counter | 2 | S/M | 185k | 12% | 191k |  |
| 4.1.3 One declaration per default | 1 | S | 55k | 12% | 57k |  |
| 4.1.4 Features page legibility, accessibility rework, shared confirm dialog and contrast | 7 | S/M | 610k | 12% | 628k |  |
| 5.1.1 Proof framework | 1 | M | 130k | 12% | 134k |  |
| 5.1.2 Entries for surfaces this project adds | 2 | S | 110k | 12% | 113k |  |
| 5.1.3 Backfill the existing stage-config surfaces | 1 | M | 130k | 12% | 134k |  |
| 6.1.1 Playwright specs | 4 | S/M | 370k | 12% | 381k |  |
| 6.1.2 Repo gates and manual verification | 2 | XS/M | 150k | 12% | 154k |  |

**Sum of task estimates** (script output, rerun after any task change): 132 sized tasks (54 M, 69 S, 9 XS), 10.99M raw tokens (10,995k), 11.32M CU (11,323k, sum of per-story rounded rows). **Verification multiplier x1.75** (spec + quality review per story, 6-verify layers, CI reruns): **19.82M CU** (19,815k). Task 4.1.5a (owner-gated flip, XS, about 20k raw) is excluded from the sum (133 task headings in all). **Repair round 7 re-sized again** (script output): +8 tasks (1.3.1c, 1.3.2g, 1.3.3c, 1.3.3d, 2.2.1d, 3.1.3b, 4.1.4g, 6.1.1d), Tasks 1.3.3b and 2.2.1c grow S to M; Epic 1.5 is unchanged at 46 tasks, 4,185k raw, 4,309k CU, 7,541k after the multiplier (38.1% of the new total). The round-6 figures that follow are historical. **Repair round 6 re-sized again** (script output, `plancheck`): +10 tasks net (0.1.1d moved here from 6.1.2c, 1.3.1b, 1.3.2f, 1.3.3b, 1.4.1b, 1.5.6h, 2.1.1e, 2.2.1c, 4.1.4e, 4.1.4f, 6.1.1c), Task 1.3.3a grows S to M, Epic 1.5 is 46 tasks, 4,185k raw, 4,309k CU, 7,541k after the multiplier (40.4% of the total). Planning overhead already spent is excluded. **Size band: this exceeds the Large band (3-15M CU) by about 4.8M CU (32%), so the honest label is "above the Large ceiling"**; `requirements.md` says Large (INFERRED) and is annotated. Repair round 4 added 12 tasks to Epic 1.5 (32 to 44; 3.04M to 4.11M raw, 4.23M CU, 7.41M CU after the multiplier, about 43% of the total): the six-writer closure and test/fake rename (1.5.4 grows from 3 tasks to 7), the analyzer extension (1.5.4f), the `create` constraint (1.5.4g), the overrun checkpoint (1.5.3e), the detector guards split three ways plus the predicate task (1.5.8 grows from 3 tasks to 6), three named Extract-first tasks in the hotspot (1.5.6b-d), the operator writers moved into Story 1.5.6 (1.5.6e) and the live-database replay (1.5.9d). **Repair round 5 re-sized Epic 1.5 again**: 44 to 45 tasks, 4,110k to 4,165k raw (4,289k CU, 7,506k after the multiplier, 42.9% of the total). Story 1.5.8 goes from 6 tasks and 705k raw to 7 tasks and 760k raw: the five detector rows the table lacked (`dequeue_backlog_items`, `blocked_by_dependency`, `trigger_chain_reconcile`, `pr_ready+merge_detection`, `pr_drift_recovery`) and the quieter dequeue logging add work, the spawn-point collapse (no single item-level dispatch exists, so the guard is written once in each of the spawn-point methods rather than once per detector) removes some, and the `-enforce-system` flag registration is the new Task 1.5.8g (S, 55k). Tasks 1.5.2b, 1.5.3c and 1.5.4f change content, not size class. Informational, not a gate: the owner pursues full scope and no cut line is proposed. The x1.75 multiplier absorbs some residual per-task optimism; the 25% overrun checkpoint (Task 1.5.3e) is the real control.

**Story 4.1.4**: all six tasks are mandatory. Tasks 4.1.4a and 4.1.4d (switch semantics, failure states, 185k raw) back UX-09/10/13/14/17; 4.1.4e (`AlertConfirmDialog`) and 4.1.4f (contrast fix) were added in round 6.

**Critical path (agent waves)**: 21 waves (W0 to W20), at most 2 stories per wave, as scheduled in "Dependency Visualization"; W10 (the choke point, which touches `server/dependencies.go`), W11 (the raw-writer closure, which touches 14 production files and 31 test files) and W20 (final verification) hold one story each. The A chain (1.1.1, 1.1.2, 1.1.3, 1.1.4, 1.2.1, 1.2.2, 1.2.3, then Epic 1.5 in W8-W15, plus 1.2.4, 1.3.1, 1.3.2, 1.3.3, 1.4.1) occupies at least one slot in every wave from W1 to W17 except W11; B, C, D and E fill the second slot where dependencies allow. Files that serialize are listed under the wave table.

**Overrun checkpoint**: owned by Task 1.5.3e, repeated in the W10 wave row and the Worker brief standard block. Trigger: cumulative actual CU of stories completed in W0-W10 exceeds their table values by more than 25%, Story 1.5.3 alone exceeds its value by more than 25%, any one story exceeds its value by more than 50%, or per-story actual usage cannot be read. Action: record actual versus table, post the delta with its cause (wrong estimate versus real surprise) and do not dispatch W11 until the owner replies; with no trigger, record and proceed. It sits after W10 because the migration waves (1.5.5 to 1.5.7 = 1.28M raw) and the recovery story 1.5.8 scale with what the gate stories reveal. Informational, not a cut line. The general rule (25% cumulative) also applies at the end of every later wave.

**Wall-clock blockers**:
- Soak parameters (W = 7/7/14 days for operator/agent/system, N = 20 and floor 100 as targets only, review date 14 days after first deploy) and the low-volume READY rule (four-part acknowledgement, Risk Control) are owner-approved 2026-10-17 and are NOT a blocker for W0. Still owner-run: `sqlite3 <live db> ".backup <copy>"` to produce the copy that Task 1.5.9d replays (the agent never opens the live database), and the dated review at first deploy + 14 days (requirements Metric 6).
- Per-class soak windows run concurrently from the deploy of Epic 1.5 (every class is in Shadow from day one), so the earliest all-enforced date is about 14 days after deploy plus the flip steps; flips are sequential (operator, agent, system) with a recommended 3-day observation between flips (INFERRED).
- Owner acknowledgement of the stranded-item preview, the `UNVERIFIED in shadow` edges and each divergent `(caller,from,to)` per class at flip time: Tyler.
- Owner decision on Q5 exit criteria and whether to flip any code default (Task 4.1.5a): Tyler.
- Soak window for the nudge flags after Story 4.1.2 ships (14 days INFERRED) before Q5 can be evaluated: Tyler.
- Enablement dates of the three live flags (not recorded): Tyler or flag-service audit lines.
- CI queue and Playwright runs for `make ready`/e2e: CI.
- Human review latency on PRs: Tyler.
- Deploy of the new binary to the live instance (the agent never runs `make install-service`): Tyler, after confirming the unit passes `--tmux-keep-server`.

---

## Repair log

Repair date 2026-10-17. Each finding ID is from `adversarial-review.md` (B/C/M) or `architecture-review.md` (AB = architecture blocker, AC = architecture concern, by order of appearance). Cites were re-opened against the current tree; where a reviewer cite was wrong or imprecise it is noted.

**Blockers**
- **B1 / AB1 (shadow compares raw Configured)**: fixed. `enforceAnswer` is implemented once and shared by `Enforce` and `Shadow`; divergence is Default versus `enforceAnswer`; raw-Configured comparison kept only in the Story 1.1.1 offline harness as informational `gate_builtin_absent_in_config`. Rewrote ACs in Stories 1.1.1, 1.1.2, 1.1.3, 1.4.1, ADR-001 (modes table, divergence classes), Observability alerts, and the `make workflow-engine-soak-status` criteria (alert classes `graph|gate_missing|gate_extra|error_kind` under `config=seeded`; engine-routed `path=transition` only). Added Task 1.1.2c (composition) and 1.1.2d (composed parity).
- **"No I/O" contradiction (AC1)**: fixed. Verified `GetByItemAndGate(context.Background(), ...)` at `session/configured_workflow_engine.go:284,374`. Shadow skips stateful gate kinds (`not_compared{reason=stateful_gate}`, no repo call asserted by a fake); Enforce keeps evaluating them with a 2s fail-closed timeout (Task 1.1.2e). ADR-001 carries the correction.
- **B2 / C5 (override_reason under Enforce)**: fixed as a decision (ADR-001): non-empty reason bypasses Configured operator gates including the synthetic deleted-stage gate, never the graph; Default guards receive the input unchanged (W0 errata 2026-10-18 corrected the earlier wording "bypasses Default guards", which widened today's behavior; see the errata entry below). Verified: handler sets `OverrideReason` and always calls `ValidateGates` (`backlog_service_lifecycle.go:929-933`); `Configured.ValidateGates` ignores it. Added ACs (enforce + failing operator gate + override; enforce + deleted-stage item + override; empty reason blocked; override never bypasses graph) in Stories 1.1.2 and 1.2.4. Removed the Unresolved Question; Task 0.1.1a now characterizes Default only and no longer blocks 1.1.2.
- **B3 / C3 (transitionOrNotify layer and cites)**: fixed. Re-derived with `git grep`/reading: engine-deniable automated callers are `transitionWithGuard` (`backlog_service_triage.go:680`), `queueBacklogItem` (`:699`/`:716`), `claimAndSpawnCandidate` (`:837`/`:839`), `JulesDispatchService` (`jules_dispatch_service.go:361`), `SyncLoop` (`backlog_sync.go:449`). Only `claimAndSpawnCandidate` swallows a denial (`default:` arm `:872-873`). Old cites `:801-810`, `:2313-2321`, `backlog_lifecycle.go:971-984` confirmed not transition sites; `backlog_lifecycle.go:1971-1975` is a storage CAS, so out of scope. Reviewer note corrected: `:1318` is in `OverrideVerdict`, not `TriggerReReview`. Helper kept in `server/services` and scoped to service-layer callers (no cycle); AC rewritten around a denied `queued->in_progress` claim, with sweep dedupe and recovery. Tasks 1.2.2a-c rewritten (typed `ErrTransitionNotAllowed`, Extract `handleClaimError` first).

**Concerns**
- **C1 (Metric 1 honesty)**: fixed. New "Success-metric scoping" section: met for RPC/MCP/SyncLoop engine-routed paths, only once enforce is on; default stage edits change nothing; proven by 1.2.4 plus 1.1.4. `requirements.md` wording change is flagged, not edited (owner approval).
- **C2 (bypass inventory)**: fixed. Recount by `git grep`: 38 sites in 14 files = 1 wrapper + 1 MCP delegate + 5 engine-routed + 31 bypass in 10 files. Inventory is repo-wide, generated first (1.2.3b); instrumentation split into 1.2.3c (S), 1.2.3d triage (M), 1.2.3e lifecycle/PR/Jules (M); rollbacks `exempt`.
- **C4 (soak evidence)**: fixed. Criterion over engine-routed edges only; `path=transition|read` label; `SeedFingerprint` now covers edges, stages, gate counts and liveness (AC3 / architecture "stale fingerprint"); one-shot `--config-diff` (Task 1.1.3f); W and N flagged as owner decisions to make before the soak starts.
- **C6 / AC2 (registry sequencing)**: fixed. Story 5.1.1 moved to Wave 1, before 2.1.1b, 1.2.4a and 5.1.2.
- **C7 / AC3 (waves, WIP cap, dependencies.go)**: fixed. One wave count (16, W0-W15), max 2 stories per wave, generated from the dependency graph; per-wave serialization rules for `dependencies.go`, `backlog.proto`, `features/page.tsx`, `feature_flag_service.go`.
- **C8 (sizing)**: fixed. Split or upsized 1.1.2 (a, c, d, e), 1.2.3, 3.1.1 (b, c), 4.1.4 (a, d); table regenerated by script from task headings.
- **C9 (4.1.4 scope)**: fixed in round 2 by splitting the story into requirement-tied tasks and tasks 4.1.4a and 4.1.4d; SUPERSEDED in round 6: all of Story 4.1.4 is mandatory (full scope), the 'optional, cut first' framing is withdrawn.
- **C10 (soft budget log-only)**: fixed. Stated in Story 2.1.3, ADR-004, ux.md copy, Observability; routing to notify is an Unresolved Question (owner).
- **AC4 (SystemRequiredEdges derivation)**: fixed. Both groups; added `ready->queued`, `queued->in_progress`, `ready->in_progress`, SyncLoop `*->archived`, `AttachSessionToItem`; the Story 1.2.4 proof edge changed from `ready->queued` to non-system `done->refining` (confirm in Task 1.2.1a).
- **AC5 (gocognit/gocyclo on modified functions)**: fixed for 1.2.2b and 4.1.3a (extract before modifying, run `make ready-complexity-gate`; 1.2.3d adds single-line instrumentation only).
- **AC6 (RepoSettings illegal state)**: fixed. Unexported bool behind `AutoMerge() (allowed, ok)`; proto field unset unless status OK, test asserted (Stories 3.1.1, 3.1.2, ADR-003).
- **AC7 (concurrent config.json writers)**: fixed as a decision: `config.UpdateConfig` helper with lost-update test (Task 2.1.1c, ADR-004). Residual: other `SaveConfig` callers remain unlocked (stated).
- **AC8 (banner reads separately cached mode)**: noted in Story 1.3.1 and the runbook (TTL about 1s).
- **Nitpicks**: negative/NaN/Inf budget validation AC added (Story 2.1.1); both budget evaluation sites (`backlog_service.go:1457-1460`, `:1482`) covered (Story 2.1.3); wave line says 2; task count recomputed; listener-misuse guard for `backlog_lifecycle.go:1092,1116` added (Story 1.1.4); `soak.go:207` cite corrected in Task 1.1.3a.
- **UX gaps**: 1.3.3 (keyboard-reachable Revert to Off / Edit stages), 4.1.4 (load-failed, toggle-failed, loading -> UX-14, UX-17, Task 4.1.4d), 2.2.1 (load-failed/loading UX-23, `inputMode=decimal` UX-24, Features->Quota link via `flagExtras` UX-18), 1.3.2 (`stage-save-status` testid), 3.1.3 (checking, stale, OK+true caveat -> UX-27/28) folded into the stories. Quota "Unsaved changes" line kept and listed as an owner-confirmable decision.
- **Residual re-review round**: (1) `config.UpdateConfig` now specifies a separate per-path outer mutex, never `saveConfigLockFor` (verified: `SaveConfig` takes it at `config/config.go:1502`, so reuse self-deadlocks); concurrent lost-update AC added to Task 2.1.1c; ADR-004 updated. (2) ADR-002 section 3 fused sentence rewritten: `done->refining` is the non-system edge the Story 1.2.4 proof disables; Group 1 and 2 edges are system-required. (3) R1 READY defined for edited config (window spanning an edit is `edited`); R2 stateful-gate edges listed `UNVERIFIED in shadow` in soak-status and the runbook; R3 alarms fire on either path, READY counts `path=transition` only (Stories 1.1.3, 1.4.1, Task 1.1.3b); R4 `Prerequisites:` line added to every story, waves unchanged. (4) Story 1.2.3 `OverrideVerdict` cite file-qualified to `backlog_service_lifecycle.go:1332`; effort text A-chain wave span corrected (W1-W11 and W13, none in W12). (5) Three owner-confirm items added to Unresolved Questions.

**Deferred / not changed**
- ~~`requirements.md` Metric 1 wording~~: resolved by the owner decision of 2026-10-17 (keep it strong; round 3 below).
- Migrating other `config.SaveConfig` callers onto `UpdateConfig` (out of scope, residual risk stated).
- Routing the soft-budget crossing to a notification (owner decision, Unresolved Questions).
- Reviewer claim "`backlog_lifecycle.go:1971-1975` matches" was not adopted: it is a storage CAS the graph cannot deny.

### Repair round 3 (2026-10-17): owner decision on Success Metric 1, pre-mortem P1-P3, consistency review

**Owner decision (binding)**: Metric 1 is NOT reworded. Editing a stage, transition or gate must change the result for ALL status-transition paths. Applied as follows.
- New [ADR-007](../decisions/ADR-007-engine-gated-transition-choke-point.md) supersedes ADR-002 D1 ("measure, don't govern"); ADR-001 and ADR-002 carry amendment/superseded notes. Choke point `Storage.GatedTransition` (+ `GatedArchive`, `GatedUnarchive`, `GatedSetPRAndTransition`, `GatedReconcileStuck`, exempt `Rollback` and `DebugForce`); raw primitives renamed `Raw*` so a stray call is a compile error; closed `TransitionCaller` registry with class, legacy policy, guard policy and declared edges.
- Population re-derived by command: 38 `TransitionBacklogItemStatus` sites (1 wrapper, 1 delegate, 5 engine-routed, 31 direct) PLUS 9 sites of four status-write primitives the earlier inventory missed (`ArchiveBacklogItem` 3, `UnarchiveBacklogItem` 1, `SetBacklogItemPRAndTransition` 4, `ReconcileStuckItems` 1). 40 to migrate, 5 to re-point, 5 of the 40 exempt through named entry points (3 rollbacks, 2 debug).
- Found while designing (VERIFIED `session/domain/backlog.go:514-574`): `validTransitions` has no `in_progress|review|pr_pending->archived`, but `ArchiveBacklogItem` archives from any status; routing archive through the graph would deny real items. Resolved by registry edge-audit (Task 1.5.1b) and a table/seed reconcile (Task 1.5.1c).
- Caller classes `operator`, `agent`, `system` with an explicit policy table (graph, Default guards, operator gates, `override_reason`, denial surfacing); operator gates DO apply to `system`, Default built-in guards do not by default (justified in ADR-007 D3). Denials of `agent`/`system` transitions go through an injected `TransitionDeniedReporter` inside the gate (never swallowed); `transition_denied` is remediation-blocked and re-driven (Story 1.5.8).
- Per-class rollout: four live flags (`-shadow`, `-enforce`, `-enforce-agent`, `-enforce-system`), per-class soak criteria and counters, recommended flip order operator, agent, system. Rationale in ADR-007 D8.
- New Epic 1.5 (9 stories, 32 tasks, waves W8-W15): 1.5.1 registry and edge audit; 1.5.2 class-aware selector and flags; 1.5.3 gate; 1.5.4 close raw primitives; 1.5.5 operator and agent writers; 1.5.6 system writers in services; 1.5.7 system writers in session; 1.5.8 denied-transition recovery; 1.5.9 strict guard and per-class Metric 1 proofs. Story 1.2.3 rewritten (inventory with ratcheted pending list; tasks 1.2.3c-e removed, their sites now migrate in Epic 1.5); Story 1.2.4 now depends on 1.5.5; former optional Task 1.2.5a became the required first commit 1.5.5e.
- `validation.md`: removed `TestBypassSites_RemainOnBuiltInGraph_WhenEnforceOn` and `TestTransitionBacklogItemStatus_DefaultFlagsChangeNothing_WhenStageEditedBeforeEnforceFlip` (they assert the weak reading); added per-class Metric 1 proofs for operator, agent and system and an every-registered-caller table. `requirements.md`: Metric 1 text untouched; added an owner-decision note and aligned Scope A, the Rabbit Hole bullet and Risk Control.
- Risks analysed in ADR-007 (table): stricter graph stranding the autonomous pipeline (`onAutonomousDriverComplete` role-to-status mapping), CAS and TOCTOU (derived `ExpectedStatus`, bounded `Unconditional` retry, `BeforeCommit` order), hot-path latency (benchmark ceiling, `recover()`), live-service restart.

**Pre-mortem (all 4 P1, 6 P2, 3 P3 addressed)**
- P1 #1 vacuous or unobserved READY: fixed. Soak AC and Task 1.1.3b: zero calls exits 1, total-sample floor, ledger history plus `binary_started_at` span at least W, expected-edge list needs N samples or owner acknowledgement; mode-resolver-returns-Shadow-with-no-flags test and a no-plain-`GetFeatureFlag` test (Tasks 1.1.3c, 1.1.3d); extended per class (Task 1.5.2d).
- P1 #2 stranded in-flight items: fixed. `--config-diff` stranded-item preview (Task 1.1.3f, now M); new Task 1.2.1c rejects edits that leave a populated status with no outgoing edge; runbook requires owner acknowledgement per previewed item (Story 1.4.1).
- P1 #3 `config.json` clobber: fixed. New Task 2.1.1d moves `FeatureFlagService` writes onto `config.UpdateConfig`; race test flag flip versus `UpdateQuotaConfig` under `-race`; VERIFIED `SaveConfig` is already atomic (unique temp file plus rename, `config/config.go:1514-1555`), added `Sync()` before rename; ADR-004 and the residual-risk text updated.
- P1 #4 agent restarts live service: fixed. "Worker brief standard block" with forbidden commands; Task 6.1.2c `scripts/verify-manual-instance.sh` (`manual-builds/manual-<N>`, distinct ports/instance/socket, `ss -ltnp` precheck, own-socket teardown, `--check` test); runbook first line confirms `--tmux-keep-server`.
- P2 #5 shadow latency/panic: `recover()`, atomic counters, 1-in-10 read sampling, benchmark (Tasks 1.1.2a, 1.1.2d). #6 ledger: ring of 200, closed-enum labels, `config.GetConfigDir()`, fresh-start on corruption (Observability, Task 1.1.3a). #7 lookup: 2s timeout, per-host breaker, cached-or-UNSPECIFIED stuck report (Task 3.1.1c, ADR-003). #8 flag-default parity through the production reader (Task 1.1.3d). #9 `manual-<N>` slot per concurrent story (wave table). #10 W/N/floor/review date proposed in Risk Control for owner acknowledgement before W0; Definition of Done for Metric 1 stated.
- P3 #11 generated-code guard and explicit staging, #12 `make ready-complexity-gate` and jscpd per story (Worker brief standard block). #13 latent system-required hole: registry edge-audit test (Task 1.5.1b).

**Consistency review (3 BLOCKER, 6 CONCERN, 12 NITPICK)**
- B1, B2: resolved by the owner decision; `requirements.md` Metric 1 kept, Scope A and the Rabbit Hole bullet now consistent with routing every path; plan text that said otherwise removed.
- B3 proof-test ordering: fixed. Story 5.1.1 ships a ratcheted pending list naming every existing and planned surface (QuotaConfig fields, the four mode flags, `DefaultCostBudgetThresholdUSD`, auto-merge status, stage-config fields); stories delete their line as they register; empty at 5.1.3; ADR-006 amended.
- C1 field name: one name, `DefaultCostBudgetThresholdUSD`, in plan, ADR-004, ADR-006, validation. C2: `ux.md` and Story 2.2.1 now say pause and resume are 1-100 (VERIFIED: the server rule covers both, defaults 20 and 15; the earlier plan text "15" for pause was wrong and is corrected). C3: Tasks 1.3.2d (UX-07) and 1.3.2e (StageForm save failure). C4: Task 3.1.4b async cache warm. C5: UX-09, 10, 13, 14, 17 marked conditional in `ux.md` and `validation.md`. C6: Task 2.1.2b quota startup line.
- NITs 1-12: all fixed (Metric 3 wording and ADR-003 `backoff` label; NFR caveat; `flagExtras` prerequisites on 1.3.3 and 2.2.1; ADR status wording; `Task 1.3.2b` cite in `ux.md`; ADR-003 task cross-reference; Unresolved Questions deduplicated; workstream count 5; mode casing note in the glossary; AC reworded to `AutoMerge() == (false, true)`; sentinel package named; preflight copy asserted in Story 3.1.4). Validation: Story 6.1.1 Files now list the two extra specs and four page helpers.

**Recounts by command (plan.md after this round)**: 12 epics (`grep -cE '^### Epic'`), 39 stories (`grep -cE '^#### Story '`), 102 task headings (`grep -cE '^##### Task '`; 101 sized plus 4.1.5a; 43 M, 52 S, 7 XS), 21 waves W0-W20 with at most 2 stories each and 0 prerequisite violations (script over the wave table and `Prerequisites:` lines), effort 8.83M CU, 15.45M CU after the x1.75 verification multiplier, which is at or just over the Large ceiling of 15M (stated in the Effort estimate; no cut line).

**Still deferred / owner decisions**: owner reading of the system-required carve-out; operator-gate policy for the `system` class and no-override for agents; soak parameters and review date; remaining `config.SaveConfig` callers; routing the soft-budget crossing to a notification; the pre-existing autonomous-completion AC-gate gap (kept, not widened).

### Repair round 4 (2026-10-17): Epic 1.5 re-review (architecture BLOCKED 3 blockers / 7 concerns; adversarial BLOCKED 3 blockers / 5 concerns)

Owner decision stands: Success Metric 1 stays STRONG. Every claim below was re-checked against the tree with `git grep` or by reading the code before acting; where a reviewer claim was wrong or imprecise it is noted.

**Blockers fixed**
- **ARCH-B1 / ADV-RB1 (rename alone does not close raw writers)**: fixed. Six raw writers, not five (added `TransitionBacklogItemStatusWithPRFields`, `ent_repository_backlog.go:1951`, reached only from `storage.go:1218`). The five `EntRepository` writers are unexported (`raw*`; `ReconcileStuckItems` split into a read-only `FindStuckInProgressCandidates` plus per-item gated writes), the `Storage` wrappers renamed `Raw*` (they stay exported because tests in other packages need them, which ADR-007 D1 now says instead of claiming a compiler wall), the strict guard (Task 1.5.9a) matches old and new names on ANY receiver, `julesPollerStorage` (`jules_session_poller.go:80-88`) and both `fakeJulesPollerStorage` types are declarations accounted for in Tasks 1.5.4e and 1.5.7c. Test rename scope recounted by command: 31 files / 205 matching lines (199 selector calls in 29 files plus 4 fake method definitions in 2 files); 28 calls on repo receivers in 7 `session` files plus one in another package that I found and the reviewers did not (`server/services/backlog_item_event_publisher_test.go:258`, `repoA.TransitionBacklogItemStatus` on an `*EntRepository`, rewritten in Task 1.5.4e). `CreateBacklogItem` is an explicit exempt `create` class (ADR-007 D1, Task 1.5.4g); the reviewers proposed a runtime rejection of non-entry statuses, which I rejected after counting about 300 test fixtures in 93 test files that create items at non-entry statuses, so the constraint is a static inventory rule plus a named `CreateBacklogItemForDebug`. Re-verified no other status writer: `SetStatus(` on `BacklogItem` builders only at `ent_repository_backlog.go:452,1484,1559,1753,1977` and `storage_backlog.go:1006` (`:2650` is a progress note); other update builders set no status; no raw SQL.
- **ARCH-B2 (`silenttransition` goes dark)**: fixed. New Task 1.5.4f extends `tools/lint/silenttransition/analyzer.go:63` to the `Gated*` and `Raw*` names, accepts a `Report` call as surfacing, updates both testdata files, adds the failing-then-passing log-only fixture, re-reads the 8 annotations on status-write calls (19 `//nolint:silenttransition` lines in all, 11 on `UpdateItemSessionEnded`), and requires `make lint-custom`; Task 1.5.9a removes the old names. ADR-007 D5 no longer says "unchanged".
- **ARCH-B3 (false `onAutonomousDriverComplete` mapping)**: fixed. VERIFIED: `toStatus` is assigned once (`autonomous_orchestration_service.go:453`, triage, `idea->ready`); the work role deliberately does not transition; the review role only closes a stuck row or ends the `ItemSession`; the `toStatus == Review` branch (`:604`) is dead and a comment at `:596-598` is stale (removed in Task 1.5.6a). Stories 1.5.1 AC 5 (set `{ready}`), 1.5.6 AC 1, 1.5.8 ACs 2-3, the ADR-007 risk table and Task 1.5.6a rewritten around the real mapping. Story 1.5.8 now enumerates every detector and respawner against the status scope it reads (`runStuckDetector` names, `remediationActionByReason`, the inline `FindReviewItemsWithoutGate` respawn, the graceful-shutdown immediate respawn in `orphaned_triage`), with one shared predicate `HasOpenTransitionDenied` (true only when the denial's `from` equals the item's current status), one required test per row, and a completeness test that fails on an unclassified new detector. `MaybeTriggerTriage` is a creation-time one-shot (five callers), not a repeating respawner as the review implied; it still gets a row and a test. Status scopes marked VERIFIED were read from the code; two table rows (`rework_blocked_stale` with `respawn_blocked_active`, and `orphaned_agent_pr` with `push_failed` and the PR paths) are marked "confirm" and are settled by reading the `Find*` query in Tasks 1.5.8c and 1.5.8d. The same hazard for `ReconcileStuck` `in_progress->review` has its own AC and test.
- **ADV-RB2 (archive edges in `validTransitions`)**: fixed by NOT changing the table. The archive edges (`X->archived` for every status including custom slugs, and `archived->archived`) join `SystemRequiredEdges()` (ADR-007 D4/D7, Task 1.5.1c); the table, seed, `SeedFingerprint` and `AllowedTransitions` output are unchanged, `workflow_engine_test.go:22` still passes, a stale-seed test proves a database without the edges still archives under every class, and the table-change alternative is Unresolved Question 1's alternative B. Corrections to the reviewers: `ArchiveBacklogItem` itself does not end sessions (it sets `archived_at`, status and the audit event); the side effects they worried about are in callers (push-before and worktree cleanup in the Archive RPC, link-session ending in `reportDuplicate`, the terminal-session sweep), so the AC is that `GatedArchive` keeps push-before via `BeforeCommit`, runs cleanup only after a successful write and runs none of it on denial. `archived->archived` is covered by the system-required set. Follow-on I found: other ACs used "operator disables `done->archived`" (Stories 1.5.5, 1.5.7), which contradicts D4 because `archiveStaleDoneItems` and `reportDuplicate` make `*->archived` system-required; those ACs now use an operator GATE on `done->archived`.
- **ADV-RB3 (agent proof contradicts D4)**: decided. The generic MCP `transition_backlog_item_status` delegate is `forwarding` and EXEMPT from system-required edges (governed by the graph like an operator move); only purpose-specific tools (`request_review`, `report_blocked`, `resume_work`, `report_duplicate`, `report_pr_created`) register fixed edges. Registry rules, Stories 1.5.1/1.5.5/1.5.9, ADR-001/002/007, `SystemRequiredEdge` glossary row and `validation.md` updated; the edge audit is non-vacuous (non-forwarding agent/system entries must have non-empty edges that are system-required, and the forwarding set is a pinned singleton).

**Concerns fixed**
- Gate carries the whole `BacklogItemPrecondition` (`repository.go:730`: `ExpectedStatus`, `ExpectedUpdatedAt`, `Note`) plus `TriggeredBy`; `ExpectedUpdatedAt` is set from `Observed.UpdatedAt` only when prebuilt `Guard` input is supplied (status-only CAS otherwise, to avoid spurious failures); Task 1.5.3a, Story 1.5.3 AC 3, ADR-007 D6.
- `ReconcileStuckItems` split preserves the "[auto]" line on `Notes` (`storage_backlog.go:996-1001`, which is the item `Notes` field, not the event `Note`) and exactly one status event and one `ChangeStatusTransition` publish per item; per-item atomicity replaces single-transaction atomicity and the PR says so (Task 1.5.4c, Story 1.5.4 AC 3).
- Named Extract-first tasks for the three hotspot functions: 1.5.6b `AutoReopenAfterFailedReview`, 1.5.6c `autoReopenForPRFix`, 1.5.6d `MaybeTriggerTriage`; Story 1.5.1 now lists Story 1.2.1 as a prerequisite (`SystemRequiredEdges()` is not in the tree).
- The `system` enforce flag is inert before Story 1.5.8: `ModeFor` degrades `system` to Shadow while `RecoveryReady()` is false (Task 1.5.2b, ADR-007 D8b, status RPC `recovery_wired`); the runbook says so.
- Wave W12 file collision removed by ownership: `forceResetItem` and the `TriggerTriage` RPC writes moved from 1.5.5 to 1.5.6 (Task 1.5.6e). The script check then found a second, new collision that the reviewers had not seen: Task 1.5.4a (production caller rename, now with its file list written out) touches `autonomous_orchestration_service.go`, which Story 4.1.2 also edits, in W11. Resolved by re-waving: 1.5.4 alone in W11, 4.1.2 to W16, 4.1.3 to W17, 4.1.1 and 5.1.2 to W18, 5.1.3 and 6.1.1 to W19; still 21 waves, W0-W20.
- Overrun checkpoint is now Task 1.5.3e (XS), a W10 exit gate, a Story 1.5.3 AC, and a Worker brief line, with an explicit trigger.
- Soak: N=20 and floor=100 are acknowledged as unreachable for the operator and system classes (about 21 live items, INFERRED from the review); READY for low-volume edges is the owner's acknowledgement of `--config-diff`, the stranded preview, the Story 1.5.9b per-caller table and a replay against a COPY of the live database (new Task 1.5.9d, `make workflow-engine-replay`, refuses the live path); Risk Control, Observability alerts, Story 1.4.1, Story 1.5.2 AC and `validation.md` updated.
- Metric 1 carve-out kept as the single explicit, owner-confirmable item in Unresolved Questions (system-required edges, including every `*->archived`, cannot be disabled; gates stay editable; the forwarding delegate is outside it; the PR template records the list generated from `SystemRequiredEdges()`).

**Deferred / not changed**
- Per-detector status scopes for the two "confirm" rows of Story 1.5.8 are settled by reading the `Find*` queries during Tasks 1.5.8c and 1.5.8d, not here.
- Whether to make the replay harness also run in CI against a fixture database is left to Task 1.5.9d.
- Reviewer nitpick on "about 29 files" for fakes: superseded by the recount above (31 test files).

**Recounts by command and script (plan.md after this round)**: 12 epics (`grep -cE '^### Epic'`), 39 stories (`grep -cE '^#### Story '`), 114 task headings (`grep -cE '^##### Task '`; 113 sized plus 4.1.5a; 49 M, 57 S, 7 XS sized), 21 waves W0-W20 with at most 2 stories each, 0 prerequisite-order violations over 39 `Prerequisites:` lines and 0 same-wave `Files:` collisions (script over the wave table, story `**Files**` and task `- Files:` lines). Epic 1.5: 9 stories, 44 tasks (was 32), 4,110k raw (was 3,035k). Effort: 9,645k raw, 9,934k CU, 17,385k CU after the x1.75 multiplier, above the 15M Large ceiling by about 16% (stated in the Effort estimate; no cut line).

**New risks introduced by this round**
- Epic 1.5 is now about 43% of the total and 1.5.4 (7 tasks, 760k raw, one wave alone) and 1.5.8 (6 tasks, 705k raw) are the largest stories; the W10 checkpoint exists to re-base them.
- Because `*->archived` is system-required for every status, an operator can never disable archiving from any stage (only gate it); this widens the carve-out (Unresolved Question 1).
- Unexporting the `EntRepository` writers breaks any out-of-tree or out-of-package test that calls them (one found, `backlog_item_event_publisher_test.go:258`); a missed one fails the build, not silently.
- The static `create` rule is lexical; a creator passing a computed `Status` needs an allow-list entry with a reason.

### Repair round 5 (2026-10-17): Epic 1.5 re-review round 2 (architecture CONCERNS 0 blockers / 5 concerns; adversarial CONCERNS 0 blockers / 5 concerns)

Owner decision stands: Success Metric 1 stays STRONG; full scope, no story cut. Each claim was re-checked in the tree before editing; corrections to the reviewers are noted.

**Fixed**
- **ARCH-C1 (analyzer mis-aimed)**: the guarantee that no `agent`/`system` denial is swallowed is now a gate unit test, `TestGatedTransition_EveryAgentOrSystemDenial_ReachesReporter` (denial sources x non-operator classes, Story 1.5.3 AC and Task 1.5.3b). The `silenttransition` extension (Task 1.5.4f, ADR-007 D5) is re-specified: `Raw*` names, and `Gated*` calls only when the `Caller` is an `operator`-class registry constant or non-constant, where the caller must return the error to a human; no "selector contains `Report`" rule; a test pins the analyzer's operator set to the registry; testdata and the Story 1.5.4 AC and `validation.md` row rewritten (a log-only agent-class `Gated*` call is deliberately not flagged).
- **ARCH-C2 (detector table)**: Story 1.5.8 rewritten against the code. Added rows for `dequeue_backlog_items` (spawn-point pre-claim skip plus the repeated `claim failed` Warn demoted to Debug, `backlog_service_triage.go:873`), `blocked_by_dependency` (resolve-only, VERIFIED by the call-site comment), `trigger_chain_reconcile` (no status writer, VERIFIED: `git grep` finds only comments in `chain_firer.go`), `pr_ready+merge_detection` (writer, grouped with `auto_archive_done`) and `pr_drift_recovery` (writer, `recoverDriftedPRItem` `->pr_pending`, as a table row). Corrections: `git grep -c 'l.runStuckDetector("'` returns 24 detector names, not 25; `rework_blocked_stale` and `respawn_blocked_active` are resolve-only (the "confirm" rows are settled). **Dispatch check (done in code, recorded in the story)**: `runStuckDetector` has no item in scope (name plus closure panic wrapper) and `remediationActionByReason` is only the operator "Retry now" map, so no single item-level dispatch hosts the guard; but both converge on six `BacklogService` methods that the periodic sweep also reaches through interfaces wired at `dependencies.go:1524-1605`. The per-detector guards therefore collapse to one guard inside each spawn point (plus `MaybeTriggerTriage` and `DequeueNextQueuedItems`), with per-detector guards only for the inline `FindReviewItemsWithoutGate` respawn (listener-local `spawnReviewGate`) and the `orphaned_triage` shape-2 marker. Detection-only rows are no longer required to skip `MarkStuck` (idempotent per row); their AC is one row and one notification. Tasks 1.5.8a-g re-cut (7 tasks, 760k raw, was 6 and 705k).
- **ARCH-C3 (RecoveryReady degrade)**: deleted. No `RecoveryReady()` global, no reporter-unwired or recovery-unwired branch, no `recovery_wired` proto field, no `enforce_degraded_total` metric. `-enforce-agent` is registered in Task 1.5.3c with the reporter wiring; `-enforce-system` in new Task 1.5.8g after the recovery guards; Task 1.5.2b reduced to `ModeFor(class)` over a class-to-flag table. The runbook states the dependency (system step requires the Story 1.5.8 completeness test green). ADR-007 D8 and its risk rows, Risk Control, Observability, wave and proto-editor notes, Story 1.4.1 and `validation.md` rows updated.
- **ARCH-C4 (denial key)**: the recorded denial carries the caller; reporter signature `Report(item, caller, from, to, reason)`; dedupe key `(item, caller, from, to, reason)` in the glossary, ADR-002 D4, ADR-007 D5, Tasks 1.2.2a/1.5.3b, Stories 1.5.3 and 1.5.8; the stuck row gains a `caller` field in Task 1.2.2a (same proto change, W6). The `MaybeTriggerTriage` row is made consistent: one report under caller `maybe_trigger_triage`, separate from `autonomous_complete`'s denial of `idea->ready`.
- **ADV-N1 (fallback drops the union)**: fallback graph = Default UNION `SystemRequiredEdges()` in the Epic 1.1 core design, glossary `enforceAnswer`, Story 1.1.2 (new AC, Task 1.1.2b), Story 1.5.1 (new AC and Task 1.5.1c test), ADR-001, ADR-002 D3, ADR-007 D4/D7. `validation.md` row for `FallsBackToDefaultGraph...` now includes `in_progress->archived` with an empty cache. Correction recorded: `SystemRequiredEdges()` and `IsSystemRequired` are NOT in the tree (`git grep` returns zero hits); Task 1.1.2b creates the set; text implying the selector "already unions" it was fixed in the glossary and Story 1.5.1's prerequisite note.
- **ADV-N2 (shared handler becomes class `agent`)**: Task 1.5.5a ACs specify caller resolution from `transitionActorFromContext(ctx)` (VERIFIED: `backlog_service_lifecycle.go:838`; the key is an unexported type, so a request cannot choose its class): `agent` gives `mcp_transition_delegate`, `user` gives `operator_transition_rpc`, an unknown value fails closed to the `agent` class (which ignores `override_reason`). The `forwarding` delegate is exempt from the "every registry entry has a call site" rule (Story 1.5.9 AC, ADR-007 D2/D9); new test `TestTransitionHandler_ResolvesCallerClassFromContextActor`; `TestMcpTransitionDelegate_UsesAgentClassAndGraph_WhenActorAgentAndEdgeDisabled` kept. Runbook note added: MCP `trigger_triage` calls the `TriggerTriage` RPC (VERIFIED `tools_backlog_dependencies.go:127`), classed `system`, so `-enforce-system` governs it.
- **ADV-N3 (strict guard matches six names)**: rule (d) added: any `.SetStatus(` on a `BacklogItem` ent builder in package `session` outside the allow-listed writer functions fails (Task 1.5.9a, ADR-007 D9, Story 1.5.9 AC); the creator's `SetStatus` (`ent_repository_backlog.go:452`) is allow-listed with reason `create` in Task 1.5.4g. Test `TestBacklogItemSetStatus_OnlyInsideAllowListedWriters`.
- **ADV-N4 (archive edges have no stage row)**: Unresolved Question 1 gains the owner sub-choice (i) virtual rows in the editor versus (ii) document archive-from-active as UI-ungateable (default), with the carve-out width (the four table-less edges: `in_progress|review|pr_pending->archived`, `archived->archived`, plus custom-slug `to=archived`; VERIFIED against `domain/backlog.go:514-574`); `design/ux.md` carries the page-note consequence; ADR-007 D12 cite corrected `:384-399` to `:374`.
- **Simplification alternative (recorded, not executed)**: owner-confirmable alternative (one enforce flag plus a per-caller revert list) added to Unresolved Questions with what it would delete and what it must not delete, to be decided at the Task 1.5.3e checkpoint (the task text now says so). The full-scope design remains the plan of record.

**Recounts (script `plancheck`, run on plan.md after this round)**: 12 epics, 39 stories, 115 task headings (114 sized plus owner-gated 4.1.5a; 49 M, 58 S, 7 XS sized), 21 waves W0-W20 with at most 2 stories each, every story once, 0 prerequisite-versus-wave violations over 39 `Prerequisites:` lines, 0 same-wave `Files:` collisions. Epic 1.5: 9 stories, 45 tasks (was 44), 4,165k raw (was 4,110k). Effort: 9,700k raw, 9,992k CU, 17,486k CU after the x1.75 multiplier, above the 15M Large ceiling by about 2.5M (17%). `validation.md` rows and summary counts updated.

**New risks introduced by this round**
- Story 1.5.8 now depends on four rows being classified by reading code during implementation (`unprocessed_review_verdict`, `orphaned_agent_pr`, `push_failed`, scope of `AutoReopenForPRFix`); a wrong classification surfaces as the completeness test failing, not silently, but can move cost between Tasks 1.5.8b-d.
- Moving the guard to the spawn points means a future respawner that bypasses those six methods (a new listener-local spawn, like `spawnReviewGate`) is not covered; the completeness test and the `D` rows are the only net.
- Flags are registered in three different tasks (shadow/operator 1.1.3c, agent 1.5.3c, system 1.5.8g), so `class_modes` shows the agent and system classes as Shadow-only until their task lands; a reviewer must not read that as a defect.
- Unknown-actor fail-closed to class `agent` is a design choice not in the reviews; it is the stricter class but could surprise a future non-MCP in-process caller that sets an unusual actor.
- `Task 1.5.4f` now needs the registry to export the operator caller names to the analyzer; an analyzer-registry coupling that the pinning test must keep honest.

### Repair round 6 (2026-10-17): triad review (product, UX, engineering; all three verdicts needs-work, no hard blocker)

Owner decisions recorded as approved 2026-10-17 and removed from the open list and the pre-W0 blockers: (1) soak parameters accepted as proposed (W = 7 days operator/agent and 14 days system, N = 20 per engine-routed edge, floor 100 calls; low-volume READY = owner acknowledgement of `--config-diff`, the stranded-item preview, the Story 1.5.9b per-caller table and a replay on a COPY of the live database); (2) archive-from-active edges documented as UI-ungateable (option A). Full scope stands; nothing was cut.

**Product**
- Outcome metric: `requirements.md` Metric 6 (stuck-item rate and manual interventions per week, read from `ListStuckBacklogItems`/`ListBacklogItems`; baseline 13 stuck rows covering 6 distinct items among 21 items, copied from the Baseline section as recorded by the audit and NOT re-measured because the live MCP was unreachable; the baseline for interventions is not recorded). Reviewed on a fixed date (first deploy + 14 days).
- Forcing function against "the flip never happens": runbook "Review dates" block and "Outcome review" step (Story 1.4.1 AC), `REVIEW DUE`/`REVIEW OVERDUE` in `make workflow-engine-soak-status` (Story 1.1.3 AC, Task 1.1.3b), validation rows M6.
- Problem statement items 1 and 2 in `requirements.md` updated to the resolved research (Open Question 1 answered; item 2 verified by command: no quota RPC in `proto/session/v1/`, only the `dependencies.go:1436` closure, soft budget only per item).
- Roadmap: a short "Roadmap fit" section in `requirements.md` (four items, why item 1 first, pointer to the cohesion audit; no roadmap document exists in the repo, `grep -n -i roadmap docs/tasks/backlog-feature-improvement.md` returns 0, so none was created).
- Runbook dry-run on the manual instance is now a Story 1.4.1 AC and Task 1.4.1b. To make that possible the guarded script moved from Task 6.1.2c (W20) to Task 0.1.1d (W0): the Worker brief made it the only way to run a build, yet it was scheduled after every story that needs it (found while fixing this gap).

**Engineering**
- New "Authn/authz" section, VERIFIED against `server/server.go:1907-1945`, `server/middleware/auth.go` and `server/actuator.go:29-31`: the local listener has either the auth middleware or `LocalWriteGuard` over five `guardedProcedures`; stage/edge/gate writes are NOT among them; actuator routes sit behind the same chain. Required: Task 2.1.1e guards `UpdateQuotaConfig` and nine stage/edge/gate writes with `ProfileRebinding`; read RPCs and actuator routes add no exemption; ACs in Stories 1.1.3, 1.3.1, 2.1.1; the three vague NFR-2 validation rows (they referred to an interceptor that does not exist) were replaced by five rows modeled on `server/gatestats_listener_test.go`.
- Coordination risk for Story 1.5.6 (and 1.2.2, 1.5.4, 1.5.8): the open item "Investigate false triage artifact does not exist error" is named with a sequencing rule (Worker brief overlap check, Story 1.5.6 AC, Task 1.5.6h, wave row W12). Its state and file overlap are UNVERIFIED: the live backlog MCP was unreachable (ECONNREFUSED) during the triad review and during this repair, so no claim is made about it beyond "reported by the engineering triad review".
- Simplification alternative withdrawn from the active plan (it was a cut list, contrary to the owner's standing full-scope instruction): removed from Unresolved Questions, Task 1.5.3e, the Effort narrative, ADR-007 and the validation owner-decisions row; it remains described in `architecture-review.md` and `adversarial-review.md`.

**UX**
- New `AlertConfirmDialog` (Task 4.1.4e) replaces "reuse `PiDisableWarningDialog`", which is `role="dialog"` with initial focus on its primary button; the enforce dialog is `role="alertdialog"` with Cancel focused, shows readiness evidence at decision time (shadow differences, soak verdict, stranded-item count; backend `class_readiness`, Task 1.3.1b; evaluator moved into `session/workflow_engine_soak.go`) and gates Enforce on an acknowledgement when evidence is not clean (UX-35).
- Contrast: Task 4.1.4f fixes the underlying defect and removes the three `color-contrast` exemptions (`tests/e2e/a11y-route-sweep.spec.ts:31-37`); Task 6.1.1c adds `/settings/backlog-quota` to the sweep with none (UX-32).
- Freshness and deep link: refetch on focus/visibility, Stale banner row, corrected engine-row live note (UX-36); hash-focus on the flag row with `id`/`tabIndex`/`scroll-margin-top` (Tasks 1.3.2f, 1.3.3b, UX-37). Combined summary line over the three class flags with plain definitions (UX-39).
- Quota: `<Link>` entry in `settings/page.tsx` plus `routes.settingsBacklogQuota` (the layout has no nav; Task 2.2.1a), leave guard (Task 2.2.1c, UX-38), `enterKeyHint` and scroll-into-view (UX-41); StageForm mode reminder above Save (Task 1.3.2b); dialog button order; "(opens in new tab)" (UX-40); PR-preflight surface and level (UX-42, Story 3.1.4).
- Test enablers: `tests/e2e/helpers/backlog-settings-mocks.ts` and `tests/e2e/pages/ThemeToggle.ts` (Task 6.1.1c); UX-26 is now an assertion.
- UX-09/10/13/14/17 and Tasks 4.1.4a/4.1.4d are mandatory; "optional", "conditional" and "cut first" wording was removed from the plan, `design/ux.md` and `validation.md` (older repair-log lines are historical and say so).

**Not fixed, with reason**
- Banner `role="status"` announcing the initial mode (UX gap 10): kept; announcing an Unknown or Shadow state on load is the intended fail-closed behavior, and changing the role per mode would break UX-01's single assertion. Recorded in `design/ux.md` section 2 (announces changes, not initial render).
- Preflight dismissal control: which control in the notification list dismisses it was not read; Task 3.1.4a records it. UNVERIFIED.
- The quota leave guard cannot veto programmatic `router.push` (Next.js app router has no route-change veto); stated as a limit.
- `RecordGateApproval` and the other `SaveConfig` callers stay unguarded/unlocked (pre-existing, stated, not widened).
- Live state of the "false triage artifact" item and the audit baseline were not re-measured (live MCP unreachable).

**Recounts by script (`plancheck` over plan.md after this round)**: 12 epics, 39 stories, 125 task headings (124 sized plus owner-gated 4.1.5a; 51 M, 65 S, 8 XS sized), 21 waves W0-W20 with at most 2 stories each, every story once, 0 prerequisite-versus-wave violations over 39 `Prerequisites:` lines, 0 same-wave `Files:` collisions (also 0 after expanding task-level shorthand paths against full story paths). Epic 1.5: 9 stories, 46 tasks, 4,185k raw (was 45 tasks, 4,165k). Effort: 10,365k raw, 10,675k CU, 18,681k CU after the x1.75 multiplier, above the 15M Large ceiling by about 3.7M (25%). `validation.md`: 202 requirement-mapped tests (119 Go unit, 17 Jest unit, 60 Go integration, 2 Jest integration, 2 Playwright integration, 2 manual) and 42 UX criteria, 17 requirements.

**New risks introduced by this round**
- `ProfileRebinding` on the nine stage/edge/gate writes could reject a legitimate non-browser client (an MCP tool or script) that sends an unexpected Origin; Task 2.1.1e's manual-instance check is the net, and the fix is in the guard, not dropping a procedure.
- The readiness evidence makes `GetWorkflowEngineStatus` depend on the soak ledger; an unreadable ledger yields `UNAVAILABLE`, which disables Enforce until acknowledged (intended, but can annoy).
- The leave guard intercepts anchor clicks and `popstate`; a future `router.push` caller bypasses it.
- Moving Task 6.1.2c to W0 puts a `Makefile` edit in four waves (W0, W3, W9, W15); the per-wave file check keeps them apart.
- Epic 1.5 grows by one XS task; total effort rises about 1.2M CU after the multiplier, mostly from UX work and the extra authz and evidence plumbing.

### Repair round 7 (2026-10-17): triad 2 (UX needs-work, engineering, product ready; no hard blocker)

Each triad-2 claim was checked against code before editing; the checks that changed a conclusion are listed first.

**Verified in code, and what it changed**
- `useDialogFocusTrap` (`web-app/src/lib/hooks/useDialogFocusTrap.ts`) DOES restore focus on unmount, to the element focused when the hook first rendered; the UX lens said it had no return-focus contract. What it lacks is an explicit target, overlay click and an inert background. `HostKeyTrustDialog.tsx` does focus Cancel (`:39`) and implements overlay-click-cancels in the component. So ownership is split: hook = initial focus, Tab trap, Escape, focus return; `AlertConfirmDialog` = portal, ARIA, overlay click, `returnFocusRef`, button row (UX-43, Task 4.1.4e).
- `ThemePicker` has no `data-testid`, and the app applies a theme as a vanilla-extract class on `<html>` (`ThemeContext.tsx:223-229`), not `data-theme`: the plan's Task 6.1.1c ("asserts `data-theme`") was wrong and is corrected (the helper clicks the `radio` named "Light"/"Dark" in the "Theme selection" radiogroup and asserts `aria-checked` plus a changed `<html>` class).
- `setKeyboardOpen(page, heightPx)` already exists in `tests/e2e/helpers/viewport-profiles.ts:56` (overrides `visualViewport.height`, dispatches `resize`); UX-41 reuses it and adds a manual phone leg because a real keyboard cannot be raised.
- `--header-height` and `--bottom-nav-height` already exist as CSS custom properties (`web-app/src/app/globals.css`), so the sticky-chrome offset is a token without a new task.
- The notification list dismisses a row with `Remove notification` (`NotificationItem.tsx:215`), `x` on a focused row, `Dismiss N informational` and `Clear informational (N)`; `INFO` maps to the informational bucket; `notification-a11y-matrix.spec.ts` seeds `ERROR` and `CUSTOM` only, so no INFO expectation exists. UX-42 is no longer UNVERIFIED; Task 3.1.4a runs first in its story.
- `Quota` is a non-pointer struct defaulted at load (`config/config.go:390,823,1673`), so "not configured" cannot be read from the loaded value; `quota_source` must come from the stored value before defaulting (ADR-004 amended).
- The task-level shorthand was in 11 task lines (1.2.1b, 1.3.2a, 1.3.2b, 1.5.4c, 1.5.7a, 1.5.7c, 3.1.3a, 3.1.4a, 4.1.3a, 4.1.4c and the 1.5.4e "remaining files"), not in 1.5.5/1.5.6 as the engineering lens said; all are replaced by real paths. Page objects were listed in Story 6.1.1 `Files` but no task owned them; Task 6.1.1d now does.

**UX gaps fixed** (design/ux.md sections 1-5, 7-9; UX-01 rewritten, UX-13/15/19/26/32/41/42 amended, UX-43 to UX-55 added)
- Dialog: overlay click and focus-return ownership (UX-43); evidence loading, Retry focus retention, single live region, layout/focus when the acknowledgement appears, `aria-describedby` hint for an `aria-disabled` Enforce (UX-44); readiness re-check on the Enforce click with the worse-evidence and re-check-failure paths (UX-45); independent per-class flags stated on purpose with the recommended order inline and a non-blocking advisory (UX-46).
- Banner: the `role=status` contradiction resolved as "container static, one nested live region empty at first render, announces changes only"; UX-01 is now testable (`toBeEmpty()` then fills on a refetch or Retry). One live-region policy for all surfaces (ux.md section 1).
- Deep link: terminal-state keyed, 10 s ceiling, no focus steal after interaction or when `activeElement` is not body, `hashchange`/`popstate`, offsets from the existing tokens (UX-47). Pending state: Space/Enter ignored, Revert `aria-disabled`, "Saving..." announced once (UX-48).
- Quota: single status region and static warning/effective line, mismatch exit with focus handoff (UX-49), `quota_source` (UX-50), history-sentinel leave guard with `router.replace`, sentinel skip when clean and `beforeunload` during a pending save (UX-51), 44px rows (UX-54).
- StageForm reminder copy for Loading, Unknown, Stale and Partly enforced from one shared status hook (UX-52). Auto-merge: "Last tried" source, missing repo URL, Retry focus identity (UX-53; the Story 3.1.2 AC sets the timestamp on failed lookups too).
- Contrast: Task 4.1.4g takes the axe baseline with the exemptions OFF before Task 4.1.4f. UX-26 is an outcome assertion; the acknowledgement label and quota rows have explicit 44px hit areas. New tasks 1.3.3c, 1.3.3d, 2.2.1d, 3.1.3b, 4.1.4g, 6.1.1d; 1.3.3b and 2.2.1c grow to M.

**Engineering gaps fixed**: "Task sizing rule and exceptions" (mechanical-rename exception for Tasks 1.5.4a and 1.5.4e with the reason splitting by package was rejected, 1.5.3c and 2.2.1a justified and not split); task-level `- Files:` lines now name real paths and the disjointness check includes them; "Dispatch preconditions" (artifacts committed, WIP cap confirmed before every wave, triage-file overlap check before W6/W11/W12/W14).

**Product gaps fixed**: "Riskiest Assumption" in `requirements.md` with its test order and falsifier; the review-overdue forcing function is visible in the Stages banner and Features summary (Tasks 1.3.1c, 1.3.2g, 1.3.3b, UX-55; no `notify()`, because a periodic notifier needs a scheduler outside this project); the table of what each open owner confirmation blocks (carve-out reading before W5; operator-gate policy and agent `override_reason` before W9; soak start date W18 / Task 4.1.1a only; Q5 only Task 4.1.5a; none blocks W0-W4). The round-6 "Not fixed" item about the banner `role="status"` is superseded by the nested live region.

**Not fixable here, or accepted (with reason)**
- Server-side enforcement of readiness: the re-check is client-side because `UpdateFeatureFlag` is a bool-only RPC shared with other clients; residual window is one RPC round trip and the write reverts in one click.
- The dialog cannot make the background `inert` (the hook has no such support); `aria-modal`, the Tab trap and the overlay are the containment. Next `pushState` same-page hash links fire neither `hashchange` nor `popstate` (no such link exists). A programmatic `router.push` is still not interceptable. A real soft keyboard cannot be raised under Playwright (manual phone leg added).
- UNVERIFIED: the live backlog MCP was unreachable (ECONNREFUSED), so the WIP cap and the open "Investigate false triage artifact does not exist error" item remain the dispatcher's first checks; the axe baseline was not run (it needs a built server; Task 4.1.4g measures it, and it is not known whether one shared token or several components cause the contrast failures); which stored-value accessor is cheapest for `quota_source` is decided in Task 2.1.1a.
- Still open owner decisions: the four unchecked confirmations, Q5, ADR-005's INFERRED exit criteria and the missing Metric 6(b) baseline are unchanged.

**Recounts by script (`plancheck.py` and the task-level check over plan.md after this round)**: 12 epics, 39 stories (all in the wave table, at most 2 per wave), 133 task headings (132 sized: 54 M, 69 S, 9 XS, plus owner-gated 4.1.5a), 21 waves W0-W20, 39 `Prerequisites:` lines with 0 prerequisite-versus-wave violations, 0 same-wave `Files:` collisions after unioning every task's `- Files:` tokens into its story (34 task tokens name paths beyond their story line, 30 source files plus 4 non-source entries; the source files are counted), Story 1.5.4 alone in W11. Epic 1.5 unchanged: 46 tasks, 4,185k raw, 4,309k CU, 7,541k after the multiplier (38.1% of the new total). Effort: 10,995k raw, 11,323k CU, 19,815k CU after x1.75, above the 15M Large ceiling by about 4.8M (32%). `validation.md`: 217 requirement-mapped tests (122 Go unit, 25 Jest unit, 60 Go integration, 6 Jest integration, 2 Playwright integration, 2 manual) and 55 UX criteria (49 Playwright-primary plus 6 with an e2e twin).

**New risks introduced by this round**
- The nested live region and the single-status-region rule change what several Jest and Playwright locators assert; a spec that still uses an unscoped `getByRole("status")` on Stages or Features fails strict mode (the specs are written against `data-testid` or row scope).
- The history sentinel adds one `pushState` entry while the quota form is dirty; a future router upgrade that changes `popstate` semantics breaks UX-51 first, and the e2e is the net.
- Re-fetching `GetWorkflowEngineStatus` on every Enforce click adds one read per confirm; negligible, and the RPC is TTL-cached.

### W0 errata (2026-10-18)

W0 implementation found these plan errors; each was checked against code before the text was corrected.
- **`override_reason` is narrower than planned** (VERIFIED `session/domain/backlog.go:710-747`, pinned by `server/services/backlog_transition_characterization_test.go`): Default `TransitionGuard` honors it only on `review|pr_pending->ready` and `->done`; `ErrACRequired`, `ErrPlanRequired`, `ErrPlanArtifactsRequired`, `ErrPlanArtifactsMissing` and `ErrUnresolvedBlockers` ignore it. Corrected: ADR-001 decision and spike note, ADR-007 class table, `enforceAnswer` glossary row, Story 1.1.2 ACs (new AC: override does not bypass AC/blocker/plan denials), Task 1.1.2c, Story 1.2.4, Story 1.5.2/1.5.5 wording, the class table, and `validation.md` rows. Semantics now: Default guard input is forwarded unchanged; a non-empty reason additionally bypasses Configured operator gates (incl. the synthetic deleted-stage gate), never the graph. Not changed (still reads as if override frees a graph-stranded item): the stranded-item ACs near Story 1.2.1/1.2.2 and "`override_reason` remains the documented exit" (Story 1.5.8); a disabled edge is not bypassed by override, so those need an owner look.
- **Cite**: `cost_budget_threshold_usd` is `session/repository.go:365-369` (ent column `session/ent/schema/backlog_item.go:137`).
- **Settings nav**: the Settings index uses `<Link>` entries, not tiles, and a second registry `web-app/src/lib/nav-pages.ts` (pinned by `web-app/src/lib/__tests__/nav-pages.test.ts`) exists. Both added to Task 2.2.1a Files (now 8 files) and the story Files line.
- **Manual slots**: port block holds 10 ports, so slots 1-5; the wave table uses only 1 and 2. Cap noted in the Worker brief.
- **Story 4.1.4 contrast DONE** (`b48a6a119`): five component defects, files `globals.css`, `settings.css.ts`, `settingsListPage.css.ts`, `theme.css.ts` (`primaryText`, `accentText` `#7c83f7`); Files lines extended. `/settings/backlog-quota` still needs its sweep entry in Task 6.1.1c. Task 4.1.4c: nudge flags were already in `knownFeatureFlags`; only the status provider was added; the "Rollback line" and default `LIVE_FLAG_NOTE` on every row were NOT implemented (test `fg1` asserts one note): residual for Story 1.3.3 / Task 4.1.3a.
- **Worker brief** gained: fresh-worktree `make proto-gen ent-gen` plus `server/web/dist/.placeholder`, avoid full `make build`; worktrees branch from `main`, plan files read from the feature-branch absolute path and never committed.

### Owner decision: operator `override_reason` and the graph (2026-10-18)
- Owner reply to the stranded-item question: "im okay with the operator overriding". Recorded as option B for the **operator** class only: a non-empty `override_reason` from an operator may also bypass the graph check, so it is the documented exit for an item stranded by a disabled edge or stage. This matches the Story 1.2.1/1.2.2 stranded-item ACs and Story 1.5.8 ("`override_reason` remains the documented exit"), which stay as written.
- Still unchanged: agents and system callers never get a graph bypass (ADR-007 D3); the Default guards' own override semantics are unchanged; `ErrACRequired`, `ErrPlanRequired`, plan-artifact and unresolved-blocker denials are not bypassed.
- Code impact (not yet implemented): `enforceAnswer` (merged in W2) currently never bypasses the graph; the operator-class graph bypass, the audit note, and `notifyManualOverride` on a graph bypass land with Story 1.2.1/1.2.4, and the ADR-001/ADR-007 wording and the Task 0.1.1 AC at line 294 (spike pinned Default behavior only) need the matching update then. Success Metric 1 reads "no status change bypasses the engine except an operator's recorded override".

### Owner decision: Metric 1 carve-out confirmed (2026-10-18)
- Owner replied "yes, confirm the carve-out and dispatch 1.2.1". The editor rejects disabling any edge an `agent` tool or `system` caller can request (ADR-007 Decision 4), including every `*->archived` edge; those edges stay gate-editable. Alternatives A and B for this item are dropped. Story 1.2.1 is cleared for dispatch.

## Errata from W5 (2026-10-18)

- Story 1.2.1 `Files:` extended to include `server/services/backlog_service_stages.go` and its test file (stage RPC guards, AC3/AC4).
- The seeded graph has no `review->archived` edge; built-in stages cannot be stranded by an edge edit; the single-edge stranded check is mostly redundant with `ValidateDisableTransition`.
- Stage disable/delete stranding check and the write are separate steps (small race window). Disabling a built-in stage that still has system-required outgoing edges is not blocked, since it is not stranded under the AC4 definition.
- The operator `override_reason` graph bypass is still unimplemented; AC4's "only exit" message depends on it (Story 1.2.4 or later, with ADR-001/ADR-007 wording update).
- Story 2.2.1: an extra test file was edited, a `backlog` flag was added, the link is unconditional, and the "Backlog resumed" text is a heuristic.
- Task 1.1.3e (per-from-stage scope) deliberately skipped; the flag stays global-only.

## Errata from W6 (2026-10-18)

- Story 1.2.2: the web-app stuck maps (`stuckReason.ts`, `StuckItemsSection.tsx` `GROUP_ORDER`) needed a `TRANSITION_DENIED` entry for `tsc` and the section's guard test; the dispatcher added them (outside the story's `Files:` line). The UI does not yet show `denied_caller/from/to/reason` (proto fields 17-20); Story 1.5.8 should surface them.
- Story 1.2.2: structured denial fields live in the stuck row's `context` string (parsed into proto fields 17-20), not new ent columns; `ErrTransitionNotAllowed` is re-exported from `session/storage_backlog.go`.
- Story 1.2.2: a snoozed `transition_denied` row is invisible to the dedupe check (`FindOpenStuckStates` excludes snoozed), so a snoozed row gets a fresh comment and notification on the next sweep. Unaddressed.
- Story 3.1.1: `RepoSettings` returns `Status=OK` with `Stale=true` when a refresh fails but an earlier OK value is retained (for Story 3.1.3).

## Errata from W7 (2026-10-19)

- Story 3.1.2 `Files:` extended to include `server/services/backlog_service.go` (`repoSettings` field and `SetRepoSettingsLookup`) and `server/dependencies.go` (injection). The RPC that populates the report is `ListStuckBacklogItems`, not `GetBacklogStuckReport`; population is in the handler (`populateAutoMerge`), not the mapper. New proto: `AutoMergeLookupStatus`, `StuckBacklogItem` fields 21-22.
- Story 1.2.3 `Files:` extended with `server/actuator_test.go`. `TransitionCaller` is a bare `string` type in `session/workflow_engine_bypass.go`; Story 1.5.1 must move or extend it. Recount matched the plan (38 + 9 sites; 40 pending across 32 entries, 5 exempt, 5 engine-routed); line numbers shifted (rollbacks now at `backlog_service_triage.go:853,2131,2487`, `SyncLoop` at `backlog_sync.go:462`). The 3 rollback and 2 debug sites are listed `pending` (not `exempt`) until Story 1.5.6.
- Workers' branches merge `main`, so merging them pulls in main's new proto (e.g. fan-out); rerun `make proto-gen` after each merge before testing.

## Errata from W8 (2026-10-19)

- `origin/main` was merged into the feature branch before W8 (clean). Main replaced pnpm with Bun (PR #1100): every "pnpm" in this plan now means the repo's Bun commands (`bun run lint:duplicates`, `bunx tsc --noEmit`, `bunx jest`). Main also gained `17eabe4cc` (pr_pending items stranded on transient GitHub rejections), which touches backlog code; check it at the W11/W12/W14 overlap checks.
- Story 1.5.1: `SystemRequiredEdges()` and the archive group were already in the tree (Story 1.2.1); the plan's "not in the tree" wording is stale. `TransitionCaller` now lives in `session/transition_callers.go` (spec type `TransitionCallerSpec`; 14 operator/agent and 25 system entries). Files extended with `session/workflow_engine_bypass.go` (type removed), `server/services/workflow_bypass_inventory_test.go` (registry-vs-inventory reconciliation) and `session/workflow_system_edges_archive_test.go`.
- Story 1.5.1 edge audit (ADR-007 Decision 7): `pr_pending->review`, `review->ready`, `queued->idea` added to the system-required core; new `systemRequiredOffGraph` group for edges written today but absent from the graph (`in_progress->done`, `ready->done`, `{idea,refining,ready,queued,in_progress}->pr_pending`), excluded from `AllowedTransitions`. `report_blocked` from `review` targets `review->review` (caller bug, fix in Task 1.5.5c). `TriggerTriage` holds two writes of different classes (`ready->idea` operator, `idea->ready` system). Reachability of `ready->done` in `pushAndCreatePR` is INFERRED from a comment.
- Story 3.1.3: the "Retry via interactive refresh" is only half built. The UI and `refresh_auto_merge` (request field 1) exist, but nothing reads the flag and nothing passes `onRefreshAutoMerge`, so Retry never renders. **New Task 3.1.3c (S, owner of the gap):** `ListStuckBacklogItems` honors `refresh_auto_merge` and bypasses the `RepoSettings` cache (add a bypass option in `github/repo_settings*.go` if needed); `useStuckBacklogItems` and `StuckItemsSection.tsx` pass `onRefreshAutoMerge`. Files: `server/services/backlog_service_stuck.go`, `server/services/backlog_stuck_rpc_test.go`, `github/repo_settings.go`, `github/repo_settings_cache.go`, `github/repo_settings_test.go`, `web-app/src/components/backlog-stuck/StuckItemsSection.tsx`, `web-app/src/lib/hooks/useStuckBacklogItems.ts` (path to confirm). The stale marker ("as of Xm ago") hard-codes the 10-minute cache TTL in the UI.
- `gofmt -l` flags `server/services/create_pull_request_test.go` and `graph_validator_stranded_test.go` (pre-existing from earlier waves; fix with the collateral-debt rule before ship).

## Owner decisions before W9 (2026-10-19)

- **System-class operator-gate policy (ADR-007 D3): CONFIRMED as planned.** System callers skip the Default built-in guards by default but DO hit Configured operator gates; an unsatisfied gate surfaces as a `transition_denied` stuck item (Story 1.2.2).
- **Agent `override_reason`: owner chose a narrower grant than the plan assumed.** Agents MAY pass `override_reason`, only for the Default `TransitionGuard`'s existing exemptions (`review|pr_pending->ready` and `->done`, today's behavior). It never bypasses a Configured operator gate, the graph, or `ErrACRequired`, `ErrPlanRequired`, `ErrPlanArtifactsRequired`, `ErrPlanArtifactsMissing`, `ErrUnresolvedBlockers`. Operators keep the broader behavior (gates, plus the graph bypass approved 2026-10-18). This replaces "agents get none" in Task 1.5.2a (agent policy), Task 1.5.5c (MCP writers) and ADR-007 D3; update those texts when each task is implemented.

## Dispatcher decision (2026-10-19): Story 3.1.4 Files extended

Adds `github/repo_settings_cache.go` (a cached-only, non-fetching peek `RepoSettingsCached`), `session/backlog_lifecycle.go` (listener fields, mutex-guarded setters, injectable clock) and `server/dependencies.go` (wiring). Disjoint from Story 1.5.2's files.
