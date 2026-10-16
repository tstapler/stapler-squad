import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import * as formStyles from "./BacklogItemForm.css";

export const root = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  padding: vars.space["4"],
});

export const title = style({
  margin: 0,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textPrimary,
});

// Touch targets stay ≥ 44px on every viewport (WCAG 2.5.5).
export const touchTarget = style({
  minHeight: "44px",
  minWidth: "44px",
});

export const textarea = style({
  minHeight: "160px",
  resize: "vertical",
});

export const actions = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space["2"],
});

export const spacer = style({
  flex: 1,
});

// formStyles.attachButton's `primary` text fails WCAG AA (3.28:1) on the dark
// theme (found by the e2e axe scan); body text color is AA on every surface.
export const attachButton = style([
  formStyles.attachButton,
  touchTarget,
  { color: vars.color.textPrimary },
]);
