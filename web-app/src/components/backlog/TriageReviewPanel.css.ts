import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

export const panel = style({
  backgroundColor: vars.color.cardBackground,
  borderLeft: `3px solid ${vars.color.warning}`,
  borderRadius: vars.radii.md,
  padding: vars.space["4"],
  marginBottom: vars.space["4"],
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const panelHeader = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
});

export const heading = style({
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  margin: 0,
});

export const dismissButton = style({
  backgroundColor: "transparent",
  color: vars.color.textMuted,
  border: "none",
  borderRadius: vars.radii.sm,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  transition: vars.transition.fast,
  ":hover": {
    color: vars.color.textPrimary,
    backgroundColor: vars.color.hoverBackground,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

export const summarySection = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const sectionLabel = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  margin: 0,
});

export const summaryText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  margin: 0,
  lineHeight: "1.6",
});

export const divider = style({
  borderTop: `1px solid ${vars.color.borderSubtle}`,
  margin: 0,
});

export const actions = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  marginTop: vars.space["1"],
});

export const applyButton = style({
  backgroundColor: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.md,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  width: "100%",
  transition: vars.transition.fast,
  ":hover": {
    backgroundColor: vars.color.primaryHover,
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

export const skipButton = style({
  backgroundColor: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  width: "100%",
  transition: vars.transition.fast,
  ":hover": {
    backgroundColor: vars.color.hoverBackground,
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

// Feedback / refine section

export const refineForm = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const refineLabel = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const refineTextarea = style({
  width: "100%",
  minHeight: "72px",
  padding: vars.space["2"],
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.sans,
  resize: "vertical",
  ":focus": {
    outline: "none",
    borderColor: vars.color.inputFocusBorder,
  },
});

export const iterationBadge = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  fontWeight: vars.fontWeight.medium,
});

export const noSuggestionsText = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  margin: 0,
  fontStyle: "italic",
});

// Implementation plan / tasks section

export const taskList = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const taskItem = style({
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space["2"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const taskBullet = style({
  flexShrink: 0,
  marginTop: "2px",
  color: vars.color.textMuted,
});

export const taskText = style({
  flex: 1,
  lineHeight: "1.5",
});

export const taskBadge = style({
  flexShrink: 0,
  fontSize: vars.fontSize.xs,
  padding: `1px ${vars.space["1"]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderSubtle}`,
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
  lineHeight: "1.6",
});

export const taskEstimateBadge = style([
  taskBadge,
  {
    backgroundColor: vars.color.hoverBackground,
    color: vars.color.textSecondary,
  },
]);

export const taskCategoryBadge = style([
  taskBadge,
  {
    backgroundColor: "transparent",
    color: vars.color.textMuted,
    textTransform: "uppercase",
    letterSpacing: "0.04em",
  },
]);

// Undo toast overlay
export const undoToastOverlay = style({
  position: "fixed",
  bottom: vars.space["6"],
  left: "50%",
  transform: "translateX(-50%)",
  zIndex: zIndex.modal,
  display: "flex",
  gap: vars.space["3"],
  alignItems: "center",
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  boxShadow: vars.shadow.md,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  whiteSpace: "nowrap",
});

export const undoButton = style({
  backgroundColor: "transparent",
  color: vars.color.primary,
  border: "none",
  padding: `0 ${vars.space["1"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  cursor: "pointer",
  textDecoration: "underline",
  ":hover": {
    color: vars.color.primaryHover,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});
