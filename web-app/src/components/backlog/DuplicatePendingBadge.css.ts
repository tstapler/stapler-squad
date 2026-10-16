import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const badge = style({
  display: "inline-flex",
  alignItems: "center",
  borderRadius: vars.radii.sm,
  padding: `1px ${vars.space["2"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  background: vars.color.errorBg,
  color: vars.color.errorText,
  border: `1px solid ${vars.color.error}`,
  whiteSpace: "nowrap",
});
