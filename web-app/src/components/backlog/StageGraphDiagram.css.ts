import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const figure = style({
  margin: 0,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const scrollArea = style({
  overflowX: "auto",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  padding: vars.space["3"],
  background: vars.color.cardBackground,
});

export const nodeRect = style({
  fill: vars.color.cardBackground,
  stroke: vars.color.borderColor,
  strokeWidth: 1,
});

export const nodeText = style({
  fill: vars.color.textPrimary,
  fontSize: "11px",
});

export const edgeLine = style({
  stroke: vars.color.textMuted,
  strokeWidth: 1,
  fill: "none",
});

export const gateBadgeText = style({
  fill: vars.color.textMuted,
  fontSize: "10px",
});

export const caption = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const empty = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  fontStyle: "italic",
});
