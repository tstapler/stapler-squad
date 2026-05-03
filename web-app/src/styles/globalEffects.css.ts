import { globalStyle } from "@vanilla-extract/css";
import { vars } from "./theme.css";

/**
 * Story 4.1 — Global scanline overlay.
 *
 * Rendered as a ::before pseudo-element on <html> so it sits above all content
 * without affecting layout. The effect is only visible when a theme sets
 * vars.color.scanlineColor to a non-transparent value (matrix / cyberpunk77).
 *
 * Wrapped in prefers-reduced-motion so it never triggers vestibular symptoms.
 */
globalStyle("html::before", {
  content: "''",
  position: "fixed",
  inset: 0,
  pointerEvents: "none",
  zIndex: 9999,
  backgroundImage: `repeating-linear-gradient(
    to bottom,
    ${vars.color.scanlineColor} 0px,
    ${vars.color.scanlineColor} 1px,
    transparent 1px,
    transparent 4px
  )`,
  "@media": {
    "(prefers-reduced-motion: reduce)": {
      display: "none",
    },
  },
});
