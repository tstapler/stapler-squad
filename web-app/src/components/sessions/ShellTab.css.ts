// +feature: shell-tabs
import { style, styleVariants } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const tabLabel = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  userSelect: "none",
});

export const errorIndicator = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.error,
  flexShrink: 0,
});

export const statusDotBase = style({
  width: "8px",
  height: "8px",
  borderRadius: vars.radii.full,
  flexShrink: 0,
});

export const statusDot = styleVariants({
  running: [statusDotBase, { backgroundColor: vars.color.success }],
  stopped: [statusDotBase, { backgroundColor: vars.color.textMuted }],
  error: [statusDotBase, { backgroundColor: vars.color.error }],
});

export const tabName = style({
  fontSize: vars.fontSize.sm,
  color: "inherit",
  maxWidth: "120px",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const actions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[1],
  marginLeft: vars.space[1],
});

export const actionButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "18px",
  height: "18px",
  borderRadius: vars.radii.sm,
  border: "none",
  backgroundColor: "transparent",
  color: vars.color.textMuted,
  cursor: "pointer",
  padding: 0,
  fontSize: vars.fontSize.xs,
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
});
