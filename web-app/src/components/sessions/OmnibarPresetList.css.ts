import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const list = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
  maxHeight: "240px",
  overflowY: "auto",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
});

export const row = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  cursor: "pointer",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
  },
});

export const rowSelected = style([
  row,
  {
    background: vars.color.hoverBackground,
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "-2px",
  },
]);

export const rowLabel = style({
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const rowArgv = style({
  color: vars.color.textSecondary,
  flex: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  fontFamily: "monospace",
  fontSize: vars.fontSize.sm,
});

export const emptyState = style({
  padding: vars.space["3"],
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.sm,
});

export const loadingState = style({
  padding: vars.space["3"],
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});

export const errorState = style({
  padding: vars.space["3"],
  background: vars.color.errorBg,
  color: vars.color.error,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
});

export const programWarning = style({
  color: vars.color.warning,
  fontSize: vars.fontSize.sm,
  marginTop: vars.space["1"],
});
