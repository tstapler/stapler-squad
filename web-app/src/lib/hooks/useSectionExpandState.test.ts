import { renderHook, act } from "@testing-library/react";
import { useSectionExpandState } from "./useSectionExpandState";

const localStorageMock = (() => {
  let store: Record<string, string> = {};
  return {
    getItem: (k: string) => store[k] ?? null,
    setItem: (k: string, v: string) => {
      store[k] = v;
    },
    removeItem: (k: string) => {
      delete store[k];
    },
    clear: () => {
      store = {};
    },
  };
})();

Object.defineProperty(window, "localStorage", { value: localStorageMock });

describe("useSectionExpandState", () => {
  beforeEach(() => {
    localStorageMock.clear();
  });

  it("initializes to defaultExpanded when localStorage has no stored value", () => {
    const { result } = renderHook(() => useSectionExpandState("itm_a1b2c3", "version-control", false));
    expect(result.current[0]).toBe(false);
  });

  it("initializes from a stored localStorage value when present", () => {
    localStorageMock.setItem("backlog-detail-section-itm_a1b2c3-version-control", "true");
    const { result } = renderHook(() => useSectionExpandState("itm_a1b2c3", "version-control", false));
    expect(result.current[0]).toBe(true);
  });

  it("persists expand state to the itemId+sectionKey-scoped localStorage key", () => {
    const { result } = renderHook(() => useSectionExpandState("itm_a1b2c3", "version-control", false));

    act(() => {
      result.current[1](true);
    });

    expect(result.current[0]).toBe(true);
    expect(localStorageMock.getItem("backlog-detail-section-itm_a1b2c3-version-control")).toBe("true");
  });

  it("uses independent storage keys per item and per section", () => {
    const { result: r1 } = renderHook(() => useSectionExpandState("itm_a", "notes", false));
    const { result: r2 } = renderHook(() => useSectionExpandState("itm_b", "notes", false));

    act(() => {
      r1.current[1](true);
    });

    expect(r1.current[0]).toBe(true);
    expect(r2.current[0]).toBe(false);
  });

  it("useSectionExpandState_should_FallBackToDefaultExpanded_When_LocalStorageThrows", () => {
    const throwingStorage = {
      getItem: jest.fn(() => {
        throw new Error("Storage error");
      }),
      setItem: jest.fn(() => {
        throw new Error("Storage error");
      }),
      removeItem: jest.fn(),
      clear: jest.fn(),
    };
    Object.defineProperty(window, "localStorage", { value: throwingStorage });

    const { result } = renderHook(() => useSectionExpandState("itm_a1b2c3", "version-control", true));

    expect(result.current[0]).toBe(true);

    // Setting a new value must not throw even though localStorage.setItem throws.
    act(() => {
      result.current[1](false);
    });
    expect(result.current[0]).toBe(false);

    Object.defineProperty(window, "localStorage", { value: localStorageMock });
  });
});
