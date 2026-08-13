# Adversarial Review: review-queue-jump-fix

**Date**: 2026-06-23
**Reviewer role**: Devil's advocate — find holes in the plan before implementation

---

## Verdict: APPROVED with one clarification needed

The fix is correct. The two-line code change is sound. The test strategy is appropriate. There are no architectural concerns. One implementation pitfall is called out below that the implementer must handle correctly.

---

## Attacks and Responses

### Attack 1: "The effect dep `[reviewQueueItems]` is the wrong dep — it should be `[allQueueItems]`"

**Claim**: If `allQueueItems` is the oracle, then the effect should fire when `allQueueItems` changes, not `reviewQueueItems`. Using `reviewQueueItems` as dep means the effect might miss a removal that happens without changing the visible queue.

**Response**: Rejected. The effect's _trigger_ is intentionally "the visible queue changed" — that's precisely the event we need to react to. If a session is removed from `allQueueItems` but `reviewQueueItems` never changes (impossible in practice, because a removal from `allQueueItems` via `removeItem` will also drop it from the derived `reviewQueueItems`), then there is no visible-queue event to react to, and no auto-advance should occur. Keeping `[reviewQueueItems]` as dep is correct.

The existing `// eslint-disable-next-line react-hooks/exhaustive-deps` comment explicitly acknowledges this intentional omission. The plan correctly preserves it.

---

### Attack 2: "`allQueueItems` is `liveItems` from Redux — it may be stale inside the effect closure"

**Claim**: React's closure rules mean the `allQueueItems` captured in the effect closure at render time may be stale by the time the effect body executes (if a render happens between dep change and effect execution).

**Response**: Rejected. React effects always run _after_ the render that updated the dep, so `allQueueItems` captured in the effect closure is the value from the same render that produced the new `reviewQueueItems`. There is no staleness here. Unlike `handleAutoAdvance` (which uses a `setTimeout` and therefore uses refs), this effect runs synchronously in the commit phase.

---

### Attack 3: "The dismiss flow race condition — `allQueueItems` may still contain the dismissed session when the effect fires"

**Claim**: `handleDismissFromQueue` calls `acknowledgeSession` → `removeItem` dispatch → then `handleAutoAdvance(id, true)`. Separately, the `[reviewQueueItems]` effect will also fire because `reviewQueueItems` changed after the `removeItem`. The plan says "allQueueItems already lacks the dismissed session" — is this actually true?

**Response**: Confirmed safe, but requires careful reasoning. `removeItem` dispatch is synchronous and Redux reducers run synchronously. After `dispatch(removeItem(id))` returns, `allQueueItems` (derived from `selectReviewQueueItemsWithLiveStatus`) is already updated in the store. The next React render (which recomputes `reviewQueueItems` via the `useEffect` that maps `queueItems` to sessions) will see the updated `allQueueItems`. So by the time the `[reviewQueueItems]` dep-effect fires, `allQueueItems` already excludes the dismissed session — `stillInQueue` is `false` — and the effect calls `handleAutoAdvance(id, true)`. This is a double-advance: once from `handleDismissFromQueue` and once from the effect.

**Wait — is this a bug?** Let's check: `handleAutoAdvance` is already called with `force=true` in `handleDismissFromQueue`. The effect will then call it again. Both calls are wrapped in `setTimeout(..., 300)`. They will both fire roughly simultaneously. The second call navigates to the _same_ next session (the selection has already moved). This is harmless but wasteful.

**IMPORTANT CLARIFICATION FOR IMPLEMENTER**: This double-advance scenario already exists today (the current guard also falls through to `handleAutoAdvance` after an explicit dismiss). The fix does not make it worse. The plan is correct. But if the team later wants to eliminate the double-advance, they could add a guard checking `selectedSession.id !== resolvedSessionId` before the effect calls `handleAutoAdvance`. That is out of scope for this fix.

---

### Attack 4: "`ReviewItem.sessionId` field name — could be wrong"

**Claim**: Maybe the field is `id` or `session_id`, not `sessionId`.

**Response**: Verified. `useReviewQueue.ts` line 247 uses `event.event.value.sessionId` on `ReviewQueueEvent`. The `reviewQueueSlice.ts` uses `item.sessionId` throughout. The existing `page.tsx` line 102 uses `item.sessionId`. The plan correctly specifies `item.sessionId`.

---

### Attack 5: "Mocking `useReviewQueueContext` in tests — will jest.mock work for a context that uses Redux internally?"

**Claim**: `useReviewQueueContext` calls `useAppSelector` internally. Mocking it at the module boundary requires the test to mock the full context, not just individual selectors, which is harder to set up.

**Response**: The plan correctly recommends mocking `useReviewQueueContext` at the module boundary with `jest.mock('@/lib/contexts/ReviewQueueContext', ...)`. This returns a mock object with `{ acknowledgeSession: jest.fn(), items: [...] }` directly — no Redux store needed in the test. The pattern is used in existing tests like `ReviewQueuePanel.test.tsx`. This is the correct approach.

---

### Attack 6: "4 test cases — is this enough? What about the `handleDismissFromQueue` path through the effect?"

**Claim**: The plan only tests the "deleted externally" effect path. The dismiss button path also interacts with the effect. Should there be tests for that?

**Response**: The dismiss path (`handleDismissFromQueue` → `acknowledgeSession` → `handleAutoAdvance`) does not use the effect — it calls `handleAutoAdvance` directly. The effect fires as a side effect of the dismiss, but the double-advance is harmless and pre-existing. The 4 tests specified in the plan are the minimum necessary to verify the fix and catch regressions. Additional dismiss-path tests would be valuable but are out of scope for this targeted fix.

---

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Field name typo (`sessionId` vs `id`) | Low | High (silent bug) | Verified in codebase; TS compiler will catch at build time |
| ESLint dep comment removed accidentally | Low | Medium (exhaustive-deps warning breaks CI lint) | Plan explicitly says to preserve the comment |
| Test mock setup incorrect (missing Redux provider) | Medium | Low (test fails at setup, not false positive) | Plan recommends module-level mock, bypasses Redux entirely |
| Double-advance on explicit dismiss | Exists today | Low (harmless UX jitter) | Pre-existing; out of scope |

---

## Summary

The plan is **correct and complete** for the stated scope. The fix is a minimal, targeted change with zero blast radius. The test strategy correctly isolates the behavior under test. No ADR is required. No architectural changes are needed.

The implementer should pay attention to:
1. Use `item.sessionId` (not `item.id`) when accessing `ReviewItem` objects
2. Preserve the `// eslint-disable-next-line react-hooks/exhaustive-deps` comment
3. Use `jest.useFakeTimers()` + `act(() => jest.runAllTimers())` in tests to advance the 300ms `setTimeout`
