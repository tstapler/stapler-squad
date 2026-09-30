import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const list = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[3],
  margin: 0,
  padding: 0,
  listStyle: "none",
});

export const review = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[1],
});

export const author = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const body = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  whiteSpace: "pre-wrap",
});
