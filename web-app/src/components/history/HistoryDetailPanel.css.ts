import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const detailPanel = style({
  width: "380px",
  flexShrink: 0,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  padding: "20px",
  maxHeight: "calc(var(--viewport-height, 100dvh) - 280px)",
  overflowY: "auto",

  "@media": {
    "(max-width: 1024px)": {
      width: "100%",
      maxHeight: "none",
    },
  },
});

export const sectionTitle = style({
  marginBottom: "12px",
  fontSize: "16px",
  fontWeight: "600",
  color: vars.color.textPrimary,
});

export const detailFields = style({
  display: "flex",
  flexDirection: "column",
  gap: "16px",
});

export const detailField = style({});

export const fieldLabel = style({
  fontWeight: "600",
  marginBottom: "4px",
  color: vars.color.textSecondary,
  fontSize: "12px",
  textTransform: "uppercase",
  letterSpacing: "0.5px",
});

export const idField = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
});

export const copyButton = style({
  background: "none",
  border: "none",
  padding: "4px 8px",
  cursor: "pointer",
  borderRadius: "4px",
  transition: "background 0.2s",

  selectors: {
    "&:hover": {
      background: vars.color.borderColor,
    },
  },
});

export const projectPath = style({
  fontFamily: "monospace",
  fontSize: "12px",
  color: vars.color.primary,
  wordBreak: "break-all",
});

export const detailActions = style({
  display: "flex",
  flexDirection: "column",
  gap: "10px",
  marginTop: "8px",
  paddingTop: "16px",
  borderTop: `1px solid ${vars.color.borderColor}`,
});

export const emptyState = style({
  padding: "40px 20px",
  textAlign: "center",
  color: vars.color.textMuted,
});

globalStyle(`${emptyState} p`, { margin: 0 });

export const emptyStateIcon = style({
  fontSize: "48px",
  marginBottom: "16px",
});

export const messagePreview = style({
  marginTop: "16px",
  paddingTop: "16px",
  borderTop: `1px solid ${vars.color.borderColor}`,
});

export const previewHeader = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: "12px",
});

export const previewMessages = style({
  display: "flex",
  flexDirection: "column",
  gap: "8px",
  maxHeight: "300px",
  overflowY: "auto",
});

export const previewMessage = style({
  display: "flex",
  gap: "8px",
  padding: "8px 10px",
  borderRadius: "6px",
  fontSize: "13px",
  lineHeight: "1.4",
});

export const userMessage = style({
  background: "rgba(59, 130, 246, 0.1)",
  border: "1px solid rgba(59, 130, 246, 0.2)",
});

export const assistantMessage = style({
  background: "rgba(34, 197, 94, 0.1)",
  border: "1px solid rgba(34, 197, 94, 0.2)",
});

export const previewRole = style({
  flexShrink: 0,
  fontSize: "14px",
});

export const previewContent = style({
  color: vars.color.textSecondary,
  wordBreak: "break-word",
  whiteSpace: "pre-wrap",
});

export const viewMoreButton = style({
  marginTop: "8px",
  padding: "8px 12px",
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  color: vars.color.primary,
  fontSize: "13px",
  cursor: "pointer",
  transition: "all 0.2s",
  textAlign: "center",

  selectors: {
    "&:hover": {
      background: vars.color.primary,
      color: vars.color.primaryText,
      borderColor: vars.color.primary,
    },
  },
});
