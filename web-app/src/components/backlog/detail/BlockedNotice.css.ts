import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const notice = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: vars.space["3"],
  borderRadius: vars.radii.md,
  background: vars.color.surfaceMuted,
  border: `1px solid ${vars.color.borderColor}`,
});

export const label = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  margin: 0,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const summaryText = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  whiteSpace: "pre-wrap",
});
