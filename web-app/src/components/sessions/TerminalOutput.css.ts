import { style, keyframes, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const pulse = keyframes({
  "0%, 100%": { opacity: 1 },
  "50%": { opacity: 0.5 },
});

const spin = keyframes({
  "0%": { transform: "rotate(0deg)" },
  "100%": { transform: "rotate(360deg)" },
});

export const container = style({
  display: "flex",
  flexDirection: "column",
  height: "100%",
  minHeight: 0,
  flex: 1,
  background: vars.color.terminalBackground,
  color: vars.color.terminalForeground,
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace",
});

export const toolbar = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "0.75rem 1rem",
  background: vars.color.cardBackground,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  flexShrink: 0,
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.5rem 0.75rem",
    },
  },
});

export const status = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
});

export const statusIndicator = style({
  width: "8px",
  height: "8px",
  borderRadius: "50%",
  animation: `${pulse} 2s infinite`,
});

export const connected = style({
  background: vars.color.success,
  boxShadow: `0 0 8px ${vars.color.success}`,
});

export const disconnected = style({
  background: vars.color.error,
  boxShadow: `0 0 8px ${vars.color.error}`,
  animationName: "none",
});

export const stabilizing = style({
  background: vars.color.warning,
  boxShadow: `0 0 8px ${vars.color.warning}`,
  animation: `${pulse} 0.8s infinite`,
});

export const statusText = style({
  fontSize: "0.875rem",
  color: vars.color.textSecondary,
  "@media": {
    // Keep a compact label visible so connectivity state isn't color-only (WCAG 1.4.1)
    "screen and (max-width: 768px)": {
      fontSize: "0.7rem",
    },
  },
});

export const externalLabel = style({
  fontSize: "12px",
  fontWeight: 600,
  color: vars.color.primary,
  padding: "2px 8px",
  background: vars.color.accentBg,
  borderRadius: "4px",
  border: `1px solid ${vars.color.borderSubtle}`,
});

export const errorText = style({
  fontSize: "0.875rem",
  color: vars.color.error,
  marginLeft: "0.5rem",
});

export const actions = style({
  display: "flex",
  gap: "0.5rem",
  "@media": {
    "screen and (max-width: 768px)": {
      gap: "0.25rem",
      overflowX: "auto",
      WebkitOverflowScrolling: "touch" as "auto",
      whiteSpace: "nowrap",
      scrollbarWidth: "none",
      msOverflowStyle: "none",
    },
  },
});

export const toolbarButton = style({
  padding: "0.5rem 0.75rem",
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "4px",
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  cursor: "pointer",
  transition: "background 0.2s, border-color 0.2s",
  selectors: {
    "&:hover": {
      background: vars.color.panelBgSecondary,
      borderColor: vars.color.borderHover,
    },
    "&:active": {
      background: vars.color.cardBackground,
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.4rem 0.6rem",
      fontSize: "0.8rem",
      minHeight: "var(--min-touch-target, 44px)",
      minWidth: "var(--min-touch-target, 44px)",
    },
  },
});

export const debugActive = style({});

// Toolbar toggle button — visible on all screen sizes
export const toolbarToggle = style({
  padding: "0.5rem",
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "4px",
  color: vars.color.textPrimary,
  fontSize: "0.75rem",
  cursor: "pointer",
  minWidth: "var(--min-touch-target, 44px)",
  minHeight: "var(--min-touch-target, 44px)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  flexShrink: 0,
  selectors: {
    "&:hover": {
      background: vars.color.panelBgSecondary,
      borderColor: vars.color.borderHover,
    },
  },
});

// Container for collapsible toolbar buttons
export const toolbarActions = style({
  display: "flex",
  gap: "0.5rem",
  flexWrap: "wrap",
  "@media": {
    "screen and (max-width: 768px)": {
      gap: "0.25rem",
      overflowX: "auto",
      WebkitOverflowScrolling: "touch" as "auto",
      whiteSpace: "nowrap",
      scrollbarWidth: "none",
      msOverflowStyle: "none",
      // Fade the right edge to signal swipeable overflow content
      maskImage: "linear-gradient(to right, black calc(100% - 32px), transparent 100%)" as string,
      WebkitMaskImage: "linear-gradient(to right, black calc(100% - 32px), transparent 100%)" as string,
    },
  },
});

export const devOnly = style({
  "@media": {
    "screen and (max-width: 768px)": {
      display: "none",
    },
  },
});

// Secondary actions group — inline on desktop, hidden on mobile (actions in overflow row instead)
export const secondaryGroup = style({
  display: "flex",
  gap: "0.5rem",
  "@media": {
    "screen and (max-width: 768px)": {
      display: "none",
    },
  },
});

// "More ▾" trigger — only visible on mobile, hidden on desktop
export const mobileMoreButton = style({
  display: "none",
  "@media": {
    "screen and (max-width: 768px)": {
      display: "flex",
      alignItems: "center",
      justifyContent: "center",
      minWidth: "var(--min-touch-target, 44px)",
      minHeight: "var(--min-touch-target, 44px)",
      whiteSpace: "nowrap",
    },
  },
});

// Active state for the More button when overflow is open
export const mobileMoreActive = style({
  background: `${vars.color.panelBgSecondary} !important`,
  borderColor: `${vars.color.borderHover} !important`,
});

// Overflow row — rendered below the toolbar when More is open; never shown on desktop
export const mobileOverflowRow = style({
  display: "none",
  "@media": {
    "screen and (max-width: 768px)": {
      display: "flex",
      gap: "0.25rem",
      padding: "0.3rem 0.75rem 0.4rem",
      background: vars.color.cardBackground,
      borderBottom: `1px solid ${vars.color.borderColor}`,
      overflowX: "auto",
      WebkitOverflowScrolling: "touch" as "auto",
      scrollbarWidth: "none",
      msOverflowStyle: "none",
      flexShrink: 0,
    },
  },
});

// Always visible — keyboard toggle and mouse mode toggle are useful on all screen sizes.
export const mobileKeyboardToggle = style({
  display: "inline-flex",
  alignItems: "center",
  "@media": {
    "screen and (max-width: 768px)": {
      minHeight: "var(--min-touch-target, 44px)",
      minWidth: "var(--min-touch-target, 44px)",
    },
  },
});

// Sticky modifier active state (CTRL / ALT armed)
export const mobileKeyActive = style({
  background: `${vars.color.accentBg} !important`,
  borderColor: `${vars.color.primary} !important`,
  color: `${vars.color.primaryText} !important`,
});

// Mouse-mode toolbar button active state
export const mouseModeActive = style({
  background: `${vars.color.accentBg} !important`,
  borderColor: `${vars.color.primary} !important`,
  color: `${vars.color.primaryText} !important`,
});

export const error = style({
  padding: "1rem",
  background: vars.color.errorBg,
  borderBottom: `1px solid ${vars.color.error}`,
  color: vars.color.error,
  fontSize: "0.875rem",
});

export const terminal = style({
  flex: 1,
  minHeight: 0,
  margin: 0,
  padding: 0,
  background: vars.color.terminalBackground,
  color: vars.color.terminalForeground,
  overflow: "hidden",
  position: "relative",
  // Safe-area padding for landscape notch (horizontal insets only)
  paddingLeft: "var(--safe-area-left, 0px)",
  paddingRight: "var(--safe-area-right, 0px)",
  "@media": {
    // Keep zero top/bottom padding — FitAddon measures the container; padding
    // causes it to undercount rows. Only respect horizontal safe-area insets.
    "screen and (max-width: 768px)": {
      padding: 0,
      paddingLeft: "var(--safe-area-left, 0px)",
      paddingRight: "var(--safe-area-right, 0px)",
    },
  },
});

export const loadingOverlay = style({
  position: "absolute",
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  background: vars.color.overlayBackground,
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  gap: "1rem",
  zIndex: 10,
  backdropFilter: "blur(2px)",
});

export const loadingSpinner = style({
  width: "48px",
  height: "48px",
  border: `4px solid ${vars.color.terminalBorder}`,
  borderTop: `4px solid ${vars.color.success}`,
  borderRadius: "50%",
  animation: `${spin} 1s linear infinite`,
});

export const loadingText = style({
  fontSize: "0.875rem",
  color: vars.color.terminalForeground,
  fontWeight: 500,
});

export const unavailableOverlay = style({
  position: "absolute",
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  background: vars.color.overlayBackground,
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  gap: "0.5rem",
  zIndex: 10,
});

export const unavailableIcon = style({
  fontSize: "2rem",
  color: vars.color.textMuted,
});

export const unavailableText = style({
  fontSize: "1rem",
  color: vars.color.textSecondary,
  fontWeight: 600,
});

export const unavailableSubtext = style({
  fontSize: "0.8125rem",
  color: vars.color.textMuted,
});

// ---- Resize overlay (Task 4.2.1 / R1.4) ----
// Non-blocking dimmed overlay shown while the server waits for tmux quiescence after resize.
// pointer-events: none ensures the user can still interact with the terminal.
export const resizingOverlay = style({
  position: "absolute",
  inset: 0,
  background: "rgba(0, 0, 0, 0.3)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  pointerEvents: "none",
  zIndex: 5,
});

const resizeSpin = keyframes({
  "0%": { transform: "rotate(0deg)" },
  "100%": { transform: "rotate(360deg)" },
});

export const resizingSpinner = style({
  width: "32px",
  height: "32px",
  border: `3px solid ${vars.color.terminalBorder}`,
  borderTop: `3px solid ${vars.color.primary}`,
  borderRadius: "50%",
  animation: `${resizeSpin} 0.8s linear infinite`,
});

export const mobileKeyboard = style({
  display: "none",
  flexDirection: "column",
  gap: "0.25rem",
  padding: "0.4rem 0.5rem",
  background: vars.color.terminalTabsBg,
  borderTop: `1px solid ${vars.color.terminalBorder}`,
  flexShrink: 0,
  "@media": {
    "screen and (max-width: 768px)": {
      display: "flex",
      paddingBottom: "max(var(--safe-area-bottom, 0px), 0.4rem)",
    },
  },
});

export const mobileKeyRow = style({
  display: "flex",
  gap: "0.3rem",
  justifyContent: "center",
});

export const mobileKey = style({
  flex: 1,
  // Tighter padding so 7 keys fit per row on a 375 px screen
  padding: "0.45rem 0.25rem",
  minWidth: 0, // allow flex shrink below content width
  minHeight: "var(--min-touch-target, 44px)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  background: vars.color.terminalHoverBg,
  border: `1px solid ${vars.color.terminalBorder}`,
  borderBottom: `3px solid ${vars.color.terminalBorder}`,
  borderRadius: "5px",
  color: vars.color.terminalForeground,
  fontFamily: "inherit",
  fontSize: "0.8rem",
  fontWeight: 500,
  cursor: "pointer",
  userSelect: "none",
  WebkitUserSelect: "none",
  touchAction: "manipulation",
  textAlign: "center",
  whiteSpace: "nowrap",
  transition: "background 0.08s, border-bottom-width 0.08s, transform 0.08s",
  selectors: {
    "&:active": {
      background: vars.color.terminalHoverBg,
      borderBottomWidth: "1px",
      transform: "translateY(2px)",
    },
  },
});

// Highlight ^C so users can find the interrupt key instantly
export const mobileKeyCtrlC = style({
  background: `${vars.color.errorBg} !important`,
  borderColor: `${vars.color.error} !important`,
  color: `${vars.color.errorText} !important`,
  fontWeight: "700",
  boxShadow: `0 0 0 1px ${vars.color.error}`,
});

// Global styles for xterm.js selectors within the terminal class
globalStyle(`${terminal} .xterm-viewport::-webkit-scrollbar`, {
  width: "12px",
  height: "12px",
});

globalStyle(`${terminal} .xterm-viewport::-webkit-scrollbar-track`, {
  background: vars.color.terminalBackground,
});

globalStyle(`${terminal} .xterm-viewport::-webkit-scrollbar-thumb`, {
  background: vars.color.terminalBorder,
  borderRadius: "6px",
  border: `2px solid ${vars.color.terminalBackground}`,
});

globalStyle(`${terminal} .xterm-viewport::-webkit-scrollbar-thumb:hover`, {
  background: vars.color.terminalHoverBg,
});

globalStyle(`${terminal} .xterm-viewport::-webkit-scrollbar-corner`, {
  background: vars.color.terminalBackground,
});

globalStyle(`${terminal} .xterm-selection`, {
  background: vars.color.accentBg,
});

globalStyle(`${terminal} .xterm-helper-textarea`, {
  fontSize: "16px !important",
});
