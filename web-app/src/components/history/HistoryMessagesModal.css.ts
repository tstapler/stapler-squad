import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

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

export const modal = style({
  background: vars.color.background,
  borderRadius: "12px",
  width: "100%",
  maxWidth: "900px",
  maxHeight: "85vh",
  display: "flex",
  flexDirection: "column",
  boxShadow: "0 20px 60px rgba(0, 0, 0, 0.3)",

  "@media": {
    "(max-width: 768px)": {
      maxHeight: "90vh",
    },
  },
});

export const modalHeader = style({
  display: "flex",
  alignItems: "center",
  gap: "16px",
  padding: "20px 24px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  flexShrink: 0,

  "@media": {
    "(max-width: 768px)": {
      flexWrap: "wrap",
      gap: "12px",
    },
  },
});

export const modalTitle = style({
  fontSize: "18px",
  fontWeight: "bold",
  margin: 0,
  color: vars.color.textPrimary,
  flexShrink: 0,
});

export const messageSearchContainer = style({
  flex: 1,
  display: "flex",
  gap: "8px",
  alignItems: "center",

  "@media": {
    "(max-width: 768px)": {
      order: 3,
      width: "100%",
    },
  },
});

export const messageSearchInput = style({
  flex: 1,
  padding: "8px 12px",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: "13px",

  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
    },
  },
});

export const modalCloseButton = style({
  background: "none",
  border: "none",
  fontSize: "24px",
  cursor: "pointer",
  padding: "4px 12px",
  color: vars.color.textSecondary,
  borderRadius: "6px",
  transition: "all 0.2s",
  flexShrink: 0,

  selectors: {
    "&:hover": {
      background: vars.color.borderColor,
      color: vars.color.textPrimary,
    },
    "&:focus": {
      outline: "none",
      boxShadow: "0 0 0 2px rgba(59, 130, 246, 0.2)",
    },
  },
});

export const modalContent = style({
  flex: 1,
  overflowY: "auto",
  padding: "20px 24px",
});

export const messageUser = style({
  marginBottom: "20px",
  padding: "16px",
  background: vars.color.cardBackground,
  borderRadius: "8px",
  borderLeft: `4px solid ${vars.color.primary}`,
});

export const messageAssistant = style({
  marginBottom: "20px",
  padding: "16px",
  background: vars.color.cardBackground,
  borderRadius: "8px",
  borderLeft: `4px solid ${vars.color.textSecondary}`,
});

export const messageHeader = style({
  display: "flex",
  justifyContent: "space-between",
  marginBottom: "12px",
});

export const messageContent = style({
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
  fontSize: "14px",
  lineHeight: "1.6",
  color: vars.color.textPrimary,
});

export const emptyStateContainer = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  padding: "60px 20px",
  textAlign: "center",
  background: vars.color.cardBackground,
  border: `2px dashed ${vars.color.borderColor}`,
  borderRadius: "8px",
});

export const emptyStateIcon = style({
  fontSize: "48px",
  marginBottom: "16px",
});

export const emptyStateTitle = style({
  fontSize: "18px",
  fontWeight: "600",
  color: vars.color.textPrimary,
  margin: "0 0 8px 0",
});

export const linkButton = style({
  background: "none",
  border: "none",
  color: vars.color.primary,
  cursor: "pointer",
  fontSize: "inherit",
  padding: 0,
  textDecoration: "underline",

  selectors: {
    "&:hover": {
      color: vars.color.primaryHover,
    },
  },
});
