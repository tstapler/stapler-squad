import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Visual tokens are DeepLinkErrorBanner's "status" (warning) treatment, reused
// verbatim: a claim conflict is informational, never an alert.
export {
  statusContainer as container,
  icon,
  headline,
  body,
  actions,
  primaryActionButton as actionButton,
} from "../DeepLinkErrorBanner.css";

export const content = style({
  display: "flex",
  flexDirection: "column",
  minWidth: 0,
  flex: 1,
});

export const formSlot = style({
  marginTop: vars.space["2"],
});

/** Visually hidden but announced: the copy-confirmation live region. */
export const srOnly = style({
  position: "absolute",
  width: "1px",
  height: "1px",
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
  whiteSpace: "nowrap",
});
