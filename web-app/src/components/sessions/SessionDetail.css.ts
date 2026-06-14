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
  padding: "0.375rem 0.75rem",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  position: "sticky",
  top: 0,
  zIndex: 10,
  flexShrink: 0,
  minHeight: "40px",
  selectors: {
    [`.${fullscreen} &`]: {
      padding: "0.25rem 0.75rem",
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.4rem 0.75rem",
      flexWrap: "nowrap",
      gap: "0.25rem",
      minHeight: 0,
    },
  },
});

export const title = style({
  margin: 0,
  fontSize: "0.9375rem",
  fontWeight: 600,
  color: vars.color.textPrimary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  selectors: {
    [`.${fullscreen} &`]: {
      fontSize: "0.875rem",
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      fontSize: "0.9rem",
      flex: "1 1 auto",
      minWidth: 0,
    },
  },
});

export const statusBadge = style({
  display: "inline-flex",
  alignItems: "center",
  padding: "0.0625rem 0.375rem",
  borderRadius: "9999px",
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  flexShrink: 0,
  marginLeft: "0.375rem",
  background: vars.color.surfaceSubtle,
  color: vars.color.textMuted,
  border: `1px solid ${vars.color.borderColor}`,
});

export const headerActions = style({
  display: "flex",
  alignItems: "center",
  gap: "0.25rem",
  "@media": {
    "screen and (max-width: 768px)": {
      flexShrink: 0,
    },
  },
});

export const fullscreenButton = style({
  background: "transparent",
  border: "none",
  fontSize: "1.1rem",
  cursor: "pointer",
  color: vars.color.textMuted,
  padding: "0.25rem 0.375rem",
  lineHeight: 1,
  transition: "color 0.2s, background 0.2s",
  borderRadius: vars.radii.sm,
  minWidth: "32px",
  minHeight: "32px",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
      background: vars.color.hoverBackground,
    },
  },
});

export const switchWorkspaceButton = style({
  display: "flex",
  alignItems: "center",
  gap: "0.25rem",
  background: vars.color.primary,
  border: "none",
  fontSize: vars.fontSize.xs,
  fontWeight: 500,
  cursor: "pointer",
  color: vars.color.primaryText,
  padding: "0.25rem 0.5rem",
  lineHeight: 1,
  transition: "background 0.2s, transform 0.1s",
  borderRadius: vars.radii.sm,
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
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  padding: "0.125rem 0.375rem",
  background: vars.color.hoverBackground,
  borderRadius: vars.radii.sm,
  whiteSpace: "nowrap",
  userSelect: "none",
});

export const navButton = style({
  background: "transparent",
  border: "none",
  fontSize: "1.1rem",
  cursor: "pointer",
  color: vars.color.textMuted,
  padding: "0.25rem 0.375rem",
  lineHeight: 1,
  transition: "color 0.2s, background 0.2s",
  borderRadius: vars.radii.sm,
  fontWeight: "bold",
  minWidth: "32px",
  minHeight: "32px",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
      background: vars.color.hoverBackground,
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
  fontSize: "1.1rem",
  cursor: "pointer",
  color: vars.color.textMuted,
  padding: "0.25rem 0.375rem",
  lineHeight: 1,
  transition: "color 0.2s, background 0.2s",
  borderRadius: vars.radii.sm,
  minWidth: "32px",
  minHeight: "32px",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  selectors: {
    "&:hover": {
      color: vars.color.error,
      background: vars.color.errorBg,
    },
  },
});

export const tabs = style({
  display: "flex",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  background: vars.color.background,
  padding: "0 0.5rem",
  gap: "0.125rem",
  flexShrink: 0,
  "@media": {
    "screen and (max-width: 768px)": {
      overflowX: "auto",
      padding: "0 0.25rem",
      gap: "0",
      scrollbarWidth: "none",
      selectors: {
        "&::-webkit-scrollbar": { display: "none" },
      },
    },
  },
});

export const tab = style({
  display: "flex",
  alignItems: "center",
  gap: "0.375rem",
  padding: "0.5rem 0.75rem",
  border: "none",
  background: "transparent",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  fontWeight: 500,
  cursor: "pointer",
  transition: "color 0.2s, border-color 0.2s",
  borderBottom: "2px solid transparent",
  position: "relative",
  whiteSpace: "nowrap",
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.5rem 0.6rem",
      fontSize: "0.75rem",
      whiteSpace: "nowrap",
      minHeight: "44px", // WCAG 2.5.5 AA minimum touch target
      display: "flex",
      alignItems: "center",
    },
  },
});

export const active = style({
  color: vars.color.primary,
  borderBottomColor: vars.color.primary,
});

export const tabIcon = style({
  fontSize: "1rem",
  lineHeight: 1,
  "@media": {
    "screen and (max-width: 768px)": {
      fontSize: "0.875rem",
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
  padding: "1rem",
  display: "flex",
  flexDirection: "column",
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.75rem",
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
  color: vars.color.textMuted,
  fontStyle: "italic",
});

export const infoGrid = style({
  display: "grid",
  gridTemplateColumns: "repeat(auto-fit, minmax(300px, 1fr))",
  gap: "1rem",
  "@media": {
    "screen and (max-width: 768px)": {
      gridTemplateColumns: "1fr",
      gap: "0.75rem",
    },
  },
});

export const infoItem = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
  padding: "0.75rem",
  background: vars.color.cardBackground,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.625rem",
    },
  },
});

export const infoLabel = style({
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
});

export const infoValue = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
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
  padding: "0.375rem 0.5rem",
  fontSize: vars.fontSize.sm,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  fontFamily: "inherit",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
    },
  },
});

export const saveButton = style({
  padding: "0.375rem 0.625rem",
  border: "none",
  borderRadius: vars.radii.sm,
  cursor: "pointer",
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  transition: "background 0.2s, transform 0.1s",
  background: vars.color.primary,
  color: vars.color.primaryText,
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
  padding: "0.375rem 0.625rem",
  border: "none",
  borderRadius: vars.radii.sm,
  cursor: "pointer",
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  transition: "background 0.2s",
  background: vars.color.cardBackground,
  color: vars.color.textMuted,
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
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
  color: vars.color.textMuted,
});

export const noTerminalIcon = style({
  fontSize: "2.5rem",
  opacity: 0.5,
});

export const noTerminalText = style({
  margin: 0,
  fontSize: "1.125rem",
  fontWeight: 600,
  color: vars.color.textPrimary,
  opacity: 0.7,
});

export const noTerminalSubtext = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  maxWidth: "400px",
  lineHeight: 1.5,
});

export const moreActionsButton = style({
  background: "transparent",
  border: "none",
  fontSize: "1.25rem",
  cursor: "pointer",
  color: vars.color.textMuted,
  padding: "0.25rem 0.375rem",
  lineHeight: 1,
  transition: "color 0.2s, background 0.2s",
  borderRadius: vars.radii.sm,
  minWidth: "32px",
  minHeight: "32px",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
      background: vars.color.hoverBackground,
    },
  },
});

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
  borderRadius: vars.radii.md,
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

export const actionButton = style({
  padding: "0.375rem 0.75rem",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  cursor: "pointer",
  fontSize: vars.fontSize.sm,
  transition: "background 0.15s",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
  },
});

export const actionButtonDanger = style({
  background: vars.color.error,
  color: vars.color.primaryText,
  border: "none",
  selectors: {
    "&:hover": {
      opacity: 0.9,
    },
  },
});

export const actionButtonSave = style({
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  selectors: {
    "&:hover": {
      background: vars.color.primaryDark,
    },
  },
});

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
      flexShrink: 0,
    },
  },
});

export const fullscreenMobileTabs = style({
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0 0.25rem",
      borderBottom: `1px solid ${vars.color.borderColor}`,
    },
  },
});

export const tabDisabled = style({
  opacity: 0.4,
  cursor: "not-allowed",
});

export const workflowSection = style({
  display: "contents",
});
