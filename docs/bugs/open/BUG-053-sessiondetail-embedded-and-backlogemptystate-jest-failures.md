# BUG-053: `SessionDetail.embedded.test.tsx` and `BacklogEmptyState.test.tsx` Fail Under `npx jest` [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-08-02, while running the full frontend jest suite (`cd web-app && npx jest --no-coverage`) as the final Phase 6 validation gate for `subagent-spawn-tracking` (backlog 9209b4b9). Not caused by that project's diff — confirmed by `git stash` and re-running both suites against the unmodified tree: both fail identically with our changes stashed out.

**Impact**: `npx jest` (no path filter) reports 2 failed test suites / 7 failed tests out of 3665 total. Does not affect runtime correctness of the shipped app — both failures are in test-only code (component tests and a mocked-rejection test), not production logic.

## Problem Description

1. **`src/components/sessions/__tests__/SessionDetail.embedded.test.tsx`** — 7 of 8 tests in the `SessionDetail — initialTab sync (Bug 3)` and embedded-rendering describe blocks fail with `TestingLibraryElementError: Unable to find an element by: [data-testid="tab-strip"]` or `expect(received).toHaveAttribute()` — `received value must be an HTMLElement or an SVGElement. Received has value: null` (e.g. `document.querySelector('[aria-labelledby="tab-terminal"]')` returns `null`). Only "does NOT render the title header when embedded=true" passes. This looks like the component under test (or a dependency it renders) no longer produces the DOM structure (`tab-strip` testid, `aria-labelledby="tab-terminal"` panel) these tests assert on — a pre-existing drift between the test file and the current `SessionDetail` implementation, unrelated to session status/subagent-count work.

2. **`src/components/backlog/BacklogEmptyState.test.tsx`** — test suite fails to load entirely: `Jest worker encountered 4 child process exceptions, exceeding retry limit`. The underlying exception (visible when run scoped/isolated) is an unhandled rejection from a test at line 120 (`.mockRejectedValue(new Error("Server error"))`) escaping the test boundary and crashing the jest worker process 4 times before jest gives up on that file.

## Verification this is pre-existing, not introduced by subagent-spawn-tracking

```
git stash                                                    # remove our diff
cd web-app && npx jest --no-coverage \
  --testPathPatterns="SessionDetail.embedded|BacklogEmptyState"
# → identical failures (7 failed in SessionDetail.embedded, worker-crash in BacklogEmptyState)
git stash pop
```

Neither failing file appears in `git diff --stat` for this branch — `subagent-spawn-tracking` only touched `SubStatusChip.tsx`, `SubStatusChip.test.tsx`, and `SessionRow.tsx` in `web-app/`.

## Suggested Investigation (not done here — out of scope for the project that surfaced this)

- `SessionDetail.embedded.test.tsx`: diff the current `SessionDetail` component's rendered DOM against what the test expects (`data-testid="tab-strip"`, `aria-labelledby="tab-terminal"` panel) — likely a refactor landed that renamed/restructured the tab UI without updating this test file.
- `BacklogEmptyState.test.tsx`: the mocked rejection at line ~120 needs to be caught/awaited properly (likely a missing `await`/`.catch` around the code path that triggers the mocked rejected promise) so it doesn't escape as an unhandled rejection and crash the jest worker.

## Related

- Same category as `BUG-051` (`session/tmux` package flaking under `make quick-check`'s parallel load) — filed per this repo's convention of documenting adjacent issues found during unrelated work rather than silently ignoring them (see `ADR-002`/`ADR-003` in `project_plans/insights-cost-pricing-gaps/decisions/` for the same pattern applied to two other findings from that project).
- Not related to `subagent-spawn-tracking` (backlog 9209b4b9) or any of its detection/proto/SubStatusChip changes.
