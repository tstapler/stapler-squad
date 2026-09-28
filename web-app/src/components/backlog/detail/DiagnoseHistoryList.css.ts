import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const sectionTitle = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
});

export const list = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  margin: 0,
  padding: 0,
  listStyle: "none",
});

export const item = style({
  display: "flex",
  alignItems: "center",
  flexWrap: "wrap",
  gap: vars.space["2"],
  padding: vars.space["2"],
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

export const timestamp = style({
  marginLeft: "auto",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
});

export const link = style({
  color: vars.color.primary,
  textDecoration: "none",
  ":hover": {
    textDecoration: "underline",
  },
});

export const emptyText = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
});

export const errorText = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.errorText,
});

export const retryButton = style({
  alignSelf: "flex-start",
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.error}`,
  background: vars.color.background,
  color: vars.color.errorText,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  cursor: "pointer",
});
