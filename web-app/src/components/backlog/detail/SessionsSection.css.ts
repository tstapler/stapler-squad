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
