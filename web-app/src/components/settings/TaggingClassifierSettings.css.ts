import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Reuses the shared settings-panel shell plus JulesSettings' form primitives —
// deliberately imported, not redeclared (jscpd duplication gate).
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
export {
  hint,
  warningText,
  actions,
  saveStatus,
  saveError,
} from "./JulesSettings.css";

export const fallbackRow = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
});

export const fallbackName = style({
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  flex: "1 1 auto",
});

export const removeBtn = style({
  color: vars.color.error,
  background: "transparent",
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: "4px",
  padding: "0.25rem 0.625rem",
  fontSize: "0.8125rem",
  cursor: "pointer",
});

export const emptyNote = style({
  color: vars.color.textMuted,
  fontSize: "0.8125rem",
  margin: 0,
});
