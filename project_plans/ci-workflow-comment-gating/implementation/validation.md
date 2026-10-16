# Validation Plan: ci-workflow-comment-gating

**Date**: 2026-08-12

## Happy Path Scenario

Given a PR where benchmark deltas are within threshold, Axe passes, Lighthouse ≥ 70,
Claude UX analysis has zero findings, feature-coverage percentage is unchanged from the
prior run, and all expected E2E video shards produced artifacts, when the PR is pushed,
then none of the four target workflows (`benchmark.yml`, `ux-analysis.yml`, `build.yml`,
`e2e-video.yml`) post a new advisory PR comment, and any stale comment left over from an
earlier "bad" push on the same PR is deleted (except `build.yml`'s coverage comment, which
is deliberately left in place on an unchanged value per Pattern Decisions' explicit
no-delete-on-unchanged rule).

## Two Known Gaps Surfaced During Validation Design — RESOLVED 2026-08-12

Both gaps below were found during this validation pass and have since been patched into
`plan.md` (Tasks 4.1.1d and 3.1.1e respectively). Left here as a record of what validation
caught, per this repo's `.claude/rules/fix-flaky-tests-dont-defer.md`-style discipline of not
silently re-deriving the same gap later.

1. **~~Phase 4 (`e2e-video.yml`) has no concurrency-block or recency-guard task.~~ Fixed:
   Task 4.1.1d now adds the `concurrency:` block.** The test
   `e2eVideoConcurrencyGuard_should_CancelSupersededRun_When_NewerPushArrives` below should now
   pass once Task 4.1.1d is implemented — it is no longer expected to fail against the plan.

2. **~~Architecture-review Concern #2 (structured marker vs. regex-scraped prose for
   `previousCoveragePct`) was not adopted.~~ Fixed: Task 3.1.1e switches to the
   `<!-- feature-coverage:pct=NN.N -->` marker.** The test
   `previousCoveragePct_should_ReturnNull_When_CommentTemplateGainsASecondPercentage` below
   should be implemented against the marker-based parser directly, not the prose-scraping regex
   it originally documented the fragility of.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| AC #1 (go-tier1 regression gate) | `tools/ci-gates/benchmark-regression.test.js` | `hasRegression_should_ReturnTrue_When_BenchstatDeltaExceeds20PercentWithSignificance` | Unit | Happy path |
| AC #1 (go-tier1 regression gate) | `tools/ci-gates/benchmark-regression.test.js` | `hasRegression_should_ReturnFalse_When_AllBenchstatDeltasAreStatisticallyInsignificantTildeMarks` | Unit | Edge/error path |
| AC #1 (frontend-throughput 2x gate) | `tools/ci-gates/benchmark-regression.test.js` | `hasRegression_should_ReturnTrue_When_ThroughputPctNumIsAtOrBelowNegative50` | Unit | Happy path |
| AC #1 (frontend-throughput 2x gate) | `tools/ci-gates/benchmark-regression.test.js` | `hasRegression_should_ReturnFalse_When_ThroughputPctNumIsNoisyButAboveNegative50Threshold` | Unit | Edge/error path — guards ADR-002's amended coarser threshold against firing on 20-49% single-sample jitter (adversarial Blocker 2) |
| AC #1 (e2e-latency 2x gate) | `tools/ci-gates/benchmark-regression.test.js` | `hasRegression_should_ReturnTrue_When_LatencyPctNumIsAtOrAbove100` | Unit | Happy path |
| AC #1 (e2e-latency 2x gate) | `tools/ci-gates/benchmark-regression.test.js` | `hasRegression_should_ReturnFalse_When_LatencyPctNumIsBelow100Threshold` | Unit | Edge/error path |
| AC #1 (all 3 jobs, live wiring) | `tests/e2e/scratch/benchmark-gate.manual.md` (checklist, not automated) | `benchmarkGate_should_PostComment_When_RealRegressionExists_And_StaySilent_When_Clean` | Integration / manual scratch-PR | Push a deliberately slowed benchmark to a throwaway branch/PR; confirm exactly one comment appears per regressed job and none for clean jobs |
| AC #2 (stale comment cleanup, benchmark.yml) | `tools/ci-gates/benchmark-regression.test.js` | `gateAction_should_ReturnDelete_When_PriorRegressionClearedOnCurrentRun` | Unit | Happy path — gate module returns a `{action: 'delete'\|'post'\|'noop'}` directive so CRUD stays untested-by-design but the decision is unit-testable |
| AC #2 (stale comment cleanup, benchmark.yml) | `tools/ci-gates/benchmark-regression.test.js` | `gateAction_should_ReturnNoop_When_NoExistingCommentAndNoRegression` | Unit | Edge/error path — no existing comment + false regression must not attempt a delete call |
| AC #2 (stale comment cleanup, benchmark.yml) | `tests/e2e/scratch/benchmark-gate.manual.md` | `staleCommentDeleteOnClean_should_RemoveMarkerComment_When_SubsequentPushClearsRegression` | Integration / manual scratch-PR | Push a regressing commit, confirm comment posts; push a fix, confirm the comment is deleted (also resolves Unresolved Question #1 — verifies `deleteComment` doesn't 403 under `pull-requests: write`) |
| AC #3 (ux-analysis actionability gate) | `tools/ci-gates/ux-actionability.test.js` | `isActionable_should_ReturnTrue_When_LighthouseScoreBelow70` | Unit | Happy path |
| AC #3 (ux-analysis actionability gate) | `tools/ci-gates/ux-actionability.test.js` | `isActionable_should_ReturnTrue_When_LighthouseScoreIsNaN_ViaExplicitIsNaNCheck_NotBareLessThanComparison` | Unit | Edge/error path — regression guard for the exact pitfall `plan.md` names: `NaN < 70` is `false` in JS, so a naive comparison alone would silently treat a Lighthouse crash as a clean pass |
| AC #3 (ux-analysis actionability gate) | `tools/ci-gates/ux-actionability.test.js` | `isActionable_should_ReturnFalse_When_AxePassesLighthouseHighAndZeroFindings` | Unit | Additional happy path — confirms the "fully green" composite case, feeds AC #6 |
| AC #3 (ux-analysis, live wiring + race guard) | `tests/e2e/scratch/ux-analysis-gate.manual.md` | `uxAnalysisConcurrencyGuard_should_CancelSupersededRun_When_NewerPushArrives` | Integration / manual scratch-PR | Push two rapid commits to the same PR (first regressing, second clean); confirm Task 2.1.1e's `concurrency:` block cancels the first run before it can delete the second run's correct state — resolves adversarial Blocker 1 for this workflow |
| AC #4 (build.yml coverage delta gate) | `tools/ci-gates/coverage-delta.test.js` | `isActionable_should_ReturnTrue_When_CurrentCoveragePctDiffersFromPrevious` | Unit | Happy path |
| AC #4 (build.yml coverage delta gate) | `tools/ci-gates/coverage-delta.test.js` | `isActionable_should_ReturnFalse_When_CoverageUnchanged_And_NoDeleteRequested` | Unit | Edge/error path — also asserts the gate's action directive is `noop`, never `delete`, on an unchanged value (Pattern Decisions' explicit no-delete-on-unchanged rule) |
| AC #4 (previousCoveragePct parsing fragility — architecture-review Concern #2, unresolved in plan.md) | `tools/ci-gates/coverage-delta.test.js` | `previousCoveragePct_should_ReturnNull_When_CommentTemplateGainsASecondPercentage` | Unit | Edge/error path — feeds a fixture comment body with two parenthesized percentages (simulating a future per-category breakdown) and asserts the regex either mismatches or silently picks the wrong one; documents the exact fragility architecture-review flagged so a future template change is caught by a failing assertion instead of discovered live. **Recommend implementing the `<!-- feature-coverage:pct=NN -->` marker fix from architecture-review before this test is written against the final code**, at which point this test is rewritten to assert marker-based parsing instead. |
| AC #4 (build.yml, live wiring + recency guard) | `tests/e2e/scratch/build-coverage-gate.manual.md` | `buildCoverageRecencyGuard_should_SkipMutation_When_RunIsSupersededByNewerPush` | Integration / manual scratch-PR | Trigger two overlapping `build.yml` runs on the same PR (simulate via two rapid pushes); confirm Task 3.1.1d's SHA-recency guard causes the superseded run to skip the comment mutation entirely — resolves adversarial Blocker 1 for `build.yml` specifically, since it cannot use a workflow-level `concurrency:` block |
| AC #5 (e2e-video.yml anomaly gate) | `tools/ci-gates/video-anomaly.test.js` | `videoAnomaly_should_ReturnTrue_When_ZeroVideoArtifactsProduced` | Unit | Happy path |
| AC #5 (e2e-video.yml anomaly gate) | `tools/ci-gates/video-anomaly.test.js` | `videoAnomaly_should_ReturnFalse_When_AllExpectedShardsProduceArtifacts` | Unit | Edge/error path |
| AC #5 (partial-shard-failure visibility — architecture-review adversarial Concern, not yet addressed in plan.md) | `tools/ci-gates/video-anomaly.test.js` | `videoAnomaly_should_ReturnFalse_When_OnlyOneOfTwoExpectedShardsProducesArtifacts_KnownVisibilityGap` | Unit | Edge/error path — asserts the *current plan's* literal `=== 0` definition (not `< matrix.length`), so this test passes today but exists to make the adversarial-review Concern's tradeoff explicit and grep-able rather than silently accepted; should be updated to `< 2` if that recommendation is taken |
| AC #5 (e2e-video.yml, live wiring) | `tests/e2e/scratch/e2e-video-gate.manual.md` | `e2eVideoCommentGate_should_StaySilent_When_NormalRun_And_Comment_When_ZeroArtifacts` | Integration / manual scratch-PR | Confirm no comment on a normal 2-shard run; force a shard failure and confirm the anomaly comment posts, then confirm it clears on the next clean run |
| AC #5 (e2e-video.yml race guard — currently unimplemented, see Known Gap #1 above) | `tests/e2e/scratch/e2e-video-gate.manual.md` | `e2eVideoConcurrencyGuard_should_CancelSupersededRun_When_NewerPushArrives` | Integration / manual scratch-PR | Same race scenario as the `ux-analysis.yml` test above, applied to `e2e-video.yml`. **Expected to fail (or reveal no `concurrency:` block exists) against the plan as currently written** — Phase 4 has no Task 4.1.1d. This test is the validation-phase proof of Known Gap #1; it should stay in the suite as a red test until a concurrency/recency-guard task is added and implemented for this workflow. |
| AC #6 (green PR → near-zero comments, composite) | *(no dedicated composition test — see note)* | — | — | Each extracted module's own happy-path test (Tasks 1.1.1e/2.1.1f/3.1.1e/4.1.1e: `hasRegression_should_ReturnFalse_When_...`, `isActionable_should_ReturnFalse_When_...`, `videoAnomaly_should_ReturnFalse_When_...`) already independently proves "clean signal → no comment" per module; a fifth cross-module composition test would only re-assert that four independent pure functions don't share state, which they don't by construction (no shared imports, no globals) — not added as its own task per this plan's Complexity-2 scoping. |
| AC #6 (green PR → near-zero comments, end-to-end) | `tests/e2e/scratch/full-suite.manual.md` | `greenScratchPR_should_ProduceZeroNewAdvisoryComments_When_AllFourWorkflowsRunClean` | Integration / manual scratch-PR | Corresponds to plan.md Task 6.1.1a — open a scratch PR with no regressions/findings/coverage-drift/video anomalies; confirm zero new comments across all four workflows in one combined run |
| AC #7 (blocking checks unaffected) | `tools/ci-gates/__tests__/workflow-config-invariants.test.js` | `axeStep_should_RetainContinueOnErrorFalse_When_UxAnalysisWorkflowYamlIsParsed` | Unit | Happy path — parses the real `.github/workflows/ux-analysis.yml` (via `js-yaml`) and asserts the Axe step's `continue-on-error` is absent or `false`, independent of any live run. Backed by plan.md Task 6.1.2c (added post-validation). |
| AC #7 (blocking checks unaffected) | `tools/ci-gates/__tests__/workflow-config-invariants.test.js` | `rpcTestCoverageStep_should_RemainBlocking_When_BuildWorkflowYamlIsParsed` | Unit | Edge/error path — parses `.github/workflows/build.yml` and asserts the "Check new RPCs have tests" step has no `continue-on-error: true`, and that `registry-validation.yml`'s divergence-failure step (lines ~47-58 per requirements.md) is untouched. Backed by plan.md Task 6.1.2c (added post-validation). |
| AC #7 (blocking checks unaffected, live) | `tests/e2e/scratch/ux-analysis-gate.manual.md` | `axeViolation_should_StillFailUxAnalysisJob_When_PRIntroducesWcagViolation` | Integration / manual scratch-PR | Deliberately introduce an Axe critical/serious violation on a scratch PR; confirm the `ux-analysis.yml` job still fails red exactly as before this item — supersedes plan.md Task 6.1.2a's pure diff-review with an actual live assertion |

## UX Acceptance Tests

N/A — pure CI/infrastructure change, no end-user UI surface.

## Test Stack

- **Unit**: Node's built-in `node:test` runner (`node --test tools/ci-gates/`), zero new
  dependency. Chosen over Jest because none of the `tools/*` directories in this repo
  currently have a test framework installed (`tools/coverage`, `tools/ux-analysis`,
  `tools/docs-gen` are plain `ts-node` scripts with no `package.json` test script), and
  `node:test` needs nothing beyond what `tools/ci-gates` will already require (Node +
  TypeScript, matching `tools/coverage`'s existing `ts-node`/`tsconfig.json` shape). Each
  extracted gate module (`benchmark-regression.js`, `ux-actionability.js`,
  `coverage-delta.js`, `video-anomaly.js`) exports pure functions taking primitive/plain-object
  inputs (parsed benchstat text, numeric pct deltas, comment-body strings) and returning a
  boolean or `{action}` directive — no GitHub API calls, no filesystem access beyond what the
  test fixture passes in directly, so no mocking framework is needed.
- **Integration**: None automated — no CI harness in this repo executes embedded
  `github-script`/`node -e` blocks or live GitHub Actions runs outside of GitHub Actions
  itself (confirmed in adversarial-review.md Minors: "No automated test/lint exists anywhere
  in this repo for `github-script` blocks embedded in workflow YAML"). "Integration" tests in
  this plan are scratch-PR checklists (`tests/e2e/scratch/*.manual.md`, new files, one per
  workflow) run by a human during Phase 6 cross-cutting verification, mirroring plan.md's own
  Risk Control staged-rollout approach.
- **E2E / Manual**: Scratch-branch/test-PR verification as described in plan.md's Risk
  Control section — push to a throwaway branch, open a PR against it, observe the Actions
  run and PR comment thread directly. Repeated independently per phase before each merges,
  per plan.md's existing convention. No new tooling required.

## Coverage Targets and How to Measure

This repo has no existing test harness for embedded `github-script`/YAML gate logic — the
`tools/ci-gates/*.js` extraction this validation plan assumes (per architecture-review's
Remediation) is itself net-new infrastructure, not an addition to an existing suite.

| Surface | Coverage expectation | How measured |
|---|---|---|
| `tools/ci-gates/*.js` (benchmark-regression, ux-actionability, coverage-delta, video-anomaly) | 100% of the threshold/branch logic (every `if`/comparison identified in the Requirement → Test Mapping table above) — these are the ~5-10 line decision functions architecture-review calls "the actual subject of this item" | `node --test --experimental-test-coverage tools/ci-gates/` (Node's built-in coverage reporter, no extra dependency); target is branch coverage on the gate functions specifically, not line coverage on the whole `tools/` tree |
| `.github/workflows/*.yml` wiring (env passthrough, `listComments`/`updateComment`/`createComment`/`deleteComment` calls, `concurrency:` blocks) | Not unit-testable by construction — no offline linter exists for embedded `github-script` JS syntax (pitfalls.md §5) and the GitHub REST calls require a live PR/token | Scratch-PR manual checklists in `tests/e2e/scratch/*.manual.md`; treat "ran the checklist, observed the expected comment behavior" as the pass/fail signal, recorded in the PR description per phase (mirrors plan.md Task 6.1.1a) |
| Workflow YAML static invariants (which steps carry `continue-on-error`) | 100% of the specific lines named in AC #7 (Axe step, "Check new RPCs have tests" step) | `tools/ci-gates/__tests__/workflow-config-invariants.test.js` parses the real YAML files with `js-yaml` (new devDependency, scoped to `tools/ci-gates/package.json`, not `web-app/`) and asserts on parsed step objects — this is the one place static analysis *can* substitute for a live run, since it's checking config shape, not runtime GitHub-API behavior |
