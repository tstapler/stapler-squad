import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

// Table/th/tr/td conventions mirror EscapeEventTable.css.ts's existing table
// so the per-session breakdown table (Story 2.4) reads as the same visual
// language as the per-session event table it sits alongside.
export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  overflowX: "auto",
});

export const table = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

export const thead = style({
  position: "sticky",
  top: 0,
  zIndex: zIndex.tableHeader,
  backgroundColor: vars.color.cardBackground,
});

export const th = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  textAlign: "left",
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  whiteSpace: "nowrap",
});

// Sortable header button — a real <button> inside the <th> per ux.md §3
// ("Every sortable header is a <button> element inside a <th>"). Reset to
// look like plain header text, with visible hover/focus-visible cues so the
// interactive affordance is perceivable, not just clickable.
export const sortButton = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  width: "100%",
  border: "none",
  background: "transparent",
  padding: 0,
  margin: 0,
  font: "inherit",
  color: "inherit",
  textTransform: "inherit",
  letterSpacing: "inherit",
  textAlign: "left",
  cursor: "pointer",
  borderRadius: vars.radii.sm,
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
    },
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.inputFocusBorder}`,
    outlineOffset: "2px",
  },
});

export const sortIcon = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  flexShrink: 0,
});

export const tr = style({
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

// Outlier row (mangle rate > 2x fleet-wide, or the 5% floor — ux.md §3): the
// background tint is a supplementary cue only. WCAG 1.4.1 requires the tint
// never be the *sole* signal, so the row also needs a `.outlierIcon` glyph +
// visually-hidden text in the markup (Story 2.4's job) — this class alone is
// not sufficient for the acceptance criterion, only the color layer of it.
export const outlierRow = style({
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  backgroundColor: vars.color.warningBg,
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.warningBg,
      opacity: 0.9,
    },
  },
});

export const outlierIcon = style({
  color: vars.color.warningText,
  marginRight: vars.space["1"],
});

// No new visually-hidden utility here: `srOnly` in
// web-app/src/components/ui/LiveRegion.css.ts already covers the "hidden
// text" half of the outlier row's non-color cue (used elsewhere via
// SessionSummaryPanel.tsx, ReviewQueuePanel's `visuallyHidden`, etc.) — the
// component built on top of these styles should import that instead of a
// duplicate defined here.

export const td = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  verticalAlign: "middle",
});

export const emptyState = style({
  padding: vars.space["8"],
  textAlign: "center",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});

export const loadingState = style({
  padding: vars.space["4"],
  textAlign: "center",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});
