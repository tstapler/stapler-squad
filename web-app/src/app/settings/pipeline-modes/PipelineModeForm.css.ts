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

export const textarea = style([
  input,
  {
    fontFamily: vars.font.mono,
    minHeight: "72px",
    resize: "vertical",
  },
]);

export const templateFieldsGrid = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});
