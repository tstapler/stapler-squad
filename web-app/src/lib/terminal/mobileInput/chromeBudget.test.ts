import { readFileSync } from "fs";
import { join } from "path";
import {
  CHIP_INLINE_PX,
  COMPOSE_FULL_PX,
  COMPOSE_ONE_LINE_PX,
  MIN_TERMINAL_ROWS_COMFORT,
  MIN_TERMINAL_ROWS_FLOOR,
  chromeBudget,
} from "./chromeBudget";

const base = { cellHeightPx: 17, toolbarPx: 44, composeOpen: true, chipVisible: true };

describe("chromeBudget", () => {
  it("constants_should_MatchPlan", () => {
    expect(MIN_TERMINAL_ROWS_FLOOR).toBe(3);
    expect(MIN_TERMINAL_ROWS_COMFORT).toBe(5);
  });

  it("chromeBudget_should_ReturnComfortableWithFullChrome_When_PortraitKeyboardViewport", () => {
    const b = chromeBudget({ ...base, viewportHeightPx: 340 });
    expect(b.density).toBe("comfortable");
    expect(b.terminalRows).toBeGreaterThanOrEqual(5);
    expect(b.chip).toBe("inline");
    expect(b.compose).toBe("full");
    expect(b.queueHints).toBe(false);
  });

  it("chromeBudget_should_ReturnCompactAndBadgeChip_When_LandscapeKeyboardViewport", () => {
    const b = chromeBudget({ ...base, viewportHeightPx: 170 });
    expect(b.density).toBe("compact");
    expect(b.chip).toBe("badge");
    expect(b.terminalRows).toBeGreaterThanOrEqual(3);
    expect(b.compose).toBe("one-line");
    expect(b.queueHints).toBe(false);
  });

  it("chromeBudget_should_CollapseComposeAndQueueHints_When_FloorStillViolatedAfterOneLine", () => {
    const b = chromeBudget({ ...base, viewportHeightPx: 44 + COMPOSE_ONE_LINE_PX + 3 * 17 - 1 });
    expect(b.density).toBe("compact");
    expect(b.chip).toBe("badge");
    expect(b.compose).toBe("collapsed");
    expect(b.queueHints).toBe(true);
    expect(b.terminalRows).toBeGreaterThanOrEqual(3);
  });

  it("chromeBudget_should_ApplyRulesInPriority_When_ShrinkingOneStepAtATime", () => {
    const rowsAt = (rows: number, extra: number) => 44 + extra + rows * 17;
    // chip fits inline with 5 rows
    expect(chromeBudget({ ...base, viewportHeightPx: rowsAt(5, COMPOSE_FULL_PX + CHIP_INLINE_PX) }).density).toBe("comfortable");
    // one px short of 5 rows: chip badge first, compose still full
    const step1 = chromeBudget({ ...base, viewportHeightPx: rowsAt(5, COMPOSE_FULL_PX + CHIP_INLINE_PX) - 1 });
    expect(step1).toMatchObject({ density: "compact", chip: "badge", compose: "full" });
    // short of 3 rows with a full compose bar: compose goes to one line next
    const step2 = chromeBudget({ ...base, viewportHeightPx: rowsAt(3, COMPOSE_FULL_PX) - 1 });
    expect(step2).toMatchObject({ chip: "badge", compose: "one-line" });
  });

  it("chromeBudget_should_NeverHideToolbarAndReportFloorViolation_When_ViewportAbsurdlyShort", () => {
    const b = chromeBudget({ ...base, viewportHeightPx: 60 });
    expect(b.toolbarPx).toBe(44);
    expect(b.floorViolated).toBe(true);
    expect(b.compose).toBe("collapsed");
  });

  it("chromeBudget_should_BeComfortable_When_KeyboardHeightUnknown", () => {
    const b = chromeBudget({ ...base, viewportHeightPx: null });
    expect(b).toMatchObject({ density: "comfortable", terminalRows: null, chip: "inline", compose: "full", queueHints: false });
  });

  it("chromeBudget_should_SkipClosedSurfaces_When_ComposeClosedAndChipHidden", () => {
    const b = chromeBudget({ ...base, composeOpen: false, chipVisible: false, viewportHeightPx: 44 + 5 * 17 });
    expect(b).toMatchObject({ density: "comfortable", compose: "closed", chip: "hidden" });
  });

  it("chromeBudget_should_ReturnRowsMatchingFinalChrome_When_360x340And320x300", () => {
    for (const h of [340, 300]) {
      const b = chromeBudget({ ...base, viewportHeightPx: h });
      expect(b.terminalRows).toBeGreaterThanOrEqual(5);
    }
    const short = chromeBudget({ ...base, viewportHeightPx: 240 });
    expect(short.terminalRows).toBeGreaterThanOrEqual(3);
  });
});

describe("chrome stack css contract", () => {
  const css = readFileSync(join(__dirname, "../../../components/sessions/MobileInputChrome.css.ts"), "utf8");

  it("chromeStack_should_NeverUsePositionFixed_When_CssScanned", () => {
    expect(css).not.toMatch(/position:\s*["']fixed["']/);
  });

  it("chromeStack_should_KeepChromeChildrenNonShrinkingAndTerminalFlexible_When_CssScanned", () => {
    expect(css).toMatch(/export const stackItem[\s\S]*?flexShrink:\s*0/);
    expect(css).toMatch(/export const terminalSlot[\s\S]*?flex:\s*"1 1 0"[\s\S]*?minHeight:\s*0/);
  });

  it("chromeStack_should_Use44pxTouchTargets_When_CssScanned", () => {
    expect(css).toMatch(/export const touchButton[\s\S]*?minHeight:\s*44/);
    expect(css).toMatch(/export const touchButton[\s\S]*?minWidth:\s*44/);
  });
});
