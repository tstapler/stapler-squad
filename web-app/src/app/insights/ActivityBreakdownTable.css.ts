// +feature: insights-activity-breakdown
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export { tableCard, tableTitle, table, th, thRight } from "./insightsTableShared.css";

export const td = style({
  padding: `${vars.space[2]} ${vars.space[2]}`,
  color: vars.color.textPrimary,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
});

export const tdRight = style([td, { textAlign: "right" }]);

export const empty = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  padding: vars.space[3],
});
