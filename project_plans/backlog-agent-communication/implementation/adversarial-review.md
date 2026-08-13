# Adversarial Review: backlog-agent-communication

**Date**: 2026-07-23
**Verdict**: CONCERNS (2 BLOCKERs identified during review were resolved by
patching `plan.md` before this document was finalized; the CONCERNS and MINORs
below remain open and should be read alongside the plan, not treated as
resolved)

**Process note**: this environment has no subagent-dispatch tool available for a
genuinely independent reviewer pass — this review was performed by the same
session that wrote the plan, applying the SDD adversarial-review rubric
deliberately adversarially (actively looking for reasons the plan is wrong,
not confirming it). This is a real limitation on independence and is disclosed
here rather than silently treated as equivalent to a separate reviewer; a human
reviewing this plan should weight that accordingly.

## Blockers (found, then resolved by editing plan.md — verify the fix on read)

- [x] **PR self-report trusted without verification (Epic 3.1, Story 3.1.1)** —
  the original `report_pr_created` design persisted the agent's claimed
  `pr_url`/`pr_number` onto the item record without confirming against GitHub
  that the PR actually exists and matches the item's branch. Unlike
  `pushAndCreatePR`'s mechanical path (trustworthy because it only ever writes
  data it itself just generated), this tool accepts an agent's self-report — a
  hallucinated, stale, or mistyped PR reference would silently poison the item
  record with a *wrong* reference, a new failure class BUG-040 never had to
  guard against (BUG-040 was a *real* PR's reference being lost, not a fake one
  trusted). **Resolution applied**: added a mandatory GitHub-verification step
  (new Task 3.1.1e) before any persist — mismatch or not-found rejects the call;
  transient API errors ask for retry rather than silently skipping verification.
- [x] **Human response to "ask for help" had no guaranteed delivery path (Epic
  5.1, Story 5.1.2)** — the original design persisted `response_text` and said it
  would be "surfaced via the next session," without specifying what causes a next
  session to exist. If the agent that called `request_help` had already exited
  (a plausible case — "genuinely stuck" often means the agent gave up), a human's
  response could sit correctly persisted and never reach any agent, silently
  defeating dimension 3's entire purpose. **Resolution applied**: `RespondToHelpRequest`
  now explicitly branches on live-session-vs-not — delivers via
  `write_to_session` when a work session is still live, or spawns a fresh session
  (`resume_session: bool`) seeded with the response when not, with durable
  `get_backlog_item` surfacing kept only as a fallback, not the primary delivery
  mechanism.

## Concerns

- [ ] **`InfraIssueReport` dedup window is a fixed magic number (1 hour) with no
  stated justification** — reasonable as an MVP default, but arbitrary. If a
  crash-loop cycles slower than hourly (e.g. every 90 minutes, matching this
  repo's own `remediationBackoffSchedule`'s 2-hour tier), the dedup won't catch
  it and the original alert-fatigue pitfall partially resurfaces. Recommendation:
  note in the implementation task that the window should be revisited against
  real crash-loop intervals once this ships, rather than treated as final.
- [ ] **Epic 6.1's dispute cap (2 lifetime per item) has no stated derivation** —
  unlike `MaxRemediationAttempts` (explicitly `len(remediationBackoffSchedule)`,
  a load-bearing invariant with a comment explaining it), "2" here is asserted
  without justification. Low risk (it's a soft ergonomic cap, not a correctness
  invariant, and the human-adjudication path remains available past the cap),
  but should get at least a one-line rationale comment at implementation time
  (e.g. "one dispute for the original verdict, one more if the re-review/
  adjudication itself seems wrong — a third disagreement should not still be
  self-adjudicated by the same implementer").
- [ ] **Task 2.1.1a explicitly defers locating `ReviewVerdictData`'s exact
  persistence file to implementation time** ("first implementation task must
  locate it before the rest proceeds") — this research pass confirmed
  `SaveReviewVerdict` exists and is called from `submitReviewVerdict`
  (`server/mcp/tools_backlog.go:505`) but did not trace its full ent-backed
  implementation. Not a blocker (the task explicitly budgets time to resolve
  this first), but flagged so Phase 4 (validate) checks this doesn't balloon
  past its ~5 minute estimate once the real location is found — if the
  underlying schema turns out to need a genuinely new entity rather than a new
  column, Task 2.1.1a's task-sizing assumption breaks and should be re-scoped.
- [ ] **Epic 6.1's "pause `autoReopenWithBackoffGate`" mechanism relies on a
  precise sequencing argument** (a dispute can only be filed *after*
  `handleReviewSessionExited`'s FAIL branch has already run once, so the actual
  guard has to live in `AutoReopenAfterFailedReview`, not the caller) — the plan
  gets this right by reasoning through the existing code's control flow (Task
  6.1.1d), but this is exactly the kind of "two calls that must agree on a
  precondition across a call boundary" shape that caused BUG-040 root cause #2.
  Recommendation carried into the plan already via the explicit design note in
  Story 6.1.1 — flagging again here because it's the single highest-risk piece
  of new control flow in this entire plan and deserves extra scrutiny (and an
  explicit regression test, already listed:
  `TestAutoReopenAfterFailedReview_should_NoOp_When_OpenDisputeExists`) during
  implementation, not just at plan-review time.
- [ ] **No explicit interaction between a `request_help` escalation and an
  in-flight `RemediationDue` backoff for the *same* item under a *different*
  reason** — e.g. an item mid-`bouncing`-backoff that also gets a
  `request_help` call. ADR-001 correctly notes `request_help`/dispute rows don't
  route through `RemediationDue` themselves, but doesn't say whether an open
  `StuckReasonHelpRequested` row should *block* other reasons' automated
  remediation (mirroring `RemediationBlocked`'s existing cross-reason-blocking
  pattern, built specifically for this kind of interaction per its own doc
  comment in `session/backlog_remediation.go`). Recommendation: at
  implementation time, add a `RemediationBlocked` check for
  `StuckReasonHelpRequested`/`StuckReasonVerdictDisputed` into
  `autoReopenWithBackoffGate` and any other automated-remediation call site —
  it would be inconsistent (and potentially confusing to the human, who is
  mid-conversation with the agent about being stuck) for an automated rework to
  fire while a help request or dispute is still open on the same item. Not
  added as a plan task because it's a small addition to existing call sites
  more naturally scoped during Epic 5/6 implementation than pre-specified here.

## Minors

- Some new-component tasks (e.g. Task 1.1.1f, 2.1.1f) name the target file as
  "confirm exact file at implementation time" rather than a fully resolved path
  — acceptable per this plan's research depth, but Phase 4 validation should
  confirm these don't hide larger frontend-architecture surprises.
- The plan does not explicitly call out `make registry-generate` as a task for
  every new backend/frontend surface (only some tasks mention a registry entry
  explicitly) — implementation should treat `.claude/rules/feature-registry.md`
  as a blanket requirement across all of Epics 3–6's new RPCs/components, not
  just the ones that spell it out.
- `ADR-003`'s deferred Epic 5.2 is not represented in the dependency
  visualization diagram at the top of `plan.md` — cosmetic, since it correctly
  has zero tasks, but a reader skimming only the diagram could miss that the
  "Master agent" question was investigated and explicitly deferred rather than
  overlooked.

## What Was Checked and Found Acceptable (no action needed)

- **`MarkStuck`'s concurrency safety for the new agent-initiated call pattern**
  (ADR-001): traced `EntRepository.MarkStuck` — it upserts via
  `OnConflictColumns(FieldItemID, FieldReason)`, meaning the DB enforces
  uniqueness on `(item_id, reason)` and concurrent `request_help`/
  `dispute_review_verdict` calls cannot create duplicate rows even without an
  application-level lock. The only residual risk from a race is a duplicate
  `Notifier.Notify` call (cosmetic, not data corruption) if two calls both pass
  the pre-check before either commits — acceptable for MVP, not worth a task.
- **Scope drift**: reviewed every epic against requirements.md's in-scope list —
  no epic introduces capability outside the four named dimensions or the two
  named pain points. Epic 3.2 (reconciliation safety net) is the closest to
  "extra scope" but is directly justified by Epic 3.1's own failure mode (agent
  crashes before calling the new tool), not speculative addition.
- **Technology bets**: no new external dependency, service, or infrastructure
  introduced anywhere in the plan — consistent with requirements.md's
  low-operational-overhead constraint and ADR-003's explicit reasoning.
- **Coverage against requirements.md's success metrics**: verified each of the
  five bulleted success metrics against the plan's "Cross-Reference" table and
  individual epic acceptance criteria — all five have a concrete answer in the
  plan (see plan.md's closing cross-reference table).
