import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: vars.space["3"],
  borderRadius: vars.radii.md,
  background: vars.color.surfaceMuted,
  border: `1px solid ${vars.color.borderColor}`,
});

export const heading = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const list = style({
  margin: 0,
  paddingLeft: vars.space["4"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const status = style({
  marginLeft: vars.space["1"],
  opacity: 0.75,
});

export const depLink = style({
  background: "none",
  border: "none",
  padding: 0,
  font: "inherit",
  color: vars.color.primary,
  cursor: "pointer",
  textDecoration: "underline",
  textAlign: "left",
});
