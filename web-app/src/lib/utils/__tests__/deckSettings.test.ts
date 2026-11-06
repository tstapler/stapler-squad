import {
  PINNED_COLLAPSE_MS,
  PINNED_COLLAPSE_STORAGE_KEY,
  UNDO_WINDOW_DEFAULT_MS,
  UNDO_WINDOW_STORAGE_KEY,
  readPinnedCollapseMs,
  readUndoWindowMs,
  BELL_POS_STORAGE_KEY,
  readBellPos,
  writeBellPos,
  resetBellPos,
  readFloatingBell,
  writeFloatingBell,
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

  it("round-trips the bell position and falls back to null on junk", () => {
    expect(readBellPos()).toBeNull();
    writeBellPos({ v: 1, edge: "right", y: 0.4 });
    expect(readBellPos()).toEqual({ v: 1, edge: "right", y: 0.4 });
    window.localStorage.setItem(BELL_POS_STORAGE_KEY, "{broken");
    expect(readBellPos()).toBeNull();
  });

  it("resets the bell position", () => {
    writeBellPos({ v: 1, edge: "left", y: 0.9 });
    resetBellPos();
    expect(readBellPos()).toBeNull();
  });

  it("floating bell defaults off and persists", () => {
    expect(readFloatingBell()).toBe(false);
    writeFloatingBell(true);
    expect(readFloatingBell()).toBe(true);
  });

  it("bell reads and writes survive blocked storage", () => {
    jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    expect(readBellPos()).toBeNull();
    expect(readFloatingBell()).toBe(false);
    expect(() => writeBellPos({ v: 1, edge: "right", y: 0.1 })).not.toThrow();
  });
});
