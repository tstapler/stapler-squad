import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  maxWidth: "32rem",
  margin: `${vars.space["8"]} auto`,
  padding: vars.space["4"],
  color: vars.color.textPrimary,
});
