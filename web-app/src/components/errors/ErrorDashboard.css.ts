import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  height: "var(--viewport-height, 100dvh)",
  overflow: "hidden",
  padding: vars.space["4"],
  backgroundColor: vars.color.background,
  color: vars.color.textPrimary,
  fontFamily: vars.font.sans,
});

export const header = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: vars.space["4"],
  flexWrap: "wrap",
  gap: vars.space["2"],
});

export const title = style({
  fontSize: vars.fontSize.xl,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  margin: 0,
});

export const headerActions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const filterRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  marginBottom: vars.space["3"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  backgroundColor: vars.color.cardBackground,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
});

export const filterLabel = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["1"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  cursor: "pointer",
  userSelect: "none",
});

export const filterCheckbox = style({
  cursor: "pointer",
  accentColor: vars.color.primary,
});

export const countBadge = style({
  marginLeft: "auto",
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  backgroundColor: vars.color.hoverBackground,
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.full,
});

export const tableWrapper = style({
  flex: 1,
  overflowY: "auto",
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
});

export const table = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: vars.fontSize.sm,
});

export const thead = style({
  position: "sticky",
  top: 0,
  zIndex: 1,
  backgroundColor: vars.color.cardBackground,
});

export const th = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  textAlign: "left",
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  whiteSpace: "nowrap",
});

export const tr = style({
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  selectors: {
    "&:last-child": {
      borderBottom: "none",
    },
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const trAcknowledged = style({
  opacity: 0.5,
});

export const td = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  verticalAlign: "top",
  color: vars.color.textPrimary,
});

export const errorType = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.errorText,
  backgroundColor: vars.color.errorBg,
  padding: `2px ${vars.space["1"]}`,
  borderRadius: vars.radii.sm,
  whiteSpace: "nowrap",
  maxWidth: "200px",
  overflow: "hidden",
  textOverflow: "ellipsis",
  display: "inline-block",
});

export const message = style({
  color: vars.color.textPrimary,
  maxWidth: "300px",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const count = style({
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.warning,
  fontVariantNumeric: "tabular-nums",
  whiteSpace: "nowrap",
});

export const timestamp = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  whiteSpace: "nowrap",
  fontVariantNumeric: "tabular-nums",
});

export const procedure = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  maxWidth: "200px",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const statusBadge = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  whiteSpace: "nowrap",
});

export const statusActive = style({
  backgroundColor: vars.color.errorBg,
  color: vars.color.errorText,
});

export const statusAcknowledged = style({
  backgroundColor: vars.color.successBg,
  color: vars.color.success,
});

export const actionCell = style({
  whiteSpace: "nowrap",
});

export const expandButton = style({
  background: "none",
  border: "none",
  color: vars.color.textMuted,
  cursor: "pointer",
  padding: `0 ${vars.space["1"]}`,
  fontSize: vars.fontSize.base,
  lineHeight: 1,
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
    },
  },
});

export const acknowledgeButton = style({
  padding: `2px ${vars.space["2"]}`,
  backgroundColor: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.sm,
  cursor: "pointer",
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.primaryHover,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const refreshButton = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textSecondary,
  cursor: "pointer",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const stackTraceRow = style({
  backgroundColor: vars.color.cardBackground,
});

export const stackTraceCell = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  paddingTop: 0,
});

export const stackTrace = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  whiteSpace: "pre-wrap",
  wordBreak: "break-all",
  backgroundColor: vars.color.background,
  border: `1px solid ${vars.color.borderSubtle}`,
  borderRadius: vars.radii.sm,
  padding: vars.space["2"],
  maxHeight: "200px",
  overflowY: "auto",
  margin: 0,
});

export const emptyState = style({
  padding: vars.space["8"],
  textAlign: "center",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});

export const errorState = style({
  padding: vars.space["4"],
  color: vars.color.errorText,
  backgroundColor: vars.color.errorBg,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  marginBottom: vars.space["3"],
});

export const loadingState = style({
  padding: vars.space["8"],
  textAlign: "center",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});
