// +feature: insights-dashboard
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const tableCard = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  padding: `${vars.space[4]} ${vars.space[4]}`,
  overflowX: "auto",
});

export const tableHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  marginBottom: vars.space[3],
  gap: vars.space[2],
  flexWrap: "wrap",
});

export const tableTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const orphanToggle = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[1],
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  cursor: "pointer",
  userSelect: "none",
  padding: `${vars.space[1]} ${vars.space[2]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderSubtle}`,
  background: "transparent",
  ":hover": {
    background: vars.color.hoverBackground,
  },
});

export const table = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: vars.fontSize.sm,
  minWidth: "600px",
});

export const th = style({
  textAlign: "left",
  padding: `${vars.space[1]} ${vars.space[2]}`,
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  whiteSpace: "nowrap",
});

export const thRight = style([
  th,
  { textAlign: "right" },
]);

export const td = style({
  padding: `${vars.space[2]} ${vars.space[2]}`,
  color: vars.color.textPrimary,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  maxWidth: "180px",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const tdRight = style([
  td,
  { textAlign: "right", color: vars.color.textSecondary },
]);

export const tdMono = style([
  td,
  { fontFamily: vars.font.mono, fontSize: vars.fontSize.xs, color: vars.color.textMuted },
]);

export const orphanBadge = style({
  display: "inline-block",
  padding: `1px ${vars.space[1]}`,
  background: vars.color.warningBg,
  color: vars.color.warningText,
  borderRadius: vars.radii.sm,
  fontSize: "10px",
  fontWeight: vars.fontWeight.medium,
  marginLeft: vars.space[1],
  verticalAlign: "middle",
});

export const empty = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  padding: `${vars.space[4]} 0`,
  textAlign: "center",
});
