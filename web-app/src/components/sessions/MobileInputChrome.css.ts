import { style } from "@vanilla-extract/css";
import { vars } from "../../styles/theme-contract.css";

/**
 * Sticky-flex chrome stack (Story 1.4.3): a column inside TerminalOutput's `--viewport-height` shell.
 * Chrome children never shrink and never use position: fixed, so the soft keyboard cannot cover them;
 * the terminal slot takes whatever height is left and may reach zero only past the budget's floor.
 */
export const stack = style({
  display: "flex",
  flexDirection: "column",
  minHeight: 0,
  width: "100%",
  maxWidth: "100%",
  overflowX: "hidden",
});

export const stackItem = style({
  flexShrink: 0,
  boxSizing: "border-box",
  maxWidth: "100%",
});

export const terminalSlot = style({
  flex: "1 1 0",
  minHeight: 0,
});

/** 44x44 touch target with an 8px gap to its neighbours. */
export const touchButton = style({
  minHeight: 44,
  minWidth: 44,
  boxSizing: "border-box",
  margin: 4,
  padding: "0 12px",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 8,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  font: "inherit",
  cursor: "pointer",
  ":focus-visible": { outline: `3px solid ${vars.color.primary}`, outlineOffset: 2 },
});

/** Visually hidden but still read by screen readers (the live regions must stay rendered). */
export const visuallyHidden = style({
  position: "absolute",
  width: 1,
  height: 1,
  margin: -1,
  padding: 0,
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
  whiteSpace: "nowrap",
  border: 0,
});
