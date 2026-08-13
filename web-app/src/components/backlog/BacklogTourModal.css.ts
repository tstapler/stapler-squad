import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const calloutBox = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  lineHeight: "1.6",
  padding: vars.space["3"],
  background: vars.color.accentBg,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  marginBottom: vars.space["4"],
});

export const flagList = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  lineHeight: "1.6",
  marginBottom: vars.space["4"],
  paddingLeft: vars.space["4"],
});
