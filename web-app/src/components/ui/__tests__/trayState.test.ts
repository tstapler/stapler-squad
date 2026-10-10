import { selectTrayBanner, selectTrayState, type TrayStateInput } from "../trayState";

const base: TrayStateInput = {
  isOffline: false,
  loading: false,
  hasLoadError: false,
  hasLoadedOnce: true,
  rowCount: 0,
  filtered: false,
  needsAttentionCount: 0,
  hasMore: false,
};

describe("selectTrayState", () => {
  it.each([
    ["loading", { loading: true, hasLoadedOnce: false }, "loading", null],
    ["offline empty", { isOffline: true }, "offline", null],
    ["load error empty", { hasLoadError: true }, "load-error", "retry"],
    ["empty filtered", { filtered: true }, "empty-filtered", "clear-filters"],
    ["all caught up", {}, "all-caught-up", null],
    ["load more", { rowCount: 5, hasMore: true, needsAttentionCount: 1 }, "load-more", "load-more"],
    ["needs-attention empty", { rowCount: 5 }, "needs-attention-empty", null],
    ["list", { rowCount: 5, needsAttentionCount: 2 }, "list", null],
  ] as const)("%s", (_name, patch, kind, exit) => {
    const state = selectTrayState({ ...base, ...patch });
    expect(state.kind).toBe(kind);
    expect(state.exit).toBe(exit);
  });

  it("never reports All caught up while offline or after a failed load (TE-1)", () => {
    expect(selectTrayState({ ...base, isOffline: true }).kind).not.toBe("all-caught-up");
    expect(selectTrayState({ ...base, hasLoadError: true }).kind).not.toBe("all-caught-up");
  });

  it("keeps offline and load error as separate states (TE-3, TE-7)", () => {
    expect(selectTrayState({ ...base, isOffline: true, hasLoadError: true }).kind).toBe("offline");
    expect(selectTrayBanner({ isOffline: false, hasLoadError: true, rowCount: 3 })).toBe("load-error");
    expect(selectTrayBanner({ isOffline: true, hasLoadError: true, rowCount: 3 })).toBe("offline");
    expect(selectTrayBanner({ isOffline: false, hasLoadError: false, rowCount: 3 })).toBeNull();
  });
});
