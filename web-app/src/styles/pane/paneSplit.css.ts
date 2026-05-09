import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars, breakpoints } from "@/styles/theme.css";

/**
 * Split container: CSS grid with a ratio-driven first column/row and a 6px handle column/row.
 * --split-ratio is set as an inline style at runtime (CSS custom property bridge).
 */
export const splitContainer = recipe({
  base: {
    display: "grid",
    width: "100%",
    height: "100%",
    overflow: "hidden",
  },
  variants: {
    direction: {
      vertical: {
        // left | handle | right
        gridTemplateColumns: "calc(var(--split-ratio, 0.5) * 100%) 6px 1fr",
        gridTemplateRows: "100%",
        "@media": {
          [`(max-width: ${breakpoints.md})`]: {
            // On mobile, vertical (side-by-side) splits stack vertically
            gridTemplateColumns: "1fr",
            gridTemplateRows: "var(--split-ratio-fr, 1fr) 6px 1fr",
          },
        },
      },
      horizontal: {
        // top | handle | bottom
        gridTemplateColumns: "100%",
        gridTemplateRows: "calc(var(--split-ratio, 0.5) * 100%) 6px 1fr",
      },
    },
  },
});

export const leafContainer = recipe({
  base: {
    display: "flex",
    flexDirection: "column",
    overflow: "hidden",
    minWidth: 0,
    minHeight: 0,
    position: "relative",
    background: vars.color.background,
    outline: "2px solid transparent",
    outlineOffset: "-2px",
    transition: "outline-color 120ms ease",
  },
  variants: {
    focused: {
      true: {
        outline: `2px solid ${vars.color.primary}`,
      },
      false: {
        outline: "2px solid transparent",
      },
    },
  },
  defaultVariants: { focused: false },
});

export const leafZoomed = style({
  position: "absolute",
  inset: 0,
  zIndex: 10,
  background: vars.color.background,
});

export const emptyPaneSlot = style({
  display: "flex",
  flex: 1,
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  gap: vars.space["2"],
  color: vars.color.textMuted,
  fontStyle: "italic",
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.sans,
  padding: vars.space["4"],
  textAlign: "center",
  background: vars.color.cardBackground,
});

export const paneBody = style({
  flex: 1,
  overflow: "hidden",
  minHeight: 0,
  display: "flex",
  flexDirection: "column",
});

export const sessionListScroll = style({
  flex: 1,
  overflowY: "auto",
  minHeight: 0,
});

export const resetLayoutBar = style({
  display: "flex",
  justifyContent: "flex-end",
  padding: `2px ${vars.space["1"]}`,
  flexShrink: 0,
  background: "transparent",
});

export const resetLayoutButton = style({
  fontSize: vars.fontSize.xs,
  padding: `2px ${vars.space["1"]}`,
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  cursor: "pointer",
  color: vars.color.textMuted,
});

export const rendererRoot = style({
  display: "flex",
  flexDirection: "column",
  flex: 1,
  overflow: "hidden",
  minHeight: 0,
});

export const rendererContent = style({
  flex: 1,
  overflow: "hidden",
  minHeight: 0,
  position: "relative",
});
