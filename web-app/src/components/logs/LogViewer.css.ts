import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  width: "100%",
  height: "100%",
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
  backgroundColor: vars.color.background,
});

/**
 * Session surface: condensed variant — no time range picker, tighter vertical space.
 * Used when source="session"; the tab panel provides a bounded height via flex layout.
 */
export const containerSession = style({
  width: "100%",
  height: "100%",
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
  minHeight: 0,
  backgroundColor: vars.color.background,
});
