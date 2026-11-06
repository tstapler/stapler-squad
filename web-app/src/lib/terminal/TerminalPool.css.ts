import { style } from "@vanilla-extract/css";

// Off-screen parking area for pooled-but-not-docked terminals (Task 3.1).
// Sized close to a typical terminal pane (rather than 0x0) so XtermTerminal's
// mount-time fit() computes reasonable cols/rows before an entry is ever
// docked -- the real fit always happens on dock via requestAnimationFrame
// (Task 3.5), so this size only affects the very first frame.
export const graveyard = style({
  position: "fixed",
  top: 0,
  left: "-9999px",
  width: "800px",
  height: "600px",
  visibility: "hidden",
  pointerEvents: "none",
  overflow: "hidden",
});

// Applied to each pool entry's host <div>. Fills whichever parent it is
// currently appended to (the graveyard, or a consumer's dock anchor) --
// docking/undocking is a plain DOM appendChild, never a style change, so
// this class never needs to change.
export const host = style({
  width: "100%",
  height: "100%",
});

// Applied by a consumer (e.g. TerminalOutput.tsx) to the anchor <div> a
// pooled terminal docks into. Same sizing as `host` above -- kept as a
// separate export so callers aren't coupled to the pool's own internal
// naming, and so this lives here rather than in each consumer's own
// stylesheet (TerminalOutput.css.ts carries unrelated pre-existing lint
// debt that a plain-content export shouldn't have to sit next to).
export const dockAnchor = style({
  width: "100%",
  height: "100%",
});
