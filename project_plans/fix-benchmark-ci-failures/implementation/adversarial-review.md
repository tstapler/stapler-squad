# Adversarial Architecture Review: fix-benchmark-ci-failures

Review date: 2026-11-04
Plan reviewed: `project_plans/fix-benchmark-ci-failures/implementation/plan.md`
Sources verified: `.github/workflows/benchmark.yml`, `web-app/tests/e2e/benchmarks/terminal-throughput.spec.ts`, `project_plans/fix-benchmark-ci-failures/research/pitfalls.md`

---

## Verdict

**Approved with two concerns.** No blockers found. Two concerns must be addressed before merging; four minor issues noted.

---

## CONCERN 1 — Plan contradicts its own pitfalls research on `test.setTimeout`

**Classification**: CONCERN

**Finding**: The pitfalls document (`research/pitfalls.md`, line 48) explicitly states:

> "At 10 runs × 60s = 600s maximum, the per-test `test.setTimeout(300_000)` (5 min) would need to be raised to **at least 650s (11 min)** to avoid the outer timeout becoming the new failure mode."

The plan chose `test.setTimeout(600_000)` = exactly 600s = 60s × 10 runs = zero overhead slack. It does not address the per-iteration overhead that the pitfalls research flagged.

**Mechanics of the gap**: Each iteration of the 10-run loop performs `setupStressTestPage(page)` (page navigation, ~1-3s), `selectPreset(page, PRESET)` (UI interaction, ~0.5s), and `getMetrics(page)` (evaluate call, ~0.1s). These are outside the `waitForTestCompletion(60_000)` window and are not budgeted in the 600s total. Conservative estimate: ~2s overhead × 10 runs = 20s unaccounted for. Total: 620s possible under full per-run load, against a 600s ceiling.

**Plan's Risk Control table acknowledgment** (plan.md, Risk Control row 3): "600_000 is set to match the new 60s × 10 ceiling exactly, but the per-run slack is now 2× the old limit — any single run up to 60s passes." This reasoning is correct for `waitForTestCompletion` individually but ignores cumulative loop overhead. A single run staying under 60s doesn't prevent the outer `test.setTimeout` from firing if page-navigation overhead accumulates across all 10 iterations.

**Counterargument considered**: The old test at 300s = 30s × 10 had the same zero-overhead-slack design and passed on ubuntu-20/22. If overhead was absorbed there, it should be absorbed now. The pitfalls research disagrees: CI CPU throttling is variable, and the fix doubles the per-run ceiling precisely because CI runs can run slower. The same conservative reasoning applies to the outer timeout.

**Recommended fix**: Change `test.setTimeout(600_000)` to `test.setTimeout(700_000)` (11m40s), giving ~100s of overhead headroom above the 60s × 10 = 600s direct product. This matches the spirit of the pitfalls doc's "at least 650s" recommendation with extra margin for CI variance. The `frontend-throughput` job's `timeout-minutes: 45` is unaffected (700s = ~12 minutes, well within the job cap after accounting for browser install and build).

---

## CONCERN 2 — go-tier2 runtime after fix: newly-compilable benchmark packages not analyzed

**Classification**: CONCERN

**Finding**: The plan models the code-gen overhead correctly (~30s for buf + ent-gen), but it does not analyze the runtime contribution of benchmark packages that were previously silently skipping due to build failures.

**Mechanics**: Before the fix, packages importing generated code (protobuf stubs, ent ORM types) fail to compile in `go-tier2`. Without `set -o pipefail`, their non-zero exit is masked by `tee`, and `go test` skips those packages in the benchmark output. After the fix, those packages compile and their benchmarks run with `count=8`. If those packages contain benchmarks with significant per-iteration cost, the 30-minute `go test -timeout=30m` flag or the 50-minute `timeout-minutes: 50` job cap could be hit on the very first post-fix main push.

**What is known**: The pitfalls research (section 2) confirms the fix adds "~30 seconds extra" for the code-gen steps themselves but does not enumerate which previously-failing packages now compile, nor estimate their benchmark runtime contribution. The go-tier2 job runs `./...` which includes every package in the repository.

**Risk level**: Medium-High on the first post-fix run; subsequent runs benefit from Go module cache. The 30-minute `-timeout=30m` flag for `go test` applies to the total wall time of all benchmarks combined. If newly-unblocked packages carry heavy `count=8` benchmarks, one missed timeout immediately causes the first "fixed" CI run to fail — a confusing regression that looks like the fix broke something new.

**Recommended mitigation**: Before merging, audit which packages require generated code (protobuf or ent imports) and estimate their benchmark count. If any carry benchmarks, run a quick `go test -bench=. -count=1 -timeout=5m <package>` locally to bound their per-iteration cost. If the total adds meaningfully to the full sweep, raise `timeout-minutes` from 50 to 60 and `-timeout=30m` to `-timeout=40m`. This is a one-time check, not ongoing maintenance.

---

## MINOR 1 — Plan's `runs-on` line numbers are off by one for all three Playwright jobs

**Classification**: MINOR

**Finding**: The plan cites these target lines for the `runs-on` change:
- Line 231: `frontend-throughput` — actual: **line 230**
- Line 343: `e2e-latency` — actual: **line 342**
- Line 547: `e2e-perf` — actual: **line 546**

Each cited line is `timeout-minutes:`, not `runs-on:`. Verified against `.github/workflows/benchmark.yml` at the current HEAD.

**Impact**: Low. An implementer reading the YAML visually will find the right line without being misled by an off-by-one citation. No functional risk.

---

## MINOR 2 — go-tier1 and frontend-bundle still on ubuntu-latest

**Classification**: MINOR

**Finding**: The plan explicitly marks pinning `go-tier1` (line 51) and `frontend-bundle` (line 187) as "optional — not required by any AC." Those two jobs do not use Playwright, so they are unaffected by the ubuntu-26 Playwright breakage today.

**Residual risk**: Both jobs will face the same OS-migration problem if ubuntu-latest resolves to ubuntu-28 LTS (expected ~April 2028) and something in those jobs' apt dependencies breaks. The plan is correct that this is not in scope; it is a known, deferred maintenance item.

---

## MINOR 3 — buf binary curl download has no integrity verification

**Classification**: MINOR

**Finding**: All six `Install buf` steps across the workflow download the binary with:
```bash
curl -sSL "https://github.com/bufbuild/buf/releases/download/v1.41.0/buf-Linux-x86_64" \
  -o /usr/local/bin/buf && chmod +x /usr/local/bin/buf
```
No SHA-256 verification follows. A MITMed download or a silently-replaced GitHub release artifact would yield malicious code that runs in CI and writes into the generated proto output.

**Context**: This is a pre-existing pattern in `go-tier1`, `e2e-latency`, `e2e-perf`, and `frontend-bundle` — not introduced by this fix. The plan copies the step verbatim from Tier 1, which is the correct approach for consistency. The risk is real but pre-existing and out of scope for this PR.

---

## MINOR 4 — Plan's internal task count is inconsistent

**Classification**: MINOR

**Finding**: The Phase 1 summary table (plan.md line 246) states:
> "7 edits grouped across 4 stories into 6 task IDs"

But the actual task tables list 7 task IDs: 1.1.1a, 1.1.1b, 1.1.1c (3), 1.1.2a, 1.1.2b, 1.1.2c (3), 1.1.3a (1) = 7 benchmark.yml tasks plus 1.2.1a and 1.2.1b = 9 total. The summary table header says "8 tasks" (Phase 1 summary), which matches the 7 benchmark.yml tasks + 2 spec tasks if 1.1.2a-c are counted as 3 or as the "1 logical insertion" mentioned in the description. The inconsistency is cosmetic and does not affect implementation.

---

## Summary

| # | Classification | Description |
|---|---|---|
| 1 | CONCERN | `test.setTimeout(600_000)` = exactly 60s × 10 runs; no slack for per-iteration overhead; contradicts pitfalls research recommendation of at least 650s |
| 2 | CONCERN | go-tier2 runtime post-fix: newly-compilable packages' benchmark contribution unanalyzed; first post-fix run risks 30m `-timeout` or 50m job cap |
| 3 | MINOR | `runs-on` line numbers off by one for all three Playwright jobs (230, 342, 546 vs cited 231, 343, 547) |
| 4 | MINOR | go-tier1 and frontend-bundle remain on ubuntu-latest; explicitly deferred, noted for future maintenance |
| 5 | MINOR | buf binary download has no sha256 verification; pre-existing pattern, out of scope |
| 6 | MINOR | Plan's internal task count annotation is inconsistent (says 6 task IDs; lists 7) |

**0 blockers / 2 concerns / 4 minors.**
