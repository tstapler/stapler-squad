import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  height: "100%",
  background: vars.color.modalBackground,
  overflow: "hidden",
});

export const scrollArea = style({
  flex: 1,
  overflowY: "auto",
  padding: `${vars.space["4"]} ${vars.space["4"]}`,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const errorBanner = style({
  display: "flex",
  alignItems: "flex-start",
  justifyContent: "space-between",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.errorBg,
  color: vars.color.error,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  border: `1px solid ${vars.color.errorDark}`,
});

export const errorBannerDismiss = style({
  background: "transparent",
  border: "none",
  color: vars.color.error,
  cursor: "pointer",
  fontSize: vars.fontSize.base,
  lineHeight: "1",
  padding: 0,
  flexShrink: 0,
});

export const header = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  flexShrink: 0,
  padding: `${vars.space["4"]} ${vars.space["6"]}`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
});

export const headerRow = style({
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space["3"],
  justifyContent: "space-between",
});

export const titleGroup = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  flex: 1,
});

export const itemTitle = style({
  fontSize: vars.fontSize.xl,
  fontWeight: vars.fontWeight.bold,
  color: vars.color.textPrimary,
  lineHeight: "1.3",
});

export const metaRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexWrap: "wrap",
});

export const statusBadge = style({
  display: "inline-flex",
  alignItems: "center",
  borderRadius: vars.radii.sm,
  padding: `2px ${vars.space["2"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  fontFamily: vars.font.mono,
});

export const statusIdea = style({
  background: vars.color.surfaceMuted,
  color: vars.color.textMuted,
  border: `1px solid ${vars.color.borderMuted}`,
});
export const statusReady = style({
  background: vars.statusBadge.inputBg,
  color: vars.statusBadge.inputFg,
  border: `1px solid ${vars.statusBadge.inputBorder}`,
});
export const statusInProgress = style({
  background: vars.statusBadge.uncommittedBg,
  color: vars.statusBadge.uncommittedFg,
  border: `1px solid ${vars.statusBadge.uncommittedBorder}`,
});
export const statusReview = style({
  background: vars.statusBadge.approvalBg,
  color: vars.statusBadge.approvalFg,
  border: `1px solid ${vars.statusBadge.approvalBorder}`,
});
export const statusDone = style({
  background: vars.statusBadge.completeBg,
  color: vars.statusBadge.completeFg,
  border: `1px solid ${vars.statusBadge.completeBorder}`,
});
export const statusArchived = style({
  background: vars.color.surfaceMuted,
  color: vars.color.textDisabled,
  border: `1px solid ${vars.color.borderMuted}`,
});
export const statusRefining = style({
  background: vars.color.warningBg,
  color: vars.color.warningText,
  border: `1px solid ${vars.color.warning}`,
});

export const priorityBadge = style({
  display: "inline-flex",
  alignItems: "center",
  borderRadius: vars.radii.sm,
  padding: `2px ${vars.space["2"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.bold,
  fontFamily: vars.font.mono,
  background: vars.color.accentBg,
  color: vars.color.primary,
  border: `1px solid ${vars.color.borderMuted}`,
});

export const dateMeta = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  fontFamily: vars.font.mono,
});

export const headerActions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexShrink: 0,
});

export const editButton = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderMuted}`,
  background: "transparent",
  color: vars.color.textSecondary,
  cursor: "pointer",
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  flexShrink: 0,
  ":hover": {
    background: vars.color.hoverBackground,
    borderColor: vars.color.borderStrong,
    color: vars.color.textPrimary,
  },
});

export const editFormWrapper = style({
  padding: vars.space["6"],
});

export const editFormHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  marginBottom: vars.space["4"],
  paddingBottom: vars.space["3"],
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
});

export const editFormTitle = style({
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const closeButton = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  width: "32px",
  height: "32px",
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderMuted}`,
  background: "transparent",
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: vars.fontSize.lg,
  flexShrink: 0,
  ":hover": {
    background: vars.color.hoverBackground,
    color: vars.color.textPrimary,
  },
});

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const sectionTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  fontFamily: vars.font.mono,
  paddingBottom: vars.space["1"],
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
});

export const description = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  lineHeight: "1.6",
  whiteSpace: "pre-wrap",
});

export const emptyText = style({
  color: vars.color.textMuted,
  fontStyle: "italic",
  fontSize: vars.fontSize.sm,
});

export const actionsPanel = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["2"],
});

export const actionButton = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderMuted}`,
  background: vars.color.accentBg,
  color: vars.color.primary,
  transition: "background 0.1s ease, border-color 0.1s ease",
  ":hover": {
    background: vars.color.accentHover,
    borderColor: vars.color.primary,
  },
  ":disabled": {
    opacity: 0.4,
    cursor: "not-allowed",
  },
});

export const actionButtonDanger = style({
  background: vars.color.errorBg,
  color: vars.color.error,
  borderColor: vars.color.errorDark,
  ":hover": {
    background: vars.color.errorBg,
    borderColor: vars.color.error,
  },
});

export const actionButtonSuccess = style({
  background: vars.statusBadge.completeBg,
  color: vars.statusBadge.completeFg,
  borderColor: vars.statusBadge.completeBorder,
  ":hover": {
    background: vars.statusBadge.completeBg,
    borderColor: vars.statusBadge.completeFg,
  },
});

export const sessionList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const sessionRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
});

export const sessionLink = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  flex: 1,
  minWidth: 0,
  textDecoration: "none",
  color: "inherit",
});

export const sessionDeleteBtn = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.errorDark}`,
  background: vars.color.errorBg,
  color: vars.color.error,
  cursor: "pointer",
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  flexShrink: 0,
  ":hover": {
    borderColor: vars.color.error,
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
});

export const sessionId = style({
  fontFamily: vars.font.mono,
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.xs,
  flex: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const sessionRole = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  textTransform: "capitalize",
});

export const sessionDate = style({
  color: vars.color.textDisabled,
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
});

export const notesTextarea = style({
  width: "100%",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.sans,
  outline: "none",
  resize: "vertical",
  minHeight: "80px",
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
  },
  boxSizing: "border-box",
});

export const inlineEditActions = style({
  display: "flex",
  gap: vars.space["2"],
  marginTop: vars.space["1"],
});

export const saveNotesButton = style({
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  cursor: "pointer",
  ":hover": { background: vars.color.primaryHover },
});

export const cancelNotesButton = style({
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  background: "transparent",
  color: vars.color.textMuted,
  border: `1px solid ${vars.color.borderMuted}`,
  cursor: "pointer",
  ":hover": { background: vars.color.hoverBackground },
});

export const artifactsPath = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  background: vars.color.surfaceMuted,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  wordBreak: "break-all",
});

export const workflowTimeline = style({
  display: "flex",
  flexDirection: "column",
  gap: 0,
  paddingLeft: vars.space["2"],
  borderLeft: `2px solid ${vars.color.borderSubtle}`,
});

export const workflowEvent = style({
  display: "flex",
  alignItems: "baseline",
  gap: vars.space["3"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  position: "relative",
  fontSize: vars.fontSize.sm,
  "::before": {
    content: '""',
    position: "absolute",
    left: `calc(-1 * ${vars.space["2"]} - 1px)`,
    top: "50%",
    transform: "translateY(-50%)",
    width: "8px",
    height: "8px",
    borderRadius: "50%",
    background: vars.color.borderSubtle,
    border: `2px solid ${vars.color.modalBackground}`,
  },
});

export const workflowEventFrom = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  flexShrink: 0,
});

export const workflowEventArrow = style({
  color: vars.color.textMuted,
  flexShrink: 0,
});

export const workflowEventTo = style({
  fontWeight: 600,
  color: vars.color.textPrimary,
  flexShrink: 0,
});

export const workflowEventMeta = style({
  marginLeft: "auto",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  whiteSpace: "nowrap",
});

export const loadingState = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  padding: vars.space["12"],
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});

export const planSummary = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  lineHeight: "1.6",
  marginBottom: vars.space["3"],
});

export const planTaskList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  marginTop: vars.space["2"],
});

export const planTask = style({
  display: "flex",
  alignItems: "baseline",
  gap: vars.space["2"],
  fontSize: vars.fontSize.sm,
  padding: `${vars.space["1"]} 0`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  selectors: {
    "&:last-child": {
      borderBottom: "none",
    },
  },
});

export const planTaskText = style({
  flex: 1,
  color: vars.color.textPrimary,
});

export const planTaskMeta = style({
  display: "flex",
  gap: vars.space["2"],
  flexShrink: 0,
});

export const planTaskBadge = style({
  fontSize: vars.fontSize.xs,
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  background: vars.color.hoverBackground,
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
});

export const sessionEndedBadge = style({
  fontSize: vars.fontSize.xs,
  padding: `1px ${vars.space["1"]}`,
  borderRadius: vars.radii.sm,
  background: vars.color.hoverBackground,
  color: vars.color.textMuted,
  marginLeft: vars.space["2"],
});

export const errorState = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  padding: vars.space["12"],
  color: vars.color.error,
  fontSize: vars.fontSize.sm,
});
