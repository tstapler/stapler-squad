import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexWrap: "wrap",
  gap: "0.5rem",
  alignItems: "center",
  marginBottom: "1rem",
});

export const pill = style({
  display: "inline-flex",
  alignItems: "center",
  gap: "0.25rem",
  padding: "0.25rem 0.5rem",
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: "16px",
  fontSize: "0.8rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
});

export const label = style({
  color: vars.color.textMuted,
});

export const value = style({
  color: vars.color.textPrimary,
  fontWeight: 500,
});

export const removeButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "16px",
  height: "16px",
  marginLeft: "0.25rem",
  padding: "0",
  background: "none",
  border: "none",
  color: vars.color.textMuted,
  cursor: "pointer",
  borderRadius: "50%",
  fontSize: "1rem",
  lineHeight: "1",
  transition: "color 0.15s, background-color 0.15s",
  selectors: {
    "&:hover": {
      color: vars.color.error,
      backgroundColor: vars.color.errorBg,
    },
    "&:focus": {
      outline: "none",
      boxShadow: `0 0 0 2px ${vars.color.accentHover}`,
    },
  },
});

export const clearAllButton = style({
  padding: "0.25rem 0.5rem",
  background: "none",
  border: "none",
  color: vars.color.primary,
  cursor: "pointer",
  fontSize: "0.8rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "color 0.15s",
  selectors: {
    "&:hover": {
      color: vars.color.primaryHover,
      textDecoration: "underline",
    },
    "&:focus": {
      outline: "none",
      textDecoration: "underline",
    },
  },
});
