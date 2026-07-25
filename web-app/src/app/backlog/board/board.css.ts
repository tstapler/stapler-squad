import { style } from "@vanilla-extract/css";
import { detailPaneBase } from "@/styles/pane/detailPaneShell.css";

export const contentArea = style({
  display: "flex",
  flex: 1,
  minHeight: 0,
  overflow: "hidden",
});

export const detailPane = style([detailPaneBase, { width: "420px" }]);
