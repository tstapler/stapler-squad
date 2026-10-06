import { style } from "@vanilla-extract/css";

// Shell, form primitives and row styles are reused, not redeclared (jscpd gate).
export {
  container,
  heading,
  description,
  loadingText,
  form,
  field,
  label,
  input,
  hint,
  warningText,
  actions,
  saveStatus,
  saveError,
} from "./TaggingClassifierSettings.css";

// Stacks label above select on narrow screens; 44px touch targets for selects.
export const featureRow = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: "0.5rem",
});

export const featureName = style({
  flex: "1 1 12rem",
  fontSize: "0.875rem",
});

export const select = style({
  flex: "1 1 10rem",
  minHeight: "44px",
});

export const inputTouch = style({ minHeight: "44px" });

export const buttonTouch = style({ minHeight: "44px" });
