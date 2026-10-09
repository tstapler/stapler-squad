import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const changesLine = style({
  display: "flex",
  alignItems: "center",
  flexWrap: "wrap",
  gap: vars.space["2"],
  minHeight: "0",
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const refreshListButton = style({
  minHeight: "44px",
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  background: vars.color.accentBg,
  color: vars.color.accentText,
  border: `1px solid ${vars.color.inputFocusBorder}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  cursor: "pointer",
  ":focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "2px" },
});

export const groupList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  minWidth: 0,
});
