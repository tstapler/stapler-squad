# Architecture Review: pr-review-followup
**Date**: 2026-08-02
**Verdict**: CONCERNS

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repo (verified:
`test -f` returns MISSING). No constitution constraints apply — this section is N/A,
no violations to list.

No BLOCKER-level findings. Four CONCERNs worth resolving before/during implementation
(none individually invalidates the plan's core design), plus forward-looking NITPICKs.

---

## Blockers

None.

---

## Concerns

- [ ] **Story 1.1.2 / Task 1.1.2d — illegal state: `HasReviewFeedback == true` with a
  zero-value `at` is reachable, and directly contradicts the plan's own cited research
  constraint.** Task 1.1.2d's own text: "parse `r.SubmittedAt` ... (log+skip the
  timestamp on parse error, matching the 'fail loudly, don't silently zero-value'
  concern from stack.md/pitfalls.md §4 — but still append with a zero `at` rather than
  dropping the review entirely...)". This does the opposite of what it cites:
  `pitfalls.md` §4 explicitly says "any new fields parsed from that payload should fail
  loudly (return an error from `parsePRStatusPayload`) ... rather than silently
  zero-valuing" — and the task silently zero-values `at` anyway. Story 1.1.3's
  acceptance criteria and Domain Glossary's `LatestFeedbackAt` doc comment
  ("zero value when `HasReviewFeedback` is false") implicitly assume every substantive
  entry carries a real timestamp; Task 1.1.2d creates the one code path that breaks
  that assumption. Functional blast radius is narrower than it looks (a zero timestamp
  is the theoretical minimum, so `.After()` comparisons downstream mostly self-correct
  rather than runaway-retrigger), but: (a) it produces a nonsensical log line — the
  Observability Plan's `"PrFeedbackAddressedAt advanced to %s"` would print
  `0001-01-01T00:00:00Z` — and (b) Story 1.1.4's four new test names
  (`_CommentedReview`, `_PlainComment`, `_NonSubstantiveIgnored`,
  `_ReviewerCommentsSectionRendered`) don't cover "substantive review with an
  unparseable `submittedAt`" at all, so this code path ships untested.
  **Remediation**: either (a) treat an unparseable timestamp as non-substantive for
  the purposes of `HasReviewFeedback`/`commentReviews` (skip it, matching the
  `isSubstantiveFeedback` filter's existing "drop it from the signal" behavior — safer
  and consistent with the cited pitfalls.md guidance), or (b) genuinely fail loudly
  (return an error from `parsePRStatusPayload` on parse failure) as pitfalls.md
  recommends. Either way, add the missing test case to Story 1.1.4 exercising this
  exact path.

- [ ] **Task 3.1.3a — the `ClearPrFeedbackAddressedAt bool` sibling-field pattern is a
  second instance of a known anti-pattern, and the plan cites the first instance as
  precedent instead of treating it as a signal to fix the root cause.**
  `session/repository.go:544-547`'s existing comment on `ReworkCapOverride` already
  anticipates this exact gap ("There is currently no way to explicitly clear ... a
  deliberate simplification; add a `ClearReworkCapOverride` bool alongside this if
  that's needed later" — verified by reading `session/repository.go:520-547`). Task
  3.1.3a follows that same advice literally, which means `BacklogItemUpdate` is now
  two `Clear*` bools away from needing a third, fourth, etc. every time a future
  nullable field needs explicit-clear semantics — `*T` alone can't distinguish
  {untouched, clear, set-to-value} and never will, so this will keep recurring. This
  is exactly the type-driven-design "illegal states via primitive obsession" pattern:
  a raw `*time.Time` plus a same-named `bool` sibling is two independent fields
  standing in for one three-state sum type, and nothing stops a caller from setting
  both a non-nil value *and* the clear flag simultaneously (an unrepresented-as-illegal
  combination). **Remediation**: not a blocker for this Small-appetite plan — but flag
  explicitly in the plan (a one-line note under Task 3.1.3a is enough) that this is the
  second occurrence of the pattern, and file a follow-up to introduce a generic
  three-state field-update type (e.g. `type FieldUpdate[T any] struct { Op FieldOp; Value T }`
  with `Op ∈ {Untouched, Clear, Set}`) to replace both `ReworkCapOverride`/
  `ClearReworkCapOverride` and this new pair once a third case appears — "rule of
  three" has now been hit at count two, which is the right moment to name the debt
  even if not the moment to pay it off.

- [ ] **Story 3.1.2 / Task 3.1.2c — the watermark write is not atomic with the
  confirmed-dispatch decision, reproducing the narrow crash-window risk pitfalls.md
  §1(c) named explicitly.** `remediatePRFixWithBackoffGate` records the remediation
  attempt (`RecordRemediationAttempt`) as part of returning `attempted=true` (per
  pitfalls.md's own citation of `backlog_remediation.go:168-193`). Task 3.1.2c's
  `UpdateBacklogItem(..., PrFeedbackAddressedAt: &ts)` call happens as a **separate**,
  subsequent write after that return. Pitfalls.md §1(c) names this precise shape as
  the residual risk: "if a dedup-marker write is implemented as a separate step from
  the gate/spawn decision ... a crash/restart ... between 'gate says due' and 'marker
  persisted' could cause the marker to never advance even though an attempt (and
  budget) was consumed" — and prescribes "persist ... transactionally with the same
  write that records the fix-spawn attempt." For the three existing (self-clearing)
  triggers this class of gap self-heals for free once GitHub-side state flips back to
  healthy; `HasReviewFeedback` is explicitly *not* self-clearing (that's this whole
  feature's premise), so a lost watermark write here means the next backoff-due tick
  re-spawns a fix session for feedback a prior session already addressed — burning a
  `maxAutoReworkIterations` slot on stale content, i.e. a narrower instance of the
  exact failure mode (pitfalls.md §1(a)) this feature exists to prevent. Bounded to a
  crash/restart landing in the gap between two sequential statements, so low
  probability, but the plan doesn't acknowledge the gap exists (Story 3.1.2's
  acceptance criteria describe the happy path only). **Remediation**: at minimum,
  document this as an accepted residual risk (mirroring how ADR-001 documents the
  "10 comments, 1 addressed" coarseness) rather than leaving it unstated; if
  inexpensive, fold the watermark write into the same DB transaction/call
  `remediatePRFixWithBackoffGate`'s attempt-recording uses, rather than a follow-on
  `UpdateBacklogItem`.

- [ ] **ADR-001 Consequences — the "competent fix session addresses the batch
  together" mitigation for the admitted coarse-grain limitation describes a hope, not
  a mitigation, and should be labeled as such.** The underlying architectural
  decision (timestamp watermark over ID-based dedup) is sound — the clock-skew
  rebuttal holds (both sides of `.After()` are GitHub-issued timestamps, verified by
  reading the actual comparison in Task 3.1.1a), and the "at most one open PR per
  item" argument correctly disqualifies the multi-entity collision problem ID-based
  dedup solves. But the ADR's own admitted weakness (10 comments arrive, 1 addressed,
  watermark advances past all 10 regardless) is not actually reduced in probability by
  "the single fix session's context includes every comment's full body" — an LLM
  agent given ten review comments in one context has no structural guarantee it
  addresses all ten, and nothing downstream re-verifies which ones it actually
  touched. Note this is *not* unique to the timestamp-watermark choice — a naive
  ID-based dedup that marks all N ids "responded to" after one dispatch (the design
  pitfalls.md/build-vs-buy.md actually describe) has the identical gap, since neither
  design re-checks per-item resolution. So this isn't a reason to prefer ID-based
  dedup after all — but ADR-001 currently presents "the fix session addresses the
  batch together" as a *mitigating factor*, which overstates confidence in something
  that's actually just an assumption about agent behavior. **Remediation**: reword
  ADR-001's Consequences section to state this as an accepted, unverified residual
  risk rather than a mitigation — one sentence: "no structural mechanism confirms a
  dispatched fix session addressed every item in a batch; this is accepted as
  consistent with the shared-cap/no-per-comment-triage decisions already made in
  Scope." No design change required.

---

## Nitpicks

- `ReconcilePRPending` (`session/backlog_lifecycle.go:3850-4113`, 263 lines before this
  plan's changes) keeps growing by Transaction-Script accretion — this plan adds a
  fourth `&&`/boolean term, following the same shape as the prior three additions.
  The plan's own "System Type Confirmation" section correctly judges this increment as
  not yet warranting a Command/Policy-object refactor, and I agree for this specific
  increment — but flagging that the function is now large enough that a fifth trigger
  (or the GraphQL `reviewThreads.isResolved` lever ADR-001 names as the next lever if
  the watermark proves too coarse) should probably be the point where this either
  becomes a small per-trigger detector list/table or gets split into
  `evaluateCIHealth`/`evaluateReviewHealth`/etc. helper functions callable from one
  orchestrating loop.
- `render()` (`session/git/worktree_git.go:472`) is accreting hardcoded, ordered `if`
  blocks per signal type (conflict → CI → blocking reviews → **new: reviewer
  comments** → general comments). Fine at 5 sections; if a 6th ever needs mid-list
  insertion, consider a small ordered slice of `(predicate, renderFunc)` pairs instead
  of manually re-threading insertion order through prose comments each time (as this
  plan's Task 1.1.3c has to do — "insert after the blockingReviews loop and before the
  generalComments block").
- `hasNewFeedback` as a plain local `bool` (Story 3.1.1) is the right call, not a
  primitive-obsession gap — it's a single-use, single-tick-lifetime computed value with
  no independent identity or reuse across function boundaries; promoting it to a named
  type would be over-engineering for a Transaction Script local. No action needed;
  noted only because the review brief asked for an explicit assessment.
- `BacklogItem`'s aggregate keeps absorbing more flat lifecycle-tracking fields
  (`ShippedSnapshotAt`, `PlanApprovedAt`, `QueuedAt`, and now `PrFeedbackAddressedAt`)
  directly on the root rather than in a nested "PR lifecycle" or "ship lifecycle" value
  object. This is a pre-existing pattern this plan continues rather than introduces,
  and matching precedent is the right call for a Small-appetite extension — but if a
  future plan adds a 5th/6th such field, that's the point to consider extracting a
  value object instead of continuing the flat-field pattern.
