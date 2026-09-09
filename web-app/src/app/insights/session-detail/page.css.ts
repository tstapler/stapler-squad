// +feature: insights-session-detail-route
import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const main = style({
  maxWidth: 900,
  margin: "0 auto",
  padding: vars.space[4],
});

export const backLink = style({
  display: "inline-block",
  marginBottom: vars.space[3],
  textDecoration: "none",
});

export const heading = style({
  outline: "none",
});
