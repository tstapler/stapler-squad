import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const wrapper = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  padding: vars.space["12"],
  gap: vars.space["6"],
  textAlign: "center",
});

export const headline = style({
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const subline = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  maxWidth: 400,
});

export const lifecycleDiagram = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexWrap: "wrap",
  justifyContent: "center",
  marginTop: vars.space["2"],
  "@media": {
    "(max-width: 480px)": {
      flexDirection: "column",
    },
  },
});

export const lifecycleNode = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  gap: "2px",
  fontSize: vars.fontSize.xs,
});

export const lifecycleNodeActive = style({
  color: vars.color.primary,
  fontWeight: vars.fontWeight.semibold,
});

export const lifecycleNodeInactive = style({
  color: vars.color.textMuted,
});

export const lifecycleArrow = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  userSelect: "none",
});

export const ctaButton = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  minHeight: 44,
  ":hover": {
    background: vars.color.primaryHover,
  },
});

export const inlineForm = style({
  width: "100%",
  maxWidth: 480,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  textAlign: "left",
});

export const formLabel = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
  display: "block",
});

export const formInput = style({
  width: "100%",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  ":focus": {
    outline: "none",
    borderColor: vars.color.inputFocusBorder,
  },
});

export const formSelect = style({
  width: "100%",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  ":focus": {
    outline: "none",
    borderColor: vars.color.inputFocusBorder,
  },
});

export const formActions = style({
  display: "flex",
  justifyContent: "flex-end",
  gap: vars.space["2"],
});

export const cancelButton = style({
  background: "none",
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.md,
  cursor: "pointer",
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.sm,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  minHeight: 44,
});

export const submitButton = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  minHeight: 44,
  ":hover": {
    background: vars.color.primaryHover,
  },
});

export const submitButtonDisabled = style({
  opacity: 0.5,
  cursor: "not-allowed",
});

export const validationError = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.errorText,
});

export const filterZeroWrapper = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  padding: vars.space["8"],
  gap: vars.space["3"],
  color: vars.color.textMuted,
  textAlign: "center",
});

export const filterZeroText = style({
  fontSize: vars.fontSize.sm,
});

export const clearFiltersButton = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  background: "none",
  border: `1px solid ${vars.color.borderMuted}`,
  color: vars.color.textSecondary,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  minHeight: 44,
  ":hover": {
    borderColor: vars.color.borderStrong,
    color: vars.color.textPrimary,
  },
});

export const footerNudge = style({
  padding: vars.space["4"],
  textAlign: "center",
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  borderTop: `1px solid ${vars.color.borderSubtle}`,
});
