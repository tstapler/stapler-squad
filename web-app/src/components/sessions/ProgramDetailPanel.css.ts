import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const panel = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  padding: vars.space[4],
  marginTop: vars.space[3],
});

export const panelHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  marginBottom: vars.space[3],
  borderBottom: `1px solid ${vars.color.borderColor}`,
  paddingBottom: vars.space[2],
});

export const panelTitle = style({
  fontSize: vars.fontSize.lg,
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const closeButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.textMuted,
  padding: vars.space[1],
  borderRadius: vars.radii.sm,
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
      background: vars.color.hoverBackground,
    },
  },
});

export const sectionTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  marginBottom: vars.space[2],
  marginTop: vars.space[4],
});

export const breakdownTable = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: vars.fontSize.sm,
});

export const th = style({
  textAlign: "left",
  padding: `${vars.space[1]} ${vars.space[2]}`,
  color: vars.color.textMuted,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  fontWeight: 500,
});

export const td = style({
  padding: `${vars.space[1]} ${vars.space[2]}`,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textPrimary,
});

export const examplesList = style({
  listStyle: "none",
  padding: 0,
  margin: 0,
  display: "flex",
  flexDirection: "column",
  gap: vars.space[1],
});

export const exampleItem = style({
  fontFamily: "monospace",
  fontSize: vars.fontSize.xs,
  background: vars.color.terminalBackground,
  color: vars.color.terminalForeground,
  padding: `${vars.space[1]} ${vars.space[2]}`,
  borderRadius: vars.radii.sm,
  overflowX: "auto",
  whiteSpace: "nowrap",
});

export const coverageBadge = style({
  display: "inline-flex",
  alignItems: "center",
  fontSize: vars.fontSize.xs,
  padding: `1px ${vars.space[1]}`,
  borderRadius: vars.radii.sm,
});

export const coverageYes = style([
  coverageBadge,
  {
    background: vars.color.successBg,
    color: vars.color.success,
  },
]);

export const coveragePartial = style([
  coverageBadge,
  {
    background: vars.color.warningBg,
    color: vars.color.warning,
  },
]);

export const coverageNo = style([
  coverageBadge,
  {
    background: vars.color.errorBg,
    color: vars.color.error,
  },
]);

export const addRuleLink = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space[1],
  fontSize: vars.fontSize.xs,
  color: vars.color.primary,
  textDecoration: "none",
  selectors: {
    "&:hover": { textDecoration: "underline" },
  },
});

export const sparklineBar = style({
  display: "inline-block",
  height: "12px",
  minWidth: "2px",
  background: vars.color.primary,
  borderRadius: "1px",
  opacity: 0.7,
});

export const trendRow = style({
  display: "flex",
  alignItems: "flex-end",
  gap: "2px",
  height: "32px",
  padding: `${vars.space[2]} 0`,
});

export const trendBar = style({
  display: "inline-block",
  minWidth: "4px",
  background: vars.color.primary,
  borderRadius: "1px 1px 0 0",
  opacity: 0.7,
});

export const trendDate = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  marginTop: vars.space[1],
});

export const trendSection = style({
  overflowX: "auto",
});

export const loadingState = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  padding: vars.space[4],
  textAlign: "center",
});

export const errorState = style({
  color: vars.color.error,
  fontSize: vars.fontSize.sm,
  padding: vars.space[3],
  background: vars.color.errorBg,
  borderRadius: vars.radii.sm,
});
