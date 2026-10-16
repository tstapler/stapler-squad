// +feature: backlog:item-budget-warning
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

// Same color tokens as ProjectedCostCard.css.ts's `warning`/`warningText`
// variants (design/ux.md's "Shared emphasis convention") — a user who's
// already learned what "over budget" looks like on the Insights page
// recognizes this banner instantly, even though it's a separate component
// answering a different question (this item's spend, not total spend).
export const banner = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  padding: `${vars.space[2]} ${vars.space[3]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.warning}`,
  background: vars.color.warningBg,
});

export const icon = style({
  flexShrink: 0,
});

export const text = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.warningText,
  fontWeight: vars.fontWeight.medium,
});
