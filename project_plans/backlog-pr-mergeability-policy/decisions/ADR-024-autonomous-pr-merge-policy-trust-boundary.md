# ADR-024: Autonomous PR-Merge Policy — Trust-Boundary Controls

**Date**: 2026-07-17
**Status**: Accepted
**Deciders**: Tyler Stapler
**Related**: ADR-025 (Phase-0 desync fix), ADR-022 (headless triage), requirements
`project_plans/backlog-pr-mergeability-policy/requirements.md`

---

## Context

The backlog "software factory" finishes work but never reaches a merged PR without repeated
manual operator clicks. Three manual gates compound into "nothing gets merged":

1. PR creation is a manual Review-Queue button (`RunOneShot`).
2. CI-failure / merge-conflict fix loops are not automatic for items that never reach
   `pr_pending`.
3. The "ready to merge" signal is not surfaced on genuine mergeability.

Closing these gates is a **trust-boundary change**. With GitHub `allow_auto_merge` enabled on
the repo and **no branch protection on `main`**, `EnablePRAutoMerge`
(`session/git/worktree_git.go:559-572`, `gh pr merge <n> --auto --squash`) merges to `main` with
**zero human review**.

**Correction to an earlier premise (Blocker-1).** An earlier draft of this ADR asserted that
GitHub's `--auto` waits for "required status checks pass," so with no required checks that is
"effectively CI green." **This is false.** `gh pr merge --auto` only waits on checks that branch
protection makes *required*; with **no** required checks configured, `--auto` does not wait for
non-required checks at all — it merges as soon as the PR is otherwise mergeable, **while CI is
still running**. Arming `--auto` at PR-create time therefore does not gate on CI at all. The design
must not rely on `--auto` to enforce "CI green": it must poll CI itself and arm auto-merge only
after an independent `GetPRStatus` confirms CI is *positively passing* (all checks concluded and
successful — not merely "no terminal failure yet," which also matches pending / not-yet-created
checks). See §(a) gate point 1 and §(c).

A critical, already-true fact: `pushAndCreatePR` calls `EnablePRAutoMerge` *unconditionally*
today (`session/backlog_lifecycle.go:1475`) for every review-passing backlog item. Part of this
trust boundary is already crossed; this ADR brings that behavior under an explicit, operator-
controlled policy rather than leaving it implicit.

`prReadyToMergeSolo` (`session/stuck_decisions.go:65-95`) *deliberately drops* the
`ApprovedCount > 0` requirement, because a solo self-authored PR can never receive an approval —
so an approval gate is structurally impossible here. **CI-green is the entire safety bar.** That
is the exposure this ADR must design controls around.

---

## Decision

Introduce an **opt-in autonomous PR-merge policy** governed by **two controls that are both
required (logical AND)** — a per-item opt-in *and* a global kill-switch — plus a set of
poll-anchored, GitHub-truth-sourced gates. No single item automates unless **both** the global
switch is ON **and** that item's per-item flag is set.

### (a) Two required controls — where each gates, and defaults

| Control | Home | Default | Role |
|---|---|---|---|
| **Global kill-switch** `backlog:auto-merge-policy` | `knownFeatureFlags` (`server/services/feature_flag_service.go:16-36`), persisted via `config.GetFeatureFlag`/`SetFeatureFlag` (`config/config.go:853-865`) | **OFF (false)** | Master arming switch for the whole subsystem. One atomic lever. |
| **Per-item flag** `AutoMergePolicy` | new `bool` column on `BacklogItem`, plain-bool per §(d) | **false** | Opt-in per item: "drive this item's finished work to a merged PR autonomously." |

The effective predicate at every automation point is:

```
policyActive(item) := globalSwitchEnabled() && item.AutoMergePolicy
```

**Three gate points**, all checking `policyActive(item)` (defense in depth — the pitfalls
research's "gate at reconciler entry so a single flip freezes the subsystem"):

1. **Auto-merge arming** — **relocated** out of `pushAndCreatePR` (`:1475`, where it fired at
   PR-create before CI started) into the `ReconcilePRPending` healthy branch, and armed only when
   `policyActive(item)` **AND CI is positively passing** (`ciPassing := !CIFailing && !CIPending`,
   confirmed by that tick's own `GetPRStatus` poll). This is the Blocker-1 fix: because `--auto`
   does not wait for non-required checks (see Context correction), arming at PR-create would merge
   to unprotected `main` while checks were still running. Arming only on a CI-passing reconcile
   tick makes "CI green" an *enforced* precondition of the merge rather than an assumed one. When
   `policyActive` is false, the PR is still created and the item surfaces the Behavior-3 "ready to
   merge" notification for a manual merge — the deliberate behavior change bringing the
   pre-existing unconditional auto-merge under policy control.
2. **Auto-PR-on-Complete for review-skipping items** — `onSessionExited`
   (`session/backlog_lifecycle.go:444-481`). A `policyActive` item whose `SkipReviewGate` is set
   is routed to `pushAndCreatePR` instead of the `in_progress → done` shortcut (resolves edge
   case E7 — a `SkipReviewGate` item otherwise reaches `done` with **no PR at all**).
3. **Auto-fix spawn loop** — the two `AutoReopenForPRFix` call sites in `ReconcilePRPending`
   (`session/backlog_lifecycle.go:1578` closed-PR branch, `:1626` unhealthy branch). When
   `policyActive` is false, the merged→`done` and "ready to merge" branches still run (so
   detection/notification work for all items), but no fix session is spawned.

**Why both, not either.** The per-item flag is an *arming* control, not a *halting* control. The
requirements' 2am failure mode: a systemic problem (a dependency bump that makes CI green on
broken code; a fix loop that starts churning) with 15+ policy items in flight would force the
operator to toggle every item individually through `UpdateBacklogItem`, racing the 60s reconcile
tick and already-armed GitHub `--auto` merges. The global switch gives **one atomic halt**. The
per-item flag alone cannot provide that; the global switch alone would be too coarse (it could
not express "this item may automate, that one may not"). Both are therefore required.

**Global default OFF is a deliberate, accepted behavior change — NOT "additive" (Concern).** The
requirements call the policy "additive / must not regress existing paths," but today
`EnablePRAutoMerge` is called *unconditionally* for every review-passing item
(`backlog_lifecycle.go:1475`): review-passing NON-policy items auto-merge today, and after this
ships they will not until the operator flips the global switch ON and opts items in. We make this
regression **explicit and accept it by design**: today's unconditional auto-merge to an
unprotected `main` with zero human review — and, per the Blocker-1 correction, *before CI even
finishes* — is itself the unsafe status quo this feature exists to gate. "Stops auto-merging until
consciously armed" is the intended safe default, not an accident. The alternative (ship the global
default ON to preserve current behavior) was **considered and rejected**: it would carry the
unsafe status quo forward silently and defeat the kill-switch's purpose as a conscious arming
gesture. This is called out in the release notes so operators relying on the old implicit
auto-merge flip the switch deliberately. Fail-safe: when the switch is unset or OFF, the
reconciler treats **every** item as non-policy regardless of its per-item flag.

**What the global switch cannot do:** it cannot un-arm a GitHub-side `--auto` merge that was
*already* armed on a previous tick. Relocating the arm into the reconciler's CI-passing branch
(gate point 1) *shrinks* this window: because arming now happens only on a tick where CI is already
green and the PR is mergeable, an armed PR typically merges within ~1 tick, so the interval during
which the switch is OFF but an armed merge is still outstanding is small (rather than the full
CI-runtime window that PR-create-time arming created). It is not zero — a PR armed on one tick that
merges before the operator flips the switch OFF is unpreventable from our side; the operator must
cancel such a merge on GitHub directly. This residual is documented, not designed away.

### (b) Fix-loop terminal behavior and the shared rework cap

The auto-fix loop reuses the existing, hardened machinery — **no parallel spawn path is
introduced** (requirements constraint; pitfalls BLOCKER §2). The churn-guard order in
`AutoReopenForPRFix` (`server/services/backlog_service_triage.go:568-623`) is load-bearing and
must not be bypassed: `tombstoneOrphanWorkSessions` → `hasActiveWorkSession` early-return
(zero status transition when a fix is already in flight) → precondition-guarded
`pr_pending → in_progress` transition (commits `af426f27` + `f8f788ab`).

**Terminal state = the existing rework cap.** `maxAutoReworkIterations = 3`
(`backlog_service_triage.go:136`) is a **single shared budget** across both the review-rework
loop (`AutoReopenAfterFailedReview`) and the PR-fix loop (`AutoReopenForPRFix`), counted over
all `SessionRoleWork` sessions for the item. On cap-hit, `AutoReopenForPRFix` leaves the item in
`pr_pending`, calls `notifyReworkCapHit` (durable `StuckReasonReworkCap` row + DB-backed
notify-once WARNING), and does **not** spawn (`:609-613`). This is the "never loop unbounded"
guarantee.

**We accept the shared budget as-is (no separate PR-fix counter).** Consequence: an item that
consumed 2 review-rework iterations before reaching a PR gets only 1 PR-fix attempt before the
cap escalates it. This is intentional — the cap bounds *total* autonomous churn per item, which
is the resource the 2026-07-12 OOM incident and the churn-bug (bucket [1] #10) were about. A
per-loop budget would double the worst-case churn per item. An item that hits the cap is not
lost: it parks in `pr_pending` with a durable `rework_cap` stuck row and an operator
notification — a clean escalation, not a dead end.

**Escalation vs "ready" notifications cannot double-fire** for the same condition: they are
keyed on different stuck reasons (`rework_cap` vs `pr_ready_unmerged`) and reached from
mutually-exclusive branches of `ReconcilePRPending` (unhealthy/cap path vs healthy path, which
resolves `pr_ready_unmerged` first). An item may legitimately surface both across its lifetime,
but never simultaneously.

### (c) Auto-merge blast radius and residual controls

With no branch protection, **CI-green is the entire safety bar** — and "CI green" must mean
*positively passing*, not "not-failing." **Pending CI must be excluded from the bar (Blocker-1).**
The coarse `PRStatus.CIFailing` bool records only terminal failures; a PR whose checks are still
running, or not yet created, has `CIFailing == false`, which the readiness predicate
(`prReadyToMergeSolo`, `CheckConclusion==""` → proceed) previously read as green. Two consequences
followed: the ready-notify could fire before CI was green, and — because `--auto` does not wait for
non-required checks — an armed merge could land while checks ran. The fix threads a tri-state CI
signal (`CIFailing` / `CIPending` / passing) from `GetPRStatus` into the `PRInfo` fed to
`prReadyToMergeSolo`, and both green-dependent gates — the ready-notify **and** the auto-merge arm
— require positive passing. The arm additionally lives in the reconciler (the only place CI is
polled), never at PR-create. We accept the CI-green-only exposure as the explicit cost of the
feature, mitigated by a defense-in-depth stack of *residual* controls (none of which is a human
review of the diff, which is structurally unavailable for a solo operator):

1. **Global kill-switch OFF by default** — the subsystem is inert until consciously armed.
2. **Per-item opt-in OFF by default** — no item automates without an explicit choice.
3. **The rework cap** — bounds autonomous churn, escalates on exhaustion.
4. **GitHub-truth anchoring (anti-fabrication)** — every merge/notify/fix decision is driven by
   an independent `IsPRMerged` / `GetPRStatus` poll (`backlog_lifecycle.go:1525,1545`), never by
   an agent's `TASK_COMPLETE` self-report or a fix session's "I fixed it" claim. A fix session's
   success is confirmed only by the *next* poll cycle's independent `GetPRStatus`. This is a hard
   requirement: an LLM in the loop can assert success that did not happen (bucket-referenced
   fabrication incident).
5. **Belt-and-suspenders conflict detection** — `HasConflicts` derives from
   `mergeStateStatus == "DIRTY"` OR `mergeable == "CONFLICTING"` (cli/cli#9583 stale-data guard),
   so a conflict appearing *after* green CI still routes the item to a rebase fix.
6. **Optional operator control: keep the headless review gate.** The per-item `SkipReviewGate`
   flag is orthogonal to `AutoMergePolicy`. An operator who wants a machine safety layer before
   auto-merge leaves `SkipReviewGate` OFF: the item then flows `work → headless review gate →
   PASS → pushAndCreatePR (auto-merge armed)`, and a FAIL routes to the rework loop instead of
   merging. `AutoMergePolicy` + `SkipReviewGate` together is the maximal-automation mode
   (CI-only gate). This composition is the deliberate two-knob design (see §Consequences).

**Explicitly NOT in this feature:** adding branch protection or required status checks, or
changing the repo-level `allow_auto_merge` setting — those are operator/settings actions. The
design *depends on* `allow_auto_merge` being on and treats the absence of branch protection as
the real blast radius rather than assuming it away.

### (d) Plain-bool vs optional-bool for the per-item flag

**Decision: plain proto3 `bool`, mirroring `AutoSpawnSession` — NOT the presence-gated
`optional bool` (PipelineMode) pattern — with the mandatory `currentFlags()` extension and a
dedicated regression test.**

The proto3 plain-`bool` trap is real: an omitted field arrives as `false`, indistinguishable
from an explicit `false`, and `UpdateBacklogItem` wraps each bool unconditionally into a non-nil
`*bool` (`backlog_service_lifecycle.go:232-237`), so any partial update that omits the flag
silently resets it to `false`. This exact bug bit `AutoSpawnSession` (commit `b28ace2f`).

We choose plain-bool anyway, for these reasons:

- **Hard requirements constraint.** The requirements (lines 74-77) mandate mirroring the
  `SkipReviewGate` / `SkipPlanning` / `AutoSpawnSession` pattern exactly. Deviating to
  `optional bool` needs stronger justification than "structurally cleaner."
- **Convention consistency.** Three sibling opt-in flags in the *same* struct and the *same*
  handler are plain-bool + `currentFlags()`. A fourth flag using a different (presence-gated)
  pattern creates a mixed idiom that raises the chance a future contributor copies the wrong
  sibling — itself a maintainability smell in a repo whose conventions are enforced against
  LLM-authored drift.
- **The failure mode is safe-direction for this flag.** A silent reset to `false` means
  *automation stops* — the item falls back to manual PR/merge. For a trust-boundary flag whose
  dangerous direction is "automate more," a silent reset toward "automate less" fails safe.
- **The mitigation is proven and cheap.** Adding the flag to the single `currentFlags()` object
  (`BacklogItemDetail.tsx:306-311`) fixes all four partial call sites at once, and a dedicated
  Jest regression test (save-notes must not reset `autoMergePolicy`) closes the residual risk at
  the test layer — a guard that would have caught the `AutoSpawnSession` bug.

**Rejected alternative — optional bool (presence-gated):** structurally immune to the trap and
already demonstrated by `PipelineMode` (`backlog_service_lifecycle.go:238-245`). Rejected because
it violates the explicit requirements constraint, introduces a mixed pattern, and its advantage
(immunity to a safe-direction failure) is neutralized by the `currentFlags()` guard plus the
regression test. The counter-risk it protects against — a silent reset to `false` producing the
exact "nothing gets merged" symptom the feature exists to kill — is genuine but is the specific
thing the regression test asserts against.

---

## Consequences

**Positive**
- Finished work reaches a merged PR with no per-item clicks, for opted-in items, once the
  subsystem is armed once.
- One atomic halt lever (global switch) for the whole autonomous subsystem.
- Reuses the hardened `pushAndCreatePR` / `AutoReopenForPRFix` / `markPRReadyUnmerged` machinery
  — single PR-creation writer, existing churn guards, existing durable notify-once.
- The pre-existing implicit auto-merge-on-review-pass becomes explicit and operator-controlled.
- Two orthogonal knobs (`AutoMergePolicy` × `SkipReviewGate`) express the full spectrum from
  "manual everything" to "CI-only autonomous merge."

**Negative**
- **Deliberate behavior change:** review-passing *non-policy* items no longer auto-merge; they
  create a PR and surface a manual-merge notification. Operators relying on the old implicit
  auto-merge must flip the global switch ON and opt items in. Documented in the release notes.
- CI-green remains the entire safety bar for policy items with `SkipReviewGate` set. No design
  control substitutes for branch protection, which is out of scope.
- The shared rework cap gives a review-heavy item fewer PR-fix attempts (accepted trade-off).
- The global switch cannot un-arm an already-armed GitHub `--auto` merge (residual, documented).

**Neutral**
- The reconciler makes a live GitHub API call per `pr_pending` item per tick regardless of
  policy; the policy read gates only the *spawn/arm*, not the *polling*, so merged/closed
  detection keeps working for all items. **The policy read must be genuinely cheap AND live
  (Concern).** It must NOT be `config.LoadConfig()` (a fresh `os.ReadFile` + full unmarshal —
  `feature_flag_service.go:139` shows `UpdateFeatureFlag` itself reloads), nor a `cfg` snapshot
  captured once at wiring (which would never observe a runtime toggle, failing the "takes effect
  without restart" AC). It is an `atomic.Bool`-backed `FeatureController.IsEnabled()` (interface
  at `session_service.go:56-60`), seeded from config at startup and toggled by `UpdateFeatureFlag`'s
  `Enable`/`Disable` — an atomic load per check: cheap and live-correct.
- Eventually-consistent with a ≤60s latency floor (the reconcile tick); event-driven paths
  (`EventExited`, PASS verdict) provide the fast path, with the tick as the safety net.
