import { style, keyframes, globalStyle } from "@vanilla-extract/css";

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
  background: "#1e1e1e",
  color: "#d4d4d4",
  fontFamily: "'Monaco', 'Menlo', 'Ubuntu Mono', 'Consolas', 'source-code-pro', monospace",
});

export const toolbar = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "0.75rem 1rem",
  background: "#2d2d30",
  borderBottom: "1px solid #3e3e42",
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
  background: "#4ec9b0",
  boxShadow: "0 0 8px #4ec9b0",
});

export const disconnected = style({
  background: "#f48771",
  boxShadow: "0 0 8px #f48771",
  animationName: "none",
});

export const stabilizing = style({
  background: "#e5c07b",
  boxShadow: "0 0 8px #e5c07b",
  animation: `${pulse} 0.8s infinite`,
});

export const statusText = style({
  fontSize: "0.875rem",
  color: "#cccccc",
  "@media": {
    "screen and (max-width: 768px)": {
      display: "none",
    },
  },
});

export const externalLabel = style({
  fontSize: "12px",
  fontWeight: 600,
  color: "#6366f1",
  padding: "2px 8px",
  background: "rgba(99, 102, 241, 0.1)",
  borderRadius: "4px",
  border: "1px solid rgba(99, 102, 241, 0.3)",
});

export const errorText = style({
  fontSize: "0.875rem",
  color: "#f48771",
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
  background: "#3e3e42",
  border: "1px solid #555555",
  borderRadius: "4px",
  color: "#cccccc",
  fontSize: "0.875rem",
  cursor: "pointer",
  transition: "background 0.2s, border-color 0.2s",
  selectors: {
    "&:hover": {
      background: "#505050",
      borderColor: "#6e6e6e",
    },
    "&:active": {
      background: "#2d2d30",
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

// Toolbar toggle button — always visible on mobile, hidden on desktop
export const toolbarToggle = style({
  padding: "0.5rem",
  background: "#3e3e42",
  border: "1px solid #555555",
  borderRadius: "4px",
  color: "#cccccc",
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
      background: "#505050",
    },
  },
  "@media": {
    // On desktop always show the full toolbar; hide the toggle
    "screen and (min-width: 1024px)": {
      display: "none",
    },
  },
});

// Container for collapsible toolbar buttons
export const toolbarActions = style({
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
    // On desktop toolbarActions always visible regardless of toolbarExpanded
    "screen and (min-width: 1024px)": {
      display: "flex !important" as "flex",
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
  background: "#505050 !important",
  borderColor: "#6e6e6e !important",
});

// Overflow row — rendered below the toolbar when More is open; never shown on desktop
export const mobileOverflowRow = style({
  display: "none",
  "@media": {
    "screen and (max-width: 768px)": {
      display: "flex",
      gap: "0.25rem",
      padding: "0.3rem 0.75rem 0.4rem",
      background: "#252526",
      borderBottom: "1px solid #3e3e42",
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
  background: "#1e4976 !important",
  borderColor: "#4a8fc7 !important",
  color: "#93d0ff !important",
});

// Mouse-mode toolbar button active state
export const mouseModeActive = style({
  background: "#1e4976 !important",
  borderColor: "#4a8fc7 !important",
  color: "#93d0ff !important",
});

export const error = style({
  padding: "1rem",
  background: "rgba(244, 135, 113, 0.1)",
  borderBottom: "1px solid #f48771",
  color: "#f48771",
  fontSize: "0.875rem",
});

export const terminal = style({
  flex: 1,
  minHeight: 0,
  margin: 0,
  padding: 0,
  background: "#1e1e1e",
  color: "#d4d4d4",
  overflow: "hidden",
  position: "relative",
  // Safe-area padding for landscape notch (horizontal insets only)
  paddingLeft: "var(--safe-area-left, 0px)",
  paddingRight: "var(--safe-area-right, 0px)",
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0.75rem",
      paddingLeft: "max(0.75rem, var(--safe-area-left, 0px))",
      paddingRight: "max(0.75rem, var(--safe-area-right, 0px))",
      fontSize: "13px",
    },
  },
});

export const loadingOverlay = style({
  position: "absolute",
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  background: "rgba(30, 30, 30, 0.95)",
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
  border: "4px solid #3e3e42",
  borderTop: "4px solid #4ec9b0",
  borderRadius: "50%",
  animation: `${spin} 1s linear infinite`,
});

export const loadingText = style({
  fontSize: "0.875rem",
  color: "#cccccc",
  fontWeight: 500,
});

export const unavailableOverlay = style({
  position: "absolute",
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  background: "rgba(30, 30, 30, 0.92)",
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  gap: "0.5rem",
  zIndex: 10,
});

export const unavailableIcon = style({
  fontSize: "2rem",
  color: "#6b7280",
});

export const unavailableText = style({
  fontSize: "1rem",
  color: "#9ca3af",
  fontWeight: 600,
});

export const unavailableSubtext = style({
  fontSize: "0.8125rem",
  color: "#6b7280",
});

export const mobileKeyboard = style({
  display: "none",
  flexDirection: "column",
  gap: "0.25rem",
  padding: "0.4rem 0.5rem",
  background: "#252526",
  borderTop: "1px solid #3e3e42",
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
  background: "#3c3c3c",
  border: "1px solid #555",
  borderBottom: "3px solid #333",
  borderRadius: "5px",
  color: "#d4d4d4",
  fontFamily: "inherit",
  fontSize: "0.78rem",
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
      background: "#555",
      borderBottomWidth: "1px",
      transform: "translateY(2px)",
    },
  },
});

// Global styles for xterm.js selectors within the terminal class
globalStyle(`${terminal} .xterm-viewport::-webkit-scrollbar`, {
  width: "12px",
  height: "12px",
});

globalStyle(`${terminal} .xterm-viewport::-webkit-scrollbar-track`, {
  background: "#1e1e1e",
});

globalStyle(`${terminal} .xterm-viewport::-webkit-scrollbar-thumb`, {
  background: "#424242",
  borderRadius: "6px",
  border: "2px solid #1e1e1e",
});

globalStyle(`${terminal} .xterm-viewport::-webkit-scrollbar-thumb:hover`, {
  background: "#4e4e4e",
});

globalStyle(`${terminal} .xterm-viewport::-webkit-scrollbar-corner`, {
  background: "#1e1e1e",
});

globalStyle(`${terminal} .xterm-selection`, {
  background: "#264f78",
});

globalStyle(`${terminal} .xterm-helper-textarea`, {
  fontSize: "16px !important",
});
