import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

/** Inline field-level error text, associated to its input via aria-describedby (ux.md Surface 5). */
export const fieldError = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.errorText,
  margin: 0,
});

export const invalidInput = style({
  borderColor: vars.color.error,
});
