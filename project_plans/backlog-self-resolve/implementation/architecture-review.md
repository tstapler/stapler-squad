# Architecture Review: backlog-self-resolve
**Date**: 2026-08-02 (re-review of blocker fixes)
**Verdict**: CONCERNS — both original blockers (this file's substring-match hazard, and
adversarial-review.md's FR10 detector mis-citation) are confirmed RESOLVED. No blocker remains
open. Several residual concerns from the fixes themselves, and from three independently-verified
secondary claims, should still be tightened before implementation starts.

## Constitution Violations
N/A — `docs/adr/ADR-000-architecture-constitution.md` does not exist anywhere in this
repository (confirmed via `find . -iname "*constitution*"`, no hits). No constitution
constraints apply to this review.

## Blockers

### 1. Story 3.1.2 / Task 3.1.2b — substring false-positive — RESOLVED, with a residual internal inconsistency
The original blocker: `strings.Contains(itemSession.VerificationNotes, "duplicate_ref="+duplicateRef)`
would misclassify `.../pull/27` as already-recorded when the stored notes actually contain
`.../pull/272` (a false positive from raw substring containment).

**Verified fix**: plan.md:290 now instructs a delimited match —
`strings.HasPrefix(line, notesMarker+" ")` against each line of `VerificationNotes` split on
`"\n"`. Checked character-by-character: for `notesMarker = "duplicate_ref=…/pull/27"`, the
compared prefix is `"…/pull/27 "` (trailing space). A stored line for `…/pull/272` reads
`"duplicate_ref=…/pull/272 reason=…"` — the character immediately following `"…/pull/27"` in that
line is `"2"`, not a space, so `HasPrefix` correctly returns `false`. The original false-positive
hazard is closed by this variant.

**Residual issue (new)**: the task presents this as one of *two* alternatives, and the other one
is broken. The primary sentence says "split `VerificationNotes` on `"\n"` and compare each line
with `==` against `notesMarker`" — but `notesMarker` itself never includes the ` reason=…` suffix,
and the real persisted format (Task 3.3.2a, plan.md:350) is *always*
`fmt.Sprintf("duplicate_ref=%s reason=%s", duplicateRef, reason)` — `reason` is a required
argument (plan.md:269), so no real stored line is ever the bare marker with nothing after it. An
implementer who builds the plain `==` variant (presented first, and not flagged as non-functional)
gets an idempotency check that **never fires** — every retry, including a genuine identical
repeat, falls through to the whitelist check, and once the item has already moved to `review`
(outside the whitelist) a legitimate retry is wrongly refused as "disallowed source status"
instead of returned as the intended no-op success (contradicting the GWT at plan.md:281). This
is a different defect class from the original blocker (an availability/correctness regression
via dead idempotency, not a false-positive collision), and it would very likely be caught by
Task 4.2.4's `TestReportDuplicate_NoOpOnExactRetry` test during implementation — but the plan
text itself should not present a non-functional option as viable. **Recommendation**: delete the
`==`-comparison sentence and keep only the `HasPrefix(line, notesMarker+" ")` version as the one
required implementation.

### 2. FR10 stuck-item detector citation (adversarial-review.md) — RESOLVED
The original blocker: the plan cited `pr_pending_no_pr`/`reconcilePRPendingWithoutPRItems`, which
is gated `if item.PrNumber != 0 { continue }` — structurally the wrong detector for FR10's actual
scenario (an item that already has a real PR reference, e.g. #281, and got stuck because a later
`report_duplicate` verification call failed).

**Verified independently** by reading `ReconcilePRPending` in full
(`session/backlog_lifecycle.go:3850-4113`, not just the plan's restatement). It is gated
`if item.PrNumber == 0 || item.PrURL == "" { continue }` (line 3857) — the complementary
condition, i.e. it only processes `pr_pending` items *with* a real PR reference, which is the
right shape. Tracing every terminal branch for such an item:

| PR's real GitHub state | Branch | Stuck reason marked |
|---|---|---|
| Merged | `l.storage.TransitionBacklogItemStatus(… BacklogStatusDone …)` (line 3922) | N/A — item leaves `pr_pending` entirely |
| Closed without merging | `remediatePRFixWithBackoffGate` (line 3998) | `StuckReasonPRNeedsFix` — `er.MarkStuck(…)` is called **unconditionally** on entry to that function (line 3805), before any backoff gating, so this fires on the same tick the closure is detected, not contingent on a fix attempt succeeding |
| Open, healthy (CI green, no blocking reviews, no conflicts), solo-mergeable | `markPRReadyUnmerged` (line 4069, gated by `prReadyToMergeSolo`) | `StuckReasonPRReadyUnmerged` — `er.MarkStuck(…)` (line 4208) fires unconditionally once solo-ready-mergeable; only the *notification* (line 4223) is threshold-gated, not the stuck-state row itself |
| Open, CI-failing / blocking reviews / merge conflict | `remediatePRFixWithBackoffGate` (line 4109) | `StuckReasonPRNeedsFix`, same unconditional `MarkStuck` as the closed-PR path |

Because `report_duplicate`'s own verification failure never touches the underlying PR's actual
GitHub state, `ReconcilePRPending` classifies the item purely by that PR's real health — one of
the four branches above will fire regardless of *why* the item is sitting at `pr_pending`. The
corrected citation (`ReconcilePRPending` → `StuckReasonPRReadyUnmerged`/`StuckReasonPRNeedsFix`)
holds up for FR10's actual scenario.

**Residual gap (new, non-blocking)**: the plan's phrasing "this detector suite runs
unconditionally for every `pr_pending` item with a real PR reference, regardless of *why* the
item is sitting there" (Observability Plan, plan.md:83) overstates it. Two early `continue` paths
exist that skip the tick without marking anything stuck: `IsPRMerged` returning an error (line
3868-3871) and `GetPRStatus` returning an error (line 3956-3958). If either GitHub call fails
persistently (revoked token, sustained API outage, the item's `RepoPath` being empty at line
3861-3863), the item would sit at `pr_pending` indefinitely with neither stuck reason ever set —
regardless of whether `report_duplicate` was ever involved. This is a pre-existing characteristic
of `ReconcilePRPending`, not something this plan introduces or worsens, and it's orthogonal to
FR10's specific scenario (which doesn't touch the PR's own GitHub calls at all) — not a blocker,
but the Observability Plan's "runs unconditionally" wording should be softened to acknowledge it.
(Separately checked: `Mergeable == "UNKNOWN"`, GitHub's transient not-yet-computed mergeability
state, is *not* a gap — `session/git/worktree_git.go:592-594`'s comment confirms this is
deliberately treated as "no signal this cycle" and self-resolves on a later poll tick, since
neither the `DIRTY`/`CONFLICTING` check nor `prReadyToMergeSolo`'s `MERGEABLE` check matches
`UNKNOWN`, so the item is simply re-evaluated next tick rather than misclassified.)

## Concerns

- [x] **RESOLVED, re-verified.** Story 3.2.2's narrative line (plan.md:311) now reads
  "returns a single `error` whose classification is `errors.Is`-checkable (a different shape
  than `verifyPR`'s `(bool, error)`...)" — consistent with the Domain Glossary and Task 3.2.2a.

- ~~Story 3.2.2's narrative line still states the old, incorrect `(bool, error)`-shaped
  contract, contradicting the corrected Domain Glossary/Task text three lines away.** Checked
  for the "consistent single-`error`-return contract" claim across all four places the plan
  discusses `verifyGitHubRefExists`: the Domain Glossary (plan.md:52) and Task 3.2.2a's code
  sketch (plan.md:317) and its GWTs (plan.md:312-313, explicitly "single-return, not
  `(false, nil)`") were all corrected and are self-consistent. But Story 3.2.2's own "As…I
  want" framing sentence (plan.md:310) was missed: "I want one function that dispatches to
  `GetPR`/`GetIssue`/`GetCommit` by ref type and **returns the same 3-way contract `verifyPR`
  already uses**" — this is the exact claim the adversarial review flagged as inaccurate (and
  the architecture-review.md original text already noted `verifyPR` genuinely differs in shape
  from a single-error-return dispatcher). An implementer skimming only story-level framing
  (rather than the corrected task-level detail) would build to the wrong signature. Low risk
  given the surrounding text is correct, but the line should be fixed for consistency —
  something like "returns a single `error`, disambiguated via `errors.Is` — a narrower contract
  than `verifyPR`'s `(bool, error)`."

- [x] **Epic 2.2 / Task 2.2.1a — `hasActiveReviewSession` export — RESOLVED, verified this pass.**
  plan.md:54 and Task 2.2.1a (plan.md:247) now correctly state `server/mcp` already imports
  `server/services` and export+reuse `HasActiveReviewSession` instead of adding a local copy.
  Independently confirmed via the actual import statements: `server/mcp/server.go:14`,
  `server/mcp/tools_github.go:16`, and `server/mcp/tools_lifecycle.go:13` all import
  `"github.com/tstapler/stapler-squad/server/services"`; a repo-wide grep for
  `"github.com/tstapler/stapler-squad/server/mcp"` inside `server/services/*.go` returns zero
  import hits (the one earlier textual match was a comment, not an import) — no cycle in either
  direction. Task 3.3.3a (plan.md:361) also correctly reuses the same exported helper rather than
  re-deriving it. No further action needed.

- [x] **Epic 3.2 (`verifyGitHubRefExists`) testability seam — RESOLVED, verified this pass.**
  Task 3.2.2a (plan.md:317) now adds the injectable `verifyGitHubRef func(ctx, ref) error` field
  on `backlogHandlers`, mirroring `verifyPRMatchesBranch`'s existing shape, and the dispatcher
  call site (plan.md:321) calls `h.verifyGitHubRef(...)` through that field rather than the
  package-level functions directly. Task 4.2.1a (plan.md:450) confirms MCP-layer tests override
  this field the same way `report_pr_created`'s tests already override `verifyPRMatchesBranch`.
  Both GitHub-verification paths in the file now share one mocking idiom.

- [x] **Task 1.2.3b/1.2.3c nonexistent-file citation — RESOLVED, verified this pass (found
  incidentally while checking the two named secondary claims; not one of the three explicitly
  requested, but the fix is visible in the same section and worth recording accurately rather
  than repeating a stale concern).** Task 1.2.3b (plan.md:191) now correctly cites
  `server/services/backlog_github_rpc_test.go:19-22`'s `resetGhBaseURL` helper instead of the
  nonexistent `github/repos_test.go`/`client_test.go`, and explicitly notes this is the *first*
  httptest-based test inside the `github` package itself rather than a "reuse an existing one in
  this package" situation. Task 1.2.3c (plan.md:195) mirrors the same correction.

## Nitpicks

- Task 4.2.6a's test name, `TestReportDuplicate_LoserGetsDistinctMessage_WhenRacingReportPRCreated`
  (plan.md:504), is stale relative to its own corrected description immediately below it
  (plan.md:501, 505): the story text explicitly disclaims "not a race" and identifies the actual
  losing call as a *second* `reportDuplicate` invocation, not one racing `reportPRCreated`. The
  rewritten test construction itself is internally coherent and testable exactly as described
  (two real tool calls composing legitimately, then a third disallowed call cleanly refused) —
  the acceptance-criteria fix the adversarial review asked for is genuinely done — but the test
  name should be renamed to something like
  `TestReportDuplicate_RefusedAfterAlreadyTransitionedToReview` so a future reader isn't misled
  by a name that contradicts the test's own narrative.
- `allowedSelfResolveSourceStatuses` (Task 2.1.1a) as a package-level `map[session.BacklogStatus]bool`
  is fine as designed — it's a read-only lookup table, never mutated after init, the same idiom
  as any Go dispatch/allow-list map; it is not the "package-level mutable-looking state" the
  plan's own doc-comment worries about. A small predicate function
  (`isAllowedSelfResolveSource(status session.BacklogStatus) bool`) would encapsulate it slightly
  better and centralize the two call sites' error-message string, but this is optional polish,
  not a defect.
- `BacklogItemPrecondition.Note` being populated by `request_review`/`report_duplicate` for the
  first time is **not** a new layering violation — the field's existing doc comment
  (`session/repository.go:556-558`) already documents exactly this use ("record why the
  transition happened, e.g. 'auto-reopened after FAIL verdict'"). The plan is the first caller
  to exercise a pre-existing, intentionally-general field as designed, not introducing an
  MCP-formatted string into a layer that wasn't expecting one.
- `github.GetPR` (new, existence-only) and `github.GetPRInfoCtx` (existing, rich `gh`-CLI-shaped
  detail) will coexist with similar names but different shapes/purposes. ADR-002 already flags
  and accepts this as a negative consequence; agreed it's not worth blocking on, but a one-line
  doc-comment cross-reference on each function pointing at the other would save a future reader
  a grep.
- `session.ItemSessionSummary.Role` is a plain `string` (not a typed `SessionRole` sum type),
  so `SessionRoleWork`/`SessionRoleReview` are untyped string constants rather than a Go
  sum-type-style enum (unlike `BacklogStatus`, which *is* a proper `type BacklogStatus string`
  newtype). This is pre-existing primitive obsession, unrelated to this plan's diff, and out of
  scope to fix here — noted for a future cleanup pass, not this feature.
- ADR-003's own stated risk-mitigation step ("grep check before merging:
  `grep -rn "TriggeredBySystem" web-app/src session server`") was run as part of this review:
  every hit is a *write* site (a call passing `TriggeredBySystem` as an argument) or a doc
  comment; no hit is a *read-side filter* comparing a stored `TriggeredBy` value against
  `"system"` to select behavior. The ADR's risk assessment is confirmed low as written — just
  flagging that Phase 5's implementer should still be the one to actually run this check (not
  rely on this review substituting for it), since new code between now and implementation could
  change the picture.
