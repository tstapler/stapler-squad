/** Pure chrome-stack budget (Story 1.4.3): decides how much phone chrome fits above the terminal. No DOM. */

export const MIN_TERMINAL_ROWS_FLOOR = 3;
export const MIN_TERMINAL_ROWS_COMFORT = 5;
/** Two wrapped message lines plus the 44px Reconnect/Dismiss row. */
export const CHIP_INLINE_PX = 56;
export const COMPOSE_FULL_PX = 96;
export const COMPOSE_ONE_LINE_PX = 44;

export type ChromeDensity = "comfortable" | "compact";
export type ChipPlacement = "hidden" | "inline" | "badge";
export type ComposePlacement = "closed" | "full" | "one-line" | "collapsed";

export interface ChromeBudgetInput {
  /** Visual viewport height; null when the keyboard height is unknown (floating/split keyboards). */
  viewportHeightPx: number | null;
  cellHeightPx: number;
  toolbarPx: number;
  composeOpen: boolean;
  chipVisible: boolean;
}

export interface ChromeBudget {
  density: ChromeDensity;
  chip: ChipPlacement;
  compose: ComposePlacement;
  toolbarPx: number;
  /** Rows left for the terminal after the chosen chrome; null when the viewport is unknown. */
  terminalRows: number | null;
  /** Hints (paste-flattened, IME fallback) wait instead of rendering when the compose bar had to collapse. */
  queueHints: boolean;
  /** True when even the collapsed layout leaves fewer than MIN_TERMINAL_ROWS_FLOOR rows (toolbar is never hidden). */
  floorViolated: boolean;
}

/** Comfortable keeps every surface in flow; compact sheds chrome in priority order: chip badge, one-line compose, collapsed compose. */
export function chromeBudget(input: ChromeBudgetInput): ChromeBudget {
  const { viewportHeightPx, cellHeightPx, toolbarPx, composeOpen, chipVisible } = input;
  const fullChip: ChipPlacement = chipVisible ? "inline" : "hidden";
  const fullCompose: ComposePlacement = composeOpen ? "full" : "closed";
  if (viewportHeightPx === null || cellHeightPx <= 0) {
    return { density: "comfortable", chip: fullChip, compose: fullCompose, toolbarPx, terminalRows: null, queueHints: false, floorViolated: false };
  }
  const rows = (chipPx: number, composePx: number) =>
    Math.max(0, Math.floor((viewportHeightPx - toolbarPx - chipPx - composePx) / cellHeightPx));
  const chipPx = (p: ChipPlacement) => (p === "inline" ? CHIP_INLINE_PX : 0);
  const composePx = (p: ComposePlacement) => (p === "full" ? COMPOSE_FULL_PX : p === "one-line" ? COMPOSE_ONE_LINE_PX : 0);

  const fullRows = rows(chipPx(fullChip), composePx(fullCompose));
  if (fullRows >= MIN_TERMINAL_ROWS_COMFORT) {
    return { density: "comfortable", chip: fullChip, compose: fullCompose, toolbarPx, terminalRows: fullRows, queueHints: false, floorViolated: false };
  }

  const chip: ChipPlacement = chipVisible ? "badge" : "hidden";
  let compose: ComposePlacement = fullCompose;
  if (rows(0, composePx(compose)) < MIN_TERMINAL_ROWS_COMFORT && compose === "full") compose = "one-line";
  let queueHints = false;
  if (rows(0, composePx(compose)) < MIN_TERMINAL_ROWS_FLOOR && compose !== "closed") {
    compose = "collapsed";
    queueHints = true;
  }
  const terminalRows = rows(0, composePx(compose));
  return { density: "compact", chip, compose, toolbarPx, terminalRows, queueHints, floorViolated: terminalRows < MIN_TERMINAL_ROWS_FLOOR };
}
