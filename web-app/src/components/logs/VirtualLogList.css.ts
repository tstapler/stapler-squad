import { style } from "@vanilla-extract/css";

export const scrollContainer = style({
  height: "100%",
  overflow: "hidden",
  overscrollBehaviorY: "contain",
  // Prevent browser from changing scroll position during content insertion,
  // which would cause false "at bottom" detection on iOS.
  overflowAnchor: "none",
});

/**
 * Visually-hidden style for screen-reader-only elements.
 * Content is announced via aria-live but not visible on screen.
 */
export const srOnly = style({
  position: "absolute",
  width: 1,
  height: 1,
  overflow: "hidden",
  clip: "rect(0, 0, 0, 0)",
  whiteSpace: "nowrap",
  // pointerEvents: none so it doesn't interfere with click targets
  pointerEvents: "none",
});
