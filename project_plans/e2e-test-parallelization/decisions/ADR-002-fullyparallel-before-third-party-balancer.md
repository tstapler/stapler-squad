# ADR-002: Try native `fullyParallel: true` before any third-party shard balancer

**Date**: 2026-10-25
**Status**: Accepted

## Context

`playwright.config.ts`'s `fullyParallel: false` means Playwright's native `--shard=N/M` balances the 10-way split **by file**, not even by test count. Phase 2 research (`architecture.md` §6, `build-vs-buy.md` §1) confirmed this is the likely cause of the measured 2.0x-2.2x shard-duration spread on run `40184788547`, since a shard that happens to draw several slow files runs far longer regardless of shard count.

Timing-aware (duration-based) shard balancers exist outside Playwright core:
- `NSXBet/test-orchestrator`: VERIFIED via GitHub API — created 2026-01-23, 4 stars, 0 forks, 2 open issues, single external org. Too immature to depend on for the main CI gate.
- Currents.dev's documented bin-packing pattern (`--test-list`, Playwright 1.56+): a pattern, not a packaged tool; Currents' own product is a paid SaaS dashboard, not a CI action.
- No GitHub Marketplace action from Microsoft or a major vendor packages "calculate Playwright shard assignment from historical timing" as a maintained product.

## Decision

Flip `fullyParallel: true` (keeping `workers: 1`) as the first and, for this round, only shard-rebalancing change (this plan's Story 3.1.1). Do not adopt a third-party timing-based balancer in this round. If `fullyParallel: true` alone doesn't close the imbalance enough (measured in Story 3.1.2's before/after comparison), the next lever is a small **in-repo** timing-based bin-packer consuming the blob-report JSON timing data the repo already produces — not an external dependency — per `build-vs-buy.md` §1's verdict.

## Consequences

- Zero new dependency in the main CI gate; the change is a one-line config flip with a trivial revert.
- Balancing granularity improves from file-count to test-count, which Playwright's own docs confirm is a real distinction (`playwright.dev/docs/test-sharding`).
- If this proves insufficient, the fallback path is already named (in-repo bin-packer) rather than left as an open question — but building it is explicitly not part of this round's scope; it would be proposed only after Story 3.1.2's CI evidence shows it's still needed.

## References

- `project_plans/e2e-test-parallelization/research/architecture.md` §6
- `project_plans/e2e-test-parallelization/research/build-vs-buy.md` §1
- `project_plans/e2e-test-parallelization/implementation/plan.md` Story 3.1.1, Story 3.1.2
