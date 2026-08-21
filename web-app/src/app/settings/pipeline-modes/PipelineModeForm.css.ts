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
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const hint = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const input = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  color: vars.color.inputText,
  fontSize: vars.fontSize.sm,
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
    outline: "none",
  },
  "::placeholder": {
    color: vars.color.placeholderColor,
  },
});

export const inputDisabled = style({
  opacity: 0.6,
  cursor: "not-allowed",
});

export const textarea = style([
  input,
  {
    fontFamily: vars.font.mono,
    minHeight: "72px",
    resize: "vertical",
  },
]);

export const checkboxRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const templateFieldsGrid = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const actionRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  marginTop: vars.space["2"],
});

export const submitBtn = style({
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.primary}`,
  background: vars.color.primary,
  color: vars.color.textInverse,
  fontWeight: 500,
  ":hover": {
    background: vars.color.primaryHover,
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
});

export const cancelBtn = style({
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textPrimary,
  ":hover": {
    background: vars.color.hoverBackground,
  },
});

export const deleteBtn = style({
  marginLeft: "auto",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.error}`,
  background: "transparent",
  color: vars.color.errorText,
  ":hover": {
    background: vars.color.errorBg,
  },
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
});

export const confirmDeleteBtn = style({
  marginLeft: "auto",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `2px solid ${vars.color.errorDark}`,
  background: vars.color.errorDark,
  color: vars.color.textInverse,
  fontWeight: 700,
  ":disabled": {
    opacity: 0.5,
    cursor: "not-allowed",
  },
});

export const errorMessage = style({
  color: vars.color.errorText,
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
});
