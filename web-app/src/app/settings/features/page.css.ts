import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// flex/minHeight/overflowY: the app shell (CockpitShell) wraps every page in
// nested `overflow: hidden` containers pinned to the viewport height — a page
// only scrolls if it opts in itself. Mirrors SessionDetail.css.ts's `content`.
export const container = style({
  maxWidth: "900px",
  margin: "0 auto",
  padding: "2rem 1.5rem",
  flex: 1,
  minHeight: 0,
  overflowY: "auto",
});

export const title = style({
  color: vars.color.textPrimary,
  fontSize: "1.5rem",
  fontWeight: 700,
  marginBottom: "0.5rem",
});

export const subtitle = style({
  color: vars.color.textSecondary,
  fontSize: "0.875rem",
  marginBottom: "2rem",
});

export const flagRow = style({
  display: "flex",
  alignItems: "flex-start",
  justifyContent: "space-between",
  gap: "1.5rem",
  padding: "1.25rem 0",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  selectors: {
    "&:last-child": { borderBottom: "none" },
  },
});

export const flagInfo = style({
  flex: 1,
  order: 1,
});

export const flagName = style({
  color: vars.color.textPrimary,
  fontWeight: 600,
  fontSize: "0.9375rem",
  textTransform: "capitalize",
  marginBottom: "0.25rem",
});

export const flagDescription = style({
  color: vars.color.textSecondary,
  fontSize: "0.8125rem",
  lineHeight: 1.4,
});

// The button is the 44x44 hit area; the visible track inside it stays 44x24.
export const toggle = style({
  order: 2,
  flexShrink: 0,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "44px",
  height: "44px",
  padding: 0,
  border: "none",
  background: "transparent",
  borderRadius: vars.radii.md,
  cursor: "pointer",
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
  selectors: {
    '&[aria-disabled="true"]': { cursor: "progress", opacity: 0.6 },
  },
});

export const toggleTrack = style({
  position: "relative",
  display: "block",
  width: "44px",
  height: "24px",
  borderRadius: "9999px",
  background: vars.color.borderColor,
  transition: "background 0.15s ease",
  selectors: {
    '[aria-checked="true"] > &': { background: vars.color.primary },
  },
});

export const toggleThumb = style({
  position: "absolute",
  top: "3px",
  left: "3px",
  width: "18px",
  height: "18px",
  borderRadius: "50%",
  background: "white",
  transition: "left 0.15s ease",
  selectors: {
    '[aria-checked="true"] > * > &': { left: "23px" },
  },
});

export const rowStatus = style({
  color: vars.color.textSecondary,
  fontSize: "0.8125rem",
  minHeight: "1.2em",
});

export const rowError = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: "0.5rem",
  marginTop: "0.5rem",
  color: vars.color.errorText,
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.md,
  padding: "0.5rem 0.75rem",
  fontSize: "0.8125rem",
});

export const retryButton = style({
  minHeight: "44px",
  padding: "0 0.75rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: "transparent",
  color: vars.color.textPrimary,
  fontSize: "0.8125rem",
  cursor: "pointer",
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

export const group = style({
  marginTop: "1.5rem",
});

export const groupHeading = style({
  color: vars.color.textPrimary,
  fontSize: "1.0625rem",
  fontWeight: 700,
  margin: 0,
  paddingBottom: "0.25rem",
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const nestedRow = style({
  marginLeft: "1.5rem",
  paddingLeft: "1rem",
  borderLeft: `2px solid ${vars.color.borderColor}`,
});

export const skeletonRow = style({
  height: "4.5rem",
  margin: "0.75rem 0",
  borderRadius: vars.radii.md,
  background: vars.color.surfaceSubtle,
});

export const badgeInert = style({
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
});

export const badge = style({
  display: "inline-block",
  padding: "0.125rem 0.5rem",
  borderRadius: "0.25rem",
  fontSize: "0.75rem",
  fontWeight: 600,
  marginLeft: "0.5rem",
  verticalAlign: "middle",
});

export const badgeEnabled = style({
  background: vars.color.successBg,
  color: vars.color.successText,
});

export const badgeDisabled = style({
  background: vars.color.borderColor,
  color: vars.color.textSecondary,
});

export const errorMessage = style({
  color: vars.color.errorText,
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.md,
  padding: "0.75rem 1rem",
  fontSize: "0.875rem",
});

export const emptyMessage = style({
  color: vars.color.textSecondary,
  fontSize: "0.875rem",
  padding: "1.5rem 0",
});

export const overrides = style({
  marginTop: "0.75rem",
});

export const overridesSummary = style({
  color: vars.color.textPrimary,
  fontSize: "0.8125rem",
  fontWeight: 600,
  cursor: "pointer",
  minHeight: "2.75rem",
  display: "flex",
  alignItems: "center",
});

export const overrideRow = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  justifyContent: "space-between",
  gap: "0.5rem 1rem",
  padding: "0.5rem 0",
});

export const overrideLabel = style({
  color: vars.color.textPrimary,
  fontSize: "0.8125rem",
  fontWeight: 600,
});

export const overrideSegments = style({
  display: "inline-flex",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  overflow: "hidden",
});

export const overrideSegment = style({
  minHeight: "2.75rem",
  minWidth: "2.75rem",
  padding: "0 0.75rem",
  border: "none",
  background: "transparent",
  color: vars.color.textPrimary,
  fontSize: "0.8125rem",
  cursor: "pointer",
});

export const overrideSegmentActive = style({
  background: vars.color.primary,
  color: "white",
});

// Deep-link target (UX-37): the offsets are the shell's own tokens, so sticky chrome never covers the row.
export const hashTargetRow = style({
  scrollMarginTop: "calc(var(--header-height) + 16px)",
  scrollMarginBottom: "calc(var(--bottom-nav-height) + 16px)",
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

export const engineIntro = style({
  color: vars.color.textSecondary,
  fontSize: "0.875rem",
  lineHeight: 1.4,
  margin: "0.75rem 0 0.25rem",
});

export const engineSummary = style({
  color: vars.color.textPrimary,
  fontSize: "0.9375rem",
  fontWeight: 600,
  margin: "0.5rem 0 0.25rem",
});

export const engineReviewOverdue = style({
  color: vars.color.warningText,
  background: vars.color.warningBg,
  border: `1px solid ${vars.color.warning}`,
  borderRadius: vars.radii.md,
  padding: "0.5rem 0.75rem",
  fontSize: "0.8125rem",
  margin: "0.25rem 0",
});

export const rowControls = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: "0.5rem 0.75rem",
  marginTop: "0.5rem",
});

export const rowControlButton = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minHeight: "44px",
  padding: "0 0.75rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: "transparent",
  color: vars.color.textPrimary,
  fontSize: "0.8125rem",
  textDecoration: "none",
  cursor: "pointer",
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
  selectors: {
    '&[aria-disabled="true"]': { cursor: "progress", opacity: 0.6 },
  },
});

export const srOnly = style({
  position: "absolute",
  width: "1px",
  height: "1px",
  margin: "-1px",
  padding: 0,
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
  whiteSpace: "nowrap",
  border: 0,
});

// Reserves three lines so evidence arriving causes no jump (UX-44).
export const evidenceRegion = style({
  minHeight: "5.25rem",
  margin: "0.75rem 0",
  padding: "0.5rem 0.75rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  lineHeight: 1.5,
});

export const evidenceNotClean = style({
  color: vars.color.warningText,
  fontWeight: 600,
});

export const ackLabel = style({
  display: "flex",
  alignItems: "center",
  gap: "0.75rem",
  width: "100%",
  minHeight: "44px",
  margin: "0.5rem 0",
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  cursor: "pointer",
});

export const ackCheckbox = style({
  width: "1.25rem",
  height: "1.25rem",
  flexShrink: 0,
});

export const dialogNote = style({
  color: vars.color.textSecondary,
  fontSize: "0.8125rem",
  lineHeight: 1.4,
  margin: "0.5rem 0",
});

export const resetButton = style({
  minHeight: "2.75rem",
  padding: "0 0.75rem",
  marginTop: "0.5rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: "transparent",
  color: vars.color.textPrimary,
  fontSize: "0.8125rem",
  cursor: "pointer",
});
