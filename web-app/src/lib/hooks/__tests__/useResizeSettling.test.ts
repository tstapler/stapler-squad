/**
 * Tests for useResizeSettling — the single "resize settling" boundary
 * (BUG-101) that owns debounce, oscillation/bounce detection, and escalating
 * backoff for the terminal resize pipeline. Exercised directly here (as
 * opposed to through useTerminalFlowControl.ts's resize()), which keeps
 * these cases pinned to the boundary itself regardless of how call sites
 * around it change.
 */

import { renderHook, act } from '@testing-library/react';
import { useResizeSettling } from '../useResizeSettling';

describe('useResizeSettling', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.spyOn(console, 'log').mockImplementation(() => {});
  });

  afterEach(() => {
    jest.restoreAllMocks();
    jest.useRealTimers();
  });

  function setup(onSettled: jest.Mock<boolean, [number, number]> = jest.fn().mockReturnValue(true)) {
    const { result } = renderHook(() => useResizeSettling({ onSettled }));
    return { result, onSettled };
  }

  it('calls onSettled immediately for the first resize', () => {
    const { result, onSettled } = setup();

    act(() => { result.current.resize(120, 40); });

    expect(onSettled).toHaveBeenCalledTimes(1);
    expect(onSettled).toHaveBeenCalledWith(120, 40);
  });

  it('does not call onSettled again for the same (cols, rows) as the last settled value', () => {
    const { result, onSettled } = setup();

    act(() => { result.current.resize(120, 40); });
    act(() => { jest.advanceTimersByTime(201); });
    const before = onSettled.mock.calls.length;

    act(() => { result.current.resize(120, 40); });

    expect(onSettled.mock.calls.length).toBe(before);
  });

  it('throttles a second distinct resize to 200ms, deferring instead of dropping it', () => {
    const { result, onSettled } = setup();

    act(() => { result.current.resize(100, 30); });
    expect(onSettled).toHaveBeenCalledTimes(1);

    act(() => { result.current.resize(110, 35); });
    // Still throttled -- not sent yet.
    expect(onSettled).toHaveBeenCalledTimes(1);

    act(() => { jest.advanceTimersByTime(201); });
    // Trailing-edge send fires after the throttle window.
    expect(onSettled).toHaveBeenCalledTimes(2);
    expect(onSettled).toHaveBeenLastCalledWith(110, 35);
  });

  it('force:true bypasses value-dedup and the time throttle', () => {
    const { result, onSettled } = setup();

    act(() => { result.current.resize(100, 30); });
    expect(onSettled).toHaveBeenCalledTimes(1);

    act(() => { result.current.resize(100, 30, true); });
    expect(onSettled).toHaveBeenCalledTimes(2);
  });

  it('holds a direct bounce-back (A -> B -> A) instead of settling immediately', () => {
    const { result, onSettled } = setup();

    act(() => { result.current.resize(100, 30); }); // A
    act(() => { jest.advanceTimersByTime(201); });
    act(() => { result.current.resize(120, 40); }); // B
    act(() => { jest.advanceTimersByTime(201); });

    const beforeBounce = onSettled.mock.calls.length;
    act(() => { result.current.resize(100, 30); }); // back to A -- a direct bounce
    expect(onSettled.mock.calls.length).toBe(beforeBounce);

    act(() => { jest.advanceTimersByTime(3001); }); // past the base hold
    expect(onSettled.mock.calls.length).toBeGreaterThan(beforeBounce);
    expect(onSettled).toHaveBeenLastCalledWith(100, 30);
  });

  it('holds a repeat of a size from 3 sends ago (wider oscillation than a direct A -> B -> A bounce)', () => {
    const { result, onSettled } = setup();

    const sizes: [number, number][] = [[10, 6], [67, 38], [67, 22]];
    for (const [cols, rows] of sizes) {
      act(() => { result.current.resize(cols, rows); });
      act(() => { jest.advanceTimersByTime(201); });
    }

    const before = onSettled.mock.calls.length;
    act(() => { result.current.resize(10, 6); }); // repeats a size 3 sends back
    expect(onSettled.mock.calls.length).toBe(before);

    act(() => { jest.advanceTimersByTime(3001); });
    expect(onSettled.mock.calls.length).toBeGreaterThan(before);
    expect(onSettled).toHaveBeenLastCalledWith(10, 6);
  });

  it('does not treat a repeat of a size older than the bounce history window as a bounce', () => {
    const { result, onSettled } = setup();
    const sizes: [number, number][] = [[10, 6], [20, 7], [30, 8], [40, 9], [50, 10], [60, 11]];
    for (const [cols, rows] of sizes) {
      act(() => { result.current.resize(cols, rows); });
      act(() => { jest.advanceTimersByTime(201); });
    }
    const before = onSettled.mock.calls.length;
    act(() => { result.current.resize(10, 6); }); // aged out of the history window
    expect(onSettled.mock.calls.length).toBeGreaterThan(before);
  });

  it('escalates the hold duration when a bounce re-triggers before the previous hold elapses', () => {
    const { result, onSettled } = setup();

    act(() => { result.current.resize(100, 30); }); // A
    act(() => { jest.advanceTimersByTime(201); });
    act(() => { result.current.resize(120, 40); }); // B
    act(() => { jest.advanceTimersByTime(201); });

    const beforeBounces = onSettled.mock.calls.length;
    act(() => { result.current.resize(100, 30); }); // bounce 1 -- holds 3000ms
    act(() => { jest.advanceTimersByTime(1000); });
    act(() => { result.current.resize(100, 30); }); // bounce 2 -- escalates to 6000ms

    act(() => { jest.advanceTimersByTime(3001); }); // old 3000ms hold would have fired
    expect(onSettled.mock.calls.length).toBe(beforeBounces);

    act(() => { jest.advanceTimersByTime(3000); }); // total 6001ms since bounce 2
    expect(onSettled.mock.calls.length).toBeGreaterThan(beforeBounces);
  });

  it('caps the escalating hold at 15000ms instead of growing unbounded', () => {
    const { result, onSettled } = setup();

    act(() => { result.current.resize(100, 30); }); // A
    act(() => { jest.advanceTimersByTime(201); });
    act(() => { result.current.resize(120, 40); }); // B
    act(() => { jest.advanceTimersByTime(201); });

    act(() => { result.current.resize(100, 30); }); // streak 1 -- holds 3000ms
    act(() => { jest.advanceTimersByTime(500); });
    act(() => { result.current.resize(100, 30); }); // streak 2 -- holds 6000ms
    act(() => { jest.advanceTimersByTime(500); });
    act(() => { result.current.resize(100, 30); }); // streak 3 -- holds 12000ms
    act(() => { jest.advanceTimersByTime(500); });

    const before = onSettled.mock.calls.length;
    act(() => { result.current.resize(100, 30); }); // streak 4 -- would hold 24000ms uncapped

    act(() => { jest.advanceTimersByTime(14999); });
    expect(onSettled.mock.calls.length).toBe(before);

    act(() => { jest.advanceTimersByTime(2); });
    expect(onSettled.mock.calls.length).toBeGreaterThan(before);
  });

  it('resets the bounce hold to the base duration after a bounce resolves', () => {
    const { result, onSettled } = setup();

    act(() => { result.current.resize(100, 30); }); // A
    act(() => { jest.advanceTimersByTime(201); });
    act(() => { result.current.resize(120, 40); }); // B
    act(() => { jest.advanceTimersByTime(201); });
    act(() => { result.current.resize(140, 50); }); // C
    act(() => { jest.advanceTimersByTime(201); });

    act(() => { result.current.resize(100, 30); }); // bounce on A -- base 3000ms hold
    act(() => { jest.advanceTimersByTime(3001); }); // resolves

    const before = onSettled.mock.calls.length;
    act(() => { result.current.resize(120, 40); }); // unrelated bounce on B

    act(() => { jest.advanceTimersByTime(2999); });
    expect(onSettled.mock.calls.length).toBe(before); // still held at base

    act(() => { jest.advanceTimersByTime(2); });
    // Base 3000ms hold elapsed -- if the streak hadn't reset this would still
    // be held (the escalated 6000ms hold from before the first bounce resolved).
    expect(onSettled.mock.calls.length).toBeGreaterThan(before);
  });

  it('does not record a failed send as the last-sent value, so a repeat is not falsely deduped', () => {
    const onSettled = jest.fn().mockReturnValueOnce(false).mockReturnValue(true);
    const { result } = renderHook(() => useResizeSettling({ onSettled }));

    act(() => { result.current.resize(90, 20); }); // fails
    expect(onSettled).toHaveBeenCalledTimes(1);

    act(() => { result.current.resize(90, 20); }); // must NOT be deduped
    expect(onSettled).toHaveBeenCalledTimes(2);
  });

  it('cancels a still-pending deferred resize when a bounce-back call dedups against the last-sent value', () => {
    const { result, onSettled } = setup();

    act(() => { result.current.resize(80, 24); }); // A -- sends immediately
    expect(onSettled).toHaveBeenCalledTimes(1);

    act(() => { jest.advanceTimersByTime(10); });
    act(() => { result.current.resize(85, 24); }); // B -- deferred, within throttle window
    expect(onSettled).toHaveBeenCalledTimes(1);

    act(() => { result.current.resize(80, 24); }); // bounce back to A -- dedups, cancels pending B
    act(() => { jest.advanceTimersByTime(300); });

    // The deferred {85, 24} must never have fired.
    expect(onSettled).not.toHaveBeenCalledWith(85, 24);
  });
});
