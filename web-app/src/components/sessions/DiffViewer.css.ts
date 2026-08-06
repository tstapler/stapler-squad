import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const diffHeader = style({
  display: "flex",
  alignItems: "center",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
});
