import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const palette = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  maxHeight: "320px",
  overflowY: "auto",
  width: "100%",
});

export const list = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
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

export const rowName = style({
  fontWeight: 600,
  color: vars.color.textPrimary,
  minWidth: "80px",
});

export const rowDesc = style({
  color: vars.color.textSecondary,
  flex: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const rowMeta = style({
  display: "flex",
  gap: vars.space["2"],
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  marginLeft: "auto",
});

export const rowProgram = style({
  color: vars.color.primary,
});

export const groupHeader = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: 700,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  borderTop: `1px solid ${vars.color.borderColor}`,
  selectors: {
    "&:first-child": {
      borderTop: "none",
    },
  },
});

export const emptyState = style({
  padding: vars.space["4"],
  textAlign: "center",
  color: vars.color.textSecondary,
});

export const emptyTitle = style({
  fontWeight: 600,
  color: vars.color.textPrimary,
  marginBottom: vars.space["1"],
});

export const emptyBody = style({
  marginBottom: vars.space["3"],
  fontSize: vars.fontSize.sm,
});

export const emptyExample = style({
  background: vars.color.surfaceSubtle,
  borderRadius: vars.radii.sm,
  padding: vars.space["2"],
  textAlign: "left",
  fontSize: vars.fontSize.xs,
  marginBottom: vars.space["3"],
  overflow: "auto",
});

export const copyButton = style({
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.sm,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  cursor: "pointer",
  fontSize: vars.fontSize.sm,
});

export const errorState = style({
  display: "flex",
  gap: vars.space["3"],
  padding: vars.space["3"],
  background: vars.color.errorBg,
  borderRadius: vars.radii.md,
});

export const errorIcon = style({
  color: vars.color.error,
  fontSize: "1.2em",
});

export const errorTitle = style({
  fontWeight: 600,
  color: vars.color.error,
});

export const errorDetail = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  marginTop: vars.space["1"],
});
