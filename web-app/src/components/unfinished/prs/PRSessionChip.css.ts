import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import { zIndex } from "@/styles/theme-contract.css";
import { MIN_TOUCH_TARGET } from "@/lib/unfinished/prTouchTokens";

const chipBase = {
  display: "inline-flex",
  alignItems: "center",
  minHeight: MIN_TOUCH_TARGET,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  textDecoration: "none",
  cursor: "pointer",
  maxWidth: "100%",
  overflowWrap: "anywhere",
} as const;

export const wrap = style({ position: "relative", display: "inline-flex", maxWidth: "100%" });
export const chip = style(chipBase);
export const menu = style({
  position: "absolute",
  top: "100%",
  left: 0,
  zIndex: zIndex.raised,
  display: "flex",
  flexDirection: "column",
  minWidth: "100%",
  margin: 0,
  padding: vars.space["1"],
  listStyle: "none",
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
});
export const menuLink = style({ ...chipBase, border: "none", color: vars.color.textPrimary });
