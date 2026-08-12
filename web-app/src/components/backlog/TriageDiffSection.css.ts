import { style } from "@vanilla-extract/css";
import { vars, breakpoints } from "@/styles/theme.css";

export const diffContainer = style({
  display: "grid",
  gridTemplateColumns: "1fr 1fr",
  gap: vars.space["4"],
  marginTop: vars.space["2"],
});

export const diffColumn = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const columnHeader = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  marginBottom: vars.space["1"],
});

export const emptyState = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  fontStyle: "italic",
});

export const addedItem = style({
  display: "flex",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  backgroundColor: vars.color.successBg,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  lineHeight: "1.5",
});

export const removedItem = style({
  display: "flex",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  backgroundColor: vars.color.errorBg,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  lineHeight: "1.5",
  textDecoration: "line-through",
});

export const diffPrefix = style({
  flexShrink: 0,
  fontWeight: vars.fontWeight.semibold,
});

export const questionsSection = style({
  marginTop: vars.space["4"],
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const questionsHeading = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textSecondary,
  margin: 0,
});

export const questionItem = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  backgroundColor: vars.color.accentBg,
  borderRadius: vars.radii.sm,
  lineHeight: "1.5",
});

export const questionRow = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const answerToggle = style({
  alignSelf: "flex-start",
  backgroundColor: "transparent",
  color: vars.color.primary,
  border: "none",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  transition: vars.transition.fast,
  ":hover": {
    textDecoration: "underline",
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

export const answerForm = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["2"]}`,
});

export const answerTextarea = style({
  width: "100%",
  minHeight: "60px",
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

export const answerActions = style({
  display: "flex",
  gap: vars.space["2"],
  "@media": {
    [`screen and (max-width: ${breakpoints.sm})`]: {
      flexDirection: "column",
    },
  },
});

export const answerSubmitButton = style({
  backgroundColor: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.md,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
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
  "@media": {
    [`screen and (max-width: ${breakpoints.sm})`]: {
      width: "100%",
    },
  },
});

export const answerCancelButton = style({
  backgroundColor: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
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
  "@media": {
    [`screen and (max-width: ${breakpoints.sm})`]: {
      width: "100%",
    },
  },
});

export const answeredMarker = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontStyle: "italic",
});
