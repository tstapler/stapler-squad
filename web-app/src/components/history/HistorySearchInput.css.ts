import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const spin = keyframes({
  to: { transform: "rotate(360deg)" },
});

export const container = style({
  position: "relative",
  width: "100%",
});

export const inputWrapper = style({
  position: "relative",
  display: "flex",
  alignItems: "center",
});

export const searchIcon = style({
  position: "absolute",
  left: "12px",
  color: vars.color.textSecondary,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "16px",
  height: "16px",
});

export const spinner = style({
  display: "inline-block",
  width: "14px",
  height: "14px",
  border: `2px solid ${vars.color.borderColor}`,
  borderTopColor: vars.color.primary,
  borderRadius: "50%",
  animation: `${spin} 0.8s linear infinite`,
});

export const input = style({
  width: "100%",
  padding: "10px 36px 10px 40px",
  fontSize: "14px",
  lineHeight: "1.5",
  color: vars.color.textPrimary,
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  outline: "none",
  transition: "border-color 0.15s ease, box-shadow 0.15s ease",

  selectors: {
    "&:focus": {
      borderColor: vars.color.primary,
      boxShadow: "0 0 0 3px rgba(59, 130, 246, 0.1)",
    },
    "&::placeholder": {
      color: vars.color.textMuted,
    },
  },
});

export const clearButton = style({
  position: "absolute",
  right: "10px",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "20px",
  height: "20px",
  padding: 0,
  color: vars.color.textSecondary,
  background: "transparent",
  border: "none",
  borderRadius: "4px",
  cursor: "pointer",
  transition: "color 0.15s ease, background-color 0.15s ease",

  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const dropdown = style({
  position: "absolute",
  top: "calc(100% + 4px)",
  left: 0,
  right: 0,
  zIndex: 50,
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  boxShadow: "0 4px 12px rgba(0, 0, 0, 0.1)",
  maxHeight: "300px",
  overflowY: "auto",
});

export const header = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  padding: "8px 12px",
  fontSize: "12px",
  color: vars.color.textSecondary,
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const clearAllButton = style({
  padding: "2px 6px",
  fontSize: "11px",
  color: vars.color.textSecondary,
  background: "transparent",
  border: "none",
  borderRadius: "4px",
  cursor: "pointer",
  transition: "color 0.15s ease, background-color 0.15s ease",

  selectors: {
    "&:hover": {
      color: vars.color.error,
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const items = style({
  padding: "4px 0",
});

export const item = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  padding: "8px 12px",
  cursor: "pointer",
  transition: "background-color 0.15s ease",

  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const removeButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "18px",
  height: "18px",
  padding: 0,
  color: vars.color.textMuted,
  background: "transparent",
  border: "none",
  borderRadius: "4px",
  cursor: "pointer",
  opacity: 0,
  transition: "opacity 0.15s ease, color 0.15s ease",

  selectors: {
    "&:hover": {
      color: vars.color.error,
    },
    [`${item}:hover &`]: {
      opacity: 1,
    },
  },
});

export const selected = style({
  backgroundColor: vars.color.hoverBackground,
});

export const historyIcon = style({
  color: vars.color.textMuted,
  display: "flex",
  alignItems: "center",
  flexShrink: 0,
});

export const query = style({
  flex: 1,
  fontSize: "13px",
  color: vars.color.textPrimary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const timestamp = style({
  fontSize: "11px",
  color: vars.color.textMuted,
  flexShrink: 0,
});
