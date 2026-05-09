import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const page = style({
  display: "flex",
  flexDirection: "column",
  height: "calc(var(--viewport-height, 100dvh) - var(--header-height))",
  overflow: "hidden",
  background: vars.color.background,
});

export const main = style({
  flex: 1,
  display: "flex",
  flexDirection: "column",
  maxWidth: "1400px",
  width: "100%",
  margin: "0 auto",
  padding: "2rem",
  gap: "1rem",
  overflow: "hidden",
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "1rem",
    },
  },
});

export const modal = style({
  position: "fixed",
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  background: "rgba(0, 0, 0, 0.8)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  padding: "2rem",
  paddingTop: "var(--header-height)",
  zIndex: 1000,
  backdropFilter: "blur(4px)",
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0",
      paddingTop: "var(--header-height)",
    },
  },
});

export const modalContent = style({
  maxWidth: "90vw",
  width: "90vw",
  maxHeight: "calc(100dvh - var(--header-height) - 4rem)",
  height: "calc(100dvh - var(--header-height) - 4rem)",
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
  background: vars.color.cardBackground,
  borderRadius: "0.5rem",
  border: `1px solid ${vars.color.borderColor}`,
  "@media": {
    "screen and (max-width: 768px)": {
      maxWidth: "100vw",
      width: "100vw",
      maxHeight: "calc(var(--viewport-height, 100dvh) - var(--header-height))",
      height: "calc(var(--viewport-height, 100dvh) - var(--header-height))",
      borderRadius: "0",
    },
  },
});

export const modalContentFullscreen = style({
  maxWidth: "98vw",
  width: "98vw",
  maxHeight: "calc(100dvh - var(--header-height))",
  height: "calc(100dvh - var(--header-height))",
  borderRadius: "0",
  "@media": {
    "screen and (max-width: 768px)": {
      maxWidth: "100vw",
      width: "100vw",
    },
  },
});

const shimmer = keyframes({
  "0%": { backgroundPosition: "200% 0" },
  "100%": { backgroundPosition: "-200% 0" },
});

export const skeletonHeader = style({
  height: "2rem",
  width: "40%",
  borderRadius: "6px",
  background: `linear-gradient(90deg, ${vars.color.cardBackground} 25%, ${vars.color.hoverBackground} 50%, ${vars.color.cardBackground} 75%)`,
  backgroundSize: "200% 100%",
  animationName: shimmer,
  animationDuration: "1.5s",
  animationIterationCount: "infinite",
  "@media": {
    "(prefers-reduced-motion: reduce)": {
      animationName: "none",
      background: vars.color.cardBackground,
    },
  },
});

export const skeletonList = style({
  display: "flex",
  flexDirection: "column",
  gap: "12px",
  marginTop: "1rem",
});

export const skeletonCard = style({
  height: "120px",
  borderRadius: "8px",
  background: `linear-gradient(90deg, ${vars.color.cardBackground} 25%, ${vars.color.hoverBackground} 50%, ${vars.color.cardBackground} 75%)`,
  backgroundSize: "200% 100%",
  animationName: shimmer,
  animationDuration: "1.5s",
  animationIterationCount: "infinite",
  border: `1px solid ${vars.color.borderColor}`,
  "@media": {
    "(prefers-reduced-motion: reduce)": {
      animationName: "none",
      background: vars.color.cardBackground,
    },
  },
});

export const toolbar = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.5rem 0",
  fontSize: "0.8125rem",
  color: vars.color.textSecondary,
});

export const autoAdvanceLabel = style({
  display: "flex",
  alignItems: "center",
  gap: "0.375rem",
  cursor: "pointer",
  userSelect: "none",
});

export const helpButton = style({
  position: "fixed",
  bottom: "1.5rem",
  right: "1.5rem",
  width: "2.25rem",
  height: "2.25rem",
  borderRadius: "50%",
  background: vars.color.primary,
  color: "white",
  border: "none",
  fontSize: "1rem",
  fontWeight: 600,
  cursor: "pointer",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: 900,
  opacity: 0.8,
  transition: "opacity 0.15s",
  selectors: {
    "&:hover": {
      opacity: 1,
    },
  },
});

export const helpOverlay = style({
  position: "fixed",
  inset: 0,
  background: "rgba(0, 0, 0, 0.7)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: 2000,
  padding: "2rem",
});

export const helpOverlayContent = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "0.5rem",
  padding: "1.5rem",
  minWidth: "22rem",
  maxWidth: "32rem",
  width: "100%",
});

export const helpOverlayHeader = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: "1.25rem",
});

export const helpOverlayCloseButton = style({
  background: "transparent",
  border: "none",
  color: vars.color.textSecondary,
  fontSize: "1.1rem",
  cursor: "pointer",
  padding: "0.25rem",
  lineHeight: "1",
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
    },
  },
});
