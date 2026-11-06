import { readFontPx, stepFontPx, DESKTOP_DEFAULT_PX } from "../fontSize";
import { readLargeFiles, writeLargeFiles } from "../largeFilesPref";

describe("session fallback when localStorage cannot be written", () => {
  beforeEach(() => {
    window.localStorage.clear();
    jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => { throw new Error("QuotaExceededError"); });
  });
  afterEach(() => jest.restoreAllMocks());

  it("fontSize_should_AccumulateSteps_When_StorageWriteThrows", () => {
    stepFontPx(1, false);
    stepFontPx(1, false);

    expect(readFontPx(false)).toBe(DESKTOP_DEFAULT_PX + 2);
  });

  it("largeFiles_should_ApplyTheToggle_When_StorageWriteThrows", () => {
    writeLargeFiles(true);

    expect(readLargeFiles()).toBe(true);
    writeLargeFiles(false);
    expect(readLargeFiles()).toBe(false);
  });
});
