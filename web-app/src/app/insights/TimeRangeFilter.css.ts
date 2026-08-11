// +feature: insights-dashboard
import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme-contract.css";

export const filterBar = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  flexWrap: "wrap",
});

export const presetGroup = style({
  display: "flex",
  gap: vars.space[1],
  flexWrap: "wrap",
});

export const presetButton = recipe({
  base: {
    padding: `${vars.space[1]} ${vars.space[3]}`,
    borderRadius: vars.radii.md,
    border: `1px solid ${vars.color.borderColor}`,
    background: "transparent",
    color: vars.color.textSecondary,
    fontSize: vars.fontSize.sm,
    cursor: "pointer",
    transition: vars.transition.fast,
    ":hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
  variants: {
    active: {
      true: {
        background: vars.color.primary,
        color: vars.color.primaryText,
        borderColor: vars.color.primary,
        ":hover": {
          background: vars.color.primaryHover,
          color: vars.color.primaryText,
        },
      },
      false: {},
    },
  },
  defaultVariants: { active: false },
});

export const customRange = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  flexWrap: "wrap",
});

export const dateInput = style({
  padding: `${vars.space[1]} ${vars.space[2]}`,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.inputBorder}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  ":focus": {
    outline: "none",
    borderColor: vars.color.inputFocusBorder,
  },
});

export const rangeError = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.errorText,
  width: "100%",
});
