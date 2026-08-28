import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const list = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[1],
  margin: 0,
  padding: 0,
  listStyle: "none",
});

export const row = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  fontSize: vars.fontSize.sm,
});

export const name = style({
  color: vars.color.textPrimary,
});

export const context = style({
  color: vars.color.textSecondary,
});

export const checkSuccess = style({ color: vars.color.success });
export const checkFailure = style({ color: vars.color.errorText });
export const checkPending = style({ color: vars.color.textSecondary });
