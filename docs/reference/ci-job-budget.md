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
| Push to main only | `main-ci.yml` | full-suite coverage gate (`test-tmux`, `test-rest`, `test`), 5-platform build matrix, `benchmark-gate` |
| Nightly / manual | `nightly.yml`, `css-differential.yml`, release and demo workflows | |

No check is a required status check: `gh api repos/tstapler/stapler-squad/branches/main` reports `protected: false` with empty `required_status_checks`, and `gh api repos/tstapler/stapler-squad/rulesets` returns `[]`. Re-verify before relying on that.

## Adding a job without blowing the budget

1. **Prefer a step over a job.** A new gate that needs the same checkout, Go setup, and generated code as an existing job is a step in that job (`go-guards`, `web-checks`, `lint`). Give later steps `if: ${{ !cancelled() }}` so one failure does not hide the rest.
2. **Keep it off PRs if it is not gating.** Benchmarks, video capture, and Lighthouse-style advisory runs belong on main, nightly, or behind a label (`pull_request: types: [labeled]` plus a job `if:` on `github.event.label.name`). If a main-only job protects a real regression class, add an alert (see `alert-on-main-failure` in `benchmark.yml`).
3. **Use a workflow-level `paths:` / `paths-ignore:` filter, not a job `if:`.** A filtered-out workflow creates no checks; an `if:`-skipped job creates a skipped one. Do not filter out `docs/registry/**`: `test-affected` diffs it.
4. **Put main-only jobs in `main-ci.yml`**, not behind `if: github.event_name != 'pull_request'` in a PR workflow.
5. **Do not use a matrix for fan-out you do not need.** A matrix leg is a check. Cross-compile and OS/arch matrices run on main only.
6. **Add `concurrency` with `cancel-in-progress: ${{ github.ref != 'refs/heads/main' }}`** so a superseded PR run frees its jobs.
7. Re-run `tools/ci/count-pr-checks.py` and put the before/after in the PR description.

The e2e shard count is a separate knob (`E2E_SHARD_TOTAL` in `build.yml` once #1317 lands) and is the largest remaining contributor.

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
| One push to main | about 37 (Build 21, Main CI 8, Benchmarks 5, Lint 1, others) | Main CI 47.8 min (`Test (server/session/config)` 1848 s) |

Queue wait over 819 jobs: median 2 s, p90 57 s, p99 212 s; 17.8% waited over 30 s. That is runner starvation at the cap.

**Rule: a PR run must stay at or under 20 concurrent jobs, and so must a push to main** (so two PRs plus a main push fit under 40, and one run fits under a Free 20). PRs meet it today; a push to main does not and is the target for the main-CI work. Re-count with `tools/ci/count-pr-checks.py` and measure peaks from the Actions API (`started_at`/`completed_at` per job).
