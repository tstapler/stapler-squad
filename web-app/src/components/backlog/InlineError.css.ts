import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const pillContainer = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `2px ${vars.space["3"]}`,
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.full,
  background: vars.color.errorBg,
  color: vars.color.errorText,
  fontSize: vars.fontSize.sm,
});

export const blockContainer = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.md,
  padding: vars.space["3"],
  background: vars.color.errorBg,
  color: vars.color.errorText,
  fontSize: vars.fontSize.sm,
});

export const icon = style({
  color: "inherit",
  flexShrink: 0,
});

export const headline = style({
  fontWeight: vars.fontWeight.semibold,
});

export const body = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.errorText,
});

export const actions = style({
  display: "flex",
  gap: vars.space["2"],
  flexWrap: "wrap",
  marginTop: vars.space["1"],
});

export const actionButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.primary,
  fontSize: vars.fontSize.sm,
  padding: 0,
  textDecoration: "underline",
  ":hover": {
    color: vars.color.primaryHover,
  },
});

export const dismissButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  padding: "2px",
  marginLeft: "auto",
  lineHeight: 1,
  minWidth: 24,
  minHeight: 24,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
});
