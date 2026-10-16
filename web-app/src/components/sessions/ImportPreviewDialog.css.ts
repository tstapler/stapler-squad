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
  animation: `${fadeIn} 0.2s ease`,
});

export const modal = style({
  background: vars.color.modalBackground,
  borderRadius: "12px",
  padding: 0,
  maxWidth: "560px",
  width: "90%",
  maxHeight: "80vh",
  display: "flex",
  flexDirection: "column",
  boxShadow: "0 20px 60px rgba(0, 0, 0, 0.3)",
  animation: `${slideUp} 0.3s ease`,
});

export const header = style({
  padding: "24px 24px 16px 24px",
  borderBottom: `1px solid ${vars.color.modalBorder}`,
});

export const title = style({
  margin: "0 0 4px 0",
  fontSize: "20px",
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const subtitle = style({
  margin: 0,
  fontSize: "14px",
  color: vars.color.textSecondary,
});

export const body = style({
  padding: "24px",
  overflowY: "auto",
  flex: 1,
  display: "flex",
  flexDirection: "column",
  gap: "18px",
});

export const loadingState = style({
  padding: "32px 0",
  textAlign: "center",
  color: vars.color.textSecondary,
  fontSize: "14px",
});

export const errorState = style({
  padding: "12px 14px",
  background: vars.color.errorBg,
  color: vars.color.errorText,
  borderRadius: "6px",
  fontSize: "13px",
});

export const contextGrid = style({
  display: "flex",
  flexDirection: "column",
  gap: "6px",
});

export const contextRow = style({
  display: "flex",
  gap: "8px",
  fontSize: "13px",
});

export const contextLabel = style({
  color: vars.color.textSecondary,
  fontWeight: 500,
  minWidth: "70px",
  flexShrink: 0,
});

export const contextValue = style({
  color: vars.color.textPrimary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  fontFamily: "monospace",
  fontSize: "12px",
});

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: "8px",
});

export const fieldLabel = style({
  display: "block",
  margin: 0,
  fontSize: "13px",
  fontWeight: 600,
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.5px",
});

export const correlationBadge = style({
  display: "inline-flex",
  alignSelf: "flex-start",
  alignItems: "center",
  gap: "6px",
  padding: "4px 10px",
  borderRadius: "999px",
  fontSize: "12px",
  fontWeight: 600,
});

export const correlationBadgeResolved = style({
  background: vars.color.successBg,
  color: vars.color.success,
});

export const correlationBadgeAmbiguous = style({
  background: vars.color.warningBg,
  color: vars.color.warning,
});

export const correlationBadgeNotFound = style({
  background: vars.color.surfaceMuted,
  color: vars.color.textSecondary,
});

export const excerptBox = style({
  padding: "10px 12px",
  background: vars.color.surfaceMuted,
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: "6px",
  fontSize: "13px",
  color: vars.color.textPrimary,
  lineHeight: 1.5,
  maxHeight: "120px",
  overflowY: "auto",
});

export const candidateList = style({
  display: "flex",
  flexDirection: "column",
  gap: "8px",
});

export const candidateOption = style({
  display: "flex",
  alignItems: "flex-start",
  gap: "10px",
  padding: "10px 12px",
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: "6px",
  cursor: "pointer",
  transition: "all 0.15s ease",
  selectors: {
    "&:hover": {
      borderColor: vars.color.borderStrong,
      background: vars.color.hoverBackground,
    },
  },
});

export const candidateOptionSelected = style({
  borderColor: vars.color.primary,
  background: vars.color.hoverBackground,
});

export const candidateRadio = style({
  marginTop: "2px",
});

export const candidateDetails = style({
  display: "flex",
  flexDirection: "column",
  gap: "2px",
  minWidth: 0,
});

export const candidatePath = style({
  fontFamily: "monospace",
  fontSize: "12px",
  color: vars.color.textSecondary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const candidateUuid = style({
  fontSize: "11px",
  color: vars.color.textTertiary,
});

export const sigstopBanner = style({
  display: "flex",
  gap: "10px",
  padding: "12px 14px",
  background: vars.color.warningBg,
  border: `1px solid ${vars.color.warning}`,
  borderRadius: "6px",
  fontSize: "13px",
  color: vars.color.textPrimary,
  lineHeight: 1.5,
});

export const footer = style({
  padding: "16px 24px",
  borderTop: `1px solid ${vars.color.modalBorder}`,
  display: "flex",
  justifyContent: "flex-end",
  gap: "12px",
});

export const cancelButton = style({
  padding: "10px 24px",
  background: vars.color.surfaceSubtle,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: "6px",
  fontSize: "14px",
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderStrong,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const confirmButton = style({
  padding: "10px 24px",
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: "6px",
  fontSize: "14px",
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.primaryHover,
      transform: "translateY(-1px)",
    },
    "&:active:not(:disabled)": {
      transform: "translateY(0)",
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});
