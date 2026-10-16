/**
 * Tests for useAbortableRequest — the shared primitive behind every hook's
 * `{ signal }` cancellation (see the hook's doc comment for the
 * useSessionVcs heap-growth incident that motivated it).
 */

import { renderHook } from "@testing-library/react";
import { useAbortableRequest } from "@/lib/hooks/useAbortableRequest";

describe("useAbortableRequest", () => {
  it("useAbortableRequest_should_returnFreshUnabortedSignal_When_startCalled", () => {
    const { result } = renderHook(() => useAbortableRequest());

    const signal = result.current();

    expect(signal.aborted).toBe(false);
  });

  it("useAbortableRequest_should_abortPriorSignal_When_startCalledAgain", () => {
    const { result } = renderHook(() => useAbortableRequest());

    const first = result.current();
    const second = result.current();

    expect(first.aborted).toBe(true);
    expect(second.aborted).toBe(false);
  });

  it("useAbortableRequest_should_abortLatestSignal_When_unmounted", () => {
    const { result, unmount } = renderHook(() => useAbortableRequest());

    const signal = result.current();
    expect(signal.aborted).toBe(false);

    unmount();

    expect(signal.aborted).toBe(true);
  });

  it("useAbortableRequest_should_notThrow_When_unmountedWithoutEverStarting", () => {
    const { unmount } = renderHook(() => useAbortableRequest());

    expect(() => unmount()).not.toThrow();
  });
});
