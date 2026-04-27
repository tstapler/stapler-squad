import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const card = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["3"]} ${vars.space["4"]}`,
  cursor: "pointer",
  transition: "background 0.15s, border-color 0.15s",
  outline: "none",
  ":hover": {
    background: vars.color.hoverBackground,
    borderColor: vars.color.borderHover,
  },
  ":focus-visible": {
    borderColor: vars.color.inputFocusBorder,
    boxShadow: `0 0 0 2px ${vars.color.inputFocusBorder}`,
  },
});

export const cardExpanded = style({
  borderBottomLeftRadius: 0,
  borderBottomRightRadius: 0,
  borderBottom: "none",
});

export const header = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  flexWrap: "wrap",
});

export const branch = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  fontWeight: 600,
  flexShrink: 0,
});

export const path = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  flexGrow: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const chips = style({
  display: "flex",
  gap: vars.space["2"],
  alignItems: "center",
  flexWrap: "wrap",
});

export const chip = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  lineHeight: 1.5,
});

export const chipUncommitted = style([
  chip,
  {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    border: `1px solid ${vars.color.warning}`,
  },
]);

export const chipAhead = style([
  chip,
  {
    background: vars.color.successBg,
    color: vars.color.success,
    border: `1px solid ${vars.color.success}`,
  },
]);

export const chipBehind = style([
  chip,
  {
    background: vars.color.accentBg,
    color: vars.color.primary,
    border: `1px solid ${vars.color.primary}`,
  },
]);

export const chipTimeout = style([
  chip,
  {
    background: vars.color.errorBg,
    color: vars.color.errorText,
    border: `1px solid ${vars.color.error}`,
  },
]);

export const actions = style({
  display: "flex",
  gap: vars.space["2"],
  marginLeft: "auto",
  opacity: 0,
  transition: "opacity 0.15s",
  selectors: {
    [`${card}:hover &, ${card}:focus-within &`]: {
      opacity: 1,
    },
  },
});

export const actionBtn = style({
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  padding: `2px ${vars.space["2"]}`,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  cursor: "pointer",
  lineHeight: 1.5,
  transition: "background 0.12s, color 0.12s, border-color 0.12s",
  ":hover": {
    background: vars.color.hoverBackground,
    color: vars.color.textPrimary,
    borderColor: vars.color.borderHover,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.inputFocusBorder}`,
    outlineOffset: "1px",
  },
});

export const dismissBtn = style([
  actionBtn,
  {
    ":hover": {
      background: vars.color.errorBg,
      color: vars.color.errorText,
      borderColor: vars.color.error,
    },
  },
]);
