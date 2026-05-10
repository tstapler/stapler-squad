import { style, globalStyle, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  width: "100%",
  height: "100%",
  display: "flex",
  flexDirection: "column",
  background: vars.color.cardBackground,
  borderRadius: "4px",
  overflow: "hidden",
  boxSizing: "border-box",
});

export const terminal = style({
  flex: 1,
  width: "100%",
  height: "100%",
  minHeight: 0,
  overflow: "hidden",
  position: "relative",
  boxSizing: "content-box",
  padding: 0,
  margin: 0,
});

// Global styles for xterm.js elements within the terminal container
globalStyle(`${terminal} .xterm`, {
  height: "100% !important",
  width: "100% !important",
  padding: "0 !important",
  margin: "0 !important",
  boxSizing: "content-box !important" as "content-box",
});

globalStyle(`${terminal} .xterm-screen`, {
  height: "100% !important",
  width: "100% !important",
  boxSizing: "content-box !important" as "content-box",
  padding: "0 !important",
  margin: "0 !important",
});

globalStyle(`${terminal} .xterm-rows`, {
  boxSizing: "content-box !important" as "content-box",
});

globalStyle(`${terminal} .xterm-viewport`, {
  overflowY: "hidden",
  scrollbarWidth: "thin",
  scrollbarColor: "rgba(255, 255, 255, 0.2) transparent",
});

globalStyle(`${terminal} .xterm-viewport::-webkit-scrollbar`, {
  width: "8px",
});

globalStyle(`${terminal} .xterm-viewport::-webkit-scrollbar-track`, {
  background: "transparent",
});

globalStyle(`${terminal} .xterm-viewport::-webkit-scrollbar-thumb`, {
  backgroundColor: "rgba(255, 255, 255, 0.2)",
  borderRadius: "4px",
});

globalStyle(`${terminal} .xterm-viewport::-webkit-scrollbar-thumb:hover`, {
  backgroundColor: "rgba(255, 255, 255, 0.3)",
});

globalStyle(`${terminal} .xterm-selection`, {
  backgroundColor: "rgba(255, 255, 255, 0.3)",
});

globalStyle(`${terminal} .xterm:focus`, {
  outline: "2px solid rgba(33, 150, 243, 0.5)",
  outlineOffset: "-2px",
});

// ---- Floating Copy button (Task 3.2.2 / R3.2) ----
// Appears above the selection end point when the user makes a text selection.
// position: fixed is set via inline style since the coordinates are dynamic.
export const floatingCopyButton = style({
  position: "fixed",
  zIndex: 9999,
  padding: `${vars.space[1]} ${vars.space[3]}`,
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  cursor: "pointer",
  boxShadow: "0 2px 8px rgba(0,0,0,0.4)",
  touchAction: "manipulation",
  userSelect: "none",
  WebkitUserSelect: "none",
  selectors: {
    "&:active": {
      opacity: 0.85,
      transform: "scale(0.97)",
    },
  },
});

const fadeInOut = keyframes({
  "0%": { opacity: 0, transform: "translateY(4px)" },
  "15%": { opacity: 1, transform: "translateY(0)" },
  "85%": { opacity: 1, transform: "translateY(0)" },
  "100%": { opacity: 0, transform: "translateY(-4px)" },
});

// Brief "Copied" toast shown after clipboard write succeeds
export const copiedToast = style({
  position: "fixed",
  bottom: "80px",
  left: "50%",
  transform: "translateX(-50%)",
  zIndex: 9999,
  padding: `${vars.space[1]} ${vars.space[3]}`,
  background: vars.color.success,
  color: vars.color.textPrimary,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  boxShadow: "0 2px 8px rgba(0,0,0,0.3)",
  pointerEvents: "none",
  animation: `${fadeInOut} 1.5s ease-in-out forwards`,
});
