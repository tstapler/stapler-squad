# Implementation Plan: ci-workflow-comment-gating

**Feature**: Gate the four non-reference CI workflows' PR comments (`benchmark.yml` ×3 jobs,
`ux-analysis.yml`, `build.yml`, `e2e-video.yml`) on real actionability, mirroring
`registry-validation.yml`'s existing pattern, while fixing the stale-comment gap that pattern
would otherwise reproduce.
**Date**: 2026-08-12
**Status**: Ready for implementation
**ADRs**: ADR-001 (delete-on-clean stale comments), ADR-002 (benchmark.yml advisory regression threshold)

---

## Domain Glossary
*(CI/workflow-level terms that appear as variables in the added `github-script`/Node/bash logic.
Not a business domain — kept short per Complexity-2 calibration.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `hasRegression` | Boolean computed per `benchmark.yml` gate site: true when the current run shows a statistically significant, threshold-exceeding performance regression vs. the cached baseline. | go-tier1 computes it via a `grep -E` regex over `benchstat -delta-test=utest` output (reusing `build.yml`'s technique verbatim, see ADR-002); frontend-throughput/e2e-latency compute it from their already-computed numeric `pct` deltas. |
| `isActionable` | Boolean gate result: true → post/update the sticky comment; false → no news, delete the existing marker comment if one exists. | Used directly in `ux-analysis.yml` and `e2e-video.yml` (as `videoAnomaly`, see below); `benchmark.yml`'s three jobs use `hasRegression` as their `isActionable` equivalent — no separate variable needed there. |
| `lighthouseParseFailed` | Boolean, true when Lighthouse score extraction produced the string `'unknown'` (so `parseInt` yields `NaN`) — distinguishes "failed to measure" from "measured and passing." | Must be its own explicit `isNaN(score)` branch: `NaN < 70` is `false` in JS, so a naive `score < 70` check alone silently treats a Lighthouse crash as a clean pass (pitfalls.md §1). |
| `findingsCount` | Number of Claude UX analysis findings, surfaced as a new `$GITHUB_OUTPUT` field (`findings_count`) written by `tools/ux-analysis/analyze.ts`. | Net-new signal — doesn't exist today. Treat a `skipped` step outcome (no `ANTHROPIC_API_KEY`, or `ts-node` unavailable) as `findingsCount = 0`, not an error. |
| `previousCoveragePct` | Numeric feature-coverage percentage parsed via regex out of the *existing* sticky comment's body in `build.yml`. | `build.yml` only. No cache/baseline file exists for this value — the sticky comment body is the only persisted "prior state" (features.md §3). |
| `currentCoveragePct` | Numeric feature-coverage percentage parsed from `steps.coverage.outputs.pct` (new output) for the current run. | `build.yml` only. |
| `videoAnomaly` | Boolean, true when `videoArtifacts.length === 0` — the sole condition under which `e2e-video.yml`'s `notify` job should still comment. | Already computed inline today; this item only restructures control flow around it — no new signal (features.md §3). |
| `existing` | The sticky-comment object (or `undefined`) found via `listComments().find(c => c.body.includes(marker))`. | Pre-existing name in all target workflows — kept as-is for consistency, not renamed. |
| Recency guard | `build.yml`-only check comparing this run's commit SHA against the PR's *current* HEAD SHA (via `github.rest.pulls.get`) before mutating the comment; skips the mutation entirely if a newer push has superseded this run. | Net-new, added to resolve adversarial-review Blocker 1 for `build.yml` specifically — see Pattern Decisions. Not needed in `ux-analysis.yml`/`e2e-video.yml`, which use a `concurrency:` block instead. |
| `marker` | Per-workflow HTML-comment constant (e.g. `<!-- benchmark-go-tier1 -->`) used to find/update/delete a workflow's own sticky comment. | Pre-existing per-workflow constants, unchanged by this item. |

---

## Pattern Decisions

*(Step 0.5 creative pass folded in: for each component, the rejected alternative(s) considered
before settling on the chosen approach.)*

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Where to build & gate | In-repo copy of `registry-validation.yml`'s gate condition, restructured (see next row) | build-vs-buy.md Option 1 (Recommended) | (a) Adopt `marocchino/sticky-pull-request-comment` / `peter-evans/create-or-update-comment`; (b) migrate scored signals to GitHub Check Runs (`checks: write`) | (a) neither action computes actionability — the actual problem — so it'd add an external dependency to solve a part that isn't broken (build-vs-buy.md Option 2). (b) is 4x the rewrite effort, needs a new permission scope, and requirements.md explicitly places it behind Open Questions, not this item's ACs (build-vs-buy.md Option 3). |
| Gate placement relative to comment lookup | **Fetch-before-gate**: `listComments`/find-`existing` runs unconditionally at the top of the script; the actionability check is an if/else around delete-vs-post, not a bare early `return` before the lookup | features.md §1, pitfalls.md §2 | Bare early-`return` gate placed *before* `listComments`, i.e. a verbatim copy of `registry-validation.yml:90-95`'s shape | `registry-validation.yml` never has a stale-comment problem because it has no sticky marker/lookup at all — it only ever `createComment`s. The four target workflows *do* have sticky lookup+update, so a verbatim-shaped early return would exit before ever reaching the code that could clear a stale "regression detected" comment, reproducing the exact bug AC #2 exists to prevent (pitfalls.md §2). |
| Stale-comment resolution when the current run is clean | **Delete-on-clean** via `github.rest.issues.deleteComment({ comment_id: existing.id })`, applied to `benchmark.yml` (×3), `ux-analysis.yml`, `e2e-video.yml` | features.md §2 (industry precedent: `marocchino/sticky-pull-request-comment`'s `delete: true` mode, `mshick/add-pr-comment`'s delete-on-status); see ADR-001 | Update-in-place to a "✅ cleared" body | An update-in-place still leaves a permanent comment artifact on every clean PR, which contradicts AC #6's "zero or near-zero advisory comments" on a fully green PR. Delete achieves near-zero; update does not (features.md §2). |
| `build.yml` coverage "prior state" | Regex-parse `previousCoveragePct` out of the *existing* sticky comment body | features.md §3, Option 1 (recommended as lower-effort) | `actions/cache` baseline file mirroring `benchmark.yml`'s cache-based baseline pattern | New cache infrastructure for a single percentage number is disproportionate effort vs. reusing state that's already sitting in the comment thread; matches this item's Low-Medium RICE effort rating (features.md §3, Option 2 rejected as "likely overkill"). |
| `build.yml` "coverage unchanged" resolution | **No delete** — an unchanged-coverage state is not a "problem that got resolved" the way a regression clearing is; leave any existing comment untouched when `currentCoveragePct === previousCoveragePct` | This plan (new reasoning, not explicitly in research) | Applying the same delete-on-clean treatment as benchmark.yml/ux-analysis.yml | Delete-on-clean models "an active problem got fixed." Coverage staying flat isn't a fixed problem — it's the steady state. Deleting a comment that was correctly reporting a real (if old) coverage number on every push where coverage happens not to move would itself be a confusing UX regression, not a noise reduction. |
| Sticky-comment CRUD (list/find/update/create, ~15 lines × 5 workflows) | **Leave inline, do not extract** | build-vs-buy.md Option 4 | Shared composite action wrapping `actions/github-script` (`script-path:` sourcing a `.js` file) | The part that's actually the subject of this item — the actionability gate — is bespoke per workflow and can't be shared; extracting only the CRUD mechanics is a legitimate but orthogonal refactor that would expand this item's blast radius into "introduce a new composite-action shape" for a repo that has no precedent for wrapping inline `github-script` bodies this way (build-vs-buy.md: `.github/actions/prepare` wraps whole third-party actions/shell, not inline JS). Noted as a follow-up, not folded in. |
| `benchmark.yml` go-tier1 regression detection | Reuse `build.yml`'s `benchmark-gate` job's exact regex/flag combo (`-delta-test=utest` + `grep -E '\+([2-9][0-9]|[1-9][0-9]{2,})\.' ... | grep -v '±'`) at the **same 20% threshold** | stack.md, features.md §3, ADR-002 | (a) Naive keyword match (e.g. search for the literal word "regression"); (b) an invented, lower advisory threshold (e.g. 10%) for earlier warning | (a) benchstat's plain-text output never contains the word "regression" — it reports deltas, not verdicts — so a keyword gate would be a 100% false negative for every run, silently hiding all real regressions (pitfalls.md §4, the single highest-risk failure mode identified in this research). (b) `build.yml`'s own top-of-file comment documents 15-25% natural variance for tmux-touching benchmarks on shared runners — a lower threshold risks recreating exactly the noise problem this item exists to fix; reusing the already-calibrated 20% number is safer than inventing an unvalidated one (see ADR-002). **go-tier1 only** — its measurement is repeated-trial (`count=8`) and significance-tested (`-delta-test=utest`), which is what the 20%/15-25%-variance figures were calibrated against. |
| `benchmark.yml` frontend-throughput / e2e-latency regression detection | **Not** the same 20% threshold — a much coarser **"2x" (double/half) obvious-swing threshold**: `frontend-throughput` flags at `pctNum <= -50` (throughput halved or worse), `e2e-latency` flags at `pctNum >= 100` (latency doubled or worse) | ADR-002 (amended), adversarial-review.md Blocker 2 | Reusing go-tier1's 20% threshold verbatim for these two jobs (the plan's original approach, before adversarial review) | Adversarial review Blocker 2: these two jobs are **single-sample** Playwright measurements with zero repeated trials and no significance test anywhere in the file (`grep -in "varia\|noise\|flak" .github/workflows/benchmark.yml` — no hits for these jobs) — the 20% figure's calibration (15-25% documented variance) is specifically for `-delta-test=utest`-filtered, repeated-trial data, which these jobs don't produce. A 20% threshold on raw single-sample noise risks firing on jitter (reproducing this item's own spam problem) or silently missing real regressions under 20% with no fallback visibility. A 2x swing is large enough to be trustworthy without any noise-floor data to calibrate against; it deliberately trades sensitivity (misses real 20-99% regressions) for zero false-positive risk, and is documented in ADR-002 as an explicit placeholder pending a possible future repeated-trials follow-up. |
| Race protection for delete-on-clean / stale mutations (out-of-order runs) | **Split by blast radius, not one mechanism for all three**: `ux-analysis.yml` and `e2e-video.yml` get a workflow-level `concurrency:` block (PR-scoped group, `cancel-in-progress: true`, mirroring `benchmark.yml`'s existing shape); `build.yml` gets an in-script **run-recency guard** (`github.rest.pulls.get` → compare current PR head SHA to this run's SHA; skip the mutation if superseded) instead of a workflow-level `concurrency:` block | adversarial-review.md Blocker 1 | (a) A single `concurrency:` block added uniformly to all 3 workflows; (b) recency-guard everywhere instead of concurrency blocks | (a) `ux-analysis.yml` and `e2e-video.yml` are single-purpose, `pull_request`-only workflows (one job each's worth of user-facing work), so cancelling a superseded run costs nothing extra — same shape `benchmark.yml` already uses. `build.yml` is **not** single-purpose: it also runs on `push` to `main` and has 5 other jobs (`prepare`, `web-build-smoke`, `test`, `build`, `install-check`, `benchmark-gate`) sharing the same workflow. A workflow-level `concurrency:` block there would cancel those unrelated jobs — including in-flight main-branch builds — just to fix a comment-posting race, a materially larger blast radius than the bug being fixed (the "side effects to flag" case Blocker 1 itself names). (b) recency-guard alone (no concurrency block) for `ux-analysis.yml`/`e2e-video.yml` would still let a stale run's *other* steps (Axe, Lighthouse, video recording) execute to completion for no purpose once superseded — pure waste with no correctness benefit over just cancelling early, so the cheaper `concurrency:` fix is strictly better there. |
| GoF creational/structural/behavioral patterns (Strategy, Decorator, Factory, Observer, etc.) | **N/A — not applicable** | — | — | This is inline JS embedded in YAML `script:`/`run:` blocks operating on primitive strings/numbers/booleans passed through `env:`, not a typed application with object graphs. Forcing an OOP pattern onto a few lines of conditional logic would itself be the kind of speculative abstraction `.claude/rules/interface-pollution-checklist.md` flags — explicitly declining to force-fit one, per this item's Complexity-2 scoping. |
| PoEAA layering (Transaction Script / Domain Model / Repository / Service Layer / Unit of Work) | **N/A — not applicable** | — | — | No persistence layer, no transactions, no domain model exists or is warranted here — each gate is a single-request, single-script conditional. |
| Type-driven design (newtypes / sum types for domain concepts) | **N/A — not applicable** | — | — | JS booleans/numbers/strings passed via GitHub Actions `env:`/`outputs:` (which are always strings on the wire regardless) are the only representation GitHub Actions supports; there is no host language type system here to encode invariants into. |

---

## Migration Plan

N/A — no schema or data changes. This item only edits `.github/workflows/*.yml` and
`tools/ux-analysis/analyze.ts`.

## Observability Plan

- **Logs**: Existing `console.log` calls in each script are kept as-is. The single most
  important addition: write the raw comparison/signal output to `$GITHUB_STEP_SUMMARY`
  **unconditionally** (before the gate check, so it always executes) in the 5 sites that
  lack this today — `benchmark.yml`'s frontend-throughput and e2e-latency compare steps,
  and `ux-analysis.yml`/`build.yml`/`e2e-video.yml`'s comment scripts. Per pitfalls.md §1,
  the sticky PR comment is today the *only* place this information is surfaced for 4 of
  the 4 target workflows — a bug in the new gate that silently suppresses a comment would
  otherwise make a real regression completely invisible, not just quieter. `benchmark.yml`'s
  go-tier1 job already does this (lines 100-101) and needs no change.
- **Metrics**: None. No telemetry pipeline observes these CI scripts; out of scope.
- **Alerts**: None new. The existing job-level blocking checks (Axe failure, registry
  divergence >2%, "Check new RPCs have tests") remain the only alerting mechanism and are
  explicitly untouched by this item (AC #7).

## Risk Control

- **Feature flag**: N/A — no runtime flag applies to CI workflow YAML. The equivalent
  safeguard is landing each workflow's gate as its own independently revertable commit
  (build-vs-buy.md: "each of the 6 gate sites is independently reviewable and revertable").
  Per `.claude/rules/fix-flaky-tests-dont-defer.md`'s sibling collateral-debt discipline,
  the Phase 5 SHA-pinning drive-by ships as its own separate commit, not mixed into the
  gate-logic diffs.
- **Rollback procedure**: `git revert` the specific commit for the affected workflow file.
  Because Phases 1-5 below are scoped one-workflow-per-phase (Phase 1 covers 3 jobs within
  one file), a bad gate in `ux-analysis.yml` can be reverted without touching `benchmark.yml`,
  `build.yml`, or `e2e-video.yml`.
- **Staged rollout**: Land Phase 1 (`benchmark.yml`) first — it carries the highest
  parsing-fragility risk (pitfalls.md §4) — and validate it via a real scratch branch/test
  PR (push to a throwaway branch, open a PR, observe the Actions run) before merging.
  GitHub Actions has no offline linter for embedded `github-script` JS syntax (pitfalls.md
  §5); a whitespace slip in a YAML block-scalar edit surfaces only at runtime, and 3 of the
  4 target workflows currently lack `continue-on-error` on their comment step (mitigated by
  a task in each phase below, but validate live before depending on it). Repeat the
  scratch-PR validation independently for Phases 2-4 before each merges.
- **Out-of-order-run race (adversarial-review Blocker 1)**: delete-on-clean and update-in-place
  both mutate a shared PR comment based on a single run's view of the world — without a guard,
  an older/slower run finishing after a newer one can silently delete or overwrite a comment the
  newer run correctly posted, making a genuinely current finding invisible with no error and no
  reviewer-visible signal. Phases 2 and 4 (`ux-analysis.yml`, `e2e-video.yml`) close this with a
  `concurrency:` block (new Task 2.1.1e / 4.1.1d) that cancels a superseded run before it can
  reach the comment step at all. Phase 3 (`build.yml`) cannot safely use the same fix — the
  workflow also runs on `push` to `main` and has 5 other jobs unrelated to the comment step, so a
  workflow-level `concurrency:` block would cancel those too — and instead gets an explicit
  run-recency guard (new Task 3.1.1d) that skips only the comment mutation when superseded. See
  the new Pattern Decisions row for the full blast-radius reasoning.

## Unresolved Questions

1. **`deleteComment` permission scope is INFERRED, not verified** (pitfalls.md §5): no
   workflow in this repo has ever called it. It should work under the already-granted
   `pull-requests: write` (same Issues Comments API family as `createComment`/`updateComment`,
   which already work), but this has not been exercised against a real PR. **Must be
   confirmed via a real dry run** (a scratch `workflow_dispatch` step against a throwaway
   comment on a test PR, or simply observing Phase 1's first live scratch-PR run) before
   relying on it in production. If it 403s, fall back to ADR-001's "update-in-place to a
   ✅ Resolved body" alternative for the affected workflow(s) only.
2. **`ts-node` availability in CI is unconfirmed for the `findingsCount` signal.**
   `ux-analysis.yml:129`'s `if command -v ts-node >/dev/null 2>&1` guard means the Claude UX
   analysis step — and therefore `findings_count` — silently never runs if `ts-node` isn't on
   PATH in the CI image. Verify presence (via `.github/actions/prepare` or a preceding step)
   before assuming this signal will ever contribute to `isActionable` in practice; if it never
   fires, `ux-analysis.yml`'s gate still works correctly on Axe+Lighthouse alone.
3. **ADR-002's thresholds are proposals, not validated constants** (amended post adversarial
   review — see ADR-002 Amendment): go-tier1 reuses `build.yml`'s already-calibrated,
   significance-tested 20% blocking-gate number, but nobody has confirmed whether 20% is the
   right sensitivity for an *advisory* (non-blocking) comment specifically. `frontend-throughput`
   and `e2e-latency` no longer reuse that 20% number at all — they use an explicitly coarser,
   unvalidated "2x" (double/half) placeholder specifically because no repeated-trial data exists
   to calibrate a tighter number for single-sample Playwright measurements (see Pattern
   Decisions). Confirm go-tier1's 20% with repo owner before merge, or accept as a reasonable
   default pending real-world tuning; the two 2x thresholds should be revisited if/when a
   repeated-trials follow-up (noted in ADR-002 Consequences) makes tighter gating possible.
4. **Stretch, explicitly not in scope**: migrate Lighthouse/benchmark/coverage scored signals to
   GitHub Check Runs (`checks: write` + `github.rest.checks.create`). Named as a fast-follow in
   build-vs-buy.md Option 3 — structurally the better long-term home for these signals (no
   sticky-comment CRUD needed at all), but a real rewrite + new permission scope, correctly
   deferred per requirements.md's Out of Scope section.
5. **Stretch, explicitly not in scope**: extract the shared sticky-comment CRUD (~75 duplicated
   lines across 5 workflows) into a composite action. Named as a fast-follow in build-vs-buy.md
   Option 4 — orthogonal to this item's acceptance criteria, worth a backlog note if a 6th such
   workflow appears.
6. **`registry-validation.yml` itself is intentionally NOT modified** by this plan, per the
   task's explicit scoping notes — including its own latent "duplicate comment across pushes"
   issue (it has no dedup/marker at all, features.md §1). That issue is out of scope for this
   item; not resolved here.
7. **[Triad review, UX/DX lens] Fail-open vs. fail-closed asymmetry on a broken measurement is
   intentional but undocumented.** `benchmark.yml`'s 3 jobs fail *closed* (`hasRegression=false`,
   no comment) when a baseline is missing/broken; `ux-analysis.yml` and `build.yml` fail *open*
   (`isActionable=true`, comment posts) on the equivalent "couldn't measure" case (e.g.
   Lighthouse `NaN`). This is defensible — a missing benchmark baseline is the expected first-run
   state (not a failure worth flagging), while a Lighthouse crash mid-run is — but it should be a
   one-line comment in each gate module (Tasks 1.1.1e/2.1.1f) rather than an implicit asymmetry a
   future reader has to reverse-engineer. Non-blocking; add during implementation of those tasks.
8. **[Triad review, UX/DX lens] `$GITHUB_STEP_SUMMARY` is a pull channel, not a push
   notification.** Pre-mortem Failure #1's mitigation (write every signal to the step summary
   unconditionally) reduces but doesn't eliminate the risk that a gate false-negative makes a
   real regression invisible — a developer still has to open the Actions run to see it, unlike
   today's always-present PR comment. Accepted tradeoff for this item (the alternative, keeping
   comments always-on, is the exact noise problem being fixed); the extracted-module unit tests
   (Tasks 1.1.1e/2.1.1f/3.1.1e/4.1.1e) are the primary defense against that false-negative case
   actually occurring, not the step summary.
9. **[Triad review, UX/DX lens] Delete-on-clean comments carry no "this clears automatically"
   explanation, and `e2e-video.yml`'s normal-case comment (with convenience GIF-preview links)
   is demoted to step-summary-only.** Both are cheap to address during implementation (a one-line
   sentence in the posted comment body for the former; confirming Task 4.1.1a's step-summary
   write preserves the same clickable links for the latter, per architecture-review's existing
   Nitpick on this) but neither blocks planning — noted here so they aren't silently dropped
   before Phase 5 implementation.

## Dependency Visualization

```
Phase 1: benchmark.yml (3 independent epics — no shared code between them)
  Epic 1.1 go-tier1: 1.1.1a → 1.1.1b → 1.1.1c → 1.1.1d → 1.1.1e (extract+pin, can run after 1.1.1b)  ─┐
  Epic 1.2 frontend-throughput: 1.2.1a → 1.2.1b → 1.2.1c                                              ─┼─  parallelizable
  Epic 1.3 e2e-latency: 1.3.1a → 1.3.1b → 1.3.1c                                                      ─┘

Phase 2: ux-analysis.yml (independent of Phase 1)
  2.1.1a (analyze.ts: emit findings_count)
        └─▶ 2.1.1b (workflow: thread findings_count into comment step env)
              └─▶ 2.1.1c (restructure gate: hoist listComments, isActionable, delete-on-clean)
                    ├─▶ 2.1.1d (continue-on-error: true)
                    ├─▶ 2.1.1e (concurrency: block — resolves adversarial Blocker 1)
                    └─▶ 2.1.1f (extract isActionable into tools/ci-gates/ux-actionability.js + test)

Phase 3: build.yml (independent of Phases 1-2)
  3.1.1a (parse currentCoveragePct as a number)
        └─▶ 3.1.1d (recency guard — resolves adversarial Blocker 1, must run before 3.1.1b's comparison)
              └─▶ 3.1.1b (restructure gate: hoist listComments, parse previousCoveragePct, compare)
                    ├─▶ 3.1.1c (step-summary write, unconditional)
                    └─▶ 3.1.1e (marker-based pct encoding + extract coverage-delta.js + test)

Phase 4: e2e-video.yml (independent of Phases 1-3)
  4.1.1a (hoist listComments, compute videoAnomaly, step-summary write)
        └─▶ 4.1.1b (restructure: normal-case branch deletes stale comment + returns; anomaly branch unchanged)
              ├─▶ 4.1.1c (continue-on-error: true)
              ├─▶ 4.1.1d (concurrency: block — resolves adversarial Blocker 1; was missing until Phase 4 validation)
              └─▶ 4.1.1e (extract videoAnomaly into tools/ci-gates/video-anomaly.js + test)

Phase 5: Drive-by — SHA-pin actions/github-script (ux-analysis.yml, build.yml, e2e-video.yml only;
                     NOT registry-validation.yml, which this item does not touch)
  Depends on: Phases 2, 3, 4 landing first (touches the same `uses:` lines their gate
  restructuring edits — sequencing avoids unrelated merge churn on the same lines).
  Ships as its OWN commit/PR per collateral-debt convention, not squashed into the gate diffs.

Phase 6: Cross-cutting verification (AC #6, AC #7 — no new workflow code, validates the whole set)
  Depends on: Phases 1-5 all merged.
  6.1.1a (scratch-PR run) ─┐
  6.1.2a (diff-review)     ┼─  parallelizable
  6.1.2b (grep for the 2 concurrency: blocks)  ┘
  6.1.2c (tools/ci-gates/__tests__/workflow-config-invariants.test.js — independent of 6.1.1a/b, runs anytime after Phase 2/3 merge)

All phases → validated via a scratch-branch/test-PR live run (pitfalls.md §5) before each merges.
```

---

## Phase 1: `benchmark.yml` — gate all 3 comment jobs on a real regression signal

### Epic 1.1: go-tier1 job
**Goal**: `benchmark.yml`'s go-tier1 comment only posts when `benchstat` shows a ≥20%
regression, and a cleared regression removes the stale comment.

#### Story 1.1.1: Gate the go-tier1 PR comment on `hasRegression`
**As a** PR author, **I want** the Go Tier-1 benchmark comment to only appear when there's a
real regression, **so that** I'm not shown a benchmark dump on every PR that touches Go code.
**Acceptance Criteria**:
- AC #1 (regression-only posting):
  - *Given* the "Compare against baseline" step produces `benchstat-tier1.txt` containing a
    row `BenchmarkFoo-8   100ns ± 2%   130ns ± 3%   +30.00%  (p=0.002 n=8+8)`, *When* the
    "Post PR comment" step runs on a `pull_request` event, *Then* `hasRegression` evaluates
    `true` (30% ≥ the 20% threshold) and the step calls `createComment` (or `updateComment` if
    a marker comment already exists) with a body containing that regression row.
  - *Given* `benchstat-tier1.txt` shows only `~` (statistically insignificant) deltas for every
    row, *When* the step runs, *Then* `hasRegression` evaluates `false` and no `createComment`/
    `updateComment` call is made.
- AC #2 (stale comment cleanup):
  - *Given* a marker `<!-- benchmark-go-tier1 -->` comment already exists on the PR reading
    "⚠️ Regression Detected: BenchmarkFoo +30%" from a previous push, *When* the current push's
    `hasRegression` evaluates `false` (the regression was fixed), *Then* the step calls
    `github.rest.issues.deleteComment({ owner, repo, comment_id: existing.id })` and the PR no
    longer shows that comment.
**Files**: `.github/workflows/benchmark.yml`

##### Task 1.1.1a: Add `-delta-test=utest` to the go-tier1 benchstat comparison (~2 min)
- In the "Compare against baseline (PR only)" step (currently around line 99), change
  `benchstat benchmarks/go/tier1-baseline.txt tier1-bench.txt > benchstat-tier1.txt 2>&1 || true`
  to `benchstat -delta-test=utest benchmarks/go/tier1-baseline.txt tier1-bench.txt > benchstat-tier1.txt 2>&1 || true`.
  This is the same flag `build.yml`'s `benchmark-gate` job already uses for statistical
  significance filtering.
- Files: `.github/workflows/benchmark.yml`

##### Task 1.1.1b: Compute `has_regression` output on the same step (~4 min)
- Add `id: compare` to the "Compare against baseline (PR only)" step (it currently has no id).
- After the `benchstat` line, append (still inside the `if [ -f ... ]` branch):
  ```bash
  if grep -E '\+([2-9][0-9]|[1-9][0-9]{2,})\.' benchstat-tier1.txt | grep -v '±' > /dev/null; then
    echo "has_regression=true" >> "$GITHUB_OUTPUT"
  else
    echo "has_regression=false" >> "$GITHUB_OUTPUT"
  fi
  ```
  In the `else` branch (no baseline yet), also add `echo "has_regression=false" >> "$GITHUB_OUTPUT"`.
- Files: `.github/workflows/benchmark.yml`

##### Task 1.1.1c: Restructure the go-tier1 comment step (hoist lookup, gate, delete-on-clean) (~5 min)
- Add `env: HAS_REGRESSION: ${{ steps.compare.outputs.has_regression }}` to the "Post PR
  comment" step.
- Rewrite the script body to: build `marker`/`output` as today → call `listComments` and find
  `existing` → `if (process.env.HAS_REGRESSION !== 'true') { if (existing) { await
  github.rest.issues.deleteComment({owner, repo, comment_id: existing.id}); } return; }` →
  only then build `body` and do the existing `existing ? updateComment : createComment` branch.
- Files: `.github/workflows/benchmark.yml`

##### Task 1.1.1d: Add `continue-on-error: true` to the go-tier1 comment step (~1 min)
- The step currently has no `continue-on-error`; add it, consistent with
  `registry-validation.yml:74`'s "advisory comment; never fail the job if posting is denied."
- Files: `.github/workflows/benchmark.yml`

##### Task 1.1.1e: Pin benchstat's version and extract the regression regex into a unit-tested module (~15 min)
**[Added post-validation — pre-mortem.md Failure #5 / P1, architecture-review Concern #1]**
The go-tier1 `hasRegression` regex (Task 1.1.1b) is, before this task, testable only via a live
scratch-PR push — pitfalls.md names a silently-matching-nothing gate as this item's single
highest-risk failure mode, and this is the one place in the plan that logic actually lands
untested.
- Confirm `benchstat`'s install step pins an exact version/commit (not `@latest`); if it floats,
  pin it so a future benchstat release can't silently change the `~`/significance-marker text
  format this regex depends on.
- Extract the `has_regression` decision (Task 1.1.1b's `grep -E` logic) into
  `tools/ci-gates/benchmark-regression.js`, exporting a pure function
  `hasRegression(benchstatOutput: string, thresholdPct: number): boolean` that the workflow step
  invokes via `node -e "require('./tools/ci-gates/benchmark-regression.js')..."` (or an inline
  `require()` inside the existing `github-script` block) instead of re-implementing the regex
  inline.
- Add `tools/ci-gates/benchmark-regression.test.js` (Node's built-in `node:test`, no new
  dependency) with fixtures covering: a `~`-only (insignificant) output → `false`; an
  exactly-at-threshold delta (e.g. `+20.00%`) → boundary behavior explicitly asserted either way;
  a clear `+30.00%` regression → `true`; and a missing-baseline / empty-input case → `false`.
- Files: `.github/workflows/benchmark.yml`, `tools/ci-gates/benchmark-regression.js` (new),
  `tools/ci-gates/benchmark-regression.test.js` (new)

---

### Epic 1.2: frontend-throughput job
**Goal**: Gate on an "obvious swing" (throughput halved or worse), reusing the pct/arrow math
already computed inline. Per ADR-002's amendment (adversarial-review Blocker 2), this job does
**not** reuse go-tier1's 20% threshold — it's a single-sample measurement with no repeated
trials or significance test, so it uses a much coarser 2x threshold instead (see Pattern
Decisions: "benchmark.yml frontend-throughput / e2e-latency regression detection").

#### Story 1.2.1: Gate the frontend-throughput PR comment on `hasRegression`
**As a** PR author, **I want** the frontend throughput comment to only appear on an obvious
throughput drop, **so that** routine PRs don't get a benchmark dump and single-sample noise
doesn't trigger a false alarm.
**Acceptance Criteria**:
- AC #1: *Given* the Node compare script computes a row with `pctNum = -60` (throughput dropped
  to less than half of baseline), *When* the "Post PR comment" step runs, *Then* `hasRegression`
  is `true` (`-60 <= -50`) and the comment posts/updates.
  - *Given* all rows have `pctNum` between -49 and +49, *When* the step runs, *Then*
    `hasRegression` is `false` and no comment is posted — including a single noisy sample
    anywhere in the 20-49% range, which this job deliberately does not flag (ADR-002 amendment:
    no noise-floor data exists to justify a tighter threshold for a single-sample measurement).
- AC #2: *Given* a marker `<!-- benchmark-frontend-throughput -->` comment exists from a prior
  regression, *When* the current run's `hasRegression` is `false`, *Then* `deleteComment` is
  called and the comment is removed.
**Files**: `.github/workflows/benchmark.yml`

##### Task 1.2.1a: Compute `hasRegression` + step-summary in the compare step (~5 min)
- Add `id: compare` to the "Compare against baseline (PR only)" step (frontend-throughput job,
  currently ~line 310, no id today).
- In the inline `node -e "..."` script, alongside the existing `pct`/`arrow`/`rows` computation,
  also track a raw numeric `pctNum = (c.value - b.value) / b.value * 100` per row (not just the
  `.toFixed(1)` display string) and compute `const hasRegression = current.some((c, i) => { ... pctNum <= -50 })`
  — **-50, not -20**: per ADR-002's amendment this job uses the coarser 2x/halved threshold, not
  go-tier1's 20% (skip rows with no baseline match). After writing `throughput-comparison.txt`, also:
  ```js
  const fence = String.fromCharCode(96,96,96);
  require('fs').appendFileSync(process.env.GITHUB_OUTPUT, 'has_regression=' + hasRegression + '\n');
  require('fs').appendFileSync(process.env.GITHUB_STEP_SUMMARY, '## Frontend Terminal Throughput\n' + fence + '\n' + rows.join('\n') + '\n' + fence + '\n');
  ```
  **Important**: avoid JS template literals (backtick strings) and literal backtick characters
  anywhere in this script — unlike the `github-script` steps, this script is the argument to
  `node -e "..."` inside a bash `run: |` block, and a literal backtick inside a double-quoted
  bash string triggers command substitution before Node ever sees it. Build the markdown fence
  via `String.fromCharCode(96,96,96)` and use `+` string concatenation, matching the existing
  script's own convention (it already avoids template literals for exactly this reason).
  In the `else` branch (no baseline), also emit `has_regression=false` to `$GITHUB_OUTPUT`.
- Files: `.github/workflows/benchmark.yml`

##### Task 1.2.1b: Restructure the frontend-throughput comment step (~5 min)
- Same restructuring as Task 1.1.1c: add `env: HAS_REGRESSION: ${{ steps.compare.outputs.has_regression }}`,
  hoist `listComments`/`existing` above the gate, `if (!hasRegression) { delete existing if
  present; return; }`, else build body and update/create.
- Files: `.github/workflows/benchmark.yml`

##### Task 1.2.1c: Add `continue-on-error: true` to the frontend-throughput comment step (~1 min)
- Files: `.github/workflows/benchmark.yml`

---

### Epic 1.3: e2e-latency job
**Goal**: Same gating behavior as frontend-throughput, with the opposite regression direction
(higher latency = worse). Per ADR-002's amendment, this job also uses the coarser 2x threshold,
not go-tier1's 20% — see Epic 1.2's Goal for the shared rationale.

#### Story 1.3.1: Gate the e2e-latency PR comment on `hasRegression`
**As a** PR author, **I want** the E2E RPC latency comment to only appear on an obvious latency
increase, **so that** routine PRs don't get a benchmark dump and single-sample noise doesn't
trigger a false alarm.
**Acceptance Criteria**:
- AC #1: *Given* the Node compare script computes a row with `pctNum = +110` (latency more than
  doubled), *When* the "Post PR comment" step runs, *Then* `hasRegression` is `true`
  (`110 >= 100`) and the comment posts/updates.
  - *Given* all rows have `pctNum` between -49 and +99, *When* the step runs, *Then*
    `hasRegression` is `false` and no comment posts — including a single noisy sample anywhere
    up to +99%, which this job deliberately does not flag (ADR-002 amendment).
- AC #2: *Given* a marker `<!-- benchmark-e2e-latency -->` comment exists from a prior
  regression, *When* the current run's `hasRegression` is `false`, *Then* `deleteComment` is
  called.
**Files**: `.github/workflows/benchmark.yml`

##### Task 1.3.1a: Compute `hasRegression` + step-summary in the compare step (~5 min)
- Add `id: compare` to the "Compare against baseline (PR only)" step (e2e-latency job, currently
  ~line 524, no id today).
- Same pattern as Task 1.2.1a, but `hasRegression = current.some((c,i) => pctNum >= 100)`
  — **100, not 20**: latency doubling (≥100%) is the regression direction here — opposite sign
  from throughput's halving — per ADR-002's amended 2x threshold for this single-sample job.
  Emit `has_regression` to `$GITHUB_OUTPUT` and the raw comparison to `$GITHUB_STEP_SUMMARY`
  unconditionally (both branches).
- Files: `.github/workflows/benchmark.yml`

##### Task 1.3.1b: Restructure the e2e-latency comment step (~5 min)
- Same restructuring as Task 1.1.1c/1.2.1b.
- Files: `.github/workflows/benchmark.yml`

##### Task 1.3.1c: Add `continue-on-error: true` to the e2e-latency comment step (~1 min)
- Files: `.github/workflows/benchmark.yml`

---

## Phase 2: `ux-analysis.yml` — gate on Axe/Lighthouse/Claude-UX actionability

### Epic 2.1: UX analysis PR comment
**Goal**: The UX analysis comment only posts when Axe fails, Lighthouse drops below 70 or fails
to measure, or Claude UX analysis reports ≥1 finding — not as an always-present status table.

#### Story 2.1.1: Gate the UX analysis PR comment on `isActionable`
**As a** PR author, **I want** the UX analysis comment to only appear when something needs my
attention, **so that** a fully green PR doesn't get a status table I have to re-verify is boring.
**Acceptance Criteria**:
- AC #3 (actionable-only posting), three sub-cases:
  - *Given* `steps.axe.outcome === 'success'`, `LIGHTHOUSE_SCORE = '85'`, `findingsCount = 0`,
    *When* the gate runs, *Then* `isActionable` is `false` and no comment is posted (any prior
    marker comment is deleted).
  - *Given* `LIGHTHOUSE_SCORE = '62'` (< 70) with Axe passing and `findingsCount = 0`, *When*
    the gate runs, *Then* `isActionable` is `true` and the comment posts, showing the
    Lighthouse row flagged.
  - *Given* `LIGHTHOUSE_SCORE = 'unknown'` (Lighthouse extraction failed, `parseInt` → `NaN`),
    *When* the gate runs, *Then* `lighthouseParseFailed` is computed via an explicit
    `isNaN(score)` check (not `score < 70` alone) and is `true`, so `isActionable` is `true`
    even though `NaN < 70` would itself evaluate `false` — the comment posts noting the score
    is unavailable.
**Files**: `.github/workflows/ux-analysis.yml`, `tools/ux-analysis/analyze.ts`

##### Task 2.1.1a: Emit `findings_count` from `analyze.ts` (~3 min)
- In `tools/ux-analysis/analyze.ts`'s `main()`, after the existing findings-summary
  `console.log` block (around line 268, right before the closing `}`), add:
  ```ts
  // Machine-readable count for CI gating; no-op when run outside GitHub Actions.
  if (process.env.GITHUB_OUTPUT) {
    fs.appendFileSync(process.env.GITHUB_OUTPUT, `findings_count=${findings.length}\n`);
  }
  ```
- Files: `tools/ux-analysis/analyze.ts`

##### Task 2.1.1b: Thread `findings_count` into the comment step's env (~2 min)
- The workflow step already has `id: screenshots` (line 112). Add
  `UX_FINDINGS_COUNT: ${{ steps.screenshots.outputs.findings_count }}` to the "Post UX analysis
  PR comment" step's `env:` block (alongside the existing `LIGHTHOUSE_SCORE`).
- Files: `.github/workflows/ux-analysis.yml`

##### Task 2.1.1c: Restructure the comment step's gate (hoist lookup, `isActionable`, delete-on-clean) (~5 min)
- Hoist `listComments`/`existing` lookup above the actionability computation.
- Compute:
  ```js
  const score = parseInt(lighthouseScore, 10);
  const lighthouseParseFailed = isNaN(score);
  const findingsCount = Number(process.env.UX_FINDINGS_COUNT || 0);
  const isActionable = axeResult !== 'success' || lighthouseParseFailed || score < 70 || findingsCount > 0;
  ```
  Write the existing table unconditionally to `$GITHUB_STEP_SUMMARY` (before the gate, so it's
  always visible per the Observability Plan). Then `if (!isActionable) { delete existing if
  present; return; }`, else build `body` (unchanged content) and update/create as today.
- Files: `.github/workflows/ux-analysis.yml`

##### Task 2.1.1d: Add `continue-on-error: true` to the comment step (~1 min)
- Currently missing on this step (pitfalls.md §3 confirmed).
- Files: `.github/workflows/ux-analysis.yml`

##### Task 2.1.1e: Add a `concurrency:` block to guard delete-on-clean against out-of-order runs (~2 min)
- Resolves adversarial-review Blocker 1 (see Pattern Decisions: "Race protection for
  delete-on-clean / stale mutations"). Without this, an older/slower run finishing after a
  newer one can delete or overwrite a comment the newer run correctly posted, silently making a
  genuinely current finding invisible.
- `ux-analysis.yml` is `pull_request`-only with a single job, so a workflow-level block is safe
  (no unrelated jobs to cancel). Add near the top of the file, after `on:` and before
  `permissions:`, mirroring `benchmark.yml:36-40`'s existing shape but scoped to this workflow
  and PR number:
  ```yaml
  concurrency:
    group: ux-analysis-${{ github.event.pull_request.number }}
    cancel-in-progress: true
  ```
- Files: `.github/workflows/ux-analysis.yml`

##### Task 2.1.1f: Extract `isActionable` into a unit-tested module (~10 min)
**[Added post-validation — pre-mortem.md Failure #1 / P1, architecture-review Concern #1]**
- Extract Task 2.1.1c's `isActionable` computation into
  `tools/ci-gates/ux-actionability.js`, exporting
  `isActionable({ axeOutcome, lighthouseScore, findingsCount }): boolean`.
- Add `tools/ci-gates/ux-actionability.test.js` covering: Axe failing → `true`; Lighthouse score
  `'unknown'` (NaN) → `true` (not silently `false`, per Story 2.1.1's `lighthouseParseFailed`
  case); Lighthouse `62` → `true`; Lighthouse `85` + `findingsCount=0` + Axe passing → `false`;
  `findingsCount>0` alone → `true`.
- Files: `.github/workflows/ux-analysis.yml`, `tools/ci-gates/ux-actionability.js` (new),
  `tools/ci-gates/ux-actionability.test.js` (new)

---

## Phase 3: `build.yml` — gate feature-coverage comment on a delta vs. prior state

### Epic 3.1: Feature-coverage PR comment
**Goal**: The comment only posts/updates when the feature-coverage percentage changed from the
last value posted to this PR, not unconditionally on every run.

#### Story 3.1.1: Gate the feature-coverage comment on `currentCoveragePct !== previousCoveragePct`
**As a** PR author, **I want** the feature-coverage comment to only appear when coverage
actually moved, **so that** an unchanged number doesn't reappear on every push.
**Acceptance Criteria**:
- AC #4:
  - *Given* an existing sticky comment body contains `"...tested (42.0%)"` so
    `previousCoveragePct = 42.0`, and the current run's `steps.coverage.outputs.pct = "48.3"`,
    *When* the gate runs, *Then* `currentCoveragePct (48.3) !== previousCoveragePct (42.0)` so
    `isActionable` is `true` and the comment updates to the new value.
  - *Given* `previousCoveragePct = 42.0` and `currentCoveragePct = 42.0` (unchanged), *When*
    the gate runs, *Then* `isActionable` is `false`; no update is made and the existing
    comment (if any) is left untouched — **not deleted**, since an unchanged number is not a
    resolved problem the way a cleared regression is (see Pattern Decisions).
  - *Given* no existing marker comment (first run on this PR), *When* the gate runs, *Then*
    `isActionable` is `true` unconditionally (there is no prior state to compare against) and
    the comment is created.
  - *Given* this run's commit is no longer the PR's current HEAD (a newer push started and/or
    finished first), *When* the gate runs, *Then* the recency guard (Task 3.1.1d) short-circuits
    before the `isActionable` comparison and no mutation happens, regardless of what
    `isActionable` would have evaluated to — resolves adversarial-review Blocker 1 for this
    workflow (see Pattern Decisions: "Race protection for delete-on-clean / stale mutations").
**Files**: `.github/workflows/build.yml`

##### Task 3.1.1a: Extract a numeric `pct` output alongside the existing `summary` string (~3 min)
- In the "Generate feature E2E coverage report" step (`id: coverage`), after the existing
  `SUMMARY=$(echo "$OUTPUT" | grep 'Feature E2E coverage:' ...)` line, add:
  ```bash
  PCT=$(echo "$OUTPUT" | grep -oE '\([0-9.]+%\)' | tr -d '()%' | head -1)
  echo "pct=${PCT:-}" >> "$GITHUB_OUTPUT"
  ```
- Files: `.github/workflows/build.yml`

##### Task 3.1.1b: Restructure the comment step's gate (hoist lookup, parse `previousCoveragePct`, compare) (~5 min)
- Add `env: COVERAGE_PCT: ${{ steps.coverage.outputs.pct }}` to the "Post feature coverage PR
  comment" step.
- Hoist `listComments`/`existing` above body construction. Compute:
  ```js
  const currentCoveragePct = parseFloat(process.env.COVERAGE_PCT);
  const prevMatch = existing ? existing.body.match(/\((\d+(?:\.\d+)?)%\)/) : null;
  const previousCoveragePct = prevMatch ? parseFloat(prevMatch[1]) : null;
  const isActionable = !existing || previousCoveragePct === null || isNaN(currentCoveragePct) || currentCoveragePct !== previousCoveragePct;
  ```
  `if (!isActionable) { return; }` (no delete — see Story 3.1.1's third AC), else build `body`
  (unchanged content) and update/create as today. Place this **after** Task 3.1.1d's recency
  guard runs, so a superseded run never reaches this comparison at all.
- Files: `.github/workflows/build.yml`

##### Task 3.1.1c: Write the coverage summary to `$GITHUB_STEP_SUMMARY` unconditionally (~2 min)
- Inside the same script, before the `isActionable` gate, add
  `require('fs').appendFileSync(process.env.GITHUB_STEP_SUMMARY, '## Feature E2E Coverage\n' + summary + '\n')`
  so the number remains visible even when the comment is suppressed. `continue-on-error: true`
  already exists on this step (line 247) — no change needed there.
- Files: `.github/workflows/build.yml`

##### Task 3.1.1d: Add a run-recency guard before mutating the comment (~4 min)
- Resolves adversarial-review Blocker 1 for `build.yml` specifically. Unlike `ux-analysis.yml`/
  `e2e-video.yml` (Tasks 2.1.1e/4.1.1d), `build.yml` does **not** get a workflow-level
  `concurrency:` block — it also runs on `push` to `main` and has 5 other jobs (`prepare`,
  `web-build-smoke`, `test`, `build`, `install-check`, `benchmark-gate`) that a workflow-level
  cancel would take down too, a much larger blast radius than fixing the comment race (see
  Pattern Decisions). Instead, guard only the comment mutation itself.
- At the very top of the "Post feature coverage PR comment" script (before the `listComments`
  call from Task 3.1.1b), add:
  ```js
  const pr = await github.rest.pulls.get({
    owner: context.repo.owner,
    repo: context.repo.repo,
    pull_number: context.payload.pull_request.number,
  });
  if (pr.data.head.sha !== context.payload.pull_request.head.sha) {
    core.info('Skipping feature-coverage comment: a newer push has superseded this run.');
    return;
  }
  ```
  This compares the PR's **current** HEAD SHA (fetched live, at execution time) against the SHA
  this specific run was triggered for (`context.payload.pull_request.head.sha`, fixed at
  trigger time) — if they differ, a newer push has already superseded this run, so skip the
  mutation entirely rather than risk overwriting a newer run's more current comment.
- Files: `.github/workflows/build.yml`

##### Task 3.1.1e: Encode `previousCoveragePct` in the marker and extract the comparison into a unit-tested module (~10 min)
**[Added post-validation — pre-mortem.md Failure #1 / P1, architecture-review Concern #2]**
- Switch the marker from the bare `<!-- feature-coverage -->` to
  `<!-- feature-coverage:pct=NN.N -->` so `previousCoveragePct` is parsed from a format this
  script owns exclusively, not scraped from the free-form summary sentence (architecture-review
  Concern #2 — a future comment-copy edit could otherwise silently break or misparse the regex).
- Extract Task 3.1.1b's comparison into `tools/ci-gates/coverage-delta.js`, exporting
  `isActionable({ existingBody, currentPct }): boolean` that parses `previousCoveragePct` via the
  anchored marker regex (`/feature-coverage:pct=([\d.]+)/`) internally.
- Add `tools/ci-gates/coverage-delta.test.js` covering: no existing comment → `true`; unchanged
  pct → `false`; changed pct → `true`; malformed/missing marker in an existing comment → `true`
  (safe fallback, matches the plan's existing null-handling).
- Files: `.github/workflows/build.yml`, `tools/ci-gates/coverage-delta.js` (new),
  `tools/ci-gates/coverage-delta.test.js` (new)

---

## Phase 4: `e2e-video.yml` — narrow the notify comment to the anomaly case

### Epic 4.1: E2E video notify comment
**Goal**: The comment only posts when zero video artifacts were produced (an actionable
anomaly), not on the normal case where videos were produced as expected.

#### Story 4.1.1: Gate the notify comment on `videoAnomaly`
**As a** PR author, **I want** the E2E video comment to only appear when something went wrong,
**so that** normal runs don't get a pure-navigation comment I never needed to click into.
**Acceptance Criteria**:
- AC #5:
  - *Given* `videoArtifacts.length === 0` (anomaly — expected videos but got none), *When* the
    `notify` job runs, *Then* the comment posts with the existing "⚠️ No video artifacts were
    produced" body (unchanged content/behavior for this branch).
  - *Given* `videoArtifacts.length === 3` (normal case, videos produced), *When* the `notify`
    job runs, *Then* no comment is posted, and if a marker `<!-- e2e-videos -->` comment exists
    from an earlier anomalous run on this PR (now fixed), it is deleted.
**Files**: `.github/workflows/e2e-video.yml`

##### Task 4.1.1a: Hoist the comment lookup and compute `videoAnomaly` before building `lines` (~4 min)
- Move the `listComments`/`existing`-find block (currently after `lines` is built) to
  immediately after `videoArtifacts` is computed, before the `if (videoArtifacts.length === 0)`
  branch. Rename that condition's boolean to `const videoAnomaly = videoArtifacts.length === 0;`
  for clarity per the Domain Glossary.
- Add an unconditional `$GITHUB_STEP_SUMMARY` write of the shard/artifact list right after
  `videoArtifacts` is computed (mirrors go-tier1's existing pattern; nothing writes to step
  summary in this file today).
- Files: `.github/workflows/e2e-video.yml`

##### Task 4.1.1b: Restructure branches — normal case deletes-and-returns, anomaly case unchanged (~4 min)
- Replace the `if (videoArtifacts.length === 0) { lines = [...] } else { lines = [...] }`
  shape with:
  ```js
  if (!videoAnomaly) {
    if (existing) {
      await github.rest.issues.deleteComment({ owner: context.repo.owner, repo: context.repo.repo, comment_id: existing.id });
    }
    return;
  }
  const lines = [ /* existing anomaly-branch content, unchanged */ ];
  const body = [marker, ...lines].join('\n');
  // existing ? updateComment : createComment, unchanged
  ```
- Files: `.github/workflows/e2e-video.yml`

##### Task 4.1.1c: Add `continue-on-error: true` to the notify step (~1 min)
- Currently missing (pitfalls.md §3 confirmed).
- Files: `.github/workflows/e2e-video.yml`

##### Task 4.1.1d: Add a `concurrency:` block to guard delete-on-clean against out-of-order runs (~2 min)
**[Added post-validation — pre-mortem.md Failure #2 / P1]** Resolves adversarial-review
Blocker 1 for this workflow. Pattern Decisions and Risk Control both already assumed this task
existed under this exact number; it did not — this closes that gap so `e2e-video.yml` actually
gets the same race protection `ux-analysis.yml` gets via Task 2.1.1e, not just a doc reference to
one.
- `e2e-video.yml` is `pull_request`-only with a single `notify` job of consequence to this item,
  so a workflow-level block is safe (no unrelated jobs to cancel), same reasoning as Task 2.1.1e.
  Add near the top of the file, after `on:` and before `permissions:`:
  ```yaml
  concurrency:
    group: e2e-video-${{ github.event.pull_request.number }}
    cancel-in-progress: true
  ```
- Files: `.github/workflows/e2e-video.yml`

##### Task 4.1.1e: Extract `videoAnomaly` into a unit-tested module (~8 min)
**[Added post-validation — triad-review engineering-lens gap]** Mirrors Tasks 1.1.1e/2.1.1f/
3.1.1e so all four workflows' gate decisions are consistently unit-testable, per
architecture-review's Concern #1 remediation, and closes `validation.md`'s reference to
`tools/ci-gates/video-anomaly.js` with a real task backing it.
- Extract `videoAnomaly = videoArtifacts.length === 0` into `tools/ci-gates/video-anomaly.js`,
  exporting `videoAnomaly(artifacts: string[]): boolean`.
- Add `tools/ci-gates/video-anomaly.test.js` covering: empty array → `true`; non-empty array →
  `false`.
- Files: `.github/workflows/e2e-video.yml`, `tools/ci-gates/video-anomaly.js` (new),
  `tools/ci-gates/video-anomaly.test.js` (new)

---

## Phase 5: Drive-by — SHA-pin `actions/github-script` (separate commit)

### Epic 5.1: Consistency pin, no behavior change
**Goal**: Pin `actions/github-script` to the SHA `benchmark.yml` already uses, in the 3 files
this item's Phases 2-4 already edit — closing a pre-existing pinning inconsistency flagged in
stack.md §1. Per `.claude/rules/fix-flaky-tests-dont-defer.md`'s collateral-debt sibling
discipline, this ships as **its own commit/PR**, not folded into the gate-logic diffs above.
`registry-validation.yml` is explicitly out of this item's scope and is NOT touched (per the
task's scoping notes).

#### Story 5.1.1: Replace floating `@v7` with the pinned SHA already used in `benchmark.yml`
**As a** maintainer, **I want** all `actions/github-script` uses pinned to the same commit SHA,
**so that** a future upstream tag mutation can't silently change behavior in 3 of 4 files.
**Acceptance Criteria**:
- *Given* `benchmark.yml` already pins `actions/github-script@f28e40c7f34bde8b3046d885e986cb6290c5673b # v7`
  in all 3 of its comment steps, *When* this story lands, *Then* `ux-analysis.yml:151`,
  `build.yml:218`, and `e2e-video.yml:246` each use the identical SHA pin, and no workflow
  behavior changes (same resolved major version, `v7`, before and after).
**Files**: `.github/workflows/ux-analysis.yml`, `.github/workflows/build.yml`, `.github/workflows/e2e-video.yml`

##### Task 5.1.1a: Pin `ux-analysis.yml` (~1 min)
- Change `uses: actions/github-script@v7` (line 151) to
  `uses: actions/github-script@f28e40c7f34bde8b3046d885e986cb6290c5673b # v7`.
- Files: `.github/workflows/ux-analysis.yml`

##### Task 5.1.1b: Pin `build.yml` (~1 min)
- Change `uses: actions/github-script@v7` (line 218) to the same SHA pin.
- Files: `.github/workflows/build.yml`

##### Task 5.1.1c: Pin `e2e-video.yml` (~1 min)
- Change `uses: actions/github-script@v7` (line 246) to the same SHA pin.
- Files: `.github/workflows/e2e-video.yml`

---

## Phase 6: Cross-cutting verification (no new code)

### Epic 6.1: Validate the whole-set acceptance criteria
**Goal**: Confirm AC #6 (green PR → near-zero comments) and AC #7 (no blocking-check behavior
change) hold across all 4 workflows together, not just per-workflow.

#### Story 6.1.1: A fully green PR produces zero advisory comments
**As a** reviewer, **I want** a clean PR to be free of advisory comment noise across all 4
workflows, **so that** the fix's actual goal (AC #6) is verified end-to-end, not just per-file.
**Acceptance Criteria**:
- AC #6: *Given* a scratch PR where: `benchmark.yml`'s 3 jobs all show `hasRegression = false`,
  `ux-analysis.yml` shows Axe passing + Lighthouse ≥ 70 + `findingsCount = 0`, `build.yml`
  shows `currentCoveragePct === previousCoveragePct`, and `e2e-video.yml` shows
  `videoArtifacts.length > 0`, *When* all 4 workflows run on that PR, *Then* zero new advisory
  comments appear from these workflows (any pre-existing stale comments from an earlier "bad"
  push on the same PR are deleted per Phases 1/2/4; `build.yml`'s comment, if any existed and
  coverage is unchanged, is left as-is per Phase 3's explicit no-delete decision).
**Files**: `.github/workflows/benchmark.yml`, `.github/workflows/ux-analysis.yml`, `.github/workflows/build.yml`, `.github/workflows/e2e-video.yml` (verification only — no edits)

##### Task 6.1.1a: Run a scratch-branch PR after Phases 1-5 merge and observe comment behavior (~5 min)
- Open a throwaway PR against a branch that doesn't trigger any real regression, confirm no new
  comments appear from any of the 4 workflows, then push a change that does trigger one signal
  (e.g. a deliberately slow benchmark) and confirm exactly one comment appears and later clears.
- Files: none (manual verification step)

#### Story 6.1.2: Existing blocking checks are unaffected
**As a** maintainer, **I want** Axe/registry-divergence/RPC-test-coverage failures to still
block exactly as before, **so that** this item's comment-gating changes don't accidentally
loosen an existing quality gate (AC #7).
**Acceptance Criteria**:
- AC #7: *Given* a PR with an Axe critical/serious violation, *When* `ux-analysis.yml` runs,
  *Then* the job still fails (`continue-on-error: false` on the Axe step, untouched by any
  task above) exactly as it did before this item.
**Files**: `.github/workflows/ux-analysis.yml`, `.github/workflows/build.yml`, `.github/workflows/registry-validation.yml` (verification only — no edits; `registry-validation.yml` is out of scope and untouched)

##### Task 6.1.2a: Diff-review confirming no blocking-check lines were touched (~3 min)
- Review the final diff across Phases 1-5 and confirm none of it touches: `ux-analysis.yml`'s
  Axe step (`continue-on-error: false`), `build.yml`'s "Check new RPCs have tests" step, or any
  line in `registry-validation.yml`.
- Files: none (review step)

##### Task 6.1.2b: Grep the merged diff for all three new `concurrency:` blocks (~2 min)
**[Added post-validation — pre-mortem.md Failure #2 / P1]** Closes the specific doc/task-list
mismatch this validation pass found (Risk Control/Pattern Decisions referenced a "Task 4.1.1d"
that didn't exist in Phase 4's task list until this validation patch added it) — a repeat of that
mismatch class should be caught here, not assumed fixed by prose alone.
- `grep -n "concurrency:" .github/workflows/ux-analysis.yml .github/workflows/e2e-video.yml`
  each return a match; `.github/workflows/build.yml` intentionally has none (Task 3.1.1d's
  recency guard is the equivalent fix there, not a concurrency block — see Pattern Decisions).
- Files: none (review step)

##### Task 6.1.2c: Add an automated workflow-YAML invariant test for AC #7's blocking steps (~10 min)
**[Added post-validation — triad-review engineering-lens gap]** `validation.md` designs
`tools/ci-gates/__tests__/workflow-config-invariants.test.js` for AC #7 but no task created it;
this closes that gap, replacing Task 6.1.2a's pure diff-review with a repeatable, automated
check that doesn't rely on a human re-reading the diff on every future PR touching these files.
- Add `tools/ci-gates/__tests__/workflow-config-invariants.test.js` (new `js-yaml` devDependency,
  scoped to a new `tools/ci-gates/package.json`) that parses `.github/workflows/ux-analysis.yml`
  and `.github/workflows/build.yml` and asserts: the Axe step has no `continue-on-error: true`;
  the "Check new RPCs have tests" step has no `continue-on-error: true`. Run alongside the other
  `tools/ci-gates/*.test.js` unit tests (`node --test tools/ci-gates/`).
- Files: `tools/ci-gates/__tests__/workflow-config-invariants.test.js` (new),
  `tools/ci-gates/package.json` (new)
