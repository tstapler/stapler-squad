import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const sectionTitle = style({
  margin: 0,
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const sectionDescription = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const optionRow = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const optionButton = style({
  position: "relative",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  padding: vars.space["3"],
  border: `2px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  background: vars.color.cardBackground,
  cursor: "pointer",
  textAlign: "left",
  transition: "border-color 0.15s ease, box-shadow 0.15s ease",
  selectors: {
    "&:hover": {
      borderColor: vars.color.borderHover,
      boxShadow: `0 0 0 1px ${vars.color.glowSecondary}`,
    },
    "&:focus-visible": {
      outline: `2px solid ${vars.color.primary}`,
      outlineOffset: "2px",
    },
  },
});

export const optionButtonActive = style({
  borderColor: vars.color.primary,
  boxShadow: `0 0 0 1px ${vars.color.glowPrimary}, inset 0 0 0 1px ${vars.color.primary}`,
});

export const optionLabel = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const optionDescription = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});
