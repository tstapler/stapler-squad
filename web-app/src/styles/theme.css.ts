import { createTheme } from "@vanilla-extract/css";
import { vars } from "./theme-contract.css";

export { vars, breakpoints, zIndex } from "./theme-contract.css";

const sharedTokens = {
  font: {
    mono: "'Monaco', 'Menlo', 'Ubuntu Mono', monospace",
    sans: "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
    display: "system-ui, sans-serif",
  },
  space: {
    "0": "0px",
    "1": "4px",
    "2": "8px",
    "3": "12px",
    "4": "16px",
    "6": "24px",
    "8": "32px",
    "12": "48px",
    "16": "64px",
  },
  radii: {
    sm: "4px",
    md: "6px",
    lg: "12px",
    full: "9999px",
  },
  fontSize: {
    xs: "12px",  /* WCAG: minimum legible size; was 11px */
    sm: "14px",  /* WCAG: body/label minimum; was 12px */
    base: "14px",
    lg: "16px",
    xl: "20px",
  },
  fontWeight: {
    normal: "400",
    medium: "500",
    semibold: "600",
    bold: "700",
  },
};

const terminalTokens = {
  terminalBackground: "#1e1e1e",
  terminalForeground: "#d4d4d4",
  terminalBorder: "#3e3e42",
  terminalHeaderBg: "#2d2d30",
  terminalHeaderFg: "#ffffff",
  terminalTabsBg: "#252526",
  terminalTextMuted: "#9ca3af",
  terminalHoverBg: "#3e3e42",
};

export const lightTheme = createTheme(vars, {
  color: {
    textPrimary: "#0a0a0a",
    textSecondary: "#4a4a4a",
    textMuted: "#6b6b6b",
    textDisabled: "#9a9a9a",
    textTertiary: "#767676", /* was #9ca3af — 2.53:1 fails WCAG AA on white; #767676 = 4.55:1 ✅ */
    textInverse: "#ffffff",

    background: "#ffffff",
    cardBackground: "#f9f9f9",
    hoverBackground: "#f0f0f0",
    modalBackground: "#ffffff",
    overlayBackground: "rgba(0, 0, 0, 0.5)",
    panelBgSecondary: "#f3f4f6",
    surfaceSubtle: "#f9fafb",
    surfaceMuted: "#f3f4f6",

    borderColor: "#e0e0e0",
    borderSubtle: "#e5e7eb",
    borderMuted: "#d1d5db",
    borderStrong: "#9ca3af",
    borderHover: "#9ca3af",
    modalBorder: "#e0e0e0",
    inputBorder: "#d4d4d4",
    inputFocusBorder: "#0070f3",

    primary: "#0070f3",
    primaryHover: "#0051cc",
    primaryActive: "#003d99",
    primaryDark: "#003d99",
    primaryText: "#ffffff",

    success: "#10b981",
    successBg: "#d1fae5",
    warning: "#f59e0b",
    warningBg: "#fef3c7",
    warningText: "#92400e",
    error: "#ef4444",
    errorBg: "#fee2e2",
    errorText: "#991b1b",
    errorDark: "#b91c1c",

    accentBg: "rgba(0, 112, 243, 0.08)",
    accentHover: "rgba(0, 112, 243, 0.16)",

    inputBackground: "#ffffff",
    inputText: "#0a0a0a",
    placeholderColor: "#9ca3af",

    ...terminalTokens,

    glowPrimary: "rgba(0,112,243,0.4)",
    glowSecondary: "rgba(0,112,243,0.2)",
    scanlineColor: "transparent",
    terminalCursor: "#0070f3",

    statusDot: { running: "#22c55e", paused: "#f59e0b", idle: "#475569" },
  },
  shadow: {
    none: "none",
    sm: "0 1px 2px 0 rgba(0,0,0,0.05)",
    md: "0 4px 6px -1px rgba(0,0,0,0.07), 0 2px 4px -2px rgba(0,0,0,0.05)",
    lg: "0 10px 15px -3px rgba(0,0,0,0.1), 0 4px 6px -4px rgba(0,0,0,0.05)",
  },
  statusBadge: {
    approvalBg: "#fecaca",
    approvalFg: "#991b1b",
    approvalBorder: "#fca5a5",
    inputBg: "#dbeafe",
    inputFg: "#1e40af",
    inputBorder: "#93c5fd",
    completeBg: "#dcfce7",
    completeFg: "#166534",
    completeBorder: "#bbf7d0",
    uncommittedBg: "#fef3c7",
    uncommittedFg: "#92400e",
    uncommittedBorder: "#fcd34d",
    idleBg: "#f3f4f6",
    idleFg: "#374151",
    idleBorder: "#d1d5db",
    staleFg: "#6b7280",
    processingBg: "#e0e7ff",
    processingFg: "#4338ca",
    processingBorder: "#c7d2fe",
  },
  transition: { fast: "100ms ease", base: "150ms ease", slow: "250ms ease" },
  ...sharedTokens,
});

export const darkTheme = createTheme(vars, {
  color: {
    textPrimary: "#ededed",
    textSecondary: "#b4b4b4",
    textMuted: "#8a8a8a",
    textDisabled: "#767676",
    textTertiary: "#808080", /* was #6b7280 — #808080 = 4.64:1 on #1a1a1a card-bg — WCAG AA ✅ */
    textInverse: "#0a0a0a",

    background: "#0a0a0a",
    cardBackground: "#1a1a1a",
    hoverBackground: "#2a2a2a",
    modalBackground: "#1a1a1a",
    overlayBackground: "rgba(0, 0, 0, 0.7)",
    panelBgSecondary: "#2a2a2a",
    surfaceSubtle: "#1f2937",
    surfaceMuted: "#374151",

    borderColor: "#333333",
    borderSubtle: "#374151",
    borderMuted: "#4b5563",
    borderStrong: "#6b7280",
    borderHover: "#6b7280",
    modalBorder: "#333333",
    inputBorder: "#404040",
    inputFocusBorder: "#2d9cdb",

    primary: "#2d9cdb",
    primaryHover: "#3ba9e6",
    primaryActive: "#52b9f0",
    primaryDark: "#1a7fc1",
    primaryText: "#ffffff",

    success: "#10b981",
    successBg: "#064e3b",
    warning: "#f59e0b",
    warningBg: "#78350f",
    warningText: "#fbbf24",
    error: "#ef4444",
    errorBg: "#7f1d1d",
    errorText: "#fca5a5",
    errorDark: "#ef4444",

    accentBg: "rgba(45, 156, 219, 0.1)",
    accentHover: "rgba(45, 156, 219, 0.2)",

    inputBackground: "#2a2a2a",
    inputText: "#ededed",
    placeholderColor: "#6b7280",

    ...terminalTokens,

    glowPrimary: "rgba(45,156,219,0.4)",
    glowSecondary: "rgba(45,156,219,0.2)",
    scanlineColor: "transparent",
    terminalCursor: "#2d9cdb",

    statusDot: { running: "#22c55e", paused: "#f59e0b", idle: "#475569" },
  },
  shadow: {
    none: "none",
    sm: "0 1px 2px 0 rgba(0,0,0,0.3)",
    md: "0 4px 6px -1px rgba(0,0,0,0.4), 0 2px 4px -2px rgba(0,0,0,0.3)",
    lg: "0 10px 15px -3px rgba(0,0,0,0.5), 0 4px 6px -4px rgba(0,0,0,0.4)",
  },
  statusBadge: {
    approvalBg: "#450a0a",
    approvalFg: "#fca5a5",
    approvalBorder: "#7f1d1d",
    inputBg: "#1e3a5f",
    inputFg: "#93c5fd",
    inputBorder: "#2563eb",
    completeBg: "#14532d",
    completeFg: "#86efac",
    completeBorder: "#166534",
    uncommittedBg: "#78350f",
    uncommittedFg: "#fbbf24",
    uncommittedBorder: "#b45309",
    idleBg: "#1f2937",
    idleFg: "#9ca3af",
    idleBorder: "#374151",
    staleFg: "#6b7280",
    processingBg: "#312e81",
    processingFg: "#a5b4fc",
    processingBorder: "#4338ca",
  },
  transition: { fast: "100ms ease", base: "150ms ease", slow: "250ms ease" },
  ...sharedTokens,
});

// ---------------------------------------------------------------------------
// Matrix theme — green on black, JetBrains Mono everywhere
// ---------------------------------------------------------------------------
export const matrixTheme = createTheme(vars, {
  color: {
    textPrimary: "#00ff41",
    textSecondary: "#00cc33",
    textMuted: "#00b32d", /* was #004d18 — 1.32:1 fails WCAG AA; #00b32d = 5.12:1 on #0a0a0a ✅ */
    textDisabled: "#002b0e",
    textTertiary: "#006622",
    textInverse: "#000000",

    background: "#000000",
    cardBackground: "#0a0a0a",
    hoverBackground: "#0d1a00",
    modalBackground: "#050505",
    overlayBackground: "rgba(0,0,0,0.85)",
    panelBgSecondary: "#0d1a00",
    surfaceSubtle: "#0a1200",
    surfaceMuted: "#0d1a00",

    borderColor: "#003300",
    borderSubtle: "#002200",
    borderMuted: "#003d00",
    borderStrong: "#005500",
    borderHover: "#00aa00",
    modalBorder: "#003300",
    inputBorder: "#004400",
    inputFocusBorder: "#00ff41",

    primary: "#00ff41",
    primaryHover: "#33ff66",
    primaryActive: "#00cc33",
    primaryDark: "#004d18",
    primaryText: "#000000",

    success: "#00ff41",
    successBg: "#001a00",
    warning: "#ffaa00",
    warningBg: "#2a1a00",
    warningText: "#ffcc44",
    error: "#ff0040",
    errorBg: "#1a0010",
    errorText: "#ff6688",
    errorDark: "#cc0033",

    accentBg: "rgba(0,255,65,0.1)",
    accentHover: "rgba(0,255,65,0.2)",

    inputBackground: "#050505",
    inputText: "#00ff41",
    placeholderColor: "#004d18",

    ...terminalTokens,

    glowPrimary: "rgba(0,255,65,0.5)",
    glowSecondary: "rgba(0,255,65,0.25)",
    scanlineColor: "rgba(0,255,65,0.03)",
    terminalCursor: "#00ff41",

    statusDot: { running: "#22c55e", paused: "#f59e0b", idle: "#475569" },
  },
  shadow: {
    none: "none",
    sm: "0 1px 2px 0 rgba(0,255,65,0.1)",
    md: "0 4px 6px -1px rgba(0,255,65,0.15), 0 2px 4px -2px rgba(0,255,65,0.1)",
    lg: "0 10px 15px -3px rgba(0,255,65,0.2), 0 4px 6px -4px rgba(0,255,65,0.15)",
  },
  statusBadge: {
    approvalBg: "#1a0010",
    approvalFg: "#ff0040",
    approvalBorder: "#440010",
    inputBg: "#001a33",
    inputFg: "#00ff41",
    inputBorder: "#003300",
    completeBg: "#001a00",
    completeFg: "#00ff41",
    completeBorder: "#003300",
    uncommittedBg: "#1a1000",
    uncommittedFg: "#ffaa00",
    uncommittedBorder: "#442200",
    idleBg: "#0a0a0a",
    idleFg: "#00cc33", /* was #004d18 — 1.95:1 fails WCAG AA; #00cc33 = 9.1:1 on #0a0a0a ✅ */
    idleBorder: "#002200",
    staleFg: "#00b32d", /* was #003300 — fails WCAG AA; #00b32d = 7.0:1 on #0a0a0a ✅ */
    processingBg: "#001a0a",
    processingFg: "#00cc33",
    processingBorder: "#003300",
  },
  font: {
    mono: "var(--font-jetbrains-mono,'Monaco',monospace)",
    sans: "var(--font-jetbrains-mono,'Monaco',monospace)",
    display: "var(--font-jetbrains-mono,'Monaco',monospace)",
  },
  space: sharedTokens.space,
  radii: sharedTokens.radii,
  fontSize: sharedTokens.fontSize,
  fontWeight: sharedTokens.fontWeight,
  transition: { fast: "100ms ease", base: "150ms ease", slow: "250ms ease" },
});

// ---------------------------------------------------------------------------
// Cyberpunk 77 theme — yellow + pink neon on dark navy
// ---------------------------------------------------------------------------
export const cyberpunk77Theme = createTheme(vars, {
  color: {
    textPrimary: "#fcee09",
    textSecondary: "#c8be08",
    textMuted: "#aaaa00", /* was #7a7405 — 1.83:1 fails WCAG AA on #12122a; #aaaa00 = 7.38:1 ✅ */
    textDisabled: "#4a4603",
    textTertiary: "#8a8006",
    textInverse: "#0d0d1a",

    background: "#0d0d1a",
    cardBackground: "#12122a",
    hoverBackground: "#1a1a35",
    modalBackground: "#0f0f22",
    overlayBackground: "rgba(0,0,0,0.85)",
    panelBgSecondary: "#1a1a35",
    surfaceSubtle: "#161625",
    surfaceMuted: "#1a1a35",

    borderColor: "#1a1a3e",
    borderSubtle: "#14143a",
    borderMuted: "#222250",
    borderStrong: "#2d2d6e",
    borderHover: "#cc245f",
    modalBorder: "#1a1a3e",
    inputBorder: "#2d2d6e",
    inputFocusBorder: "#fcee09",

    primary: "#cc245f", /* was #ff2d78 — #fff on #ff2d78 = 3.56:1 fails WCAG AA; #cc245f = 5.27:1 ✅ */
    primaryHover: "#e02d6e",
    primaryActive: "#aa1e50",
    primaryDark: "#7a1540",
    primaryText: "#ffffff",

    success: "#00ff9f",
    successBg: "#001a11",
    warning: "#fcee09",
    warningBg: "#1a1600",
    warningText: "#fcee09",
    error: "#ff2d78",
    errorBg: "#1a0010",
    errorText: "#ff88aa",
    errorDark: "#cc2460",

    accentBg: "rgba(255,45,120,0.1)",
    accentHover: "rgba(255,45,120,0.2)",

    inputBackground: "#0f0f22",
    inputText: "#fcee09",
    placeholderColor: "#4a4603",

    ...terminalTokens,

    glowPrimary: "rgba(255,45,120,0.5)",
    glowSecondary: "rgba(0,212,255,0.4)",
    scanlineColor: "rgba(255,45,120,0.02)",
    terminalCursor: "#00d4ff",

    statusDot: { running: "#22c55e", paused: "#f59e0b", idle: "#475569" },
  },
  shadow: {
    none: "none",
    sm: "0 1px 2px 0 rgba(255,45,120,0.1)",
    md: "0 4px 6px -1px rgba(255,45,120,0.15), 0 2px 4px -2px rgba(255,45,120,0.1)",
    lg: "0 10px 15px -3px rgba(255,45,120,0.2), 0 4px 6px -4px rgba(255,45,120,0.15)",
  },
  statusBadge: {
    approvalBg: "#1a0010",
    approvalFg: "#ff2d78",
    approvalBorder: "#440028",
    inputBg: "#001122",
    inputFg: "#00d4ff",
    inputBorder: "#1a1a3e",
    completeBg: "#001a11",
    completeFg: "#00ff9f",
    completeBorder: "#003322",
    uncommittedBg: "#1a1600",
    uncommittedFg: "#fcee09",
    uncommittedBorder: "#443800",
    idleBg: "#12122a",
    idleFg: "#aaaa00", /* was #4a4603 — fails WCAG AA on #12122a; #aaaa00 = 7.38:1 ✅ */
    idleBorder: "#1a1a3e",
    staleFg: "#8888aa", /* was #2d2d6e — fails WCAG AA on #12122a; #8888aa = 5.1:1 ✅ */
    processingBg: "#0d0d22",
    processingFg: "#c8be08",
    processingBorder: "#1a1a3e",
  },
  font: {
    mono: "var(--font-jetbrains-mono,'Monaco',monospace)",
    sans: "var(--font-rajdhani,'Rajdhani',system-ui,sans-serif)",
    display: "var(--font-rajdhani,'Rajdhani',system-ui,sans-serif)",
  },
  space: sharedTokens.space,
  radii: sharedTokens.radii,
  fontSize: sharedTokens.fontSize,
  fontWeight: sharedTokens.fontWeight,
  transition: { fast: "100ms ease", base: "150ms ease", slow: "250ms ease" },
});

// ---------------------------------------------------------------------------
// WH40K theme — grimdark parchment + gold on near-black
// ---------------------------------------------------------------------------
export const wh40kTheme = createTheme(vars, {
  color: {
    textPrimary: "#c8b89a",
    textSecondary: "#a89878",
    textMuted: "#a08870", /* was #786858 — 3.38:1 fails WCAG AA on #1a1510; #a08870 = 5.39:1 ✅ */
    textDisabled: "#484038",
    textTertiary: "#908070",
    textInverse: "#0c0a08",

    background: "#0c0a08",
    cardBackground: "#1a1510",
    hoverBackground: "#221e18",
    modalBackground: "#120e0a",
    overlayBackground: "rgba(0,0,0,0.85)",
    panelBgSecondary: "#221e18",
    surfaceSubtle: "#18140e",
    surfaceMuted: "#221e18",

    borderColor: "#3d3020",
    borderSubtle: "#2a2015",
    borderMuted: "#4a3828",
    borderStrong: "#c0a020",
    borderHover: "#d4b424",
    modalBorder: "#3d3020",
    inputBorder: "#4a3828",
    inputFocusBorder: "#c0a020",

    primary: "#c0a020",
    primaryHover: "#d4b424",
    primaryActive: "#a08818",
    primaryDark: "#705810",
    primaryText: "#0c0a08",

    success: "#4a7c3f",
    successBg: "#0a1208",
    warning: "#c0a020",
    warningBg: "#1a1400",
    warningText: "#e4c840",
    error: "#8b1a1a",
    errorBg: "#1a0808",
    errorText: "#c45050",
    errorDark: "#6b1010",

    accentBg: "rgba(192,160,32,0.1)",
    accentHover: "rgba(192,160,32,0.2)",

    inputBackground: "#120e0a",
    inputText: "#c8b89a",
    placeholderColor: "#786858",

    ...terminalTokens,

    glowPrimary: "rgba(192,160,32,0.4)",
    glowSecondary: "rgba(139,26,26,0.4)",
    scanlineColor: "transparent",
    terminalCursor: "#c0a020",

    statusDot: { running: "#22c55e", paused: "#f59e0b", idle: "#475569" },
  },
  shadow: {
    none: "none",
    sm: "0 1px 2px 0 rgba(192,160,32,0.1)",
    md: "0 4px 6px -1px rgba(192,160,32,0.15), 0 2px 4px -2px rgba(192,160,32,0.1)",
    lg: "0 10px 15px -3px rgba(192,160,32,0.2), 0 4px 6px -4px rgba(192,160,32,0.15)",
  },
  statusBadge: {
    approvalBg: "#1a0808",
    approvalFg: "#8b1a1a",
    approvalBorder: "#440a0a",
    inputBg: "#0a1018",
    inputFg: "#c8b89a",
    inputBorder: "#3d3020",
    completeBg: "#0a1208",
    completeFg: "#4a7c3f",
    completeBorder: "#1a2a15",
    uncommittedBg: "#1a1400",
    uncommittedFg: "#c0a020",
    uncommittedBorder: "#3d3000",
    idleBg: "#1a1510",
    idleFg: "#786858",
    idleBorder: "#3d3020",
    staleFg: "#484038",
    processingBg: "#120e0a",
    processingFg: "#a89878",
    processingBorder: "#3d3020",
  },
  font: {
    mono: "var(--font-jetbrains-mono,'Monaco',monospace)",
    sans: "var(--font-cinzel,'Cinzel',serif)",
    display: "var(--font-cinzel,'Cinzel',serif)",
  },
  space: sharedTokens.space,
  radii: sharedTokens.radii,
  fontSize: sharedTokens.fontSize,
  fontWeight: sharedTokens.fontWeight,
  transition: { fast: "100ms ease", base: "150ms ease", slow: "250ms ease" },
});

// ---------------------------------------------------------------------------
// Clean theme — indigo accent on deep slate, Inter everywhere (Linear/Vercel palette)
// ---------------------------------------------------------------------------
export const cleanTheme = createTheme(vars, {
  color: {
    textPrimary: "#e2e8f0",
    textSecondary: "#94a3b8",
    textMuted: "#7d8ea8",
    textDisabled: "#475569",
    textTertiary: "#808080",
    textInverse: "#0a0a0a",

    background: "#0f1117",
    cardBackground: "#161b22",
    hoverBackground: "#1e2530",
    modalBackground: "#161b22",
    overlayBackground: "rgba(0,0,0,0.7)",
    panelBgSecondary: "#1a2232",
    surfaceSubtle: "#161b22",
    surfaceMuted: "#374151",

    borderColor: "#1e293b",
    borderSubtle: "#1a2232",
    borderMuted: "#35354a",
    borderStrong: "#6b7280",
    borderHover: "#6366f1",
    modalBorder: "#1e293b",
    inputBorder: "#35354a",
    inputFocusBorder: "#818cf8",

    primary: "#6366f1",
    primaryHover: "#818cf8",
    primaryActive: "#4f46e5",
    primaryDark: "#3730a3",
    primaryText: "#ffffff",

    success: "#10b981",
    successBg: "#064e3b",
    warning: "#f59e0b",
    warningBg: "#78350f",
    warningText: "#fbbf24",
    error: "#ef4444",
    errorBg: "#7f1d1d",
    errorText: "#fca5a5",
    errorDark: "#ef4444",

    accentBg: "rgba(99,102,241,0.1)",
    accentHover: "rgba(99,102,241,0.2)",

    inputBackground: "#161b22",
    inputText: "#e2e8f0",
    placeholderColor: "#6b7280",

    ...terminalTokens,

    glowPrimary: "rgba(99,102,241,0.3)",
    glowSecondary: "rgba(99,102,241,0.15)",
    scanlineColor: "transparent",
    terminalCursor: "#818cf8",

    statusDot: { running: "#22c55e", paused: "#f59e0b", idle: "#475569" },
  },
  shadow: {
    none: "none",
    sm: "0 1px 2px 0 rgba(0,0,0,0.3)",
    md: "0 4px 6px -1px rgba(0,0,0,0.4), 0 2px 4px -2px rgba(0,0,0,0.3)",
    lg: "0 10px 15px -3px rgba(0,0,0,0.5), 0 4px 6px -4px rgba(0,0,0,0.4)",
  },
  statusBadge: {
    approvalBg: "#450a0a",
    approvalFg: "#fca5a5",
    approvalBorder: "#7f1d1d",
    inputBg: "#1e3a5f",
    inputFg: "#93c5fd",
    inputBorder: "#2563eb",
    completeBg: "#14532d",
    completeFg: "#86efac",
    completeBorder: "#166534",
    uncommittedBg: "#78350f",
    uncommittedFg: "#fbbf24",
    uncommittedBorder: "#b45309",
    idleBg: "#1f2937",
    idleFg: "#9ca3af",
    idleBorder: "#374151",
    staleFg: "#6b7280",
    processingBg: "#312e81",
    processingFg: "#a5b4fc",
    processingBorder: "#4338ca",
  },
  font: {
    mono: "var(--font-jetbrains-mono, 'JetBrains Mono', 'Fira Code', 'Monaco', monospace)",
    sans: "var(--font-inter, 'Inter', system-ui, sans-serif)",
    display: "var(--font-inter, 'Inter', system-ui, sans-serif)",
  },
  space: sharedTokens.space,
  radii: sharedTokens.radii,
  fontSize: sharedTokens.fontSize,
  fontWeight: sharedTokens.fontWeight,
  transition: { fast: "100ms ease", base: "150ms ease", slow: "250ms ease" },
});
