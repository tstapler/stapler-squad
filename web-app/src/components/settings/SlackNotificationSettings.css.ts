import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Shared settings-panel-with-a-form shell (container/heading/description/
// form/field/label/inputRow/input) — see settingsPanelForm.css.ts's doc
// comment. Extracted from here (and from JulesSettings.css.ts, which
// redeclared the same block) to fix a jscpd duplication-gate finding.
export {
  container,
  heading,
  description,
  loadingText,
  form,
  field,
  label,
  inputRow,
  input,
} from "@/styles/settingsPanelForm.css";

export const inputInvalid = style({
  borderColor: vars.color.error,
});

export const hint = style({
  color: vars.color.textMuted,
  fontSize: "0.8125rem",
  margin: 0,
});

export const errorText = style({
  color: vars.color.errorText,
  fontSize: "0.8125rem",
  margin: 0,
});

export const removeBtn = style({
  padding: "0.375rem 0.75rem",
  backgroundColor: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: "4px",
  color: vars.color.errorText,
  fontSize: "0.8125rem",
  cursor: "pointer",
  whiteSpace: "nowrap",
  selectors: {
    "&:hover": {
      opacity: 0.9,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const confirmRemoveBtn = style({
  padding: "0.375rem 0.75rem",
  backgroundColor: vars.color.errorDark,
  border: `2px solid ${vars.color.errorDark}`,
  borderRadius: "4px",
  color: vars.color.textInverse,
  fontSize: "0.8125rem",
  fontWeight: vars.fontWeight.bold,
  cursor: "pointer",
  whiteSpace: "nowrap",
});

export const toggleRow = style({
  display: "flex",
  alignItems: "flex-start",
  gap: "0.5rem",
});

export const toggleLabel = style({
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
});

export const betaNote = style({
  display: "block",
  color: vars.color.textMuted,
  fontSize: "0.75rem",
});

export const testResultAlert = style({
  color: vars.color.errorText,
  backgroundColor: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: "4px",
  padding: "0.5rem 0.75rem",
  fontSize: "0.8125rem",
});

export const testResultStatus = style({
  color: vars.color.successText,
  backgroundColor: vars.color.successBg,
  border: `1px solid ${vars.color.success}`,
  borderRadius: "4px",
  padding: "0.5rem 0.75rem",
  fontSize: "0.8125rem",
});

export const deliveryStatus = style({
  color: vars.color.textMuted,
  fontSize: "0.8125rem",
  borderTop: `1px solid ${vars.color.borderColor}`,
  paddingTop: "0.75rem",
});

export const deliveryStatusFailed = style({
  color: vars.color.errorText,
});

export const actions = style({
  display: "flex",
  gap: "0.5rem",
  paddingTop: "0.5rem",
});

export const saveError = style({
  color: vars.color.errorText,
  fontSize: "0.8125rem",
});
