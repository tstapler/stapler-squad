import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import { visuallyHidden } from "@/styles/a11y.css";
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
  visuallyHidden,
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

// Stage-executor table (Story 5.1.1) — sibling section to templateFieldsGrid,
// distinct concern (execution config vs. prompt content) per design/ux.md.
export const stageExecutorSection = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const sectionHeading = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textPrimary,
  margin: 0,
});

export const stageExecutorTable = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: vars.fontSize.sm,
});

export const stageExecutorHeaderCell = style({
  textAlign: "left",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  color: vars.color.textMuted,
  fontWeight: 600,
  fontSize: vars.fontSize.xs,
});

export const stageExecutorRoleCell = style({
  textAlign: "left",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  fontWeight: 600,
  color: vars.color.textPrimary,
  whiteSpace: "nowrap",
});

export const stageExecutorCell = style({
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
});

export const stageFieldError = style([
  errorMessage,
  {
    marginTop: vars.space["1"],
    display: "flex",
    flexDirection: "column",
    gap: vars.space["2"],
  },
]);

export const forceOverrideLabel = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  fontSize: vars.fontSize.xs,
  color: vars.color.errorText,
});
