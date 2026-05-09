import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  padding: "20px",
  maxWidth: "1200px",
  margin: "0 auto",
});

export const title = style({
  marginBottom: "20px",
  fontSize: "24px",
  fontWeight: "bold",
  color: vars.color.textPrimary,
});

export const content = style({
  display: "flex",
  gap: "20px",
  "@media": {
    "screen and (max-width: 768px)": {
      flexDirection: "column",
    },
  },
});

export const fileList = style({
  width: "250px",
  flexShrink: 0,
  "@media": {
    "screen and (max-width: 768px)": {
      width: "100%",
      maxHeight: "220px",
      overflowY: "auto",
      borderBottom: `1px solid ${vars.color.borderColor}`,
    },
  },
});

export const sectionTitle = style({
  marginBottom: "10px",
  fontSize: "18px",
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const fileListItems = style({
  display: "flex",
  flexDirection: "column",
  gap: "5px",
});

export const fileButton = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  padding: "10px",
  textAlign: "left",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "4px",
  backgroundColor: vars.color.cardBackground,
  cursor: "pointer",
  fontWeight: "normal",
  color: vars.color.textPrimary,
  transition: "background-color 0.2s, border-color 0.2s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const fileButtonSelected = style({
  backgroundColor: vars.color.primary,
  color: vars.color.primaryText,
  fontWeight: 600,
  borderColor: vars.color.primary,
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.primaryHover,
    },
  },
});

export const fileButtonModified = style({
  borderColor: vars.color.warning,
});

export const fileIcon = style({
  flexShrink: 0,
  fontSize: "14px",
});

export const fileName = style({
  flex: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const fileModifiedDot = style({
  color: vars.color.warning,
  fontSize: "10px",
  flexShrink: 0,
});

export const fileShortcut = style({
  flexShrink: 0,
  fontSize: "11px",
  padding: "2px 6px",
  borderRadius: "3px",
  backgroundColor: "rgba(255, 255, 255, 0.1)",
  color: vars.color.textMuted,
  fontFamily: "monospace",
});

export const unsavedCount = style({
  marginLeft: "8px",
  fontSize: "12px",
  fontWeight: "normal",
  padding: "2px 8px",
  borderRadius: "10px",
  backgroundColor: "rgba(245, 158, 11, 0.15)",
  color: vars.color.warning,
});

export const editor = style({
  flex: 1,
  "@media": {
    "screen and (max-width: 768px)": {
      width: "100%",
      minWidth: 0,
    },
  },
});

export const editorHeader = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: "10px",
});

export const editorTitle = style({
  fontSize: "18px",
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const modifiedIndicator = style({
  color: vars.color.warning,
  marginLeft: "10px",
});

export const buttonGroup = style({
  display: "flex",
  gap: "10px",
});

export const textarea = style({
  width: "100%",
  height: "600px",
  fontFamily: "monospace",
  fontSize: "14px",
  padding: "10px",
  backgroundColor: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: "4px",
  color: vars.color.inputText,
  resize: "vertical",
  transition: "border-color 0.2s",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
    },
  },
});

export const emptyState = style({
  padding: "40px",
  textAlign: "center",
  color: vars.color.textMuted,
  border: `2px dashed ${vars.color.borderColor}`,
  borderRadius: "4px",
});

export const validationBadgeValid = style({
  display: "inline-flex",
  alignItems: "center",
  padding: "4px 12px",
  borderRadius: "4px",
  fontSize: "13px",
  fontWeight: 500,
  backgroundColor: "rgba(40, 167, 69, 0.15)",
  color: "#28a745",
  border: "1px solid rgba(40, 167, 69, 0.3)",
});

export const validationBadgeError = style({
  display: "inline-flex",
  alignItems: "center",
  padding: "4px 12px",
  borderRadius: "4px",
  fontSize: "13px",
  fontWeight: 500,
  backgroundColor: "rgba(220, 53, 69, 0.15)",
  color: "#dc3545",
  border: "1px solid rgba(220, 53, 69, 0.3)",
});

export const validationPanel = style({
  marginTop: "10px",
  border: "1px solid rgba(220, 53, 69, 0.3)",
  borderRadius: "4px",
  backgroundColor: "rgba(220, 53, 69, 0.05)",
  overflow: "hidden",
});

export const validationPanelHeader = style({
  padding: "8px 12px",
  backgroundColor: "rgba(220, 53, 69, 0.1)",
  borderBottom: "1px solid rgba(220, 53, 69, 0.2)",
});

export const validationPanelTitle = style({
  fontSize: "13px",
  fontWeight: 600,
  color: "#dc3545",
});

export const validationPanelContent = style({
  maxHeight: "100px",
  overflowY: "auto",
});

export const validationError = style({
  display: "flex",
  alignItems: "flex-start",
  gap: "8px",
  padding: "8px 12px",
  cursor: "pointer",
  transition: "background-color 0.15s",
  borderBottom: "1px solid rgba(220, 53, 69, 0.1)",
  selectors: {
    "&:last-child": {
      borderBottom: "none",
    },
    "&:hover": {
      backgroundColor: "rgba(220, 53, 69, 0.1)",
    },
  },
});

export const validationErrorSeverity = style({
  color: "#dc3545",
});

export const validationWarningSeverity = style({
  color: "#ffc107",
});

export const validationErrorLocation = style({
  fontFamily: "monospace",
  fontSize: "12px",
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
});

export const validationErrorMessage = style({
  fontSize: "13px",
  color: vars.color.textPrimary,
  wordBreak: "break-word",
});

export const shortcutsHelp = style({
  marginTop: "20px",
  padding: "12px",
  borderRadius: "6px",
  backgroundColor: "rgba(255, 255, 255, 0.03)",
  border: `1px solid ${vars.color.borderColor}`,
  "@media": {
    "screen and (max-width: 768px)": {
      display: "none",
    },
  },
});

export const shortcutsTitle = style({
  fontSize: "12px",
  fontWeight: 600,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.5px",
  marginBottom: "8px",
});

export const shortcutItem = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  fontSize: "12px",
  color: vars.color.textSecondary,
  marginBottom: "6px",
  selectors: {
    "&:last-child": {
      marginBottom: 0,
    },
  },
});

export const networkSection = style({
  marginTop: "32px",
  paddingTop: "24px",
  borderTop: `1px solid ${vars.color.borderColor}`,
});

export const networkCard = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  padding: "20px",
  display: "flex",
  flexDirection: "column",
  gap: "14px",
  maxWidth: "600px",
});

export const networkRow = style({
  display: "flex",
  flexDirection: "column",
  gap: "4px",
});

export const networkLabel = style({
  fontSize: "12px",
  fontWeight: 600,
  textTransform: "uppercase",
  letterSpacing: "0.5px",
  color: vars.color.textMuted,
});

export const networkValue = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
});

export const networkValueText = style({
  fontFamily: "monospace",
  fontSize: "13px",
  color: vars.color.textPrimary,
  background: "rgba(255, 255, 255, 0.05)",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "4px",
  padding: "4px 8px",
  flex: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const networkCopyBtn = style({
  padding: "4px 10px",
  fontSize: "12px",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "4px",
  background: "rgba(255, 255, 255, 0.05)",
  color: vars.color.textSecondary,
  cursor: "pointer",
  whiteSpace: "nowrap",
  transition: "background-color 0.15s",
  selectors: {
    "&:hover": {
      background: "rgba(255, 255, 255, 0.1)",
    },
  },
});

export const networkLink = style({
  color: vars.color.primary,
  textDecoration: "none",
  fontSize: "13px",
  selectors: {
    "&:hover": {
      textDecoration: "underline",
    },
  },
});

export const networkDisabledNote = style({
  color: vars.color.textMuted,
  fontSize: "13px",
  fontStyle: "italic",
});

export const securitySection = style({
  marginTop: "32px",
  paddingTop: "24px",
  borderTop: `1px solid ${vars.color.borderColor}`,
});

export const securityCard = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  padding: "20px",
  display: "flex",
  flexDirection: "column",
  gap: "14px",
  maxWidth: "480px",
});

export const securityRow = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
});

export const securityLabel = style({
  fontSize: "14px",
  fontWeight: 500,
  color: vars.color.textSecondary,
});

export const statusEnabled = style({
  color: "#28a745",
  fontWeight: 600,
  fontSize: "14px",
});

export const statusDisabled = style({
  color: vars.color.textMuted,
  fontSize: "14px",
});

export const securityActions = style({
  display: "flex",
  gap: "10px",
  flexWrap: "wrap",
  paddingTop: "4px",
});

export const securityError = style({
  margin: 0,
  fontSize: "0.85rem",
  color: vars.color.error,
  background: "rgba(248, 81, 73, 0.1)",
  border: "1px solid rgba(248, 81, 73, 0.3)",
  borderRadius: "6px",
  padding: "0.5rem 0.75rem",
});

export const securitySuccess = style({
  margin: 0,
  fontSize: "0.85rem",
  color: "#28a745",
  background: "rgba(40, 167, 69, 0.1)",
  border: "1px solid rgba(40, 167, 69, 0.3)",
  borderRadius: "6px",
  padding: "0.5rem 0.75rem",
});

export const hostnamesList = style({
  display: "flex",
  flexDirection: "column",
  gap: "8px",
  background: "rgba(0, 0, 0, 0.1)",
  padding: "10px",
  borderRadius: "6px",
  border: `1px solid ${vars.color.borderColor}`,
});

export const hostnameItem = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: "12px",
});

export const hostnameText = style({
  fontFamily: "monospace",
  fontSize: "13px",
  color: vars.color.textPrimary,
  flex: 1,
});
