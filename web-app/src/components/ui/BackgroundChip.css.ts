import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const chip = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `0 ${vars.space[2]}`,
  background: vars.color.surfaceMuted,
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  lineHeight: 1.5,
  whiteSpace: "nowrap",
});
