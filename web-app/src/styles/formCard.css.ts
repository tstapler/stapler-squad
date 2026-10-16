import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

/**
 * Shared settings form-card chrome — used by the "manage a list of rule-like
 * rows, edit one via an inline form card" settings screens (DirectoryRulesManager,
 * ProfilesManager).
 */
export const actionsRow = style({
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
