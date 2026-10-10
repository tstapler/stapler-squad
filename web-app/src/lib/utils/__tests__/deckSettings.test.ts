import {
  PINNED_COLLAPSE_MS,
  PINNED_COLLAPSE_STORAGE_KEY,
  UNDO_WINDOW_DEFAULT_MS,
  UNDO_WINDOW_STORAGE_KEY,
  readPinnedCollapseMs,
  readUndoWindowMs,
} from "@/lib/utils/deckSettings";

describe("deckSettings", () => {
  afterEach(() => {
    window.localStorage.clear();
    jest.restoreAllMocks();
  });

  it("defaults the pinned collapse to 8s", () => {
    expect(PINNED_COLLAPSE_MS).toBe(8_000);
    expect(readPinnedCollapseMs()).toBe(8_000);
  });

  it.each([8_000, 15_000, 30_000])("reads the %ims setting", (ms) => {
    window.localStorage.setItem(PINNED_COLLAPSE_STORAGE_KEY, String(ms));
    expect(readPinnedCollapseMs()).toBe(ms);
  });

  it("reads Never as null", () => {
    window.localStorage.setItem(PINNED_COLLAPSE_STORAGE_KEY, "never");
    expect(readPinnedCollapseMs()).toBeNull();
  });

  it("falls back to the default for an unknown value or when storage throws", () => {
    window.localStorage.setItem(PINNED_COLLAPSE_STORAGE_KEY, "1");
    expect(readPinnedCollapseMs()).toBe(8_000);

    jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    expect(readPinnedCollapseMs()).toBe(8_000);
    expect(readUndoWindowMs()).toBe(UNDO_WINDOW_DEFAULT_MS);
  });

  it("reads the undo window choices", () => {
    window.localStorage.setItem(UNDO_WINDOW_STORAGE_KEY, "15000");
    expect(readUndoWindowMs()).toBe(15_000);
    window.localStorage.setItem(UNDO_WINDOW_STORAGE_KEY, "999");
    expect(readUndoWindowMs()).toBe(8_000);
  });
});
