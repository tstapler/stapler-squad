import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
});

export const heading = style({
  color: vars.color.textPrimary,
  fontSize: "1.25rem",
  fontWeight: 600,
  margin: 0,
});

export const loadingText = style({
  color: vars.color.textMuted,
});

export const hint = style({
  color: vars.color.textMuted,
  fontSize: "0.8125rem",
  margin: 0,
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

export const select = style({
  padding: "0.5rem 0.75rem",
  backgroundColor: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: "4px",
  color: vars.color.inputText,
  fontSize: "0.875rem",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
    },
  },
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

export const tagList = style({
  display: "flex",
  flexWrap: "wrap",
  gap: "0.375rem",
});

export const tag = style({
  display: "inline-flex",
  alignItems: "center",
  gap: "0.25rem",
  padding: "0.25rem 0.5rem",
  backgroundColor: vars.color.accentBg,
  border: `1px solid ${vars.color.borderSubtle}`,
  borderRadius: "12px",
  color: vars.color.textPrimary,
  fontSize: "0.8125rem",
});

export const tagRemove = style({
  background: "none",
  border: "none",
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: "0.75rem",
  padding: "0 0.125rem",
  lineHeight: "1",
  selectors: {
    "&:hover": {
      color: vars.color.error,
    },
  },
});

export const tagInputRow = style({
  display: "flex",
  gap: "0.5rem",
  alignItems: "center",
});

export const envVarTable = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
});

export const envVarRow = style({
  display: "flex",
  gap: "0.5rem",
  alignItems: "center",
});

export const deleteBtn = style({
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
  },
});

export const actions = style({
  display: "flex",
  gap: "0.5rem",
  paddingTop: "0.5rem",
});
