import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

/** Used for diff added lines and approved review counts. */
export const diffAdded = style({
  color: vars.color.success,
});
