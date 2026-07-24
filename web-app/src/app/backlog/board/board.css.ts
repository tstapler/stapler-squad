import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const contentArea = style({
  display: "flex",
  flex: 1,
  minHeight: 0,
  overflow: "hidden",
});

export const detailPane = style({
  width: "420px",
  minWidth: "240px",
  maxWidth: "800px",
  borderLeft: `1px solid ${vars.color.borderColor}`,
  flexShrink: 0,
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
  "@media": {
    "(max-width: 768px)": {
      position: "fixed",
      inset: 0,
      width: "100% !important" as "inherit",
      zIndex: "500",
      background: vars.color.modalBackground,
    },
  },
});
