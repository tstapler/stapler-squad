/**
 * Tests for useAbortableEffect — the useEffect+useAbortableRequest wrapper
 * most hooks in this directory use instead of hand-rolling an
 * AbortController ref.
 */

import { renderHook } from "@testing-library/react";
import { useAbortableEffect } from "@/lib/hooks/useAbortableEffect";

describe("useAbortableEffect", () => {
  it("useAbortableEffect_should_runEffectWithUnabortedSignal_When_mounted", () => {
    const effect = jest.fn().mockResolvedValue(undefined);

    renderHook(() => useAbortableEffect(effect, []));

    expect(effect).toHaveBeenCalledTimes(1);
    const signal = effect.mock.calls[0][0] as AbortSignal;
    expect(signal.aborted).toBe(false);
  });

  it("useAbortableEffect_should_abortPriorSignalAndRerun_When_depsChange", () => {
    const effect = jest.fn().mockResolvedValue(undefined);

    const { rerender } = renderHook(({ id }) => useAbortableEffect(effect, [id]), {
      initialProps: { id: "a" },
    });

    const firstSignal = effect.mock.calls[0][0] as AbortSignal;
    expect(firstSignal.aborted).toBe(false);

    rerender({ id: "b" });

    expect(effect).toHaveBeenCalledTimes(2);
    expect(firstSignal.aborted).toBe(true);
    const secondSignal = effect.mock.calls[1][0] as AbortSignal;
    expect(secondSignal.aborted).toBe(false);
  });

  it("useAbortableEffect_should_notRerun_When_depsUnchanged", () => {
    const effect = jest.fn().mockResolvedValue(undefined);

    const { rerender } = renderHook(({ id }) => useAbortableEffect(effect, [id]), {
      initialProps: { id: "a" },
    });

    rerender({ id: "a" });

    expect(effect).toHaveBeenCalledTimes(1);
  });

  it("useAbortableEffect_should_abortSignal_When_unmounted", () => {
    const effect = jest.fn().mockResolvedValue(undefined);

    const { unmount } = renderHook(() => useAbortableEffect(effect, []));

    const signal = effect.mock.calls[0][0] as AbortSignal;
    unmount();

    expect(signal.aborted).toBe(true);
  });

  it("useAbortableEffect_should_notSurfaceUnhandledRejection_When_effectThrowsWithoutCatching", async () => {
    const effect = jest.fn().mockRejectedValue(new Error("boom"));

    expect(() => renderHook(() => useAbortableEffect(effect, []))).not.toThrow();

    // Flush the microtask queue so the rejected promise's .catch() runs
    // before the test ends — otherwise Jest can report it as unhandled.
    await Promise.resolve();
    await Promise.resolve();
  });
});
