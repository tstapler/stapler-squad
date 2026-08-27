import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const column = style({
  display: "flex",
  flexDirection: "column",
  width: "320px",
  flexShrink: 0,
  background: vars.color.surfaceMuted,
  border: `1px solid ${vars.color.borderSubtle}`,
  borderRadius: vars.radii.md,
  maxHeight: "100%",
  minHeight: 0,
});

export const columnHeader = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["3"]} ${vars.space["3"]} ${vars.space["2"]}`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
});

export const columnTitle = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  flex: 1,
});

export const columnCount = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "22px",
  height: "20px",
  borderRadius: vars.radii.full,
  background: vars.color.cardBackground,
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.bold,
  padding: `0 ${vars.space["1"]}`,
});

export const columnCards = style({
  flex: 1,
  minHeight: 0,
  overflowY: "auto",
  padding: vars.space["2"],
});

// Applied while a dragged card hovers over this column's droppable area (dnd-kit's `isOver`) —
// a visible drop-target affordance distinct from the column's resting state.
export const columnDropOver = style({
  outline: `2px dashed ${vars.color.primary}`,
  outlineOffset: "-2px",
  borderRadius: vars.radii.sm,
});

// Applied to columnCards while a dragged card is hovering over this column as a valid drop
// target (dnd-kit's useDroppable isOver).
export const columnCardsOver = style({
  outline: `2px dashed ${vars.color.primary}`,
  outlineOffset: "-2px",
});

// Visual language borrowed from SessionListEmptyState.css.ts (muted text, small centered
// copy) rather than that component itself, which is sized for the whole-list empty state.
export const emptyColumn = style({
  margin: 0,
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  fontStyle: "italic",
  textAlign: "center",
  padding: `${vars.space["6"]} ${vars.space["2"]}`,
});
