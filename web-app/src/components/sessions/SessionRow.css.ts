import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

// Pulse animation for running status dot — only active when reduced motion is not requested
const pulseOpacity = keyframes({
  "0%": { opacity: 1 },
  "50%": { opacity: 0.4 },
  "100%": { opacity: 1 },
});

export const row = style({
  display: "grid",
  // name (1fr) always wins — path shrinks to 0 before name does
  gridTemplateColumns: "8px 1fr auto minmax(0, 220px) 32px auto",
  alignItems: "center",
  gap: vars.space["2"],
  padding: "0 12px",
  height: "38px",
  cursor: "pointer",
  borderRadius: vars.radii.sm,
  listStyle: "none",
  position: "relative",
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      transition: vars.transition.fast,
    },
  },
  ":hover": {
    background: vars.color.hoverBackground,
  },
});

export const statusDot = style({
  width: "8px",
  height: "8px",
  borderRadius: vars.radii.full,
  flexShrink: 0,
  selectors: {
    '&[data-status="running"]': {
      background: vars.color.statusDot.running,
    },
    '&[data-status="paused"]': {
      background: vars.color.statusDot.paused,
    },
    '&[data-status="idle"]': {
      background: vars.color.statusDot.idle,
    },
    '&[data-status="loading"]': {
      background: vars.color.statusDot.idle,
    },
    '&[data-status="needs-approval"]': {
      background: vars.color.statusDot.paused,
    },
  },
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      selectors: {
        '&[data-status="running"]': {
          animationName: pulseOpacity,
          animationDuration: "2s",
          animationIterationCount: "infinite",
          animationTimingFunction: "ease-in-out",
        },
      },
    },
  },
});

export const name = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const agentIcon = style({
  fontSize: vars.fontSize.sm,
  flexShrink: 0,
  display: "flex",
  alignItems: "center",
});

export const path = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  minWidth: 0,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  direction: "ltr",
  // Below 480px the path column collapses to nothing — tooltip still works
  "@media": {
    "(max-width: 480px)": {
      display: "none",
    },
  },
});

export const elapsed = style({
  fontSize: "11px",
  color: vars.color.textMuted,
  fontVariantNumeric: "tabular-nums",
  minWidth: "32px",
  textAlign: "right",
});

export const actions = style({
  display: "flex",
  gap: vars.space["1"],
  opacity: 0,
  "@media": {
    "(prefers-reduced-motion: no-preference)": {
      transition: vars.transition.fast,
    },
  },
  selectors: {
    [`${row}:hover &`]: {
      opacity: 1,
    },
  },
});

export const actionButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.textMuted,
  padding: "2px 4px",
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  lineHeight: 1,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  ":hover": {
    color: vars.color.textPrimary,
    background: vars.color.hoverBackground,
  },
});

export const groupHeader = style({
  height: "24px",
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  paddingLeft: "8px",
  paddingTop: "8px",
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  listStyle: "none",
});
