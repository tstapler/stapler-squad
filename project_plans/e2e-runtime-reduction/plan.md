# E2E runtime reduction: scoping study

**Status**: scoping only, nothing here is implemented. **Measured**: 2026-11-16 against `origin/main` at `b7dbf54a8`.
**Objective (owner, 2026-11-16)**: minimise total billed runner-minutes of the e2e jobs per PR run by using the CPU and memory already paid for. Wall-clock is a secondary guard.

Labels: **VERIFIED** = a command or URL below produced it; **INFERRED** = derived from verified numbers with a stated assumption. Raw data lives only in the author's scratchpad; every figure can be regenerated with the commands in [Reproduce](#reproduce).

## 1. What already exists (not redone here)

| Work | Where | Outcome relevant now |
|---|---|---|
| Per-shard parallelism, `fullyParallel`, shard-count checkpoint | `project_plans/e2e-test-parallelization/decisions/` | Count-based `--shard` made the spread worse (checkpoint finding) |
| Per-worker server pool (`E2E_WORKERS` servers per shard) | `docs/explanation/e2e-per-worker-server-seam.md`, `project_plans/e2e-per-worker-server/implementation/plan.md` | Shipped: `tests/e2e/helpers/server-pool.ts`; `build.yml` sets `E2E_SHARD_TOTAL: 5`, `E2E_WORKERS: 3` |
| Duration-balanced shard assignment, `--global-timeout` budget | `tests/e2e/shard-timings.json`, `tests/e2e/scripts/shard-assign.ts`, `tests/e2e/scripts/run-ci-shard.sh`, `docs/reference/ci-job-budget.md` | Shipped; timings file is stale (section 3.4) |
| PR spec selection (`all`/`subset`/`none`) | `tests/e2e/scripts/select-specs.ts`, `replay-selection.ts`, `docs/reference/ci-job-budget.md` "Affected-spec selection" | Shipped 2026-11-09 (`80176bddc`) |
| Baseline comparator + main-only confirmation rerun | `scripts/e2e-baseline-compare.ts`, `scripts/e2e-confirm.ts`, `build.yml` job `e2e-bun` | Main only; fails every main run (section 3.5) |
| Known-failure triage | `project_plans/e2e-preexisting-failures/`, `project_plans/e2e-baseline-fail-fixes/`, `tests/e2e/baseline/baseline-fail-triage.json` | Baseline still records 319 `fail` rows (`next-e2e-baseline.json` `meta.counts`) |
| CI check budget, concurrency ceiling | `docs/reference/ci-job-budget.md`, `project_plans/e2e-test-parallelization/decisions/concurrency-ceiling-finding.md` | Rule: at most 20 concurrent jobs per run; no check is a required status check |

**Which e2e policy is in force (ADR-009 vs `select-specs.ts`)**
- ADR-009 (`project_plans/bazel-conversion/decisions/ADR-009-selective-e2e-and-expensive-tests-in-bazel.md`) was accepted 2026-11-05 with the "Full e2e run" outcome: spike S14 failed and the owner chose "e2e stays a full run in CI, with no e2e selection" (recorded as `adr_009_e2e` in `project_plans/bazel-conversion/implementation/owner-attestations.yaml:104`). What it rejected was Bazel observed-dependency slices.
- Four days later `select-specs.ts` landed: a path-rule selector that falls back to `all` on any shared surface. It is live on PRs (`build.yml` job `e2e-env`, "PR runs only"). `docs/reference/bazel-slow-suites.md` still classifies the shards as `manifest`.
- **Verdict**: `select-specs.ts` is what runs today; ADR-009's text was never amended. Treat this as an open owner decision (section 6, D1), not a settled conflict. VERIFIED by reading both files plus `git log` of `select-specs.ts`.

## 2. Measurement method

- 40 most recent completed (success or failure) `Build` PR runs, job timings from `gh api .../runs/<id>/jobs` (list: `gh run list --workflow Build --event pull_request`). 35 ran e2e; 34 of those ran the full 5 shards, 1 ran a 1-shard subset, 5 skipped e2e (`select-specs` `none` or irrelevant diff). Run IDs are in the first column of the Appendix.
- Test-level data from one run's artifacts (`e2e-bun-report`, `e2e-shard-timings`, `util-*`): run [43825413838](https://github.com/tstapler/stapler-squad/actions/runs/43825413838) (PR #1670, green). One run is a sample; shard durations in the other 33 full runs are consistent with it (Appendix), but per-spec failure counts could differ run to run. Artifacts expire after 7 days.
- Main-branch data: 21 completed `Build` push runs on `main`, 2026-11-14 to 2026-11-16 (`gh run list --workflow Build --event push --branch main`).
- Utilization: `python3 -I tools/ci/utilization-report.py --workflow Build --event pull_request --runs 20` and the `util-*` artifacts of run 43825413838. Runners are 4 vCPU / about 16 GB.

## 3. Current state

### 3.1 Runner-minutes per PR run (primary metric)

34 full 5-shard runs. "Billed" rounds each job up to a whole minute (GitHub's per-job rounding); the repo is public so the minutes are free, but the same quantity is the runner capacity consumed.

| Quantity | p50 | p90 | Source |
|---|---|---|---|
| e2e jobs, raw runner-minutes | 92.0 | 97.6 | VERIFIED, 34 runs |
| e2e jobs, billed minutes | 96.5 | 101.4 | VERIFIED |
| whole `Build` workflow billed minutes | 132.5 | 140.7 | VERIFIED |
| e2e share of `Build` billed minutes | 73% | | VERIFIED (96.5 / 132.5) |

Per-job breakdown (seconds, same 34 runs):

| Job | p50 | p90 | Notes |
|---|---|---|---|
| `e2e environment` | 11 | 15 | reads Playwright version |
| `e2e binary (Bun build)` | 280 | 379 | "Build binary" 246 s plus "Build e2e seed tools" 73 s (main run 43823186573 step timings); built once, shared by artifact |
| `e2e (Bun build) shard N` x5, each | 1106 | 1286 | 170 shard jobs |
| all five shards, summed | 5124 (85.4 min) | 5434 | 88% of e2e raw minutes |
| `e2e (Bun build)` gate | 64 | 72 | merge reports, comparator on main only |

Setup versus test inside one shard (run 43825413838, shard 1; other shards within 2 s): container init 25 s, checkout 5 s, download binary 6 s, apt (`unzip tmux git`) 7 s, about 43 s in total, 4% of a 1,100 s job. Server pool start is 1.6 s (3 servers, shard 3 log: 07:50:55.13 to 07:50:56.75). So duplicated per-shard setup costs about 3.6 runner-minutes per PR run, not the dominant waste. Bun install and Playwright browsers cost under 5 s per shard (browsers come from the pinned image).

The binary job is one build shared by all shards, so it is not duplicated per shard. Its 280 s p50 is 4.7 runner-minutes (4.9% of e2e).

### 3.2 Utilization (the "paid-for but idle" view)

VERIFIED, `utilization-report.py` over 20 runs:

| Job | n | wall med/p90 s | cpu avg med % | cpu p95 % | iowait % | mem peak MB (of about 16,000) | psi cpu % | verdict |
|---|---|---|---|---|---|---|---|---|
| e2e-bun-shard | 65 | 1057/1278 | 61.7 | 94.3 | 0.2 | 4306 | 26.9 | right-sized |
| e2e-binary-bun | 13 | 287/357 | 76.2 | 99.9 | 2.2 | 4378 | 45.2 | saturated |

Per shard in run 43825413838 (`util-e2e-bun-shard-N`): cpu avg 65.3 / 69.7 / 48.9 / 55.8 / 44.3 %, peak memory 3.8 to 4.7 GB, steal 0.

Reading (INFERRED): memory is never the constraint (about 27% used). CPU averages 45 to 70% but p95 is 90%+ and CPU pressure stall is 20 to 32%, meaning the three Playwright workers plus Chromium plus the server plus tmux contend in bursts. Idle capacity exists mostly where load is not parallel: the serial terminal-stress pass (`E2E_WORKERS=1`, `run-ci-shard.sh`) and the tail of each shard while the heaviest worker finishes. Raising `E2E_WORKERS` is therefore a bounded win (see O5), not a free one.

The largest utilization waste is not idle cores but cores spent on work that cannot change a verdict (3.3).

### 3.3 Where the test-seconds go (run 43825413838, VERIFIED)

Report: 2,889 test instances, 209 spec files, 11,864 test-seconds (every attempt counted), merged-report wall 1,025 s.

| Cut | Test-seconds | Share |
|---|---|---|
| Tests that ended `unexpected` (failed after retry): 259 instances in 56 files | 5,071 | 42.7% |
| of which baseline class `fail` (known permanent failures): 217 instances | 4,171 | 35.2% |
| of which baseline class `pass`: 33 instances (new failures) | 685 | 5.8% |
| Time spent in retry attempts (`retries: 1` in `tests/e2e/playwright.config.ts`): 269 tests | 2,672 | 22.5% |
| Project `chromium-dom` (1,413 tests) | 6,219 | 52.4% |
| Project `chromium` (1,419 tests) | 5,451 | 45.9% |
| Project `mobile-chromium` (57 tests) | 194 | 1.6% |

Findings:
1. The PR gate ignores per-test failures (only a crashed or empty shard fails; see `build.yml` e2e-bun comment), yet 259 failing tests run to the 30 s test timeout twice. Example: all 16 `touch-targets.spec.ts` failures sit near 30.6 s each (two attempts of the 30 s timeout), 490 s for one file.
2. 204 of 209 spec files run in both `chromium` (WebGL) and `chromium-dom` (DOM renderer, `playwright.config.ts` project comment: for tests that read rendered terminal text). Only 1 file is chromium-only.
3. Failing seconds are concentrated: top files by failing seconds are `touch-targets` 490, `session-create-new-project` 456, `notification-tray-variants` 344, `session-create-existing-worktree` 304, `session-create-directory` 301, `terminal-flickering` 289, `input-drop-badge` 239 (7 files, 2,423 s, 20% of all test-seconds).
4. The eight specs the owner named account for little of it: `backlog-result-panels-mobile` 49 s, `omnibar-backlog-compose` 78, `scroll-forward-return-to-live` 81, `terminal-resync-stagger` 27, `jump-to-session` 48, `notification-tray-variants` 378, `multi-pr-session-mobile` 36, `route-focus-and-announce` 29: 726 s in total, 6.1%, and 378 of that is one file. (Note `route-focus-and-announce` and most of `multi-pr-session-mobile` passed in this run.) Fixing only those eight is necessary for main's gate (3.5) but is a minor runtime lever. The lever is the whole baseline-`fail` class.

### 3.4 Shard imbalance (a wall-clock issue, not a minutes issue)

In 34 of 34 five-shard runs one shard is short (about 450 to 620 s) while four run 1,000 to 1,480 s (Appendix). The `drift` step output on main run [43823186573](https://github.com/tstapler/stapler-squad/actions/runs/43823186573) says: "Shard plan vs measured durations: loads 779s, 2709s, 2624s, 2898s, 3068s ... shard-timings.json is stale". Cause (VERIFIED): `tests/e2e/shard-timings.json` carries `"notification-contrast-over-xterm.spec.ts": 3856` while the measured value is 528, so the planner isolates that file on shard 1. Rebalancing does not change total minutes (sum is conserved) but would cut p50 wall-clock of the slowest shard from 1,106 s towards the mean of about 1,025 s (INFERRED; 5,124 / 5).

### 3.5 Critical path, rerun and budget behaviour

- PR critical path (35 runs with e2e): first job created to e2e gate end, p50 1,730 s, p90 1,906 s. Chain is `Generate & Build Web UI` (p50 49 s) then binary (p50 280 s) then shards (p50 1,106 s plus 25 s init) then gate (p50 64 s). The e2e gate is the last job to finish in 26 of 35 runs; `Test (affected packages only)` in 7, `Go guards` in 2. VERIFIED.
- Queue time is not a factor: shard queue p50 2 s, p90 3 s (negative values in the raw data are reruns reusing `created_at` and were excluded from the narrative).
- Shard budget cut-off (`E2E_SHARD_BUDGET_S` 2,280 s): longest shard seen is 1,481 s, so 0 of 170 shard jobs hit it. The 40 minute `timeout` is not binding.
- Main push runs: the `e2e (Bun build)` gate failed in 21 of 21 completed `Build` push runs sampled. Each selected 26 to 36 tests for the confirmation rerun (cap 40); the gate job then takes p50 409 s, max 829 s (versus 64 s on PRs), about 5.8 extra runner-minutes p50 per main push, and still fails because the same tests fail again in isolation (run 43823186573: `backlog-result-panels-mobile`, `omnibar-backlog-compose`, `scroll-forward-return-to-live`, `terminal-resync-stagger`, `multi-pr-session-mobile` named FAIL by the comparator). Rerun rate is therefore 100% of main runs, confirmation recovers nothing in the sampled run, and it is pure cost until the failing specs are fixed or quarantined.
- PR selection effect: of 40 sampled PR runs, 5 skipped e2e (12.5%) and 1 ran a subset. `replay-selection.ts --prs 60` (VERIFIED) reports all=48 (80%), subset=1, none=11 (18%), "mean recorded spec-seconds saved per PR run: 4232 of 21200 (20%)". The 48 `all` PRs are mostly `refactor(session)` Go moves, because any `session/`, `server/` or shared surface selects everything.

## 4. Options judged on runner-minutes

Baseline: 96.5 billed minutes per full PR run (p50). Lane model for INFERRED rows: a shard job runs 3 concurrent lanes, so removing T test-seconds saves about T / 3 job-seconds; real savings are lower where contention drops (PSI stall falls) and higher where retries and timeouts are removed. Savings are not additive across O1 to O3 (the same seconds overlap).

Cost classes: S = one agent wave plus one CI verification run; M = two to three waves plus soak; L = more than three waves or an owner-gated rollout. "CI runs" are full-suite verification runs of about 96 runner-minutes each.

| # | Option | Expected saving (runner-min per PR run) | Wall effect | Cost | Risk | Blocked by |
|---|---|---|---|---|---|---|
| O1 | Quarantine the baseline-`fail` class on PR shards (generate a grep-invert from `next-e2e-baseline.json`; keep it running on main and nightly) | about 23 (4,171 s / 3 / 60); lower bound 11.6 if only their retries are dropped (about half of those seconds are attempt 2) | about -20% | S to M: generator script plus unit test, 2 CI runs | A genuinely fixed test stays hidden until baseline refresh; comparator must treat skipped as not-regressed (INFERRED, needs a check in `e2e-baseline-compare.test.ts`) | D2 |
| O2 | Fix root causes of the top failing files and the eight named specs, then shrink the baseline | up to 23 (same pool as O1) plus main gate -5.8 per push (rerun disappears) | same | M to L: about 7 files (touch-targets, session-create-*, terminal-flickering, input-drop-badge, notification-tray-variants, ...) each one root-cause wave; baseline re-record | Fixes are product-behaviour specific; some are real bugs | none; prior art `project_plans/e2e-baseline-fail-fixes/` |
| O3 | Stop running every spec in both `chromium` and `chromium-dom`: annotate which renderer a spec needs and run the rest once | up to 34.6 (6,219 s / 3 / 60) if all DOM runs went; plan on about 17 (half) | -15% to -25% | M: audit 204 files with a script that greps for `.xterm-rows`/`countRenderedRows`, then change `testMatch` | Loses DOM-vs-WebGL parity coverage on specs that do not read terminal text; covered by nightly full matrix | D3 |
| O4 | Reduce retry cost: `retries: 0` for classes known deterministic, shorter `timeout` than 30 s for specs failing on a wait | 5 to 12 (2,672 s retry time / 3 / 60 = 14.8 max; part is flake recovery worth keeping) | -5% to -10% | S | Real flakes (5 flaky in the sample) fail instead of passing on retry | D2 |
| O5 | Raise `E2E_WORKERS` 3 to 4 on the 4 vCPU shards | 0 to 13 (cpu avg med 61.7%, p95 94%, PSI stall 26.9%: at most 10 to 15% of 85 min) | down | M: an A/B across 3 runs; measure stall and flake | CPU stall already 27%; the comment in `playwright.config.ts` says stress specs starve under siblings; flake rise (see `fix-flaky-tests-dont-defer`) | none, but gate on O1 first so the signal is not drowned by failing tests |
| O6 | Bin-pack by measured CPU-seconds and refresh `shard-timings.json` (drop the 3,856 s outlier; add outlier guard to `drift`) | about 0 (sum conserved) | -80 to -180 s on the slowest shard (INFERRED) | S | none | none |
| O7 | Change shard count | 5 to 4: about -0.7 (one fewer 43 s setup, plus rounding); 5 to 6: about +0.7 | 4 shards: slowest shard about +20% (INFERRED); 6: about -15% | S (one env var `E2E_SHARD_TOTAL`) | Check budget: each shard is one check (see `docs/reference/ci-job-budget.md`, 20-concurrent rule) | none |
| O8 | Widen PR selection: add a rule that internal Go moves with no `server/` handler or embedded-web change select a smaller subset (use the observed 48 `all` refactor PRs as the replay corpus) | mean about 20% of spec-seconds today (about 19 min of 96.5); widening could add to it | down | M: rule plus replay evidence | Soundness: `select-specs.ts` falls back to `all` deliberately; ADR-009 | D1 |
| O9 | Full suite only on `main` and nightly; PR runs a smoke subset | up to about 70 (to 20 to 30 min per PR) | large | L: smoke-set curation, escape detector (ADR-009 safety nets) | Regressions merge and surface later; contradicts ADR-009 Full e2e run | D1, D4 |
| O10 | Binary job: cache Go build cache and start the seed-tool build in parallel with the web build (it is 73 s of 280 s) | 1 to 3 (of 4.7) | -1 to -2 min | S | cache churn; follow `cache-main-writes` policy in `ci-job-budget.md` | none |
| O11 | Gate job: skip container pull plus `bun install` on PRs where nothing is compared (it runs 64 s p50 on PRs to merge and assert) | about 1 | small | S | check count unchanged only if the job is kept | none |
| O12 | Move UI-only specs (touch targets, a11y route sweep, contrast) to jest or component tests | per spec: `touch-targets` 493 s, `a11y-route-sweep` 180, `notification-contrast-over-xterm` 528: about 6.7 total | down | L per spec | loses real-browser layout truth; `docs/reference/feature-testing-registry.md` registries | none; do last |
| O13 | Share more setup across shards (browsers, apt, Bun) | about 3.6 upper bound (43 s x 5 / 60) | -40 s | M | marginal; the image already carries browsers | none |

Quantified tradeoff for "more shards": per added shard the duplicated setup is about 43 s (about 0.7 runner-minutes, 4% of a shard) plus per-job minute rounding, so total minutes rise about 0.7 per added shard while the slowest shard falls only if timings are balanced (3.4). With the present stale plan, going from 5 to 6 shards would help little because the long pole is a handful of heavy files. Fewer, fuller shards save minutes only in the 0.7 per shard range; the real lever is test-seconds (O1 to O4), which are 90%+ of the cost.

## 5. Recommended sequence

Order is by minutes saved per cost, with low-risk first and the ADR-gated work last.

**PR 1 (S): "ci(e2e): skip known-failing baseline tests on PR shards and refresh shard timings"**
- Generate the skip list from `tests/e2e/baseline/next-e2e-baseline.json` rows with class `fail` (the repo prefers Go helpers; the e2e scripts are Bun TypeScript today, so decide at implementation), pass it to `run-ci-shard.sh` on `pull_request` only; main and nightly keep running everything.
- Refresh `tests/e2e/shard-timings.json` from a full uncut run after the skip takes effect (the 3,856 s outlier goes away; selection and drift gates already exist).
- Expected: e2e billed p50 96.5 to about 73 (INFERRED), slowest shard 1,106 s to about 800 s. Verification: one full PR run read with the billed-minutes recipe in [Reproduce](#reproduce) plus the `utilization-report.py` verdict. Size: one agent wave, about 2 CI runs (about 200 runner-minutes).

**PR 2 (M): "test(e2e): fix the top failing spec files" (one PR per root cause, in order)**
- `touch-targets` (490 s, all timeouts at 30 s), `session-create-new-project` (456), `session-create-existing-worktree` (304), `session-create-directory` (301), `terminal-flickering` (289), `input-drop-badge` (239), then the eight named specs. Re-record the baseline after each batch.
- Stops the main gate failing 21 of 21 runs and removes the 26 to 36 test confirmation rerun (about 5.8 runner-minutes per push). Size: about 3 agent waves with a worktree each; evidence: main `e2e (Bun build)` gate turns green in the next 3 push runs.

**PR 3 (M): "ci(e2e): run chromium-dom only for specs that read the DOM renderer"**
- A script classifies the 204 dual-run files, a manifest records the decision, `playwright.config.ts` uses it for `testMatch`. Nightly keeps the full matrix.
- Expected: about 17 further minutes (INFERRED). Size: 2 waves, 2 CI runs.

Then, in order of value: O5 (workers=4 A/B), O10/O11, O8 (needs D1), O7 (only if wall-clock regresses past the guard). O9 and O12 only if D1/D4 are decided yes.

## 6. Owner decisions

- **D1 (ADR-009)**: Confirm that `select-specs.ts` PR selection is acceptable given ADR-009's "Full e2e run, no e2e selection", and either amend ADR-009 or revert selection. Needed before O8 or O9. Also update `docs/reference/bazel-slow-suites.md` row "build.yml e2e (Bun build) shards".
- **D2**: Allow PR runs to skip baseline-`fail`-class tests (O1/O4). They already cannot fail the PR gate; this removes their runtime. Needs agreement that a skipped test that later starts passing is surfaced by main and nightly, not by the PR.
- **D3**: Allow a spec to run in only one renderer project (O3); the nightly keeps both.
- **D4**: Only if O9 is wanted: change which jobs are required checks. Today no check is required (`docs/reference/ci-job-budget.md`); if branch protection is added, require the `e2e-bun` aggregate job, not shards, and keep the check count within the 20-concurrent rule.
- **D5**: Accept the metric below; in particular whether runner-minutes (not wall-clock) is the goal when the two disagree (more shards).

### Decisions recorded (owner, 2026-11-16)

| ID | Decision | Effect |
|---|---|---|
| D1 | Amend ADR-009: PR runs may select specs with `select-specs.ts`; `main` and `workflow_dispatch` stay full | Amendment appended to `project_plans/bazel-conversion/decisions/ADR-009-selective-e2e-and-expensive-tests-in-bazel.md`. Unblocks O8. |
| D2 | PR runs may skip baseline-`fail`-class tests and cut retries; `main` and nightly still run them | Implemented in PR 1: `E2E_SKIP_BASELINE_FAIL=1` on `pull_request` makes `run-ci-shard.sh` pass `--grep-invert` built by `tests/e2e/scripts/baseline-skip.ts`. The skipped set is printed and written to each shard's step summary. Skipped tests never run, so their retries disappear with them; retries for other tests stay at 1. |
| D3 | DOM-reading specs run only in `chromium-dom` on PRs; nightly keeps both projects | Separate later PR (PR 3 above), not part of PR 1. |
| D5 | The goal is total runner-minutes, not wall-clock, when they disagree | Section 7 targets stand; wall-clock stays a guard only. |

PR 1 follow-ups (2026-11-16, from review of #1677):
- Re-record `tests/e2e/baseline/next-e2e-baseline.json` from a recent full main run. Its `capturedAt` is 2026-10-29 and 44 of the 58 skipped spec files have commits since; the skip summary now prints the baseline age and warns past 14 days. Not done in PR 1: the baseline is a three-run, per-attempt record from the Next build, and one main artifact does not reproduce that shape.
- A spec file changed by the PR is never skipped (`E2E_KEEP_SPECS` from `e2e-env`'s `changed-specs`), so a PR that repairs a baseline-fail test sees it run.
- `shard-timings.json` was measured on run 43825413838, which included the failing tests. PR runs skip them, so real loads are lighter and the balance is approximate; refresh after the baseline is re-recorded.

## 7. Measurable target

| Metric | Today (VERIFIED) | After PR 1 | After PR 1 to 3 | How measured |
|---|---|---|---|---|
| e2e billed runner-minutes per full PR run, p50 / p90 | 96.5 / 101.4 | 75 / 80 | 55 / 60 | billed-minutes recipe (Reproduce) over 30 PR runs |
| Shard CPU avg median, psi cpu | 61.7%, 26.9% | not lower than today | 70% or more, psi no higher than 30% | `utilization-report.py` |
| Main `e2e (Bun build)` gate | fails 21 of 21 | n/a | green; gate p50 under 120 s (today 409 s) | `gh run list --event push` |
| Wall-clock guard: first job to gate end, p50 | 1,730 s | not above 1,730 s | 1,300 s or less | job-timestamp recipe (Reproduce): earliest job `created_at` to gate `completed_at` |

Accept PR 1 only if p50 minutes fall by at least 15 and the PR-run flaky count does not rise (today: 5 flaky of 2,889 in the sampled run).

## 8. Self-review: gaps and limits

- Test-level numbers (3.3) come from a single run (43825413838); the failing set varies slightly run to run. Mitigation: PR 1 should re-measure on two more runs before skipping, which is cheap (artifacts for the next merged PRs).
- Lane model (T / 3) assumes concurrent lanes are fully busy; actual savings are INFERRED and may be lower. Validate in PR 1.
- The binary job's internal split (246 s build + 73 s seed tools) is from main run 43823186573 step timings; PR runs have p50 binary 280 s total.
- Fixed per-test overhead (login, context, page load) cannot be separated from `report.json`: median test is 1.6 s, 717 of 2,889 tests are under 1 s, but the setup share is not measured. Add a Playwright fixture timer before O13 or any per-test trimming.
- Plan tier and concurrency cap remain INFERRED (see `docs/reference/ci-job-budget.md`).
- Not measured: confirmation rerun rate on PRs (it runs on main only by design); stress-pass minutes per shard (inside the shard step, about 110 s in the sampled shard 3 per its log).
- `tests/e2e/shard-timings.json` was last refreshed 2026-11-09 (`ded261933`), before the suite grew; the drift output flags it stale.

## Appendix: per-run shard durations (seconds, sorted, VERIFIED from job timestamps)

| Run | Shards | Gate |
|---|---|---|
| [43825413838](https://github.com/tstapler/stapler-squad/actions/runs/43825413838) | 510 1065 1086 1142 1182 | 59 |
| [43824982590](https://github.com/tstapler/stapler-squad/actions/runs/43824982590) | 521 999 1000 1130 1142 | 65 |
| [43819074809](https://github.com/tstapler/stapler-squad/actions/runs/43819074809) | 463 1035 1059 1125 1209 | 65 |
| [43814285392](https://github.com/tstapler/stapler-squad/actions/runs/43814285392) | 588 1004 1114 1197 1439 | 59 |
| [43684629414](https://github.com/tstapler/stapler-squad/actions/runs/43684629414) | 599 1141 1198 1224 1304 | 66 |
| [43666839204](https://github.com/tstapler/stapler-squad/actions/runs/43666839204) | 495 1092 1108 1134 1481 | 59 |
| [43653359229](https://github.com/tstapler/stapler-squad/actions/runs/43653359229) | 500 983 1173 1180 1467 | 71 |
| [43557488363](https://github.com/tstapler/stapler-squad/actions/runs/43557488363) | 500 967 1012 1110 1235 | 73 |

(8 of 34 full runs shown; the rest are in the same ranges: shard min 463 to 623 s, max 1,095 to 1,481 s.)

## Reproduce

```bash
# job timings for the last 40 completed PR Build runs
gh run list --workflow Build --event pull_request --limit 100 --json databaseId,conclusion \
  --jq '.[]|select(.conclusion=="success" or .conclusion=="failure")|.databaseId' | head -40
gh api "repos/tstapler/stapler-squad/actions/runs/<id>/jobs?per_page=100" --paginate \
  --jq '.jobs[]|[.run_id,.name,.conclusion,.created_at,.started_at,.completed_at]|@tsv'
# artifacts (7-day retention)
gh run download <id> -n e2e-bun-report ; gh run download <id> -n e2e-shard-timings ; gh run download <id> -p 'util-*'
# utilization and selection
python3 -I tools/ci/utilization-report.py --workflow Build --event pull_request --runs 20
cd tests/e2e && bun scripts/replay-selection.ts --prs 60
```

Billed minutes per run = sum over `e2e*` jobs of `ceil((completed_at - started_at)/60)`. Test-seconds per cut = sum of `results[].duration` per test in `report.json`, joined to `next-e2e-baseline.json` by spec, project and title.
