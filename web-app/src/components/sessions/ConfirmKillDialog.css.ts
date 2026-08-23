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
  maxWidth: "520px",
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
  lineHeight: 1.5,
});

export const body = style({
  padding: "24px",
  overflowY: "auto",
  flex: 1,
  display: "flex",
  flexDirection: "column",
  gap: "12px",
});

export const errorState = style({
  padding: "12px 14px",
  background: vars.color.errorBg,
  color: vars.color.errorText,
  borderRadius: "6px",
  fontSize: "13px",
  lineHeight: 1.5,
});

export const optionCard = style({
  display: "flex",
  flexDirection: "column",
  gap: "4px",
  padding: "12px 14px",
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: "6px",
});

export const optionTitle = style({
  fontSize: "13px",
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const optionDescription = style({
  fontSize: "13px",
  color: vars.color.textSecondary,
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

export const killButton = style({
  padding: "10px 24px",
  background: vars.color.error,
  color: vars.color.textInverse,
  border: "none",
  borderRadius: "6px",
  fontSize: "14px",
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      filter: "brightness(1.1)",
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});
