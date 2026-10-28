# ADR-003: Go on the per-worker N-server rearchitecture — sequenced behind a cheaper duration-aware shard balancer

**Date**: 2026-10-27
**Status**: Accepted
**Supersedes**: extends [ADR-001](ADR-001-isolate-via-seam-defer-n-server-rearchitecture.md) (not reversed — ADR-001's "isolate via seam, defer implementation" call was correct given what was known 2026-10-25; this ADR acts on Story 3.3.1's checkpoint, which ADR-001 itself named as the forcing function for revisiting it)

## Context

[Backlog item `0892086f-79c1-4b8d-a0bf-15867facdf4d`](../../../project_plans/e2e-test-parallelization/decisions/post-phase3-checkpoint-finding.md) asks for a go/no-go decision on scheduling the deferred per-worker N-server rearchitecture, after Story 3.3.1's checkpoint found Phase 3's cheaper levers (`fullyParallel: true` + chromium-only shard installs) produced `percent_improvement = -43.3%` — a regression, not an improvement — well below the plan's 10% floor.

This ADR does two things ADR-001 and the seam doc's own research did not: (1) corrects a scope error found by re-verifying the design against the current codebase, and (2) explains *why* the cheaper Phase 3 lever regressed instead of helping, which changes the recommended sequencing.

### Correction: the seam design's "two call sites" claim is wrong

[`docs/explanation/e2e-per-worker-server-seam.md`](../../../docs/explanation/e2e-per-worker-server-seam.md) and `research/architecture.md` §2 both assert that overriding Playwright's built-in `baseURL` fixture is sufficient because "every existing spec that calls `page.goto('/')` or a relative path keeps working unchanged," with only two named exceptions (`connection-count-indicator.spec.ts`, `launcher-presets.spec.ts`) that read `process.env.TEST_SERVER_TESTDIR` directly.

That claim was never verified against actual spec bodies — it was an assumption about how the suite navigates. Verified now, directly:

```
$ grep -rl "^const BASE_URL = process\.env" tests/e2e --include="*.ts" | wc -l
140   # spec files with their own module-level BASE_URL constant
$ grep -rl "^const BASE_URL = process\.env" tests/e2e/pages --include="*.ts" | wc -l
17    # shared page-helper files, same pattern
```

140 of 185 top-level spec files, plus all 17 files in `tests/e2e/pages/`, declare `const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544'` at module scope and use it directly in `page.goto(BASE_URL + path)` / `request.post(`${BASE_URL}/api/...`)` calls — e.g. [`smoke.spec.ts:6`](../../../tests/e2e/smoke.spec.ts#L6), [`pages/BacklogMutations.ts:3`](../../../tests/e2e/pages/BacklogMutations.ts#L3). None of these read Playwright's injected `baseURL` fixture at all, so a fixture override — the seam doc's "load-bearing choice" — has **zero effect** on any of them. Worse: a module-level `const` is evaluated once at **import time**, which for a worker-scoped fixture happens *before* that fixture can possibly have run (fixtures resolve lazily on first `test()`/`beforeAll()` use, not at module load) — so even a non-fixture per-worker `process.env` write done correctly inside a worker-scoped setup would still race every one of these 157 files' own eager reads.

Only 62 spec files are clean today — confirmed by spot-reading three (`backlog-copy-link.spec.ts`, `backlog-board-live-updates.spec.ts`, `session-auto-approve.spec.ts`): they use `page.goto('/relative/path')` directly or a page-object `.goto()` wrapper, which **does** resolve through Playwright's fixture-managed `baseURL` today and would in fact keep working unchanged under the seam design, exactly as advertised — but that's 33% of the spec files, not ~99%.

**Net correction**: the real migration surface is on the order of 157 files (140 specs + 17 page helpers), not 2. It's still bounded and mechanical — nearly every instance is one of two textually-identical patterns — but it is a two-orders-of-magnitude larger blast radius than ADR-001 and the seam doc assumed when they called this "a known, bounded starting point."

### Why the cheaper Phase 3 lever regressed instead of helping

`research/architecture.md` §6 found the dominant cost is shard-duration **imbalance** (2.0x-2.2x spread between fastest/slowest shard, same 10-way split), not fixed per-job overhead, and recommended "duration-aware shard rebalancing (bin-packing spec files into N buckets by last-run duration, read from the existing blob-report JSON)" as the lever that directly attacks it.

Story 3.1.1 did not implement that. Per `build-vs-buy.md` §1's verdict (quoted in `plan.md:60`), it shipped Playwright's **native** `fullyParallel: true` instead — a built-in flag that splits the suite by *test count*, not by *measured historical duration*. A shard with one 90-second test and twenty 2-second tests is exactly as imbalanced under `fullyParallel: true`'s count-based split as it was before; the flag has no notion of which tests are historically slow. Story 3.3.1's checkpoint measured the result directly: the spread got *worse* (2.35x/2.48x vs. 2.0x/2.16x baseline), not better.

This matters for sequencing: the project has now tried the cheap, dependency-free bet on fixing shard imbalance and it did not pay off, but it did not rule out the *specific* lever the research actually recommended (real duration-aware bin-packing) — only the proxy Playwright happened to ship as a free config flip. A custom balancer reading the blob-report timing data this repo already produces is still unexplored, is far cheaper than the N-server rearchitecture (no fixture rearchitecture, no 157-file codemod, no new resource-contention/isolation risk), and targets the exact problem architecture.md §6 identified as dominant.

## Decision

**Go** on the per-worker N-server rearchitecture — schedule it as real implementation work (see [`project_plans/e2e-per-worker-server/implementation/plan.md`](../../e2e-per-worker-server/implementation/plan.md)) — **sequenced behind, not instead of**, a new Epic 0: a custom duration-aware shard balancer using the suite's own blob-report timing data.

Rationale for Go (not stay deferred):
- Both of Phase 3's cheap, low-risk levers have now been tried and the combined result is a regression against the Success Metric; "do nothing" has already been tried and failed to move the needle the project exists to move.
- The corrected 157-file migration cost is larger than previously documented but is still mechanical and codemod-able (ast-grep/gritql pattern: `const BASE_URL = process.env.TEST_SERVER_URL || '...'` → shared lazy accessor), not open-ended rewrite work.
- The three empirical risks pitfalls.md §4 named (port exhaustion, resource contention, isolation completeness) are real but are the kind of risk resolved by a bounded validation plan (repeated CI runs, failure-clustering analysis), not reasons to avoid starting.

Rationale for sequencing Epic 0 first, not abandoning the custom balancer in favor of jumping straight to N-server work:
- It is a strictly cheaper, lower-risk shot at the exact problem architecture.md already proved is dominant, using data the suite already produces.
- If it closes most of the gap on its own, the N-server rearchitecture's scope (or urgency) may shrink — that's new information worth having before committing the larger investment, not a reason to delay indefinitely (Epic 0 is cost-class S, a few CI cycles, not a multi-week blocker).
- If it doesn't close the gap, Epics 1-4 (the N-server work) proceed with the same real numbers more clearly isolating what imbalance-fixing alone can't solve.

## Consequences

- A new SDD project, `project_plans/e2e-per-worker-server/`, is scheduled with its own `implementation/plan.md` (Epic 0 duration-aware balancer, Epics 1-4 the N-server rearchitecture + empirical validation + rollout decision).
- `docs/explanation/e2e-per-worker-server-seam.md` is updated with a dated correction pointing here, so a future implementer starts from the corrected 157-file scope, not the original "two call sites" claim.
- Epic 0's custom balancer, if it ships, is itself staged as its own PR per the original requirements' Risk Control section — same discipline that governed Phase 3.
- The N-server work stays out of `build.yml`'s main gate until Epic 3's empirical validation passes; `playwright.multiworker.config.ts` is a new, opt-in, parallel config file, not a modification of `playwright.config.ts`, so every existing CI shard job and local `npx playwright test` run is unaffected while this is proven out — unchanged from ADR-001's original risk-containment strategy.

## References

- [Backlog item `0892086f-79c1-4b8d-a0bf-15867facdf4d`](../../../project_plans/e2e-test-parallelization/decisions/post-phase3-checkpoint-finding.md)
- [ADR-001: Isolate via seam, defer N-server rearchitecture](ADR-001-isolate-via-seam-defer-n-server-rearchitecture.md)
- [`docs/explanation/e2e-per-worker-server-seam.md`](../../../docs/explanation/e2e-per-worker-server-seam.md)
- `project_plans/e2e-test-parallelization/research/architecture.md` §2, §6-7
- `project_plans/e2e-test-parallelization/research/build-vs-buy.md` §1
- `project_plans/e2e-test-parallelization/research/pitfalls.md` §4
- `project_plans/e2e-test-parallelization/implementation/plan.md` Story 3.1.1, Epic 3.3
- [`project_plans/e2e-per-worker-server/implementation/plan.md`](../../e2e-per-worker-server/implementation/plan.md)
