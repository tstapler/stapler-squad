import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const dialog = style({
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  padding: 0,
  maxWidth: "480px",
  width: "90vw",
  boxShadow: "0 20px 60px rgba(0,0,0,0.3)",

  selectors: {
    "&::backdrop": {
      background: "rgba(0,0,0,0.5)",
      backdropFilter: "blur(2px)",
    },
  },
});

export const form = style({
  padding: "24px",
  display: "flex",
  flexDirection: "column",
  gap: "16px",
});

export const title = style({
  fontSize: vars.fontSize.xl,
  fontWeight: vars.fontWeight.semibold,
  margin: 0,
  color: vars.color.textPrimary,
});

export const subtitle = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  margin: 0,
});

export const errorBanner = style({
  background: vars.color.warningBg,
  color: vars.color.warning,
  padding: "8px 12px",
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
});

export const field = style({
  display: "flex",
  flexDirection: "column",
  gap: "6px",
});

export const label = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const input = style({
  padding: "8px 12px",
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.inputBackground ?? vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.base,

  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
      boxShadow: `0 0 0 2px ${vars.color.glowPrimary}`,
    },
  },
});

export const radioGroup = style({
  border: "none",
  padding: 0,
  margin: 0,
  display: "flex",
  flexDirection: "column",
  gap: "8px",
});

export const radioLabel = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  cursor: "pointer",
});

export const slider = style({
  width: "100%",
  accentColor: vars.color.primary,
});

export const sliderLabels = style({
  display: "flex",
  justifyContent: "space-between",
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const actions = style({
  display: "flex",
  gap: "8px",
  justifyContent: "flex-end",
  paddingTop: "8px",
});
