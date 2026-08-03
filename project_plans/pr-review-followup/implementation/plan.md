# Implementation Plan: pr-review-followup

**Feature**: Detect unaddressed `COMMENTED`-state GitHub reviews and substantive plain
PR comments (Copilot's typical review posture) as a new signal in `ReconcilePRPending`,
dedup via a per-item timestamp watermark so already-addressed feedback never
re-triggers a fix session, request a Copilot review once at PR-creation time, and fix
the pre-existing `STUCK_REASON_PR_NEEDS_FIX` proto-enum gap the reuse-first design
depends on.
**Date**: 2026-08-02
**Status**: Ready for implementation
**ADRs**: [ADR-001: Per-item timestamp watermark for PR-feedback dedup](../decisions/ADR-001-timestamp-watermark-dedup.md)

---

## Step 0.5 — Alternatives Considered (Creative Pass)

Three high-level approaches were weighed for the core "detect new comment feedback,
don't re-trigger on old feedback" mechanism:

1. **Timestamp watermark** — one nullable `time.Time` column on `BacklogItem`
   (`pr_feedback_addressed_at`), advanced to the newest GitHub-assigned
   `submittedAt`/`createdAt` every time a fix is confirmed dispatched.
   *Strength*: trivial to reason about (one `.After()` comparison), no unbounded
   growth, matches the existing single-scalar-watermark precedent already on this
   exact schema (`shipped_snapshot_at`, `plan_approved_at`, `queued_at`).
   *Weakness*: cannot distinguish "10 comments arrived, 1 was addressed" from "10
   comments arrived, all addressed" within one tick — a coarser grain than per-ID
   tracking.
2. **ID-based dedup set** — persist a list/set of GitHub review+comment IDs already
   responded to (JSON blob, mirroring `shipped_file_stats`'s JSON-string-column
   precedent on the same schema).
   *Strength*: precise per-comment resolution; two comments with identical
   timestamps never collide.
   *Weakness*: unbounded growth requiring pruning, JSON encode/decode on every read,
   and solves a multi-PR-per-item collision problem this domain doesn't have (one
   item has at most one open PR at a time — `item.PrNumber`/`item.PrURL` are
   wholesale-replaced, not appended-to, on PR close).
3. **GraphQL `reviewThreads.isResolved`-based suppression** — treat a human manually
   resolving the GitHub review thread as the suppression signal (CodeRabbit's
   Autofix precedent).
   *Strength*: gives the repo owner a manual override with zero new stapler-squad UI
   (resolving the thread on GitHub IS the escape hatch).
   *Weakness*: requires a second `gh api graphql` call per `pr_pending` item per
   tick (roughly doubles this feature's GitHub API volume), and top-level PR
   comments have no `isResolved` concept at all — would only cover half the signal.

**Chosen: Approach 1 (timestamp watermark).** See ADR-001 for the full reasoning,
including the explicit resolution of the contradiction between build-vs-buy.md/
pitfalls.md (which argued for ID-based dedup) and architecture.md (which argued for
a timestamp watermark). Approaches 2 and 3 are recorded as rejected alternatives in
the Pattern Decisions table below, not silently dropped.

---

## System Type Confirmation

**Confirmed**: this is a Transaction Script extension of an existing polling/
reconciliation loop in a Go backend (`session/backlog_lifecycle.go`'s
`ReconcilePRPending`), not new infrastructure. Every artifact this plan touches
already exists and already has three working analogs (`CIFailing`,
`HasBlockingReviews`, `HasConflicts`) driving the exact same
detect → gate → spawn control flow via `remediatePRFixWithBackoffGate`. No new
actor, no new bounded context, no Event-Command-Policy table is warranted
(architecture.md §5) — this plan adds one new boolean-shaped signal, one new
persisted timestamp, and one new `&&` term to an existing `if`.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `PRStatus` | Struct in `session/git/worktree_git.go:421` holding the CI/review/conflict/feedback state of one PR, derived from a single `gh pr view --json ...` call. | Existing; extended by this plan. |
| `parsePRStatusPayload` | Pure, I/O-free function (`worktree_git.go:549`) that unmarshals `gh pr view`'s JSON and evaluates every `PRStatus` signal. | Existing; extended by this plan. |
| `GetPRStatus` | Thin I/O wrapper (`worktree_git.go:528`) that runs `gh pr view` and calls `parsePRStatusPayload`. | Existing; unchanged shape, only the `--json` unmarshal target struct grows. |
| `reviewInfo` | Existing unexported struct (`worktree_git.go:418`, `{author, body string}`) backing `blockingReviews` (`CHANGES_REQUESTED` reviews). | Untouched — `HasBlockingReviews`'s meaning is not redefined. |
| `prFeedbackItem` | **New** unexported struct (`{author, body string; at time.Time}`) replacing `generalComments`'s `[]string` element type and backing the new `commentReviews` slice. | New this plan. |
| `commentReviews` | **New** unexported `[]prFeedbackItem` field on `PRStatus` capturing substantive `COMMENTED`-state reviews (Copilot's typical posture) that today are silently dropped. | New this plan. |
| `generalComments` | Existing unexported field on `PRStatus`, retyped from `[]string` to `[]prFeedbackItem` so each comment carries its GitHub `createdAt` timestamp; `render()`'s rendered text is byte-identical to today. | Retyped, not renamed; rendering behavior preserved. |
| `HasReviewFeedback` | **New** exported `bool` field on `PRStatus`: true when at least one substantive `COMMENTED` review or substantive plain comment exists. | New this plan. Does not affect `HasBlockingReviews`. |
| `LatestFeedbackAt` | **New** exported `time.Time` field on `PRStatus`: the newest GitHub-assigned `submittedAt`/`createdAt` among all substantive feedback captured this call; zero value when `HasReviewFeedback` is false. | New this plan. |
| `isSubstantiveFeedback` | **New** unexported predicate (`worktree_git.go`) — `len(strings.TrimSpace(body)) >= substantiveFeedbackMinLen` (10 runes). | New this plan; resolves Open Question (2). |
| `render()` | Existing method (`worktree_git.go:472`) assembling `FeedbackText` from `PRStatus`'s captured fields, conflict-first. | Extended with a new `## Reviewer comments` section; existing section order preserved. |
| `FeedbackText` | Existing exported `string` field — the combined human-readable summary handed to the fix agent via `fixCtx`. | Unchanged assembly order; gains new content. |
| `PrFeedbackAddressedAt` | **New** nullable `time.Time` field/column, threaded through `session/ent/schema/backlog_item.go`, `ent.BacklogItem`, `BacklogItemData`, `BacklogItemUpdate` — the per-item high-water mark: the `LatestFeedbackAt` value a fix session has already been dispatched to address. | New this plan; the dedup watermark (ADR-001). |
| `hasNewFeedback` | **New** local boolean in `ReconcilePRPending` — `prStatus.HasReviewFeedback && (item.PrFeedbackAddressedAt == nil || prStatus.LatestFeedbackAt.After(*item.PrFeedbackAddressedAt))`. | New this plan; the actual trigger condition. |
| `ReconcilePRPending` | Existing method (`backlog_lifecycle.go:3850`) — the 60s-tick sweep over every `pr_pending` item; gains a fourth gate disjunct and a watermark-persist step. | Existing; extended. |
| `remediatePRFixWithBackoffGate` | Existing method (`backlog_lifecycle.go:3804`) wrapping every `AutoReopenForPRFix` dispatch in the shared, durable, time-based exponential-backoff gate. | Existing; reused unmodified — no parallel spawn mechanism. |
| `RemediationDue` | Existing `Storage` method (`session/backlog_remediation.go:168`) — the time-based (30m/2h/8h/24h/72h, cap 5) gate `remediatePRFixWithBackoffGate` calls before dispatching. | Existing; unmodified. Answers "is it time to try again", not "is there anything new". |
| `MarkStuck` | Existing method opening/refreshing a `BacklogStuckState` row for `(itemID, StuckReasonPRNeedsFix)`. | Existing; unmodified — the new signal reuses the same reason, no new row type. |
| `StuckReasonPRNeedsFix` | Existing `domain.StuckReason` constant (`session/domain/backlog.go:132`, value `"pr_needs_fix"`) — shared by CI/review/conflict/comment-feedback triggers alike. | Existing; unmodified. |
| `AutoReopenForPRFix` | Existing method (`server/services/backlog_service_triage.go:1444`) — transitions the item to `in_progress` and spawns a fix work session, gated by `effectiveReworkCap`. | Existing; unmodified — no new spawn path. |
| `effectiveReworkCap` | Existing method (`backlog_service_triage.go:88`) — the shared, trigger-agnostic rework budget (default `maxAutoReworkIterations = 3`) all four triggers draw from. | Existing; unmodified per constraint. |
| `prCreator` | Existing interface (`backlog_lifecycle.go:235`) implemented by `*git.GitWorktree`, consumed by `pushAndCreatePR`. | Extended with one new method this plan. |
| `RequestCopilotReview` | **New** method on `*git.GitWorktree` (near `EnablePRAutoMerge`, `worktree_git.go:650`) — `gh pr edit <n> --add-reviewer copilot-pull-request-reviewer[bot]`, best-effort. | New this plan. |
| `pushAndCreatePR` | Existing method (`backlog_lifecycle.go:~3230-3320`) — the ship-flow's PR-creation step, already calls `EnablePRAutoMerge` best-effort after `CreatePR`. | Existing; gains one more best-effort call after `EnablePRAutoMerge`. |
| `STUCK_REASON_PR_NEEDS_FIX` | **New** proto enum value (`proto/session/v1/backlog.proto`, value `13`) closing the gap where `StuckReasonPRNeedsFix` items render "Unknown reason" on `/unfinished`. | New this plan — in scope, not deferred (Open Question 4). |
| `toProtoStuckReason` / `fromProtoStuckReason` | Existing switch pair (`server/services/backlog_service_stuck.go:28`, `:63`) mapping `domain.StuckReason` ↔ proto `StuckReason`; both currently fall through to `UNSPECIFIED`/`""` for `pr_needs_fix`. | Existing; each gains one `case`. |
| `BacklogItemData` / `BacklogItemUpdate` | Existing DTOs (`session/repository.go:345`, `:498`) — the read/write model for a backlog item, mirrored field-for-field from `ent.BacklogItem`. | Existing; each gains one field. |
| `backlogItemToData` | Existing mapping function (`session/ent_repository_backlog.go:170`) — `*ent.BacklogItem` → `BacklogItemData`. | Existing; gains one field mapping. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Dedup/staleness mechanism | Single per-item timestamp watermark (`PrFeedbackAddressedAt`) | architecture.md §1c | ID-based set of seen review/comment IDs (build-vs-buy.md §3, pitfalls.md §1) | See ADR-001 — resolved explicitly, not by default. Core reasoning: an item has at most one open PR at a time, and both sides of the comparison (`LatestFeedbackAt`, the stored watermark) originate from GitHub's own server-assigned `submittedAt`/`createdAt`, never from this app's local clock — so the clock-skew risk that motivates ID-based dedup in a *cross-system* comparison does not apply here. `.After()` on two GitHub-issued timestamps is monotonic for this single-PR-per-item domain. An ID set solves a "which of N concurrent items already saw this" problem this domain structurally doesn't have. |
| New signal integration into `ReconcilePRPending` | Transaction Script extension — one more `&&` term in the existing gate | features.md §1, architecture.md §5 | Command/Policy object per trigger type | No new actor, no new branching business process — CI/review/conflict/comment-feedback all already share one detect→gate→spawn flow. Introducing a Command per trigger would be speculative layering for a single boolean. |
| Copilot review request timing | Always attempted, one-shot, at PR-creation time inside `pushAndCreatePR` | features.md §4 ("Unstated needs"), stack.md §4 | Settings-toggle-gated opt-in | `pushAndCreatePR` fires once per PR, not on a recurring tick — it structurally cannot runaway-retrigger the way a 60s-loop signal could, so there is no cost/noise risk a toggle would be protecting against. A toggle would be speculative config for a low-risk, best-effort call (resolves Open Question 3). |
| Copilot reviewer identifier | Unconditional legacy literal login `copilot-pull-request-reviewer[bot]` via `--add-reviewer` | architecture.md §4, build-vs-buy.md §4 | Runtime `gh --version` detection, switching to `@copilot` on ≥2.88.0 | The literal bot login is a normal reviewer login accepted by `--add-reviewer` on every `gh` version tested (2.86 and later) — only the `@copilot` *alias* is version-gated. Unconditional use of the literal login avoids a runtime-detection branch and its own untested failure mode, at zero functional cost (resolves Open Question 5). |
| "Substantive" feedback filter | `len(strings.TrimSpace(body)) >= 10` runes | pitfalls.md §1(a)/(c) rabbit-hole flag; no existing precedent found | Keyword/NLP denylist (e.g. exclude "LGTM", "nice", bot status text) | Small appetite; a length threshold is a one-line, fully deterministic, directly-unit-testable guard against the concrete "bare LGTM" example named in requirements.md's Rabbit Holes, without maintaining a denylist that will always be incomplete (resolves Open Question 2). |
| Watermark write timing | Persist `PrFeedbackAddressedAt` only after `remediatePRFixWithBackoffGate` returns `attempted=true, fixErr=nil` | pitfalls.md §1(c), BUG-040 precedent (`backlog_lifecycle.go:4016-4024`) | Persist watermark speculatively before/at dispatch | BUG-040 established "mutate durable state only once the outcome is confirmed" for this exact function; a speculative pre-dispatch write risks losing track of unaddressed feedback if the dispatch call itself fails or no-ops (e.g. `attempted=false` because the backoff gate wasn't due). |
| `STUCK_REASON_PR_NEEDS_FIX` proto gap | Fix now, in this project's scope | ux.md — flagged as blocking, not optional | Defer as a separate pre-existing-bug ticket | This project's entire premise is "reuse `StuckReasonPRNeedsFix`/`/unfinished` — no new visibility mechanism" (requirements.md, In Scope). That reuse is broken today (renders "Unknown reason", "Retry now" non-functional) for *all four* triggers sharing this reason, not just the new one — shipping the new comment-feedback trigger without this fix ships a broken UI path for it on day one (resolves Open Question 4). |

---

## Migration Plan

- **Migration file**: none hand-written — ent's schema-driven migrator auto-generates
  the `ALTER TABLE backlog_items ADD COLUMN pr_feedback_addressed_at ...` at
  application startup (this repo's existing ent migration pattern; confirm via
  `grep -rn "AutoMigrate\|Schema.Create" session/ent_repository.go` if unfamiliar —
  no separate `.sql` migration file exists for any other `backlog_item` field
  either, e.g. `shipped_snapshot_at`).
- **Reversibility**: fully reversible — a single nullable, no-default column with no
  backfill. Dropping it (or leaving it unused) has no cascading effect on any other
  column or index.
- **Zero-downtime strategy**: additive nullable column, same shape as
  `shipped_snapshot_at`/`plan_approved_at`/`queued_at` before it — old code paths
  that don't know about the column are unaffected; no data needs to exist in it for
  existing rows (`nil` is the correct "no feedback-triggered fix dispatched yet"
  state for every pre-existing item).
- **Rollback procedure**: revert the `session/git/worktree_git.go`,
  `session/ent/schema/backlog_item.go` (+ regenerated `session/ent/*`),
  `session/repository.go`, `session/ent_repository_backlog.go`, and
  `session/backlog_lifecycle.go` commits. The column itself can be left in place
  (unused nullable columns are inert) or dropped in a follow-up migration — no
  cross-column data dependency exists. **Correction to requirements.md's Risk
  Control line**: that section states "no schema/data migration involved" — this is
  stale; an ent schema migration (§ above) IS required by this plan. Rollback is
  still low-risk (additive nullable column, no backfill), just not "no migration."

---

## Observability Plan

- **Logs**:
  - `ReconcilePRPending`'s existing dispatch log line (`backlog_lifecycle.go:4107`)
    gains a fourth `%v`: `"... (CI=%v, reviews=%v, conflict=%v, feedback=%v)"`,
    passing `hasNewFeedback` (not raw `prStatus.HasReviewFeedback`, which would
    spuriously log `true` for already-addressed feedback that isn't triggering
    anything this tick).
  - New `log.InfoLog` line when `PrFeedbackAddressedAt` is persisted:
    `"[BacklogLifecycle] ReconcilePRPending item=%s PrFeedbackAddressedAt advanced to %s (PR #%d)"`.
  - New `log.WarningLog` line (mirrors `EnablePRAutoMerge`'s existing pattern,
    `backlog_lifecycle.go:3304-3311`) when `RequestCopilotReview` fails.
  - **New (pre-mortem.md P1 — required, not optional)**: when a `hasNewFeedback`
    dispatch covers more than one substantive item (`len(commentReviews) +
    len(substantive generalComments) > 1`), log the count and authors of every item
    included in that dispatch's context:
    `"[BacklogLifecycle] ReconcilePRPending item=%s dispatching PR-fix session covering %d feedback item(s) from [%s] — watermark advances to %s regardless of which items the session actually addresses"`.
    Rationale: the timestamp watermark (ADR-001, adversarial-review.md Concern-1)
    advances past the whole batch on dispatch, not per-item, so a partially-addressed
    multi-item batch is otherwise silently unrecoverable — this log line is the one
    place an operator can discover it happened. This does not fix the coarse-grain
    dedup gap (out of appetite per ADR-001); it makes the gap discoverable.
- **Metrics**: none added — this repo has no existing metrics pipeline for
  `ReconcilePRPending`'s other three triggers either (log-line-only observability is
  the established convention here).
- **Alerts**: none added — `StuckReasonPRNeedsFix` surfacing on `/unfinished` (once
  Epic 4.1/4.2 land) is this repo's existing alerting surface for "automation
  couldn't resolve it"; no new alert channel is warranted for a Small-appetite
  extension of an already-alerted path.

---

## Risk Control

- **Feature flag**: none. Per requirements.md ("No feature flag") and consistent
  with the other three `ReconcilePRPending` triggers, which also ship unflagged.
- **Rollback procedure**: revert the commits touching `session/git/worktree_git.go`,
  `session/ent/schema/backlog_item.go` + regenerated `session/ent/*`,
  `session/repository.go`, `session/ent_repository_backlog.go`,
  `session/backlog_lifecycle.go`, `proto/session/v1/backlog.proto` + regenerated
  bindings, `server/services/backlog_service_stuck.go`,
  `web-app/src/components/backlog-stuck/stuckReason.ts` +
  `stuckReason.css.ts`. No data migration/backfill to unwind (see Migration Plan
  correction above — the requirements.md line claiming no migration is stale).
- **Staged rollout**: none — this repo has no staged-rollout mechanism for backend
  reconciler logic; the existing `RemediationDue` 5-attempt/72h-span backoff and the
  shared `effectiveReworkCap = 3` are themselves the blast-radius control (bounded,
  not staged). Ship behind normal PR review + CI, same as the three existing
  triggers.

---

## Unresolved Questions

None blocking implementation. One item is explicitly deferred (not unresolved):
GraphQL `reviewThreads.isResolved` as a manual-override suppression signal
(requirements.md Open Questions, features.md §4) is deliberately **not** built —
Approach 3 in the Creative Pass above, rejected for doubling API call volume and
only covering half the feedback surface (review threads, not top-level comments).
If the timestamp-watermark dedup proves too coarse in practice (e.g. the "10
comments, 1 addressed" grain problem named in ADR-001), that GraphQL path is the
documented next lever — not a gap in this plan.

---

## Dependency Visualization

```
Phase 1: Signal detection (worktree_git.go)          Phase 2: Dedup persistence (ent + repository)
┌─────────────────────────────────┐                  ┌──────────────────────────────────┐
│ 1.1 payload struct + COMMENTED   │                  │ 2.1 ent schema field + regenerate │
│     case + isSubstantiveFeedback │                  │ 2.2 BacklogItemData/Update +      │
│ 1.2 HasReviewFeedback/           │                  │     backlogItemToData mapping     │
│     LatestFeedbackAt computation │                  └──────────────┬───────────────────┘
│ 1.3 render() new section         │                                 │
│ 1.4 regression tests             │                                 │
└──────────────┬────────────────────                                 │
               │                                                     │
               └───────────────────┬─────────────────────────────────┘
                                    ▼
                    Phase 3: ReconcilePRPending wiring
                    ┌─────────────────────────────────────┐
                    │ 3.1 hasNewFeedback gate               │
                    │ 3.2 log line update                   │
                    │ 3.3 watermark persist (post-dispatch)  │
                    │ 3.4 watermark clear (closed-PR branch) │
                    └──────────────┬─────────────────────────┘
                                   │
                                   ▼
                    Phase 6: Regression validation
                    (existing CI/review/conflict tests pass unchanged)

Phase 4: STUCK_REASON_PR_NEEDS_FIX gap        Phase 5: Copilot review request
(independent — no dependency on 1-3)          (independent — no dependency on 1-3)
┌───────────────────────────────┐             ┌────────────────────────────────────┐
│ 4.1 proto enum + make proto-gen │             │ 5.1 RequestCopilotReview method     │
│ 4.2 Go switch cases (x2)        │             │ 5.2 prCreator interface + wiring    │
│ 4.3 frontend Record maps + CSS  │             │ 5.3 REAL dry-run verification task  │
└───────────────────────────────┘             └────────────────────────────────────┘
```

Phases 1→2→3 are sequential (3 depends on both 1's `PRStatus` fields and 2's
persistence layer). Phases 4 and 5 have no dependency on 1-3 or each other and can
run in parallel with them. Phase 6 depends on 1-3 being complete.

---

## Phase 1: Signal Detection

### Epic 1.1: Extend `parsePRStatusPayload` to capture `COMMENTED` reviews and comment timestamps

**Goal**: Widen the existing single `gh pr view` JSON unmarshal to capture
`submittedAt`/`createdAt` and add a `COMMENTED` review case, without touching
`HasBlockingReviews`'s `CHANGES_REQUESTED`-only meaning.

#### Story 1.1.1: Widen the payload struct and add the substantive-feedback filter
**As a** fix agent, **I want** the parser to see review/comment timestamps and a
substantiveness filter, **so that** trivial feedback (a bare "LGTM") never becomes a
trigger and every kept item carries a real GitHub timestamp for dedup.
**Acceptance Criteria**:
- `payload.Reviews[]` gains `SubmittedAt string \`json:"submittedAt"\`` and
  `payload.Comments[]` gains `CreatedAt string \`json:"createdAt"\`` — no `--json`
  flag change needed (verified live in stack.md §1: both sub-fields are already
  returned whenever the parent `reviews`/`comments` top-level field is requested).
  - *Given* `gh pr view 152 --json reviews` returns
    `{"reviews":[{"state":"COMMENTED","body":"Consider extracting this into a helper function.","author":{"login":"copilot-pull-request-reviewer[bot]"},"submittedAt":"2026-08-02T14:32:07Z"}]}`,
    *When* `parsePRStatusPayload` unmarshals it, *Then* `payload.Reviews[0].SubmittedAt == "2026-08-02T14:32:07Z"`.
- `isSubstantiveFeedback(body string) bool` returns `len(strings.TrimSpace(body)) >= 10`.
  - *Given* `body = "lgtm"` (4 runes after trim), *When* `isSubstantiveFeedback(body)` is called, *Then* it returns `false`.
  - *Given* `body = "Consider extracting this into a helper function."` (49 runes), *When* called, *Then* it returns `true`.
  - *Given* `body = "   "` (whitespace only), *When* called, *Then* it returns `false`.
**Files**: `session/git/worktree_git.go`

##### Task 1.1.1a: Add `SubmittedAt`/`CreatedAt` fields to the payload struct (~3 min)
- In the anonymous `payload` struct inside `parsePRStatusPayload` (`worktree_git.go:550-578`), add `SubmittedAt string \`json:"submittedAt"\`` to the `Reviews` element (after `Author`) and `CreatedAt string \`json:"createdAt"\`` to the `Comments` element (after `Author`).
- Files: `session/git/worktree_git.go`

##### Task 1.1.1b: Add `isSubstantiveFeedback` and `substantiveFeedbackMinLen` (~2 min)
- Add `const substantiveFeedbackMinLen = 10` and
  `func isSubstantiveFeedback(body string) bool { return len(strings.TrimSpace(body)) >= substantiveFeedbackMinLen }`
  near the top of `worktree_git.go` (alongside other small helpers, e.g. near `reviewInfo`'s definition at line 418).
- Files: `session/git/worktree_git.go`

##### Task 1.1.1c: Add `TestIsSubstantiveFeedback` table test (~4 min)
- New test in `session/git/worktree_git_test.go`: cases `""` → false, `"   "` → false, `"lgtm"` → false, `"nice"` → false, `"Consider extracting this into a helper function."` → true, exactly-10-rune body → true (boundary).
- Files: `session/git/worktree_git_test.go`

#### Story 1.1.2: Introduce `prFeedbackItem`, capture `COMMENTED` reviews, retype `generalComments`
**As a** fix agent, **I want** `COMMENTED` reviews captured (today silently dropped)
and every comment/review timestamped, **so that** the new signal has real data to
compute from.
**Acceptance Criteria**:
- A `COMMENTED`-state review with a substantive body is captured into a new
  `commentReviews []prFeedbackItem` field; a non-substantive one is not.
  - *Given* two reviews on PR #152 — `{state:"COMMENTED", body:"lgtm", submittedAt:"2026-08-02T14:00:00Z"}` and `{state:"COMMENTED", body:"Consider extracting this into a helper function.", submittedAt:"2026-08-02T14:32:07Z"}` — *When* `parsePRStatusPayload` runs, *Then* `len(status.commentReviews) == 1` and its `at` field equals `2026-08-02T14:32:07Z` parsed as `time.Time`.
- `generalComments` is retyped from `[]string` to `[]prFeedbackItem`; `render()`'s
  output text is unchanged (verified by Story 1.1.4's regression test).
  - *Given* a comment `{body:"Please rebase.", author:{login:"tstapler"}, createdAt:"2026-08-02T13:00:00Z"}`, *When* parsed, *Then* `status.generalComments[0] == prFeedbackItem{author:"tstapler", body:"Please rebase.", at: <parsed time>}`.
**Files**: `session/git/worktree_git.go`

##### Task 1.1.2a: Define `prFeedbackItem` struct (~2 min)
- Add `type prFeedbackItem struct { author, body string; at time.Time }` near `reviewInfo`'s definition (`worktree_git.go:418`).
- Files: `session/git/worktree_git.go`

##### Task 1.1.2b: Retype `generalComments` field declaration (~2 min)
- On `PRStatus` (`worktree_git.go:466`), change `generalComments []string` to `generalComments []prFeedbackItem`; update its doc comment to note it now carries timestamps.
- Files: `session/git/worktree_git.go`

##### Task 1.1.2c: Add `commentReviews []prFeedbackItem` field to `PRStatus` (~2 min)
- Add a new unexported field after `generalComments` (`worktree_git.go:466`): `commentReviews []prFeedbackItem // unexported; substantive COMMENTED-state reviews, captured for the HasReviewFeedback signal`.
- Files: `session/git/worktree_git.go`

##### Task 1.1.2d: Add the `COMMENTED` case to the review switch, parse `submittedAt` (~4 min)
- In the review loop (`worktree_git.go:627-636`), add `case "COMMENTED":` after `case "CHANGES_REQUESTED":` — parse `r.SubmittedAt` with `time.Parse(time.RFC3339, r.SubmittedAt)`. **On parse error, do NOT zero-value `at`** (adversarial-review.md BLOCKER: a zero-value `at` can lose to an already-persisted, later `PrFeedbackAddressedAt` watermark, silently suppressing detection of genuinely new feedback — the inverse, silent-miss version of the exact risk ADR-001 reasoned about). Instead: log the parse failure at warn level (still following stack.md/pitfalls.md §4's "fail loudly" spirit — the log line makes it visible, not silent) and fall back to `at: time.Now()`. `time.Now()` is safe specifically because it is guaranteed to be no earlier than any prior GitHub-issued watermark this process could have already persisted, so it can only ever push `LatestFeedbackAt` later, never mask a real later item under an earlier one. Still append to `status.commentReviews` only `if isSubstantiveFeedback(r.Body)`.
- Files: `session/git/worktree_git.go`

##### Task 1.1.2e: Parse `createdAt` and populate retyped `generalComments` (~3 min)
- In the comments loop (`worktree_git.go:639-641`), parse `c.CreatedAt` the same way, and change the append to `status.generalComments = append(status.generalComments, prFeedbackItem{author: c.Author.Login, body: c.Body, at: parsedAt})` — every comment is still included unconditionally (unchanged behavior), only the element type changes.
- Files: `session/git/worktree_git.go`

#### Story 1.1.3: Compute `HasReviewFeedback`/`LatestFeedbackAt` and render the new section
**As a** `ReconcilePRPending` caller, **I want** one bool and one timestamp
summarizing "is there new substantive feedback and when did the newest arrive",
**so that** the reconciler doesn't need to re-derive it from raw slices.
**Acceptance Criteria**:
- `HasReviewFeedback` is true and `LatestFeedbackAt` equals the max timestamp across
  substantive `commentReviews` entries and substantive `generalComments` entries.
  - *Given* `commentReviews = [{at: 2026-08-02T14:32:07Z}]` and `generalComments = [{body:"Please rebase.", at: 2026-08-02T15:10:00Z}]` (both substantive — "Please rebase." is 14 runes), *When* `parsePRStatusPayload` finishes, *Then* `status.HasReviewFeedback == true` and `status.LatestFeedbackAt == 2026-08-02T15:10:00Z` (the later of the two).
  - *Given* no `COMMENTED` reviews and only a comment body `"lgtm"`, *When* parsed, *Then* `status.HasReviewFeedback == false` and `status.LatestFeedbackAt.IsZero() == true`.
- `render()` emits a `## Reviewer comments` section for `commentReviews`, placed
  after the existing `## Review: changes requested by @...` block(s) and before
  `## PR comments`; existing sections' text is byte-identical to before this change.
  - *Given* `commentReviews = [{author:"copilot-pull-request-reviewer[bot]", body:"Consider extracting this into a helper function."}]`, *When* `render()` runs, *Then* the output contains `"## Reviewer comments\n@copilot-pull-request-reviewer[bot]: Consider extracting this into a helper function.\n\n"`.
**Files**: `session/git/worktree_git.go`

##### Task 1.1.3a: Compute `HasReviewFeedback`/`LatestFeedbackAt` at the end of `parsePRStatusPayload` (~4 min)
- After the comments loop (`worktree_git.go:641`, before `status.FeedbackText = status.render()`), add the max-timestamp computation described in the Acceptance Criteria above, iterating `status.commentReviews` (already substantive-filtered on insert) and `status.generalComments` (filtered inline via `isSubstantiveFeedback(gc.body)`).
- Files: `session/git/worktree_git.go`

##### Task 1.1.3b: Add `HasReviewFeedback`/`LatestFeedbackAt` exported fields to `PRStatus` (~2 min)
- Add both fields to the exported section of `PRStatus` (`worktree_git.go:454`, after `ChangesRequestedCount`), each with the doc comment text from the Domain Glossary entries above.
- Files: `session/git/worktree_git.go`

##### Task 1.1.3c: Add the `## Reviewer comments` section to `render()` (~3 min)
- In `render()` (`worktree_git.go:509-521`), insert a new block after the `blockingReviews` loop and before the `generalComments` block: `if len(s.commentReviews) > 0 { sb.WriteString("## Reviewer comments\n"); for _, cr := range s.commentReviews { sb.WriteString("@" + cr.author + ": " + cr.body + "\n\n") } }`.
- Files: `session/git/worktree_git.go`

##### Task 1.1.3d: Update the `generalComments` render loop for the new element type (~2 min)
- In the existing `## PR comments` block (`worktree_git.go:516-521`), change `for _, c := range s.generalComments { sb.WriteString(c + "\n\n") }` to `for _, c := range s.generalComments { sb.WriteString("@" + c.author + ": " + c.body + "\n\n") }` — this reproduces the exact string the old code built inline at append-time (Task 1.1.2e moved the `"@"+login+": "+body` construction from append-time to render-time; net output is unchanged).
- Files: `session/git/worktree_git.go`

#### Story 1.1.4: Regression tests for the new signal and existing-behavior preservation
**As a** maintainer, **I want** table-driven tests proving the new signal works and
the three existing signals are unregressed, **so that** this ships with the same
test rigor as `HasBlockingReviews`/`CIFailing`/`HasConflicts`.
**Acceptance Criteria**:
- New tests `TestParsePRStatusPayload_HasReviewFeedback_CommentedReview`,
  `TestParsePRStatusPayload_HasReviewFeedback_PlainComment`,
  `TestParsePRStatusPayload_HasReviewFeedback_NonSubstantiveIgnored`,
  `TestParsePRStatusPayload_ReviewerCommentsSectionRendered` pass.
  - *Given* raw JSON `{"reviews":[{"state":"COMMENTED","body":"Consider extracting this into a helper function.","author":{"login":"copilot-pull-request-reviewer[bot]"},"submittedAt":"2026-08-02T14:32:07Z"}],"comments":[],"reviews":[...]}` (full fixture per Story 1.1.2), *When* `TestParsePRStatusPayload_HasReviewFeedback_CommentedReview` runs, *Then* `status.HasReviewFeedback == true` and `status.LatestFeedbackAt.Format(time.RFC3339) == "2026-08-02T14:32:07Z"`.
- All five pre-existing tests (`TestParsePRStatusPayload_ConflictDetection`,
  `_ConflictGuidanceText`, `_CIFailing`, `_HasBlockingReviews`,
  `_ConflictSectionOrderedFirst` — `worktree_git_test.go:146,229,271,312,353`) pass
  unchanged.
  - *Given* the existing `TestParsePRStatusPayload_HasBlockingReviews` fixture (a `CHANGES_REQUESTED` review), *When* run against the modified `parsePRStatusPayload`, *Then* `status.HasBlockingReviews == true` and `status.HasReviewFeedback == false` (a `CHANGES_REQUESTED` review is never counted toward the new signal — only `COMMENTED`).
- **New (adversarial-review.md BLOCKER fix)**: `TestReconcilePRPending_HasNewFeedback_UnparseableTimestampStillAdvancesWatermark` proves the `time.Now()` fallback in Task 1.1.2d cannot be masked by an existing watermark.
  - *Given* an item with `PrFeedbackAddressedAt` already set to an earlier real timestamp (e.g. `2026-08-01T10:00:00Z`, from a prior addressed-feedback cycle), and this tick's `commentReviews` contains one new substantive `COMMENTED` review whose `submittedAt` fails `time.Parse` (e.g. malformed string), *When* `parsePRStatusPayload` then `ReconcilePRPending`'s `hasNewFeedback` computation run, *Then* the fallback `at: time.Now()` is strictly after the stored watermark, so `LatestFeedbackAt.After(*item.PrFeedbackAddressedAt) == true` and `hasNewFeedback == true` — the item takes the spawn branch, not the healthy branch.
**Files**: `session/git/worktree_git_test.go`, `session/backlog_lifecycle_test.go`

##### Task 1.1.4a: Add `TestParsePRStatusPayload_HasReviewFeedback_CommentedReview` (~4 min)
- Files: `session/git/worktree_git_test.go`

##### Task 1.1.4b: Add `TestParsePRStatusPayload_HasReviewFeedback_PlainComment` (~4 min)
- Files: `session/git/worktree_git_test.go`

##### Task 1.1.4c: Add `TestParsePRStatusPayload_HasReviewFeedback_NonSubstantiveIgnored` (~4 min)
- Covers both a bare `"lgtm"` `COMMENTED` review and a bare `"lgtm"` plain comment — asserts `HasReviewFeedback == false` for both, and asserts the (non-substantive) review body is still absent from `commentReviews` while a non-substantive *plain comment* still appears in `generalComments`/`FeedbackText` (existing "include all comments" behavior preserved even though it doesn't count toward the signal).
- Files: `session/git/worktree_git_test.go`

##### Task 1.1.4d: Add `TestParsePRStatusPayload_ReviewerCommentsSectionRendered` (~3 min)
- Asserts `FeedbackText` contains `"## Reviewer comments\n"` and that it appears after `"## Review: changes requested"` when both are present, and before `"## PR comments"`.
- Files: `session/git/worktree_git_test.go`

##### Task 1.1.4e: Run the full `session/git` package test suite and confirm zero regressions (~3 min)
- `go test ./session/git/... -run TestParsePRStatusPayload -v`
- Files: none (verification task)

---

## Phase 2: Dedup Watermark Persistence

### Epic 2.1: Ent schema field + regeneration

#### Story 2.1.1: Add `pr_feedback_addressed_at` to the `BacklogItem` ent schema
**As a** the reconciler, **I want** a durable per-item watermark column, **so that**
`hasNewFeedback` can be computed against persisted state, not just this tick's `gh`
output.
**Acceptance Criteria**:
- `session/ent/schema/backlog_item.go` declares
  `field.Time("pr_feedback_addressed_at").Optional().Nillable()` with a comment
  explaining GitHub never clears `COMMENTED` reviews/comments on push (why the
  watermark exists) — mirrors `shipped_snapshot_at`'s exact `Optional().Nillable()`
  shape (`backlog_item.go:98-101`).
  - *Given* a fresh `ent.BacklogItem` row created before this migration runs, *When* the column is added and the row is re-fetched, *Then* `item.PrFeedbackAddressedAt == nil`.
- `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (the exact command from `session/ent/generate.go`) regenerates `session/ent/*` cleanly and `go build ./...` succeeds.
**Files**: `session/ent/schema/backlog_item.go`, `session/ent/*` (generated)

##### Task 2.1.1a: Add the `field.Time` declaration (~3 min)
- Insert immediately after the `shipped_snapshot_at` field block (`backlog_item.go:98-101`), before `shipped_file_stats`, with the comment text from ADR-001/Domain Glossary's `PrFeedbackAddressedAt` entry (trimmed to the essential "why", per this repo's CLAUDE.md proportionality rule — not the full research narrative).
- Files: `session/ent/schema/backlog_item.go`

##### Task 2.1.1b: Regenerate ent code (~2 min)
- Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` exactly (per `.claude/rules/ent-schema-generation.md` — omitting `--feature sql/upsert` silently breaks `UpsertRule`-style methods).
- Files: `session/ent/*` (generated — do not hand-edit)

##### Task 2.1.1c: `go build ./...` and confirm clean compile (~2 min)
- Files: none (verification task)

### Epic 2.2: Repository DTO plumbing

#### Story 2.2.1: Thread `PrFeedbackAddressedAt` through `BacklogItemData`/`BacklogItemUpdate`
**As a** `ReconcilePRPending`, **I want** the watermark readable via
`item.PrFeedbackAddressedAt` (the `*ent.BacklogItem` already used in the loop) and
writable via `UpdateBacklogItem`, **so that** the gate/persist logic in Phase 3 has
something to call.
**Acceptance Criteria**:
- `BacklogItemData.PrFeedbackAddressedAt *time.Time` mirrors `ShippedSnapshotAt`'s
  exact shape (`session/repository.go:420`).
  - *Given* `backlogItemToData(item)` where `item.PrFeedbackAddressedAt` points to `2026-08-02T14:32:07Z`, *When* called, *Then* the returned `BacklogItemData.PrFeedbackAddressedAt` points to the same time value.
- `BacklogItemUpdate.PrFeedbackAddressedAt *time.Time` follows the existing
  partial-update-presence convention (nil = untouched, non-nil = set) —
  `session/repository.go:537`.
  - *Given* `UpdateBacklogItem(ctx, itemID, BacklogItemUpdate{PrFeedbackAddressedAt: &ts}, nil)` where `ts = 2026-08-02T15:10:00Z`, *When* the item is re-fetched, *Then* `fetched.PrFeedbackAddressedAt` is non-nil and equals `ts`.
**Files**: `session/repository.go`, `session/ent_repository_backlog.go`

##### Task 2.2.1a: Add the field to `BacklogItemData` (~2 min)
- Add `PrFeedbackAddressedAt *time.Time` after `ShippedSnapshotAt` (`session/repository.go:420`), with a doc comment referencing this as the comment-feedback dedup watermark.
- Files: `session/repository.go`

##### Task 2.2.1b: Add the field to `BacklogItemUpdate` (~2 min)
- Add `PrFeedbackAddressedAt *time.Time` after `ShippedSnapshotAt` (`session/repository.go:537`), same partial-update-presence doc convention.
- Files: `session/repository.go`

##### Task 2.2.1c: Map the field in `backlogItemToData` (~2 min)
- In `backlogItemToData` (`session/ent_repository_backlog.go:170`), add `PrFeedbackAddressedAt: item.PrFeedbackAddressedAt,` after `ShippedSnapshotAt:`.
- Files: `session/ent_repository_backlog.go`

##### Task 2.2.1d: Wire the setter in `UpdateBacklogItem` (~3 min)
- In `UpdateBacklogItem` (`session/ent_repository_backlog.go:531`, near the `ShippedSnapshotAt` setter block at `:622-624`), add `if update.PrFeedbackAddressedAt != nil { u.SetPrFeedbackAddressedAt(*update.PrFeedbackAddressedAt) }`.
- Files: `session/ent_repository_backlog.go`

##### Task 2.2.1e: Add the field to `updatedFieldsFromBacklogItemUpdate`'s change-tracking list (~2 min)
- In `updatedFieldsFromBacklogItemUpdate` (`session/ent_repository_backlog.go:657`, near the `ShippedSnapshotAt` entry at `:725-727`), add `if update.PrFeedbackAddressedAt != nil { fields = append(fields, "prFeedbackAddressedAt") }`.
- Files: `session/ent_repository_backlog.go`

##### Task 2.2.1f: Add a repository-level round-trip test (~4 min)
- New test in `session/ent_repository_backlog_test.go` mirroring the existing `ShippedSnapshotAt` round-trip test (`:153-177`): create an item, assert `PrFeedbackAddressedAt` is nil, call `UpdateBacklogItem` with a timestamp, re-fetch, assert it round-trips.
- Files: `session/ent_repository_backlog_test.go`

---

## Phase 3: `ReconcilePRPending` Wiring

### Epic 3.1: Gate extension, watermark persist/clear, logging

#### Story 3.1.1: Compute `hasNewFeedback` and extend the healthy-branch gate
**As a** `ReconcilePRPending`, **I want** to treat unaddressed substantive feedback
as a fourth trigger alongside CI/reviews/conflicts, **so that** Copilot-style
`COMMENTED` reviews stop sitting unaddressed forever.
**Acceptance Criteria**:
- The gate at `backlog_lifecycle.go:4048` falls through to the spawn branch when
  `hasNewFeedback` is true, even if CI/reviews/conflicts are all healthy.
  - *Given* item `f47ac10b-58cc-4372-a567-0e02b2c3d479` with `PrNumber=152`, `PrFeedbackAddressedAt=nil`, and `prStatus = {CIFailing:false, HasBlockingReviews:false, HasConflicts:false, HasReviewFeedback:true, LatestFeedbackAt: 2026-08-02T14:32:07Z}`, *When* `ReconcilePRPending` processes this item, *Then* `hasNewFeedback == true` and the item does NOT take the "healthy" branch (`resolveStuckLogged`/`markPRReadyUnmerged` at `:4048-4079`) — it proceeds to the spawn branch at `:4101`.
  - *Given* the same item but `PrFeedbackAddressedAt = 2026-08-02T14:32:07Z` (equal to `LatestFeedbackAt`, i.e. already addressed) and all four booleans otherwise identical, *When* processed, *Then* `hasNewFeedback == false` (`.After()` on equal timestamps is false) and the item takes the healthy branch, resolving `StuckReasonPRNeedsFix` exactly as today.
**Files**: `session/backlog_lifecycle.go`

##### Task 3.1.1a: Compute `hasNewFeedback` local variable (~3 min)
- Immediately before the gate at `backlog_lifecycle.go:4048`, add:
  ```go
  hasNewFeedback := prStatus.HasReviewFeedback &&
      (item.PrFeedbackAddressedAt == nil || prStatus.LatestFeedbackAt.After(*item.PrFeedbackAddressedAt))
  ```
- Files: `session/backlog_lifecycle.go`

##### Task 3.1.1b: Extend the healthy-branch condition (~2 min)
- Change `if !prStatus.CIFailing && !prStatus.HasBlockingReviews && !prStatus.HasConflicts {` (`:4048`) to also require `&& !hasNewFeedback`.
- Files: `session/backlog_lifecycle.go`

#### Story 3.1.2: Update the dispatch log line and persist the watermark post-dispatch
**As a** an operator debugging via logs, **I want** to see which signal(s) drove a
given `ReconcilePRPending` spawn, **so that** a comment-feedback-triggered fix is
distinguishable from a CI-triggered one in the log stream.
**Acceptance Criteria**:
- The dispatch log line (`:4107-4108`) includes a fourth `feedback=%v` argument set
  to `hasNewFeedback`.
  - *Given* the first Given/When/Then in Story 3.1.1 (feedback-only trigger), *When* the log line fires, *Then* it reads `"... → in_progress for PR fix (CI=false, reviews=false, conflict=false, feedback=true)"`.
- `PrFeedbackAddressedAt` is persisted to `prStatus.LatestFeedbackAt` only when
  `remediatePRFixWithBackoffGate` returns `attempted=true, fixErr=nil` AND
  `hasNewFeedback` was true for this tick.
  - *Given* `remediatePRFixWithBackoffGate` returns `(false, nil)` (backoff not yet due), *When* the dispatch block runs, *Then* `UpdateBacklogItem` is NOT called for `PrFeedbackAddressedAt` — the watermark stays at its prior value so this feedback is retried once the backoff opens.
  - *Given* `remediatePRFixWithBackoffGate` returns `(true, nil)` for item `f47ac10b-58cc-4372-a567-0e02b2c3d479` with `prStatus.LatestFeedbackAt = 2026-08-02T14:32:07Z`, *When* the dispatch block runs, *Then* `UpdateBacklogItem` is called with `PrFeedbackAddressedAt: &(2026-08-02T14:32:07Z)`.
- **New (pre-mortem.md P1)**: when a dispatch covers >1 substantive feedback item, the
  batch-coverage log line (Observability Plan) fires with the correct count and
  author list.
  - *Given* `commentReviews = [{author:"copilot-pull-request-reviewer[bot]"}]` and `generalComments` containing one substantive entry `{author:"tstapler"}` in the same tick, *When* the dispatch block runs, *Then* the log line reports `covering 2 feedback item(s) from [copilot-pull-request-reviewer[bot], tstapler]`.
**Files**: `session/backlog_lifecycle.go`

##### Task 3.1.2a: Update the log line (~2 min)
- Change `:4107-4108` to include `feedback=%v` and pass `hasNewFeedback` as the fourth format argument.
- Files: `session/backlog_lifecycle.go`

##### Task 3.1.2b: Capture `remediatePRFixWithBackoffGate`'s `attempted` return value (~2 min)
- Change `:4109` from `if _, fixErr := l.remediatePRFixWithBackoffGate(...); fixErr != nil {` to `attempted, fixErr := l.remediatePRFixWithBackoffGate(...)`.
- Files: `session/backlog_lifecycle.go`

##### Task 3.1.2c: Persist the watermark on confirmed dispatch (~4 min)
- After the existing `if fixErr != nil { log.ErrorLog... }` block, add an `else if attempted && hasNewFeedback { ... }` branch that calls `l.storage.UpdateBacklogItem` with `PrFeedbackAddressedAt: &ts` (`ts := prStatus.LatestFeedbackAt`), logging a warning (not treating it as fatal to the tick) on update failure — matches the exact pattern in architecture.md §2b.4.
- Files: `session/backlog_lifecycle.go`

##### Task 3.1.2d: Add the new info-log line for a successful watermark persist (~2 min)
- Inside the success path of Task 3.1.2c, add the `log.InfoLog` line specified in the Observability Plan.
- Files: `session/backlog_lifecycle.go`

##### Task 3.1.2e: Add the multi-item batch-coverage log line (~3 min) — pre-mortem.md P1
- Immediately before the dispatch call, when `hasNewFeedback` is true, count substantive items (`len(commentReviews) + len(substantive generalComments)`) and, if `> 1`, log the batch-coverage line from the Observability Plan with authors joined by `", "`.
- Files: `session/backlog_lifecycle.go`

##### Task 3.1.2f: Add the batch-coverage log regression test (~4 min)
- New test asserting the log line fires with the correct count/author list for a 2-item batch, and does NOT fire for a single-item dispatch.
- Files: `session/backlog_lifecycle_test.go`

#### Story 3.1.3: Clear the watermark when a closed-without-merging PR's fields are cleared
**As a** the reconciler, **I want** a fresh PR (after a closed-without-merging cycle)
to start with a clean feedback watermark, **so that** stale feedback from a
now-defunct PR never suppresses detection on the new one.
**Acceptance Criteria**:
- The existing PR-field-clear block (`:4038-4044`) also clears
  `PrFeedbackAddressedAt`.
  - *Given* item `f47ac10b-58cc-4372-a567-0e02b2c3d479` with `PrFeedbackAddressedAt = 2026-08-02T14:32:07Z` whose PR #152 was just closed without merging and `AutoReopenForPRFix` is confirmed to have transitioned it off `pr_pending`, *When* the field-clear block runs, *Then* `UpdateBacklogItem` is called with `PrFeedbackAddressedAt: nil` (a non-nil pointer to the zero value is NOT what's wanted — see Task detail) alongside the existing `PrURL`/`PrNumber` clears.
**Files**: `session/backlog_lifecycle.go`

##### Task 3.1.3a: Extend the field-clear `UpdateBacklogItem` call (~3 min)
- At `:4038-4044`, the update struct needs a way to explicitly set `PrFeedbackAddressedAt` back to nil while still following the "nil pointer = untouched" convention used everywhere else in `BacklogItemUpdate`. Since a plain `*time.Time` field can't distinguish "clear it" from "don't touch it" using only `nil`, add a sibling `ClearPrFeedbackAddressedAt bool` field to `BacklogItemUpdate` (documented as a one-off, mirroring the `ClearReworkCapOverride` pattern the repository.go comment at `:544-547` already anticipates for exactly this "no way to explicitly clear" gap) and honor it in `UpdateBacklogItem`'s setter (`u.ClearPrFeedbackAddressedAt()`, ent's generated nil-setter method).
- Files: `session/repository.go`, `session/ent_repository_backlog.go`, `session/backlog_lifecycle.go`

##### Task 3.1.3b: Add a regression test for the clear-on-closed-PR path (~4 min)
- Extend or add alongside the existing closed-without-merging tests in `session/backlog_lifecycle_test.go` — assert `PrFeedbackAddressedAt` is nil after a closed-without-merging cycle that previously had a non-nil watermark.
- Files: `session/backlog_lifecycle_test.go`

---

## Phase 4: `STUCK_REASON_PR_NEEDS_FIX` Proto Enum Gap

### Epic 4.1: Proto + Go mapping

#### Story 4.1.1: Add the missing enum value and wire both Go switches
**As a** repo owner viewing `/unfinished`, **I want** `pr_needs_fix` items to show a
real reason instead of "Unknown reason", **so that** "Retry now" works and this
project's reuse-first design isn't shipping a broken UI path.
**Acceptance Criteria**:
- `proto/session/v1/backlog.proto`'s `StuckReason` enum gains
  `STUCK_REASON_PR_NEEDS_FIX = 13;` (next available value after `12`).
  - *Given* `make proto-gen` has run, *When* Go code references `sessionv1.StuckReason_STUCK_REASON_PR_NEEDS_FIX`, *Then* it compiles and equals `13`.
- `toProtoStuckReason`/`fromProtoStuckReason` (`server/services/backlog_service_stuck.go:28`, `:63`) each gain a `case` for `domain.StuckReasonPRNeedsFix` ↔ `sessionv1.StuckReason_STUCK_REASON_PR_NEEDS_FIX`.
  - *Given* a `BacklogStuckState` row with `reason = "pr_needs_fix"`, *When* `toProtoStuckReason(domain.StuckReasonPRNeedsFix)` is called, *Then* it returns `sessionv1.StuckReason_STUCK_REASON_PR_NEEDS_FIX`, not `STUCK_REASON_UNSPECIFIED`.
  - *Given* `fromProtoStuckReason(sessionv1.StuckReason_STUCK_REASON_PR_NEEDS_FIX)`, *When* called, *Then* it returns `domain.StuckReasonPRNeedsFix`, and `domain.StuckReasonPRNeedsFix.IsValid() == true`.
**Files**: `proto/session/v1/backlog.proto`, `server/services/backlog_service_stuck.go`

##### Task 4.1.1a: Add the proto enum value (~2 min)
- Add `STUCK_REASON_PR_NEEDS_FIX = 13;` after `STUCK_REASON_REWORK_BLOCKED_STALE = 12;` (`proto/session/v1/backlog.proto:1017`), with a doc comment mirroring the `REWORK_BLOCKED_STALE` comment style, referencing `domain.StuckReasonPRNeedsFix`.
- Files: `proto/session/v1/backlog.proto`

##### Task 4.1.1b: Run `make proto-gen` (~2 min)
- Regenerates `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`.
- Files: `session/gen/session/v1/*` (generated), `web-app/src/gen/session/v1/*` (generated)

##### Task 4.1.1c: Add the `toProtoStuckReason` case (~2 min)
- Add `case domain.StuckReasonPRNeedsFix: return sessionv1.StuckReason_STUCK_REASON_PR_NEEDS_FIX` before the `default:` in `toProtoStuckReason` (`backlog_service_stuck.go:28-56`).
- Files: `server/services/backlog_service_stuck.go`

##### Task 4.1.1d: Add the `fromProtoStuckReason` case (~2 min)
- Add `case sessionv1.StuckReason_STUCK_REASON_PR_NEEDS_FIX: return domain.StuckReasonPRNeedsFix` before the `default:` in `fromProtoStuckReason` (`backlog_service_stuck.go:63-91`).
- Files: `server/services/backlog_service_stuck.go`

##### Task 4.1.1e: Add Go round-trip tests for both mapping functions (~4 min)
- New test(s) in `server/services/backlog_service_stuck_test.go` (or the existing test file covering this pair, if one exists — search `grep -n "toProtoStuckReason\|fromProtoStuckReason" server/services/*_test.go` first): assert `toProtoStuckReason(domain.StuckReasonPRNeedsFix) == sessionv1.StuckReason_STUCK_REASON_PR_NEEDS_FIX` and the reverse.
- Files: `server/services/backlog_service_stuck_test.go`

### Epic 4.2: Frontend Record maps

#### Story 4.2.1: Register the new reason in the three `Record<StuckReason, T>` maps and the CSS chip class
**As a** repo owner, **I want** the `/unfinished` chip to render a real label/icon/
color for `pr_needs_fix` items, **so that** the TypeScript compiler (not a runtime
fallback) enforces every `StuckReason` has coverage — the documented reason this
repo uses `Record` types here instead of a lookup-with-fallback function.
**Acceptance Criteria**:
- `STUCK_REASON_LABELS`, `STUCK_REASON_ICONS`, `STUCK_REASON_CLASS` (`web-app/src/components/backlog-stuck/stuckReason.ts:15,32,49`) each gain a `[StuckReason.PR_NEEDS_FIX]` entry; `stuckReason.css.ts` gains a `chipPrNeedsFix` style.
  - *Given* a `StuckBacklogItem` with `reason = StuckReason.PR_NEEDS_FIX`, *When* `getStuckReasonLabel(reason)` is called, *Then* it returns a real string (e.g. `"PR needs attention"`), not `"Unknown reason"`.
  - *Given* the same, *When* the component renders, *Then* TypeScript compiles cleanly (the `Record<StuckReason, T>` type would otherwise fail to compile with a missing key, per the file's own header comment — this IS the test).
**Files**: `web-app/src/components/backlog-stuck/stuckReason.ts`, `web-app/src/components/backlog-stuck/stuckReason.css.ts`

##### Task 4.2.1a: Add `chipPrNeedsFix` to `stuckReason.css.ts` (~3 min)
- Mirror `chipAbandonedReview`'s shape (`stuckReason.css.ts:26-32`) using `vars.color.warningBg`/`warningText`/`warning` (a comment-feedback nudge is a "needs attention," not "hard failure," severity — same tier as `AbandonedReview`, distinct from the `errorBg` tier used by `chipReworkCap`/`chipBouncing`).
- Files: `web-app/src/components/backlog-stuck/stuckReason.css.ts`

##### Task 4.2.1b: Add the three `Record` entries (~3 min)
- `STUCK_REASON_LABELS[StuckReason.PR_NEEDS_FIX] = "PR needs attention"`,
  `STUCK_REASON_ICONS[StuckReason.PR_NEEDS_FIX] = "🟡"`,
  `STUCK_REASON_CLASS[StuckReason.PR_NEEDS_FIX] = styles.chipPrNeedsFix`.
- Files: `web-app/src/components/backlog-stuck/stuckReason.ts`

##### Task 4.2.1c: `cd web-app && npx tsc --noEmit` to confirm the `Record` types compile (~2 min)
- Files: none (verification task)

---

## Phase 5: Copilot Review Request Wiring

### Epic 5.1: `RequestCopilotReview` method and ship-flow wiring

#### Story 5.1.1: Add `RequestCopilotReview` and call it best-effort after `EnablePRAutoMerge`
**As a** the ship pipeline, **I want** every PR it opens to get a Copilot review
requested automatically, **so that** requirements.md's "PRs opened by the ship
pipeline get a Copilot review requested automatically" success metric is met without
a manual step.
**Acceptance Criteria**:
- `(*GitWorktree) RequestCopilotReview(prNumber int) error` runs
  `gh pr edit <prNumber> --add-reviewer copilot-pull-request-reviewer[bot]`,
  best-effort (mirrors `EnablePRAutoMerge`'s exact shape at `worktree_git.go:650`).
  - *Given* PR #152 in a repo where Copilot code review is enabled, *When* `RequestCopilotReview(152)` is called, *Then* it runs `gh pr edit 152 --add-reviewer copilot-pull-request-reviewer[bot]` and returns `nil` on a zero exit code.
  - *Given* PR #152 in a repo where Copilot code review is NOT enabled for the org, *When* called, *Then* `gh` exits non-zero and the method returns a wrapped error — the caller (Task 5.1.1c) must NOT fail PR creation on this error.
- `pushAndCreatePR` calls it once, right after the existing `EnablePRAutoMerge` call
  (`backlog_lifecycle.go:3304-3314`), logging a warning + sending a notification on
  failure (same pattern as `EnablePRAutoMerge`'s own failure handling immediately
  above it).
  - *Given* `RequestCopilotReview` returns an error for item `f47ac10b-58cc-4372-a567-0e02b2c3d479`'s PR #152, *When* `pushAndCreatePR` continues, *Then* the item still transitions to `pr_pending` (unaffected) and a `NOTIFICATION_TYPE_WARNING` notification is sent, matching the auto-merge-failure notification's severity/priority.
**Files**: `session/git/worktree_git.go`, `session/backlog_lifecycle.go`

##### Task 5.1.1a: Add `RequestCopilotReview` method (~4 min)
- Add directly after `EnablePRAutoMerge` (`worktree_git.go:650-...`), following its exact structure: `checkGHCLI()`, 30s-timeout context, `safeexec.CommandContext(ctx, "gh", "pr", "edit", strconv.Itoa(prNumber), "--add-reviewer", "copilot-pull-request-reviewer[bot]")`, `cmd.Dir = g.worktreePath`, wrapped error on failure. Doc comment explains the unconditional legacy-login choice (Pattern Decisions table) in one sentence — not the full gh-version research narrative.
- Files: `session/git/worktree_git.go`

##### Task 5.1.1b: Add `RequestCopilotReview(prNumber int) error` to the `prCreator` interface (~2 min)
- Add after `EnablePRAutoMerge(prNumber int) error` (`backlog_lifecycle.go:239`).
- Files: `session/backlog_lifecycle.go`

##### Task 5.1.1c: Call it in `pushAndCreatePR` after `EnablePRAutoMerge` (~4 min)
- After the `EnablePRAutoMerge` success/failure block (`backlog_lifecycle.go:3304-3314`), add the equivalent `if reviewErr := g.RequestCopilotReview(prNumber); reviewErr != nil { log.WarningLog...; l.notify(...) } else { log.InfoLog... }` block, using `NotificationType_NOTIFICATION_TYPE_WARNING` / `NotificationPriority_NOTIFICATION_PRIORITY_LOW` (lower priority than auto-merge failure — a missing Copilot review is a missed nicety, not a missed automatic-merge path).
- Files: `session/backlog_lifecycle.go`

##### Task 5.1.1d: Add/update the mock `prCreator` implementations used by existing tests (~4 min)
- `grep -rn "EnablePRAutoMerge" session/backlog_lifecycle_test.go` to find every test double implementing `prCreator`; add a `RequestCopilotReview(prNumber int) error` method (return `nil` by default) to each so the interface addition doesn't break compilation.
- Files: `session/backlog_lifecycle_test.go`

##### Task 5.1.1e: Add a unit test for `pushAndCreatePR`'s best-effort handling of a `RequestCopilotReview` failure (~4 min)
- Configure the mock `prCreator` to return an error from `RequestCopilotReview`; assert the item still transitions to `pr_pending` and a notification is sent — mirrors whatever existing test covers `EnablePRAutoMerge`'s failure path.
- Files: `session/backlog_lifecycle_test.go`

#### Story 5.1.2: Real dry-run verification against an actual PR (not sandboxed)
**As a** the implementer, **I want** to confirm `gh pr edit --add-reviewer` actually
mutates a real PR's reviewer list, **so that** this doesn't ship on the strength of
the sandboxed research environment's unverified stub response (stack.md §1 flagged
this explicitly: a mutating `gh pr edit` call there returned a suspicious `ok
edited`-shaped response without demonstrably touching `reviewRequests`).
**Acceptance Criteria**:
- Against a real, disposable test PR in a repo the operator controls (not the
  research sandbox), `gh pr edit <n> --add-reviewer copilot-pull-request-reviewer[bot]`
  is run, and `gh pr view <n> --json reviewRequests` is checked BEFORE and AFTER to
  confirm the reviewer list actually changed.
  - *Given* a scratch PR with `reviewRequests: []` before the call, *When* `gh pr edit <n> --add-reviewer copilot-pull-request-reviewer[bot]` runs and `gh pr view <n> --json reviewRequests` is re-checked, *Then* the after-state includes an entry for Copilot (or, if the org doesn't have Copilot code review enabled, the command's non-zero exit / error message is captured verbatim as evidence of the actual failure mode, not assumed).
**Files**: none (manual verification task — no code changes; the outcome informs whether Task 5.1.1a's error handling needs adjustment)

##### Task 5.1.2a: Run the real dry-run against a scratch PR in this repo or a throwaway repo (~5 min)
- Document the exact before/after `gh pr view --json reviewRequests` output (paste into the PR description or a code-review comment when this work ships) — this is the evidence the sandboxed research environment could not produce.
- Files: none

##### Task 5.1.2b: If Copilot code review is not enabled on the target org/repo, capture the exact `gh` error text and confirm Task 5.1.1a's error wrapping surfaces it usefully in logs (~3 min)
- Files: none (may produce a follow-up task if the error text needs better wrapping — not expected, but this is the check)

---

## Phase 6: Regression Validation

### Epic 6.1: Full-suite confirmation

#### Story 6.1.1: Confirm zero regressions across the full affected test surface
**As a** the implementer, **I want** every existing `CIFailing`/`HasBlockingReviews`/
`HasConflicts` test and every `ReconcilePRPending` test to still pass unchanged,
**so that** requirements.md's "Zero regressions" success metric is met with evidence,
not assertion.
**Acceptance Criteria**:
- `go test ./session/... ./server/... -count=1` passes with zero failures.
  - *Given* all Phase 1-5 changes applied, *When* `make build && make test` runs, *Then* the exit code is 0 and no test in `worktree_git_test.go`, `backlog_lifecycle_test.go`, `ent_repository_backlog_test.go`, or `backlog_service_stuck_test.go` fails.
- `make lint` passes.
**Files**: none (verification task)

##### Task 6.1.1a: `make build && make test` (~5 min, longer-running)
- Files: none

##### Task 6.1.1b: `make lint` (~3 min)
- Files: none

##### Task 6.1.1c: `cd web-app && npx jest --no-coverage --testPathPatterns="stuckReason"` (if any test file targets it) or confirm via `npx tsc --noEmit` only, since `stuckReason.ts` has no dedicated test file today (~2 min)
- `grep -rn "stuckReason" web-app/src/components/backlog-stuck/*.test.*` first to confirm whether a test file exists; if not, `npx tsc --noEmit` (Task 4.2.1c) is the only frontend gate for this specific file.
- Files: none
