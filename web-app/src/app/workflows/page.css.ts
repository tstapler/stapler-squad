import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const page = style({
  display: "flex",
  flexDirection: "column",
  minHeight: "calc(var(--viewport-height, 100dvh) - var(--header-height))",
  background: vars.color.background,
});

export const main = style({
  flex: 1,
  maxWidth: "1200px",
  width: "100%",
  margin: "0 auto",
  padding: "2rem",
  overflowY: "auto",
  display: "flex",
  flexDirection: "column",
  gap: "2rem",
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "1rem",
    },
  },
});
