import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

/**
 * "Show N more" control (useShowMore, Task 3.1.4c2) — a real <button>
 * meeting the ≥44×44px touch-target requirement (ux.md Surface 9), not a
 * bare text link.
 */
export const showMoreButton = style({
  marginTop: vars.space["2"],
  minHeight: "44px",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: "none",
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  color: vars.color.primary,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  alignSelf: "flex-start",

  ":hover": {
    color: vars.color.primaryHover,
    background: vars.color.hoverBackground,
  },
});

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
