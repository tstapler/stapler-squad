import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  height: "100%",
  padding: "16px",
  overflowY: "auto",
  background: vars.color.background,
  color: vars.color.textPrimary,
});

const pulse = keyframes({
  "0%, 100%": { opacity: 1 },
  "50%": { opacity: 0.5 },
});

export const skeleton = style({
  display: "flex",
  flexDirection: "column",
  gap: "10px",
  padding: "12px 0",
});

export const skeletonBar = style({
  height: "14px",
  borderRadius: vars.radii.sm,
  background: vars.color.hoverBackground,
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animationName: pulse,
      animationDuration: "1.5s",
      animationTimingFunction: "ease-in-out",
      animationIterationCount: "infinite",
    },
  },
});

export const error = style({
  display: "flex",
  alignItems: "center",
  gap: "12px",
  padding: "16px",
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: "8px",
  color: vars.color.error,
});

export const errorIcon = style({
  fontSize: "20px",
});

export const retryButton = style({
  marginLeft: "auto",
  padding: "6px 12px",
  background: "transparent",
  border: "1px solid currentColor",
  borderRadius: "4px",
  color: "inherit",
  cursor: "pointer",
  fontSize: "12px",
  selectors: {
    "&:hover": {
      background: vars.color.errorBg,
    },
  },
});

export const empty = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  color: vars.color.textSecondary,
  textAlign: "center",
});
