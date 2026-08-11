import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  position: "relative",
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
});

export const label = style({
  fontSize: "0.85rem",
  fontWeight: 500,
  color: vars.color.terminalTextMuted,
});

export const trigger = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.5rem",
  backgroundColor: vars.color.terminalBackground,
  border: `1px solid ${vars.color.terminalBorder}`,
  color: vars.color.terminalForeground,
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.9rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "border-color 0.2s",
  minWidth: "120px",
  selectors: {
    "&:hover": {
      borderColor: vars.color.terminalBorder,
    },
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
    },
  },
});

export const text = style({
  flex: 1,
  textAlign: "left",
});

export const chevron = style({
  fontSize: "0.7rem",
  color: vars.color.terminalTextMuted,
});

export const dropdown = style({
  position: "absolute",
  top: "100%",
  left: "0",
  marginTop: "4px",
  backgroundColor: vars.color.terminalBackground,
  border: `1px solid ${vars.color.terminalBorder}`,
  borderRadius: "6px",
  boxShadow: "0 4px 12px rgba(0, 0, 0, 0.4)",
  zIndex: 100,
  minWidth: "160px",
  overflow: "hidden",
});

export const actions = style({
  display: "flex",
  gap: "0.5rem",
  padding: "0.5rem",
});

export const actionButton = style({
  flex: 1,
  padding: "0.25rem 0.5rem",
  background: "none",
  border: `1px solid ${vars.color.terminalBorder}`,
  color: vars.color.primary,
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.75rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.terminalHoverBg,
    },
  },
});

export const divider = style({
  height: "1px",
  backgroundColor: vars.color.terminalBorder,
});

export const options = style({
  display: "flex",
  flexDirection: "column",
  padding: "0.5rem",
  maxHeight: "200px",
  overflowY: "auto",
});

export const option = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.5rem",
  cursor: "pointer",
  borderRadius: "4px",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.terminalHoverBg,
    },
  },
});

export const checkbox = style({
  width: "16px",
  height: "16px",
  accentColor: vars.color.primary,
  cursor: "pointer",
});

export const optionLabel = style({
  fontSize: "0.85rem",
  fontWeight: 500,
  textTransform: "uppercase",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
});
