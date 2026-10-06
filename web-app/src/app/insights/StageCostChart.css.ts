// +feature: insights-dashboard
// Story 5.2.1: StageCostChart reuses ModelBreakdownChart.css.ts's chartCard/
// chartWrap/emptyChart/legendRow/legendItem/legendDot/unpricedLabel classes
// verbatim (imported directly at the call site, not re-declared here) to
// avoid the jscpd duplication gate flagging near-identical card styling —
// this file holds only the styles genuinely new to this chart: the
// clickable/keyboard-focusable legend entry and its "active filter" ring.
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const legendButton = style({
  cursor: "pointer",
  border: "none",
  background: "transparent",
  padding: 0,
  font: "inherit",
  selectors: {
    "&:focus-visible": {
      outline: `2px solid ${vars.color.primary}`,
      outlineOffset: "2px",
      borderRadius: vars.radii.sm,
    },
  },
});

// Applied to the active (bar-clicked / cross-filtering) legend entry so a
// user who clicked "work" and looked away doesn't forget the sessions table
// is filtered (ux.md Surface B+C interaction flow, step 6).
export const legendButtonActive = style({
  boxShadow: `0 0 0 2px ${vars.color.primary}`,
  borderRadius: vars.radii.sm,
});
