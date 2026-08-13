import { style } from "@vanilla-extract/css";
import { vars } from "../../styles/theme-contract.css";

export const badgeContainer = style({
  display: "flex",
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  overflow: "hidden",
  flexShrink: 0,
});

export const badgeButton = style({
  padding: `${vars.space[1]} ${vars.space[2]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: 500,
  background: "transparent",
  color: vars.color.textMuted,
  cursor: "pointer",
  border: "none",
  lineHeight: 1.5,
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
  },
});

export const badgeActive = style({
  background: vars.color.primary,
  color: vars.color.primaryText,
  cursor: "default",
  selectors: {
    "&:hover": {
      background: vars.color.primary,
    },
  },
});
