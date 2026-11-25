import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const badge = style({
  display: "inline-flex",
  alignItems: "center",
  borderRadius: vars.radii.sm,
  padding: `0 ${vars.space["1"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  height: "20px",
  whiteSpace: "nowrap",
  border: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textPrimary,
});
