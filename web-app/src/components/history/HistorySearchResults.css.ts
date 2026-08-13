import { style, keyframes, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const spin = keyframes({
  to: { transform: "rotate(360deg)" },
});

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: "16px",
});

export const header = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  padding: "8px 0",
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const resultCount = style({
  fontSize: "14px",
  fontWeight: "500",
  color: vars.color.textPrimary,
});

export const queryTime = style({
  fontSize: "12px",
  color: vars.color.textMuted,
});

export const results = style({
  display: "flex",
  flexDirection: "column",
  gap: "12px",
});

export const resultCard = style({
  padding: "16px",
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  cursor: "pointer",
  transition: "border-color 0.15s ease, box-shadow 0.15s ease",

  selectors: {
    "&:hover": {
      borderColor: vars.color.primary,
      boxShadow: "0 2px 8px rgba(0, 0, 0, 0.05)",
    },
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
      boxShadow: "0 0 0 3px rgba(59, 130, 246, 0.1)",
    },
  },
});

export const resultHeader = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  marginBottom: "8px",
});

export const sessionName = style({
  fontSize: "15px",
  fontWeight: "600",
  color: vars.color.textPrimary,
  margin: 0,
  flex: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const modelBadge = style({
  padding: "2px 8px",
  fontSize: "11px",
  fontWeight: "500",
  color: vars.color.primary,
  backgroundColor: "rgba(59, 130, 246, 0.1)",
  borderRadius: "12px",
  flexShrink: 0,
});

export const resultMeta = style({
  display: "flex",
  alignItems: "center",
  gap: "12px",
  marginBottom: "12px",
  fontSize: "12px",
  color: vars.color.textSecondary,
});

export const projectPath = style({
  display: "flex",
  alignItems: "center",
  gap: "4px",
  maxWidth: "200px",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

globalStyle(`${projectPath} svg`, { flexShrink: 0, opacity: 0.7 });

export const date = style({
  color: vars.color.textMuted,
});

export const score = style({
  marginLeft: "auto",
  padding: "2px 6px",
  fontSize: "11px",
  color: vars.color.success,
  backgroundColor: "rgba(16, 185, 129, 0.1)",
  borderRadius: "4px",
});

export const snippets = style({
  display: "flex",
  flexDirection: "column",
  gap: "8px",
});

export const snippet = style({
  display: "flex",
  gap: "8px",
  padding: "8px 12px",
  backgroundColor: vars.color.hoverBackground,
  borderRadius: "6px",
  fontSize: "13px",
  lineHeight: "1.5",
});

export const snippetRole = style({
  flexShrink: 0,
  fontSize: "11px",
  fontWeight: "600",
  textTransform: "uppercase",
  color: vars.color.textMuted,
  minWidth: "48px",
});

export const snippetText = style({
  color: vars.color.textSecondary,
  wordBreak: "break-word",
});

export const highlight = style({
  backgroundColor: "rgba(250, 204, 21, 0.4)",
  color: vars.color.textPrimary,
  padding: "1px 2px",
  borderRadius: "2px",
});

export const loadMoreContainer = style({
  display: "flex",
  justifyContent: "center",
  padding: "16px 0",
});

export const loadMoreButton = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  padding: "10px 20px",
  fontSize: "14px",
  fontWeight: "500",
  color: vars.color.primary,
  backgroundColor: "transparent",
  border: `1px solid ${vars.color.primary}`,
  borderRadius: "8px",
  cursor: "pointer",
  transition: "background-color 0.15s ease, color 0.15s ease",

  selectors: {
    "&:hover:not(:disabled)": {
      backgroundColor: vars.color.primary,
      color: "white",
    },
    "&:disabled": {
      opacity: 0.6,
      cursor: "not-allowed",
    },
  },
});

export const remainingCount = style({
  fontSize: "12px",
  opacity: 0.8,
});

export const buttonSpinner = style({
  display: "inline-block",
  width: "14px",
  height: "14px",
  border: "2px solid currentColor",
  borderTopColor: "transparent",
  borderRadius: "50%",
  animation: `${spin} 0.8s linear infinite`,
});

export const emptyState = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  padding: "48px 24px",
  textAlign: "center",
});

export const emptyIcon = style({
  color: vars.color.textMuted,
  marginBottom: "16px",
});

export const emptyText = style({
  fontSize: "16px",
  fontWeight: "500",
  color: vars.color.textPrimary,
  margin: "0 0 8px",
});

export const emptyHint = style({
  fontSize: "14px",
  color: vars.color.textSecondary,
  margin: 0,
});

export const loadingState = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  padding: "48px 24px",
  gap: "12px",
});

export const loadingSpinner = style({
  width: "32px",
  height: "32px",
  border: `3px solid ${vars.color.borderColor}`,
  borderTopColor: vars.color.primary,
  borderRadius: "50%",
  animation: `${spin} 0.8s linear infinite`,
});

export const errorState = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  padding: "48px 24px",
  gap: "12px",
  textAlign: "center",
});

export const errorIcon = style({
  color: vars.color.error,
});

export const errorText = style({
  fontSize: "14px",
  color: vars.color.error,
  margin: 0,
});
