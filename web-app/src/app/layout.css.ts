import { style } from "@vanilla-extract/css";

export const mainContent = style({
  flex: 1,
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
  selectors: {
    // Programmatic focus target only (tabIndex -1, not in the Tab order); a pane-sized ring after every navigation would be noise.
    "&:focus, &:focus-visible": { outline: "none" },
  },
});
