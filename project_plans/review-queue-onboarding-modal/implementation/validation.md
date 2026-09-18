# Validation Plan: review-queue-onboarding-modal

**Date**: 2026-08-15

Complexity 1, no new code: this plan verifies existing behavior and records
per-criterion outcomes via MCP tool calls (see plan.md) rather than writing
new unit/integration tests. The standard unit/error/integration triad
doesn't apply — each acceptance criterion maps to an existing command whose
output is the evidence, not a new test file.

## Happy Path Scenario

Given the fix already on `HEAD` (`2811df54e`) and this session's two
research passes concluding AC4 conflicts with AC2, when the verification
commands below are re-run and their output matches what requirements.md
claims, then AC1/2/3/5/6/7 are reported `pass` with fresh evidence, AC4 is
reported `fail` with the research citations, and `request_review` surfaces
the AC4-vs-AC2 conflict to a human for a final decision.

## Requirement → Verification Mapping

| Criterion | Verification Command / Check | Type | Evidence Recorded |
|---|---|---|---|
| AC1: dismissal helper on every `goto()` in review-queue.spec.ts | `grep -n dismissOnboardingIfPresent tests/e2e/review-queue.spec.ts` | Static/grep | Line numbers where the helper follows each `goto()`, including tests with no `waitForSelector` (per the original backlog AC1 text) |
| AC2: 0 failed across 22 instances | `cd tests/e2e && npx playwright test review-queue.spec.ts --reporter=list` | E2E run | Exact `X failed, Y passed, Z skipped` line from output |
| AC3: deterministic under `--retries=0` | `cd tests/e2e && npx playwright test review-queue.spec.ts --reporter=list --retries=0` | E2E run | Same, with `--retries=0` in the command recorded |
| AC4: fix changes only the dismissal step (no production/test-intent changes) | `git show HEAD -- web-app/src/components/sessions/ReviewQueuePanel.tsx tests/e2e/review-queue.spec.ts` — already reviewed in `research/pitfalls.md` §1-2 | Diff review (already done) | Cite `research/architecture.md` + `research/pitfalls.md` in the `fail` note — **this criterion is expected to fail**, not pass |
| AC5: shared `OnboardingPage.ts` helper, plain function | `grep -n "export" tests/e2e/pages/OnboardingPage.ts` + `grep -rn "from.*OnboardingPage" tests/e2e/*.spec.ts` | Static/grep | Function signature (not a class) + both spec files' import lines |
| AC6: Session Creation Flow tests get the dismissal step | `grep -n -A2 "test.describe('Session Creation Flow" tests/e2e/review-queue.spec.ts` | Static/grep | Both tests in that block call `dismissOnboardingIfPresent` |
| AC7: escalation-reasoning.spec.ts matches pre-migration pass count | `git diff 303d43fad:tests/e2e/escalation-reasoning.spec.ts HEAD:tests/e2e/escalation-reasoning.spec.ts` (baseline diff) + `cd tests/e2e && npx playwright test escalation-reasoning.spec.ts --reporter=list` (fresh run) | Diff + E2E run | Diff limited to the dismissal-helper swap; fresh run's pass/fail count |

## Contingency (from adversarial-review.md's blocker)

If any command's output doesn't match what requirements.md/plan.md claims
(e.g. a new failure appears in `review-queue.spec.ts`), **stop** before
recording any `pass` status for the affected criterion — do not substitute
requirements.md's earlier numbers for a fresh command's actual output. This
is the one genuine "test" this validation plan gates on: the re-run itself
is the check, and a mismatch means the fix has regressed and needs new root-
cause work, not a report_progress call.

## Test Stack

- **E2E**: Playwright (`tests/e2e/`), existing suite — no new tests written.
- **Static verification**: `grep`/`git diff` against the current worktree —
  no new tooling.

## Coverage Targets

N/A — no new code, no coverage delta. The existing `review-queue.spec.ts`
and `escalation-reasoning.spec.ts` suites are unchanged by this pass (see
plan.md's "Out of scope" section).
