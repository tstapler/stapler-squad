import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const page = style({
  maxWidth: "900px",
  margin: "0 auto",
  padding: `${vars.space["4"]} ${vars.space["4"]} ${vars.space["8"]}`,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const backLink = style({
  alignSelf: "flex-start",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  textDecoration: "none",
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
    },
  },
});

export const title = style({
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.bold,
  color: vars.color.textPrimary,
  margin: 0,
});
