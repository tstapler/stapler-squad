import { renderHook, act } from '@testing-library/react';
import { useDebouncedCallback } from '../useDebounce';

describe('useDebouncedCallback', () => {
  beforeEach(() => { jest.useFakeTimers(); });
  afterEach(() => { jest.useRealTimers(); });

  it('useDebouncedCallback_should_invokeCallbackExactlyOnce_When_calledTwiceInSameTick', () => {
    const cb = jest.fn();
    const { result } = renderHook(() => useDebouncedCallback(cb, 300));

    act(() => {
      result.current('first');
      result.current('second');
    });
    act(() => { jest.advanceTimersByTime(300); });

    expect(cb).toHaveBeenCalledTimes(1);
    expect(cb).toHaveBeenCalledWith('second');
  });

  it('useDebouncedCallback_should_returnStableIdentity_When_callbackAndDelayUnchanged', () => {
    const cb = jest.fn();
    const { result, rerender } = renderHook(() => useDebouncedCallback(cb, 300));
    const first = result.current;
    rerender();
    expect(result.current).toBe(first);
  });
});
