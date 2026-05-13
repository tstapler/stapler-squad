import { keyframes, style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const spinKeyframes = keyframes({
  from: { transform: "rotate(0deg)" },
  to: { transform: "rotate(360deg)" },
});

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const sectionTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  marginBottom: vars.space["1"],
});

const verdictCardBase = style({
  borderRadius: vars.radii.md,
  padding: vars.space["3"],
  borderLeft: "4px solid",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const verdictCard = verdictCardBase;

export const verdictCardPass = style([
  verdictCardBase,
  {
    borderLeftColor: vars.color.success,
    background: vars.color.successBg,
  },
]);

export const verdictCardPartial = style([
  verdictCardBase,
  {
    borderLeftColor: vars.color.warning,
    background: vars.color.warningBg,
  },
]);

export const verdictCardFail = style([
  verdictCardBase,
  {
    borderLeftColor: vars.color.error,
    background: vars.color.errorBg,
  },
]);

export const verdictCardPending = style([
  verdictCardBase,
  {
    borderLeftColor: vars.color.textMuted,
    background: vars.color.cardBackground,
  },
]);

export const verdictHeader = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const verdictIcon = style({});

export const verdictIconPass = style({
  color: vars.color.success,
  fontWeight: vars.fontWeight.bold,
});

export const verdictIconPartial = style({
  color: vars.color.warning,
});

export const verdictIconFail = style({
  color: vars.color.error,
});

export const verdictIconPending = style({
  color: vars.color.textMuted,
  animationName: spinKeyframes,
  animationDuration: "1s",
  animationTimingFunction: "linear",
  animationIterationCount: "infinite",
  display: "inline-block",
});

export const verdictLabel = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.bold,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  fontFamily: vars.font.mono,
});

export const verdictLabelPass = style({
  color: vars.color.success,
});

export const verdictLabelPartial = style({
  color: vars.color.warning,
});

export const verdictLabelFail = style({
  color: vars.color.errorText,
});

export const verdictLabelPending = style({
  color: vars.color.textMuted,
});

export const verdictSummary = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const criteriaList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  marginTop: vars.space["1"],
  listStyle: "none",
  margin: 0,
  padding: 0,
});

export const criteriaItem = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  fontSize: vars.fontSize.sm,
});

export const criteriaIconPass = style({
  color: vars.color.success,
});

export const criteriaIconFail = style({
  color: vars.color.error,
});

export const actions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexWrap: "wrap",
});

const buttonBase = style({
  display: "inline-flex",
  alignItems: "center",
  minHeight: "44px",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
});

export const primaryButton = style([
  buttonBase,
  {
    padding: `${vars.space["2"]} ${vars.space["4"]}`,
    background: vars.color.primary,
    color: vars.color.primaryText,
    border: "none",
    fontWeight: vars.fontWeight.medium,
    ":hover": {
      background: vars.color.primaryHover,
    },
  },
]);

export const secondaryButton = style([
  buttonBase,
  {
    padding: `${vars.space["2"]} ${vars.space["4"]}`,
    background: "none",
    border: `1px solid ${vars.color.borderMuted}`,
    color: vars.color.textSecondary,
    fontWeight: vars.fontWeight.medium,
    ":hover": {
      borderColor: vars.color.borderStrong,
      color: vars.color.textPrimary,
    },
  },
]);

export const dangerButton = style([
  buttonBase,
  {
    padding: `${vars.space["2"]} ${vars.space["4"]}`,
    background: vars.color.error,
    color: vars.color.primaryText,
    border: "none",
    fontWeight: vars.fontWeight.medium,
    ":hover": {
      background: vars.color.errorDark,
    },
  },
]);

export const disabledButton = style({
  opacity: 0.5,
  cursor: "not-allowed",
});

export const skipLink = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  textDecoration: "underline",
  marginTop: vars.space["1"],
  padding: 0,
});

export const overrideSection = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  marginTop: vars.space["2"],
});

export const overrideToggle = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
  textDecoration: "underline",
  padding: 0,
});

export const overrideForm = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const formLabel = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const formTextarea = style({
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

export const formHint = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const formActions = style({
  display: "flex",
  gap: vars.space["2"],
});

export const skipGateConfirmation = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  padding: vars.space["3"],
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.md,
  background: vars.color.surfaceMuted,
  marginTop: vars.space["2"],
});

export const skipGateWarning = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const skipGateBody = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
});
