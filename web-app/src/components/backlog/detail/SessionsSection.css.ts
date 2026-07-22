import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// "Show N more" control (useShowMore, Task 3.1.4c2) — shared with
// ProgressHistorySection and WorkflowHistorySection, see detailShared.css.ts.
export { showMoreButton } from "./detailShared.css";

/**
 * Story 4.1.4: takes the same flex slot `sessionLink`'s <a> occupied for a
 * real session row, sized to fill the row's main axis while leaving room for
 * the sibling delete button (`sessionRowMain` is a flex row).
 */
export const diagnosticRowWrapper = style({
  flex: 1,
  minWidth: 0,
});

export const diagnosticRowTitle = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  minWidth: 0,
  flex: 1,
});
