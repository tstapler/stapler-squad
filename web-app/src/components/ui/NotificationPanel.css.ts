import { style, keyframes, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const fadeIn = keyframes({
  from: { opacity: 0 },
  to: { opacity: 1 },
});

export const overlay = style({
  position: "fixed",
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  backgroundColor: vars.color.overlayBackground,
  zIndex: 9998,
  animation: `${fadeIn} 0.3s ease-out`,
});

export const panel = style({
  position: "fixed",
  top: 0,
  right: 0,
  bottom: 0,
  width: "400px",
  maxWidth: "90vw",
  backgroundColor: vars.color.background,
  boxShadow: "-2px 0 10px rgba(0, 0, 0, 0.2)",
  zIndex: 9999,
  transform: "translateX(100%)",
  transition: "transform 0.3s ease-out",
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
  "@media": {
    "screen and (max-width: 768px)": {
      width: "100%",
      maxWidth: "100vw",
    },
  },
});

export const panelOpen = style({
  transform: "translateX(0)",
});

export const header = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "1rem 1.25rem",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  backgroundColor: vars.color.cardBackground,
  flexShrink: 0,
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.75rem 1rem",
    },
  },
});

export const title = style({
  fontSize: "1.25rem",
  fontWeight: 600,
  margin: 0,
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  color: vars.color.textPrimary,
  "@media": {
    "screen and (max-width: 768px)": {
      fontSize: "1.125rem",
    },
  },
});

export const unreadBadge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "1.5rem",
  height: "1.5rem",
  padding: "0 0.5rem",
  backgroundColor: vars.color.error,
  color: "white",
  borderRadius: "12px",
  fontSize: "0.75rem",
  fontWeight: 600,
});

export const headerActions = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
});

export const markAllButton = style({
  padding: "0.5rem 0.75rem",
  backgroundColor: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  cursor: "pointer",
  fontSize: "0.875rem",
  color: vars.color.textPrimary,
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const clearButton = style({
  padding: "0.5rem 0.75rem",
  backgroundColor: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  cursor: "pointer",
  fontSize: "0.875rem",
  color: vars.color.error,
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const closeButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "2rem",
  height: "2rem",
  backgroundColor: "transparent",
  border: "none",
  borderRadius: "50%",
  cursor: "pointer",
  fontSize: "1.5rem",
  color: vars.color.textSecondary,
  transition: "background-color 0.2s ease",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const content = style({
  flex: 1,
  overflowY: "auto",
  padding: 0,
});

export const empty = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  padding: "2rem",
  textAlign: "center",
  color: vars.color.textSecondary,
});

export const emptyIcon = style({
  fontSize: "4rem",
  marginBottom: "1rem",
  opacity: 0.3,
});

export const emptyText = style({
  fontSize: "1.125rem",
  fontWeight: 500,
  margin: "0 0 0.5rem 0",
  color: vars.color.textPrimary,
});

export const emptySubtext = style({
  fontSize: "0.875rem",
  margin: 0,
  color: vars.color.textSecondary,
});

export const list = style({
  display: "flex",
  flexDirection: "column",
});

export const item = style({
  padding: "1rem 1.25rem",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  borderLeft: "3px solid var(--priority-color)",
  backgroundColor: vars.color.background,
  transition: "background-color 0.2s ease",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.875rem 1rem",
    },
  },
});

export const unread = style({
  backgroundColor: "rgba(0, 112, 243, 0.07)",
});

export const read = style({
  opacity: 0.7,
});

export const itemHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  marginBottom: "0.5rem",
});

export const itemTitle = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  fontSize: "0.875rem",
  color: vars.color.textPrimary,
  flexWrap: "wrap",
  flex: 1,
  minWidth: 0,
});

globalStyle(`${itemTitle} strong`, {
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
  flex: "0 1 auto",
  minWidth: 0,
});

export const typeIcon = style({
  fontSize: "1rem",
  flexShrink: 0,
});

export const typeLabel = style({
  fontSize: "0.625rem",
  fontWeight: 600,
  textTransform: "uppercase",
  letterSpacing: "0.3px",
  padding: "2px 4px",
  borderRadius: "3px",
  color: "white",
  whiteSpace: "nowrap",
  flexShrink: 0,
});

export const itemContext = style({
  fontSize: "0.75rem",
  color: vars.color.textSecondary,
  marginBottom: "0.5rem",
  fontWeight: 500,
});

export const itemWorkingDir = style({
  fontSize: "0.75rem",
  color: vars.color.textSecondary,
  marginBottom: "0.5rem",
  display: "flex",
  alignItems: "center",
  gap: "0.25rem",
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
});

export const itemActions = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
});

export const focusButton = style({
  padding: "0.375rem 0.5rem",
  backgroundColor: "transparent",
  color: vars.color.primary,
  border: `1px solid ${vars.color.primary}`,
  borderRadius: "6px",
  cursor: "pointer",
  fontSize: "0.75rem",
  fontWeight: 500,
  transition: "all 0.2s ease",
  whiteSpace: "nowrap",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.primary,
      color: "white",
    },
  },
});

export const unreadDot = style({
  display: "inline-block",
  width: "8px",
  height: "8px",
  backgroundColor: vars.color.primary,
  borderRadius: "50%",
});

export const removeButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "44px",
  minHeight: "44px",
  backgroundColor: "transparent",
  border: "none",
  borderRadius: "50%",
  cursor: "pointer",
  fontSize: "1rem",
  color: vars.color.textMuted,
  transition: "all 0.2s ease",
  opacity: 0,
  flexShrink: 0,
  selectors: {
    [`${item}:hover &`]: {
      opacity: 1,
    },
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      opacity: 1,
    },
  },
});

export const itemMessage = style({
  margin: "0 0 0.75rem 0",
  fontSize: "0.875rem",
  lineHeight: 1.4,
  color: vars.color.textPrimary,
});

export const itemFooter = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: "0.5rem",
});

export const timestamp = style({
  fontSize: "0.75rem",
  color: vars.color.textMuted,
});

export const viewButton = style({
  padding: "0.375rem 0.75rem",
  backgroundColor: vars.color.primary,
  color: "white",
  border: "none",
  borderRadius: "6px",
  cursor: "pointer",
  fontSize: "0.75rem",
  fontWeight: 500,
  transition: "background-color 0.2s ease",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.primaryHover,
    },
  },
});

export const loadMore = style({
  display: "flex",
  justifyContent: "center",
  padding: "1rem",
});

export const loadMoreButton = style({
  padding: "0.5rem 1.5rem",
  backgroundColor: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  cursor: "pointer",
  fontSize: "0.875rem",
  color: vars.color.textPrimary,
  transition: "all 0.2s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      backgroundColor: vars.color.hoverBackground,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "not-allowed",
    },
  },
});

export const itemSubtitle = style({
  fontSize: "0.8rem",
  fontWeight: 500,
  color: vars.color.textSecondary,
  marginBottom: "0.125rem",
});

export const approvalDetails = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.25rem",
  margin: "0.375rem 0",
  padding: "0.5rem 0.625rem",
  backgroundColor: "rgba(0, 0, 0, 0.04)",
  borderLeft: `2px solid ${vars.color.warning}`,
  borderRadius: "0 4px 4px 0",
  fontSize: "0.8rem",
});

export const approvalTool = style({
  color: vars.color.textSecondary,
  fontWeight: 500,
});

export const approvalCommand = style({
  fontFamily: "'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace",
  fontSize: "0.78rem",
  color: vars.color.textPrimary,
  background: "transparent",
  whiteSpace: "pre-wrap",
  wordBreak: "break-all",
  padding: 0,
});

export const approvalCwd = style({
  color: vars.color.textMuted,
  fontSize: "0.75rem",
});

export const approveButton = style({
  padding: "0.25rem 0.625rem",
  fontSize: "0.78rem",
  fontWeight: 600,
  border: `1px solid ${vars.color.success}`,
  borderRadius: "4px",
  background: "transparent",
  color: vars.color.success,
  cursor: "pointer",
  transition: "background-color 0.15s, color 0.15s, opacity 0.15s",
  selectors: {
    "&:hover:not(:disabled)": {
      backgroundColor: vars.color.success,
      color: "white",
    },
    "&:disabled": {
      opacity: 0.45,
      cursor: "not-allowed",
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.75rem 1rem",
      minHeight: "44px",
    },
  },
});

export const denyButton = style({
  padding: "0.25rem 0.625rem",
  fontSize: "0.78rem",
  fontWeight: 600,
  border: `1px solid ${vars.color.error}`,
  borderRadius: "4px",
  background: "transparent",
  color: vars.color.error,
  cursor: "pointer",
  transition: "background-color 0.15s, color 0.15s, opacity 0.15s",
  selectors: {
    "&:hover:not(:disabled)": {
      backgroundColor: vars.color.error,
      color: "white",
    },
    "&:disabled": {
      opacity: 0.45,
      cursor: "not-allowed",
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.75rem 1rem",
      minHeight: "44px",
    },
  },
});

export const resolvedBadge = style({
  display: "inline-flex",
  alignItems: "center",
  padding: "0.2rem 0.5rem",
  fontSize: "0.75rem",
  fontWeight: 600,
  borderRadius: "4px",
  border: "1px solid transparent",
  whiteSpace: "nowrap",
});

export const countBadge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "1.25rem",
  height: "1.25rem",
  padding: "0 0.375rem",
  backgroundColor: vars.color.textSecondary,
  color: vars.color.background,
  borderRadius: "10px",
  fontSize: "0.6875rem",
  fontWeight: 600,
  marginLeft: "0.25rem",
});

export const filterBar = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
  padding: "0.75rem 1.25rem",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  backgroundColor: vars.color.cardBackground,
  flexShrink: 0,
});

export const searchInput = style({
  width: "100%",
  padding: "0.5rem 0.75rem",
  fontSize: "0.875rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  backgroundColor: vars.color.background,
  color: vars.color.textPrimary,
  outline: "none",
  transition: "border-color 0.15s ease, box-shadow 0.15s ease",
  boxSizing: "border-box",
  selectors: {
    "&::placeholder": {
      color: vars.color.textMuted,
    },
    "&:focus": {
      borderColor: vars.color.primary,
      boxShadow: `0 0 0 2px color-mix(in srgb, ${vars.color.primary} 20%, transparent)`,
    },
  },
});

export const filterPills = style({
  display: "flex",
  gap: "0.375rem",
  flexWrap: "wrap",
});

export const filterPill = style({
  padding: "0.25rem 0.625rem",
  fontSize: "0.75rem",
  fontWeight: 500,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "12px",
  background: "transparent",
  color: vars.color.textSecondary,
  cursor: "pointer",
  transition: "all 0.15s ease",
  whiteSpace: "nowrap",
  selectors: {
    "&:hover": {
      borderColor: vars.color.primary,
      color: vars.color.primary,
    },
  },
});

export const filterPillActive = style({
  backgroundColor: vars.color.primary,
  borderColor: vars.color.primary,
  color: "white",
});

// Auto-handled (auto_approved) collapsible section

export const autoHandledSection = style({
  borderTop: `1px solid ${vars.color.borderColor}`,
  backgroundColor: vars.color.cardBackground,
  flexShrink: 0,
});

export const autoHandledHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  width: "100%",
  padding: "0.625rem 1.25rem",
  backgroundColor: "transparent",
  border: "none",
  cursor: "pointer",
  fontSize: "0.8125rem",
  fontWeight: 500,
  color: vars.color.textSecondary,
  textAlign: "left",
  transition: "background-color 0.15s ease",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const autoHandledHeaderLeft = style({
  display: "flex",
  alignItems: "center",
  gap: "0.375rem",
});

export const autoHandledBadge = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "1.25rem",
  height: "1.25rem",
  padding: "0 0.375rem",
  backgroundColor: vars.color.textSecondary,
  color: vars.color.background,
  borderRadius: "10px",
  fontSize: "0.6875rem",
  fontWeight: 600,
});

export const autoHandledChevron = style({
  fontSize: "0.625rem",
  color: vars.color.textMuted,
  transition: "transform 0.2s ease",
  flexShrink: 0,
});

export const autoHandledChevronOpen = style({
  transform: "rotate(180deg)",
});

export const autoHandledList = style({
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
  maxHeight: "40vh",
  overflowY: "auto",
});

export const autoHandledItem = style({
  display: "flex",
  alignItems: "flex-start",
  gap: "0.5rem",
  padding: "0.5rem 1.25rem",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  fontSize: "0.8125rem",
  color: vars.color.textSecondary,
  opacity: 0.75,
  selectors: {
    "&:last-child": {
      borderBottom: "none",
    },
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
      opacity: 1,
    },
  },
});

export const autoHandledDecision = style({
  fontSize: "0.75rem",
  fontWeight: 600,
  flexShrink: 0,
  marginTop: "0.0625rem",
});

export const autoHandledContent = style({
  flex: 1,
  minWidth: 0,
});

export const autoHandledTitle = style({
  fontWeight: 500,
  color: vars.color.textPrimary,
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
});

export const autoHandledMeta = style({
  fontSize: "0.75rem",
  color: vars.color.textMuted,
  display: "flex",
  gap: "0.5rem",
  flexWrap: "wrap",
  marginTop: "0.125rem",
});

export const autoHandledTimestamp = style({
  fontSize: "0.75rem",
  color: vars.color.textMuted,
  flexShrink: 0,
  marginTop: "0.0625rem",
});
