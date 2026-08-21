// +feature: insights-dashboard
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const tableCard = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  padding: `${vars.space[4]} ${vars.space[4]}`,
});

export const tableTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  marginBottom: vars.space[3],
});

export const table = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: vars.fontSize.sm,
});

export const th = style({
  textAlign: "left",
  padding: `${vars.space[1]} ${vars.space[2]}`,
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
});

export const thRight = style([
  th,
  { textAlign: "right" },
]);

export const td = style({
  padding: `${vars.space[2]} ${vars.space[2]}`,
  color: vars.color.textPrimary,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  maxWidth: "200px",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const tdRight = style([
  td,
  { textAlign: "right", color: vars.color.textSecondary },
]);

export const empty = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  padding: `${vars.space[3]} 0`,
});
