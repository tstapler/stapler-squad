import { style, styleVariants } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

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
