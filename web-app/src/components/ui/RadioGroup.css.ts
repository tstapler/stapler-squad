import { style } from "@vanilla-extract/css";
import { vars } from "../../styles/theme-contract.css";

export const field = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const groupLabel = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const radioGroup = style({
  display: "flex",
  gap: vars.space["1"],
  flexWrap: "wrap",
});

export const radioBtn = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textSecondary,
  cursor: "pointer",
  transition: "all 0.1s ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderHover,
    },
    "&:focus-visible": {
      outline: `2px solid ${vars.color.primary}`,
      outlineOffset: "2px",
    },
  },
});

export const radioBtnActive = style({
  background: vars.color.primary,
  color: vars.color.primaryText,
  borderColor: vars.color.primary,
  selectors: {
    "&:hover": {
      background: vars.color.primaryHover,
      borderColor: vars.color.primaryHover,
    },
  },
});

export const hint = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});
