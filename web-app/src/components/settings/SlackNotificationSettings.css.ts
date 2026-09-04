import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
  maxWidth: "640px",
});

export const heading = style({
  color: vars.color.textPrimary,
  fontSize: "1.25rem",
  fontWeight: 600,
  margin: 0,
});

export const description = style({
  color: vars.color.textMuted,
  fontSize: "0.875rem",
  margin: 0,
});

export const loadingText = style({
  color: vars.color.textMuted,
});

export const form = style({
  display: "flex",
  flexDirection: "column",
  gap: "1.25rem",
});

export const field = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
});

export const label = style({
  color: vars.color.textSecondary,
  fontSize: "0.875rem",
  fontWeight: 600,
});

export const inputRow = style({
  display: "flex",
  gap: "0.5rem",
  alignItems: "center",
});

export const input = style({
  padding: "0.5rem 0.75rem",
  backgroundColor: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: "4px",
  color: vars.color.inputText,
  fontSize: "0.875rem",
  flex: 1,
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
    },
  },
});

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
