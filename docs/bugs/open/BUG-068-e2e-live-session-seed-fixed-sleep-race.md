# BUG-068: `tests/e2e/helpers/test-server.ts` Live Session Seeding Uses a Fixed Sleep Instead of Polling for Actual Queue State [SEVERITY: Low]

**Status**: Open
**Discovered**: 2026-08-10, while running `tests/e2e/review-queue.spec.ts` and
`review-queue-severity.spec.ts` locally as part of sdd:6-verify Layer 4 for the
`review-queue-severity` feature.

## Symptom

`tests/e2e/review-queue.spec.ts`'s "Review Queue Acknowledge Flow — UI Contract" describe
block fails reproducibly (2/2 local runs, isolated and chained) with:

```
TimeoutError: page.waitForSelector: Timeout 10000ms exceeded.
  - waiting for locator('[data-testid="review-queue-loaded"]')
```

Page snapshot at failure time shows `Review Queue 0` / "No sessions need attention!" while
the seeded `e2e-review-1`/`e2e-review-2`/`e2e-review-3` sessions appear instead under a
"Currently Working" panel as "In progress — nothing to do yet" — i.e. by the time the test
runs, the poller has classified them as `WorkingState.ACTIVE`/`PROCESSING`, not
`ReasonIdle`. `tests/e2e/review-queue.spec.ts:58` and `:68` (unrelated `/sessions/new` wizard
tests, no review-queue dependency) also failed in the same runs, indicating the underlying
issue is a general environment/timing sensitivity, not scoped to review-queue rendering
itself.

## Root Cause (confirmed)

`tests/e2e/helpers/test-server.ts`'s `seedLiveSessions()`:

```ts
// Review queue poller fires every 2s; idle threshold is 5s. Wait 12s to be safe.
console.log(`Waiting for review queue poller to detect ${created} idle sessions...`);
await new Promise(resolve => setTimeout(resolve, 12000));
console.log('✅ Live sessions seeded for review queue tests');
```

This is a fixed-duration sleep, not a poll against the actual review-queue state — the
"✅ Live sessions seeded" log line is emitted unconditionally after 12s, regardless of
whether the sessions are actually present in the queue at that moment. Depending on machine
load and how long browser launch + navigation takes after `globalSetup` returns, the
seeded bash sessions can drift past the idle threshold and get reclassified before (or
during) the test's own assertions run — a race by construction, not a one-off flake.

## Ruled Out

Confirmed via `git diff <review-queue-severity base>..HEAD -- session/review_queue_poller.go`
that the `review-queue-severity` feature's changes to that file are scoped entirely to the
`reason == ReasonApprovalPending` enrichment block (adding `risk_level` metadata to an item
*already* selected for the queue) — no touch to `ReasonIdle`/`WorkingState` derivation or
`checkSession`'s inclusion decision. `review-queue-severity.spec.ts` (which mocks
`GetReviewQueue` directly rather than depending on this live-seeding race) passed 8/8 across
two separate runs in the same session this was discovered in, including a
`review-queue-loaded` wait identical to the failing spec's.

## Suggested Fix

Replace the fixed `setTimeout(resolve, 12000)` with a poll loop against
`GetReviewQueue`/`ListPendingApprovals` (or a lighter status-only endpoint) that resolves as
soon as the expected session count is actually present, with a generous upper-bound timeout
as the true failure case. Mirrors the "poll for a real signal instead of sleep-and-hope"
principle already applied elsewhere in this test suite (e.g. `waitForSelector` usage, and
`.claude/rules/e2e-test-conventions.md`'s "No `waitForTimeout`" rule for spec files itself —
this fixed sleep lives in the shared helper, not a spec file, so it isn't caught by that
lint rule today).
