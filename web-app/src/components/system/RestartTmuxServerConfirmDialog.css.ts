import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";
import {
  confirmOverlay as overlay,
  confirmDialog as dialog,
  confirmHeading as heading,
  confirmBody as body,
  confirmActions as actions,
  confirmSecondaryButton as secondaryButton,
} from "@/styles/modalChrome.css";

export { overlay, dialog, heading, body, actions, secondaryButton };

// Distinct from confirmPrimaryButton: this action is destructive (kills every
// live session on the server), so it's styled with the error palette rather
// than the ordinary primary-action color.
export const dangerButton = style({
  backgroundColor: vars.color.error,
  color: vars.color.errorText,
  border: "none",
  borderRadius: vars.radii.md,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  transition: vars.transition.fast,
  ":hover": {
    opacity: 0.9,
  },
  ":disabled": {
    opacity: 0.6,
    cursor: "not-allowed",
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.error}`,
    outlineOffset: "2px",
  },
});
