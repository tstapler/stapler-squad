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

export const hint = style({
  color: vars.color.textMuted,
  fontSize: "0.8125rem",
  margin: 0,
});

export const warningText = style({
  color: vars.color.warningText,
  backgroundColor: vars.color.warningBg,
  border: `1px solid ${vars.color.warning}`,
  borderRadius: "4px",
  padding: "0.375rem 0.625rem",
  fontSize: "0.8125rem",
  margin: 0,
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

export const testResultStatus = style({
  color: vars.color.textPrimary,
  backgroundColor: vars.color.accentBg,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: "4px",
  padding: "0.5rem 0.75rem",
  fontSize: "0.8125rem",
});

export const repoList = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.375rem",
  margin: 0,
  padding: 0,
  listStyle: "none",
});

export const repoRow = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: "0.5rem",
  padding: "0.5rem 0.75rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "4px",
});

export const repoName = style({
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const revokeBtn = style({
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

export const revokedNote = style({
  color: vars.color.successText,
  fontSize: "0.8125rem",
  margin: 0,
});

export const emptyRepoNote = style({
  color: vars.color.textMuted,
  fontSize: "0.8125rem",
  margin: 0,
});

export const usageNote = style({
  color: vars.color.textMuted,
  fontSize: "0.8125rem",
  margin: 0,
});

export const actions = style({
  display: "flex",
  gap: "0.75rem",
  alignItems: "center",
  paddingTop: "0.5rem",
});

export const saveStatus = style({
  color: vars.color.successText,
  fontSize: "0.8125rem",
});

export const saveError = style({
  color: vars.color.errorText,
  fontSize: "0.8125rem",
});
