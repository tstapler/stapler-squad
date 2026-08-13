import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  position: "relative",
  flex: 1,
  maxWidth: "400px",
});

export const inputWrapper = style({
  position: "relative",
  display: "flex",
  alignItems: "center",
});

export const input = style({
  width: "100%",
  padding: "0.5rem 2rem 0.5rem 0.5rem",
  backgroundColor: vars.color.background,
  border: `1px solid ${vars.color.inputBorder}`,
  color: vars.color.textPrimary,
  borderRadius: "4px",
  fontSize: "0.9rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "border-color 0.2s",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
    },
    "&::placeholder": {
      color: vars.color.textMuted,
    },
  },
});

export const clearButton = style({
  position: "absolute",
  right: "8px",
  background: "none",
  border: "none",
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: "1.2rem",
  lineHeight: "1",
  padding: "0.25rem",
  transition: "color 0.15s",
  selectors: {
    "&:hover": {
      color: vars.color.error,
    },
  },
});

export const dropdown = style({
  position: "absolute",
  top: "100%",
  left: "0",
  right: "0",
  marginTop: "4px",
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  boxShadow: vars.shadow.md,
  zIndex: 100,
  overflow: "hidden",
});

export const header = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  padding: "0.5rem 0.75rem",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  fontSize: "0.75rem",
  color: vars.color.textMuted,
  textTransform: "uppercase",
});

export const clearAllButton = style({
  background: "none",
  border: "none",
  color: vars.color.primary,
  cursor: "pointer",
  fontSize: "0.75rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  textTransform: "uppercase",
  selectors: {
    "&:hover": {
      color: vars.color.error,
    },
  },
});

export const items = style({
  maxHeight: "250px",
  overflowY: "auto",
});

export const item = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.5rem 0.75rem",
  cursor: "pointer",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const itemSelected = style({
  backgroundColor: vars.color.hoverBackground,
});

export const historyIcon = style({
  fontSize: "0.75rem",
  color: vars.color.textMuted,
});

export const query = style({
  flex: 1,
  color: vars.color.textPrimary,
  fontSize: "0.85rem",
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
});

export const timestamp = style({
  fontSize: "0.7rem",
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
});

export const removeButton = style({
  background: "none",
  border: "none",
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: "1rem",
  lineHeight: "1",
  padding: "0.125rem 0.25rem",
  opacity: 0,
  transition: "opacity 0.15s, color 0.15s",
  selectors: {
    [`${item}:hover &`]: {
      opacity: 1,
    },
    "&:hover": {
      color: vars.color.error,
    },
  },
});
