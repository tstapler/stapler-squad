import { style, globalStyle, keyframes } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

export const container = style({
  width: "100%",
  height: "100%",
  display: "flex",
  flexDirection: "column",
  position: "relative",
  background: vars.color.cardBackground,
  borderRadius: "4px",
  overflow: "hidden",
  boxSizing: "border-box",
});

// "Copy all" button — always visible in the top-right corner of the terminal.
// Semi-transparent so it doesn't distract from the content, but easy to tap on mobile.
export const scrollbackCopyButton = style({
  position: "absolute",
  top: vars.space["2"],
  right: vars.space["2"],
  zIndex: zIndex.floatingTerminalUI,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  background: "rgba(0, 0, 0, 0.45)",
  color: "rgba(255, 255, 255, 0.75)",
  border: "1px solid rgba(255, 255, 255, 0.15)",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  touchAction: "manipulation",
  userSelect: "none",
  WebkitUserSelect: "none",
  lineHeight: 1,
  selectors: {
    "&:hover": {
      background: "rgba(0, 0, 0, 0.65)",
      color: "rgba(255, 255, 255, 0.95)",
      borderColor: "rgba(255, 255, 255, 0.3)",
    },
    "&:active": {
      opacity: 0.8,
    },
  },
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
  // Prevent the browser from claiming touch events as window scroll.
  // All touch handling is delegated to useTerminalGestures so the terminal
  // scroll never leaks into the page scroll.
  touchAction: "none",
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
// left/top are set dynamically via inline style; display toggled via ref.
export const floatingCopyButton = style({
  position: "fixed",
  zIndex: zIndex.floatingTerminalUI,
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
  "0%": { opacity: 0, transform: "translateX(-50%) translateY(4px)" },
  "15%": { opacity: 1, transform: "translateX(-50%) translateY(0)" },
  "85%": { opacity: 1, transform: "translateX(-50%) translateY(0)" },
  "100%": { opacity: 0, transform: "translateX(-50%) translateY(-4px)" },
});

// Base layout for the "Copied" toast — no animation here.
// The animation class (copiedToastVisible) is added/removed separately to allow restarting.
export const copiedToast = style({
  position: "fixed",
  bottom: "80px",
  left: "50%",
  transform: "translateX(-50%)",
  zIndex: zIndex.floatingTerminalUI,
  padding: `${vars.space[1]} ${vars.space[3]}`,
  background: vars.color.success,
  color: vars.color.textPrimary,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  boxShadow: "0 2px 8px rgba(0,0,0,0.3)",
  pointerEvents: "none",
});

// Animation-only class — add/remove this to restart the keyframe without re-mounting.
export const copiedToastVisible = style({
  animation: `${fadeInOut} 1.5s ease-in-out forwards`,
});

// ---- Custom left-side scroll track ----
// Positioned on the LEFT edge so it doesn't conflict with the browser's own
// scrollbar which always lives on the right. Shows only when scrollback exists.
// The track itself has no pointer events; the thumb overrides that.
export const scrollTrack = style({
  position: "absolute",
  left: 0,
  top: 0,
  bottom: 0,
  width: 8,
  zIndex: zIndex.floatingTerminalUI,
  // Track itself receives clicks (for jump-to-position); thumb overrides with its own handlers.
  pointerEvents: "auto",
  cursor: "pointer",
  touchAction: "none",
});

export const scrollThumb = style({
  position: "absolute",
  left: 1,
  top: 0,
  width: 6,
  borderRadius: 3,
  background: "rgba(255, 255, 255, 0.28)",
  pointerEvents: "auto",
  touchAction: "none",
  cursor: "grab",
  userSelect: "none",
  WebkitUserSelect: "none",
  selectors: {
    "&:active": {
      background: "rgba(255, 255, 255, 0.55)",
      cursor: "grabbing",
    },
  },
});

// ---- Mobile selection handles ----
// Two draggable circles rendered via portal below the selection start/end.
// Mimic native iOS/Android text-selection handle UX on the canvas terminal.
// Shown only on touch devices (pointer: coarse); hidden on desktop.
export const selectionHandle = style({
  position: "fixed",
  width: 22,
  height: 22,
  borderRadius: "50%",
  background: "#2B7FE3",
  border: "2.5px solid white",
  boxShadow: "0 2px 6px rgba(0, 0, 0, 0.4)",
  touchAction: "none",
  zIndex: 1090,
  cursor: "default",
  userSelect: "none",
  WebkitUserSelect: "none",
  // Transform centers the circle on the anchor x-coordinate and positions
  // it flush below the text row (y = bottom of row).
  transform: "translate(-50%, 0)",
  // Vertical cursor bar above the circle (visual stem)
  "::before": {
    content: '""',
    position: "absolute",
    width: 2,
    height: 14,
    background: "#2B7FE3",
    borderRadius: 1,
    bottom: "100%",
    left: "50%",
    transform: "translateX(-50%)",
    marginBottom: 1,
  },
});
