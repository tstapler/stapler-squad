import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  gap: vars.space["3"],
  padding: vars.space["12"],
  color: vars.color.textMuted,
});

export const icon = style({
  fontSize: "32px",
  opacity: 0.4,
});

export const headline = style({
  margin: 0,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textSecondary,
});

export const body = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  textAlign: "center",
});

export const hint = style({
  margin: 0,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  opacity: 0.7,
});

export const kbd = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  padding: "1px 6px",
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
});
