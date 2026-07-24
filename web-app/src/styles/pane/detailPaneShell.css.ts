import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

// Shared by backlog/backlog.css.ts and backlog/board/board.css.ts — both pages
// render the same slide-over item detail pane, mobile full-screen overlay
// included. Width is set per-consumer (list page: inline style, resizable;
// board page: fixed) via style([detailPaneBase, { width: ... }]) composition.
export const detailPaneBase = style({
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
      zIndex: zIndex.dropdown,
      background: vars.color.modalBackground,
    },
  },
});
