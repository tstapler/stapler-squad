import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  flex: 1,
  minHeight: 0,
});

export const board = style({
  display: "flex",
  gap: vars.space["4"],
  overflowX: "auto",
  padding: vars.space["4"],
  flex: 1,
  minHeight: 0,
  alignItems: "stretch",
  "@media": {
    "(max-width: 768px)": {
      gap: vars.space["3"],
      padding: vars.space["3"],
      scrollSnapType: "x mandatory",
    },
  },
});

// Alternative to `board` used when a swimlane grouping strategy is active (Task 6.1.1a):
// swimlane rows stack vertically instead of columns laying out directly in a single row --
// each row's own inner flex-row (BoardSwimlane.css.ts's `swimlaneRow`) handles the horizontal
// column layout.
export const boardRows = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
  overflowY: "auto",
  padding: vars.space["4"],
  flex: 1,
  minHeight: 0,
});

export const boardHeader = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["3"]} ${vars.space["4"]} 0`,
});

export const boardHeaderSearch = style({
  flex: "1 1 220px",
  minWidth: "160px",
});

// Visually-hidden but screen-reader-audible region announcing drag/move outcomes.
export const liveRegion = style({
  position: "absolute",
  width: 1,
  height: 1,
  overflow: "hidden",
  clipPath: "inset(50%)",
  whiteSpace: "nowrap",
});

const toastBase = style({
  position: "fixed",
  bottom: vars.space["4"],
  left: "50%",
  transform: "translateX(-50%)",
  zIndex: zIndex.toast,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  boxShadow: vars.shadow.md,
});

export const toastError = style([
  toastBase,
  { background: vars.color.errorBg, color: vars.color.errorText },
]);

export const toastWarning = style([
  toastBase,
  { background: vars.color.warningBg, color: vars.color.warningText },
]);
