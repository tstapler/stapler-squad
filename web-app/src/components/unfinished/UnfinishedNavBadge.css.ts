import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const badge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "1.25rem",
  height: "1.25rem",
  padding: `0 ${vars.space["2"]}`,
  background: vars.color.warning,
  color: vars.color.textInverse,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  borderRadius: vars.radii.full,
  lineHeight: 1,
  pointerEvents: "none",
});

export const inline = style({
  marginLeft: vars.space["2"],
  verticalAlign: "middle",
});
