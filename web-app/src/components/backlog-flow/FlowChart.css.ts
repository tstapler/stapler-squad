import { style, styleVariants } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// The visually hidden data table is absolutely positioned and a table ignores width:1px, so without this
// containing block it widens the page past a 320px viewport.
export const figure = style({
  position: "relative",
  overflowX: "clip",
  margin: 0,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const caption = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const plot = style({
  display: "block",
  maxWidth: "100%",
});

export const empty = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
});

export const note = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.warningText,
});

export const legend = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["3"],
  margin: 0,
  padding: 0,
  listStyle: "none",
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const legendItem = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
});

export const toggle = style({
  alignSelf: "flex-start",
  display: "inline-flex",
  alignItems: "center",
  minHeight: "24px",
  padding: 0,
  fontSize: vars.fontSize.sm,
  color: `color-mix(in srgb, ${vars.color.primary} 85%, ${vars.color.textPrimary})`,
  background: "none",
  border: "none",
  textDecoration: "underline",
  cursor: "pointer",
});

// Same hiding as the shared sr-only style, so the table stays in the accessibility tree.
export const tableHidden = style({
  position: "absolute",
  width: "1px",
  height: "1px",
  padding: 0,
  margin: "-1px",
  overflow: "hidden",
  clipPath: "inset(50%)",
  whiteSpace: "nowrap",
  borderWidth: 0,
  selectors: {
    // A drill button inside must be visible when it has keyboard focus (WCAG 2.4.7).
    "&:focus-within": {
      position: "static",
      width: "auto",
      height: "auto",
      margin: 0,
      overflow: "visible",
      clipPath: "none",
      whiteSpace: "normal",
    },
  },
});

export const table = style({
  borderCollapse: "collapse",
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

export const cell = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  border: `1px solid ${vars.color.borderColor}`,
  textAlign: "left",
});

// One token per series slot; the shape or pattern, not the hue, is what tells series apart.
const SERIES_TOKENS = {
  0: vars.color.primary,
  1: vars.color.warning,
  2: vars.color.success,
  3: vars.color.error,
  4: vars.color.critical,
};

export const seriesStroke = styleVariants(SERIES_TOKENS, (token) => ({
  stroke: token,
  fill: "none",
  strokeWidth: 2,
}));

export const seriesFill = styleVariants(SERIES_TOKENS, (token) => ({ fill: token }));

export const patternStroke = styleVariants(SERIES_TOKENS, (token) => ({ stroke: token, strokeWidth: 1.5 }));

export const bar = style({
  stroke: vars.color.background,
  strokeWidth: 0.5,
  transition: "height 0.2s ease, y 0.2s ease",
  "@media": {
    "(prefers-reduced-motion: reduce)": { transition: "none" },
  },
});

export const axis = style({
  stroke: vars.color.borderStrong,
  strokeWidth: 1,
});

/** A clickable chart point; pointer-only, the data table carries the keyboard path. */
export const drillPoint = style({ cursor: "pointer" });
