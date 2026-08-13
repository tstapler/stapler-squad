import { recipe } from "@vanilla-extract/recipes";
import { vars, breakpoints } from "@/styles/theme.css";

/**
 * Three-column session cockpit grid.
 *
 * Col 1 — session list: 320px fixed
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
        gridTemplateRows: "1fr",
      },
    },
  },
  variants: {
    contextPanelOpen: {
      true: {
        gridTemplateColumns: "var(--list-col-width, 320px) 6px 1fr 320px",
        "@media": {
          [`(max-width: ${breakpoints.inner})`]: {
            gridTemplateColumns: "var(--list-col-width, 240px) 6px 1fr 320px",
          },
          [`(max-width: ${breakpoints.md})`]: {
            gridTemplateColumns: "1fr",
            gridTemplateRows: "1fr",
          },
        },
      },
      false: {
        // Use CSS custom property for resizable list column width (US-1)
        gridTemplateColumns: "var(--list-col-width, 320px) 6px 1fr",
        "@media": {
          [`(max-width: ${breakpoints.inner})`]: {
            gridTemplateColumns: "var(--list-col-width, 240px) 6px 1fr",
          },
          [`(max-width: ${breakpoints.md})`]: {
            gridTemplateColumns: "1fr",
            gridTemplateRows: "1fr",
          },
        },
      },
    },
  },
  defaultVariants: { contextPanelOpen: false },
});

export const sessionListColumn = recipe({
  base: {
    overflowY: "auto",
    overflowX: "hidden",
    borderRight: `1px solid ${vars.color.borderColor}`,
    display: "flex",
    flexDirection: "column",
  },
  variants: {
    sessionSelected: {
      true: {
        "@media": {
          [`(max-width: ${breakpoints.md})`]: {
            maxHeight: 0,
            overflow: "hidden",
            borderRight: "none",
            borderBottom: "none",
          },
        },
      },
      false: {
        "@media": {
          [`(max-width: ${breakpoints.md})`]: {
            borderRight: "none",
            borderBottom: `1px solid ${vars.color.borderColor}`,
            flex: 1,
            minHeight: 0,
            overflow: "auto",
          },
        },
      },
    },
  },
  defaultVariants: { sessionSelected: false },
});

export const detailColumn = recipe({
  base: {
    display: "flex",
    flexDirection: "column",
    overflow: "hidden",
    minWidth: 0,
  },
  variants: {
    sessionSelected: {
      true: {
        "@media": {
          [`(max-width: ${breakpoints.md})`]: {
            flex: 1,
            minHeight: 0,
          },
        },
      },
      false: {
        "@media": {
          [`(max-width: ${breakpoints.md})`]: {
            display: "none",
          },
        },
      },
    },
  },
  defaultVariants: { sessionSelected: false },
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
