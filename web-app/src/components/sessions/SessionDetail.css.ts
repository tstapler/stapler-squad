import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const tabFadeIn = keyframes({
  from: { opacity: 0, transform: "translateY(4px)" },
  to: { opacity: 1, transform: "translateY(0)" },
});

export const container = style({
  display: "flex",
  flexDirection: "column",
  height: "calc(var(--viewport-height, 100dvh) - var(--header-height))",
  minHeight: 0,
  overflow: "hidden",
  background: vars.color.terminalBackground,
});

export const fullscreen = style({
  height: "var(--viewport-height, 100dvh)",
});

export const header = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "1.5rem",
  borderBottom: `1px solid ${vars.color.terminalBorder}`,
  background: vars.color.terminalHeaderBg,
  position: "sticky",
  top: 0,
  zIndex: 10,
  flexShrink: 0,
  selectors: {
    [`.${fullscreen} &`]: {
      padding: "0.5rem 1rem",
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.75rem 1rem",
      flexWrap: "wrap",
      rowGap: "0.5rem",
    },
  },
});

export const title = style({
  margin: 0,
  fontSize: "1.5rem",
  fontWeight: 600,
  color: vars.color.terminalHeaderFg,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  selectors: {
    [`.${fullscreen} &`]: {
      fontSize: "1rem",
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      fontSize: "1.125rem",
      flex: "1 1 100%",
      maxWidth: "calc(100vw - 120px)",
    },
  },
});

export const statusBadge = style({
  display: "inline-flex",
  alignItems: "center",
  padding: "0.125rem 0.5rem",
  borderRadius: "9999px",
  fontSize: "0.75rem",
  fontWeight: 600,
  flexShrink: 0,
  marginLeft: "0.5rem",
  background: vars.color.terminalHoverBg,
  color: vars.color.terminalTextMuted,
  border: `1px solid ${vars.color.terminalBorder}`,
});

export const headerActions = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  "@media": {
    "screen and (max-width: 768px)": {
      width: "100%",
    },
  },
});

export const fullscreenButton = style({
  background: "transparent",
  border: "none",
  fontSize: "1.25rem",
  cursor: "pointer",
  color: vars.color.terminalTextMuted,
  padding: "0.5rem",
  lineHeight: 1,
  transition: "color 0.2s, background 0.2s",
  borderRadius: "4px",
  minWidth: "var(--min-touch-target, 44px)",
  minHeight: "var(--min-touch-target, 44px)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  selectors: {
    "&:hover": {
      color: vars.color.terminalHeaderFg,
      background: vars.color.terminalHoverBg,
    },
  },
});

export const switchWorkspaceButton = style({
  display: "flex",
  alignItems: "center",
  gap: "0.25rem",
  background: vars.color.primary,
  border: "none",
  fontSize: "0.875rem",
  fontWeight: 500,
  cursor: "pointer",
  color: "white",
  padding: "0.375rem 0.75rem",
  lineHeight: 1,
  transition: "background 0.2s, transform 0.1s",
  borderRadius: "6px",
  selectors: {
    "&:hover": {
      background: vars.color.primaryDark,
      transform: "translateY(-1px)",
    },
    "&:active": {
      transform: "translateY(0)",
    },
  },
});

export const queuePosition = style({
  fontSize: "0.8rem",
  color: vars.color.terminalTextMuted,
  padding: "0.25rem 0.5rem",
  background: vars.color.terminalHoverBg,
  borderRadius: "4px",
  whiteSpace: "nowrap",
  userSelect: "none",
});

export const navButton = style({
  background: "transparent",
  border: "none",
  fontSize: "1.25rem",
  cursor: "pointer",
  color: vars.color.terminalTextMuted,
  padding: "0.5rem",
  lineHeight: 1,
  transition: "color 0.2s, background 0.2s",
  borderRadius: "4px",
  fontWeight: "bold",
  minWidth: "var(--min-touch-target, 44px)",
  minHeight: "var(--min-touch-target, 44px)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  selectors: {
    "&:hover": {
      color: vars.color.terminalHeaderFg,
      background: vars.color.terminalHoverBg,
    },
    "&:disabled": {
      opacity: 0.3,
      cursor: "not-allowed",
    },
  },
});

export const closeButton = style({
  background: "transparent",
  border: "none",
  fontSize: "1.5rem",
  cursor: "pointer",
  color: vars.color.terminalTextMuted,
  padding: "0.5rem",
  lineHeight: 1,
  transition: "color 0.2s",
  minWidth: "var(--min-touch-target, 44px)",
  minHeight: "var(--min-touch-target, 44px)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  selectors: {
    "&:hover": {
      color: vars.color.terminalHeaderFg,
    },
  },
});

export const tabs = style({
  display: "flex",
  borderBottom: `1px solid ${vars.color.terminalBorder}`,
  background: vars.color.terminalTabsBg,
  padding: "0 1rem",
  gap: "0.5rem",
  "@media": {
    "screen and (max-width: 768px)": {
      overflowX: "auto",
      padding: "0 0.5rem",
      gap: "0.25rem",
    },
  },
});

export const tab = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "1rem 1.5rem",
  border: "none",
  background: "transparent",
  color: vars.color.terminalTextMuted,
  fontSize: "0.95rem",
  fontWeight: 500,
  cursor: "pointer",
  transition: "color 0.2s, border-color 0.2s",
  borderBottom: "2px solid transparent",
  position: "relative",
  selectors: {
    "&:hover": {
      color: vars.color.terminalHeaderFg,
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.75rem 1rem",
      fontSize: "0.875rem",
      whiteSpace: "nowrap",
    },
  },
});

export const active = style({
  color: vars.color.primary,
  borderBottomColor: vars.color.primary,
});

export const tabIcon = style({
  fontSize: "1.1rem",
  lineHeight: 1,
  "@media": {
    "screen and (max-width: 768px)": {
      fontSize: "1rem",
    },
  },
});

export const tabLabel = style({
  lineHeight: 1,
});

export const content = style({
  flex: 1,
  minHeight: 0,
  overflowY: "auto",
  padding: "1.5rem",
  display: "flex",
  flexDirection: "column",
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "1rem",
    },
  },
});

export const fullscreenContent = style({
  padding: 0,
  overflow: "hidden",
});

export const tabContent = style({
  flex: 1,
  minHeight: 0,
  height: "100%",
  display: "flex",
  flexDirection: "column",
  animation: `${tabFadeIn} 0.2s ease-out both`,
});

export const placeholder = style({
  padding: "2rem",
  textAlign: "center",
  color: vars.color.terminalTextMuted,
  fontStyle: "italic",
});

export const infoGrid = style({
  display: "grid",
  gridTemplateColumns: "repeat(auto-fit, minmax(300px, 1fr))",
  gap: "1.5rem",
  "@media": {
    "screen and (max-width: 768px)": {
      gridTemplateColumns: "1fr",
      gap: "1rem",
    },
  },
});

export const infoItem = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
  padding: "1rem",
  background: vars.color.terminalTabsBg,
  borderRadius: "8px",
  border: `1px solid ${vars.color.terminalBorder}`,
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.75rem",
    },
  },
});

export const infoLabel = style({
  fontSize: "0.875rem",
  fontWeight: 600,
  color: vars.color.terminalTextMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
});

export const infoValue = style({
  fontSize: "1rem",
  color: vars.color.terminalForeground,
  wordBreak: "break-all",
  fontWeight: 500,
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
});

export const editButton = style({
  background: "transparent",
  border: "none",
  cursor: "pointer",
  padding: "0.25rem",
  fontSize: "0.875rem",
  opacity: 0.6,
  transition: "opacity 0.2s",
  selectors: {
    "&:hover": {
      opacity: 1,
    },
  },
});

export const editContainer = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  width: "100%",
});

export const editInput = style({
  flex: 1,
  padding: "0.5rem",
  fontSize: "1rem",
  border: `1px solid ${vars.color.terminalBorder}`,
  borderRadius: "4px",
  background: vars.color.terminalBackground,
  color: vars.color.terminalForeground,
  fontFamily: "inherit",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
    },
  },
});

export const saveButton = style({
  padding: "0.5rem 0.75rem",
  border: "none",
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "1rem",
  fontWeight: 600,
  transition: "background 0.2s, transform 0.1s",
  background: vars.color.primary,
  color: "white",
  selectors: {
    "&:hover": {
      background: vars.color.primaryDark,
      transform: "translateY(-1px)",
    },
    "&:active": {
      transform: "translateY(0)",
    },
  },
});

export const cancelButton = style({
  padding: "0.5rem 0.75rem",
  border: "none",
  borderRadius: "4px",
  cursor: "pointer",
  fontSize: "1rem",
  fontWeight: 600,
  transition: "background 0.2s, transform 0.1s",
  background: vars.color.terminalTabsBg,
  color: vars.color.terminalTextMuted,
  selectors: {
    "&:hover": {
      background: vars.color.terminalHoverBg,
      color: vars.color.terminalForeground,
    },
    "&:active": {
      transform: "translateY(0)",
    },
  },
});

export const noTerminalPlaceholder = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  height: "100%",
  minHeight: "200px",
  gap: "0.75rem",
  padding: "2rem",
  textAlign: "center",
  color: vars.color.terminalTextMuted,
});

export const noTerminalIcon = style({
  fontSize: "2.5rem",
  opacity: 0.5,
});

export const noTerminalText = style({
  margin: 0,
  fontSize: "1.125rem",
  fontWeight: 600,
  color: vars.color.terminalForeground,
  opacity: 0.7,
});

export const noTerminalSubtext = style({
  margin: 0,
  fontSize: "0.875rem",
  color: vars.color.terminalTextMuted,
  maxWidth: "400px",
  lineHeight: 1.5,
});

// ⋯ More actions button in header
export const moreActionsButton = style({
  background: "transparent",
  border: "none",
  fontSize: "1.5rem",
  cursor: "pointer",
  color: vars.color.terminalTextMuted,
  padding: "0.5rem",
  lineHeight: 1,
  transition: "color 0.2s, background 0.2s",
  borderRadius: "4px",
  minWidth: "var(--min-touch-target, 44px)",
  minHeight: "var(--min-touch-target, 44px)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  selectors: {
    "&:hover": {
      color: vars.color.terminalHeaderFg,
      background: vars.color.terminalHoverBg,
    },
  },
});

// Action sheet list
export const actionSheet = style({
  display: "flex",
  flexDirection: "column",
  gap: "4px",
  marginTop: "0.5rem",
});

export const actionSheetItem = style({
  minHeight: "52px",
  padding: "0 16px",
  textAlign: "left",
  fontSize: "16px",
  borderRadius: "8px",
  background: "transparent",
  border: "none",
  cursor: "pointer",
  color: vars.color.textPrimary,
  display: "flex",
  alignItems: "center",
  transition: "background 0.12s",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
    "&:active": {
      background: vars.color.hoverBackground,
    },
  },
});

export const actionSheetItemDestructive = style({
  color: vars.color.error,
});

export const actionDivider = style({
  border: "none",
  borderTop: `1px solid ${vars.color.borderColor}`,
  margin: "8px 0",
});

// Generic action buttons for confirm dialogs
export const actionButton = style({
  padding: "0.5rem 1rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  cursor: "pointer",
  fontSize: "0.875rem",
  transition: "background 0.15s",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
  },
});

export const actionButtonDanger = style({
  background: vars.color.error,
  color: "white",
  border: "none",
  selectors: {
    "&:hover": {
      opacity: 0.9,
    },
  },
});

export const actionButtonSave = style({
  background: vars.color.primary,
  color: "white",
  border: "none",
  selectors: {
    "&:hover": {
      background: vars.color.primaryDark,
    },
  },
});

// Mobile-fullscreen overrides — applied when isFullscreen is true.
// Vanilla-extract can't express @media inside compound selectors, so we use
// separate exported classes that SessionDetail.tsx applies conditionally.

export const fullscreenMobileHeader = style({
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.25rem 0.5rem",
      flexWrap: "nowrap",
      rowGap: 0,
    },
  },
});

export const fullscreenMobileTitle = style({
  "@media": {
    "screen and (max-width: 768px)": {
      flex: "1 1 auto",
      fontSize: "0.875rem",
      whiteSpace: "nowrap",
      overflow: "hidden",
      textOverflow: "ellipsis",
      minWidth: 0,
    },
  },
});

export const fullscreenMobileHeaderActions = style({
  "@media": {
    "screen and (max-width: 768px)": {
      width: "auto",
      flexShrink: 0,
    },
  },
});

// Shrinks tabs in fullscreen mode on mobile to reclaim vertical space without hiding them entirely.
export const fullscreenMobileTabs = style({
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0 0.25rem",
      borderBottom: `1px solid ${vars.color.terminalBorder}`,
    },
  },
});
