import { style } from "@vanilla-extract/css";
import { vars } from "../../styles/theme.css";

export const tooltipContent = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  padding: "4px 8px",
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  boxShadow: "0 4px 12px rgba(0,0,0,0.3)",
  zIndex: 9999,
  userSelect: "none",
  animationDuration: "400ms",
  animationTimingFunction: "cubic-bezier(0.16, 1, 0.3, 1)",
  willChange: "transform, opacity",
  selectors: {
    '&[data-state="delayed-open"][data-side="top"]': {
      animationName: "slideDownAndFade",
    },
    '&[data-state="delayed-open"][data-side="right"]': {
      animationName: "slideLeftAndFade",
    },
    '&[data-state="delayed-open"][data-side="bottom"]': {
      animationName: "slideUpAndFade",
    },
    '&[data-state="delayed-open"][data-side="left"]': {
      animationName: "slideRightAndFade",
    },
  },
});

export const tooltipArrow = style({
  fill: vars.color.cardBackground,
});
