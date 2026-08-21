import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const step = style({
  display: "flex",
  flexDirection: "column",
  gap: "1.5rem",
  "@media": {
    "screen and (max-width: 768px)": {
      gap: "1.25rem",
    },
  },
});

globalStyle(`${step} h2`, { margin: 0, fontSize: "1.5rem", fontWeight: 600, color: vars.color.textPrimary });

export const description = style({
  margin: 0,
  color: vars.color.textSecondary,
  fontSize: "0.9375rem",
  "@media": {
    "screen and (max-width: 768px)": {
      fontSize: "0.875rem",
    },
  },
});

export const field = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
});

globalStyle(`${field} label`, {
  fontWeight: 500,
  fontSize: "0.9375rem",
  color: vars.color.textPrimary,
});

globalStyle(`${field} input[type="text"]`, {
  padding: "0.625rem 0.875rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  fontSize: "0.9375rem",
  transition: "border-color 0.2s, box-shadow 0.2s",
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
});

globalStyle(`${field} input[type="number"]`, {
  padding: "0.625rem 0.875rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  fontSize: "0.9375rem",
  transition: "border-color 0.2s, box-shadow 0.2s",
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
});

globalStyle(`${field} select`, {
  padding: "0.625rem 0.875rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  fontSize: "0.9375rem",
  transition: "border-color 0.2s, box-shadow 0.2s",
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
});

globalStyle(`${field} textarea`, {
  padding: "0.625rem 0.875rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  fontSize: "0.9375rem",
  transition: "border-color 0.2s, box-shadow 0.2s",
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
});

globalStyle(`${field} input[type="text"]::placeholder`, {
  color: vars.color.placeholderColor,
  opacity: 1,
});

globalStyle(`${field} input[type="number"]::placeholder`, {
  color: vars.color.placeholderColor,
  opacity: 1,
});

globalStyle(`${field} textarea::placeholder`, {
  color: vars.color.placeholderColor,
  opacity: 1,
});

globalStyle(`${field} input:focus`, {
  outline: "none",
  borderColor: vars.color.primary,
  boxShadow: "0 0 0 3px rgba(0, 112, 243, 0.1)",
});

globalStyle(`${field} select:focus`, {
  outline: "none",
  borderColor: vars.color.primary,
  boxShadow: "0 0 0 3px rgba(0, 112, 243, 0.1)",
});

globalStyle(`${field} textarea:focus`, {
  outline: "none",
  borderColor: vars.color.primary,
  boxShadow: "0 0 0 3px rgba(0, 112, 243, 0.1)",
});

export const required = style({
  color: vars.color.error,
});

export const error = style({
  borderColor: `${vars.color.error} !important`,
  selectors: {
    "&:focus": {
      boxShadow: "0 0 0 3px rgba(239, 68, 68, 0.1) !important",
    },
  },
});

export const errorMessage = style({
  color: vars.color.error,
  fontSize: "0.875rem",
});

export const hint = style({
  color: vars.color.textMuted,
  fontSize: "0.875rem",
});

export const checkbox = style({
  display: "flex",
  alignItems: "center",
  gap: "0.625rem",
  cursor: "pointer",
});

globalStyle(`${checkbox} input[type="checkbox"]`, {
  width: "1.125rem",
  height: "1.125rem",
  cursor: "pointer",
});

globalStyle(`${checkbox} span`, { fontWeight: 500, color: vars.color.textPrimary });

export const branchPreview = style({
  display: "flex",
  alignItems: "center",
  gap: "0.75rem",
  padding: "0.625rem 0.875rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
});

export const branchPreviewName = style({
  flex: 1,
  fontFamily: "monospace",
  fontSize: "0.9375rem",
  color: vars.color.textSecondary,
});

export const branchCustomizeButton = style({
  background: "none",
  border: "none",
  color: vars.color.primary,
  fontSize: "0.875rem",
  cursor: "pointer",
  padding: 0,
  whiteSpace: "nowrap",
  textDecoration: "underline",
  selectors: {
    "&:hover": {
      opacity: 0.75,
    },
  },
});

export const branchCustomHint = style({
  display: "flex",
  justifyContent: "flex-end",
});

export const buttonPrimary = style({
  padding: "0.625rem 1.5rem",
  borderRadius: "6px",
  fontWeight: 500,
  fontSize: "0.9375rem",
  cursor: "pointer",
  transition: "all 0.2s",
  border: "none",
  background: vars.color.primary,
  color: "white",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.primaryDark,
      transform: "translateY(-1px)",
      boxShadow: "0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06)",
    },
    "&:active:not(:disabled)": {
      transform: "translateY(0)",
    },
    "&:disabled": {
      opacity: 0.6,
      cursor: "not-allowed",
    },
  },
});

export const buttonSecondary = style({
  padding: "0.625rem 1.5rem",
  borderRadius: "6px",
  fontWeight: 500,
  fontSize: "0.9375rem",
  cursor: "pointer",
  transition: "all 0.2s",
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.surfaceSubtle,
      borderColor: vars.color.borderStrong,
    },
    "&:disabled": {
      opacity: 0.6,
      cursor: "not-allowed",
    },
  },
});

export const reviewSection = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.75rem",
  padding: "1.25rem",
  background: vars.color.surfaceSubtle,
  borderRadius: "8px",
  border: `1px solid ${vars.color.borderColor}`,
});

globalStyle(`${reviewSection} h3`, {
  margin: 0,
  fontSize: "1rem",
  fontWeight: 600,
  color: vars.color.textPrimary,
  paddingBottom: "0.5rem",
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const reviewItem = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.25rem",
});

export const reviewLabel = style({
  fontSize: "0.875rem",
  fontWeight: 500,
  color: vars.color.textSecondary,
});

export const reviewValue = style({
  fontSize: "0.9375rem",
  color: vars.color.textPrimary,
  padding: "0.5rem 0.75rem",
  background: "white",
  borderRadius: "4px",
  border: `1px solid ${vars.color.borderColor}`,
  wordBreak: "break-word",
});

export const submitError = style({
  padding: "1rem",
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: "6px",
  color: vars.color.errorDark,
  fontSize: "0.9375rem",
  marginTop: "1rem",
});

globalStyle(`${submitError} strong`, { fontWeight: 600 });

export const defaultsNotice = style({
  display: "inline-block",
  fontSize: "0.75rem",
  fontWeight: 400,
  color: vars.color.textMuted,
  fontStyle: "italic",
  marginLeft: "0.75rem",
  verticalAlign: "middle",
});

export const successMessage = style({
  padding: "0.75rem 1rem",
  background: vars.color.successBg,
  border: `1px solid ${vars.color.success}`,
  borderRadius: "6px",
  color: vars.color.success,
  fontSize: "0.875rem",
});

export const modalOverlay = style({
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
});

export const modalContent = style({
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: "8px",
  padding: "1.5rem",
  maxWidth: "480px",
  width: "90%",
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
});

globalStyle(`${modalContent} h3`, {
  margin: 0,
  fontSize: "1.25rem",
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const modalActions = style({
  display: "flex",
  justifyContent: "flex-end",
  gap: "0.75rem",
  marginTop: "0.5rem",
});
