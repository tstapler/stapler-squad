import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const pulseKeyframes = keyframes({
  "0%, 100%": {
    opacity: 1,
    transform: "scale(1)",
  },
  "50%": {
    opacity: 0.6,
    transform: "scale(1.2)",
  },
});

export const container = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  position: "relative",
});

export const toggleButton = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.5rem 0.75rem",
  backgroundColor: vars.color.terminalBackground,
  border: `1px solid ${vars.color.terminalBorder}`,
  color: vars.color.terminalTextMuted,
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  transition: "all 0.2s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.terminalHoverBg,
      borderColor: vars.color.terminalBorder,
      color: vars.color.terminalForeground,
    },
  },
});

export const toggleButtonActive = style({
  backgroundColor: vars.color.accentBg,
  borderColor: vars.color.primary,
  color: vars.color.primary,
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.accentHover,
    },
  },
});

export const indicator = style({
  width: "8px",
  height: "8px",
  borderRadius: "50%",
  backgroundColor: vars.color.terminalTextMuted,
  transition: "background-color 0.2s",
});

export const indicatorActive = style({
  backgroundColor: vars.color.primary,
});

export const indicatorPulse = style({
  animation: `${pulseKeyframes} 1.5s ease-in-out infinite`,
});

export const label = style({
  fontWeight: 500,
});

export const pauseButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "32px",
  height: "32px",
  padding: "0",
  backgroundColor: vars.color.terminalBackground,
  border: `1px solid ${vars.color.terminalBorder}`,
  color: vars.color.terminalTextMuted,
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.85rem",
  transition: "all 0.2s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.terminalHoverBg,
      borderColor: vars.color.terminalBorder,
      color: vars.color.terminalForeground,
    },
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
    },
  },
});

export const settingsButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "32px",
  height: "32px",
  padding: "0",
  backgroundColor: vars.color.terminalBackground,
  border: `1px solid ${vars.color.terminalBorder}`,
  color: vars.color.terminalTextMuted,
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "0.85rem",
  transition: "all 0.2s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.terminalHoverBg,
      borderColor: vars.color.terminalBorder,
      color: vars.color.terminalForeground,
    },
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
    },
  },
});

export const dropdown = style({
  position: "absolute",
  top: "100%",
  right: "0",
  marginTop: "4px",
  backgroundColor: vars.color.terminalBackground,
  border: `1px solid ${vars.color.terminalBorder}`,
  borderRadius: "6px",
  boxShadow: "0 4px 12px rgba(0, 0, 0, 0.4)",
  zIndex: 100,
  minWidth: "140px",
  overflow: "hidden",
});

export const dropdownHeader = style({
  padding: "0.5rem 0.75rem",
  fontSize: "0.75rem",
  fontWeight: 600,
  color: vars.color.terminalTextMuted,
  textTransform: "uppercase",
  borderBottom: `1px solid ${vars.color.terminalBorder}`,
});

export const options = style({
  display: "flex",
  flexDirection: "column",
  padding: "0.25rem",
});

export const option = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "0.5rem 0.75rem",
  background: "none",
  border: "none",
  color: vars.color.terminalForeground,
  cursor: "pointer",
  fontSize: "0.85rem",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
  borderRadius: "4px",
  transition: "background-color 0.15s",
  textAlign: "left",
  width: "100%",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.terminalHoverBg,
    },
  },
});

export const optionSelected = style({
  color: vars.color.primary,
});

export const check = style({
  fontSize: "0.8rem",
});

export const lastUpdate = style({
  fontSize: "0.75rem",
  color: vars.color.terminalTextMuted,
  whiteSpace: "nowrap",
});
