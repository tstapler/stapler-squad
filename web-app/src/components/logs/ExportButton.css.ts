import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  position: "relative",
});

export const button = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.5rem 0.75rem",
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  color: vars.color.textPrimary,
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "all 0.2s",
  selectors: {
    "&:hover:not(:disabled)": {
      backgroundColor: vars.color.hoverBackground,
      borderColor: vars.color.textMuted,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const dropdown = style({
  position: "absolute",
  top: "100%",
  right: "0",
  marginTop: "4px",
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  boxShadow: vars.shadow.md,
  zIndex: 100,
  minWidth: "180px",
  overflow: "hidden",
});

export const option = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.625rem 0.75rem",
  background: "none",
  border: "none",
  color: vars.color.textPrimary,
  cursor: "pointer",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  width: "100%",
  textAlign: "left",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const icon = style({
  fontSize: "0.9rem",
  width: "20px",
  textAlign: "center",
});

export const optionLabel = style({
  fontWeight: 500,
});

export const optionDesc = style({
  fontSize: "0.75rem",
  color: vars.color.textMuted,
  marginLeft: "auto",
});

export const divider = style({
  height: "1px",
  backgroundColor: vars.color.borderColor,
  margin: "0.25rem 0",
});
