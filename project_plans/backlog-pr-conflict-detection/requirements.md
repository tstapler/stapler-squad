# Requirements: backlog-pr-conflict-detection

**Date**: 2026-07-12
**Type**: feature addition
**Complexity**: 3 — system design (modest size, but autonomous git-history mutation raises the risk bar enough to warrant explicit observability/risk-control treatment)

## Problem Statement

Stapler Squad's backlog automation already auto-fixes two of the three ways a shipped PR can get stuck: failing CI checks and human `CHANGES_REQUESTED` reviews. `ReconcilePRPending` (`session/backlog_lifecycle.go:530-585`) polls every `pr_pending` backlog item, calls `GetPRStatus` (`session/git/worktree_git.go:338-438`, via `gh pr view --json statusCheckRollup,reviews,comments`), and if either condition is true, spawns a capped, autonomous fix session via `PRFixSpawner.AutoReopenForPRFix` (`backlog_service_triage.go:438`, capped at `maxAutoReworkIterations = 3`).

What's missing: `PRStatus` (`worktree_git.go:327-334`) has exactly two booleans, `CIFailing` and `HasBlockingReviews`. The `gh pr view` call backing it never requests `mergeable`/`mergeStateStatus` (`worktree_git.go:346`). A PR whose branch has fallen behind `main` and developed merge conflicts, but whose CI is still green (or hasn't rerun) and has no requested changes, hits `!CIFailing && !HasBlockingReviews == true` and the reconciler silently treats it as healthy — it sits in `pr_pending` forever with no autonomous action and no visible signal that anything is wrong.

This is not hypothetical: three separate PRs in this same session (#147, #148, #150's predecessor branches) required a human (this session, manually) to detect the conflict via `gh pr view --json mergeable`, rebase, resolve `.gitignore`-corruption conflicts, and force-push — work the existing autonomous pipeline should have done itself, the same way it already does for CI failures.

Separately, review-comment detection (`HasBlockingReviews`, `FeedbackText` from review bodies + general PR comments, `worktree_git.go:418-434`) is already implemented and unified with the CI-failure path — but has no dedicated regression test coverage confirming it actually works end-to-end. This project should add that coverage as part of hardening the same reconciliation loop being extended.

## Baseline

Today, a backlog PR with silent merge conflicts:
- Shows no distinguishing signal in `PRStatus`, `PRStatusPoller`, or `WorktreePRPoller` (`session/pr_status_poller.go`, `session/worktree_pr_poller.go`) — none of the three track mergeable state; the pollers exist only to feed a UI badge from `state`/`checkConclusion`/`approvedCount`/`changesReqCount`/`isDraft`.
- Never triggers `AutoReopenForPRFix`.
- Requires a human to run `gh pr view --json mergeable,mergeStateStatus` manually to even notice, then manually rebase and force-push.

## Users / Consumers

The backlog automation system itself (`BacklogLifecycleListener`, `ReconcilePRPending`) is the primary consumer of the new detection signal. Indirect beneficiary: whoever owns backlog items that get shipped as PRs (today, just the repo owner) — they currently have to notice and fix conflicts by hand.

## Success Metrics

- A backlog PR whose branch has real merge conflicts against its base is detected by `ReconcilePRPending` within one polling interval, without a human running `gh pr view` manually.
- Detection triggers the same `AutoReopenForPRFix` path already used for CI failures/review comments (per user decision — reuse, don't build a parallel mechanism), capped by the existing `maxAutoReworkIterations` safety net so a conflict that can't be autonomously resolved doesn't loop forever.
- Existing review-comment detection (`HasBlockingReviews`) gains regression test coverage proving it fires correctly, since it currently has none.
- Zero regressions to the existing CI-failure and review-comment detection paths.

## Appetite

Medium (up to a week)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints

- Must reuse `AutoReopenForPRFix` (per user decision in ideation) rather than building a dedicated rebase-only flow — lowest risk, most consistent with the existing CI-failure/review-comment pattern, capped by the existing iteration limit.
- No new external integrations — `gh` CLI is already the exclusive PR-status data source (`GetPRStatus`) and must remain so.
- Must not touch `session/pr_status_poller.go`/`session/worktree_pr_poller.go`'s existing UI-badge behavior — those are a separate, display-only concern from `BacklogLifecycleListener`'s `ReconcilePRPending`, which is what actually drives autonomous action.

## Non-functional Requirements

- **Performance SLO**: not specified — this extends an existing poll-based reconciliation loop; no new latency-sensitive path.
- **Scalability**: not applicable — same order of magnitude as existing `pr_pending` item count (typically single digits).
- **Security classification**: internal — operates on the user's own repos via their existing authenticated `gh` CLI session.
- **Data residency**: no special requirements.

## Scope

### In Scope
- Extend `GetPRStatus`'s `gh pr view` call to also fetch `mergeable`/`mergeStateStatus`.
- Add a conflict signal to `PRStatus` (alongside `CIFailing`/`HasBlockingReviews`).
- Extend `ReconcilePRPending`'s gate to also trigger `AutoReopenForPRFix` on detected conflicts, reusing the existing spawn path and `maxAutoReworkIterations` cap.
- A conflict-specific `fixCtx`/prompt addition so the spawned session understands it's rebasing, not necessarily re-implementing acceptance criteria (still the generic fix path, but the prompt content should say what's actually wrong).
- Regression tests: new coverage for conflict detection, plus first-ever coverage for the existing review-comment detection path.

### Out of Scope
- A dedicated, narrower "rebase-only" flow distinct from the general fix-session path (explicitly rejected in ideation).
- Automatic force-push / merge without going through the same worktree+session flow every other autonomous fix uses.
- Changes to `PRStatusPoller`/`WorktreePRPoller`'s UI-facing badge computation.
- Fetching actual CI failure log content for the pre-existing `CIFailing` path (a separate, already-identified gap — not this project).
- Any change to the review-comment *detection* mechanism itself (already correct) beyond adding tests for it. **Exception**: a non-behavioral internal reshape of `PRStatus`'s `FeedbackText` assembly (accumulating into an unexported `render()` method over captured fields instead of an interleaved `strings.Builder`) is in scope — it's required once a third boolean (`HasConflicts`) joins `CIFailing`/`HasBlockingReviews` on the same struct, to avoid the type allowing `FeedbackText` to drift out of sync with the booleans (see Phase 3 architecture review). Detection logic and behavior for CI/review signals must remain byte-for-byte identical; only the internal assembly mechanism changes.

## Rabbit Holes

- **Conflict resolution confidence**: what happens when the spawned fix session attempts a rebase and hits a conflict it can't confidently resolve (e.g. the exact `.gitignore`-corruption pattern manually fixed 3 times this session)? The existing `maxAutoReworkIterations` cap bounds *how many times* it retries, but doesn't itself detect "this session is guessing." Flag for Phase 3 planning: should the fix-session prompt include explicit guidance to leave conflict markers / stop and flag rather than guess when a hunk isn't a trivial rebase-forward?
- **Mergeable-state polling lag**: GitHub computes `mergeable`/`mergeStateStatus` asynchronously after certain events; `gh pr view` can return `UNKNOWN` transiently. Needs a defined behavior (skip this cycle vs. treat as conflict) so a transient `UNKNOWN` doesn't spuriously trigger a fix session.
- **Cap interaction**: does a conflict-triggered spawn share the same `maxAutoReworkIterations` counter as CI-failure/review-triggered spawns for the same item, or does it need its own counter? Sharing is simpler and matches "reuse the same generic fix path," but could mean a PR that first hit CI failures has fewer remaining attempts left for a later conflict. Flag for Phase 3.

## Alternatives Considered

- **Dedicated rebase-only flow** (separate from `AutoReopenForPRFix`): considered and explicitly rejected during ideation — more code, inconsistent with the existing pattern, for a Medium-appetite project that should stay close to what already works.
- **Webhook-based conflict detection**: rejected — the existing pipeline is poll-based throughout (`ReconcilePRPending`, `PRStatusPoller`, `WorktreePRPoller` all poll); introducing webhooks for just this one signal would be a new integration class out of proportion to the problem.

## Feasibility Risks

- `gh pr view --json mergeable,mergeStateStatus` behavior around `UNKNOWN`/pending computation needs to be handled defensively (see Rabbit Holes) or the feature could produce false-positive conflict spawns.
- Autonomous conflict resolution is inherently higher-stakes than autonomous CI-failure fixing: a wrong rebase resolution can silently drop or corrupt code rather than just fail loudly like a broken build. The existing `AutoReopenForPRFix` path was designed around CI/review feedback, not around "resolve these specific conflicting hunks" — reusing it means trusting the same general-purpose agent judgment for a qualitatively different task.

## Observability Requirements

Log when a conflict is detected and a fix session is spawned (matching the existing `log.InfoLog`/`log.WarningLog` pattern already used in `ReconcilePRPending`/`AutoReopenForPRFix`), including the item ID and which signal (conflict vs. CI vs. review) triggered the spawn, so operators can distinguish conflict-driven autonomous fixes from the pre-existing two triggers in logs. No new metrics/alerting infrastructure — standard request/event logging is sufficient at this scale.

## Risk Control

- Bounded by the existing `maxAutoReworkIterations = 3` cap (pending Phase 3 decision on whether it's shared or separate per trigger type — see Rabbit Holes) — a conflict that can't be resolved in 3 autonomous attempts leaves the item for manual action rather than looping indefinitely, matching current behavior for the other two triggers.
- No feature flag planned — this is an extension of an already-live, already-autonomous pipeline (`ReconcilePRPending` already runs unconditionally against `pr_pending` items); gating just the conflict-detection addition behind a flag would be inconsistent with how its sibling triggers (CI, reviews) already ship.
- Rollback: revert the `PRStatus`/`GetPRStatus`/`ReconcilePRPending` changes; no schema/data migration involved, so rollback is a plain code revert.

## Open Questions

- Should the conflict-triggered spawn share `maxAutoReworkIterations` with CI/review-triggered spawns for the same item, or track separately? (Rabbit Hole — resolve in Phase 3.)
- How should transient `mergeable: UNKNOWN` from GitHub's async computation be handled — skip the cycle, or treat conservatively as non-conflicting? (Rabbit Hole — resolve in Phase 3.)
- Should the fix-session prompt include explicit "if you can't confidently resolve a conflict, stop and leave markers rather than guess" guidance, and if so, how does the reconciler detect that outcome vs. a successful rebase? (Rabbit Hole — resolve in Phase 3.)
