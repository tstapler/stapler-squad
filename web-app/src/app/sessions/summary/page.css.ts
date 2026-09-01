import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const page = style({
  maxWidth: "900px",
  margin: "0 auto",
  padding: `${vars.space["4"]} ${vars.space["4"]} ${vars.space["8"]}`,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  // Root layout's `mainContent` sets overflow: hidden — this page must opt in
  // to scroll (per docs/reference/css-architecture.md's Page Scroll
  // Convention) or the markdown summary body, which can be arbitrarily long,
  // gets clipped with no scrollbar.
  height: "100%",
  overflowY: "auto",
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
