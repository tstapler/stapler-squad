/**
 * Tests for useDropEpisodeCoalescer (Story 2.3, Task 2.3.3a/2.3.3b).
 *
 * Written and passing before the hook is wired into TerminalOutput.tsx
 * (Task 2.3.3c), per architecture-review.md's testability-gap concern.
 */
import { renderHook, act } from '@testing-library/react';
import { useDropEpisodeCoalescer } from '../useDropEpisodeCoalescer';

describe('useDropEpisodeCoalescer', () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it('coalesces 3 report(1) calls within the 400ms window into one onFlush call with the summed count', () => {
    const onFlush = jest.fn();
    const { result } = renderHook(() => useDropEpisodeCoalescer(onFlush, 400));

    act(() => {
      result.current(1);
      jest.advanceTimersByTime(100);
      result.current(1);
      jest.advanceTimersByTime(100);
      result.current(1);
    });

    // Window hasn't elapsed since the last report() yet.
    expect(onFlush).not.toHaveBeenCalled();

    act(() => {
      jest.advanceTimersByTime(400);
    });

    expect(onFlush).toHaveBeenCalledTimes(1);
    expect(onFlush).toHaveBeenCalledWith(3);
  });

  it('a report() call after the window has already flushed produces a second, independent onFlush call (not merged with the first episode)', () => {
    const onFlush = jest.fn();
    const { result } = renderHook(() => useDropEpisodeCoalescer(onFlush, 400));

    act(() => {
      result.current(2);
    });
    act(() => {
      jest.advanceTimersByTime(400);
    });

    expect(onFlush).toHaveBeenCalledTimes(1);
    expect(onFlush).toHaveBeenNthCalledWith(1, 2);

    // A brand-new episode starts here — must not carry the first episode's
    // count forward (design/ux.md §2.3 Case C: "replace, don't merge").
    act(() => {
      result.current(1);
    });
    act(() => {
      jest.advanceTimersByTime(400);
    });

    expect(onFlush).toHaveBeenCalledTimes(2);
    expect(onFlush).toHaveBeenNthCalledWith(2, 1);
  });

  it('restarts the window on every report() call within it, not just the first', () => {
    const onFlush = jest.fn();
    const { result } = renderHook(() => useDropEpisodeCoalescer(onFlush, 400));

    act(() => {
      result.current(1);
      jest.advanceTimersByTime(300);
      result.current(1);
      // 300ms after the first report, window would have elapsed if it
      // hadn't been restarted by the second report() call.
      jest.advanceTimersByTime(300);
    });

    expect(onFlush).not.toHaveBeenCalled();

    act(() => {
      jest.advanceTimersByTime(100);
    });

    expect(onFlush).toHaveBeenCalledTimes(1);
    expect(onFlush).toHaveBeenCalledWith(2);
  });
});
