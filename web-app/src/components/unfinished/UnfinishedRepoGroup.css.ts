import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const group = style({
  marginBottom: vars.space["4"],
});

export const header = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  cursor: "pointer",
  userSelect: "none",
  outline: "none",
  ":hover": {
    background: vars.color.hoverBackground,
  },
  ":focus-visible": {
    boxShadow: `0 0 0 2px ${vars.color.inputFocusBorder}`,
  },
});

export const chevron = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  transition: "transform 0.15s",
  display: "inline-block",
});

export const chevronExpanded = style({
  transform: "rotate(90deg)",
});

export const repoName = style({
  fontWeight: 600,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  fontFamily: vars.font.mono,
});

export const count = style({
  marginLeft: vars.space["2"],
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  background: vars.color.surfaceSubtle,
  borderRadius: vars.radii.full,
  padding: `2px ${vars.space["2"]}`,
});

export const itemList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  paddingLeft: vars.space["4"],
  marginTop: vars.space["2"],
});
