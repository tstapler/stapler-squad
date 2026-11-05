# Requirements: fix-benchmark-ci-failures

## Problem Statement

The benchmark CI workflow (`benchmark.yml`) has been failing persistently on
`main` since October 31, 2026. GitHub Actions issue #1362 has accumulated 21+
failure comments from the `alert-on-main-failure` job, indicating every push to
main triggers at least one failing benchmark job. Each failure emits a noisy
issue comment, clogging the issue tracker.

**Reference failing run:** commit `3e1c3f56b`, run
[41253069144](https://github.com/tstapler/stapler-squad/actions/runs/41253069144)

## Identified Root Causes

### 1. `ubuntu-latest` migrated to Ubuntu 26 (October 19, 2026)

GitHub Actions' `ubuntu-latest` label began migrating to Ubuntu 26 on October
19, 2026. The benchmark workflow's Playwright-dependent jobs (`e2e-latency`,
`frontend-throughput`, `e2e-perf`) use `runs-on: ubuntu-latest`.

- **E2E RPC Latency** (original failing run): Failed at "Install Playwright
  browsers" — all 3 retry attempts (8-minute timeout each) exhausted in ~11
  minutes total, suggesting each attempt failed with an error rather than a
  timeout. Ubuntu 26 apt package names or Playwright `--with-deps` dep list may
  not align.
- **Frontend Terminal Throughput** (subsequent runs): Playwright installs but
  "Run terminal throughput benchmark" fails. The spec calls
  `waitForTestCompletion(page, 30_000)` — a 30-second per-run timeout across 10
  runs. Ubuntu 26 runners may render slower or start the Bun dev server slower,
  triggering these Playwright assertion timeouts.

### 2. Tier 2 benchmark job missing code generation steps

`go-tier2` runs `go test -bench=. ... ./...` without first running
`buf generate proto` or `ent generate`. Because generated ent files are
gitignored (`session/ent/*.go`, etc.), packages that import generated ent types
fail to build silently. Three packages are affected:
- `session/ent/backlogstage`
- `session/ent/backlogprogressnote`
- `session/ent/backlogitemdependency`

Tier 2 has no `set -o pipefail`, so `tee` masks the non-zero `go test` exit
code — the job reports ✓ but silently omits benchmarks for ent-dependent
packages. The GH Actions annotation surface shows these as `X`-level errors.

### 3. Terminal-throughput spec timing too tight for new runners

The terminal-throughput spec uses:
- `waitForTestCompletion(page, 30_000)` — 30s per completion wait
- `STATUS_CHANGE_TIMEOUT_MS = 5000` — 5s to see status change from idle
- 10 total runs × (page nav + render + settle) ≈ 300s total test timeout

On Ubuntu 26 runners, which may have different resource allocation than Ubuntu
24, these timing constraints can be exceeded even without a code bug. The spec
already has `retries: 2` in `playwright.config.ts`, which helps but doesn't fix
root-cause timing issues.

## Acceptance Criteria

1. **AC-1**: The `benchmark.yml` Playwright-dependent jobs (`e2e-latency`,
   `frontend-throughput`, `e2e-perf`) specify a pinned Ubuntu version (e.g.,
   `ubuntu-24.04`) instead of `ubuntu-latest`, or Playwright is upgraded to a
   version that explicitly supports Ubuntu 26 with `--with-deps`.

2. **AC-2**: The `go-tier2` job in `benchmark.yml` includes `buf generate proto`
   and `ent generate` steps (matching Tier 1's setup) so that `go test ./...`
   can build all packages cleanly.

3. **AC-3**: After the fix is merged, the next push to `main` that triggers the
   benchmark workflow results in all benchmark jobs passing (zero new comments on
   issue #1362).

4. **AC-4**: The terminal-throughput Playwright spec either has increased
   timeouts or is structured so it does not time out on the target CI runner
   class used after AC-1 is applied.

5. **AC-5**: The `go-tier2` job either uses `set -o pipefail` or logs a
   clear error when packages fail to build, so silent partial-benchmark runs no
   longer appear as false passes.

## Out of Scope

- Fixing the content/assertions of the benchmarks themselves (only CI
  infrastructure is in scope).
- Migrating benchmark jobs to self-hosted runners.
- Changing the benchmark alerting strategy (issue #1362 comment policy).
