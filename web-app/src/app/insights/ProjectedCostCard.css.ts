// +feature: insights-dashboard
import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme-contract.css";

export const card = recipe({
  base: {
    background: vars.color.cardBackground,
    border: `1px solid ${vars.color.borderColor}`,
    borderRadius: vars.radii.lg,
    padding: `${vars.space[4]} ${vars.space[4]}`,
    display: "flex",
    flexDirection: "column",
    gap: vars.space[1],
  },
  variants: {
    warning: {
      true: {
        borderColor: vars.color.warning,
        background: vars.color.warningBg,
      },
      false: {},
    },
  },
  defaultVariants: { warning: false },
});

export const label = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
});

export const value = style({
  fontSize: vars.fontSize.xl,
  fontWeight: vars.fontWeight.bold,
  color: vars.color.textPrimary,
  lineHeight: 1.2,
});

export const sub = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

export const budgetInput = style({
  marginTop: vars.space[2],
  padding: `${vars.space[1]} ${vars.space[2]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.inputBorder}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  fontSize: vars.fontSize.xs,
  width: "120px",
  ":focus": {
    outline: "none",
    borderColor: vars.color.inputFocusBorder,
  },
});

export const warningText = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.warningText,
  fontWeight: vars.fontWeight.medium,
});

export const inputError = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.errorText,
});
