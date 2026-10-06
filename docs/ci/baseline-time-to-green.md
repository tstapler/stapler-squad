# CI Baseline: Time-to-Green and Queue-Time Share

Pre-fix baseline for `project_plans/ci-speed/implementation/plan.md` Epic 1.2
(Stories 1.2.1/1.2.2). This is the "before" number Epic 5.1's post-ship
measurement (Phase 5) must reproduce with the identical methodology and diff
against. No CI behavior was changed to produce this data — read-only queries
against `tstapler/stapler-squad` via `gh`.

Data gathered: 2026-08-28. Sample: last 20 merged PRs, spanning
2026-08-25T05:28Z–2026-08-28T17:49Z.

## Methodology

1. Enumerate the PR sample:
   ```
   gh pr list --repo tstapler/stapler-squad --state merged --limit 20 \
     --json number,mergedAt,headRefOid,createdAt
   ```
2. Fetch triggered workflow runs (needed `--limit 1500` and the `startedAt`
   field, beyond the plan's suggested `--limit 50`/100, to cover all 20 PRs
   including two from three days back — `gh run list`'s default 20-run page
   only reaches ~2 hours into the past on this repo's PR volume):
   ```
   gh run list --repo tstapler/stapler-squad \
     --json databaseId,name,event,createdAt,startedAt,updatedAt,headSha,status,conclusion \
     --limit 1500
   ```
   Runs were matched to each PR by `headSha == <PR's headRefOid>` (`jq`).
   Every one of the 20 PRs had 9–10 matching runs (185 runs total); no PR
   was dropped for missing data.
3. Per PR: `time_to_green = max(updatedAt across matched runs) − min(createdAt across matched runs)`.
4. Per run: `queue_s = startedAt − createdAt`, `compute_s = updatedAt − startedAt`,
   `total_s = updatedAt − createdAt`.
5. Median/p90 computed with standard nearest-rank (median: middle of sorted
   values, or mean of the two middle values for even n; p90: `ceil(0.9n)`-th
   sorted value).

## Stale-trigger-config check (research/pitfalls.md §0)

`research/pitfalls.md` §0 flags `demo-publish.yml` moving to
`workflow_dispatch`-only on 2026-07-04 as a workflow that could pollute a
before/after average if pre-/post-cutover runs were mixed. This sample window
(2026-08-25 to 2026-08-28) is entirely **after** that cutover, and confirmed
empirically: `demo-publish.yml` produced **zero** runs matched to any of the
20 sampled PRs (it only fires on manual dispatch now, never on a PR push), so
it doesn't enter this baseline at all.

More generally, I checked `.github/workflows/*.yml` commit history for the
full sample window plus 2 days of lead-in (`gh api
"repos/tstapler/stapler-squad/commits?path=.github/workflows&since=2026-08-23&until=2026-08-29"`):
five commits touched workflow files in that range. Two changed `on:` trigger
scope — `33de907` (mcp-integration.yml path-filter widen) and `53655ed`
(build.yml/mcp-integration.yml: `pull_request` `paths:` filter removed in
favor of a job-level `detect-changes` gate) — but both landed **2026-08-24**,
before the earliest sampled PR event (PR #622, created 2026-08-25T04:22Z), so
every PR in the sample is on the same (post-change) side of both; no
mid-sample `on:` split. A third commit (`b5a200ea6`, lint.yml) and a fourth
(`8f96a44ab`, build.yml) changed step *bodies* only (ESLint file-scoping
logic; splitting the `-race` test invocation into two calls), not `on:`
triggers — outside this exclusion criterion's scope, but noted here for
completeness. `8f96a44ab` landed 2026-08-25T08:17Z, between PR #622
(05:28) and PR #623 (06:40)'s merges but after both PRs' runs had already
started — so it affects at most PR #621/#622 (2 of 20 samples), which sit at
the **low** end of the time-to-green distribution (32.9m/31.5m vs. a 29.5m
median), meaning it does not inflate the reported baseline.

## Aggregate Time-to-Green (Story 1.2.1)

| PR | Merged (UTC) | Matched runs | Time-to-green (min) |
|---|---|---|---|
| #622 | 2026-08-25T05:28:15Z | 9 | 31.5 |
| #623 | 2026-08-25T06:40:00Z | 9 | 27.4 |
| #621 | 2026-08-25T07:39:17Z | 9 | 32.9 |
| #629 | 2026-08-25T19:47:30Z | 9 | 80.4 |
| #625 | 2026-08-25T19:13:39Z | 9 | 60.1 |
| #626 | 2026-08-25T19:16:44Z | 9 | 65.1 |
| #631 | 2026-08-25T20:42:18Z | 9 | 50.0 |
| #627 | 2026-08-25T21:02:55Z | 9 | 42.0 |
| #630 | 2026-08-25T23:35:42Z | 10 | 45.0 |
| #634 | 2026-08-26T17:00:05Z | 9 | 19.7 |
| #628 | 2026-08-26T17:32:51Z | 10 | 44.3 |
| #636 | 2026-08-27T04:45:54Z | 9 | 18.3 |
| #635 | 2026-08-27T10:20:25Z | 9 | 21.8 |
| #642 | 2026-08-27T10:27:56Z | 9 | 20.3 |
| #645 | 2026-08-28T04:27:11Z | 10 | 46.8 |
| #643 | 2026-08-28T04:30:50Z | 9 | 23.9 |
| #644 | 2026-08-28T04:52:11Z | 10 | 20.6 |
| #646 | 2026-08-28T05:21:18Z | 9 | 20.3 |
| #647 | 2026-08-28T07:02:24Z | 10 | 19.9 |
| #641 | 2026-08-28T17:49:23Z | 9 | 20.1 |

**n = 20, median = 29.45 min, p90 = 60.08 min** (min 18.3m, max 80.4m).

The tail is dominated by `build.yml` and `mcp-integration.yml`/`Benchmarks`
runs that finish last within a PR's run set (consistent with
requirements.md's per-workflow baseline table, where `build.yml` and
`mcp-integration.yml` are the longest-running workflows).

## Queue-Time Share (Story 1.2.2)

Computed from the same 185 matched runs, reusing `createdAt`/`startedAt`/
`updatedAt` already captured in Task 1.2.1a's `gh run list` call (no
additional `gh run view` calls were needed — `startedAt` is a valid `gh run
list --json` field).

- **Per-run queue-time share** (`queue_s / total_s`, median/p90 across all
  185 runs): **median = 0%, p90 = 0%**. 182 of 185 runs (98.4%) had zero
  measurable queue time (`startedAt == createdAt`) — this repo is not
  hitting GitHub's free-tier 20-concurrent-job ceiling during the sampled
  window.
- **Aggregate (time-weighted) queue-time share**: `sum(queue_s) / sum(total_s)`
  across all 185 runs = **4.8%** (5,512s queue / 114,790s total wall-clock).
  Only 3 runs had any nonzero queue time, all outliers:

  | PR | Workflow | Queue time | Total time | Queue share |
  |---|---|---|---|---|
  | #631 | Generated Protobuf Guard | 2,795s (46.6m) | 2,997s | 93% |
  | #630 | Build | 1,816s (30.3m) | 2,699s | 67% |
  | #628 | Build | 901s (15.0m) | 2,411s | 37% |

- **Concurrent same-PR queueing overlaps**: checked every pair of runs
  within each of the 20 PRs for overlapping `[createdAt, startedAt]`
  windows. **0 overlaps found across all 20 PRs** (0-in-20, i.e. 0%).

### Decision rule (pre-mortem.md Failure #1 / plan.md Story 1.2.2)

Per the plan's stated rule: promote Epic 3.5 ("Per-PR job-fan-out
reduction") to required scope if median queue-time share > 15% of total
wall-clock, **or** concurrent same-PR overlaps appear in >1-in-5 sampled
runs.

- Median queue-time share: **0%** (not > 15%).
- Concurrent same-PR overlaps: **0-in-20** (not > 1-in-5 = 20%).

**Neither trigger condition is met → Epic 3.5 is OPTIONAL/lower-priority.**
GitHub-side queueing is not a meaningful contributor to this repo's current
CI wall-clock — the 3 outlier runs above (all `Build`/`Generated Protobuf
Guard`, all sharing the same job-runner pool) look like isolated contention
spikes, not a systemic 20-concurrent-job ceiling problem, and don't change
the conclusion at either the per-run or aggregate level. Phase 2–4's
compute-focused fixes are not undermined by an unaddressed queueing
bottleneck, per this story's purpose (closing pre-mortem.md's P1 finding).

## Raw data

Commands above were run against `tstapler/stapler-squad` on 2026-08-28. The
raw `gh pr list` (20 entries) and `gh run list` (1,500 entries, superset
covering the full sample) JSON outputs were not committed — this doc's
tables are the reduction, and Epic 5.1 should re-run the same `gh`
invocations against a fresh sample rather than diffing against saved JSON.
