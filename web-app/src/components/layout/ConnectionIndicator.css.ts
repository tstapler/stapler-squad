import { style, styleVariants, keyframes } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

export const button = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  padding: "4px 6px",
  borderRadius: 6,
  display: "flex",
  alignItems: "center",
  gap: 6,
  fontSize: "0.75rem",
  fontWeight: 600,
  userSelect: "none",
  ":hover": {
    opacity: 0.8,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: 2,
  },
  selectors: {
    "&:disabled": {
      cursor: "default",
    },
  },
});

const dotBase = style({
  width: 8,
  height: 8,
  borderRadius: "50%",
  flexShrink: 0,
  display: "inline-block",
});

export const dots = styleVariants({
  connected: [dotBase, { background: vars.color.success }],
  stale: [dotBase, { background: vars.color.warning }],
  disconnected: [dotBase, { background: vars.color.error }],
});

const labelBase = style({
  // Hidden on narrow viewports, visible on >=640px
  display: "none",
  "@media": {
    "screen and (min-width: 640px)": {
      display: "inline",
    },
  },
});

export const labels = styleVariants({
  connected: [labelBase, { color: vars.color.success }],
  stale: [labelBase, { color: vars.color.warningText }],
  disconnected: [labelBase, { color: vars.color.errorText }],
});

// Spinner animation for reconnecting states
const spin = keyframes({
  from: { transform: "rotate(0deg)" },
  to: { transform: "rotate(360deg)" },
});

export const spinner = style({
  width: 8,
  height: 8,
  borderRadius: "50%",
  flexShrink: 0,
  display: "inline-block",
  border: `2px solid ${vars.color.warning}`,
  borderTopColor: "transparent",
  animation: `${spin} 0.8s linear infinite`,
});

// Visually hidden aria-live region
export const ariaLiveRegion = style({
  position: "absolute",
  width: 1,
  height: 1,
  padding: 0,
  margin: -1,
  overflow: "hidden",
  clip: "rect(0, 0, 0, 0)",
  whiteSpace: "nowrap",
  borderWidth: 0,
});

// Tooltip shown when reconnecting
export const tooltip = style({
  position: "absolute",
  top: "100%",
  right: 0,
  marginTop: 4,
  background: vars.color.background,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 6,
  padding: `${vars.space[2]} ${vars.space[3]}`,
  whiteSpace: "nowrap",
  zIndex: zIndex.dropdown,
  fontSize: "0.75rem",
  boxShadow: "0 2px 8px rgba(0,0,0,0.15)",
});

export const tooltipReloadLink = style({
  color: vars.color.primary,
  textDecoration: "underline",
  cursor: "pointer",
});

export const wrapper = style({
  position: "relative",
  display: "inline-block",
});
