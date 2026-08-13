# Adversarial Review: ci-hookurl-race-flake
**Date**: 2026-08-01
**Verdict**: CONCERNS

## Blockers
_(none — the adoption is faithful to the prior, already-reviewed plan; every re-verified
file:line claim checked out against the current tree; the core `-p 1` + `require.Eventually`
+ 60s mechanism is sound and reversible; and the one genuinely new task, 2.1.2b, is
correctly scoped as diagnostic-only with no code-change authority of its own.)_

## Concerns

- [ ] **The "20-run" success-metric citation is a stale/undisambiguated cross-project
  reference, and this project's own AC #1 ("exact N and method decided during planning")
  is never actually decided.** `plan.md` states twice — Observability Plan ("the 20-run
  window named in requirements.md's Success Metrics") and Risk Control ("after the 20-run
  observation window") — that the 20-run figure comes from "requirements.md." But
  **this project's own `requirements.md` (`project_plans/ci-hookurl-race-flake/requirements.md`)
  has no "Success Metrics" section and the string "20" does not appear in it anywhere**
  (confirmed via `grep -n "Success Metrics\|20" project_plans/ci-hookurl-race-flake/requirements.md`
  — zero matches). The 20-run figure only exists in the *sibling* project's doc
  (`project_plans/flaky-hook-url-tests/requirements.md:47-49`). This project's own AC #1
  explicitly delegates a decision to the planning phase — "verified by N consecutive green
  CI runs post-fix... exact N and method decided during planning" — and this plan.md never
  makes that decision explicitly; it silently inherits "20" from the other project without
  ever saying so. A reader who opens only this project's directory (the document's own
  stated self-containment goal) will follow the "requirements.md's Success Metrics" citation,
  find nothing, and have no way to know the number came from elsewhere or that it was ever
  deliberately chosen for *this* project's AC #1.
  — Recommendation: add one sentence, e.g. "N=20 is adopted from
  `flaky-hook-url-tests/requirements.md`'s Success Metrics (this project's own requirements.md
  does not define an N) — chosen because [X]," turning the inheritance into an explicit
  planning decision rather than an unstated citation that doesn't resolve.

- [ ] **Task 3.1.3's "pre-registered presumptive cause" is a comment + PR-description
  sentence, not a mechanism that changes the probability, impact, or detectability of a
  `testSocketOnce` recurrence — calling it "Risk Control" overstates what it does.** It
  only helps if a future engineer (a) notices the recurrence, (b) happens to read that
  specific comment near `waitForTmuxTeardown` or that specific PR's description before
  reflexively re-tuning, and (c) acts on it instead of bumping the timeout again — none of
  which is enforced by tooling, and the plan itself elsewhere (§Provenance) explicitly
  accepts *not* fixing the underlying mechanism. This is the *only* concrete new task this
  plan adds beyond what the sibling plan already had for pre-mortem P1, so it's worth being
  honest about its actual power: it reduces future *diagnosis time* for a human who finds it,
  not recurrence *probability*, and it is not "prevention" in the sense the Risk Control
  section's heading implies.
  — Recommendation: either reframe this as "diagnosis aid," not "risk control," in the
  section heading, or strengthen it cheaply — e.g. file the `testSocketOnce` follow-up
  ticket *now* (it's already fully diagnosed per architecture.md §2, not contingent on a
  future recurrence to be actionable) rather than only "if it recurs, file a follow-up
  ticket," which defers a five-minute action to a moment when it's more likely to be
  forgotten under incident pressure.

- [ ] **AC #2 ("documented rationale, not a bare number bump") is satisfied only if a human
  manually copies sentences out of this plan into the shipping PR description — nothing
  enforces that.** Task 2.1.2's Given/When/Then says the wall-clock delta is "written into
  the PR description as an explicit, accepted trade-off," and Task 3.1.3 says its sentence
  is "include[d] ... in the shipping PR description" — both are prose instructions to
  whichever future session executes this plan, with no PR template checklist item, no CI
  check, and no linked requirement gating merge on their presence. If the implementing
  session (a fresh context, per SDD convention) skips or forgets either, AC #2 silently
  regresses to "just a number bump" while the rationale survives only in a planning doc a
  PR reviewer has no reason to open.
  — Recommendation: have `sdd:7-ship`/the PR description generator consume this plan's
  Task 2.1.2 and 3.1.3 sentences directly (e.g. a short "PR description must-includes"
  list at the top of Step 4), rather than relying on each task's own prose to be
  remembered at ship time.

- [ ] **The divergent `build-vs-buy.md` recommendation is rejected by restating ADR-001's
  conclusion, not by directly refuting `build-vs-buy.md`'s own textual argument.** This
  project's four research passes disagree with each other: `stack.md`, `architecture.md`,
  and `pitfalls.md` all converge on `-p 1`/ADR-001, while this project's own
  `build-vs-buy.md` independently recommends tagging the two tests `//go:build integration`
  (or `testing.Short()`), arguing this "directly satisfies AC #2 ... and AC #3" because the
  existing convention's rationale is already documented at its other call sites. The
  Unresolved Questions section rejects this by saying it "would stop them from running (and
  being covered) in the default gating invocation" — true, but AC #3's actual text is
  "narrows or removes `-race` coverage **for any package**" (emphasis added), and tagging
  just 2 of many tests in the `server` package doesn't narrow *package*-level `-race`
  coverage the way AC #3 is worded — the `server` package as a whole would still be fully
  `-race`-gated. The plan's rebuttal doesn't engage with `build-vs-buy.md`'s framing on
  those terms; it wins by appeal to ADR-001's authority rather than by showing where
  `build-vs-buy.md`'s own AC #2/#3 argument is wrong. The conclusion (keep `-p 1`, don't
  isolate) is very likely still correct — pitfalls.md §5 independently ranks isolation as
  risk tier 3 of 5, and the two-tests'-worth of coverage lost to the advisory
  (`continue-on-error: true`) lane is a real, if narrow, regression — but the plan should
  say that explicitly rather than only citing where the sibling project already decided
  differently.
  — Recommendation: add one sentence directly rebutting `build-vs-buy.md`'s AC #3 framing:
  e.g. "even though this doesn't narrow *package*-level coverage, it does move these two
  tests' own coverage contribution from gating to advisory-only (`continue-on-error: true`),
  which is the specific trade-off AC #3 requires being explicit about — and is not adopted
  because Epic 2's `-p 1` achieves the goal with zero coverage-gating impact at all."

## Minors
- Task 2.1.2 ("Size: 5 min") and the new Task 2.1.2b ("Size: 10 min") both understate real
  wall-clock effort: 2.1.2 requires 6 full `go test -race -coverprofile` runs across 3
  package trees (3 baseline + 3 with `-p 1`), and 2.1.2b requires a `-count=10` stress run
  under artificial `yes`-loop contention plus JSON capture/interpretation — either is
  plausibly 15-40+ minutes of real machine time, not 5-10. This sizing convention is
  inherited from the adopted plan (already present there for Task 2.1.2), not introduced by
  this adoption, but 2.1.2b compounds it with a second optimistically-sized measurement task.
  Not a blocker — "Size" appears to mean authoring/reviewing effort throughout this
  document, not execution wall-clock — but worth a one-line clarification so a future
  reader doesn't mistake it for a time-boxing promise.
- Task 2.1.2b's "optionally via `gotestsum --jsonfile=...`" is a soft, local-only,
  non-committed external tool dependency. Harmless since it's explicitly optional
  (`go test -json` alone is the zero-dependency path) and never lands in `go.mod` or
  `build.yml`, but the plan doesn't say so explicitly — worth one clause confirming this
  doesn't trip the sibling project's "no new external dependencies" constraint, which was
  written with CI-committed tooling in mind, not throwaway local diagnostics.
- The Provenance section's claim that two of the sibling adversarial/architecture review's
  concerns are "already closed by `validation.md`" refers to
  `flaky-hook-url-tests/implementation/validation.md` (confirmed to exist and to contain the
  relevant sections), not a `validation.md` under this project's own directory (confirmed:
  no `project_plans/ci-hookurl-race-flake/implementation/validation.md` exists yet — this
  plan is Phase 3 output, Phase 4 hasn't run). This is expected and not a defect, but
  "already closed" reads as more final than "closed in the sibling project's doc, to be
  reproduced in this project's own Phase 4" — a reader who takes literally the stated goal
  of being "self-contained... without a reader needing to open the other project's
  directory" won't yet find that content here.
- Verified as accurate, no correction needed: every file:line reference re-checked
  independently against the current tree (`server/server_integration_test.go:336-540`,
  `.github/workflows/build.yml:119-168,362-373`) matches the plan's own re-verification
  table exactly, including the `timeout-minutes` claim (only `benchmark-gate`, line 365,
  sets one) and the `needs:` job graph (only `benchmark-gate` depends on `test`).
