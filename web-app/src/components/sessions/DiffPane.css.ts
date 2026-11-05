import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const pane = style({
  display: "flex",
  flexDirection: "column",
  height: "100%",
  minHeight: 0,
  selectors: {
    "&:focus-visible": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "-2px" },
  },
});
