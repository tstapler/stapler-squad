import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const badge = style({
  display: "inline-block",
  fontSize: "0.75rem",
  fontWeight: 400,
  color: vars.color.textMuted,
  fontStyle: "italic",
  marginLeft: "0.5rem",
});
