import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import { zIndex } from "@/styles/theme-contract.css";

export const container = style({
  position: "relative",
  width: "100%",
});

export const input = style({
  width: "100%",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.mono,
  outline: "none",
  transition: "border-color 0.15s ease",
  boxSizing: "border-box",
  textOverflow: "ellipsis",
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
    boxShadow: `0 0 0 2px ${vars.color.accentBg}`,
  },
  "::placeholder": {
    color: vars.color.placeholderColor,
    fontFamily: vars.font.sans,
  },
  ":disabled": {
    opacity: 0.6,
    cursor: "not-allowed",
    background: vars.color.surfaceMuted,
  },
});

export const inputError = style({
  borderColor: `${vars.color.error} !important` as "inherit",
  ":focus": {
    borderColor: vars.color.error,
    boxShadow: `0 0 0 2px ${vars.color.errorBg}`,
  },
});

export const hint = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  marginTop: vars.space["1"],
});

export const githubHint = style({
  color: vars.color.primary,
  fontSize: vars.fontSize.xs,
  marginTop: vars.space["1"],
});

export const dropdownWrapper = style({
  position: "absolute",
  top: "calc(100% + 2px)",
  left: 0,
  right: 0,
  zIndex: zIndex.dropdown,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  boxShadow: "0 4px 12px rgba(0, 0, 0, 0.15)",
  overflow: "hidden",
  background: vars.color.cardBackground,
});
