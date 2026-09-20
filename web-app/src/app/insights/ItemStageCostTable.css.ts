// +feature: insights-dashboard
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const card = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  padding: `${vars.space[3]} ${vars.space[4]}`,
});

export const title = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  marginBottom: vars.space[2],
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

export const thRight = style([th, { textAlign: "right" }]);

export const td = style({
  padding: `${vars.space[1]} ${vars.space[2]}`,
  color: vars.color.textPrimary,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
});

export const tdRight = style([td, { textAlign: "right", color: vars.color.textSecondary }]);

// Shares StageCostChart's unpriced-badge / rework-indicator emphasis
// convention (bold + icon + text, never color alone) — ux.md's "Shared
// emphasis convention" section names both as using the same generic
// warning glyph since they never co-occur in the same row.
export const emphasis = style({
  color: vars.color.warningText,
  fontWeight: vars.fontWeight.semibold,
});

export const errorState = style({
  color: vars.color.error,
  fontSize: vars.fontSize.sm,
  padding: `${vars.space[2]} 0`,
});
