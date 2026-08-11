# Implementation Plan: backlog-pr-mergeability-policy

**Feature**: An opt-in per-item `AutoMergePolicy` flag that, gated behind a required global
kill-switch, drives a finished backlog item's work to a merged PR autonomously — auto-create PR
on Complete, auto-fix CI/conflict loops, and notify only on genuine mergeability — using the
existing `pushAndCreatePR` / `ReconcilePRPending` / `AutoReopenForPRFix` / `markPRReadyUnmerged`
machinery, with no parallel spawn path.
**Date**: 2026-07-17
**Status**: Ready for implementation
**ADRs**: ADR-024 (autonomous PR-merge policy trust boundary), ADR-025 (orphan-PR adoption
reconciler / Phase-0 desync fix)

---

## Scope Decision (requirements Open Question #1)

**The `review`/`BOUNCING`-vs-`pr_pending` desync is IN SCOPE as Phase 0** — see ADR-025. It is a
hard prerequisite: Behavior 2 (auto-fix) and Behavior 3 (ready-to-merge notify) both run *only*
inside `ReconcilePRPending`, which is structurally unreachable for the orphaned item class the
feature targets (Path-B / manual-`RunOneShot` PRs, and the precondition-failure road). Phase 0
(an orphan-PR adoption detector) plus Behavior 1's single-writer routing makes those items
reachable. **Phase 4 (Behavior 2) depends on Phase 0.**

---

## Dependency Visualization

```
Phase 0: Orphan-PR Adoption Detector (desync prerequisite, ADR-025)
   │  (makes ReconcilePRPending reachable for orphaned items)
   │
   ├──────────────────────────────────────────────┐
   │                                               │
Phase 1: Per-item AutoMergePolicy flag        Phase 2: Global kill-switch
   (ent→proto→domain→repo→RPC→form→currentFlags)   (knownFeatureFlags + config read
   │                                               + listener SetGlobalPolicyEnabled)
   │                                               │
   └───────────────┬───────────────────────────────┘
                   │  policyActive(item) := globalOn() && item.AutoMergePolicy
                   │
        ┌──────────┼───────────────┬──────────────────────┐
        │          │               │                      │
   Phase 3      Phase 4         Phase 5                    │
   Behavior 1   Behavior 2      Behavior 3                 │
   CI tri-state auto-fix gate   ready-to-merge notify      │
   + auto-PR    (needs Ph0)     (copy extension)           │
   on Complete  (gate 3 @1578,  (extend markPRReadyUnmerged)│
   + arm gate   :1626)          gated on ciPassing         │
   (relocated   │               │                          │
   to reconciler,│              │                          │
   gates 1,2)   │               │                          │
        └──────────┴───────────────┴──────────────────────┘
                   │
              Phase 6: Feature registry + Go/Jest unit tests + e2e Playwright
                       (needs all prior phases)

Critical path: Phase 0 → Phase 4 → Phase 6   (Phases 1 & 2 parallel with Phase 0)
```

**Gate predicate used throughout** (ADR-024 §a):
`policyActive(item) := l.globalPolicyEnabled() && item.AutoMergePolicy`
Three gate points: (1) auto-merge arm — **relocated** from `pushAndCreatePR:1475` into the
`ReconcilePRPending` healthy branch (`:1584-1609`), where auto-merge is armed only when
`policyActive(item)` **AND CI is actually passing** (see Blocker-1 fix, Phase 3 Epic 3.0/3.1);
(2) auto-PR-on-Complete for `SkipReviewGate` items `onSessionExited` `:444-481`; (3) auto-fix
spawn `:1578` + `:1626`.

**CI tri-state (Blocker-1 correctness premise).** "CI green" must mean *positively passing*, not
merely *not-failing*. Today `CIFailing` is the only CI signal; pending / not-yet-created checks
read as `CheckConclusion==""` = green, so (a) the ready-notify can fire before CI is truly green
and (b) arming `gh pr merge --auto` on an unprotected `main` merges **before non-required checks
finish** (`--auto` does not wait for non-required checks). Both gates that depend on green — the
Behavior-3 ready-notify and the auto-merge arm — must require a positive `ciPassing` signal
(checks concluded, none failing, none pending). This is why the arm is relocated to the reconciler
(the only place CI truth is polled), never fired at PR-create time when CI has not yet started.

**Hard invariants for every phase:**
- Every status write goes through `TransitionBacklogItemStatus` — never a raw ent `SetStatus`
  (or the `BacklogStatusEvent` audit row is lost). `in_progress → pr_pending` is **not** a valid
  transition (domain/backlog.go): adopt/route `in_progress` items via `in_progress → review →
  pr_pending`, never directly.
- Never `_ = s.storage.Update(...)`-swallow errors — log at Warning/Error or surface via notify.
- No new spawn path — reuse `AutoReopenForPRFix`; respect the guard order
  `tombstoneOrphanWorkSessions → hasActiveWorkSession → transition`.
- Merge/notify/fix/arm decisions anchor on polled GitHub truth (`IsPRMerged`, `GetPRStatus`),
  never on an agent self-report. Auto-merge is armed only after a `GetPRStatus` poll confirms CI
  positively passing — never optimistically at PR-create.

---

## Phase 0: Orphan-PR Adoption Detector (desync prerequisite — ADR-025)

Gates Phase 4. Makes `ReconcilePRPending` reachable for items with a live PR that the reconciler
cannot see, across **both** orphan classes (ADR-025 §Context):
- **Class A — Path-B (`RunOneShot`)**: PR truth stamped only on the session `Instance`
  (`GitHubPRURL`); item has `PrNumber == 0`, status `review`/`in_progress`.
- **Class B — precondition-failure road**: `pushAndCreatePR` writes item `PrURL`/`PrNumber`
  (`:1460`) **before** the `review → pr_pending` transition (`:1489`); if that transition loses a
  race and fails, the item is left with **`PrNumber > 0` + status still `review`/`in_progress`**.
  Filtering `PrNumber == 0` misses this class entirely (BLOCKER-2). Phase 3.2 (E7) adds a *second*
  `pushAndCreatePR` caller that can manufacture exactly this orphan.

**Write-order note (BLOCKER-2):** we deliberately keep `pushAndCreatePR`'s field-write-before-
transition order — writing PR fields first means the PR reference is never lost if the transition
fails (Class B is recoverable). The detector below is the backstop that covers that intermediate
state; the entry guard added in Phase 3 Epic 3.2 prevents a duplicate PR on the retry.

### Epic 0.1: Adopt orphaned PRs into `pr_pending`

  #### Story 0.1.1: A backlog item with a live PR (on the item OR its Instance) becomes reconciler-visible
  **As a** backlog operator **I want** an item whose PR was created out-of-band (manual
  Review-Queue button, or a failed `review→pr_pending` precondition) to be promoted to
  `pr_pending` with its PR fields stamped **so that** the merge/CI/conflict reconciler can see and
  drive it.
  **Acceptance Criteria**:
  - An item in `review` or `in_progress` is adopted to `pr_pending` when **either** the item
    already carries `PrNumber > 0` / `PrURL != ""` (Class B) **or** its linked session
    `InstanceData` carries a non-empty `GitHubPRURL` (Class A). The `PrNumber == 0` filter is
    **removed** as the sole gate.
  - PR fields are stamped from whichever source has them (item fields left as-is for Class B;
    parsed from the Instance URL for Class A) via `UpdateBacklogItem`.
  - The transition uses `TransitionBacklogItemStatus`. `review → pr_pending` is direct;
    `in_progress` items are routed `in_progress → review → pr_pending` (two guarded transitions —
    `in_progress → pr_pending` is **not** a valid transition per domain/backlog.go).
  - Runs for all items (not policy-gated); adoption of a non-policy item does NOT spawn a fix.
  - Panic-isolated within the sweep; a lookup failure for one item does not abort the detector.
  **Files**: `session/backlog_lifecycle.go`, `server/dependencies.go`

    ##### Task 0.1.1a: Thread a NON-HYDRATING Instance-PR lookup into the listener (~4 min)
    - In `session/backlog_lifecycle.go`, add a field `instancePRLookup func(sessionUUID string)
      (prURL string, ok bool)` to `BacklogLifecycleListener` (struct at `:93`) and a setter
      `SetInstancePRLookup(fn ...)` mirroring `SetSessionLivenessChecker` (`:927` wiring site).
    - In `server/dependencies.go` near the other `backlogLifecycleListener.Set*` wiring
      (`:918-936`), wire it to read the **persisted** `InstanceData.GitHubPRURL`
      (`session/storage.go:49`) via `storage.FindInstanceDataByID(sessionUUID)`
      (`session/storage.go:392`) — a plain persisted-state read. Do **NOT** route through
      `registry.WithInstance`/`Acquire` (the liveness-checker path at `:927-934`): for an
      ended session that hydrates a `LiveInstance` and fires `onConstruct` as a side effect of a
      mere URL probe, every sweep, for every orphan (CONCERN — dead-session hydration). This also
      means adoption depends on ended-session `InstanceData` being retained; `FindInstanceDataByID`
      reads that retained store directly.
    - Files: `session/backlog_lifecycle.go`, `server/dependencies.go`

    ##### Task 0.1.1b: Implement `reconcileOrphanedPRs` detector (~5 min)
    - Add `func (l *BacklogLifecycleListener) reconcileOrphanedPRs(ctx, er *EntRepository)` in
      `session/backlog_lifecycle.go`. Query items in `review`/`in_progress`. For each:
      - **Class B** (`item.PrNumber > 0` || `item.PrURL != ""`): fields already present — skip the
        Instance lookup and go straight to the transition.
      - **Class A** (`item.PrNumber == 0` && `item.PrURL == ""`): list `ItemSessions`, resolve the
        work session's Instance PR URL via `l.instancePRLookup`; only adopt when exactly one
        work-session Instance carries a PR URL (conservative — avoids stamping a wrong PR number).
        Parse the PR number (reuse the existing PR-URL parse helper — grep `pull/` in `session/`)
        and write `PrURL`+`PrNumber` via `UpdateBacklogItem`.
      - **Transition** (both classes): if the item is `review`, `TransitionBacklogItemStatus(→
        pr_pending, ExpectedStatus: review)`. If `in_progress`, first
        `TransitionBacklogItemStatus(→ review, ExpectedStatus: in_progress)` then `(→ pr_pending,
        ExpectedStatus: review)` — a failure of the first short-circuits (log, skip).
    - Log adoptions at Info; log (do not swallow) failures at Warning.
    - Files: `session/backlog_lifecycle.go`

    ##### Task 0.1.1c: Register the detector in the sweep (~2 min)
    - In `ReconcileStuck` (`session/backlog_lifecycle.go:806-914`), add a `runStuckDetector`
      entry `reconcile_orphaned_prs` calling `l.reconcileOrphanedPRs(ctx, er)`, placed BEFORE the
      `pr_ready+merge_detection` step (`:912-914`) so a newly-adopted item is polled in the same
      sweep.
    - Files: `session/backlog_lifecycle.go`

  #### Story 0.1.2: Unit test the adoption detector
  **Acceptance Criteria**: table test covering (a) `review` Class-A item (Instance PR, PrNumber==0)
  → adopted to `pr_pending` with fields set; (b) `in_progress` Class-A item → adopted via the
  two-step `in_progress → review → pr_pending`; (c) **`review` Class-B item (`PrNumber>0`, no
  Instance PR) → adopted** (the precondition-failure orphan — this is the BLOCKER-2 regression
  guard); (d) `in_progress` Class-B item → adopted via two-step; (e) item already `pr_pending` →
  untouched; (f) `review` item with no PR anywhere → untouched.
  **Files**: `session/backlog_lifecycle_test.go` (or the existing backlog lifecycle test file)

    ##### Task 0.1.2a: Write `TestReconcileOrphanedPRs_*` (~5 min)
    - Use a fake `instancePRLookup` and an in-memory/ent test storage; assert status transitions
      and PR-field writes for all six cases. Assert a `BacklogStatusEvent` row exists for each
      adoption (including the two-step `in_progress` intermediate `review` hop).
    - Files: `session/backlog_lifecycle_test.go`

---

## Phase 1: Per-item `AutoMergePolicy` flag (8-layer wiring)

Parallel with Phase 0 and Phase 2. Plain proto3 `bool` mirroring `AutoSpawnSession` (ADR-024 §d).
Name: ent `auto_merge_policy`, proto `auto_merge_policy`, domain `AutoMergePolicy`, TS
`autoMergePolicy`.

### Epic 1.1: Persistence + wire format (ent → proto → domain → repo)

  #### Story 1.1.1: The flag persists and round-trips through every layer
  **As a** developer **I want** `AutoMergePolicy` defined once in ent and threaded through proto,
  domain, and repository mapping **so that** it survives create/read/update.
  **Acceptance Criteria**: `go build ./...` passes; a Create with the flag true reads back true;
  a partial Update with the pointer set writes it; ent regen used `--feature sql/upsert`.
  **Files**: `session/ent/schema/backlog_item.go`, `proto/session/v1/backlog.proto`,
  `session/repository.go`, `session/ent_repository_backlog.go`, `server/services/backlog_service.go`

    ##### Task 1.1.1a: ent schema field + regenerate (~4 min)
    - Add `field.Bool("auto_merge_policy").Default(false).Comment("When true (and the global
      backlog:auto-merge-policy switch is on), the item's finished work is auto-PR'd,
      auto-merge-armed, and auto-fixed toward a merged PR without manual clicks.")` alongside
      `auto_spawn_session` in `session/ent/schema/backlog_item.go:43-45`.
    - Regenerate: `cd session/ent && go run -mod=mod entgo.io/ent/cmd/ent generate
      --feature sql/upsert ./schema` (per `.claude/rules/ent-schema-generation.md`).
    - Commit all regenerated `session/ent/` files together.
    - Files: `session/ent/schema/backlog_item.go` (+ generated `session/ent/**`)

    ##### Task 1.1.1b: proto fields in 3 messages + `make proto-gen` (~3 min)
    - Add `bool auto_merge_policy = 26;` to `BacklogItem` (after `:119-120`),
      `bool auto_merge_policy = 12;` to `CreateBacklogItemRequest` (after `:187-188`),
      `bool auto_merge_policy = 14;` to `UpdateBacklogItemRequest` (after `:227-228`) in
      `proto/session/v1/backlog.proto`.
    - Run `make proto-gen`.
    - Files: `proto/session/v1/backlog.proto` (+ generated Go/TS bindings)

    ##### Task 1.1.1c: domain structs (~2 min)
    - `session/repository.go`: add `AutoMergePolicy bool` to `BacklogItemData` (by `:352`) and
      `AutoMergePolicy *bool` to `BacklogItemUpdate` (by `:440`).
    - Files: `session/repository.go`

    ##### Task 1.1.1d: repository mapping — 3 sites (~3 min)
    - `session/ent_repository_backlog.go`: ent→domain `AutoMergePolicy: item.AutoMergePolicy,`
      (by `:145`); Create `.SetAutoMergePolicy(data.AutoMergePolicy)` (by `:210`); Update
      pointer-nil-check `if update.AutoMergePolicy != nil { u.SetAutoMergePolicy(*update.AutoMergePolicy) }`
      (by `:443-445`).
    - Files: `session/ent_repository_backlog.go`

### Epic 1.2: RPC handler read/write

  #### Story 1.2.1: The flag is readable and writable over ConnectRPC
  **Acceptance Criteria**: `backlogItemToProto` emits it; Create handler copies it; Update handler
  wraps it unconditionally (plain-bool convention, NOT presence-gated).
  **Files**: `server/services/backlog_service.go`, `server/services/backlog_service_lifecycle.go`

    ##### Task 1.2.1a: domain→proto + Create handler (~2 min)
    - `backlog_service.go`: `AutoMergePolicy: item.AutoMergePolicy,` in `backlogItemToProto`
      (by `:501`).
    - `backlog_service_lifecycle.go`: `AutoMergePolicy: req.Msg.AutoMergePolicy,` in the Create
      `BacklogItemData` literal (by `:160`).
    - Files: `server/services/backlog_service.go`, `server/services/backlog_service_lifecycle.go`

    ##### Task 1.2.1b: Update handler unconditional wrap (~2 min)
    - `backlog_service_lifecycle.go` (by `:236-237`): `autoMerge := req.Msg.AutoMergePolicy;
      update.AutoMergePolicy = &autoMerge`. Do NOT presence-gate (that is the PipelineMode
      pattern at `:243`, deliberately different — see ADR-024 §d).
    - Files: `server/services/backlog_service_lifecycle.go`

### Epic 1.3: Frontend form + the proto3-reset guard (BLOCKER-class)

  #### Story 1.3.1: The flag is editable and never silently reset by unrelated saves
  **As an** operator **I want** an "Auto-merge policy" checkbox that survives note/AC edits **so
  that** opting an item in is durable.
  **Acceptance Criteria**: checkbox present in the form; the flag is in the form's emitted
  payload and dep array; **`currentFlags()` includes `autoMergePolicy`** so all four partial
  `updateBacklogItem` call sites preserve it.
  **Files**: `web-app/src/components/backlog/BacklogItemForm.tsx`,
  `web-app/src/components/backlog/BacklogItemDetail.tsx`

    ##### Task 1.3.1a: Form state + checkbox + payload (~4 min)
    - `BacklogItemForm.tsx`: `const [autoMergePolicy, setAutoMergePolicy] =
      useState(initialValues?.autoMergePolicy ?? false);` (by `:72`); include `autoMergePolicy`
      in the submit payload (by `:207`) and the `useCallback` dep array (by `:216`); clone the
      `auto-spawn-session` checkbox block (`:399-407`) with id `backlog-auto-merge-policy`,
      `data-testid="backlog-auto-merge-policy-checkbox"`, and hint text describing the trust
      boundary ("Auto-create PR, arm auto-merge, and auto-fix toward merge. Requires the global
      auto-merge switch.").
    - Files: `web-app/src/components/backlog/BacklogItemForm.tsx`

    ##### Task 1.3.1b: Extend `currentFlags()` — the single load-bearing edit (~2 min)
    - `BacklogItemDetail.tsx`: add `autoMergePolicy: item?.autoMergePolicy ?? false,` to the
      `currentFlags()` object (`:306-311`). This fixes all four partial call sites (`:319`,
      `:386`, `:406`, `:440`) at once. Verify no other partial `updateBacklogItem` call omits it.
    - Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

---

## Phase 2: Global kill-switch (`backlog:auto-merge-policy`)

Parallel with Phase 0/1. Provides `l.globalPolicyEnabled() bool` that the behavior gates read.
Default OFF (fail-safe). Reuses the existing feature-flag mechanism — zero new RPC plumbing.

### Epic 2.1: Register + wire the global switch

  #### Story 2.1.1: A single config-persisted switch arms/disarms the whole subsystem
  **As an** operator **I want** one toggle that halts all autonomous PR/merge/fix behavior **so
  that** I have an atomic kill-switch independent of per-item flags.
  **Acceptance Criteria**: the flag appears in the Feature Flags UI (via `knownFeatureFlags`);
  `GetFeatureFlag("backlog:auto-merge-policy")` returns false by default; the listener reads it
  at runtime (each tick), so flipping it takes effect without restart.
  **Files**: `server/services/feature_flag_service.go`, `session/backlog_lifecycle.go`,
  `server/dependencies.go`

    ##### Task 2.1.1a: Add to `knownFeatureFlags` (~2 min)
    - Append `{name: "backlog:auto-merge-policy", description: "Autonomous PR creation,
      auto-merge, and CI/conflict auto-fix for items opted in via their Auto-merge policy flag.
      Master switch; default off."}` to `knownFeatureFlags`
      (`server/services/feature_flag_service.go:16-36`).
    - Files: `server/services/feature_flag_service.go`

    ##### Task 2.1.1b: Runtime reader on the listener — cheap AND live (~4 min)
    - `session/backlog_lifecycle.go`: add field `globalPolicyEnabled func() bool` to the struct
      (`:93`) with a nil-safe accessor `func (l *BacklogLifecycleListener) isGlobalPolicyEnabled()
      bool { if l.globalPolicyEnabled == nil { return false }; return l.globalPolicyEnabled() }`
      and setter `SetGlobalPolicyEnabled(fn func() bool)`.
    - **Do NOT capture a `cfg` snapshot and call `cfg.GetFeatureFlag(...)` in the closure**
      (CONCERN — stale-or-expensive): `UpdateFeatureFlag` calls `config.LoadConfig()` fresh
      (`feature_flag_service.go:139`) and mutates *that* instance, so a `cfg` captured at wiring
      time never observes the runtime toggle — Story 2.1.1's "takes effect without restart" AC
      fails. Re-`LoadConfig()` per call is a disk read + full unmarshal per `pr_pending` item per
      60s tick (contradicting ADR-024 §Neutral's "cheap in-memory field check").
    - Instead wire a live, cheap `FeatureController` (interface at `session_service.go:56-60`):
      add a minimal `atomic.Bool`-backed controller for `backlog:auto-merge-policy`, seeded at
      startup from `cfg.GetFeatureFlag("backlog:auto-merge-policy")` (so persisted state survives
      restart) and toggled by `UpdateFeatureFlag`'s `Enable`/`Disable` (`feature_flag_service.go:149-156`).
      Register it via `sessionService.SetFeatureController("backlog:auto-merge-policy", ctrl)`
      (mirror the existing `"backlog"` controller wiring at `dependencies.go:941`), and set
      `backlogLifecycleListener.SetGlobalPolicyEnabled(ctrl.IsEnabled)` — an atomic load: cheap
      **and** live-correct.
    - Files: `session/backlog_lifecycle.go`, `server/dependencies.go`,
      `server/services/feature_flag_service.go` (controller registration)

    ##### Task 2.1.1c: `policyActive` helper (~2 min)
    - `session/backlog_lifecycle.go`: add `func (l *BacklogLifecycleListener) policyActive(item
      *BacklogItemData) bool { return l.isGlobalPolicyEnabled() && item.AutoMergePolicy }` (and/or
      an `*ent.BacklogItem` overload for the reconciler, which holds `*ent.BacklogItem`). Single
      chokepoint for all three gates.
    - Files: `session/backlog_lifecycle.go`

---

## Phase 3: Behavior 1 — auto-create PR on Complete + CI-passing auto-merge arm gate

Depends on Phase 1 + Phase 2. Implements ADR-024 gate points 1 and 2. **Blocker-1 fix**: the
auto-merge arm is *relocated* from `pushAndCreatePR` (fired at PR-create, before CI starts) into
the reconciler's healthy branch, where it is gated on a positive `ciPassing` signal.

### Epic 3.0: CI tri-state signal (failing / pending / passing) — Blocker-1 core

  #### Story 3.0.1: The reconciler can distinguish "CI passing" from "CI pending" from "CI failing"
  **As** the merge/notify subsystem **I want** a positive "all checks concluded and passing"
  signal (not merely "no terminal failure") **so that** pending / not-yet-created CI never reads
  as green for either the ready-notify or the auto-merge arm.
  **Acceptance Criteria**: `PRStatus` exposes a `CIPending bool` (checks present but not all
  concluded); `ReconcilePRPending` maps the tri-state into `github.PRInfo.CheckConclusion`
  (`failure` / `pending` / `""`) so `prReadyToMergeSolo` returns false while pending; a local
  `ciPassing := !prStatus.CIFailing && !prStatus.CIPending` is available for the arm gate.
  **Files**: `session/git/worktree_git.go`, `session/backlog_lifecycle.go`

    ##### Task 3.0.1a: Add `CIPending` to `PRStatus` + derive it in `GetPRStatus` (~4 min)
    - `session/git/worktree_git.go`: add a `CIPending bool` field to `PRStatus` (`:330`, alongside
      `CIFailing`). In the `StatusCheckRollup` loop (`:512-533`), besides the existing terminal-
      failure detection, set `status.CIPending = true` when at least one check is still non-terminal
      — i.e. `Status` (`:464`) is not `COMPLETED` (queued/in_progress) or `Conclusion` is empty/
      `PENDING` — and it is not already a terminal failure. Terminal-failure detection is unchanged;
      `CIPending` is purely additive. Document that an empty rollup (no checks at all) leaves both
      `CIFailing` and `CIPending` false — the "no CI configured" case, still treated as passing
      (see Minor: no-checks-yet residual below).
    - Files: `session/git/worktree_git.go`

    ##### Task 3.0.1b: Map the tri-state into `PRInfo` in the healthy branch (~3 min)
    - `session/backlog_lifecycle.go`, `ReconcilePRPending` healthy branch (`:1593-1602`): replace
      the `if prStatus.CIFailing { info.CheckConclusion = "failure" }` with a tri-state map:
      `failure` when `CIFailing`, else `pending` when `CIPending`, else leave `""`. `prReadyToMergeSolo`
      already returns false for any `CheckConclusion` outside `{"success",""}` (`stuck_decisions.go:88-93`),
      so `pending` now correctly blocks both the notify and (via `ciPassing`) the arm. Compute
      `ciPassing := !prStatus.CIFailing && !prStatus.CIPending` here for Epic 3.1.
    - Files: `session/backlog_lifecycle.go`

### Epic 3.1: Arm auto-merge only when policy-active AND CI actually passing (relocated)

  #### Story 3.1.1: Auto-merge is armed in the reconciler, only for policy-active + CI-passing items
  **As an** operator **I want** auto-merge armed only when the item is policy-active **and CI has
  actually passed** **so that** (a) non-policy PRs surface a manual-merge notification instead of
  merging unreviewed, and (b) no PR is armed to merge to unprotected `main` while its checks are
  still running (Blocker-1: `gh pr merge --auto` does NOT wait for non-required checks).
  **Acceptance Criteria**:
  - `pushAndCreatePR` no longer calls `EnablePRAutoMerge` (it only creates the PR + transitions to
    `pr_pending`). The arm moves to the reconciler.
  - In `ReconcilePRPending`'s healthy branch, `EnablePRAutoMerge` is called only when
    `l.policyActive(item) && ciPassing && prReadyToMergeSolo(info)`. Arming is idempotent
    (`gh pr merge --auto` is a no-op when already enabled), so re-calling on subsequent healthy
    ticks is safe; once green + mergeable the PR merges and the next poll detects merged→`done`.
  - Non-policy items (or policy items still pending) are NOT armed; they still get merged→`done`
    detection and the Behavior-3 notify. The existing auto-merge-failure WARNING notify is preserved.
  **Files**: `session/backlog_lifecycle.go`

    ##### Task 3.1.1a: Remove the arm from `pushAndCreatePR` (~2 min)
    - Delete the `EnablePRAutoMerge` block (`:1468-1485`) from `pushAndCreatePR`. The function now
      ends: create PR → write PR fields → transition to `pr_pending`. (Non-policy items therefore
      never arm — the deliberate behavior change bringing the pre-existing unconditional auto-merge
      under policy control, ADR-024 §a.)
    - Files: `session/backlog_lifecycle.go`

    ##### Task 3.1.1b: Add `EnablePRAutoMerge` to the `prPendingChecker` interface (~2 min)
    - `session/backlog_lifecycle.go`: add `EnablePRAutoMerge(prNumber int) error` to the
      `prPendingChecker` interface (`:55-58`) so the reconciler's checker `g` can arm. The
      production factory returns a `git.GitWorktree`, which already implements it (`:67` shows the
      same method on `prCreator`) — no factory change needed; test fakes add a stub.
    - Files: `session/backlog_lifecycle.go`

    ##### Task 3.1.1c: Arm in the healthy+passing branch (~3 min)
    - In `ReconcilePRPending`'s healthy branch (`:1604`), when `prReadyToMergeSolo(info)` is true:
      if `l.policyActive(item) && ciPassing`, call `g.EnablePRAutoMerge(item.PrNumber)`, preserving
      the existing success/failure notify+log (the `:1475-1485` block, relocated here). Then call
      `markPRReadyUnmerged` (Phase 5 branches the copy) regardless of policy — for policy items it
      is the fallback signal if an armed merge never lands; for non-policy items it is the manual-
      merge prompt. No error swallowing.
    - Files: `session/backlog_lifecycle.go`

### Epic 3.2: Auto-PR on Complete for review-skipping policy items (edge case E7)

  #### Story 3.2.1: A policy item with `SkipReviewGate` gets a PR instead of `done`
  **As an** operator **I want** a `SkipReviewGate` + policy item's completed work to auto-create a
  PR **so that** it does not silently reach `done` with no PR (E7).
  **Acceptance Criteria**: in `onSessionExited`, a policy-active item with `SkipReviewGate` is
  routed to `pushAndCreatePR` (transition `in_progress → review` first to satisfy
  `pushAndCreatePR`'s `ExpectedStatus: review` precondition, then call it in a bounded goroutine)
  instead of `in_progress → done`; a non-policy `SkipReviewGate` item is unchanged (→ `done`); a
  policy item WITHOUT `SkipReviewGate` is unchanged (→ review gate → PASS → `pushAndCreatePR`,
  which already auto-creates). Precedence: policy > SkipReviewGate.
  **Files**: `session/backlog_lifecycle.go`

    ##### Task 3.2.1a: Policy branch in `onSessionExited` (~5 min)
    - In the work-exit fork (`:444-481`), before the `toStatus`/review-gate logic, compute
      `autoPR := l.policyActive(item) && item.SkipReviewGate`. When `autoPR`: set
      `toStatus = BacklogStatusReview`, do the guarded `in_progress → review` transition (existing
      precondition block `:449-457`), then spawn a bounded goroutine (reuse the `reviewSem`
      pattern `:470-480`) that calls `l.pushAndCreatePR(ctx, item, is)` — NOT `spawnReviewGate`.
      Leave the non-`autoPR` paths (`:444-481`) exactly as they are.
    - Note: `is` here is the exited work `ItemSessionSummary` — exactly what `pushAndCreatePR`
      needs. Do not create a second spawn path (reuse `pushAndCreatePR`, ADR-024 §b).
    - Files: `session/backlog_lifecycle.go`

    ##### Task 3.2.1b: `pushAndCreatePR` entry idempotency guard (~2 min)
    - CONCERN: `pushAndCreatePR` has no "already has a PR / already past review" short-circuit, and
      this epic adds a *second* caller (`onSessionExited`). A retried/duplicated `EventExited` or a
      race with the PASS path could create two PRs for one item. Add an entry guard at the top of
      `pushAndCreatePR`: reload the item and return early (log at Info) when `item.PrNumber > 0` or
      the item's status is already `pr_pending`/`done`. This also hardens the Class-B orphan story
      (a duplicate call after a precondition-failure won't stamp a second PR).
    - Files: `session/backlog_lifecycle.go`

---

## Phase 4: Behavior 2 — auto-fix loop gate (depends on Phase 0 + 1 + 2)

Gates the existing fix-spawn on policy; the loop machinery, guards, and shared cap are unchanged.

### Epic 4.1: Policy-gate the fix-spawn branches

  #### Story 4.1.1: Fixes are spawned only for policy-active items; detection stays universal
  **As an** operator **I want** the CI/conflict/closed-PR auto-fix session to spawn only for
  policy-active items **so that** non-policy items still get merged/notify detection but are not
  auto-reworked.
  **Acceptance Criteria**: both `AutoReopenForPRFix` call sites (`:1578` closed-PR branch,
  `:1626` unhealthy branch) are guarded by `l.policyActive(item)`; the merged→`done` branch, the
  `GetPRStatus` poll, and `markPRReadyUnmerged` remain unconditional; the churn-guard order
  inside `AutoReopenForPRFix` and the shared `maxAutoReworkIterations=3` cap are untouched.
  **Files**: `session/backlog_lifecycle.go`

    ##### Task 4.1.1a: Guard the two spawn calls (~4 min)
    - In `ReconcilePRPending`, wrap the closed-PR-branch `AutoReopenForPRFix` (`:1578`) and the
      unhealthy-branch `AutoReopenForPRFix` (`:1626`) in `if l.policyActive(item) { ... } else {
      log.InfoLog... "fix skipped by policy" }`. `item` is the `*ent.BacklogItem` from
      `FindPRPendingItems` — use the `*ent.BacklogItem` `policyActive` overload (Task 2.1.1c).
    - Do NOT gate the merged→`done` transition (`:1531+`), the `GetPRStatus` poll (`:1545`), or
      `markPRReadyUnmerged` — detection/notification must work for all items.
    - Files: `session/backlog_lifecycle.go`

  #### Story 4.1.2: Verify the shared-cap terminal state for the PR-fix loop
  **Acceptance Criteria**: an integration/unit test confirms a policy item hitting
  `maxAutoReworkIterations` in the PR-fix loop stays in `pr_pending`, fires `notifyReworkCapHit`
  (durable `rework_cap` row), and does not spawn — reusing the existing cap (no new counter).
  **Files**: `server/services/backlog_service_triage_test.go`

    ##### Task 4.1.2a: Cap-terminal test for PR-fix (~5 min)
    - If not already covered near `AutoReopenForPRFix` (`:568-623`) tests, add a case asserting
      cap-hit → no spawn + `notifyReworkCapHit`. Assert the churn guard (`hasActiveWorkSession`
      early-return) still short-circuits with zero status transition when a fix is in flight.
    - Files: `server/services/backlog_service_triage_test.go`

---

## Phase 5: Behavior 3 — ready-to-merge notification (extend `markPRReadyUnmerged`)

Depends on Phase 0 (reachability) + Phase 1 (policy flag for copy) + **Epic 3.0 (CI tri-state)**.
Behavior 3 largely EXISTS: `markPRReadyUnmerged` (`:1637-1666`) fires a durable, notify-once
WARNING only when the healthy branch's `prReadyToMergeSolo(info)` is true. **Blocker-1**: before
Epic 3.0, `prReadyToMergeSolo` saw pending CI as green (`CheckConclusion==""`), so the notify
could fire before CI was truly green. With Epic 3.0's tri-state map, pending CI sets
`CheckConclusion="pending"` → `prReadyToMergeSolo` returns false → **the ready-notify no longer
fires while CI is pending**. This phase makes the copy reflect policy state and adds a regression
test that pending CI does not notify.

### Epic 5.1: Policy-aware notification copy

  #### Story 5.1.1: The "ready to merge" notice distinguishes auto-merge from manual-merge
  **As an** operator **I want** the ready-to-merge notification to say "will auto-merge" for
  policy-active items and "merge it on GitHub" for non-policy items **so that** I know whether
  action is required.
  **Acceptance Criteria**: `markPRReadyUnmerged` message branches on policy state; the durable
  notify-once dedup (`NotifiedAt`) and 30-min `prReadyThreshold` behavior are unchanged; still
  fires exactly once per ready-episode (re-arms after a fix cycle resolves the row).
  **Files**: `session/backlog_lifecycle.go`

    ##### Task 5.1.1a: Branch the notification message (~3 min)
    - In `markPRReadyUnmerged` (`:1637-1666`), pass/read the item's policy state and adjust the
      message text: policy-active → "PR #N is green and mergeable; auto-merge is armed and it will
      merge once checks settle." vs non-policy → existing "... Merge it on GitHub." Keep the
      `MarkStuck`/`NotifiedAt`/threshold logic identical.
    - The caller (`ReconcilePRPending` healthy branch `:1604-1605`) has the `*ent.BacklogItem`;
      thread `policyActive` (or the raw flag) into `markPRReadyUnmerged`.
    - Files: `session/backlog_lifecycle.go`

  #### Story 5.1.2: Reachability + exactly-once + pending-CI-suppression test
  **Acceptance Criteria**: a test confirms an adopted/policy item reaching a green+mergeable
  `pr_pending` state fires `markPRReadyUnmerged` once; a second sweep with the row already
  notified does NOT re-fire; a fix cycle that resolves then re-opens the row re-arms it; **and a
  `pr_pending` item whose `GetPRStatus` reports `CIPending` does NOT fire the ready-notify**
  (Blocker-1 regression guard).
  **Files**: `session/backlog_lifecycle_test.go`

    ##### Task 5.1.2a: Notify-once / re-arm / pending-suppression test (~5 min)
    - Include a case where the fake `GetPRStatus` returns `CIPending: true, Mergeable: "MERGEABLE"`
      and assert `markPRReadyUnmerged` does NOT notify and auto-merge is NOT armed.
    - Files: `session/backlog_lifecycle_test.go`

---

## Phase 6: Feature registry + tests + e2e

Depends on all prior phases. Per `.claude/rules/feature-registry.md` +
`.claude/rules/e2e-test-conventions.md`. This is NOT a new session-creation mode, so the
7-touchpoint session-creation rule does NOT apply; the feature-registry + e2e rules DO.

### Epic 6.1: Backend/Go unit coverage for the gates

  #### Story 6.1.1: The three gate points are unit-tested
  **Acceptance Criteria**: tests assert (a) auto-merge armed only when `policyActive` **AND
  `ciPassing`** — never at PR-create, never while `CIPending`, and never for non-policy items;
  (b) E7 routing (policy+skipReview → PR, non-policy+skipReview → done, policy+review → unchanged);
  (c) fix-spawn gated on `policyActive` with detection universal; plus the proto3-reset Go-side
  round-trip (Create true → read true; partial Update with pointer nil leaves value intact).
  **Files**: `server/services/backlog_service_test.go`, `session/backlog_lifecycle_test.go`

    ##### Task 6.1.1a: Gate-point tests (~5 min)
    - Mirror the `AutoSpawnSession` template at `backlog_service_test.go:1922-1985` for the
      persistence round-trip; add gate assertions using fakes for `globalPolicyEnabled` and the
      PR checker/fix spawner.
    - Files: `server/services/backlog_service_test.go`, `session/backlog_lifecycle_test.go`

### Epic 6.2: Frontend Jest — proto3-reset regression guard

  #### Story 6.2.1: An unrelated partial save does not reset `autoMergePolicy`
  **As a** developer **I want** a test proving a save-notes call preserves `autoMergePolicy`
  **so that** the AutoSpawnSession silent-reset bug cannot recur for this flag.
  **Acceptance Criteria**: a Jest/RTL test renders `BacklogItemDetail` with an item where
  `autoMergePolicy=true`, triggers a notes save, and asserts the `updateBacklogItem` payload
  includes `autoMergePolicy: true` (via `currentFlags()`).
  **Files**: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

    ##### Task 6.2.1a: currentFlags-preservation test (~5 min)
    - Run `cd web-app && npx jest --no-coverage --testPathPatterns="BacklogItemDetail"`.
    - Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

### Epic 6.3: Feature registry entries

  #### Story 6.3.1: Registry reflects the new backend + frontend surface
  **Acceptance Criteria**: per-feature JSON under `docs/registry/features/` for the backend
  gate(s) and the frontend checkbox; `make registry-generate` run; `coverage-gaps.json` count
  does not grow.
  **Files**: `docs/registry/features/backend/backlog-auto-merge-policy.json`,
  `docs/registry/features/frontend/backlog-auto-merge-policy-toggle.json`

    ##### Task 6.3.1a: Create per-feature JSONs + regenerate (~4 min)
    - Backend entry `id: "backlog:auto-merge-policy"` (or the existing backlog RPC ids the change
      touches, with `tested:true` + testIds); frontend entry
      `id: "backlog-auto-merge-policy-toggle"`, `filePath:
      "web-app/src/components/backlog/BacklogItemForm.tsx"`, `testIds` matching the e2e describe.
    - Run `make registry-generate`; verify `docs/registry/coverage-gaps.json` did not grow.
    - Files: `docs/registry/features/backend/backlog-auto-merge-policy.json`,
      `docs/registry/features/frontend/backlog-auto-merge-policy-toggle.json`

### Epic 6.4: e2e Playwright

  #### Story 6.4.1: The toggle is settable and durable end-to-end
  **Acceptance Criteria**: a spec sets the "Auto-merge policy" checkbox on a backlog item, saves,
  reloads, and asserts it stays checked (guards the proto3-reset trap at the UI boundary); uses
  `data-testid` locators, no `waitForTimeout`, feature-annotation header.
  **Files**: `tests/e2e/backlog-auto-merge-policy.spec.ts`,
  `tests/e2e/pages/BacklogPage.ts` (extend existing helper)

    ##### Task 6.4.1a: Spec + page-helper method (~5 min)
    - `// @feature backlog:auto-merge-policy` header; `test.describe('backlog-auto-merge-policy',
      ...)`; toggle `getByTestId('backlog-auto-merge-policy-checkbox')`, save, reload, re-assert
      checked. Add a `setAutoMergePolicy(checked)` helper to `tests/e2e/pages/BacklogPage.ts`.
    - Run against `http://localhost:8544` per `.claude/rules/e2e-test-conventions.md`.
    - Files: `tests/e2e/backlog-auto-merge-policy.spec.ts`, `tests/e2e/pages/BacklogPage.ts`

---

## Accepted Risks / Residuals

- **No-checks-yet vs no-CI ambiguity (Minor).** An empty `StatusCheckRollup` — a repo with no CI
  *or* a PR whose checks GitHub has not yet created — leaves both `CIFailing` and `CIPending`
  false, so `ciPassing` is true and the item can arm/notify. Epic 3.0 closes the primary hole
  (checks that exist but are running now read as pending), but cannot distinguish "no checks ever"
  from "no checks *yet*" without knowing the repo's expected/required checks (out of scope — we do
  not add required-check config). Accepted: this repo always produces CI checks, and the arm is
  now poll-gated in the reconciler (≥1 tick after PR-create), by which point GitHub has created
  the check runs, so the realistic window is negligible.
- **Shared rework cap starves the PR-fix loop (CONCERN, accepted per ADR-024 §b).** A review-heavy
  item may get only 1 PR-fix attempt before cap escalation. Mitigation: Task 5.1.1a-adjacent —
  `notifyReworkCapHit`'s PR-fix message states the budget was shared with review-rework so the
  operator understands a one-fix-then-escalate outcome. A dedicated PR-fix budget is deferred
  unless premature escalations appear in practice.
- **Global switch cannot un-arm an already-armed `--auto` merge (residual, ADR-024 §c).** Relocating
  the arm to the reconciler *shrinks* this window (arming happens only on a CI-passing tick, and an
  already-passing PR merges within ~1 tick), but a PR armed on one tick that merges before the
  operator flips the switch OFF is still unpreventable from our side. Documented, not designed away.
- **GitHub-outage terminal escalation (Minor).** `IsPRMerged`/`GetPRStatus` errors `continue`, so a
  policy item can sit in `pr_pending` during an outage with no operator signal. Deferred (no
  "N consecutive failed reconciles" stuck reason in this feature); noted for a follow-up.

## Verification Gate (before shipping)

- `make build && make test` green; `make lint` green (gofmt first).
- `cd web-app && npx jest --no-coverage --testPathPatterns="BacklogItemDetail"` green.
- e2e: start test server, `cd tests/e2e && npx playwright test backlog-auto-merge-policy.spec.ts`.
- `make registry-generate` produces no net `coverage-gaps.json` growth.
- Manual smoke: global switch OFF ⇒ no auto-merge/auto-fix even for a policy-flagged item; global
  ON + item flag ⇒ completed `SkipReviewGate` item auto-creates a PR, and **once CI goes green**
  the reconciler arms auto-merge and the item reaches `done` (verify auto-merge is NOT armed while
  CI is still running); a manually-orphaned item (both a Path-B Instance-only PR and a Class-B
  `PrNumber>0`+`review` precondition-failure orphan) is adopted into `pr_pending` on the next sweep;
  flipping the global switch OFF at runtime immediately stops new arming without a restart.
- ent regen confirmed run with `--feature sql/upsert`; all `session/ent/` changes committed
  together.

## Task Count Summary

- Phase 0: 1 epic, 2 stories, 4 tasks
- Phase 1: 3 epics, 3 stories, 8 tasks
- Phase 2: 1 epic, 1 story, 3 tasks
- Phase 3: 3 epics (3.0 CI tri-state, 3.1 relocated arm, 3.2 E7 + entry guard), 3 stories, 7 tasks
- Phase 4: 1 epic, 2 stories, 2 tasks
- Phase 5: 1 epic, 2 stories, 2 tasks
- Phase 6: 4 epics, 4 stories, 4 tasks
- **Totals: 14 epics, 17 stories, 30 tasks**

Delta from the reviewed plan (+1 epic, +1 story, +5 tasks): Blocker-1 added Epic 3.0 (CI
tri-state, 2 tasks) and split the arm into relocate/interface/arm (3 tasks — was 1); Blocker-2
broadened the Phase-0 detector (no task count change — rewrote 0.1.1a/b and 0.1.2); the
idempotency CONCERN added Task 3.2.1b. The kill-switch and hydration CONCERNs were fixed in place
(no new tasks).
