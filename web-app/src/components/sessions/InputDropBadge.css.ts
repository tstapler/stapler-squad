import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme-contract.css";

// Task 4.2.1.2 — a drop is a warning-severity event, not a hard error, so
// this reuses the warning token trio rather than a bespoke color.
//
// MAJOR 4 fix: `top` is deliberately NOT the same `vars.space[2]` token used
// by TerminalOutput.css.ts's `reconnectingBanner`/`hardFailedBanner`. All
// three share the same horizontal centering (`left: 50%` +
// `translateX(-50%)`), and a drop is most likely to occur *during* a
// reconnect (drops are reported at the top of every connect() attempt) — so
// with identical `top` values this badge (much higher zIndex) would render
// directly on top of "Reconnecting terminal…" for its full 4s dwell, fully
// hiding it. Offset below the banner's expected height instead so both can
// be visible simultaneously, stacked rather than overlapping.
export const badge = style({
  position: "absolute",
  top: vars.space[12],
  left: "50%",
  transform: "translateX(-50%)",
  zIndex: zIndex.floatingTerminalUI,
  display: "inline-flex",
  alignItems: "center",
  gap: 6,
  padding: "4px 10px",
  borderRadius: vars.radii.lg,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  background: vars.color.warningBg,
  color: vars.color.warningText,
  border: `1px solid ${vars.color.warning}`,
  boxShadow: vars.shadow.md,
  pointerEvents: "none",
  whiteSpace: "nowrap",
});

export const icon = style({
  width: 14,
  height: 14,
  flexShrink: 0,
});

export const text = style({
  whiteSpace: "nowrap",
});
