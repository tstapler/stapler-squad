import { style } from "@vanilla-extract/css";

// ---------------------------------------------------------------------------
// T3: LogViewerToolbar — collapsible search bar for narrow screens (< 430px)
// ---------------------------------------------------------------------------

export const toolbar = style({
  display: "flex",
  flexDirection: "column",
  gap: 4,
  padding: "6px 8px",
  borderBottom: "1px solid rgba(255,255,255,0.08)",
  flexShrink: 0,
});

export const toolbarRow = style({
  display: "flex",
  alignItems: "center",
  gap: 6,
});

export const searchWrapper = style({
  position: "relative",
  flex: 1,
});

export const searchInput = style({
  width: "100%",
  boxSizing: "border-box",
  minHeight: 36,
  padding: "4px 32px 4px 10px",
  borderRadius: 6,
  border: "1px solid rgba(255,255,255,0.2)",
  background: "rgba(255,255,255,0.05)",
  color: "inherit",
  fontSize: 13,
  fontFamily: "monospace",
  outline: "none",
  selectors: {
    "&:focus": {
      borderColor: "rgba(59,130,246,0.6)",
      boxShadow: "0 0 0 2px rgba(59,130,246,0.2)",
    },
  },
});

export const matchCounter = style({
  position: "absolute",
  right: 8,
  top: "50%",
  transform: "translateY(-50%)",
  fontSize: 10,
  color: "rgba(156,163,175,0.7)",
  pointerEvents: "none",
  whiteSpace: "nowrap",
});

/**
 * T3: Magnifying-glass icon button — shown on narrow screens (< 431px) when
 * search is collapsed. Hidden on wide screens via CSS.
 */
export const searchIconButton = style({
  minWidth: 44,
  minHeight: 44,
  display: "none",
  alignItems: "center",
  justifyContent: "center",
  border: "none",
  background: "transparent",
  cursor: "pointer",
  color: "inherit",
  // T2: eliminate iOS 300ms tap delay
  touchAction: "manipulation",
  fontSize: 18,
  flexShrink: 0,
  "@media": {
    "(max-width: 430px)": {
      display: "flex",
    },
  },
});

/**
 * T3: Wrapper for the search input row.
 * On wide screens (≥ 431px): always visible as a flex row.
 * On narrow screens (< 430px): hidden by default, shown when JS adds
 * the `searchExpanded` data attribute to the toolbar.
 * We use a data attribute so vanilla-extract can generate a selector.
 */
export const searchExpandableRow = style({
  display: "flex",
  flex: 1,
  // On narrow screens: hidden unless expanded
  "@media": {
    "(max-width: 430px)": {
      display: "none",
    },
  },
});

/**
 * T3: When search is expanded on narrow screens, show the expandable row as
 * a full-width row below the chips. Applied via JS className toggle.
 */
export const searchExpandableRowOpen = style({
  "@media": {
    "(max-width: 430px)": {
      display: "flex",
      width: "100%",
    },
  },
});

/**
 * Live-tail toggle button — always visible in the toolbar row.
 * Green dot when live, muted when paused. min-height 44px for mobile tap target.
 */
export const liveTailButton = style({
  display: "flex",
  alignItems: "center",
  gap: 5,
  minHeight: 44,
  padding: "0 10px",
  border: "1px solid rgba(255,255,255,0.15)",
  borderRadius: 6,
  background: "transparent",
  cursor: "pointer",
  color: "inherit",
  fontSize: 12,
  fontWeight: 600,
  flexShrink: 0,
  touchAction: "manipulation",
  whiteSpace: "nowrap",
  selectors: {
    "&:focus-visible": {
      outline: "2px solid rgba(59,130,246,0.6)",
      outlineOffset: 2,
    },
  },
});

export const liveTailDot = style({
  width: 8,
  height: 8,
  borderRadius: "50%",
  background: "rgba(156,163,175,0.5)",
  flexShrink: 0,
  selectors: {
    "[data-live='true'] &": {
      background: "#22c55e",
      boxShadow: "0 0 4px #22c55e",
    },
  },
});

export const searchDoneButton = style({
  minWidth: 44,
  minHeight: 44,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  border: "none",
  background: "transparent",
  cursor: "pointer",
  color: "rgba(59,130,246,0.9)",
  touchAction: "manipulation",
  fontSize: 13,
  fontWeight: 600,
  flexShrink: 0,
  padding: "0 4px",
});
