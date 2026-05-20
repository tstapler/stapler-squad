// +feature: insights-dashboard
import { style, styleVariants } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const badge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: "3px",
  padding: `1px ${vars.space[2]}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  fontFamily: vars.font.mono,
  lineHeight: 1.4,
  whiteSpace: "nowrap",
  border: "1px solid transparent",
});

export const badgeVariant = styleVariants({
  normal: {
    background: vars.color.surfaceSubtle,
    color: vars.color.textSecondary,
    borderColor: vars.color.borderSubtle,
  },
  warning: {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    borderColor: vars.color.warning,
  },
  alert: {
    background: vars.color.errorBg,
    color: vars.color.errorText,
    borderColor: vars.color.error,
  },
});
