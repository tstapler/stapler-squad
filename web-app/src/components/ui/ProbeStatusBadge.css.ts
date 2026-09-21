import { keyframes, style, styleVariants } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const spin = keyframes({ to: { transform: "rotate(360deg)" } });

export const root = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "flex-start",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  minWidth: 0,
});

export const tone = styleVariants({
  success: { background: vars.color.successBg, color: vars.color.successText },
  neutral: { background: vars.color.hoverBackground, color: vars.color.textSecondary },
  warning: { background: vars.color.warningBg, color: vars.color.warningText },
});

export const status = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space["2"],
  flex: "1 1 12rem",
  minWidth: 0,
  overflowWrap: "anywhere",
});

export const icon = style({ flexShrink: 0, width: "16px", height: "16px" });

export const spinner = style({
  flexShrink: 0,
  width: "16px",
  height: "16px",
  border: "2px solid currentColor",
  borderTopColor: "transparent",
  borderRadius: "50%",
  animation: `${spin} 0.8s linear infinite`,
});

export const spinnerStatic = style({
  flexShrink: 0,
  width: "16px",
  height: "16px",
  border: "2px solid currentColor",
  borderTopColor: "transparent",
  borderRadius: "50%",
});

export const path = style({
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  maxWidth: "100%",
});

export const pathExpanded = style({ overflowWrap: "anywhere" });

export const detail = style({
  flexBasis: "100%",
  fontSize: vars.fontSize.xs,
  opacity: 0.9,
});

// 44px targets: full-width row under the text below 640px, inline after it above.
export const button = style({
  minHeight: "44px",
  minWidth: "44px",
  padding: `0 ${vars.space["4"]}`,
  border: "1px solid currentColor",
  borderRadius: vars.radii.md,
  background: "transparent",
  color: "inherit",
  font: "inherit",
  cursor: "pointer",
  flexBasis: "100%",
  selectors: { "&:disabled": { cursor: "progress", opacity: 0.7 } },
  "@media": { "(min-width: 640px)": { flexBasis: "auto" } },
});

export const toggle = style([button, { border: "none", textDecoration: "underline", flexBasis: "auto" }]);
