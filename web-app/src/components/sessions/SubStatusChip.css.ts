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
  flexShrink: 0,
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

// Softer than chipNeedsApproval — informational warning, not user-blocking.
// Same amber bg but border matches background and weight is normal to reduce urgency.
export const chipTestsFailing = style([
  chip,
  {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    border: `1px solid ${vars.color.warningBg}`,
    fontWeight: vars.fontWeight.normal,
  },
]);

// Neutral/transient — not urgent, just an API constraint.
export const chipRateLimited = style([
  chip,
  {
    background: vars.color.surfaceSubtle,
    color: vars.color.textSecondary,
    border: `1px solid ${vars.color.borderColor}`,
  },
]);

export const chipIdle = style([
  chip,
  {
    background: vars.statusBadge.idleBg,
    color: vars.statusBadge.idleFg,
    border: `1px solid ${vars.statusBadge.idleBorder}`,
    opacity: 0.8,
  },
]);

export const chipInputRequired = style([
  chip,
  {
    background: vars.statusBadge.inputBg,
    color: vars.statusBadge.inputFg,
    border: `1px solid ${vars.statusBadge.inputBorder}`,
  },
]);

export const chipReady = style([
  chip,
  {
    background: vars.color.successBg,
    color: vars.color.success,
    border: `1px solid ${vars.color.success}`,
    opacity: 0.85,
  },
]);

export const chipSuccess = style([
  chip,
  {
    background: vars.statusBadge.completeBg,
    color: vars.statusBadge.completeFg,
    border: `1px solid ${vars.statusBadge.completeBorder}`,
  },
]);

// Neutral/transient — agent is doing work autonomously; no user action needed.
export const chipWaitingForAgent = style([
  chip,
  {
    background: vars.color.accentBg,
    color: vars.color.primary,
    border: `1px solid ${vars.color.primary}`,
    fontWeight: vars.fontWeight.normal,
    opacity: 0.85,
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
    // For reduced-motion users show a static filled dot rather than a partial arc.
    "(prefers-reduced-motion: reduce)": {
      border: "none",
      background: "currentColor",
      opacity: 0.6,
    },
  },
});
