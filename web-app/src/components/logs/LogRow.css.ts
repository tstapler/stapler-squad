import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";

// ---------------------------------------------------------------------------
// T5: Log-level color recipe — WCAG AA compliant hardcoded values.
// The theme does not have level-specific tokens, so we hardcode the palette
// here as documented in the plan (features.md §3).
// ---------------------------------------------------------------------------

export const levelBadge = recipe({
  base: {
    display: "inline-block",
    padding: "1px 6px",
    borderRadius: 3,
    fontSize: 11,
    fontWeight: 700,
    fontFamily: "monospace",
    letterSpacing: "0.05em",
    minWidth: 44,
    textAlign: "center",
  },
  variants: {
    level: {
      ERROR: { background: "#B91C1C", color: "#FFFFFF" },
      WARN: { background: "#D97706", color: "#1A1A1A" },
      INFO: { background: "#1D4ED8", color: "#FFFFFF" },
      DEBUG: { background: "#6B7280", color: "#FFFFFF" },
      TRACE: { background: "#4B5563", color: "#FFFFFF" },
      UNKNOWN: { background: "transparent", color: "inherit" },
    },
  },
  defaultVariants: { level: "UNKNOWN" },
});

export const rowTint = recipe({
  base: {
    // T1/T2: position:relative needed for absolute-positioned gutterAbsolute
    position: "relative",
    display: "flex",
    alignItems: "flex-start",
    borderBottom: "1px solid",
    borderBottomColor: "rgba(255,255,255,0.06)",
    cursor: "pointer",
    minHeight: 28,
    // T2: eliminate iOS 300ms tap delay at the row level
    touchAction: "manipulation",
  },
  variants: {
    level: {
      ERROR: { backgroundColor: "rgba(185,28,28,0.08)" },
      WARN: { backgroundColor: "rgba(217,119,6,0.06)" },
      INFO: { backgroundColor: "transparent" },
      DEBUG: { backgroundColor: "transparent" },
      TRACE: { backgroundColor: "transparent" },
      UNKNOWN: { backgroundColor: "transparent" },
    },
    isSelected: {
      true: { outline: "1px solid rgba(59,130,246,0.5)", outlineOffset: -1 },
      false: {},
    },
  },
  defaultVariants: { level: "UNKNOWN", isSelected: false },
});

// ---------------------------------------------------------------------------
// T1: Split-column layout — absolutely-positioned gutter that occludes
// scrolling text on iOS Safari where position:sticky inside overflow-x:auto
// is broken (iOS < 17). The gutter sits over the left edge of bodyScrollable.
// ---------------------------------------------------------------------------

/** JS constant for gutter width — must stay in sync with the CSS below. */
export const GUTTER_WIDTH_PX = 88;
/** Narrow-screen gutter width (≤ 380px) — hides line number, keeps badge. */
export const GUTTER_WIDTH_NARROW_PX = 44;

export const gutterAbsolute = style({
  // T1: Absolute over the left edge of the row — always visible on scroll
  position: "absolute",
  left: 0,
  top: 0,
  bottom: 0,
  width: GUTTER_WIDTH_PX,
  zIndex: 10,
  // Solid background occludes scrolling text beneath gutter
  backgroundColor: "var(--background, #111)",
  display: "flex",
  alignItems: "center",
  gap: 4,
  padding: "2px 6px",
  userSelect: "none",
  flexShrink: 0,
  // T2: manipulation eliminates iOS 300ms tap delay on gutter interactions
  touchAction: "manipulation",
  "@media": {
    "(max-width: 380px)": {
      width: GUTTER_WIDTH_NARROW_PX,
    },
  },
});

export const bodyScrollable = style({
  flex: 1,
  overflowX: "auto",
  // T1: padding-left matches gutter width so text starts after the gutter
  paddingLeft: GUTTER_WIDTH_PX,
  whiteSpace: "nowrap",
  fontFamily: "monospace",
  fontSize: 12,
  lineHeight: 1.5,
  // T2: pan-x pan-y allows horizontal scroll; prevents browser back-nav swipe
  touchAction: "pan-x pan-y",
  // Legacy iOS momentum scrolling
  WebkitOverflowScrolling: "touch",
  "@media": {
    "(max-width: 380px)": {
      paddingLeft: GUTTER_WIDTH_NARROW_PX,
    },
  },
});

// Keep legacy aliases so any remaining imports from Epic 3 still compile
/** @deprecated Use gutterAbsolute */
export const gutterCol = gutterAbsolute;
/** @deprecated Use bodyScrollable */
export const bodyCol = bodyScrollable;

export const lineNumber = style({
  color: "rgba(156,163,175,0.7)",
  fontSize: 10,
  minWidth: 28,
  textAlign: "right",
  // T1: hide line number on very narrow screens; keep level badge visible
  "@media": {
    "(max-width: 380px)": {
      display: "none",
    },
  },
});

// ---------------------------------------------------------------------------
// T4: Timestamp abbreviation — pure CSS responsive, no JS media query needed
// ---------------------------------------------------------------------------

export const timestampFull = style({
  color: "rgba(156,163,175,0.5)",
  marginRight: 8,
  fontSize: 11,
  "@media": {
    "(max-width: 430px)": {
      display: "none",
    },
  },
});

export const timestampShort = style({
  display: "none",
  color: "rgba(156,163,175,0.5)",
  marginRight: 8,
  fontSize: 11,
  "@media": {
    "(max-width: 430px)": {
      display: "inline",
    },
  },
});

export const mark = style({
  backgroundColor: "rgba(253,224,71,0.4)",
  color: "inherit",
  borderRadius: 2,
});

// Keep backward compat alias for any existing imports of `row`
export const row = rowTint({ level: "UNKNOWN", isSelected: false });
