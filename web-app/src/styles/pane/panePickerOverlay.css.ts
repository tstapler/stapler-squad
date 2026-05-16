import { style } from "@vanilla-extract/css";
import { keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const fadeIn = keyframes({
  from: { opacity: 0 },
  to: { opacity: 1 },
});

const slideUp = keyframes({
  from: { opacity: 0, transform: "translateY(8px)" },
  to: { opacity: 1, transform: "translateY(0)" },
});

export const pickerOverlay = style({
  position: "absolute",
  inset: 0,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  background: "rgba(0,0,0,0.55)",
  zIndex: 20,
  cursor: "pointer",
  animation: `${fadeIn} 80ms ease`,
});

export const pickerActionBar = style({
  position: "absolute",
  bottom: 0,
  left: 0,
  right: 0,
  zIndex: 25,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  background: "rgba(10, 10, 10, 0.88)",
  backdropFilter: "blur(12px)",
  borderTop: "1px solid rgba(255, 255, 255, 0.08)",
  animation: `${slideUp} 120ms ease`,
});

export const pickerActionButton = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `5px ${vars.space["3"]}`,
  borderRadius: vars.radii.md,
  border: "1px solid rgba(255, 255, 255, 0.15)",
  background: "rgba(255, 255, 255, 0.06)",
  color: "rgba(255, 255, 255, 0.9)",
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.sans,
  cursor: "pointer",
  transition: "background 100ms, border-color 100ms",
  whiteSpace: "nowrap",
  selectors: {
    "&:hover": {
      background: "rgba(255, 255, 255, 0.14)",
      borderColor: "rgba(255, 255, 255, 0.3)",
    },
  },
});

export const pickerActionKbd = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "18px",
  height: "18px",
  padding: "0 3px",
  borderRadius: "3px",
  background: "rgba(255, 255, 255, 0.12)",
  border: "1px solid rgba(255, 255, 255, 0.2)",
  fontSize: "11px",
  fontFamily: vars.font.mono,
  fontWeight: vars.fontWeight.bold,
  color: "rgba(255, 255, 255, 0.7)",
  lineHeight: 1,
  marginLeft: vars.space["1"],
});

export const pickerLabel = style({
  width: "56px",
  height: "56px",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  borderRadius: vars.radii.lg,
  background: vars.color.primary,
  color: vars.color.primaryText,
  fontSize: "28px",
  fontWeight: vars.fontWeight.bold,
  fontFamily: vars.font.mono,
  boxShadow: vars.shadow.lg,
  pointerEvents: "none",
  userSelect: "none",
});
