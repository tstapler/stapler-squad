import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const swimlane = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const swimlaneLabel = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const swimlaneRow = style({
  display: "flex",
  gap: vars.space["4"],
  overflowX: "auto",
  alignItems: "stretch",
  "@media": {
    "(max-width: 768px)": {
      gap: vars.space["3"],
      scrollSnapType: "x mandatory",
    },
  },
});
