import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  maxWidth: "900px",
  margin: "0 auto",
  padding: "2rem 1.5rem",
});

export const title = style({
  color: vars.color.textPrimary,
  fontSize: "1.5rem",
  fontWeight: 700,
  marginBottom: "1.5rem",
});

export const sections = style({
  display: "flex",
  flexDirection: "column",
  gap: "2rem",
});

export const section = style({
  borderBottom: `1px solid ${vars.color.borderColor}`,
  paddingBottom: "2rem",
  selectors: {
    "&:last-child": {
      borderBottom: "none",
      paddingBottom: 0,
    },
  },
});
