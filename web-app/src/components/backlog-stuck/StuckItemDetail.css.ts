import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const detail = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderTop: "none",
  borderBottomLeftRadius: vars.radii.md,
  borderBottomRightRadius: vars.radii.md,
  padding: vars.space["4"],
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  fontSize: vars.fontSize.sm,
});

export const row = style({
  display: "flex",
  gap: vars.space["2"],
  alignItems: "baseline",
  flexWrap: "wrap",
});

export const label = style({
  color: vars.color.textMuted,
  fontWeight: 600,
  minWidth: "90px",
  flexShrink: 0,
});

export const value = style({
  color: vars.color.textSecondary,
});

export const actionCopy = style({
  color: vars.color.textPrimary,
  fontWeight: 500,
});

export const prLink = style({
  color: vars.color.primary,
  textDecoration: "none",
  ":hover": {
    textDecoration: "underline",
  },
});

export const unknownNote = style({
  color: vars.color.textMuted,
  fontStyle: "italic",
});
