import { style } from "@vanilla-extract/css";

/** Custom property the adapter sets to park the textarea on the cursor row (the bridge for a runtime value). */
export const CURSOR_ROW_VAR = "--mobile-input-cursor-row";

/**
 * Visually hidden but focusable: 1px, transparent, no pointer events. Never `display:none`,
 * `visibility:hidden` or `aria-hidden`, which would stop the soft keyboard and TalkBack seeing it.
 */
export const textarea = style({
  position: "absolute",
  top: `var(${CURSOR_ROW_VAR}, 0px)`,
  left: 0,
  width: "1px",
  height: "1px",
  margin: 0,
  padding: 0,
  border: 0,
  overflow: "hidden",
  resize: "none",
  opacity: 0,
  pointerEvents: "none",
  outline: "none",
  caretColor: "transparent",
  background: "transparent",
  // 16px stops mobile browsers zooming the page when the field takes focus.
  fontSize: "16px",
});
