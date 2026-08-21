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

export const headerRow = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
});

export const loadingText = style({
  color: vars.color.textMuted,
});

export const emptyText = style({
  color: vars.color.textMuted,
  fontSize: "0.875rem",
});

export const ruleRow = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "flex-start",
  gap: "1rem",
  padding: "0.75rem",
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
});

export const ruleInfo = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.25rem",
  minWidth: 0,
});

export const rulePath = style({
  color: vars.color.textPrimary,
  fontWeight: 600,
  fontSize: "0.9375rem",
  fontFamily: "monospace",
  wordBreak: "break-all",
});

export const ruleMeta = style({
  color: vars.color.textMuted,
  fontSize: "0.75rem",
});

export const ruleActions = style({
  display: "flex",
  gap: "0.375rem",
  flexShrink: 0,
});

export const formCard = style({
  padding: "1rem",
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
});

export const formTitle = style({
  color: vars.color.textPrimary,
  fontSize: "1rem",
  fontWeight: 600,
  margin: "0 0 0.75rem 0",
});

export const formFields = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.75rem",
});

export const field = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.375rem",
});

export const label = style({
  color: vars.color.textSecondary,
  fontSize: "0.8125rem",
  fontWeight: 600,
});

export const checkboxLabel = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  color: vars.color.textSecondary,
  fontSize: "0.8125rem",
  fontWeight: 600,
  cursor: "pointer",
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

export const inputError = style({
  borderColor: vars.color.error,
});

export const fieldError = style({
  color: vars.color.error,
  fontSize: "0.75rem",
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

export const overridesSection = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.75rem",
  padding: "0.75rem",
  backgroundColor: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "4px",
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

export const formActions = style({
  display: "flex",
  gap: "0.5rem",
  paddingTop: "0.75rem",
});
