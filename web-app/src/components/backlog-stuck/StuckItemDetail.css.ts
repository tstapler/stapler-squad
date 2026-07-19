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

export const overrideForm = style({
  display: "flex",
  gap: vars.space["2"],
  alignItems: "center",
  flexWrap: "wrap",
});

export const overrideInput = style({
  width: "72px",
  minHeight: "44px",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
});

export const overrideButton = style({
  minHeight: "44px",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.primary}`,
  background: vars.color.primary,
  color: vars.color.textInverse,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  cursor: "pointer",
  ":disabled": {
    opacity: 0.6,
    cursor: "not-allowed",
  },
});

export const overrideUnlimitedButton = style({
  minHeight: "44px",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  ":disabled": {
    opacity: 0.6,
    cursor: "not-allowed",
  },
});

export const overrideStatus = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
});
