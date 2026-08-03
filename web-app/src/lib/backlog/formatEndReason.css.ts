import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const chip = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  lineHeight: 1.5,
  flexShrink: 0,
  whiteSpace: "nowrap",
});

export const chipWarning = style([
  chip,
  {
    background: vars.color.warningBg,
    color: vars.color.warningText,
    border: `1px solid ${vars.color.warning}`,
  },
]);

export const chipError = style([
  chip,
  {
    background: vars.color.errorBg,
    color: vars.color.errorText,
    border: `1px solid ${vars.color.error}`,
  },
]);
