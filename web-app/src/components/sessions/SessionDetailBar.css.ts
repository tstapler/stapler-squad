import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const bar = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[3],
  minHeight: "44px", /* WCAG 2.1 AA touch target minimum; was 40px */
  padding: `0 ${vars.space[3]}`,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  flexShrink: 0,
  overflow: "hidden",
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.sm,
});

export const branchName = style({
  color: vars.color.textPrimary,
  fontWeight: vars.fontWeight.medium,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  maxWidth: "200px",
  flexShrink: 1,
});

export const pathText = style({
  color: vars.color.textMuted,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  flex: 1,
  minWidth: 0,
});

export const spacer = style({
  flex: 1,
});

export const shortcutHints = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[3],
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  flexShrink: 0,
  "@media": {
    "screen and (max-width: 768px)": {
      display: "none",
    },
  },
});

export const hint = style({
  display: "flex",
  alignItems: "center",
  gap: "4px",
});

export const backButton = style({
  display: "none",
  alignItems: "center",
  justifyContent: "center",
  padding: `0 ${vars.space[3]}`,
  minHeight: "44px", /* WCAG 2.1 AA touch target minimum */
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  color: vars.color.textSecondary,
  cursor: "pointer",
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      display: "flex",
    },
  },
});
