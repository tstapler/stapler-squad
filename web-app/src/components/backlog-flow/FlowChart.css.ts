import { style, styleVariants } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const figure = style({
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
  padding: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.primary,
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
