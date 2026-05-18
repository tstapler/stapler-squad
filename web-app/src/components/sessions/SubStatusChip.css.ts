import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "../../styles/theme-contract.css";

// Spinning animation for the processing indicator
const spin = keyframes({
  "0%": { transform: "rotate(0deg)" },
  "100%": { transform: "rotate(360deg)" },
});

export const chip = style({
  display: "inline-flex",
  alignItems: "center",
  gap: "4px",
  padding: "2px 8px",
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  whiteSpace: "nowrap",
  lineHeight: 1.4,
});

export const chipNeedsApproval = style([
  chip,
  {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    border: `1px solid ${vars.color.warning}`,
  },
]);

export const chipProcessing = style([
  chip,
  {
    background: vars.color.accentBg,
    color: vars.color.primary,
    border: `1px solid ${vars.color.primary}`,
  },
]);

export const chipError = style([
  chip,
  {
    background: vars.color.errorBg,
    color: vars.color.errorText,
    border: `1px solid ${vars.color.error}`,
  },
]);

export const chipTestsFailing = style([
  chip,
  {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    border: `1px solid ${vars.color.warning}`,
  },
]);

export const chipRateLimited = style([
  chip,
  {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    border: `1px solid ${vars.color.warning}`,
  },
]);

export const spinner = style({
  display: "inline-block",
  width: "10px",
  height: "10px",
  border: `2px solid currentColor`,
  borderTopColor: "transparent",
  borderRadius: vars.radii.full,
  flexShrink: 0,
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animationName: spin,
      animationDuration: "0.8s",
      animationIterationCount: "infinite",
      animationTimingFunction: "linear",
    },
  },
});
