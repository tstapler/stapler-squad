# Validation Plan: fix-benchmark-ci-failures

## Happy Path (Step 0)

Given the benchmark workflow on the `main` branch with all three fixes applied (ubuntu-24.04 runner pinned, buf/ent generate steps added to go-tier2, and terminal-throughput timeouts doubled), when a push to `main` triggers the benchmark workflow, then all jobs — go-tier2, frontend-throughput, e2e-latency, e2e-perf — complete green and GitHub issue #1362 receives no new failure comment from the alert-on-main-failure job.

---

## Test Cases by Requirement

### AC-1: frontend-throughput, e2e-latency, e2e-perf use ubuntu-24.04

**File:** `.github/workflows/benchmark.yml`

#### Unit Test — Happy Path
**Name:** `benchmark_yml_playwright_jobs_use_ubuntu_24_04`

Verify check (run from repo root):
```bash
grep -A2 'frontend-throughput:$' .github/workflows/benchmark.yml | grep 'runs-on: ubuntu-24.04'
grep -A2 'e2e-latency:$'         .github/workflows/benchmark.yml | grep 'runs-on: ubuntu-24.04'
grep -A2 'e2e-perf:$'            .github/workflows/benchmark.yml | grep 'runs-on: ubuntu-24.04'
```
Expected: each command exits 0 and prints the matching line.

Specific lines to confirm after fix:
- Line 230 (frontend-throughput `runs-on`): `ubuntu-24.04`
- Line 342 (e2e-latency `runs-on`): `ubuntu-24.04`
- Line 546 (e2e-perf `runs-on`): `ubuntu-24.04`

#### Unit Test — Error Path
**Name:** `benchmark_yml_playwright_jobs_do_not_use_ubuntu_latest`

Verify that `ubuntu-latest` does not appear on any `runs-on` line inside these three jobs:
```bash
# Extract only the three job blocks and confirm no ubuntu-latest remains
awk '/^  frontend-throughput:/,/^  [a-z]/' .github/workflows/benchmark.yml | grep 'ubuntu-latest' | wc -l
awk '/^  e2e-latency:/,/^  [a-z]/'         .github/workflows/benchmark.yml | grep 'ubuntu-latest' | wc -l
awk '/^  e2e-perf:/,/^  [a-z]/'            .github/workflows/benchmark.yml | grep 'ubuntu-latest' | wc -l
```
Expected: all three commands print `0`.

#### Integration Test — CI Run
**Name:** `ci_playwright_benchmark_jobs_pass_on_ubuntu_24_04`

Trigger: push to `main` (or `gh workflow run benchmark.yml`).
Observable: GitHub Actions shows frontend-throughput, e2e-latency, and e2e-perf jobs with status ✅ (not "Failed to install Playwright dependencies").
Verify via:
```bash
gh run list --workflow=benchmark.yml --branch=main --limit=1 --json conclusion,jobs \
  --jq '.[] | .conclusion, (.jobs[] | select(.name | test("frontend-throughput|e2e-latency|e2e-perf")) | {name, conclusion})'
```
Expected: `conclusion` = `success`; each named job shows `conclusion: "success"`.

---

### AC-2: go-tier2 includes buf generate + ent generate steps

**File:** `.github/workflows/benchmark.yml` (go-tier2 job, starting at line 141)

#### Unit Test — Happy Path (buf generate step present)
**Name:** `go_tier2_has_buf_generate_step`

```bash
awk '/^  go-tier2:/,/^  [a-z][a-z]/' .github/workflows/benchmark.yml \
  | grep 'buf generate proto --template buf.gen.go-only.yaml'
```
Expected: exits 0, prints the matching run line.

#### Unit Test — Happy Path (ent generate step present)
**Name:** `go_tier2_has_ent_generate_step`

```bash
awk '/^  go-tier2:/,/^  [a-z][a-z]/' .github/workflows/benchmark.yml \
  | grep 'entgo.io/ent/cmd/ent generate --feature sql/upsert'
```
Expected: exits 0, prints the matching run line.

#### Unit Test — Error Path (steps are ordered before benchmark run)
**Name:** `go_tier2_generate_steps_precede_benchmark_run`

The buf generate step must appear before "Run full benchmark suite" within the go-tier2 job. Verify ordering by line number:
```bash
buf_line=$(grep -n 'buf generate proto --template buf.gen.go-only.yaml' .github/workflows/benchmark.yml \
  | awk -F: 'NR==1{print $1}')
bench_line=$(grep -n 'go test' .github/workflows/benchmark.yml \
  | awk -F: 'NR==1{print $1}')
[ "$buf_line" -lt "$bench_line" ] && echo PASS || echo FAIL
```
Expected: prints `PASS`.

#### Integration Test — go-tier2 benchmark run succeeds with generated code
**Name:** `ci_go_tier2_benchmark_passes_with_generated_code`

Trigger: push to `main`.
Observable: go-tier2 job exits green; `tier2-bench.txt` artifact (or the cache `bench-tier2-<run_id>`) is non-empty and does not contain `no test files` for packages that require generated output (e.g. `session/ent/...`).
Verify via:
```bash
gh run list --workflow=benchmark.yml --branch=main --limit=1 --json jobs \
  --jq '.[] | .jobs[] | select(.name == "Go Benchmarks (Tier 2 - Extended)") | .conclusion'
```
Expected: `"success"`.

---

### AC-3: All jobs pass on next push to main (zero new comments on issue #1362)

This is a composite integration test across all five ACs.

#### Integration Test — alert-on-main-failure job does not comment
**Name:** `ci_benchmark_workflow_passes_no_issue_1362_comment`

Trigger: push to `main` after all fixes land.
Observable:
1. GitHub Actions run conclusion is `success`.
2. No new comment is posted to issue #1362 (the alert-on-main-failure job only runs on `failure`).

Verify:
```bash
# Step 1: check overall workflow run
gh run list --workflow=benchmark.yml --branch=main --limit=1 --json conclusion --jq '.[].conclusion'
# Expected: "success"

# Step 2: check issue #1362 for new comments since the commit timestamp
gh api repos/tstapler/stapler-squad/issues/1362/comments \
  --jq 'last | {created_at, body}' 
# Expected: created_at predates the fix commit; no bot comment mentioning "benchmark failure"
```

---

### AC-4: terminal-throughput spec does not time out on CI runner

**File:** `web-app/tests/e2e/benchmarks/terminal-throughput.spec.ts`

#### Unit Test — Happy Path (test.setTimeout raised to 700s)
**Name:** `terminal_throughput_spec_uses_700s_total_timeout`

```bash
grep 'test.setTimeout' web-app/tests/e2e/benchmarks/terminal-throughput.spec.ts | grep '700_000'
```
Expected: exits 0, prints the matching line (line 44 after fix; 700s provides ~100s overhead slack above 60s × 10 runs).

#### Unit Test — Happy Path (waitForTestCompletion timeout doubled)
**Name:** `terminal_throughput_spec_uses_60s_per_run_timeout`

```bash
grep 'waitForTestCompletion' web-app/tests/e2e/benchmarks/terminal-throughput.spec.ts | grep '60_000'
```
Expected: exits 0, prints the matching line (line 64 after fix).

#### Unit Test — Error Path (old tight values removed)
**Name:** `terminal_throughput_spec_no_300s_or_30s_timeouts`

```bash
grep -E 'test\.setTimeout\(300_000\)|waitForTestCompletion\(page, 30_000\)' \
  web-app/tests/e2e/benchmarks/terminal-throughput.spec.ts | wc -l
```
Expected: prints `0`.

#### Integration Test — frontend-throughput job completes within wall-clock budget
**Name:** `ci_frontend_throughput_job_completes_without_timeout`

Trigger: push to `main`.
Observable: frontend-throughput job exits green (not "Timeout" / "Job timed out") within its 45-minute job `timeout-minutes` budget.
Verify:
```bash
gh run list --workflow=benchmark.yml --branch=main --limit=1 --json jobs \
  --jq '.[] | .jobs[] | select(.name == "Frontend Terminal Throughput") | {conclusion, completedAt, startedAt}'
```
Expected: `conclusion: "success"` and elapsed time < 2700 s (45 min).

---

### AC-5: go-tier2 uses set -o pipefail

**File:** `.github/workflows/benchmark.yml` (go-tier2 "Run full benchmark suite" step)

#### Unit Test — Happy Path
**Name:** `go_tier2_benchmark_step_has_pipefail`

```bash
awk '/^  go-tier2:/,/^  [a-z][a-z]/' .github/workflows/benchmark.yml \
  | grep 'set -o pipefail'
```
Expected: exits 0, prints the matching line. Confirms it appears inside the go-tier2 job block (not in an unrelated job).

#### Unit Test — Error Path (pipefail not present without fix)
**Name:** `go_tier2_benchmark_step_without_pipefail_would_swallow_build_errors`

This is a documentation-level assertion: before the fix, `go test ... ./... | tee tier2-bench.txt` with a failing `go test` exits 0 (tee's exit code). The pipefail option propagates the left-hand side's non-zero exit. Verify after fix that the step's `run:` block begins with `set -o pipefail` before the `go test` invocation:
```bash
awk '/^  go-tier2:/,/^  [a-z][a-z]/' .github/workflows/benchmark.yml \
  | awk '/set -o pipefail/{found=1} /go test/{if (!found) print "MISSING pipefail before go test"; exit}'
```
Expected: no output (pipefail appears first).

#### Integration Test — go-tier2 fails fast when generated code is absent
**Name:** `ci_go_tier2_fails_visibly_on_build_error`

This test is ENVIRONMENT-CONSTRAINED: it can only be validated in a branch run where generated code is intentionally absent, not on `main`. Document as a manual dry-run verification step:
1. Create a test branch; remove `go generate` from go-tier2 (simulate the pre-fix state).
2. Push and observe go-tier2 exits non-zero (the GHA log shows "exit status 1") rather than silently producing an empty benchmark file.

---

## Migration Plan

N/A — no data migration.

---

## Coverage Summary

| Requirement | Unit (happy) | Unit (error) | Integration | Coverage |
|---|---|---|---|---|
| AC-1: ubuntu-24.04 for Playwright jobs | 1 | 1 | 1 | Full |
| AC-2: buf + ent generate in go-tier2 | 2 | 1 | 1 | Full |
| AC-3: all jobs pass, no issue #1362 comment | — | — | 1 (composite) | Integration only |
| AC-4: terminal-throughput timeouts doubled | 2 | 1 | 1 | Full |
| AC-5: set -o pipefail in go-tier2 | 1 | 1 | 1 (manual) | Full |
| **Totals** | **6** | **4** | **5** | **5/5 ACs covered** |

Requirements coverage fraction: **5/5 (100%)**.

Migration tests: **N/A**.
