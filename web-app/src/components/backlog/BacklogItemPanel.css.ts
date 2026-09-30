import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const slideInFromRight = keyframes({
  from: { transform: "translateX(100%)", opacity: 0 },
  to: { transform: "translateX(0)", opacity: 1 },
});

export const panel = style({
  display: "flex",
  flexDirection: "column",
  height: "100%",
  borderLeft: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  overflow: "hidden",
});

export const toggle = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  gap: vars.space["2"],
  width: "32px",
  minWidth: "32px",
  height: "32px",
  minHeight: "32px",
  padding: 0,
  border: "none",
  background: "transparent",
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: vars.fontSize.xs,
  fontWeight: "600",
  transition: "color 0.2s, background 0.2s",
  borderRight: `1px solid ${vars.color.borderColor}`,
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
      background: vars.color.hoverBackground,
    },
  },
});

export const toggleIcon = style({
  fontSize: "0.875rem",
  lineHeight: 1,
  display: "flex",
  alignItems: "center",
});

export const toggleLabel = style({
  fontSize: "0.625rem",
  fontWeight: "600",
  color: vars.color.textMuted,
  letterSpacing: "0.05em",
});

export const content = style({
  flex: 1,
  minHeight: 0,
  overflowY: "auto",
  display: "flex",
  flexDirection: "column",
  padding: vars.space["3"],
  gap: vars.space["3"],
  animation: `${slideInFromRight} 0.2s ease-out`,
});

export const loading = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});

export const error = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  color: vars.color.error,
  fontSize: vars.fontSize.sm,
});

export const header = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexWrap: "wrap",
});

export const priorityBadge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "28px",
  height: "24px",
  padding: `0 ${vars.space["1"]}`,
  borderRadius: vars.radii.sm,
  background: vars.color.surfaceSubtle,
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  fontWeight: "600",
  border: `1px solid ${vars.color.borderColor}`,
  whiteSpace: "nowrap",
});

export const statusChip = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `0.25rem ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  background: vars.color.primary,
  color: vars.color.primaryText,
  fontSize: vars.fontSize.xs,
  fontWeight: "600",
  whiteSpace: "nowrap",
});

// Sweep fix (backlog-event-driven-updates Phase 5 compliance sweep, 2026-07-22):
// Story 5.4.1's own acceptance criterion ("a verdict recorded shortly after
// ... BacklogItemPanel reflects the new status and verdict") and ux.md UX
// AC #14 both require the verdict to appear here, mirroring
// BacklogItemCard.css.ts's verdict badge variants (same token usage, no
// hardcoded colors per docs/reference/css-architecture.md).
const verdictBadgeBase = style({
  display: "inline-flex",
  alignItems: "center",
  borderRadius: vars.radii.sm,
  padding: `0 ${vars.space["1"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: "700",
  fontFamily: vars.font.mono,
  height: "20px",
  border: "1px solid transparent",
});

export const verdictBadgePass = style([
  verdictBadgeBase,
  { color: vars.color.success, background: vars.color.successBg, borderColor: vars.color.success },
]);

export const verdictBadgePartial = style([
  verdictBadgeBase,
  { color: vars.color.warning, background: vars.color.warningBg, borderColor: vars.color.warning },
]);

export const verdictBadgeFail = style([
  verdictBadgeBase,
  { color: vars.color.errorText, background: vars.color.errorBg, borderColor: vars.color.error },
]);

export const verdictBadgeUnverifiable = style([
  verdictBadgeBase,
  { color: vars.color.textMuted, background: vars.color.cardBackground, borderColor: vars.color.borderMuted },
]);

export const verdictSummary = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  lineHeight: 1.4,
});

export const title = style({
  fontSize: vars.fontSize.sm,
  fontWeight: "600",
  color: vars.color.textPrimary,
  lineHeight: 1.4,
  textDecoration: "none",
  cursor: "pointer",
  transition: "color 0.2s",
  selectors: {
    "&:hover": {
      color: vars.color.primary,
      textDecoration: "underline",
    },
  },
});

export const criteriaSection = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const criteriaHeader = style({
  fontSize: vars.fontSize.xs,
  fontWeight: "600",
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
});

export const criteriaList = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const criterionRow = style({
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space["2"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  lineHeight: 1.4,
});

export const criterionDone = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "16px",
  height: "16px",
  fontSize: "0.75rem",
  color: vars.color.success,
  fontWeight: "600",
  flexShrink: 0,
  marginTop: "2px",
});

export const criterionPending = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "16px",
  height: "16px",
  fontSize: "0.75rem",
  color: vars.color.textMuted,
  flexShrink: 0,
  marginTop: "2px",
});

export const criterionText = style({
  flex: 1,
});

export const actions = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  marginTop: "auto",
  paddingTop: vars.space["3"],
  borderTop: `1px solid ${vars.color.borderColor}`,
});

export const actionLink = style({
  fontSize: vars.fontSize.sm,
  fontWeight: "500",
  color: vars.color.primary,
  textDecoration: "none",
  cursor: "pointer",
  transition: "color 0.2s",
  selectors: {
    "&:hover": {
      color: vars.color.primaryDark,
      textDecoration: "underline",
    },
  },
});
