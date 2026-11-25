import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

/** Centered modal on wide screens; the shared bottom-sheet look (mobilePaneTabStrip.css) applies at phone width. */
export const pickerSheetWide = style({
  "@media": {
    "(min-width: 601px)": {
      maxWidth: "560px",
      maxHeight: "70dvh",
      margin: "auto",
      borderRadius: vars.radii.md,
      border: `1px solid ${vars.color.modalBorder}`,
    },
  },
});

export const pickerTitle = style({
  margin: 0,
  padding: `${vars.space["3"]} ${vars.space["3"]} 0`,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const emptyPaneButton = style({
  marginTop: vars.space["1"],
  minHeight: "40px",
  padding: `0 ${vars.space["4"]}`,
  background: vars.color.primary,
  color: vars.color.onPrimaryFill,
  border: "none",
  borderRadius: vars.radii.md,
  fontFamily: vars.font.sans,
  fontSize: vars.fontSize.sm,
  fontStyle: "normal",
  cursor: "pointer",
  ":focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "2px" },
});
