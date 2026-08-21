import { keyframes, style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const boardWrapper = style({
  display: "flex",
  flexDirection: "column",
  flex: 1,
  minHeight: 0,
});

export const boardToolbar = style({
  display: "flex",
  justifyContent: "flex-end",
  padding: `${vars.space["2"]} ${vars.space["4"]} 0`,
});

export const board = style({
  display: "flex",
  gap: vars.space["4"],
  overflowX: "auto",
  padding: `0 ${vars.space["4"]} ${vars.space["4"]}`,
  minHeight: "400px",
  flex: 1,
  alignItems: "flex-start",
});

export const column = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  minWidth: "240px",
  maxWidth: "300px",
  width: "280px",
  flexShrink: 0,
});

export const columnHeader = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} 0`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
});

export const columnTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  fontFamily: vars.font.mono,
  flex: 1,
});

export const columnCount = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "20px",
  height: "20px",
  borderRadius: vars.radii.full,
  background: vars.color.surfaceMuted,
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.bold,
  fontFamily: vars.font.mono,
  padding: `0 ${vars.space["1"]}`,
});

export const columnCards = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  flex: 1,
});

// Epic 6.4 (backlog-event-driven-updates): brief fade+slight-scale exit for a
// card whose item just left this column via a genuine live status change
// (ux.md §7 — "reads as moved," paired with the flash on entry into the
// destination column via BacklogItemCard's `forceJustChanged`). Applied to
// the `role="listitem"` wrapper around BacklogItemCard, mirroring
// backlog.css.ts's `tableRowExiting` for the list view.
const cardExitKeyframes = keyframes({
  "0%": { opacity: 1, transform: "scale(1)" },
  "100%": { opacity: 0, transform: "scale(0.96)" },
});

export const cardExiting = style({
  pointerEvents: "none",
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animationName: cardExitKeyframes,
      animationDuration: "200ms",
      animationTimingFunction: "ease-out",
      animationFillMode: "forwards",
    },
    // Reduced motion: removal is already instant (driven by JS, see
    // BacklogBoard.tsx), so no transition/animation is applied here — just
    // avoid a stray opaque flash of the card before it's removed from the
    // DOM.
    "(prefers-reduced-motion: reduce)": {
      opacity: 1,
    },
  },
});

export const emptyColumn = style({
  color: vars.color.textDisabled,
  fontSize: vars.fontSize.sm,
  fontStyle: "italic",
  textAlign: "center",
  padding: `${vars.space["6"]} ${vars.space["2"]}`,
});

// Loading skeleton
export const skeletonCard = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: vars.space["3"],
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  opacity: 0.6,
});

export const skeletonLine = style({
  height: "12px",
  borderRadius: vars.radii.sm,
  background: vars.color.surfaceMuted,
  animation: "pulse 1.5s ease-in-out infinite",
});

export const skeletonLineShort = style({
  width: "60%",
});
