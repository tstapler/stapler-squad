import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const fadeIn = keyframes({
  from: { opacity: 0 },
  to: { opacity: 1 },
});

const slideIn = keyframes({
  from: { transform: "translateY(-10px)", opacity: 0 },
  to: { transform: "translateY(0)", opacity: 1 },
});

export const modalOverlay = style({
  position: "fixed",
  inset: 0,
  background: "rgba(0, 0, 0, 0.6)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: 1000,
  animation: `${fadeIn} 0.15s ease-out`,
});

export const modal = style({
  background: vars.color.background,
  borderRadius: "12px",
  boxShadow: "0 20px 40px rgba(0, 0, 0, 0.3)",
  maxWidth: "600px",
  width: "90%",
  maxHeight: "80vh",
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
  animation: `${slideIn} 0.15s ease-out`,
});

export const header = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  padding: "1rem 1.5rem",
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const headerTitle = style({
  fontSize: "1.125rem",
  fontWeight: 600,
  color: vars.color.textPrimary,
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
});

export const vcsIcon = style({
  fontSize: "1.25rem",
});

export const closeButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  padding: "0.25rem",
  color: vars.color.textSecondary,
  borderRadius: "4px",
  transition: "all 0.15s ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
});

export const content = style({
  flex: 1,
  overflowY: "auto",
  padding: "1.5rem",
});

export const currentInfo = style({
  background: vars.color.cardBackground,
  borderRadius: "8px",
  padding: "1rem",
  marginBottom: "1.5rem",
});

export const currentInfoTitle = style({
  fontSize: "0.75rem",
  fontWeight: 600,
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  marginBottom: "0.5rem",
});

export const currentInfoValue = style({
  fontFamily: vars.font.mono,
  fontSize: "0.875rem",
  color: vars.color.textPrimary,
});

export const section = style({
  marginBottom: "1.5rem",
});

export const sectionTitle = style({
  fontSize: "0.875rem",
  fontWeight: 600,
  color: vars.color.textPrimary,
  marginBottom: "0.75rem",
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
});

export const sectionIcon = style({
  fontSize: "1rem",
});

export const targetList = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.25rem",
  maxHeight: "200px",
  overflowY: "auto",
});

export const targetItem = style({
  display: "flex",
  alignItems: "center",
  padding: "0.625rem 0.75rem",
  borderRadius: "6px",
  cursor: "pointer",
  transition: "all 0.15s ease",
  border: "1px solid transparent",
  selectors: {
    "&:hover": {
      background: vars.color.cardBackground,
    },
  },
});

export const selected = style({
  background: vars.color.accentBg,
  borderColor: vars.color.primary,
});

export const current = style({
  background: vars.color.successBg,
  borderColor: vars.color.success,
});

export const targetItemIcon = style({
  marginRight: "0.5rem",
  fontSize: "1rem",
});

export const targetItemName = style({
  flex: 1,
  fontSize: "0.875rem",
  color: vars.color.textPrimary,
  fontFamily: vars.font.mono,
});

export const targetItemMeta = style({
  fontSize: "0.75rem",
  color: vars.color.textSecondary,
  marginLeft: "0.5rem",
});

export const currentBadge = style({
  fontSize: "0.625rem",
  fontWeight: 600,
  textTransform: "uppercase",
  background: vars.color.success,
  color: "white",
  padding: "0.125rem 0.375rem",
  borderRadius: "4px",
  marginLeft: "0.5rem",
});

export const strategySection = style({
  marginTop: "1.5rem",
  paddingTop: "1.5rem",
  borderTop: `1px solid ${vars.color.borderColor}`,
});

export const strategyOptions = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
});

export const strategyOption = style({
  display: "flex",
  alignItems: "flex-start",
  padding: "0.75rem",
  borderRadius: "6px",
  cursor: "pointer",
  border: `1px solid ${vars.color.borderColor}`,
  transition: "all 0.15s ease",
  selectors: {
    "&:hover": {
      background: vars.color.cardBackground,
    },
  },
});

export const strategyRadio = style({
  marginRight: "0.75rem",
  marginTop: "0.125rem",
});

export const strategyContent = style({
  flex: 1,
});

export const strategyLabel = style({
  fontSize: "0.875rem",
  fontWeight: 600,
  color: vars.color.textPrimary,
  marginBottom: "0.25rem",
});

export const strategyDescription = style({
  fontSize: "0.75rem",
  color: vars.color.textSecondary,
  lineHeight: 1.4,
});

export const footer = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  padding: "1rem 1.5rem",
  borderTop: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
});

export const footerButtons = style({
  display: "flex",
  gap: "0.75rem",
});

export const cancelButton = style({
  padding: "0.5rem 1rem",
  borderRadius: "6px",
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.background,
  color: vars.color.textPrimary,
  cursor: "pointer",
  fontSize: "0.875rem",
  fontWeight: 500,
  transition: "all 0.15s ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
  },
});

export const switchButton = style({
  padding: "0.5rem 1rem",
  borderRadius: "6px",
  border: "none",
  background: vars.color.primary,
  color: "white",
  cursor: "pointer",
  fontSize: "0.875rem",
  fontWeight: 500,
  transition: "all 0.15s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.accentHover,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const loading = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  padding: "3rem",
  color: vars.color.textSecondary,
});

export const error = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  gap: "1rem",
  padding: "2rem",
  textAlign: "center",
});

export const errorIcon = style({
  fontSize: "2rem",
});

export const errorMessage = style({
  color: vars.color.error,
  fontSize: "0.875rem",
});

export const retryButton = style({
  padding: "0.5rem 1rem",
  borderRadius: "6px",
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.background,
  color: vars.color.textPrimary,
  cursor: "pointer",
  fontSize: "0.875rem",
});

export const emptyState = style({
  textAlign: "center",
  padding: "1rem",
  color: vars.color.textSecondary,
  fontStyle: "italic",
  fontSize: "0.875rem",
});

export const filterInput = style({
  width: "100%",
  padding: "0.5rem 0.75rem",
  borderRadius: "6px",
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.background,
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  marginBottom: "0.75rem",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
      boxShadow: `0 0 0 2px ${vars.color.accentBg}`,
    },
    "&::placeholder": {
      color: vars.color.textTertiary,
    },
  },
});

export const successBanner = style({
  background: vars.color.successBg,
  border: `1px solid ${vars.color.success}`,
  borderRadius: "6px",
  padding: "0.75rem",
  marginBottom: "1rem",
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  fontSize: "0.8125rem",
  color: "#065f46",
});

export const warningBanner = style({
  background: vars.color.warningBg,
  border: `1px solid ${vars.color.warning}`,
  borderRadius: "6px",
  padding: "0.75rem",
  marginBottom: "1rem",
  display: "flex",
  alignItems: "flex-start",
  gap: "0.5rem",
});

export const warningIcon = style({
  fontSize: "1rem",
});

export const warningText = style({
  fontSize: "0.8125rem",
  color: vars.color.warningText,
  lineHeight: 1.4,
});
