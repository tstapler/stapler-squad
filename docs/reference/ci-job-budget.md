# CI job budget

Every job in a triggered workflow is a check run, and a job skipped by `if:` still counts. GitHub-hosted runners cap concurrent jobs per plan (Free 20, Pro 40, Team 60; this account's tier is unconfirmed, see [Limits](#github-actions-limits-and-measured-usage) and `project_plans/e2e-test-parallelization/decisions/concurrency-ceiling-finding.md`), so the number of checks per PR run is the budget, not just runner minutes.

Count it from the YAML (no API, no run needed):

```bash
uv run --with pyyaml python tools/ci/count-pr-checks.py          # current workflows
uv run --with pyyaml python tools/ci/count-pr-checks.py <dir>    # any workflows dir, e.g. a `git archive` of an older ref
```

The script is an upper bound for jobs whose skip comes from `needs:` or a `detect-changes` output.

## Where each check runs

| Trigger | Workflow | Notes |
|---|---|---|
| Every non-docs PR | `build.yml` | `detect-changes`, `prepare`, `web-checks` (smoke next+bun, built-assets, supply chain, Storybook), `test-affected`, `go-guards` (delivery gate, git oracle, install check, PTY race), the e2e jobs |
| Every PR | `backlog-scaffolding-guard.yml` | `pr-diff-guards`: API-only diff checks; the one check a docs-only PR gets |
| PR touching Go/web/proto paths | `lint.yml` | one `lint` job (Go, web, ShellCheck, buf breaking) |
| PR touching Go files | `goreleaser-check.yml` | config check + CGO_ENABLED=0 build smoke in one job |
| PR touching `server/mcp/**`, `server/services/**` | `mcp-integration.yml` | one job, two sequential suites |
| PR touching web build paths | `ux-analysis.yml` | Axe + Lighthouse |
| PR labelled `run-benchmarks` | `benchmark.yml` | also runs on every push to main; a failing main run opens an issue |
| PR labelled `record-video` | `e2e-video.yml` | non-gating feature videos |
| Push to main only | `main-ci.yml` | full-suite coverage gate (`test-tmux`, `test-rest`, `test`), 5-platform build matrix |
| PR touching `website/**`, `tests/e2e/website-demos/**`, `tests/demo/**` or docs sources | `website-ci.yml` | one `checks` job; its smoke steps (build the server binary from the PR, record the `dashboard-lifecycle` clip, build with `TOURS_MEDIA_MODE=smoke`) are steps, not jobs, so the check count is unchanged |
| Push to main touching website or promo paths; weekly cron; manual | `deploy-website.yml` | `decide`, `binary`, `record` (matrix: 2 shards, one check each), `build`, `mediafresh`, `termcheck-dist`, `deploy`, `publish-lkg`, `notify`. Never a PR check. A content-only push skips `binary` and `record` and ships the `promo-media-lkg` artifact |
| Nightly / manual | `nightly.yml`, `css-differential.yml`, release workflows | `demo-publish.yml` is retired (its `demo-gifs-latest` release is no longer written) |

No check is a required status check: `gh api repos/tstapler/stapler-squad/branches/main` reports `protected: false` with empty `required_status_checks`, and `gh api repos/tstapler/stapler-squad/rulesets` returns `[]`. Re-verify before relying on that.

## Adding a job without blowing the budget

1. **Prefer a step over a job.** A new gate that needs the same checkout, Go setup, and generated code as an existing job is a step in that job (`go-guards`, `web-checks`, `lint`). Give later steps `if: ${{ !cancelled() }}` so one failure does not hide the rest.
2. **Keep it off PRs if it is not gating.** Benchmarks, video capture, and Lighthouse-style advisory runs belong on main, nightly, or behind a label (`pull_request: types: [labeled]` plus a job `if:` on `github.event.label.name`). If a main-only job protects a real regression class, add an alert (see `alert-on-main-failure` in `benchmark.yml`).
3. **Use a workflow-level `paths:` / `paths-ignore:` filter, not a job `if:`.** A filtered-out workflow creates no checks; an `if:`-skipped job creates a skipped one. Do not filter out `docs/registry/**`: `test-affected` diffs it.
4. **Put main-only jobs in `main-ci.yml`**, not behind `if: github.event_name != 'pull_request'` in a PR workflow.
5. **Do not use a matrix for fan-out you do not need.** A matrix leg is a check. Cross-compile and OS/arch matrices run on main only.
6. **Add `concurrency` with `cancel-in-progress: ${{ github.ref != 'refs/heads/main' }}`** so a superseded PR run frees its jobs.
7. Re-run `tools/ci/count-pr-checks.py` and put the before/after in the PR description.

For the promo-materials-refresh workflow change (Story 2.4.1) the PR-side count is unchanged by design: `website-ci.yml` stays one job and `demo-publish.yml` never ran on PRs. The new jobs live in `deploy-website.yml`, which has no `pull_request` trigger. Measured with `tools/ci/count-pr-checks.py`: 20 checks on a code PR touching every filtered path and 1 on a docs-only PR, before (spike notes) and after. Put the numbers in the PR description; the matrix is 2 shards on `push` to main until the Task 0.1.1b measurement shows headroom for 4.

The e2e shard count is a separate knob (`E2E_SHARD_TOTAL` in `build.yml` once #1317 lands) and is the largest remaining contributor.

Within a shard, spec files are assigned by recorded duration (`tests/e2e/shard-timings.json`, `tests/e2e/scripts/shard-assign.ts`) rather than Playwright's count-based `--shard`, which once left one shard at 36 min next to others at 16. Refresh the file from the `e2e-shard-timings` artifact the `e2e (Bun build)` job uploads, taken from a full uncut run (a cut-off shard has no durations for the specs it never reached). Each shard also stops itself with `--global-timeout` inside `E2E_SHARD_BUDGET_S` (default 38 min), so an overrunning shard still writes its reports instead of being killed with none. It still fails loudly (`assert-e2e-report.mjs` names the global-timeout cut-off), and the confirmation rerun does not run for a failed shard. A missing or unusable shard plan also fails the shard; there is no count-based fallback, because mixing schemes across shards would lose and duplicate tests.

## Cache policy

The Actions cache is capped at 10 GB per repo and evicts oldest-first past it. A PR run can read caches from its own ref and from main, never from sibling PRs, so a PR that saves a cache only ever feeds itself. Before this policy 7 `setup-go-*` entries (five of them 1.1 GB copies saved from PR refs) plus 3.3 GB of Bazel entries from one PR pushed the repo to 11.2 GB of 48 entries, and every key was a content hash only, so a saved cache was never refreshed.

- **Only main writes.** `.github/actions/cache-main-writes` runs `actions/cache` on the default branch and `actions/cache/restore` everywhere else (the post-save step is skipped, and a log line says "not saving"). Use it for any cache bigger than a few MB; do not add a bare `actions/cache`.
- **Dated keys.** `rolling: true` appends the UTC date, so the first main run of each day saves a fresh entry and PRs restore the newest one by prefix (`restore-keys`). Same-day reruns hit the exact key and do not re-save.
- **One owner per entry.** `setup-go-cached` (Go module + build cache; `scope` `test` or `lint`, lint also holds the golangci-lint cache) is saved only by the job with `save: 'true'` (`main-ci.yml` `test-rest`, `lint.yml` `lint`), so concurrent main jobs do not race to save partial contents. On main it first runs `tools/ci/trim-go-cache.sh` (drop entries unused for 7 days, cap at 1500 MB).
- **Bazel.** `bazel-cache` restores/saves the disk and repository caches and exports `BAZEL_FLAGS`. To use a remote cache later, pass its URL as that action's `remote-cache` input; nothing else changes.
- **Bounded size.** `cache-cleanup.yml` (daily, after each Main CI run, or by hand) runs `tools/ci/prune-actions-caches.py`: newest 2 entries per family on main, closed or 3-day-idle PR entries, then oldest-used-first down to 7.5 GB; it never deletes a family's newest main entry. A manual run is a dry run unless `apply` is set. Check the total with `gh api repos/tstapler/stapler-squad/actions/cache/usage`.

## Runner utilization

Heavy jobs (`prepare`, `web-checks`, `e2e-binary-bun`, `e2e-bun-shard`, `test-affected`, `go-guards`, `lint`, `mcp-integration`, `ux-analysis`, and the `main-ci.yml` test jobs) run a background sampler (`.github/actions/runner-utilization`, `tools/ci/runner-utilization.py`) and a final `if: always()` report step. Each job's summary shows cores busy, iowait, steal, peak memory, and PSI stall share, and uploads a tiny `util-<label>` artifact (7 days). Both steps swallow their own errors and cannot fail a job.

Aggregate across recent runs (needs `gh`; artifacts expire after 7 days):

```bash
python3 -I tools/ci/utilization-report.py --workflow Build --event pull_request --runs 20
python3 -I tools/ci/utilization-report.py --workflow Lint --runs 20
# owner's machine only, never CI:
python3 -I tools/ci/utilization-report.py --workflow Build --prom-out /var/lib/node_exporter/textfile/ci.prom
python3 -I tools/ci/utilization-report.py --workflow Build --push-vm http://localhost:8428
```

Verdicts: `under-used` (median CPU avg < 40%), `right-sized`, `saturated` (>= 90% CPU or >= 40% CPU PSI stall), `io-bound` (iowait > 20%). Add a new heavy job by placing the sampler step right after checkout and the report step last, with a unique `label` per matrix leg.

## GitHub Actions limits and measured usage

Source: <https://docs.github.com/en/actions/reference/limits>, checked 2026-10-31. The repo is public, so standard-runner minutes are free (<https://docs.github.com/en/actions/administering-github-actions/usage-limits-billing-and-administration>); only the limits below bind.

| Limit | Value |
|---|---|
| Concurrent jobs, standard runners | Free 20, Pro 40, Team 60, Enterprise 500 |
| Concurrent macOS jobs | 5 on Free/Pro/Team (we run none: every `runs-on` is `ubuntu-*`) |
| Job matrix | 256 jobs per workflow run |
| Re-runs | 50 per workflow run |
| Workflow runs queued | 500 per 10 s; trigger events 1500 per 10 s per repo |
| Job execution time | 6 h |
| Cache | 10 GB per repo (73 entries / 5.4 GB on 2026-10-31); 200 uploads/min, 1500 downloads/min |
| `GITHUB_TOKEN` API rate | 1,000 requests per hour per repo |

**Plan tier is unconfirmed.** `gh api user` exposes no `plan` and the token lacks the `user` scope. Across 102 sampled runs on 2026-10-31 running jobs reached exactly 40 repeatedly and never exceeded it, which suggests a 40-job cap (Pro), but that is inferred; a Free account would cap at 20. Design for 20, treat 40 as headroom.

Measured on 2026-10-31 (`gh run list --limit 400`, job data for 102 runs; runners are 4 vCPU / 16 GB):

| Trigger | Concurrent jobs | Critical path (median wall) |
|---|---|---|
| One PR (all workflows) | about 18 (Build 13 max, 11 median, plus Lint, MCP, GoReleaser, UX, Diff Guards at 1 each) | Build 17.8 min, Lint 15.3 min |
| One push to main | about 36 after retiring `Benchmark Gate` (was about 37: Build 21, Main CI 8 then 7, Benchmarks 5, Lint 1, others) | Main CI was 47.8 min, the 45-min timeout of `Benchmark Gate (main only)`; without it the longest job is `Test (server/session/config)` at an estimated 14 min on a green run (the first of two passes in one sampled log took 816 s; the 1848 s figure included a failure rerun) |

Queue wait over 819 jobs: median 2 s, p90 57 s, p99 212 s; 17.8% waited over 30 s. That is runner starvation at the cap.

**Rule: a PR run must stay at or under 20 concurrent jobs, and so must a push to main** (so two PRs plus a main push fit under 40, and one run fits under a Free 20). PRs meet it today; a push to main does not and is the target for the main-CI work. Re-count with `tools/ci/count-pr-checks.py` and measure peaks from the Actions API (`started_at`/`completed_at` per job).

`Benchmark Gate (main only)` in `main-ci.yml` was retired: it ran `go test -bench=. -count=5 ./...`, hit its 45-minute timeout and was cancelled on every one of the 7 Main CI runs checked on 2026-10-31, so it never saved a baseline and never compared anything. What remains on push to main is `benchmark.yml`: it runs the Go Tier 1/Tier 2, E2E perf, RPC latency and terminal throughput jobs and saves their baselines, and `alert-on-main-failure` opens an issue when a main run fails outright. Regression comparison (`benchdiff`, `fail-on-regression: 20`) runs only on `run-benchmarks`-labelled PRs, so a main-vs-previous-main Go benchmark regression is not gated anywhere today; adding a main-time compare step to `benchmark.yml` is a separate change.
