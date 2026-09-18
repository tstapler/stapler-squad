// Epic 6.1 (terminal:resync-stagger) — unit tests for the per-`SessionDetailView`
// stagger coordinator (`ResyncStaggerQueue`/`useResyncStaggerQueue` in
// SessionDetailView.tsx). Covers Tasks 6.1.1.4 (jitter spread + preempt),
// 6.1.1.5 (cross-session non-coordination — a documented, deliberate gap, not
// an oversight) and 6.1.1.6 (unmount cleanup cancels pending timers).

import { renderHook, act } from '@testing-library/react';
import { ResyncStaggerQueue, useResyncStaggerQueue } from '../SessionDetailView';

describe('ResyncStaggerQueue', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  test('staggerQueue_should_SpreadResyncCallsAcrossJitterWindow_When_ThreeInstancesBecomeVisibleWithin50ms', () => {
    // Deterministic jitter: Math.random() -> 0.1, 0.5, 0.9 => delays of
    // 30ms, 150ms, 270ms out of the 300ms window (see RESYNC_STAGGER_JITTER_MAX_MS).
    const randomValues = [0.1, 0.5, 0.9];
    let callIndex = 0;
    jest.spyOn(Math, 'random').mockImplementation(() => randomValues[callIndex++]);

    const queue = new ResyncStaggerQueue();
    const fireA = jest.fn();
    const fireB = jest.fn();
    const fireC = jest.fn();

    queue.schedule('instance-a', fireA, { preempt: false });
    queue.schedule('instance-b', fireB, { preempt: false });
    queue.schedule('instance-c', fireC, { preempt: false });

    // None fire synchronously.
    expect(fireA).not.toHaveBeenCalled();
    expect(fireB).not.toHaveBeenCalled();
    expect(fireC).not.toHaveBeenCalled();

    act(() => {
      jest.advanceTimersByTime(30);
    });
    expect(fireA).toHaveBeenCalledTimes(1);
    expect(fireB).not.toHaveBeenCalled();
    expect(fireC).not.toHaveBeenCalled();

    act(() => {
      jest.advanceTimersByTime(120); // total 150ms
    });
    expect(fireB).toHaveBeenCalledTimes(1);
    expect(fireC).not.toHaveBeenCalled();

    act(() => {
      jest.advanceTimersByTime(120); // total 270ms
    });
    expect(fireC).toHaveBeenCalledTimes(1);
  });

  // ── Task 7.1.1.1 (Epic 7.1 observability) — resync-burst-size log ────────

  test('staggerQueue_should_LogBurstSize_When_ThreeInstancesAreScheduledTogether', () => {
    const debugSpy = jest.spyOn(console, 'debug').mockImplementation(() => {});
    const queue = new ResyncStaggerQueue();

    queue.schedule('instance-a', jest.fn(), { preempt: false });
    queue.schedule('instance-b', jest.fn(), { preempt: false });
    queue.schedule('instance-c', jest.fn(), { preempt: false });

    expect(debugSpy).toHaveBeenCalledWith(expect.stringContaining('burst size=1'));
    expect(debugSpy).toHaveBeenCalledWith(expect.stringContaining('burst size=2'));
    expect(debugSpy).toHaveBeenCalledWith(expect.stringContaining('burst size=3'));
  });

  test('staggerQueue_should_LogBatchedBurstSize_When_BatchingEnabledAndMultipleInstancesShareWindow', () => {
    const debugSpy = jest.spyOn(console, 'debug').mockImplementation(() => {});
    const queue = new ResyncStaggerQueue(true);

    queue.schedule('instance-a', jest.fn(), { preempt: false });
    queue.schedule('instance-b', jest.fn(), { preempt: false });

    expect(debugSpy).toHaveBeenCalledWith(expect.stringContaining('burst size=2'));
    expect(debugSpy).toHaveBeenCalledWith(expect.stringContaining('batched=true'));
  });

  test('staggerQueue_should_PreemptQueuedEntries_When_NewInstanceBecomesVisibleWhileOthersQueued', () => {
    jest.spyOn(Math, 'random').mockReturnValue(0.5);

    const queue = new ResyncStaggerQueue();
    const fireQueued = jest.fn();
    const firePreempt = jest.fn();

    queue.schedule('instance-a', fireQueued, { preempt: false });
    expect(fireQueued).not.toHaveBeenCalled();

    // A different instance preempts — fires immediately, synchronously,
    // without waiting for any jitter delay.
    queue.schedule('instance-b', firePreempt, { preempt: true });
    expect(firePreempt).toHaveBeenCalledTimes(1);

    // The originally-queued entry (a different instanceId) is untouched by
    // another instance's preempt — it still fires on its own timer.
    act(() => {
      jest.advanceTimersByTime(150);
    });
    expect(fireQueued).toHaveBeenCalledTimes(1);
  });

  test('staggerQueue_should_PreemptItsOwnQueuedEntry_When_SameInstanceReschedulesWithPreempt', () => {
    jest.spyOn(Math, 'random').mockReturnValue(0.5);

    const queue = new ResyncStaggerQueue();
    const firstFire = jest.fn();
    const secondFire = jest.fn();

    queue.schedule('instance-a', firstFire, { preempt: false });
    // Re-scheduling the same instanceId with preempt:true cancels the
    // pending queued entry and fires the new one immediately.
    queue.schedule('instance-a', secondFire, { preempt: true });
    expect(secondFire).toHaveBeenCalledTimes(1);

    act(() => {
      jest.advanceTimersByTime(300);
    });
    // The superseded first entry must never fire.
    expect(firstFire).not.toHaveBeenCalled();
  });
});

describe('useResyncStaggerQueue', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    jest.restoreAllMocks();
  });

  test('staggerCoordinator_should_NotCoordinateAcrossMultipleSessionDetailViews_When_TwoSessionsBecomeVisibleSimultaneously', () => {
    // Task 6.1.1.5 — the stagger coordinator is deliberately scoped per
    // `SessionDetailView` instance (one `ResyncStaggerQueue` per call to
    // useResyncStaggerQueue), not shared across sessions/views. Two
    // independent hook instances (standing in for two mounted
    // SessionDetailViews) must not stagger each other's resyncs relative to
    // one another — each queue only staggers entries scheduled through
    // itself. This is a documented, deliberate gap (see SessionDetailView.tsx
    // doc comments on ResyncStaggerQueue) and a known follow-up candidate for
    // cross-session coordination, not something this test is asserting as
    // desirable — only as the current, intentional behavior.
    jest.spyOn(Math, 'random').mockReturnValue(0); // 0ms jitter for both

    const { result: viewA } = renderHook(() => useResyncStaggerQueue(true));
    const { result: viewB } = renderHook(() => useResyncStaggerQueue(true));

    const fireA = jest.fn();
    const fireB = jest.fn();

    const scheduleA = viewA.current('pool-1');
    const scheduleB = viewB.current('pool-1'); // same instanceId, different view/queue

    act(() => {
      scheduleA?.(fireA, { preempt: false });
      scheduleB?.(fireB, { preempt: false });
    });

    // Both fire independently on their own 0ms timer — neither view's queue
    // knows about, delays, or preempts the other's entry.
    act(() => {
      jest.advanceTimersByTime(0);
    });
    expect(fireA).toHaveBeenCalledTimes(1);
    expect(fireB).toHaveBeenCalledTimes(1);
  });

  test('staggerCoordinator_should_ClearPendingTimers_When_ComponentUnmounts', () => {
    // Task 6.1.1.6 — a queued stagger callback must never fire after the
    // owning SessionDetailView (and its useResyncStaggerQueue instance)
    // unmounts.
    jest.spyOn(Math, 'random').mockReturnValue(0.5); // 150ms delay

    const { result, unmount } = renderHook(() => useResyncStaggerQueue(true));
    const fire = jest.fn();

    const schedule = result.current('pool-1');
    act(() => {
      schedule?.(fire, { preempt: false });
    });
    expect(fire).not.toHaveBeenCalled();

    unmount();

    act(() => {
      jest.advanceTimersByTime(300);
    });
    expect(fire).not.toHaveBeenCalled();
  });

  test('useResyncStaggerQueue_should_ReturnUndefinedSchedule_When_FlagIsDisabled', () => {
    // AC7 flag-off parity: when `terminal:resync-stagger` is off, the factory
    // must hand back `undefined` for every instance so callers
    // (TerminalOutput/useVisibilityResync) fall back to their pre-Epic-6.1
    // synchronous-fire behavior instead of routing through a queue.
    const { result } = renderHook(() => useResyncStaggerQueue(false));
    expect(result.current('pool-1')).toBeUndefined();
  });

  test('staggerCoordinator_should_SendNSeparateRequests_When_BatchingFlagOff', () => {
    // AC6b hard requirement (Epic 5.2/Task 5.2.1.3, `terminal:resync-batching`
    // default off): with the stagger flag on but the batching flag off (the
    // default combination), three sibling resyncs queued in the same window
    // must still fire as three independent, separately-timed calls —
    // unchanged from pre-Epic-5.2 behavior — not coalesced onto one shared
    // batch timer. Each `fire` stands in for a caller sending its own
    // `CurrentPaneRequest`, so three independent fires means three separate
    // wire sends, not one `BatchedCurrentPaneRequest`.
    const randomValues = [0.1, 0.5, 0.9];
    let callIndex = 0;
    jest.spyOn(Math, 'random').mockImplementation(() => randomValues[callIndex++]);

    // batchingEnabled defaults to false when omitted — this call site
    // deliberately mirrors useResyncStaggerQueue(enabled) with no second
    // argument, matching the default-off flag combination.
    const { result } = renderHook(() => useResyncStaggerQueue(true));

    const fireA = jest.fn();
    const fireB = jest.fn();
    const fireC = jest.fn();
    const schedule = result.current('pool-1');

    act(() => {
      schedule?.(fireA, { preempt: false });
    });
    const scheduleB = result.current('pool-2');
    const scheduleC = result.current('pool-3');
    act(() => {
      scheduleB?.(fireB, { preempt: false });
      scheduleC?.(fireC, { preempt: false });
    });

    // Distinct per-instance jitter delays (30ms, 150ms, 270ms) — each fires
    // on its own schedule, never grouped into a single batched callback.
    act(() => {
      jest.advanceTimersByTime(30);
    });
    expect(fireA).toHaveBeenCalledTimes(1);
    expect(fireB).not.toHaveBeenCalled();
    expect(fireC).not.toHaveBeenCalled();

    act(() => {
      jest.advanceTimersByTime(120); // total 150ms
    });
    expect(fireB).toHaveBeenCalledTimes(1);
    expect(fireC).not.toHaveBeenCalled();

    act(() => {
      jest.advanceTimersByTime(120); // total 270ms
    });
    expect(fireC).toHaveBeenCalledTimes(1);
  });

  test('staggerCoordinator_should_CoalesceSameWindowFiresOntoOneSharedTimer_When_BatchingFlagOn', () => {
    // Task 5.2.1.3's actual behavior change: with `terminal:resync-batching`
    // on, entries scheduled while a coalescing window is open share one
    // timer, so they fire together rather than at three independently
    // jittered delays. See ResyncStaggerQueue.scheduleBatched's doc comment
    // for the documented limitation that this coalesces *when* fires happen,
    // not yet the wire-level request count (that requires hook changes
    // outside Epic 5.2's file scope).
    jest.spyOn(Math, 'random').mockReturnValue(0.5); // 150ms shared delay

    const { result } = renderHook(() => useResyncStaggerQueue(true, true));

    const fireA = jest.fn();
    const fireB = jest.fn();
    const fireC = jest.fn();

    act(() => {
      result.current('pool-1')?.(fireA, { preempt: false });
      result.current('pool-2')?.(fireB, { preempt: false });
      result.current('pool-3')?.(fireC, { preempt: false });
    });

    // Not fired before the shared window elapses.
    act(() => {
      jest.advanceTimersByTime(100);
    });
    expect(fireA).not.toHaveBeenCalled();
    expect(fireB).not.toHaveBeenCalled();
    expect(fireC).not.toHaveBeenCalled();

    // All three fire together once the single shared timer elapses.
    act(() => {
      jest.advanceTimersByTime(50); // total 150ms
    });
    expect(fireA).toHaveBeenCalledTimes(1);
    expect(fireB).toHaveBeenCalledTimes(1);
    expect(fireC).toHaveBeenCalledTimes(1);
  });

  test('useResyncStaggerQueue_should_PickUpLiveBatchingFlagToggle_When_FlagFlipsWithoutRemount', () => {
    // Regression test: `batchingEnabled` comes from
    // `useFeatureFlag("terminal:resync-batching")`, which is live context
    // state that can change (e.g. toggled from /settings/features) without
    // the owning SessionDetailView remounting. The lazy `queueRef.current`
    // construction in useResyncStaggerQueue used to capture batchingEnabled
    // only once, at first-enable time, so a live toggle was silently
    // ignored for the rest of the component's lifetime. This asserts the
    // queue picks up the new mode on the very next schedule() call after the
    // flag flips, with no dropped or duplicated fires across the toggle.
    jest.spyOn(Math, 'random').mockReturnValue(0.5); // 150ms delay/shared window

    const { result, rerender } = renderHook(
      ({ batching }: { batching: boolean }) => useResyncStaggerQueue(true, batching),
      { initialProps: { batching: false } },
    );

    // Starts in non-batching mode: two entries fire independently, each on
    // its own timer (both 150ms here since jitter is mocked constant).
    const fireA = jest.fn();
    const fireB = jest.fn();
    act(() => {
      result.current('pool-1')?.(fireA, { preempt: false });
      result.current('pool-2')?.(fireB, { preempt: false });
    });
    act(() => {
      jest.advanceTimersByTime(150);
    });
    expect(fireA).toHaveBeenCalledTimes(1);
    expect(fireB).toHaveBeenCalledTimes(1);

    // Flip the flag live, without unmounting — mirrors toggling
    // terminal:resync-batching from /settings/features while this view stays
    // open.
    rerender({ batching: true });

    // New entries scheduled after the toggle must now coalesce onto one
    // shared batching timer instead of independent per-instance timers.
    const fireC = jest.fn();
    const fireD = jest.fn();
    act(() => {
      result.current('pool-3')?.(fireC, { preempt: false });
      result.current('pool-4')?.(fireD, { preempt: false });
    });
    act(() => {
      jest.advanceTimersByTime(100);
    });
    // Not yet fired — still waiting on the shared coalescing window, proving
    // the post-toggle schedule() calls actually took the batching branch.
    expect(fireC).not.toHaveBeenCalled();
    expect(fireD).not.toHaveBeenCalled();

    act(() => {
      jest.advanceTimersByTime(50); // total 150ms
    });
    expect(fireC).toHaveBeenCalledTimes(1);
    expect(fireD).toHaveBeenCalledTimes(1);

    // No entry fired more than once across the whole toggle boundary.
    expect(fireA).toHaveBeenCalledTimes(1);
    expect(fireB).toHaveBeenCalledTimes(1);
  });
});
