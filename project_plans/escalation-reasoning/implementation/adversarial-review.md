# Adversarial Review: escalation-reasoning

**Date**: 2026-08-02
**Verdict**: CONCERNS

Reviewed `implementation/plan.md` (all 772 lines) against `requirements.md` and all 6
`research/*.md` files, cross-checked against the live code (`server/services/approval_handler.go`,
`approval_store.go`, `analytics_store.go`, `session/review_queue_poller.go`,
`web-app/src/components/sessions/ReviewQueuePanel.tsx`, `proto/session/v1/types.proto`). Most of
the plan's factual claims (line numbers, existing field names, `CheckSession` synchronicity, proto
field number 17, `Decision`/`RuleID` string literals used by `ComputeSummary`) were spot-checked
against the actual source and hold up. No blockers found — the concerns below are real gaps but
none breaks a stated AC or requires a design rework.

## Blockers

None.

## Concerns

- [ ] **Domain-age `IsNewlyRegistered` error path has no acknowledgment in the new UI-facing
  feature.** `approval_handler.go:241-245` already `continue`s past a domain whose age check
  errors (logged at `Warn`, swallowed otherwise); if that was the only domain in the command and
  the classifier subsequently escalates for an unrelated reason (or no reason at all), the
  reviewer will see a plain "no rule matched"/explicit-rule reason with zero indication a domain
  check was attempted and came back inconclusive. This is pre-existing behavior the plan correctly
  leaves untouched, but it's a new *consequence* once the reason becomes screen-visible (AC1 asks
  for a reason "sourced from the real escalation path, not a generic string" — this case shows a
  real but incomplete reason). Neither `pitfalls.md`'s edge-case list nor Story 2.1.1/5.1.1's test
  ACs mention it. — Recommend a one-line code comment at `approval_handler.go:243-244` noting the
  interaction (silenced domain-check errors surface as whatever the classifier decides
  afterward), so a future reader doesn't have to re-derive it; a dedicated test is optional given
  the low blast radius.

- [ ] **Epic 3.2's "no-match only" Create Rule gating narrows AC3's literal scope without
  stakeholder sign-off, and the plan's "Unresolved Questions: None" undersells that.**
  `architecture.md:440-446` explicitly flagged two readings of AC3 — (a) purely descriptive (no
  gating change, only the AC7 intent swap) vs. (b) prescriptive (hide the button outside no-match)
  — as an open question for the plan phase. `ux.md` (§5, point 4) resolves it in favor of (b), and
  the plan adopts that. The reasoning is sound, but the practical effect is real: *today* the
  button shows for **any** escalation with `tool_input_command` (explicit-rule, domain-age,
  unclassifiable included); after this change it's hidden for all but no-match — an existing UI
  capability removed for 3 of 4 categories, directed by no single AC's literal text. Because this
  backlog item went through the no-interactive-ideation pipeline (`requirements.md:5-6`), there
  was no way to confirm this with whoever filed it. The plan's "Unresolved Questions" section
  states "None," which is accurate only because the question was resolved via research judgment,
  not because there was nothing left to decide. — Recommend calling this out explicitly in the PR
  description (not just the plan) so a human reviewer can veto before merge if intent differs.

- [ ] **Phase 7 registers only one of the two new frontend features Epic 4.2 actually adds.**
  Epic 7.1's own goal text says "the frontend gets its first registry entries for these
  components" (plural), and `features.md` #5 (registry pattern check) explicitly names two
  candidate entries — `escalation-reasoning-display.json` for `ReviewQueuePanel.tsx` and
  `approval-analytics-reason-breakdown.json` for the new table in `ApprovalAnalyticsPanel.tsx`.
  Task 7.1.1b creates only the first. The "Escalation Reasons" table added in Story 4.2.1
  (`ApprovalAnalyticsPanel.tsx`, with its own Jest test in Task 4.2.1c) is a genuinely new
  user-facing feature with no registry file anywhere in the plan, which is a direct violation of
  `.claude/rules/feature-registry.md`'s "New UI feature → create
  `docs/registry/features/frontend/<feature>.json`." `ApprovalAnalyticsPanel.tsx` already has zero
  registry entries today (a pre-existing gap per `features.md`), so `make registry-generate`'s
  coverage-gaps check (Task 7.1.1c) is unlikely to catch this omission on its own. — Recommend
  adding a second Task 7.1.1 sub-task creating
  `docs/registry/features/frontend/approval-analytics-reason-breakdown.json` with `testIds`
  populated from Task 4.2.1c.

## Minors

- Epic 2.2's 4-struct manual-copy pattern (`PendingApproval` → `PersistedApproval` →
  `persistToDiskLocked` → `loadFromDisk` → `ApprovalMetadata` → `GetApprovalMetadataBySession`) is
  the codebase's existing convention (matches how `Cwd`/`Orphaned`/etc. are threaded today) and is
  well-mitigated here by the orphaned-reload regression test (Task 5.1.1c), which will fail loudly
  if any single copy step is missed. But there's still no generic/reflection-based struct-parity
  test guarding *all* fields across these structs — a future 3rd or 4th field added by a different
  feature inherits the same "forgot one copy site" footgun with only whatever field-specific test
  that feature's author remembers to write, same as this one. Worth a backlog note for a generic
  parity test, not a blocker here.
- Naming inconsistency between hops: `PersistedApproval.EscalationCategory` serializes as JSON key
  `escalation_category` (Domain Glossary), while the same value lands in `ReviewItem.Metadata` as
  key `escalation_reason_category` (note the extra `_reason`). Both are deliberate per the plan,
  but the differing names for one conceptual value across two hops of the same pipeline is a small
  grep/maintenance friction for whoever debugs this later.
- Task 3.2.2a's Jest coverage only asserts button *absence* for the `domain-age` category; it
  doesn't add explicit negative cases for `explicit-rule` or `unclassifiable`, even though the
  gating condition (`=== "no-match"`) necessarily hides the button for those too. Low risk given
  the trivial boolean condition, but a one-line parametrized addition (`test.each`) would fully
  close out AC3/AC7 against all stated categories instead of just one representative negative case.
- Task 5.1.1a's "baseline" verification step only runs `TestHandlePermissionRequest` and
  `TestReviewQueueDeterminer`; it doesn't explicitly baseline-run the full
  `approval_service_test.go` suite (where Task 5.1.1c's new orphaned-reload test lands) before
  Phase 1-4 changes land. Low-stakes given `make test`/`make ci` catches any regression there
  regardless — just a process nit in how the AC's "zero diffs" claim gets established.
