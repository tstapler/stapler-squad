import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const form = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const fieldGroup = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const label = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
  display: "flex",
  alignItems: "center",
  gap: vars.space["1"],
});

export const required = style({
  color: vars.color.error,
  fontSize: vars.fontSize.xs,
  lineHeight: "1",
});

export const input = style({
  width: "100%",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.sans,
  outline: "none",
  transition: "border-color 0.15s ease",
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
    boxShadow: `0 0 0 2px ${vars.color.accentBg}`,
  },
  "::placeholder": {
    color: vars.color.placeholderColor,
  },
  boxSizing: "border-box",
});

export const textarea = style({
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
  transition: "border-color 0.15s ease",
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
    boxShadow: `0 0 0 2px ${vars.color.accentBg}`,
  },
  "::placeholder": {
    color: vars.color.placeholderColor,
  },
  boxSizing: "border-box",
});

export const select = style({
  width: "100%",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.sans,
  outline: "none",
  cursor: "pointer",
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
  },
  boxSizing: "border-box",
});

export const checkboxRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  cursor: "pointer",
});

export const checkboxInput = style({
  width: "16px",
  height: "16px",
  cursor: "pointer",
  accentColor: vars.color.primary,
});

export const checkboxLabel = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  cursor: "pointer",
});

export const acSection = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const acSectionHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
});

export const acList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const acRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const acInput = style({
  flex: 1,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.sans,
  outline: "none",
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
  },
});

export const acStatusSelect = style({
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  outline: "none",
  cursor: "pointer",
  minWidth: "100px",
});

export const removeButton = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  width: "24px",
  height: "24px",
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderMuted}`,
  background: "transparent",
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: vars.fontSize.sm,
  flexShrink: 0,
  ":hover": {
    background: vars.color.errorBg,
    color: vars.color.error,
    borderColor: vars.color.error,
  },
});

export const addButton = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  background: "transparent",
  color: vars.color.primary,
  border: `1px dashed ${vars.color.borderMuted}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  alignSelf: "flex-start",
  ":hover": {
    background: vars.color.accentBg,
    borderColor: vars.color.primary,
  },
});

export const formActions = style({
  display: "flex",
  gap: vars.space["2"],
  justifyContent: "flex-end",
  paddingTop: vars.space["2"],
  borderTop: `1px solid ${vars.color.borderSubtle}`,
});

export const submitButton = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  transition: "background 0.1s ease",
  ":hover": {
    background: vars.color.primaryHover,
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
});

export const cancelButton = style({
  display: "inline-flex",
  alignItems: "center",
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  ":hover": {
    background: vars.color.hoverBackground,
    borderColor: vars.color.borderStrong,
  },
});

export const errorMessage = style({
  color: vars.color.error,
  fontSize: vars.fontSize.xs,
  marginTop: vars.space["1"],
});

export const twoColumn = style({
  display: "grid",
  gridTemplateColumns: "1fr 1fr",
  gap: vars.space["3"],
  "@media": {
    "(max-width: 480px)": {
      gridTemplateColumns: "1fr",
    },
  },
});
