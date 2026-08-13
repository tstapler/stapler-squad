import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const fadeIn = keyframes({
  from: { opacity: 0 },
  to: { opacity: 1 },
});

const slideUp = keyframes({
  from: { transform: "translateY(20px)", opacity: 0 },
  to: { transform: "translateY(0)", opacity: 1 },
});

const spin = keyframes({
  to: { transform: "rotate(360deg)" },
});

export const overlay = style({
  position: "fixed",
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  background: vars.color.overlayBackground,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: 1000,
  animation: `${fadeIn} 0.2s ease-out`,
});

export const menu = style({
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  width: "90%",
  maxWidth: "600px",
  maxHeight: "80vh",
  display: "flex",
  flexDirection: "column",
  boxShadow: "0 10px 40px rgba(0, 0, 0, 0.3)",
  animation: `${slideUp} 0.2s ease-out`,
});

export const header = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "20px 24px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const menuTitle = style({
  margin: 0,
  fontSize: "20px",
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const closeButton = style({
  background: "none",
  border: "none",
  fontSize: "32px",
  lineHeight: 1,
  color: vars.color.textMuted,
  cursor: "pointer",
  padding: 0,
  width: "32px",
  height: "32px",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  borderRadius: "4px",
  transition: "all 0.15s ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
});

export const contentArea = style({
  flex: 1,
  overflowY: "auto",
  padding: "24px",
});

export const section = style({
  marginBottom: "32px",
  selectors: {
    "&:last-child": {
      marginBottom: 0,
    },
  },
});

export const sectionTitle = style({
  margin: "0 0 16px 0",
  fontSize: "14px",
  fontWeight: 600,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.5px",
});

export const toggleRow = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "16px",
  background: vars.color.cardBackground,
  borderRadius: "8px",
  cursor: "pointer",
  transition: "background 0.15s ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
  },
});

export const toggleLabel = style({
  flex: 1,
  marginRight: "16px",
});

export const toggleName = style({
  display: "block",
  fontSize: "15px",
  fontWeight: 500,
  color: vars.color.textPrimary,
  marginBottom: "4px",
});

export const toggleDescription = style({
  display: "block",
  fontSize: "13px",
  color: vars.color.textMuted,
});

export const permissionWarning = style({
  color: vars.color.warning,
  fontStyle: "italic",
});

export const toggle = style({
  position: "relative",
  width: "48px",
  height: "28px",
  background: vars.color.borderColor,
  border: "none",
  borderRadius: "14px",
  cursor: "pointer",
  transition: "background 0.2s ease",
  flexShrink: 0,
  selectors: {
    "&:hover": {
      background: vars.color.textMuted,
    },
  },
});

export const toggleOn = style({
  background: `${vars.color.success} !important` as "inherit",
});

export const toggleSlider = style({
  position: "absolute",
  top: "2px",
  left: "2px",
  width: "24px",
  height: "24px",
  background: vars.color.primaryText,
  borderRadius: "50%",
  transition: "transform 0.2s ease",
  boxShadow: "0 2px 4px rgba(0, 0, 0, 0.2)",
  selectors: {
    [`${toggleOn} &`]: {
      transform: "translateX(20px)",
    },
  },
});

export const commandList = style({
  background: vars.color.cardBackground,
  padding: "16px",
  borderRadius: "8px",
});

export const command = style({
  display: "block",
  fontFamily: '"Monaco", "Courier New", monospace',
  fontSize: "13px",
  color: vars.color.success,
  marginBottom: "8px",
  padding: "8px 12px",
  background: vars.color.accentBg,
  borderRadius: "4px",
  wordBreak: "break-all",
});

export const commandDescription = style({
  display: "block",
  fontSize: "13px",
  color: vars.color.textMuted,
  paddingLeft: "12px",
});

export const debugLink = style({
  display: "flex",
  alignItems: "center",
  padding: "16px",
  background: vars.color.cardBackground,
  borderRadius: "8px",
  textDecoration: "none",
  transition: "background 0.15s ease",
  gap: "12px",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
  },
});

export const debugLinkIcon = style({
  fontSize: "24px",
  flexShrink: 0,
});

export const debugLinkContent = style({
  flex: 1,
});

export const debugLinkName = style({
  display: "block",
  fontSize: "15px",
  fontWeight: 500,
  color: vars.color.textPrimary,
  marginBottom: "4px",
});

export const debugLinkDescription = style({
  display: "block",
  fontSize: "13px",
  color: vars.color.textMuted,
});

export const footer = style({
  padding: "16px 24px",
  borderTop: `1px solid ${vars.color.borderColor}`,
  display: "flex",
  justifyContent: "flex-end",
});

export const doneButton = style({
  padding: "10px 24px",
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: "6px",
  fontSize: "14px",
  fontWeight: 500,
  cursor: "pointer",
  transition: "all 0.15s ease",
  selectors: {
    "&:hover": {
      background: vars.color.primaryHover,
      transform: "translateY(-1px)",
      boxShadow: "0 4px 8px rgba(0, 0, 0, 0.2)",
    },
  },
});

export const noteInputRow = style({
  marginBottom: "12px",
});

export const noteInput = style({
  width: "100%",
  padding: "10px 12px",
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  color: vars.color.textPrimary,
  fontSize: "14px",
  outline: "none",
  boxSizing: "border-box",
  transition: "border-color 0.15s ease",
  selectors: {
    "&:focus": {
      borderColor: vars.color.primary,
    },
    "&::placeholder": {
      color: vars.color.textMuted,
    },
  },
});

export const snapshotButton = style({
  width: "100%",
  padding: "12px 16px",
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  color: vars.color.textPrimary,
  fontSize: "15px",
  fontWeight: 500,
  cursor: "pointer",
  transition: "all 0.15s ease",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  gap: "8px",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.primary,
    },
    "&:disabled": {
      opacity: 0.6,
      cursor: "not-allowed",
    },
  },
});

export const spinner = style({
  display: "inline-block",
  width: "14px",
  height: "14px",
  border: `2px solid ${vars.color.borderColor}`,
  borderTopColor: vars.color.primary,
  borderRadius: "50%",
  animation: `${spin} 0.8s linear infinite`,
  flexShrink: 0,
});

export const snapshotResult = style({
  marginTop: "12px",
  padding: "12px",
  background: vars.color.cardBackground,
  borderRadius: "8px",
  borderLeft: `3px solid ${vars.color.success}`,
});

export const snapshotResultText = style({
  fontSize: "13px",
  color: vars.color.textPrimary,
  marginBottom: "8px",
});

export const snapshotFilePath = style({
  display: "block",
  fontFamily: '"Monaco", "Courier New", monospace',
  fontSize: "12px",
  color: vars.color.textMuted,
  wordBreak: "break-all",
  padding: "6px 8px",
  background: vars.color.accentBg,
  borderRadius: "4px",
  userSelect: "all",
  cursor: "text",
});

export const snapshotError = style({
  marginTop: "12px",
  padding: "12px",
  background: vars.color.cardBackground,
  borderRadius: "8px",
  borderLeft: `3px solid ${vars.color.error}`,
  fontSize: "13px",
  color: vars.color.error,
});
