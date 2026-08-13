import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const page = style({
  minHeight: "var(--viewport-height, 100dvh)",
  background: vars.color.background,
  padding: "2rem",
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "1rem",
    },
  },
});

export const container = style({
  maxWidth: "900px",
  margin: "0 auto",
});

export const header = style({
  textAlign: "center",
  marginBottom: "3rem",
  "@media": {
    "screen and (max-width: 768px)": {
      marginBottom: "2rem",
    },
  },
});
