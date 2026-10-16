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

export const toggle = style({
  flexShrink: 0,
  width: "2.75rem",
  height: "1.5rem",
  borderRadius: "9999px",
  border: "none",
  cursor: "pointer",
  transition: "background 0.15s ease",
  position: "relative",
});

export const toggleThumb = style({
  position: "absolute",
  top: "0.1875rem",
  width: "1.125rem",
  height: "1.125rem",
  borderRadius: "50%",
  background: "white",
  transition: "left 0.15s ease",
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
  background: vars.color.success,
  color: "white",
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
