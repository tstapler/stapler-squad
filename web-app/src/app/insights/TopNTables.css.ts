// +feature: insights-dashboard
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export { tableCard, tableTitle, table, th, thRight } from "./insightsTableShared.css";

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
