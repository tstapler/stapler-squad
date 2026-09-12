import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import {
  actionRow,
  cancelBtn,
  checkboxRow,
  confirmDeleteBtn,
  deleteBtn,
  errorMessage,
  fieldGroup,
  form,
  hint,
  input,
  inputDisabled,
  label,
  submitBtn,
} from "@/styles/settingsResourceForm.css";

export {
  actionRow,
  cancelBtn,
  checkboxRow,
  confirmDeleteBtn,
  deleteBtn,
  errorMessage,
  fieldGroup,
  form,
  hint,
  input,
  inputDisabled,
  label,
  submitBtn,
};

export const inputInvalid = style({
  borderColor: vars.color.error,
});

export const textarea = style([
  input,
  {
    minHeight: "56px",
    resize: "vertical",
  },
]);

export const flagsRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["4"],
  flexWrap: "wrap",
});

export const sectionHeading = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const transitionList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const transitionCard = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: vars.space["3"],
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  background: vars.color.cardBackground,
});

export const transitionHeaderRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const gateList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  marginTop: vars.space["1"],
});

export const gateCheckboxRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const gateSubFields = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  marginLeft: vars.space["6"],
  paddingLeft: vars.space["3"],
  borderLeft: `2px solid ${vars.color.borderColor}`,
});

export const fieldError = style({
  color: vars.color.errorText,
  fontSize: vars.fontSize.xs,
});

export const addBtn = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
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

export const removeBtn = style({
  marginLeft: "auto",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.error}`,
  background: "transparent",
  color: vars.color.errorText,
});

export const graphSection = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const warningBanner = style({
  color: vars.color.textPrimary,
  background: "#fff8e1",
  border: "1px solid #f0c040",
  borderRadius: vars.radii.sm,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  alignItems: "flex-start",
});
