import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars, breakpoints } from "@/styles/theme.css";

/**
 * Three-column session cockpit grid.
 *
 * Col 1 — session list: 280px fixed
 * Col 2 — terminal/detail: fills remaining space
 * Col 3 — context panel: 320px, slides in when open
 *
 * Story 2.3 mobile responsive:
 * - <= md (768px): single column stack. Session list shows at top (limited height),
 *   detail panel fills remaining space.
 * - <= inner (900px): session list narrows to 240px.
 */
export const cockpitGrid = recipe({
  base: {
    display: "grid",
    height: "100%",
    overflow: "hidden",
    "@media": {
      [`(max-width: ${breakpoints.md})`]: {
        // Mobile: vertical stack
        gridTemplateColumns: "1fr",
        gridTemplateRows: "auto 1fr",
      },
    },
  },
  variants: {
    contextPanelOpen: {
      true: {
        gridTemplateColumns: "280px 1fr 320px",
        "@media": {
          [`(max-width: ${breakpoints.inner})`]: {
            gridTemplateColumns: "240px 1fr 280px",
          },
          [`(max-width: ${breakpoints.md})`]: {
            gridTemplateColumns: "1fr",
            gridTemplateRows: "auto 1fr",
          },
        },
      },
      false: {
        gridTemplateColumns: "280px 1fr",
        "@media": {
          [`(max-width: ${breakpoints.inner})`]: {
            gridTemplateColumns: "240px 1fr",
          },
          [`(max-width: ${breakpoints.md})`]: {
            gridTemplateColumns: "1fr",
            gridTemplateRows: "auto 1fr",
          },
        },
      },
    },
  },
  defaultVariants: { contextPanelOpen: false },
});

export const sessionListColumn = style({
  overflowY: "auto",
  overflowX: "hidden",
  borderRight: `1px solid ${vars.color.borderColor}`,
  display: "flex",
  flexDirection: "column",
  "@media": {
    [`(max-width: ${breakpoints.md})`]: {
      borderRight: "none",
      borderBottom: `1px solid ${vars.color.borderColor}`,
      // On mobile with a session selected, collapse session list to a compact strip;
      // without a selection it fills available height.
      maxHeight: "40vh",
    },
  },
});

export const detailColumn = style({
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
  minWidth: 0,
  "@media": {
    [`(max-width: ${breakpoints.md})`]: {
      // Take remaining space on mobile
      flex: 1,
      minHeight: 0,
    },
  },
});

export const contextPanel = recipe({
  base: {
    display: "flex",
    flexDirection: "column",
    overflow: "hidden",
    borderLeft: `1px solid ${vars.color.borderColor}`,
    background: vars.color.cardBackground,
    transition: "transform 200ms ease",
    "@media": {
      "(prefers-reduced-motion: reduce)": {
        transition: "none",
      },
      [`(max-width: ${breakpoints.md})`]: {
        position: "fixed",
        bottom: 0,
        left: 0,
        right: 0,
        height: "50vh",
        borderLeft: "none",
        borderTop: `1px solid ${vars.color.borderColor}`,
        zIndex: 500,
      },
    },
  },
  variants: {
    open: {
      true: { transform: "translateX(0)" },
      false: { transform: "translateX(100%)" },
    },
  },
  defaultVariants: { open: false },
});
