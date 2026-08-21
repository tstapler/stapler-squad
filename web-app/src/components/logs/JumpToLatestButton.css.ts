import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import { zIndex } from "@/styles/theme-contract.css";

export const pill = style({
  position: "fixed",
  // Anchor above the iOS home indicator — safe-area-inset-bottom clears the home bar
  bottom: "calc(env(safe-area-inset-bottom, 0px) + 16px)",
  right: vars.space["4"],
  zIndex: zIndex.raised,
  display: "flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  backgroundColor: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  // Pill shape
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  cursor: "pointer",
  boxShadow: vars.shadow.md,
  transition: "opacity 0.15s, transform 0.15s",
  // Eliminate iOS 300ms tap delay
  touchAction: "manipulation",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.primaryHover,
    },
    "&:active": {
      backgroundColor: vars.color.primaryActive,
      transform: "scale(0.97)",
    },
  },
});
