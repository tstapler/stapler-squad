import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  padding: "20px",
  maxWidth: "1600px",
  margin: "0 auto",
  minHeight: "calc(var(--viewport-height, 100dvh) - 80px)",
  display: "flex",
  flexDirection: "column",
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "16px",
    },
  },
});

export const header = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: "16px",
  "@media": {
    "screen and (max-width: 768px)": {
      flexDirection: "column",
      alignItems: "flex-start",
      gap: "12px",
    },
  },
});

export const title = style({
  fontSize: "24px",
  fontWeight: "bold",
  color: vars.color.textPrimary,
  margin: 0,
});

export const groupingIndicator = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  fontSize: "14px",
  color: vars.color.textSecondary,
  background: vars.color.cardBackground,
  padding: "6px 12px",
  borderRadius: "6px",
  border: `1px solid ${vars.color.borderColor}`,
});

export const shortcutHint = style({
  fontSize: "12px",
  color: vars.color.textMuted,
});

export const errorBanner = style({
  display: "flex",
  alignItems: "center",
  gap: "16px",
  padding: "12px 16px",
  background: "rgba(220, 38, 38, 0.1)",
  border: "1px solid rgba(220, 38, 38, 0.3)",
  borderRadius: "8px",
  marginBottom: "16px",
});

export const errorContent = style({
  display: "flex",
  alignItems: "center",
  gap: "12px",
  flex: 1,
});

export const errorIcon = style({
  fontSize: "20px",
});

export const errorTitle = style({
  fontWeight: 600,
  color: vars.color.textPrimary,
  marginBottom: "2px",
});

export const content = style({
  display: "flex",
  gap: "24px",
  flex: 1,
  minHeight: 0,
  "@media": {
    "screen and (max-width: 1024px)": {
      flexDirection: "column",
    },
  },
});

export const entryList = style({
  flex: 1,
  minWidth: 0,
  minHeight: 0,
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
});

export const sectionTitle = style({
  marginBottom: "12px",
  fontSize: "16px",
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const loadMoreContainer = style({
  display: "flex",
  justifyContent: "center",
  padding: "16px 0 8px",
});

export const keyboardHints = style({
  display: "flex",
  justifyContent: "center",
  gap: "20px",
  padding: "12px 16px",
  marginTop: "16px",
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  fontSize: "12px",
  color: vars.color.textMuted,
  "@media": {
    "screen and (max-width: 768px)": {
      display: "none",
    },
  },
});

export const modalOverlay = style({
  position: "fixed",
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  background: "rgba(0, 0, 0, 0.6)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: 1000,
  padding: "20px",
});

export const resumeModal = style({
  background: vars.color.background,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: "12px",
  padding: "28px 32px",
  width: "100%",
  maxWidth: "520px",
  boxShadow: "0 20px 60px rgba(0, 0, 0, 0.3)",
  display: "flex",
  flexDirection: "column",
  gap: "16px",
});

export const resumeModalTitle = style({
  fontSize: "18px",
  fontWeight: 700,
  color: vars.color.textPrimary,
  margin: 0,
});

export const resumeModalSubtitle = style({
  fontSize: "13px",
  color: vars.color.textSecondary,
  margin: 0,
});

export const resumeModalField = style({
  display: "flex",
  flexDirection: "column",
  gap: "6px",
});

export const resumeModalLabel = style({
  fontSize: "12px",
  fontWeight: 600,
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.04em",
});

export const resumeModalInput = style({
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: "6px",
  color: vars.color.inputText,
  fontSize: "14px",
  padding: "8px 12px",
  width: "100%",
  boxSizing: "border-box",
  outline: "none",
  selectors: {
    "&:focus": {
      borderColor: vars.color.inputFocusBorder,
    },
  },
});

export const resumeModalPath = style({
  fontFamily: "monospace",
  fontSize: "12px",
  color: vars.color.textMuted,
  background: vars.color.cardBackground,
  padding: "6px 10px",
  borderRadius: "4px",
  wordBreak: "break-all",
});

export const resumeModalActions = style({
  display: "flex",
  justifyContent: "flex-end",
  gap: "10px",
  paddingTop: "4px",
});
