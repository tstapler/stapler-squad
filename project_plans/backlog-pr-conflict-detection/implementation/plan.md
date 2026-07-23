# Implementation Plan: backlog-pr-conflict-detection

**Feature**: Detect GitHub merge conflicts on `pr_pending` backlog PRs and auto-spawn a fix session via the existing `AutoReopenForPRFix` pipeline, reusing the same trigger mechanism already used for CI failures and blocking reviews.
**Date**: 2026-07-12
**Status**: Ready for implementation
**ADRs**: None (no genuinely non-standard choice — every decision below follows an existing convention already present in this codebase; see Pattern Decisions)

---

## CREATIVE Pass (Step 0.5) — Alternatives Considered

1. **Extend the existing `PRStatus`/`GetPRStatus`/`ReconcilePRPending` pipeline in place** (chosen). *Strength*: minimal footprint — 2 production files, reuses a pipeline already proven for two other trigger types, matches the explicit "reuse `AutoReopenForPRFix`" constraint from ideation. *Weakness*: `FeedbackText` stays an unstructured string, so the log line is still the only place that records which signal(s) fired — an existing, accepted limitation, not a new one.
2. **Structured `TriggerReason` enum threaded through `ItemSessionData`** (rejected). *Strength*: would enable per-trigger-type iteration caps and richer observability later. *Weakness*: requires an ent schema migration and touches `CreateItemSession`/`SpawnSessionFromItem` call sites — crosses into new production surface far beyond this project's Medium appetite, and requirements.md's own research (architecture.md §3) already shows the shared cap is *correct*, not a gap to close.
3. **Dedicated rebase-only flow with its own spawner** (rejected). *Strength*: could add a hard pre-push gate (e.g. diff-preservation check) that isn't achievable inside the generic `AutoReopenForPRFix` path. *Weakness*: explicitly rejected during ideation (requirements.md Out of Scope) — duplicate code path, inconsistent with the CI/review precedent.

Approach 1 is used throughout this plan. Approaches 2 and 3 are recorded in the Pattern Decisions table below.

---

## Domain Glossary

*(Ubiquitous language — every domain term that appears as a type, method, or variable name. Exact names here must be used consistently in code, tests, and comments.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `PRStatus` | Struct holding the combined CI/review/conflict signal for one PR (`session/git/worktree_git.go:326`) | Exported surface extended with `HasConflicts bool`, not restructured; gains unexported fields (`conflict`, `failedChecks`, `blockingReview`, `generalComments`) that back `render()` (architecture-review.md Concern A) |
| `HasConflicts` | New `PRStatus` field; `true` when GitHub reports `mergeStateStatus == "DIRTY"` or `mergeable == "CONFLICTING"` (belt-and-suspenders OR — see "Conflict condition" below) | Sibling to existing `CIFailing`/`HasBlockingReviews` — independent, not mutually exclusive |
| `CIFailing` | Existing `PRStatus` field; `true` when at least one CI check has a terminal failure | Unchanged by this project; gains first-ever regression tests |
| `HasBlockingReviews` | Existing `PRStatus` field; `true` when a reviewer left a `CHANGES_REQUESTED` review | Unchanged by this project; gains first-ever regression tests |
| `FeedbackText` | Existing `PRStatus` field; a single pre-rendered markdown string, computed by `render()` from structured fields captured during evaluation rather than built via interleaved `strings.Builder` writes | New `## Merge conflict` section (from the captured `conflict` field) is rendered first |
| `GetPRStatus` | Method on `*GitWorktree` that shells out to `gh pr view` and returns `*PRStatus` (`worktree_git.go:338`) | Becomes a thin I/O wrapper; JSON parsing/evaluation moves into `parsePRStatusPayload` |
| `parsePRStatusPayload` | **New** pure function: `(raw []byte) (*PRStatus, error)` — JSON-unmarshals `gh pr view`'s combined output, evaluates all four signals (CI, reviews, conflict, comments) into small structured fields, then sets `FeedbackText = status.render()` | Extracted specifically so it is table-testable without a live authenticated `gh` CLI |
| `render` | **New** unexported `(*PRStatus) string` method; assembles `FeedbackText` from the `conflict`, `failedChecks`, `blockingReview` fields in a fixed order (conflict first) | Replaces interleaved `sb.WriteString` calls during evaluation — keeps bool-setting and text-rendering from drifting out of sync (architecture-review.md Concern A) |
| `conflict` / `failedChecks` / `blockingReview` | **New** unexported `PRStatus` fields captured during evaluation: `conflict *conflictInfo` (nil unless `HasConflicts`), `failedChecks []string`, `blockingReview *reviewInfo` (nil unless a `CHANGES_REQUESTED` review exists) | Inputs to `render()`; not exported, not part of the public `PRStatus` API surface |
| `mergeable` | GitHub GraphQL `MergeableState` field on the PR: `MERGEABLE` \| `CONFLICTING` \| `UNKNOWN` | Fetched via `gh pr view --json mergeable`; one of two OR'd conflict signals (see `mergeStateStatus`) |
| `mergeStateStatus` | GitHub GraphQL `MergeStateStatus` field: `CLEAN` \| `DIRTY` \| `BLOCKED` \| `BEHIND` \| `DRAFT` \| `HAS_HOOKS` \| `UNSTABLE` \| `UNKNOWN` | Fetched alongside `mergeable`; `DIRTY` is OR'd with `mergeable == CONFLICTING` as a belt-and-suspenders check — cli/cli#9583 documents `mergeable` returning stale/incorrect data vs. `mergeStateStatus` for the same PR (stack.md §3) |
| Conflict condition (`mss`/`mg`) | The evaluation `mss == "DIRTY" \|\| mg == "CONFLICTING"`, where `mss = strings.ToUpper(payload.MergeStateStatus)` and `mg = strings.ToUpper(payload.Mergeable)` | Both fields must independently be non-matching (e.g. both `UNKNOWN`) for `HasConflicts` to stay `false` — an OR of two fields, not a single `mergeable == CONFLICTING` check |
| `ReconcilePRPending` | Poll-loop method that transitions `pr_pending` items and spawns fix sessions (`session/backlog_lifecycle.go:530-585`) | Gate extended to a 3-way OR; log line extended with `conflict=%v` |
| `fixCtx` | Local variable in `ReconcilePRPending` — the rendered string passed to `AutoReopenForPRFix` as `fixContext` | Unchanged code; automatically carries the new `## Merge conflict` section because it's already `prStatus.FeedbackText` |
| `AutoReopenForPRFix` | Method on `*BacklogService` (implements `PRFixSpawner`) that reopens a `pr_pending` item and spawns a work session (`server/services/backlog_service_triage.go:438`) | Zero changes — signature and cap logic are already signal-agnostic |
| `PRFixSpawner` | Consumer-defined interface in `session/backlog_lifecycle.go:36` with one method, `AutoReopenForPRFix` | Unchanged |
| `maxAutoReworkIterations` | Constant (`= 3`) capping work-session spawns per backlog item, shared across all trigger types by construction (`backlog_service_triage.go:37`) | Unchanged — already shared; see Pattern Decisions |
| `workCount` | Local variable inside `AutoReopenForPRFix` counting `ItemSession`s with `Role == SessionRoleWork` for the item | Unchanged; counts conflict-triggered spawns identically to CI/review-triggered ones |
| `BacklogLifecycleListener` | The struct hosting `ReconcilePRPending` and the `prFixSpawner` field | Gains one new unexported interface + one new package-level factory var (see below) |
| `prPendingChecker` | **New** consumer-side interface defined in `backlog_lifecycle.go`, scoped to the two methods `ReconcilePRPending` actually calls: `IsPRMerged(int) (bool, error)` and `GetPRStatus(int) (*git.PRStatus, error)` | Testability seam — `*git.GitWorktree` already satisfies it with zero changes to package `git` |
| `newPRPendingChecker` | **New** package-level `var` in `backlog_lifecycle.go` holding a factory function, defaulting to `git.NewGitWorktreeFromStorage`; overridable in tests | Mirrors the existing `timeNow` seam pattern already used in `session/instance_workspace.go:581` |
| `pr_pending` | Backlog item status (`BacklogStatusPRPending`) meaning "PR is open, waiting on merge or fix" | Unchanged; the status this whole feature operates on |
| `fakePRPendingChecker` | **New** test double implementing `prPendingChecker`, used in `backlog_lifecycle_test.go` to inject canned `IsPRMerged`/`GetPRStatus` results | Test-only type |
| `fakePRFixSpawner` | **New** test double implementing `PRFixSpawner`, records whether/how `AutoReopenForPRFix` was called | Test-only type; same shape as the existing `mockReviewGateSpawner` pattern already in `backlog_lifecycle_test.go:424` |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `PRStatus` signal shape | Independent bool-per-signal (extend existing shape) | Existing convention (`CIFailing`/`HasBlockingReviews`) | Sum type / `TriggerReason` enum (CREATIVE alt. 2) | Signals are not mutually exclusive — a PR can be `CIFailing` *and* `HasConflicts` simultaneously (features.md §2A); a sum type would force picking one winner, breaking the existing "append, don't choose" `FeedbackText` precedent |
| Mergeability interpretation | Extracted pure function `parsePRStatusPayload(raw []byte) (*PRStatus, error)` | type-driven-design (parse-at-the-boundary) + build-vs-buy.md §3 recommendation | Inline evaluation inside `GetPRStatus` | `GetPRStatus` requires a live, authenticated `gh` CLI (`checkGHCLI()`) and is therefore untestable in CI without network access; extracting the pure JSON-in/struct-out logic makes it directly table-testable, satisfying this project's explicit test-coverage mandate for both the new conflict signal and the existing untested CI/review signals |
| `FeedbackText` assembly | Structured capture (`conflict`/`failedChecks`/`blockingReview` fields set during evaluation) + a single `render()` method called once at the end of `parsePRStatusPayload` | architecture-review.md Concern A | Keep the existing interleaved `sb.WriteString` calls, one per signal, as each bool is set | Independently-settable bools plus interleaved string-building means `FeedbackText` and the bools it should reflect are set in two different places for the same signal — easy for a future signal addition to update one and forget the other; `render()` derives text from the same fields the bools are set from, so they can't drift apart |
| `ReconcilePRPending`'s git dependency | Consumer-defined interface `prPendingChecker` (2 methods) + package-var factory `newPRPendingChecker` | interface-pollution-checklist.md (interface lives in consumer package, scoped to exactly what's called) + existing `timeNow` var-override precedent (`instance_workspace.go:581`) | Leave `git.NewGitWorktreeFromStorage` called directly; test only via real git/gh fixtures | Direct construction is *why* `ReconcilePRPending` has zero test coverage today (requirements.md) — the interface is minimal (not speculative: exactly the 2 methods called, defined where consumed) and `*git.GitWorktree` satisfies it with no change to package `git` |
| Third-signal gate | Extend the existing flat OR at line 568 to a 3-way OR | Existing convention — the gate is already a pure OR with no signal-specific branching | Guard clause per signal, or a Strategy/Chain-of-Responsibility per trigger type | 3 booleans with identical downstream handling (same `fixCtx`, same spawn call) — a behavioral pattern would be over-engineering; "no new pattern needed" is the honest answer here |
| Conflict-fix prompt guidance | Guidance text written directly inside `render()`'s conflict branch, keyed off the captured `status.conflict` field | Reshaped per architecture-review.md Concern A — same output text, computed inside the new `render()` method instead of an inline `sb.WriteString` interleaved with bool-setting | New structured `ConflictGuidance` field threaded separately through `PRFixSpawner`/`AutoReopenForPRFix`/`ItemSessionData` | `fixContext` is still a single generic string pipe all the way to the spawned session's prompt (architecture.md §4); a new pipeline field would require touching 3 files this project explicitly does not need to touch — `render()` keeps the reshape local to `PRStatus` |
| Iteration cap sharing | No new code — `workCount` already counts all `SessionRoleWork` sessions regardless of trigger (architecture.md §3) | Existing convention | Separate per-trigger-type counter (CREATIVE alt. 2) | Resolves requirements.md's "Cap interaction" Open Question: sharing is not a decision to make, it's the existing, already-correct behavior |

---

## Observability Plan

- **Logs**: one new `%v` on the existing `log.InfoLog.Printf` call in `ReconcilePRPending` (line 579-580) — `conflict=%v` alongside the existing `CI=%v, reviews=%v`. No new log statements, no new logger calls.
- **Metrics**: none. Matches requirements.md's Observability Requirements — "No new metrics/alerting infrastructure — standard request/event logging is sufficient at this scale."
- **Alerts**: none required.

## Risk Control

- **Feature flag**: not gated — consistent with the sibling CI/review triggers, which already run unconditionally against all `pr_pending` items. Gating only the conflict signal would be inconsistent with how this pipeline already ships.
- **Rollback procedure**: plain code revert of the `PRStatus`/`GetPRStatus`/`ReconcilePRPending` changes. No schema or data migration is involved.
- **Bounding mechanism**: the existing `maxAutoReworkIterations = 3` cap, already shared across all trigger types by construction (see Pattern Decisions) — a conflict that can't be resolved in 3 autonomous attempts leaves the item in `pr_pending` for manual action, matching current behavior for CI/review triggers exactly.
- **Human review requirement (pre-mortem Failure #1, P1)**: conflict-fix PRs must re-enter full human review and must never be treated as auto-mergeable or silently proceed straight to merge on green CI alone — a force-pushed rebase resets GitHub's incremental review state, so "CI green" is not equivalent to "reviewed" the way it is for an appended-commit CI/review fix. Verified: `AutoReopenForPRFix` already routes every trigger type (CI, review, and conflict) through the identical `in_progress` → `review` → review-gate → `pr_pending` cycle with no merge shortcut (see Epic 2.1's Goal note) — this plan adds no new merge path and must not add one. See the Unresolved Questions follow-up on `EnablePRAutoMerge` for the one pre-existing, out-of-scope residual risk this doesn't fully close.
- **Staged rollout**: full rollout on merge (no cohort/percentage rollout mechanism exists in this pipeline, nor is one needed at this item volume — requirements.md: "typically single digits").

## Unresolved Questions

All three Open Questions from requirements.md were resolved during research (see architecture.md/stack.md/pitfalls.md) and are **not** blocking this plan:

- ~~Cap sharing~~ — **Resolved**: already shared by construction (`workCount` counts all `SessionRoleWork` sessions regardless of trigger). No code change needed.
- ~~`UNKNOWN` handling~~ — **Resolved**: never treat as conflicting. Falls out for free from the `mss == "DIRTY" || mg == "CONFLICTING"` OR condition (Task 1.1.1d) — `UNKNOWN` matches neither comparison, on either field, so no special-case code is required.
- ~~Prompt guidance for low-confidence conflicts~~ — **Resolved**: yes, include "leave markers and stop" guidance in the conflict `FeedbackText` section (Epic 1.2). Mechanically *detecting* that outcome (vs. a clean resolution) remains unsolved and is intentionally out of scope — nothing in `ReconcilePRPending`/`AutoReopenForPRFix` inspects spawned-session output content today, for any trigger type.

One item remains genuinely open, flagged as a separate follow-up rather than a task in this plan:

- [ ] **go-git concurrent-worktree-read audit** (pitfalls.md §5) — sweep `session/git/*.go` for other unlocked go-git read call sites beyond the already-fixed `getHeadCommitSHA` incident (PR #151, commits `dce6a644`/`4cbb5294`). **Does not block any story in this plan**: `GetPRStatus`'s new code (the extended `gh pr view` call) is a pure `gh` CLI/network call with no local git state read, per pitfalls.md's own finding — this project's new code is unaffected by that risk class. Recommend as a separate, out-of-appetite follow-up project — owner: whoever picks up the next `session/git` hardening pass.
- [ ] **`EnablePRAutoMerge` doesn't distinguish a rebase-reset diff from an incremental one** (pre-mortem Failure #1, P1 — see Epic 2.1's Goal note): the pipeline's one existing auto-merge mechanism (`session/git/worktree_git.go:443`, called unconditionally whenever any PR reaches `pr_pending`) is "best-effort" and relies entirely on GitHub branch-protection settings to gate the actual merge. It was designed when every fix cycle produced an incrementally-reviewable diff (new commits appended). A conflict-triggered fix breaks that assumption — it force-pushes a full history rewrite, resetting the diff view — but `EnablePRAutoMerge` has no way to know which kind of push just happened. **Does not block this plan**: fixing it cheaply would require distinguishing trigger types at the point `pushAndCreatePR` decides whether to call `EnablePRAutoMerge`, which needs the `TriggerReason` plumbing already rejected in Pattern Decisions (CREATIVE alt. 2) as out of this Medium-appetite project's scope. Recommend as a separate fast-follow — owner: whoever next touches `pushAndCreatePR`'s auto-merge call, or whoever eventually justifies adding `TriggerReason` plumbing for other reasons.

---

## Dependency Visualization

```
Phase 1 (Conflict Detection Core)
  Epic 1.1 (PRStatus + payload)
    1.1.1a → 1.1.1b → 1.1.1c → 1.1.1d
                                  ↓
  Epic 1.2 (Prompt guidance)   [depends on 1.1.1d]
    1.2.1a
                                  ↓
  Epic 1.3 (Tests)             [depends on 1.1.1d, 1.2.1a]
    1.3.1a  (conflict table test, 7 cases)
    1.3.1b  (conflict guidance-text substrings test — depends on 1.2.1a)
    1.3.2a, 1.3.2b  (CI/review regression tests — independent of 1.3.1a)
    1.3.3a  (FeedbackText ordering test — depends on 1.2.1a)

Phase 2 (Reconciliation Gate Extension)   [depends on Task 1.1.1a only —
                                            needs PRStatus.HasConflicts to exist]
  Epic 2.1 (Seam + gate + log line)
    2.1.1a → 2.1.1b → 2.1.2a → 2.1.2b
                                  ↓
  Epic 2.2 (Tests)             [depends on Epic 2.1 complete]
    2.2.1a → 2.2.2a → 2.2.2b  (log-line conflict=true assertion — depends on 2.2.2a)
           → 2.2.3a  (also asserts log conflict=false)
           → 2.2.3b  (also asserts log conflict=false)
           → 2.2.4a
```

---

## Phase 1: Conflict Detection Core

**Goal**: `GetPRStatus` fetches and correctly interprets GitHub's mergeable state, sets `PRStatus.HasConflicts`, renders conflict-specific fix guidance, and every signal (new and pre-existing) is table-tested without requiring a live `gh` CLI.

### Epic 1.1: `PRStatus` struct and payload extension

**Goal**: The mergeable/mergeStateStatus data reaches `PRStatus.HasConflicts` through the same `gh pr view` call already used for CI/review data, with zero new process spawns.

#### Story 1.1.1: Add `HasConflicts` end-to-end and extract `parsePRStatusPayload`

**As a** backlog automation operator, **I want** `PRStatus` to carry a conflict signal derived from GitHub's own mergeable-state computation, **so that** `ReconcilePRPending` can later act on it the same way it acts on CI failures and blocking reviews.

**Acceptance Criteria**:
- `PRStatus` (`session/git/worktree_git.go:326-334`) has a new `HasConflicts bool` field, positioned between `HasBlockingReviews` and `FeedbackText`.
  - *Given* the `PRStatus` struct definition, *When* a developer reads it, *Then* they see `CIFailing`, `HasBlockingReviews`, `HasConflicts`, `FeedbackText` in that order, each with a doc comment matching the existing style.
- `GetPRStatus`'s `gh pr view` call requests `mergeable` and `mergeStateStatus` in addition to the existing three fields.
  - *Given* `GetPRStatus(152)` is called, *When* the `gh` command is constructed, *Then* its `--json` flag value is exactly `"statusCheckRollup,reviews,comments,mergeable,mergeStateStatus"`.
- JSON parsing and signal evaluation are extracted into `parsePRStatusPayload(raw []byte) (*PRStatus, error)`, a pure function with no I/O; `GetPRStatus` becomes `checkGHCLI` → build/run `gh` command → `return parsePRStatusPayload(raw)`.
  - *Given* raw bytes `{"statusCheckRollup":[],"reviews":[],"comments":[],"mergeable":"MERGEABLE","mergeStateStatus":"CLEAN"}`, *When* `parsePRStatusPayload` is called directly (no `gh` process, no network), *Then* it returns `&PRStatus{CIFailing: false, HasBlockingReviews: false, HasConflicts: false, FeedbackText: ""}` with a `nil` error.
  - `FeedbackText` is computed by an unexported `render()` method from structured fields (`conflict`, `failedChecks`, `blockingReview`) captured during evaluation — not built via `sb.WriteString` calls interleaved with bool-setting (architecture-review.md Concern A). This is an internal reshape only; the JSON-in/struct-out contract above is unaffected.

**Files**:
- `session/git/worktree_git.go`

##### Task 1.1.1a: Add `HasConflicts bool` field to `PRStatus` (~2 min)
- In `worktree_git.go:326-334`, insert `HasConflicts bool` between `HasBlockingReviews` and `FeedbackText`, with doc comment: `// HasConflicts is true when GitHub reports mergeStateStatus == "DIRTY" or mergeable == "CONFLICTING" — its branch cannot be merged as-is and needs a rebase. Both fields are checked (see Task 1.1.1d) because gh's mergeable field has been observed returning stale data (cli/cli#9583).`
- Files: `session/git/worktree_git.go`

##### Task 1.1.1b: Extend `--json` flag and payload struct (~3 min)
- At `worktree_git.go:346`, change `"statusCheckRollup,reviews,comments"` to `"statusCheckRollup,reviews,comments,mergeable,mergeStateStatus"`.
- In the anonymous `payload` struct (currently `worktree_git.go:353-377`), add two fields after `Comments`: `Mergeable string \`json:"mergeable"\`` and `MergeStateStatus string \`json:"mergeStateStatus"\``.
- Files: `session/git/worktree_git.go`

##### Task 1.1.1c: Extract `parsePRStatusPayload` from `GetPRStatus`, reshaped around a `render()` method (~8 min)
- Move the existing body of `GetPRStatus` from `var payload struct {...}` (currently line 353) through `return status, nil` (currently line 437) into a new top-level function `func parsePRStatusPayload(raw []byte) (*PRStatus, error)`, taking `raw` as a parameter instead of closing over it.
- Reshape `FeedbackText` construction per architecture-review.md Concern A: instead of interleaving `sb.WriteString` calls with bool-setting, capture each signal onto a small unexported field and derive `FeedbackText` from those fields in one place:
  ```go
  type conflictInfo struct{ mergeStateStatus string }
  type reviewInfo struct{ author, body string }

  type PRStatus struct {
      CIFailing          bool
      HasBlockingReviews bool
      HasConflicts       bool
      FeedbackText       string

      failedChecks    []string      // unexported; captured CI failures, consumed by render()
      blockingReview  *reviewInfo   // unexported; nil unless a CHANGES_REQUESTED review exists
      conflict        *conflictInfo // unexported; nil unless HasConflicts
      generalComments []string      // unexported; existing "general comments" section content (unchanged behavior, just relocated)
  }

  // render assembles FeedbackText from the fields captured during evaluation,
  // in a fixed order (conflict first — features.md §2A), so FeedbackText can
  // never drift from the bools it's derived from.
  func (s *PRStatus) render() string {
      var sb strings.Builder
      if s.conflict != nil {
          sb.WriteString("## Merge conflict\n")
          // guidance text added in Epic 1.2 (Task 1.2.1a)
      }
      // existing "## Failing CI checks" rendering, unchanged, now reading
      // from s.failedChecks instead of a local slice.
      // existing "## Review: changes requested by @..." rendering, unchanged,
      // now reading from s.blockingReview instead of local variables.
      // existing general-comments rendering, unchanged, now reading from
      // s.generalComments instead of a local slice.
      return sb.String()
  }
  ```
  The existing CI-checks and review-rendering bodies move as-is into `render()` — this task changes *where* the string is assembled, not the per-signal rendering logic itself.
- `parsePRStatusPayload` ends with `status.FeedbackText = status.render()` instead of returning mid-build.
- Rewrite `GetPRStatus` to: call `checkGHCLI()`, build/run the `gh` command as today, then `return parsePRStatusPayload(raw)`.
- Add a doc comment on `parsePRStatusPayload`: `// parsePRStatusPayload parses gh pr view's combined JSON output, evaluates all PR-status signals into structured fields, and renders FeedbackText from them. It has no I/O dependency and is directly unit-testable.`
- Behavior must be byte-for-byte identical to today's `FeedbackText` output for the three existing signals — this task changes *how* the string is assembled, not what it contains.
- Files: `session/git/worktree_git.go`

##### Task 1.1.1d: Add conflict evaluation block — OR condition against both fields (~5 min)
- In `parsePRStatusPayload`, immediately after `status := &PRStatus{}` and *before* the CI-checks evaluation loop, add:
  ```go
  // Evaluate mergeability first — a PR that can't even be rebased makes
  // CI/review feedback moot until it's mergeable again. Check both fields:
  // cli/cli#9583 documents gh's `mergeable` field returning stale/incorrect
  // data vs. `mergeStateStatus` for the same PR (stack.md §3), so this is a
  // belt-and-suspenders OR, not a single-field check. UNKNOWN on both fields
  // falls through to "no signal this cycle" by construction — neither
  // comparison below matches "UNKNOWN".
  mss := strings.ToUpper(payload.MergeStateStatus)
  mg := strings.ToUpper(payload.Mergeable)
  if mss == "DIRTY" || mg == "CONFLICTING" {
      status.HasConflicts = true
      status.conflict = &conflictInfo{mergeStateStatus: payload.MergeStateStatus}
  }
  ```
- This sets `status.conflict`, which `render()` (Task 1.1.1c) turns into the `## Merge conflict` header; `render()`'s fixed ordering (conflict first) is what guarantees `## Merge conflict` always precedes `## Failing CI checks` when both apply (features.md §2A) — no ordering-of-writes convention to maintain during evaluation.
- Files: `session/git/worktree_git.go`

---

### Epic 1.2: Conflict-specific prompt guidance

**Goal**: The `## Merge conflict` section gives the spawned fix session concrete, high-value safety guidance (force-with-lease, `.gitignore`-suspicion, leave-markers-and-stop, mandatory diff-stat report) rather than a bare "rebase and resolve conflicts" instruction.

**Why a diff-stat requirement matters (pre-mortem Failure #1, P1)**: prose guidance alone is unverified — nothing checks that the spawned session actually followed `--force-with-lease`, the `.gitignore`-suspicion rule, or the leave-markers-and-stop rule. Worse, a successful force-pushed rebase resets the PR's GitHub diff view and invalidates prior review-comment anchors, so a human reviewer is structurally more likely to skim "CI green, looks rebased" than re-diff the full changeset from scratch — the exact `.gitignore`-corruption pattern that motivated this project, now automated instead of manual. Requiring the fix session to report a concrete `git diff --stat` gives the guidance a verifiable artifact instead of only an instruction. This does not *prove* the session followed the other rules — nothing in `ReconcilePRPending`/`AutoReopenForPRFix` inspects spawned-session output content, for any trigger type (see Unresolved Questions) — but it gives the human reviewer, who is now required to look (see Epic 2.1's human-review note below), something concrete to check against instead of trusting the session's own account.

#### Story 1.2.1: Add force-with-lease, config-file-suspicion, stop-if-unsure, and mandatory diff-stat guidance

**As a** backlog automation operator, **I want** the conflict-fix prompt to carry the specific mitigations identified in pitfalls research plus a mandatory verifiable diff summary, **so that** an autonomous rebase doesn't repeat the `.gitignore`-corruption incident, doesn't force-push over concurrent work, and leaves a human reviewer something concrete to check instead of "CI green, looks rebased."

**Acceptance Criteria**:
- The `## Merge conflict` section's body text (a) instructs `--force-with-lease` instead of `--force`, (b) instructs preferring the more-complete side of a conflict on suspiciously short/placeholder-like config files, naming `.gitignore` specifically, (c) instructs leaving conflict markers and stopping rather than guessing on a non-trivial conflict, and (d) instructs the session to run `git diff --stat` against the pre-rebase base branch and paste that output verbatim into its final report and into the PR description before finishing, specifically calling out `.gitignore` and other suspicious config/lockfiles if their line-count delta looks disproportionate.
  - *Given* `payload.Mergeable == "CONFLICTING"` and `payload.MergeStateStatus == "DIRTY"`, *When* `parsePRStatusPayload` renders `FeedbackText`, *Then* the string contains all four substrings: `"--force-with-lease"`, `".gitignore"`, `"leave the conflict markers"` (or equivalent stop-and-flag phrasing), and `"git diff --stat"`.
  - Guidance renders identically regardless of which field tripped the OR condition — *Given* `payload.Mergeable == "MERGEABLE"` and `payload.MergeStateStatus == "DIRTY"` (the cli/cli#9583 stale-`mergeable` case), *When* `parsePRStatusPayload` renders `FeedbackText`, *Then* the same four substrings are present, because guidance is keyed off `status.conflict != nil`, not off which specific field matched.

**Files**:
- `session/git/worktree_git.go`

##### Task 1.2.1a: Write the guidance text into `render()`'s conflict branch (~4 min)
- In `render()` (Task 1.1.1c), replace the `// guidance text added in Epic 1.2 (Task 1.2.1a)` placeholder with:
  ```go
  sb.WriteString(fmt.Sprintf(
      "This PR's branch has merge conflicts against its base branch (mergeStateStatus=%s) "+
          "and cannot be merged as-is.\n"+
          "Rebase onto the base branch and resolve conflicts. This is not necessarily a "+
          "re-implementation of the original acceptance criteria — the PR's existing changes "+
          "are still correct, they just need to be replayed onto a moved base.\n\n"+
          "Follow these rules when resolving:\n"+
          "- Push with `git push --force-with-lease`, never `--force`. This fails safely if "+
          "the remote branch moved since you last fetched it, instead of silently discarding commits.\n"+
          "- If a conflicting file is a config file (for example `.gitignore`) and one side of "+
          "the conflict looks suspiciously short or placeholder-like compared to the other, prefer "+
          "the longer/more-complete side rather than guessing — this repo has hit real `.gitignore` "+
          "truncation incidents from automated rebases before.\n"+
          "- If you cannot confidently resolve a conflicting hunk, leave the conflict markers in "+
          "place, do not force-push, and say so clearly in your final message instead of guessing.\n"+
          "- Before finishing, run `git diff --stat` comparing your final branch against the base "+
          "branch's pre-rebase state, and paste that output verbatim into both your final report "+
          "and the PR description. Call out `.gitignore` or any other config/lockfile whose line-count "+
          "delta looks disproportionate. This rebase will force-push over the PR's existing diff, which "+
          "resets GitHub's review view — the diff-stat is the one artifact a human reviewer can check "+
          "against your summary instead of trusting it on faith.\n\n",
      s.conflict.mergeStateStatus))
  ```
- Note: this reads `s.conflict.mergeStateStatus` (captured by Task 1.1.1d), not `payload.MergeStateStatus` — `render()` has no access to the raw `payload` struct, only to the fields captured on `PRStatus` during evaluation.
- Files: `session/git/worktree_git.go`

---

### Epic 1.3: Table-driven and regression test coverage

**Goal**: Every signal `parsePRStatusPayload` evaluates — new (conflict) and pre-existing (CI, reviews) — has direct unit coverage with zero dependency on a live `gh` CLI.

#### Story 1.3.1: Table-driven test for conflict detection

**As a** backend engineer, **I want** a table-driven test over the documented `mergeable`/`mergeStateStatus` enum combinations, **so that** the conflict signal is provably correct for both the trigger case and the near-miss cases that must NOT trigger.

**Acceptance Criteria**:
- A single table-driven test in `worktree_git_test.go` covers exactly these 7 cases:
  1. `mergeable=MERGEABLE, mergeStateStatus=CLEAN` → `HasConflicts=false` (healthy — matches live-verified PR #151 data from stack.md)
  2. `mergeable=CONFLICTING, mergeStateStatus=DIRTY` → `HasConflicts=true`
  3. `mergeable=CONFLICTING, mergeStateStatus=BLOCKED` → `HasConflicts=true` (mergeable is authoritative even when mergeStateStatus reports an unrelated block)
  4. `mergeable=UNKNOWN, mergeStateStatus=UNKNOWN` → `HasConflicts=false` (transient — no signal)
  5. `mergeable=MERGEABLE, mergeStateStatus=BLOCKED` → `HasConflicts=false` (required check/review gate — NOT a conflict)
  6. `mergeable=MERGEABLE, mergeStateStatus=BEHIND` → `HasConflicts=false` (behind base — NOT a conflict)
  7. `mergeable=MERGEABLE, mergeStateStatus=DIRTY` → `HasConflicts=true` (the cli/cli#9583 stale-`mergeable`-field scenario, stack.md §3 — `mergeStateStatus` alone must be sufficient to trigger the conflict signal even when `mergeable` reports a healthy state)
  - *Given* case 2's raw JSON `{"statusCheckRollup":[],"reviews":[],"comments":[],"mergeable":"CONFLICTING","mergeStateStatus":"DIRTY"}`, *When* `parsePRStatusPayload` is called, *Then* `status.HasConflicts == true` and `status.FeedbackText` starts with `"## Merge conflict\n"`.
  - *Given* case 5's raw JSON `{"statusCheckRollup":[],"reviews":[],"comments":[],"mergeable":"MERGEABLE","mergeStateStatus":"BLOCKED"}`, *When* `parsePRStatusPayload` is called, *Then* `status.HasConflicts == false` and `status.FeedbackText == ""`.
  - *Given* case 7's raw JSON `{"statusCheckRollup":[],"reviews":[],"comments":[],"mergeable":"MERGEABLE","mergeStateStatus":"DIRTY"}`, *When* `parsePRStatusPayload` is called, *Then* `status.HasConflicts == true` and `status.FeedbackText` starts with `"## Merge conflict\n"` — proving the OR condition catches a conflict that a stale `mergeable` field alone would miss.

**Files**:
- `session/git/worktree_git_test.go`

##### Task 1.3.1a: Add `TestParsePRStatusPayload_ConflictDetection` (~6 min)
- Add a table-driven test with 7 subtests (`t.Run`), one per case above, each constructing a raw JSON literal (as `[]byte`) and asserting `HasConflicts` and (for the trigger cases) that `FeedbackText` contains `"## Merge conflict"`.
- Files: `session/git/worktree_git_test.go`

##### Task 1.3.1b: Add `TestParsePRStatusPayload_ConflictGuidanceText` (~4 min)
- New test covering Story 1.2.1's guidance-text AC directly — not covered by Task 1.3.1a, which only asserts `HasConflicts` and the `"## Merge conflict\n"` prefix.
- Subtest 1: raw JSON with `mergeable=CONFLICTING, mergeStateStatus=DIRTY` (Story 1.2.1's primary AC case). Assert on the rendered `FeedbackText`:
  - `strings.Contains(FeedbackText, "--force-with-lease")`
  - `strings.Contains(FeedbackText, ".gitignore")`
  - `strings.Contains(FeedbackText, "leave the conflict markers")`
  - `strings.Contains(FeedbackText, "git diff --stat")` (pre-mortem Failure #1, P1 — the mandatory diff-stat-reporting instruction)
- Subtest 2: raw JSON with `mergeable=MERGEABLE, mergeStateStatus=DIRTY` (case 7 above, the cli/cli#9583 scenario). Assert the same four substrings are present — proving guidance text renders identically regardless of which field tripped the OR condition (Story 1.2.1's second AC bullet).
- Files: `session/git/worktree_git_test.go`

#### Story 1.3.2: Regression coverage for existing, currently-untested CI/review signals

**As a** backend engineer, **I want** dedicated tests for `CIFailing` and `HasBlockingReviews`, **so that** the pre-existing, never-tested detection logic (requirements.md's explicit scope: "hardening the same reconciliation loop being extended") is verified before this project extends it further.

**Acceptance Criteria**:
- `CIFailing` is proven `true` for a terminal-failure conclusion and `false` for a non-terminal (e.g. `IN_PROGRESS`) check.
  - *Given* raw JSON with `statusCheckRollup: [{"name":"build","conclusion":"FAILURE"}]`, *When* `parsePRStatusPayload` runs, *Then* `status.CIFailing == true` and `FeedbackText` contains `"## Failing CI checks"` and `"build FAILED"`.
- `HasBlockingReviews` is proven `true` for a `CHANGES_REQUESTED` review and `false` for an `APPROVED` review.
  - *Given* raw JSON with `reviews: [{"state":"CHANGES_REQUESTED","body":"Fix the null check","author":{"login":"reviewer1"}}]`, *When* `parsePRStatusPayload` runs, *Then* `status.HasBlockingReviews == true` and `FeedbackText` contains `"## Review: changes requested by @reviewer1"` and `"Fix the null check"`.

**Files**:
- `session/git/worktree_git_test.go`

##### Task 1.3.2a: Add `TestParsePRStatusPayload_CIFailing` (~4 min)
- Two subtests: terminal `FAILURE` conclusion sets `CIFailing=true` with the check name in `FeedbackText`; `IN_PROGRESS`/no terminal conclusion leaves `CIFailing=false` and no `## Failing CI checks` section.
- Files: `session/git/worktree_git_test.go`

##### Task 1.3.2b: Add `TestParsePRStatusPayload_HasBlockingReviews` (~4 min)
- Two subtests: `CHANGES_REQUESTED` review sets `HasBlockingReviews=true` with author/body in `FeedbackText`; `APPROVED`-only reviews leave `HasBlockingReviews=false` and no `## Review:` section.
- Files: `session/git/worktree_git_test.go`

#### Story 1.3.3: `FeedbackText` section-ordering regression test

**As a** backend engineer, **I want** a test proving the conflict section is always ordered first when combined with other signals, **so that** the "conflicts are read before CI/review feedback" design decision (features.md §2A) doesn't silently regress.

**Acceptance Criteria**:
- When a PR has both `CIFailing=true` and `HasConflicts=true`, `FeedbackText` contains `"## Merge conflict"` at an earlier string index than `"## Failing CI checks"`.
  - *Given* raw JSON combining a `FAILURE` check with `mergeable=CONFLICTING, mergeStateStatus=DIRTY`, *When* `parsePRStatusPayload` runs, *Then* `strings.Index(FeedbackText, "## Merge conflict") < strings.Index(FeedbackText, "## Failing CI checks")`.

**Files**:
- `session/git/worktree_git_test.go`

##### Task 1.3.3a: Add `TestParsePRStatusPayload_ConflictSectionOrderedFirst` (~3 min)
- Construct raw JSON with both a failing check and a `CONFLICTING`/`DIRTY` mergeable state; assert the index ordering described above.
- Files: `session/git/worktree_git_test.go`

---

## Phase 2: Reconciliation Gate Extension

**Goal**: `ReconcilePRPending` triggers `AutoReopenForPRFix` on `HasConflicts` alone (in addition to the existing two triggers), logs which signal(s) fired, and — for the first time — has direct unit test coverage for its gate logic, including the two pre-existing untested triggers.

### Epic 2.1: Testability seam + gate/log line extension

**Goal**: `ReconcilePRPending`'s git/PR-status dependency is injectable via a minimal, consumer-scoped interface (following the existing `timeNow` var-override precedent), and its gate/log line are extended for the new signal.

**Human-review requirement (pre-mortem Failure #1, P1)**: conflict-triggered fix cycles must not be treated as auto-mergeable, and must not silently proceed straight to merge just because CI is green. This is not a code change this project makes — it's a behavioral constraint on the spawn path this project must not violate. Verified against current code (`server/services/backlog_service_triage.go`, `session/backlog_lifecycle.go`): `AutoReopenForPRFix` already treats every trigger type uniformly — CI-failure, review, and (after this project) conflict-triggered spawns all transition the item `pr_pending → in_progress`, run the same work session, then re-enter the standard `review` status and review gate (`session/review_gate.go`) before `pushAndCreatePR` reuses the existing PR and transitions it back to `pr_pending`. No trigger type has, or after this project will have, a code path that skips that cycle. So: **no new gating code is needed or added by this plan** — Task 2.1.2a/2.1.2b's gate/log changes are the only code changes in this epic, exactly as already planned.

However, one asymmetry is worth surfacing rather than silently assuming "identical to CI/review, nothing to see here": the one *existing*, pre-this-project auto-merge mechanism in the pipeline is GitHub's own native auto-merge, enabled unconditionally every time any PR reaches `pr_pending` (`session/backlog_lifecycle.go` calling `EnablePRAutoMerge`, `session/git/worktree_git.go:443`) — it is "best-effort" and gated entirely by the repo's GitHub branch-protection settings (required approvals/checks), not by anything in this app. That mechanism does not distinguish an incrementally-reviewable push (what CI/review-triggered fixes produce — new commits appended, existing diff/review context intact) from a force-pushed history rewrite (what a conflict-triggered fix produces — the entire diff view resets, per Failure #1). This pre-existing behavior is **not changed by this project** and is out of this Medium-appetite plan's scope to fix (it would require the `TriggerReason` plumbing already rejected in Pattern Decisions, so a conflict-fix's completion could be distinguished from a CI-fix's completion before deciding whether to leave GitHub auto-merge enabled). It is recorded as a residual risk and flagged as a fast-follow in Unresolved Questions below, not a task in this plan.

#### Story 2.1.1: Introduce the `prPendingChecker` seam

**As a** backend engineer, **I want** `ReconcilePRPending`'s PR-status dependency to be swappable in tests, **so that** the gate logic can be verified without a live, authenticated `gh` CLI or real GitHub state — the reason it has zero coverage today.

**Acceptance Criteria**:
- A new unexported interface `prPendingChecker` is defined in `backlog_lifecycle.go`, with exactly the two methods `ReconcilePRPending` calls: `IsPRMerged(prNumber int) (bool, error)` and `GetPRStatus(prNumber int) (*git.PRStatus, error)`.
  - *Given* `*git.GitWorktree` already implements both methods with those exact signatures, *When* `prPendingChecker` is declared, *Then* `*git.GitWorktree` satisfies it with no changes to package `git`.
- A package-level `var newPRPendingChecker = func(repoPath string) prPendingChecker { return git.NewGitWorktreeFromStorage(repoPath, repoPath, "", "", "") }` replaces the inline construction at line 544.
  - *Given* production code with no test override, *When* `ReconcilePRPending` runs, *Then* behavior is identical to today — same constructor, same arguments.

**Files**:
- `session/backlog_lifecycle.go`

##### Task 2.1.1a: Define `prPendingChecker` interface and `newPRPendingChecker` var (~4 min)
- Near the top of `backlog_lifecycle.go`, alongside the existing `PRFixSpawner`/`AutoReopenSpawner` interfaces, add:
  ```go
  // prPendingChecker is the subset of GitWorktree's PR-status behavior that
  // ReconcilePRPending depends on. Defined here (the consumer) rather than in
  // package git, scoped to exactly what's called.
  type prPendingChecker interface {
      IsPRMerged(prNumber int) (bool, error)
      GetPRStatus(prNumber int) (*git.PRStatus, error)
  }

  // newPRPendingChecker constructs the PR-status checker for a given repo path.
  // Overridable in tests (mirrors the timeNow seam in instance_workspace.go:581).
  var newPRPendingChecker = func(repoPath string) prPendingChecker {
      return git.NewGitWorktreeFromStorage(repoPath, repoPath, "", "", "")
  }
  ```
- Files: `session/backlog_lifecycle.go`

##### Task 2.1.1b: Use `newPRPendingChecker` in `ReconcilePRPending` (~2 min)
- At `backlog_lifecycle.go:544`, replace `g := git.NewGitWorktreeFromStorage(repoPath, repoPath, "", "", "")` with `g := newPRPendingChecker(repoPath)`.
- No other line changes — `g.IsPRMerged(...)` and `g.GetPRStatus(...)` calls at lines 547/563 are unchanged (interface satisfies both).
- Files: `session/backlog_lifecycle.go`

#### Story 2.1.2: Extend the gate to a 3-way OR and the log line to record `conflict=%v`

**As a** backlog automation operator, **I want** `ReconcilePRPending` to spawn a fix session when `HasConflicts` is true, and the log to say so, **so that** merge conflicts get the same autonomous handling CI failures and blocking reviews already get.

**Acceptance Criteria**:
- Line 568's gate becomes `if !prStatus.CIFailing && !prStatus.HasBlockingReviews && !prStatus.HasConflicts { continue }`.
  - *Given* `prStatus = &PRStatus{CIFailing: false, HasBlockingReviews: false, HasConflicts: true}`, *When* the gate is evaluated, *Then* execution proceeds past the `continue` to the spawn block (does not skip).
- The log line at 579-580 gains a third `%v`: `"... (CI=%v, reviews=%v, conflict=%v)"`, passing `prStatus.HasConflicts` as the third argument.
  - *Given* the same `prStatus` above for item `f47ac10b-58cc-4372-a567-0e02b2c3d479`, *When* the spawn path logs, *Then* the log line reads `... item=f47ac10b-... → in_progress for PR fix (CI=false, reviews=false, conflict=true)`.
- No special-casing is added for conflict-triggered spawns regarding merge eligibility (pre-mortem Failure #1, P1): the item transitions through the identical `in_progress` → `review` → review-gate → `pr_pending` cycle as CI/review-triggered spawns, with no shortcut to merge.
  - *Given* a conflict-triggered spawn via the extended gate, *When* its work session completes, *Then* it re-enters `AutoReopenForPRFix`'s existing lifecycle unchanged — same status transitions, same review gate, same `pushAndCreatePR` reuse-existing-PR path as CI/review triggers (verified against current code; see Epic 2.1's Goal note above). This AC records a verification, not a new behavior — this project adds no code for it.

**Files**:
- `session/backlog_lifecycle.go`

##### Task 2.1.2a: Extend the gate at line 568 (~2 min)
- Add `&& !prStatus.HasConflicts` to the `if` condition.
- Files: `session/backlog_lifecycle.go`

##### Task 2.1.2b: Extend the log line at 579-580 (~2 min)
- Change the format string to include `, conflict=%v)` and add `prStatus.HasConflicts` as the final `Printf` argument.
- Files: `session/backlog_lifecycle.go`

---

### Epic 2.2: Regression and new spawn-behavior tests

**Goal**: `ReconcilePRPending`'s gate is directly unit-tested for the new conflict trigger and — for the first time — for the two pre-existing CI/review triggers, using the `prPendingChecker` seam from Epic 2.1.

#### Story 2.2.1: Test doubles for the gate

**As a** backend engineer, **I want** `fakePRPendingChecker` and `fakePRFixSpawner` test doubles, **so that** every gate-behavior test in this epic can construct exact `PRStatus` combinations without touching git or GitHub.

**Acceptance Criteria**:
- `fakePRPendingChecker` implements `prPendingChecker`, returning caller-configured `merged`/`mergedErr`/`status`/`statusErr` values.
- `fakePRFixSpawner` implements `PRFixSpawner`, recording `spawnCalled bool` and `lastFixContext string` on each `AutoReopenForPRFix` call — same shape as the existing `mockReviewGateSpawner` at `backlog_lifecycle_test.go:424`.
  - *Given* `fakePRFixSpawner{}`, *When* `AutoReopenForPRFix(ctx, "item-1", "PR #152 ... needs fixes")` is called, *Then* `spawnCalled == true` and `lastFixContext == "PR #152 ... needs fixes"`.

**Files**:
- `session/backlog_lifecycle_test.go`

##### Task 2.2.1a: Add `fakePRPendingChecker` and `fakePRFixSpawner` types (~5 min)
- Add both types near the existing `mockReviewGateSpawner` definition, following its naming/field conventions.
- Files: `session/backlog_lifecycle_test.go`

#### Story 2.2.2: Conflict signal alone is sufficient to trigger a spawn (new coverage)

**As a** backend engineer, **I want** a test proving `HasConflicts=true` alone spawns a fix session when `CIFailing`/`HasBlockingReviews` are both false, **so that** the new trigger is proven to match the existing OR-gate precedent, not just compile.

**Acceptance Criteria**:
- With a `pr_pending` item (`PrNumber=152`, `PrURL="https://github.com/TylerStaplerAtFanatics/stapler-squad/pull/152"`), `newPRPendingChecker` overridden to return a `fakePRPendingChecker{merged: false, status: &git.PRStatus{HasConflicts: true, FeedbackText: "## Merge conflict\n..."}}`, and `listener.SetPRFixSpawner(fakeSpawner)`, `ReconcilePRPending` calls `fakeSpawner.AutoReopenForPRFix` exactly once.
  - *Given* item ID `f47ac10b-58cc-4372-a567-0e02b2c3d479` in `pr_pending` with the fake checker above, *When* `ReconcilePRPending(ctx, er)` runs, *Then* `fakeSpawner.spawnCalled == true` and `fakeSpawner.lastFixContext` contains `"## Merge conflict"`.
- The log line emitted by the same spawn path records the conflict signal, per requirements.md's Observability Requirements ("which signal ... triggered the spawn") and Story 2.1.2's exact log-format AC — no task previously verified this string is actually produced.
  - *Given* the same `prStatus` as above (`CIFailing: false, HasBlockingReviews: false, HasConflicts: true`), *When* `ReconcilePRPending` logs the spawn while `log.InfoLog` is redirected to a `log.NewDummyLogger` (see `log/log_test.go:186`, `NewDummyLogger`), *Then* the captured output contains `conflict=true`.

**Files**:
- `session/backlog_lifecycle_test.go`

##### Task 2.2.2a: Add `TestReconcilePRPending_SpawnsFixSession_WhenHasConflictsTrue_Alone` (~5 min)
- Create a `pr_pending` `BacklogItemData` via `storage.CreateBacklogItem`, override `newPRPendingChecker` for the test duration (`t.Cleanup` to restore), wire `fakePRFixSpawner` via `listener.SetPRFixSpawner`, call `ReconcilePRPending`, assert spawn occurred with conflict-section content in the fix context.
- Files: `session/backlog_lifecycle_test.go`

##### Task 2.2.2b: Add `TestReconcilePRPending_LogsConflictTrue_WhenConflictTriggersSpawn` (~4 min)
- Redirect `log.InfoLog` to a `log.NewDummyLogger(buf, "INFO: ")` backed by a `*bytes.Buffer` (`t.Cleanup` to restore the original logger) for the duration of the test.
- Reuse the same `pr_pending` item / `fakePRPendingChecker{status: &git.PRStatus{HasConflicts: true, ...}}` setup as Task 2.2.2a, call `ReconcilePRPending`, then assert `strings.Contains(buf.String(), "conflict=true")`.
- Files: `session/backlog_lifecycle_test.go`

#### Story 2.2.3: Regression coverage for the pre-existing CI/review triggers

**As a** backend engineer, **I want** tests proving `CIFailing=true` alone and `HasBlockingReviews=true` alone each still trigger a spawn — and that the log line correctly reports `conflict=false` when the conflict signal didn't fire, **so that** the reconciliation loop this project extends is hardened everywhere requirements.md asks for it, not just at the new signal.

**Acceptance Criteria**:
- `HasConflicts=false, CIFailing=true, HasBlockingReviews=false` → spawn occurs, and the log line reports `conflict=false`.
  - *Given* `fakePRPendingChecker{status: &git.PRStatus{CIFailing: true, FeedbackText: "## Failing CI checks\n- build FAILED\n"}}`, *When* `ReconcilePRPending` runs, *Then* `fakeSpawner.spawnCalled == true`, and, with `log.InfoLog` redirected per Task 2.2.2b's pattern, the captured line contains `CI=true` and `conflict=false`.
- `HasConflicts=false, CIFailing=false, HasBlockingReviews=true` → spawn occurs, and the log line reports `conflict=false`.
  - *Given* `fakePRPendingChecker{status: &git.PRStatus{HasBlockingReviews: true, FeedbackText: "## Review: changes requested by @reviewer1\n"}}`, *When* `ReconcilePRPending` runs, *Then* `fakeSpawner.spawnCalled == true`, and, with `log.InfoLog` redirected per Task 2.2.2b's pattern, the captured line contains `reviews=true` and `conflict=false`.

**Files**:
- `session/backlog_lifecycle_test.go`

##### Task 2.2.3a: Add `TestReconcilePRPending_SpawnsFixSession_WhenCIFailingTrue` (~5 min)
- Also redirect `log.InfoLog` (Task 2.2.2b's pattern) and assert the captured line contains `CI=true` and `conflict=false` — proving the log line doesn't spuriously report a conflict when only CI failed.
- Files: `session/backlog_lifecycle_test.go`

##### Task 2.2.3b: Add `TestReconcilePRPending_SpawnsFixSession_WhenHasBlockingReviewsTrue` (~5 min)
- Also redirect `log.InfoLog` (Task 2.2.2b's pattern) and assert the captured line contains `reviews=true` and `conflict=false` — proving the log line doesn't spuriously report a conflict when only a review blocked.
- Files: `session/backlog_lifecycle_test.go`

#### Story 2.2.4: Healthy PR does not spawn (gate stays closed)

**As a** backend engineer, **I want** a test proving a PR with all three signals false does not trigger a spawn, **so that** the extended 3-way gate doesn't regress into over-triggering.

**Acceptance Criteria**:
- `HasConflicts=false, CIFailing=false, HasBlockingReviews=false` → no spawn, item remains `pr_pending`.
  - *Given* `fakePRPendingChecker{status: &git.PRStatus{}}` (all signals false, matching live-verified PR #151 data), *When* `ReconcilePRPending` runs, *Then* `fakeSpawner.spawnCalled == false` and the item's status is unchanged.

**Files**:
- `session/backlog_lifecycle_test.go`

##### Task 2.2.4a: Add `TestReconcilePRPending_NoSpawn_WhenAllSignalsFalse` (~3 min)
- Files: `session/backlog_lifecycle_test.go`

---

## Task Summary

| Phase | Epic | Stories | Tasks |
|---|---|---|---|
| Phase 1 | 1.1 PRStatus + payload extension | 1 | 4 |
| Phase 1 | 1.2 Conflict-specific prompt guidance | 1 | 1 |
| Phase 1 | 1.3 Table-driven + regression tests | 3 | 5 |
| Phase 2 | 2.1 Seam + gate + log line | 2 | 4 |
| Phase 2 | 2.2 Regression + new spawn tests | 4 | 6 |
| **Total** | **5 epics** | **11 stories** | **20 tasks** |

**Production files touched**: 2 (`session/git/worktree_git.go`, `session/backlog_lifecycle.go`) — matches the pre-established fact that exactly 2 files need production code changes; the `prPendingChecker` testability seam lives inside `backlog_lifecycle.go` and does not add a third file.
**Test files touched**: 2 (`session/git/worktree_git_test.go`, `session/backlog_lifecycle_test.go`).
