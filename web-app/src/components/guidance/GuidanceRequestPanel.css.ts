import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const panel = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderLeft: `3px solid ${vars.color.borderHover}`,
  borderRadius: vars.radii.lg,
  padding: vars.space["4"],
});

export const heading = style({
  margin: 0,
  marginBottom: vars.space["3"],
  fontSize: vars.fontSize.base,
  fontWeight: 700,
  color: vars.color.textPrimary,
});

export const errorText = style({
  color: vars.color.errorText,
  fontSize: vars.fontSize.sm,
  marginBottom: vars.space["2"],
});

export const list = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const item = style({
  borderTop: `1px solid ${vars.color.borderColor}`,
  paddingTop: vars.space["3"],
  selectors: {
    "&:first-child": {
      borderTop: "none",
      paddingTop: 0,
    },
  },
});

export const itemHeader = style({
  display: "flex",
  alignItems: "baseline",
  gap: vars.space["2"],
  marginBottom: vars.space["2"],
});

const badgeBase = {
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  whiteSpace: "nowrap" as const,
  flexShrink: 0,
};

export const badgePending = style({
  ...badgeBase,
  background: vars.color.warningBg,
  color: vars.color.warningText,
  border: `1px solid ${vars.color.warning}`,
});

export const badgeAnswered = style({
  ...badgeBase,
  background: vars.color.successBg,
  color: vars.color.successText,
  border: `1px solid ${vars.color.success}`,
});

export const questionText = style({
  margin: 0,
  fontSize: vars.fontSize.base,
  color: vars.color.textPrimary,
});

export const answerText = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const actions = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["2"],
});

export const answerButton = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: vars.color.surfaceSubtle,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.15s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.borderColor,
    },
    "&:disabled": {
      opacity: 0.6,
      cursor: "not-allowed",
    },
  },
});

export const shortAnswerForm = style({
  display: "flex",
  gap: vars.space["2"],
});

export const shortAnswerInput = style({
  flex: 1,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: vars.color.background,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
});
