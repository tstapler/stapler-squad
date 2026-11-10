import { style } from "@vanilla-extract/css";
import { vars, breakpoints } from "@/styles/theme.css";

export const pageRoot = style({
  display: "flex",
  flexDirection: "column",
  height: "100%",
  overflowY: "auto",
  overflowX: "hidden",
  maxWidth: "800px",
  margin: "0 auto",
  padding: `${vars.space[4]} ${vars.space[4]} 0`,
  paddingRight: `calc(var(--tray-clearance, 0px) + ${vars.space[4]})`,
  "@media": {
    [`screen and (max-width: ${breakpoints.md})`]: {
      padding: `${vars.space[2]} ${vars.space[2]} 0`,
      paddingRight: `calc(var(--tray-clearance, 0px) + ${vars.space[2]})`,
    },
  },
});
