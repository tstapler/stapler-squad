import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import {
  animatedOverlay,
  animatedModal,
  animatedHeader,
  animatedTitle,
  animatedSubtitleBase,
} from "@/styles/modalChrome.css";

export const overlay = animatedOverlay;
export const modal = animatedModal;
export const header = animatedHeader;
export const title = animatedTitle;
export const subtitle = animatedSubtitleBase;

export const body = style({
  padding: "24px",
  overflowY: "auto",
  flex: 1,
});

export const fieldGroup = style({
  marginBottom: "20px",
});

export const fieldLabel = style({
  display: "block",
  margin: "0 0 6px 0",
  fontSize: "13px",
  fontWeight: 600,
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.5px",
});

export const titleInput = style({
  width: "100%",
  padding: "10px 14px",
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: "6px",
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
  fontSize: "14px",
  transition: "all 0.2s ease",
  boxSizing: "border-box",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
      boxShadow: "0 0 0 3px rgba(0, 112, 243, 0.1)",
    },
    "&::placeholder": {
      color: vars.color.textTertiary,
    },
  },
});

export const conflictHint = style({
  display: "block",
  margin: "6px 0 0 0",
  fontSize: "12px",
  color: vars.color.warning,
  fontWeight: 500,
});

export const tagsSection = style({
  marginBottom: "20px",
});

export const tagInputRow = style({
  display: "flex",
  gap: "8px",
  marginBottom: "10px",
});

export const tagInput = style({
  flex: 1,
  padding: "8px 12px",
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: "6px",
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
  fontSize: "13px",
  transition: "all 0.2s ease",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
      boxShadow: "0 0 0 3px rgba(0, 112, 243, 0.1)",
    },
    "&::placeholder": {
      color: vars.color.textTertiary,
    },
  },
});

export const addTagButton = style({
  padding: "8px 16px",
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: "6px",
  fontSize: "13px",
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      background: vars.color.primaryHover,
    },
  },
});

export const tagsList = style({
  display: "flex",
  flexWrap: "wrap",
  gap: "8px",
});

export const tagItem = style({
  display: "flex",
  alignItems: "center",
  gap: "6px",
  padding: "6px 10px",
  background: vars.color.surfaceMuted,
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: "6px",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderStrong,
    },
  },
});

export const tagText = style({
  fontSize: "13px",
  color: vars.color.textPrimary,
  fontWeight: 500,
});

export const removeTagButton = style({
  padding: 0,
  width: "18px",
  height: "18px",
  background: vars.color.errorBg,
  color: vars.color.errorText,
  border: "none",
  borderRadius: "50%",
  fontSize: "18px",
  lineHeight: 1,
  cursor: "pointer",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      background: vars.color.error,
      transform: "scale(1.1)",
    },
  },
});

export const emptyTags = style({
  margin: 0,
  padding: "16px",
  textAlign: "center",
  color: vars.color.textTertiary,
  fontSize: "13px",
});

export const tagError = style({
  margin: "4px 0 0 0",
  color: vars.color.error,
  fontSize: "12px",
  fontWeight: 500,
});

export const contextSection = style({
  marginBottom: 0,
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

export const resumeButton = style({
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
