import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const wrapper = style({
  position: "relative",
  display: "inline-flex",
});

export const triggerButton = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
  background: "none",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  cursor: "pointer",
  whiteSpace: "nowrap",
  ":hover": {
    color: vars.color.textPrimary,
    background: vars.color.hoverBackground,
  },
});

export const triggerButtonActive = style({
  color: vars.color.textPrimary,
  background: vars.color.hoverBackground,
  borderColor: vars.color.inputFocusBorder,
});

export const dropdown = style({
  position: "absolute",
  top: "calc(100% + 4px)",
  right: 0,
  zIndex: 200,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  boxShadow: "0 4px 12px rgba(0,0,0,0.15)",
  padding: vars.space["2"],
  minWidth: "160px",
});

export const dropdownTitle = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.06em",
  padding: `0 ${vars.space["1"]} ${vars.space["1"]}`,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  marginBottom: vars.space["1"],
});

export const checkboxRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["1"]} ${vars.space["1"]}`,
  borderRadius: vars.radii.sm,
  cursor: "pointer",
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  userSelect: "none",
  ":hover": {
    background: vars.color.hoverBackground,
  },
});

export const checkbox = style({
  width: "14px",
  height: "14px",
  flexShrink: 0,
  cursor: "pointer",
  accentColor: vars.color.primary,
});
