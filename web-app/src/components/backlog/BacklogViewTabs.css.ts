import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const tabBar = style({
  display: "flex",
  gap: 0,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  padding: `0 ${vars.space["6"]}`,
  flexShrink: 0,
});

export const tab = style({
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textMuted,
  textDecoration: "none",
  transition: "color 0.1s ease, border-color 0.1s ease",
  cursor: "pointer",
  background: "none",
  border: "none",
  borderBottom: "2px solid transparent",
  ":hover": {
    color: vars.color.textPrimary,
  },
});

// Underline and bold, so the active tab is not marked by color alone.
export const tabActive = style({
  color: vars.color.primary,
  fontWeight: vars.fontWeight.semibold,
  borderBottom: `2px solid ${vars.color.primary}`,
});
