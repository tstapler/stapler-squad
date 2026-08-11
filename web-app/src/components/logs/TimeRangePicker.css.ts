import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  position: "relative",
  display: "inline-block",
});

export const trigger = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.5rem 0.75rem",
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textPrimary,
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.9rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "border-color 0.2s, background-color 0.2s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
      borderColor: vars.color.inputBorder,
    },
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
      boxShadow: `0 0 0 2px ${vars.color.accentBg}`,
    },
  },
});

export const icon = style({
  fontSize: "1rem",
});

export const label = style({
  flex: 1,
  textAlign: "left",
  minWidth: "120px",
});

export const chevron = style({
  fontSize: "0.7rem",
  color: vars.color.textMuted,
});

export const dropdown = style({
  position: "absolute",
  top: "100%",
  left: "0",
  marginTop: "4px",
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  boxShadow: vars.shadow.md,
  zIndex: 100,
  minWidth: "200px",
  overflow: "hidden",
});

export const presets = style({
  display: "flex",
  flexDirection: "column",
  padding: "0.5rem",
});

export const presetButton = style({
  padding: "0.5rem 0.75rem",
  background: "none",
  border: "none",
  color: vars.color.textPrimary,
  textAlign: "left",
  cursor: "pointer",
  borderRadius: "4px",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
    "&:focus": {
      outline: "none",
      boxShadow: `inset 0 0 0 2px ${vars.color.accentHover}`,
    },
  },
});

export const presetButtonActive = style({
  backgroundColor: vars.color.primary,
  color: vars.color.primaryText,
});

export const divider = style({
  height: "1px",
  backgroundColor: vars.color.borderColor,
  margin: "0.25rem 0",
});

export const customButton = style({
  width: "100%",
  padding: "0.5rem 0.75rem",
  background: "none",
  border: "none",
  color: vars.color.primary,
  textAlign: "left",
  cursor: "pointer",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const customRange = style({
  padding: "0.75rem",
});

export const customHeader = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  marginBottom: "0.75rem",
  fontSize: "0.85rem",
  color: vars.color.textMuted,
});

export const backButton = style({
  background: "none",
  border: "none",
  color: vars.color.primary,
  cursor: "pointer",
  fontSize: "0.85rem",
  padding: "0",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  selectors: {
    "&:hover": {
      textDecoration: "underline",
    },
  },
});

export const customInputs = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
  marginBottom: "0.75rem",
});

export const inputGroup = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.25rem",
});

globalStyle(`${inputGroup} span`, { fontSize: "0.75rem", color: vars.color.textMuted });

export const dateInput = style({
  padding: "0.5rem",
  backgroundColor: vars.color.background,
  border: `1px solid ${vars.color.inputBorder}`,
  color: vars.color.textPrimary,
  borderRadius: "4px",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
    },
  },
});

export const applyButton = style({
  width: "100%",
  padding: "0.5rem",
  backgroundColor: vars.color.primary,
  border: "none",
  color: vars.color.primaryText,
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "background-color 0.2s",
  selectors: {
    "&:hover:not(:disabled)": {
      backgroundColor: vars.color.primaryHover,
    },
    "&:disabled": {
      backgroundColor: vars.color.borderColor,
      color: vars.color.textMuted,
      cursor: "not-allowed",
    },
  },
});
