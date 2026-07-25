# ADR-025: Orphan-PR Adoption Reconciler (Phase-0 Desync Fix)

**Date**: 2026-07-17
**Status**: Accepted
**Deciders**: Tyler Stapler
**Related**: ADR-024 (trust-boundary controls). Phase 0 of
`project_plans/backlog-pr-mergeability-policy/implementation/plan.md`.

---

## Context

Behavior 2 (auto-fix loop) is *defined* as "built on `ReconcilePRPending`." That reconciler is
**structurally unreachable** for the exact class of items the feature targets.

There is exactly **one** production code path that transitions a backlog item into `pr_pending`:
`pushAndCreatePR` (`session/backlog_lifecycle.go:1487-1492`), fired **only** on a PASS review
verdict, and preconditioned on the item currently being `review`. Item `PrNumber` / `PrURL` are
written in only that one place. `FindPRPendingItems` (`session/storage_backlog.go:550-577`)
filters `status == pr_pending AND PrNumber > 0`, and `ReconcilePRPending` additionally
early-`continue`s on `PrNumber == 0 || PrURL == ""` (`backlog_lifecycle.go:1515`).

Two independent roads produce an item with a **live GitHub PR but invisible to the reconciler**:

1. **Manual Review-Queue path (Path B).** The operator clicks "Create PR" →
   `SessionService.RunOneShot` (`server/services/session_service.go:3405-3495`), which runs an
   LLM that opens a PR and stamps the URL onto the **session `Instance`** (`inst.SetGitHubPR`,
   `:3480`) — it never touches the backlog item's `status`, `PrNumber`, or `PrURL`. This is a
   two-writer split-brain: PR truth lives on the `Instance`, the reconciler's world-view lives on
   the item, and nothing reconciles them. This is PR #157's exact state (`review`, `PrNumber==0`,
   green CI, `DIRTY`/`CONFLICTING`).
2. **Precondition-failure road.** Even on the automated PASS path, `pushAndCreatePR` writes the
   item's `PrURL` / `PrNumber` at `:1460` **before** its `review → pr_pending` transition at
   `:1489` (preconditioned on `ExpectedStatus: "review"`). If a concurrent
   `AutoReopenAfterFailedReview` already rolled the item to `in_progress`, the transition fails,
   the code logs and returns — leaving the item with **`PrNumber > 0` + `PrURL` set but status
   still `review`/`in_progress`** while the PR is live on GitHub. This is **Class B**. It is
   unreachable by all three existing rescue paths: the original adoption detector filtered
   `PrNumber == 0`; `FindPRPendingItems` and `BackfillMissingPRNumbers`
   (`storage_backlog.go:550-582`) both require status `pr_pending`. It is therefore a *permanent*
   orphan, not the transient one an earlier draft assumed — which is why the detector's filter is
   broadened (Decision Part 1) rather than gated solely on `PrNumber == 0`. The
   field-write-before-transition order is kept deliberately (writing fields first means the PR
   reference is never lost if the transition fails), with the broadened detector as the backstop.

Behavior 1 done correctly (policy items route through Path A / `pushAndCreatePR`) makes *new*
policy items reach `pr_pending` for free — but does nothing for already-orphaned items or the
precondition-failure road. Shipping Behavior 2 without a reconciliation backstop delivers a loop
that never engages for the very items that motivate the feature.

**Scope decision: this is a Phase-0 prerequisite bug fix, IN SCOPE.** It is a hard dependency of
Behavior 2 and Behavior 3 (both run only inside `ReconcilePRPending`).

---

## Decision

Add an **orphan-PR adoption detector** to the existing `ReconcileStuck` sweep, and rely on
Behavior 1's Path-A routing to prevent *new* orphans — a two-part "adopt existing, prevent
future" strategy.

### Part 1 — Adoption detector (the backstop)

A new detector, run as one of the panic-isolated `runStuckDetector` steps in `ReconcileStuck`
(`session/backlog_lifecycle.go:806-914`), that:

1. Finds backlog items in `review` or `in_progress` that have a live PR reachable by **either**
   road (BLOCKER-2 — the original `PrNumber == 0`-only filter missed Class B entirely):
   - **Class A (Path-B):** item `PrNumber == 0` **and** `PrURL == ""`, but its linked session
     `InstanceData` carries a non-empty `github_pr_url`. Extract the PR number from the Instance
     URL and write `PrURL` / `PrNumber` onto the item (`UpdateBacklogItem`).
   - **Class B (precondition-failure road):** item already has `PrNumber > 0` **or** `PrURL != ""`
     (fields written by `pushAndCreatePR:1460` *before* its `pr_pending` transition failed). No
     field write needed — the PR reference is already on the item; it just needs the transition.
   The Instance lookup for Class A reads persisted `InstanceData.GitHubPRURL` directly (a
   non-hydrating `FindInstanceDataByID`-style read), NOT the live registry `Acquire` path, to
   avoid hydrating a `LiveInstance` / firing `onConstruct` for a dead session on every sweep.
2. Transitions the item to `pr_pending` via **`TransitionBacklogItemStatus`** (never a raw ent
   `SetStatus`) so a `BacklogStatusEvent` audit row is written. `review → pr_pending` is a valid
   direct transition; **`in_progress → pr_pending` is NOT** (`session/domain/backlog.go`
   transition table — `in_progress` reaches only `review`/`ready`/`refining`/`idea`), so an
   `in_progress` orphan is routed via the two valid hops `in_progress → review → pr_pending`,
   each guarded by its `ExpectedStatus` precondition; a first-hop failure short-circuits.
3. Runs for **all** items, not just policy-enabled ones — it fixes the split-brain so the
   reconciler can observe ground truth. The downstream auto-*fix* spawn remains policy-gated
   (ADR-024 gate point 3), so an adopted **non-policy** legacy orphan gets merged→`done`
   detection and the "ready to merge" notification, but is **not** auto-fixed — the operator
   handles it. An adopted **policy** item additionally enters the auto-fix loop.

### Part 2 — Prevent future orphans (single writer)

Behavior 1 routes every policy-enabled item's PR creation through `pushAndCreatePR` (the single
writer that stamps item PR fields *and* transitions to `pr_pending`; auto-merge is armed later, in
the reconciler's CI-passing branch, per ADR-024 gate point 1). Policy items therefore never take
Path B. `RunOneShot` is left intact for its existing non-backlog and manual uses.

**Prevent Class-B duplicates (entry guard).** Because Behavior 1's E7 path adds a *second*
`pushAndCreatePR` caller (`onSessionExited`), `pushAndCreatePR` gains an entry idempotency guard
(plan Task 3.2.1b): it returns early when the item already has `PrNumber > 0` or is already
`pr_pending`/`done`. This prevents a retried `EventExited` or a race with the PASS path from
creating a second PR for a Class-B orphan the detector is about to adopt.

---

## Alternatives Rejected

### (a) Full Path-A/Path-B convergence — rip out or rewire `RunOneShot`
Make the Review-Queue "Create PR" button always invoke the backlog PR-creation machinery and
eliminate the out-of-band writer entirely. Rejected as the *primary* Phase-0 mechanism because
`RunOneShot` is a generic, session-scoped one-shot runner that also serves **non-backlog**
sessions (the button is keyed on `sessionId`, not a backlog item); rewiring it is a larger,
riskier change to `session_service.go` and the Review-Queue frontend than the desync fix
requires. It is partially *subsumed* by Behavior 1 (policy items never use Path B) and remains a
reasonable future cleanup, but the adoption detector is the smaller, additive, lower-risk change
that also catches the precondition-failure road, which convergence alone would not.

### (b) Adoption detector only, no Behavior-1 routing guarantee
Rely solely on the detector to mop up orphans after the fact. Rejected: it would let policy items
keep creating orphans on every run and depend on a ≤60s-latency sweep to repair each one — a
correctness-by-cleanup design. Behavior 1's single-writer routing is the structural fix;
the detector is the backstop for the two roads that routing cannot close.

---

## Consequences

**Positive**
- `ReconcilePRPending` (and therefore Behaviors 2 and 3) become reachable for the orphaned and
  precondition-failed item classes that motivate the feature, including the live PR #157 class.
- Additive and low-risk: a new panic-isolated detector, no change to `RunOneShot` or the
  transition table (`review → pr_pending` and `in_progress → pr_pending`-via-`review` are already
  valid / reachable).
- Every adoption write goes through `TransitionBacklogItemStatus`, preserving the audit trail and
  the desync-diagnosis evidence.

**Negative**
- The split-brain root cause (two writers) is *contained*, not *eliminated* — `RunOneShot` can
  still create an Instance-only PR, now healed on the next sweep rather than prevented. Full
  convergence remains a future option.
- The detector makes a per-orphan judgement ("this Instance PR belongs to this item"); it must be
  conservative (only adopt when the linked session Instance unambiguously carries the PR) to
  avoid stamping a wrong PR number onto an item.

**Neutral**
- Retroactively unsticking the *currently* stuck items / merging PR #157 by hand remains a
  separate maintenance task (requirements out-of-scope); this ADR prevents the class going
  forward and makes the reconciler able to drive such items once adopted.
