import { renderHook, act, waitFor } from "@testing-library/react";
import { usePersistedViewState, type PersistedFieldsConfig } from "./usePersistedViewState";

interface TestState {
  search: string;
  count: number;
  tags: Set<string>;
}

function fields(): PersistedFieldsConfig<TestState> {
  return {
    search: { key: "test-search", defaultValue: "" },
    count: {
      key: "test-count",
      defaultValue: 0,
      isValid: (v) => typeof v === "number",
    },
    tags: {
      key: "test-tags",
      defaultValue: new Set<string>(),
      serialize: (value) => Array.from(value),
      deserialize: (raw) => new Set(raw as string[]),
      isValid: (v) => v instanceof Set,
    },
  };
}

describe("usePersistedViewState", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("hydrates without error and settles on hardcoded defaults when nothing is persisted", async () => {
    const { result } = renderHook(() => usePersistedViewState<TestState>(fields()));

    await waitFor(() => expect(result.current.isHydrated).toBe(true));
    expect(result.current.state.search).toBe("");
    expect(result.current.state.count).toBe(0);
    expect(result.current.state.tags).toEqual(new Set());
  });

  it("loads persisted values from localStorage on hydration", async () => {
    localStorage.setItem("test-search", JSON.stringify("hello"));
    localStorage.setItem("test-count", JSON.stringify(42));
    localStorage.setItem("test-tags", JSON.stringify(["a", "b"]));

    const { result } = renderHook(() => usePersistedViewState<TestState>(fields()));
    await waitFor(() => expect(result.current.isHydrated).toBe(true));

    expect(result.current.state.search).toBe("hello");
    expect(result.current.state.count).toBe(42);
    expect(result.current.state.tags).toEqual(new Set(["a", "b"]));
  });

  it("round-trips a setter change through localStorage to a fresh mount", async () => {
    const { result, unmount } = renderHook(() => usePersistedViewState<TestState>(fields()));
    await waitFor(() => expect(result.current.isHydrated).toBe(true));

    act(() => {
      result.current.setters.search("persisted-value");
    });

    await waitFor(() => expect(localStorage.getItem("test-search")).toBe(JSON.stringify("persisted-value")));

    unmount();

    const { result: remounted } = renderHook(() => usePersistedViewState<TestState>(fields()));
    await waitFor(() => expect(remounted.current.isHydrated).toBe(true));
    expect(remounted.current.state.search).toBe("persisted-value");
  });

  it("falls back to the default when the persisted value is corrupted JSON", async () => {
    localStorage.setItem("test-search", "{not valid json");

    const { result } = renderHook(() => usePersistedViewState<TestState>(fields()));
    await waitFor(() => expect(result.current.isHydrated).toBe(true));

    expect(result.current.state.search).toBe("");
  });

  it("falls back to the default when a persisted value fails isValid", async () => {
    localStorage.setItem("test-count", JSON.stringify("not-a-number"));

    const { result } = renderHook(() => usePersistedViewState<TestState>(fields()));
    await waitFor(() => expect(result.current.isHydrated).toBe(true));

    expect(result.current.state.count).toBe(0);
  });

  it("does not crash and still updates in-memory state when localStorage.setItem throws", async () => {
    const { result } = renderHook(() => usePersistedViewState<TestState>(fields()));
    await waitFor(() => expect(result.current.isHydrated).toBe(true));

    const spy = jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("quota exceeded");
    });

    expect(() => {
      act(() => {
        result.current.setters.search("still-works-in-memory");
      });
    }).not.toThrow();

    expect(result.current.state.search).toBe("still-works-in-memory");

    spy.mockRestore();
  });

  it("resetToDefaults restores default state in memory and removes all localStorage keys", async () => {
    const { result } = renderHook(() => usePersistedViewState<TestState>(fields()));
    await waitFor(() => expect(result.current.isHydrated).toBe(true));

    act(() => {
      result.current.setters.search("changed");
      result.current.setters.count(7);
    });
    await waitFor(() => expect(localStorage.getItem("test-search")).toBe(JSON.stringify("changed")));

    act(() => {
      result.current.resetToDefaults();
    });

    expect(result.current.state.search).toBe("");
    expect(result.current.state.count).toBe(0);
    expect(result.current.state.tags).toEqual(new Set());
    expect(localStorage.getItem("test-search")).toBeNull();
    expect(localStorage.getItem("test-count")).toBeNull();
    expect(localStorage.getItem("test-tags")).toBeNull();
  });

  it("clearStorage removes keys without touching in-memory state", async () => {
    const { result } = renderHook(() => usePersistedViewState<TestState>(fields()));
    await waitFor(() => expect(result.current.isHydrated).toBe(true));

    act(() => {
      result.current.setters.search("kept-in-memory");
    });
    await waitFor(() => expect(localStorage.getItem("test-search")).toBe(JSON.stringify("kept-in-memory")));

    act(() => {
      result.current.clearStorage();
    });

    expect(result.current.state.search).toBe("kept-in-memory");
    expect(localStorage.getItem("test-search")).toBeNull();
  });

  it("namespaces different field-key sets so they don't collide", async () => {
    localStorage.setItem("prefix-a-search", JSON.stringify("value-a"));
    localStorage.setItem("prefix-b-search", JSON.stringify("value-b"));

    const fieldsA: PersistedFieldsConfig<{ search: string }> = {
      search: { key: "prefix-a-search", defaultValue: "" },
    };
    const fieldsB: PersistedFieldsConfig<{ search: string }> = {
      search: { key: "prefix-b-search", defaultValue: "" },
    };

    const { result: a } = renderHook(() => usePersistedViewState(fieldsA));
    const { result: b } = renderHook(() => usePersistedViewState(fieldsB));

    await waitFor(() => expect(a.current.isHydrated).toBe(true));
    await waitFor(() => expect(b.current.isHydrated).toBe(true));

    expect(a.current.state.search).toBe("value-a");
    expect(b.current.state.search).toBe("value-b");
  });
});
