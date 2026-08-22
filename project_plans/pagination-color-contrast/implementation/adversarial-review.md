# Adversarial Review: pagination-color-contrast

**Date**: 2026-08-16
**Verdict**: CLEAN
**Re-review note**: iteration 1 of max 2 — re-verified prior blocker/concerns only, did
not re-run a full fresh review.

## Blockers

None.

## Concerns

None.

## Minors

- The PR-description-only record of the deferred `primaryHover` gap (Task 1.1.4a) has no durability mechanism beyond the PR body — once merged, it's not filed as a backlog item or tracked issue, so it depends on someone reading historical PR descriptions to rediscover it. A one-line backlog/issue reference alongside the PR note would survive better than PR-body text alone.
- Task 1.1.3b's "no jarring hue shift" acceptance criterion is inherently subjective with no objective pass bar (unlike the 1% pixel-diff threshold already enforced by Task 1.1.3a's snapshot diff, which is the more rigorous check covering the same ground). Low risk given the snapshot diff already catches material regressions, but worth noting the manual check adds little beyond the automated one.
- New (found during this re-review, low severity): the plan's fallback documentation for Task 1.1.3a is internally inconsistent. The Pattern Decisions table (plan.md:21) and Task 1.1.1b's own text (plan.md:144-146) both claim the `node -e` one-liner is "the documented fallback verification path for Tasks 1.1.2b and 1.1.3a." But Task 1.1.3a's actual fallback note (plan.md:260-262) points to Task 1.1.3b's manual visual check instead, not to 1.1.1b's contrast calculation — which is arguably the more sensible fallback for a *visual*-regression task (a numeric contrast ratio doesn't verify pixel-level snapshot state), but it contradicts the other two references. Not a functional gap — 1.1.3a does have a working fallback — just a citation mismatch worth tidying in a future pass.

## Resolution log

- Item 1 (blocker): RESOLVED — Task 1.1.2a (plan.md:179-195) runs `make build` (full `next build` → `server/web/dist` → `go build` chain) as an explicit prerequisite, and the dependency diagram (plan.md:53-77) shows `1.1.2a` gating `1.1.2b`, `1.1.3a`, and `1.1.3b`, with prose (plan.md:70-77) explicitly naming `ensureBinary()`'s stale-binary reuse as the failure mode being closed.
- Item 2 (concern): RESOLVED — Task 1.1.1b (plan.md:130-147) explicitly states "Do NOT edit `web-app/scripts/check-theme-contrast.ts`" and instead runs a `node -e` WCAG-luminance one-liner; the Pattern Decisions table (plan.md:21) documents this as the chosen approach with the full-script-resync option listed as rejected ("scope creep beyond this ticket's complexity-1 ... boundary").
- Item 3 (concern): RESOLVED — Task 1.1.2b (plan.md:202-203) contains an explicit fallback referencing "Task 1.1.1b's `node -e` contrast calculation." Task 1.1.3a (plan.md:260-262) also contains an explicit fallback, though it references Task 1.1.3b's manual check rather than 1.1.1b as the Pattern Decisions table claims — see new minor above; the underlying concern (no fallback path at all) is resolved since both tasks now have one.
- Item 4 (concern): RESOLVED — Story 1.1.3's header (plan.md:216-221) and Task 1.1.3a's rationale (plan.md:253-256) both state the corrected rationale ("local dev-run hygiene, not a CI gate... `e2e-video.yml`'s `FEATURE_SPECS` allowlist excludes it, and `ux-analysis.yml` only runs `accessibility.spec.ts`. The plan previously stated CI would fail on this; that claim was wrong and is corrected here.").
- Item 5 (concern): RESOLVED — Task 1.1.2a (plan.md:189-193) runs `make lint` explicitly alongside `make build`, citing CLAUDE.md's required-gate language.
