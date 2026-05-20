// +feature: insights-dashboard
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const chartCard = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  padding: `${vars.space[4]} ${vars.space[4]}`,
});

export const chartTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  marginBottom: vars.space[3],
});

export const chartWrap = style({
  width: "100%",
  height: "240px",
});

export const emptyChart = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  height: "240px",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});

export const legend = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space[3],
  marginTop: vars.space[2],
});

export const legendItem = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[1],
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

export const legendSwatch = style({
  width: "10px",
  height: "10px",
  borderRadius: "2px",
  flexShrink: 0,
});
