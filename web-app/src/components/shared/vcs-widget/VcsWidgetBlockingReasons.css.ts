import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const list = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[1],
  margin: 0,
  padding: 0,
  listStyle: "none",
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

export const staleNotice = style({
  color: vars.color.warningText,
  fontSize: vars.fontSize.xs,
});
