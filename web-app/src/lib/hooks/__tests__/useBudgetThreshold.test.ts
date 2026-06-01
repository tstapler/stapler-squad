import { renderHook, act } from "@testing-library/react";
import { useBudgetThreshold } from "@/lib/hooks/useBudgetThreshold";

const STORAGE_KEY = "insights_budget_threshold_usd";

describe("useBudgetThreshold", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("useBudgetThreshold_should_returnIsHydratedFalse_When_mountedWithoutEffect", () => {
    // isHydrated initializes to false — check before effects run
    let capturedInitial: boolean | undefined;
    renderHook(() => {
      const hook = useBudgetThreshold();
      // Capture on the very first render, before useEffect fires
      if (capturedInitial === undefined) capturedInitial = hook.isHydrated;
      return hook;
    });
    expect(capturedInitial).toBe(false);
  });

  it("useBudgetThreshold_should_loadFromLocalStorage_When_hydrated", async () => {
    localStorage.setItem(STORAGE_KEY, "50");
    const { result } = renderHook(() => useBudgetThreshold());
    await act(async () => {});
    expect(result.current.isHydrated).toBe(true);
    expect(result.current.threshold).toBe(50);
  });

  it("useBudgetThreshold_should_writeToLocalStorage_When_setThresholdCalled", async () => {
    const { result } = renderHook(() => useBudgetThreshold());
    await act(async () => {});
    act(() => {
      result.current.setThreshold(100);
    });
    expect(localStorage.getItem(STORAGE_KEY)).toBe("100");
    expect(result.current.threshold).toBe(100);
  });

  it("useBudgetThreshold_should_removeFromLocalStorage_When_setThresholdNull", async () => {
    localStorage.setItem(STORAGE_KEY, "50");
    const { result } = renderHook(() => useBudgetThreshold());
    await act(async () => {});
    act(() => {
      result.current.setThreshold(null);
    });
    expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
    expect(result.current.threshold).toBeNull();
  });

  it("useBudgetThreshold_should_returnNullThreshold_When_noStoredValue", async () => {
    const { result } = renderHook(() => useBudgetThreshold());
    await act(async () => {});
    expect(result.current.threshold).toBeNull();
  });
});
