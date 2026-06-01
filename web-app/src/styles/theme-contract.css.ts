import { createThemeContract } from "@vanilla-extract/css";

export const vars = createThemeContract({
  color: {
    // Text
    textPrimary: null,
    textSecondary: null,
    textMuted: null,
    textDisabled: null,
    textTertiary: null,
    textInverse: null,

    // Surfaces / backgrounds
    background: null,
    cardBackground: null,
    hoverBackground: null,
    modalBackground: null,
    overlayBackground: null,
    panelBgSecondary: null,
    surfaceSubtle: null,
    surfaceMuted: null,

    // Borders
    borderColor: null,
    borderSubtle: null,
    borderMuted: null,
    borderStrong: null,
    borderHover: null,
    modalBorder: null,
    inputBorder: null,
    inputFocusBorder: null,

    // Primary action
    primary: null,
    primaryHover: null,
    primaryActive: null,
    primaryDark: null,
    primaryText: null,

    // Status
    success: null,
    successBg: null,
    warning: null,
    warningBg: null,
    warningText: null,
    error: null,
    errorBg: null,
    errorText: null,
    errorDark: null,

    // Accent tints
    accentBg: null,
    accentHover: null,

    // Inputs
    inputBackground: null,
    inputText: null,
    placeholderColor: null,

    // Terminal (always dark — same value in both themes)
    terminalBackground: null,
    terminalForeground: null,
    terminalBorder: null,
    terminalHeaderBg: null,
    terminalHeaderFg: null,
    terminalTabsBg: null,
    terminalTextMuted: null,
    terminalHoverBg: null,

    // Log level badge colors (semantic colors for log-level chips/rows)
    logError: null,   // ERROR badge background
    logWarn: null,    // WARN badge background
    logInfo: null,    // INFO badge background
    logDebug: null,   // DEBUG badge background
    logTrace: null,   // TRACE badge background
    logOnDark: null,  // text on dark log badges (error/info/debug/trace)
    logOnAmber: null, // text on amber log badge (warn)
    logLive: null,    // live indicator dot / success accent

    // Cyberpunk / glow tokens
    glowPrimary: null,
    glowSecondary: null,
    scanlineColor: null,
    terminalCursor: null,

    // Status dots (session running/paused/idle indicators)
    statusDot: {
      running: null,
      paused: null,
      idle: null,
    },
  },
  statusBadge: {
    approvalBg: null,
    approvalFg: null,
    approvalBorder: null,
    inputBg: null,
    inputFg: null,
    inputBorder: null,
    completeBg: null,
    completeFg: null,
    completeBorder: null,
    uncommittedBg: null,
    uncommittedFg: null,
    uncommittedBorder: null,
    idleBg: null,
    idleFg: null,
    idleBorder: null,
    staleFg: null,
    processingBg: null,
    processingFg: null,
    processingBorder: null,
  },
  font: {
    mono: null,
    sans: null,
    display: null,
  },
  space: {
    "0": null,
    "1": null,
    "2": null,
    "3": null,
    "4": null,
    "6": null,
    "8": null,
    "12": null,
    "16": null,
  },
  radii: {
    sm: null,
    md: null,
    lg: null,
    full: null,
  },
  fontSize: {
    xs: null,
    sm: null,
    base: null,
    lg: null,
    xl: null,
  },
  fontWeight: {
    normal: null,
    medium: null,
    semibold: null,
    bold: null,
  },
  shadow: {
    none: null,
    sm: null,
    md: null,
    lg: null,
  },
  transition: {
    fast: null,
    base: null,
    slow: null,
  },
});

// Plain constants — CSS custom properties cannot be used inside @media queries,
// so breakpoints and z-index values are exported as typed literals, not theme tokens.

export const breakpoints = {
  sm: "640px",
  md: "768px",
  lg: "1024px",
  xl: "1280px",
  outer: "390px",
  fold: "600px",
  inner: "900px",
} as const;

// Named z-index ladder — every fixed/absolute element must reference a slot here.
// Adding a new layer requires updating this map, which makes ordering conflicts visible.
export const zIndex = {
  base: 0,
  tableHeader: 1,   // sticky <th> within a scroll container — only competes with sibling tds
  raised: 10,
  header: 100,
  dropdown: 500,
  slideOver: 700,
  modal: 1000,
  // Navigation overlay stack (1040–1065).  Values chosen so the bottom nav and its
  // sub-menus sit above all other page content, and the mobile pane picker sits above the nav.
  bottomNavMoreBackdrop: 1040,
  bottomNavMoreSheet: 1045,
  bottomNav: 1050,
  mobilePickerBackdrop: 1060,
  mobilePickerSheet: 1065,
  // Full-page dialog overlays must sit above the bottom nav stack.
  dialog: 1070,
  // Toast sits above dialog-level overlays but below the Radix modal overlay (1100),
  // so notifications are hidden behind modals rather than covering form actions.
  toast: 1080,
  floatingTerminalUI: 1085,
  tooltip: 1100,
} as const;
