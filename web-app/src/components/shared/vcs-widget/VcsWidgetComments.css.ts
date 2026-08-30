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

export const comment = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[1],
});

export const commentMeta = style({
  display: "flex",
  alignItems: "baseline",
  gap: vars.space[2],
});

export const author = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const timestamp = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

export const body = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  whiteSpace: "pre-wrap",
});

export const viewLink = style({
  display: "inline-flex",
  alignSelf: "flex-start",
  fontSize: vars.fontSize.xs,
  color: vars.color.primary,
  textDecoration: "none",
  selectors: {
    "&:hover": { textDecoration: "underline" },
  },
});

export const status = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});
