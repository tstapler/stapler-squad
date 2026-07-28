import {
  shouldFit,
  shouldSendResize,
  shouldAbandonWebgl,
  type ResizeEvent,
} from "../resizeConvergence";

describe("shouldFit", () => {
  it("shouldFit_should_returnFalse_When_proposedEqualsCurrentDimensions", () => {
    // Given terminal.cols=84, rows=60 and proposeDimensions() returns {cols:84,rows:60}
    // after a 0.3px container wobble (WebGL glyph-metric mismatch case).
    expect(shouldFit({ cols: 84, rows: 60 }, { cols: 84, rows: 60 })).toBe(false);
  });

  it("shouldFit_should_returnTrue_When_proposedDiffersOnColsOnly", () => {
    expect(shouldFit({ cols: 85, rows: 60 }, { cols: 84, rows: 60 })).toBe(true);
  });

  it("shouldFit_should_returnTrue_When_proposedDiffersOnRowsOnly", () => {
    expect(shouldFit({ cols: 84, rows: 61 }, { cols: 84, rows: 60 })).toBe(true);
  });

  it("shouldFit_should_returnFalse_When_proposedColsIsUndefined", () => {
    expect(shouldFit({ cols: undefined, rows: 60 }, { cols: 84, rows: 60 })).toBe(false);
  });

  it("shouldFit_should_returnFalse_When_proposedRowsIsUndefined", () => {
    expect(shouldFit({ cols: 84, rows: undefined }, { cols: 84, rows: 60 })).toBe(false);
  });

  it("shouldFit_should_returnFalse_When_proposedIsUndefined", () => {
    expect(shouldFit(undefined, { cols: 84, rows: 60 })).toBe(false);
  });
});

describe("shouldSendResize", () => {
  it("shouldSendResize_should_returnTrue_When_lastSentIsNull", () => {
    expect(shouldSendResize({ cols: 100, rows: 30 }, null)).toBe(true);
  });

  it("shouldSendResize_should_returnFalse_When_lastSentEqualsNext", () => {
    const lastSent = { cols: 100, rows: 30 };
    expect(shouldSendResize({ cols: 100, rows: 30 }, lastSent)).toBe(false);
  });

  it("shouldSendResize_should_returnTrue_When_lastSentDiffersOnColsOnly", () => {
    const lastSent = { cols: 100, rows: 30 };
    expect(shouldSendResize({ cols: 110, rows: 30 }, lastSent)).toBe(true);
  });

  it("shouldSendResize_should_returnTrue_When_lastSentDiffersOnRowsOnly", () => {
    const lastSent = { cols: 100, rows: 30 };
    expect(shouldSendResize({ cols: 100, rows: 35 }, lastSent)).toBe(true);
  });

  it("shouldSendResize_should_returnTrue_When_bothDimensionsDiffer", () => {
    const lastSent = { cols: 100, rows: 30 };
    expect(shouldSendResize({ cols: 110, rows: 35 }, lastSent)).toBe(true);
  });
});

describe("shouldAbandonWebgl", () => {
  it("shouldAbandonWebgl_should_returnTrue_When_mostRecentValueRecursThreeTimesWithinWindow", () => {
    // Given history = [{84,60,at:1000}, {85,60,at:1400}, {84,60,at:1900}] and a new resize
    // {84,60,at:2300} appended before the check, three {84,60} entries (1000, 1900, 2300)
    // fall within the 2000ms window ending at 2300; the 85,60 entry at 1400 does not match
    // the most recent value and is excluded.
    const history: ResizeEvent[] = [
      { cols: 84, rows: 60, at: 1000 },
      { cols: 85, rows: 60, at: 1400 },
      { cols: 84, rows: 60, at: 1900 },
      { cols: 84, rows: 60, at: 2300 },
    ];
    expect(shouldAbandonWebgl(history, 2300)).toBe(true);
  });

  it("shouldAbandonWebgl_should_returnFalse_When_oldestMatchingEntryAgesOutOfWindow", () => {
    // Given the same three-entry history but the new entry arrives at at:3200 (2200ms after
    // the oldest {84,60} entry at 1000), the 1000ms entry has aged out of the 2000ms window,
    // leaving only 2 matching entries.
    const history: ResizeEvent[] = [
      { cols: 84, rows: 60, at: 1000 },
      { cols: 85, rows: 60, at: 1400 },
      { cols: 84, rows: 60, at: 1900 },
      { cols: 84, rows: 60, at: 3200 },
    ];
    expect(shouldAbandonWebgl(history, 3200)).toBe(false);
  });

  it("shouldAbandonWebgl_should_includeEntry_When_ageExactlyEqualsWindowMs", () => {
    // Boundary case: now - e.at === windowMs is inclusive (<=), documented and asserted here.
    const history: ResizeEvent[] = [
      { cols: 84, rows: 60, at: 0 },
      { cols: 84, rows: 60, at: 1000 },
      { cols: 84, rows: 60, at: 2000 },
    ];
    // now=2000, oldest entry at=0 -> age is exactly 2000ms === windowMs, must still count.
    expect(shouldAbandonWebgl(history, 2000)).toBe(true);
  });

  it("shouldAbandonWebgl_should_excludeEntry_When_ageExceedsWindowMsByOneMs", () => {
    const history: ResizeEvent[] = [
      { cols: 84, rows: 60, at: 0 },
      { cols: 84, rows: 60, at: 1000 },
      { cols: 84, rows: 60, at: 2001 },
    ];
    // now=2001, oldest entry at=0 -> age is 2001ms, exceeds windowMs=2000, ages out.
    // Only 2 matching entries remain (1000, 2001) — below threshold of 3.
    expect(shouldAbandonWebgl(history, 2001)).toBe(false);
  });

  it("shouldAbandonWebgl_should_countOnlyMostRecentValue_When_historyAlternatesBetweenTwoSizes", () => {
    // Alternating A/B/A/B/A sequence where only the most-recent value's count matters.
    const history: ResizeEvent[] = [
      { cols: 84, rows: 60, at: 0 }, // A
      { cols: 85, rows: 60, at: 200 }, // B
      { cols: 84, rows: 60, at: 400 }, // A
      { cols: 85, rows: 60, at: 600 }, // B
      { cols: 84, rows: 60, at: 800 }, // A (most recent)
    ];
    // Most recent value is A ({84,60}), which recurs 3 times (at 0, 400, 800) within window.
    expect(shouldAbandonWebgl(history, 800)).toBe(true);
  });

  it("shouldAbandonWebgl_should_returnFalse_When_mostRecentValueOnlyRecursTwice", () => {
    const history: ResizeEvent[] = [
      { cols: 84, rows: 60, at: 0 }, // A
      { cols: 85, rows: 60, at: 200 }, // B
      { cols: 85, rows: 60, at: 400 }, // B (most recent) — only recurs twice
    ];
    expect(shouldAbandonWebgl(history, 400)).toBe(false);
  });

  it("shouldAbandonWebgl_should_returnFalse_When_historyIsEmpty", () => {
    expect(shouldAbandonWebgl([], 1000)).toBe(false);
  });
});
