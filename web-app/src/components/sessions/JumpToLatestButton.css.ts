import { style, keyframes } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

const fadeIn = keyframes({
  "0%": { opacity: 0 },
  "100%": { opacity: 1 },
});

// Fixed width so the two route labels ("Jump to latest" / "Page down to latest") never shift layout.
export const button = style({
  position: "absolute",
  right: vars.space[2],
  zIndex: zIndex.floatingTerminalUI,
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  gap: vars.space[1],
  minHeight: "44px",
  minWidth: "44px",
  width: "11.5rem",
  padding: `${vars.space[1]} ${vars.space[3]}`,
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  boxShadow: "0 2px 8px rgba(0,0,0,0.3)",
  cursor: "pointer",
  animation: `${fadeIn} 120ms ease-out`,
  transition: "opacity 120ms ease-out",
  selectors: {
    '&[data-corner="bottom-right"]': { bottom: vars.space[2] },
    '&[data-corner="top-right"]': { top: vars.space[2] },
  },
  "@media": {
    "(prefers-reduced-motion: reduce)": {
      transition: "none",
      animation: "none",
    },
  },
});

export const newOutputDot = style({
  width: "8px",
  height: "8px",
  borderRadius: vars.radii.full,
  background: vars.color.primaryText,
  flexShrink: 0,
});
