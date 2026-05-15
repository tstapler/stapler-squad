import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const shimmerMove = keyframes({
  "0%": { backgroundPosition: "200% 0" },
  "100%": { backgroundPosition: "-200% 0" },
});

// Shimmer background — applied to individual placeholder bars
const shimmerBase = {
  background: `linear-gradient(90deg, ${vars.color.cardBackground} 25%, ${vars.color.hoverBackground} 50%, ${vars.color.cardBackground} 75%)`,
  backgroundSize: "200% 100%",
} as const;

export const skeletonRow = style({
  display: "grid",
  gridTemplateColumns: "8px 1fr auto 220px 32px auto",
  alignItems: "center",
  gap: vars.space["2"],
  padding: "0 12px",
  height: "38px",
});

export const dot = style({
  width: "8px",
  height: "8px",
  borderRadius: vars.radii.full,
  flexShrink: 0,
  background: vars.color.hoverBackground,
});

export const nameBar = style({
  height: "10px",
  borderRadius: vars.radii.full,
  width: "120px",
  ...shimmerBase,
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animationName: shimmerMove,
      animationDuration: "1.5s",
      animationIterationCount: "infinite",
      animationTimingFunction: "linear",
    },
  },
});

export const agentPlaceholder = style({
  width: "14px",
  height: "14px",
  borderRadius: vars.radii.sm,
  background: vars.color.hoverBackground,
  flexShrink: 0,
});

export const pathBar = style({
  height: "8px",
  borderRadius: vars.radii.full,
  width: "160px",
  opacity: 0.6,
  ...shimmerBase,
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animationName: shimmerMove,
      animationDuration: "1.5s",
      animationIterationCount: "infinite",
      animationTimingFunction: "linear",
    },
  },
});

export const timeBar = style({
  height: "8px",
  borderRadius: vars.radii.full,
  width: "28px",
  opacity: 0.5,
  ...shimmerBase,
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      animationName: shimmerMove,
      animationDuration: "1.5s",
      animationIterationCount: "infinite",
      animationTimingFunction: "linear",
    },
  },
});

export const actionsSpacer = style({
  width: "16px",
});
